package notify

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/internal/notify/tmpl"
)

// TemplateSource 是一种事件的全局消息模板原文。Title、Text 空着的一项用内置文字。
type TemplateSource struct {
	EventType string
	Title     string
	Text      string
}

// Templates 是内存里的全局消息模板（按事件类型一套）：启动时从库里读入，保存或恢复默认时整体替换。并发安全。
// 发送前经 Apply 套用；模板渲染出错或结果为空时保留内置文字，通知不会因为模板写坏而发不出去。
type Templates struct {
	mu  sync.RWMutex
	set map[string]compiled

	logf   func(format string, args ...any)
	warnMu sync.Mutex
	warned map[string]time.Time
	now    func() time.Time
}

type compiled struct {
	title, text *tmpl.Template
}

// warnEvery 是同一事件的模板出错时最多多久记一条警告
const warnEvery = time.Minute

// NewTemplates 建一个空的模板集（全部用内置文字）。logf 为 nil 时不记警告。
func NewTemplates(logf func(format string, args ...any)) *Templates {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Templates{set: map[string]compiled{}, logf: logf, warned: map[string]time.Time{}, now: time.Now}
}

// Replace 整体换成 srcs。每份都按防护规则重新解析，不合格的当没有模板并记一条警告（库里的数据可能早于现在的规则）。
func (t *Templates) Replace(srcs []TemplateSource) {
	set := make(map[string]compiled, len(srcs))
	for _, s := range srcs {
		if Spec(s.EventType) == nil {
			continue
		}
		var c compiled
		if strings.TrimSpace(s.Title) != "" {
			if p, err := tmpl.Parse(s.Title, tmpl.Title, ListVars()); err == nil {
				c.title = p
			} else {
				t.logf("[消息模板] %s 的标题模板不能用，改用内置标题: %v", s.EventType, err)
			}
		}
		if strings.TrimSpace(s.Text) != "" {
			if p, err := tmpl.Parse(s.Text, tmpl.Text, ListVars()); err == nil {
				c.text = p
			} else {
				t.logf("[消息模板] %s 的正文模板不能用，改用内置正文: %v", s.EventType, err)
			}
		}
		if c.title != nil || c.text != nil {
			set[s.EventType] = c
		}
	}
	t.mu.Lock()
	t.set = set
	t.mu.Unlock()
}

// Apply 按 n.Event 的类型套用模板，重新生成标题与正文；没有事件、没有模板、渲染出错或结果为空时保留原来的。
// 按钮等其余字段原样保留。
func (t *Templates) Apply(n Notification) Notification {
	if t == nil || n.Event == nil {
		return n
	}
	t.mu.RLock()
	c, ok := t.set[n.Event.Type]
	t.mu.RUnlock()
	if !ok {
		return n
	}
	data := n.Event.Data(n.Title, n.Text)
	if c.title != nil {
		if s, err := c.title.Execute(data); err != nil {
			t.warn(n.Event.Type, "标题", err)
		} else if strings.TrimSpace(s) != "" {
			n.Title = s
		}
	}
	if c.text != nil {
		if s, err := c.text.Execute(data); err != nil {
			t.warn(n.Event.Type, "正文", err)
		} else if strings.TrimSpace(s) != "" {
			n.Text = s
		}
	}
	return n
}

func (t *Templates) warn(typ, part string, err error) {
	t.warnMu.Lock()
	now := t.now()
	last, seen := t.warned[typ]
	if !seen || now.Sub(last) >= warnEvery {
		t.warned[typ] = now
		t.warnMu.Unlock()
		t.logf("[消息模板] %s 的%s模板渲染失败，改用内置%s: %v", typ, part, part, err)
		return
	}
	t.warnMu.Unlock()
}

// RenderSample 用这种事件的示例数据渲染一份模板（变量名写错会报错），返回渲染结果：保存前的校验与预览用。
// src 为空时返回内置文字的示例。
func RenderSample(eventType string, kind tmpl.Kind, src string) (string, error) {
	ev, title, text := SampleEvent(eventType)
	if ev == nil {
		return "", fmt.Errorf("%w: 没有这种事件：%s", tmpl.ErrInvalid, eventType)
	}
	if strings.TrimSpace(src) == "" {
		if kind == tmpl.Title {
			return title, nil
		}
		return text, nil
	}
	p, err := tmpl.Parse(src, kind, ListVars())
	if err != nil {
		return "", err
	}
	out, err := p.Check(ev.Data(title, text))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("%w: 渲染结果是空的", tmpl.ErrInvalid)
	}
	return out, nil
}
