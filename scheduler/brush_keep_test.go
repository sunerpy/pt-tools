package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// brushKeepRig 是一个分享率达到就删的刷流任务，名下有一个已经达标的种子。
func brushKeepRig(t *testing.T, tags string) (*brushRig, *models.BrushTask, *models.BrushTorrent) {
	t.Helper()
	r := newBrushRig(t)
	task := r.task(t, func(bt *models.BrushTask) { bt.RemoveRatio = 2; bt.Enabled = false })
	bt := &models.BrushTorrent{
		TaskID: task.ID, InfoHash: "h1", SiteName: "hdsky", TorrentID: "1", Title: "T1",
		AddedAt: r.clock.Now().Add(-time.Hour), State: models.BrushTorrentActive, DownloaderID: 1,
	}
	require.NoError(t, models.NewBrushRepository(r.db.DB).RecordAdded(bt, "2026-10-06"))
	r.dl.torrents = []downloader.Torrent{{
		ID: "h1", InfoHash: "h1", Tags: "hdsky,pt-tools-brush," + models.BrushTaskTag(task.ID) + tags, Progress: 1, Ratio: 3,
	}}
	return r, task, bt
}

func brushRow(t *testing.T, r *brushRig, id uint) models.BrushTorrent {
	t.Helper()
	var row models.BrushTorrent
	require.NoError(t, r.db.DB.First(&row, id).Error)
	return row
}

// 带保留标签或保护标签的刷流种子交还给用户（终态），不删；之后刷流不再管它。
func TestBrushMonitor_HandsOverKeptTorrents(t *testing.T) {
	for _, tc := range []struct {
		name, tags, protect, reason string
	}{
		{"保留标签", ",pt-tools-keep", "", "种子带保留标签（pt-tools-keep），交还给用户"},
		{"保护标签", ",fav", "fav", "种子带保护标签「fav」，交还给用户"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, task, bt := brushKeepRig(t, tc.tags)
			if tc.protect != "" {
				require.NoError(t, r.db.DB.Create(&models.SettingsGlobal{CleanupProtectTags: tc.protect}).Error)
			}
			res, err := r.mon.RunTask(context.Background(), task.ID)
			require.NoError(t, err)
			assert.Empty(t, r.dl.removedSingle)
			assert.Equal(t, 0, res.Removed)
			assert.Equal(t, 1, res.Gone)
			row := brushRow(t, r, bt.ID)
			assert.Equal(t, models.BrushTorrentGone, row.State)
			assert.Equal(t, tc.reason, row.RemoveReason)

			// 取消保留以后也不回到刷流：记录已是终态
			r.dl.torrents[0].Tags = "hdsky,pt-tools-brush," + models.BrushTaskTag(task.ID)
			r.clock.Advance(time.Hour)
			_, err = r.mon.RunTask(context.Background(), task.ID)
			require.NoError(t, err)
			assert.Empty(t, r.dl.removedSingle)
		})
	}
}

// 这一轮已经决定删、还没进删除锁时，网页点了保留（交还刷流记录）又马上取消保留：锁里复查记录已不是 active，不删。
func TestBrushMonitor_SkipsRecordHandedOverBeforeLock(t *testing.T) {
	r, task, bt := brushKeepRig(t, "")
	repo := models.NewBrushRepository(r.db.DB)
	r.mon.beforeRemoveLock = func(models.BrushTorrent) {
		n, err := repo.EndActiveByHash(1, "H1", "种子带保留标签（pt-tools-keep），交还给用户", r.clock.Now())
		require.NoError(t, err)
		require.Equal(t, 1, n)
		// 取消保留：标签去掉，记录仍是交还以后的终态
	}
	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Empty(t, r.dl.removedSingle, "记录已经交还，不删")
	assert.Equal(t, 0, res.Removed)
	assert.Equal(t, models.BrushTorrentGone, brushRow(t, r, bt.ID).State)
	assert.Equal(t, "种子带保留标签（pt-tools-keep），交还给用户", brushRow(t, r, bt.ID).RemoveReason, "只交还一次")
}

// 进删除锁以前种子刚被打上保留标签（记录还没交还）：锁里重新读种子、复查标签，交还而不删。
func TestBrushMonitor_RechecksTagsUnderLock(t *testing.T) {
	r, task, bt := brushKeepRig(t, "")
	r.mon.beforeRemoveLock = func(models.BrushTorrent) {
		r.dl.torrents[0].Tags += ",pt-tools-keep"
	}
	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Empty(t, r.dl.removedSingle)
	assert.Equal(t, 1, res.Gone)
	row := brushRow(t, r, bt.ID)
	assert.Equal(t, models.BrushTorrentGone, row.State)
	assert.Equal(t, "种子带保留标签（pt-tools-keep），交还给用户", row.RemoveReason)
}

// 进删除锁以前种子已经不在下载器里：记成 gone，不删。
func TestBrushMonitor_TorrentGoneBeforeLock(t *testing.T) {
	r, task, bt := brushKeepRig(t, "")
	r.mon.beforeRemoveLock = func(models.BrushTorrent) { r.dl.torrents = nil }
	_, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Empty(t, r.dl.removedSingle)
	assert.Equal(t, models.BrushTorrentGone, brushRow(t, r, bt.ID).State)
}

// 读不出保护标签时分不清哪些种子受保护：这一轮只采样、不删。
func TestBrushMonitor_NoRemovalWhenProtectTagsUnreadable(t *testing.T) {
	r, task, bt := brushKeepRig(t, "")
	require.NoError(t, r.db.DB.Migrator().DropTable(&models.SettingsGlobal{}))
	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Empty(t, r.dl.removedSingle)
	require.NotEmpty(t, res.Errors)
	assert.Contains(t, res.Errors[0], "这一轮不删种")
	assert.Equal(t, models.BrushTorrentActive, brushRow(t, r, bt.ID).State)
}
