package definitions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func init() {
	RegisterFixtureSuite(FixtureSuite{
		SiteID:   "u2dmhy",
		Search:   testU2DMHYSearch,
		Detail:   testU2DMHYDetail,
		UserInfo: testU2DMHYUserInfo,
	})
}

// --- Fixtures ---
//
// 结构照搬真实页面，但标题、副标题、种子 ID、用户名与用户 ID 均已替换为虚构值。
//
// 搜索页需要覆盖的真实特征：
//   - 标题链接与副标题都带 class="tooltip"（前者是 <a>，后者是 <span>）
//   - 促销剩余时间是 [剩余 <time title="...">] 而非 <span title="...">
//   - pro_free2up / pro_2up / pro_50pctdown 三种促销图标共存，且 pro_free 是 pro_free2up 的前缀
//   - 大小列写作 "9.328<br />GiB"（数字与二进制单位被 <br /> 分开）
//   - 生存时间列同样用 <time title="...">，内含 &shy; 软连字符
//   - 分类单元格是纯文本链接，没有 img[alt]

const u2dmhySearchFixture = `<html><body>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%">
<tr><td class="colhead" style="padding: 0">分类</td><td class="colhead"><a href="?sort=1&amp;type=asc">名称</a></td><td class="colhead"><img class="comments" src="pic/trans.gif" alt="comments" title="评论数" /></td><td class="colhead"><img class="time" src="pic/trans.gif" alt="time" title="生存时间" /></td><td class="colhead"><img class="size" src="pic/trans.gif" alt="Size" title="大小" /></td><td class="colhead"><img class="seeders" src="pic/trans.gif" alt="Seeders" title="种子数" /></td><td class="colhead"><img class="leechers" src="pic/trans.gif" alt="Leechers" title="下载数" /></td><td class="colhead"><img class="snatched" src="pic/trans.gif" alt="Snatched" title="完成数" /></td></tr>
<tr>
<td class="rowfollow nowrap" valign="middle"><a style='' href="?cat=16">BDMV</a></td><td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%">
<tr><td class="embedded overflow-control"><img class="sticky" src="pic/trans.gif" alt="Sticky" title="置顶" />&nbsp;<a class="tooltip" href="details.php?id=770001&amp;hit=1">[示例动画剧场版][Sample Anime Movie][サンプル劇場版][BDMV][1080p][MOVIE][AVC FLAC][JPN]</a></td><td width="40" class="embedded" style="text-align: right;" valign="middle"><a href="download.php?id=770001"><img class="download" src="pic/trans.gif" style="padding-bottom: 2px;" alt="download" title="下载种子" /></a><a id="bookmark0" href="javascript: bookmark(770001,0);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td></tr><tr><td class="embedded overflow-control"><b>[<span class="new">新！</span>]</b>&nbsp;<b>[<span class="hot">热门</span>]</b>&nbsp; <img class="pro_free2up" src="pic/trans.gif" alt="2X Free" />[<b>剩余 <time title="2026-07-28 11:46:31">20&shy;小时&shy;34&shy;分钟</time></b>]&nbsp;<span class="tooltip">虚构副标题一 | 自购自抓</span></td><td class="embedded" style="text-align: right;" valign="middle"><a target="_blank" href="http://anidb.net/a90001"><span style="padding-top: 2px;font-size:7pt;color:#3C4954;">8.85</span></a></td></tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=770001&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><time title="2026-07-27 10:58:20">4&shy;小时<br />14&shy;分钟</time></td><td class="rowfollow">46.717<br />GiB</td><td class="rowfollow " align="center"><b><a href="details.php?id=770001&amp;hit=1&amp;dllist=1#seeders">74</a></b></td><td class="rowfollow ">7</td><td class="rowfollow "><a href="viewsnatches.php?id=770001"><b>142</b></a></td></tr>
<tr>
<td class="rowfollow nowrap" valign="middle"><a style='' href="?cat=40">Other</a></td><td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%">
<tr><td class="embedded overflow-control"><a class="tooltip" href="details.php?id=770002&amp;hit=1">[示例特别篇][Sample TV Special][サンプルSP][HDTVraw][1440x1080i][MPEG2-TS AAC][Sample-Team]</a></td><td width="40" class="embedded" style="text-align: right;" valign="middle"><a href="download.php?id=770002"><img class="download" src="pic/trans.gif" style="padding-bottom: 2px;" alt="download" title="下载种子" /></a></td></tr><tr><td class="embedded overflow-control"><b>[<span class="hot">热门</span>]</b>&nbsp; <img class="pro_2up" src="pic/trans.gif" alt="2X" />&nbsp;<span class="tooltip">虚构副标题二｜禁转</span></td><td class="embedded" style="text-align: right;" valign="middle"></td></tr></table></td><td class="rowfollow"><b><a href="details.php?id=770002&amp;hit=1&amp;cmtpage=1#startcomments">4</a></b></td><td class="rowfollow nowrap"><time title="2019-12-01 00:37:29">6&shy;年<br />7&shy;月</time></td><td class="rowfollow">9.328<br />GiB</td><td class="rowfollow " align="center"><b><a href="details.php?id=770002&amp;hit=1&amp;dllist=1#seeders">28</a></b></td><td class="rowfollow ">0</td><td class="rowfollow "><a href="viewsnatches.php?id=770002"><b>220</b></a></td></tr>
<tr>
<td class="rowfollow nowrap" valign="middle"><a style='' href="?cat=12">BDRip</a></td><td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%">
<tr><td class="embedded overflow-control"><a class="tooltip" href="details.php?id=770003&amp;hit=1">[示例电视动画][Sample TV Anime][サンプルTV][BDRip][1920x1080][TV 01-12 Fin][H264 FLAC MKV]</a></td><td width="40" class="embedded" style="text-align: right;" valign="middle"><a href="download.php?id=770003"><img class="download" src="pic/trans.gif" style="padding-bottom: 2px;" alt="download" title="下载种子" /></a></td></tr><tr><td class="embedded overflow-control"><b>[<span class="hot">热门</span>]</b>&nbsp; <img class="pro_50pctdown" src="pic/trans.gif" alt="50%" />&nbsp;<span class="tooltip"></span></td><td class="embedded" style="text-align: right;" valign="middle"></td></tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=770003&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><time title="2026-07-20 08:15:00">7&shy;天<br />6&shy;小时</time></td><td class="rowfollow">816.30<br />MiB</td><td class="rowfollow " align="center"><b><a href="details.php?id=770003&amp;hit=1&amp;dllist=1#seeders">74</a></b></td><td class="rowfollow ">10</td><td class="rowfollow "><a href="viewsnatches.php?id=770003"><b>189</b></a></td></tr>
<tr>
<td class="rowfollow nowrap" valign="middle"><a style='' href="?cat=30">Lossless<br />Music</a></td><td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%">
<tr><td class="embedded overflow-control"><a class="tooltip" href="details.php?id=770004&amp;hit=1">[示例原声集][Sample Original Soundtrack][FLAC+CUE][WEB]</a></td><td width="40" class="embedded" style="text-align: right;" valign="middle"><a href="download.php?id=770004"><img class="download" src="pic/trans.gif" style="padding-bottom: 2px;" alt="download" title="下载种子" /></a></td></tr><tr><td class="embedded overflow-control"><span class="tooltip">虚构副标题四</span></td><td class="embedded" style="text-align: right;" valign="middle"></td></tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=770004&amp;type=torrent" title="添加评论">1</a></td><td class="rowfollow nowrap"><time title="2026-06-30 22:00:00">27&shy;天</time></td><td class="rowfollow">1.971<br />GiB</td><td class="rowfollow " align="center"><b><a href="details.php?id=770004&amp;hit=1&amp;dllist=1#seeders">5</a></b></td><td class="rowfollow ">1</td><td class="rowfollow "><a href="viewsnatches.php?id=770004"><b>33</b></a></td></tr>
</table>
</body></html>`

// 详情页：本站没有 input[name='torrent_name']，标题只在 h1#top 文本里（走共享解析器的文本兜底），
// 种子 ID 仍来自字幕上传表单的 detail_torrent_id；
// 「基本信息」行首字段是「发布时间:」、冒号半角、体积为二进制单位；
// 促销状态由「流量优惠」行的 img.pro_* 表示，而非 <font class="free">。
const u2dmhyDetailFixture = `<html><body>
<h1 align="center" id="top">[示例特别篇][Sample TV Special][サンプルSP][HDTVraw][1440x1080i][MPEG2-TS AAC][Sample-Team]</h1><h3>(#770002)</h3>
<table width="90%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead" width="13%">下载</td><td class="rowfollow" width="87%" align="left"><a class="index" href="download.php?id=770002">[U2].Sample.TV.Special.ts.torrent</a>&nbsp;<a class="index" href="download.php?id=770002&amp;zip=1">[ZIP]</a></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">虚构副标题二｜禁转</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b>发布时间:</b>&nbsp;<time title="2019-12-01 00:37:29">6&shy;年&shy;7&shy;月&shy;前</time>&nbsp;&nbsp;&nbsp;<b>大小:</b>&nbsp;9.328 GiB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;Other</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">流量优惠</td><td class="rowfollow" valign="top" align="left"> <img class="pro_2up" src="pic/trans.gif" alt="2X" />&nbsp;&nbsp;&nbsp;<a href="promotion.php?action=torrent&amp;id=770002" class="faqlink">优惠历史</a></td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><form method="post" action="subtitles.php"><input type="hidden" name="detail_torrent_id" value="770002" /><button>上传字幕</button></form></td></tr>
</table>
</body></html>`

// 限时 2X 免费的详情页：h1#top 尾部带 [免费] 促销标记（应被剥离），
// 「流量优惠」行渲染出 [剩余 <time title>]（结束时间应被解析）。
const u2dmhyDetailPromoFixture = `<html><body>
<h1 align="center" id="top">[示例动画剧场版][Sample Anime Movie][BDMV][1080p][JPN] [免费]</h1><h3>(#770001)</h3>
<table width="90%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b>发布时间:</b>&nbsp;<time title="2026-07-27 10:58:20">4&shy;小时&shy;前</time>&nbsp;&nbsp;&nbsp;<b>大小:</b>&nbsp;46.717 GiB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;BDMV</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">流量优惠</td><td class="rowfollow" valign="top" align="left"> <img class="pro_free2up" src="pic/trans.gif" alt="2X Free" />[<b>剩余 <time title="2026-07-28 11:46:31">20&shy;小时&shy;34&shy;分钟</time></b>]</td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><form method="post" action="subtitles.php"><input type="hidden" name="detail_torrent_id" value="770001" /></form></td></tr>
</table>
</body></html>`

// 无促销的详情页：标题尾部是合法的规格方括号，必须保留而不能被促销剥离逻辑吃掉。
const u2dmhyDetailPlainFixture = `<html><body>
<h1 align="center" id="top">[示例电视动画 第一季 / Sample Show S01]中日双语字幕  [1080p BluRay]</h1><h3>(#770003)</h3>
<table width="90%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b>发布时间:</b>&nbsp;<time title="2026-07-20 08:15:00">7&shy;天&shy;前</time>&nbsp;&nbsp;&nbsp;<b>大小:</b>&nbsp;816.30 MiB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;BDRip</td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><form method="post" action="subtitles.php"><input type="hidden" name="detail_torrent_id" value="770003" /></form></td></tr>
</table>
</body></html>`

// index.php 顶部 #info_block：用户名链接的 class 是「等级名_Name」；
// 做种数在 img.arrowup 之后的 <a ...ullist=1#seedlist>，下载数是 img.arrowdown 之后的裸文本节点。
const u2dmhyIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td><table width="100%" cellspacing="0" cellpadding="0" border="0"><tr><td class="bottom" align="left"><span class="medium">欢迎回来, <span class="nowrap"><a  href='userdetails.php?id=90042' style="color:#;" class="NexusMaster_Name"><b><bdo dir='ltr'>fixture_user</bdo></b></a></span> [<a href="logout.php">退出</a>] [<a href="promotion.php?action=my">魔法</a>]<br /><span class="color_ratio">分享率:</span> 4.639 <span class="color_uploaded">上传量:</span> 13.923 TiB <span class="color_downloaded">下载量:</span> 3.001 TiB <a href="ucoin.php" class="faqlink">UCoin</a>: <span class="ucoin-notation ucoin-collapsed" title="0.0010 逸"><span class="ucoin-symbol ucoin-gem">9</span><span class="ucoin-symbol ucoin-gold">69</span></span> <a href="invite.php?id=90042" class="faqlink">邀请</a>: 0 <span class="color_active">客户端</span>: <a href="userdetails.php?id=90042&amp;clientlist=1#btclients">3</a> (<img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" /><a href="userdetails.php?id=90042&amp;ullist=1#seedlist">16</a> <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />0)</span></td></tr></table></td></tr></table>
</body></html>`

// userdetails.php 正文表格：标签为简体（加入日期 / 最近动向 / 传输 / 等级）；
// 「传输」行是内嵌 table，标签写作 <strong>上传量</strong> + 半角冒号 + 两个空格，
// 实际上传/下载没有「量」字；「原积分」行只有历史链接、没有数值，魔力值只能取自 UCoin 行。
const u2dmhyUserdetailsFixture = `<html><body>
<h1 align="center">fixture_target 的详细资料</h1>
<table width="100%" border="1" cellspacing="0" cellpadding="5">
<tr><td width = '1%' class="rowhead nowrap" valign="top" align="right">加入日期</td><td width = '99%' class="rowfollow" valign="top" align="left"><time>2018-01-24 20:00:03</time> (<time title="2018-01-24 20:00:03">8&shy;年&shy;6&shy;月&shy;前</time>/443 周 4 天 前)</td></tr>
<tr><td width = '1%' class="rowhead nowrap" valign="top" align="right">最近动向</td><td width = '99%' class="rowfollow" valign="top" align="left"><time>2026-07-27 15:12:31</time> (<time title="2026-07-27 15:12:31">现在</time>)</td></tr>
<tr><td width = '1%' class="rowhead nowrap" valign="top" align="right">传输<br />[<a class="faqlink" href="traffichistory.php?id=90042">历史</a>]</td><td width = '99%' class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>:  <font color="">4.639</font></td><td class="embedded">&nbsp;&nbsp;<img src="pic/smilies/5.gif" alt="" /></td></tr><tr><td class="embedded"><strong>上传量</strong>:  13.923 TiB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>:  3.001 TiB</td></tr><tr><td class="embedded"><strong>实际上传</strong>: 7.375 TiB</td><td class="embedded">&nbsp;&nbsp;<strong>实际下载</strong>: 18.151 TiB</td></tr></table></td></tr>
<tr><td width = '1%' class="rowhead nowrap" valign="top" align="right">BT时间</td><td width = '99%' class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>做种时间</strong>:  38852&shy;天&nbsp;17:48:04</td><td class="embedded">&nbsp;&nbsp;<strong>下载时间</strong>:  101&shy;天&nbsp;01:55:29</td></tr></table></td></tr>
<tr><td width = '1%' class="rowhead nowrap" valign="top" align="right">等级</td><td width = '99%' class="rowfollow" valign="top" align="left"><img alt="宅神" title="宅神" src="pic/nexus.gif" /></td></tr>
<tr><td width = '1%' class="rowhead nowrap" valign="top" align="right">经验值</td><td width = '99%' class="rowfollow" valign="top" align="left"><b>魔法使 LV1</b> EXP: 16/100<br /></td></tr>
<tr><td width = '1%' class="rowhead nowrap" valign="top" align="right">原积分</td><td width = '99%' class="rowfollow" valign="top" align="left"><a class="faqlink" href="bonushistory.php?id=90042">历史</a></td></tr>
<tr><td width = '1%' class="rowhead nowrap" valign="top" align="right">UCoin<br />[<a href="ucoin.php?id=90042" class="faqlink">详情</a>]</td><td width = '99%' class="rowfollow" valign="top" align="left"><span class="ucoin-notation" title="9,692,300.77"><span class="ucoin-symbol ucoin-gem">9</span><span class="ucoin-symbol ucoin-gold">69</span><span class="ucoin-symbol ucoin-silver">23</span></span><br />(9,692,300.771)</td></tr>
<tr><td width = '1%' class="rowhead nowrap" valign="top" align="right">种子评论</td><td width = '99%' class="rowfollow" valign="top" align="left">0</td></tr>
</table>
</body></html>`

// --- Helpers ---

func getU2DMHYDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("u2dmhy")
	require.True(t, ok, "u2dmhy definition not found")
	return def
}

// --- Suite: Search ---

func testU2DMHYSearch(t *testing.T) {
	def := getU2DMHYDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(u2dmhySearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{
		BaseURL:   server.URL,
		Cookie:    "test_cookie=1",
		Selectors: def.Selectors,
	})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 4, "should parse 4 torrent rows")

	twoXFree := items[0]
	assert.Equal(t, "770001", twoXFree.ID)
	assert.Equal(t, "[示例动画剧场版][Sample Anime Movie][サンプル劇場版][BDMV][1080p][MOVIE][AVC FLAC][JPN]", twoXFree.Title)
	assert.Equal(t, "虚构副标题一 | 自购自抓", twoXFree.Subtitle)
	// pro_free 是 pro_free2up 的前缀，必须判成 2X Free 而不是 Free
	assert.Equal(t, v2.Discount2xFree, twoXFree.DiscountLevel)
	// 促销结束时间取名称子表内的 [剩余 <time title>]，不能误取第 4 列的生存时间
	require.False(t, twoXFree.DiscountEndTime.IsZero(), "discount end time should come from [剩余 <time title>]")
	assert.Equal(t, 2026, twoXFree.DiscountEndTime.Year())
	assert.Equal(t, 7, int(twoXFree.DiscountEndTime.Month()))
	assert.Equal(t, 28, twoXFree.DiscountEndTime.Day())
	// 46.717 GiB，大小列的数字与单位被 <br /> 分开
	assert.Equal(t, int64(50161996791), twoXFree.SizeBytes)
	assert.Equal(t, 74, twoXFree.Seeders)
	assert.Equal(t, 7, twoXFree.Leechers)
	assert.Equal(t, 142, twoXFree.Snatched)
	assert.Positive(t, twoXFree.UploadedAt)

	twoXUp := items[1]
	assert.Equal(t, "770002", twoXUp.ID)
	assert.Equal(t, "虚构副标题二｜禁转", twoXUp.Subtitle)
	assert.Equal(t, v2.Discount2xUp, twoXUp.DiscountLevel)
	assert.True(t, twoXUp.DiscountEndTime.IsZero(), "permanent 2X has no [剩余] marker")
	assert.Equal(t, int64(10015863734), twoXUp.SizeBytes)
	assert.Equal(t, 28, twoXUp.Seeders)
	assert.Equal(t, 0, twoXUp.Leechers)
	assert.Equal(t, 220, twoXUp.Snatched)

	half := items[2]
	assert.Equal(t, "770003", half.ID)
	assert.Equal(t, v2.DiscountPercent50, half.DiscountLevel)
	assert.Empty(t, half.Subtitle, "empty <span class=tooltip> yields empty subtitle")
	assert.Equal(t, int64(855952588), half.SizeBytes)
	assert.Equal(t, 74, half.Seeders)
	assert.Equal(t, 10, half.Leechers)
	assert.Equal(t, 189, half.Snatched)

	plain := items[3]
	assert.Equal(t, "770004", plain.ID)
	assert.Equal(t, v2.DiscountNone, plain.DiscountLevel)
	assert.Equal(t, "虚构副标题四", plain.Subtitle)
	assert.Equal(t, int64(2116345135), plain.SizeBytes)
	assert.Equal(t, 5, plain.Seeders)
	assert.Equal(t, 1, plain.Leechers)
	assert.Equal(t, 33, plain.Snatched)
}

// --- Suite: Detail ---

func testU2DMHYDetail(t *testing.T) {
	def := getU2DMHYDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	t.Run("TitleFromTextFallback", func(t *testing.T) {
		doc := FixtureDoc(t, "u2dmhy_detail", u2dmhyDetailFixture)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "770002", info.TorrentID)
		// 无 input[name='torrent_name']，标题只能来自 h1#top 文本
		require.NotEmpty(t, info.Title, "title must come from the h1#top text fallback")
		assert.Equal(t, "[示例特别篇][Sample TV Special][サンプルSP][HDTVraw][1440x1080i][MPEG2-TS AAC][Sample-Team]", info.Title)
		assert.Equal(t, v2.Discount2xUp, info.DiscountLevel)
		assert.True(t, info.DiscountEnd.IsZero())
		// 「基本信息」行首字段是发布时间，须跳过它找到半角冒号的「大小:」与二进制单位
		assert.InDelta(t, 9.328*1024, info.SizeMB, 0.001)
		assert.False(t, info.HasHR, "site has no H&R marker")
	})

	t.Run("PromoSuffixStripped", func(t *testing.T) {
		doc := FixtureDoc(t, "u2dmhy_detail_promo", u2dmhyDetailPromoFixture)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "770001", info.TorrentID)
		// 尾部 [免费] 是促销标记，由共享解析器剥离
		assert.Equal(t, "[示例动画剧场版][Sample Anime Movie][BDMV][1080p][JPN]", info.Title)
		assert.Equal(t, v2.Discount2xFree, info.DiscountLevel)
		require.False(t, info.DiscountEnd.IsZero(), "促销剩余时间应从「流量优惠」行的 time[title] 解析")
		assert.Equal(t, 2026, info.DiscountEnd.Year())
		assert.Equal(t, 7, int(info.DiscountEnd.Month()))
		assert.Equal(t, 28, info.DiscountEnd.Day())
		assert.InDelta(t, 46.717*1024, info.SizeMB, 0.001)
	})

	t.Run("PlainTitleKeepsBracket", func(t *testing.T) {
		doc := FixtureDoc(t, "u2dmhy_detail_plain", u2dmhyDetailPlainFixture)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "770003", info.TorrentID)
		// 尾部方括号是规格信息而非促销标记，不能被剥离
		assert.Equal(t, "[示例电视动画 第一季 / Sample Show S01]中日双语字幕  [1080p BluRay]", info.Title)
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel)
		assert.InDelta(t, 816.30, info.SizeMB, 0.001)
		assert.False(t, info.HasHR)
	})
}

// --- Suite: UserInfo ---

func testU2DMHYUserInfo(t *testing.T) {
	def := getU2DMHYDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "u2dmhy_index", u2dmhyIndexFixture)
		fields := map[string]string{
			"id":   "90042",
			"name": "fixture_user",
			// 做种数在 <a ...ullist=1>，下载数是 img.arrowdown 之后的裸文本
			"seeding":  "16",
			"leeching": "0",
		}
		for field, expected := range fields {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q not found", field)
				assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel))
			})
		}
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "u2dmhy_userdetails", u2dmhyUserdetailsFixture)
		fields := map[string]string{
			// 13.923 TiB / 3.001 TiB / 7.375 TiB / 18.151 TiB 按二进制单位换算为字节
			"uploaded":       "15308500393525",
			"downloaded":     "3299634394955",
			"trueUploaded":   "8108898254848",
			"trueDownloaded": "19957235555762",
			"ratio":          "4.639",
			"levelName":      "宅神",
			// 站点货币是 UCoin，parseNumber 的大数结果以科学计数法字符串返回
			"bonus": "9.692300771e+06",
			// parseTime 按站点 TimezoneOffset(+0800) 解析
			"joinTime":     "1516795203",
			"lastAccessAt": "1785136351",
		}
		for field, expected := range fields {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q not found", field)
				assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel))
			})
		}

		// 保号探测只读取 UserInfo.LastAccess，必须为正的 Unix 秒
		got := driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["lastAccessAt"])
		require.NotEmpty(t, got, "lastAccessAt must be parsed for the login probe")
		assert.Regexp(t, `^\d+$`, got)
		assert.Greater(t, got, "0")
	})
}

// --- Standalone Tests ---

func TestU2dmhy_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":       u2dmhySearchFixture,
		"detail":       u2dmhyDetailFixture,
		"detail_promo": u2dmhyDetailPromoFixture,
		"detail_plain": u2dmhyDetailPlainFixture,
		"index":        u2dmhyIndexFixture,
		"userdetails":  u2dmhyUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
