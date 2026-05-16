package models

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupChatOpsTestDB 创建独立的 in-memory SQLite，仅 AutoMigrate ChatOps 五张表
// 用于隔离 ChatOps 模型相关测试，避免引入其他模型的副作用。
func setupChatOpsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "open sqlite memory")
	require.NoError(t, db.AutoMigrate(
		&NotificationConf{},
		&ChannelBinding{},
		&ActionAudit{},
		&BotToken{},
		&NotificationOutbox{},
	), "automigrate chatops models")
	return db
}

// TestAutoMigrate_CreatesAllTables 验证全量 AutoMigrate（含 NewDB 流程使用的所有模型）
// 能在干净的 :memory: SQLite 上成功创建 5 张 ChatOps 新表，使用单数表名。
func TestAutoMigrate_CreatesAllTables(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// 与 init.go 保持一致的全量迁移（确保新表与旧表共存）
	require.NoError(t, db.AutoMigrate(
		&SchemaVersion{},
		&TorrentInfo{},
		&TorrentInfoArchive{},
		&AdminUser{},
		&SettingsGlobal{},
		&QbitSettings{},
		&SiteSetting{},
		&RSSSubscription{},
		&DownloaderSetting{},
		&DownloaderDirectory{},
		&SiteTemplate{},
		&FilterRule{},
		&RSSFilterAssociation{},
		&FaviconCache{},
		&SiteRateLimit{},
		// ChatOps 五张新表
		&NotificationConf{},
		&ChannelBinding{},
		&ActionAudit{},
		&BotToken{},
		&NotificationOutbox{},
	))

	migrator := db.Migrator()
	for _, name := range []string{
		"notification_conf",
		"channel_binding",
		"action_audit",
		"bot_token",
		"notification_outbox",
	} {
		assert.Truef(t, migrator.HasTable(name), "table %q should exist", name)
	}
}

// TestTableName_Singular 验证 5 个 ChatOps model 都返回单数表名，
// 避免 GORM 默认复数化与项目命名风格不一致。
func TestTableName_Singular(t *testing.T) {
	cases := []struct {
		model interface{ TableName() string }
		want  string
	}{
		{NotificationConf{}, "notification_conf"},
		{ChannelBinding{}, "channel_binding"},
		{ActionAudit{}, "action_audit"},
		{BotToken{}, "bot_token"},
		{NotificationOutbox{}, "notification_outbox"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, tc.model.TableName())
	}
}

// TestNotificationConf_RoundTrip 验证 NotificationConf 的 CRUD 行为。
func TestNotificationConf_RoundTrip(t *testing.T) {
	db := setupChatOpsTestDB(t)

	conf := NotificationConf{
		ChannelType: "telegram",
		Name:        "tg-main",
		ConfigJSON:  `{"bot_token":"plain"}`,
		Enabled:     true,
	}
	require.NoError(t, db.Create(&conf).Error)
	require.NotZero(t, conf.ID)
	require.False(t, conf.CreatedAt.IsZero())
	require.False(t, conf.UpdatedAt.IsZero())

	var got NotificationConf
	require.NoError(t, db.First(&got, conf.ID).Error)
	assert.Equal(t, "telegram", got.ChannelType)
	assert.Equal(t, "tg-main", got.Name)
	assert.Equal(t, `{"bot_token":"plain"}`, got.ConfigJSON)
	assert.True(t, got.Enabled)

	require.NoError(t, db.Model(&got).Update("enabled", false).Error)
	var updated NotificationConf
	require.NoError(t, db.First(&updated, conf.ID).Error)
	assert.False(t, updated.Enabled)

	require.NoError(t, db.Delete(&NotificationConf{}, conf.ID).Error)
	var count int64
	require.NoError(t, db.Model(&NotificationConf{}).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

// TestChannelBinding_Index 验证 channel_binding 表存在
// (notification_conf_id, channel_user_id) 的命名复合索引。
func TestChannelBinding_Index(t *testing.T) {
	db := setupChatOpsTestDB(t)

	type indexInfo struct {
		Name string
	}
	var idxs []indexInfo
	require.NoError(t, db.Raw(
		`SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = ?`,
		"channel_binding",
	).Scan(&idxs).Error)

	require.NotEmpty(t, idxs, "channel_binding 应有至少一个索引")

	found := false
	for _, idx := range idxs {
		if idx.Name == "idx_channel_binding_conf_user" {
			found = true
			break
		}
	}
	assert.True(t, found, "应存在命名索引 idx_channel_binding_conf_user，实际索引: %+v", idxs)

	// 进一步验证索引覆盖列：通过 PRAGMA index_info 查询
	type indexCol struct {
		Name string
	}
	var cols []indexCol
	require.NoError(t, db.Raw(`PRAGMA index_info("idx_channel_binding_conf_user")`).Scan(&cols).Error)
	colNames := make([]string, 0, len(cols))
	for _, c := range cols {
		colNames = append(colNames, c.Name)
	}
	assert.Contains(t, colNames, "notification_conf_id")
	assert.Contains(t, colNames, "channel_user_id")
}

// TestActionAudit_DescIndex 验证 action_audit 表存在
// (notification_conf_id, channel_user_id, created_at) 的命名复合索引，用于按时间倒序查询。
func TestActionAudit_DescIndex(t *testing.T) {
	db := setupChatOpsTestDB(t)

	type indexInfo struct {
		Name string
	}
	var idxs []indexInfo
	require.NoError(t, db.Raw(
		`SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = ?`,
		"action_audit",
	).Scan(&idxs).Error)
	require.NotEmpty(t, idxs)

	found := false
	for _, idx := range idxs {
		if idx.Name == "idx_action_audit_conf_user_time" {
			found = true
			break
		}
	}
	assert.True(t, found, "应存在命名索引 idx_action_audit_conf_user_time，实际索引: %+v", idxs)

	type indexCol struct {
		Name string
	}
	var cols []indexCol
	require.NoError(t, db.Raw(`PRAGMA index_info("idx_action_audit_conf_user_time")`).Scan(&cols).Error)
	colNames := make([]string, 0, len(cols))
	for _, c := range cols {
		colNames = append(colNames, c.Name)
	}
	assert.Contains(t, colNames, "notification_conf_id")
	assert.Contains(t, colNames, "channel_user_id")
	assert.Contains(t, colNames, "created_at")
}

// TestNotificationOutbox_StatusTransitions 验证 outbox 4 种状态的写入与读取，
// 并验证 (status, next_retry_at) 索引存在以支持 worker 扫描。
func TestNotificationOutbox_StatusTransitions(t *testing.T) {
	db := setupChatOpsTestDB(t)

	now := time.Now()
	rows := []NotificationOutbox{
		{NotificationConfID: 1, PayloadJSON: `{"a":1}`, Status: "pending", RetryCount: 0, NextRetryAt: now},
		{NotificationConfID: 1, PayloadJSON: `{"a":2}`, Status: "sent", RetryCount: 1, NextRetryAt: now.Add(time.Minute), SentAt: ptrTime(now.Add(time.Second))},
		{NotificationConfID: 1, PayloadJSON: `{"a":3}`, Status: "failed", RetryCount: 2, NextRetryAt: now.Add(2 * time.Minute), ErrorMsg: "network"},
		{NotificationConfID: 1, PayloadJSON: `{"a":4}`, Status: "dead", RetryCount: 3, NextRetryAt: now.Add(3 * time.Minute), ErrorMsg: "max retry"},
	}
	for i := range rows {
		require.NoError(t, db.Create(&rows[i]).Error)
	}

	for _, status := range []string{"pending", "sent", "failed", "dead"} {
		var count int64
		require.NoError(
			t,
			db.Model(&NotificationOutbox{}).Where("status = ?", status).Count(&count).Error,
			"count by status %s", status,
		)
		assert.Equal(t, int64(1), count, "status=%s 应有 1 条", status)
	}

	// 验证 (status, next_retry_at) 索引存在
	type indexInfo struct{ Name string }
	var idxs []indexInfo
	require.NoError(t, db.Raw(
		`SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = ?`,
		"notification_outbox",
	).Scan(&idxs).Error)

	found := false
	for _, idx := range idxs {
		if idx.Name == "idx_notification_outbox_status_retry" {
			found = true
			break
		}
	}
	assert.True(t, found, "应存在命名索引 idx_notification_outbox_status_retry，实际: %+v", idxs)
}

func ptrTime(t time.Time) *time.Time { return &t }
