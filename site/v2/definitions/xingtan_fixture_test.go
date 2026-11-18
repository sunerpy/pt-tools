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
		SiteID:   "xingtan",
		Search:   testXingtanSearch,
		Detail:   testXingtanDetail,
		UserInfo: testXingtanUserInfo,
	})
}

// --- Fixtures ---
//
// 结构照搬 issue #880 附带的真实页面，标题、作者、出版社、种子 ID、用户名、用户 ID、勋章图片与统计数值
// 都换成了虚构值，审核状态的 SVG 图标只保留一段很短的 path。
//
// 搜索页覆盖的真实特征：
//   - 没有海报缩略图；名称单元格末尾有审核状态 <span title="通过|未审"><svg>…</svg></span>，排在 <br /> 前面
//   - 副标题是 <br /> 之后的裸文本，前面有「审7」「确认」「官方」这类 <span title=""> 标签，后面跟做种进度条 <div>
//   - 名称子表里多一个时魔估算单元格 div.bonus-data；列表在「发布者」后面还有「编辑者」「行为」两列
//   - 限时免费的图标带 onmouseover 提示；永久 2X 免费、50% 的图标只有 title

const xingtanPassSpan = `<span style="margin-left: 6px" title="通过"><svg t="1655145688503" class="icon" viewBox="0 0 1413 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" p-id="16225" width="16" height="16"><path d="M1381.8 107.4L1274.6 0.4 465.2 809.8l-322.6-322.7L35.4 594.1l430 430z" fill="#1afa29" p-id="16226"></path></svg></span>`

const xingtanPendingSpan = `<span style="margin-left: 6px" title="未审"><svg t="1655184824967" class="icon" viewBox="0 0 1024 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" p-id="34118" width="16" height="16"><path d="M450.2 772.2l0 92.5 92.5 0 0-92.5L450.2 772.2z" p-id="34119" fill="#e78d0f"></path></svg></span>`

const xingtanSearchFixture = `<html><body>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%">
<tr>
<td class="colhead" style="padding: 0px">类型</td>
<td class="colhead"><a href="?sort=1&amp;type=asc">标题</a> </td>
<td class="colhead"><a href="?sort=3&amp;type=desc"><img class="comments" src="pic/trans.gif" alt="comments" title="评论数"/></a></td>
<td class="colhead"><a href="?sort=4&amp;type=desc"><img class="time" src="pic/trans.gif" alt="time" title="存活时间"/></a> </td>
<td class="colhead"><a href="?sort=5&amp;type=desc"><img class="size" src="pic/trans.gif" alt="size" title="大小"/></a></td>
<td class="colhead"><a href="?sort=7&amp;type=desc"><img class="seeders" src="pic/trans.gif" alt="seeders" title="种子数"/></a></td>
<td class="colhead"><a href="?sort=8&amp;type=desc"><img class="leechers" src="pic/trans.gif" alt="leechers" title="下载数"/></a></td>
<td class="colhead"><a href="?sort=6&amp;type=desc"><img class="snatched" src="pic/trans.gif" alt="snatched" title="完成数"/></a></td>
<td class="colhead"><a href="?sort=9&amp;type=desc">发布者</a> <td class="colhead"><a href="?sort=10&amp;type=desc">编辑者</a> </td>
<td class="colhead">行为</td>
</tr>
<tr style="background-color: #89c9e6">
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=947"><img src="pic/cattrans.gif" alt="高等教育" title="高等教育" style="background-image: url(pic/category/jiaoyuxue/scenetorrents/gdjy.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr style="background-color: #89c9e6"><td class="embedded" style='padding-left: 5px'><img class="sticky" src="pic/trans.gif" alt="Sticky" title="一级置顶" />&nbsp;<img class="sticky" src="pic/trans.gif" alt="Sticky" title="一级置顶" />&nbsp;<a title="虚构专业目录（示例版）" href="details.php?id=99201&amp;hit=1"><b>虚构专业目录（示例版）</b></a> <b>[<font class='hot'>热门</font>]</b>` + xingtanPassSpan + `<br /><span style="background-color:#000033;color:#ffffff;border-radius:5px;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">审7</span>作者：示例作者/示例出版社/2024【PDF】<div style="padding: 1px;margin-top: 2px;border: 1px solid #838383" title="seeding 100%"><div style="width: 100%;background-color: green;height: 2px"></div></div></td><td class="embedded" style="text-align: right; width: 120px;padding: 4px"><div style="display: flex;"><div class='bonus-data' data-num-bonus='0.02' data-size-bonus='{&quot;size_bonus&quot;:&quot;0.0002&quot;,&quot;l_bonus&quot;:&quot;500.00&quot;,&quot;A&quot;:&quot;300.0&quot;}'></div><div class="bonus-result"></div></div></td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=99201"><img class="download" src="pic/trans.gif" style='padding-bottom: 2px;' alt="download" title="下载本种" /></a><br /><a id="bookmark0" href="javascript: bookmark(99201,0);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=99201&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2024-12-11 21:34:30">1年<br />7月</span></td><td class="rowfollow">273.39<br />KB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99201&amp;hit=1&amp;dllist=1#seeders">30</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><a href="viewsnatches.php?id=99201"><b>150</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
<td class="rowfollow"><span class="nowrap"><a href="https://xingtan.one/userdetails.php?id=90009" class='Auditor_Name'><b>fixture_editor</b></a><img src="pic/medals/fixture-medal.png" title="虚构勋章" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"/></span></td>
<td class="rowfollow"></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=812"><img src="pic/cattrans.gif" alt="工学" title="工学" style="background-image: url(pic/category/jiaoyuxue/scenetorrents/gx.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style='padding-left: 5px'><a title="虚构控制工程基础-第2版" href="details.php?id=99202&amp;hit=1"><b>虚构控制工程基础-第2版</b></a> <img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-07-04 22:33:44&quot;&gt;1天17时&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000,'fade','both','styleClass','niceTitle', 'fadeMax',87, 'maxWidth', 300);" /> <font color='#0000FF'>剩余时间：<span title="2026-07-04 22:33:44">1天17时</span></font>` + xingtanPendingSpan + `<br /><span style="background-color:#006400;color:#ffffff;border-radius:5px;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">确认</span>作者：示例作者/示例大学出版社/2008【EPUB】</td><td class="embedded" style="text-align: right; width: 120px;padding: 4px"><div style="display: flex;"><div class='bonus-data' data-num-bonus='2.19' data-size-bonus='{&quot;size_bonus&quot;:&quot;0.0001&quot;,&quot;l_bonus&quot;:&quot;500.00&quot;,&quot;A&quot;:&quot;300.0&quot;}'></div><div class="bonus-result"></div></div></td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=99202"><img class="download" src="pic/trans.gif" style='padding-bottom: 2px;' alt="download" title="下载本种" /></a><br /><a id="bookmark1" href="javascript: bookmark(99202,1);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=99202&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-07-02 23:37:52">7时<br />37分</span></td><td class="rowfollow">9.28<br />MB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99202&amp;hit=1&amp;dllist=1#seeders">1</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=99202&amp;hit=1&amp;dllist=1#leechers">4</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><span class="nowrap"><a href="https://xingtan.one/userdetails.php?id=90010" class='User_Name'><b>fixture_uploader</b></a></span></td>
<td class="rowfollow"></td>
<td class="rowfollow"></td>
</tr>
<tr style="background-color: #89c9e6">
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=820"><img src="pic/cattrans.gif" alt="电子信息" title="电子信息" style="background-image: url(pic/category/jiaoyuxue/scenetorrents/dzxx.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr style="background-color: #89c9e6"><td class="embedded" style='padding-left: 5px'><img class="sticky" src="pic/trans.gif" alt="Sticky" title="一级置顶" />&nbsp;<a title="【示例】虚构开发板资料（含软件）" href="details.php?id=99203&amp;hit=1"><b>【示例】虚构开发板资料（含软件）</b></a> <b>[<font class='hot'>热门</font>]</b> <img class="pro_free2up" src="pic/trans.gif" alt="2X Free" title="2X免费" />` + xingtanPassSpan + `<br /><span style="background-color:#0000ff;color:#ffffff;border-radius:5px;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">官方</span><span style="background-color:#009900;color:#ffffff;border-radius:5px;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">首发</span>【示例】虚构开发板资料工具/AB盘 (2022)/【PDF】【RAR】等<div style="padding: 1px;margin-top: 2px;border: 1px solid #838383" title="seeding 100%"><div style="width: 100%;background-color: green;height: 2px"></div></div></td><td class="embedded" style="text-align: right; width: 120px;padding: 4px"><div style="display: flex;"><div class='bonus-data' data-num-bonus='0.5' data-size-bonus='{&quot;size_bonus&quot;:&quot;1.4&quot;,&quot;l_bonus&quot;:&quot;500.00&quot;,&quot;A&quot;:&quot;300.0&quot;}'></div><div class="bonus-result"></div></div></td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=99203"><img class="download" src="pic/trans.gif" style='padding-bottom: 2px;' alt="download" title="下载本种" /></a><br /><a id="bookmark2" href="javascript: bookmark(99203,2);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><b><a href="details.php?id=99203&amp;hit=1&amp;cmtpage=1#startcomments" >12</a></b></td><td class="rowfollow nowrap"><span title="2023-09-10 11:12:13">2年<br />10月</span></td><td class="rowfollow">41.71<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99203&amp;hit=1&amp;dllist=1#seeders">88</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=99203&amp;hit=1&amp;dllist=1#leechers">2</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=99203"><b>640</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
<td class="rowfollow"></td>
<td class="rowfollow"></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=791"><img src="pic/cattrans.gif" alt="小学教育" title="小学教育" style="background-image: url(pic/category/jiaoyuxue/scenetorrents/xxjy.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style='padding-left: 5px'><a title="虚构课本练习合集" href="details.php?id=99204&amp;hit=1"><b>虚构课本练习合集</b></a> <img class="pro_50pctdown" src="pic/trans.gif" alt="50%" title="50%" />` + xingtanPassSpan + `<br /><span style="background-color:#000033;color:#ffffff;border-radius:5px;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">审10</span>作者：示例主编/示例出版社/2020【PDF】<div style="padding: 1px;margin-top: 2px;border: 1px solid #838383" title="seeding 100%"><div style="width: 100%;background-color: green;height: 2px"></div></div></td><td class="embedded" style="text-align: right; width: 120px;padding: 4px"><div style="display: flex;"><div class='bonus-data' data-num-bonus='0.06' data-size-bonus='{&quot;size_bonus&quot;:&quot;1.4&quot;,&quot;l_bonus&quot;:&quot;500.00&quot;,&quot;A&quot;:&quot;300.0&quot;}'></div><div class="bonus-result"></div></div></td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=99204"><img class="download" src="pic/trans.gif" style='padding-bottom: 2px;' alt="download" title="下载本种" /></a><br /><a id="bookmark3" href="javascript: bookmark(99204,3);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=99204&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2024-12-10 11:56:12">1年<br />7月</span></td><td class="rowfollow">2.73<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99204&amp;hit=1&amp;dllist=1#seeders">53</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><a href="viewsnatches.php?id=99204"><b>325</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
<td class="rowfollow"></td>
<td class="rowfollow"></td>
</tr>
</table>
</body></html>`

// 详情页（限时免费）：没有 torrent_name / detail_torrent_id 隐藏域，标题只能取 h1#top 的文字，
// h1 末尾是「&nbsp;&nbsp;&nbsp; [免费] 剩余时间：1天17时」再加审核状态「未审」的 SVG；
// 种子编号取快速评论表单里的 input[name='pid']。
const xingtanDetailFixture = `<html><body>
<h1 align="center" id="top">虚构控制工程基础-第2版&nbsp;&nbsp;&nbsp; <b>[<font class='free' >免费</font>]</b> <font color='#0000FF'>剩余时间：<span title="2026-07-04 22:33:44">1天17时</span></font>` + xingtanPendingSpan + `</h1>
<table class="main" width="100%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead" width="13%">下载</td><td class="rowfollow" width="87%" align="left"><a class="index" href="download.php?id=99202">[XT].虚构控制工程基础 (示例作者) .epub.torrent</a>&nbsp;&nbsp;<a id="bookmark0" href="javascript: bookmark(99202,0);"><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a>&nbsp;&nbsp;&nbsp;由&nbsp;<span class="nowrap"><a href="https://xingtan.one/userdetails.php?id=90010" class='User_Name'><b>fixture_uploader</b></a> (<b class='User_Name'>童生</b>)</span>发布于<span title="2026-07-02 23:37:52">7时37分钟前</span></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">作者：示例作者/示例大学出版社/2008【EPUB】</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>9.28 MB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;专业区:机械设计制造及其自动化&nbsp;&nbsp;&nbsp;<b>类型: </b>书籍&nbsp;&nbsp;&nbsp;<b>格式: </b>EPUB</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">种子链接</td><td class="rowfollow" valign="top" align="left"><a href="details.php?id=99202">details.php?id=99202</a></td></tr>
</table>
<form id="compose" name="comment" method="post" action="comment.php?action=add&amp;type=torrent" onsubmit="return postvalid(this);"><input type="hidden" name="pid" value="99202" /><br /><textarea name='body' cols="100" rows="8" style="width: 450px"></textarea></form>
</body></html>`

// 详情页（无促销）：h1 只有标题和审核状态「通过」的 SVG。
const xingtanDetailPlainFixture = `<html><body>
<h1 align="center" id="top">虚构专业目录（示例版）` + xingtanPassSpan + `</h1>
<table class="main" width="100%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead" width="13%">下载</td><td class="rowfollow" width="87%" align="left"><a class="index" href="download.php?id=99201">[XT].虚构专业目录（示例版）.pdf.torrent</a>&nbsp;&nbsp;&nbsp;由&nbsp;<i>匿名</i>发布于<span title="2024-12-11 21:34:30">1年7月前</span></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">作者：示例作者/示例出版社/2024【PDF】</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>273.39 KB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;专业区:高等教育&nbsp;&nbsp;&nbsp;<b>类型: </b>文档&nbsp;&nbsp;&nbsp;<b>格式: </b>PDF</td></tr>
</table>
<form id="compose" name="comment" method="post" action="comment.php?action=add&amp;type=torrent" onsubmit="return postvalid(this);"><input type="hidden" name="pid" value="99201" /><br /><textarea name='body' cols="100" rows="8" style="width: 450px"></textarea></form>
</body></html>`

// index.php：页头的魔力值叫「杏仁值」，后面跟着每小时的杏仁值「[12.5<i …></i>/h]」，没有 H&R 计数器。
const xingtanIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%">
    <tr>
        <td>
            <table width="100%" cellspacing="0" cellpadding="0" border="0">
                <tr>
                    <td class="bottom" align="left">
                        <span class="medium">
                            欢迎回来, <span class="nowrap"><a href="https://xingtan.one/userdetails.php?id=90001" class='EliteUser_Name'><b>fixture_user</b></a><img src="pic/medals/fixture-medal.png" title="虚构勋章" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"/></span>                            [<a href="logout.php">退出</a>]
                            [<a href="usercp.php">&nbsp;控制面板&nbsp;</a>]
                            <font class='color_class'>等级 </font>:[<a
                                href="class.php"><b class='EliteUser_Name'>秀才</b></a>]

                            <font class='color_bonus'>杏仁值 </font>[<a
                                href="mybonus.php">使用</a>]: 12,345.6<i class='fa-brands fa-fulcrum'></i>[12.5<i class='fa-brands fa-fulcrum'></i>/h]                            <a href="attendance.php" class="">[签到已得100, 补签卡: 1]</a>                            <a href="medal.php">[勋章]</a>
                            <font class='color_invite'>邀请 </font>[<a
                                href="invite.php?id=90001">发送</a>]: 1(0)                            <br/>
                            <font class="color_ratio">分享率:</font> 1.230                            <font
                                class='color_uploaded'>上传量:</font> 1.200 TB                            <font
                                class='color_downloaded'> 下载量:</font> 999.00 GB                            <font class='color_active'>当前活动:</font> <img
                                class="arrowup" alt="Torrents seeding"
                                title="当前做种"
                                src="pic/trans.gif"/>123 <img class="arrowdown" alt="Torrents leeching"
                                                           title="当前下载"
                                                           src="pic/trans.gif"/>2&nbsp;&nbsp;
                            <font class='color_slots'>连接数：</font>无限制                            <a href='#' onclick="get_keeper_rss()" style="font-weight: bold;color:indianred">[保种链接]</a>
                        </span>
                    </td>
                </tr>
            </table>
        </td>
    </tr>
</table>
</body></html>`

// userdetails.php：魔力值一行叫「杏仁值」，后面没有 [明细]；「最近动向」之后依次是空的「最近动向(地点)」
// 和「最近活跃」（做种活动时间），最近访问时间要取「最近动向」。
const xingtanUserdetailsFixture = `<html><body>
<table class="main" width="100%" border="1" cellspacing="0" cellpadding="5">
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">用户ID/UID</td><td width="99%" class="rowfollow" valign="top" align="left">90001</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">加入日期</td><td width="99%" class="rowfollow" valign="top" align="left">2024-03-05 06:07:08 (<span title="2024-03-05 06:07:08">2年4月前</span>, 121周)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-07-02 09:10:11 (<span title="2026-07-02 09:10:11">&lt; 1分钟前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向(地点)</td><td width="99%" class="rowfollow" valign="top" align="left"></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近活跃</td><td width="99%" class="rowfollow" valign="top" align="left">2026-07-02 08:40:00 (<span title="2026-07-02 08:40:00">30分钟前</span>,seed)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">传输</td><td width="99%" class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>: <font color="">1.230</font>（<strong>实际分享率</strong>：0.305）</td><td class="embedded">&nbsp;&nbsp;<img src="pic/smilies/2.gif" alt="" /></td></tr><tr><td class="embedded"><strong>上传量</strong>: 1.200 TB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>: 999.00 GB</td></tr><tr><td class="embedded"><strong>实际上传量</strong>: 1.500 TB</td><td class="embedded">&nbsp;&nbsp;<strong>实际下载量</strong>: 4.920 TB</td><td class="embedded text-muted">&nbsp;&nbsp;实际上传/下载量 (仅用于记录, 不参与分享率计算)</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">等级</td><td width="99%" class="rowfollow" valign="top" align="left"><img alt="Elite User" title="Elite User" src="pic/elite.gif" /> </td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">杏仁值</td><td width="99%" class="rowfollow" valign="top" align="left">12,345.6</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">做种积分</td><td width="99%" class="rowfollow" valign="top" align="left">23,456.7&nbsp;&nbsp;<span class='text-muted'>(更新时间: 2026-07-02 06:18:24)</span></td></tr>
</table>
</body></html>`

// --- Helpers ---

func getXingtanDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("xingtan")
	require.True(t, ok, "xingtan definition not found")
	return def
}

// --- Suite: Search ---

func testXingtanSearch(t *testing.T) {
	def := getXingtanDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(xingtanSearchFixture))
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

	plain := items[0]
	assert.Equal(t, "99201", plain.ID)
	assert.Equal(t, "虚构专业目录（示例版）", plain.Title)
	// 审核状态的 SVG 在 <br /> 前面，「审7」标签在副标题前面，进度条 <div> 在后面
	assert.Equal(t, "作者：示例作者/示例出版社/2024【PDF】", plain.Subtitle)
	assert.Equal(t, v2.DiscountNone, plain.DiscountLevel)
	assert.True(t, plain.DiscountEndTime.IsZero())
	// "273.39<br />KB" 按 1024 进制换算
	assert.Equal(t, int64(279951), plain.SizeBytes)
	assert.Equal(t, 30, plain.Seeders)
	assert.Equal(t, 0, plain.Leechers)
	assert.Equal(t, 150, plain.Snatched)
	assert.Equal(t, "高等教育", plain.Category)
	assert.Positive(t, plain.UploadedAt)
	assert.False(t, plain.HasHR)

	free := items[1]
	assert.Equal(t, "99202", free.ID)
	assert.Equal(t, "虚构控制工程基础-第2版", free.Title)
	assert.Equal(t, "作者：示例作者/示例大学出版社/2008【EPUB】", free.Subtitle)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	// 结束时间来自促销图标的 onmouseover 提示
	require.False(t, free.DiscountEndTime.IsZero(), "discount end time should come from onmouseover tooltip")
	assert.Equal(t, "2026-07-04 22:33:44", free.DiscountEndTime.Format("2006-01-02 15:04:05"))
	assert.Equal(t, int64(9730785), free.SizeBytes)
	assert.Equal(t, 1, free.Seeders)
	assert.Equal(t, 4, free.Leechers)
	assert.Equal(t, 0, free.Snatched)
	assert.Equal(t, "工学", free.Category)

	twoUpFree := items[2]
	assert.Equal(t, "99203", twoUpFree.ID)
	assert.Equal(t, "【示例】虚构开发板资料工具/AB盘 (2022)/【PDF】【RAR】等", twoUpFree.Subtitle)
	assert.Equal(t, v2.Discount2xFree, twoUpFree.DiscountLevel)
	// 永久 2X 免费的图标没有 onmouseover，也没有剩余时间
	assert.True(t, twoUpFree.DiscountEndTime.IsZero(), "permanent promotion has no end time")
	assert.Equal(t, int64(44785771479), twoUpFree.SizeBytes)
	assert.Equal(t, 88, twoUpFree.Seeders)
	assert.Equal(t, 2, twoUpFree.Leechers)
	assert.Equal(t, 640, twoUpFree.Snatched)

	halfDown := items[3]
	assert.Equal(t, "99204", halfDown.ID)
	assert.Equal(t, "作者：示例主编/示例出版社/2020【PDF】", halfDown.Subtitle)
	assert.Equal(t, v2.DiscountPercent50, halfDown.DiscountLevel)
	assert.True(t, halfDown.DiscountEndTime.IsZero())
	assert.Equal(t, int64(2931315179), halfDown.SizeBytes)
	assert.Equal(t, 53, halfDown.Seeders)
	assert.Equal(t, 325, halfDown.Snatched)
	assert.Equal(t, "小学教育", halfDown.Category)
}

// --- Suite: Detail ---

func testXingtanDetail(t *testing.T) {
	def := getXingtanDef(t)

	t.Run("TimedFree", func(t *testing.T) {
		doc := FixtureDoc(t, "xingtan_detail", xingtanDetailFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		// 种子编号来自快速评论表单的 pid
		assert.Equal(t, "99202", info.TorrentID)
		// h1 的文字末尾是「[免费] 剩余时间：1天17时」，标题里不能留下促销与剩余时间
		assert.Equal(t, "虚构控制工程基础-第2版", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		// 结束时间只认 <font> 里的 span，不能取到审核状态 <span title="未审">
		require.False(t, info.DiscountEnd.IsZero(), "discount end time should be parsed")
		assert.Equal(t, "2026-07-04 22:33:44", info.DiscountEnd.Format("2006-01-02 15:04:05"))
		assert.Empty(t, info.DiscountErr, "the review-status span must not be read as the end time")
		assert.InDelta(t, 9.28, info.SizeMB, 0.01)
		assert.False(t, info.HasHR)

		assert.Equal(t, "download.php?id=99202",
			doc.Find(def.Selectors.DetailDownloadLink).First().AttrOr("href", ""))
		assert.Equal(t, "作者：示例作者/示例大学出版社/2008【EPUB】",
			strings.TrimSpace(doc.Find(def.Selectors.DetailSubtitle).First().Text()))
	})

	t.Run("NoPromotion", func(t *testing.T) {
		doc := FixtureDoc(t, "xingtan_detail_plain", xingtanDetailPlainFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "99201", info.TorrentID)
		assert.Equal(t, "虚构专业目录（示例版）", info.Title)
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel)
		assert.True(t, info.DiscountEnd.IsZero())
		assert.Empty(t, info.DiscountErr)
		assert.InDelta(t, 273.39/1024, info.SizeMB, 0.001)
		assert.False(t, info.HasHR)
	})
}

// --- Suite: UserInfo ---

func testXingtanUserInfo(t *testing.T) {
	def := getXingtanDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "xingtan_index", xingtanIndexFixture)
		fields := map[string]string{
			"id":       "90001",
			"name":     "fixture_user",
			"seeding":  "123",
			"leeching": "2",
			// 「[12.5<i …></i>/h]」里的每小时杏仁值
			"bonusPerHour": "12.5",
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
		doc := FixtureDoc(t, "xingtan_userdetails", xingtanUserdetailsFixture)
		fields := map[string]string{
			// 1.200 TB / 999.00 GB / 1.500 TB / 4.920 TB 按 1024 进制换算为字节
			"uploaded":       "1319413953331",
			"downloaded":     "1072668082176",
			"trueUploaded":   "1649267441664",
			"trueDownloaded": "5409597208657",
			"ratio":          "1.230",
			"levelName":      "Elite User",
			// 「杏仁值」行就是魔力值
			"bonus":        "12345.6",
			"seedingBonus": "23456.7",
			"joinTime":     "1709590028",
			// 取「最近动向」，不是后面的「最近活跃」
			"lastAccessAt": "1782954611",
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

func TestXingtan_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":       xingtanSearchFixture,
		"detail":       xingtanDetailFixture,
		"detail_plain": xingtanDetailPlainFixture,
		"index":        xingtanIndexFixture,
		"userdetails":  xingtanUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
