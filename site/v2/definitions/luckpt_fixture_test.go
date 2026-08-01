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
		SiteID:   "luckpt",
		Search:   testLuckptSearch,
		Detail:   testLuckptDetail,
		UserInfo: testLuckptUserInfo,
	})
}

// --- Fixtures ---
//
// 结构照搬真实采集页面，但所有标题、副标题、种子 ID、用户名、用户 ID 与图床地址均替换为虚构值。
//
// 搜索页需要覆盖的真实特征：
//   - 名称列第一个 td.embedded 是封面缩略图（真实页面 20/20 行都有 data-has-cover=1）
//   - 名称单元格前缀可能有多个 img.sticky、<b>(新)</b>、<b>[经典]</b>
//   - 促销结束时间同时出现在促销图标的 onmouseover 提示与
//     <font color='#0000FF'>剩余时间：<span title>…</span></font> 中
//   - 名称单元格里还有一个审核状态 <span title="通过">（含内嵌 svg），非促销行也存在，
//     因此促销结束时间选择器必须限定在 font:contains('剩余时间') 内
//   - 副标题是 <br /> 之后的裸文本，前面还有 1~3 个彩色标签 <span>（官方/中字/完结…）
//   - 大小列是 "46.11<br />GB"（数字与单位被 <br /> 分开）

const luckptApprovedIcon = `<span style="margin-left: 6px" title="通过"><svg t="1655145688503" class="icon" viewBox="0 0 1413 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" width="16" height="16"><path d="M1381.8 107.5L1274.7 0.4 465.3 809.9 142.6 487.2 35.4 594.2l430.1 430z" fill="#1afa29"></path></svg></span>`

const luckptTagSpans = `<span style="background-color:#0000ff;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">官方</span><span style="background-color:#006400;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">中字</span><span style="background-color:#f02498;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">完结</span>`

const luckptSearchFixture = `<html><body>
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
<tr class='nexus-sticky nexus-sticky--1' style="--nexus-torrent-tint: rgba(254, 215, 170, 0.56)">
<td class="rowfollow nowrap" data-col="type" data-label="类型" data-cat="c_anime" valign="middle" style='padding: 0px'><a href="?cat=405"><img class="c_anime" src="pic/cattrans.gif" alt="动画" title="动画" /></a></td>
<td class="rowfollow" data-col="name" data-label="标题" data-has-cover="1" data-cover-src="https://img.example.invalid/cover-a.webp" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style="text-align: center;width: 60px;height: 60px"><img src="pic/misc/spinner.svg" data-src="https://img.example.invalid/cover-a.webp" class="nexus-lazy-load" loading="lazy" width="60" height="60" alt="" /></td><td class="embedded" style='padding-left: 5px'><img class="sticky" src="pic/trans.gif" alt="Sticky" title="一级置顶" />&nbsp;<img class="sticky" src="pic/trans.gif" alt="Sticky" title="一级置顶" />&nbsp;<a title="Fixture.Anime.S02.2026.1080p.BluRay.Remux.AVC.TrueHD.5.1-FIXTURE"  href="details.php?id=88001&amp;hit=1"><b>Fixture.Anime.S02.2026.1080p.BluRay.Remux.AVC.TrueHD.5.1-FIXTURE</b></a><b> (<font class='new'>新</font>)</b> <b>[<font class='classic'>经典</font>]</b> <img class="pro_free" src="pic/trans.gif" alt="Free"  onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-08-03 10:42:47&quot;&gt;6天19时&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000);" /> <font color='#0000FF'>剩余时间：<span title="2026-08-03 10:42:47">6天19时</span></font>` + luckptApprovedIcon + `<br />` + luckptTagSpans + `虚构副标题一 [内封中字]</td><td class="embedded" style="text-align: right; width: 40px;padding: 4px"><img src="pic/imdb2.png" alt="imdb" title="imdb" /><span data-imdbid="tt0000001">N/A</span></td><td width="20" class="embedded" data-col="quick" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=88001"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a><br /><a id="bookmark0"  href="javascript: bookmark(88001,0);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow" data-col="comments" data-label="评论数"><a href="comment.php?action=add&amp;pid=88001&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap" data-col="time" data-label="存活时间"><span title="2026-07-27 10:42:47">4时<br />30分钟</span></td><td class="rowfollow" data-col="size" data-label="大小">46.11<br />GB</td><td class="rowfollow" data-col="seeders" data-label="种子数" align="center"><b><a href="details.php?id=88001&amp;hit=1&amp;dllist=1#seeders">9</a></b></td>
<td class="rowfollow" data-col="leechers" data-label="下载数">0</td>
<td class="rowfollow" data-col="snatched" data-label="完成数"><a href="viewsnatches.php?id=88001"><b>10</b></a></td>
<td class="rowfollow" data-col="uploader" data-label="发布者"><span class="nowrap"><a  href="https://pt.luckpt.de/userdetails.php?id=70002" class='SysOp_Name'><b>fixture_target</b></a></span></td>
</tr>
<tr>
<td class="rowfollow nowrap" data-col="type" data-label="类型" data-cat="c_anime" valign="middle" style='padding: 0px'><a href="?cat=405"><img class="c_anime" src="pic/cattrans.gif" alt="动画" title="动画" /></a></td>
<td class="rowfollow" data-col="name" data-label="标题" data-has-cover="1" data-cover-src="https://img.example.invalid/cover-b.webp" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style="text-align: center;width: 60px;height: 60px"><img src="pic/misc/spinner.svg" data-src="https://img.example.invalid/cover-b.webp" class="nexus-lazy-load" loading="lazy" width="60" height="60" alt="" /></td><td class="embedded" style='padding-left: 5px'><a title="Fixture.Anime.S00.2026.1080p.BluRay.Remux.AVC.LPCM.2.0-FIXTURE"  href="details.php?id=88002&amp;hit=1"><b>Fixture.Anime.S00.2026.1080p.BluRay.Remux.AVC.LPCM.2.0-FIXTURE</b></a> <img class="pro_free2up" src="pic/trans.gif" alt="Free 2X"  onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;twoupfree&quot;&gt;2X免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-08-01 16:17:11&quot;&gt;5天1时&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000);" /> <font color='#00CC66'>剩余时间：<span title="2026-08-01 16:17:11">5天1时</span></font>` + luckptApprovedIcon + `<br />` + luckptTagSpans + `虚构副标题二 [内封中字]</td><td width="20" class="embedded" data-col="quick" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=88002"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td>
</tr></table></td><td class="rowfollow" data-col="comments" data-label="评论数"><a href="comment.php?action=add&amp;pid=88002&amp;type=torrent" title="添加评论">2</a></td><td class="rowfollow nowrap" data-col="time" data-label="存活时间"><span title="2026-07-23 16:17:11">3天<br />22时</span></td><td class="rowfollow" data-col="size" data-label="大小">17.49<br />GB</td><td class="rowfollow" data-col="seeders" data-label="种子数" align="center"><b><a href="details.php?id=88002&amp;hit=1&amp;dllist=1#seeders">17</a></b></td>
<td class="rowfollow" data-col="leechers" data-label="下载数">0</td>
<td class="rowfollow" data-col="snatched" data-label="完成数"><a href="viewsnatches.php?id=88002"><b>37</b></a></td>
<td class="rowfollow" data-col="uploader" data-label="发布者"><i>匿名</i></td>
</tr>
<tr>
<td class="rowfollow nowrap" data-col="type" data-label="类型" data-cat="c_tvseries" valign="middle" style='padding: 0px'><a href="?cat=402"><img class="c_tvseries" src="pic/cattrans.gif" alt="电视剧" title="电视剧" /></a></td>
<td class="rowfollow" data-col="name" data-label="标题" data-has-cover="1" data-cover-src="https://img.example.invalid/cover-c.webp" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style="text-align: center;width: 60px;height: 60px"><img src="pic/misc/spinner.svg" data-src="https://img.example.invalid/cover-c.webp" class="nexus-lazy-load" loading="lazy" width="60" height="60" alt="" /></td><td class="embedded" style='padding-left: 5px'><a title="Fixture.Show.S01.2026.2160p.WEB-DL.HEVC.HDR10-FIXTURE"  href="details.php?id=88003&amp;hit=1"><b>Fixture.Show.S01.2026.2160p.WEB-DL.HEVC.HDR10-FIXTURE</b></a> <b>[<font class='classic'>经典</font>]</b>` + luckptApprovedIcon + `<br />` + luckptTagSpans + `虚构副标题三 | 全集 | 类型: 剧情</td><td width="20" class="embedded" data-col="quick" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=88003"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td>
</tr></table></td><td class="rowfollow" data-col="comments" data-label="评论数"><a href="comment.php?action=add&amp;pid=88003&amp;type=torrent" title="添加评论">5</a></td><td class="rowfollow nowrap" data-col="time" data-label="存活时间"><span title="2026-07-05 08:00:00">22天<br />6时</span></td><td class="rowfollow" data-col="size" data-label="大小">74.41<br />GB</td><td class="rowfollow" data-col="seeders" data-label="种子数" align="center"><b><a href="details.php?id=88003&amp;hit=1&amp;dllist=1#seeders">13</a></b></td>
<td class="rowfollow" data-col="leechers" data-label="下载数">0</td>
<td class="rowfollow" data-col="snatched" data-label="完成数"><a href="viewsnatches.php?id=88003"><b>24</b></a></td>
<td class="rowfollow" data-col="uploader" data-label="发布者"><i>匿名</i></td>
</tr>
</table>
</body></html>`

// 详情页：标题/ID 来自 <form> 内的隐藏域（本站保留），h1 中的促销标记为
// <font class='free' >（单引号 + 尾随空格），后面紧跟真实渲染的 剩余时间 span 与审核状态 span；
// 「基本信息」行使用全角冒号 + 十进制单位。
// 真实页面的「种子链接」行会渲染带 downhash 的一次性链接，属于凭据，fixture 中整行省略。
const luckptDetailFixture = `<html><body>
<h1 align="center" id="top">Fixture.Anime.S02.2026.1080p.BluRay.Remux.AVC.TrueHD.5.1-FIXTURE&nbsp;&nbsp;&nbsp; <b>[<font class='free' >免费</font>]</b> <font color='#0000FF'>剩余时间：<span title="2026-08-03 10:42:47">6天19时</span></font>` + luckptApprovedIcon + `</h1>
<table width="100%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead" width="13%">下载</td><td class="rowfollow" width="87%" align="left"><a class="index" href="download.php?id=88001">[LuckPT].Fixture.Anime.S02.torrent</a>&nbsp;&nbsp;&nbsp;由&nbsp;<span class="nowrap"><a  href="https://pt.luckpt.de/userdetails.php?id=70002" class='SysOp_Name'><b>fixture_target</b></a></span>发布于<span title="2026-07-27 10:42:47">4时30分钟前</span></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">虚构副标题一 [内封中字]</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">标签</td><td class="rowfollow" valign="top" align="left">` + luckptTagSpans + `</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>46.11 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;动画&nbsp;&nbsp;&nbsp;<b>媒介: </b>Remux&nbsp;&nbsp;&nbsp;<b>编码: </b>H.264/AVC&nbsp;&nbsp;&nbsp;<b>分辨率: </b>1080p/1080i&nbsp;&nbsp;&nbsp;<b>音频: </b>TrueHD</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">行为</td><td class="rowfollow" valign="top" align="left"><a title="下载种子" href="download.php?id=88001"><img class="dt_download" src="pic/trans.gif" alt="download" />&nbsp;<b><font class="small">下载种子</font></b></a></td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><table border="0" cellspacing="0"><tr><td class="embedded">该种子暂无字幕</td></tr></table><table border="0" cellspacing="0"><tr><td class="embedded"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.Anime.S02.2026.1080p.BluRay.Remux.AVC.TrueHD.5.1-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="88001" /><input type="hidden" name="in_detail" value="in_detail" /><input type="submit" value="上传字幕" /></form></td></tr></table></td></tr>
</table>
</body></html>`

// 同一站点也会把体积渲染成二进制单位（GiB），SizeRegex 的单位组必须同时接受十进制与 IEC 单位。
const luckptDetailGiBFixture = `<html><body>
<h1 align="center" id="top">Fixture.Show.S01.2026.2160p.WEB-DL.HEVC.HDR10-FIXTURE</h1>
<table width="100%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>43.05 GiB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;电视剧</td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.Show.S01.2026.2160p.WEB-DL.HEVC.HDR10-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="88003" /></form></td></tr>
</table>
</body></html>`

// index.php 顶部 #info_block：id / name / 幸运星（本站的魔力值）/ 做种下载数均取自这里。
// 注意 class 属性带空格（class = 'color_bonus'），且幸运星标签后跟了两个 [链接] 才是冒号；
// 用户链接是绝对地址。
const luckptIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr>
	<td><table width="100%" cellspacing="0" cellpadding="0" border="0"><tr>
		<td class="bottom" align="left">
            <span class="medium">
                欢迎回来, <span class="nowrap"><a  href="https://pt.luckpt.de/userdetails.php?id=70001" class='User_Name'><b>fixture_user</b></a><img src="https://img.example.invalid/medal-1.webp" title="虚构勋章一" class="nexus-username-medal preview" /></span>                                [<a href="logout.php">退出</a>]
                [<a href="usercp.php">控制面板</a>]
                <font class = 'color_bonus'>幸运星 </font>[<a href="mybonus.php">详情</a>][<a href="bonusshop.php">商城</a>]: 86,666.1
                 <a href="attendance.php" class="">[签到已得1000, 补签卡: 3]</a>
                <font class = 'color_invite'>邀请 </font>[<a href="invite.php?id=70001">发送</a>]: 10(0)                                <br />
	            <font class="color_ratio">分享率:</font> 31.432                <font class='color_uploaded'>上传量:</font> 2.828 TB                <font class='color_downloaded'> 下载量:</font> 91.82 GB                <font class='color_active'>当前活动:</font> <img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />81  <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />0&nbsp;&nbsp;
                <font class='color_connectable'>可连接:</font><b><font color="green">是</font></b> <font class='color_bonus'>认领: </font> [<a href="claim.php?uid=70001">108/1000</a>]            </span>
        </td>
	</tr></table></td>
</tr></table>
</body></html>`

// userdetails.php 正文表格：「传输」行是合并单元格，同一行里同时有
// 分享率/实际分享率、上传量/实际上传量/保种上传量、下载量/实际下载量，
// 字面量正则只能命中不带前缀的那一项；「最近动向」之后紧跟同样命中
// :contains('最近动向') 的「最近动向(地点)」行。
const luckptUserdetailsFixture = `<html><body>
<h1 style='margin:0px'><span class="nowrap"><b>fixture_target</b></span></h1>
<table width="100%" border="1" cellspacing="0" cellpadding="5">
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">等级</td><td width="99%" class="rowfollow" valign="top" align="left"><img alt="Elite User" title="Elite User" src="pic/elite.gif" /> </td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">加入日期</td><td width="99%" class="rowfollow" valign="top" align="left">2025-07-13 14:24:16 (<span title="2025-07-13 14:24:16">1年0月前</span>, 54.1周)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-07-27 15:06:52 (<span title="2026-07-27 15:06:52">6分钟前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向(地点)</td><td width="99%" class="rowfollow" valign="top" align="left">home</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">BT客户端</td><td width="99%" class="rowfollow" valign="top" align="left">qBittorrent/5.2.3</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">传输</td><td width="99%" class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>:  <font color="">1,234.567</font>（<strong>实际分享率</strong>：3.138）</td><td class="embedded">&nbsp;&nbsp;<img src="pic/smilies/163.gif" alt="" /></td></tr><tr><td class="embedded"><strong>上传量</strong>:  12.345 TB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>:  45.67 GB</td></tr><tr><td class="embedded"><strong>实际上传量</strong>:  6.789 TB</td><td class="embedded">&nbsp;&nbsp;<strong>实际下载量</strong>:  2.164 TB</td><td class="embedded text-muted">&nbsp;&nbsp;实际上传/下载量 (仅用于记录, 不参与分享率计算)</td></tr><tr><td class="embedded"><strong>保种上传量</strong>:  8.936 TB</td><td class="embedded">&nbsp;&nbsp;<strong>保种下载量</strong>:  493.50 GB</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">BT时间</td><td width="99%" class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>做种时间</strong>:  1024天09:29:36</td><td class="embedded">&nbsp;&nbsp;<strong>下载时间</strong>:  31天01:09:40</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">种子评论</td><td width="99%" class="rowfollow" valign="top" align="left">34</td></tr>
</table>
</body></html>`

// --- Helpers ---

func getLuckptDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("luckpt")
	require.True(t, ok, "luckpt definition not found")
	return def
}

// --- Suite: Search ---

func testLuckptSearch(t *testing.T) {
	def := getLuckptDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(luckptSearchFixture))
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
	assert.Equal(t, "88001", free.ID)
	assert.Equal(t, "Fixture.Anime.S02.2026.1080p.BluRay.Remux.AVC.TrueHD.5.1-FIXTURE", free.Title)
	// 副标题是 <br /> 之后的裸文本，前面还有 3 个彩色标签 span，需要 SubtitleSelector 才能取到
	assert.Equal(t, "虚构副标题一 [内封中字]", free.Subtitle)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	// 大小列为 "46.11<br />GB"，parseSize 去空白后按 1024 进制换算
	assert.Equal(t, int64(49510235504), free.SizeBytes)
	assert.Equal(t, 9, free.Seeders)
	assert.Equal(t, 0, free.Leechers)
	assert.Equal(t, 10, free.Snatched)
	assert.Equal(t, "动画", free.Category)
	// 促销结束时间取名称单元格内 font:contains('剩余时间') 下的 span[title]，
	// 不能误取审核状态 span[title="通过"]，也不能误取第 4 列的存活时间
	require.False(t, free.DiscountEndTime.IsZero(), "discount end time should be parsed")
	assert.Equal(t, 2026, free.DiscountEndTime.Year())
	assert.Equal(t, 8, int(free.DiscountEndTime.Month()))
	assert.Equal(t, 3, free.DiscountEndTime.Day())
	assert.Positive(t, free.UploadedAt)

	// pro_free2up 必须判为 2X 免费：定义里不设置自定义 DiscountMapping，
	// 依赖驱动内建的有序判定（先 free2up 再 free）
	free2up := items[1]
	assert.Equal(t, "88002", free2up.ID)
	assert.Equal(t, "虚构副标题二 [内封中字]", free2up.Subtitle)
	assert.Equal(t, v2.Discount2xFree, free2up.DiscountLevel)
	assert.Equal(t, int64(18779744501), free2up.SizeBytes)
	assert.Equal(t, 17, free2up.Seeders)
	assert.Equal(t, 37, free2up.Snatched)
	require.False(t, free2up.DiscountEndTime.IsZero())
	assert.Equal(t, 1, free2up.DiscountEndTime.Day())

	// 无促销行同样带 span[title="通过"]，结束时间必须为零值
	normal := items[2]
	assert.Equal(t, "88003", normal.ID)
	assert.Equal(t, "Fixture.Show.S01.2026.2160p.WEB-DL.HEVC.HDR10-FIXTURE", normal.Title)
	assert.Equal(t, "虚构副标题三 | 全集 | 类型: 剧情", normal.Subtitle)
	assert.Equal(t, v2.DiscountNone, normal.DiscountLevel)
	assert.True(t, normal.DiscountEndTime.IsZero(), "no discount -> no end time")
	assert.Equal(t, int64(79897129123), normal.SizeBytes)
	assert.Equal(t, 13, normal.Seeders)
	assert.Equal(t, 0, normal.Leechers)
	assert.Equal(t, 24, normal.Snatched)
	assert.Equal(t, "电视剧", normal.Category)
}

// --- Suite: Detail ---

func testLuckptDetail(t *testing.T) {
	def := getLuckptDef(t)

	t.Run("Free", func(t *testing.T) {
		doc := FixtureDoc(t, "luckpt_detail", luckptDetailFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "88001", info.TorrentID)
		assert.Equal(t, "Fixture.Anime.S02.2026.1080p.BluRay.Remux.AVC.TrueHD.5.1-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		require.False(t, info.DiscountEnd.IsZero(), "discount end time should be parsed")
		assert.Equal(t, 2026, info.DiscountEnd.Year())
		assert.Equal(t, 8, int(info.DiscountEnd.Month()))
		assert.Equal(t, 3, info.DiscountEnd.Day())
		// 「基本信息」标签单元格 + .Next() 兄弟节点，全角冒号 + 十进制 GB
		assert.InDelta(t, 46.11*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})

	t.Run("BinaryUnit", func(t *testing.T) {
		doc := FixtureDoc(t, "luckpt_detail_gib", luckptDetailGiBFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "88003", info.TorrentID)
		assert.Equal(t, "Fixture.Show.S01.2026.2160p.WEB-DL.HEVC.HDR10-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel)
		assert.True(t, info.DiscountEnd.IsZero())
		// GiB 与 GB 同样按 1024 换算，SizeRegex 的单位组必须放行 IEC 单位
		assert.InDelta(t, 43.05*1024, info.SizeMB, 0.1)
		// 采集样本三页均无 hitandrun / hit_run.gif，本站未启用 H&R
		assert.False(t, info.HasHR)
	})
}

// --- Suite: UserInfo ---

func testLuckptUserInfo(t *testing.T) {
	def := getLuckptDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "luckpt_index", luckptIndexFixture)
		fields := map[string]string{
			"id":   "70001",
			"name": "fixture_user",
			// 幸运星 86,666.1：标签后跟 [详情][商城] 两个链接才是冒号
			"bonus":    "86666.1",
			"seeding":  "81",
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
		doc := FixtureDoc(t, "luckpt_userdetails", luckptUserdetailsFixture)
		fields := map[string]string{
			// 12.345 TB / 6.789 TB / 45.67 GB 按 1024 进制换算为字节
			"uploaded":     "13573471044894",
			"trueUploaded": "7464584440971",
			"downloaded":   "49037789102",
			// 1,234.567 需要 parseNumber 去掉千分位
			"ratio":     "1234.567",
			"levelName": "Elite User",
			// parseTime 按站点 TimezoneOffset(+0800) 解析
			"joinTime":     "1752387856",
			"lastAccessAt": "1785136012",
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

func TestLuckpt_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"approved_icon": luckptApprovedIcon,
		"tag_spans":     luckptTagSpans,
		"search":        luckptSearchFixture,
		"detail":        luckptDetailFixture,
		"detail_gib":    luckptDetailGiBFixture,
		"index":         luckptIndexFixture,
		"userdetails":   luckptUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
