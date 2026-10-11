package assistant

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

type fakeReplier struct {
	mu   sync.Mutex
	sent []string
	wake chan struct{}
}

func newFakeReplier() *fakeReplier { return &fakeReplier{wake: make(chan struct{}, 64)} }

func (f *fakeReplier) Reply(_ context.Context, _ notify.InboundMessage, r chatops.Reply) error {
	f.mu.Lock()
	f.sent = append(f.sent, r.Text)
	f.mu.Unlock()
	f.wake <- struct{}{}
	return nil
}

func (f *fakeReplier) texts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sent...)
}

// waitFor 等到发出去的回复里有一条含 want。
func (f *fakeReplier) waitFor(t *testing.T, want string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		for _, s := range f.texts() {
			if strings.Contains(s, want) {
				return
			}
		}
		select {
		case <-f.wake:
		case <-deadline:
			t.Fatalf("等不到含「%s」的回复，已发：%q", want, f.texts())
		}
	}
}

func chatopsEnv(t *testing.T) (*env, *ChatOpsHandler, *fakeReplier) {
	t.Helper()
	e := newEnv(t)
	_, err := e.svc.SaveSettings(context.Background(), SettingsInput{
		Enabled: true, BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model", ChatOps: true,
	})
	require.NoError(t, err)
	rp := newFakeReplier()
	return e, NewChatOpsHandler(e.svc, rp), rp
}

func inbound(text string, admin bool) chatops.InboundAssistantMessage {
	return chatops.InboundAssistantMessage{
		Msg: notify.InboundMessage{ChannelType: "telegram", SourceConfID: 7, ChannelUserID: "u-1", ChatID: "c-1", Text: text},
		Src: chatops.Source{ChannelType: "telegram", ChannelConfID: 7, ChannelUserID: "u-1", Username: "alice", BindingID: 3, PtAdmin: admin},
	}
}

var chatopsAdmin = Actor{Owner: "chatops:3", Entry: models.AssistantEntryChatOps}

func TestChatOpsAnswersAsyncInRollingConversation(t *testing.T) {
	e, h, rp := chatopsEnv(t)
	ctx := context.Background()
	assert.True(t, h.Enabled(ctx))
	e.llm.push(reply{text: "**站点**都正常。"}, reply{text: "还在呢。"})

	r, result := h.Handle(ctx, inbound("站点怎么样", true))
	assert.Equal(t, "success", result)
	assert.Empty(t, r.Text, "不在入站路径上等回答")
	rp.waitFor(t, "站点都正常。")
	assert.NotContains(t, strings.Join(rp.texts(), "\n"), "**", "通知渠道按纯文本发")

	// 等第一个运行结束再发第二条（滚动会话：同一个会话）
	conv, ok, err := e.svc.LatestConversation(ctx, chatopsAdmin)
	require.NoError(t, err)
	require.True(t, ok)
	require.Eventually(t, func() bool { return !e.svc.isRunning(conv.ID) }, 5*time.Second, 10*time.Millisecond)
	_, result = h.Handle(ctx, inbound("还在吗", true))
	assert.Equal(t, "success", result)
	rp.waitFor(t, "还在呢。")
	again, _, _ := e.svc.LatestConversation(ctx, chatopsAdmin)
	assert.Equal(t, conv.ID, again.ID, "同一个绑定接着用同一个会话")
	second := e.llm.requests()[1].Messages
	assert.Equal(t, "站点怎么样", second[1]["content"], "历史里有上一轮")
	assert.Equal(t, models.AssistantEntryChatOps, conv.Entry)
}

func TestChatOpsBusyRepliesImmediately(t *testing.T) {
	e, h, _ := chatopsEnv(t)
	block := make(chan struct{})
	defer close(block)
	e.llm.push(reply{text: "想想", block: block})
	ctx := context.Background()
	_, result := h.Handle(ctx, inbound("第一个", true))
	require.Equal(t, "success", result)
	<-e.llm.started
	r, result := h.Handle(ctx, inbound("第二个", true))
	assert.Equal(t, "denied:busy", result)
	assert.Contains(t, r.Text, "还在回答")
}

func TestChatOpsConfirmationByReply(t *testing.T) {
	e, h, rp := chatopsEnv(t)
	e.svc.confirmUnit = time.Minute
	e.llm.push(call("c1", "pause_torrent", `{"downloader_id":1,"task_id":"abc"}`), reply{text: "暂停好了。"})
	ctx := context.Background()
	_, result := h.Handle(ctx, inbound("暂停 abc", true))
	require.Equal(t, "success", result)
	rp.waitFor(t, "需要你确认：暂停种子：下载器 1，任务 abc")
	assert.Empty(t, e.backend.writes())

	// 两条「确认」同时到：只有一条算数，操作执行一次
	var wg sync.WaitGroup
	results := make([]string, 2)
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = h.Handle(ctx, inbound("确认！", true))
		}()
	}
	wg.Wait()
	// 赢的一条批准；另一条要么撞上条件更新（已经处理过），要么到得晚、确认已经没了，当成新问题撞上进行中的运行
	approved := 0
	for _, r := range results {
		switch r {
		case "confirm:approved":
			approved++
		case "confirm:decided", "denied:busy":
		default:
			t.Errorf("意外的结果 %s", r)
		}
	}
	assert.Equal(t, 1, approved)
	rp.waitFor(t, "暂停好了。")
	assert.Len(t, e.backend.writes(), 1, "只执行一次")
	audits := e.audit.all()
	require.Len(t, audits, 1)
	assert.Equal(t, "chatops:3", audits[0].ChannelUserID)
	assert.Equal(t, "assistant", audits[0].ChannelType)
}

func TestChatOpsOtherTextDenies(t *testing.T) {
	e, h, rp := chatopsEnv(t)
	e.svc.confirmUnit = time.Minute
	e.llm.push(call("c1", "delete_torrent", `{"downloader_id":1,"task_id":"abc"}`), reply{text: "没有删。"})
	ctx := context.Background()
	h.Handle(ctx, inbound("删掉 abc", true))
	rp.waitFor(t, "需要你确认")
	r, result := h.Handle(ctx, inbound("算了，先看看下载器", true))
	assert.Equal(t, "confirm:denied", result)
	assert.Contains(t, r.Text, "已取消")
	rp.waitFor(t, "没有删。")
	assert.Empty(t, e.backend.writes())
}

func TestChatOpsTimeoutNotice(t *testing.T) {
	e, h, rp := chatopsEnv(t)
	e.svc.confirmUnit = 50 * time.Millisecond
	e.llm.push(call("c1", "pause_torrent", `{"downloader_id":1,"task_id":"abc"}`), reply{text: "没有执行。"})
	h.Handle(context.Background(), inbound("暂停 abc", true))
	rp.waitFor(t, "确认超时，没有执行：暂停种子")
	assert.Empty(t, e.backend.writes())
}

func TestChatOpsNonAdminHasNoWriteTools(t *testing.T) {
	e, h, rp := chatopsEnv(t)
	e.llm.push(call("c1", "pause_torrent", `{"downloader_id":1,"task_id":"abc"}`), reply{text: "这里不能暂停。"})
	_, result := h.Handle(context.Background(), inbound("暂停 abc", false))
	require.Equal(t, "success", result)
	rp.waitFor(t, "这里不能暂停。")
	for _, w := range writeTools {
		assert.NotContains(t, e.llm.requests()[0].toolNames(), w)
	}
	assert.Empty(t, e.backend.requests())
	for _, s := range rp.texts() {
		assert.NotContains(t, s, "需要你确认")
	}
}

func TestChatOpsNewChatAndSwitch(t *testing.T) {
	e, h, _ := chatopsEnv(t)
	ctx := context.Background()
	src := inbound("", true).Src
	require.NoError(t, h.NewChat(ctx, src))
	first, _, _ := e.svc.LatestConversation(ctx, chatopsAdmin)
	require.NoError(t, h.NewChat(ctx, src))
	second, _, _ := e.svc.LatestConversation(ctx, chatopsAdmin)
	assert.NotEqual(t, first.ID, second.ID, "/newchat 以后用新会话")

	_, err := e.svc.SaveSettings(ctx, SettingsInput{Enabled: true, BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model"})
	require.NoError(t, err)
	assert.False(t, h.Enabled(ctx), "ChatOps 开关关着")
	assert.Error(t, h.NewChat(ctx, src))
}

func TestChatOpsEventText(t *testing.T) {
	long := strings.Repeat("长", chatopsReplyRunes+10)
	got := chatopsEventText(Event{Type: EventMessage, Message: &MessageView{Role: "assistant", Content: "## 标题\n" + long}}, "")
	assert.True(t, strings.HasPrefix(got, "标题\n"))
	assert.Contains(t, got, "到网页上的 AI 助手里看完整的回答")
	assert.Empty(t, chatopsEventText(Event{Type: EventMessage, Message: &MessageView{
		Role: "assistant", Content: "我查一下", ToolCalls: []ToolCallView{{ID: "c1", Name: "list_tasks"}},
	}}, ""), "带工具调用的中间消息不发")
	assert.Empty(t, chatopsEventText(Event{Type: EventDelta, Text: "增量"}, ""))
	assert.Equal(t, "已停止回答。", chatopsEventText(Event{Type: EventMessage, Message: &MessageView{Role: "notice", Content: "已停止回答。"}}, ""))
	en := chatopsEventText(Event{Type: EventConfirmation, Confirmation: &ConfirmationView{
		Status: "pending", Summary: "x", ExpiresAt: time.Now(),
	}}, "en")
	assert.Contains(t, en, "Please confirm: x")
	assert.True(t, isYes(" OK. "))
	assert.True(t, isYes("确认！"))
	assert.False(t, isYes("确认一下这个是什么"))
}
