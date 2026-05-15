package telegram

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mymmrac/telego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

type capturedSend struct {
	params *telego.SendMessageParams
}

type fakeBot struct {
	mu      sync.Mutex
	sends   []capturedSend
	sendErr error
}

func (f *fakeBot) GetMe(_ context.Context) (*telego.User, error) {
	return &telego.User{ID: 1, IsBot: true, FirstName: "fake"}, nil
}

func (f *fakeBot) SendMessage(_ context.Context, p *telego.SendMessageParams) (*telego.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	f.sends = append(f.sends, capturedSend{params: p})
	return &telego.Message{MessageID: 1}, nil
}

func (f *fakeBot) lastSend() *telego.SendMessageParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sends) == 0 {
		return nil
	}
	return f.sends[len(f.sends)-1].params
}

func (f *fakeBot) sendCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sends)
}

type fakeSource struct {
	ch      chan telego.Update
	err     error
	started atomic.Bool
}

func newFakeSource() *fakeSource {
	return &fakeSource{ch: make(chan telego.Update, 8)}
}

func (s *fakeSource) source() updateSource {
	return func(ctx context.Context) (<-chan telego.Update, error) {
		s.started.Store(true)
		if s.err != nil {
			return nil, s.err
		}
		return s.ch, nil
	}
}

func (s *fakeSource) push(u telego.Update) { s.ch <- u }

func (s *fakeSource) close() { close(s.ch) }

func newChannelWithFakes(t *testing.T, cfg *Config, bot *fakeBot, src *fakeSource, plain []byte) *TelegramChannel {
	t.Helper()
	c := New()
	c.factory = func(_ *Config) (botAPI, updateSource, error) {
		return bot, src.source(), nil
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = c.Close(ctx)
	})
	_ = cfg
	_ = plain
	return c
}

func TestTelegramAdapter_Init_ValidConfig(t *testing.T) {
	bot := &fakeBot{}
	src := newFakeSource()
	plain := []byte(`{"bot_token":"123:abcdefghijklmnopqrstuvwxyzABCDEFGHIJK","allowed_users":[111],"admin_users":[111],"default_chat_id":555}`)

	c := newChannelWithFakes(t, nil, bot, src, plain)
	conf := &models.NotificationConf{ID: 7, ChannelType: ChannelType, ConfigJSON: string(plain)}

	err := c.Init(context.Background(), conf)
	require.NoError(t, err)
	require.True(t, c.Healthy())

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if src.started.Load() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.True(t, src.started.Load(), "long-poll loop did not start")

	require.NoError(t, c.Close(context.Background()))
	require.False(t, c.Healthy())
}

func TestTelegramAdapter_Init_InvalidConfig(t *testing.T) {
	cases := []struct {
		name      string
		plain     []byte
		errSubstr string
	}{
		{"bad json", []byte(`{not-json`), "解析"},
		{"missing token", []byte(`{}`), "bot_token"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bot := &fakeBot{}
			src := newFakeSource()
			c := newChannelWithFakes(t, nil, bot, src, tc.plain)
			conf := &models.NotificationConf{ID: 1, ConfigJSON: string(tc.plain)}

			err := c.Init(context.Background(), conf)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errSubstr)
			require.False(t, c.Healthy(), "Init failure must not mark adapter healthy (non-fatal)")
		})
	}
}

func TestTelegramAdapter_Init_FailNonFatal(t *testing.T) {
	c := New()
	conf := &models.NotificationConf{ID: 1, ConfigJSON: "{invalid"}

	err := c.Init(context.Background(), conf)
	require.Error(t, err)
	require.False(t, c.Healthy())

	sendErr := c.Send(context.Background(), notify.Notification{Text: "ping"})
	require.Error(t, sendErr, "Send must error when channel never initialized")
	assert.Contains(t, sendErr.Error(), "not initialized")
}

type recordingHandler struct {
	mu     sync.Mutex
	called int
	last   notify.InboundMessage
}

func (h *recordingHandler) Handle(_ context.Context, msg notify.InboundMessage) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.called++
	h.last = msg
	return nil
}

func (h *recordingHandler) callCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.called
}

func waitForSend(t *testing.T, bot *fakeBot, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if bot.sendCount() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %d sends; got %d", want, bot.sendCount())
}

func TestTelegramAdapter_PermissionGate_NonAdminCommand(t *testing.T) {
	bot := &fakeBot{}
	src := newFakeSource()
	plain := []byte(`{"bot_token":"123:abcdefghijklmnopqrstuvwxyzABCDEFGHIJK","allowed_users":[222],"admin_users":[111]}`)

	c := newChannelWithFakes(t, nil, bot, src, plain)
	handler := &recordingHandler{}
	c.OnInbound(handler.Handle)

	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ID: 9, ConfigJSON: string(plain)}))

	src.push(telego.Update{
		UpdateID: 1,
		Message: &telego.Message{
			MessageID: 10,
			From:      &telego.User{ID: 222, Username: "alice"},
			Chat:      telego.Chat{ID: 222, Type: "private"},
			Text:      "/pause abc",
		},
	})

	waitForSend(t, bot, 1)

	last := bot.lastSend()
	require.NotNil(t, last)
	assert.Empty(t, last.ParseMode, "deny reply must be plain text (no parse_mode)")
	assert.Contains(t, last.Text, "管理员")
	assert.Equal(t, 0, handler.callCount(), "handler must NOT be invoked when permission denied")
}

func TestTelegramAdapter_PermissionGate_NotInWhitelist(t *testing.T) {
	bot := &fakeBot{}
	src := newFakeSource()
	plain := []byte(`{"bot_token":"123:abcdefghijklmnopqrstuvwxyzABCDEFGHIJK","allowed_users":[222],"admin_users":[111]}`)

	c := newChannelWithFakes(t, nil, bot, src, plain)
	handler := &recordingHandler{}
	c.OnInbound(handler.Handle)

	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ID: 9, ConfigJSON: string(plain)}))

	src.push(telego.Update{
		UpdateID: 2,
		Message: &telego.Message{
			MessageID: 11,
			From:      &telego.User{ID: 999, Username: "stranger"},
			Chat:      telego.Chat{ID: 999, Type: "private"},
			Text:      "hello",
		},
	})

	waitForSend(t, bot, 1)
	assert.Equal(t, 0, handler.callCount())
	assert.Contains(t, bot.lastSend().Text, "权限")
}

func TestTelegramAdapter_PermissionGate_AdminAllowed(t *testing.T) {
	bot := &fakeBot{}
	src := newFakeSource()
	plain := []byte(`{"bot_token":"123:abcdefghijklmnopqrstuvwxyzABCDEFGHIJK","allowed_users":[111],"admin_users":[111]}`)

	c := newChannelWithFakes(t, nil, bot, src, plain)
	handler := &recordingHandler{}
	c.OnInbound(handler.Handle)

	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ID: 9, ConfigJSON: string(plain)}))

	src.push(telego.Update{
		UpdateID: 3,
		Message: &telego.Message{
			MessageID: 12,
			From:      &telego.User{ID: 111, Username: "admin"},
			Chat:      telego.Chat{ID: 111, Type: "private"},
			Text:      "/sites",
		},
	})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if handler.callCount() == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.Equal(t, 1, handler.callCount())
	assert.Equal(t, "111", handler.last.ChannelUserID)
	assert.Equal(t, "/sites", handler.last.Text)
	assert.Equal(t, ChannelType, handler.last.ChannelType)
	assert.Equal(t, uint(9), handler.last.SourceConfID)
	assert.Equal(t, 0, bot.sendCount(), "no auto-reply should be sent for permitted command")
}

func TestTelegramAdapter_Send_PlainText(t *testing.T) {
	bot := &fakeBot{}
	src := newFakeSource()
	plain := []byte(`{"bot_token":"123:abcdefghijklmnopqrstuvwxyzABCDEFGHIJK","default_chat_id":555}`)

	c := newChannelWithFakes(t, nil, bot, src, plain)
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ID: 1, ConfigJSON: string(plain)}))

	n := notify.Notification{
		Title: "[FREE] Movie_Title (2024).mkv",
		Text:  "Size: 1.2 GB | _internal_",
	}
	require.NoError(t, c.Send(context.Background(), n))

	last := bot.lastSend()
	require.NotNil(t, last)
	assert.Empty(t, last.ParseMode, "default Send must NOT set parse_mode")
	assert.True(t, strings.Contains(last.Text, "[FREE] Movie_Title (2024).mkv"),
		"raw title must be preserved verbatim, got %q", last.Text)
	assert.True(t, strings.Contains(last.Text, "_internal_"))
	assert.Equal(t, int64(555), last.ChatID.ID)
}

func TestTelegramAdapter_Send_HTMLForSystem(t *testing.T) {
	bot := &fakeBot{}
	src := newFakeSource()
	plain := []byte(`{"bot_token":"123:abcdefghijklmnopqrstuvwxyzABCDEFGHIJK","default_chat_id":555}`)

	c := newChannelWithFakes(t, nil, bot, src, plain)
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ID: 1, ConfigJSON: string(plain)}))

	n := notify.Notification{
		Title:   "System Alert",
		Text:    "Disk space low",
		Targets: map[string]string{"parse_mode": "HTML"},
	}
	require.NoError(t, c.Send(context.Background(), n))

	last := bot.lastSend()
	require.NotNil(t, last)
	assert.Equal(t, "HTML", last.ParseMode)
	assert.Contains(t, last.Text, "<b>System Alert</b>")
}

func TestTelegramAdapter_Send_TargetsChatID(t *testing.T) {
	bot := &fakeBot{}
	src := newFakeSource()
	plain := []byte(`{"bot_token":"123:abcdefghijklmnopqrstuvwxyzABCDEFGHIJK","default_chat_id":555}`)

	c := newChannelWithFakes(t, nil, bot, src, plain)
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ID: 1, ConfigJSON: string(plain)}))

	n := notify.Notification{
		Title:   "Hi",
		Text:    "ping",
		Targets: map[string]string{"chat_id": "777"},
	}
	require.NoError(t, c.Send(context.Background(), n))

	last := bot.lastSend()
	require.NotNil(t, last)
	assert.Equal(t, int64(777), last.ChatID.ID, "Targets.chat_id must override default_chat_id")
}

func TestTelegramAdapter_Send_NoChatID(t *testing.T) {
	bot := &fakeBot{}
	src := newFakeSource()
	plain := []byte(`{"bot_token":"123:abcdefghijklmnopqrstuvwxyzABCDEFGHIJK"}`)

	c := newChannelWithFakes(t, nil, bot, src, plain)
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ID: 1, ConfigJSON: string(plain)}))

	err := c.Send(context.Background(), notify.Notification{Text: "ping"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default_chat_id")
}

func TestTelegramAdapter_RegisteredInDefaultRegistry(t *testing.T) {
	ch, err := notify.DefaultRegistry().Make(ChannelType)
	require.NoError(t, err)
	require.NotNil(t, ch)
	assert.Equal(t, ChannelType, ch.Type())
	assert.True(t, ch.SupportsInbound())
}

func TestEscapeMarkdownV2(t *testing.T) {
	in := "Movie_Title (2024).mkv"
	out := EscapeMarkdownV2(in)
	for _, ch := range []string{"_", "(", ")", "."} {
		assert.Contains(t, out, "\\"+ch)
	}
	assert.NotEqual(t, in, out)
}
