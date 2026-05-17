package chatops

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
)

type stubBindings struct {
	mu       sync.Mutex
	binding  BindingInfo
	exists   bool
	err      error
	lastChan string
	lastUser string
}

func (s *stubBindings) FindByChannelUser(_ context.Context, channelType, channelUserID string) (BindingInfo, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastChan = channelType
	s.lastUser = channelUserID
	if s.err != nil {
		return BindingInfo{}, false, s.err
	}
	return s.binding, s.exists, nil
}

type stubRegistry struct {
	specs map[string]CommandSpec
}

func newStubRegistry(specs ...CommandSpec) *stubRegistry {
	r := &stubRegistry{specs: make(map[string]CommandSpec, len(specs))}
	for _, s := range specs {
		r.specs[s.Name] = s
	}
	return r
}

func (r *stubRegistry) Get(name string) (CommandSpec, bool) { s, ok := r.specs[name]; return s, ok }
func (r *stubRegistry) List() []CommandSpec {
	out := make([]CommandSpec, 0, len(r.specs))
	for _, s := range r.specs {
		out = append(out, s)
	}
	return out
}

type stubRateLimiter struct {
	mu      sync.Mutex
	allow   bool
	calls   int
	lastCmd string
}

func (s *stubRateLimiter) Allow(_, _, command string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.lastCmd = command
	return s.allow
}

type stubSessions struct {
	mu      sync.Mutex
	pending map[string]SessionState
}

func newStubSessions() *stubSessions { return &stubSessions{pending: map[string]SessionState{}} }

func (s *stubSessions) key(channel string, _ uint, userID string) string {
	return channel + "|" + userID
}

func (s *stubSessions) Pending(channel string, confID uint, userID string) (SessionState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.pending[s.key(channel, confID, userID)]
	return st, ok
}

func (s *stubSessions) Set(channel string, confID uint, userID string, state SessionState, _ time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[s.key(channel, confID, userID)] = state
}

func (s *stubSessions) Clear(channel string, confID uint, userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, s.key(channel, confID, userID))
}

type stubReplier struct {
	mu      sync.Mutex
	replies []Reply
}

func (s *stubReplier) Reply(_ context.Context, _ notify.InboundMessage, reply Reply) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.replies = append(s.replies, reply)
	return nil
}

func (s *stubReplier) lastReply() (Reply, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.replies) == 0 {
		return Reply{}, false
	}
	return s.replies[len(s.replies)-1], true
}

type stubAudit struct {
	mu      sync.Mutex
	entries []AuditEntry
}

func (s *stubAudit) Record(_ context.Context, e AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
	return nil
}

func (s *stubAudit) snapshot() []AuditEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AuditEntry, len(s.entries))
	copy(out, s.entries)
	return out
}

type consumeArgs struct {
	code, channelType, channelUserID string
}

type stubBindCoder struct {
	mu       sync.Mutex
	consumed []consumeArgs
	err      error
}

func (s *stubBindCoder) ConsumeCode(_ context.Context, code, channelType, channelUserID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consumed = append(s.consumed, consumeArgs{code, channelType, channelUserID})
	return s.err
}

type chainFixture struct {
	chain     *MessageChain
	bindings  *stubBindings
	registry  *stubRegistry
	bindCoder *stubBindCoder
	audit     *stubAudit
	rl        *stubRateLimiter
	sessions  *stubSessions
	replier   *stubReplier
}

func newChain(t *testing.T, specs ...CommandSpec) *chainFixture {
	t.Helper()
	bindings := &stubBindings{}
	registry := newStubRegistry(specs...)
	bindCoder := &stubBindCoder{}
	audit := &stubAudit{}
	rl := &stubRateLimiter{allow: true}
	sessions := newStubSessions()
	replier := &stubReplier{}
	chain := NewMessageChain(registry, bindings, bindCoder, audit, rl, sessions, replier)
	return &chainFixture{
		chain:     chain,
		bindings:  bindings,
		registry:  registry,
		bindCoder: bindCoder,
		audit:     audit,
		rl:        rl,
		sessions:  sessions,
		replier:   replier,
	}
}

func mkMsg(text string) notify.InboundMessage {
	return notify.InboundMessage{
		ChannelType:   "telegram",
		SourceConfID:  7,
		ChannelUserID: "u-999",
		Username:      "alice",
		ChatID:        "c-1",
		Text:          text,
	}
}

func lastResult(entries []AuditEntry) string {
	if len(entries) == 0 {
		return ""
	}
	return entries[len(entries)-1].Result
}

func TestProcess_NotBound_OnlyBindAllowed(t *testing.T) {
	f := newChain(t, CommandSpec{Name: "status", Handler: func(context.Context, []string, Source) (Reply, error) {
		return Reply{Text: "ok"}, nil
	}})
	f.bindings.exists = false

	err := f.chain.Process(context.Background(), mkMsg("/status"))
	require.NoError(t, err)

	entries := f.audit.snapshot()
	require.Len(t, entries, 1)
	assert.Equal(t, "denied:not_bound", entries[0].Result)
	assert.Equal(t, "status", entries[0].Command)

	reply, ok := f.replier.lastReply()
	require.True(t, ok)
	assert.Contains(t, reply.Text, "/bind")
}

func TestProcess_NotBound_BindAllowed(t *testing.T) {
	f := newChain(t)
	f.bindings.exists = false

	err := f.chain.Process(context.Background(), mkMsg("/bind ABCD2345"))
	require.NoError(t, err)

	require.Len(t, f.bindCoder.consumed, 1)
	assert.Equal(t, "ABCD2345", f.bindCoder.consumed[0].code)
	assert.Equal(t, "telegram", f.bindCoder.consumed[0].channelType)
	assert.Equal(t, "u-999", f.bindCoder.consumed[0].channelUserID)

	entries := f.audit.snapshot()
	require.Len(t, entries, 1)
	assert.Equal(t, "success", entries[0].Result)
	assert.Equal(t, "bind", entries[0].Command)

	reply, ok := f.replier.lastReply()
	require.True(t, ok)
	assert.Contains(t, reply.Text, "绑定成功")
}

func TestProcess_RateLimited_SilentDrop(t *testing.T) {
	called := false
	f := newChain(t, CommandSpec{Name: "torrents", Handler: func(context.Context, []string, Source) (Reply, error) {
		called = true
		return Reply{Text: "list"}, nil
	}})
	f.bindings.exists = true
	f.bindings.binding = BindingInfo{ID: 1, ConfID: 7, Allowed: true, PtAdmin: false}
	f.rl.allow = false

	err := f.chain.Process(context.Background(), mkMsg("/torrents"))
	require.NoError(t, err)

	assert.False(t, called, "handler must not run when rate-limited")
	_, replied := f.replier.lastReply()
	assert.False(t, replied, "rate-limit must be silent (no reply)")

	entries := f.audit.snapshot()
	require.Len(t, entries, 1)
	assert.Equal(t, "denied:rate_limit", entries[0].Result)
}

func TestProcess_UnknownCommand(t *testing.T) {
	f := newChain(t)
	f.bindings.exists = true
	f.bindings.binding = BindingInfo{ID: 1, ConfID: 7, Allowed: true}

	err := f.chain.Process(context.Background(), mkMsg("/whatever"))
	require.NoError(t, err)

	assert.Equal(t, "denied:unknown_command", lastResult(f.audit.snapshot()))
	reply, ok := f.replier.lastReply()
	require.True(t, ok)
	assert.Contains(t, reply.Text, "/help")
}

func TestProcess_AdminRequired_Deny(t *testing.T) {
	called := false
	f := newChain(t, CommandSpec{Name: "config", AdminOnly: true, Handler: func(context.Context, []string, Source) (Reply, error) {
		called = true
		return Reply{}, nil
	}})
	f.bindings.exists = true
	f.bindings.binding = BindingInfo{ID: 1, ConfID: 7, Allowed: true, PtAdmin: false}

	err := f.chain.Process(context.Background(), mkMsg("/config show"))
	require.NoError(t, err)

	assert.False(t, called, "admin-only handler must not run for non-admin")
	assert.Equal(t, "denied:not_admin", lastResult(f.audit.snapshot()))
	reply, ok := f.replier.lastReply()
	require.True(t, ok)
	assert.Contains(t, reply.Text, "管理员")
}

func TestProcess_HappyPath_AuditSuccess(t *testing.T) {
	var captured Source
	f := newChain(t, CommandSpec{Name: "status", Handler: func(_ context.Context, _ []string, src Source) (Reply, error) {
		captured = src
		return Reply{Text: "all ok"}, nil
	}})
	f.bindings.exists = true
	f.bindings.binding = BindingInfo{ID: 9, ConfID: 7, Allowed: true, PtAdmin: true, ReplyLang: "zh"}

	err := f.chain.Process(context.Background(), mkMsg("/status"))
	require.NoError(t, err)

	entries := f.audit.snapshot()
	require.Len(t, entries, 1)
	assert.Equal(t, "success", entries[0].Result)
	assert.Equal(t, "status", entries[0].Command)

	assert.Equal(t, uint(9), captured.BindingID)
	assert.Equal(t, uint(7), captured.ChannelConfID)
	assert.True(t, captured.PtAdmin)
	assert.Equal(t, "zh", captured.ReplyLang)

	reply, ok := f.replier.lastReply()
	require.True(t, ok)
	assert.Equal(t, "all ok", reply.Text)
}

func TestProcess_NotBound_FreeText_Ignored(t *testing.T) {
	f := newChain(t)
	f.bindings.exists = false

	err := f.chain.Process(context.Background(), mkMsg("hello"))
	require.NoError(t, err)

	entries := f.audit.snapshot()
	require.Len(t, entries, 1)
	assert.Equal(t, "denied:not_bound", entries[0].Result)
}

func TestProcess_FreeText_Bound_RepliesWithHelpHint(t *testing.T) {
	f := newChain(t)
	f.bindings.exists = true
	f.bindings.binding = BindingInfo{ID: 1, ConfID: 7, Allowed: true}

	err := f.chain.Process(context.Background(), mkMsg("just chatting"))
	require.NoError(t, err)

	entries := f.audit.snapshot()
	require.Len(t, entries, 1)
	assert.Equal(t, "user_message_hinted", entries[0].Result)
	reply, replied := f.replier.lastReply()
	assert.True(t, replied)
	assert.Contains(t, reply.Text, "/help")
	assert.Contains(t, reply.Text, "命令消息")
}

func TestProcess_BindingNotAllowed(t *testing.T) {
	f := newChain(t, CommandSpec{Name: "status", Handler: func(context.Context, []string, Source) (Reply, error) {
		return Reply{Text: "ok"}, nil
	}})
	f.bindings.exists = true
	f.bindings.binding = BindingInfo{ID: 1, ConfID: 7, Allowed: false}

	err := f.chain.Process(context.Background(), mkMsg("/status"))
	require.NoError(t, err)
	assert.Equal(t, "denied:not_allowed", lastResult(f.audit.snapshot()))
}

func TestProcess_Session_DeliversReplyToPendingHandler(t *testing.T) {
	called := false
	var capturedArgs []string
	f := newChain(t)
	f.bindings.exists = true
	f.bindings.binding = BindingInfo{ID: 1, ConfID: 7, Allowed: true}
	f.sessions.Set("telegram", 7, "u-999", SessionState{
		Step: "confirm",
		Handler: func(_ context.Context, args []string, _ Source) (Reply, error) {
			called = true
			capturedArgs = args
			return Reply{Text: "session-ok"}, nil
		},
	}, time.Minute)

	err := f.chain.Process(context.Background(), mkMsg("yes please"))
	require.NoError(t, err)

	assert.True(t, called)
	assert.Equal(t, []string{"yes please"}, capturedArgs)
	assert.Equal(t, "success", lastResult(f.audit.snapshot()))
	_, stillPending := f.sessions.Pending("telegram", 7, "u-999")
	assert.False(t, stillPending, "session should be cleared after delivery")
}

func TestProcess_LookupError_BubblesUp(t *testing.T) {
	f := newChain(t)
	f.bindings.err = errors.New("db down")

	err := f.chain.Process(context.Background(), mkMsg("/status"))
	require.Error(t, err)
	assert.Equal(t, "error:lookup_binding", lastResult(f.audit.snapshot()))
}
