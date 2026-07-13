package telegram

import (
	"context"
	"errors"
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

type capturedEditMarkup struct {
	params *telego.EditMessageReplyMarkupParams
}

type fakeBot struct {
	mu          sync.Mutex
	sends       []capturedSend
	sendErr     error
	answers     []*telego.AnswerCallbackQueryParams
	answerErr   error
	editMarkups []capturedEditMarkup
	editErr     error
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

func (f *fakeBot) AnswerCallbackQuery(_ context.Context, p *telego.AnswerCallbackQueryParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answers = append(f.answers, p)
	return f.answerErr
}

func (f *fakeBot) EditMessageReplyMarkup(_ context.Context, p *telego.EditMessageReplyMarkupParams) (*telego.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.editErr != nil {
		return nil, f.editErr
	}
	f.editMarkups = append(f.editMarkups, capturedEditMarkup{params: p})
	return &telego.Message{MessageID: p.MessageID}, nil
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

func (f *fakeBot) lastAnswer() *telego.AnswerCallbackQueryParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.answers) == 0 {
		return nil
	}
	return f.answers[len(f.answers)-1]
}

func (f *fakeBot) editMarkupCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.editMarkups)
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

func TestTelegramAdapter_PermissionGate_AllowedUserCanSendCommands(t *testing.T) {
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
			Text:      "/help",
		},
	})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if handler.callCount() == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.Equal(t, 1, handler.callCount(), "allowed_users user must reach inbound handler for any command (admin-only check happens in chain layer)")
	assert.Equal(t, "/help", handler.last.Text)
	assert.Equal(t, "222", handler.last.ChannelUserID)
	assert.Equal(t, 0, bot.sendCount(), "adapter must not auto-reply when user is on whitelist")
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

func TestDefaultBotFactory_ProxyURL(t *testing.T) {
	const validToken = "123:abcdefghijklmnopqrstuvwxyzABCDEFGHI"
	cases := []struct {
		name     string
		proxyURL string
		wantErr  bool
		errSub   string
	}{
		{"empty falls back to env", "", false, ""},
		{"valid http proxy", "http://127.0.0.1:1080", false, ""},
		{"valid socks5 proxy", "socks5://user:pass@127.0.0.1:1080", false, ""},
		{"invalid proxy scheme parse", "://bad-url", true, "proxy_url"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				BotToken: validToken,
				ProxyURL: tc.proxyURL,
			}
			bot, src, err := defaultBotFactory(cfg)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errSub)
				assert.Nil(t, bot)
				assert.Nil(t, src)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, bot)
			assert.NotNil(t, src)
		})
	}
}

// TestBuildInlineKeyboard_AllBranches exercises every filtering rule in
// buildInlineKeyboard: empty input, skipped empty rows, buttons with empty
// text, URL vs callback precedence, buttons with neither URL nor callback, and
// the all-empty-collapses-to-nil case.
func TestBuildInlineKeyboard_AllBranches(t *testing.T) {
	assert.Nil(t, buildInlineKeyboard(nil))
	assert.Nil(t, buildInlineKeyboard([][]notify.Button{}))

	// A row that is entirely empty is skipped; a row whose buttons are all
	// invalid produces no output row → overall nil.
	assert.Nil(t, buildInlineKeyboard([][]notify.Button{
		{},
		{{Text: ""}},  // empty text → skipped
		{{Text: "x"}}, // no URL and no callback → skipped
	}))

	markup := buildInlineKeyboard([][]notify.Button{
		{
			{Text: "开", CallbackData: "dl:1"},
			{Text: "链接", URL: "https://e", CallbackData: "ignored"}, // URL wins
		},
		{}, // skipped
		{
			{Text: ""}, // skipped
			{Text: "仅回调", CallbackData: "ig:2"},
		},
	})
	require.NotNil(t, markup)
	require.Len(t, markup.InlineKeyboard, 2)

	row0 := markup.InlineKeyboard[0]
	require.Len(t, row0, 2)
	assert.Equal(t, "开", row0[0].Text)
	assert.Equal(t, "dl:1", row0[0].CallbackData)
	assert.Equal(t, "https://e", row0[1].URL)
	assert.Empty(t, row0[1].CallbackData, "URL button must not carry callback data")

	row1 := markup.InlineKeyboard[1]
	require.Len(t, row1, 1)
	assert.Equal(t, "ig:2", row1[0].CallbackData)
}

// TestComposeText_AllBranches covers the four combinations of Title/Text and
// the HTML bold wrapping.
func TestComposeText_AllBranches(t *testing.T) {
	assert.Equal(t, "T\nB", composeText(notify.Notification{Title: "T", Text: "B"}, ""))
	assert.Equal(t, "<b>T</b>\nB", composeText(notify.Notification{Title: "T", Text: "B"}, "HTML"))
	assert.Equal(t, "OnlyTitle", composeText(notify.Notification{Title: "OnlyTitle"}, ""))
	assert.Equal(t, "OnlyText", composeText(notify.Notification{Text: "OnlyText"}, ""))
}

// TestTelegram_Send_WithButtons verifies Send attaches an inline keyboard and
// disables web preview when requested.
func TestTelegram_Send_WithButtons(t *testing.T) {
	bot := &fakeBot{}
	src := newFakeSource()
	plain := []byte(`{"bot_token":"123:abcdefghijklmnopqrstuvwxyzABCDEFGHIJK","default_chat_id":555}`)
	c := newChannelWithFakes(t, nil, bot, src, plain)
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ID: 1, ConfigJSON: string(plain)}))

	n := notify.Notification{
		Title:             "Has buttons",
		Text:              "body",
		DisableWebPreview: true,
		Buttons: [][]notify.Button{
			{{Text: "下载", CallbackData: "dl:5"}},
		},
	}
	require.NoError(t, c.Send(context.Background(), n))

	last := bot.lastSend()
	require.NotNil(t, last)
	require.NotNil(t, last.ReplyMarkup)
	require.NotNil(t, last.LinkPreviewOptions)
	assert.True(t, last.LinkPreviewOptions.IsDisabled)
}

// TestTelegram_Send_Uninitialized covers the guard when the channel has no bot.
func TestTelegram_Send_Uninitialized(t *testing.T) {
	c := New()
	c.logger = sLogger()
	err := c.Send(context.Background(), notify.Notification{Title: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

// TestTelegram_Send_BotError verifies a bot SendMessage failure surfaces.
func TestTelegram_Send_BotError(t *testing.T) {
	bot := &fakeBot{sendErr: errors.New("boom")}
	src := newFakeSource()
	plain := []byte(`{"bot_token":"123:abcdefghijklmnopqrstuvwxyzABCDEFGHIJK","default_chat_id":555}`)
	c := newChannelWithFakes(t, nil, bot, src, plain)
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ID: 1, ConfigJSON: string(plain)}))

	err := c.Send(context.Background(), notify.Notification{Title: "x", Text: "y"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SendMessage")
}

// TestTelegram_HandleUpdate_Guards covers the early-return branches of
// handleUpdate: nil message, nil From, and empty text.
func TestTelegram_HandleUpdate_Guards(t *testing.T) {
	bot := &fakeBot{}
	src := newFakeSource()
	plain := []byte(`{"bot_token":"123:abcdefghijklmnopqrstuvwxyzABCDEFGHIJK","allowed_users":[111],"default_chat_id":555}`)
	c := newChannelWithFakes(t, nil, bot, src, plain)
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ID: 1, ConfigJSON: string(plain)}))

	// nil message and nil callback → nothing happens.
	c.handleUpdate(context.Background(), telego.Update{})
	// message with nil From.
	c.handleUpdate(context.Background(), telego.Update{Message: &telego.Message{Text: "hi"}})
	// message with blank text.
	c.handleUpdate(context.Background(), telego.Update{Message: &telego.Message{
		From: &telego.User{ID: 111}, Chat: telego.Chat{ID: 555}, Text: "   ",
	}})

	assert.Equal(t, 0, bot.sendCount(), "guard branches must not send anything")
}

// TestTelegram_ReplyDenied_Path drives handleUpdate with a non-whitelisted user
// so replyDenied issues the denial message via the bot.
func TestTelegram_ReplyDenied_Path(t *testing.T) {
	bot := &fakeBot{}
	src := newFakeSource()
	plain := []byte(`{"bot_token":"123:abcdefghijklmnopqrstuvwxyzABCDEFGHIJK","allowed_users":[111],"default_chat_id":555}`)
	c := newChannelWithFakes(t, nil, bot, src, plain)
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ID: 1, ConfigJSON: string(plain)}))

	c.handleUpdate(context.Background(), telego.Update{Message: &telego.Message{
		From: &telego.User{ID: 99999}, Chat: telego.Chat{ID: 12345}, Text: "/help",
	}})

	last := bot.lastSend()
	require.NotNil(t, last)
	assert.Equal(t, denyMessage, last.Text)
	assert.Equal(t, int64(12345), last.ChatID.ID)
}

// TestTelegram_RunInbound_SourceError covers the long-poll start failure path:
// the source returns an error, runInbound marks the channel unhealthy and
// closes the done channel.
func TestTelegram_RunInbound_SourceError(t *testing.T) {
	c := New()
	c.logger = sLogger()
	c.pollDone = make(chan struct{})
	failing := func(_ context.Context) (<-chan telego.Update, error) {
		return nil, errors.New("poll boom")
	}

	go c.runInbound(context.Background(), failing)

	select {
	case <-c.pollDone:
	case <-time.After(time.Second):
		t.Fatal("runInbound never finished after source error")
	}
	assert.False(t, c.Healthy())
}

// TestTelegram_RunInbound_NilSource covers the nil-source early return.
func TestTelegram_RunInbound_NilSource(t *testing.T) {
	c := New()
	c.logger = sLogger()
	c.pollDone = make(chan struct{})
	go c.runInbound(context.Background(), nil)
	select {
	case <-c.pollDone:
	case <-time.After(time.Second):
		t.Fatal("runInbound with nil source never finished")
	}
}

// TestTelegram_RunInbound_ChannelClosed drives runInbound over a real source
// channel directly (no Init, to avoid a second poll goroutine): one update is
// handled, then closing the channel ends the loop and closes pollDone.
func TestTelegram_RunInbound_ChannelClosed(t *testing.T) {
	src := newFakeSource()
	c := New()
	c.logger = sLogger()
	c.cfg = &Config{}
	c.pollDone = make(chan struct{})

	go c.runInbound(context.Background(), src.source())
	src.push(telego.Update{Message: &telego.Message{
		From: &telego.User{ID: 111}, Chat: telego.Chat{ID: 555}, Text: "/ping",
	}})
	src.close()

	select {
	case <-c.pollDone:
	case <-time.After(2 * time.Second):
		t.Fatal("runInbound did not exit after channel close")
	}
}

// TestTelegram_HandleCallback_UnknownVerb covers dispatchCallbackAction's
// default and "dt" branches without an action handler.
func TestTelegram_HandleCallback_MiscVerbs(t *testing.T) {
	logger := sLogger()
	assert.Equal(t, "未知操作", dispatchCallbackAction(context.Background(), nil, "??", "", 1, logger, 1))
	assert.Equal(t, "请查看消息中的链接", dispatchCallbackAction(context.Background(), nil, "dt", "", 1, logger, 1))
	assert.Equal(t, "已记录下载请求 #7（处理中）", dispatchCallbackAction(context.Background(), nil, "dl", "7", 1, logger, 1))
	assert.Equal(t, "已忽略 #8", dispatchCallbackAction(context.Background(), nil, "ig", "8", 1, logger, 1))
}
