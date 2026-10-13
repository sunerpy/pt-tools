// Package tmpl 是通知的模板引擎：Go 的 text/template，加上一组受控的函数与防护。
// 用户写的模板在通知管线里执行，所以要保证它跑得完、输出有限：源码、输出与函数的中间结果都有上限，不能定义子模板，
// range 只能遍历声明为列表的变量，整份模板按最坏情况算的迭代次数也有上限，一次渲染里所有函数处理的字节数合计也有上限。
package tmpl

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"text/template"
	"text/template/parse"
)

// Kind 是模板的用途，决定输出上限。
type Kind int

const (
	// Title 是消息标题
	Title Kind = iota
	// Text 是消息正文
	Text
	// Body 是 Webhook 的请求体
	Body
	// URL 是 Webhook 的地址
	URL
)

const (
	// MaxSource 是模板源码的上限（字节）
	MaxSource = 8 << 10
	// MaxList 是一个列表变量最多的项数：事件建出来时就截到这么多，迭代预算按它算
	MaxList = 100
	// MaxRangeDepth 是 range 最多嵌套几层
	MaxRangeDepth = 2
	// MaxCost 是整份模板按最坏情况算的迭代次数上限
	MaxCost = 20000
)

// Limit 是这种用途的输出上限（字节）。
func (k Kind) Limit() int {
	switch k {
	case Title:
		return 1 << 10
	case Text:
		return 16 << 10
	case Body:
		return 64 << 10
	default:
		return 4 << 10
	}
}

// ErrInvalid 包着所有「模板写得不对」的错误（解析、防护、渲染）：接口据此回 400，发送时据此退回内置文字。
var ErrInvalid = errors.New("模板不对")

type invalidError struct{ msg string }

func (e *invalidError) Error() string { return e.msg }
func (e *invalidError) Unwrap() error { return ErrInvalid }

func invalid(format string, args ...any) error {
	return &invalidError{msg: fmt.Sprintf(format, args...)}
}

// Template 是解析好、通过了防护检查的模板。
type Template struct {
	t    *template.Template
	kind Kind
}

// Parse 解析模板并做结构检查。lists 是可以 range 的列表变量名（事件目录里声明为列表的变量）。
func Parse(src string, kind Kind, lists map[string]bool) (*Template, error) {
	if strings.TrimSpace(src) == "" {
		return nil, invalid("模板是空的")
	}
	if len(src) > MaxSource {
		return nil, invalid("模板太长：最多 %d 字节", MaxSource)
	}
	t, err := template.New(name).Funcs(funcs(nil)).Option("missingkey=zero").Parse(src)
	if err != nil {
		return nil, invalid("%s", cleanErr(err))
	}
	if len(t.Templates()) != 1 || t.Tree == nil {
		return nil, invalid("不支持 define、block：模板里只能有一段")
	}
	if _, err := cost(t.Tree, t.Root, 0, lists); err != nil {
		return nil, err
	}
	return &Template{t: t, kind: kind}, nil
}

// name 是模板在错误信息里的名字（cleanErr 按它认出位置）
const name = "模板"

// Execute 渲染模板：变量缺了按零值处理（数据里已经放好所有事件的全部变量，正常不会缺）。
func (t *Template) Execute(data any) (string, error) {
	return t.exec(data, false)
}

// Check 渲染一遍，变量名写错时报错：保存与预览用（示例数据里有全部变量）。
func (t *Template) Check(data any) (string, error) {
	return t.exec(data, true)
}

// exec 每次都在模板的副本上绑定一份新的函数：函数共用这一次渲染的工作量预算。副本共用解析树，同一个模板可以同时渲染。
func (t *Template) exec(data any, strict bool) (string, error) {
	x, err := t.t.Clone()
	if err != nil {
		return "", invalid("%s", cleanErr(err))
	}
	x.Funcs(funcs(newBudget()))
	if strict {
		x.Option("missingkey=error")
	}
	w := &capWriter{max: t.kind.Limit()}
	if err := x.Execute(w, data); err != nil {
		if errors.Is(err, errTooLong) {
			return "", invalid("渲染结果太长：最多 %d 字节", t.kind.Limit())
		}
		return "", invalid("%s", cleanErr(err))
	}
	return w.b.String(), nil
}

var errTooLong = errors.New("output too long")

// capWriter 超过上限就报错，text/template 随之停止渲染。
type capWriter struct {
	b   strings.Builder
	max int
}

func (w *capWriter) Write(p []byte) (int, error) {
	if w.b.Len()+len(p) > w.max {
		return 0, errTooLong
	}
	return w.b.Write(p)
}

// cost 是一段节点按最坏情况算的迭代次数：每个 range 按 MaxList 项计，成本是 MaxList×(1+循环体)，
// if、with 的各个分支与 range 的 else 都计入。同时检查 range 的对象与嵌套层数、拒绝 {{template}}。
func cost(tree *parse.Tree, list *parse.ListNode, depth int, lists map[string]bool) (int, error) {
	if list == nil {
		return 0, nil
	}
	total := 0
	for _, n := range list.Nodes {
		switch n := n.(type) {
		case *parse.RangeNode:
			if depth+1 > MaxRangeDepth {
				return 0, invalid("%srange 最多嵌套 %d 层", at(tree, n), MaxRangeDepth)
			}
			if !rangesOverList(n.Pipe, lists) {
				return 0, invalid("%srange 只能遍历列表变量（%s），不能遍历数字、函数结果或别的变量", at(tree, n), listNames(lists))
			}
			body, err := cost(tree, n.List, depth+1, lists)
			if err != nil {
				return 0, err
			}
			els, err := cost(tree, n.ElseList, depth, lists)
			if err != nil {
				return 0, err
			}
			total += MaxList*(1+body) + els
		case *parse.IfNode:
			c, err := branchCost(tree, &n.BranchNode, depth, lists)
			if err != nil {
				return 0, err
			}
			total += c
		case *parse.WithNode:
			c, err := branchCost(tree, &n.BranchNode, depth, lists)
			if err != nil {
				return 0, err
			}
			total += c
		case *parse.TemplateNode:
			return 0, invalid("%s不支持 template：模板里只能有一段", at(tree, n))
		}
		if total > MaxCost {
			return 0, invalid("%s循环太多：按每个列表 %d 项算，最坏要迭代 %d 次以上（上限 %d）", at(tree, n), MaxList, total, MaxCost)
		}
	}
	return total, nil
}

func branchCost(tree *parse.Tree, b *parse.BranchNode, depth int, lists map[string]bool) (int, error) {
	body, err := cost(tree, b.List, depth, lists)
	if err != nil {
		return 0, err
	}
	els, err := cost(tree, b.ElseList, depth, lists)
	if err != nil {
		return 0, err
	}
	return body + els, nil
}

// rangesOverList 报告 range 的管道是不是恰好一个以列表变量结尾的字段链（.items、$.items、$x.items）。
// 数字（Go 1.22 起 range 能直接遍历整数）、变量本身、函数结果（包括 len）都不行。
func rangesOverList(pipe *parse.PipeNode, lists map[string]bool) bool {
	if pipe == nil || len(pipe.Cmds) != 1 || len(pipe.Cmds[0].Args) != 1 {
		return false
	}
	var ident []string
	switch a := pipe.Cmds[0].Args[0].(type) {
	case *parse.FieldNode:
		ident = a.Ident
	case *parse.VariableNode:
		if len(a.Ident) < 2 {
			return false
		}
		ident = a.Ident
	default:
		return false
	}
	return len(ident) > 0 && lists[ident[len(ident)-1]]
}

func listNames(lists map[string]bool) string {
	names := make([]string, 0, len(lists))
	for k := range lists {
		names = append(names, "."+k)
	}
	slices.Sort(names)
	if len(names) == 0 {
		return "这里没有列表变量"
	}
	return strings.Join(names, "、")
}

// at 是节点在源码里的位置，写成「第 N 行：」。
func at(tree *parse.Tree, n parse.Node) string {
	loc, _ := tree.ErrorContext(n)
	if m := posRe.FindStringSubmatch(loc); m != nil {
		return "第 " + m[1] + " 行："
	}
	return ""
}

var (
	posRe     = regexp.MustCompile(`^` + name + `:(\d+)`)
	errPosRe  = regexp.MustCompile(`^template: ` + name + `:(\d+)(?::\d+)?: (.*)$`)
	noKeyRe   = regexp.MustCompile(`map has no entry for key "([^"]*)"`)
	noFuncRe  = regexp.MustCompile(`function "([^"]*)" not defined`)
	executeRe = regexp.MustCompile(`^executing "` + name + `" at <([^>]*)>: `)
)

// cleanErr 把 text/template 的错误写成给人看的一句：位置换成「第 N 行」，常见的几种换成中文。
func cleanErr(err error) string {
	msg := err.Error()
	line := ""
	if m := errPosRe.FindStringSubmatch(msg); m != nil {
		line, msg = "第 "+m[1]+" 行：", m[2]
	} else {
		msg = strings.TrimPrefix(msg, "template: ")
	}
	where := ""
	if m := executeRe.FindStringSubmatch(msg); m != nil {
		where, msg = m[1], msg[len(m[0]):]
	}
	switch {
	case noKeyRe.MatchString(msg):
		return line + "没有这个变量：" + noKeyRe.FindStringSubmatch(msg)[1]
	case noFuncRe.MatchString(msg):
		return line + "没有这个函数：" + noFuncRe.FindStringSubmatch(msg)[1]
	case where != "":
		return line + "在 " + where + "：" + msg
	}
	return line + msg
}
