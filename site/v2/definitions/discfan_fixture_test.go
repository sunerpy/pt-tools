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
		SiteID:   "discfan",
		Search:   testDiscfanSearch,
		Detail:   testDiscfanDetail,
		UserInfo: testDiscfanUserInfo,
	})
}

// --- Fixtures ---
//
// 结构复刻真实采集页面（繁体界面、9 列种子表、傳送 合并单元格、基本資訊 体积行），
// 用户名/用户 ID/种子 ID/图片地址均为虚构值。

// 搜索页：3 行——限时免费（图标带 onmouseover）、2X 免费（无 onmouseover，即永久促销无结束时间）、
// 无促销（副标题前带语言标签 <span>）。
const discfanSearchFixture = `<html><body>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%">
<tr>
<td class="colhead" style="padding: 0px">類型</td>
<td class="colhead"><a href="?sort=1&amp;type=asc">標題</a></td>
<td class="colhead"><img class="comments" alt="comments" title="評論數" /></td>
<td class="colhead"><img class="time" alt="time" title="存活時間" /></td>
<td class="colhead"><img class="size" alt="size" title="大小" /></td>
<td class="colhead"><img class="seeders" alt="seeders" title="種子數" /></td>
<td class="colhead"><img class="leechers" alt="leechers" title="下載數" /></td>
<td class="colhead"><img class="snatched" alt="snatched" title="完成數" /></td>
<td class="colhead"><a href="?sort=9&amp;type=desc">發佈者</a></td>
</tr>
<tr style="background-color: #aadbf3">
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=411"><img class="c_sacd" src="pic/cattrans.gif" alt="剧集" title="剧集" /></a><img src="pic/cattrans.gif" alt="WEB_DL" title="WEB_DL" /></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="https://img.example.invalid/poster-a.jpg" class="nexus-lazy-load" /></td><td class="embedded" style='padding-left: 5px'><img class="sticky" src="pic/trans.gif" alt="Sticky" title="二級置頂" />&nbsp;<a title="Sample.Series.2016.V2.WEB-DL.4k.H265.AAC-DEMO"  href="details.php?id=10001&amp;hit=1"><b>Sample.Series.2016.V2.WEB-DL.4k.H265.AAC-DEMO</b></a> <img class="pro_free" src="pic/trans.gif" alt="Free"  onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免費&lt;/font&gt;&lt;/b&gt;剩餘時間：&lt;b&gt;&lt;span title=&quot;2026-08-01 23:42:19&quot;&gt;5天8時&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500);" /> <font color='#0000FF'>剩餘時間：<span title="2026-08-01 23:42:19">5天8時</span></font><br />範例劇集/示例剧集 全34集|主演：甲乙丙  丁戊己 高码率版本</td><td class="embedded" style="text-align: right; width: 40px;padding: 4px"><div><img src="pic/imdb2.png" alt="imdb" title="imdb" /><span data-imdbid="tt0000001">N/A</span></div></td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=10001"><img class="download" src="pic/trans.gif" alt="download" title="下載本種" /></a><br /><a id="bookmark0"  href="javascript: bookmark(10001,0);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td>
<td class="rowfollow"><a href="comment.php?action=add&amp;pid=10001&amp;type=torrent" title="添加評論">0</a></td>
<td class="rowfollow nowrap"><span title="2021-02-09 14:42:19">5年<br />6月</span></td>
<td class="rowfollow">188.30<br />GB</td>
<td class="rowfollow" align="center"><b><a href="details.php?id=10001&amp;hit=1&amp;dllist=1#seeders">3</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><a href="viewsnatches.php?id=10001"><b>23</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=404"><img class="c_movies" src="pic/cattrans.gif" alt="电影 - 中国香港" title="电影 - 中国香港" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="" class="nexus-lazy-load" /></td><td class="embedded" style='padding-left: 5px'><a title="Sample.Collection.1987-1991.1080p.Blu-ray.AVC-DEMO"  href="details.php?id=10002&amp;hit=1"><b>Sample.Collection.1987-1991.1080p.Blu-ray.AVC-DEMO</b></a> <img class="pro_free2up" src="pic/trans.gif" alt="2X Free" /><br />範例合集1+2 | 主演：庚辛壬 癸子丑 [国粤中字]</td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=10002"><img class="download" src="pic/trans.gif" alt="download" title="下載本種" /></a><br /><a id="bookmark1"  href="javascript: bookmark(10002,0);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td>
<td class="rowfollow"><a href="comment.php?action=add&amp;pid=10002&amp;type=torrent" title="添加評論">5</a></td>
<td class="rowfollow nowrap"><span title="2017-04-22 20:36:34">9年<br />3月</span></td>
<td class="rowfollow">44.48<br />GB</td>
<td class="rowfollow" align="center"><b><a href="details.php?id=10002&amp;hit=1&amp;dllist=1#seeders">1</a></b></td>
<td class="rowfollow">2</td>
<td class="rowfollow"><a href="viewsnatches.php?id=10002"><b>90</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=401"><img class="c_doc" src="pic/cattrans.gif" alt="电影 - 中国大陆" title="电影 - 中国大陆" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="" class="nexus-lazy-load" /></td><td class="embedded" style='padding-left: 5px'><a title="Sample.Movie.1979.1080p.USA.Blu-ray.AVC.DTS-HD.MA.2.0"  href="details.php?id=10003&amp;hit=1"><b>Sample.Movie.1979.1080p.USA.Blu-ray.AVC.DTS-HD.MA.2.0</b></a> <b>[<font class='hot'>熱門</font>]</b><br /><span style="background-color:#FF1493;color:#ffffff;font-size:12px;padding:1px 2px" title="">粤语</span><span style="background-color:#6a3906;color:#ffffff;font-size:12px;padding:1px 2px" title="">国语</span>範例電影</td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=10003"><img class="download" src="pic/trans.gif" alt="download" title="下載本種" /></a><br /><a id="bookmark2"  href="javascript: bookmark(10003,0);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td>
<td class="rowfollow"><a href="comment.php?action=add&amp;pid=10003&amp;type=torrent" title="添加評論">2</a></td>
<td class="rowfollow nowrap"><span title="2026-07-25 18:20:00">1天<br />6時</span></td>
<td class="rowfollow">17.86<br />GB</td>
<td class="rowfollow" align="center"><b><a href="details.php?id=10003&amp;hit=1&amp;dllist=1#seeders">6</a></b></td>
<td class="rowfollow">1</td>
<td class="rowfollow"><a href="viewsnatches.php?id=10003"><b>10</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
</table>
</body></html>`

// 详情页：h1#top 内为标题文本 + [免費] 促销标记 + 剩餘時間；
// 标题/种子 ID 取自字幕上传表单内的隐藏域；体积行标签为繁体「基本資訊」。
const discfanDetailFixture = `<html><body>
<h1 align="center" id="top">Sample.Series.2016.V2.WEB-DL.4k.H265.AAC-DEMO&nbsp;&nbsp;&nbsp; <b>[<font class='free' >免費</font>]</b> <font color='#0000FF'>剩餘時間：<span title="2026-08-01 23:42:19">5天8時</span></font></h1>
<table width="97%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead" width="13%">下載</td><td class="rowfollow" width="87%" align="left"><a class="index" href="download.php?id=10001">[DiscFan].Sample.Series.2016.V2.WEB-DL.4k.H265.AAC-DEMO.torrent</a>&nbsp;&nbsp;&nbsp;由&nbsp;<i>匿名</i>發布于<span title="2021-02-09 14:42:19">5年6月前</span></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副標題</td><td class="rowfollow" valign="top" align="left">範例劇集/示例剧集 全34集|主演：甲乙丙  丁戊己 高码率版本</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本資訊</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>188.30 GB&nbsp;&nbsp;&nbsp;<b>類別:</b>&nbsp;剧集&nbsp;&nbsp;&nbsp;<b>來源: </b>Web-DL</td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Sample.Series.2016.V2.WEB-DL.4k.H265.AAC-DEMO" /><input type="hidden" name="detail_torrent_id" value="10001" /><input type="hidden" name="in_detail" value="in_detail" /><input type="submit" value="上傳字幕" /></form></td></tr>
</table>
</body></html>`

// 详情页（含 H&R 标记，且无促销）：验证 HRKeywords 与「未映射 font class 被跳过」两条路径。
const discfanDetailHRFixture = `<html><body>
<h1 align="center" id="top">Sample.Movie.2020.1080p.Blu-ray.AVC-DEMO&nbsp;&nbsp;&nbsp; <b>[<font class='hot'>熱門</font>]</b></h1>
<table width="97%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead nowrap" valign="top" align="right">基本資訊</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>8.50 GB&nbsp;&nbsp;&nbsp;<b>類別:</b>&nbsp;电影</td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Sample.Movie.2020.1080p.Blu-ray.AVC-DEMO" /><input type="hidden" name="detail_torrent_id" value="10004" /></form></td></tr>
</table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

// index.php 顶部 #info_block（每页共用）。
const discfanIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr>
	<td><table width="100%" cellspacing="0" cellpadding="0" border="0"><tr>
		<td class="bottom" align="left">
			<span class="medium">
				歡迎回來, <span class="nowrap"><a  href="https://discfan.net/userdetails.php?id=99001" class='UltimateUser_Name'><b>ptuser01</b></a></span>                [<a href="logout.php">退出</a>]
				<font class = 'color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 1,234,567.8                 <a href="medal.php">[勛章]</a>
				<font class = 'color_invite'>邀請 </font>[<a href="invite.php?id=99001">發送</a>]: 3(0)                                <br />
				<font class="color_ratio">分享率：</font> 2.000                <font class='color_uploaded'>上傳量：</font> 2.500 TB                <font class='color_downloaded'> 下載量：</font> 1.250 TB                <font class='color_active'>當前活動：</font> <img class="arrowup" alt="Torrents seeding" title="當前做種" src="pic/trans.gif" />42  <img class="arrowdown" alt="Torrents leeching" title="當前下載" src="pic/trans.gif" />3&nbsp;&nbsp;
			</span>
		</td>
	</tr></table></td>
</tr></table>
</body></html>`

// userdetails.php 正文：注意传输行标签是繁体「傳送」，且为嵌套 table 的合并单元格；
// 「最近動向」与「最近動向(地點)」两行同时存在，取 First() 必须命中时间行。
const discfanUserdetailsFixture = `<html><body>
<h1 style='margin:0px'><span class="nowrap"><b>SampleUser</b></span></h1>
<table width="100%" border="1" cellspacing="0" cellpadding="5">
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">用戶ID/UID</td><td width="99%" class="rowfollow" valign="top" align="left">99001</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">邀請人</td><td width="99%" class="rowfollow" valign="top" align="left"><span class="nowrap"><a  href="https://discfan.net/userdetails.php?id=99002" class='Moderator_Name'><b>inviter01</b></a></span></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">加入日期</td><td width="99%" class="rowfollow" valign="top" align="left">2023-05-06 07:08:09 (<span title="2023-05-06 07:08:09">3年2月前</span>, 116.9周)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近動向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-07-27 05:15:44 (<span title="2026-07-27 05:15:44">9時52分前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近動向(地點)</td><td width="99%" class="rowfollow" valign="top" align="left">torrents</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">傳送</td><td width="99%" class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>:  <font color="#770000">2.000</font>（<strong>實際分享率</strong>：0.667）</td><td class="embedded">&nbsp;&nbsp;<img src="pic/smilies/34.gif" alt="" /></td></tr><tr><td class="embedded"><strong>上傳量</strong>:  2.500 TB</td><td class="embedded">&nbsp;&nbsp;<strong>下載量</strong>:  1.250 TB</td></tr><tr><td class="embedded"><strong>實際上傳量</strong>:  500.00 GB</td><td class="embedded">&nbsp;&nbsp;<strong>實際下載量</strong>:  750.00 GB</td><td class="embedded text-muted">&nbsp;&nbsp;實際上傳/下載量 (僅用於記錄, 不參與分享率計算)</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">等級</td><td width="99%" class="rowfollow" valign="top" align="left"><img alt="Power User" title="Power User" src="pic/user.gif" /> </td></tr>
</table>
</body></html>`

// --- Helpers ---

func getDiscfanDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("discfan")
	require.True(t, ok, "discfan definition not found")
	return def
}

// --- Suite: Search ---

func testDiscfanSearch(t *testing.T) {
	def := getDiscfanDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(discfanSearchFixture))
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
	require.Len(t, items, 3, "should parse 3 torrent rows (colhead row excluded)")

	free := items[0]
	assert.Equal(t, "10001", free.ID)
	assert.Equal(t, "Sample.Series.2016.V2.WEB-DL.4k.H265.AAC-DEMO", free.Title)
	assert.Equal(t, "範例劇集/示例剧集 全34集|主演：甲乙丙  丁戊己 高码率版本", free.Subtitle)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	// 促销结束时间只能从促销图标的 onmouseover 提示中解析
	require.False(t, free.DiscountEndTime.IsZero(), "discount end time should come from onmouseover tooltip")
	assert.Equal(t, "2026-08-01 23:42:19", free.DiscountEndTime.Format("2006-01-02 15:04:05"))
	assert.Equal(t, int64(202185585459), free.SizeBytes) // 188.30 GB（size 单元格被 <br /> 分成两行）
	assert.Equal(t, 3, free.Seeders)
	assert.Equal(t, 0, free.Leechers)
	assert.Equal(t, 23, free.Snatched)
	assert.Equal(t, "剧集", free.Category)
	assert.Equal(t, int64(1612881739), free.UploadedAt)

	twoXFree := items[1]
	assert.Equal(t, "10002", twoXFree.ID)
	assert.Equal(t, v2.Discount2xFree, twoXFree.DiscountLevel)
	assert.Equal(t, "範例合集1+2 | 主演：庚辛壬 癸子丑 [国粤中字]", twoXFree.Subtitle)
	// 永久促销没有 onmouseover 提示，结束时间未知
	assert.True(t, twoXFree.DiscountEndTime.IsZero(), "permanent promotion has no end time")
	assert.Equal(t, 1, twoXFree.Seeders)
	assert.Equal(t, 2, twoXFree.Leechers)
	assert.Equal(t, 90, twoXFree.Snatched)

	plain := items[2]
	assert.Equal(t, "10003", plain.ID)
	assert.Equal(t, v2.DiscountNone, plain.DiscountLevel, "[熱門] 不是促销标记")
	// 副标题前有两个语言标签 <span>，需被正则跳过
	assert.Equal(t, "範例電影", plain.Subtitle)
	assert.Equal(t, 6, plain.Seeders)
	assert.Equal(t, 1, plain.Leechers)
	assert.Equal(t, 10, plain.Snatched)
}

// --- Suite: Detail ---

func testDiscfanDetail(t *testing.T) {
	def := getDiscfanDef(t)

	t.Run("Free", func(t *testing.T) {
		doc := FixtureDoc(t, "discfan_detail", discfanDetailFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "10001", info.TorrentID)
		assert.Equal(t, "Sample.Series.2016.V2.WEB-DL.4k.H265.AAC-DEMO", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		require.False(t, info.DiscountEnd.IsZero(), "discount end time should be parsed")
		assert.Equal(t, "2026-08-01 23:42:19", info.DiscountEnd.Format("2006-01-02 15:04:05"))
		// 繁体「基本資訊」标签列 + 全角冒号「大小：」+ 十进制 GB
		assert.InDelta(t, 188.30*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})

	t.Run("WithHRAndNonPromotionFont", func(t *testing.T) {
		doc := FixtureDoc(t, "discfan_detail_hr", discfanDetailHRFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "10004", info.TorrentID)
		assert.Equal(t, "Sample.Movie.2020.1080p.Blu-ray.AVC-DEMO", info.Title)
		assert.True(t, info.HasHR, "should detect HR from hit_run.gif")
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel, "font.hot 未映射，应保持无促销")
		assert.InDelta(t, 8.50*1024, info.SizeMB, 0.1)
	})
}

// --- Suite: UserInfo ---

func testDiscfanUserInfo(t *testing.T) {
	def := getDiscfanDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "discfan_index", discfanIndexFixture)
		fields := map[string]string{
			"id":       "99001",
			"name":     "ptuser01",
			"seeding":  "42",
			"leeching": "3",
			"bonus":    "1.2345678e+06",
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
		doc := FixtureDoc(t, "discfan_userdetails", discfanUserdetailsFixture)
		fields := map[string]string{
			"uploaded":       "2748779069440",
			"downloaded":     "1374389534720",
			"ratio":          "2",
			"trueUploaded":   "536870912000",
			"trueDownloaded": "805306368000",
			"levelName":      "Power User",
			"joinTime":       "1683328089",
			"lastAccessAt":   "1785100544",
		}
		for field, expected := range fields {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q not found", field)
				assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel))
			})
		}
	})
}

// --- Standalone Tests ---

// 保号探测只读取 UserInfo.LastAccess，缺失或解析失败会导致线上探测报错。
func TestDiscfan_UserInfo_LastAccessParsed(t *testing.T) {
	def := getDiscfanDef(t)
	driver := newTestNexusPHPDriver(def)
	doc := FixtureDoc(t, "discfan_userdetails", discfanUserdetailsFixture)

	sel, ok := def.UserInfo.Selectors["lastAccessAt"]
	require.True(t, ok, "lastAccessAt selector is required for the login-state probe")

	raw := driver.ExtractFieldValuePublic(doc, sel)
	ts, err := strconv.ParseInt(raw, 10, 64)
	require.NoError(t, err, "lastAccessAt should be a unix timestamp, got %q", raw)
	assert.Greater(t, ts, int64(0), "lastAccessAt must be > 0")
}

func TestDiscfan_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      discfanSearchFixture,
		"detail":      discfanDetailFixture,
		"detail_hr":   discfanDetailHRFixture,
		"index":       discfanIndexFixture,
		"userdetails": discfanUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
