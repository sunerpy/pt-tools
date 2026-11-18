package subscribe

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/serverindex"
	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// delFixture 是删除用的场景：最后生还者第 1 季在追；E01、E02 硬链接进了剧集库（真的文件）；下载器 qb（编号 1）里有 E01（h1，H&R 72 小时）、
// E02（h2，和别处来的辅种 x9 共用数据）与第 1 季的整季包（h3，H&R 72 小时没做满）；qb2（编号 2）里有同一个 hash 的 E01（别的站推送的）。
type delFixture struct {
	sub      *models.MediaSubscription
	lib      models.MediaLibrary
	e01, e02 models.MediaTransferHistory
	qb2      *fakeDL
}

func (e *env) delFixture() delFixture {
	e.t.Helper()
	var f delFixture
	root := e.t.TempDir()
	f.lib = models.MediaLibrary{Name: "剧集库", Kind: models.MediaKindTV, Path: filepath.Join(root, "tv"), Enabled: true}
	require.NoError(e.t, e.db.Create(&f.lib).Error)
	f.sub = e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	row := func(ep int, hash string) models.MediaTransferHistory {
		src := filepath.Join(root, "dl", hash, "e.mkv")
		target := filepath.Join(f.lib.Path, "最后生还者 (2023)", "Season 02", fmt.Sprintf("S02E%02d.mkv", ep))
		require.NoError(e.t, os.MkdirAll(filepath.Dir(src), 0o755))
		require.NoError(e.t, os.WriteFile(src, []byte(hash), 0o644))
		require.NoError(e.t, os.MkdirAll(filepath.Dir(target), 0o755))
		require.NoError(e.t, os.Link(src, target))
		id, err := transfer.FileID(target)
		require.NoError(e.t, err)
		r := models.MediaTransferHistory{
			SourcePath: src, TargetPath: target, TargetFileID: id, MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Episode: ep,
			Status: models.MediaTransferDone, Mode: models.MediaModeHardlink, InfoHash: hash, DownloaderID: 1, LibraryID: f.lib.ID, Size: 1 << 30,
			TorrentName: fmt.Sprintf("The.Last.of.Us.S02E%02d.1080p-HHWEB", ep), Title: "最后生还者",
		}
		require.NoError(e.t, e.db.Create(&r).Error)
		return r
	}
	f.e01, f.e02 = row(1, "h1"), row(2, "h2")
	pushed := true
	info := func(site, id, hash string, dl uint, hr bool, title string) {
		h := hash
		require.NoError(e.t, e.db.Create(&models.TorrentInfo{
			SiteName: site, TorrentID: id, TorrentHash: &h, DownloaderID: ptrUint(dl), Title: title, IsPushed: &pushed, HasHR: hr, HRSeedTimeH: 72,
			MediaType: models.MediaKindTV, TMDBID: 100088, MediaSeason: 2,
		}).Error)
	}
	info("hdsky", "11", "h1", 1, true, "The.Last.of.Us.S02E01.1080p-HHWEB")
	info("hdsky", "12", "h2", 1, false, "The.Last.of.Us.S02E02.1080p-HHWEB")
	info("ourbits", "31", "h3", 1, true, "The.Last.of.Us.S02.1080p-YYWEB")
	info("ourbits", "21", "h1", 2, true, "The.Last.of.Us.S02E01.1080p-HHWEB")
	old := time.Now().Add(-30 * 24 * time.Hour).Unix()
	seeding := func(hash string, hours int64, mod func(*downloader.Torrent)) downloader.Torrent {
		t := downloader.Torrent{
			ID: "id-" + hash, InfoHash: hash, Name: hash, Progress: 1, IsCompleted: true, State: downloader.TorrentSeeding, SeedingTime: hours * 3600,
			DateAdded: old, SavePath: "/downloads", ContentPath: "/downloads/" + hash, Ratio: 1.5,
		}
		if mod != nil {
			mod(&t)
		}
		return t
	}
	e.dl.setTorrents(
		seeding("h1", 100, nil),
		seeding("h2", 100, nil),
		seeding("x9", 100, func(t *downloader.Torrent) { t.ContentPath = "/downloads/h2" }),
		seeding("h3", 10, nil),
	)
	f.qb2 = &fakeDL{}
	f.qb2.setTorrents(seeding("h1", 1, nil))
	e.svc.cfg.Downloaders = fakeDLs{dl: e.dl, others: map[uint]*fakeDL{2: f.qb2}}
	return f
}

func season2(eps ...int) DeleteSelector {
	return DeleteSelector{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Episodes: eps}
}

func torrentsOf(p *DeletePreview) map[TorrentKey]DeleteTorrent {
	out := map[TorrentKey]DeleteTorrent{}
	for _, t := range p.Torrents {
		out[TorrentKey{DownloaderID: t.DownloaderID, Hash: t.Hash}] = t
	}
	return out
}

func stepOf(t *testing.T, res *DeleteResult, kind string, match func(DeleteStep) bool) DeleteStep {
	t.Helper()
	for _, st := range res.Steps {
		if st.Kind == kind && match(st) {
			return st
		}
	}
	require.FailNow(t, "没有这一步", "%s %+v", kind, res.Steps)
	return DeleteStep{}
}

func torrentStep(t *testing.T, res *DeleteResult, dl uint, hash string) DeleteStep {
	return stepOf(t, res, DeleteStepTorrent, func(st DeleteStep) bool { return st.DownloaderID == dl && st.Hash == hash })
}

// 预览：库里的文件、关联的种子（能不能删按保护规则；种子里还有没选的集的默认不勾；写明和哪些种子共用数据）、这一季的订阅与问题。
func TestPreviewDelete(t *testing.T) {
	e := newEnv(t)
	f := e.delFixture()
	p, err := e.svc.PreviewDelete(e.ctx, season2())
	require.NoError(t, err)
	assert.Empty(t, p.Errors)
	require.Len(t, p.Files, 2)
	assert.Equal(t, organize.FileOurs, p.Files[0].State)
	assert.Equal(t, models.MediaModeHardlink, p.Files[0].Mode)
	assert.Equal(t, []ItemSubscription{{ID: f.sub.ID, Status: models.MediaSubActive, Season: 2}}, p.Subscriptions)

	got := torrentsOf(p)
	require.Len(t, got, 4, "辅种 x9 不是这一部的种子，不列")
	h1 := got[TorrentKey{1, "h1"}]
	assert.True(t, h1.Deletable)
	assert.Equal(t, "H&R 已满足", h1.Reason)
	assert.Equal(t, "E01", h1.Episodes)
	assert.False(t, h1.CoversMore)
	assert.Empty(t, h1.Shares)
	h2 := got[TorrentKey{1, "h2"}]
	assert.True(t, h2.Deletable)
	assert.Equal(t, []ShareRef{{Hash: "x9", Name: "x9"}}, h2.Shares, "和辅种共用数据")
	pack := got[TorrentKey{1, "h3"}]
	assert.False(t, pack.Deletable)
	assert.Equal(t, GuardHR, pack.Rule)
	assert.Equal(t, "H&R 还差 3 天", pack.Reason)
	assert.Equal(t, "H&R：要求做种 72 小时，已做 10.0 小时", pack.Protection)
	assert.Equal(t, "H&R 要求做种 3 天，还差 3 天", pack.Why)
	assert.Empty(t, h1.Why, "不受保护的没有")
	assert.Equal(t, "整季", pack.Episodes)
	other := got[TorrentKey{2, "h1"}]
	assert.Equal(t, "qb2", other.Downloader, "同一个 hash 在另一台下载器上是另一个种子")
	assert.False(t, other.Deletable)

	// 只选第 2 集：整季包写明还有别的集，第 1 集的种子不列
	p, err = e.svc.PreviewDelete(e.ctx, season2(2))
	require.NoError(t, err)
	require.Len(t, p.Files, 1)
	assert.Equal(t, f.e02.ID, p.Files[0].ID)
	got = torrentsOf(p)
	require.Len(t, got, 2)
	assert.False(t, got[TorrentKey{1, "h2"}].CoversMore)
	assert.True(t, got[TorrentKey{1, "h3"}].CoversMore)
	assert.Equal(t, "整季包，还有这次没选的集", got[TorrentKey{1, "h3"}].More)

	// 下载器读不到：写进 errors，那台的种子不列；读不到保护规则：一律受保护
	f.qb2.listErr = errors.New("连不上")
	require.NoError(t, e.db.Migrator().DropTable(&models.SettingsGlobal{}))
	p, err = e.svc.PreviewDelete(e.ctx, season2())
	require.NoError(t, err)
	assert.Len(t, p.Torrents, 3)
	joined := strings.Join(p.Errors, "；")
	assert.Contains(t, joined, "读不到下载器「qb2」里的种子")
	assert.Contains(t, joined, "读不到删种的保护规则")
	for _, tr := range p.Torrents {
		assert.False(t, tr.Deletable)
		assert.Equal(t, "受保护：读不到保护规则", tr.Reason)
	}

	for _, bad := range []DeleteSelector{
		{MediaType: models.MediaKindMovie, TMDBID: 1, Season: 1},
		{MediaType: models.MediaKindTV, TMDBID: 0},
		{MediaType: "person", TMDBID: 1},
		{MediaType: models.MediaKindTV, TMDBID: 1, Season: -1},
		{MediaType: models.MediaKindTV, TMDBID: 1, Episodes: []int{0}},
	} {
		_, err := e.svc.PreviewDelete(e.ctx, bad)
		require.ErrorIs(t, err, ErrInvalid, "%+v", bad)
	}
}

// 选择器：集号去重排序；种子在不在范围里、是不是还有没选的集；问题在不在范围里。
func TestDeleteSelectorScope(t *testing.T) {
	sel, err := season2(3, 1, 3).normalized()
	require.NoError(t, err)
	assert.Equal(t, []int{1, 3}, sel.Episodes)
	// 一次最多选 500 集（集号都对时也一样）
	many := make([]int, maxDeleteEpisodes+1)
	for i := range many {
		many[i] = i + 1
	}
	_, err = DeleteSelector{MediaType: models.MediaKindTV, TMDBID: 1, Episodes: many}.normalized()
	require.ErrorIs(t, err, ErrInvalid)
	_, err = DeleteSelector{MediaType: models.MediaKindTV, TMDBID: 1, Episodes: many[:maxDeleteEpisodes]}.normalized()
	require.NoError(t, err)

	ref := func(season int, whole bool, eps ...int) *torrentRef {
		return &torrentRef{season: season, whole: whole, eps: eps}
	}
	assert.True(t, sel.inScope(ref(2, false, 3, 4)))
	assert.False(t, sel.inScope(ref(2, false, 2)))
	assert.False(t, sel.inScope(ref(1, false, 1)), "别的季")
	assert.True(t, sel.inScope(ref(0, false, 3)), "不知道是哪一季的也列")
	assert.False(t, sel.inScope(ref(0, false, 9)), "不知道是哪一季、集号也对不上")
	assert.True(t, sel.inScope(ref(2, true)))
	assert.True(t, sel.inScope(ref(2, false)), "不知道是哪几集的也列")
	more, note := sel.coversMore(ref(2, false, 3, 4, 5))
	assert.True(t, more)
	assert.Equal(t, "种子里还有 E04–E05", note)
	more, _ = sel.coversMore(ref(2, false, 1, 3))
	assert.False(t, more)
	more, note = sel.coversMore(ref(0, false, 1))
	assert.True(t, more)
	assert.Equal(t, "不知道是哪一季的种子", note)
	more, note = sel.coversMore(ref(2, false))
	assert.True(t, more)
	assert.Equal(t, "不知道种子里是哪几集", note)
	whole, err := season2().normalized()
	require.NoError(t, err)
	more, _ = whole.coversMore(ref(2, true))
	assert.False(t, more, "整季选时整季包不算多")
	movie := DeleteSelector{MediaType: models.MediaKindMovie, TMDBID: 1}
	assert.True(t, movie.inScope(ref(3, false)))
	more, _ = movie.coversMore(ref(0, false))
	assert.False(t, more)

	assert.True(t, sel.issueInScope(models.MediaIssue{Season: 2, Episode: 3}))
	assert.False(t, sel.issueInScope(models.MediaIssue{Season: 2, Episode: 2}))
	assert.False(t, sel.issueInScope(models.MediaIssue{Season: 1, Episode: 1}))
	assert.True(t, whole.issueInScope(models.MediaIssue{Season: 2, Episode: 9}))
	assert.True(t, movie.issueInScope(models.MediaIssue{}))
}

// 同一个 hash 在两台下载器上：只确认了其中一台，另一台不删；不是这一部的种子不删；下载器里已经没有的算成功。
func TestDeleteItemConfirmsPerDownloader(t *testing.T) {
	e := newEnv(t)
	f := e.delFixture()
	// 两台上的 E01 都受保护（H&R 没做满）
	tors := e.dl.torrents
	tors[0].SeedingTime = 3600
	e.dl.setTorrents(tors...)
	res, err := e.svc.DeleteItem(e.ctx, DeleteRequest{
		DeleteSelector: season2(1),
		Torrents: []DeleteTorrentReq{
			{TorrentKey: TorrentKey{1, "H1"}, DeleteData: true},
			{TorrentKey: TorrentKey{2, "h1"}, DeleteData: true},
			{TorrentKey: TorrentKey{1, "x9"}},
			{TorrentKey: TorrentKey{1, "gone"}},
		},
		ConfirmProtected: []TorrentKey{{1, "h1"}},
		Subscription:     DeleteSubKeep,
	})
	require.NoError(t, err)
	st := torrentStep(t, res, 1, "h1")
	assert.True(t, st.OK, st.Message)
	// 原因用弹窗里的短句（H&R 72 小时写成 3 天，做了 1 小时）
	assert.Equal(t, "连数据删了；受保护（H&R 要求做种 3 天，还差 3 天），确认了删除", st.Message)
	st = torrentStep(t, res, 2, "h1")
	assert.False(t, st.OK)
	assert.Equal(t, "受保护，没有删：H&R 要求做种 3 天，还差 3 天", st.Message)
	st = torrentStep(t, res, 1, "x9")
	assert.False(t, st.OK)
	assert.Equal(t, "不是这一部的种子，没有删", st.Message)
	// gone 不是这一部的（没有记录），照样拒绝
	assert.False(t, torrentStep(t, res, 1, "gone").OK)
	assert.Equal(t, 3, res.Failed)
	assert.Equal(t, []string{"id-h1:true"}, e.dl.removed)
	assert.Empty(t, f.qb2.removed)
	assert.Equal(t, models.MediaSubActive, e.subRow(f.sub.ID).Status, "选了保留，订阅不动")

	// 已经不在下载器里了：算成功
	res, err = e.svc.DeleteItem(e.ctx, DeleteRequest{DeleteSelector: season2(1), Torrents: []DeleteTorrentReq{{TorrentKey: TorrentKey{1, "h1"}}}, Subscription: DeleteSubKeep})
	require.NoError(t, err)
	_ = res
}

// 执行时不信预览：预览以后才带上保留标签的种子照样拦下；同一台下载器里还有别的种子用着数据时只删种子，一起删时连数据删。
func TestDeleteItemRechecksAndKeepsSharedData(t *testing.T) {
	e := newEnv(t)
	e.delFixture()
	p, err := e.svc.PreviewDelete(e.ctx, season2())
	require.NoError(t, err)
	require.True(t, torrentsOf(p)[TorrentKey{1, "h1"}].Deletable)

	tors := e.dl.torrents
	tors[0].Tags = "pt-tools-keep"
	e.dl.setTorrents(tors...)
	res, err := e.svc.DeleteItem(e.ctx, DeleteRequest{
		DeleteSelector: season2(),
		Torrents:       []DeleteTorrentReq{{TorrentKey: TorrentKey{1, "h1"}, DeleteData: true}, {TorrentKey: TorrentKey{1, "h2"}, DeleteData: true}},
		Subscription:   DeleteSubKeep,
	})
	require.NoError(t, err)
	st := torrentStep(t, res, 1, "h1")
	assert.False(t, st.OK)
	assert.Equal(t, "受保护，没有删：带保留标签 pt-tools-keep", st.Message)
	st = torrentStep(t, res, 1, "h2")
	assert.True(t, st.OK)
	assert.Equal(t, "只删了种子：数据还有别的种子在用，留着", st.Message)
	assert.Equal(t, []string{"id-h2:false"}, e.dl.removed)
}

// 两个要一起删的种子共用数据：都删时连数据删；受保护又没确认的那个不删时，另一个只删种子（不把它在用的数据删掉）。
func TestDeleteItemSharedTogether(t *testing.T) {
	e := newEnv(t)
	e.delFixture()
	// h4 是 E02 的另一个站的种子，和 h2 是同一份数据
	h4 := "h4"
	pushed := true
	require.NoError(t, e.db.Create(&models.TorrentInfo{
		SiteName: "ourbits", TorrentID: "42", TorrentHash: &h4, DownloaderID: ptrUint(1), Title: "The.Last.of.Us.S02E02.1080p-HHWEB", IsPushed: &pushed,
		MediaType: models.MediaKindTV, TMDBID: 100088, MediaSeason: 2,
	}).Error)
	tors := e.dl.torrents
	tors[2] = downloader.Torrent{
		ID: "id-h4", InfoHash: "h4", Name: "h4", Progress: 1, IsCompleted: true, State: downloader.TorrentSeeding, SeedingTime: 1,
		DateAdded: time.Now().Unix(), SavePath: "/downloads", ContentPath: "/downloads/h2",
	}
	e.dl.setTorrents(tors...)
	req := DeleteRequest{
		DeleteSelector: season2(2),
		Torrents:       []DeleteTorrentReq{{TorrentKey: TorrentKey{1, "h2"}, DeleteData: true}, {TorrentKey: TorrentKey{1, "h4"}, DeleteData: true}},
		Subscription:   DeleteSubKeep,
	}
	// h4 刚加进来（最短保种 24 小时）：没确认时不删，h2 只删种子
	res, err := e.svc.DeleteItem(e.ctx, req)
	require.NoError(t, err)
	assert.False(t, torrentStep(t, res, 1, "h4").OK)
	assert.Contains(t, torrentStep(t, res, 1, "h4").Message, "受保护，没有删：加进来不到 24 小时")
	assert.Equal(t, []string{"id-h2:false"}, e.dl.removed)

	// 确认了 h4：两个一起连数据删
	e.dl.removed = nil
	tors = e.dl.torrents
	e.dl.setTorrents(append(tors, downloader.Torrent{
		ID: "id-h2", InfoHash: "h2", Name: "h2", Progress: 1, IsCompleted: true, State: downloader.TorrentSeeding,
		DateAdded: 1, SavePath: "/downloads", ContentPath: "/downloads/h2",
	})...)
	req.ConfirmProtected = []TorrentKey{{1, "h4"}}
	res, err = e.svc.DeleteItem(e.ctx, req)
	require.NoError(t, err)
	assert.Zero(t, res.Failed, "%+v", res.Steps)
	assert.ElementsMatch(t, []string{"id-h2:true", "id-h4:true"}, e.dl.removed)
}

// 库文件与订阅：先暂停订阅；记录逐条连文件删，不是这一部的不删，移动整理的拒绝，导入的已有文件要另外确认；
// 删了的集的问题改成解决了；连数据删种子时留着的硬链接标上 keep_links；订阅里还在下载的种子改成失败。
func TestDeleteItemFilesAndSubscription(t *testing.T) {
	e := newEnv(t)
	f := e.delFixture()
	other := models.MediaTransferHistory{SourcePath: "/s/9", TargetPath: "/t/9", MediaType: models.MediaKindTV, TMDBID: 100088, Season: 1, Episode: 1, Status: models.MediaTransferDone}
	moved := models.MediaTransferHistory{
		SourcePath: "/s/m", TargetPath: "/t/m", MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Episode: 3,
		Status: models.MediaTransferDone, Mode: models.MediaModeMove,
	}
	existing := models.MediaTransferHistory{
		SourcePath: "/s/x", TargetPath: "/t/x", MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Episode: 4,
		Status: models.MediaTransferDone, Mode: models.MediaModeExisting,
	}
	require.NoError(t, e.db.Create(&other).Error)
	require.NoError(t, e.db.Create(&moved).Error)
	require.NoError(t, e.db.Create(&existing).Error)
	// E02 服务器里删了的问题；订阅里 E02 还在下载的种子记录
	srv := models.MediaServer{Name: "jf", Kind: models.MediaServerJellyfin, URL: "http://jf", Enabled: true}
	require.NoError(t, e.db.Create(&srv).Error)
	require.NoError(t, serverindex.Raise(e.db, serverindex.IssueKey{Kind: models.MediaIssueServerRemoved, ServerID: srv.ID, ServerItemID: "e2"},
		serverindex.IssueInfo{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Episode: 2}, e.Now()))
	require.NoError(t, e.db.Create(&models.MediaSubscriptionTorrent{
		SubscriptionID: f.sub.ID, SiteName: "hdsky", TorrentID: "11", InfoHash: "H1", DownloaderID: 1, Episode: 1, Status: models.MediaSubTorrentDownloading,
	}).Error)

	res, err := e.svc.DeleteItem(e.ctx, DeleteRequest{
		DeleteSelector: season2(),
		HistoryIDs:     []uint{f.e02.ID, f.e02.ID, other.ID, moved.ID, existing.ID, 9999},
		Torrents:       []DeleteTorrentReq{{TorrentKey: TorrentKey{1, "h1"}, DeleteData: true}},
	})
	require.NoError(t, err)
	require.Equal(t, DeleteStepSubscription, res.Steps[0].Kind, "先暂停订阅")
	assert.Equal(t, "暂停了", res.Steps[0].Message)
	assert.Equal(t, models.MediaSubPaused, e.subRow(f.sub.ID).Status)

	st := stepOf(t, res, DeleteStepFile, func(s DeleteStep) bool { return s.ID == f.e02.ID })
	assert.True(t, st.OK, st.Message)
	assert.Equal(t, f.e02.TargetPath, st.Target)
	_, statErr := os.Stat(f.e02.TargetPath)
	assert.True(t, os.IsNotExist(statErr), "库里的硬链接删了")
	_, statErr = os.Stat(f.e02.SourcePath)
	assert.NoError(t, statErr, "种子里的数据不受影响")
	assert.Equal(t, "不是这一部的记录，没有删", stepOf(t, res, DeleteStepFile, func(s DeleteStep) bool { return s.ID == other.ID }).Message)
	assert.Contains(t, stepOf(t, res, DeleteStepFile, func(s DeleteStep) bool { return s.ID == moved.ID }).Message, "移动整理的文件是唯一的一份")
	assert.Contains(t, stepOf(t, res, DeleteStepFile, func(s DeleteStep) bool { return s.ID == existing.ID }).Message, "导入时就在库里的文件")
	assert.Equal(t, "整理记录已经不在了", stepOf(t, res, DeleteStepFile, func(s DeleteStep) bool { return s.ID == 9999 }).Message)
	assert.Equal(t, 4, res.Failed)

	var is models.MediaIssue
	require.NoError(t, e.db.Where("server_item_id = ?", "e2").First(&is).Error)
	assert.Equal(t, models.MediaIssueResolved, is.Status, "E02 没有已整理记录了")

	var e01 models.MediaTransferHistory
	require.NoError(t, e.db.First(&e01, f.e01.ID).Error)
	assert.True(t, e01.KeepLinks, "连数据删了 E01 的种子、留着库里的文件")
	var subTorrent models.MediaSubscriptionTorrent
	require.NoError(t, e.db.Where("torrent_id = ?", "11").First(&subTorrent).Error)
	assert.Equal(t, models.MediaSubTorrentFailed, subTorrent.Status)

	// 导入的已有文件：另外确认了才删
	res, err = e.svc.DeleteItem(e.ctx, DeleteRequest{DeleteSelector: season2(4), HistoryIDs: []uint{existing.ID}, AllowExisting: true, Subscription: DeleteSubKeep})
	require.NoError(t, err)
	assert.Zero(t, res.Failed, "%+v", res.Steps)
}

// 订阅的三种处理：暂停（完成的也暂停，原来暂停的、待确认的不动）、删除、保留。
func TestDeleteItemSubscriptionActions(t *testing.T) {
	e := newEnv(t)
	f := e.delFixture()
	sel := season2(1)
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", f.sub.ID).Update("status", models.MediaSubDone).Error)
	res, err := e.svc.DeleteItem(e.ctx, DeleteRequest{DeleteSelector: sel})
	require.NoError(t, err)
	assert.Equal(t, "暂停了", res.Steps[0].Message)
	assert.Equal(t, models.MediaSubPaused, e.subRow(f.sub.ID).Status, "完成的也暂停：TMDB 加了集会重新打开")

	res, err = e.svc.DeleteItem(e.ctx, DeleteRequest{DeleteSelector: sel, Subscription: DeleteSubPause})
	require.NoError(t, err)
	assert.Equal(t, "原来就是暂停的", res.Steps[0].Message)

	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", f.sub.ID).Update("status", models.MediaSubPending).Error)
	res, err = e.svc.DeleteItem(e.ctx, DeleteRequest{DeleteSelector: sel})
	require.NoError(t, err)
	assert.Equal(t, "还没确认的订阅，没有动", res.Steps[0].Message)
	assert.Equal(t, models.MediaSubPending, e.subRow(f.sub.ID).Status)

	res, err = e.svc.DeleteItem(e.ctx, DeleteRequest{DeleteSelector: sel, Subscription: DeleteSubDelete})
	require.NoError(t, err)
	assert.Equal(t, "删除了订阅", res.Steps[0].Message)
	_, err = e.svc.subRow(e.ctx, f.sub.ID)
	require.ErrorIs(t, err, ErrNotFound)

	// 完成的电影订阅不会再找：不暂停，还是「已完成」
	movie := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", movie.ID).Update("status", models.MediaSubDone).Error)
	res, err = e.svc.DeleteItem(e.ctx, DeleteRequest{DeleteSelector: DeleteSelector{MediaType: models.MediaKindMovie, TMDBID: 693134}})
	require.NoError(t, err)
	assert.Equal(t, "已经完成了，不会再去下，没有动", res.Steps[0].Message)
	assert.Equal(t, models.MediaSubDone, e.subRow(movie.ID).Status)
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", movie.ID).Update("status", models.MediaSubActive).Error)
	_, err = e.svc.DeleteItem(e.ctx, DeleteRequest{DeleteSelector: DeleteSelector{MediaType: models.MediaKindMovie, TMDBID: 693134}})
	require.NoError(t, err)
	assert.Equal(t, models.MediaSubPaused, e.subRow(movie.ID).Status, "在找的电影订阅照样暂停")

	// 没有订阅、什么也没选：不对
	_, err = e.svc.DeleteItem(e.ctx, DeleteRequest{DeleteSelector: sel})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.DeleteItem(e.ctx, DeleteRequest{DeleteSelector: sel, Subscription: "drop", HistoryIDs: []uint{1}})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.DeleteItem(e.ctx, DeleteRequest{DeleteSelector: sel, Torrents: []DeleteTorrentReq{{TorrentKey: TorrentKey{0, "h1"}}}})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.DeleteItem(e.ctx, DeleteRequest{DeleteSelector: DeleteSelector{MediaType: models.MediaKindTV}})
	require.ErrorIs(t, err, ErrInvalid)
}

// 下载器连不上、读不到保护规则、删除失败：这几步写明原因，别的照做。
func TestDeleteItemDownloaderErrors(t *testing.T) {
	e := newEnv(t)
	f := e.delFixture()
	f.qb2.listErr = errors.New("连不上")
	res, err := e.svc.DeleteItem(e.ctx, DeleteRequest{
		DeleteSelector: season2(1), HistoryIDs: []uint{f.e01.ID},
		Torrents: []DeleteTorrentReq{{TorrentKey: TorrentKey{2, "h1"}}}, Subscription: DeleteSubKeep,
	})
	require.NoError(t, err)
	assert.Contains(t, torrentStep(t, res, 2, "h1").Message, "读不到下载器「qb2」里的种子")
	assert.True(t, stepOf(t, res, DeleteStepFile, func(s DeleteStep) bool { return s.ID == f.e01.ID }).OK, "库文件照删")

	require.NoError(t, e.db.Migrator().DropTable(&models.SettingsGlobal{}))
	res, err = e.svc.DeleteItem(e.ctx, DeleteRequest{DeleteSelector: season2(2), Torrents: []DeleteTorrentReq{{TorrentKey: TorrentKey{1, "h2"}}}, Subscription: DeleteSubKeep})
	require.NoError(t, err)
	assert.Contains(t, torrentStep(t, res, 1, "h2").Message, "读不到删种的保护规则，没有删")
	assert.Empty(t, e.dl.removed)
}

// 条目详情：库里那一行写库文件不见了的问题；一季的集写在集卡上（一个文件几集时每一集都写）。
func TestItemLibraryMissingIssues(t *testing.T) {
	e := newEnv(t)
	f := e.delFixture()
	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("id = ?", f.e02.ID).Update("episode_end", 3).Error)
	require.NoError(t, serverindex.Raise(e.db, serverindex.MissingKey(f.e02.ID),
		serverindex.IssueInfo{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Episode: 2}, e.Now()))
	d, err := e.svc.Item(e.ctx, models.MediaKindTV, 100088)
	require.NoError(t, err)
	require.Len(t, d.NAS.Library.Missing, 1)
	assert.Equal(t, ItemServerIssue{ID: d.NAS.Library.Missing[0].ID, Season: 2, Episode: 2}, d.NAS.Library.Missing[0])

	s1, err := e.svc.SeasonItem(e.ctx, 100088, 2)
	require.NoError(t, err)
	byEp := map[int]SeasonEpisode{}
	for _, ep := range s1.Episodes {
		byEp[ep.Episode] = ep
	}
	assert.Nil(t, byEp[1].LibraryIssue)
	require.NotNil(t, byEp[2].LibraryIssue)
	require.NotNil(t, byEp[3].LibraryIssue, "E02–E03 一个文件")
	assert.Equal(t, d.NAS.Library.Missing[0].ID, byEp[3].LibraryIssue.ID)
}

// 删除弹窗里受保护的短句：按命中的规则写，H&R 写要求与还差多久，下载中写进度。
func TestProtectWhy(t *testing.T) {
	dl := downloader.Torrent{Progress: 0.634}
	done := downloader.Torrent{Progress: 1}
	cases := []struct {
		p    Protection
		t    downloader.Torrent
		want string
	}{
		{Protection{}, done, ""},
		{Protection{Rule: GuardKeepTag, Tag: "keep"}, done, "带保留标签 keep"},
		{Protection{Rule: GuardProtectTag, Tag: "珍藏"}, done, "带保护标签 珍藏"},
		{Protection{Rule: GuardHR}, done, "H&R 要求不明，一直保护"},
		{Protection{Rule: GuardHR, Hours: 168, Left: 2 * 86400}, done, "H&R 要求做种 7 天，还差 2 天"},
		{Protection{Rule: GuardHR, Hours: 36, Left: 5 * 3600}, done, "H&R 要求做种 36 小时，还差 5 小时"},
		{Protection{Rule: GuardHR, Hours: 72}, done, "H&R 要求做种 3 天"},
		{Protection{Rule: GuardDownloading}, dl, "还在下载（63%）"},
		{Protection{Rule: GuardDownloading}, done, "还在校验"},
		{Protection{Rule: GuardMinRetain, Hours: 24, Left: 3 * 3600}, done, "加进来不到 24 小时，还差 3 小时"},
		{Protection{Rule: GuardMinRetain, Hours: 24}, done, "加进来不到 24 小时"},
		{Protection{Rule: "unknown", Text: "读不到删种的保护规则"}, done, "读不到删种的保护规则"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, protectWhy(c.p, c.t), "%+v", c.p)
	}
}
