package definitions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func init() {
	RegisterFixtureSuite(FixtureSuite{
		SiteID:   "ptchdbits",
		Search:   testPTCHDBitsSearch,
		Detail:   testPTCHDBitsDetail,
		UserInfo: testPTCHDBitsUserInfo,
	})
}

// 结构照 2026-11-02 的真实页面（简体界面、文字显示促销），标题、ID、时间与统计数都换成了示例值。
// 第 5 行是图标显示的写法（img.pro_*）。
const ptchdbitsSearchFixture = `<html><body>
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
<td class="colhead">进度</td>
</tr>
<tr class='thirtypercentdown_bg'>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=401"><img class="c_movies" src="pic/cattrans.gif" alt="Movies" title="Movies" /></a><a href="?cat401=1&medium3=1"><img src="pic/cattrans.gif" alt="3" title="3" /></a></td>
<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr class='thirtypercentdown_bg'><td class="embedded"><img class="sticky2" src="pic/trans.gif" alt="Sticky" title="" />&nbsp;<a href="details.php?id=600101&amp;hit=1"   ><b>Sample.Movie.2025.REMUX.Bluray.1080p.AVC.DTS-HDMA5.1-CHD</b></a> <b>[<font class='free' (限时<span title="2026-06-28 09:11:50">22时38分</span>)>免费</font>]</b>(限时<span title="2026-06-28 09:11:50">22时38分</span>)<br /> <font class='subtitle' font-weight='600' >&nbsp;<div  style="display:inline-block;height:14px;line-height: 12px" ><div class="tag tag-gf">官方</div><div class="tag tag-sub">中字</div></div>&nbsp;样片/Sample Movie&nbsp;<div class="circle" style="display:inline-block;"><div class="circle-text" style="font-family: SimHei">h3</div></div></font></td><td width="50" class="embedded" align="center" valign="middle"><a href="download.php?id=600101"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=600101&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-06-27 09:11:50">1天</span></td><td class="rowfollow">28.35<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=600101&amp;hit=1&amp;dllist=1#seeders"><font color="#ff0000">15</font></a></b></td>
<td class="rowfollow"><b><a href="details.php?id=600101&amp;hit=1&amp;dllist=1#leechers">3</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=600101"><b>22</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
<td class="rowfollow">--</td>
</tr>
<tr class='thirtypercentdown_bg'>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=401"><img class="c_movies" src="pic/cattrans.gif" alt="Movies" title="Movies" /></a></td>
<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr class='thirtypercentdown_bg'><td class="embedded"><img class="sticky2" src="pic/trans.gif" alt="Sticky" title="" />&nbsp;<a href="details.php?id=600102&amp;hit=1"   ><b>Sample.Show.1991.UHD.BluRay.2160p.HEVC.DTS-HD.MA7.1-CHD</b></a> <b>[<font class='hot'>热门</font>]</b> <b>[<font class='halfdown' >50%</font>]</b><br /> <font class='subtitle' font-weight='600' >&nbsp;<div  style="display:inline-block;height:14px;line-height: 12px" ><div class="tag tag-diy">DIY</div><div class="tag tag-sub">中字</div></div>&nbsp;样例 / Sample Show [DIY 简繁字幕]&nbsp;<div class="circle" style="display:inline-block;"><div class="circle-text" style="font-family: SimHei">h5</div></div></font></td><td width="50" class="embedded" align="center" valign="middle"><a href="download.php?id=600102"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td>
</tr></table></td><td class="rowfollow"><b><a href="details.php?id=600102&amp;hit=1&amp;cmtpage=1#startcomments" >12</a></b></td><td class="rowfollow nowrap"><span title="2026-06-17 07:25:45">10天<br />4时</span></td><td class="rowfollow">77.87<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=600102&amp;hit=1&amp;dllist=1#seeders">381</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><a href="viewsnatches.php?id=600102"><b>805</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
<td class="rowfollow">--</td>
</tr>
<tr class='halfdown_bg'>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=402"><img class="c_tvseries" src="pic/cattrans.gif" alt="TV Series" title="TV Series" /></a></td>
<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr class='halfdown_bg'><td class="embedded"><img class="sticky1" src="pic/trans.gif" alt="Sticky" title="" />&nbsp;<a title="Sample Table S01 1080p WEB-DL H.264-CHDWEB"  href="details.php?id=600103&amp;hit=1"   ><b>Sample Table S01 1080p WEB-DL H.264-CHDWEB</b></a> <b>[<font class='free' >免费</font>]</b><br /> <font class='subtitle' font-weight='600' >&nbsp;<div  style="display:inline-block;height:14px;line-height: 12px" ><div class="tag tag-gf">官方</div></div>&nbsp;韩剧：样例 第4集 | 类型:剧情 | 内封官方中字</font></td><td width="20" class="embedded" valign="middle"><a href="download.php?id=600103"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=600103&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-06-27 00:05:07">12时<br />6分</span></td><td class="rowfollow">3.34<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=600103&amp;hit=1&amp;dllist=1#seeders">71</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=600103&amp;hit=1&amp;dllist=1#leechers">1</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=600103"><b>107</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
<td class="rowfollow">--</td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=402"><img class="c_tvseries" src="pic/cattrans.gif" alt="TV Series" title="TV Series" /></a></td>
<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr><td class="embedded"><a title="Tom and Jerry Sample S01 1080p WEB-DL-CHDWEB"  href="details.php?id=600104&amp;hit=1"   ><b>Tom and Jerry Sample S01 1080p WEB-DL-CHDWEB</b></a> <b>[<font class='hot'>热门</font>]</b><br /> <font class='subtitle' font-weight='600' >&nbsp;<div  style="display:inline-block;height:14px;line-height: 12px" ></div>&nbsp;猫和老鼠 Tom &amp; Jerry &lt;特别版&gt; Grey's 第1集</font></td><td width="20" class="embedded" valign="middle"><a href="download.php?id=600104"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=600104&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-06-26 23:37:27">12时<br />33分</span></td><td class="rowfollow">5.45<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=600104&amp;hit=1&amp;dllist=1#seeders">42</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=600104&amp;hit=1&amp;dllist=1#leechers">3</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=600104"><b>84</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
<td class="rowfollow">--</td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=401"><img class="c_movies" src="pic/cattrans.gif" alt="Movies" title="Movies" /></a></td>
<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr><td class="embedded"><a href="details.php?id=600105&amp;hit=1"   ><b>Sample.Icon.2024.1080p.BluRay-CHD</b></a> <img class="pro_50pctdown2up" src="pic/trans.gif" alt="2X 50%" title="2X 50%" />(限时<span title="2026-06-29 10:00:00">1天</span>)<br /> <font class='subtitle' font-weight='600' >样品</font></td><td width="20" class="embedded" valign="middle"><a href="download.php?id=600105"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=600105&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-06-25 09:11:50">2天</span></td><td class="rowfollow">10.00<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=600105&amp;hit=1&amp;dllist=1#seeders">8</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=600105&amp;hit=1&amp;dllist=1#leechers">1</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=600105"><b>5</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
<td class="rowfollow">--</td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=401"><img class="c_movies" src="pic/cattrans.gif" alt="Movies" title="Movies" /></a></td>
<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr><td class="embedded"><a href="details.php?id=600106&amp;hit=1"   ><b>Sample.Double.2023.1080p.BluRay.x265.10bit-CHD</b></a> <b>[<font class='twoupfree' (限时<span title="2026-06-30 12:00:00">2天</span>)>2X免费</font>]</b>(限时<span title="2026-06-30 12:00:00">2天</span>)</td><td width="20" class="embedded" valign="middle"><a href="download.php?id=600106"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=600106&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-06-27 12:00:00">21分</span></td><td class="rowfollow">7.08<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=600106&amp;hit=1&amp;dllist=1#seeders"><font color="#ff0000">1</font></a></b></td>
<td class="rowfollow"><b><a href="details.php?id=600106&amp;hit=1&amp;dllist=1#leechers">72</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><i>匿名</i></td>
<td class="rowfollow">--</td>
</tr>
</table>
</body></html>`

// ptchdbitsInfoBlock 是每一页顶部的用户信息栏（统计数是示例值）：「H&R:」链接到 hnr.php，后面跟着 H&R 记录数。
const ptchdbitsInfoBlock = `<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr>
<td><table width="100%" cellspacing="0" cellpadding="0" border="0"><tr>
<td class="bottom" align="left"><span class="medium">欢迎回来, <span class="nowrap"><a href="userdetails.php?id=126666" class='VeteranUser_Name'><b>testuser</b></a></span>  [<a href="logout.php">退出</a>]     [<a href="getrss.php">RSS下载</a>] <font class = 'color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 123,456.7 <font class = 'color_invite'>邀请 </font>[<a href="invite.php?id=126666">发送</a>]: 0  <a href='hnr.php?id=126666'><font class = 'color_bonus'>H&R:</font></a>2<br />
<font class="color_ratio">分享率：</font> 12.345  <font class='color_uploaded'>上传量：</font> 12.345 TB<font class='color_downloaded'> 下载量：</font> 1.000 TB  <font class='color_active'>当前活动：</font> <img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />12  <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />1&nbsp;&nbsp;<font class='color_connectable'>可连接：</font>未知 <font class='color_slots'>连接数：</font>无限制 <font class="color_ratio">做种积分: </font>54321.0</span></td>
</tr></table></td></tr></table>`

// 文字显示的限时免费、H3 种子：「基本信息」里有「H&R: 3day」
const ptchdbitsDetailFixture = `<html><body>` + ptchdbitsInfoBlock + `
<h1 align="center" id="top">Sample.Movie.2025.REMUX.Bluray.1080p.AVC.DTS-HDMA5.1-CHD&nbsp;&nbsp;&nbsp; <b>[<font class='free' (限时<span title="2026-06-28 09:11:50">22时38分</span>)>免费</font>]</b>(限时<span title="2026-06-28 09:11:50">22时38分</span>)</h1>
<table width="940" cellspacing="0" cellpadding="5">
<tr><td class="rowhead nowrap" valign="top" align="right">下载</td><td class="rowfollow" valign="top" align="left"><a class="index" href="download.php?id=600101">[CHDBits].Sample.Movie.2025.REMUX.Bluray.1080p.AVC.DTS-HDMA5.1-CHD.torrent</a></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">样片/Sample Movie</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>28.35 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;Movies&nbsp;&nbsp;&nbsp;<b>来源:&nbsp;</b>官方&nbsp;&nbsp;&nbsp;<b>媒介:&nbsp;</b>Remux&nbsp;&nbsp;&nbsp;<b>制作组:&nbsp;</b>REMUX&nbsp;&nbsp;&nbsp;<b>H&R:&nbsp;</b>3day</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">字幕</td><td class="rowfollow" valign="top" align="left"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Sample.Movie.2025.REMUX.Bluray.1080p.AVC.DTS-HDMA5.1-CHD" /><input type="hidden" name="detail_torrent_id" value="600101" /><input type="hidden" name="in_detail" value="in_detail" /><input type="submit" value="上传字幕" /></form></td></tr>
</table>
</body></html>`

// 50%、H5 种子：「H&R: 5day」
const ptchdbitsDetailHalfdownFixture = `<html><body>` + ptchdbitsInfoBlock + `
<h1 align="center" id="top">Sample.Show.1991.UHD.BluRay.2160p.HEVC.DTS-HD.MA7.1-CHD&nbsp;&nbsp;&nbsp; <b>[<font class='halfdown' >50%</font>]</b></h1>
<table width="940" cellspacing="0" cellpadding="5">
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>77.87 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;Movies&nbsp;&nbsp;&nbsp;<b>制作组:&nbsp;</b>CHDBits&nbsp;&nbsp;&nbsp;<b>H&R:&nbsp;</b>5day</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">字幕</td><td class="rowfollow" valign="top" align="left"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Sample.Show.1991.UHD.BluRay.2160p.HEVC.DTS-HD.MA7.1-CHD" /><input type="hidden" name="detail_torrent_id" value="600102" /></form></td></tr>
</table>
</body></html>`

// 图标显示的免费、不是 H&R 的种子：「基本信息」里没有 H&R 一项，页头照样有「H&R:」链接
const ptchdbitsDetailIconFixture = `<html><body>` + ptchdbitsInfoBlock + `
<h1 align="center" id="top">Sample.Table.S01.1080p.WEB-DL.H.264-CHDWEB&nbsp;&nbsp;&nbsp;
  <img class="pro_free" src="pic/trans.gif" alt="Free" title="免费" /> (限时<span title="2026-06-28 09:11:50">22时38分</span>)
</h1>
<table width="940" cellspacing="0" cellpadding="5">
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>3.34 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;TV Series&nbsp;&nbsp;&nbsp;<b>制作组:&nbsp;</b>CHDWEB</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">字幕</td><td class="rowfollow" valign="top" align="left"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Sample.Table.S01.1080p.WEB-DL.H.264-CHDWEB" /><input type="hidden" name="detail_torrent_id" value="600103" /></form></td></tr>
</table>
</body></html>`

const ptchdbitsIndexFixture = `<html><body>` + ptchdbitsInfoBlock + `</body></html>`

const ptchdbitsUserdetailsFixture = `<html><body>` + ptchdbitsInfoBlock + `
<table width="100%" border="1" cellspacing="0" cellpadding="5">
  <tr><td class="rowhead nowrap" valign="top" align="right">加入日期</td><td class="rowfollow" valign="top" align="left">2024-01-02 03:04:05 (<span title="2024-01-02 03:04:05">2年5月前</span>)</td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">最近动向</td><td class="rowfollow" valign="top" align="left">2026-06-27 10:33:44 (<span title="2026-06-27 10:33:44">&lt; 1分前</span>)</td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">传输</td><td class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>:  <font color="">12.345</font></td></tr><tr><td class="embedded"><strong>上传量</strong>:  12.345 TB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>:  1.000 TB<strong>&nbsp;&nbsp;做种积分: </strong>54321.0</td></tr></table></td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">等级</td><td class="rowfollow" valign="top" align="left"><img alt="(高烧)Veteran User" title="(高烧)Veteran User" src="pic/veteran.gif" /> </td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">魔力值</td><td class="rowfollow" valign="top" align="left">123456.7</td></tr>
</table>
</body></html>`

// 魔力值页有好几句「你当前每小时能获取…」：做种（B）、做种体积（C）、官种保种（D），合计（F）在最后
const ptchdbitsMybonusFixture = `<html><body>` + ptchdbitsInfoBlock + `
<table width="940" cellpadding="3"><tr><td class="text" align="left">
<h1>做种每小时将得到如下的魔力值B</h1><ul><li>魔力:123456.7   做种积分:54321.0   当前时间:2026-06-27 10:33:44</li></ul><br /><div align="center">你当前每小时能获取66.6个魔力值 (A = 1234.5)</div><table align="center" border="0" width="400"><tr><td class="loadbarbg" style="border: none; padding: 0px;"><img class="loadbargreen" src="pic/trans.gif" style="width: 222px;" alt="55.5%" /></td></tr></table><br />
<h1>做种大小额外奖励魔力值C</h1><ul><li>做种总大小在2000G以内，C项没有魔力值。</li></ul><div align="center">你当前每小时能获取2个魔力值</div><table align="center" border="0" width="400"><tr><td class="loadbarbg"><img class="loadbarred" src="pic/trans.gif" style="width: 10px;" alt="2.5%" /></td></tr></table><br />
<h1>官方种保种额外奖励魔力值D（仅对大于3G的种子生效）</h1><ul><li>该项魔力值为每个非官方种子获得魔力值加总，但总魔力值不大于80魔力值每小时。</li></ul><div align="center">你当前每小时能获取0个魔力值</div><table align="center" border="0" width="400"><tr><td class="loadbarbg"><img class="loadbarred" src="pic/trans.gif" style="width: 0px;" alt="0%" /></td></tr></table><br />
<h1>合计获得魔力值F</h1><div align="center">你当前每小时能获取68.6个魔力值</div><table align="center" border="0" width="400"><tr><td class="loadbarbg"><img class="loadbarred" src="pic/trans.gif" style="width: 60px;" alt="15%" /></td></tr></table>
<h1>其他获取方式</h1><ul><li>发布新种子 = 15个魔力值</li></ul>
</td></tr></table>
</body></html>`

// 做种列表（getusertorrentlistajax.php?type=seeding）：开头只写了条数，没有合计体积，按表头「大小」那一列加起来。
// 第二行的标题里有「1940 Blu-ray 2160p」这类像体积的字。IP 一列删掉了
const ptchdbitsSeedingFixture = `<b>2</b>条记录<br /><table border="1" cellspacing="0" cellpadding="5" width="900"><tr><td class="colhead" style="padding: 0px">类型</td><td class="colhead" align="center">标题</td><td class="colhead" align="center"><img class="size" src="pic/trans.gif" alt="size" title="大小" /></td><td class="colhead" align="center"><img class="seeders" src="pic/trans.gif" alt="seeders" title="种子数" /></td><td class="colhead" align="center"><img class="leechers" src="pic/trans.gif" alt="leechers" title="下载数" /></td><td class="colhead" align="center">上传</td><td class="colhead" align="center">下载</td><td class="colhead" align="center">分享率</td></tr>
<tr><td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="torrents.php?allsec=1&amp;cat=401"><img class="c_movies" src="pic/cattrans.gif" alt="Movies" title="Movies" /></a></td>
<td class="rowfollow" width="100%" align="left"><a href="details.php?id=600101&amp;hit=1" title="Sample Movie 2008 1080p BluRay x265 10bit-CHD"><b>Sample Movie 2008 1080p BluRay x265 10bit-CHD</b></a><br />样片 10bit HEVC版本</td><td class="rowfollow" align="center">6.63<br />GB</td><td class="rowfollow" align="center">597</td><td class="rowfollow" align="center">0</td><td class="rowfollow" align="center">0.00<br />KB</td><td class="rowfollow" align="center">0.00<br />KB</td><td class="rowfollow" align="center">---</td></tr>
<tr><td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="torrents.php?allsec=1&amp;cat=401"><img class="c_movies" src="pic/cattrans.gif" alt="Movies" title="Movies" /></a></td>
<td class="rowfollow" width="100%" align="left"><a href="details.php?id=600102&amp;hit=1" title="Sample Story 1940 Blu-ray 2160p UHD"><b>Sample Story 1940 Blu-ray 2160p UHD</b></a><br />样例故事</td><td class="rowfollow" align="center">1.50<br />TB</td><td class="rowfollow" align="center">20</td><td class="rowfollow" align="center">1</td><td class="rowfollow" align="center">1.20<br />GB</td><td class="rowfollow" align="center">0.00<br />KB</td><td class="rowfollow" align="center">Inf.</td></tr>
</table>
`

func getPTCHDBitsDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("ptchdbits")
	require.True(t, ok, "ptchdbits definition not found")
	return def
}

// newPTCHDBitsSite 按真正运行时的建法建站点：SiteFactory 把定义里的选择器并进默认值（优惠映射、副标题的取法都要用得上）。
func newPTCHDBitsSite(t *testing.T, baseURL string) v2.Site {
	t.Helper()
	site, err := v2.NewSiteFactory(nil).CreateSite(v2.SiteConfig{
		ID: "ptchdbits", BaseURL: baseURL, Options: json.RawMessage(`{"cookie":"test_cookie=1"}`),
	})
	require.NoError(t, err)
	return site
}

func ptchdbitsCST(value string) time.Time {
	tm, err := time.ParseInLocation(time.DateTime, value, time.FixedZone("CST", 8*3600))
	if err != nil {
		panic(err)
	}
	return tm
}

func testPTCHDBitsSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(ptchdbitsSearchFixture))
	}))
	defer server.Close()

	items, err := newPTCHDBitsSite(t, server.URL).Search(context.Background(), v2.SearchQuery{FreeOnly: true})
	require.NoError(t, err)
	require.Len(t, items, 6)
	before := ptchdbitsCST("2026-06-27 12:00:00")

	// 文字显示的限时免费：结束时间取标题后面的「(限时<span title>)」；H3 圆圈是 H&R；副标题不带标签与圆圈里的字
	free := items[0]
	assert.Equal(t, "600101", free.ID)
	assert.Equal(t, "Sample.Movie.2025.REMUX.Bluray.1080p.AVC.DTS-HDMA5.1-CHD", free.Title)
	assert.Equal(t, "样片/Sample Movie", free.Subtitle)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	assert.True(t, free.DiscountEndTime.Equal(ptchdbitsCST("2026-06-28 09:11:50")), free.DiscountEndTime)
	assert.Equal(t, v2.FreeUntil, free.Promotion(before).Validity)
	assert.True(t, free.HasHR)
	assert.InDelta(t, 28.35*(1<<30), float64(free.SizeBytes), 1)
	assert.Equal(t, 15, free.Seeders)
	assert.Equal(t, 3, free.Leechers)
	assert.Equal(t, 22, free.Snatched)
	assert.Equal(t, "Movies", free.Category)

	// 文字显示的 50%（前面的「热门」不是促销）、H5
	half := items[1]
	assert.Equal(t, "600102", half.ID)
	assert.Equal(t, v2.DiscountPercent50, half.DiscountLevel)
	assert.True(t, half.DiscountEndTime.IsZero())
	assert.True(t, half.HasHR)
	assert.Equal(t, "样例 / Sample Show [DIY 简繁字幕]", half.Subtitle)

	// 没写限时的免费：结束时间未知，不当成长期免费；行背景 halfdown_bg 是置顶级别，不是 50%
	noEnd := items[2]
	assert.Equal(t, v2.DiscountFree, noEnd.DiscountLevel)
	assert.True(t, noEnd.DiscountEndTime.IsZero())
	assert.Equal(t, v2.FreeUnknown, noEnd.Promotion(before).Validity)
	assert.False(t, noEnd.HasHR)
	assert.Equal(t, "韩剧：样例 第4集 | 类型:剧情 | 内封官方中字", noEnd.Subtitle)

	// 只有「热门」：没有优惠；副标题里的实体还原
	plain := items[3]
	assert.Equal(t, v2.DiscountNone, plain.DiscountLevel)
	assert.False(t, plain.HasHR)
	assert.Equal(t, "猫和老鼠 Tom & Jerry <特别版> Grey's 第1集", plain.Subtitle)

	// 图标显示：pro_50pctdown2up 是 2X 50%，不是 50%
	icon := items[4]
	assert.Equal(t, v2.Discount2x50, icon.DiscountLevel)
	assert.True(t, icon.DiscountEndTime.Equal(ptchdbitsCST("2026-06-29 10:00:00")), icon.DiscountEndTime)
	assert.Equal(t, "样品", icon.Subtitle)

	// 文字显示的 2X 免费：按最长的关键词认，不当成免费或 2X
	double := items[5]
	assert.Equal(t, v2.Discount2xFree, double.DiscountLevel)
	assert.True(t, double.DiscountEndTime.Equal(ptchdbitsCST("2026-06-30 12:00:00")), double.DiscountEndTime)
	assert.Empty(t, double.Subtitle)
	assert.Equal(t, 1, double.Seeders)
}

func testPTCHDBitsDetail(t *testing.T) {
	def := getPTCHDBitsDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	t.Run("文字显示的限时免费、H3", func(t *testing.T) {
		info := parser.ParseAll(FixtureDoc(t, "ptchdbits_detail", ptchdbitsDetailFixture).Selection)
		assert.Equal(t, "600101", info.TorrentID)
		assert.Equal(t, "Sample.Movie.2025.REMUX.Bluray.1080p.AVC.DTS-HDMA5.1-CHD", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		assert.True(t, info.DiscountEnd.Equal(ptchdbitsCST("2026-06-28 09:11:50")), info.DiscountEnd)
		assert.InDelta(t, 28.35*1024, info.SizeMB, 0.1)
		assert.True(t, info.HasHR, "「基本信息」里的 H&R: 3day")
	})

	t.Run("50%、H5", func(t *testing.T) {
		info := parser.ParseAll(FixtureDoc(t, "ptchdbits_detail_halfdown", ptchdbitsDetailHalfdownFixture).Selection)
		assert.Equal(t, "600102", info.TorrentID)
		assert.Equal(t, v2.DiscountPercent50, info.DiscountLevel)
		assert.True(t, info.DiscountEnd.IsZero())
		assert.InDelta(t, 77.87*1024, info.SizeMB, 0.1)
		assert.True(t, info.HasHR, "「基本信息」里的 H&R: 5day")
	})

	t.Run("图标显示的免费、不是 H&R", func(t *testing.T) {
		info := parser.ParseAll(FixtureDoc(t, "ptchdbits_detail_icon", ptchdbitsDetailIconFixture).Selection)
		assert.Equal(t, "600103", info.TorrentID)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		assert.True(t, info.DiscountEnd.Equal(ptchdbitsCST("2026-06-28 09:11:50")), info.DiscountEnd)
		assert.InDelta(t, 3.34*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR, "页头的「H&R:」链接不算")
	})

	// H&R 种子按较长的 H5 算做种要求
	assert.Equal(t, 120, v2.ResolveHRSeedTimeH(def, 10<<30, 0))
	assert.False(t, def.HREnabled, "整站不是 H&R")
}

func testPTCHDBitsUserInfo(t *testing.T) {
	def := getPTCHDBitsDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "ptchdbits_index", ptchdbitsIndexFixture)
		fields := map[string]string{
			"id":             "126666",
			"name":           "testuser",
			"bonus":          "123456.7",
			"seedingBonus":   "54321",
			"ratio":          "12.345",
			"uploaded":       "13573471044894",
			"downloaded":     "1099511627776",
			"seeding":        "12",
			"leeching":       "1",
			"hnrUnsatisfied": "2",
		}
		for field, expected := range fields {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok, "selector %q missing", field)
			assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel), "field %q", field)
		}
	})

	t.Run("MybonusPage", func(t *testing.T) {
		doc := FixtureDoc(t, "ptchdbits_mybonus", ptchdbitsMybonusFixture)
		assert.Equal(t, "68.6", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["bonusPerHour"]),
			"取「合计获得魔力值F」那句，不是第一句的做种魔力值")
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "ptchdbits_userdetails", ptchdbitsUserdetailsFixture)
		expected := map[string]string{
			"levelName":    "(高烧)Veteran User",
			"joinTime":     "1704135845",
			"lastAccessAt": "1782527624",
		}
		for field, want := range expected {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok, "selector %q missing", field)
			assert.Equal(t, want, driver.ExtractFieldValuePublic(doc, sel), "field %q", field)
		}
	})

	t.Run("LevelNames", func(t *testing.T) {
		for name, id := range map[string]int{
			"(未烧)User":         1,
			"(感冒)Power User":   2,
			"(高烧)Veteran User": 6,
			"(骨灰)Nexus Master": 9,
			"贵宾(VIP)":          v2.MinVipLevelID,
		} {
			assert.Equal(t, id, v2.GuessUserLevelID(&v2.UserInfo{LevelName: name}, def.LevelRequirements), name)
		}
	})

	// 整个流程按真正运行时的建法走：首页、魔力值页、用户详情页、做种列表
	t.Run("GetUserInfo", func(t *testing.T) {
		pages := map[string]string{
			"/index.php":                  ptchdbitsIndexFixture,
			"/mybonus.php":                ptchdbitsMybonusFixture,
			"/userdetails.php":            ptchdbitsUserdetailsFixture,
			"/getusertorrentlistajax.php": ptchdbitsSeedingFixture,
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			page, ok := pages[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			if r.URL.Path == "/userdetails.php" && r.URL.Query().Get("id") != "126666" {
				http.Error(w, "unexpected user id", http.StatusBadRequest)
				return
			}
			if r.URL.Path == "/getusertorrentlistajax.php" && (r.URL.Query().Get("userid") != "126666" || r.URL.Query().Get("type") != "seeding") {
				http.Error(w, "unexpected seeding query", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(page))
		}))
		defer server.Close()

		info, err := newPTCHDBitsSite(t, server.URL).GetUserInfo(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "126666", info.UserID)
		assert.Equal(t, "testuser", info.Username)
		assert.InDelta(t, 12.345, info.Ratio, 0.0001)
		assert.InDelta(t, 123456.7, info.Bonus, 0.01)
		assert.InDelta(t, 54321.0, info.SeedingBonus, 0.01)
		assert.InDelta(t, 68.6, info.BonusPerHour, 0.001)
		assert.Equal(t, 2, info.HnRUnsatisfied)
		assert.Equal(t, 12, info.Seeding)
		assert.Equal(t, "(高烧)Veteran User", info.LevelName)
		assert.Equal(t, int64(1704135845), info.JoinDate)
		assert.Positive(t, info.LastAccess)
		assert.InDelta(t, 6.63*(1<<30)+1.50*(1<<40), float64(info.SeederSize), 2)
	})
}

func TestPTCHDBits_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":          ptchdbitsSearchFixture,
		"detail":          ptchdbitsDetailFixture,
		"detail_halfdown": ptchdbitsDetailHalfdownFixture,
		"detail_icon":     ptchdbitsDetailIconFixture,
		"index":           ptchdbitsIndexFixture,
		"userdetails":     ptchdbitsUserdetailsFixture,
		"mybonus":         ptchdbitsMybonusFixture,
		"seeding":         ptchdbitsSeedingFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
			assert.False(t, strings.Contains(data, "passkey="), "%s: 不能带 passkey", name)
		})
	}
}
