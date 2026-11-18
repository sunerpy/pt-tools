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

// 刷流的「没有活动」走完一整轮（先采样、再判断）：按下载器记的最后活动时间删，不按刷流自己的采样。
//   - fresh：刚开始采样（第一次采样把记录上的最后活动时间写成这一刻），下载器记的是 7 小时前：删。
//   - busy：采样 7 小时没涨，下载器记的是 1 小时前：不删。
//   - noinfo：下载器没给最后活动时间，采样 7 小时没涨：按采样删。
func TestBrushMonitor_InactiveByDownloaderLastActivity(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, func(bt *models.BrushTask) { bt.RemoveInactiveH = 6; bt.Enabled = false })
	tag := models.BrushTaskTag(task.ID)
	repo := models.NewBrushRepository(r.db.DB)
	now := r.clock.Now()
	add := func(hash string, sampled bool) *models.BrushTorrent {
		bt := &models.BrushTorrent{
			TaskID: task.ID, InfoHash: hash, SiteName: "hdsky", TorrentID: hash, Title: "T-" + hash,
			AddedAt: now.Add(-8 * time.Hour), State: models.BrushTorrentActive, DownloaderID: 1,
		}
		require.NoError(t, repo.RecordAdded(bt, "2026-10-06"))
		if sampled {
			require.NoError(t, repo.RecordSample(bt, models.BrushTorrentSample{At: now.Add(-7 * time.Hour), Uploaded: 100}, 1, 1, 3600, time.UTC))
		}
		return bt
	}
	fresh := add("fresh", false)
	busy := add("busy", true)
	noinfo := add("noinfo", true)
	r.dl.torrents = []downloader.Torrent{
		{ID: "id-fresh", InfoHash: "fresh", Tags: tag, Progress: 1, State: downloader.TorrentSeeding, TotalUploaded: 100, LastActivity: now.Add(-7 * time.Hour).Unix()},
		{ID: "id-busy", InfoHash: "busy", Tags: tag, Progress: 1, State: downloader.TorrentSeeding, TotalUploaded: 100, LastActivity: now.Add(-time.Hour).Unix()},
		{ID: "id-noinfo", InfoHash: "noinfo", Tags: tag, Progress: 1, State: downloader.TorrentSeeding, TotalUploaded: 100},
	}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"id-fresh", "id-noinfo"}, r.dl.removedSingle, res.Errors)
	assert.Equal(t, 2, res.Removed)
	assert.Equal(t, models.BrushTorrentRemoved, brushRow(t, r, fresh.ID).State)
	assert.Equal(t, "已 7.0 小时没有上传或下载", brushRow(t, r, fresh.ID).RemoveReason)
	assert.Equal(t, models.BrushTorrentActive, brushRow(t, r, busy.ID).State)
	assert.Equal(t, models.BrushTorrentRemoved, brushRow(t, r, noinfo.ID).State)
	assert.Equal(t, "已 7.0 小时没有上传或下载", brushRow(t, r, noinfo.ID).RemoveReason)
}
