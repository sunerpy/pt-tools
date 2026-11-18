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

// SeedGuard 与原来的判定（hardProtection 再 softProtection，自动删种 splitProtectedWhy 的做法）逐条一致：
// 用例取自上面与 cleanup 测试里的保护规则，两边都跑、都等于期望的原因文字。
func TestSeedGuardMatchesCleanupRules(t *testing.T) {
	db := setupTestDB(t)
	for _, r := range []struct {
		hash string
		h    int
	}{{"hknown", 72}, {"hunknown", 0}} {
		hash := r.hash
		require.NoError(t, db.DB.Create(&models.TorrentInfo{SiteName: "s", TorrentID: r.hash, TorrentHash: &hash, HasHR: true, HRSeedTimeH: r.h}).Error)
	}
	now := time.Now().Unix()
	on := func(mut func(*models.SettingsGlobal)) *models.SettingsGlobal {
		cfg := &models.SettingsGlobal{CleanupProtectHR: true, CleanupProtectDL: true, CleanupMinRetainH: 24, CleanupProtectTags: "fav, 收藏"}
		if mut != nil {
			mut(cfg)
		}
		return cfg
	}
	old := now - 30*3600
	cases := []struct {
		name string
		cfg  *models.SettingsGlobal
		tor  downloader.Torrent
		rule string
		want string
	}{
		{"保留标签（qB 标签）", on(nil), downloader.Torrent{Tags: "hdsky, pt-tools-keep", DateAdded: old}, GuardKeepTag, "带保留标签（pt-tools-keep）"},
		{"保留标签在分类里、大小写不敏感", on(nil), downloader.Torrent{Category: "PT-Tools-Keep", DateAdded: old}, GuardKeepTag, "带保留标签（pt-tools-keep）"},
		{"保留标签在 Transmission label 里", on(nil), downloader.Torrent{Label: "pt-tools-keep", DateAdded: old}, GuardKeepTag, "带保留标签（pt-tools-keep）"},
		{"保护标签", on(nil), downloader.Torrent{Tags: "a,收藏", DateAdded: old}, GuardProtectTag, "带保护标签「收藏」"},
		{"保留标签先于保护标签", on(nil), downloader.Torrent{Tags: "fav,pt-tools-keep", DateAdded: old}, GuardKeepTag, "带保留标签（pt-tools-keep）"},
		{"标签先于 H&R", on(nil), downloader.Torrent{InfoHash: "HKNOWN", Tags: "fav", DateAdded: old}, GuardProtectTag, "带保护标签「fav」"},
		{"H&R 没做满", on(nil), downloader.Torrent{InfoHash: "HKNOWN", SeedingTime: 3600, DateAdded: old, State: downloader.TorrentSeeding}, GuardHR, "H&R：要求做种 72 小时，已做 1.0 小时"},
		{"H&R 做满了", on(nil), downloader.Torrent{InfoHash: "hknown", SeedingTime: 72 * 3600, DateAdded: old, State: downloader.TorrentSeeding}, "", ""},
		{"H&R 要求不明", on(nil), downloader.Torrent{InfoHash: "hunknown", SeedingTime: 999 * 3600, DateAdded: old}, GuardHR, "H&R：站点没有给出做种要求，一直保护（可以在站点设置里填「H&R 做种要求」）"},
		{"没打开 H&R 保护", on(func(c *models.SettingsGlobal) { c.CleanupProtectHR = false }), downloader.Torrent{InfoHash: "hknown", DateAdded: old, State: downloader.TorrentSeeding}, "", ""},
		{"H&R 先于下载中", on(nil), downloader.Torrent{InfoHash: "hknown", DateAdded: old, State: downloader.TorrentDownloading}, GuardHR, "H&R：要求做种 72 小时，已做 0.0 小时"},
		{"下载中", on(nil), downloader.Torrent{InfoHash: "x", DateAdded: old, State: downloader.TorrentDownloading}, GuardDownloading, "正在下载或校验（打开了「保护下载中的种子」）"},
		{"校验中", on(nil), downloader.Torrent{InfoHash: "x", DateAdded: old, State: downloader.TorrentChecking}, GuardDownloading, "正在下载或校验（打开了「保护下载中的种子」）"},
		{"没打开保护下载中", on(func(c *models.SettingsGlobal) { c.CleanupProtectDL = false }), downloader.Torrent{InfoHash: "x", DateAdded: old, State: downloader.TorrentDownloading}, "", ""},
		{"最短保种时间", on(nil), downloader.Torrent{InfoHash: "x", DateAdded: now - 3600, State: downloader.TorrentSeeding}, GuardMinRetain, "加进下载器不到 24 小时（最短保种时间），还差 23.0 小时"},
		{"下载中先于最短保种", on(nil), downloader.Torrent{InfoHash: "x", DateAdded: now - 3600, State: downloader.TorrentDownloading}, GuardDownloading, "正在下载或校验（打开了「保护下载中的种子」）"},
		{"最短保种为 0", on(func(c *models.SettingsGlobal) { c.CleanupMinRetainH = 0 }), downloader.Torrent{InfoHash: "x", DateAdded: now - 60, State: downloader.TorrentSeeding}, "", ""},
		{"不知道加进来的时间", on(nil), downloader.Torrent{InfoHash: "x", State: downloader.TorrentSeeding}, "", ""},
		{"都不受保护", on(nil), downloader.Torrent{InfoHash: "x", Tags: "pt-tools-keeper", DateAdded: old, State: downloader.TorrentSeeding}, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, err := NewSeedGuard(db.DB, tc.cfg)
			require.NoError(t, err)
			hr, err := loadHRInfo(db.DB)
			require.NoError(t, err)
			legacy := hardProtection(tc.cfg, tc.tor, hr, splitTags(tc.cfg.CleanupProtectTags))
			if legacy == "" {
				legacy = softProtection(tc.cfg, tc.tor, g.now)
			}
			got := g.Check(tc.tor)
			assert.Equal(t, tc.want, got.Text)
			assert.Equal(t, tc.rule, got.Rule)
			assert.Equal(t, legacy, g.Why(tc.tor), "和原来的判定一样")
		})
	}
}

// Hard 只看保留标签、保护标签与 H&R：下载中、最短保种挡不住免费到期没下完的兜底。规则的附加信息给删除弹窗写短的说明。
func TestSeedGuardHardAndDetails(t *testing.T) {
	db := setupTestDB(t)
	hash := "hk"
	require.NoError(t, db.DB.Create(&models.TorrentInfo{SiteName: "s", TorrentID: "1", TorrentHash: &hash, HasHR: true, HRSeedTimeH: 10}).Error)
	cfg := &models.SettingsGlobal{CleanupProtectHR: true, CleanupProtectDL: true, CleanupMinRetainH: 24, CleanupProtectTags: "fav"}
	g, err := NewSeedGuard(db.DB, cfg)
	require.NoError(t, err)

	fresh := downloader.Torrent{InfoHash: "x", DateAdded: g.now - 3600, State: downloader.TorrentDownloading}
	assert.Equal(t, GuardDownloading, g.Check(fresh).Rule)
	assert.Empty(t, g.Hard(fresh).Rule, "下载中与最短保种不是 hard 保护")

	hr := g.Hard(downloader.Torrent{InfoHash: "HK", SeedingTime: 4 * 3600})
	assert.Equal(t, Protection{Rule: GuardHR, Text: "H&R：要求做种 10 小时，已做 4.0 小时", Hours: 10, Left: 6 * 3600}, hr)
	assert.Equal(t, "fav", g.Hard(downloader.Torrent{Tags: "fav"}).Tag)
	assert.Equal(t, "pt-tools-keep", g.Hard(downloader.Torrent{Tags: "pt-tools-keep"}).Tag)
	retain := g.Check(downloader.Torrent{InfoHash: "x", DateAdded: g.now - 3600, State: downloader.TorrentSeeding})
	assert.Equal(t, int64(23*3600), retain.Left)
	assert.Equal(t, 24, retain.Hours)

	assert.Equal(t, []string{"下载中", "最短保种 24 小时", "H&R", "保留与保护标签"}, g.Rules())
	g2, err := NewSeedGuard(db.DB, &models.SettingsGlobal{})
	require.NoError(t, err)
	assert.Equal(t, []string{"保留标签"}, g2.Rules(), "保留标签一直开着")
	assert.Empty(t, g2.Why(downloader.Torrent{InfoHash: "hk", DateAdded: g.now - 60}), "什么都没开时只看保留标签")

	// 按库里的全局设置建；还没有全局设置时按默认值
	lg, err := LoadSeedGuard(db.DB)
	require.NoError(t, err)
	assert.Equal(t, []string{"最短保种 24 小时", "H&R", "保留标签"}, lg.Rules())
	row := models.SettingsGlobal{DownloadDir: "/d"}
	require.NoError(t, db.DB.Create(&row).Error)
	// 建行时零值会换成列的默认值：另外改
	require.NoError(t, db.DB.Model(&row).Updates(map[string]any{"cleanup_protect_dl": true, "cleanup_protect_hr": false, "cleanup_min_retain_h": 0}).Error)
	lg, err = LoadSeedGuard(db.DB)
	require.NoError(t, err)
	assert.Equal(t, []string{"下载中", "保留标签"}, lg.Rules(), "库里的设置：没开 H&R 保护与最短保种")

	// 读不到 H&R 信息：建不出来，调用方不删
	require.NoError(t, db.DB.Migrator().DropTable(&models.TorrentInfoArchive{}))
	_, err = NewSeedGuard(db.DB, cfg)
	require.Error(t, err)
	_, err = LoadSeedGuard(db.DB)
	require.Error(t, err)
	require.NoError(t, db.DB.Migrator().DropTable(&models.SettingsGlobal{}))
	_, err = LoadSeedGuard(db.DB)
	require.ErrorContains(t, err, "全局设置")
}
