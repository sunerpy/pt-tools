package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// setupAuditTestDB 创建独立的 in-memory SQLite，仅 AutoMigrate ActionAudit 表。
func setupAuditTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "open sqlite memory")
	require.NoError(t, db.AutoMigrate(&models.ActionAudit{}), "automigrate action_audit")
	return db
}

// TestAuditRedaction_Token 验证 token 字段被替换为 [REDACTED]。
func TestAuditRedaction_Token(t *testing.T) {
	in := map[string]any{"token": "secret123", "name": "alice"}
	got := redact(in)
	assert.Equal(t, "[REDACTED]", got["token"])
	assert.Equal(t, "alice", got["name"])
	// 原始 map 不应被修改
	assert.Equal(t, "secret123", in["token"], "原始 args 必须保持不可变")
}

// TestAuditRedaction_NestedPasskey 验证嵌套 map 中 passkey 也被 redact。
func TestAuditRedaction_NestedPasskey(t *testing.T) {
	in := map[string]any{
		"site": map[string]any{
			"passkey": "x",
			"url":     "https://example.com",
		},
	}
	got := redact(in)
	site, ok := got["site"].(map[string]any)
	require.True(t, ok, "site 应为 map")
	assert.Equal(t, "[REDACTED]", site["passkey"])
	assert.Equal(t, "https://example.com", site["url"])
	// 原始嵌套 map 不应被修改
	origSite := in["site"].(map[string]any)
	assert.Equal(t, "x", origSite["passkey"], "原始嵌套 map 必须保持不可变")
}

// TestAuditRedaction_CaseInsensitive 验证大写键名也被 redact。
func TestAuditRedaction_CaseInsensitive(t *testing.T) {
	in := map[string]any{
		"PASSWORD":    "x",
		"Cookie":      "session=abc",
		"API_Key":     "k1",
		"MySecretVal": "s",
	}
	got := redact(in)
	assert.Equal(t, "[REDACTED]", got["PASSWORD"])
	assert.Equal(t, "[REDACTED]", got["Cookie"])
	assert.Equal(t, "[REDACTED]", got["API_Key"])
	assert.Equal(t, "[REDACTED]", got["MySecretVal"])
}

// TestAuditRedaction_NoFalsePositive 验证 username 等无关键键不被误 redact。
func TestAuditRedaction_NoFalsePositive(t *testing.T) {
	in := map[string]any{
		"username": "alice",
		"email":    "alice@example.com",
		"site_id":  uint(123),
	}
	got := redact(in)
	assert.Equal(t, "alice", got["username"])
	assert.Equal(t, "alice@example.com", got["email"])
	assert.Equal(t, uint(123), got["site_id"])
}

// TestAuditRedaction_SliceOfMaps 验证切片内 map 中敏感字段被 redact。
func TestAuditRedaction_SliceOfMaps(t *testing.T) {
	in := map[string]any{
		"sites": []any{
			map[string]any{"name": "s1", "passkey": "p1"},
			map[string]any{"name": "s2", "token": "t2"},
		},
	}
	got := redact(in)
	sites, ok := got["sites"].([]any)
	require.True(t, ok)
	require.Len(t, sites, 2)
	s1 := sites[0].(map[string]any)
	s2 := sites[1].(map[string]any)
	assert.Equal(t, "s1", s1["name"])
	assert.Equal(t, "[REDACTED]", s1["passkey"])
	assert.Equal(t, "s2", s2["name"])
	assert.Equal(t, "[REDACTED]", s2["token"])
}

// TestPrune_Time 验证 created_at < now-90d 的记录被删除。
func TestPrune_Time(t *testing.T) {
	db := setupAuditTestDB(t)
	svc := NewAuditService(db)

	now := time.Now()
	old := now.Add(-100 * 24 * time.Hour)
	recent := now.Add(-1 * 24 * time.Hour)

	// 插入 5 行老记录 + 3 行新记录
	for i := 0; i < 5; i++ {
		require.NoError(t, db.Create(&models.ActionAudit{
			NotificationConfID: 1,
			ChannelType:        "telegram",
			ChannelUserID:      "u1",
			Command:            "ping",
			ArgsJSON:           "{}",
			Result:             "ok",
			CreatedAt:          old,
		}).Error)
	}
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&models.ActionAudit{
			NotificationConfID: 1,
			ChannelType:        "telegram",
			ChannelUserID:      "u1",
			Command:            "ping",
			ArgsJSON:           "{}",
			Result:             "ok",
			CreatedAt:          recent,
		}).Error)
	}

	deleted, err := svc.Prune(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 5, deleted, "应删除 5 行老记录")

	var remaining int64
	require.NoError(t, db.Model(&models.ActionAudit{}).Count(&remaining).Error)
	assert.EqualValues(t, 3, remaining, "应剩余 3 行新记录")
}

// TestPrune_Cap 验证行数超过 cap 时删除最早溢出量。
// 测试用 cap=50 + 100 行验证逻辑（不真插 500k）。
func TestPrune_Cap(t *testing.T) {
	db := setupAuditTestDB(t)
	svc := NewAuditServiceWithCap(db, 50, 90*24*time.Hour)

	// 插入 100 行，CreatedAt 递增（最早 i=0，最新 i=99）
	base := time.Now().Add(-1 * time.Hour)
	for i := 0; i < 100; i++ {
		require.NoError(t, db.Create(&models.ActionAudit{
			NotificationConfID: 1,
			ChannelType:        "telegram",
			ChannelUserID:      "u1",
			Command:            "ping",
			ArgsJSON:           "{}",
			Result:             "ok",
			CreatedAt:          base.Add(time.Duration(i) * time.Second),
		}).Error)
	}

	deleted, err := svc.Prune(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 50, deleted, "应删除 50 行最早记录")

	var remaining int64
	require.NoError(t, db.Model(&models.ActionAudit{}).Count(&remaining).Error)
	assert.EqualValues(t, 50, remaining, "应剩余 50 行")

	// 验证保留的是最新的 50 行
	var oldest models.ActionAudit
	require.NoError(t, db.Order("created_at ASC").First(&oldest).Error)
	expectedOldestIdx := 50 // i=50 是第 51 行
	expectedTime := base.Add(time.Duration(expectedOldestIdx) * time.Second)
	assert.WithinDuration(t, expectedTime, oldest.CreatedAt, time.Second)
}

// TestRecord_PersistsRedactedArgs 验证 Record 写入 DB 后 ArgsJSON 已经被 redact。
func TestRecord_PersistsRedactedArgs(t *testing.T) {
	db := setupAuditTestDB(t)
	svc := NewAuditService(db)

	err := svc.Record(context.Background(), AuditEntry{
		NotificationConfID: 1,
		ChannelType:        "telegram",
		ChannelUserID:      "u1",
		Command:            "bind",
		Args: map[string]any{
			"token":    "supersecret-abc-123",
			"username": "alice",
		},
		Result:    "ok",
		LatencyMs: 42,
	})
	require.NoError(t, err)

	var rows []models.ActionAudit
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)

	row := rows[0]
	assert.Equal(t, "bind", row.Command)
	assert.Equal(t, "ok", row.Result)
	assert.EqualValues(t, 42, row.LatencyMs)

	assert.Contains(t, row.ArgsJSON, "[REDACTED]", "ArgsJSON 应包含 [REDACTED]")
	assert.NotContains(t, row.ArgsJSON, "supersecret-abc-123", "ArgsJSON 不应包含原始 token")
	assert.Contains(t, row.ArgsJSON, "alice", "ArgsJSON 应保留非敏感字段")

	// 反序列化回 map 再校验
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(row.ArgsJSON), &got))
	assert.Equal(t, "[REDACTED]", got["token"])
	assert.Equal(t, "alice", got["username"])
}

// TestQuery_FilterAndPaginate 验证 Query 支持 channel_user_id / command / result / 时间窗 / 分页。
func TestQuery_FilterAndPaginate(t *testing.T) {
	db := setupAuditTestDB(t)
	svc := NewAuditService(db)

	now := time.Now()
	// 插入 5 行混合数据
	rows := []models.ActionAudit{
		{NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1", Command: "ping", ArgsJSON: "{}", Result: "ok", CreatedAt: now.Add(-5 * time.Minute)},
		{NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1", Command: "ping", ArgsJSON: "{}", Result: "ok", CreatedAt: now.Add(-4 * time.Minute)},
		{NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u2", Command: "ping", ArgsJSON: "{}", Result: "ok", CreatedAt: now.Add(-3 * time.Minute)},
		{NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1", Command: "bind", ArgsJSON: "{}", Result: "error", CreatedAt: now.Add(-2 * time.Minute)},
		{NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1", Command: "bind", ArgsJSON: "{}", Result: "ok", CreatedAt: now.Add(-1 * time.Minute)},
	}
	for i := range rows {
		require.NoError(t, db.Create(&rows[i]).Error)
	}

	// 过滤 user=u1 → 4 行
	items, total, err := svc.Query(context.Background(), AuditQuery{
		ChannelUserID: "u1",
		Page:          1,
		PageSize:      10,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 4, total)
	assert.Len(t, items, 4)

	// 过滤 command=bind + result=ok → 1 行
	items2, total2, err := svc.Query(context.Background(), AuditQuery{
		Command:  "bind",
		Result:   "ok",
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total2)
	require.Len(t, items2, 1)
	assert.Equal(t, "bind", items2[0].Command)
	assert.Equal(t, "ok", items2[0].Result)

	// 分页：PageSize=2, Page=1 → 应返回 2 行（最新优先）
	items3, total3, err := svc.Query(context.Background(), AuditQuery{
		Page:     1,
		PageSize: 2,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 5, total3)
	assert.Len(t, items3, 2)
	// 倒序：第一行应是最新（u1 bind ok）
	assert.True(t, strings.HasPrefix(items3[0].ChannelUserID, "u"), "应有 channel_user_id")
	assert.True(t, items3[0].CreatedAt.After(items3[1].CreatedAt) || items3[0].CreatedAt.Equal(items3[1].CreatedAt))
}
