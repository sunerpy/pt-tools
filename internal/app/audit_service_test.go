// MIT License
// Copyright (c) 2025 pt-tools

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

func TestQuery_CommandAndResultFilters(t *testing.T) {
	db := setupAuditTestDB(t)
	svc := NewAuditService(db)
	now := time.Now()
	rows := []models.ActionAudit{
		{NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1", Command: "ping", ArgsJSON: "{}", Result: "ok", CreatedAt: now.Add(-3 * time.Minute)},
		{NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1", Command: "bind", ArgsJSON: "{}", Result: "error", CreatedAt: now.Add(-2 * time.Minute)},
	}
	for i := range rows {
		require.NoError(t, db.Create(&rows[i]).Error)
	}

	items, total, err := svc.Query(context.Background(), AuditQuery{Command: "bind"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, "bind", items[0].Command)

	items, total, err = svc.Query(context.Background(), AuditQuery{Result: "error"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, "error", items[0].Result)
}

// 画板 25 的 q 写的是「筛选命令、触发用户…」：一个词要同时在命令与触发用户里模糊匹配。
//
// 为什么要在服务端：这个接口是分页的，前端在本页里筛会让页脚的 total 与表里的行数对不上，
// 而且用户要找的那条很可能不在当前这一页。Command 那个精确匹配连「命令名写一半」都搜不到。
func TestQuery_KeywordMatchesCommandOrChannelUser(t *testing.T) {
	db := setupAuditTestDB(t)
	svc := NewAuditService(db)
	now := time.Now()
	rows := []models.ActionAudit{
		{NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "alice", Command: "site list", ArgsJSON: "{}", Result: "ok", CreatedAt: now.Add(-3 * time.Minute)},
		{NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "bob", Command: "task push", ArgsJSON: "{}", Result: "ok", CreatedAt: now.Add(-2 * time.Minute)},
	}
	for i := range rows {
		require.NoError(t, db.Create(&rows[i]).Error)
	}

	// 命令名写一半也要命中（精确匹配做不到这件事）
	items, total, err := svc.Query(context.Background(), AuditQuery{Keyword: "push"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, "task push", items[0].Command)

	// 按触发用户找
	items, total, err = svc.Query(context.Background(), AuditQuery{Keyword: "ali"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, "alice", items[0].ChannelUserID)

	// 只有空白等于不筛
	_, total, err = svc.Query(context.Background(), AuditQuery{Keyword: "   "})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
}

func TestQuery_TimeWindowAndPaginationDefaults(t *testing.T) {
	db := setupAuditTestDB(t)
	svc := NewAuditService(db)
	now := time.Now()
	require.NoError(t, db.Create(&models.ActionAudit{
		NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1",
		Command: "ping", ArgsJSON: "{}", Result: "ok", CreatedAt: now.Add(-10 * time.Minute),
	}).Error)
	require.NoError(t, db.Create(&models.ActionAudit{
		NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1",
		Command: "ping", ArgsJSON: "{}", Result: "ok", CreatedAt: now.Add(-1 * time.Minute),
	}).Error)

	// Since window keeps only the recent row; negative page/pageSize clamp to defaults.
	items, total, err := svc.Query(context.Background(), AuditQuery{
		Since: now.Add(-5 * time.Minute), Page: -1, PageSize: -1,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)

	// Until window keeps only the old row.
	_, total, err = svc.Query(context.Background(), AuditQuery{Until: now.Add(-5 * time.Minute)})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
}

/*
 * 审计页的时间窗来自前端 toISOString()，到这里是 UTC 的 time.Time；
 * 而 Record 用 time.Now() 写库，是进程本地时区。glebarez/sqlite 绑定 time.Time 时
 * 按值自带的时区格式化成「2006-01-02 15:04:05.999999999-07:00」文本，再做字符串比较 ——
 * 两边时区不同，窗口就整体错开一个时差。官方镜像 TZ=Asia/Shanghai 时错 8 小时：
 * 「最近 1 小时」里查不到刚写入的记录，反而会查到 8 小时前的。
 *
 * 改 time.Local 是进程级全局状态：这条测试不能 t.Parallel，结束时由 t.Cleanup 还原。
 */
func TestQuery_UTCWindowMatchesRowsWrittenInLocalZone(t *testing.T) {
	origLocal := time.Local
	time.Local = time.FixedZone("UTC+8", 8*60*60)
	t.Cleanup(func() { time.Local = origLocal })

	db := setupAuditTestDB(t)
	svc := NewAuditService(db)
	ctx := context.Background()

	require.NoError(t, svc.Record(ctx, AuditEntry{
		NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1",
		Command: "fresh", Result: "success",
	}))
	// 8 小时前写入的旧记录：时区按错时，它恰好会落进 UTC 表示的「最近 1 小时」
	require.NoError(t, db.Create(&models.ActionAudit{
		NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1",
		Command: "stale", ArgsJSON: "{}", Result: "success",
		CreatedAt: time.Now().Add(-8 * time.Hour),
	}).Error)

	commandsIn := func(since, until time.Time) []string {
		t.Helper()
		items, total, err := svc.Query(ctx, AuditQuery{Since: since, Until: until})
		require.NoError(t, err)
		require.Len(t, items, total)
		out := make([]string, 0, len(items))
		for _, it := range items {
			out = append(out, it.Command)
		}
		return out
	}

	now := time.Now().UTC()
	assert.Equal(t, []string{"fresh"}, commandsIn(now.Add(-time.Hour), now.Add(time.Minute)),
		"UTC 的「最近 1 小时」必须查到刚写入的记录，且不能混进 8 小时前的记录")
	assert.Empty(t, commandsIn(now.Add(-2*time.Hour), now.Add(-time.Hour)),
		"窗口外（2 小时前到 1 小时前）没有记录")
	assert.Equal(t, []string{"stale"}, commandsIn(now.Add(-9*time.Hour), now.Add(-7*time.Hour)),
		"8 小时前的记录只能落在它自己的时间窗里")
}

func TestRecord_NilArgs(t *testing.T) {
	db := setupAuditTestDB(t)
	svc := NewAuditService(db)
	require.NoError(t, svc.Record(context.Background(), AuditEntry{
		NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1",
		Command: "ping", Result: "ok", LatencyMs: 5,
	}))
	var cnt int64
	require.NoError(t, db.Model(&models.ActionAudit{}).Count(&cnt).Error)
	assert.EqualValues(t, 1, cnt)
}

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

func TestAuditService_Stats(t *testing.T) {
	t.Run("empty DB returns zeros", func(t *testing.T) {
		db := setupAuditTestDB(t)
		svc := NewAuditService(db)

		got, err := svc.Stats(context.Background())
		require.NoError(t, err)
		assert.EqualValues(t, 0, got.TotalCount)
		assert.EqualValues(t, 0, got.TodayCount)
		assert.EqualValues(t, 0, got.SuccessRate)
		assert.EqualValues(t, 0, got.MaxLatencyMs)
		assert.EqualValues(t, 0, got.AvgLatencyMs)
	})

	t.Run("3 success + 1 error gives 75% success rate and correct max latency", func(t *testing.T) {
		db := setupAuditTestDB(t)
		svc := NewAuditService(db)

		now := time.Now()
		rows := []models.ActionAudit{
			{NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1", Command: "ping", ArgsJSON: "{}", Result: "success", LatencyMs: 10, CreatedAt: now.Add(-3 * time.Minute)},
			{NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1", Command: "ping", ArgsJSON: "{}", Result: "success", LatencyMs: 20, CreatedAt: now.Add(-2 * time.Minute)},
			{NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1", Command: "ping", ArgsJSON: "{}", Result: "success", LatencyMs: 30, CreatedAt: now.Add(-1 * time.Minute)},
			{NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1", Command: "ping", ArgsJSON: "{}", Result: "error", LatencyMs: 100, CreatedAt: now.Add(-30 * time.Second)},
		}
		for i := range rows {
			require.NoError(t, db.Create(&rows[i]).Error)
		}

		got, err := svc.Stats(context.Background())
		require.NoError(t, err)
		assert.EqualValues(t, 4, got.TotalCount)
		assert.InDelta(t, 75.0, got.SuccessRate, 0.001)
		assert.EqualValues(t, 100, got.MaxLatencyMs)
		assert.InDelta(t, 40.0, got.AvgLatencyMs, 0.001)
	})

	t.Run("today_count counts only rows since local midnight", func(t *testing.T) {
		db := setupAuditTestDB(t)
		svc := NewAuditService(db)

		now := time.Now()
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		yesterday := midnight.Add(-2 * time.Hour)
		earlierToday := midnight.Add(1 * time.Hour)

		require.NoError(t, db.Create(&models.ActionAudit{
			NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1",
			Command: "ping", ArgsJSON: "{}", Result: "success", CreatedAt: yesterday,
		}).Error)
		require.NoError(t, db.Create(&models.ActionAudit{
			NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1",
			Command: "ping", ArgsJSON: "{}", Result: "success", CreatedAt: earlierToday,
		}).Error)
		require.NoError(t, db.Create(&models.ActionAudit{
			NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1",
			Command: "ping", ArgsJSON: "{}", Result: "denied", CreatedAt: now,
		}).Error)

		got, err := svc.Stats(context.Background())
		require.NoError(t, err)
		assert.EqualValues(t, 3, got.TotalCount)
		assert.EqualValues(t, 2, got.TodayCount)
	})
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

/*
 * 钉子：结果与通道筛选必须按**生产里真实存在的值**筛。
 *
 * 四个真实缺陷，都是「控件在，但筛出来是空」这一类：
 *   ① 前端多选把「success,error」逗号拼起来发过来，服务端按单值 `result = ?` 匹配，
 *      这个组合一行都匹配不到 —— 界面上「筛完什么都没有」，读起来像真的没有记录；
 *   ② handler 压根没读 channel_type，那枚通道筛选是个纯装饰的空控件；
 *   ③ 接通之后前端发的是 `qq` / `wecom`，而生产写入的是适配器 Type() 的返回值
 *      `qq_onebot` / `wecom_webhook` —— 选「QQ」把真实 QQ 记录筛成零条；
 *   ④ 结果在生产里带原因后缀（`denied:not_bound` / `error:lookup_binding`，
 *      只有 success 是裸值），按等值筛 `denied` 同样零条。
 *
 * 所以这里的造数**只用生产链真实写入的值**（见 internal/chatops/message_chain.go
 * 与各适配器的 Type()）—— 上一版用 "qq" / 裸 "denied" 造数，等于在自己造的世界里通过。
 */
func TestQuery_ResultAndChannelTypeUseProductionValues(t *testing.T) {
	db := setupAuditTestDB(t)
	svc := NewAuditService(db)
	now := time.Now()
	rows := []models.ActionAudit{
		{NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "a", Command: "c1", ArgsJSON: "{}", Result: "success", CreatedAt: now.Add(-6 * time.Minute)},
		{NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "b", Command: "c2", ArgsJSON: "{}", Result: "denied:not_bound", CreatedAt: now.Add(-5 * time.Minute)},
		{NotificationConfID: 2, ChannelType: "qq_onebot", ChannelUserID: "c", Command: "c3", ArgsJSON: "{}", Result: "error:lookup_binding", CreatedAt: now.Add(-4 * time.Minute)},
		{NotificationConfID: 2, ChannelType: "qq_onebot", ChannelUserID: "d", Command: "c4", ArgsJSON: "{}", Result: "success", CreatedAt: now.Add(-3 * time.Minute)},
		{NotificationConfID: 3, ChannelType: "wecom_webhook", ChannelUserID: "e", Command: "c5", ArgsJSON: "{}", Result: "denied:rate_limit", CreatedAt: now.Add(-2 * time.Minute)},
	}
	for i := range rows {
		require.NoError(t, db.Create(&rows[i]).Error)
	}

	// 裸值照旧
	_, total, err := svc.Query(context.Background(), AuditQuery{Result: "success"})
	require.NoError(t, err)
	assert.Equal(t, 2, total)

	// 带后缀的要按前缀命中 —— 这是分段器上「被拒绝」那一档
	_, total, err = svc.Query(context.Background(), AuditQuery{Result: "denied"})
	require.NoError(t, err)
	assert.Equal(t, 2, total, "denied 要命中 denied:not_bound / denied:rate_limit")

	_, total, err = svc.Query(context.Background(), AuditQuery{Result: "error"})
	require.NoError(t, err)
	assert.Equal(t, 1, total, "error 要命中 error:lookup_binding")

	// 多值：原先一行都匹配不到的那种入参
	_, total, err = svc.Query(context.Background(), AuditQuery{Result: "success,error"})
	require.NoError(t, err)
	assert.Equal(t, 3, total, "「成功 + 出错」要拿到三条，不是零条")

	// 通道筛选按生产 ID 筛
	_, total, err = svc.Query(context.Background(), AuditQuery{ChannelType: "qq_onebot"})
	require.NoError(t, err)
	assert.Equal(t, 2, total)

	_, total, err = svc.Query(context.Background(), AuditQuery{ChannelType: "wecom_webhook"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)

	// 旧短名也归一到生产 ID，而不是静默返回空
	_, total, err = svc.Query(context.Background(), AuditQuery{ChannelType: "qq"})
	require.NoError(t, err)
	assert.Equal(t, 2, total, "短名 qq 要归一到 qq_onebot")

	_, total, err = svc.Query(context.Background(), AuditQuery{ChannelType: "wecom"})
	require.NoError(t, err)
	assert.Equal(t, 1, total, "短名 wecom 要归一到 wecom_webhook")

	_, total, err = svc.Query(context.Background(), AuditQuery{ChannelType: "telegram,qq_onebot"})
	require.NoError(t, err)
	assert.Equal(t, 4, total)

	// 两条筛选叠着走 AND
	_, total, err = svc.Query(context.Background(), AuditQuery{ChannelType: "qq_onebot", Result: "success"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)

	// 空白项丢掉，不会退化成「匹配空字符串」
	_, total, err = svc.Query(context.Background(), AuditQuery{Result: " , "})
	require.NoError(t, err)
	assert.Equal(t, 5, total, "全是空白等于不筛")

	// 前缀匹配不能扩大：denied 不许把 error:* 也捞进来
	items, _, err := svc.Query(context.Background(), AuditQuery{Result: "denied"})
	require.NoError(t, err)
	for _, it := range items {
		assert.True(t, strings.HasPrefix(it.Result, "denied"), "命中了不该命中的 %q", it.Result)
	}
}
