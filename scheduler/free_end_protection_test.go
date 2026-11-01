package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 打开了「免费结束自动删除」：带保留、保护标签或 H&R 没做满的种子不删，改成暂停，写明原因；读不出 H&R 信息时同样不删。
func TestHandleFreeEnded_AutoDeleteRespectsProtection(t *testing.T) {
	setup := func(t *testing.T, tags string, hr bool) (*schedFakeDownloader, *FreeEndMonitor, models.TorrentInfo) {
		fake := newSchedFakeDownloader("qb1")
		m, db := newFreeEndMonitorWithFake(t, fake)
		require.NoError(t, db.DB.Create(&models.SettingsGlobal{
			DownloadDir: t.TempDir(), AutoDeleteOnFreeEnd: true, CleanupProtectHR: true, CleanupProtectTags: "fav",
		}).Error)
		inc := downloader.Torrent{ID: "task-p", InfoHash: "task-p", Progress: 0.4, State: downloader.TorrentDownloading, Tags: tags}
		fake.torrents = []downloader.Torrent{inc}
		freeEnd := time.Now().Add(-time.Minute)
		hash := "task-p"
		tor := models.TorrentInfo{
			SiteName: "s", TorrentID: "p1", Title: "Protected", PauseOnFreeEnd: true, IsFree: true, TorrentHash: &hash,
			FreeEndTime: &freeEnd, DownloaderTaskID: "task-p", DownloaderName: "qb1", HasHR: hr, HRSeedTimeH: 72,
		}
		require.NoError(t, db.DB.Create(&tor).Error)
		return fake, m, tor
	}
	paused := func(t *testing.T, m *FreeEndMonitor, id uint) models.TorrentInfo {
		var got models.TorrentInfo
		require.NoError(t, m.db.First(&got, id).Error)
		assert.True(t, got.IsPausedBySystem)
		return got
	}

	for _, tc := range []struct {
		name, tags string
		hr         bool
		why        string
	}{
		{"保留标签", models.KeepTag, false, "带保留标签（pt-tools-keep）"},
		{"保护标签", "fav", false, "带保护标签「fav」"},
		{"H&R 没做满", "", true, "H&R：要求做种 72 小时，已做 0.0 小时"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake, m, tor := setup(t, tc.tags, tc.hr)
			m.handleFreeEndedTorrent(tor)
			assert.Empty(t, fake.removedSingle)
			assert.Equal(t, []string{"task-p"}, fake.pausedIDs)
			got := paused(t, m, tor.ID)
			assert.Equal(t, "免费期结束，下载未完成（受保护，没有删除："+tc.why+"）", got.PauseReason)
		})
	}

	t.Run("读不出 H&R 信息", func(t *testing.T) {
		fake, m, tor := setup(t, "", false)
		require.NoError(t, m.db.Migrator().DropTable(&models.TorrentInfoArchive{}))
		m.handleFreeEndedTorrent(tor)
		assert.Empty(t, fake.removedSingle)
		got := paused(t, m, tor.ID)
		assert.Equal(t, "免费期结束，下载未完成（受保护，没有删除：读不到 H&R 信息）", got.PauseReason)
	})

	t.Run("没有受保护的照常删", func(t *testing.T) {
		fake, m, tor := setup(t, "", false)
		m.handleFreeEndedTorrent(tor)
		assert.Equal(t, []string{"task-p"}, fake.removedSingle)
	})
}
