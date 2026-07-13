// MIT License
// Copyright (c) 2025 pt-tools

package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// setupBindingTestDB 创建独立 :memory: SQLite 并 AutoMigrate ChatOps 表。
// 每个测试唯一的 cache name + single conn 防止并发场景的 "no such table"。
func setupBindingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=busy_timeout(5000)", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(
		&models.NotificationConf{},
		&models.ChannelBinding{},
		&models.BotToken{},
	))
	return db
}

// newBindingService 测试辅助：构造一个使用 in-memory db、固定 admin 的 service。
func newBindingService(t *testing.T) (BindingService, *gorm.DB) {
	t.Helper()
	db := setupBindingTestDB(t)
	svc := NewBindingService(db, "admin-1")
	return svc, db
}

func TestIssueCode_HappyPath(t *testing.T) {
	svc, db := newBindingService(t)
	ctx := context.Background()

	dto, err := svc.IssueCode(ctx, 7, "tg-main", 5*time.Minute)
	require.NoError(t, err)
	assert.NotEmpty(t, dto.Code)
	assert.Len(t, dto.Code, 8)
	assert.Equal(t, uint(7), dto.ConfID)
	assert.Equal(t, "tg-main", dto.Label)
	require.NotNil(t, dto.ExpiresAt)
	assert.WithinDuration(t, time.Now().Add(5*time.Minute), *dto.ExpiresAt, 2*time.Second)

	var rows []models.BotToken
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, "bind_code", rows[0].Kind)
	assert.Equal(t, "bind:7|label:tg-main", rows[0].Scope)
	// CodeOrTokenHash 存明文 code（plan: 5min TTL 单次使用，不强制 hash）
	assert.Equal(t, dto.Code, rows[0].CodeOrTokenHash)
	assert.Nil(t, rows[0].UsedAt)
	assert.Equal(t, "admin-1", rows[0].CreatedBy)
}

func TestIssueCode_RespectsMaxActiveCap(t *testing.T) {
	svc, _ := newBindingService(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := svc.IssueCode(ctx, 1, "x", 5*time.Minute)
		require.NoError(t, err, "issue #%d should succeed", i+1)
	}
	_, err := svc.IssueCode(ctx, 1, "x", 5*time.Minute)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTooManyActiveCodes), "expected ErrTooManyActiveCodes, got %v", err)
}

func TestIssueCode_ExpiredCodesDoNotCountTowardCap(t *testing.T) {
	svc, db := newBindingService(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := svc.IssueCode(ctx, 1, "x", 5*time.Minute)
		require.NoError(t, err)
	}
	// 强制 3 个全部过期
	require.NoError(t, db.Model(&models.BotToken{}).
		Where("kind = ?", "bind_code").
		UpdateColumn("expires_at", time.Now().Add(-time.Minute)).Error)

	// 第 4 个应当可以发放
	_, err := svc.IssueCode(ctx, 1, "x", 5*time.Minute)
	require.NoError(t, err)
}

func TestIssueCode_UsedCodesDoNotCountTowardCap(t *testing.T) {
	svc, db := newBindingService(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := svc.IssueCode(ctx, 1, "x", 5*time.Minute)
		require.NoError(t, err)
	}
	// 标记 3 个已使用
	now := time.Now()
	require.NoError(t, db.Model(&models.BotToken{}).
		Where("kind = ?", "bind_code").
		UpdateColumn("used_at", now).Error)

	_, err := svc.IssueCode(ctx, 1, "x", 5*time.Minute)
	require.NoError(t, err)
}

func TestListPendingCodes(t *testing.T) {
	svc, db := newBindingService(t)
	ctx := context.Background()

	c1, err := svc.IssueCode(ctx, 1, "a", 5*time.Minute)
	require.NoError(t, err)
	c2, err := svc.IssueCode(ctx, 2, "b", 5*time.Minute)
	require.NoError(t, err)

	// 让 c2 过期
	require.NoError(t, db.Model(&models.BotToken{}).
		Where("code_or_token_hash = ?", c2.Code).
		UpdateColumn("expires_at", time.Now().Add(-time.Minute)).Error)

	pending, err := svc.ListPendingCodes(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, c1.Code, pending[0].Code)
}

func TestConsumeCode_HappyPath(t *testing.T) {
	svc, db := newBindingService(t)
	ctx := context.Background()

	issued, err := svc.IssueCode(ctx, 9, "tg-main", 5*time.Minute)
	require.NoError(t, err)

	binding, err := svc.ConsumeCode(ctx, issued.Code, "telegram", "user-42")
	require.NoError(t, err)
	assert.NotZero(t, binding.ID)
	assert.Equal(t, uint(9), binding.ConfID)
	assert.Equal(t, "telegram", binding.ChannelType)
	assert.Equal(t, "user-42", binding.ChannelUserID)
	assert.Equal(t, "tg-main", binding.Label)
	assert.Equal(t, "zh", binding.ReplyLang)
	assert.True(t, binding.PtAdmin)
	assert.True(t, binding.Allowed)

	// 验证 binding 写入
	var bRows []models.ChannelBinding
	require.NoError(t, db.Find(&bRows).Error)
	require.Len(t, bRows, 1)
	assert.True(t, bRows[0].PtAdmin)
	assert.True(t, bRows[0].Allowed)

	// 验证 token used_at
	var token models.BotToken
	require.NoError(t, db.Where("code_or_token_hash = ?", issued.Code).First(&token).Error)
	require.NotNil(t, token.UsedAt)
}

func TestConsumeCode_Expired(t *testing.T) {
	svc, db := newBindingService(t)
	ctx := context.Background()

	issued, err := svc.IssueCode(ctx, 1, "x", 5*time.Minute)
	require.NoError(t, err)

	require.NoError(t, db.Model(&models.BotToken{}).
		Where("code_or_token_hash = ?", issued.Code).
		UpdateColumn("expires_at", time.Now().Add(-time.Minute)).Error)

	_, err = svc.ConsumeCode(ctx, issued.Code, "telegram", "u")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCodeUsedOrExpired))
}

func TestConsumeCode_Used(t *testing.T) {
	svc, _ := newBindingService(t)
	ctx := context.Background()

	issued, err := svc.IssueCode(ctx, 1, "x", 5*time.Minute)
	require.NoError(t, err)

	_, err = svc.ConsumeCode(ctx, issued.Code, "telegram", "u1")
	require.NoError(t, err)

	_, err = svc.ConsumeCode(ctx, issued.Code, "telegram", "u2")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCodeUsedOrExpired))
}

func TestConsumeCode_Unknown(t *testing.T) {
	svc, _ := newBindingService(t)
	ctx := context.Background()

	_, err := svc.ConsumeCode(ctx, "NOTEXIST", "telegram", "u")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCodeUsedOrExpired))
}

func TestConsumeCode_Race(t *testing.T) {
	svc, _ := newBindingService(t)
	ctx := context.Background()

	issued, err := svc.IssueCode(ctx, 1, "race-test", 5*time.Minute)
	require.NoError(t, err)

	const goroutines = 2
	results := make(chan error, goroutines)
	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			_, e := svc.ConsumeCode(ctx, issued.Code, "telegram", "user-"+string(rune('A'+idx)))
			results <- e
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	var successes, expired int
	for e := range results {
		if e == nil {
			successes++
		} else if errors.Is(e, ErrCodeUsedOrExpired) {
			expired++
		} else {
			t.Fatalf("unexpected error: %v", e)
		}
	}
	assert.Equal(t, 1, successes, "exactly one goroutine should succeed")
	assert.Equal(t, 1, expired, "the other should get ErrCodeUsedOrExpired")
}

func TestListBindings(t *testing.T) {
	svc, _ := newBindingService(t)
	ctx := context.Background()

	c1, err := svc.IssueCode(ctx, 1, "a", 5*time.Minute)
	require.NoError(t, err)
	_, err = svc.ConsumeCode(ctx, c1.Code, "telegram", "ua")
	require.NoError(t, err)

	c2, err := svc.IssueCode(ctx, 2, "b", 5*time.Minute)
	require.NoError(t, err)
	_, err = svc.ConsumeCode(ctx, c2.Code, "qq", "ub")
	require.NoError(t, err)

	list, err := svc.ListBindings(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
}

func TestRevoke(t *testing.T) {
	svc, db := newBindingService(t)
	ctx := context.Background()

	c, err := svc.IssueCode(ctx, 1, "a", 5*time.Minute)
	require.NoError(t, err)
	binding, err := svc.ConsumeCode(ctx, c.Code, "telegram", "u")
	require.NoError(t, err)

	require.NoError(t, svc.Revoke(ctx, binding.ID))

	var count int64
	require.NoError(t, db.Model(&models.ChannelBinding{}).Where("id = ?", binding.ID).Count(&count).Error)
	assert.Equal(t, int64(0), count, "binding row should be deleted")
}

func TestSetReplyLang(t *testing.T) {
	svc, db := newBindingService(t)
	ctx := context.Background()

	c, err := svc.IssueCode(ctx, 1, "a", 5*time.Minute)
	require.NoError(t, err)
	binding, err := svc.ConsumeCode(ctx, c.Code, "telegram", "u")
	require.NoError(t, err)

	require.NoError(t, svc.SetReplyLang(ctx, binding.ID, "en"))

	var got models.ChannelBinding
	require.NoError(t, db.First(&got, binding.ID).Error)
	assert.Equal(t, "en", got.ReplyLang)
}

func TestSetReplyLang_Invalid(t *testing.T) {
	svc, _ := newBindingService(t)
	ctx := context.Background()

	c, err := svc.IssueCode(ctx, 1, "a", 5*time.Minute)
	require.NoError(t, err)
	binding, err := svc.ConsumeCode(ctx, c.Code, "telegram", "u")
	require.NoError(t, err)

	err = svc.SetReplyLang(ctx, binding.ID, "fr")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidReplyLang))
}

func TestIssueCode_PermanentTTL(t *testing.T) {
	svc, db := newBindingService(t)
	ctx := context.Background()

	dto, err := svc.IssueCode(ctx, 1, "perma", 0)
	require.NoError(t, err)
	require.Nil(t, dto.ExpiresAt, "permanent code DTO must have nil ExpiresAt")

	var row models.BotToken
	require.NoError(t, db.First(&row, "code_or_token_hash = ?", dto.Code).Error)
	require.Nil(t, row.ExpiresAt, "permanent code DB row must have NULL expires_at")
}

func TestIssueCode_CustomTTL(t *testing.T) {
	svc, _ := newBindingService(t)
	ctx := context.Background()

	dto, err := svc.IssueCode(ctx, 1, "1d", 24*time.Hour)
	require.NoError(t, err)
	require.NotNil(t, dto.ExpiresAt)
	assert.WithinDuration(t, time.Now().Add(24*time.Hour), *dto.ExpiresAt, 2*time.Second)
}

func TestListPendingCodes_PermanentVisible(t *testing.T) {
	svc, _ := newBindingService(t)
	ctx := context.Background()

	c, err := svc.IssueCode(ctx, 1, "perma", 0)
	require.NoError(t, err)

	pending, err := svc.ListPendingCodes(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, c.Code, pending[0].Code)
	assert.Nil(t, pending[0].ExpiresAt)
}

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

func TestConsumeCode_LookupError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.BotToken{}, &models.ChannelBinding{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	svc := NewBindingService(db, "admin")
	_, err = svc.ConsumeCode(context.Background(), "code", "telegram", "u")
	require.Error(t, err)
}

func TestRevoke_DBError(t *testing.T) {
	db := setupClosedNotifDB(t)
	svc := NewBindingService(db, "admin")
	err := svc.Revoke(context.Background(), 1)
	require.Error(t, err)
}
