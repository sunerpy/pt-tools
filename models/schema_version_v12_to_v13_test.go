package models

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// v13：没有下载器编号的旧种子记录按现在的下载器名字补上编号（活动表与归档表），删种按编号认下载器；
// 名字对不上任何下载器的留空（认不出是哪个下载器的），已有编号的不动。叫这个名字的下载器是在推送以后才建的，
// 说明名字被复用过（原来的下载器改了名）：那条记录不是它的，留空。
func TestMigrationV12ToV13BackfillsDownloaderID(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "v12.db")), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { closeTestGorm(db) })
	require.NoError(t, db.AutoMigrate(&SchemaVersion{}, &TorrentInfo{}, &TorrentInfoArchive{}, &DownloaderSetting{}))
	require.NoError(t, db.Create(&SchemaVersion{Version: 12, Description: "test v12", AppVersion: "test"}).Error)
	now := time.Now()
	early, pushedAt := now.AddDate(0, 0, -60), now.AddDate(0, 0, -30)
	qb := DownloaderSetting{Name: "qb1", Type: "qbittorrent", URL: "http://a", CreatedAt: early}
	tr := DownloaderSetting{Name: "tr", Type: "transmission", URL: "http://b", CreatedAt: early}
	reused := DownloaderSetting{Name: "qb2", Type: "qbittorrent", URL: "http://c", CreatedAt: now} // 推送以后才建的
	for _, ds := range []*DownloaderSetting{&qb, &tr, &reused} {
		require.NoError(t, db.Create(ds).Error)
	}
	pushed := true
	rows := []TorrentInfo{
		{SiteName: "s", TorrentID: "legacy", DownloaderName: "qb1", IsPushed: &pushed, PushTime: &pushedAt},
		{SiteName: "s", TorrentID: "legacy-no-push-time", DownloaderName: "qb1", IsPushed: &pushed, CreatedAt: pushedAt},
		{SiteName: "s", TorrentID: "reused-name", DownloaderName: "qb2", IsPushed: &pushed, PushTime: &pushedAt},
		{SiteName: "s", TorrentID: "gone", DownloaderName: "deleted-dl", IsPushed: &pushed, PushTime: &pushedAt},
		{SiteName: "s", TorrentID: "has-id", DownloaderName: "qb1", DownloaderID: &tr.ID, IsPushed: &pushed, PushTime: &pushedAt},
		{SiteName: "s", TorrentID: "no-name"},
	}
	for i := range rows {
		require.NoError(t, db.Create(&rows[i]).Error)
	}
	require.NoError(t, db.Create(&TorrentInfoArchive{SiteName: "s", TorrentID: "archived", DownloaderName: "tr", IsPushed: &pushed, PushTime: &pushedAt}).Error)

	sm := NewSchemaManager(db, "test")
	require.NoError(t, sm.RunMigrations())

	id := func(torrentID string) *uint {
		var ti TorrentInfo
		require.NoError(t, db.Where("torrent_id = ?", torrentID).First(&ti).Error)
		return ti.DownloaderID
	}
	require.NotNil(t, id("legacy"))
	assert.Equal(t, qb.ID, *id("legacy"), "按现在的名字补上编号")
	require.NotNil(t, id("legacy-no-push-time"))
	assert.Equal(t, qb.ID, *id("legacy-no-push-time"), "没有推送时间时按记录的创建时间比")
	assert.Nil(t, id("reused-name"), "叫这个名字的下载器是推送以后才建的：名字被复用过，不算它的")
	assert.Nil(t, id("gone"), "名字对不上任何下载器的留空")
	require.NotNil(t, id("has-id"))
	assert.Equal(t, tr.ID, *id("has-id"), "已有编号的不动")
	assert.Nil(t, id("no-name"))
	var a TorrentInfoArchive
	require.NoError(t, db.Where("torrent_id = ?", "archived").First(&a).Error)
	require.NotNil(t, a.DownloaderID)
	assert.Equal(t, tr.ID, *a.DownloaderID, "归档表一样补")

	version, err := sm.GetCurrentVersion()
	require.NoError(t, err)
	assert.Equal(t, CurrentSchemaVersion, version)
	assert.GreaterOrEqual(t, CurrentSchemaVersion, 13)
}
