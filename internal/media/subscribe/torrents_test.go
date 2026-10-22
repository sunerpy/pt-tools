package subscribe

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/models"
)

// 追剧详情的种子带上每个文件整理、刮削得怎样；整理没成的能重新整理（按订阅的条目），整理好的能重新刮削；
// 标成失败的种子重新整理以后改回等整理，整理成了马上标成已入库；运行记录里各记一条
func TestTorrentViewsAndActions(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	tv := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	e.search.set(e.item("hdsky", "91", "The.Last.of.Us.S02E01.1080p.WEB-DL.H264-X", "", 3, 10))
	_, err := e.svc.SearchNow(e.ctx, tv.ID)
	require.NoError(t, err)
	links := e.linked(tv.ID)
	require.Len(t, links, 1)
	tor := links[0]
	hash := e.hashOf("91")

	// 还在下载、没有整理记录：两个都不能点
	d, err := e.svc.Subscription(e.ctx, tv.ID)
	require.NoError(t, err)
	require.Len(t, d.TorrentList, 1)
	assert.Empty(t, d.TorrentList[0].Files)
	assert.False(t, d.TorrentList[0].CanOrganize)
	assert.False(t, d.TorrentList[0].CanRescrape)

	// 一个文件整理好了（刮削部分成）、一个失败、一个失败但在自动重试（整季包是这样，单集也一样按种子算）
	done := e.organized(hash, models.MediaKindTV, 100088, 2, 1, "/lib/S02E01.mkv")
	require.NoError(t, e.db.Model(&done).Updates(map[string]any{"scrape_status": models.MediaScrapePartial, "scrape_note": "豆瓣没取到"}).Error)
	failed := models.MediaTransferHistory{InfoHash: hash, SourcePath: "/dl/pack/S02E02.mkv", MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Episode: 2, Status: models.MediaTransferFailed, Message: "目标已有同名文件"}
	require.NoError(t, e.db.Create(&failed).Error)
	retry := time.Now().Add(time.Hour)
	queued := models.MediaTransferHistory{InfoHash: hash, SourcePath: "/dl/pack/S02E03.mkv", MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Episode: 3, Status: models.MediaTransferFailed, NextRetryAt: &retry}
	require.NoError(t, e.db.Create(&queued).Error)
	e.org.setRetrying(queued.ID, true)

	d, err = e.svc.Subscription(e.ctx, tv.ID)
	require.NoError(t, err)
	v := d.TorrentList[0]
	require.Len(t, v.Files, 3)
	assert.Equal(t, "1", v.Files[0].Name)
	assert.Equal(t, models.MediaScrapePartial, v.Files[0].ScrapeStatus)
	assert.Equal(t, "S02E02.mkv", v.Files[1].Name)
	assert.Equal(t, "目标已有同名文件", v.Files[1].Message)
	assert.True(t, v.Files[2].Retrying)
	assert.True(t, v.CanOrganize, "有失败、不在重试的文件")
	assert.True(t, v.CanRescrape, "有整理好的文件")

	// 重新整理：按订阅的条目与媒体库整理这个种子
	res, err := e.svc.OrganizeTorrent(e.ctx, tv.ID, tor.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	require.Len(t, e.org.organized, 1)
	req := e.org.organized[0]
	assert.Equal(t, hash, req.Hash)
	assert.Equal(t, tor.DownloaderID, req.DownloaderID)
	assert.Equal(t, models.MediaKindTV, req.MediaType)
	assert.Equal(t, 100088, req.TMDBID)

	// 重新刮削：按整理好的那条记录
	sr, err := e.svc.RescrapeTorrent(e.ctx, tv.ID, tor.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, sr.Written)
	assert.Equal(t, []uint{done.ID}, e.org.rescraped)

	runs := e.runs(tv.ID)
	var texts []string
	for _, r := range runs {
		texts = append(texts, r.Summary)
	}
	assert.Contains(t, texts, "重新整理「The.Last.of.Us.S02E01.1080p.WEB-DL.H264-X」：新入库 1 个文件")
	assert.Contains(t, texts, "重新刮削「The.Last.of.Us.S02E01.1080p.WEB-DL.H264-X」：刮削了 1 条整理记录、写了 2 个文件")

	// 不是这个订阅的种子、没有整理好的文件
	_, err = e.svc.OrganizeTorrent(e.ctx, tv.ID+1, tor.ID)
	require.ErrorIs(t, err, ErrNotFound)
	other := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	_, err = e.svc.RescrapeTorrent(e.ctx, other.ID, tor.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

// 7 天没入库标成失败的种子：重新整理以后改回等整理（7 天从现在算），整理成了就标成已入库
func TestOrganizeFailedTorrent(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	tv := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	e.search.set(e.item("hdsky", "91", "The.Last.of.Us.S02E01.1080p.WEB-DL.H264-X", "", 3, 10))
	_, err := e.svc.SearchNow(e.ctx, tv.ID)
	require.NoError(t, err)
	e.advance(8 * 24 * time.Hour)
	e.svc.refresh(e.ctx)
	tor := e.linked(tv.ID)[0]
	require.Equal(t, models.MediaSubTorrentFailed, tor.Status)

	d, err := e.svc.Subscription(e.ctx, tv.ID)
	require.NoError(t, err)
	assert.True(t, d.TorrentList[0].CanOrganize, "失败、还没有整理记录：能重新整理")
	assert.False(t, d.TorrentList[0].CanRescrape)

	// 整理在后台排队：种子改回等整理，下一拍不会马上又标成失败
	_, err = e.svc.OrganizeTorrent(e.ctx, tv.ID, tor.ID)
	require.NoError(t, err)
	tor = e.linked(tv.ID)[0]
	assert.Equal(t, models.MediaSubTorrentDownloading, tor.Status)
	assert.Equal(t, "重新整理了，等整理入库", tor.Message)
	e.svc.refresh(e.ctx)
	assert.Equal(t, models.MediaSubTorrentDownloading, e.linked(tv.ID)[0].Status)

	// 整理成了：下一拍标成已入库
	e.organized(e.hashOf("91"), models.MediaKindTV, 100088, 2, 1, "/lib/S02E01.mkv")
	e.svc.refresh(e.ctx)
	assert.Equal(t, models.MediaSubTorrentDone, e.linked(tv.ID)[0].Status)

	// 洗版换掉的旧版本不能再整理、刮削
	require.NoError(t, e.db.Model(&models.MediaSubscriptionTorrent{}).Where("id = ?", tor.ID).Update("status", models.MediaSubTorrentReplaced).Error)
	d, err = e.svc.Subscription(e.ctx, tv.ID)
	require.NoError(t, err)
	assert.False(t, d.TorrentList[0].CanOrganize)
	assert.False(t, d.TorrentList[0].CanRescrape)
	// 动作本身也拒绝：App API、MCP（带 confirm=true）直接调动作，不经过详情里的 CanOrganize。
	// 旧版本还在下载器里做种（H&R、保留）时，重新整理会把特意删掉的旧文件放回库里
	organized, rescraped := len(e.org.organized), len(e.org.rescraped)
	_, err = e.svc.OrganizeTorrent(e.ctx, tv.ID, tor.ID)
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "洗版")
	_, err = e.svc.RescrapeTorrent(e.ctx, tv.ID, tor.ID)
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "洗版")
	assert.Len(t, e.org.organized, organized, "没有去整理")
	assert.Len(t, e.org.rescraped, rescraped, "没有去刮削")
}

// 重新整理以后每一集的情况马上跟着变，不等后台刷新（以前详情里的每一集要等下一次刷新才从「下载中」「缺」变成已入库）
func TestOrganizeTorrentUpdatesEpisodes(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	tv := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	e.search.set(e.item("hdsky", "91", "The.Last.of.Us.S02E01.1080p.WEB-DL.H264-X", "", 3, 10))
	_, err := e.svc.SearchNow(e.ctx, tv.ID)
	require.NoError(t, err)
	tor := e.linked(tv.ID)[0]
	d, err := e.svc.Subscription(e.ctx, tv.ID)
	require.NoError(t, err)
	require.NotNil(t, d.Progress)
	require.Equal(t, models.MediaEpisodeDownloading, d.Progress.Episodes[0].State)

	e.org.onOrganize = func(organize.Request) {
		e.organized(e.hashOf("91"), models.MediaKindTV, 100088, 2, 1, "/lib/S02E01.mkv")
	}
	_, err = e.svc.OrganizeTorrent(e.ctx, tv.ID, tor.ID)
	require.NoError(t, err)
	d, err = e.svc.Subscription(e.ctx, tv.ID)
	require.NoError(t, err)
	assert.Equal(t, models.MediaEpisodeLibrary, d.Progress.Episodes[0].State, "不用等后台刷新")
	assert.Equal(t, 1, d.Progress.InLibrary)
	assert.Equal(t, models.MediaSubTorrentDone, e.linked(tv.ID)[0].Status)
}

// 重新整理写进运行记录的结果：Created 是新放进库的，Done 是库里本来就是这个文件的（以前把 Done 写成「整理了」，
// 新整理了一个文件时记成「整理了 0 个文件」）
func TestOrganizeText(t *testing.T) {
	for _, c := range []struct {
		res  *organize.Result
		want string
	}{
		{nil, "没有结果"},
		{&organize.Result{Queued: true}, "排上队了，稍后在整理历史里看结果"},
		{&organize.Result{Created: 1}, "新入库 1 个文件"},
		{&organize.Result{Created: 3, Done: 1, Skipped: 1, Failed: 1, Messages: []string{"E05.mkv：目标已有同名文件"}}, "新入库 3 个文件，1 个已在库里，1 个跳过，1 个失败，E05.mkv：目标已有同名文件"},
		{&organize.Result{Failed: 1, Plan: &organize.Plan{Problem: "pt-tools 里找不到 /dl/a.mkv"}}, "新入库 0 个文件，1 个失败，pt-tools 里找不到 /dl/a.mkv"},
	} {
		assert.Equal(t, c.want, organizeText(c.res))
	}
}
