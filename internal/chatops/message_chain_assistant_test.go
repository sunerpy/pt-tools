package chatops

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubAssistant struct {
	mu      sync.Mutex
	enabled bool
	got     []InboundAssistantMessage
	reply   Reply
	result  string
}

func (s *stubAssistant) Enabled(context.Context) bool { return s.enabled }

func (s *stubAssistant) Handle(_ context.Context, in InboundAssistantMessage) (Reply, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.got = append(s.got, in)
	return s.reply, s.result
}

func boundChain(t *testing.T) *chainFixture {
	t.Helper()
	f := newChain(t)
	f.bindings.exists = true
	f.bindings.binding = BindingInfo{ID: 3, ConfID: 7, ChannelType: "telegram", ChannelUserID: "u-999", PtAdmin: true, Allowed: true}
	return f
}

// 助手启用时，绑定用户的非命令消息交给它：只记一条命令为 assistant 的审计（不带原文），立即回复照发。
func TestProcess_NonCommandGoesToAssistant(t *testing.T) {
	f := boundChain(t)
	a := &stubAssistant{enabled: true, reply: Reply{Text: "已确认，正在执行。"}, result: "confirm:approved"}
	f.chain.SetAssistant(a)

	require.NoError(t, f.chain.Process(context.Background(), mkMsg("确认")))
	require.Len(t, a.got, 1)
	assert.Equal(t, "确认", a.got[0].Msg.Text)
	assert.Equal(t, uint(3), a.got[0].Src.BindingID)
	assert.True(t, a.got[0].Src.PtAdmin)
	entries := f.audit.snapshot()
	require.Len(t, entries, 1)
	assert.Equal(t, "assistant", entries[0].Command)
	assert.Equal(t, "confirm:approved", entries[0].Result)
	assert.Empty(t, entries[0].Args, "消息原文不进审计")
	assert.Equal(t, "assistant", f.rl.lastCmd)
	reply, ok := f.replier.lastReply()
	require.True(t, ok)
	assert.Equal(t, "已确认，正在执行。", reply.Text)

	// 没有立即回复（回答之后异步发）时不发空消息
	a.reply = Reply{}
	a.result = "success"
	f.replier.replies = nil
	require.NoError(t, f.chain.Process(context.Background(), mkMsg("站点数据怎么样")))
	_, ok = f.replier.lastReply()
	assert.False(t, ok)
}

func TestProcess_AssistantDisabledKeepsHint(t *testing.T) {
	f := boundChain(t)
	a := &stubAssistant{enabled: false}
	f.chain.SetAssistant(a)
	require.NoError(t, f.chain.Process(context.Background(), mkMsg("你好")))
	assert.Empty(t, a.got)
	assert.Equal(t, "user_message_hinted", lastResult(f.audit.snapshot()))
	reply, _ := f.replier.lastReply()
	assert.Contains(t, reply.Text, "/help")

	f.chain.SetAssistant(nil)
	require.NoError(t, f.chain.Process(context.Background(), mkMsg("你好")))
	assert.Equal(t, "user_message_hinted", lastResult(f.audit.snapshot()))
}

func TestProcess_AssistantRateLimited(t *testing.T) {
	f := boundChain(t)
	f.rl.allow = false
	a := &stubAssistant{enabled: true}
	f.chain.SetAssistant(a)
	require.NoError(t, f.chain.Process(context.Background(), mkMsg("你好")))
	assert.Empty(t, a.got)
	assert.Equal(t, "denied:rate_limit", lastResult(f.audit.snapshot()))
}

// 命令照旧走命令；没绑定的用户的非命令消息不交给助手。
func TestProcess_AssistantOnlyForBoundNonCommands(t *testing.T) {
	f := boundChain(t)
	a := &stubAssistant{enabled: true}
	f.chain.SetAssistant(a)
	require.NoError(t, f.chain.Process(context.Background(), mkMsg("/unknown")))
	assert.Empty(t, a.got)
	f.bindings.exists = false
	require.NoError(t, f.chain.Process(context.Background(), mkMsg("你好")))
	assert.Empty(t, a.got)
	assert.Equal(t, "denied:not_bound", lastResult(f.audit.snapshot()))
}
