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
		SiteID:   "kufei",
		Search:   testKufeiSearch,
		Detail:   testKufeiDetail,
		UserInfo: testKufeiUserInfo,
	})
}

// --- Fixtures ---
//
// 结构照搬 issue #870 附带的真实页面（CHD/scenetorrents 分类图标皮肤），标题、副标题、种子 ID、
// 用户名、用户 ID、海报地址与统计数值都换成了虚构值。
//
// 搜索页覆盖的真实特征：
//   - 名称子表第一个 td.embedded 是 46x46 海报缩略图，名称单元格排在它后面
//   - 限时促销的剩余时间同时出现在促销图标的 onmouseover 提示与可见的 <font color>剩余时间：<span title> 里
//   - 永久促销（如 50%）的图标只有 title，没有 onmouseover，也没有剩余时间
//   - 副标题是 <br /> 之后的裸文本，前面可能有几个 <span title="">徽章</span>
//   - 大小列是 "559.35<br />MB"（数字与单位被 <br /> 分开）
//   - 评论数为 0 时是「添加评论」链接，完成数为 0 时是裸数字

const kufeiSearchFixture = `<html><body>
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
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=405"><img class="c_anime" src="pic/cattrans.gif" alt="Animations/动画、动漫" title="Animations/动画、动漫" style="background-image: url(pic/category/chd/scenetorrents/chs/catsprites.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="pic/attachments/fixture-poster-1.jpg" class="nexus-lazy-load" style="max-height: 46px;max-width: 46px" /></td><td class="embedded" style='padding-left: 5px'><a title="Fixture.Anime.S01E06.2026.1080p.WEB-DL.H264.AAC-FIXTURE" href="details.php?id=99101&amp;hit=1"><b>Fixture.Anime.S01E06.2026.1080p.WEB-DL.H264.AAC-FIXTURE</b></a> <img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-07-03 21:00:00&quot;&gt;1天23时&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000,'fade','both','styleClass','niceTitle', 'fadeMax',87, 'maxWidth', 300);" /> <font color='#0000FF'>剩余时间：<span title="2026-07-03 21:00:00">1天23时</span></font><br /><span style="background-color:#E76955;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">动漫</span><span style="background-color:#006400;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">中字</span>虚构动画 第01季 第06集 | 类型: 剧情 / 动画</td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=99101"><img class="download" src="pic/trans.gif" style='padding-bottom: 2px;' alt="download" title="下载本种" /></a><br /><a id="bookmark0" href="javascript: bookmark(99101,0);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=99101&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-07-01 21:00:00">1分钟</span></td><td class="rowfollow">559.35<br />MB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99101&amp;hit=1&amp;dllist=1#seeders">5</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=99101&amp;hit=1&amp;dllist=1#leechers">2</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr style="background-color: #89c9e6">
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=401"><img class="c_movies" src="pic/cattrans.gif" alt="Movies/电影" title="Movies/电影" style="background-image: url(pic/category/chd/scenetorrents/chs/catsprites.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr style="background-color: #89c9e6"><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="pic/attachments/fixture-poster-2.jpg" class="nexus-lazy-load" style="max-height: 46px;max-width: 46px" /></td><td class="embedded" style='padding-left: 5px'><img class="sticky" src="pic/trans.gif" alt="Sticky" title="一级置顶" />&nbsp;<img class="sticky" src="pic/trans.gif" alt="Sticky" title="一级置顶" />&nbsp;<a title="Fixture.Movie.Collection.2026.1080p.x265.10bit" href="details.php?id=99102&amp;hit=1"><b>Fixture.Movie.Collection.2026.1080p.x265.10bit</b></a> <b>[<font class='recommended'>推荐</font>]</b> <img class="pro_50pctdown" src="pic/trans.gif" alt="50%" title="50%" /><br /><span style="background-color:#6a3906;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">国语</span><span style="background-color:#006400;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">中字</span>虚构电影合集 终极收藏版 1080p 10bit</td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=99102"><img class="download" src="pic/trans.gif" style='padding-bottom: 2px;' alt="download" title="下载本种" /></a><br /><a id="bookmark1" href="javascript: bookmark(99102,1);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><b><a href="details.php?id=99102&amp;hit=1&amp;cmtpage=1#startcomments" >20</a></b></td><td class="rowfollow nowrap"><span title="2024-03-05 06:07:08">2年<br />4月</span></td><td class="rowfollow">1.250<br />TB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99102&amp;hit=1&amp;dllist=1#seeders">40</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=99102&amp;hit=1&amp;dllist=1#leechers">3</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=99102"><b>120</b></a></td>
<td class="rowfollow"><span class="nowrap"><a href="https://kufei.org/userdetails.php?id=90009" class='EliteUser_Name'><b>fixture_uploader</b></a></span></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=402"><img class="c_tvseries" src="pic/cattrans.gif" alt="TV Series/电视剧" title="TV Series/电视剧" style="background-image: url(pic/category/chd/scenetorrents/chs/catsprites.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="pic/attachments/fixture-poster-3.jpg" class="nexus-lazy-load" style="max-height: 46px;max-width: 46px" /></td><td class="embedded" style='padding-left: 5px'><a title="Fixture.Series.S01.2026.2160p.WEB-DL.H265.AAC-FIXTURE" href="details.php?id=99103&amp;hit=1"><b>Fixture.Series.S01.2026.2160p.WEB-DL.H265.AAC-FIXTURE</b></a> <img class="pro_free2up" src="pic/trans.gif" alt="2X Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;twoupfree&quot;&gt;2X免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-07-05 07:08:09&quot;&gt;11时6分钟&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000,'fade','both','styleClass','niceTitle', 'fadeMax',87, 'maxWidth', 300);" /> <font color='#00CC66'>剩余时间：<span title="2026-07-05 07:08:09">11时6分钟</span></font><br /><span style="background-color:#6a3906;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">英语</span>虚构剧集 &amp; 幕后花絮 全10集</td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=99103"><img class="download" src="pic/trans.gif" style='padding-bottom: 2px;' alt="download" title="下载本种" /></a><br /><a id="bookmark2" href="javascript: bookmark(99103,2);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><b><a href="details.php?id=99103&amp;hit=1&amp;cmtpage=1#startcomments" >4</a></b></td><td class="rowfollow nowrap"><span title="2026-07-02 09:10:11">3天<br />2时</span></td><td class="rowfollow">15.78<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99103&amp;hit=1&amp;dllist=1#seeders">12</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><a href="viewsnatches.php?id=99103"><b>33</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=406"><img class="c_mv" src="pic/cattrans.gif" alt="MV/演唱会" title="MV/演唱会" style="background-image: url(pic/category/chd/scenetorrents/chs/catsprites.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="pic/attachments/fixture-poster-4.jpg" class="nexus-lazy-load" style="max-height: 46px;max-width: 46px" /></td><td class="embedded" style='padding-left: 5px'><a title="Fixture.Concert.2026.1080p.WEB-DL-FIXTURE" href="details.php?id=99104&amp;hit=1"><b>Fixture.Concert.2026.1080p.WEB-DL-FIXTURE</b></a><br />虚构演唱会 现场版</td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=99104"><img class="download" src="pic/trans.gif" style='padding-bottom: 2px;' alt="download" title="下载本种" /></a><br /><a id="bookmark3" href="javascript: bookmark(99104,3);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><b><a href="details.php?id=99104&amp;hit=1&amp;cmtpage=1#startcomments" >1</a></b></td><td class="rowfollow nowrap"><span title="2025-06-07 10:20:30">1年<br />1月</span></td><td class="rowfollow">272.99<br />MB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99104&amp;hit=1&amp;dllist=1#seeders">3</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><a href="viewsnatches.php?id=99104"><b>15</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
</table>
</body></html>`

// kufeiInfoBlock 是每个页面顶部的 #info_block。页头有 H&R 计数器「H&R: [0/0/10]」，
// 它不是种子的 H&R 标记，详情页不能因此判成 H&R 种子。
const kufeiInfoBlock = `<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr>
	<td><table width="100%" cellspacing="0" cellpadding="0" border="0"><tr>
		<td class="bottom" align="left">
            <span class="medium">
                欢迎回来, <span class="nowrap"><a href="https://kufei.org/userdetails.php?id=90001" class='PowerUser_Name'><b class="rainbow">fixture_user</b></a></span>                [<a href="logout.php">退出</a>]
                [<a href="usercp.php">控制面板</a>]
                [<a href="torrents.php?inclbookmarked=1&amp;allsec=1&amp;incldead=0">收藏</a>]
                <font class = 'color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 12,345.6                 <a href="attendance.php" class="">[签到已得100, 补签卡: 2]</a>                <a href="medal.php">[勋章]</a>
                <a href="task.php">[任务]</a>
                <font class = 'color_invite'>邀请 </font>[<a href="invite.php?id=90001">发送</a>]: 0(0)                                <br />
	            <font class="color_ratio">分享率:</font> 3.229                <font class='color_uploaded'>上传量:</font> 1.200 TB                <font class='color_downloaded'> 下载量:</font> 380.50 GB                <font class='color_active'>当前活动:</font> <img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />23  <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />1&nbsp;&nbsp;
                <font class='color_connectable'>可连接:</font><b><font color="green">是</font></b>                <font class='color_slots'>连接数：</font>无限制                <font class='color_bonus'>H&R: </font> [<a href="myhr.php">0/<font color="red">0</font>/10</a>]                <font class='color_bonus'>认领: </font> [<a href="claim.php?uid=90001">0/1000</a>]            </span>
        </td>
	</tr></table></td>
</tr></table>`

// 详情页（限时免费）：标题 / ID 取自 <form> 内的隐藏域，h1 的促销标记是 <font class='free' >，
// 剩余时间在无 class 的 <font color='#0000FF'> 里；「基本信息」行是全角冒号 + 十进制单位。
const kufeiDetailFixture = `<html><body>` + kufeiInfoBlock + `
<h1 align="center" id="top">Fixture.Anime.S01E06.2026.1080p.WEB-DL.H264.AAC-FIXTURE&nbsp;&nbsp;&nbsp; <b>[<font class='free' >免费</font>]</b> <font color='#0000FF'>剩余时间：<span title="2026-07-03 21:00:00">1天23时</span></font></h1>
<table class="main" width="100%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead" width="13%">下载</td><td class="rowfollow" width="87%" align="left"><a class="index" href="download.php?id=99101">[库非].Fixture.Anime.S01E06.2026.1080p.WEB-DL.H264.AAC-FIXTURE.torrent</a>&nbsp;&nbsp;<a id="bookmark0" href="javascript: bookmark(99101,0);"><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a>&nbsp;&nbsp;&nbsp;由&nbsp;<i>匿名</i>发布于<span title="2026-07-01 21:00:00">1分钟前</span></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">虚构动画 第01季 第06集 | 类型: 剧情 / 动画</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>559.35 MB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;Animations/动画、动漫&nbsp;&nbsp;&nbsp;<b>媒介: </b>WEB-DL&nbsp;&nbsp;&nbsp;<b>编码: </b>X264&nbsp;&nbsp;&nbsp;<b>分辨率: </b>1080p&nbsp;&nbsp;&nbsp;<b>制作组: </b>Other&nbsp;&nbsp;&nbsp;<b>音频编码: </b>AAC</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">行为</td><td class="rowfollow" valign="top" align="left"><a title="下载种子" href="download.php?id=99101"><img class="dt_download" src="pic/trans.gif" alt="download" />&nbsp;<b><font class="small">下载种子</font></b></a></td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><table border="0" cellspacing="0"><tr><td class="embedded"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.Anime.S01E06.2026.1080p.WEB-DL.H264.AAC-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="99101" /></form></td></tr></table></td></tr>
</table>
</body></html>`

// 详情页（永久 50%）：h1 只有 <font class='halfdown' >50%</font>，没有剩余时间。
const kufeiDetailPermanentFixture = `<html><body>` + kufeiInfoBlock + `
<h1 align="center" id="top">Fixture.Movie.2026.2160p.WEB-DL.AAC2.0.H265-FIXTURE&nbsp;&nbsp;&nbsp; <b>[<font class='halfdown' >50%</font>]</b></h1>
<table class="main" width="100%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead" width="13%">下载</td><td class="rowfollow" width="87%" align="left"><a class="index" href="download.php?id=99105">[库非].Fixture.Movie.2026.2160p.WEB-DL.AAC2.0.H.265-FIXTURE.mkv.torrent</a>&nbsp;&nbsp;&nbsp;由&nbsp;<i>匿名</i>发布于<span title="2026-06-28 12:43:40">2天18时前</span></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">虚构电影 | 2026 | 2160p SDR</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>4.01 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;Movies/电影&nbsp;&nbsp;&nbsp;<b>媒介: </b>WEB-DL</td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.Movie.2026.2160p.WEB-DL.AAC2.0.H265-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="99105" /></form></td></tr>
</table>
</body></html>`

// index.php：id / name / 做种下载数取自 #info_block，用户链接是绝对地址。
const kufeiIndexFixture = `<html><body>` + kufeiInfoBlock + `</body></html>`

// userdetails.php：「传输」行是内嵌小表格，「最近动向」后面紧跟一行空的「最近动向(地点)」，
// 魔力值与做种积分各占一行，做种积分后面带「(更新时间: …)」。
const kufeiUserdetailsFixture = `<html><body>
<h1 style='margin:0px'><span class="nowrap"><b>fixture_user</b></span></h1>
<table class="main" width="100%" border="1" cellspacing="0" cellpadding="5">
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">用户ID/UID</td><td width="99%" class="rowfollow" valign="top" align="left">90001</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">加入日期</td><td width="99%" class="rowfollow" valign="top" align="left">2025-01-02 03:04:05 (<span title="2025-01-02 03:04:05">1年6月前</span>, 78.0周)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-07-01 08:30:00 (<span title="2026-07-01 08:30:00">&lt; 1分钟前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向(地点)</td><td width="99%" class="rowfollow" valign="top" align="left"></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">BT客户端</td><td width="99%" class="rowfollow" valign="top" align="left">qBittorrent/5.1.2</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">传输</td><td width="99%" class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>: <font color="">3.229</font>（<strong>实际分享率</strong>：1.024）</td><td class="embedded">&nbsp;&nbsp;<img src="pic/smilies/163.gif" alt="" /></td></tr><tr><td class="embedded"><strong>上传量</strong>: 1.200 TB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>: 380.50 GB</td></tr><tr><td class="embedded"><strong>实际上传量</strong>: 860.20 GB</td><td class="embedded">&nbsp;&nbsp;<strong>实际下载量</strong>: 840.10 GB</td><td class="embedded text-muted">&nbsp;&nbsp;实际上传/下载量 (仅用于记录, 不参与分享率计算)</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">等级</td><td width="99%" class="rowfollow" valign="top" align="left"><img alt="Power User" title="Power User" src="pic/power.gif" /> </td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">H&amp;R</td><td width="99%" class="rowfollow" valign="top" align="left"><a href="myhr.php?uid=90001">0/0/10</a></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">魔力值</td><td width="99%" class="rowfollow" valign="top" align="left">12,345.6&nbsp;&nbsp;<a href="bonus-log.php?uid=90001" target="_blank" class="altlink">[明细]</a></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">做种积分</td><td width="99%" class="rowfollow" valign="top" align="left">23,456.7&nbsp;&nbsp;<span class='text-muted'>(更新时间: 2026-06-30 04:52:40)</span></td></tr>
</table>
</body></html>`

// --- Helpers ---

func getKufeiDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("kufei")
	require.True(t, ok, "kufei definition not found")
	return def
}

// --- Suite: Search ---

func testKufeiSearch(t *testing.T) {
	def := getKufeiDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(kufeiSearchFixture))
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

	free := items[0]
	assert.Equal(t, "99101", free.ID)
	assert.Equal(t, "Fixture.Anime.S01E06.2026.1080p.WEB-DL.H264.AAC-FIXTURE", free.Title)
	// 副标题在 <br /> 之后，被两个 <span title=""> 徽章挡在前面
	assert.Equal(t, "虚构动画 第01季 第06集 | 类型: 剧情 / 动画", free.Subtitle)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	// 未配置 DiscountEndTime 选择器，结束时间来自促销图标的 onmouseover 提示
	require.False(t, free.DiscountEndTime.IsZero(), "discount end time should come from onmouseover tooltip")
	assert.Equal(t, "2026-07-03 21:00:00", free.DiscountEndTime.Format("2006-01-02 15:04:05"))
	// "559.35<br />MB" 按 1024 进制换算
	assert.Equal(t, int64(586520985), free.SizeBytes)
	assert.Equal(t, 5, free.Seeders)
	assert.Equal(t, 2, free.Leechers)
	assert.Equal(t, 0, free.Snatched)
	assert.Equal(t, "Animations/动画、动漫", free.Category)
	assert.Positive(t, free.UploadedAt)
	assert.False(t, free.HasHR)

	permanent := items[1]
	assert.Equal(t, "99102", permanent.ID)
	assert.Equal(t, "Fixture.Movie.Collection.2026.1080p.x265.10bit", permanent.Title)
	assert.Equal(t, "虚构电影合集 终极收藏版 1080p 10bit", permanent.Subtitle)
	assert.Equal(t, v2.DiscountPercent50, permanent.DiscountLevel)
	// 永久 50% 的图标没有 onmouseover，也没有剩余时间
	assert.True(t, permanent.DiscountEndTime.IsZero(), "permanent promotion has no end time")
	assert.Equal(t, int64(1374389534720), permanent.SizeBytes)
	assert.Equal(t, 40, permanent.Seeders)
	assert.Equal(t, 3, permanent.Leechers)
	assert.Equal(t, 120, permanent.Snatched)
	assert.Equal(t, "Movies/电影", permanent.Category)

	twoUpFree := items[2]
	assert.Equal(t, "99103", twoUpFree.ID)
	// html 取值保留了 &amp;，replace 过滤器要还原成 &
	assert.Equal(t, "虚构剧集 & 幕后花絮 全10集", twoUpFree.Subtitle)
	assert.Equal(t, v2.Discount2xFree, twoUpFree.DiscountLevel)
	assert.Equal(t, "2026-07-05 07:08:09", twoUpFree.DiscountEndTime.Format("2006-01-02 15:04:05"))
	assert.Equal(t, int64(16943645982), twoUpFree.SizeBytes)
	assert.Equal(t, 12, twoUpFree.Seeders)
	assert.Equal(t, 0, twoUpFree.Leechers)
	assert.Equal(t, 33, twoUpFree.Snatched)

	plain := items[3]
	assert.Equal(t, "99104", plain.ID)
	assert.Equal(t, "Fixture.Concert.2026.1080p.WEB-DL-FIXTURE", plain.Title)
	// 没有徽章时副标题紧跟在 <br /> 后面
	assert.Equal(t, "虚构演唱会 现场版", plain.Subtitle)
	assert.Equal(t, v2.DiscountNone, plain.DiscountLevel)
	assert.True(t, plain.DiscountEndTime.IsZero())
	assert.Equal(t, int64(286250762), plain.SizeBytes)
	assert.Equal(t, 3, plain.Seeders)
	assert.Equal(t, 15, plain.Snatched)
}

// --- Suite: Detail ---

func testKufeiDetail(t *testing.T) {
	def := getKufeiDef(t)

	t.Run("TimedFree", func(t *testing.T) {
		doc := FixtureDoc(t, "kufei_detail", kufeiDetailFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "99101", info.TorrentID)
		assert.Equal(t, "Fixture.Anime.S01E06.2026.1080p.WEB-DL.H264.AAC-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		require.False(t, info.DiscountEnd.IsZero(), "discount end time should be parsed")
		assert.Equal(t, "2026-07-03 21:00:00", info.DiscountEnd.Format("2006-01-02 15:04:05"))
		assert.Empty(t, info.DiscountErr)
		assert.InDelta(t, 559.35, info.SizeMB, 0.01)
		// 页头「H&R: [0/0/10]」计数器不是 H&R 标记
		assert.False(t, info.HasHR, "header H&R counter must not mark the torrent as H&R")

		assert.Equal(t, "download.php?id=99101",
			doc.Find(def.Selectors.DetailDownloadLink).First().AttrOr("href", ""))
		assert.Equal(t, "虚构动画 第01季 第06集 | 类型: 剧情 / 动画",
			strings.TrimSpace(doc.Find(def.Selectors.DetailSubtitle).First().Text()))
	})

	t.Run("PermanentHalfDown", func(t *testing.T) {
		doc := FixtureDoc(t, "kufei_detail_permanent", kufeiDetailPermanentFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "99105", info.TorrentID)
		assert.Equal(t, "Fixture.Movie.2026.2160p.WEB-DL.AAC2.0.H265-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountPercent50, info.DiscountLevel)
		assert.True(t, info.DiscountEnd.IsZero(), "permanent promotion has no end time")
		assert.Empty(t, info.DiscountErr)
		assert.InDelta(t, 4.01*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})
}

// --- Suite: UserInfo ---

func testKufeiUserInfo(t *testing.T) {
	def := getKufeiDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "kufei_index", kufeiIndexFixture)
		fields := map[string]string{
			// 用户链接是绝对地址，querystring 过滤器仍能取到 id
			"id":       "90001",
			"name":     "fixture_user",
			"seeding":  "23",
			"leeching": "1",
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
		doc := FixtureDoc(t, "kufei_userdetails", kufeiUserdetailsFixture)
		fields := map[string]string{
			// 1.200 TB / 380.50 GB / 860.20 GB / 840.10 GB 按 1024 进制换算为字节
			"uploaded":       "1319413953331",
			"downloaded":     "408558764032",
			"trueUploaded":   "923632717004",
			"trueDownloaded": "902050506342",
			// 分享率取 3.229，不能被后面的「（实际分享率：1.024）」抢走
			"ratio":     "3.229",
			"levelName": "Power User",
			// 「12,345.6 [明细]」与「23,456.7 (更新时间: …)」都只取第一个数
			"bonus":        "12345.6",
			"seedingBonus": "23456.7",
			// parseTime 按站点 TimezoneOffset(+0800) 解析；最近动向取第一行，不是空的「最近动向(地点)」
			"joinTime":     "1735758245",
			"lastAccessAt": "1782865800",
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

func TestKufei_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":           kufeiSearchFixture,
		"detail":           kufeiDetailFixture,
		"detail_permanent": kufeiDetailPermanentFixture,
		"index":            kufeiIndexFixture,
		"userdetails":      kufeiUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
