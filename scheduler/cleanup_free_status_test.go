package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 「免费到期未完成删除」按 RSS 最近一次读到的免费资格判断（docs/design/free-expiry-safety.md B.3.3）：
// 已推送的种子每轮刷新资格，免费延长的等新的结束时间，取消或提前结束的按记下的免费结束时间（free_ended_at）删。
func TestCleanup_FreeExpiredFollowsRefreshedStatus(t *testing.T) {
	cm := newCleanupMonitorWithFake(t, newSchedFakeDownloader("qb1"))
	cfg := baseCfg()
	cfg.CleanupDelFreeExpired = true
	now := time.Now()
	past, future := now.Add(-time.Minute), now.Add(3*time.Hour)
	pushed := true
	rows := []struct {
		hash string
		cols map[string]any
		want bool
		why  string
	}{
		{"hcancelled", map[string]any{"is_free": false, "free_validity": "not_free", "free_level": "NONE", "free_end_time": nil, "free_ended_at": past}, true, "免费取消：按记下的免费结束时间删"},
		{"hextended", map[string]any{"is_free": true, "free_validity": "free_until", "free_end_time": future}, false, "免费延长：等新的结束时间"},
		{"hexpired", map[string]any{"is_free": true, "free_validity": "free_until", "free_end_time": past}, true, "结束时间已过"},
		{"hlegacyhalf", map[string]any{"is_free": false, "free_end_time": past}, false, "旧记录里半价的结束时间不算免费到期"},
	}
	for i, r := range rows {
		ti := models.TorrentInfo{SiteName: "springsunday", TorrentID: fmt.Sprintf("c%d", i), TorrentHash: &r.hash, IsPushed: &pushed, DownloaderName: "qb1"}
		require.NoError(t, cm.db.Create(&ti).Error)
		require.NoError(t, cm.db.Model(&models.TorrentInfo{}).Where("id = ?", ti.ID).Updates(r.cols).Error)
	}
	for _, r := range rows {
		tor := seedingTorrent(r.hash, r.hash, r.hash, 1, 0)
		tor.Progress = 0.5
		got := cm.deleteReasons(cfg, tor, "qb1")
		if r.want {
			assert.Contains(t, got, "免费期过了还没下完", r.why)
		} else {
			assert.NotContains(t, got, "免费期过了还没下完", r.why)
		}
	}
}

// 同一个哈希还可能有别的下载器、别的站点的记录：只看这个下载器上的记录，不按别处免费已过的记录删这个下载器里的任务；
// 这个下载器上同一个哈希有一条还在免费期内（或从来不是免费推的）时也不删——删种会删任务和数据，分不清时宁可不删。
func TestCleanup_FreeExpiredUsesThisDownloadersRecords(t *testing.T) {
	fake := newSchedFakeDownloader("qb-b")
	cm := newCleanupMonitorWithFake(t, fake)
	cfg := baseCfg()
	cfg.CleanupScope = "database"
	cfg.CleanupDelFreeExpired = true
	cfg.CleanupProtectDL = false
	cfg.CleanupProtectHR = false
	past, future := time.Now().Add(-time.Minute), time.Now().Add(3*time.Hour)
	pushed := true
	add := func(id, hash, dl string, cols map[string]any) {
		ti := models.TorrentInfo{SiteName: "s", TorrentID: id, TorrentHash: &hash, IsPushed: &pushed, DownloaderName: dl}
		require.NoError(t, cm.db.Create(&ti).Error)
		require.NoError(t, cm.db.Model(&models.TorrentInfo{}).Where("id = ?", ti.ID).Updates(cols).Error)
	}
	// hother：下载器 qb-a 上免费已过；qb-b 上同一个哈希从来不是免费推的
	add("a1", "hother", "qb-a", map[string]any{"is_free": true, "free_validity": "free_until", "free_end_time": past})
	add("b1", "hother", "qb-b", map[string]any{"is_free": false, "free_validity": "not_free"})
	// hmixed：qb-b 上两条记录（不同站点或编号），一条免费已过，一条还在免费期内
	add("b2", "hmixed", "qb-b", map[string]any{"is_free": true, "free_validity": "free_until", "free_end_time": past})
	add("b3", "hmixed", "qb-b", map[string]any{"is_free": true, "free_validity": "free_until", "free_end_time": future})
	// hmine：qb-b 上只有这一条，免费已过
	add("b4", "hmine", "qb-b", map[string]any{"is_free": true, "free_validity": "free_until", "free_end_time": past})
	for _, h := range []string{"hother", "hmixed", "hmine"} {
		tor := seedingTorrent(h, h, h, 1, 0)
		tor.State, tor.Progress = downloader.TorrentDownloading, 0.5
		fake.torrents = append(fake.torrents, tor)
	}

	var deleted []string
	for _, it := range cm.Preview(t.Context(), cfg)[0].Items {
		deleted = append(deleted, it.Name)
	}
	assert.ElementsMatch(t, []string{"hmine"}, deleted, "只按这个下载器上的记录、而且都免费已过才删")
}

// 下载器可以改名，名字也可能被后建的下载器复用：免费到期按记录里的下载器编号认，不按名字。
// 改名后原来的记录照样管（该删的删，不让任务继续计费下载）；新下载器复用了旧名字时，不按旧下载器的记录删它的任务。
func TestCleanup_FreeExpiredFollowsDownloaderID(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	pushed := true
	setup := func(t *testing.T) (*CleanupMonitor, *schedFakeDownloader, *models.SettingsGlobal) {
		fake := newSchedFakeDownloader("qb1")
		cm := newCleanupMonitorWithFake(t, fake)
		cfg := baseCfg()
		cfg.CleanupScope = "all"
		cfg.CleanupDelFreeExpired = true
		cfg.CleanupProtectDL = false
		cfg.CleanupProtectHR = false
		tor := seedingTorrent("h1", "h1", "h1", 1, 0)
		tor.State, tor.Progress = downloader.TorrentDownloading, 0.5
		fake.torrents = []downloader.Torrent{tor}
		return cm, fake, cfg
	}
	addSetting := func(t *testing.T, cm *CleanupMonitor, name string) uint {
		ds := models.DownloaderSetting{Name: name, Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}
		require.NoError(t, cm.db.Create(&ds).Error)
		return ds.ID
	}
	addExpired := func(t *testing.T, cm *CleanupMonitor, dlID uint, dlName string) {
		hash := "h1"
		ti := models.TorrentInfo{
			SiteName: "s", TorrentID: "r1", TorrentHash: &hash, IsPushed: &pushed,
			DownloaderID: &dlID, DownloaderName: dlName, IsFree: true, FreeEndTime: &past,
		}
		require.NoError(t, cm.db.Create(&ti).Error)
	}
	deleted := func(cm *CleanupMonitor, cfg *models.SettingsGlobal) []string {
		var names []string
		for _, it := range cm.Preview(context.Background(), cfg)[0].Items {
			names = append(names, it.Name)
		}
		return names
	}

	t.Run("改名后照样按编号认出来", func(t *testing.T) {
		cm, _, cfg := setup(t)
		id := addSetting(t, cm, "qb1")  // 现在叫 qb1
		addExpired(t, cm, id, "qb-old") // 推送时还叫 qb-old
		assert.Equal(t, []string{"h1"}, deleted(cm, cfg))
	})

	t.Run("复用旧名字的新下载器不按旧下载器的记录删", func(t *testing.T) {
		cm, _, cfg := setup(t)
		oldID := addSetting(t, cm, "qb-old") // 原来叫 qb1，后来改名 qb-old
		addSetting(t, cm, "qb1")             // 新建的下载器用了 qb1 这个名字
		addExpired(t, cm, oldID, "qb1")
		assert.Empty(t, deleted(cm, cfg))
	})
}

// 没有下载器编号、升级时也补不上的旧记录认不出是哪个下载器的：不按它删任务（下载器改名、旧名字被新下载器复用时，
// 按名字认会把旧下载器的记录算到新下载器上）。
func TestCleanup_FreeExpiredIgnoresUnattributedLegacyRecord(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	cfg := baseCfg()
	cfg.CleanupScope = "all"
	cfg.CleanupDelFreeExpired = true
	cfg.CleanupProtectDL = false
	cfg.CleanupProtectHR = false
	for _, name := range []string{"qb-old", "qb1"} { // 原来叫 qb1 的改名 qb-old，新下载器复用了 qb1
		require.NoError(t, cm.db.Create(&models.DownloaderSetting{Name: name, Type: "qbittorrent", URL: "http://127.0.0.1:1"}).Error)
	}
	past := time.Now().Add(-time.Minute)
	pushed := true
	hash := "h1"
	require.NoError(t, cm.db.Create(&models.TorrentInfo{
		SiteName: "s", TorrentID: "old", TorrentHash: &hash, IsPushed: &pushed,
		DownloaderName: "qb1", IsFree: true, FreeEndTime: &past, // 没有编号
	}).Error)
	tor := seedingTorrent("h1", "h1", "h1", 1, 0)
	tor.State, tor.Progress = downloader.TorrentDownloading, 0.5
	fake.torrents = []downloader.Torrent{tor}

	assert.Empty(t, cm.Preview(context.Background(), cfg)[0].Items)
}
