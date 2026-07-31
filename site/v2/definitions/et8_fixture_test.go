package definitions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func init() {
	RegisterFixtureSuite(FixtureSuite{
		SiteID:   "et8",
		Search:   testEt8Search,
		Detail:   testEt8Detail,
		UserInfo: testEt8UserInfo,
	})
}

// --- Fixtures ---
//
// 结构取自 et8.org 真实页面（浏览器扩展采集），所有用户名 / ID / 标题均已改为虚构值。
// 列表页共 10 列，第 9 列为 et8 特有的「进度」列（真实页面中该单元格无 rowfollow class）。

// 真实页面上只有限时优惠（2X 免费）才带 onmouseover tooltip 与可见「剩余时间」；
// pro_free / pro_50pctdown / pro_30pctdown / pro_2up 均为长期优惠，无结束时间。
const et8SearchFixture = `<html><body>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%" id="torrenttable">
<tr>
<td class="colhead" style="padding: 0px">类型</td>
<td class="colhead"><a href="?sort=1&amp;type=asc">标题</a></td>
<td class="colhead"><img class="comments" alt="comments" title="评论数" /></td>
<td class="colhead"><img class="time" alt="time" title="存活时间" /></td>
<td class="colhead"><img class="size" alt="size" title="大小" /></td>
<td class="colhead"><img class="seeders" alt="seeders" title="种子数" /></td>
<td class="colhead"><img class="leechers" alt="leechers" title="下载数" /></td>
<td class="colhead"><img class="snatched" alt="snatched" title="完成数" /></td>
<td class="colhead">进度</td>
<td class="colhead"><a href="?sort=9&amp;type=desc">发布者</a></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=622" title="Movies.电影"><img class="movies" src="pic/cattrans.gif" alt="Movies.电影" /></a></td>
<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr><td class="embedded"><font color=red>[置顶++]</font>&nbsp;<a title="[TCCF首发]Example.Movie.1936.BluRay.1080p.DD1.0.x264-EXGRP"  href="details.php?id=90001&amp;hit=1"><b>[TCCF首发]Example.Movie.1936.BluRay.1080p.DD1.0.x264-EXGRP</b></a><b> (<font class='new'>新</font>)</b> <b>[<font class='hot'>热门</font>]</b> <img class="pro_free2up" src="pic/trans.gif" alt="2X Free"  onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;twoupfree&quot;&gt;2X免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-07-29 14:25:51&quot;&gt;1天23时&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000,'fade','both','styleClass','niceTitle', 'fadeMax',87, 'maxWidth', 300);" />&nbsp;剩余时间：<b><span title="2026-07-29 14:25:51">1天23时</span></b><br />示例电影[全新修复完整版|原盘压制|简英字幕]</td><td width="20" class="embedded" style="text-align: right; " valign="middle"><a href="download.php?id=90001"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=90001&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-07-27 14:25:51">45分</span></td><td class="rowfollow">4.17<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=90001&amp;hit=1&amp;dllist=1#seeders"><font color="">22</font></a></b></td>
<td class="rowfollow"><b><a href="details.php?id=90001&amp;hit=1&amp;dllist=1#leechers">6</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=90001"><b>23</b></a></td>
<td align="center">-</td><td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=633" title="Elearning - 电子书/非小说"><img class="elearning" src="pic/cattrans.gif" alt="Elearning - 电子书/非小说" /></a></td>
<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr><td class="embedded"><a title="Example.Cartoon.Idioms.Collection"  href="details.php?id=90002&amp;hit=1"><b>Example.Cartoon.Idioms.Collection</b></a> <img class="pro_free" src="pic/trans.gif" alt="Free" title="免费" /><br />示例成語動畫</td><td width="20" class="embedded" style="text-align: right; " valign="middle"><a href="download.php?id=90002"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=90002&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-07-20 09:12:00">7天<br />5时</span></td><td class="rowfollow">88.5<br />MB</td><td class="rowfollow" align="center"><b><a href="details.php?id=90002&amp;hit=1&amp;dllist=1#seeders"><font color="">5</font></a></b></td>
<td class="rowfollow">1</td>
<td class="rowfollow"><a href="viewsnatches.php?id=90002"><b>12</b></a></td>
<td align="center">-</td><td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=624" title="Documentaries.纪录片"><img class="documentaries" src="pic/cattrans.gif" alt="Documentaries.纪录片" /></a></td>
<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr><td class="embedded"><a title="Example.Nature.2026.E205.1080p.HDTV.H264.AAC-EXEDU"  href="details.php?id=90003&amp;hit=1"><b>Example.Nature.2026.E205.1080p.HDTV.H264.AAC-EXEDU</b></a> <img class="pro_50pctdown" src="pic/trans.gif" alt="50%" title="50%" /><br />示例纪录片 | 2026年 第205期 | [国语/中字]</td><td width="20" class="embedded" style="text-align: right; " valign="middle"><a href="download.php?id=90003"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=90003&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-07-26 22:00:29">17时<br />11分</span></td><td class="rowfollow">2.79<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=90003&amp;hit=1&amp;dllist=1#seeders"><font color="">18</font></a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><a href="viewsnatches.php?id=90003"><b>30</b></a></td>
<td align="center">-</td><td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=633" title="Elearning - 杂项学习 "><img class="elearning" src="pic/cattrans.gif" alt="Elearning - 杂项学习 " /></a></td>
<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr><td class="embedded"><a title="Example.Textbook.Grade2.Chinese"  href="details.php?id=90004&amp;hit=1"><b>Example.Textbook.Grade2.Chinese</b></a><br />示例國小國語2上電子教科書</td><td width="20" class="embedded" style="text-align: right; " valign="middle"><a href="download.php?id=90004"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=90004&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-07-13 07:09:27">14天<br />8时</span></td><td class="rowfollow">11.12<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=90004&amp;hit=1&amp;dllist=1#seeders"><font color="">4</font></a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><a href="viewsnatches.php?id=90004"><b>9</b></a></td>
<td align="center">-</td><td class="rowfollow"><i>匿名</i></td>
</tr>
</table>
</body></html>`

const et8IndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr>
<td><table width="100%" cellspacing="0" cellpadding="0" border="0"><tr>
<td class="bottom" align="left"><span class="medium">欢迎回来, <span class="nowrap"><a  href="userdetails.php?id=10086" class='VeteranUser_Name'><b>fixture_user</b></a></span>  [<a href="logout.php">退出</a>]  [<a href="torrents.php?inclbookmarked=1&amp;allsec=1">收藏</a>] <font class = 'color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 1,219,747.6 <font class = 'color_invite'>邀请 </font>[<a href="invite.php?id=10086">发送</a>]: 4<br />
<font class="color_ratio">分享率：</font> 3.907  <font class = 'color_uploaded'>上传量：</font> 3.913 TB<font class='color_downloaded'> 下载量：</font> 1.001 TB  <font class='color_active'>当前活动：</font> <img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />285  <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />0&nbsp;&nbsp;<font class = 'color_connectable'>可连接：</font><a href="https://et8.org/faq.php#id21"><b><font color="green">是</font></b></a></span></td>
</tr></table></td>
</tr></table>
</body></html>`

const et8UserdetailsFixture = `<html><body>
<table>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">等级</td><td width="99%" class="rowfollow" valign="top" align="left"><img alt="养老族" title="养老族" src="pic/retiree.gif" /> &nbsp;86</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">邀请</td><td class="rowfollow" valign="top" align="left">26</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">加入日期</td><td width="99%" class="rowfollow" valign="top" align="left">2012-05-29 18:35:08 (<span title="2012-05-29 18:35:08">14年4月前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-07-18 15:48:13 (<span title="2026-07-18 15:48:13">8天23时前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向(地点)</td><td width="99%" class="rowfollow" valign="top" align="left"></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">传输</td><td width="99%" class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>:  <font color="">82.107</font></td><td class="embedded">&nbsp;&nbsp;<img src="pic/smilies/super.gif" alt="" /></td></tr><tr><td class="embedded"><strong>上传量</strong>:  389.081 TB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>:  4.739 TB</td></tr></table></td></tr>
</table>
</body></html>`

// 详情页真实样本为 2X 免费（h1 中的 class 是 twoupfree，而非 free）
const et8DetailFixture = `<html><body>
<h1 align="center" id="top"><font color=red>[置顶++]</font> [TCCF首发]Example.Movie.1936.BluRay.1080p.DD1.0.x264-EXGRP <b>[<font class='twoupfree'  onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;twoupfree&quot;&gt;2X免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-07-29 14:25:51&quot;&gt;1天23时&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000,'fade','both','styleClass','niceTitle', 'fadeMax',87, 'maxWidth', 300);">2X免费</font>]</b>&nbsp;剩余时间：<b><span title="2026-07-29 14:25:51">1天23时</span></b></h1>
<table>
<tr><td class="rowhead" width="13%">下载</td><td class="rowfollow" width="87%" align="left"><a class="index" href="download.php?id=90001">[TCCF].Example.Movie.1936.BluRay.1080p.DD1.0.x264-EXGRP.torrent</a></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">示例电影[全新修复完整版|原盘压制|简英字幕]</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>4.17 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;Movies.电影&nbsp;&nbsp;&nbsp;<b>媒介:&nbsp;</b>Encode&nbsp;&nbsp;&nbsp;<b>编码:&nbsp;</b>x264&nbsp;&nbsp;&nbsp;<b>分辨率:&nbsp;</b>1080p</td></tr>
</table>
<form><input type="hidden" name="torrent_name" value="[TCCF首发]Example.Movie.1936.BluRay.1080p.DD1.0.x264-EXGRP" />
<input type="hidden" name="detail_torrent_id" value="90001" />
<input type="hidden" name="in_detail" value="in_detail" /></form>
</body></html>`

// 无优惠种子：h1 中仅有无 class 的 <font color=red>，DiscountSelector 不应误命中
const et8DetailNoPromoFixture = `<html><body>
<h1 align="center" id="top">Example.Textbook.Grade2.Chinese</h1>
<table>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>88.5 MB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;Elearning&nbsp;&nbsp;&nbsp;<b>媒介:&nbsp;</b>Other</td></tr>
</table>
<form><input type="hidden" name="torrent_name" value="Example.Textbook.Grade2.Chinese" />
<input type="hidden" name="detail_torrent_id" value="90003" /></form>
</body></html>`

// --- Helpers ---

func getEt8Def(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("et8")
	require.True(t, ok, "et8 definition not found")
	return def
}

// --- Suite: Search ---

func testEt8Search(t *testing.T) {
	def := getEt8Def(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(et8SearchFixture))
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
	require.Len(t, items, 4, "should parse 4 torrent rows (colhead row excluded)")

	twoUpFree := items[0]
	assert.Equal(t, "90001", twoUpFree.ID)
	assert.Equal(t, "[TCCF首发]Example.Movie.1936.BluRay.1080p.DD1.0.x264-EXGRP", twoUpFree.Title)
	assert.Equal(t, "示例电影[全新修复完整版|原盘压制|简英字幕]", twoUpFree.Subtitle)
	assert.Equal(t, v2.Discount2xFree, twoUpFree.DiscountLevel)
	// 站点未把结束时间放进独立可选中的元素，只能从 onmouseover tooltip 的实体编码里兜底解析
	require.False(t, twoUpFree.DiscountEndTime.IsZero(), "discount end time should come from onmouseover tooltip")
	assert.Equal(t, "2026-07-29 14:25:51", twoUpFree.DiscountEndTime.Format("2006-01-02 15:04:05"))
	assert.Equal(t, "Movies.电影", twoUpFree.Category)
	assert.Equal(t, 22, twoUpFree.Seeders)
	assert.Equal(t, 6, twoUpFree.Leechers)
	assert.Equal(t, 23, twoUpFree.Snatched)
	// 大小单元格为 "4.17<br />GB"
	assert.Equal(t, int64(4477503406), twoUpFree.SizeBytes)
	assert.NotZero(t, twoUpFree.UploadedAt, "upload time should be parsed from td4 span[title]")

	free := items[1]
	assert.Equal(t, "90002", free.ID)
	assert.Equal(t, "Example.Cartoon.Idioms.Collection", free.Title)
	assert.Equal(t, "示例成語動畫", free.Subtitle)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	assert.True(t, free.DiscountEndTime.IsZero(), "长期免费无结束时间")
	assert.Equal(t, 5, free.Seeders)
	assert.Equal(t, 1, free.Leechers)
	assert.Equal(t, 12, free.Snatched)
	assert.Equal(t, int64(92798976), free.SizeBytes)

	halfDown := items[2]
	assert.Equal(t, "90003", halfDown.ID)
	assert.Equal(t, "示例纪录片 | 2026年 第205期 | [国语/中字]", halfDown.Subtitle)
	assert.Equal(t, v2.DiscountPercent50, halfDown.DiscountLevel)
	assert.True(t, halfDown.DiscountEndTime.IsZero())
	assert.Equal(t, "Documentaries.纪录片", halfDown.Category)
	assert.Equal(t, 18, halfDown.Seeders)
	assert.Equal(t, 0, halfDown.Leechers)
	assert.Equal(t, 30, halfDown.Snatched)
	assert.Equal(t, int64(2995739688), halfDown.SizeBytes)

	plain := items[3]
	assert.Equal(t, "90004", plain.ID)
	assert.Equal(t, "示例國小國語2上電子教科書", plain.Subtitle)
	assert.Equal(t, v2.DiscountNone, plain.DiscountLevel)
	assert.True(t, plain.DiscountEndTime.IsZero())
	assert.Equal(t, 4, plain.Seeders)
	assert.Equal(t, 0, plain.Leechers)
	assert.Equal(t, 9, plain.Snatched)
	assert.Equal(t, int64(11940009082), plain.SizeBytes)
}

// --- Suite: Detail ---

func testEt8Detail(t *testing.T) {
	def := getEt8Def(t)

	t.Run("TwoUpFree", func(t *testing.T) {
		doc := FixtureDoc(t, "et8_detail", et8DetailFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "90001", info.TorrentID)
		assert.Equal(t, "[TCCF首发]Example.Movie.1936.BluRay.1080p.DD1.0.x264-EXGRP", info.Title)
		assert.NotEmpty(t, info.Title)
		assert.Equal(t, v2.Discount2xFree, info.DiscountLevel)
		require.False(t, info.DiscountEnd.IsZero(), "discount end time should be parsed from h1 span[title]")
		assert.Equal(t, "2026-07-29 14:25:51", info.DiscountEnd.Format("2006-01-02 15:04:05"))
		// 4.17 GB -> MB（不是 0，也不是 4.17）
		assert.InDelta(t, 4.17*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR, "et8 无 H&R")
	})

	t.Run("NoPromotion", func(t *testing.T) {
		doc := FixtureDoc(t, "et8_detail_nopromo", et8DetailNoPromoFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "90003", info.TorrentID)
		assert.Equal(t, "Example.Textbook.Grade2.Chinese", info.Title)
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel)
		assert.True(t, info.DiscountEnd.IsZero())
		assert.InDelta(t, 88.5, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})
}

// --- Suite: UserInfo ---

func testEt8UserInfo(t *testing.T) {
	def := getEt8Def(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "et8_index", et8IndexFixture)
		fields := map[string]string{
			"id":       "10086",
			"name":     "fixture_user",
			"seeding":  "285",
			"leeching": "0",
			// parseNumber 产出 float64，toString 用 %v 渲染，量级达到 1e6 后为科学计数法；
			// setUserInfoField 再走 strconv.ParseFloat 还原，见下方 bonus_roundtrip
			"bonus": "1.2197476e+06",
		}
		for field, expected := range fields {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q not found", field)
				got := driver.ExtractFieldValuePublic(doc, sel)
				assert.Equal(t, expected, got)
			})
		}

		t.Run("bonus_roundtrip", func(t *testing.T) {
			got := driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["bonus"])
			f, err := strconv.ParseFloat(got, 64)
			require.NoError(t, err)
			assert.InDelta(t, 1219747.6, f, 0.01)
		})
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "et8_userdetails", et8UserdetailsFixture)
		exact := map[string]string{
			"uploaded":     "427799083646713", // 389.081 TB
			"downloaded":   "5210585604030",   // 4.739 TB
			"ratio":        "82.107",
			"levelName":    "养老族",
			"joinTime":     "1338287708", // 2012-05-29 18:35:08 +0800
			"lastAccessAt": "1784360893", // 2026-07-18 15:48:13 +0800
		}
		for field, expected := range exact {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q not found", field)
				got := driver.ExtractFieldValuePublic(doc, sel)
				assert.Equal(t, expected, got)
			})
		}

		// 保号探测（lastAccessAt）必须产出正数时间戳
		t.Run("lastAccessAt_positive", func(t *testing.T) {
			sel := def.UserInfo.Selectors["lastAccessAt"]
			got := driver.ExtractFieldValuePublic(doc, sel)
			ts, err := strconv.ParseInt(got, 10, 64)
			require.NoError(t, err, "lastAccessAt 应为 Unix 时间戳字符串，实际 %q", got)
			assert.Greater(t, ts, int64(0), "lastAccessAt 必须 > 0，否则保号探测会失败")
		})
	})
}

// --- Standalone Tests ---

func TestEt8_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":         et8SearchFixture,
		"index":          et8IndexFixture,
		"userdetails":    et8UserdetailsFixture,
		"detail":         et8DetailFixture,
		"detail_nopromo": et8DetailNoPromoFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
