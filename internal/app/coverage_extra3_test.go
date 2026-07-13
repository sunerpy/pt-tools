// MIT License
// Copyright (c) 2025 pt-tools

package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/models"
)

func TestIssueCode_GenError(t *testing.T) {
	db := setupBindingTestDB(t)
	svc := &bindingService{
		db:        db,
		createdBy: "admin",
		now:       time.Now,
		gen:       func() (string, error) { return "", errors.New("gen boom") },
	}
	_, err := svc.IssueCode(context.Background(), 1, "x", time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "generate bind code")
}

func TestToBindingDTO_CopiesLastActiveAt(t *testing.T) {
	now := time.Now()
	dto := toBindingDTO(models.ChannelBinding{
		NotificationConfID: 3, ChannelType: "telegram", ChannelUserID: "u",
		Label: "l", ReplyLang: "en", PtAdmin: true, Allowed: true,
		LastActiveAt: &now,
	})
	assert.Equal(t, uint(3), dto.ConfID)
	assert.Equal(t, "en", dto.ReplyLang)
	assert.False(t, dto.LastActiveAt.IsZero())
}

func TestListBindings_ReturnsDTOs(t *testing.T) {
	svc, db := newBindingService(t)
	require.NoError(t, db.Create(&models.ChannelBinding{
		NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1", PtAdmin: true, Allowed: true,
	}).Error)
	list, err := svc.ListBindings(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "u1", list[0].ChannelUserID)
}

func TestConsumeCode_MissingArgs(t *testing.T) {
	svc, _ := newBindingService(t)
	_, err := svc.ConsumeCode(context.Background(), "", "telegram", "u")
	require.ErrorIs(t, err, ErrCodeUsedOrExpired)
	_, err = svc.ConsumeCode(context.Background(), "c", "", "u")
	require.ErrorIs(t, err, ErrCodeUsedOrExpired)
	_, err = svc.ConsumeCode(context.Background(), "c", "telegram", "")
	require.ErrorIs(t, err, ErrCodeUsedOrExpired)
}

func TestAuditRecord_MarshalError(t *testing.T) {
	db := setupAuditTestDB(t)
	svc := NewAuditService(db)
	err := svc.Record(context.Background(), AuditEntry{
		Command: "x",
		Args:    map[string]any{"bad": make(chan int)},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "序列化")
}

func setupClosedDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.ActionAudit{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	return db
}

func TestAuditQuery_CountError(t *testing.T) {
	db := setupClosedDB(t)
	svc := NewAuditService(db)
	_, _, err := svc.Query(context.Background(), AuditQuery{})
	require.Error(t, err)
}

func TestAuditStats_Error(t *testing.T) {
	db := setupClosedDB(t)
	svc := NewAuditService(db)
	_, err := svc.Stats(context.Background())
	require.Error(t, err)
}

func TestAuditPrune_Error(t *testing.T) {
	db := setupClosedDB(t)
	svc := NewAuditService(db)
	_, err := svc.Prune(context.Background())
	require.Error(t, err)
}

func TestAuditQuery_PageSizeClampMax(t *testing.T) {
	db := setupAuditTestDB(t)
	svc := NewAuditService(db)
	_, _, err := svc.Query(context.Background(), AuditQuery{PageSize: 9999})
	require.NoError(t, err)
}

func TestExceededHourlyQuota_CountError(t *testing.T) {
	db := setupClosedForRSS(t)
	n := NewRSSNotifier(db, &fakePushSvc{}).(*rssNotifier)
	rss := &models.RSSConfig{ID: 1, MaxNotificationsPerHour: 5}
	_, err := n.exceededHourlyQuota(context.Background(), rss)
	require.Error(t, err)
}

func setupClosedForRSS(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.RSSNotificationLog{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	return db
}

func TestTelegramTestChatID_InvalidTypes(t *testing.T) {
	setupTestKey(t)
	bad, err := crypto.Encrypt([]byte(`{"default_chat_id":{"nested":1}}`))
	require.NoError(t, err)
	_, err = telegramTestChatID(models.NotificationConf{ConfigJSON: bad})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default_chat_id")

	bad2, err := crypto.Encrypt([]byte(`{"admin_users":123}`))
	require.NoError(t, err)
	_, err = telegramTestChatID(models.NotificationConf{ConfigJSON: bad2})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "admin_users")
}

func TestEnqueue_CreateError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.NotificationOutbox{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	svc := NewNotificationService(db, nil, 0).(*notificationService)
	err = svc.Enqueue(context.Background(), Notification{Title: "t"}, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outbox")
}
