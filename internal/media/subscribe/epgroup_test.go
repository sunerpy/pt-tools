package subscribe

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/epgroup"
	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 剧集组（M9b）：最后生还者（100088）在假 TMDB 上有两个剧集组——
//   - 绝对集号（一组）：第 1 季 9 集接着第 2 季 3 集，绝对集号 10 是第 2 季第 1 集；
//   - DVD（特别篇一组 + 两组）：第 1 组是第 1 季，第 2 组是第 2 季，但顺序是第 2、1、3 集（换算出的集不连着）。
const (
	tlouAbsolute = "5b11ba820e0a265a53000ce9"
	tlouDVD      = "5c8a1ef80e0a2625f5a4f1b0"
)

func groupEpisodes(season, from, to, order int) string {
	var parts []string
	for n := from; n <= to; n++ {
		parts = append(parts, fmt.Sprintf(`{"season_number":%d,"episode_number":%d,"order":%d}`, season, n, order))
		order++
	}
	return strings.Join(parts, ",")
}

// withGroups 在假 TMDB 上加最后生还者的两个剧集组。
func (e *env) withGroups() {
	e.tmdb.set("/3/tv/100088/episode_groups", `{"id":100088,"results":[
		{"id":"`+tlouAbsolute+`","name":"Absolute Order","type":2,"group_count":1,"episode_count":12},
		{"id":"`+tlouDVD+`","name":"DVD","type":3,"group_count":3,"episode_count":13,"network":{"name":"HBO"}}]}`)
	e.tmdb.set("/3/tv/episode_group/"+tlouAbsolute, `{"id":"`+tlouAbsolute+`","name":"Absolute Order","type":2,"groups":[
		{"id":"g1","name":"Episodes","order":1,"episodes":[`+groupEpisodes(1, 1, 9, 0)+`,`+groupEpisodes(2, 1, 3, 9)+`]}]}`)
	e.tmdb.set("/3/tv/episode_group/"+tlouDVD, `{"id":"`+tlouDVD+`","name":"DVD","type":3,"groups":[
		{"id":"s","name":"Specials","order":0,"episodes":[{"season_number":0,"episode_number":1,"order":0}]},
		{"id":"d1","name":"Disc 1","order":1,"episodes":[`+groupEpisodes(1, 1, 9, 0)+`]},
		{"id":"d2","name":"Disc 2","order":2,"episodes":[
			{"season_number":2,"episode_number":2,"order":0},{"season_number":2,"episode_number":1,"order":1},{"season_number":2,"episode_number":3,"order":2}]}]}`)
}

// tlouGroupSub 建最后生还者第 2 季的订阅，种子按 group 写季集。
func (e *env) tlouGroupSub(group string) *models.MediaSubscription {
	e.t.Helper()
	e.withGroups()
	return e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, EpisodeGroupID: &group})
}

func groupMapper(t *testing.T, g *tmdb.EpisodeGroup, season int, episodes ...int) *episodeMapper {
	t.Helper()
	se := &tmdb.Season{Number: season}
	for _, n := range episodes {
		se.Episodes = append(se.Episodes, tmdb.Episode{Number: n})
	}
	mp := newEpisodeMapper(newEpisodeIndex(se, nil), nil)
	mp.group, mp.season = epgroup.New(g), season
	return mp
}

func absoluteGroup() *tmdb.EpisodeGroup {
	var eps []tmdb.EpisodeGroupEpisode
	for n := 1; n <= 9; n++ {
		eps = append(eps, tmdb.EpisodeGroupEpisode{Season: 1, Episode: n, Order: n - 1})
	}
	for n := 1; n <= 3; n++ {
		eps = append(eps, tmdb.EpisodeGroupEpisode{Season: 2, Episode: n, Order: 8 + n})
	}
	return &tmdb.EpisodeGroup{EpisodeGroupSummary: tmdb.EpisodeGroupSummary{ID: tlouAbsolute, Name: "Absolute Order"}, Groups: []tmdb.EpisodeGroupPart{{Episodes: eps}}}
}

func TestRegroup(t *testing.T) {
	mp := groupMapper(t, absoluteGroup(), 2, 1, 2, 3)
	cases := []struct {
		name string
		in   meta.Meta
		want meta.Meta
		res  regrouped
	}{
		{"绝对集号 10 是第 2 季第 1 集", meta.Meta{Episode: 10}, meta.Meta{Season: 2, Episode: 1}, regrouped{}},
		{"跨季的范围只留这一季的", meta.Meta{Episode: 9, EpisodeEnd: 11}, meta.Meta{Season: 2, Episode: 1, EpisodeEnd: 2}, regrouped{}},
		{"都在第 1 季：不是这个订阅的", meta.Meta{Episode: 5}, meta.Meta{Episode: 5}, regrouped{other: true}},
		{
			"剧集组里没有：照默认的季集",
			meta.Meta{Season: 2, Episode: 2},
			meta.Meta{Season: 2, Episode: 2},
			regrouped{note: "剧集组「Absolute Order」里没有第 2 组第 2 集，按 TMDB 默认的季集算"},
		},
		{"一整组正好是这一季的全部集：还是整季", meta.Meta{Season: 1}, meta.Meta{Season: 2}, regrouped{}},
		{"季集都没写：不换算", meta.Meta{NameEN: "x"}, meta.Meta{NameEN: "x"}, regrouped{}},
	}
	for _, c := range cases {
		got, res := mp.regroup(c.in)
		assert.Equal(t, c.want, got, c.name)
		assert.Equal(t, c.res, res, c.name)
	}

	// 这一季多了一集（TMDB 上第 2 季有 4 集）：一整组换算出的 3 集不是整季，写成范围
	four := groupMapper(t, absoluteGroup(), 2, 1, 2, 3, 4)
	got, _ := four.regroup(meta.Meta{Season: 1})
	assert.Equal(t, meta.Meta{Season: 2, Episode: 1, EpisodeEnd: 3}, got)
	// 不知道这一季有哪些集时也写成范围
	noIndex := groupMapper(t, absoluteGroup(), 2)
	noIndex.index = nil
	got, _ = noIndex.regroup(meta.Meta{Season: 1})
	assert.Equal(t, meta.Meta{Season: 2, Episode: 1, EpisodeEnd: 3}, got)

	// 换算出的集不连着：不要
	shuffled := groupMapper(t, &tmdb.EpisodeGroup{EpisodeGroupSummary: tmdb.EpisodeGroupSummary{Name: "DVD"}, Groups: []tmdb.EpisodeGroupPart{{Episodes: []tmdb.EpisodeGroupEpisode{
		{Season: 2, Episode: 1}, {Season: 2, Episode: 3}, {Season: 2, Episode: 2},
	}}}}, 2, 1, 2, 3)
	_, res := shuffled.regroup(meta.Meta{Episode: 1, EpisodeEnd: 2})
	assert.Equal(t, "按剧集组「DVD」换算是第 2 季的 E01、E03，不连着，没有用它", res.reject)

	// 读不到剧集组：不要，写明原因
	broken := &episodeMapper{groupErr: "读不到剧集组：TMDB 不能访问", season: 2}
	_, res = broken.regroup(meta.Meta{Episode: 10})
	assert.Equal(t, "读不到剧集组：TMDB 不能访问", res.reject)

	// 没选剧集组：原样
	var none *episodeMapper
	got, res = none.regroup(meta.Meta{Episode: 10})
	assert.Equal(t, meta.Meta{Episode: 10}, got)
	assert.Equal(t, regrouped{}, res)
	assert.Nil(t, none.forTorrent(meta.Meta{Season: 2}))
	plain := newEpisodeMapper(nil, nil)
	assert.Same(t, plain, plain.forTorrent(meta.Meta{Season: 2}), "没选剧集组时就是原来的映射")
}

// 文件名里只写了集：按种子标题写的组算（标题只写了集时是第 1 组）。
func TestRegroupFilesUseTitleSeason(t *testing.T) {
	g := &tmdb.EpisodeGroup{Groups: []tmdb.EpisodeGroupPart{
		{Episodes: []tmdb.EpisodeGroupEpisode{{Season: 1, Episode: 1}, {Season: 1, Episode: 2}}},
		{Episodes: []tmdb.EpisodeGroupEpisode{{Season: 1, Episode: 3}, {Season: 2, Episode: 1}}},
	}}
	mp := groupMapper(t, g, 2, 1)
	pack := mp.forTorrent(meta.Meta{Season: 2})
	got, res := pack.regroup(meta.Meta{Episode: 2})
	assert.Equal(t, regrouped{}, res)
	assert.Equal(t, meta.Meta{Season: 2, Episode: 1}, got, "第 2 组第 2 集")
	assert.Zero(t, mp.packSeason, "forTorrent 不改原来的映射")
	_, res = mp.regroup(meta.Meta{Episode: 2})
	assert.True(t, res.other, "种子标题没写季：第 1 组第 2 集是第 1 季的")
}

func TestEpisodeGroupInput(t *testing.T) {
	e := newEnv(t)
	e.withGroups()
	list, err := e.svc.EpisodeGroups(e.ctx, 100088)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, tmdb.EpisodeGroupSummary{ID: tlouDVD, Name: "DVD", Type: tmdb.EpisodeGroupDVD, GroupCount: 3, EpisodeCount: 13, Network: "HBO"}, list[1])
	_, err = e.svc.EpisodeGroups(e.ctx, 0)
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.EpisodeGroups(e.ctx, 404404)
	require.ErrorIs(t, err, tmdb.ErrNotFound)

	tv := func(group string) SubscriptionInput {
		return SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, EpisodeGroupID: &group}
	}
	for _, c := range []struct{ group, want string }{
		{"abc", "剧集组编号不对"},
		{"5b11ba820e0a265a53000000", "TMDB 上这部剧没有这个剧集组"},
	} {
		_, err = e.svc.CreateSubscription(e.ctx, tv(c.group), models.MediaSubFromManual)
		require.ErrorIs(t, err, ErrInvalid, c.group)
		assert.Contains(t, err.Error(), c.want)
	}
	group := tlouAbsolute
	_, err = e.svc.CreateSubscription(e.ctx, SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, EpisodeGroupID: &group}, models.MediaSubFromManual)
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "电影没有剧集组")

	sub := e.sub(tv(" " + tlouAbsolute + " "))
	assert.Equal(t, tlouAbsolute, sub.EpisodeGroupID, "去掉空白")

	// 改成另一个剧集组：马上搜一次，运行记录里写着
	e.db.Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).Update("next_search_at", e.Now().Add(48*3600e9))
	e.advance(60e9)
	dvd := tlouDVD
	up, err := e.svc.UpdateSubscription(e.ctx, sub.ID, SubscriptionInput{EpisodeGroupID: &dvd})
	require.NoError(t, err)
	assert.Equal(t, tlouDVD, up.EpisodeGroupID)
	require.NotNil(t, up.NextSearchAt)
	sameTime(t, e.Now(), *up.NextSearchAt, "换了剧集组马上搜")
	assert.Contains(t, e.runs(sub.ID)[0].Summary, "种子的季集按 TMDB 剧集组换算")

	// 不带时不变；写错的不改；空串是不用剧集组
	up, err = e.svc.UpdateSubscription(e.ctx, sub.ID, SubscriptionInput{})
	require.NoError(t, err)
	assert.Equal(t, tlouDVD, up.EpisodeGroupID)
	bad := "5b11ba820e0a265a53000000"
	_, err = e.svc.UpdateSubscription(e.ctx, sub.ID, SubscriptionInput{EpisodeGroupID: &bad})
	require.ErrorIs(t, err, ErrInvalid)
	assert.Equal(t, tlouDVD, e.subRow(sub.ID).EpisodeGroupID)
	empty := ""
	up, err = e.svc.UpdateSubscription(e.ctx, sub.ID, SubscriptionInput{EpisodeGroupID: &empty})
	require.NoError(t, err)
	assert.Empty(t, up.EpisodeGroupID)
	assert.NotContains(t, describe(up), "剧集组")
}

// 订阅对种子：按绝对集号写的第 10 集是第 2 季第 1 集；剧集组里没有的照默认的季集（第 2 季第 2 集照样要），
// 默认的季集也不是这一季的写明原因；换算出是第 1 季的不写原因。
func TestEpisodeGroupMatchAbsolute(t *testing.T) {
	e := newEnv(t)
	sub := e.tlouGroupSub(tlouAbsolute)
	e.enable(nil)
	abs := e.item("hdsky", "101", "The.Last.of.Us.E10.1080p.WEB-DL.H264-GRP", "", 2, 10)
	raw := e.item("hdsky", "102", "The.Last.of.Us.S02E02.1080p.WEB-DL.H264-GRP", "", 2, 10)
	s1 := e.item("hdsky", "103", "The.Last.of.Us.E05.1080p.WEB-DL.H264-GRP", "", 2, 10)
	missing := e.item("hdsky", "104", "The.Last.of.Us.E15.1080p.WEB-DL.H264-GRP", "", 2, 10)
	e.search.set(abs, raw, s1, missing)

	msg, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"101", "102"}, pushedIDs(e), msg)
	eps := map[string]int{}
	for _, l := range e.linked(sub.ID) {
		eps[l.TorrentID] = l.Episode
	}
	assert.Equal(t, map[string]int{"101": 1, "102": 2}, eps, "台账按 TMDB 默认的集")
	steps := runSteps(e, sub.ID)
	assert.Contains(t, steps, "info:The.Last.of.Us.S02E02.1080p.WEB-DL.H264-GRP：剧集组「Absolute Order」里没有第 2 组第 2 集，按 TMDB 默认的季集算")
	assert.Contains(t, steps, "reject:The.Last.of.Us.E15.1080p.WEB-DL.H264-GRP：剧集组「Absolute Order」里没有第 1 组第 15 集，按 TMDB 默认的季集算，不是第 2 季的")
	assert.NotContains(t, steps, "E05", "换算出是第 1 季的不是这个追剧的种子，不写")
	assert.Empty(t, e.ledger(sub).missing, "第 1、2 集都在下载（第 3 集还没播）")
}

// 换算出的集不连着的不要；RSS 交来的种子按名字快速筛，季集在评估时换算。
func TestEpisodeGroupRejectAndRSS(t *testing.T) {
	e := newEnv(t)
	sub := e.tlouGroupSub(tlouDVD)
	e.enable(nil)
	// DVD 第 2 组是第 2 季的第 2、1、3 集：第 2–3 集换算出的第 1、3 集不连着
	pair := e.item("hdsky", "201", "The.Last.of.Us.S02E02-E03.1080p.WEB-DL.H264-GRP", "", 2, 10)
	e.search.set(pair)
	msg, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Empty(t, pushedIDs(e), msg)
	assert.Contains(t, msg, "按剧集组「DVD」换算是第 2 季的 E01、E03，不连着，没有用它")

	// DVD 第 2 组第 1 集是第 2 季第 2 集：RSS 交来的 S02E01 按名字放过快速筛，评估时换算后推送
	e.svc.considerOffer(e.ctx, Candidate{Site: "hdsky", Item: e.item("hdsky", "202", "The.Last.of.Us.S02E01.1080p.WEB-DL.H264-GRP", "", 2, 10), From: fromRSS, SeenAt: e.Now()})
	assert.Equal(t, []string{"202"}, pushedIDs(e))
	links := e.linked(sub.ID)
	require.Len(t, links, 1)
	assert.Equal(t, 2, links[0].Episode)

	// 读不到剧集组时不要种子，写明原因
	e.tmdb.set("/3/tv/episode_group/"+tlouDVD, "404")
	e.db.Where("key LIKE ?", "%epgroup%").Delete(&models.MediaCache{})
	e.search.set(e.item("hdsky", "203", "The.Last.of.Us.S02E03.1080p.WEB-DL.H264-GRP", "", 2, 10))
	msg, err = e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"202"}, pushedIDs(e), msg)
	assert.Contains(t, msg, "TMDB 上找不到追剧选的剧集组（可能被删了）：在追剧的「修改」里换一个剧集组或不用剧集组")
}

// 整季包里的文件：文件名里只写了集的按种子标题写的组换算，只下缺的集。
func TestEpisodeGroupPackFiles(t *testing.T) {
	e := newEnv(t)
	// 第 3 集也播完了（建订阅以前改：季详情有缓存）
	e.tmdb.set("/3/tv/100088/season/2", `{"season_number":2,"episodes":[{"episode_number":1,"air_date":"2025-04-13"},
		{"episode_number":2,"air_date":"2025-04-20"},{"episode_number":3,"air_date":"2025-04-27"}]}`)
	sub := e.tlouGroupSub(tlouAbsolute)
	e.enable(nil)
	e.organized("h1", models.MediaKindTV, 100088, 2, 1, "/lib/tlou/S02E01.mkv")
	title := "The.Last.of.Us.E10-E12.1080p.WEB-DL.H264-GRP"
	files := []string{"The.Last.of.Us.E10.mkv", "The.Last.of.Us.E11.mkv", "The.Last.of.Us.E12.mkv"}
	pack := e.item("hdsky", "301", title, "", 6, 10)
	e.site.files["301"] = packBytes(title, files...)
	hash := e.hashOf("301")
	var list []downloader.TorrentFile
	for i, f := range files {
		list = append(list, downloader.TorrentFile{Index: i, Name: title + "/" + f, Priority: 1})
	}
	e.dl.setFiles(hash, list...)
	e.search.set(pack)

	msg, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"301"}, pushedIDs(e), msg)
	assert.Contains(t, msg, "只下 E02、E03")
	assert.Equal(t, []prioCall{{ID: hash, Indexes: []int{0}, Priority: 0}}, e.dl.prioCalls(), "绝对集号 10 是库里已有的第 1 集")
	links := e.linked(sub.ID)
	require.Len(t, links, 1)
	assert.Equal(t, "[2,3]", links[0].CoveredEpisodes)
}

// 整理入库拿到追剧选的剧集组；追完以后建的下一季沿用。
func TestEpisodeGroupMediaForAndNextSeason(t *testing.T) {
	e := newEnv(t)
	sub := e.tlouGroupSub(tlouAbsolute)
	require.NoError(t, e.db.Create(&models.MediaSubscriptionTorrent{SubscriptionID: sub.ID, InfoHash: "abc", DownloaderID: 1, SiteName: "hdsky", TorrentID: "9"}).Error)
	hint, found, err := e.svc.MediaFor(e.ctx, 1, "ABC")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, tlouAbsolute, hint.EpisodeGroup)
	assert.Equal(t, 100088, hint.TMDBID)

	e.tmdb.set("/3/tv/100088/season/3", `{"season_number":3,"episodes":[{"episode_number":1,"name":"一","air_date":"2027-01-10"}]}`)
	row := e.subRow(sub.ID)
	e.svc.followNextSeason(e.ctx, &row)
	row = e.subRow(sub.ID)
	require.NotZero(t, row.NextSeasonID)
	assert.Equal(t, tlouAbsolute, e.subRow(row.NextSeasonID).EpisodeGroupID, "剧集组是这部剧的：下一季沿用")
}
