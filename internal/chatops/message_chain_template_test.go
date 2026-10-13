package chatops

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
)

// 回复发出去以前套用消息模板：处理器给了事件的按事件套；没给的按 reply.other（命令名、做成了没有）；
// 模板出错时发内置文字；按钮原样保留；没接模板时什么都不改。
func TestMessageChainAppliesReplyTemplates(t *testing.T) {
	statusEvent := notify.NewEvent(notify.EventReplyStatus, time.Now(), map[string]any{"tasks": 3, "running": 1})
	f := newChain(t,
		CommandSpec{Name: "status", Handler: func(context.Context, []string, Source) (Reply, error) {
			return Reply{Text: "状态概览", Event: statusEvent}, nil
		}},
		CommandSpec{Name: "pause", Handler: func(context.Context, []string, Source) (Reply, error) {
			return Reply{Text: "已暂停 abc", Buttons: [][]Button{{{Text: "恢复", CallbackData: "resume:abc"}}}}, nil
		}},
		CommandSpec{Name: "resume", Handler: func(context.Context, []string, Source) (Reply, error) {
			return Reply{Text: "恢复失败：不存在", Failed: true}, nil
		}},
		CommandSpec{Name: "boom", Handler: func(context.Context, []string, Source) (Reply, error) {
			return Reply{Text: "出错了"}, errors.New("boom")
		}},
		CommandSpec{Name: "admin", AdminOnly: true, Handler: func(context.Context, []string, Source) (Reply, error) {
			return Reply{Text: "x"}, nil
		}},
	)
	f.bindings.exists = true
	f.bindings.binding = BindingInfo{ID: 1, ConfID: 7, Allowed: true}
	ctx := context.Background()
	last := func() Reply {
		t.Helper()
		r, ok := f.replier.lastReply()
		require.True(t, ok)
		return r
	}

	// 没接模板：内置文字
	require.NoError(t, f.chain.Process(ctx, mkMsg("/status")))
	assert.Equal(t, "状态概览", last().Text)

	set := notify.NewTemplates(nil)
	set.Replace([]notify.TemplateSource{
		{EventType: notify.EventReplyStatus, Text: "{{.running}}/{{.tasks}} 个任务在跑"},
		{EventType: notify.EventReplyOther, Text: "{{if .ok}}✅{{else}}❌{{end}} /{{.command}} {{.text}}"},
	})
	f.chain.SetTemplates(set)

	require.NoError(t, f.chain.Process(ctx, mkMsg("/status")))
	assert.Equal(t, "1/3 个任务在跑", last().Text)

	require.NoError(t, f.chain.Process(ctx, mkMsg("/pause abc")))
	r := last()
	assert.Equal(t, "✅ /pause 已暂停 abc", r.Text)
	assert.Equal(t, [][]Button{{{Text: "恢复", CallbackData: "resume:abc"}}}, r.Buttons, "按钮原样保留")

	require.NoError(t, f.chain.Process(ctx, mkMsg("/resume abc")))
	assert.Equal(t, "❌ /resume 恢复失败：不存在", last().Text)

	require.NoError(t, f.chain.Process(ctx, mkMsg("/boom")))
	assert.Equal(t, "❌ /boom 出错了", last().Text, "处理器返回错误也算没做成")

	require.NoError(t, f.chain.Process(ctx, mkMsg("/nope")))
	assert.Equal(t, "❌ /nope 命令未知，发送 /help 查看", last().Text)

	require.NoError(t, f.chain.Process(ctx, mkMsg("/admin")))
	assert.Equal(t, "❌ /admin 需管理员权限", last().Text)

	require.NoError(t, f.chain.Process(ctx, mkMsg("随便说说")))
	assert.Contains(t, last().Text, "✅ / 🤖 我只识别命令消息", "不是命令时 command 是空")

	f.bindings.exists = false
	require.NoError(t, f.chain.Process(ctx, mkMsg("/status")))
	assert.Equal(t, "❌ /status 请先 /bind <绑定码> 完成绑定", last().Text)

	// 模板渲染出错：发内置文字
	set.Replace([]notify.TemplateSource{{EventType: notify.EventReplyOther, Text: "{{index .missing 0}}"}})
	f.bindings.exists = true
	require.NoError(t, f.chain.Process(ctx, mkMsg("/pause abc")))
	assert.Equal(t, "已暂停 abc", last().Text)
}

// 回复会话里的下一步按 session:<步骤> 套用 reply.other。
func TestMessageChainSessionReplyTemplate(t *testing.T) {
	f := newChain(t)
	f.bindings.exists = true
	f.bindings.binding = BindingInfo{ID: 1, ConfID: 7, Allowed: true}
	set := notify.NewTemplates(nil)
	set.Replace([]notify.TemplateSource{{EventType: notify.EventReplyOther, Text: "[{{.command}}] {{.text}}"}})
	f.chain.SetTemplates(set)
	f.sessions.Set("telegram", 7, "u-999", SessionState{Step: "sub_pick", Handler: func(context.Context, []string, Source) (Reply, error) {
		return Reply{Text: "已订阅"}, nil
	}}, time.Minute)

	require.NoError(t, f.chain.Process(context.Background(), mkMsg("1")))
	r, ok := f.replier.lastReply()
	require.True(t, ok)
	assert.Equal(t, "[session:sub_pick] 已订阅", r.Text)
}
