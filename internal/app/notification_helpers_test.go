// MIT License
// Copyright (c) 2025 pt-tools

// Coverage for NotificationService.GetConf + TestConf/testChatID helpers,
// and the raw* JSON coercion helpers.

package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/models"
)

func TestGetConf_NotFoundAndZeroID(t *testing.T) {
	db := setupTestDB(t)
	svc := NewNotificationService(db, &mockNotifyManager{}, 0)

	_, err := svc.GetConf(context.Background(), 0)
	require.Error(t, err)

	_, err = svc.GetConf(context.Background(), 999)
	require.ErrorIs(t, err, ErrConfNotFound)
}

func TestGetConf_DecryptsConfigJSON(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)
	cipher, err := crypto.Encrypt([]byte(`{"bot_token":"abc","default_chat_id":123}`))
	require.NoError(t, err)
	row := models.NotificationConf{ChannelType: "telegram", Name: "tg", ConfigJSON: cipher, Enabled: true}
	require.NoError(t, db.Create(&row).Error)

	svc := NewNotificationService(db, &mockNotifyManager{}, 0)
	dto, err := svc.GetConf(context.Background(), row.ID)
	require.NoError(t, err)
	assert.Equal(t, "telegram", dto.ChannelType)
	assert.Contains(t, string(dto.ConfigJSON), "bot_token")
}

func TestTestConf_TelegramWithDefaultChatID(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)
	cipher, err := crypto.Encrypt([]byte(`{"bot_token":"t","default_chat_id":"999"}`))
	require.NoError(t, err)
	row := models.NotificationConf{ChannelType: "telegram", Name: "tg", ConfigJSON: cipher, Enabled: true}
	require.NoError(t, db.Create(&row).Error)

	mgr := &mockNotifyManager{}
	svc := NewNotificationService(db, mgr, 0)
	require.NoError(t, svc.TestConf(context.Background(), row.ID))
	assert.Contains(t, mgr.calls, row.ID)
}

func TestTestConf_QQWithAdminUsers(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)
	cipher, err := crypto.Encrypt([]byte(`{"admin_qq_users":[12345]}`))
	require.NoError(t, err)
	row := models.NotificationConf{ChannelType: "qq_onebot", Name: "qq", ConfigJSON: cipher, Enabled: true}
	require.NoError(t, db.Create(&row).Error)

	mgr := &mockNotifyManager{}
	svc := NewNotificationService(db, mgr, 0)
	require.NoError(t, svc.TestConf(context.Background(), row.ID))
	assert.Contains(t, mgr.calls, row.ID)
}

func TestTestConf_NotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := NewNotificationService(db, &mockNotifyManager{}, 0)
	require.ErrorIs(t, svc.TestConf(context.Background(), 404), ErrConfNotFound)
}

func TestTestConf_TelegramNoRecipient(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.ChannelBinding{}))
	cipher, err := crypto.Encrypt([]byte(`{"bot_token":"t"}`))
	require.NoError(t, err)
	row := models.NotificationConf{ChannelType: "telegram", Name: "tg", ConfigJSON: cipher, Enabled: true}
	require.NoError(t, db.Create(&row).Error)

	svc := NewNotificationService(db, &mockNotifyManager{}, 0)
	err = svc.TestConf(context.Background(), row.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "无可用收件人")
}

func TestQQTestChatID(t *testing.T) {
	setupTestKey(t)
	admin, err := crypto.Encrypt([]byte(`{"admin_qq_users":[111,222]}`))
	require.NoError(t, err)
	got, err := qqTestChatID(models.NotificationConf{ConfigJSON: admin})
	require.NoError(t, err)
	assert.Equal(t, "111", got)

	allowed, err := crypto.Encrypt([]byte(`{"allowed_qq_users":[333]}`))
	require.NoError(t, err)
	got, err = qqTestChatID(models.NotificationConf{ConfigJSON: allowed})
	require.NoError(t, err)
	assert.Equal(t, "333", got)

	empty, err := crypto.Encrypt([]byte(`{}`))
	require.NoError(t, err)
	got, err = qqTestChatID(models.NotificationConf{ConfigJSON: empty})
	require.NoError(t, err)
	assert.Equal(t, "", got)
}

func TestTelegramTestChatID(t *testing.T) {
	setupTestKey(t)
	// default_chat_id as string.
	c1, err := crypto.Encrypt([]byte(`{"default_chat_id":"555"}`))
	require.NoError(t, err)
	got, err := telegramTestChatID(models.NotificationConf{ConfigJSON: c1})
	require.NoError(t, err)
	assert.Equal(t, "555", got)

	// admin_users as int slice.
	c2, err := crypto.Encrypt([]byte(`{"admin_users":[777,888]}`))
	require.NoError(t, err)
	got, err = telegramTestChatID(models.NotificationConf{ConfigJSON: c2})
	require.NoError(t, err)
	assert.Equal(t, "777", got)
}

func TestRawStringOrInt64(t *testing.T) {
	got, err := rawStringOrInt64([]byte(`"abc"`))
	require.NoError(t, err)
	assert.Equal(t, "abc", got)

	got, err = rawStringOrInt64([]byte(`42`))
	require.NoError(t, err)
	assert.Equal(t, "42", got)

	got, err = rawStringOrInt64([]byte(`0`))
	require.NoError(t, err)
	assert.Equal(t, "", got)

	_, err = rawStringOrInt64([]byte(`{"x":1}`))
	require.Error(t, err)
}

func TestRawStringOrInt64Slice(t *testing.T) {
	got, err := rawStringOrInt64Slice([]byte(`["a","b"]`))
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, got)

	got, err = rawStringOrInt64Slice([]byte(`[1,2,3]`))
	require.NoError(t, err)
	assert.Equal(t, []string{"1", "2", "3"}, got)

	_, err = rawStringOrInt64Slice([]byte(`"notaslice"`))
	require.Error(t, err)
}

func TestFirstBindingChatID(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.ChannelBinding{}))
	svc := NewNotificationService(db, &mockNotifyManager{}, 0).(*notificationService)

	// No binding -> empty, no error.
	got, err := svc.firstBindingChatID(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, "", got)

	require.NoError(t, db.Create(&models.ChannelBinding{
		NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u-admin", PtAdmin: true,
	}).Error)
	got, err = svc.firstBindingChatID(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, "u-admin", got)
}
