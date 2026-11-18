package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 免费到期监控只管免费的种子：以前 RSS 把半价、2X 的优惠结束时间也记成了免费到期（库里还有这样的记录），
// 这些种子到点不能被暂停或自动删除。
func TestFreeEndMonitor_IgnoresNonFreeTorrents(t *testing.T) {
	for _, tc := range []struct {
		name     string
		isFree   bool
		wantHand bool
	}{
		{"half-price record is left alone", false, false},
		{"free record is paused", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newSchedFakeDownloader("qb1")
			fake.torrentByID["task-x"] = downloader.Torrent{ID: "task-x", Progress: 0.4, State: downloader.TorrentDownloading, TotalSize: 500}
			m, db := newFreeEndMonitorWithFake(t, fake)

			past := time.Now().Add(-time.Minute)
			tor := models.TorrentInfo{
				SiteName: "s", TorrentID: "t-" + tc.name, Title: tc.name, PauseOnFreeEnd: true, IsFree: tc.isFree,
				FreeLevel:   map[bool]string{true: "FREE", false: "PERCENT_50"}[tc.isFree],
				FreeEndTime: &past, DownloaderTaskID: "task-x", DownloaderName: "qb1",
			}
			require.NoError(t, db.DB.Create(&tor).Error)

			m.checkAndProcessExpiredTorrents()
			m.wg.Wait()
			require.NoError(t, m.loadPendingTasksFromDB())
			m.wg.Wait()

			var got models.TorrentInfo
			require.NoError(t, db.DB.First(&got, tor.ID).Error)
			if tc.wantHand {
				assert.Equal(t, []string{"task-x"}, fake.pausedIDs)
				assert.True(t, got.IsPausedBySystem)
			} else {
				assert.Empty(t, fake.pausedIDs, "a non-free torrent must not be paused at a promotion's end")
				assert.False(t, got.IsPausedBySystem)
			}
		})
	}
}

func TestFreeEndMonitor_DoesNotScheduleNonFree(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	future := time.Now().Add(2 * time.Hour)
	half := models.TorrentInfo{
		SiteName: "s", TorrentID: "half", Title: "Half", PauseOnFreeEnd: true, IsFree: false, FreeLevel: "PERCENT_50",
		FreeEndTime: &future, DownloaderTaskID: "task-half", DownloaderName: "qb1",
	}
	free := models.TorrentInfo{
		SiteName: "s", TorrentID: "free", Title: "Free", PauseOnFreeEnd: true, IsFree: true, FreeLevel: "FREE",
		FreeEndTime: &future, DownloaderTaskID: "task-free", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&half).Error)
	require.NoError(t, db.DB.Create(&free).Error)

	m.ScheduleTorrent(half)
	m.rescheduleMissingFutureTorrents(time.Now())
	require.NoError(t, m.loadPendingTasksFromDB())

	m.mu.Lock()
	_, halfPending := m.pendingTasks[half.ID]
	_, freePending := m.pendingTasks[free.ID]
	m.mu.Unlock()
	assert.False(t, halfPending, "a non-free torrent must not get a free-end timer")
	assert.True(t, freePending)
	m.CancelTorrent(free.ID)
	m.CancelTorrent(half.ID)
}
