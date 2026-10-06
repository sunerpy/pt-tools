package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// springsunday 已有同一种子 ID 的记录时：保留 springsunday 那条，删掉 cmct 的重复，其余改名。
// 原来整批改名撞唯一索引 (site_name, torrent_id)，错误被忽略，cmct 的种子记录全部留在 cmct 下，
// 而 cmct 站点已经删了。
func TestMigrateCmctToSpringSunday_TorrentConflictKeepsSpringRow(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{}, &TorrentInfo{})
	require.NoError(t, db.Create(&SiteSetting{Name: "cmct", AuthMethod: "cookie", Cookie: "ck"}).Error)
	require.NoError(t, db.Create(&SiteSetting{Name: "springsunday", AuthMethod: "cookie"}).Error)
	require.NoError(t, db.Create(&TorrentInfo{SiteName: "springsunday", TorrentID: "100", Title: "spring"}).Error)
	require.NoError(t, db.Create(&TorrentInfo{SiteName: "cmct", TorrentID: "100", Title: "cmct-dup"}).Error)
	require.NoError(t, db.Create(&TorrentInfo{SiteName: "cmct", TorrentID: "200", Title: "cmct-only"}).Error)

	require.NoError(t, SyncSitesFromRegistry(db, nil))

	var rows []TorrentInfo
	require.NoError(t, db.Order("torrent_id").Find(&rows).Error)
	require.Len(t, rows, 2)
	assert.Equal(t, "springsunday", rows[0].SiteName)
	assert.Equal(t, "spring", rows[0].Title, "同一种子保留 springsunday 原有的记录")
	assert.Equal(t, "springsunday", rows[1].SiteName)
	assert.Equal(t, "cmct-only", rows[1].Title)
}

// 任一步失败整体回滚：原来后两步的错误被忽略，cmct 站点照样删掉，用户数据只迁了一半。
func TestMigrateCmctToSpringSunday_FailureRollsBack(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{}, &TorrentInfo{})
	require.NoError(t, db.Exec("CREATE TABLE user_info (id INTEGER PRIMARY KEY, site TEXT, username TEXT)").Error)
	require.NoError(t, db.Exec("INSERT INTO user_info (site, username) VALUES ('cmct','alice')").Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER fail_user_info BEFORE UPDATE OF site ON user_info
		BEGIN SELECT RAISE(ABORT, 'disk I/O error'); END`).Error)
	require.NoError(t, db.Create(&SiteSetting{Name: "cmct", AuthMethod: "cookie", Cookie: "ck"}).Error)
	require.NoError(t, db.Create(&TorrentInfo{SiteName: "cmct", TorrentID: "t1"}).Error)

	require.Error(t, SyncSitesFromRegistry(db, nil))

	var sites []SiteSetting
	require.NoError(t, db.Find(&sites).Error)
	require.Len(t, sites, 1)
	assert.Equal(t, "cmct", sites[0].Name, "失败时站点改名也回滚，下次启动重试")
	var ti TorrentInfo
	require.NoError(t, db.Where("torrent_id = ?", "t1").First(&ti).Error)
	assert.Equal(t, "cmct", ti.SiteName)
}

// 迁移的每一步失败都整体回滚：cmct 站点留着，下次启动重试。
func TestMigrateCmctToSpringSunday_StepFailuresRollBack(t *testing.T) {
	const raise = "BEGIN SELECT RAISE(ABORT, 'disk I/O error'); END"
	cases := []struct {
		name   string
		spring bool     // springsunday 站点已存在（合并分支）
		setup  []string // 建数据与触发器的 SQL
	}{
		{
			name:   "合并：RSS 改挂失败",
			spring: true,
			setup: []string{
				"INSERT INTO rss_subscriptions (site_id, name, url) SELECT id, 'r', 'https://x/rss' FROM site_settings WHERE name = 'cmct'",
				"CREATE TRIGGER t BEFORE UPDATE OF site_id ON rss_subscriptions " + raise,
			},
		},
		{
			name:   "合并：保存 springsunday 失败",
			spring: true,
			setup:  []string{"CREATE TRIGGER t BEFORE UPDATE ON site_settings WHEN NEW.name = 'springsunday' " + raise},
		},
		{
			name:   "合并：删除 cmct 失败",
			spring: true,
			setup:  []string{"CREATE TRIGGER t BEFORE DELETE ON site_settings " + raise},
		},
		{
			name:  "改名：保存失败",
			setup: []string{"CREATE TRIGGER t BEFORE UPDATE ON site_settings " + raise},
		},
		{
			name:   "种子记录：删除重复失败",
			spring: true,
			setup: []string{
				"INSERT INTO torrent_infos (site_name, torrent_id) VALUES ('springsunday', '1'), ('cmct', '1')",
				"CREATE TRIGGER t BEFORE DELETE ON torrent_infos " + raise,
			},
		},
		{
			name: "种子记录：改名失败",
			setup: []string{
				"INSERT INTO torrent_infos (site_name, torrent_id) VALUES ('cmct', '1')",
				"CREATE TRIGGER t BEFORE UPDATE OF site_name ON torrent_infos " + raise,
			},
		},
		{
			name:  "用户数据：查询失败",
			setup: []string{"CREATE TABLE user_info (id INTEGER PRIMARY KEY, site TEXT)"},
		},
		{
			name: "用户数据：清理空的 springsunday 记录失败",
			setup: []string{
				"CREATE TABLE user_info (id INTEGER PRIMARY KEY, site TEXT, username TEXT)",
				"INSERT INTO user_info (site, username) VALUES ('springsunday', ''), ('cmct', 'alice')",
				"CREATE TRIGGER t BEFORE DELETE ON user_info " + raise,
			},
		},
		{
			name: "用户数据：删除 cmct 记录失败",
			setup: []string{
				"CREATE TABLE user_info (id INTEGER PRIMARY KEY, site TEXT, username TEXT)",
				"INSERT INTO user_info (site, username) VALUES ('springsunday', 'current'), ('cmct', 'old')",
				"CREATE TRIGGER t BEFORE DELETE ON user_info " + raise,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newMemDB(t, &SiteSetting{}, &RSSSubscription{}, &TorrentInfo{})
			require.NoError(t, db.Create(&SiteSetting{Name: "cmct", AuthMethod: "cookie", Cookie: "ck"}).Error)
			if tc.spring {
				require.NoError(t, db.Create(&SiteSetting{Name: "springsunday", AuthMethod: "cookie"}).Error)
			}
			for _, stmt := range tc.setup {
				require.NoError(t, db.Exec(stmt).Error, stmt)
			}

			require.Error(t, SyncSitesFromRegistry(db, nil))

			var cmct int64
			require.NoError(t, db.Model(&SiteSetting{}).Where("name = ?", "cmct").Count(&cmct).Error)
			assert.EqualValues(t, 1, cmct, "失败时整体回滚，cmct 站点留着")
		})
	}
}
