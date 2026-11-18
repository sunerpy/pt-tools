package subscribe

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func (e *env) block(in BlockInput) *BlockResult {
	e.t.Helper()
	res, err := e.svc.AddBlocks(e.ctx, in)
	require.NoError(e.t, err)
	return res
}

func runTexts(runs []RunView) string {
	var b strings.Builder
	for _, r := range runs {
		b.WriteString(r.Summary + "\n")
		for _, st := range r.Steps {
			b.WriteString(st.Text + "\n")
		}
	}
	return b.String()
}

// 拉黑的输入：剧集要具体的季；对所有条目的类型、季、条目名清空；下载器里认领的种子不能拉黑；原因有上限。
func TestAddBlocksValidation(t *testing.T) {
	e := newEnv(t)
	item := []BlockItem{{Site: "hdsky", TorrentID: "1", Title: "A"}}
	cases := map[string]BlockInput{
		"没有种子":   {MediaType: models.MediaKindMovie, TMDBID: 693134},
		"剧集没有季":  {MediaType: models.MediaKindTV, TMDBID: 100088, Items: item},
		"类型不对":   {MediaType: "music", TMDBID: 1, Items: item},
		"编号是负的":  {TMDBID: -1, Items: item},
		"站点为空":   {MediaType: models.MediaKindMovie, TMDBID: 693134, Items: []BlockItem{{TorrentID: "1"}}},
		"认领的种子":  {MediaType: models.MediaKindMovie, TMDBID: 693134, Items: []BlockItem{{Site: "downloader:1", TorrentID: "ab"}}},
		"原因太长":   {MediaType: models.MediaKindMovie, TMDBID: 693134, Items: item, Reason: strings.Repeat("长", maxBlockReason+1)},
		"没有订阅排除": {MediaType: models.MediaKindMovie, TMDBID: 693134, Items: item, ExcludeGroup: true},
	}
	for name, in := range cases {
		_, err := e.svc.AddBlocks(e.ctx, in)
		require.ErrorIs(t, err, ErrInvalid, name)
	}
	many := make([]BlockItem, maxBlockItems+1)
	for i := range many {
		many[i] = BlockItem{Site: "hdsky", TorrentID: string(rune('a' + i%26)), Title: "x"}
	}
	_, err := e.svc.AddBlocks(e.ctx, BlockInput{Items: many})
	require.ErrorIs(t, err, ErrInvalid)

	res := e.block(BlockInput{MediaType: models.MediaKindTV, TMDBID: 0, Season: 3, MediaTitle: "丢掉", Items: item, Reason: "  假种 "})
	require.Len(t, res.Blocks, 1)
	b := res.Blocks[0]
	assert.Equal(t, "", b.MediaType, "对所有条目的不记类型")
	assert.Zero(t, b.Season)
	assert.Empty(t, b.MediaTitle)
	assert.Equal(t, "假种", b.Reason)
}

// 同一个种子对同一个条目再拉黑一次只改原因；对所有条目与对一个条目分开记；列表新的在前；移出黑名单。
func TestAddBlocksUpsertListRemove(t *testing.T) {
	e := newEnv(t)
	movie := BlockInput{
		MediaType: models.MediaKindMovie, TMDBID: 693134, Season: 9, MediaTitle: "沙丘2 (2024)", Reason: "音画不同步",
		Items: []BlockItem{{Site: "hdsky", TorrentID: "12", Title: "Dune.Part.Two"}, {Site: "HDSky", TorrentID: "12", Title: "重复的"}},
	}
	first := e.block(movie)
	require.Len(t, first.Blocks, 1, "同一个种子（站点不分大小写）一次只记一行")
	assert.Zero(t, first.Blocks[0].Season, "电影不记季")
	assert.Equal(t, "沙丘2 (2024)", first.Blocks[0].MediaTitle)

	e.advance(1)
	movie.Reason, movie.Items = "画质差", movie.Items[:1]
	again := e.block(movie)
	require.Len(t, again.Blocks, 1)
	assert.Equal(t, first.Blocks[0].ID, again.Blocks[0].ID, "同一个键改原因，不新增")
	assert.Equal(t, "画质差", again.Blocks[0].Reason)

	e.advance(1)
	global := e.block(BlockInput{Items: []BlockItem{{Site: "hdsky", TorrentID: "12", Title: "Dune.Part.Two"}}})
	assert.NotEqual(t, first.Blocks[0].ID, global.Blocks[0].ID, "对所有条目的另记一行")

	all, err := e.svc.Blocks(e.ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, global.Blocks[0].ID, all[0].ID, "新的在前")

	idx, err := e.svc.blocksFor(e.ctx, models.MediaKindMovie, 693134, 0)
	require.NoError(t, err)
	got, ok := idx.get("HDSKY", "12")
	require.True(t, ok)
	assert.Equal(t, first.Blocks[0].ID, got.ID, "两种都有时用这个条目的（原因更具体）")
	other, err := e.svc.blocksFor(e.ctx, models.MediaKindMovie, 1, 0)
	require.NoError(t, err)
	got, ok = other.get("hdsky", "12")
	require.True(t, ok, "对所有条目的对别的条目也生效")
	assert.Zero(t, got.TMDBID)

	globals, err := e.svc.GlobalBlocks(e.ctx)
	require.NoError(t, err)
	require.Len(t, globals, 1)
	assert.Equal(t, global.Blocks[0].ID, globals[BlockKey("HDSky", "12")].ID)

	require.NoError(t, e.svc.RemoveBlock(e.ctx, global.Blocks[0].ID))
	require.ErrorIs(t, e.svc.RemoveBlock(e.ctx, global.Blocks[0].ID), ErrNotFound)
	all, err = e.svc.Blocks(e.ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

// 订阅不再选拉黑的种子（主动搜索与 RSS 都走 evaluate），运行记录写「已拉黑：原因」；都拉黑了时说明写「都已拉黑」；
// 按条目存：删了订阅再建仍然有效。
func TestEvaluateSkipsBlocked(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	best := e.item("hdsky", "12", "Dune.Part.Two.2024.2160p.UHD.BluRay.x265-FRDS", "沙丘2", 60, 20)
	next := e.item("hdsky", "11", "Dune.Part.Two.2024.1080p.WEB-DL.H264-X", "沙丘2", 8, 50)
	e.search.set(best, next)
	res := e.block(BlockInput{
		MediaType: models.MediaKindMovie, TMDBID: 693134, Reason: "音画不同步",
		Items: []BlockItem{{Site: "hdsky", TorrentID: "12", Title: best.Title}},
	})
	require.Len(t, res.Blocks, 1)
	assert.Contains(t, runTexts(e.runs(m.ID)), "不要这个种子："+best.Title+"（音画不同步）", "有订阅时记一笔")

	msg, err := e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	pushes := e.gotPushes()
	require.Len(t, pushes, 1, msg)
	assert.Equal(t, "11", pushes[0].TorrentID, "最好的那个拉黑了，推下一个")
	assert.Contains(t, runTexts(e.runs(m.ID)), best.Title+"：已拉黑：音画不同步")

	// 删了订阅再建：拉黑还在；对所有条目拉黑的也生效
	require.NoError(t, e.svc.DeleteSubscription(e.ctx, m.ID))
	m2 := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	e.block(BlockInput{Items: []BlockItem{{Site: "hdsky", TorrentID: "11", Title: next.Title}}})
	msg, err = e.svc.SearchNow(e.ctx, m2.ID)
	require.NoError(t, err)
	assert.Equal(t, "找到 2 个，都已拉黑："+best.Title+"：已拉黑：音画不同步；"+next.Title+"：已拉黑", msg)
	assert.Len(t, e.gotPushes(), 1, "都拉黑了不推")

	// 拉黑和别的原因混在一起：说明里写其中几个已拉黑
	e.search.set(best, e.item("hdsky", "13", "Dune.Part.Two.2024.720p.HDTV.x264-X", "", 3, 5))
	p, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "只要 4K", Resolutions: []string{"2160p"}})
	require.NoError(t, err)
	_, err = e.svc.UpdateSubscription(e.ctx, m2.ID, SubscriptionInput{ProfileID: p.ID})
	require.NoError(t, err)
	msg, err = e.svc.SearchNow(e.ctx, m2.ID)
	require.NoError(t, err)
	assert.Contains(t, msg, "找到 2 个，都不符合质量档案")
	assert.Contains(t, msg, "（其中 1 个已拉黑）")
}

// 按条目搜索把拉黑的成员排到组的最后、整组拉黑的组排到最后并标出来，blocked_count 是拉黑的成员数。
func TestSearchMediaMarksBlocked(t *testing.T) {
	e := newEnv(t)
	e.search.set(e.tlouItems()...)
	tv := func(items ...BlockItem) BlockInput {
		return BlockInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Reason: "假种", Items: items}
	}
	e.block(tv(BlockItem{Site: "hdsky", TorrentID: "1", Title: "a"}))
	e.block(BlockInput{Items: []BlockItem{{Site: "hdsky", TorrentID: "4", Title: "整季包"}}})
	// 别的季、别的条目拉黑的不算
	e.block(BlockInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 1, Items: []BlockItem{{Site: "pttime", TorrentID: "3"}}})

	res := e.searchMedia(MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	require.Len(t, res.Groups, 3)
	assert.Equal(t, 2, res.BlockedCount)
	g := res.Groups[0]
	assert.Equal(t, []string{"mteam:2", "ourbits:11", "hdsky:1"}, groupSites(g), "拉黑的成员排到组的最后，首选换成别的站点")
	assert.False(t, g.Blocked)
	require.NotNil(t, g.Members[2].Block)
	assert.Equal(t, "假种", g.Members[2].Block.Reason)
	assert.False(t, g.Members[2].Block.All)
	assert.Nil(t, g.Members[0].Block)
	assert.Equal(t, []string{"pttime:3"}, groupSites(res.Groups[1]), "别的季拉黑的不算")
	last := res.Groups[2]
	assert.Equal(t, []string{"hdsky:4"}, groupSites(last), "整组拉黑的排到最后")
	assert.True(t, last.Blocked)
	require.NotNil(t, last.Members[0].Block)
	assert.True(t, last.Members[0].Block.All)
}

// 追剧详情里「不要这个种子」：拉黑、还在下载的改成失败并马上再搜，记一笔运行记录；可以顺手把制作组加进标题排除词（不重复加）。
func TestBlockSubscriptionTorrent(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	sub := e.follow(nil)
	e.search.set(e.item("mteam", "402", "Natural.High.S04E02.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "", 2, 10))
	_, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	linked := e.linked(sub.ID)
	require.Len(t, linked, 1)
	require.Equal(t, models.MediaSubTorrentDownloading, linked[0].Status)

	_, err = e.svc.BlockSubscriptionTorrent(e.ctx, sub.ID, linked[0].ID+100, BlockTorrentInput{})
	require.ErrorIs(t, err, ErrNotFound)
	_, err = e.svc.BlockSubscriptionTorrent(e.ctx, sub.ID+100, linked[0].ID, BlockTorrentInput{})
	require.ErrorIs(t, err, ErrNotFound)

	e.advance(1)
	res, err := e.svc.BlockSubscriptionTorrent(e.ctx, sub.ID, linked[0].ID, BlockTorrentInput{Reason: "音画不同步", ExcludeGroup: true})
	require.NoError(t, err)
	require.Len(t, res.Blocks, 1)
	b := res.Blocks[0]
	assert.Equal(t, []any{"mteam", "402", models.MediaKindTV, 231620, 4}, []any{b.SiteName, b.TorrentID, b.MediaType, b.TMDBID, b.Season})
	assert.Equal(t, subTitle(sub), b.MediaTitle)
	assert.Equal(t, "HHWEB", res.ExcludedGroup)

	after := e.linked(sub.ID)
	require.Len(t, after, 1)
	assert.Equal(t, models.MediaSubTorrentFailed, after[0].Status)
	assert.Equal(t, "不要这个种子：音画不同步", after[0].Message)
	row := e.subRow(sub.ID)
	assert.Equal(t, "HHWEB", row.ExcludeKeywords)
	require.NotNil(t, row.NextSearchAt)
	assert.Equal(t, e.Now(), *row.NextSearchAt, "马上再搜一次")
	assert.Equal(t, models.MediaEpisodeMissing, e.state(sub, 2), "这一集重新算缺的")
	assert.Contains(t, runTexts(e.runs(sub.ID)),
		"不要这个种子：Natural.High.S04E02.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB（音画不同步）；以后不要制作组 HHWEB（加进了标题排除词）；重新找资源")

	// 拉黑了、标成失败的不给重新整理（会改回下载中）：详情里没有按钮，直接调也拒绝；移出黑名单以后又能整理
	d, err := e.svc.Subscription(e.ctx, sub.ID)
	require.NoError(t, err)
	require.Len(t, d.TorrentList, 1)
	assert.False(t, d.TorrentList[0].CanOrganize)
	assert.True(t, d.TorrentList[0].Blocked)
	_, err = e.svc.OrganizeTorrent(e.ctx, sub.ID, linked[0].ID)
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "拉黑了")
	require.NoError(t, e.svc.RemoveBlock(e.ctx, b.ID))
	d, err = e.svc.Subscription(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.True(t, d.TorrentList[0].CanOrganize, "移出黑名单以后又能整理")
	assert.False(t, d.TorrentList[0].Blocked)

	// 再来一次：制作组已经在排除词里，不重复加；种子已经不在下载，不再改状态
	res, err = e.svc.BlockSubscriptionTorrent(e.ctx, sub.ID, linked[0].ID, BlockTorrentInput{ExcludeGroup: true})
	require.NoError(t, err)
	assert.Empty(t, res.ExcludedGroup)
	assert.Equal(t, "HHWEB", e.subRow(sub.ID).ExcludeKeywords)
	assert.Equal(t, "不要这个种子：音画不同步", e.linked(sub.ID)[0].Message)

	// 下载器里认领的种子不能拉黑
	require.NoError(t, e.db.Model(&models.MediaSubscriptionTorrent{}).Where("id = ?", linked[0].ID).Update("adopted", true).Error)
	_, err = e.svc.BlockSubscriptionTorrent(e.ctx, sub.ID, linked[0].ID, BlockTorrentInput{})
	require.ErrorIs(t, err, ErrInvalid)
}

// 按条目搜索里「不要」时顺手加制作组：有订阅才行，认不出制作组时报错，排除词太长时报错（拉黑照样记下）。
func TestAddBlocksExcludeGroup(t *testing.T) {
	e := newEnv(t)
	sub := e.follow(func(in *SubscriptionInput) {
		kw := "ADWeb"
		in.ExcludeKeywords = &kw
	})
	in := BlockInput{
		MediaType: models.MediaKindTV, TMDBID: 231620, Season: 4, ExcludeGroup: true,
		Items: []BlockItem{{Site: "mteam", TorrentID: "9", Title: "Natural.High.S04E01.2023.1080p.WEB-DL.H.264-PTerWEB"}},
	}
	res := e.block(in)
	assert.Equal(t, "PTerWEB", res.ExcludedGroup)
	assert.Equal(t, "ADWeb\nPTerWEB", e.subRow(sub.ID).ExcludeKeywords)
	assert.Equal(t, subTitle(sub), res.Blocks[0].MediaTitle, "没给条目名时用订阅的名字")
	assert.Contains(t, runTexts(e.runs(sub.ID)), "以后不要制作组 PTerWEB")

	in.Items = []BlockItem{{Site: "mteam", TorrentID: "10", Title: "现在就出发 第四季"}}
	_, err := e.svc.AddBlocks(e.ctx, in)
	require.ErrorIs(t, err, ErrInvalid, "认不出制作组")

	long := strings.Repeat("x", 1020)
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).Update("exclude_keywords", long).Error)
	in.Items = []BlockItem{{Site: "mteam", TorrentID: "11", Title: "Natural.High.S04E03.2023.1080p.WEB-DL.H.264-QAWEB"}}
	_, err = e.svc.AddBlocks(e.ctx, in)
	require.ErrorIs(t, err, ErrInvalid, "排除词太长")
	idx, err := e.svc.blocksFor(e.ctx, models.MediaKindTV, 231620, 4)
	require.NoError(t, err)
	_, ok := idx.get("mteam", "11")
	assert.True(t, ok, "拉黑照样记下")

	// 多个站点上的同一资源一起拉黑：运行记录写几个站点
	in.ExcludeGroup = false
	in.Items = []BlockItem{{Site: "mteam", TorrentID: "20", Title: "Natural.High.S04E05.2160p-HHWEB"}, {Site: "hdsky", TorrentID: "21", Title: "Natural.High.S04E05.2160p-HHWEB"}}
	res = e.block(in)
	assert.Len(t, res.Blocks, 2)
	assert.Contains(t, runTexts(e.runs(sub.ID)), "不要这个资源（2 个站点上的种子）：Natural.High.S04E05.2160p-HHWEB")
}
