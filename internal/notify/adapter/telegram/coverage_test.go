package telegram

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mymmrac/telego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

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
