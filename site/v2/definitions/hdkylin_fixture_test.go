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
		SiteID:   "hdkylin",
		Search:   testHdkylinSearch,
		Detail:   testHdkylinDetail,
		UserInfo: testHdkylinUserInfo,
	})
}

// --- Fixtures ---
//
// 结构照搬 issue #881 附带的真实页面，标题、副标题、种子 ID、用户名、用户 ID、标签图标地址与统计数值
// 都换成了虚构值，审核状态的 SVG 图标只保留一段很短的 path。
//
// 搜索页覆盖的真实特征：
//   - 没有海报缩略图；td.embedded 依次为：名称 / IMDb+豆瓣评分 / 下载按钮
//   - 名称后面依次是促销图标、SeedBox 图标、审核状态 <span title="通过|未审"><svg>…</svg></span>，都在 <br /> 前面
//   - 副标题前的标签 <span> 里可能嵌一张不带结束斜杠的小图标（<span ...><img src="..." style="...">中字</span>）
//   - 做种数少时数字包在 <font color="#aa0000"> 里；列表最后多一列「置顶」

const hdkylinPassSpan = `<span style="margin-left: 6px" title="通过"><svg t="1655145688503" class="icon" viewBox="0 0 1413 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" p-id="16225" width="16" height="16"><path d="M1381.8 107.4L1274.6 0.4 465.2 809.8l-322.6-322.7L35.4 594.1l430 430z" fill="#1afa29" p-id="16226"></path></svg></span>`

const hdkylinPendingSpan = `<span style="margin-left: 6px" title="未审"><svg t="1655184824967" class="icon" viewBox="0 0 1024 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" p-id="34118" width="16" height="16"><path d="M450.2 772.2l0 92.5 92.5 0 0-92.5L450.2 772.2z" p-id="34119" fill="#e78d0f"></path></svg></span>`

const hdkylinRatingCell = `<td class="embedded" style="text-align: right; width: 40px;padding: 4px"><div style="display: flex;flex-direction: column"><div style="display: flex;align-content: center;justify-content: space-between;padding: 2px 0"><img src="pic/imdb2.png" alt="imdb" title="imdb" style="max-width: 16px;max-height: 16px"/><span>N/A</span></div><div style="display: flex;align-content: center;justify-content: space-between;padding: 2px 0"><img src="pic/douban2.png" alt="douban" title="douban" style="max-width: 16px;max-height: 16px"/><span>N/A</span></div></div></td>`

const hdkylinSearchFixture = `<html><body>
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
<td class="colhead">置顶</td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=401"><img class="c_movies" src="pic/cattrans.gif" alt="Movies/电影" title="Movies/电影" style="background-image: url(pic/category/综合区/scenetorrents/chs/dianying.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style='padding-left: 5px'><img class="sticky" src="pic/trans.gif" alt="Sticky" title="三级置顶" />&nbsp;<img class="sticky" src="pic/trans.gif" alt="Sticky" title="三级置顶" />&nbsp;<img class="sticky" src="pic/trans.gif" alt="Sticky" title="三级置顶" />&nbsp;<a title="Fixture.Movie.2019.1080p.BluRay.x264.AAC.2.0-FIXTURE" href="details.php?id=99301&amp;hit=1" target="_blank"><b>Fixture.Movie.2019.1080p.BluRay.x264.AAC.2.0-FIXTURE</b></a> <img class="pro_50pctdown" src="pic/trans.gif" alt="50%" title="50%" /><img src="pic/misc/seed-box.png" style="vertical-align: bottom; height: 16px; margin-left: 4px" title="SeedBox" />` + hdkylinPassSpan + `<br /><span style="background-color:background-color: #FFE53B; background-image: linear-gradient(147deg, #FFE53B 0%, #FF2525 74%);;color:#191970;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="官方标签">官种</span><span style="background-color:background-color: #FAACA8; background-image: linear-gradient(19deg, #FAACA8 0%, #DDD6F3 100%);;color:#191970;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">特效字幕</span><span style="background-color:background-color: #ff0000; background-image: linear-gradient(62deg, #ff0000 0%, #ae1010 100%);;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title=""><img src="pic/fixture-tag-zh.png" style="height: 12px; width: auto;padding: 0 3px 0 1px;">中字</span><span style="background-color:;background: url('pic/fixture-fire.gif') repeat-x;background-size: cover;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="加成专用">麒麟火</span>虚构电影 / Fixture Movie | 类型：剧情 / 悬疑 | 演员：示例演员甲 / 示例演员乙</td>` + hdkylinRatingCell + `<td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=99301"><img class="download" src="pic/trans.gif" style='padding-bottom: 2px;' alt="download" title="下载本种" /></a><br /><a id="bookmark0" href="javascript: bookmark(99301,0);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=99301&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2025-04-22 23:13:00">1年<br />2月</span></td><td class="rowfollow">28.34<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99301&amp;hit=1&amp;dllist=1#seeders">66</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><a href="viewsnatches.php?id=99301"><b>210</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
<td class="rowfollow"><a href="javascript:promotion('99301');"><img class="staff_delete" src="pic/fixture-pin.png" alt="D" title="置顶" /></a></tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=401"><img class="c_movies" src="pic/cattrans.gif" alt="Movies/电影" title="Movies/电影" style="background-image: url(pic/category/综合区/scenetorrents/chs/dianying.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style='padding-left: 5px'><a title="Fixture.Saga.Part.1.2011.1080p.Blu-ray.AVC.DTS-HD.MA.7.1-FIXTURE" href="details.php?id=99302&amp;hit=1" target="_blank"><b>Fixture.Saga.Part.1.2011.1080p.Blu-ray.AVC.DTS-HD.MA.7.1-FIXTURE</b></a> <img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-07-05 07:08:09&quot;&gt;23时56分钟&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000,'fade','both','styleClass','niceTitle', 'fadeMax',87, 'maxWidth', 300);" /> <font color='#0000FF'>剩余时间：<span title="2026-07-05 07:08:09">23时56分钟</span></font>` + hdkylinPendingSpan + `<br /><span style="background-color:background-color: #74EBD5; background-image: linear-gradient(90deg, #74EBD5 0%, #15391b 100%);;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="英文字幕"><img src="pic/fixture-tag-en.png" style="height: 12px; width: auto;padding: 0 3px 0 1px;">英字</span><span style="background-color:background-color: #000000; background-image: linear-gradient(135deg, #000000 0%, #1212aa 49%);;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title=""><img src="pic/fixture-tag-bd.png" style="height: 12px; width: auto;padding: 0 3px 0 1px;">Blu-ray</span>虚构传奇(上)/示例译名(港)/示例译名(台)</td>` + hdkylinRatingCell + `<td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=99302"><img class="download" src="pic/trans.gif" style='padding-bottom: 2px;' alt="download" title="下载本种" /></a><br /><a id="bookmark1" href="javascript: bookmark(99302,1);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=99302&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-07-04 07:13:56">3分钟</span></td><td class="rowfollow">41.71<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99302&amp;hit=1&amp;dllist=1#seeders"><font color="#aa0000">1</font></a></b></td>
<td class="rowfollow"><b><a href="details.php?id=99302&amp;hit=1&amp;dllist=1#leechers">9</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><i>匿名</i></td>
<td class="rowfollow"><a href="javascript:promotion('99302');"><img class="staff_delete" src="pic/fixture-pin.png" alt="D" title="置顶" /></a></tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=402"><img class="c_tvseries" src="pic/cattrans.gif" alt="TV Series/电视剧" title="TV Series/电视剧" style="background-image: url(pic/category/综合区/scenetorrents/chs/dianshiju.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style='padding-left: 5px'><a title="Fixture.Series.S02.2026.2160p.WEB-DL.H265.DDP5.1-FIXTURE" href="details.php?id=99303&amp;hit=1" target="_blank"><b>Fixture.Series.S02.2026.2160p.WEB-DL.H265.DDP5.1-FIXTURE</b></a> <img class="pro_free2up" src="pic/trans.gif" alt="2X Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;twoupfree&quot;&gt;2X免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-07-03 21:00:00&quot;&gt;1天22时&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000,'fade','both','styleClass','niceTitle', 'fadeMax',87, 'maxWidth', 300);" /> <font color='#00CC66'>剩余时间：<span title="2026-07-03 21:00:00">1天22时</span></font><img src="pic/misc/seed-box.png" style="vertical-align: bottom; height: 16px; margin-left: 4px" title="SeedBox" />` + hdkylinPendingSpan + `<br />虚构剧集 第二季 全8集</td>` + hdkylinRatingCell + `<td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=99303"><img class="download" src="pic/trans.gif" style='padding-bottom: 2px;' alt="download" title="下载本种" /></a><br /><a id="bookmark2" href="javascript: bookmark(99303,2);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><b><a href="details.php?id=99303&amp;hit=1&amp;cmtpage=1#startcomments" >3</a></b></td><td class="rowfollow nowrap"><span title="2026-07-02 09:10:11">1天<br />12时</span></td><td class="rowfollow">18.50<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99303&amp;hit=1&amp;dllist=1#seeders">24</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=99303&amp;hit=1&amp;dllist=1#leechers">5</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=99303"><b>48</b></a></td>
<td class="rowfollow"><span class="nowrap"><a href="https://www.hdkyl.in/userdetails.php?id=90009" class='PowerUser_Name'><b>fixture_uploader</b></a></span></td>
<td class="rowfollow"><a href="javascript:promotion('99303');"><img class="staff_delete" src="pic/fixture-pin.png" alt="D" title="置顶" /></a></tr>
</table>
</body></html>`

// 详情页（限时免费）：标题 / ID 取自隐藏域，h1 末尾在剩余时间后面还有审核状态「未审」的 SVG。
const hdkylinDetailFixture = `<html><body>
<h1 align="center" id="top">Fixture.Saga.Part.1.2011.1080p.Blu-ray.AVC.DTS-HD.MA.7.1-FIXTURE&nbsp;&nbsp;&nbsp; <b>[<font class='free' >免费</font>]</b> <font color='#0000FF'>剩余时间：<span title="2026-07-05 07:08:09">23时56分钟</span></font>` + hdkylinPendingSpan + `</h1>
<table class="main" width="100%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead" width="13%">下载</td><td class="rowfollow" width="87%" align="left"><a class="index" href="download.php?id=99302">[HD-Kylin].Fixture.Saga.Part.1.2011.1080p.Blu-ray.AVC.DTS-HD.MA.7.1-FIXTURE.torrent</a>&nbsp;&nbsp;<a id="bookmark0" href="javascript: bookmark(99302,0);"><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a>&nbsp;&nbsp;&nbsp;由&nbsp;<i>匿名</i>发布于<span title="2026-07-04 07:13:56">3分钟前</span></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">虚构传奇(上)/示例译名(港)/示例译名(台)</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>41.71 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;Movies/电影&nbsp;&nbsp;&nbsp;<b>年代: </b>Earlier/更早&nbsp;&nbsp;&nbsp;<b>媒介: </b>Blu-ray(原盘)&nbsp;&nbsp;&nbsp;<b>视频编码: </b>H.264/AVC&nbsp;&nbsp;&nbsp;<b>音频编码: </b>DTS-HD MA</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">魔力值奖励</td><td class="rowfollow" valign="top" align="left">0</td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.Saga.Part.1.2011.1080p.Blu-ray.AVC.DTS-HD.MA.7.1-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="99302" /></form></td></tr>
</table>
</body></html>`

// 详情页（无促销）：h1 只有标题与审核状态「通过」的 SVG；隐藏域里的标题已经还原了 &#039; 实体。
const hdkylinDetailPlainFixture = `<html><body>
<h1 align="center" id="top">It&#039;s.a.Fixture.Life.1946.BluRay.1080p.x265.10bit-FIXTURE` + hdkylinPassSpan + `</h1>
<table class="main" width="100%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead" width="13%">下载</td><td class="rowfollow" width="87%" align="left"><a class="index" href="download.php?id=99304">[HD-Kylin].It&#039;s.a.Fixture.Life.1946.BluRay.1080p.x265.10bit-FIXTURE.torrent</a>&nbsp;&nbsp;&nbsp;由&nbsp;<span class="nowrap"><a href="https://www.hdkyl.in/userdetails.php?id=90009" class='PowerUser_Name'><b>fixture_uploader</b></a> (<b class='PowerUser_Name'>Power User</b>)</span>发布于<span title="2026-06-30 12:00:00">4天前</span></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">【虚构片名】 黑白原版 国英双语 10bit HEVC</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>5.66 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;Movies/电影&nbsp;&nbsp;&nbsp;<b>媒介: </b>Blu-ray(原盘)</td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="It's.a.Fixture.Life.1946.BluRay.1080p.x265.10bit-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="99304" /></form></td></tr>
</table>
</body></html>`

// index.php：#info_block 里第一个链接是头像（usercp.php），用户链接是绝对地址；
// 用户名后面的图标带 onmouseover 提示；魔力值写在按钮里。
const hdkylinIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr>
	<td><table width="100%" cellspacing="0" cellpadding="0" border="0"><tr>
		<td class="bottom" align="left" style="display:inline-flex; align-items:center;">
		<a href="/usercp.php?action=personal">
		<img src="pic/default_avatar.png" class="hdk-avatar">
		</a>
            <span class="medium">
                <b><span class="nowrap"><a href="https://www.hdkyl.in/userdetails.php?id=90001" class='PowerUser_Name'><b class="rainbow">fixture_user</b></a></span><i class="fab fa-vaadin" onmouseover="domTT_activate(this, event, 'content', 'UID:<b></b>90001<br>保种数:<b>87</b><br>做种量: <b>1.000TB</b><br>上传量: <b>2.500TB</b><br>时魔: <b>开发中</b>', 'trail', true, 'delay', 0,'styleClass','niceTitle','maxWidth', 600);"></i></b>
                [<a href="logout.php">退出</a>]
                <font class="color_ratio">分享率:</font> 3.937                <font class='color_uploaded'>上传量:</font> 2.500 TB                <font class='color_downloaded'> 下载量:</font> 650.25 GB
                <font class='color_active'>当前活动:</font> <img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />87  <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />0&nbsp;&nbsp;
                <font class='color_slots'>连接数：</font>无限制                <br />
                <a href="usercp.php" style="padding: 2px 6px;background-color:#1faded;color: #fff;border-radius: 4px;">&nbsp;&nbsp;控制面板&nbsp;&nbsp;</a>
                <a href="mybonus.php" style="padding: 2px 6px;background-color:#1faded;color: #fff;border-radius: 4px;">魔力值 : 12,345.6</a>
                <a href="attendance.php" class="" style="padding: 2px 6px;background-color:#1faded;color: #fff;border-radius: 4px;">[签到已得100, 补签卡: 1]</a>
                <a href="invite.php?id=90001" style="padding: 2px 6px;background-color:#1faded;color: #fff;border-radius: 4px;">邀请 : 0(0)</a>
            </span>
        </td>
	</tr></table></td>
</tr></table>
</body></html>`

// userdetails.php：魔力值一行没有 [明细]，做种积分有两个千分位逗号。
const hdkylinUserdetailsFixture = `<html><body>
<table class="main" width="100%" border="1" cellspacing="0" cellpadding="5">
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">用户ID/UID</td><td width="99%" class="rowfollow" valign="top" align="left">90001</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">邀请人</td><td width="99%" class="rowfollow" valign="top" align="left"><i>匿名</i></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">加入日期</td><td width="99%" class="rowfollow" valign="top" align="left">2023-09-10 11:12:13 (<span title="2023-09-10 11:12:13">2年10月前</span>, 147周)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-07-04 22:33:44 (<span title="2026-07-04 22:33:44">&lt; 1分钟前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">传输</td><td width="99%" class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>: <font color="">3.937</font>（<strong>实际分享率</strong>：0.431）</td><td class="embedded">&nbsp;&nbsp;<img src="pic/smilies/1.png" alt="" /></td></tr><tr><td class="embedded"><strong>上传量</strong>: 2.500 TB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>: 650.25 GB</td></tr><tr><td class="embedded"><strong>实际上传量</strong>: 860.20 GB</td><td class="embedded">&nbsp;&nbsp;<strong>实际下载量</strong>: 1.948 TB</td><td class="embedded text-muted">&nbsp;&nbsp;实际上传/下载量 (仅用于记录, 不参与分享率计算)</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">等级</td><td width="99%" class="rowfollow" valign="top" align="left"><img alt="Power User" title="Power User" src="pic/power.gif" /> </td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">魔力值</td><td width="99%" class="rowfollow" valign="top" align="left">12,345.6</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">做种积分</td><td width="99%" class="rowfollow" valign="top" align="left">1,234,567.8&nbsp;&nbsp;<span class='text-muted'>(更新时间: 2026-07-04 06:00:00)</span></td></tr>
</table>
</body></html>`

// --- Helpers ---

func getHdkylinDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("hdkylin")
	require.True(t, ok, "hdkylin definition not found")
	return def
}

// --- Suite: Search ---

func testHdkylinSearch(t *testing.T) {
	def := getHdkylinDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(hdkylinSearchFixture))
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

	halfDown := items[0]
	assert.Equal(t, "99301", halfDown.ID)
	assert.Equal(t, "Fixture.Movie.2019.1080p.BluRay.x264.AAC.2.0-FIXTURE", halfDown.Title)
	// 四个标签里第三个嵌了一张没有结束斜杠的 <img>，正则要整段吞掉标签
	assert.Equal(t, "虚构电影 / Fixture Movie | 类型：剧情 / 悬疑 | 演员：示例演员甲 / 示例演员乙", halfDown.Subtitle)
	assert.Equal(t, v2.DiscountPercent50, halfDown.DiscountLevel)
	assert.True(t, halfDown.DiscountEndTime.IsZero(), "permanent promotion has no end time")
	assert.Equal(t, int64(30429843292), halfDown.SizeBytes)
	assert.Equal(t, 66, halfDown.Seeders)
	assert.Equal(t, 0, halfDown.Leechers)
	assert.Equal(t, 210, halfDown.Snatched)
	assert.Equal(t, "Movies/电影", halfDown.Category)
	assert.Positive(t, halfDown.UploadedAt)
	assert.False(t, halfDown.HasHR)

	free := items[1]
	assert.Equal(t, "99302", free.ID)
	assert.Equal(t, "虚构传奇(上)/示例译名(港)/示例译名(台)", free.Subtitle)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	require.False(t, free.DiscountEndTime.IsZero(), "discount end time should come from onmouseover tooltip")
	assert.Equal(t, "2026-07-05 07:08:09", free.DiscountEndTime.Format("2006-01-02 15:04:05"))
	assert.Equal(t, int64(44785771479), free.SizeBytes)
	// 做种数包在 <font color="#aa0000"> 里
	assert.Equal(t, 1, free.Seeders)
	assert.Equal(t, 9, free.Leechers)
	assert.Equal(t, 0, free.Snatched)

	twoUpFree := items[2]
	assert.Equal(t, "99303", twoUpFree.ID)
	// 没有标签时副标题紧跟在 <br /> 后面
	assert.Equal(t, "虚构剧集 第二季 全8集", twoUpFree.Subtitle)
	assert.Equal(t, v2.Discount2xFree, twoUpFree.DiscountLevel)
	assert.Equal(t, "2026-07-03 21:00:00", twoUpFree.DiscountEndTime.Format("2006-01-02 15:04:05"))
	assert.Equal(t, int64(19864223744), twoUpFree.SizeBytes)
	assert.Equal(t, 24, twoUpFree.Seeders)
	assert.Equal(t, 5, twoUpFree.Leechers)
	assert.Equal(t, 48, twoUpFree.Snatched)
	assert.Equal(t, "TV Series/电视剧", twoUpFree.Category)
}

// --- Suite: Detail ---

func testHdkylinDetail(t *testing.T) {
	def := getHdkylinDef(t)

	t.Run("TimedFree", func(t *testing.T) {
		doc := FixtureDoc(t, "hdkylin_detail", hdkylinDetailFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "99302", info.TorrentID)
		assert.Equal(t, "Fixture.Saga.Part.1.2011.1080p.Blu-ray.AVC.DTS-HD.MA.7.1-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		// 结束时间只认 <font> 里的 span，不能取到审核状态 <span title="未审">
		require.False(t, info.DiscountEnd.IsZero(), "discount end time should be parsed")
		assert.Equal(t, "2026-07-05 07:08:09", info.DiscountEnd.Format("2006-01-02 15:04:05"))
		assert.Empty(t, info.DiscountErr, "the review-status span must not be read as the end time")
		assert.InDelta(t, 41.71*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)

		assert.Equal(t, "download.php?id=99302",
			doc.Find(def.Selectors.DetailDownloadLink).First().AttrOr("href", ""))
		assert.Equal(t, "虚构传奇(上)/示例译名(港)/示例译名(台)",
			strings.TrimSpace(doc.Find(def.Selectors.DetailSubtitle).First().Text()))
	})

	t.Run("NoPromotion", func(t *testing.T) {
		doc := FixtureDoc(t, "hdkylin_detail_plain", hdkylinDetailPlainFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "99304", info.TorrentID)
		assert.Equal(t, "It's.a.Fixture.Life.1946.BluRay.1080p.x265.10bit-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel)
		assert.True(t, info.DiscountEnd.IsZero())
		assert.Empty(t, info.DiscountErr)
		assert.InDelta(t, 5.66*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})
}

// --- Suite: UserInfo ---

func testHdkylinUserInfo(t *testing.T) {
	def := getHdkylinDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "hdkylin_index", hdkylinIndexFixture)
		fields := map[string]string{
			// 第一个链接是头像（usercp.php），用户链接排在它后面
			"id":       "90001",
			"name":     "fixture_user",
			"seeding":  "87",
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
		doc := FixtureDoc(t, "hdkylin_userdetails", hdkylinUserdetailsFixture)
		fields := map[string]string{
			// 2.500 TB / 650.25 GB / 860.20 GB / 1.948 TB 按 1024 进制换算为字节
			"uploaded":       "2748779069440",
			"downloaded":     "698200621056",
			"trueUploaded":   "923632717004",
			"trueDownloaded": "2141848650907",
			"ratio":          "3.937",
			"levelName":      "Power User",
			"bonus":          "12345.6",
			// 两个千分位逗号都要去掉；parseNumber 对百万以上的数输出科学计数法，与其他站点的测试一致
			"seedingBonus": "1.2345678e+06",
			"joinTime":     "1694315533",
			"lastAccessAt": "1783175624",
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

func TestHdkylin_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":       hdkylinSearchFixture,
		"detail":       hdkylinDetailFixture,
		"detail_plain": hdkylinDetailPlainFixture,
		"index":        hdkylinIndexFixture,
		"userdetails":  hdkylinUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
