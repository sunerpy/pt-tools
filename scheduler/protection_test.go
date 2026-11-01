package scheduler

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// addActiveBrush 在下载器 dlName 上记一个刷流任务正在管的种子（没有这个下载器的设置时先建一个），返回刷流记录。
func addActiveBrush(t *testing.T, db *gorm.DB, dlName, hash string) models.BrushTorrent {
	t.Helper()
	var ds models.DownloaderSetting
	if err := db.Where("name = ?", dlName).Take(&ds).Error; err != nil {
		ds = models.DownloaderSetting{Name: dlName, Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}
		require.NoError(t, db.Create(&ds).Error)
	}
	task := models.BrushTask{Name: "brush-" + hash, SiteName: "s", DownloaderID: ds.ID}
	require.NoError(t, db.Create(&task).Error)
	bt := models.BrushTorrent{
		TaskID: task.ID, InfoHash: hash, SiteName: "s", TorrentID: "t-" + hash,
		DownloaderID: ds.ID, State: models.BrushTorrentActive, AddedAt: time.Now(),
	}
	require.NoError(t, db.Create(&bt).Error)
	return bt
}

func TestTagProtection(t *testing.T) {
	cases := []struct {
		name string
		tor  downloader.Torrent
		want string
	}{
		{"保留标签（qB 标签）", downloader.Torrent{Tags: "hdsky, pt-tools-keep"}, "带保留标签（pt-tools-keep）"},
		{"保留标签大小写不敏感", downloader.Torrent{Tags: "PT-Tools-Keep"}, "带保留标签（pt-tools-keep）"},
		{"保留标签在分类里", downloader.Torrent{Category: "pt-tools-keep"}, "带保留标签（pt-tools-keep）"},
		{"保留标签在 Transmission label 里", downloader.Torrent{Label: "pt-tools-keep"}, "带保留标签（pt-tools-keep）"},
		{"保护标签", downloader.Torrent{Tags: "a,fav"}, "带保护标签「fav」"},
		{"保留标签先于保护标签", downloader.Torrent{Tags: "fav,pt-tools-keep"}, "带保留标签（pt-tools-keep）"},
		{"都没有", downloader.Torrent{Tags: "pt-tools-keeper"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tagProtection(tc.tor, []string{"fav"}))
		})
	}
}

// 保留标签不受「保护 H&R 种子」等开关影响，任何自动删种都不删；H&R 只在打开了保护时算。
func TestHardProtection(t *testing.T) {
	hr := map[string]hrInfo{"h1": {HasHR: true, HRSeedTimeH: 72}}
	tor := downloader.Torrent{InfoHash: "H1", SeedingTime: 3600}
	off := &models.SettingsGlobal{}
	assert.Empty(t, hardProtection(off, tor, hr, nil), "没打开 H&R 保护")
	on := &models.SettingsGlobal{CleanupProtectHR: true}
	assert.Equal(t, "H&R：要求做种 72 小时，已做 1.0 小时", hardProtection(on, tor, hr, nil))
	tor.SeedingTime = 72 * 3600
	assert.Empty(t, hardProtection(on, tor, hr, nil), "做满了")
	tor.Tags = "pt-tools-keep"
	assert.Equal(t, "带保留标签（pt-tools-keep）", hardProtection(off, tor, hr, nil))
}

// 读不到 H&R 信息时分不清哪些种子受保护：自动删种这一轮不删，结果里写明原因。
func TestCleanupFailsClosedWhenHRUnreadable(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	cfg := baseCfg()
	cfg.CleanupMaxSeedTimeH = 1
	cfg.CleanupProtectDL = false
	fake.torrents = []downloader.Torrent{seedingTorrent("d1", "hdel", "Del", 10, 0.1)}
	require.NoError(t, cm.db.Migrator().DropTable(&models.TorrentInfoArchive{}))

	res := cm.processDownloader(cfg, fake, "qb1")
	assert.Contains(t, res.Error, "这一轮不删")
	assert.Empty(t, fake.removedBatch)
}

// 判断之后、删除之前点了「保留」（或打上保护标签、种子被删掉）：删除锁里按重新读到的列表复查，不删。
// 复查拿掉的种子和要删的共用数据时，要删的只删种子、保留数据。
func TestCleanupRechecksUnderRemovalLock(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	cfg := baseCfg()
	cfg.CleanupMaxSeedTimeH = 1
	cfg.CleanupProtectDL = false
	cfg.CleanupProtectHR = false
	cfg.CleanupRemoveData = true
	cfg.CleanupProtectTags = "fav"
	a := seedingTorrent("a", "ha", "A", 10, 0.1)
	b := seedingTorrent("b", "hb", "B", 10, 0.1)
	c := seedingTorrent("c", "hc", "C", 10, 0.1)
	gone := seedingTorrent("g", "hg", "Gone", 10, 0.1)
	// a、b 是同一份数据（辅种）
	a.SavePath, a.ContentPath = "/data", "/data/x"
	b.SavePath, b.ContentPath = "/data", "/data/x"
	fake.torrents = []downloader.Torrent{a, b, c, gone}
	fake.beforeSecondList = func() {
		a.Tags = "pt-tools-keep"
		c.Tags = "fav"
		fake.torrents = []downloader.Torrent{a, b, c}
	}

	res := cm.processDownloader(cfg, fake, "qb1")
	require.Empty(t, res.Error)
	assert.Equal(t, 1, res.Deleted)
	assert.Contains(t, res.Notes, "3 个种子删除前不在下载器里了，或者带上了保留、保护标签，没有删")
	require.Len(t, fake.removedBatch, 1)
	assert.Equal(t, []string{"b"}, fake.removedBatch[0])
	assert.Equal(t, []bool{false}, fake.removeDataFlags, "B 和留下的 A 共用数据：只删种子")
	require.Len(t, res.Items, 1)
	assert.Equal(t, "B", res.Items[0].Name)
	assert.True(t, res.Items[0].KeepData)
}

// 删除前重新读种子列表失败：这一轮不删。
func TestCleanupRecheckListFailure(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	cfg := baseCfg()
	cfg.CleanupMaxSeedTimeH = 1
	cfg.CleanupProtectDL = false
	cfg.CleanupProtectHR = false
	fake.torrents = []downloader.Torrent{seedingTorrent("d1", "hdel", "Del", 10, 0.1)}
	fake.beforeSecondList = func() { fake.getAllErr = fmt.Errorf("boom") }

	res := cm.processDownloader(cfg, fake, "qb1")
	assert.Contains(t, res.Error, "删除前重新读种子列表失败")
	assert.Empty(t, fake.removedBatch)
}

// 删掉的种子只把这台下载器上的记录标成已过期（按下载器编号认，改名也认得出）。
func TestCleanupUpdateDatabaseByDownloaderID(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	ds := models.DownloaderSetting{Name: "qb1", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}
	require.NoError(t, cm.db.Create(&ds).Error)
	other := models.DownloaderSetting{Name: "qb2", Type: "qbittorrent", URL: "http://127.0.0.1:2", Enabled: true}
	require.NoError(t, cm.db.Create(&other).Error)
	hash := "hx"
	mine := models.TorrentInfo{SiteName: "s", TorrentID: "1", TorrentHash: &hash, DownloaderID: &ds.ID, DownloaderName: "qb-old"}
	theirs := models.TorrentInfo{SiteName: "s", TorrentID: "2", TorrentHash: &hash, DownloaderID: &other.ID, DownloaderName: "qb2"}
	require.NoError(t, cm.db.Create(&mine).Error)
	require.NoError(t, cm.db.Create(&theirs).Error)

	cm.updateDatabase([]downloader.Torrent{{InfoHash: "HX"}}, "qb1")
	var got []models.TorrentInfo
	require.NoError(t, cm.db.Order("id").Find(&got).Error)
	assert.True(t, got[0].IsExpired)
	assert.False(t, got[1].IsExpired)
}

// 刷流交还（点了保留又取消、删了任务）以后，种子马上归自动删种管：不再按 pt-tools-brush 标签跳过。
func TestCleanupTakesOverHandedOverBrushTorrents(t *testing.T) {
	for _, tc := range []struct {
		name    string
		release func(t *testing.T, db *gorm.DB, bt models.BrushTorrent)
	}{
		{"保留以后又取消保留", func(t *testing.T, db *gorm.DB, bt models.BrushTorrent) {
			n, err := models.NewBrushRepository(db).EndActiveByHash(bt.DownloaderID, "HB", "种子带保留标签（pt-tools-keep），交还给用户", time.Now())
			require.NoError(t, err)
			require.Equal(t, 1, n)
		}},
		{"删了刷流任务", func(t *testing.T, db *gorm.DB, bt models.BrushTorrent) {
			require.NoError(t, models.NewBrushRepository(db).DeleteTask(bt.TaskID))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newSchedFakeDownloader("qb1")
			cm := newCleanupMonitorWithFake(t, fake)
			cfg := baseCfg()
			cfg.CleanupMaxSeedTimeH = 1
			cfg.CleanupProtectDL, cfg.CleanupProtectHR = false, false
			tor := seedingTorrent("b", "hb", "Brush", 10, 1)
			tor.Tags = "s,pt-tools-brush,pt-brush-1"
			fake.torrents = []downloader.Torrent{tor}
			bt := addActiveBrush(t, cm.db, "qb1", "hb")

			r := cm.Preview(t.Context(), cfg)[0]
			assert.Equal(t, 1, r.Brush)
			assert.Empty(t, r.Items)

			tc.release(t, cm.db, bt)
			r = cm.Preview(t.Context(), cfg)[0]
			assert.Equal(t, 0, r.Brush)
			require.Len(t, r.Items, 1)
			assert.Equal(t, "Brush", r.Items[0].Name)
		})
	}
}
