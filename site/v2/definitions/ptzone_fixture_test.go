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
		SiteID:   "ptzone",
		Search:   testPtzoneSearch,
		Detail:   testPtzoneDetail,
		UserInfo: testPtzoneUserInfo,
	})
}

// --- Fixtures ---
//
// 结构复刻真实采集页面（繁体界面、9 列种子表、名称单元格内的审核状态 <span title="通過">、
// 傳送 合并单元格、基本資訊 体积行），用户名/用户 ID/种子 ID/图片地址均为虚构值。

// 搜索页：3 行
//  1. 2X 免费且为**永久**促销（图标只有 title，无 onmouseover、无「剩餘時間」），完成数带千分位；
//  2. 限时免费（图标带 onmouseover + 可见的 <font color> 剩餘時間），副标题前有语言标签 <span>；
//  3. 无促销（仅 [熱門] 标记），用于验证审核状态 <span title="通過"> 不会被当成促销结束时间。
const ptzoneSearchFixture = `<html><body>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%">
<tr>
<td class="colhead" style="padding: 0px">類型</td>
<td class="colhead"><a href="?sort=1&amp;type=asc">標題</a></td>
<td class="colhead"><a href="?sort=3&amp;type=desc"><img class="comments" src="pic/trans.gif" alt="comments" title="評論數" /></a></td>
<td class="colhead"><a href="?sort=4&amp;type=desc"><img class="time" src="pic/trans.gif" alt="time" title="存活時間" /></a></td>
<td class="colhead"><a href="?sort=5&amp;type=desc"><img class="size" src="pic/trans.gif" alt="size" title="大小" /></a></td>
<td class="colhead"><a href="?sort=7&amp;type=desc"><img class="seeders" src="pic/trans.gif" alt="seeders" title="種子數" /></a></td>
<td class="colhead"><a href="?sort=8&amp;type=desc"><img class="leechers" src="pic/trans.gif" alt="leechers" title="下載數" /></a></td>
<td class="colhead"><a href="?sort=6&amp;type=desc"><img class="snatched" src="pic/trans.gif" alt="snatched" title="完成數" /></a></td>
<td class="colhead"><a href="?sort=9&amp;type=desc">發佈者</a></td>
</tr>
<tr style="background-color: #89c9e6">
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=408"><img class="c_others" src="pic/cattrans.gif" alt="Others(其它）" title="Others(其它）" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr style="background-color: #89c9e6"><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="https://img.example.invalid/cover-a.jpg" class="nexus-lazy-load" /></td><td class="embedded" style='padding-left: 5px'><img class="sticky" src="pic/trans.gif" alt="Sticky" title="一級置頂" />&nbsp;<a title="Sample.Guide.PT.V1.0.PDF"  href="details.php?id=20001&amp;hit=1"><b>Sample.Guide.PT.V1.0.PDF</b></a> <b>[<font class='hot'>熱門</font>]</b> <img class="pro_free2up" src="pic/trans.gif" alt="2X Free" title="2X免費" /><span style="margin-left: 6px" title="通過"><svg t="1655145688503" class="icon" viewBox="0 0 1413 1024" version="1.1" width="16" height="16"><path d="M1381 107L1274 0 465 809 142 487 35 594l430 430z" fill="#1afa29"></path></svg></span><br />範例入門教材 保證學以致用 【新手必看】</td><td class="embedded" style="text-align: right; width: 40px;padding: 4px"><div><img src="pic/imdb2.png" alt="imdb" title="imdb" /><span>N/A</span></div></td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=20001"><img class="download" src="pic/trans.gif" alt="download" title="下載本種" /></a><br /><a id="bookmark0"  href="javascript: bookmark(20001,0);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td>
<td class="rowfollow"><b><a href="details.php?id=20001&amp;hit=1&amp;cmtpage=1#startcomments" >60</a></b></td>
<td class="rowfollow nowrap"><span title="2024-11-12 08:09:10">1年<br />8月</span></td>
<td class="rowfollow">2.26<br />MB</td>
<td class="rowfollow" align="center"><b><a href="details.php?id=20001&amp;hit=1&amp;dllist=1#seeders">12</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=20001&amp;hit=1&amp;dllist=1#leechers">1</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=20001"><b>7,170</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr style="background-color: #aadbf3">
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=405"><img class="c_anime" src="pic/cattrans.gif" alt="Animations(动漫)" title="Animations(动漫)" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr style="background-color: #aadbf3"><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="https://img.example.invalid/cover-b.jpg" class="nexus-lazy-load" /></td><td class="embedded" style='padding-left: 5px'><a title="Sample.Anime.2025.2160p.WEB-DL.H.265.DDP5.1-DEMO"  href="details.php?id=20002&amp;hit=1"><b>Sample.Anime.2025.2160p.WEB-DL.H.265.DDP5.1-DEMO</b></a> <img class="pro_free" src="pic/trans.gif" alt="Free"  onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免費&lt;/font&gt;&lt;/b&gt;剩餘時間：&lt;b&gt;&lt;span title=&quot;2026-08-06 12:34:56&quot;&gt;2天21時&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500);" /> <font color='#0000FF'>剩餘時間：<span title="2026-08-06 12:34:56">2天21時</span></font><span style="margin-left: 6px" title="未審"><svg t="1655184824967" class="icon" viewBox="0 0 1024 1024" version="1.1" width="16" height="16"><path d="M512 64l448 832H64z" fill="#f4ea2a"></path></svg></span><br /><span style="background-color:#0000ff;color:#ffffff;font-size:12px;padding:1px 2px" title="">官方</span><span style="background-color:#6a3906;color:#ffffff;font-size:12px;padding:1px 2px" title="">国语</span><span style="background-color:#006400;color:#ffffff;font-size:12px;padding:1px 2px" title="">中字</span>範例動畫 / 示例动画 | 类型：动画 / 奇幻 | 演员：甲乙丙 / 丁戊己</td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=20002"><img class="download" src="pic/trans.gif" alt="download" title="下載本種" /></a><br /><a id="bookmark1"  href="javascript: bookmark(20002,1);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td>
<td class="rowfollow"><b><a href="details.php?id=20002&amp;hit=1&amp;cmtpage=1#startcomments" >3</a></b></td>
<td class="rowfollow nowrap"><span title="2026-07-30 10:00:00">4天<br />6時</span></td>
<td class="rowfollow">12.34<br />GB</td>
<td class="rowfollow" align="center"><b><a href="details.php?id=20002&amp;hit=1&amp;dllist=1#seeders">34</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=20002&amp;hit=1&amp;dllist=1#leechers"><font color="#550000">2</font></a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=20002"><b>56</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr style="background-color: #aadbf3">
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=401"><img class="c_movies" src="pic/cattrans.gif" alt="Movies(电影)" title="Movies(电影)" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr style="background-color: #aadbf3"><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="" class="nexus-lazy-load" /></td><td class="embedded" style='padding-left: 5px'><a title="Sample.Movie.1979.1080p.Blu-ray.AVC.DTS-HD.MA.2.0-DEMO"  href="details.php?id=20003&amp;hit=1"><b>Sample.Movie.1979.1080p.Blu-ray.AVC.DTS-HD.MA.2.0-DEMO</b></a> <b>[<font class='hot'>熱門</font>]</b><span style="margin-left: 6px" title="通過"><svg t="1655145688503" class="icon" viewBox="0 0 1413 1024" version="1.1" width="16" height="16"><path d="M1381 107L1274 0 465 809 142 487 35 594l430 430z" fill="#1afa29"></path></svg></span><br />範例電影</td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=20003"><img class="download" src="pic/trans.gif" alt="download" title="下載本種" /></a><br /><a id="bookmark2"  href="javascript: bookmark(20003,2);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td>
<td class="rowfollow"><b><a href="details.php?id=20003&amp;hit=1&amp;cmtpage=1#startcomments" >1</a></b></td>
<td class="rowfollow nowrap"><span title="2026-07-31 11:22:33">3天<br />5時</span></td>
<td class="rowfollow">700.00<br />MB</td>
<td class="rowfollow" align="center"><b><a href="details.php?id=20003&amp;hit=1&amp;dllist=1#seeders">5</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><a href="viewsnatches.php?id=20003"><b>9</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
</table>
</body></html>`

// 详情页（真实采集页面对应形态）：h1#top 内为标题文本 + [2X免費] 促销标记 + 审核状态图标；
// 标题/种子 ID 取自字幕上传表单内的隐藏域；体积行标签为繁体「基本資訊」且单位为 MB。
const ptzoneDetailFixture = `<html><body>
<h1 align="center" id="top">Sample.Guide.PT.V1.0.PDF&nbsp;&nbsp;&nbsp; <b>[<font class='twoupfree' >2X免費</font>]</b><span style="margin-left: 6px" title="通過"><svg t="1655145688503" class="icon" viewBox="0 0 1413 1024" version="1.1" width="16" height="16"><path d="M1381 107L1274 0 465 809 142 487 35 594l430 430z" fill="#1afa29"></path></svg></span></h1>
<table width="97%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead" width="13%">下載</td><td class="rowfollow" width="87%" align="left"><a class="index" href="download.php?id=20001">[PTzone]Sample.Guide.PT.V1.0.PDF.torrent</a>&nbsp;&nbsp;&nbsp;由&nbsp;<i>匿名</i>發布於<span title="2024-11-12 08:09:10">1年8月前</span></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副標題</td><td class="rowfollow" valign="top" align="left">範例入門教材 保證學以致用 【新手必看】</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本資訊</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>2.26 MB&nbsp;&nbsp;&nbsp;<b>類別:</b>&nbsp;Others(其它）&nbsp;&nbsp;&nbsp;<b>媒介: </b>CD&nbsp;&nbsp;&nbsp;<b>編碼: </b>Other</td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Sample.Guide.PT.V1.0.PDF" /><input type="hidden" name="detail_torrent_id" value="20001" /><input type="hidden" name="in_detail" value="in_detail" /><input type="submit" value="上傳字幕" /></form></td></tr>
</table>
</body></html>`

// 详情页（限时免费 + H&R 标记 + GB 体积）：验证 EndTimeSelector、HRKeywords，
// 以及 [熱門] 这类未映射的 font class 会被 ParseDiscount 跳过。
const ptzoneDetailFreeTimedFixture = `<html><body>
<h1 align="center" id="top">Sample.Anime.2025.2160p.WEB-DL.H.265.DDP5.1-DEMO&nbsp;&nbsp;&nbsp; <b>[<font class='hot'>熱門</font>]</b> <b>[<font class='free' >免費</font>]</b> <font color='#0000FF'>剩餘時間：<span title="2026-08-06 12:34:56">2天21時</span></font><span style="margin-left: 6px" title="通過"><svg viewBox="0 0 1413 1024" width="16" height="16"><path d="M1381 107L1274 0 465 809z" fill="#1afa29"></path></svg></span></h1>
<table width="97%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead nowrap" valign="top" align="right">基本資訊</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>8.50 GB&nbsp;&nbsp;&nbsp;<b>類別:</b>&nbsp;Animations(动漫)</td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Sample.Anime.2025.2160p.WEB-DL.H.265.DDP5.1-DEMO" /><input type="hidden" name="detail_torrent_id" value="20002" /></form></td></tr>
</table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

// index.php 顶部 #info_block（每页共用，userdetails.php 同样带此块）。
const ptzoneIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr>
	<td><table width="100%" cellspacing="0" cellpadding="0" border="0"><tr>
		<td class="bottom" align="left">
			<span class="medium">
				歡迎回來, <span class="nowrap"><a  href="https://ptzone.xyz/userdetails.php?id=99001" class='PowerUser_Name'><b>ptuser01</b></a></span>                [<a href="logout.php">退出</a>]
				[<a href="usercp.php">&nbsp;控制面板&nbsp;</a>]
				<font class = 'color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 123,456.7                 <a href="attendance.php" class="faqlink">[簽到得魔力]</a>                <a href="medal.php">[勛章]</a>
				<font class = 'color_invite'>邀請 </font>[<a href="invite.php?id=99001">發送</a>]: 11(0)                                <br />
				<font class="color_ratio">分享率：</font> 2.500                <font class='color_uploaded'>上傳量：</font> 300.00 GB                <font class='color_downloaded'> 下載量：</font> 120.00 GB                <font class='color_active'>當前活動：</font> <img class="arrowup" alt="Torrents seeding" title="當前做種" src="pic/trans.gif" />42  <img class="arrowdown" alt="Torrents leeching" title="當前下載" src="pic/trans.gif" />3&nbsp;&nbsp;
				<font class='color_connectable'>可連接：</font><b><font color="green">是</font></b> <font class='color_slots'>連接數：</font>無限制
			</span>
		</td>
	</tr></table></td>
</tr></table>
</body></html>`

// userdetails.php 正文：传输行标签是繁体「傳送」，且为嵌套 table 的合并单元格；
// 「最近動向」与「最近動向(地點)」两行同时存在（后者真实站点为空），取 First() 必须命中时间行。
const ptzoneUserdetailsFixture = `<html><body>
<h1 style='margin:0px'><span class="nowrap"><b>ptuser01</b></span></h1>
<table width="100%" border="1" cellspacing="0" cellpadding="5">
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">用戶ID/UID</td><td width="99%" class="rowfollow" valign="top" align="left">99001</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">邀請</td><td width="99%" class="rowfollow" valign="top" align="left"><a href="invite.php?id=99001" title="傳送邀請">11(0)</a></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">加入日期</td><td width="99%" class="rowfollow" valign="top" align="left">2024-03-04 05:06:07 (<span title="2024-03-04 05:06:07">2年5月前</span>, 125.2周)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近動向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-07-30 09:08:07 (<span title="2026-07-30 09:08:07">&lt; 1分前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近動向(地點)</td><td width="99%" class="rowfollow" valign="top" align="left"></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">傳送</td><td width="99%" class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>:  <font color="">2.500</font>（<strong>實際分享率</strong>：0.048）</td><td class="embedded">&nbsp;&nbsp;<img src="pic/smilies/3.gif" alt="" /></td></tr><tr><td class="embedded"><strong>上傳量</strong>:  300.00 GB</td><td class="embedded">&nbsp;&nbsp;<strong>下載量</strong>:  120.00 GB</td></tr><tr><td class="embedded"><strong>實際上傳量</strong>:  50.00 GB</td><td class="embedded">&nbsp;&nbsp;<strong>實際下載量</strong>:  1.500 TB</td><td class="embedded text-muted">&nbsp;&nbsp;實際上傳/下載量 (僅用於記錄, 不參與分享率計算)</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">等級</td><td width="99%" class="rowfollow" valign="top" align="left"><img alt="Power User" title="Power User" src="pic/power.gif" /> </td></tr>
</table>
</body></html>`

// --- Helpers ---

func getPtzoneDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("ptzone")
	require.True(t, ok, "ptzone definition not found")
	return def
}

// --- Suite: Search ---

func testPtzoneSearch(t *testing.T) {
	def := getPtzoneDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(ptzoneSearchFixture))
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

	permanent2xFree := items[0]
	assert.Equal(t, "20001", permanent2xFree.ID)
	assert.Equal(t, "Sample.Guide.PT.V1.0.PDF", permanent2xFree.Title)
	assert.Equal(t, "範例入門教材 保證學以致用 【新手必看】", permanent2xFree.Subtitle)
	assert.Equal(t, v2.Discount2xFree, permanent2xFree.DiscountLevel, "pro_free2up 必须先于 free 命中")
	// 永久促销：图标无 onmouseover，单元格内也没有 <font color> 剩餘時間；
	// 同时验证审核状态 <span title="通過"> 不会被误当成结束时间
	assert.True(t, permanent2xFree.DiscountEndTime.IsZero(), "permanent promotion has no end time")
	assert.Equal(t, int64(2369781), permanent2xFree.SizeBytes) // 2.26 MB（size 单元格被 <br /> 分成两行）
	assert.Equal(t, 12, permanent2xFree.Seeders)
	assert.Equal(t, 1, permanent2xFree.Leechers)
	// 完成数为带千分位的「7,170」：共享驱动先经 extractNumber 去除分隔符再转换，
	// 因此能正确解析。真实采集页面 50 行中有 12 行是这种写法，
	// 此处保留该样本作为共享层千分位解析的回归守卫
	assert.Equal(t, 7170, permanent2xFree.Snatched)
	assert.Equal(t, "Others(其它）", permanent2xFree.Category)
	assert.Equal(t, int64(1731398950), permanent2xFree.UploadedAt)

	timedFree := items[1]
	assert.Equal(t, "20002", timedFree.ID)
	assert.Equal(t, v2.DiscountFree, timedFree.DiscountLevel)
	// 副标题前有三个语言/来源标签 <span>，需被正则跳过
	assert.Equal(t, "範例動畫 / 示例动画 | 类型：动画 / 奇幻 | 演员：甲乙丙 / 丁戊己", timedFree.Subtitle)
	require.False(t, timedFree.DiscountEndTime.IsZero(), "timed promotion should expose an end time")
	assert.Equal(t, "2026-08-06 12:34:56", timedFree.DiscountEndTime.Format("2006-01-02 15:04:05"))
	assert.Equal(t, int64(13249974108), timedFree.SizeBytes) // 12.34 GB
	assert.Equal(t, 34, timedFree.Seeders)
	assert.Equal(t, 2, timedFree.Leechers)
	assert.Equal(t, 56, timedFree.Snatched)

	plain := items[2]
	assert.Equal(t, "20003", plain.ID)
	assert.Equal(t, v2.DiscountNone, plain.DiscountLevel, "[熱門] 不是促销标记")
	assert.Equal(t, "範例電影", plain.Subtitle)
	assert.True(t, plain.DiscountEndTime.IsZero(), "審核狀態 span[title] 不能被当成促销结束时间")
	assert.Equal(t, 5, plain.Seeders)
	assert.Equal(t, 0, plain.Leechers)
	assert.Equal(t, 9, plain.Snatched)
}

// --- Suite: Detail ---

func testPtzoneDetail(t *testing.T) {
	def := getPtzoneDef(t)

	t.Run("TwoUpFree", func(t *testing.T) {
		doc := FixtureDoc(t, "ptzone_detail", ptzoneDetailFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "20001", info.TorrentID)
		assert.Equal(t, "Sample.Guide.PT.V1.0.PDF", info.Title)
		// 促销标记是 <font class='twoupfree' >（单引号 + 尾随空格），不是普通 free
		assert.Equal(t, v2.Discount2xFree, info.DiscountLevel)
		// 该页无「剩餘時間」，且 h1#top 末尾的 <span title="通過"> 不能被当成结束时间
		assert.True(t, info.DiscountEnd.IsZero(), "審核狀態 span[title] 不能被解析成结束时间")
		// 繁体「基本資訊」标签列 + 全角冒号「大小：」+ MB 为 ParseSizeMB 的基准单位
		assert.InDelta(t, 2.26, info.SizeMB, 0.001)
		assert.False(t, info.HasHR)
	})

	t.Run("TimedFreeWithHR", func(t *testing.T) {
		doc := FixtureDoc(t, "ptzone_detail_free_timed", ptzoneDetailFreeTimedFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "20002", info.TorrentID)
		assert.Equal(t, "Sample.Anime.2025.2160p.WEB-DL.H.265.DDP5.1-DEMO", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel, "font.hot 未映射，应继续查找 font.free")
		require.False(t, info.DiscountEnd.IsZero(), "discount end time should be parsed")
		assert.Equal(t, "2026-08-06 12:34:56", info.DiscountEnd.Format("2006-01-02 15:04:05"))
		assert.InDelta(t, 8704.0, info.SizeMB, 0.1) // 8.50 GB → 8.50*1024 MB
		assert.True(t, info.HasHR, "should detect HR from hit_run.gif")
	})
}

// --- Suite: UserInfo ---

func testPtzoneUserInfo(t *testing.T) {
	def := getPtzoneDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "ptzone_index", ptzoneIndexFixture)
		fields := map[string]string{
			"id":       "99001",
			"name":     "ptuser01",
			"seeding":  "42",
			"leeching": "3",
			"bonus":    "123456.7",
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
		doc := FixtureDoc(t, "ptzone_userdetails", ptzoneUserdetailsFixture)
		fields := map[string]string{
			"uploaded":       "322122547200",
			"downloaded":     "128849018880",
			"ratio":          "2.5",
			"trueUploaded":   "53687091200",
			"trueDownloaded": "1649267441664",
			"levelName":      "Power User",
			"joinTime":       "1709499967",
			"lastAccessAt":   "1785373687",
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
func TestPtzone_UserInfo_LastAccessParsed(t *testing.T) {
	def := getPtzoneDef(t)
	driver := newTestNexusPHPDriver(def)
	doc := FixtureDoc(t, "ptzone_userdetails", ptzoneUserdetailsFixture)

	sel, ok := def.UserInfo.Selectors["lastAccessAt"]
	require.True(t, ok, "lastAccessAt selector is required for the login-state probe")

	raw := driver.ExtractFieldValuePublic(doc, sel)
	ts, err := strconv.ParseInt(raw, 10, 64)
	require.NoError(t, err, "lastAccessAt should be a unix timestamp, got %q", raw)
	assert.Greater(t, ts, int64(0), "lastAccessAt must be > 0")
}

func TestPtzone_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":           ptzoneSearchFixture,
		"detail":           ptzoneDetailFixture,
		"detail_freetimed": ptzoneDetailFreeTimedFixture,
		"index":            ptzoneIndexFixture,
		"userdetails":      ptzoneUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
