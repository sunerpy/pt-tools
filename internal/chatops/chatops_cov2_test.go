package chatops

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecuteBind_MissingCode drives executeBind's empty-args branch via an
// unbound user sending "/bind" with no code.
func TestExecuteBind_MissingCode(t *testing.T) {
	f := newChain(t)
	f.bindings.exists = false

	require.NoError(t, f.chain.Process(context.Background(), mkMsg("/bind")))
	assert.Equal(t, "denied:missing_code", lastResult(f.audit.snapshot()))
	reply, ok := f.replier.lastReply()
	require.True(t, ok)
	assert.Contains(t, reply.Text, "用法")
}

// TestExecuteBind_Success drives the happy path where ConsumeCode succeeds.
func TestExecuteBind_Success(t *testing.T) {
	f := newChain(t)
	f.bindings.exists = false

	require.NoError(t, f.chain.Process(context.Background(), mkMsg("/bind CODE123")))
	assert.Equal(t, "success", lastResult(f.audit.snapshot()))
	reply, ok := f.replier.lastReply()
	require.True(t, ok)
	assert.Contains(t, reply.Text, "绑定成功")
}

// TestExecuteBind_ConsumeError drives the ConsumeCode-error branch.
func TestExecuteBind_ConsumeError(t *testing.T) {
	f := newChain(t)
	f.bindings.exists = false
	f.bindCoder.err = errors.New("bad code")

	require.NoError(t, f.chain.Process(context.Background(), mkMsg("/bind BADCODE")))
	assert.Equal(t, "error:bind_failed", lastResult(f.audit.snapshot()))
	reply, ok := f.replier.lastReply()
	require.True(t, ok)
	assert.Contains(t, reply.Text, "绑定失败")
}

// TestExecuteBind_NoBindCoder covers the nil-bindCoder error return by building
// a chain with a nil BindCodeConsumer.
func TestExecuteBind_NoBindCoder(t *testing.T) {
	registry := newStubRegistry()
	bindings := &stubBindings{exists: false}
	audit := &stubAudit{}
	rl := &stubRateLimiter{allow: true}
	sessions := newStubSessions()
	replier := &stubReplier{}
	chain := NewMessageChain(registry, bindings, nil, audit, rl, sessions, replier)

	err := chain.Process(context.Background(), mkMsg("/bind CODE"))
	require.Error(t, err)
	assert.Equal(t, "error:no_binding_service", lastResult(audit.snapshot()))
}

func TestParseCommand_Branches(t *testing.T) {
	name, args := parseCommand("/status now here")
	assert.Equal(t, "status", name)
	assert.Equal(t, []string{"now", "here"}, args)

	name, _ = parseCommand("/Help@mybot")
	assert.Equal(t, "help", name, "@bot suffix must be stripped and lowercased")

	name, args = parseCommand("no-slash")
	assert.Equal(t, "", name)
	assert.Nil(t, args)

	name, _ = parseCommand("/")
	assert.Equal(t, "", name, "slash with no fields yields empty command")

	name, _ = parseCommand("/   ")
	assert.Equal(t, "", name)
}

// TestTryReply_NilReplier ensures tryReply is a safe no-op when the replier is
// nil (built directly to bypass newChain's non-nil replier).
func TestTryReply_NilReplier(t *testing.T) {
	registry := newStubRegistry(CommandSpec{Name: "status", Handler: func(context.Context, []string, Source) (Reply, error) {
		return Reply{Text: "hi"}, nil
	}})
	bindings := &stubBindings{exists: true, binding: BindingInfo{ConfID: 7, Allowed: true}}
	chain := NewMessageChain(registry, bindings, &stubBindCoder{}, &stubAudit{}, &stubRateLimiter{allow: true}, newStubSessions(), nil)

	assert.NotPanics(t, func() {
		_ = chain.Process(context.Background(), mkMsg("/status"))
	})
}

func TestNewTokenLimiter_Defaults(t *testing.T) {
	l := newTokenLimiter(RateLimitSpec{})
	require.NotNil(t, l)
	assert.Equal(t, defaultRateLimitBurst, l.Burst())

	l2 := newTokenLimiter(RateLimitSpec{Per: time.Second, Burst: 3})
	require.NotNil(t, l2)
	assert.Equal(t, 3, l2.Burst())
}

func TestGenerateBindCode_UniqueValidChars(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		code, err := GenerateBindCode()
		require.NoError(t, err)
		require.Len(t, code, 8)
		for _, c := range code {
			assert.Contains(t, bindcodeCharset, string(c), "code must only use unambiguous charset")
		}
		seen[code] = true
	}
	assert.Greater(t, len(seen), 40, "codes should be effectively unique")
}

func TestCommandRegistry_RegisterAndGetWithAliases(t *testing.T) {
	r := NewCommandRegistry()
	r.Register(CommandSpec{
		Name:    "status",
		Aliases: []string{"st", "stat"},
		Handler: func(context.Context, []string, Source) (Reply, error) { return Reply{}, nil },
	})

	spec, ok := r.Get("status")
	require.True(t, ok)
	assert.Equal(t, "status", spec.Name)

	spec, ok = r.Get("st")
	require.True(t, ok)
	assert.Equal(t, "status", spec.Name, "alias must resolve to canonical command")

	spec, ok = r.Get("/STAT")
	require.True(t, ok)
	assert.Equal(t, "status", spec.Name, "normalization strips slash and lowercases")

	_, ok = r.Get("")
	assert.False(t, ok)

	_, ok = r.Get("nonexistent")
	assert.False(t, ok)
}

func TestCommandRegistry_Register_EmptyNamePanics(t *testing.T) {
	r := NewCommandRegistry()
	assert.Panics(t, func() { r.Register(CommandSpec{Name: "   "}) })
}

func TestCommandRegistry_Register_DuplicatePanics(t *testing.T) {
	r := NewCommandRegistry()
	r.Register(CommandSpec{Name: "dup"})
	assert.Panics(t, func() { r.Register(CommandSpec{Name: "dup"}) })
}

func TestCommandRegistry_Register_NameConflictsWithAlias(t *testing.T) {
	r := NewCommandRegistry()
	r.Register(CommandSpec{Name: "a", Aliases: []string{"b"}})
	assert.Panics(t, func() { r.Register(CommandSpec{Name: "b"}) },
		"registering a command whose name equals an existing alias must panic")
}

func TestCommandRegistry_Register_AliasConflictsWithCommand(t *testing.T) {
	r := NewCommandRegistry()
	r.Register(CommandSpec{Name: "a"})
	assert.Panics(t, func() { r.Register(CommandSpec{Name: "c", Aliases: []string{"a"}}) })
}

func TestCommandRegistry_Register_AliasConflictsWithAlias(t *testing.T) {
	r := NewCommandRegistry()
	r.Register(CommandSpec{Name: "a", Aliases: []string{"x"}})
	assert.Panics(t, func() { r.Register(CommandSpec{Name: "b", Aliases: []string{"x"}}) })
}

func TestNormalizeAliases_DuplicatePanics(t *testing.T) {
	assert.Panics(t, func() {
		_ = normalizeAliases([]string{"dup", "/DUP"})
	}, "duplicate aliases (after normalization) must panic")
}

func TestNormalizeAliases_SkipsEmpty(t *testing.T) {
	got := normalizeAliases([]string{"", "  ", "keep"})
	assert.Equal(t, []string{"keep"}, got)
}

// TestSessionStore_CleanupLoopEvictsExpired starts a real SessionStore and
// verifies an expired entry is removed by the background cleanup loop.
func TestSessionStore_CleanupLoopEvictsExpired(t *testing.T) {
	s := NewSessionStore()
	t.Cleanup(s.Stop)

	// Set with a tiny TTL, wait for it to lapse, then run cleanup directly to
	// assert eviction without waiting for the long ticker period.
	s.Set("telegram", 1, "u1", SessionState{Step: "x"}, time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	s.cleanupExpired(time.Now())

	_, ok := s.Pending("telegram", 1, "u1")
	assert.False(t, ok, "expired session must be evicted")
}
