package subscribe

import (
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// tlouItems 是按条目搜「最后生还者」第 2 季时各站点回的种子：
//   - h1：同一个资源在 hdsky、ourbits 上都有 InfoHash；mteam 上的没有 InfoHash，但体积、分辨率、制作组、集都一样 → 并进 h1；
//   - h2：hdsky 上的整季包；
//   - pttime 上的 1080p 单集（没有 InfoHash，自己一组）；
//   - 别的剧、第 1 季的种子对不上这个条目。
func (e *env) tlouItems() []v2.TorrentItem {
	ep1 := "The.Last.of.Us.S02E01.2160p.WEB-DL.H265.HDR10-CHDWEB"
	a := e.item("hdsky", "1", ep1, "最后生还者 第二季 第01集", 4, 40)
	a.InfoHash = "h1"
	a2 := e.item("ourbits", "11", ep1, "", 4, 10)
	a2.InfoHash = "H1"
	b := e.item("mteam", "2", ep1, "", 4, 25)
	d := e.item("hdsky", "4", "The.Last.of.Us.S02.2160p.WEB-DL.H265-HHWEB", "最后生还者 第二季 全3集", 12, 5)
	d.InfoHash = "h2"
	c := e.item("pttime", "3", "The.Last.of.Us.S02E01.1080p.WEB-DL.H264-ADWeb", "", 1.5, 200)
	other := e.item("hdsky", "5", "Some.Other.Show.S02E01.1080p.WEB-DL-X", "", 1, 3)
	s1 := e.item("hdsky", "6", "The.Last.of.Us.S01E01.2160p.WEB-DL.H265-CHDWEB", "", 4, 9)
	return []v2.TorrentItem{a, a2, b, d, c, other, s1}
}

func (e *env) searchMedia(in MediaSearchInput) *MediaSearchResult {
	e.t.Helper()
	res, err := e.svc.SearchMedia(e.ctx, in)
	require.NoError(e.t, err)
	return res
}

func groupSites(g MediaSearchGroup) []string {
	var out []string
	for _, m := range g.Members {
		out = append(out, m.Site+":"+m.TorrentID)
	}
	return out
}

// 分组与排序：同 InfoHash 的跨站结果一组，没有 InfoHash 但特征相同的并进去；组内、组间按档案的比法排，首选在最前。
func TestSearchMediaGroupsAndOrder(t *testing.T) {
	e := newEnv(t)
	e.search.set(e.tlouItems()...)
	res := e.searchMedia(MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})

	assert.Equal(t, MediaSearchItem{
		MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Title: "最后生还者", OriginalTitle: "The Last of Us", Year: 2025,
		PosterPath: "/tlou.jpg", Seasons: 2, TotalEpisodes: 3,
	}, res.Item)
	assert.Nil(t, res.Subscription)
	require.NotNil(t, res.Profile, "没有档案时按分辨率与来源比")
	assert.Zero(t, res.Profile.ID)

	require.Len(t, res.Groups, 3)
	assert.Equal(t, []string{"hdsky:1", "mteam:2", "ourbits:11"}, groupSites(res.Groups[0]), "做种多的是首选")
	assert.Equal(t, "h:h1", res.Groups[0].GroupID)
	assert.Equal(t, []string{"hdsky:4"}, groupSites(res.Groups[1]), "同分的组按首选的做种数排")
	assert.Equal(t, []string{"pttime:3"}, groupSites(res.Groups[2]), "1080p 的分低，排最后")
	assert.Equal(t, 5, res.Total)
	assert.Equal(t, 2, res.UnmatchedCount)
	assert.Empty(t, res.Unmatched, "没要时不返回对不上的")

	g := res.Groups[0]
	assert.Equal(t, "2160p", g.Meta.Resolution)
	assert.Equal(t, &MediaSearchSpan{Start: 1, End: 1}, g.Span)
	require.NotNil(t, g.Verdict)
	assert.Empty(t, g.Verdict.Reject)
	assert.Greater(t, g.Verdict.Score, res.Groups[2].Verdict.Score)
	assert.LessOrEqual(t, g.Verdict.Score, 100)
	assert.Equal(t, &MediaSearchSpan{Whole: true}, res.Groups[1].Span)

	assert.Equal(t, []string{"The Last of Us S02", "最后生还者"}, res.Keywords)
	for _, q := range e.search.queries {
		assert.True(t, q.RawTitles)
		assert.True(t, q.KeepDuplicates, "跨站的同一资源要保留每个来源")
	}

	more := e.searchMedia(MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, IncludeUnmatched: true})
	require.Len(t, more.Unmatched, 2)
	for _, m := range more.Unmatched {
		assert.NotEmpty(t, m.Reason)
	}
}

// 桥接：A 是 h1、C 是 h2、B 没有 InfoHash 而三者特征相同。B 不并进任何一个有 InfoHash 的组，A、C 也不因 B 连到一起。
func TestSearchMediaBridgeStaysApart(t *testing.T) {
	e := newEnv(t)
	title := "The.Last.of.Us.S02E01.2160p.WEB-DL.H265-CHDWEB"
	a := e.item("hdsky", "a", title, "", 4, 30)
	a.InfoHash = "h1"
	b := e.item("mteam", "b", title, "", 4, 20)
	c := e.item("ourbits", "c", title, "", 4, 10)
	c.InfoHash = "h2"
	e.search.set(a, b, c)
	res := e.searchMedia(MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	require.Len(t, res.Groups, 3)
	for _, g := range res.Groups {
		assert.Len(t, g.Members, 1, g.GroupID)
	}

	// 特征相同但制作组不同的不合并；体积不知道（0）的也不合并
	x := e.item("mteam", "x", title, "", 4, 20)
	y := e.item("pttime", "y", strings.Replace(title, "CHDWEB", "HHWEB", 1), "", 4, 10)
	z1 := e.item("hdsky", "z1", title, "", 0, 5)
	z2 := e.item("ourbits", "z2", title, "", 0, 5)
	z1.SizeBytes, z2.SizeBytes = 0, 0
	e.search.set(x, y, z1, z2)
	res = e.searchMedia(MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	assert.Len(t, res.Groups, 4)

	// 没有 InfoHash、特征相同的跨站结果一组
	w := e.item("pttime", "w", title, "", 4, 10)
	e.search.set(x, w)
	res = e.searchMedia(MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	require.Len(t, res.Groups, 1)
	assert.Equal(t, []string{"mteam:x", "pttime:w"}, groupSites(res.Groups[0]))
}

// 同一批结果打乱顺序 20 次，分组、组的顺序与组内顺序都不变。
func TestSearchMediaStableUnderShuffle(t *testing.T) {
	e := newEnv(t)
	items := e.tlouItems()
	tie := e.item("hdhome", "7", "The.Last.of.Us.S02.2160p.WEB-DL.H265-HHWEB", "", 12, 5)
	tie.InfoHash = "h2"
	items = append(items, tie)
	e.search.set(items...)
	want := e.searchMedia(MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	shape := func(r *MediaSearchResult) []string {
		var out []string
		for _, g := range r.Groups {
			out = append(out, g.GroupID+"="+strings.Join(groupSites(g), ","))
		}
		return out
	}
	require.Contains(t, shape(want), "h:h2=hdhome:7,hdsky:4", "完全同分时按站点名排")
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 20; i++ {
		rng.Shuffle(len(items), func(a, b int) { items[a], items[b] = items[b], items[a] })
		e.search.set(items...)
		got := e.searchMedia(MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
		require.Equal(t, shape(want), shape(got), "第 %d 次", i)
	}
}

// 每集的情况只从库里读：已入库、下载中（订阅的种子与带这个条目的推送记录，带站点）、缺、还没播；每组写明能补上哪几集。
func TestSearchMediaEpisodes(t *testing.T) {
	e := newEnv(t)
	e.organized("x1", models.MediaKindTV, 100088, 2, 1, "/lib/tlou/s02e01.mkv")
	pushed := true
	h := "pushedhash"
	require.NoError(t, e.db.Create(&models.TorrentInfo{
		SiteName: "ourbits", TorrentID: "99", TorrentHash: &h, Title: "The.Last.of.Us.S02E02.1080p.WEB-DL-XX", IsPushed: &pushed,
		MediaType: models.MediaKindTV, TMDBID: 100088, MediaSeason: 2,
	}).Error)
	done := "donehash"
	require.NoError(t, e.db.Create(&models.TorrentInfo{
		SiteName: "ourbits", TorrentID: "98", TorrentHash: &done, Title: "The.Last.of.Us.S02E03.1080p.WEB-DL-XX", IsPushed: &pushed,
		IsCompleted: true, MediaType: models.MediaKindTV, TMDBID: 100088, MediaSeason: 2,
	}).Error)
	e.search.set(e.tlouItems()...)
	res := e.searchMedia(MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})

	require.Len(t, res.Episodes, 3)
	assert.Equal(t, MediaSearchEpisode{Episode: 1, State: models.MediaEpisodeLibrary, AirDate: "2025-04-13", Name: "未来"}, res.Episodes[0])
	assert.Equal(t, MediaSearchEpisode{Episode: 2, State: models.MediaEpisodeDownloading, Site: "ourbits", AirDate: "2025-04-20", Name: "穿越"}, res.Episodes[1])
	assert.Equal(t, models.MediaEpisodeUpcoming, res.Episodes[2].State, "下完了的推送记录不算在下载；第 3 集还没播")

	single := res.Groups[0]
	assert.True(t, single.InLibrary, "第 1 集已入库")
	assert.Empty(t, single.Covers)
	pack := res.Groups[1]
	assert.False(t, pack.InLibrary)
	assert.Equal(t, []int{2}, pack.Downloading)
	assert.Empty(t, pack.Covers, "缺的只有在下载的第 2 集")

	// 第 2 集不在下载了：整季包能补上它
	require.NoError(t, e.db.Model(&models.TorrentInfo{}).Where("torrent_id = ?", "99").Update("is_completed", true).Error)
	res = e.searchMedia(MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	assert.Equal(t, models.MediaEpisodeMissing, res.Episodes[1].State)
	assert.Equal(t, []int{2}, res.Groups[1].Covers)
}

// 订阅的种子在下载、台账里等版本的集：照台账读，不重建。
func TestSearchMediaSubscriptionStates(t *testing.T) {
	e := newEnv(t)
	sub := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	require.NoError(t, e.db.Create(&models.MediaSubscriptionTorrent{
		SubscriptionID: sub.ID, SiteName: "hdsky", TorrentID: "50", Title: "The.Last.of.Us.S02E01.1080p-X", Status: models.MediaSubTorrentDownloading,
		Episode: 1,
	}).Error)
	require.NoError(t, e.db.Create(&models.MediaSubscriptionTorrent{
		SubscriptionID: sub.ID, SiteName: models.MediaAdoptedSitePrefix + "1", TorrentID: "abc", Title: "x", Status: models.MediaSubTorrentDownloading,
		Episode: 3, Adopted: true,
	}).Error)
	require.NoError(t, e.db.Where("subscription_id = ? AND episode = ?", sub.ID, 2).Delete(&models.MediaSubscriptionEpisode{}).Error)
	require.NoError(t, e.db.Create(&models.MediaSubscriptionEpisode{SubscriptionID: sub.ID, Episode: 2, State: models.MediaEpisodeWaitingVersion}).Error)
	e.search.set(e.tlouItems()...)
	res := e.searchMedia(MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	require.NotNil(t, res.Subscription)
	assert.Equal(t, sub.ID, res.Subscription.ID)
	assert.Equal(t, models.MediaSubActive, res.Subscription.Status)
	assert.Equal(t, MediaSearchEpisode{Episode: 1, State: models.MediaEpisodeDownloading, Site: "hdsky", AirDate: "2025-04-13", Name: "未来"}, res.Episodes[0])
	assert.Equal(t, models.MediaEpisodeWaitingVersion, res.Episodes[1].State)
	assert.Equal(t, models.MediaEpisodeDownloading, res.Episodes[2].State)
	assert.Empty(t, res.Episodes[2].Site, "下载器里认领的种子不写站点")
	assert.Equal(t, []int{2}, res.Groups[1].Covers, "等版本的集也算能补上")
}

// 档案只影响这一次搜索：订阅用档案 A 时，请求 0 按 A、请求 B 按 B、请求 -1 不打分，都不改订阅；没有订阅时请求 0 用默认档案。
func TestSearchMediaProfiles(t *testing.T) {
	e := newEnv(t)
	a, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "只要 4K", Resolutions: []string{"2160p"}, Codecs: []string{"H.265"}})
	require.NoError(t, err)
	b, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "只要 1080p", Resolutions: []string{"1080p"}})
	require.NoError(t, err)
	sub := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, ProfileID: a.ID})
	e.search.set(e.tlouItems()...)
	in := MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2}
	verdictOf := func(r *MediaSearchResult, site string) *MediaSearchVerdict {
		for _, g := range r.Groups {
			for _, m := range g.Members {
				if m.Site == site {
					return m.Verdict
				}
			}
		}
		t.Fatalf("没有 %s 的结果", site)
		return nil
	}

	res := e.searchMedia(in)
	assert.Equal(t, &MediaSearchProfile{ID: a.ID, Name: "只要 4K"}, res.Profile)
	assert.Empty(t, verdictOf(res, "hdsky").Reject)
	assert.Equal(t, "分辨率 1080p 不在档案里；编码 H.264 不在档案里", verdictOf(res, "pttime").Reject, "列出全部原因")
	assert.Equal(t, "pttime:3", strings.Join(groupSites(res.Groups[len(res.Groups)-1]), ","), "不符合的排在后面")

	in.ProfileID = int(b.ID)
	res = e.searchMedia(in)
	assert.Equal(t, b.ID, res.Profile.ID)
	assert.Empty(t, verdictOf(res, "pttime").Reject)
	assert.Contains(t, verdictOf(res, "hdsky").Reject, "2160p")
	assert.Equal(t, []string{"pttime:3"}, groupSites(res.Groups[0]), "通过档案的在前")

	in.ProfileID = -1
	res = e.searchMedia(in)
	assert.Nil(t, res.Profile)
	assert.Nil(t, verdictOf(res, "pttime"), "不按档案时不打分")
	assert.Equal(t, []string{"pttime:3"}, groupSites(res.Groups[0]), "不打分时做种多的在前")

	assert.Equal(t, a.ID, e.subRow(sub.ID).ProfileID, "订阅的档案没变")

	// 没有订阅：请求 0 用设置里的默认档案
	e.tmdb.set("/3/tv/100088/season/1", `{"season_number":1,"air_date":"2023-01-15","episodes":[{"episode_number":1,"name":"当你迷失在黑暗中","air_date":"2023-01-15"}]}`)
	e.enable(func(s *Settings) { s.DefaultProfileID = b.ID })
	res = e.searchMedia(MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 1})
	assert.Nil(t, res.Subscription)
	assert.Equal(t, b.ID, res.Profile.ID)

	_, err = e.svc.SearchMedia(e.ctx, MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, ProfileID: 999})
	assert.ErrorIs(t, err, ErrInvalid, "档案不存在")
}

// 只读：不推送、不写运行记录、不重建追剧台账，订阅本身不变；订阅的标题关键词不符合的写进原因。
func TestSearchMediaReadOnly(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	exclude := "ADWeb"
	sub := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, ExcludeKeywords: &exclude})
	before := e.subRow(sub.ID)
	var runs, eps int64
	require.NoError(t, e.db.Model(&models.MediaSubscriptionRun{}).Count(&runs).Error)
	require.NoError(t, e.db.Model(&models.MediaSubscriptionEpisode{}).Count(&eps).Error)
	e.search.set(e.tlouItems()...)

	res := e.searchMedia(MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	last := res.Groups[len(res.Groups)-1]
	require.Equal(t, "pttime", last.Members[0].Site)
	assert.Contains(t, last.Verdict.Reject, "标题含排除词 adweb")

	assert.Empty(t, e.gotPushes())
	var runs2, eps2 int64
	require.NoError(t, e.db.Model(&models.MediaSubscriptionRun{}).Count(&runs2).Error)
	require.NoError(t, e.db.Model(&models.MediaSubscriptionEpisode{}).Count(&eps2).Error)
	assert.Equal(t, runs, runs2, "不写运行记录")
	assert.Equal(t, eps, eps2, "不重建台账")
	after := e.subRow(sub.ID)
	assert.Equal(t, before.UpdatedAt, after.UpdatedAt)
	assert.Equal(t, before.Message, after.Message)
	assert.Equal(t, before.NextSearchAt, after.NextSearchAt)
}

// 电影：库里有时整组写「库里都有了」；推送了还没下完时写在下载。站点出错的写进 errors。
func TestSearchMediaMovie(t *testing.T) {
	e := newEnv(t)
	end := e.Now().Add(5 * time.Hour)
	it := e.item("hdsky", "m1", "Dune.Part.Two.2024.2160p.UHD.BluRay.Remux.HDR-FRDS", "沙丘2 中字", 60, 30)
	it.DiscountLevel, it.DiscountEndTime, it.HasHR = v2.DiscountFree, end, true
	e.search.set(it)
	e.search.siteErrs = []v2.SearchError{{Site: "mteam", Error: "timeout"}}
	res := e.searchMedia(MediaSearchInput{MediaType: models.MediaKindMovie, TMDBID: 693134, Season: 3})
	assert.Zero(t, res.Item.Season, "电影没有季")
	assert.Empty(t, res.Episodes)
	require.NotNil(t, res.Movie)
	assert.Equal(t, models.MediaEpisodeMissing, res.Movie.State)
	require.Len(t, res.Groups, 1)
	m := res.Groups[0].Members[0]
	assert.True(t, m.Free)
	assert.Equal(t, int64(5*3600), m.FreeLeftSeconds)
	assert.True(t, m.HR)
	assert.Equal(t, string(v2.DiscountFree), m.Discount)
	assert.Equal(t, []MediaSearchError{{Site: "mteam", Error: "timeout"}}, res.Errors)
	assert.Nil(t, res.Groups[0].Span)

	e.organized("dunehash", models.MediaKindMovie, 693134, 0, 0, "/lib/movies/dune2.mkv")
	res = e.searchMedia(MediaSearchInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	assert.Equal(t, models.MediaEpisodeLibrary, res.Movie.State)
	assert.True(t, res.Groups[0].InLibrary)
}

func TestSearchMediaInvalid(t *testing.T) {
	e := newEnv(t)
	for _, in := range []MediaSearchInput{
		{MediaType: "book", TMDBID: 1},
		{MediaType: models.MediaKindTV},
		{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 201},
		{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, ProfileID: -2},
	} {
		_, err := e.svc.SearchMedia(e.ctx, in)
		assert.ErrorIs(t, err, ErrInvalid, "%+v", in)
	}
	e.search.err = assert.AnError
	_, err := e.svc.SearchMedia(e.ctx, MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	assert.Error(t, err, "每个关键字都搜失败时报错")
}
