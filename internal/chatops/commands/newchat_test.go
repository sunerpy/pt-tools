package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/chatops"
)

type stubNewChat struct {
	calls int
	err   error
}

func (s *stubNewChat) NewChat(context.Context, chatops.Source) error {
	s.calls++
	return s.err
}

func TestNewChatCommand(t *testing.T) {
	t.Cleanup(func() { SetAssistant(nil) })
	src := chatops.Source{BindingID: 3}
	SetAssistant(nil)
	r, err := newchatHandler(context.Background(), nil, src)
	require.NoError(t, err)
	assert.True(t, r.Failed)
	assert.Contains(t, r.Text, "不可用")

	s := &stubNewChat{}
	SetAssistant(s)
	r, err = newchatHandler(context.Background(), nil, src)
	require.NoError(t, err)
	assert.False(t, r.Failed)
	assert.Contains(t, r.Text, "已开新对话")
	assert.Equal(t, 1, s.calls)

	s.err = errors.New("AI 助手没有启用")
	r, _ = newchatHandler(context.Background(), nil, src)
	assert.True(t, r.Failed)
	assert.Contains(t, r.Text, "没有启用")

	spec, ok := chatops.DefaultRegistry().Get("newchat")
	require.True(t, ok)
	assert.False(t, spec.AdminOnly, "普通绑定也能开新对话")
}
