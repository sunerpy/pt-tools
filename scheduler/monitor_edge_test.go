package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// === drainAndDebounce: returns after the debounce window with no events ===

func TestDrainAndDebounce_ReturnsAfterQuiet(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	ch := make(chan events.Event)

	start := time.Now()
	done := make(chan struct{})
	go func() {
		cm.drainAndDebounce(ch)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(diskEventDebounce + 2*time.Second):
		t.Fatal("drainAndDebounce did not return")
	}
	assert.GreaterOrEqual(t, time.Since(start), diskEventDebounce-100*time.Millisecond)
}

// === drainAndDebounce: extends window on further DiskSpaceLow events ===

func TestDrainAndDebounce_ResetsOnEvent(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	ch := make(chan events.Event, 4)

	done := make(chan struct{})
	go func() {
		cm.drainAndDebounce(ch)
		close(done)
	}()

	// Send one DiskSpaceLow to reset the timer, then let it drain.
	ch <- events.Event{Type: events.DiskSpaceLow}

	select {
	case <-done:
	case <-time.After(diskEventDebounce + 2*time.Second):
		t.Fatal("drainAndDebounce did not return after reset")
	}
}

// === drainAndDebounce: returns immediately when ctx cancelled ===

func TestDrainAndDebounce_CtxCancel(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cm.cancel() // pre-cancel

	ch := make(chan events.Event)
	done := make(chan struct{})
	go func() {
		cm.drainAndDebounce(ch)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("drainAndDebounce did not return on ctx cancel")
	}
}

// === scheduleTaskLockedWithAdvance: nil FreeEndTime is a no-op ===

func TestScheduleTaskLocked_NilFreeEndTime(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, _ := newFreeEndMonitorWithFake(t, fake)

	m.mu.Lock()
	before := len(m.pendingTasks)
	m.scheduleTaskLockedWithAdvance(models.TorrentInfo{ID: 1, FreeEndTime: nil}, 0)
	after := len(m.pendingTasks)
	m.mu.Unlock()

	assert.Equal(t, before, after, "nil FreeEndTime must not add a pending task")
}

// === markCompleted directly ===

func TestMarkCompleted_Direct(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	tor := models.TorrentInfo{SiteName: "s", TorrentID: "mc1", Title: "MC"}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.markCompleted(tor, 4242)

	var updated models.TorrentInfo
	require.NoError(t, db.DB.First(&updated, tor.ID).Error)
	assert.True(t, updated.IsCompleted)
	assert.InDelta(t, 100.0, updated.Progress, 0.01)
	assert.Equal(t, int64(4242), updated.TorrentSize)
}

// === CancelTorrent removes a scheduled task ===

func TestCancelTorrent_RemovesPending(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.torrentByID["task-cancel"] = downloader.Torrent{
		ID: "task-cancel", Progress: 0.5, State: downloader.TorrentDownloading,
	}
	m, db := newFreeEndMonitorWithFake(t, fake)

	future := time.Now().Add(time.Hour)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "c1", Title: "Cancel", PauseOnFreeEnd: true,
		FreeEndTime: &future, DownloaderTaskID: "task-cancel", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.ScheduleTorrent(tor)
	m.mu.Lock()
	_, exists := m.pendingTasks[tor.ID]
	m.mu.Unlock()
	require.True(t, exists)

	m.CancelTorrent(tor.ID)
	m.mu.Lock()
	_, stillThere := m.pendingTasks[tor.ID]
	m.mu.Unlock()
	assert.False(t, stillThere, "CancelTorrent should remove the pending task")
}
