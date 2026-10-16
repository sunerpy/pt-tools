package models

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// v14：已推送的种子记下「把它推进下载器的规则」（pushed_filter_rule_id）：RSS 从此每轮刷新 filter_rule_id，
// 升级前推送的记录 filter_rule_id 还是推送时的值，照抄过去；没推送的不填，已经填了的不动。活动表与归档表一样。
func TestMigrationV13ToV14BackfillsPushedFilterRule(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "v13.db")), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { closeTestGorm(db) })
	require.NoError(t, db.AutoMigrate(&SchemaVersion{}, &TorrentInfo{}, &TorrentInfoArchive{}))
	require.NoError(t, db.Create(&SchemaVersion{Version: 13, Description: "test v13", AppVersion: "test"}).Error)
	one, two := uint(1), uint(2)
	pushed := true
	for _, ti := range []TorrentInfo{
		{SiteName: "s", TorrentID: "pushed", FilterRuleID: &one, IsPushed: &pushed},
		{SiteName: "s", TorrentID: "staged", FilterRuleID: &one},
		{SiteName: "s", TorrentID: "free", IsPushed: &pushed},
	} {
		require.NoError(t, db.Create(&ti).Error)
	}
	require.NoError(t, db.Create(&TorrentInfoArchive{SiteName: "s", TorrentID: "archived", FilterRuleID: &two, IsPushed: &pushed}).Error)

	sm := NewSchemaManager(db, "test")
	require.NoError(t, sm.RunMigrations())

	pushedRule := func(table, id string) *uint {
		var v *uint
		require.NoError(t, db.Table(table).Select("pushed_filter_rule_id").Where("torrent_id = ?", id).Scan(&v).Error)
		return v
	}
	require.NotNil(t, pushedRule("torrent_infos", "pushed"))
	assert.Equal(t, one, *pushedRule("torrent_infos", "pushed"))
	assert.Nil(t, pushedRule("torrent_infos", "staged"), "没推送的不填")
	assert.Nil(t, pushedRule("torrent_infos", "free"), "免费通道推的没有规则")
	require.NotNil(t, pushedRule("torrent_info_archives", "archived"))
	assert.Equal(t, two, *pushedRule("torrent_info_archives", "archived"))

	version, err := sm.GetCurrentVersion()
	require.NoError(t, err)
	assert.Equal(t, CurrentSchemaVersion, version)
	assert.GreaterOrEqual(t, CurrentSchemaVersion, 14)
}
