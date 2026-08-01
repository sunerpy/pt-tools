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
		SiteID:   "railgunpt",
		Search:   testRailgunptSearch,
		Detail:   testRailgunptDetail,
		UserInfo: testRailgunptUserInfo,
	})
}

// --- Fixtures ---
//
// 结构照搬真实抓取页面（CHD/scenetorrents 分类图标皮肤），但所有标题、副标题、种子 ID、
// 用户名、用户 ID、图床地址均已替换为虚构值。
//
// 搜索页需要覆盖的真实特征：
//   - 名称子表第一个 td.embedded 是 46x46 海报缩略图，真正的名称单元格排在它后面
//   - 名称单元格前缀可能带多个 img.sticky 与 <b>[热门]</b>
//   - 促销剩余时间同时出现在促销图标的 onmouseover 提示与可见的「剩余时间：<span title>」中，
//     后者有时包在 <font color>里、有时是裸文本
//   - 副标题是 <br /> 之后的裸文本节点，前面可能有 1~3 个 <span title="">徽章</span>
//   - 存在只有徽章、没有真实副标题的行，以及完全没有 <br /> 的行
//   - 大小列是 "2.17<br />GB"（数字与单位被 <br /> 分开）

const railgunptSearchFixture = `<html><body>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%">
<tr>
<td class="colhead" style="padding: 0px">类型</td>
<td class="colhead"><a href="?sort=1&amp;type=asc">标题</a></td>
<td class="colhead"><a href="?sort=3&amp;type=desc"><img class="comments" src="pic/trans.gif" alt="comments" title="评论数" /></a></td>
<td class="colhead"><a href="?sort=4&amp;type=desc"><img class="time" src="pic/trans.gif" alt="time" title="存活时间" /></a></td>
<td class="colhead"><a href="?sort=5&amp;type=desc"><img class="size" src="pic/trans.gif" alt="size" title="大小" /></a></td>
<td class="colhead"><a href="?sort=7&amp;type=desc"><img class="seeders" src="pic/trans.gif" alt="seeders" title="种子数" /></a></td>
<td class="colhead"><a href="?sort=8&amp;type=desc"><img class="leechers" src="pic/trans.gif" alt="leechers" title="下载数" /></a></td>
<td class="colhead"><a href="?sort=6&amp;type=desc"><img class="snatched" src="pic/trans.gif" alt="snatched" title="完成数" /></a></td>
<td class="colhead"><a href="?sort=9&amp;type=desc">发布者</a></td>
</tr>
<tr style="background-color: #89c9e6">
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=410"><img class="c_tv" src="pic/cattrans.gif" alt="剧集" title="剧集" style="background-image: url(pic/category/chd/scenetorrents/chs/tv.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr style="background-color: #89c9e6"><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="pic/attachments/fixture-poster-1.jpg" class="nexus-lazy-load" style="max-height: 46px;max-width: 46px" /></td><td class="embedded" style='padding-left: 5px'><img class="sticky" src="pic/trans.gif" alt="Sticky" title="一级置顶" />&nbsp;<a title="Fixture.Show.2026.S01E09.1080p.WEB-DL.AVC.AAC-FIXTURE"  href="details.php?id=99001&amp;hit=1"><b>Fixture.Show.2026.S01E09.1080p.WEB-DL.AVC.AAC-FIXTURE</b></a> <b>[<font class='hot'>热门</font>]</b> <img class="pro_free2up" src="pic/trans.gif" alt="2X Free"  onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;twoupfree&quot;&gt;2X免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-08-26 14:35:13&quot;&gt;29天23时&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000,'fade','both','styleClass','niceTitle', 'fadeMax',87, 'maxWidth', 300);" /> <font color='#00CC66'>剩余时间：<span title="2026-08-26 14:35:13">29天23时</span></font><br /><span style="background-color:#0000ff;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">官方</span><span style="background-color:#006400;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">中字</span>虚构副标题一 / Fixture Subtitle One<div style="padding: 1px;margin-top: 2px;border: 1px solid #838383" title="inactivity 0%"><div style="width: 0%;background-color: #aaa;height: 2px"></div></div></td><td class="embedded" style="text-align: right; width: 40px;padding: 4px"><div style="display: flex;flex-direction: column"><div><img src="pic/imdb2.png" alt="imdb" title="imdb" style="max-width: 16px;max-height: 16px"/><span>N/A</span></div><div><img src="pic/douban2.png" alt="douban" title="douban" style="max-width: 16px;max-height: 16px"/><span>N/A</span></div></div></td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=99001"><img class="download" src="pic/trans.gif" style='padding-bottom: 2px;' alt="download" title="下载本种" /></a><br /><a id="bookmark0"  href="javascript: bookmark(99001,0);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><b><a href="details.php?id=99001&amp;hit=1&amp;cmtpage=1#startcomments" >4</a></b></td><td class="rowfollow nowrap"><span title="2026-07-27 14:35:13">39<br />分</span></td><td class="rowfollow">2.17<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99001&amp;hit=1&amp;dllist=1#seeders">61</a></b></td>
<td class="rowfollow">3</td>
<td class="rowfollow"><a href="viewsnatches.php?id=99001"><b>87</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=408"><img class="c_hqaudio" src="pic/cattrans.gif" alt="音乐" title="音乐" style="background-image: url(pic/category/chd/scenetorrents/chs/hqaudio.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="pic/attachments/fixture-poster-2.jpg" class="nexus-lazy-load" style="max-height: 46px;max-width: 46px" /></td><td class="embedded" style='padding-left: 5px'><a title="Fixture.Album.2026.FLAC-FIXTURE"  href="details.php?id=99002&amp;hit=1"><b>Fixture.Album.2026.FLAC-FIXTURE</b></a> <b>[<font class='hot'>热门</font>]</b></td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=99002"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td>
</tr></table></td><td class="rowfollow"><b><a href="details.php?id=99002&amp;hit=1&amp;cmtpage=1#startcomments" >1</a></b></td><td class="rowfollow nowrap"><span title="2025-03-27 14:46:44">1年<br />4月</span></td><td class="rowfollow">172.65<br />MB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99002&amp;hit=1&amp;dllist=1#seeders">146</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><a href="viewsnatches.php?id=99002"><b>781</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=401"><img class="c_movie" src="pic/cattrans.gif" alt="电影" title="电影" style="background-image: url(pic/category/chd/scenetorrents/chs/movie.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="pic/attachments/fixture-poster-3.jpg" class="nexus-lazy-load" style="max-height: 46px;max-width: 46px" /></td><td class="embedded" style='padding-left: 5px'><a title="Fixture.Movie.2026.2160p.UHD.BluRay-FIXTURE"  href="details.php?id=99003&amp;hit=1"><b>Fixture.Movie.2026.2160p.UHD.BluRay-FIXTURE</b></a> <img class="pro_50pctdown" src="pic/trans.gif" alt="50%"  onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;halfdown&quot;&gt;50%&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-12-24 14:17:21&quot;&gt;4月29天&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000,'fade','both','styleClass','niceTitle', 'fadeMax',87, 'maxWidth', 300);" /> 剩余时间：<span title="2026-12-24 14:17:21">4月29天</span><br /><span style="background-color:#6a3906;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">国语</span><div style="padding: 1px;margin-top: 2px;border: 1px solid #838383" title="inactivity 0%"><div style="width: 0%;background-color: #aaa;height: 2px"></div></div></td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=99003"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td>
</tr></table></td><td class="rowfollow"><b><a href="details.php?id=99003&amp;hit=1&amp;cmtpage=1#startcomments" >0</a></b></td><td class="rowfollow nowrap"><span title="2026-07-25 09:12:00">2天<br />5时</span></td><td class="rowfollow">34.50<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99003&amp;hit=1&amp;dllist=1#seeders">9</a></b></td>
<td class="rowfollow">12</td>
<td class="rowfollow"><a href="viewsnatches.php?id=99003"><b>4</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
</table>
</body></html>`

// 详情页：标题 / ID 取自 <form> 内的隐藏域（本站保留），h1 的促销标记是
// <font class='twoupfree' >（注意单引号 + 尾随空格），剩余时间在无 class 的 <font color> 里；
// 「基本信息」行使用全角冒号 + 十进制单位。
const railgunptDetailFixture = `<html><body>
<h1 align="center" id="top">Fixture.Show.2026.S01E09.1080p.WEB-DL.AVC.AAC-FIXTURE&nbsp;&nbsp;&nbsp; <b>[<font class='twoupfree' >2X免费</font>]</b> <font color='#00CC66'>剩余时间：<span title="2026-08-26 14:35:13">29天23时</span></font></h1>
<table class="main" width="100%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead" width="13%">下载</td><td class="rowfollow" width="87%" align="left"><a class="index" href="download.php?id=99001">[RailgunPT].Fixture.Show.2026.S01E09.1080p.WEB-DL.AVC.AAC-FIXTURE.torrent</a>&nbsp;&nbsp;&nbsp;由&nbsp;<i>匿名</i>发布于<span title="2026-07-27 14:35:13">39分钟前</span></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">虚构副标题一 / Fixture Subtitle One</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>2.17 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;剧集&nbsp;&nbsp;&nbsp;<b>媒介: </b>WEB-DL&nbsp;&nbsp;&nbsp;<b>编码: </b>H264&nbsp;&nbsp;&nbsp;<b>分辨率: </b>1080p/i&nbsp;&nbsp;&nbsp;<b>音频: </b>Other</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">行为</td><td class="rowfollow" valign="top" align="left"><a title="下载种子" href="download.php?id=99001"><img class="dt_download" src="pic/trans.gif" alt="download" />&nbsp;<b><font class="small">下载种子</font></b></a></td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><table border="0" cellspacing="0"><tr><td class="embedded"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.Show.2026.S01E09.1080p.WEB-DL.AVC.AAC-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="99001" /></form></td></tr></table></td></tr>
</table>
</body></html>`

// 无促销 + IEC 单位（GiB）的详情页：本站「基本信息」行偶尔使用二进制单位，
// 用于验证 SizeRegex 的单位组同时接受 GB 与 GiB。
const railgunptDetailIECFixture = `<html><body>
<h1 align="center" id="top">Fixture.Movie.2026.2160p.UHD.BluRay-FIXTURE</h1>
<table class="main" width="100%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>34.50 GiB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;电影&nbsp;&nbsp;&nbsp;<b>媒介: </b>Blu-ray</td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.Movie.2026.2160p.UHD.BluRay-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="99003" /></form></td></tr>
</table>
</body></html>`

// index.php 顶部 #info_block：id / name / 魔力值 / 做种下载数均取自这里。
// 注意用户链接是绝对地址，且 class 属性带空格（class = 'color_bonus'），与真实页面一致。
const railgunptIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr>
	<td><table width="100%" cellspacing="0" cellpadding="0" border="0"><tr>
		<td class="bottom" align="left">
            <span class="medium">
                欢迎回来, <span class="nowrap"><a  href="https://bilibili.download/userdetails.php?id=90001" class='User_Name'><b>fixture_user</b></a><img src="pic/medals/fixture-medal.png" title="虚构勋章" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"/></span>                [<a href="logout.php">退出</a>]
                [<a href="usercp.php">&nbsp;控制面板&nbsp;</a>]
                <font class = 'color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 64,077.7                 <a href="attendance.php" class="">[签到已得170, 补签卡: 0]</a>                <a href="medal.php">[勋章]</a>
                <font class = 'color_invite'>邀请 </font>[<a href="invite.php?id=90001">发送</a>]: 5(0)                                <br />
	            <font class="color_ratio">分享率:</font> 11.000                <font class='color_uploaded'>上传量:</font> 110.00 GB                <font class='color_downloaded'> 下载量:</font> 10.00 GB                <font class='color_active'>当前活动:</font> <img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />14  <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />0&nbsp;&nbsp;
                <font class='color_connectable'>可连接:</font><b><font color="green">是</font></b>            </span>
        </td>
	</tr></table></td>
</tr></table>
</body></html>`

// userdetails.php 正文表格：「传输」行是内嵌小表格，上传/下载量与「实际上传量 / 实际下载量」
// 并列，分享率后面还跟着「（实际分享率：x）」；本站只有一个「最近动向」行。
const railgunptUserdetailsFixture = `<html><body>
<h1 style='margin:0px'><span class="nowrap"><b>fixture_target</b></span><img src="pic/flag/china.gif" alt="中国" style='margin-left: 8pt' /></h1>
<table class="main" width="100%" border="1" cellspacing="0" cellpadding="5">
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">用户ID/UID</td><td width="99%" class="rowfollow" valign="top" align="left">90002</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">邀请</td><td width="99%" class="rowfollow" valign="top" align="left">没有邀请资格</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">加入日期</td><td width="99%" class="rowfollow" valign="top" align="left">2025-04-29 16:04:51 (<span title="2025-04-29 16:04:51">1年3月前</span>, 64周)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-07-27 14:35:13 (<span title="2026-07-27 14:35:13">39分钟前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">BT客户端</td><td width="99%" class="rowfollow" valign="top" align="left">qBittorrent/5.1.2</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">传输</td><td width="99%" class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>:  <font color="">31.453</font>（<strong>实际分享率</strong>：2.541）</td><td class="embedded">&nbsp;&nbsp;<img src="pic/smilies/163.gif" alt="" /></td></tr><tr><td class="embedded"><strong>上传量</strong>:  130.393 TB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>:  4.146 TB</td></tr><tr><td class="embedded"><strong>实际上传量</strong>:  78.269 TB</td><td class="embedded">&nbsp;&nbsp;<strong>实际下载量</strong>:  30.801 TB</td><td class="embedded text-muted">&nbsp;&nbsp;实际上传/下载量 (仅用于记录, 不参与分享率计算)</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">BT时间</td><td width="99%" class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>做种时间</strong>:  420天02:47:36</td><td class="embedded">&nbsp;&nbsp;<strong>下载时间</strong>:  33天17:58:19</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">性别</td><td width="99%" class="rowfollow" valign="top" align="left">保密</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">等级</td><td width="99%" class="rowfollow" valign="top" align="left"><img alt="Nexus Master" title="Nexus Master" src="pic/nexus.gif" /> </td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">种子评论</td><td width="99%" class="rowfollow" valign="top" align="left">0</td></tr>
</table>
</body></html>`

// --- Helpers ---

func getRailgunptDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("railgunpt")
	require.True(t, ok, "railgunpt definition not found")
	return def
}

// --- Suite: Search ---

func testRailgunptSearch(t *testing.T) {
	def := getRailgunptDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(railgunptSearchFixture))
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
	require.Len(t, items, 3, "should parse 3 torrent rows")

	twoUpFree := items[0]
	assert.Equal(t, "99001", twoUpFree.ID)
	assert.Equal(t, "Fixture.Show.2026.S01E09.1080p.WEB-DL.AVC.AAC-FIXTURE", twoUpFree.Title)
	// 副标题在 <br /> 之后，且被两个 <span title=""> 徽章挡在前面，必须靠 SubtitleSelector 取到
	assert.Equal(t, "虚构副标题一 / Fixture Subtitle One", twoUpFree.Subtitle)
	assert.Equal(t, v2.Discount2xFree, twoUpFree.DiscountLevel)
	// 大小列为 "2.17<br />GB"，parseSize 去掉空白后按 1024 进制换算
	assert.Equal(t, int64(2330019758), twoUpFree.SizeBytes)
	assert.Equal(t, 61, twoUpFree.Seeders)
	assert.Equal(t, 3, twoUpFree.Leechers)
	assert.Equal(t, 87, twoUpFree.Snatched)
	assert.Equal(t, "剧集", twoUpFree.Category)
	// 未配置 DiscountEndTime 选择器，结束时间来自促销图标的 onmouseover 提示
	require.False(t, twoUpFree.DiscountEndTime.IsZero(), "discount end time should come from onmouseover tooltip")
	assert.Equal(t, "2026-08-26 14:35:13", twoUpFree.DiscountEndTime.Format("2006-01-02 15:04:05"))
	assert.Positive(t, twoUpFree.UploadedAt)

	plain := items[1]
	assert.Equal(t, "99002", plain.ID)
	assert.Equal(t, "Fixture.Album.2026.FLAC-FIXTURE", plain.Title)
	// 该行完全没有 <br />，副标题为空
	assert.Empty(t, plain.Subtitle)
	assert.Equal(t, v2.DiscountNone, plain.DiscountLevel)
	assert.True(t, plain.DiscountEndTime.IsZero(), "no discount -> no end time")
	assert.Equal(t, int64(181036646), plain.SizeBytes)
	assert.Equal(t, 146, plain.Seeders)
	assert.Equal(t, 0, plain.Leechers)
	assert.Equal(t, 781, plain.Snatched)

	halfDown := items[2]
	assert.Equal(t, "99003", halfDown.ID)
	assert.Equal(t, "Fixture.Movie.2026.2160p.UHD.BluRay-FIXTURE", halfDown.Title)
	// 只有徽章 span、没有真实副标题的行不能把徽章文字当成副标题
	assert.Empty(t, halfDown.Subtitle)
	assert.Equal(t, v2.DiscountPercent50, halfDown.DiscountLevel)
	assert.Equal(t, "2026-12-24 14:17:21", halfDown.DiscountEndTime.Format("2006-01-02 15:04:05"))
	assert.Equal(t, int64(37044092928), halfDown.SizeBytes)
	assert.Equal(t, 9, halfDown.Seeders)
	assert.Equal(t, 12, halfDown.Leechers)
	assert.Equal(t, 4, halfDown.Snatched)
	assert.Equal(t, "电影", halfDown.Category)
}

// --- Suite: Detail ---

func testRailgunptDetail(t *testing.T) {
	def := getRailgunptDef(t)

	t.Run("TwoUpFree", func(t *testing.T) {
		doc := FixtureDoc(t, "railgunpt_detail", railgunptDetailFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "99001", info.TorrentID)
		assert.Equal(t, "Fixture.Show.2026.S01E09.1080p.WEB-DL.AVC.AAC-FIXTURE", info.Title)
		// h1 里的促销 class 是 twoupfree（不是 free），必须映射为 2X免费
		assert.Equal(t, v2.Discount2xFree, info.DiscountLevel)
		require.False(t, info.DiscountEnd.IsZero(), "discount end time should be parsed")
		assert.Equal(t, "2026-08-26 14:35:13", info.DiscountEnd.Format("2006-01-02 15:04:05"))
		// 「基本信息」标签单元格 + .Next() 兄弟节点，全角冒号 + 十进制 GB
		assert.InDelta(t, 2.17*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR, "站点无 H&R")
	})

	t.Run("IECUnitNoDiscount", func(t *testing.T) {
		doc := FixtureDoc(t, "railgunpt_detail_iec", railgunptDetailIECFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "99003", info.TorrentID)
		assert.Equal(t, "Fixture.Movie.2026.2160p.UHD.BluRay-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel)
		assert.True(t, info.DiscountEnd.IsZero())
		// 单位组同时接受 GiB
		assert.InDelta(t, 34.5*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})
}

// --- Suite: UserInfo ---

func testRailgunptUserInfo(t *testing.T) {
	def := getRailgunptDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "railgunpt_index", railgunptIndexFixture)
		fields := map[string]string{
			// 用户链接是绝对地址，querystring 过滤器仍能取到 id
			"id":       "90001",
			"name":     "fixture_user",
			"seeding":  "14",
			"leeching": "0",
			"bonus":    "64077.7",
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
		doc := FixtureDoc(t, "railgunpt_userdetails", railgunptUserdetailsFixture)
		fields := map[string]string{
			// 130.393 TB / 4.146 TB / 78.269 TB / 30.801 TB 按 1024 进制换算为字节
			"uploaded":       "143368619680595",
			"downloaded":     "4558575208759",
			"trueUploaded":   "86057675594399",
			"trueDownloaded": "33866057647128",
			// 分享率取 31.453，不能被后面的「（实际分享率：2.541）」抢走
			"ratio":     "31.453",
			"levelName": "Nexus Master",
			// parseTime 按站点 TimezoneOffset(+0800) 解析
			"joinTime":     "1745913891",
			"lastAccessAt": "1785134113",
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

func TestRailgunpt_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      railgunptSearchFixture,
		"detail":      railgunptDetailFixture,
		"detail_iec":  railgunptDetailIECFixture,
		"index":       railgunptIndexFixture,
		"userdetails": railgunptUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
