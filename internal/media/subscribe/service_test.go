package subscribe

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestSettingsValidation(t *testing.T) {
	e := newEnv(t)
	set, err := e.svc.Settings(e.ctx)
	require.NoError(t, err)
	assert.False(t, set.Enabled)
	assert.Equal(t, DefaultSearchIntervalHours, set.SearchIntervalHours)
	assert.Equal(t, models.MediaUpgradeKeep, set.UpgradeOld)

	_, err = e.svc.SaveSettings(e.ctx, Settings{SearchIntervalHours: 2})
	require.ErrorIs(t, err, ErrInvalid, "最少 6 小时")
	_, err = e.svc.SaveSettings(e.ctx, Settings{UpgradeOld: "burn"})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.SaveSettings(e.ctx, Settings{DefaultProfileID: 9})
	require.ErrorIs(t, err, ErrInvalid)
	set = e.enable(func(s *Settings) {
		s.SearchSkipSites = []string{" hdsky ", "hdsky", ""}
		s.UpgradeOld = models.MediaUpgradeDelete
	})
	assert.True(t, set.Enabled)
	assert.Equal(t, []string{"hdsky"}, set.SearchSkipSites)
	assert.Equal(t, models.MediaUpgradeDelete, set.UpgradeOld)
}

func TestProfilesCRUD(t *testing.T) {
	e := newEnv(t)
	p, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: " 4K ", Resolutions: []string{"2160P", "1080p", "2160p"}, Sources: []string{"bluray"}, HDR: models.MediaPrefAvoid, Groups: []string{"FRDS", "frds", ""}})
	require.NoError(t, err)
	assert.Equal(t, "4K", p.Name)
	assert.Equal(t, []string{"2160p", "1080p"}, p.Resolutions, "规整成可选值的写法、去重")
	assert.Equal(t, []string{"BluRay"}, p.Sources)
	assert.Equal(t, []string{"FRDS"}, p.Groups)

	for _, bad := range []ProfileInput{
		{Name: ""},
		{Name: "x", Resolutions: []string{"8K"}},
		{Name: "x", ChineseSubs: models.MediaPrefAvoid},
		{Name: "x", MinSizeGB: 10, MaxSizeGB: 5},
		{Name: "x", MinSeeders: -1},
		{Name: "4K"},
	} {
		_, perr := e.svc.SaveProfile(e.ctx, 0, bad)
		require.ErrorIs(t, perr, ErrInvalid, bad)
	}
	p2, err := e.svc.SaveProfile(e.ctx, p.ID, ProfileInput{Name: "4K HDR", HDR: models.MediaPrefRequire})
	require.NoError(t, err)
	assert.Equal(t, p.ID, p2.ID)
	assert.Empty(t, p2.Resolutions)

	e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, ProfileID: p.ID})
	require.ErrorIs(t, e.svc.DeleteProfile(e.ctx, p.ID), ErrInvalid, "订阅在用")
	list, err := e.svc.Profiles(e.ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	other, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "其他"})
	require.NoError(t, err)
	e.enable(func(s *Settings) { s.DefaultProfileID = other.ID })
	require.ErrorIs(t, e.svc.DeleteProfile(e.ctx, other.ID), ErrInvalid, "默认档案")
	e.enable(nil)
	require.NoError(t, e.svc.DeleteProfile(e.ctx, other.ID))
	require.ErrorIs(t, e.svc.DeleteProfile(e.ctx, other.ID), ErrNotFound)
}

func TestSubscriptionCRUD(t *testing.T) {
	e := newEnv(t)
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, Season: 3, Sites: []string{"hdsky"}})
	assert.Equal(t, "沙丘2", m.Title)
	assert.Equal(t, "Dune: Part Two", m.OriginalTitle)
	assert.Equal(t, 2024, m.Year)
	assert.Equal(t, "tt15239678", m.IMDbID)
	assert.Equal(t, 0, m.Season, "电影没有季")
	assert.Contains(t, m.Aliases, "Dune Part Two")
	assert.Equal(t, models.MediaSubActive, m.Status)
	require.NotNil(t, m.NextSearchAt, "建好就排上搜索")

	_, err := e.svc.CreateSubscription(e.ctx, SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134}, models.MediaSubFromManual)
	require.ErrorIs(t, err, ErrInvalid, "订阅过了")

	tv := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Pending: true})
	assert.Equal(t, 2, tv.Season, "没填季时订最新一季")
	assert.Equal(t, 3, tv.TotalEpisodes)
	assert.Equal(t, 2025, tv.Year, "用这一季的年份")
	assert.Equal(t, models.MediaSubPending, tv.Status)
	_, err = e.svc.CreateSubscription(e.ctx, SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 9}, models.MediaSubFromManual)
	require.ErrorIs(t, err, ErrInvalid, "TMDB 上没有这一季")
	_, err = e.svc.CreateSubscription(e.ctx, SubscriptionInput{MediaType: "book", TMDBID: 1}, models.MediaSubFromManual)
	require.ErrorIs(t, err, ErrInvalid)

	got, err := e.svc.SetStatus(e.ctx, tv.ID, models.MediaSubActive)
	require.NoError(t, err)
	assert.Equal(t, models.MediaSubActive, got.Status, "确认")
	_, err = e.svc.SetStatus(e.ctx, tv.ID, models.MediaSubDone)
	require.ErrorIs(t, err, ErrInvalid)

	up, err := e.svc.UpdateSubscription(e.ctx, m.ID, SubscriptionInput{Sites: []string{"mteam"}, Category: "movies", Upgrade: true})
	require.NoError(t, err)
	assert.Equal(t, `["mteam"]`, up.Sites)
	assert.True(t, up.Upgrade)
	assert.Equal(t, 693134, up.TMDBID, "条目不变")

	list, err := e.svc.Subscriptions(e.ctx, SubscriptionQuery{Keyword: "沙丘"})
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, []string{"mteam"}, list[0].Sites)
	require.NotNil(t, list[0].Progress)
	assert.Equal(t, []int{1}, list[0].Progress.Missing)
	_, err = e.svc.Subscriptions(e.ctx, SubscriptionQuery{Status: "bad"})
	require.ErrorIs(t, err, ErrInvalid)

	d, err := e.svc.Subscription(e.ctx, tv.ID)
	require.NoError(t, err)
	require.Len(t, d.Progress.Episodes, 3)
	assert.Equal(t, EpisodeUpcoming, d.Progress.Episodes[2].State)
	assert.Equal(t, []int{1, 2}, d.Progress.Missing)

	require.NoError(t, e.svc.DeleteSubscription(e.ctx, m.ID))
	require.ErrorIs(t, e.svc.DeleteSubscription(e.ctx, m.ID), ErrNotFound)
}

func TestKeywords(t *testing.T) {
	movie := &models.MediaSubscription{MediaType: models.MediaKindMovie, Title: "沙丘2", OriginalTitle: "Dune: Part Two", Year: 2024}
	assert.Equal(t, []string{"Dune  Part Two 2024", "沙丘2"}, keywords(movie))
	tv := &models.MediaSubscription{MediaType: models.MediaKindTV, Title: "葬送的芙莉莲", OriginalTitle: "葬送のフリーレン", Season: 1, Aliases: encodeList([]string{"Sousou no Frieren"})}
	assert.Equal(t, []string{"Sousou no Frieren S01", "葬送的芙莉莲 S01"}, keywords(tv), "原名不是拉丁字母时用英文别名")
	same := &models.MediaSubscription{MediaType: models.MediaKindMovie, Title: "Heat", OriginalTitle: "Heat", Year: 1995}
	assert.Equal(t, []string{"Heat 1995"}, keywords(same))
}

// 电影：搜到的种子里挑档案里分最高的推送，记下；下载过以后不再推（没开洗版）
func TestSearchMoviePushesBest(t *testing.T) {
	e := newEnv(t)
	e.enable(func(s *Settings) { s.SearchSkipSites = []string{"mteam"} })
	e.svc.cfg.SiteNames = func() []string { return []string{"hdsky", "mteam", "ourbits"} }
	p, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "好", Resolutions: []string{"2160p", "1080p"}})
	require.NoError(t, err)
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, ProfileID: p.ID, Category: "movies"})
	e.search.set(
		e.item("hdsky", "11", "Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "沙丘2", 8, 50),
		e.item("hdsky", "12", "Dune.Part.Two.2024.2160p.UHD.BluRay.x265-FRDS", "沙丘2", 60, 20),
		e.item("hdsky", "13", "Dune.Part.Two.2024.720p.HDTV.x264-X", "", 3, 5),
		e.item("hdsky", "14", "Dune.2021.2160p.UHD.BluRay.x265-X", "沙丘", 60, 99),
	)
	msg, err := e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	assert.Contains(t, msg, "下载了 Dune.Part.Two.2024.2160p.UHD.BluRay")
	pushes := e.gotPushes()
	require.Len(t, pushes, 1)
	assert.Equal(t, "12", pushes[0].TorrentID)
	assert.Equal(t, "hdsky", pushes[0].SiteID)
	assert.Equal(t, "subscription", pushes[0].Source)
	assert.Equal(t, "movies", pushes[0].Category)
	assert.NotZero(t, pushes[0].DownloaderID, "用默认下载器")
	q := e.search.queries
	require.NotEmpty(t, q)
	assert.Equal(t, []string{"hdsky", "ourbits"}, q[0].Sites, "不参与主动搜索的站点去掉")
	assert.True(t, q[0].RawTitles, "要站点上原样的标题")

	links := e.linked(m.ID)
	require.Len(t, links, 1)
	assert.Equal(t, models.MediaSubTorrentDownloading, links[0].Status)
	assert.Equal(t, e.hashOf("12"), links[0].InfoHash)
	row := e.subRow(m.ID)
	assert.Contains(t, row.Message, "下载了")
	require.NotNil(t, row.NextSearchAt)
	assert.Equal(t, e.Now().Add(DefaultSearchIntervalHours*time.Hour), *row.NextSearchAt)

	msg, err = e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	assert.Equal(t, "已经下载过了", msg)
	assert.Len(t, e.gotPushes(), 1)
}

func TestSearchRejectsAndErrors(t *testing.T) {
	e := newEnv(t)
	e.enable(func(s *Settings) { s.NotifyChannels = []uint{3} })
	p, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "只要 4K", Resolutions: []string{"2160p"}})
	require.NoError(t, err)
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, ProfileID: p.ID})
	e.search.set(e.item("hdsky", "11", "Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "", 8, 50))
	msg, err := e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	assert.Contains(t, msg, "都不符合质量档案")
	assert.Contains(t, msg, "分辨率 1080p 不在档案里")

	e.search.set(e.item("ourbits", "21", "Dune.Part.Two.2024.2160p.WEB-DL.H265-X", "", 20, 5))
	msg, err = e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	assert.Contains(t, msg, "站点 ourbits 没有启用")

	e.search.set(e.item("hdsky", "22", "Dune.Part.Two.2024.2160p.WEB-DL.H265-X", "", 20, 5))
	e.pushErr = assert.AnError
	msg, err = e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	assert.Contains(t, msg, "推送 Dune.Part.Two.2024.2160p.WEB-DL.H265-X 失败")
	assert.Empty(t, e.linked(m.ID), "推送失败的不记下，下次还能再试")

	// 磁盘空间、站点容量这些闸门拒绝时没有报错，只是没有成功：同样不记下、不通知
	e.pushErr = nil
	e.pushRes = &ptinternal.PushTorrentResult{Message: "磁盘空间不足 (有效 1.0 GB <= 10.0 GB)，暂停推送"}
	msg, err = e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	assert.Contains(t, msg, "推送 Dune.Part.Two.2024.2160p.WEB-DL.H265-X 失败：磁盘空间不足")
	assert.Empty(t, e.linked(m.ID), "闸门拒绝的不记下")
	assert.Empty(t, e.notices, "没有下载不通知")
	e.pushRes = nil

	_, err = e.svc.SetStatus(e.ctx, m.ID, models.MediaSubPaused)
	require.NoError(t, err)
	_, err = e.svc.SearchNow(e.ctx, m.ID)
	require.ErrorIs(t, err, ErrInvalid, "暂停的不搜")
}

// RSS 拉到的种子对上订阅时推送；总开关关着时后台什么都不做
func TestRSSOfferAndTick(t *testing.T) {
	e := newEnv(t)
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, Sites: []string{"hdsky"}})
	e.search.set(e.item("hdsky", "11", "Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "", 8, 50))
	e.svc.Tick(e.ctx)
	assert.Empty(t, e.search.queries, "总开关关着")

	e.enable(nil)
	other := e.item("mteam", "31", "Dune.Part.Two.2024.2160p.WEB-DL.H265-X", "", 20, 0)
	e.svc.considerOffer(e.ctx, Candidate{Site: "mteam", Item: other, From: fromRSS})
	assert.Empty(t, e.gotPushes(), "订阅限定了站点")
	it := e.item("hdsky", "32", "Dune.Part.Two.2024.2160p.WEB-DL.H265-X", "", 20, 0)
	e.svc.considerOffer(e.ctx, Candidate{Site: "hdsky", Item: it, From: fromRSS})
	require.Len(t, e.gotPushes(), 1)
	assert.Equal(t, "32", e.gotPushes()[0].TorrentID)
	e.svc.considerOffer(e.ctx, Candidate{Site: "hdsky", Item: it, From: fromRSS})
	assert.Len(t, e.gotPushes(), 1, "同一个种子不重复推")

	// 到期的订阅后台会搜（这个已经下载过了，不再推）
	e.svc.Tick(e.ctx)
	assert.NotEmpty(t, e.search.queries)
	assert.Len(t, e.gotPushes(), 1)
	assert.Equal(t, "已经下载过了", e.subRow(m.ID).Message)
}

// 剧集：这一季还在播时一集一集补缺的，已入库的、下载中的不再下；整季包这时不要
func TestTVEpisodesWhileAiring(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	tv := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	e.organized("e1hash", models.MediaKindTV, 100088, 2, 1, "/lib/tlou/s02e01.mkv")
	e.search.set(
		e.item("hdsky", "41", "The.Last.of.Us.S02E01.1080p.WEB-DL.H264-X", "", 3, 10),
		e.item("hdsky", "42", "The.Last.of.Us.S02E02.1080p.WEB-DL.H264-X", "", 3, 10),
		e.item("hdsky", "43", "The.Last.of.Us.S02E02.2160p.WEB-DL.H265-X", "", 8, 10),
		e.item("hdsky", "44", "The.Last.of.Us.S02.2160p.WEB-DL.H265-X", "", 30, 10),
	)
	msg, err := e.svc.SearchNow(e.ctx, tv.ID)
	require.NoError(t, err)
	assert.Contains(t, msg, "下载了 The.Last.of.Us.S02E02.2160p")
	pushes := e.gotPushes()
	require.Len(t, pushes, 1, "缺第 2 集，只下最好的那个第 2 集")
	assert.Equal(t, "43", pushes[0].TorrentID)
	links := e.linked(tv.ID)
	require.Len(t, links, 1)
	assert.Equal(t, 2, links[0].Episode)

	msg, err = e.svc.SearchNow(e.ctx, tv.ID)
	require.NoError(t, err)
	assert.Equal(t, "缺的集都在下载", msg)

	p, err := e.svc.progress(e.ctx, tv, true)
	require.NoError(t, err)
	assert.Equal(t, 3, p.Total)
	assert.Equal(t, 2, p.Aired)
	assert.Equal(t, 1, p.InLibrary)
	assert.Equal(t, 1, p.Downloading)
	assert.Empty(t, p.Missing)
}

// 剧集：这一季播完、全缺时下载整季包；都入库以后订阅完成
func TestTVSeasonPackAfterAiringAndDone(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	e.advance(60 * 24 * time.Hour) // 第 3 集也播完了
	tv := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	e.search.set(
		e.item("hdsky", "51", "The.Last.of.Us.S02E01.2160p.WEB-DL.H265-X", "", 8, 10),
		e.item("hdsky", "52", "The.Last.of.Us.S02.1080p.WEB-DL.H264-X", "", 12, 10),
	)
	msg, err := e.svc.SearchNow(e.ctx, tv.ID)
	require.NoError(t, err)
	assert.Contains(t, msg, "下载了 The.Last.of.Us.S02.1080p", "缺一半以上时要整季包")
	links := e.linked(tv.ID)
	require.Len(t, links, 1)
	assert.True(t, links[0].Complete)

	hash := e.hashOf("52")
	for ep := 1; ep <= 3; ep++ {
		e.organized(hash, models.MediaKindTV, 100088, 2, ep, "/lib/tlou/s02e0"+string(rune('0'+ep))+".mkv")
	}
	e.svc.refresh(e.ctx)
	assert.Equal(t, models.MediaSubTorrentDone, e.linked(tv.ID)[0].Status)
	row := e.subRow(tv.ID)
	assert.Equal(t, models.MediaSubDone, row.Status)
	assert.Equal(t, "这一季都入库了", row.Message)
}

// 洗版：先下到 1080p，后来有 2160p UHD 原盘；新版本入库后旧版本的库文件删掉、旧种子按设置删掉，达到目标后订阅完成
func TestUpgradeMovie(t *testing.T) {
	e := newEnv(t)
	e.enable(func(s *Settings) { s.UpgradeOld = models.MediaUpgradeDelete })
	p, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "4K 原盘", Resolutions: []string{"2160p", "1080p"}, Sources: []string{"UHD BluRay", "WEB-DL"}})
	require.NoError(t, err)
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, ProfileID: p.ID, Upgrade: true})
	e.search.set(e.item("hdsky", "61", "Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "", 8, 50))
	_, err = e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	require.Len(t, e.gotPushes(), 1)
	old := e.organized(e.hashOf("61"), models.MediaKindMovie, 693134, 0, 0, "/lib/dune2/1080p.mkv")
	e.svc.refresh(e.ctx)
	assert.Equal(t, models.MediaSubActive, e.subRow(m.ID).Status, "还没到目标质量")
	assert.Empty(t, e.org.retired, "第一版不替换别的")

	e.search.set(
		e.item("hdsky", "61", "Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "", 8, 50),
		e.item("hdsky", "62", "Dune.Part.Two.2024.1080p.WEB-DL.H264-Y", "", 8, 80),
		e.item("hdsky", "63", "Dune.Part.Two.2024.2160p.UHD.BluRay.x265-FRDS", "", 60, 20),
	)
	msg, err := e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	assert.Contains(t, msg, "下载了 Dune.Part.Two.2024.2160p.UHD.BluRay")
	require.Len(t, e.gotPushes(), 2)

	msg, err = e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	assert.Equal(t, "洗版：上一个版本还在下载", msg)

	e.organized(e.hashOf("63"), models.MediaKindMovie, 693134, 0, 0, "/lib/dune2/2160p.mkv")
	e.svc.refresh(e.ctx)
	assert.Equal(t, []uint{old.ID}, e.org.retired, "旧版本的库文件删掉")
	links := e.linked(m.ID)
	require.Len(t, links, 2)
	assert.Equal(t, models.MediaSubTorrentReplaced, links[0].Status)
	assert.Contains(t, links[0].Message, "旧种子连数据删掉了")
	assert.Equal(t, []string{e.hashOf("61") + ":true"}, e.dl.removed)
	assert.Equal(t, models.MediaSubTorrentDone, links[1].Status)
	row := e.subRow(m.ID)
	assert.Equal(t, models.MediaSubDone, row.Status)
	assert.Contains(t, row.Message, "达到了洗版的目标")
}

// 洗版的新版本和旧版本整理成同一个文件名：先删掉旧版本的库文件再重新整理
func TestUpgradeSameTargetRetries(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	p, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "4K", Resolutions: []string{"2160p", "1080p"}})
	require.NoError(t, err)
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, ProfileID: p.ID, Upgrade: true})
	e.search.set(e.item("hdsky", "71", "Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "", 8, 50))
	_, err = e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	old := e.organized(e.hashOf("71"), models.MediaKindMovie, 693134, 0, 0, "/lib/dune2/dune2.mkv")
	e.svc.refresh(e.ctx)
	e.search.set(e.item("hdsky", "72", "Dune.Part.Two.2024.2160p.WEB-DL.H265-X", "", 20, 50))
	_, err = e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	skipped := models.MediaTransferHistory{InfoHash: e.hashOf("72"), SourcePath: "/dl/72.mkv", TargetPath: "/lib/dune2/dune2.mkv", MediaType: models.MediaKindMovie, TMDBID: 693134, Status: models.MediaTransferSkipped}
	require.NoError(t, e.db.Create(&skipped).Error)
	e.svc.refresh(e.ctx)
	assert.Equal(t, []uint{old.ID}, e.org.retired)
	assert.Equal(t, []uint{skipped.ID, old.ID}, e.org.retried, "新版本没整理成：把删掉的旧版本整理回来")
	// 旧种子默认继续做种
	links := e.linked(m.ID)
	assert.Equal(t, models.MediaSubTorrentDone, links[0].Status)
	assert.Empty(t, e.dl.removed)

	// 这次整理成了：不再恢复旧版本，新种子入库
	e.org.retryOK = true
	require.NoError(t, e.db.Model(&old).Update("status", models.MediaTransferDone).Error)
	e.org.retired, e.org.retried = nil, nil
	e.svc.refresh(e.ctx)
	assert.Equal(t, []uint{old.ID}, e.org.retired)
	assert.Equal(t, []uint{skipped.ID}, e.org.retried)
	assert.Equal(t, models.MediaSubTorrentDone, e.linkOf(m.ID, "72").Status)
}

// 下载种子文件期间订阅被暂停：推送前再看一眼，不推
func TestPushRechecksSubscription(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	e.search.set(e.item("hdsky", "p1", "Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "", 8, 50))
	e.site.onDownload = func(string) {
		_, err := e.svc.SetStatus(e.ctx, m.ID, models.MediaSubPaused)
		require.NoError(t, err)
	}
	msg, err := e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	assert.Contains(t, msg, "暂停或删除")
	assert.Empty(t, e.gotPushes())
	assert.Empty(t, e.linked(m.ID))
}

// 订阅删掉以后还留着的种子记录（推送和删除撞上了）：刷新时清掉
func TestRefreshDropsOrphanTorrents(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	orphan := models.MediaSubscriptionTorrent{SubscriptionID: 999, SiteName: "hdsky", TorrentID: "x", InfoHash: "h", Status: models.MediaSubTorrentDownloading, CreatedAt: e.Now()}
	require.NoError(t, e.db.Create(&orphan).Error)
	e.svc.refresh(e.ctx)
	var n int64
	require.NoError(t, e.db.Model(&models.MediaSubscriptionTorrent{}).Where("id = ?", orphan.ID).Count(&n).Error)
	assert.Zero(t, n)
}

// 下载了订阅的资源时按设置发通知；剧集写明集
func TestNotifyOnDownload(t *testing.T) {
	e := newEnv(t)
	e.enable(func(s *Settings) { s.NotifyChannels = []uint{3} })
	tv := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	e.search.set(e.item("hdsky", "81", "The.Last.of.Us.S02E01.2160p.WEB-DL.H265-X", "", 8, 10))
	_, err := e.svc.SearchNow(e.ctx, tv.ID)
	require.NoError(t, err)
	require.Len(t, e.notices, 1)
	n := e.notices[0]
	assert.Equal(t, []uint{3}, n.Channels)
	assert.Equal(t, NoticeDownloaded, n.Kind)
	assert.Contains(t, n.Title, "最后生还者 (2025) 第 2 季")
	assert.Contains(t, n.Body, "第 1 集")
	assert.Contains(t, n.Body, "2160p")
	assert.True(t, strings.HasSuffix(n.ImageURL, "/w500/tlou.jpg"))
}

// 下载中的种子 7 天还没入库：标成失败，这些集重新算缺的
func TestStuckTorrentFails(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	tv := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	e.search.set(e.item("hdsky", "91", "The.Last.of.Us.S02E01.1080p.WEB-DL.H264-X", "", 3, 10))
	_, err := e.svc.SearchNow(e.ctx, tv.ID)
	require.NoError(t, err)
	e.advance(8 * 24 * time.Hour)
	e.svc.refresh(e.ctx)
	links := e.linked(tv.ID)
	assert.Equal(t, models.MediaSubTorrentFailed, links[0].Status)
	p, err := e.svc.progress(e.ctx, tv, false)
	require.NoError(t, err)
	assert.Contains(t, p.Missing, 1)
}

func TestExplore(t *testing.T) {
	e := newEnv(t)
	e.organized("h", models.MediaKindMovie, 1, 0, 0, "/lib/x.mkv")
	sub := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	page, err := e.svc.Explore(e.ctx, models.MediaKindMovie, "trending", 1, "")
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	assert.True(t, page.Items[0].Subscribed)
	assert.Equal(t, sub.ID, page.Items[0].SubscriptionID)
	assert.False(t, page.Items[0].InLibrary)
	assert.True(t, page.Items[1].InLibrary)
	assert.False(t, page.Items[1].Subscribed)

	s, err := e.svc.Explore(e.ctx, models.MediaKindTV, ExploreSearch, 1, "最后生还者")
	require.NoError(t, err)
	require.Len(t, s.Items, 1)
	assert.Equal(t, 100088, s.Items[0].ID)
	_, err = e.svc.Explore(e.ctx, models.MediaKindTV, ExploreSearch, 1, " ")
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.Explore(e.ctx, models.MediaKindTV, "top", 1, "")
	require.ErrorIs(t, err, ErrInvalid)
}

func TestDownloaderFor(t *testing.T) {
	e := newEnv(t)
	second := models.DownloaderSetting{Name: "tr", Type: "transmission", URL: "http://tr", Enabled: true}
	require.NoError(t, e.db.Create(&second).Error)
	off := models.DownloaderSetting{Name: "off", Type: "qbittorrent", URL: "http://off"}
	require.NoError(t, e.db.Create(&off).Error)
	set, err := e.svc.Settings(e.ctx)
	require.NoError(t, err)
	id, err := e.svc.downloaderFor(e.ctx, &models.MediaSubscription{}, set)
	require.NoError(t, err)
	assert.Equal(t, uint(1), id, "pt-tools 的默认下载器")
	set.DefaultDownloaderID = second.ID
	id, err = e.svc.downloaderFor(e.ctx, &models.MediaSubscription{}, set)
	require.NoError(t, err)
	assert.Equal(t, second.ID, id)
	id, err = e.svc.downloaderFor(e.ctx, &models.MediaSubscription{DownloaderID: off.ID}, set)
	require.NoError(t, err)
	assert.Equal(t, second.ID, id, "订阅选的下载器停用了就用设置里的")
}

// 半价这类优惠也有结束时间，但不是免费到期：推送时不能写成免费到期，否则免费到期清理会删掉没下完的种子
func TestPushNonFreeDiscountHasNoFreeEnd(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	p, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "好", Resolutions: []string{"2160p", "1080p"}})
	require.NoError(t, err)
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, ProfileID: p.ID})
	half := e.item("hdsky", "21", "Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "沙丘2", 8, 50)
	half.DiscountLevel, half.DiscountEndTime = v2.DiscountPercent50, e.Now().Add(time.Hour)
	e.search.set(half)
	_, err = e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	pushes := e.gotPushes()
	require.Len(t, pushes, 1)
	require.NotNil(t, pushes[0].Meta)
	assert.False(t, pushes[0].Meta.IsFree)
	assert.Nil(t, pushes[0].Meta.FreeEndTime, "半价的结束时间不是免费到期")
}
