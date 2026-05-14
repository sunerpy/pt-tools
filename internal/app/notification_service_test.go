package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/models"
)

// setupTestDB 创建独立的 in-memory SQLite，仅 AutoMigrate NotificationService 用到的 ChatOps 表。
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "open sqlite memory")
	require.NoError(t, db.AutoMigrate(
		&models.NotificationConf{},
		&models.NotificationOutbox{},
	), "automigrate chatops models")
	return db
}

func setupTestKey(t *testing.T) {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i % 256)
	}
	t.Setenv("PT_TOOLS_SECRET_KEY", base64.StdEncoding.EncodeToString(key))
	crypto.ResetForTest()
}

// mockNotifyManager 实现 NotifyManager 接口，用于注入投递行为
type mockNotifyManager struct {
	mu        sync.Mutex
	calls     []uint
	delay     time.Duration
	returnErr error
}

func (m *mockNotifyManager) Send(ctx context.Context, confID uint, n Notification) error {
	m.mu.Lock()
	m.calls = append(m.calls, confID)
	delay := m.delay
	retErr := m.returnErr
	m.mu.Unlock()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return retErr
}

func (m *mockNotifyManager) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// TestCreateConf_EncryptsConfig 验证 CreateConf 写入 DB 的 ConfigJSON 是密文
// 直接 SELECT config_json，断言不含明文，且能 Decrypt 还原。
func TestCreateConf_EncryptsConfig(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)

	svc := NewNotificationService(db, &mockNotifyManager{}, 5*time.Second)

	plaintext := `{"bot_token":"super-secret-token-12345"}`
	dto, err := svc.CreateConf(context.Background(), CreateConfReq{
		ChannelType: "telegram",
		Name:        "test-bot",
		ConfigJSON:  json.RawMessage(plaintext),
		Enabled:     true,
	})
	require.NoError(t, err, "CreateConf should not error")
	require.NotZero(t, dto.ID, "DTO should have ID")

	// 直接 SELECT config_json 验证是密文
	var stored string
	err = db.Raw("SELECT config_json FROM notification_conf WHERE id = ?", dto.ID).
		Scan(&stored).Error
	require.NoError(t, err)
	assert.NotEmpty(t, stored, "stored config_json should not be empty")
	assert.NotContains(t, stored, "super-secret-token-12345", "plaintext should NOT appear in DB")
	assert.NotContains(t, stored, "bot_token", "plaintext field name should NOT appear")

	// 验证可以 Decrypt 还原
	decrypted, err := crypto.Decrypt(stored)
	require.NoError(t, err, "Decrypt should succeed")
	assert.JSONEq(t, plaintext, string(decrypted), "decrypted should match plaintext")
}

// TestPush_SyncSuccess 验证 channel 在线时立即投递成功
func TestPush_SyncSuccess(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)

	mock := &mockNotifyManager{delay: 0, returnErr: nil}
	svc := NewNotificationService(db, mock, 5*time.Second)

	// 先创建一个 conf
	dto, err := svc.CreateConf(context.Background(), CreateConfReq{
		ChannelType: "telegram",
		Name:        "test",
		ConfigJSON:  json.RawMessage(`{"bot_token":"xxx"}`),
		Enabled:     true,
	})
	require.NoError(t, err)

	// Push
	n := Notification{Title: "hi", Text: "hello", SourceConfID: dto.ID}
	err = svc.Push(context.Background(), n)
	require.NoError(t, err, "Push should succeed sync")

	// 验证 manager.Send 被调用过
	assert.Equal(t, 1, mock.callCount(), "manager.Send should be called once")

	// 验证 outbox 表没有 pending 记录
	var pendingCount int64
	require.NoError(t, db.Model(&models.NotificationOutbox{}).
		Where("status = ?", "pending").Count(&pendingCount).Error)
	assert.Equal(t, int64(0), pendingCount, "outbox should NOT have pending row")
}

// TestPush_FallbackOutbox 验证 channel 超时（>5s 模拟为 short timeout）后，转入 outbox
func TestPush_FallbackOutbox(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)

	// 模拟 send 超时：timeout=50ms，delay=200ms
	mock := &mockNotifyManager{delay: 200 * time.Millisecond, returnErr: nil}
	svc := NewNotificationService(db, mock, 50*time.Millisecond)

	dto, err := svc.CreateConf(context.Background(), CreateConfReq{
		ChannelType: "telegram",
		Name:        "test",
		ConfigJSON:  json.RawMessage(`{"bot_token":"xxx"}`),
		Enabled:     true,
	})
	require.NoError(t, err)

	n := Notification{Title: "hi", Text: "hello", SourceConfID: dto.ID}
	pushErr := svc.Push(context.Background(), n)
	// Push 在超时后转 outbox，返回 nil（已成功入队）
	require.NoError(t, pushErr, "Push should fallback to outbox without erroring")

	// outbox 应有 pending 记录
	var rows []models.NotificationOutbox
	require.NoError(t, db.Where("status = ?", "pending").Find(&rows).Error)
	require.Len(t, rows, 1, "should have exactly 1 pending outbox row")
	assert.Equal(t, dto.ID, rows[0].NotificationConfID, "outbox row should link to conf")
	assert.NotEmpty(t, rows[0].PayloadJSON, "outbox payload should not be empty")
}

// TestListConfs_RoundTrip 验证 List 返回 Create 写入的 conf
func TestListConfs_RoundTrip(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)
	svc := NewNotificationService(db, &mockNotifyManager{}, 5*time.Second)

	_, err := svc.CreateConf(context.Background(), CreateConfReq{
		ChannelType: "telegram",
		Name:        "tg-1",
		ConfigJSON:  json.RawMessage(`{"bot_token":"a"}`),
		Enabled:     true,
	})
	require.NoError(t, err)

	_, err = svc.CreateConf(context.Background(), CreateConfReq{
		ChannelType: "qq_onebot",
		Name:        "qq-1",
		ConfigJSON:  json.RawMessage(`{"endpoint":"http://x"}`),
		Enabled:     false,
	})
	require.NoError(t, err)

	got, err := svc.ListConfs(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2, "should list 2 confs")

	names := map[string]bool{}
	for _, d := range got {
		names[d.Name] = true
		// 不应回吐密文 / 明文
		assert.Empty(t, d.ConfigJSON, "ListConfs should not include ConfigJSON in DTO")
	}
	assert.True(t, names["tg-1"], "tg-1 in list")
	assert.True(t, names["qq-1"], "qq-1 in list")
}

// TestPush_ManagerError_FallbackOutbox 验证 manager 显式返回错误时也会落 outbox
func TestPush_ManagerError_FallbackOutbox(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)

	mock := &mockNotifyManager{delay: 0, returnErr: errors.New("connection refused")}
	svc := NewNotificationService(db, mock, 5*time.Second)

	dto, err := svc.CreateConf(context.Background(), CreateConfReq{
		ChannelType: "telegram",
		Name:        "test",
		ConfigJSON:  json.RawMessage(`{"bot_token":"xxx"}`),
		Enabled:     true,
	})
	require.NoError(t, err)

	n := Notification{Title: "fail", Text: "world", SourceConfID: dto.ID}
	require.NoError(t, svc.Push(context.Background(), n), "Push fallback to outbox returns nil")

	var pendingCount int64
	require.NoError(t, db.Model(&models.NotificationOutbox{}).
		Where("status = ?", "pending").Count(&pendingCount).Error)
	assert.Equal(t, int64(1), pendingCount, "outbox should have 1 pending row")
}
