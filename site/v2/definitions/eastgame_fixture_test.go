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
		SiteID:   "eastgame",
		Search:   testEastgameSearch,
		Detail:   testEastgameDetail,
		UserInfo: testEastgameUserInfo,
	})
}

// --- Fixtures ---
//
// 结构照搬真实页面（BlueGene 皮肤），但所有标题、副标题、种子 ID、用户名和用户 ID 均已替换为虚构值。
//
// 搜索页需要覆盖的真实特征：
//   - 名称单元格前缀可能带多个 img.sticky、<b>(新)</b>、<b>[热门]</b>
//   - 促销结束时间同时出现在 img.pro_free 的 onmouseover 提示与 [剩余时间：<span title>] 中
//   - 副标题是 <br /> 之后的裸文本节点
//   - 大小列是 "1.95<br />GB"（数字与单位被 <br /> 分开）
//   - 名称子表内还有豆瓣/IMDb 评分单元格与下载单元格（均为 td.embedded）

const eastgameSearchFixture = `<html><body>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%">
<tr>
<td class="colhead" style="padding: 0px">类型</td>
<td class="colhead"><a href="?sort=1&amp;type=asc">标题</a></td>
<td class="colhead"><img class="comments" src="pic/trans.gif" alt="comments" title="评论数" /></td>
<td class="colhead"><img class="time" src="pic/trans.gif" alt="time" title="存活时间" /></td>
<td class="colhead"><img class="size" src="pic/trans.gif" alt="size" title="大小" /></td>
<td class="colhead"><img class="seeders" src="pic/trans.gif" alt="seeders" title="种子数" /></td>
<td class="colhead"><img class="leechers" src="pic/trans.gif" alt="leechers" title="下载数" /></td>
<td class="colhead"><img class="snatched" src="pic/trans.gif" alt="snatched" title="完成数" /></td>
<td class="colhead">发布者</td>
</tr>
<tr class='triple_sticky'>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=438"><img class="c_movie" src="pic/cattrans.gif" alt="电影 (Movie)" title="电影 (Movie)" /></a></td>
<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr class='triple_sticky'><td class="embedded"><img class="sticky" src="pic/trans.gif" alt="Sticky" title="置顶" /><img class="sticky" src="pic/trans.gif" alt="Sticky" title="置顶" /><img class="sticky" src="pic/trans.gif" alt="Sticky" title="置顶" />&nbsp;<a title="Fixture.Movie.2026.BluRay.720p.x264.AC3-FIXTURE"  href="details.php?id=99001&amp;hit=1"><b>Fixture.Movie.2026.BluRay.720p.x264.AC3-FIXTURE</b></a><b> (<font class='new'>新</font>)</b> <b>[<font class='hot'>热门</font>]</b> <img class="pro_free" src="pic/trans.gif" alt="Free"  onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-07-28 03:01:34&quot;&gt;19时45分&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000);" />[剩余时间：<span title="2026-07-28 03:01:34">19时45分</span>]<br />虚构副标题一</td>        <td class="embedded" style="vertical-align: middle; text-align: right; width: 48px;">
            <img src='pic/douban.png' alt='douban' style="vertical-align: middle;"/>
            <span class="rating">5.7</span>
            <br>
            <img src='pic/imdb.png' alt='imdb' style="vertical-align: middle;"/>
            <span class="rating"><span style='color: silver'>N/A</span></span>
        </td>
        <td width="20" class="embedded" style="text-align: right; " valign="middle"><a href="download.php?id=99001"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a><br /><a id="bookmark0"  href="javascript: bookmark(99001,0);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=99001&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-07-26 03:01:33">1天<br />4时</span></td><td class="rowfollow">1.95<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99001&amp;hit=1&amp;dllist=1#seeders">33</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=99001&amp;hit=1&amp;dllist=1#leechers">2</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=99001"><b>112</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=449"><img class="c_study" src="pic/cattrans.gif" alt="资料（E-Learning）" title="资料（E-Learning）" /></a></td>
<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr><td class="embedded"><a title="Fixture.Guide.v2.0-FIXTURE"  href="details.php?id=99002&amp;hit=1"><b>Fixture.Guide.v2.0-FIXTURE</b></a><br />虚构副标题二</td>        <td class="embedded" style="vertical-align: middle; text-align: right; width: 48px;">
            <img src='pic/douban.png' alt='douban' style="vertical-align: middle;"/>
            <span class="rating"><span style='color: silver'>N/A</span></span>
        </td>
        <td width="20" class="embedded" style="text-align: right; " valign="middle"><a href="download.php?id=99002"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=99002&amp;type=torrent" title="添加评论">3</a></td><td class="rowfollow nowrap"><span title="2026-05-01 12:00:00">2月<br />26天</span></td><td class="rowfollow">816.30<br />MB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99002&amp;hit=1&amp;dllist=1#seeders">206</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=99002&amp;hit=1&amp;dllist=1#leechers">1</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=99002"><b>0</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
</table>
</body></html>`

// 详情页：标题/ID 来自 <form> 内的隐藏域（本站保留），h1 中的促销标记同时带 onmouseover 与真实
// [剩余时间：<span title>]；「基本信息」行使用全角冒号与十进制单位。
const eastgameDetailFixture = `<html><body>
<h1 align="center" id="top">Fixture.Movie.2026.BluRay.720p.x264.AC3-FIXTURE&nbsp;&nbsp;&nbsp; <b>[<font class='free'  onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-07-28 03:01:34&quot;&gt;19时45分&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500);">免费</font>]</b>[剩余时间：<span title="2026-07-28 03:01:34">19时45分</span>]</h1>
<table width="1040" cellspacing="0" cellpadding="5">
<tr><td class="rowhead" width="13%">下载</td><td class="rowfollow" width="87%" align="left"><a class="index" href="download.php?id=99001">Fixture.Movie.2026.BluRay.720p.x264.AC3-FIXTURE.mkv.torrent</a>&nbsp;&nbsp;&nbsp;由&nbsp;<i>匿名</i>发布于<span title="2026-07-26 03:01:33">1天4时前</span></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">虚构副标题一</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>1.95 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;电影 (Movie)&nbsp;&nbsp;&nbsp;<b>媒介:&nbsp;</b>Encode&nbsp;&nbsp;&nbsp;<b>分辨率:&nbsp;</b>720p</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">行为</td><td class="rowfollow" valign="top" align="left"><a title="下载种子" href="download.php?id=99001"><img class="dt_download" src="pic/trans.gif" alt="download" />&nbsp;<b><font class="small">下载种子</font></b></a></td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><table border="0" cellspacing="0"><tr><td class="embedded"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.Movie.2026.BluRay.720p.x264.AC3-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="99001" /></form></td></tr></table></td></tr>
</table>
</body></html>`

const eastgameDetailWithHRFixture = `<html><body>
<h1 align="center" id="top">Fixture.HR.Show.S01.WEB-DL.1080p-FIXTURE</h1>
<table width="1040" cellspacing="0" cellpadding="5">
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>8.50 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;电视剧 (TV Series)</td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.HR.Show.S01.WEB-DL.1080p-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="99003" /></form></td></tr>
</table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

// index.php 顶部 #info_block：id / name / 魔力值 / 做种下载数均取自这里。
// 注意 class 属性带空格（class = 'color_bonus'），与真实页面一致。
const eastgameIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr>
	<td><table width="100%" cellspacing="0" cellpadding="0" border="0"><tr>
		<td class="bottom" align="left"><span class="medium">欢迎回来, <span class="nowrap"><a  href="userdetails.php?id=90001" class='User_Name'><b>fixture_user</b></a></span>  [<a href="logout.php">退出</a>] <font class = 'color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 101,003.0 <font class = 'color_invite'>邀请 </font>[<a href="invite.php?id=90001">发送</a>]: 0<br />
	<font class="color_ratio">分享率：</font> 无限  <font class='color_uploaded'>上传量：</font> 229.18 GB<font class='color_downloaded'> 下载量：</font> 0.00 KB  <font class='color_active'>当前活动：</font> <img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />99  <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />0&nbsp;&nbsp;<font class='color_connectable'>可连接：</font><b><font color="green">是</font></b></span></td>
	</tr></table></td>
</tr></table>
</body></html>`

// userdetails.php 正文表格：注意「传输」行把上传量写成
// <strong>总上传量/奖励上传量/纯上传量</strong>，而「最近动向」之后紧跟同样命中
// :contains('最近动向') 的「最近动向(地点)」空行。
const eastgameUserdetailsFixture = `<html><body>
<h1 style='margin:0px'><span class="nowrap"><b>fixture_target</b></span></h1>
<table width="100%" border="1" cellspacing="0" cellpadding="5">
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">邀请</td><td width="99%" class="rowfollow" valign="top" align="left">没有邀请资格</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">加入日期</td><td width="99%" class="rowfollow" valign="top" align="left">2025-01-17 05:22:29 (<span title="2025-01-17 05:22:29">1年6月前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-07-25 01:57:00 (<span title="2026-07-25 01:57:00">2天5时前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向(地点)</td><td width="99%" class="rowfollow" valign="top" align="left"></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">BT客户端</td><td width="99%" class="rowfollow" valign="top" align="left">qBittorrent/5.0.4</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">传输</td><td width="99%" class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>:  <font color="">38.040</font></td><td class="embedded">&nbsp;&nbsp;<img src="pic/smilies/163.gif" alt="" /></td></tr><tr><td class="embedded"><strong>总上传量/奖励上传量/纯上传量</strong>:  5.405 TB/120.00 GB/5.288 TB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>:  145.50 GB</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">BT时间</td><td width="99%" class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>做种时间</strong>:  870天02:47:36</td><td class="embedded">&nbsp;&nbsp;<strong>下载时间</strong>:  383天17:58:19</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">等级</td><td width="99%" class="rowfollow" valign="top" align="left"><img alt="Elite User" title="Elite User" src="pic/elite.gif" /> </td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">种子评论</td><td width="99%" class="rowfollow" valign="top" align="left">6</td></tr>
</table>
</body></html>`

// --- Helpers ---

func getEastgameDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("eastgame")
	require.True(t, ok, "eastgame definition not found")
	return def
}

// --- Suite: Search ---

func testEastgameSearch(t *testing.T) {
	def := getEastgameDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(eastgameSearchFixture))
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
	require.Len(t, items, 2, "should parse 2 torrent rows")

	free := items[0]
	assert.Equal(t, "99001", free.ID)
	assert.Equal(t, "Fixture.Movie.2026.BluRay.720p.x264.AC3-FIXTURE", free.Title)
	// 副标题是 <br /> 之后的裸文本，需要 SubtitleSelector 才能取到
	assert.Equal(t, "虚构副标题一", free.Subtitle)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	// 大小列为 "1.95<br />GB"，parseSize 去空白后按十进制单位换算
	assert.Equal(t, int64(2093796556), free.SizeBytes)
	assert.Equal(t, 33, free.Seeders)
	assert.Equal(t, 2, free.Leechers)
	assert.Equal(t, 112, free.Snatched)
	assert.Equal(t, "电影 (Movie)", free.Category)
	// 促销结束时间取名称单元格内 [剩余时间：<span title>]，不能误取第 4 列的发布时间
	require.False(t, free.DiscountEndTime.IsZero(), "discount end time should be parsed")
	assert.Equal(t, 2026, free.DiscountEndTime.Year())
	assert.Equal(t, 7, int(free.DiscountEndTime.Month()))
	assert.Equal(t, 28, free.DiscountEndTime.Day())
	assert.Positive(t, free.UploadedAt)

	normal := items[1]
	assert.Equal(t, "99002", normal.ID)
	assert.Equal(t, "Fixture.Guide.v2.0-FIXTURE", normal.Title)
	assert.Equal(t, "虚构副标题二", normal.Subtitle)
	assert.Equal(t, v2.DiscountNone, normal.DiscountLevel)
	assert.True(t, normal.DiscountEndTime.IsZero(), "no discount -> no end time")
	assert.Equal(t, int64(855952588), normal.SizeBytes)
	assert.Equal(t, 206, normal.Seeders)
	assert.Equal(t, 1, normal.Leechers)
	assert.Equal(t, 0, normal.Snatched)
}

// --- Suite: Detail ---

func testEastgameDetail(t *testing.T) {
	def := getEastgameDef(t)

	t.Run("Free", func(t *testing.T) {
		doc := FixtureDoc(t, "eastgame_detail", eastgameDetailFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "99001", info.TorrentID)
		assert.Equal(t, "Fixture.Movie.2026.BluRay.720p.x264.AC3-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		require.False(t, info.DiscountEnd.IsZero(), "discount end time should be parsed")
		assert.Equal(t, 2026, info.DiscountEnd.Year())
		assert.Equal(t, 7, int(info.DiscountEnd.Month()))
		assert.Equal(t, 28, info.DiscountEnd.Day())
		// 「基本信息」标签单元格 + .Next() 兄弟节点，全角冒号 + 十进制 GB
		assert.InDelta(t, 1.95*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})

	t.Run("WithHR", func(t *testing.T) {
		doc := FixtureDoc(t, "eastgame_detail_hr", eastgameDetailWithHRFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "99003", info.TorrentID)
		assert.Equal(t, "Fixture.HR.Show.S01.WEB-DL.1080p-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel)
		assert.InDelta(t, 8.5*1024, info.SizeMB, 0.1)
		assert.True(t, info.HasHR, "should detect HR from hit_run.gif")
	})
}

// --- Suite: UserInfo ---

func testEastgameUserInfo(t *testing.T) {
	def := getEastgameDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "eastgame_index", eastgameIndexFixture)
		fields := map[string]string{
			"id":       "90001",
			"name":     "fixture_user",
			"seeding":  "99",
			"leeching": "0",
			"bonus":    "101003",
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
		doc := FixtureDoc(t, "eastgame_userdetails", eastgameUserdetailsFixture)
		fields := map[string]string{
			// 5.405 TB / 5.288 TB / 145.50 GB 按 1024 进制换算为字节
			"uploaded":     "5942860348129",
			"trueUploaded": "5814217487679",
			"downloaded":   "156229435392",
			"ratio":        "38.04",
			"levelName":    "Elite User",
			// parseTime 按站点 TimezoneOffset(+0800) 解析
			"joinTime":     "1737062549",
			"lastAccessAt": "1784915820",
		}
		for field, expected := range fields {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q not found", field)
				assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel))
			})
		}

		// 保号探测只读取 UserInfo.LastAccess，必须为正的 Unix 秒
		sel := def.UserInfo.Selectors["lastAccessAt"]
		got := driver.ExtractFieldValuePublic(doc, sel)
		require.NotEmpty(t, got, "lastAccessAt must be parsed for the login probe")
		assert.Regexp(t, `^\d+$`, got)
		assert.Greater(t, got, "0")
	})
}

// --- Standalone Tests ---

func TestEastgame_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      eastgameSearchFixture,
		"detail":      eastgameDetailFixture,
		"detail_hr":   eastgameDetailWithHRFixture,
		"index":       eastgameIndexFixture,
		"userdetails": eastgameUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
