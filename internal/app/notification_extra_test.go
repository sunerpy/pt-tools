// MIT License
// Copyright (c) 2025 pt-tools

// Coverage for NotificationService CreateConf/UpdateConf/DeleteConf/Enqueue
// error + guard branches not exercised elsewhere, and BindingService Revoke /
// SetReplyLang not-found paths.

package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

func TestCreateConf_Validation(t *testing.T) {
	db := setupTestDB(t)
	svc := NewNotificationService(db, &mockNotifyManager{}, 0)
	ctx := context.Background()

	_, err := svc.CreateConf(ctx, CreateConfReq{})
	require.Error(t, err)
	_, err = svc.CreateConf(ctx, CreateConfReq{ChannelType: "telegram"})
	require.Error(t, err)
	_, err = svc.CreateConf(ctx, CreateConfReq{ChannelType: "telegram", Name: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config_json")
}

func TestUpdateConf_ZeroIDAndNotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := NewNotificationService(db, &mockNotifyManager{}, 0)
	ctx := context.Background()

	require.Error(t, svc.UpdateConf(ctx, 0, UpdateConfReq{}))

	name := "new"
	err := svc.UpdateConf(ctx, 999, UpdateConfReq{Name: &name})
	require.ErrorIs(t, err, ErrConfNotFound)
}

func TestUpdateConf_NoFieldsIsNoop(t *testing.T) {
	db := setupTestDB(t)
	svc := NewNotificationService(db, &mockNotifyManager{}, 0)
	require.NoError(t, svc.UpdateConf(context.Background(), 5, UpdateConfReq{}))
}

func TestDeleteConf_ZeroIDAndNotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := NewNotificationService(db, &mockNotifyManager{}, 0)
	ctx := context.Background()

	require.Error(t, svc.DeleteConf(ctx, 0))
	require.ErrorIs(t, svc.DeleteConf(ctx, 999), ErrConfNotFound)
}

func TestEnqueue_PersistsOutbox(t *testing.T) {
	db := setupTestDB(t)
	svc := NewNotificationService(db, &mockNotifyManager{}, 0)
	err := svc.Enqueue(context.Background(), Notification{Title: "t", Text: "b"}, 3)
	require.NoError(t, err)

	var cnt int64
	require.NoError(t, db.Model(&models.NotificationOutbox{}).Count(&cnt).Error)
	assert.EqualValues(t, 1, cnt)
}

func TestBindingService_RevokeNotFound(t *testing.T) {
	svc, _ := newBindingService(t)
	err := svc.Revoke(context.Background(), 9999)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestBindingService_SetReplyLangNotFound(t *testing.T) {
	svc, _ := newBindingService(t)
	err := svc.SetReplyLang(context.Background(), 9999, "en")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestBindingService_IssueCodeNoTTL(t *testing.T) {
	svc, _ := newBindingService(t)
	dto, err := svc.IssueCode(context.Background(), 1, "", 0)
	require.NoError(t, err)
	assert.Nil(t, dto.ExpiresAt)
	assert.NotEmpty(t, dto.Code)
	_ = time.Now
}
