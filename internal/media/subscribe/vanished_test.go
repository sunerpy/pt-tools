package subscribe

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// otherTorrent 是下载器里一个和订阅无关的种子（下载器列出来不是空的）。
var otherTorrent = downloader.Torrent{ID: "other", InfoHash: "ffff000000000000000000000000000000000000", Name: "Other.Show.S01E01.1080p.WEB-DL-XX"}

// relist 换掉下载器里的种子，再过 2 分钟（种子列表缓存 1 分钟）刷新一次。
func (e *env) relist(list ...downloader.Torrent) {
	e.dl.setTorrents(list...)
	e.advance(2 * time.Minute)
	e.svc.refresh(e.ctx)
}

// 下载中的订阅种子从下载器里没了：连着 10 分钟看不到才判失败，判了以后这一集马上重新找、记一条运行记录；
// 下载器列不出来、列出来是空的不判；中间又看到了重新计时。
func TestVanishedTorrentFails(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	tv, tor := followOneEpisode(t, e)
	ours := downloader.Torrent{ID: tor.InfoHash, InfoHash: strings.ToUpper(tor.InfoHash), Name: tor.Title}
	status := func() models.MediaSubscriptionTorrent { return e.linked(tv.ID)[0] }

	// 列出来是空的、列不出来：看不出种子在不在，不判
	e.relist()
	e.advance(time.Hour)
	e.svc.refresh(e.ctx)
	assert.Equal(t, models.MediaSubTorrentDownloading, status().Status, "列出来是空的不判")
	e.dl.mu.Lock()
	e.dl.listErr = errors.New("连接超时")
	e.dl.mu.Unlock()
	e.relist(otherTorrent)
	e.advance(time.Hour)
	e.svc.refresh(e.ctx)
	assert.Equal(t, models.MediaSubTorrentDownloading, status().Status, "列不出来不判")
	e.dl.mu.Lock()
	e.dl.listErr = nil
	e.dl.mu.Unlock()

	// 看不到 5 分钟又看到了：重新计时
	e.relist(otherTorrent)
	e.advance(5 * time.Minute)
	e.svc.refresh(e.ctx)
	e.relist(otherTorrent, ours)
	assert.Equal(t, models.MediaSubTorrentDownloading, status().Status)
	e.relist(otherTorrent)
	e.advance(9 * time.Minute)
	e.svc.refresh(e.ctx)
	assert.Equal(t, models.MediaSubTorrentDownloading, status().Status, "这次才看不到 9 分钟")
	runsBefore := len(e.runs(tv.ID))

	e.advance(time.Minute)
	e.svc.refresh(e.ctx)
	got := status()
	assert.Equal(t, models.MediaSubTorrentFailed, got.Status)
	assert.Equal(t, "种子已经不在下载器里了（被删掉了），第 1 集重新算缺的", got.Message)
	sub := e.subRow(tv.ID)
	require.NotNil(t, sub.NextSearchAt)
	assert.False(t, sub.NextSearchAt.After(e.Now()), "马上重新找")
	runs := e.runs(tv.ID)
	require.Len(t, runs, runsBefore+1)
	assert.Contains(t, runs[0].Summary, "「"+tor.Title+"」已经不在下载器里了（可能被刷流或手动删掉），第 1 集重新算缺的，马上重新找")
	assert.Equal(t, models.MediaRunSchedule, runs[0].Trigger)

	// 判过一次就不再判、不再记运行记录
	e.advance(time.Hour)
	e.svc.refresh(e.ctx)
	assert.Len(t, e.runs(tv.ID), runsBefore+1)
}

// 种子从下载器里没了，可这期间整理入库了（下完删掉的、移到别的下载器的）：标成已入库，不判失败。
func TestVanishedButOrganizedIsDone(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	tv, tor := followOneEpisode(t, e)
	e.relist(otherTorrent)
	e.advance(11 * time.Minute)
	e.organized(tor.InfoHash, models.MediaKindTV, 100088, 2, 1, "/lib/S02E01.mkv")
	e.svc.refresh(e.ctx)
	assert.Equal(t, models.MediaSubTorrentDone, e.linked(tv.ID)[0].Status)
}

// 记录刚改过（刚推送、刚「重新整理」）的不判：种子看不到满 10 分钟，记录改过也满 10 分钟才判。
func TestVanishedIgnoresFreshRows(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	tv, tor := followOneEpisode(t, e)
	e.relist(otherTorrent)
	e.advance(6 * time.Minute)
	require.NoError(t, e.db.Model(&models.MediaSubscriptionTorrent{}).Where("id = ?", tor.ID).Update("updated_at", e.Now()).Error)
	e.advance(5 * time.Minute)
	e.svc.refresh(e.ctx)
	assert.Equal(t, models.MediaSubTorrentDownloading, e.linked(tv.ID)[0].Status, "看不到 11 分钟，记录 5 分钟前改过")
	e.advance(6 * time.Minute)
	e.svc.refresh(e.ctx)
	assert.Equal(t, models.MediaSubTorrentFailed, e.linked(tv.ID)[0].Status)
}

// 追剧不认领刷流推送的种子（刷流按自己的规则随时删掉）；别的照常认领。
func TestAdoptSkipsBrushTorrents(t *testing.T) {
	e := newEnv(t)
	sub := e.follow(func(in *SubscriptionInput) { in.LockVersion = off() })
	brush := "AAAA0000000000000000000000000000000000E2"
	require.NoError(t, e.db.Create(&models.TorrentInfo{SiteName: "hdsky", TorrentID: "9001", TorrentHash: &brush, DownloadSource: models.DownloadSourceBrush}).Error)
	e.dl.setTorrents(
		downloader.Torrent{InfoHash: brush, Name: "Natural.High.S04E02.2023.1080p.TX.WEB-DL.H.264.DDP2.0-HHWEB"},
		downloader.Torrent{InfoHash: "cccc0000000000000000000000000000000000e3", Name: "Natural.High.S04E03.2023.1080p.TX.WEB-DL.H.264.DDP2.0-HHWEB"},
	)
	lg := e.ledger(sub)
	assert.Equal(t, models.MediaEpisodeMissing, lg.row(2).State, "刷流下的种子不认领")
	assert.Equal(t, models.MediaEpisodeDownloading, lg.row(3).State)
	linked := e.linked(sub.ID)
	require.Len(t, linked, 1)
	assert.Equal(t, "cccc0000000000000000000000000000000000e3", linked[0].TorrentID)
}
