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
		SiteID:   "cspt",
		Search:   testCsptSearch,
		Detail:   testCsptDetail,
		UserInfo: testCsptUserInfo,
	})
}

// --- Fixtures ---
//
// 结构照搬真实页面（Tailwind 皮肤），但所有标题、副标题、种子 ID、用户名、用户 ID
// 与统计数值均已替换为虚构值，勋章图片与邮箱/IP 行整段移除。
//
// 搜索页是 div 布局，需要覆盖的真实特征：
//   - 表头行同样带 torrent-cat / torrent-title / torrent-info，但不带
//     torrent-table-sub-info，也没有 details.php 链接 —— 必须被行选择器排除
//   - 每行三个并列单元格：div.torrent-cat、div.torrent-title、包裹
//     div.torrent-info 的统计列，另有 div.torrent-manage 放收藏/下载
//   - 促销结束时间同时出现在 img.pro_free 的 onmouseover 提示与
//     <font color>剩余时间：<span title> 中
//   - 永久免费的行只有 img.pro_free[title=免费]，既无 onmouseover 也无剩余时间
//   - 标题单元格内还有 <span title="通过"> 状态图标，不能被当成促销结束时间
//   - 副标题有独立类名 torrent-info-text-small_name

const csptSearchFixture = `<html><body>
<div class="torrents">
<div class="flex h-[30px] bg-[main_tittle] m-auto radius-[3px]"><div class="torrent-cat"><span class="text-[fff] font-bold text-[16px] m-auto ">类型</span></div><div class="torrent-title items-center justify-center a:tittle-[fff] a "><a class="text-[fff] text-[16px] font-bold m-auto" href="?sort=1&amp;type=asc">标题</a></div><div class="flex w-[20%] items-center"><div class="torrent-info"><a class="torrent-info-icon" href="?sort=3&amp;type=desc"><img class="comments" src="pic/icc/chat-message.png" alt="comments" title="评论数"/></a><a class="torrent-info-icon" href="?sort=5&amp;type=desc"><img class="size" src="pic/icc/wealth-1.png" alt="size" title="大小"/></a></div></div><div class="torrent-manage"></div> </div>
<div class="flex flex-col w-[100%] m-auto torrent-table-for-spider"><div class="flex bg-[green] position-r torrent-table-sub-info "  style="background-color: #89c9e6"><div class="torrent-cat" >
<div class="rounded-md"><a href="?cat=405"><img class="c_anime" src="pic/cattrans.gif" alt="动漫" title="动漫" style="background-image: url(pic/category/chs/catsprites.png);" /></a></div>
</div><div class="flex torrent-table-for-spider-info torrent-title items-center gap-x-[15px] justify-between"><div class="flex flex-col gap-y-1 w-[calc(100%-40px)]" ><div class="flex items-center flex-wrap"><img class="sticky position-a" src="pic/icc/top-zhiding.png" alt="置顶图标" title="一级置顶" /><a target="_blank" class="flex items-center gap-x-[5px] font-bold text-[fff] text-[9pt] torrent-info-text-name text-start" title="Fixture.Anime.Movie.2026.1080p.WEB-DL.H.265.DDP.5.1-FIXTURE"  href="details.php?id=88801&amp;hit=1"><b class="flex items-center truncate"  >Fixture.Anime.Movie.2026.1080p.WEB-DL.H.265.DDP.5.1-FIXTURE</b></a><div class="flex items-center gap-x-[5px]"><b> (<font class='new'>新</font>)</b> <img class="pro_free" src="pic/trans.gif" alt="Free"  onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-07-31 12:00:00&quot;&gt;2天22时&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000);" /> <font color='#0000FF'>剩余时间：<span title="2026-07-31 12:00:00">2天22时</span></font><span style="margin-left: 6px" title="通过"><svg t="1655145688503" class="icon" viewBox="0 0 1413 1024" width="16" height="16"><path d="M1381 107L1274 0 465 809 142 487 35 594l430 430z" fill="#1afa29"></path></svg></span></div> </div><div class='flex items-center gap-x-[8px]'><div class='overflow-hidden truncate text-[9pt] font-[500] flex-1 text-start torrent-info-text-small_name' >虚构副标题一 | 类型：喜剧/动画 | [英/国]音轨</div></div><div class="flex gap-x-[5px] text-[9pt] items-center pr-5"><div class="flex text-[9pt] text-[000] font-bold items-center"><img src="pic/imdb2.png" alt="imdb" title="imdb" /><span>&nbsp; N/A</span></div><span style="background-color:#0000ff;color:#FFFFFF" title="">官种</span><div class="flex items-center torrent-info-text-author"><i>匿名</i></div>
</div><div class="w-full pr-[10px] mb-[5px]" ></div></div></div><div class="flex w-[20%] items-center" ><div class="torrent-info"><div class="torrent-info-text torrent-info-text-comments"><a href="comment.php?action=add&amp;pid=88801&amp;type=torrent" title="添加评论">0</a></div><div class="torrent-info-text torrent-info-text-added"><span title="2026-07-27 14:07:42">1时6分钟</span></div><div class="torrent-info-text torrent-info-text-size" >12.98 GB</div><div class="torrent-info-text torrent-info-text-seeders" align="center"><b><a href="details.php?id=88801&amp;hit=1&amp;dllist=1#seeders"><font color="#ee0000">1</font></a></b></div>
<div class="torrent-info-text torrent-info-text-leechers"><b><a href="details.php?id=88801&amp;hit=1&amp;dllist=1#leechers">35</a></b></div>
<div class="torrent-info-text torrent-info-text-finished">0</div>
</div></div><div class="torrent-manage"><a id="bookmark0"  href="javascript: bookmark(88801,0);" ><img class="delbookmark" src="pic/icc/cart-add.png" alt="Unbookmarked" title="收藏" /></a><a class="inline-flex" href="download.php?id=88801"><img class="download" src="pic/icc/install-black.png" alt="download" title="下载本种" /></a></div>
</div>
</div>
<div class="flex flex-col w-[100%] m-auto torrent-table-for-spider"><div class="flex bg-[green] position-r torrent-table-sub-info "><div class="torrent-cat" >
<div class="rounded-md"><a href="?cat=401"><img class="c_movie" src="pic/cattrans.gif" alt="电影" title="电影" /></a></div>
</div><div class="flex torrent-table-for-spider-info torrent-title items-center gap-x-[15px] justify-between"><div class="flex flex-col gap-y-1 w-[calc(100%-40px)]" ><div class="flex items-center flex-wrap"><a target="_blank" class="flex items-center gap-x-[5px] font-bold text-[fff] text-[9pt] torrent-info-text-name text-start" title="Fixture.Guide.v2.0-FIXTURE"  href="details.php?id=88802&amp;hit=1"><b class="flex items-center truncate"  >Fixture.Guide.v2.0-FIXTURE</b></a><div class="flex items-center gap-x-[5px]"><span style="margin-left: 6px" title="通过"><svg t="1655145688503" class="icon" viewBox="0 0 1413 1024" width="16" height="16"><path d="M1381 107L1274 0 465 809z" fill="#1afa29"></path></svg></span></div> </div><div class='flex items-center gap-x-[8px]'><div class='overflow-hidden truncate text-[9pt] font-[500] flex-1 text-start torrent-info-text-small_name' >虚构副标题二</div></div><div class="flex items-center torrent-info-text-author"><i>匿名</i></div></div></div><div class="flex w-[20%] items-center" ><div class="torrent-info"><div class="torrent-info-text torrent-info-text-comments"><a href="comment.php?action=add&amp;pid=88802&amp;type=torrent" title="添加评论">3</a></div><div class="torrent-info-text torrent-info-text-added"><span title="2026-05-01 12:00:00">2月26天</span></div><div class="torrent-info-text torrent-info-text-size" >816.30 MB</div><div class="torrent-info-text torrent-info-text-seeders" align="center"><b><a href="details.php?id=88802&amp;hit=1&amp;dllist=1#seeders">206</a></b></div>
<div class="torrent-info-text torrent-info-text-leechers"><b><a href="details.php?id=88802&amp;hit=1&amp;dllist=1#leechers">1</a></b></div>
<div class="torrent-info-text torrent-info-text-finished">0</div>
</div></div><div class="torrent-manage"><a class="inline-flex" href="download.php?id=88802"><img class="download" src="pic/icc/install-black.png" alt="download" title="下载本种" /></a></div>
</div>
</div>
<div class="flex flex-col w-[100%] m-auto torrent-table-for-spider"><div class="flex bg-[green] position-r torrent-table-sub-info "><div class="torrent-cat" >
<div class="rounded-md"><a href="?cat=402"><img class="c_tvseries" src="pic/cattrans.gif" alt="剧集" title="剧集" /></a></div>
</div><div class="flex torrent-table-for-spider-info torrent-title items-center gap-x-[15px] justify-between"><div class="flex flex-col gap-y-1 w-[calc(100%-40px)]" ><div class="flex items-center flex-wrap"><a target="_blank" class="flex items-center gap-x-[5px] font-bold text-[fff] text-[9pt] torrent-info-text-name text-start" title="Fixture.Forever.Free.Pack-FIXTURE"  href="details.php?id=88803&amp;hit=1"><b class="flex items-center truncate"  >Fixture.Forever.Free.Pack-FIXTURE</b></a><div class="flex items-center gap-x-[5px]"><img class="pro_free" src="pic/trans.gif" alt="Free" title="免费" /><span style="margin-left: 6px" title="通过"><svg t="1655145688503" class="icon" viewBox="0 0 1413 1024" width="16" height="16"><path d="M1381 107L1274 0 465 809z" fill="#1afa29"></path></svg></span></div> </div><div class='flex items-center gap-x-[8px]'><div class='overflow-hidden truncate text-[9pt] font-[500] flex-1 text-start torrent-info-text-small_name' >虚构副标题三</div></div></div></div><div class="flex w-[20%] items-center" ><div class="torrent-info"><div class="torrent-info-text torrent-info-text-comments">0</div><div class="torrent-info-text torrent-info-text-added"><span title="2025-12-31 08:00:00">6月27天</span></div><div class="torrent-info-text torrent-info-text-size" >1.57 GB</div><div class="torrent-info-text torrent-info-text-seeders" align="center"><b><a href="details.php?id=88803&amp;hit=1&amp;dllist=1#seeders">12</a></b></div>
<div class="torrent-info-text torrent-info-text-leechers"><b><a href="details.php?id=88803&amp;hit=1&amp;dllist=1#leechers">4</a></b></div>
<div class="torrent-info-text torrent-info-text-finished">88</div>
</div></div><div class="torrent-manage"><a class="inline-flex" href="download.php?id=88803"><img class="download" src="pic/icc/install-black.png" alt="download" title="下载本种" /></a></div>
</div>
</div>
</div>
</body></html>`

// 详情页：常规 table 布局，标题/ID 来自隐藏域；h1 内促销标记是单引号 class='free'，
// 剩余时间包在 <font color> 里，后面紧跟 <span title="通过"> 状态图标；
// 「基本信息」标签文本被 <span class="td-text"> 包裹，值单元格类名为 rowfollow main_rowfollw。
// 顶栏刻意保留 [H&R]: [0/0/10] 计数器，用于验证 HRKeywords 不会把它误判为 H&R 种子。
const csptDetailFixture = `<html><body>
<div class="menu-base-info"><div class="menu-base-info-items">[H&amp;R]:
    [<a href="myhr.php">0/<font color="red">0</font>/10</a>]</div></div>
<h1 align="center" id="top">Fixture.Anime.Movie.2026.1080p.WEB-DL.H.265.DDP.5.1-FIXTURE&nbsp;&nbsp;&nbsp; <b>[<font class='free' >免费</font>]</b> <font color='#0000FF'>剩余时间：<span title="2026-07-31 12:00:00">2天22时</span></font><span style="margin-left: 6px" title="通过"><svg t="1655145688503" class="icon" viewBox="0 0 1413 1024" width="16" height="16"><path d="M1381 107L1274 0 465 809z" fill="#1afa29"></path></svg></span></h1>
<table width="100%" cellspacing="0" cellpadding="5" class="main_table">
<tr><td class="rowhead nowrap main_rowhead" valign="center" align="right"><span class="td-text main_td_text">下载</span></td><td class="rowfollow main_rowfollw" valign="center" align="left"><a class="index" href="download.php?id=88801">[FIXTURE].Fixture.Anime.Movie.2026.1080p.WEB-DL.torrent</a>&nbsp;&nbsp;&nbsp;由&nbsp;<i>匿名</i>发布</td></tr>
<tr><td class="rowhead nowrap main_rowhead" valign="center" align="right"><span class="td-text main_td_text">副标题</span></td><td class="rowfollow main_rowfollw" valign="center" align="left">虚构副标题一 | 类型：喜剧/动画 | [英/国]音轨</td></tr>
<tr><td class="rowhead nowrap main_rowhead" valign="center" align="right"><span class="td-text main_td_text">基本信息</span></td><td class="rowfollow main_rowfollw" valign="center" align="left"><b><b>大小：</b></b>12.98 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;动漫&nbsp;&nbsp;&nbsp;<b>媒介: </b>WEB-DL&nbsp;&nbsp;&nbsp;<b>编码: </b>H.265/HEVC&nbsp;&nbsp;&nbsp;<b>分辨率: </b>1080p</td></tr>
<tr><td class="rowhead nowrap main_rowhead" valign="center" align="right"><span class="td-text main_td_text">行为</span></td><td class="rowfollow main_rowfollw" valign="center" align="left"><a title="下载种子" href="download.php?id=88801"><img class="dt_download" src="pic/trans.gif" alt="download" />&nbsp;<b><font class="small">下载种子</font></b></a></td></tr>
<tr><td class="rowhead nowrap main_rowhead" valign="center" align="right"><span class="td-text main_td_text">字幕</span></td><td class="rowfollow main_rowfollw" valign="center" align="left">该种子暂无字幕<form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.Anime.Movie.2026.1080p.WEB-DL.H.265.DDP.5.1-FIXTURE" /><input class="button-black" type="hidden" name="detail_torrent_id" value="88801" /></form></td></tr>
</table>
</body></html>`

// 无促销的详情页：h1 里没有 font.free，也没有剩余时间。
const csptDetailPlainFixture = `<html><body>
<div class="menu-base-info"><div class="menu-base-info-items">[H&amp;R]:
    [<a href="myhr.php">0/<font color="red">0</font>/10</a>]</div></div>
<h1 align="center" id="top">Fixture.Guide.v2.0-FIXTURE<span style="margin-left: 6px" title="通过"><svg viewBox="0 0 1413 1024" width="16" height="16"><path d="M1381 107L1274 0z" fill="#1afa29"></path></svg></span></h1>
<table width="100%" cellspacing="0" cellpadding="5" class="main_table">
<tr><td class="rowhead nowrap main_rowhead" valign="center" align="right"><span class="td-text main_td_text">基本信息</span></td><td class="rowfollow main_rowfollw" valign="center" align="left"><b><b>大小：</b></b>816.30 MB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;资料</td></tr>
<tr><td class="rowhead nowrap main_rowhead" valign="center" align="right"><span class="td-text main_td_text">字幕</span></td><td class="rowfollow main_rowfollw" valign="center" align="left"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.Guide.v2.0-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="88802" /></form></td></tr>
</table>
</body></html>`

// index.php 顶栏：站点没有 #info_block（源码已注释），个人信息全在 div.menu-base-info。
// 做种/下载数没有 img.arrowup/arrowdown，只能靠 svg <title> 文案定位随后的 <a>；
// 魔力值在本站叫「金元宝」，链接指向 mybonus.php。
const csptIndexFixture = `<html><body>
<div class="user-info-container">
<span class="user-id"><span class="nowrap"><a href="https://cspt.top/userdetails.php?id=90101" class='PowerUser_Name'><b>cspt_fixture_user</b></a></span><span class="uname-medal"></span></span>
<p class="user-Uid">UID：90101</p>
<div class="user-info">
<div class="menu-base-info">
<div class="menu-base-info-items"><div><a href="attendance.php" class="">[签到已得170]</a></div></div>
<div class="menu-base-info-items">[H&amp;R]: [<a href="myhr.php">0/<font color="red">0</font>/10</a>]</div>
<div class="menu-base-info-items"><svg role="img" width="16px" height="16px" viewBox="0 0 24 24" aria-labelledby="arrowUpIconTitle" fill="none"><title id="arrowUpIconTitle">上传量</title><path d="M18 9l-6-6-6 6"/></svg>&nbsp;<a href="userdetails.php?id=90101"><font class="color_uploaded" style="display: none;">上传量:</font> 512.00 GB </a></div>
<div class="menu-base-info-items"><svg role="img" width="16px" height="16px" viewBox="0 0 24 24" aria-labelledby="arrowDownIconTitle" fill="none"><title id="arrowDownIconTitle">下载量</title><path d="M6 15l6 6 6-6"/></svg>&nbsp;<a href="userdetails.php?id=90101"><font class="color_downloaded" style="display: none;"> 下载量:</font> 128.00 GB </a></div>
<div class="menu-base-info-items"><svg role="img" width="16px" height="16px" viewBox="0 0 24 24" aria-labelledby="shareAndroidIconTitle" fill="none"><title id="shareAndroidIconTitle">分享率</title><circle cx="6" cy="12" r="2"/></svg>&nbsp;<a href="userdetails.php?id=90101"><font class="color_ratio" style="display: none;">分享率:</font>4.000</a></div>
<div class="menu-base-info-items"><svg t="1746456215620" class="icon" viewBox="0 0 1380 1024" width="18" height="18"><title id="dolarIconTitle">元宝</title><path d="M391 327C407 208 516 116 647 116z" fill="#FFDB60"></path></svg>&nbsp;<a href="mybonus.php?id=90101">12,345.6</a></div>
<div class="menu-base-info-items"><svg t="1742174330017" class="icon" viewBox="0 0 1024 1024" width="16" height="16"><title id="downing">当前做种</title><path d="M799 352C786 212 667 102 523 102z" fill="#1afa29"></path></svg>&nbsp;<a href="userdetails.php?id=90101">128</a></div>
<div class="menu-base-info-items"><svg t="1742174528229" class="icon" viewBox="0 0 1024 1024" width="16" height="16"><title id="downing">正在下载</title><path d="M874 789c0 46-37 85-85 85z" fill="#d81e06"></path></svg>&nbsp;<a href="userdetails.php?id=90101">3</a></div>
</div>
</div>
</div>
</body></html>`

// userdetails.php 正文表格：「传输」行是合并单元格（内嵌 table），同时存在
// 上传量/实际上传量、下载量/实际下载量、分享率/实际分享率，正则必须靠
// <strong> 紧跟标签名来区分；顶栏同样保留，用于验证「邀请人」行的 EliteUser_Name
// 不会被 span.user-id 限定的 name 选择器误取。
const csptUserdetailsFixture = `<html><body>
<span class="user-id"><span class="nowrap"><a href="https://cspt.top/userdetails.php?id=90101" class='PowerUser_Name'><b>cspt_fixture_user</b></a></span></span>
<div class="menu-base-info">
<div class="menu-base-info-items"><svg class="icon" viewBox="0 0 1380 1024" width="18" height="18"><title id="dolarIconTitle">元宝</title></svg>&nbsp;<a href="mybonus.php?id=90101">12,345.6</a></div>
<div class="menu-base-info-items"><svg class="icon" viewBox="0 0 1024 1024" width="16" height="16"><title id="downing">当前做种</title></svg>&nbsp;<a href="userdetails.php?id=90101">128</a></div>
<div class="menu-base-info-items"><svg class="icon" viewBox="0 0 1024 1024" width="16" height="16"><title id="downing">正在下载</title></svg>&nbsp;<a href="userdetails.php?id=90101">3</a></div>
</div>
<table width="100%" border="1" cellspacing="0" cellpadding="5" class="main_table">
<tr><td width="1%" class="rowhead nowrap main_rowhead" valign="top" align="right">用户ID/UID</td><td width="99%" class="rowfollow main_rowfollw" valign="top" align="left">90101</td></tr>
<tr><td width="1%" class="rowhead nowrap main_rowhead" valign="top" align="right">邀请人</td><td width="99%" class="rowfollow main_rowfollw" valign="top" align="left"><span class="nowrap"><a  href="https://cspt.top/userdetails.php?id=90999" class='EliteUser_Name'><b>cspt_fixture_inviter</b></a></span></td></tr>
<tr><td width="1%" class="rowhead nowrap main_rowhead" valign="top" align="right">加入日期</td><td width="99%" class="rowfollow main_rowfollw" valign="top" align="left">2025-03-04 09:10:11 (<span title="2025-03-04 09:10:11">1年4月前</span>, 60周)</td></tr>
<tr><td width="1%" class="rowhead nowrap main_rowhead" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow main_rowfollw" valign="top" align="left">2026-07-20 08:30:00 (<span title="2026-07-20 08:30:00">7天前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap main_rowhead" valign="top" align="right">传输</td><td width="99%" class="rowfollow main_rowfollw" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>:  <font color="">4.000</font>（<strong>实际分享率</strong>：1.600）</td><td class="embedded">&nbsp;&nbsp;<img src="pic/smilies/5.gif" alt="" /></td></tr><tr><td class="embedded"><strong>上传量</strong>:  512.00 GB <span class="text-muted">(当月: 100.00 GB)</span></td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>:  128.00 GB <span class="text-muted">(当月: 20.00 GB)</span></td></tr><tr><td class="embedded"><strong>实际上传量</strong>:  400.50 GB</td><td class="embedded">&nbsp;&nbsp;<strong>实际下载量</strong>:  250.25 GB</td><td class="embedded text-muted">&nbsp;&nbsp;实际上传/下载量 (仅用于记录, 不参与分享率计算)</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap main_rowhead" valign="top" align="right">BT时间</td><td width="99%" class="rowfollow main_rowfollw" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>做种时间</strong>:  120天02:47:36</td><td class="embedded">&nbsp;&nbsp;<strong>下载时间</strong>:  30天17:58:19</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap main_rowhead" valign="top" align="right">等级</td><td width="99%" class="rowfollow main_rowfollw" valign="top" align="left"><img alt="Power User" title="Power User" src="pic/power.gif" style="max-width: 100px;" /> </td></tr>
<tr><td width="1%" class="rowhead nowrap main_rowhead" valign="top" align="right">金元宝</td><td width="99%" class="rowfollow main_rowfollw" valign="top" align="left">12,345.6</td></tr>
<tr><td width="1%" class="rowhead nowrap main_rowhead" valign="top" align="right">做种积分</td><td width="99%" class="rowfollow main_rowfollw" valign="top" align="left">193,370.8</td></tr>
</table>
</body></html>`

// --- Helpers ---

func getCsptDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("cspt")
	require.True(t, ok, "cspt definition not found")
	return def
}

// --- Suite: Search ---

func testCsptSearch(t *testing.T) {
	def := getCsptDef(t)

	// 先证明行选择器排除了表头行：表头同样带 torrent-title / torrent-cat / torrent-info，
	// 但没有 torrent-table-sub-info，也没有 details.php 链接。
	t.Run("HeaderRowExcluded", func(t *testing.T) {
		doc := FixtureDoc(t, "cspt_search", csptSearchFixture)
		assert.Equal(t, 4, doc.Find("div.torrent-title").Length(), "1 表头 + 3 种子行都带 torrent-title")
		assert.Equal(t, 4, doc.Find("div.torrent-info").Length(), "1 表头 + 3 种子行都带 torrent-info")
		assert.Equal(t, 3, doc.Find(def.Selectors.TableRows).Length(), "行选择器只能命中 3 个真实种子行")
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(csptSearchFixture))
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
	require.Len(t, items, 3, "表头行必须被排除，只解析 3 个种子行")

	free := items[0]
	assert.Equal(t, "88801", free.ID)
	assert.Equal(t, "Fixture.Anime.Movie.2026.1080p.WEB-DL.H.265.DDP.5.1-FIXTURE", free.Title)
	assert.Equal(t, "虚构副标题一 | 类型：喜剧/动画 | [英/国]音轨", free.Subtitle)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	assert.Equal(t, int64(13937168875), free.SizeBytes)
	assert.Equal(t, 1, free.Seeders)
	assert.Equal(t, 35, free.Leechers)
	assert.Equal(t, 0, free.Snatched)
	assert.Equal(t, "动漫", free.Category)
	// 促销结束时间取 <font color> 内的 span，不能误取 title="通过" 或统计列的发布时间
	require.False(t, free.DiscountEndTime.IsZero(), "discount end time should be parsed")
	assert.Equal(t, "2026-07-31 12:00:00", free.DiscountEndTime.Format("2006-01-02 15:04:05"))
	// 发布时间来自 div.torrent-info-text-added 的 span[title]
	assert.Equal(t, int64(1785161262), free.UploadedAt)

	normal := items[1]
	assert.Equal(t, "88802", normal.ID)
	assert.Equal(t, "Fixture.Guide.v2.0-FIXTURE", normal.Title)
	assert.Equal(t, "虚构副标题二", normal.Subtitle)
	assert.Equal(t, v2.DiscountNone, normal.DiscountLevel)
	assert.True(t, normal.DiscountEndTime.IsZero(), "无促销 -> 无结束时间；title=\"通过\" 不得被误解析")
	assert.Equal(t, int64(855952588), normal.SizeBytes)
	assert.Equal(t, 206, normal.Seeders)
	assert.Equal(t, 1, normal.Leechers)
	assert.Equal(t, 0, normal.Snatched)

	// 永久免费：只有 img.pro_free[title=免费]，既无 onmouseover 也无剩余时间 span
	forever := items[2]
	assert.Equal(t, "88803", forever.ID)
	assert.Equal(t, "虚构副标题三", forever.Subtitle)
	assert.Equal(t, v2.DiscountFree, forever.DiscountLevel)
	assert.True(t, forever.DiscountEndTime.IsZero(), "永久免费无结束时间")
	assert.Equal(t, int64(1685774663), forever.SizeBytes)
	assert.Equal(t, 12, forever.Seeders)
	assert.Equal(t, 4, forever.Leechers)
	assert.Equal(t, 88, forever.Snatched)
	assert.Equal(t, "剧集", forever.Category)
}

// --- Suite: Detail ---

func testCsptDetail(t *testing.T) {
	def := getCsptDef(t)

	t.Run("Free", func(t *testing.T) {
		doc := FixtureDoc(t, "cspt_detail", csptDetailFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "88801", info.TorrentID)
		assert.Equal(t, "Fixture.Anime.Movie.2026.1080p.WEB-DL.H.265.DDP.5.1-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		require.False(t, info.DiscountEnd.IsZero(), "discount end time should be parsed")
		assert.Equal(t, "2026-07-31 12:00:00", info.DiscountEnd.Format("2006-01-02 15:04:05"))
		// 「基本信息」标签单元格 + .Next() 兄弟节点，全角冒号 + 十进制 GB
		assert.InDelta(t, 12.98*1024, info.SizeMB, 0.1)
		// 顶栏有 [H&R]: [0/0/10] 计数器，不能被 HRKeywords 误判
		assert.False(t, info.HasHR, "顶栏 H&R 计数器不得被识别为 H&R 种子")
	})

	t.Run("Plain", func(t *testing.T) {
		doc := FixtureDoc(t, "cspt_detail_plain", csptDetailPlainFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "88802", info.TorrentID)
		assert.Equal(t, "Fixture.Guide.v2.0-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel)
		assert.True(t, info.DiscountEnd.IsZero())
		assert.InDelta(t, 816.30, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})
}

// --- Suite: UserInfo ---

func testCsptUserInfo(t *testing.T) {
	def := getCsptDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "cspt_index", csptIndexFixture)
		fields := map[string]string{
			"id":   "90101",
			"name": "cspt_fixture_user",
			// 做种/下载数靠 svg <title> 文案定位，两个 svg 的 title id 都是 downing
			"seeding":  "128",
			"leeching": "3",
			// 金元宝：顶栏 mybonus.php 链接
			"bonus": "12345.6",
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
		doc := FixtureDoc(t, "cspt_userdetails", csptUserdetailsFixture)
		fields := map[string]string{
			// 512.00 GB / 128.00 GB / 400.50 GB / 250.25 GB 按 1024 进制换算为字节
			"uploaded":       "549755813888",
			"downloaded":     "137438953472",
			"trueUploaded":   "430033600512",
			"trueDownloaded": "268703891456",
			"ratio":          "4",
			"levelName":      "Power User",
			// name 被 span.user-id 限定，不会取到「邀请人」行的 EliteUser_Name
			"name": "cspt_fixture_user",
			// parseTime 按站点 TimezoneOffset(+0800) 解析
			"joinTime":     "1741050611",
			"lastAccessAt": "1784507400",
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

func TestCspt_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":       csptSearchFixture,
		"detail":       csptDetailFixture,
		"detail_plain": csptDetailPlainFixture,
		"index":        csptIndexFixture,
		"userdetails":  csptUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
