// Package tmpl 是通知的模板引擎：Go 的 text/template，加上一组受控的函数与防护。
// 用户写的模板在通知管线里执行，所以要保证它很快跑完、输出有限：源码、输出与函数的中间结果都有上限，不能定义子模板，
// range 只能遍历声明为列表的变量，整份模板按最坏情况算的执行步数与声明的变量个数有上限，
// 一次渲染里所有函数处理的字节数合计也有上限。
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
	// Reply 是 ChatOps 命令回复与 AI 助手回复的正文（助手的回答可能很长）
	Reply
)

const (
	// MaxSource 是模板源码的上限（字节）
	MaxSource = 8 << 10
	// MaxList 是一个列表变量最多的项数：事件建出来时就截到这么多，迭代预算按它算
	MaxList = 100
	// MaxRangeDepth 是 range 最多嵌套几层
	MaxRangeDepth = 2
	// MaxCost 是整份模板按最坏情况算的执行步数上限：每个节点、每条命令、每个参数各算一步，range 的循环体按 MaxList 遍算。
	// 内置函数（len、slice、and、or、not）、取字段、取变量每一步都很快，不另外计量，靠它限住总的步数。
	MaxCost = 200000
	// MaxVars 是一份模板最多声明几个变量：取变量要从最近声明的一个往回找，变量越多每一步越慢
	MaxVars = 64
)

// Limit 是这种用途的输出上限（字节）。
func (k Kind) Limit() int {
	switch k {
	case Title:
		return 1 << 10
	case Text:
		return 16 << 10
	case Body, Reply:
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
	c := &checker{tree: t.Tree, lists: lists}
	if _, err := c.cost(t.Root, 0); err != nil {
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

// checker 检查解析出来的模板：range 的对象与嵌套层数、不许 {{template}}、按最坏情况算的执行步数、声明的变量个数。
type checker struct {
	tree  *parse.Tree
	lists map[string]bool
	vars  int
}

// cost 是一段节点按最坏情况算的执行步数：每个节点算一步，加上它的管道的步数；range 是 MaxList×(1+循环体)，
// if、with 的各个分支与 range 的 else 都计入。
func (c *checker) cost(list *parse.ListNode, depth int) (int, error) {
	if list == nil {
		return 0, nil
	}
	total := 0
	for _, n := range list.Nodes {
		total++
		switch n := n.(type) {
		case *parse.ActionNode:
			s, err := c.pipe(n, n.Pipe)
			if err != nil {
				return 0, err
			}
			total += s
		case *parse.RangeNode:
			if depth+1 > MaxRangeDepth {
				return 0, invalid("%srange 最多嵌套 %d 层", at(c.tree, n), MaxRangeDepth)
			}
			if !rangesOverList(n.Pipe, c.lists) {
				return 0, invalid("%srange 只能遍历列表变量（%s），不能遍历数字、函数结果或别的变量", at(c.tree, n), listNames(c.lists))
			}
			s, err := c.pipe(n, n.Pipe)
			if err != nil {
				return 0, err
			}
			body, err := c.cost(n.List, depth+1)
			if err != nil {
				return 0, err
			}
			els, err := c.cost(n.ElseList, depth)
			if err != nil {
				return 0, err
			}
			total += s + MaxList*(1+body) + els
		case *parse.IfNode:
			s, err := c.branch(n, &n.BranchNode, depth)
			if err != nil {
				return 0, err
			}
			total += s
		case *parse.WithNode:
			s, err := c.branch(n, &n.BranchNode, depth)
			if err != nil {
				return 0, err
			}
			total += s
		case *parse.TemplateNode:
			return 0, invalid("%s不支持 template：模板里只能有一段", at(c.tree, n))
		}
		if total > MaxCost {
			return 0, invalid("%s模板太复杂：按每个列表 %d 项算，最坏要执行 %d 步以上（上限 %d）", at(c.tree, n), MaxList, total, MaxCost)
		}
	}
	return total, nil
}

func (c *checker) branch(n parse.Node, b *parse.BranchNode, depth int) (int, error) {
	s, err := c.pipe(n, b.Pipe)
	if err != nil {
		return 0, err
	}
	body, err := c.cost(b.List, depth)
	if err != nil {
		return 0, err
	}
	els, err := c.cost(b.ElseList, depth)
	if err != nil {
		return 0, err
	}
	return s + body + els, nil
}

// pipe 是一个管道的步数：每个声明、每条命令、每个参数各算一步，括号里的子管道按它自己算。同时数声明的变量。
func (c *checker) pipe(n parse.Node, p *parse.PipeNode) (int, error) {
	if p == nil {
		return 0, nil
	}
	if c.vars += len(p.Decl); c.vars > MaxVars {
		return 0, invalid("%s变量太多：一份模板最多声明 %d 个变量", at(c.tree, n), MaxVars)
	}
	steps := len(p.Decl)
	for _, cmd := range p.Cmds {
		steps++
		for _, a := range cmd.Args {
			s, err := c.arg(n, a)
			if err != nil {
				return 0, err
			}
			steps += s
		}
	}
	return steps, nil
}

func (c *checker) arg(n, a parse.Node) (int, error) {
	switch a := a.(type) {
	case *parse.PipeNode:
		return c.pipe(n, a)
	case *parse.ChainNode:
		s, err := c.arg(n, a.Node)
		return 1 + s, err
	}
	return 1, nil
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
