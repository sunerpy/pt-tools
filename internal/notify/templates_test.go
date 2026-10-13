package notify

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type logs struct {
	mu   sync.Mutex
	msgs []string
}

func (l *logs) logf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.msgs = append(l.msgs, fmt.Sprintf(format, args...))
}

func (l *logs) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.msgs)
}

func matchedNotification() Notification {
	ev, title, text := SampleEvent(EventRSSMatched)
	return Notification{
		Event: ev, Title: title, Text: text, SourceConfID: 3,
		Buttons: [][]Button{{{Text: "立即下载", CallbackData: "dl:1"}}},
	}
}

func TestTemplatesApply(t *testing.T) {
	l := &logs{}
	ts := NewTemplates(l.logf)
	n := matchedNotification()

	// 没有模板、没有事件：原样
	assert.Equal(t, n, ts.Apply(n))
	plain := Notification{Title: "t", Text: "x"}
	assert.Equal(t, plain, ts.Apply(plain))
	var nilSet *Templates
	assert.Equal(t, n, nilSet.Apply(n))

	ts.Replace([]TemplateSource{{
		EventType: EventRSSMatched,
		Title:     "[{{.site}}] {{.torrent_title}}",
		Text:      "{{.size}}{{if .free}} 免费，剩 {{.free_remaining}}{{end}}\n原文：{{.title}}",
	}})
	got := ts.Apply(n)
	assert.Equal(t, "[hdtime] Fake.Movie.2026.1080p.WEB-DL.H.264-FAKE", got.Title)
	assert.Equal(t, "15.00 GB 免费，剩 23h59min\n原文："+n.Title, got.Text)
	assert.Equal(t, n.Buttons, got.Buttons, "按钮原样保留")
	assert.Equal(t, n.SourceConfID, got.SourceConfID)

	// 只配了正文：标题用内置的
	ts.Replace([]TemplateSource{{EventType: EventRSSMatched, Text: "{{.site}}"}})
	got = ts.Apply(n)
	assert.Equal(t, n.Title, got.Title)
	assert.Equal(t, "hdtime", got.Text)

	// 别的事件不受影响
	other, otitle, otext := SampleEvent(EventTest)
	on := Notification{Event: other, Title: otitle, Text: otext}
	assert.Equal(t, on, ts.Apply(on))
}

func TestTemplatesFallBackToBuiltin(t *testing.T) {
	l := &logs{}
	ts := NewTemplates(l.logf)
	now := time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)
	ts.now = func() time.Time { return now }
	n := matchedNotification()

	// 渲染出错（比较不了的类型）与结果为空：保留内置文字，同一事件每分钟最多记一条警告
	ts.Replace([]TemplateSource{{EventType: EventRSSMatched, Title: `{{if eq .site 1}}x{{end}}`, Text: `{{if false}}x{{end}}`}})
	got := ts.Apply(n)
	assert.Equal(t, n.Title, got.Title)
	assert.Equal(t, n.Text, got.Text)
	ts.Apply(n)
	assert.Equal(t, 1, l.count())
	now = now.Add(warnEvery)
	ts.Apply(n)
	assert.Equal(t, 2, l.count())

	// 库里存着按现在的规则不合格的模板（range 一个数字）：当没有模板，记一条警告
	l2 := &logs{}
	ts = NewTemplates(l2.logf)
	ts.Replace([]TemplateSource{{EventType: EventRSSMatched, Text: `{{range .size_bytes}}x{{end}}`}, {EventType: "nope", Text: "{{.x}}"}})
	assert.Equal(t, n, ts.Apply(n))
	require.Equal(t, 1, l2.count())
	assert.Contains(t, l2.msgs[0], "range 只能遍历列表变量")
}
