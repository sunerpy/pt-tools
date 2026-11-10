package definitions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func init() {
	RegisterFixtureSuite(FixtureSuite{
		SiteID:   "oshenpt",
		Search:   testOshenptSearch,
		Detail:   testOshenptDetail,
		UserInfo: testOshenptUserInfo,
	})
}

// --- Fixtures ---
//
// 结构照搬 issue #882 附带的真实页面（CHD/scenetorrents 分类图标皮肤），标题、副标题、种子 ID、
// 用户名、用户 ID、海报地址、IMDb 编号与统计数值都换成了虚构值。
//
// 搜索页覆盖的真实特征：
//   - 名称子表依次为：46x46 海报缩略图 / 名称 / IMDb 评分 / 下载按钮
//   - 新种在标题后面带 <b> (<font class='new'>新</font>)</b>
//   - 限时免费的剩余时间同时在促销图标的 onmouseover 提示与可见的 <font color='#0000FF'> 里
//   - 副标题是 <br /> 之后的裸文本，前面可能有几个 <span title="">标签</span>，后面可能跟进度条 <div>

const oshenptSearchFixture = `<html><body>
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
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=401"><img class="c_movie" src="pic/cattrans.gif" alt="电影" title="电影" style="background-image: url(pic/category/chd/scenetorrents/chs/catsprites.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="pic/attachments/fixture-poster-1.jpg" class="nexus-lazy-load" style="max-height: 46px;max-width: 46px" /></td><td class="embedded" style='padding-left: 5px'><a title="Fixture.Action.2026.UHD.BluRay.2160p.HDR.x265.10bit.TrueHD.7.1-FIXTURE" href="details.php?id=99401&amp;hit=1"><b>Fixture.Action.2026.UHD.BluRay.2160p.HDR.x265.10bit.TrueHD.7.1-FIXTURE</b></a> <img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-07-03 21:00:00&quot;&gt;1月29天&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000,'fade','both','styleClass','niceTitle', 'fadeMax',87, 'maxWidth', 300);" /> <font color='#0000FF'>剩余时间：<span title="2026-07-03 21:00:00">1月29天</span></font><br /><span style="background-color:#006400;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">中字</span><span style="background-color:#38b03f;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">HDR</span>虚构动作片 4K 2160p UHD 压制 简繁双语 主演：示例演员 | FIXTURE<div style="padding: 1px;margin-top: 2px;border: 1px solid #838383" title="leeching 0%"><div style="width: 0%;background-color: blue;height: 2px"></div></div></td><td class="embedded" style="text-align: right; width: 40px;padding: 4px"><div style="display: flex;flex-direction: column"><div style="display: flex;align-content: center;justify-content: space-between;padding: 2px 0"><img src="pic/imdb2.png" alt="imdb" title="imdb" style="max-width: 16px;max-height: 16px"/><span data-imdbid="tt0000001">N/A</span></div></div></td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=99401"><img class="download" src="pic/trans.gif" style='padding-bottom: 2px;' alt="download" title="下载本种" /></a><br /><a id="bookmark0" href="javascript: bookmark(99401,0);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=99401&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-07-02 09:10:11">2时<br />5分钟</span></td><td class="rowfollow">15.78<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99401&amp;hit=1&amp;dllist=1#seeders">5</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=99401&amp;hit=1&amp;dllist=1#leechers">1</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=404"><img class="c_doc" src="pic/cattrans.gif" alt="纪录片" title="纪录片" style="background-image: url(pic/category/chd/scenetorrents/chs/catsprites.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="pic/attachments/fixture-poster-2.jpg" class="nexus-lazy-load" style="max-height: 46px;max-width: 46px" /></td><td class="embedded" style='padding-left: 5px'><a title="Fixture.Documentary.2014.1080p.WEB-DL.x265.AC3.2.0-FIXTURE" href="details.php?id=99402&amp;hit=1"><b>Fixture.Documentary.2014.1080p.WEB-DL.x265.AC3.2.0-FIXTURE</b></a><b> (<font class='new'>新</font>)</b><br /><span style="background-color:#6a3906;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">国语</span><span style="background-color:#006400;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">中字</span><span style="background-color:#38b03f;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">HDR</span>[Checked by fixture] 【虚构纪录片】 国语/中英硬字幕 HEVC版本</td><td class="embedded" style="text-align: right; width: 40px;padding: 4px"><div style="display: flex;flex-direction: column"><div style="display: flex;align-content: center;justify-content: space-between;padding: 2px 0"><img src="pic/imdb2.png" alt="imdb" title="imdb" style="max-width: 16px;max-height: 16px"/><span data-imdbid="tt0000002">N/A</span></div></div></td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=99402"><img class="download" src="pic/trans.gif" style='padding-bottom: 2px;' alt="download" title="下载本种" /></a><br /><a id="bookmark1" href="javascript: bookmark(99402,1);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=99402&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-07-02 05:22:43">1时<br />57分钟</span></td><td class="rowfollow">2.73<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99402&amp;hit=1&amp;dllist=1#seeders">9</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow">0</td>
<td class="rowfollow"><span class="nowrap"><a href="https://www.oshen.win/userdetails.php?id=90009" class='PowerUser_Name'><b>fixture_uploader</b></a></span></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=402"><img class="c_tv" src="pic/cattrans.gif" alt="剧集" title="剧集" style="background-image: url(pic/category/chd/scenetorrents/chs/catsprites.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="pic/attachments/fixture-poster-3.jpg" class="nexus-lazy-load" style="max-height: 46px;max-width: 46px" /></td><td class="embedded" style='padding-left: 5px'><a title="Fixture.Drama.S01.2026.1080p.WEB-DL.H264.AAC-FIXTURE" href="details.php?id=99403&amp;hit=1"><b>Fixture.Drama.S01.2026.1080p.WEB-DL.H264.AAC-FIXTURE</b></a> <img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-07-05 07:08:09&quot;&gt;3天10时&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000,'fade','both','styleClass','niceTitle', 'fadeMax',87, 'maxWidth', 300);" /> <font color='#0000FF'>剩余时间：<span title="2026-07-05 07:08:09">3天10时</span></font><br />虚构剧集 全12集 &amp; 特别篇</td><td class="embedded" style="text-align: right; width: 40px;padding: 4px"><div style="display: flex;flex-direction: column"><div style="display: flex;align-content: center;justify-content: space-between;padding: 2px 0"><img src="pic/imdb2.png" alt="imdb" title="imdb" style="max-width: 16px;max-height: 16px"/><span data-imdbid="tt0000003">N/A</span></div></div></td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=99403"><img class="download" src="pic/trans.gif" style='padding-bottom: 2px;' alt="download" title="下载本种" /></a><br /><a id="bookmark2" href="javascript: bookmark(99403,2);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><b><a href="details.php?id=99403&amp;hit=1&amp;cmtpage=1#startcomments" >2</a></b></td><td class="rowfollow nowrap"><span title="2026-07-01 21:00:00">1天<br />12时</span></td><td class="rowfollow">5.66<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99403&amp;hit=1&amp;dllist=1#seeders">17</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=99403&amp;hit=1&amp;dllist=1#leechers">2</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=99403"><b>31</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
</table>
</body></html>`

// 详情页（限时免费）：标题 / ID 取自隐藏域，h1 的剩余时间在无 class 的 <font color='#0000FF'> 里。
const oshenptDetailFixture = `<html><body>
<h1 align="center" id="top">Fixture.Action.2026.UHD.BluRay.2160p.HDR.x265.10bit.TrueHD.7.1-FIXTURE&nbsp;&nbsp;&nbsp; <b>[<font class='free' >免费</font>]</b> <font color='#0000FF'>剩余时间：<span title="2026-07-03 21:00:00">1月29天</span></font></h1>
<table class="main" width="100%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead" width="13%">下载</td><td class="rowfollow" width="87%" align="left"><a class="index" href="download.php?id=99401">[OshenPT].Fixture.Action.2026.UHD.BluRay.2160p.HDR.x265.10bit.TrueHD.7.1-FIXTURE.torrent</a>&nbsp;&nbsp;&nbsp;由&nbsp;<i>匿名</i>发布于<span title="2026-07-02 09:10:11">2时5分钟前</span></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">虚构动作片 4K 2160p UHD 压制 简繁双语 主演：示例演员 | FIXTURE</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>15.78 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;电影&nbsp;&nbsp;&nbsp;<b>媒介: </b>Encode&nbsp;&nbsp;&nbsp;<b>编码: </b>H.265/HEVC&nbsp;&nbsp;&nbsp;<b>分辨率: </b>4K/UHD&nbsp;&nbsp;&nbsp;<b>制作组: </b>Other</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">魔力值奖励</td><td class="rowfollow" valign="top" align="left">0</td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.Action.2026.UHD.BluRay.2160p.HDR.x265.10bit.TrueHD.7.1-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="99401" /></form></td></tr>
</table>
</body></html>`

// 详情页（无促销）：h1 只有标题。
const oshenptDetailPlainFixture = `<html><body>
<h1 align="center" id="top">Fixture.Documentary.2014.1080p.WEB-DL.x265.AC3.2.0-FIXTURE</h1>
<table class="main" width="100%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead" width="13%">下载</td><td class="rowfollow" width="87%" align="left"><a class="index" href="download.php?id=99402">[OshenPT].Fixture.Documentary.2014.1080p.WEB-DL.x265.AC3.2.0-FIXTURE.torrent</a>&nbsp;&nbsp;&nbsp;由&nbsp;<span class="nowrap"><a href="https://www.oshen.win/userdetails.php?id=90009" class='PowerUser_Name'><b>fixture_uploader</b></a></span>发布于<span title="2026-07-02 05:22:43">1时57分钟前</span></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">[Checked by fixture] 【虚构纪录片】 国语/中英硬字幕 HEVC版本</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>2.73 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;纪录片&nbsp;&nbsp;&nbsp;<b>媒介: </b>Encode</td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.Documentary.2014.1080p.WEB-DL.x265.AC3.2.0-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="99402" /></form></td></tr>
</table>
</body></html>`

// index.php：#info_block 里的用户链接是绝对地址，没有 H&R 计数器。
const oshenptIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr>
	<td><table width="100%" cellspacing="0" cellpadding="0" border="0"><tr>
		<td class="bottom" align="left">
            <span class="medium">
                欢迎回来, <span class="nowrap"><a href="https://www.oshen.win/userdetails.php?id=90001" class='PowerUser_Name'><b>fixture_user</b></a></span>                [<a href="logout.php">退出</a>]
                [<a href="usercp.php">控制面板</a>]
                [<a href="torrents.php?inclbookmarked=1&amp;allsec=1&amp;incldead=0">收藏</a>]
                <font class = 'color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 1,234,567.8                 <a href="attendance.php" class="">[签到已得100, 补签卡: 1]</a>                <a href="medal.php">[勋章]</a>
                <a href="task.php">[任务]</a>
                <font class = 'color_invite'>邀请 </font>[<a href="invite.php?id=90001">发送</a>]: 0(0)                                <br />
	            <font class="color_ratio">分享率:</font> 1.323                <font class='color_uploaded'>上传量:</font> 860.20 GB                <font class='color_downloaded'> 下载量:</font> 650.25 GB                <font class='color_active'>当前活动:</font> <img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />45  <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />3&nbsp;&nbsp;
                <font class='color_connectable'>可连接:</font><b><font color="green">是</font></b>                <font class='color_slots'>连接数：</font>无限制            </span>
        </td>
	</tr></table></td>
</tr></table>
</body></html>`

// userdetails.php：「传输」行是内嵌小表格，魔力值与做种积分各占一行。
const oshenptUserdetailsFixture = `<html><body>
<table class="main" width="100%" border="1" cellspacing="0" cellpadding="5">
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">用户ID/UID</td><td width="99%" class="rowfollow" valign="top" align="left">90001</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">邀请人</td><td width="99%" class="rowfollow" valign="top" align="left"><i>匿名</i></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">加入日期</td><td width="99%" class="rowfollow" valign="top" align="left">2025-02-03 04:05:06 (<span title="2025-02-03 04:05:06">1年5月前</span>, 73.4周)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-07-03 21:00:00 (<span title="2026-07-03 21:00:00">&lt; 1分钟前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">传输</td><td width="99%" class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>: <font color="">1.323</font>（<strong>实际分享率</strong>：0.453）</td><td class="embedded">&nbsp;&nbsp;<img src="pic/smilies/1.gif" alt="" /></td></tr><tr><td class="embedded"><strong>上传量</strong>: 860.20 GB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>: 650.25 GB</td></tr><tr><td class="embedded"><strong>实际上传量</strong>: 380.50 GB</td><td class="embedded">&nbsp;&nbsp;<strong>实际下载量</strong>: 840.10 GB</td><td class="embedded text-muted">&nbsp;&nbsp;实际上传/下载量 (仅用于记录, 不参与分享率计算)</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">等级</td><td width="99%" class="rowfollow" valign="top" align="left"><img alt="Power User" title="Power User" src="pic/power.gif" /> </td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">魔力值</td><td width="99%" class="rowfollow" valign="top" align="left">1,234,567.8&nbsp;&nbsp;<a href="bonus-log.php?uid=90001" target="_blank" class="altlink">[明细]</a></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">做种积分</td><td width="99%" class="rowfollow" valign="top" align="left">234,567.8&nbsp;&nbsp;<span class='text-muted'>(更新时间: 2026-07-03 06:00:00)</span></td></tr>
</table>
</body></html>`

// --- Helpers ---

func getOshenptDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("oshenpt")
	require.True(t, ok, "oshenpt definition not found")
	return def
}

// --- Suite: Search ---

func testOshenptSearch(t *testing.T) {
	def := getOshenptDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(oshenptSearchFixture))
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

	free := items[0]
	assert.Equal(t, "99401", free.ID)
	assert.Equal(t, "Fixture.Action.2026.UHD.BluRay.2160p.HDR.x265.10bit.TrueHD.7.1-FIXTURE", free.Title)
	// 副标题前有两个标签，后面跟进度条 <div>
	assert.Equal(t, "虚构动作片 4K 2160p UHD 压制 简繁双语 主演：示例演员 | FIXTURE", free.Subtitle)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	require.False(t, free.DiscountEndTime.IsZero(), "discount end time should come from onmouseover tooltip")
	assert.Equal(t, "2026-07-03 21:00:00", free.DiscountEndTime.Format("2006-01-02 15:04:05"))
	assert.Equal(t, int64(16943645982), free.SizeBytes)
	assert.Equal(t, 5, free.Seeders)
	assert.Equal(t, 1, free.Leechers)
	assert.Equal(t, 0, free.Snatched)
	assert.Equal(t, "电影", free.Category)
	assert.Positive(t, free.UploadedAt)
	assert.False(t, free.HasHR)

	plain := items[1]
	assert.Equal(t, "99402", plain.ID)
	assert.Equal(t, "Fixture.Documentary.2014.1080p.WEB-DL.x265.AC3.2.0-FIXTURE", plain.Title)
	// 标题后面的「(新)」不进副标题；三个标签挡在副标题前面
	assert.Equal(t, "[Checked by fixture] 【虚构纪录片】 国语/中英硬字幕 HEVC版本", plain.Subtitle)
	assert.Equal(t, v2.DiscountNone, plain.DiscountLevel)
	assert.True(t, plain.DiscountEndTime.IsZero())
	assert.Equal(t, int64(2931315179), plain.SizeBytes)
	assert.Equal(t, 9, plain.Seeders)
	assert.Equal(t, 0, plain.Leechers)
	assert.Equal(t, 0, plain.Snatched)
	assert.Equal(t, "纪录片", plain.Category)

	noTags := items[2]
	assert.Equal(t, "99403", noTags.ID)
	// 没有标签时副标题紧跟在 <br /> 后面，&amp; 要还原成 &
	assert.Equal(t, "虚构剧集 全12集 & 特别篇", noTags.Subtitle)
	assert.Equal(t, v2.DiscountFree, noTags.DiscountLevel)
	assert.Equal(t, "2026-07-05 07:08:09", noTags.DiscountEndTime.Format("2006-01-02 15:04:05"))
	assert.Equal(t, int64(6077378723), noTags.SizeBytes)
	assert.Equal(t, 17, noTags.Seeders)
	assert.Equal(t, 2, noTags.Leechers)
	assert.Equal(t, 31, noTags.Snatched)
}

// --- Suite: Detail ---

func testOshenptDetail(t *testing.T) {
	def := getOshenptDef(t)

	t.Run("TimedFree", func(t *testing.T) {
		doc := FixtureDoc(t, "oshenpt_detail", oshenptDetailFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "99401", info.TorrentID)
		assert.Equal(t, "Fixture.Action.2026.UHD.BluRay.2160p.HDR.x265.10bit.TrueHD.7.1-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		require.False(t, info.DiscountEnd.IsZero(), "discount end time should be parsed")
		assert.Equal(t, "2026-07-03 21:00:00", info.DiscountEnd.Format("2006-01-02 15:04:05"))
		assert.Empty(t, info.DiscountErr)
		assert.InDelta(t, 15.78*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)

		assert.Equal(t, "download.php?id=99401",
			doc.Find(def.Selectors.DetailDownloadLink).First().AttrOr("href", ""))
		assert.Equal(t, "虚构动作片 4K 2160p UHD 压制 简繁双语 主演：示例演员 | FIXTURE",
			strings.TrimSpace(doc.Find(def.Selectors.DetailSubtitle).First().Text()))
	})

	t.Run("NoPromotion", func(t *testing.T) {
		doc := FixtureDoc(t, "oshenpt_detail_plain", oshenptDetailPlainFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "99402", info.TorrentID)
		assert.Equal(t, "Fixture.Documentary.2014.1080p.WEB-DL.x265.AC3.2.0-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel)
		assert.True(t, info.DiscountEnd.IsZero())
		assert.Empty(t, info.DiscountErr)
		assert.InDelta(t, 2.73*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})
}

// --- Suite: UserInfo ---

func testOshenptUserInfo(t *testing.T) {
	def := getOshenptDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "oshenpt_index", oshenptIndexFixture)
		fields := map[string]string{
			"id":       "90001",
			"name":     "fixture_user",
			"seeding":  "45",
			"leeching": "3",
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
		doc := FixtureDoc(t, "oshenpt_userdetails", oshenptUserdetailsFixture)
		fields := map[string]string{
			// 860.20 GB / 650.25 GB / 380.50 GB / 840.10 GB 按 1024 进制换算为字节
			"uploaded":       "923632717004",
			"downloaded":     "698200621056",
			"trueUploaded":   "408558764032",
			"trueDownloaded": "902050506342",
			"ratio":          "1.323",
			"levelName":      "Power User",
			// parseNumber 对百万以上的数输出科学计数法，与其他站点的测试一致
			"bonus":        "1.2345678e+06",
			"seedingBonus": "234567.8",
			"joinTime":     "1738526706",
			"lastAccessAt": "1783083600",
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

func TestOshenpt_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":       oshenptSearchFixture,
		"detail":       oshenptDetailFixture,
		"detail_plain": oshenptDetailPlainFixture,
		"index":        oshenptIndexFixture,
		"userdetails":  oshenptUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
