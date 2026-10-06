package notify

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

type recordingLiveSender struct {
	confIDs []uint
	sent    []Notification
	err     error
}

func (r *recordingLiveSender) Send(_ context.Context, confID uint, n Notification) error {
	r.confIDs = append(r.confIDs, confID)
	r.sent = append(r.sent, n)
	return r.err
}

// 注入 LiveSender 后，重试经运行中的通道投递，不再按配置另起实例（QQ 会抢端口、Telegram 会多开长轮询）。
func TestOutbox_LiveSenderDeliversWithoutNewChannel(t *testing.T) {
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	withFixedNow(t, now)
	db := newOutboxTestDB(t)
	outbox := seedOutbox(t, db, 0, now)

	ch := &mockOutboxChannel{}
	worker := newOutboxWorkerWithMock(t, db, ch)
	live := &recordingLiveSender{}
	worker.SetLiveSender(live)

	require.NoError(t, worker.Tick(context.Background()))

	assert.Zero(t, ch.calls, "不经 registry 新建的实例发送")
	require.Equal(t, []uint{outbox.NotificationConfID}, live.confIDs)
	assert.Equal(t, "hello", live.sent[0].Title)
	var got models.NotificationOutbox
	require.NoError(t, db.First(&got, outbox.ID).Error)
	assert.Equal(t, "sent", got.Status)
}

func TestOutbox_LiveSenderFailureBacksOff(t *testing.T) {
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	withFixedNow(t, now)
	db := newOutboxTestDB(t)
	outbox := seedOutbox(t, db, 0, now)
	worker := newOutboxWorkerWithMock(t, db, &mockOutboxChannel{})
	worker.SetLiveSender(&recordingLiveSender{err: errors.New("通知通道未运行")})

	require.NoError(t, worker.Tick(context.Background()))

	var got models.NotificationOutbox
	require.NoError(t, db.First(&got, outbox.ID).Error)
	assert.Equal(t, "pending", got.Status)
	assert.Equal(t, 1, got.RetryCount)
	assert.Contains(t, got.ErrorMsg, "通知通道未运行")
}

type initRecordingChannel struct {
	mockOutboxChannel
	gotConfig string
	closed    bool
}

func (c *initRecordingChannel) Init(_ context.Context, conf *models.NotificationConf) error {
	c.gotConfig = conf.ConfigJSON
	return nil
}

func (c *initRecordingChannel) Close(_ context.Context) error { c.closed = true; return nil }

// 回退路径（没有 LiveSender）：库里的 ConfigJSON 是密文，初始化前先解密；临时实例用完关闭。
func TestOutbox_FallbackDecryptsConfigAndClosesChannel(t *testing.T) {
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	withFixedNow(t, now)
	db := newOutboxTestDB(t)
	outbox := seedOutbox(t, db, 0, now)
	require.NoError(t, db.Model(&models.NotificationConf{}).Where("id = ?", outbox.NotificationConfID).
		Update("config_json", "CIPHERTEXT").Error)

	ch := &initRecordingChannel{}
	registry := NewRegistry()
	registry.Register("mock", func() Channel { return ch })
	worker := NewOutboxWorker(db, registry, 10*time.Millisecond)
	worker.SetConfigDecrypter(func(s string) (string, error) {
		if s != "CIPHERTEXT" {
			return "", errors.New("unexpected")
		}
		return `{"webhook_key":"k"}`, nil
	})

	require.NoError(t, worker.Tick(context.Background()))

	assert.Equal(t, `{"webhook_key":"k"}`, ch.gotConfig, "初始化拿到的是明文配置")
	assert.True(t, ch.closed, "临时实例用完关闭")
	var got models.NotificationOutbox
	require.NoError(t, db.First(&got, outbox.ID).Error)
	assert.Equal(t, "sent", got.Status)
}
