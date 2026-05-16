package telegram

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/mymmrac/telego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

type recordedCall struct {
	verb   string
	logID  uint
	userID int64
}

type stubAction struct {
	mu       sync.Mutex
	calls    []recordedCall
	dlErr    error
	ignErr   error
	dlCount  int32
	ignCount int32
}

func (s *stubAction) OnRSSDownload(_ context.Context, logID uint, userID int64) error {
	atomic.AddInt32(&s.dlCount, 1)
	s.mu.Lock()
	s.calls = append(s.calls, recordedCall{verb: "dl", logID: logID, userID: userID})
	s.mu.Unlock()
	return s.dlErr
}

func (s *stubAction) OnRSSIgnore(_ context.Context, logID uint, userID int64) error {
	atomic.AddInt32(&s.ignCount, 1)
	s.mu.Lock()
	s.calls = append(s.calls, recordedCall{verb: "ig", logID: logID, userID: userID})
	s.mu.Unlock()
	return s.ignErr
}

func (s *stubAction) snapshot() []recordedCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]recordedCall, len(s.calls))
	copy(out, s.calls)
	return out
}

func newCallbackTestChannel(t *testing.T) (*TelegramChannel, *fakeBot, *stubAction) {
	t.Helper()
	bot := &fakeBot{}
	src := newFakeSource()
	plain := []byte(`{"bot_token":"123:abcdefghijklmnopqrstuvwxyzABCDEFGHIJK","allowed_users":[111],"admin_users":[111],"default_chat_id":555}`)
	c := newChannelWithFakes(t, nil, bot, src, plain)
	conf := &models.NotificationConf{ID: 7, ChannelType: ChannelType, ConfigJSON: string(plain)}
	require.NoError(t, c.Init(context.Background(), conf))
	action := &stubAction{}
	c.SetCallbackActionHandler(action)
	return c, bot, action
}

func makeCallbackUpdate(data string, userID int64) telego.Update {
	return telego.Update{
		CallbackQuery: &telego.CallbackQuery{
			ID:   "cbq-1",
			From: telego.User{ID: userID, Username: "tester"},
			Data: data,
			Message: &telego.Message{
				MessageID: 4242,
				Chat:      telego.Chat{ID: 555},
			},
		},
	}
}

func TestTelegramCallback_DownloadDispatchesAndClearsButtons(t *testing.T) {
	c, bot, action := newCallbackTestChannel(t)

	c.handleUpdate(context.Background(), makeCallbackUpdate("dl:42", 111))

	assert.EqualValues(t, 1, atomic.LoadInt32(&action.dlCount))
	assert.EqualValues(t, 0, atomic.LoadInt32(&action.ignCount))
	calls := action.snapshot()
	require.Len(t, calls, 1)
	assert.Equal(t, "dl", calls[0].verb)
	assert.EqualValues(t, 42, calls[0].logID)
	assert.EqualValues(t, 111, calls[0].userID)

	ans := bot.lastAnswer()
	require.NotNil(t, ans)
	assert.Contains(t, ans.Text, "已加入下载队列")

	assert.Equal(t, 1, bot.editMarkupCount(), "inline keyboard must be cleared after dispatch")
}

func TestTelegramCallback_IgnoreDispatchesAndClearsButtons(t *testing.T) {
	c, bot, action := newCallbackTestChannel(t)

	c.handleUpdate(context.Background(), makeCallbackUpdate("ig:99", 111))

	assert.EqualValues(t, 1, atomic.LoadInt32(&action.ignCount))
	assert.EqualValues(t, 0, atomic.LoadInt32(&action.dlCount))
	calls := action.snapshot()
	require.Len(t, calls, 1)
	assert.Equal(t, "ig", calls[0].verb)
	assert.EqualValues(t, 99, calls[0].logID)

	ans := bot.lastAnswer()
	require.NotNil(t, ans)
	assert.Contains(t, ans.Text, "已忽略")

	assert.Equal(t, 1, bot.editMarkupCount())
}

func TestTelegramCallback_ActionErrorSurfacesInAck(t *testing.T) {
	c, bot, action := newCallbackTestChannel(t)
	action.dlErr = errors.New("boom")

	c.handleUpdate(context.Background(), makeCallbackUpdate("dl:7", 111))

	ans := bot.lastAnswer()
	require.NotNil(t, ans)
	assert.Contains(t, ans.Text, "下载触发失败")
	assert.Contains(t, ans.Text, "boom")
}

func TestTelegramCallback_MalformedDataIgnoredGracefully(t *testing.T) {
	c, bot, action := newCallbackTestChannel(t)

	c.handleUpdate(context.Background(), makeCallbackUpdate("garbage-no-colon", 111))

	assert.EqualValues(t, 0, atomic.LoadInt32(&action.dlCount))
	assert.EqualValues(t, 0, atomic.LoadInt32(&action.ignCount))
	ans := bot.lastAnswer()
	require.NotNil(t, ans)
	assert.Equal(t, "未知操作", ans.Text)
	assert.Equal(t, 0, bot.editMarkupCount(), "no markup edit for unknown verbs")
}

func TestTelegramCallback_NumericPayloadValidation(t *testing.T) {
	c, bot, action := newCallbackTestChannel(t)

	c.handleUpdate(context.Background(), makeCallbackUpdate("dl:not-a-number", 111))

	assert.EqualValues(t, 0, atomic.LoadInt32(&action.dlCount))
	ans := bot.lastAnswer()
	require.NotNil(t, ans)
	assert.Contains(t, ans.Text, "参数无效")
}

func TestTelegramCallback_StubModeWithoutHandlerKeepsLegacyAck(t *testing.T) {
	bot := &fakeBot{}
	src := newFakeSource()
	plain := []byte(`{"bot_token":"123:abcdefghijklmnopqrstuvwxyzABCDEFGHIJK","allowed_users":[111],"admin_users":[111],"default_chat_id":555}`)
	c := newChannelWithFakes(t, nil, bot, src, plain)
	conf := &models.NotificationConf{ID: 7, ChannelType: ChannelType, ConfigJSON: string(plain)}
	require.NoError(t, c.Init(context.Background(), conf))

	c.handleUpdate(context.Background(), makeCallbackUpdate("dl:7", 111))

	ans := bot.lastAnswer()
	require.NotNil(t, ans)
	assert.Contains(t, ans.Text, "已记录下载请求")
	assert.Equal(t, 0, bot.editMarkupCount(), "stub mode does not edit markup")
}

func TestTelegramCallback_DenyForUnauthorizedUser(t *testing.T) {
	c, bot, action := newCallbackTestChannel(t)

	c.handleUpdate(context.Background(), makeCallbackUpdate("dl:42", 999))

	assert.EqualValues(t, 0, atomic.LoadInt32(&action.dlCount))
	ans := bot.lastAnswer()
	require.NotNil(t, ans)
	assert.Equal(t, denyMessage, ans.Text)
	assert.True(t, ans.ShowAlert)
}
