package notify

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/glebarez/sqlite"

	"github.com/sunerpy/pt-tools/models"
)

type mockOutboxChannel struct {
	initErr error
	sendErr error
	calls   int
}

func (m *mockOutboxChannel) Type() string { return "mock" }

func (m *mockOutboxChannel) Init(_ context.Context, _ *models.NotificationConf) error {
	return m.initErr
}

func (m *mockOutboxChannel) SupportsInbound() bool { return false }

func (m *mockOutboxChannel) Send(_ context.Context, _ Notification) error {
	m.calls++
	return m.sendErr
}

func (m *mockOutboxChannel) OnInbound(_ InboundHandler) {}

func (m *mockOutboxChannel) Close(_ context.Context) error { return nil }

func (m *mockOutboxChannel) Healthy() bool { return true }

func newOutboxTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(&models.NotificationConf{}, &models.NotificationOutbox{}))
	return db
}

func seedOutbox(t *testing.T, db *gorm.DB, retryCount int, now time.Time) models.NotificationOutbox {
	t.Helper()

	conf := models.NotificationConf{
		ChannelType: "mock",
		Name:        "mock channel",
		Enabled:     true,
	}
	require.NoError(t, db.Create(&conf).Error)

	outbox := models.NotificationOutbox{
		NotificationConfID: conf.ID,
		PayloadJSON:        `{"title":"hello","text":"world","channel_type":"mock","source_conf_id":1}`,
		Status:             "pending",
		RetryCount:         retryCount,
		NextRetryAt:        now.Add(-time.Second),
	}
	require.NoError(t, db.Create(&outbox).Error)
	return outbox
}

func withFixedNow(t *testing.T, now time.Time) {
	t.Helper()
	original := nowFn
	nowFn = func() time.Time { return now }
	t.Cleanup(func() { nowFn = original })
}

func newOutboxWorkerWithMock(t *testing.T, db *gorm.DB, ch *mockOutboxChannel) *OutboxWorker {
	t.Helper()

	registry := NewRegistry()
	registry.Register("mock", func() Channel { return ch })
	return NewOutboxWorker(db, registry, 10*time.Millisecond)
}

func TestOutbox_SuccessTransitionsToSent(t *testing.T) {
	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	withFixedNow(t, now)
	db := newOutboxTestDB(t)
	outbox := seedOutbox(t, db, 0, now)
	ch := &mockOutboxChannel{}
	worker := newOutboxWorkerWithMock(t, db, ch)

	require.NoError(t, worker.Tick(context.Background()))

	var got models.NotificationOutbox
	require.NoError(t, db.First(&got, outbox.ID).Error)
	require.Equal(t, "sent", got.Status)
	require.Equal(t, 1, ch.calls)
	require.NotNil(t, got.SentAt)
	require.True(t, got.SentAt.Equal(now))
}

func TestOutbox_FailureBackoff_10s(t *testing.T) {
	assertFailureBackoff(t, 0, 1, 10*time.Second)
}

func TestOutbox_FailureBackoff_60s(t *testing.T) {
	assertFailureBackoff(t, 1, 2, 60*time.Second)
}

func TestOutbox_FailureBackoff_300s(t *testing.T) {
	assertFailureBackoff(t, 2, 3, 300*time.Second)
}

func assertFailureBackoff(t *testing.T, initialRetry, expectedRetry int, expectedDelay time.Duration) {
	t.Helper()

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	withFixedNow(t, now)
	db := newOutboxTestDB(t)
	outbox := seedOutbox(t, db, initialRetry, now)
	ch := &mockOutboxChannel{sendErr: errors.New("temporary failure")}
	worker := newOutboxWorkerWithMock(t, db, ch)

	require.NoError(t, worker.Tick(context.Background()))

	var got models.NotificationOutbox
	require.NoError(t, db.First(&got, outbox.ID).Error)
	require.Equal(t, "pending", got.Status)
	require.Equal(t, expectedRetry, got.RetryCount)
	require.WithinDuration(t, now.Add(expectedDelay), got.NextRetryAt, 2*time.Second)
	require.Equal(t, "temporary failure", got.ErrorMsg)
}

func TestOutbox_TerminalDead(t *testing.T) {
	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	withFixedNow(t, now)
	db := newOutboxTestDB(t)
	outbox := seedOutbox(t, db, 3, now)
	ch := &mockOutboxChannel{sendErr: errors.New("terminal failure")}
	worker := newOutboxWorkerWithMock(t, db, ch)

	require.NoError(t, worker.Tick(context.Background()))

	var got models.NotificationOutbox
	require.NoError(t, db.First(&got, outbox.ID).Error)
	require.Equal(t, "dead", got.Status)
	require.Equal(t, 3, got.RetryCount)
	require.Equal(t, "terminal failure", got.ErrorMsg)
}

func TestOutbox_GracefulStop(t *testing.T) {
	db := newOutboxTestDB(t)
	ch := &mockOutboxChannel{}
	worker := newOutboxWorkerWithMock(t, db, ch)

	worker.Start(context.Background())
	stopped := make(chan struct{})
	go func() {
		worker.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop within 1s")
	}
}
