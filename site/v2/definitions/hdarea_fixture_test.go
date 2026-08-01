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
		SiteID:   "hdarea",
		Search:   testHdareaSearch,
		Detail:   testHdareaDetail,
		UserInfo: testHdareaUserInfo,
	})
}

// --- Fixtures ---
//
// 结构照搬真实页面，但所有标题、副标题、种子 ID、用户名、用户 ID 和图片地址均已替换为虚构值。
//
// 搜索页需要覆盖的真实特征：
//   - 页面上有 **两个** table.torrents：先是「预告区（前 5 最新预告）」面板
//     （表头 td.colhead_notice + 内容藏在 tbody#kokk），再才是真正的种子列表（表头 td.colhead）。
//     真实抓包时预告区为空，无法证明选择器是否越界，因此这里**故意**给预告区塞进一条
//     带 table.torrentname 的行；若 TableRows 少了 :has(td.colhead)，解析结果会变成 3 条。
//   - 名称单元格是两个 div：第一个 div 放标题 + 促销图标 + (剩余时间)，
//     第二个 div 带 title 属性放副标题。
//   - 促销结束时间同时出现在 img.pro_free 的 onmouseover 提示与
//     (<font color='...'>剩余时间：<span title>) 中，且不同促销等级 font color 不同。
//   - 名称子表第二个单元格（width=110）内有豆瓣评分 <span title="豆瓣评分">，
//     它同样是 span[title]，必须不能被当成促销结束时间。
//   - 列序为 10 列：类型 / 海报 / 标题 / 评论数 / 存活时间 / 大小 / 种子数 / 下载数 / 完成数 / 发布者。
//   - 大小列是 "482.76<br />MB"（数字与单位被 <br /> 分开）。

const hdareaSearchFixture = `<html><body>
<table width="100%" class="main" border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded">
<table class="torrents" cellspacing="0" cellpadding="5" width="100%">
<tbody><tr><td class="colhead_notice" align="center" colspan="10"><a href="javascript: klappe_news('okk')"><img class="plus" src="pic/trans.gif" id="picokk" alt="Show/Hide" />预告区（前5最新预告）</a> - [<a href="advance_notice.php">更多</a>]</td></tr></tbody>
<tbody id="kokk" style="display: none;">
	<tr class='nonstick_outer_bg'>
	<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=401"><img class="c_movie" src="pic/cattrans.gif" alt="Movies" title="Movies" /></a></td>
	<td class="rowfollow" align="center" valign="middle" style="padding: 3px;"><img class="torrent-poster-thumb" src="pic/trans.gif" /></td>
	<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr><td class="embedded"><div style="display: flex; align-items: center; max-width: 775px;"><a title="Fixture.Advance.Notice.2026.1080p-FIXTURE" href="details.php?id=88109"><b>Fixture.Advance.Notice.2026.1080p-FIXTURE</b></a></div><div style="color: #666;" title="虚构预告区条目">虚构预告区条目</div></td></tr></table></td>
	<td class="rowfollow">0</td>
	<td class="rowfollow nowrap"><span title="2026-07-27 10:00:00">&lt; 1分</span></td>
	<td class="rowfollow">1.00<br />GB</td>
	<td class="rowfollow" align="center"><b>0</b></td>
	<td class="rowfollow"><b>0</b></td>
	<td class="rowfollow">0</td>
	<td class="rowfollow"><i>匿名</i></td>
	</tr>
</tbody></table>
</td></tr></table>
<br />
<table class="torrents" cellspacing="0" cellpadding="5" width="100%">
<tr>
<td class="colhead" style="padding: 0px">类型</td>
<td class="colhead">海报</td>
<td class="colhead"><a href="?sort=1&amp;type=asc">标题</a></td>
<td class="colhead"><a href="?sort=3&amp;type=desc"><img class="comments" src="pic/trans.gif" alt="comments" title="评论数" /></a></td>
<td class="colhead"><a href="?sort=4&amp;type=desc"><img class="time" src="pic/trans.gif" alt="time" title="存活时间" /></a></td>
<td class="colhead"><a href="?sort=5&amp;type=desc"><img class="size" src="pic/trans.gif" alt="size" title="大小" /></a></td>
<td class="colhead"><a href="?sort=7&amp;type=desc"><img class="seeders" src="pic/trans.gif" alt="seeders" title="种子数" /></a></td>
<td class="colhead"><a href="?sort=8&amp;type=desc"><img class="leechers" src="pic/trans.gif" alt="leechers" title="下载数" /></a></td>
<td class="colhead"><a href="?sort=6&amp;type=desc"><img class="snatched" src="pic/trans.gif" alt="snatched" title="完成数" /></a></td>
<td class="colhead"><a href="?sort=9&amp;type=desc">发布者</a></td>
</tr>
<tr class='nonstick_outer_bg'>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=402"><img class="c_tvseries" src="pic/cattrans.gif" alt="TV Series" title="TV Series" style="background-image: url(pic/catsprites.png);" /></a></td>
<td class="rowfollow" align="center" valign="middle" style="padding: 3px;"><img class="torrent-poster-thumb lazy-loading" src="pic/trans.gif" data-src="pic/poster/fixture-a.jpg" /></td>
<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr class='nonstick_inner_bg'><td class="embedded"><div style="display: flex; align-items: center; max-width: 775px;"><a title="Fixture.Show.S01E03.2026.2160p.WEB-DL.H265-FIXTURE"  href="details.php?id=88101" style="min-width: 0; white-space: nowrap;"><b>Fixture.Show.S01E03.2026.2160p.WEB-DL.H265-FIXTURE</b></a><span style="flex-shrink: 0; white-space: nowrap;"><b> (<font class='new'>新</font>)</b> <img class="pro_free" src="pic/trans.gif" alt="Free"  onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-07-28 15:09:57&quot;&gt;23时59分&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000);" /> (<font color='#0000FF'>剩余时间：<span title="2026-07-28 15:09:57">23时59分</span></font>)</span></div><div style="color: #666; max-width: 775px; white-space: nowrap; padding-top: 2px;" title="虚构副标题一 第03集 | 国语 | 中字 | 官方">虚构副标题一 第03集 | 国语 | 中字 | 官方</div></td>
<td width="110" class="embedded" style="text-align: right;" valign="middle">
  <div style="display: flex; justify-content: flex-end; align-items: center; gap: 6px;">
    <div><div style="display: flex; align-items: center;"><a href="https://example.invalid/subject/00000001/" target="_blank"><span style="display: inline-flex;" title="豆瓣评分"><span>豆瓣</span><span>0.0</span></span></a></div></div>
    <div style="text-align: left; line-height: 1; flex-shrink: 0;"><a href="download.php?id=88101"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a>&nbsp<a href="download_zip.php?id=88101"><img class="download_zip" src="pic/trans.gif" alt="download_zip" title="打包下载" /></a></div>
  </div>
</td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=88101&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-07-27 15:09:57">&lt; 1分</span></td><td class="rowfollow">482.76<br />MB</td><td class="rowfollow" align="center"><b><a href="details.php?id=88101&amp;dllist=1#seeders">1</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=88101&amp;dllist=1#leechers">2</a></b></td>
<td class="rowfollow">3</td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr class='nonstick_outer_bg'>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=403"><img class="c_tvshows" src="pic/cattrans.gif" alt="TV Shows" title="TV Shows" style="background-image: url(pic/catsprites.png);" /></a></td>
<td class="rowfollow" align="center" valign="middle" style="padding: 3px;"><img class="torrent-poster-thumb lazy-loading" src="pic/trans.gif" data-src="pic/poster/fixture-b.jpg" /></td>
<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr class='nonstick_inner_bg'><td class="embedded"><div style="display: flex; align-items: center; max-width: 775px;"><a title="Fixture.Variety.S01E06.2026.1080p.WEB-DL.H264-FIXTURE"  href="details.php?id=88102" style="min-width: 0; white-space: nowrap;"><b>Fixture.Variety.S01E06.2026.1080p.WEB-DL.H264-FIXTURE</b></a><span style="flex-shrink: 0; white-space: nowrap;"><b> (<font class='new'>新</font>)</b> <img class="pro_free2up" src="pic/trans.gif" alt="2X Free"  onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;twoupfree&quot;&gt;2X免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-07-28 14:08:49&quot;&gt;22时58分&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000);" /> (<font color='#00CC66'>剩余时间：<span title="2026-07-28 14:08:49">22时58分</span></font>)</span></div><div style="color: #666; max-width: 775px; white-space: nowrap; padding-top: 2px;" title="虚构副标题二 第06集 | 中字 | 官方">虚构副标题二 第06集 | 中字 | 官方</div></td>
<td width="110" class="embedded" style="text-align: right;" valign="middle"><a href="download.php?id=88102"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=88102&amp;type=torrent" title="添加评论">5</a></td><td class="rowfollow nowrap"><span title="2026-07-27 14:08:49">&lt; 1分</span></td><td class="rowfollow">2.05<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=88102&amp;dllist=1#seeders">12</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=88102&amp;dllist=1#leechers">4</a></b></td>
<td class="rowfollow">6</td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr class='nonstick_outer_bg'>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=401"><img class="c_movie" src="pic/cattrans.gif" alt="Movies" title="Movies" style="background-image: url(pic/catsprites.png);" /></a></td>
<td class="rowfollow" align="center" valign="middle" style="padding: 3px;"><img class="torrent-poster-thumb lazy-loading" src="pic/trans.gif" data-src="pic/poster/fixture-c.jpg" /></td>
<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr class='nonstick_inner_bg'><td class="embedded"><div style="display: flex; align-items: center; max-width: 775px;"><a title="Fixture.Movie.2026.BluRay.1080p.x264-FIXTURE"  href="details.php?id=88103" style="min-width: 0; white-space: nowrap;"><b>Fixture.Movie.2026.BluRay.1080p.x264-FIXTURE</b></a></div><div style="color: #666; max-width: 775px; white-space: nowrap; padding-top: 2px;" title="虚构副标题三 | 国语 | 中字">虚构副标题三 | 国语 | 中字</div></td>
<td width="110" class="embedded" style="text-align: right;" valign="middle">
  <div><a href="https://example.invalid/subject/00000003/" target="_blank"><span style="display: inline-flex;" title="豆瓣评分"><span>豆瓣</span><span>8.1</span></span></a></div>
  <div><a href="download.php?id=88103"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></div>
</td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=88103&amp;type=torrent" title="添加评论">1</a></td><td class="rowfollow nowrap"><span title="2026-07-20 08:30:00">7天</span></td><td class="rowfollow">16.87<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=88103&amp;dllist=1#seeders">88</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=88103&amp;dllist=1#leechers">0</a></b></td>
<td class="rowfollow">120</td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
</table>
</body></html>`

// 详情页：标题/ID 来自「字幕」行 <form> 内的隐藏域（本站保留），h1#top 的促销标记为
// <font class='free' >（单引号 + 尾随空格）；「基本信息」行使用全角冒号，且真实体积单位是 MB。
const hdareaDetailFixture = `<html><body>
<h1 align="center" id="top">Fixture.Show.S01E03.2026.2160p.WEB-DL.H265-FIXTURE&nbsp;&nbsp;&nbsp; <b>[<font class='free' >免费</font>]</b>&nbsp; (<font color='#0000FF'>剩余时间：<span title="2026-07-28 15:09:57">23时59分</span></font>)</h1>
<table width="1280" cellspacing="0" cellpadding="5">
<tr><td class="rowhead" width="13%">下载</td><td class="rowfollow" width="87%" align="left"><a class="index" href="download.php?id=88101">[HDArea].Fixture.Show.S01E03.2026.2160p.WEB-DL.H265-FIXTURE.torrent</a>&nbsp;&nbsp;&nbsp;由&nbsp;<i>匿名</i>发布于<span title="2026-07-27 15:09:57">&lt; 1分前</span></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">虚构副标题一 第03集 | 国语 | 中字 | 官方</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>482.76 MB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;TV Series&nbsp;&nbsp;&nbsp;<b>媒介:&nbsp;</b>WEB-DL&nbsp;&nbsp;&nbsp;<b>编码:&nbsp;</b>H.265(x265/HEVC)&nbsp;&nbsp;&nbsp;<b>分辨率:&nbsp;</b>4K</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">行为</td><td class="rowfollow" valign="top" align="left"><a title="下载种子" href="download.php?id=88101"><img class="dt_download" src="pic/trans.gif" alt="download" /></a></td></tr>
<tr><td class="rowhead nowrap">下载链接</td><td valign="top" align="left" class="rowfollow">https://hdarea.club/download.php?id=88101&amp;passkey=FIXTUREPASSKEY<br /><strong style=color:#F00 >链接包含个人passkey, 请勿泄露</strong></td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><table border="0" cellspacing="0"><tr><td class="embedded"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.Show.S01E03.2026.2160p.WEB-DL.H265-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="88101" /></form></td></tr></table></td></tr>
</table>
</body></html>`

// 无促销 + 带 H&R 的详情页，用于确认促销为 DiscountNone、结束时间为零值，
// 且体积单位为 GB 时能正确换算。
const hdareaDetailWithHRFixture = `<html><body>
<h1 align="center" id="top">Fixture.Movie.2026.BluRay.1080p.x264-FIXTURE</h1>
<table width="1280" cellspacing="0" cellpadding="5">
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>16.87 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;Movies</td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.Movie.2026.BluRay.1080p.x264-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="88103" /></form></td></tr>
</table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

// index.php 顶部 #info_block：id / name / 魔力值 / 做种下载数均取自这里。
// 注意 class 属性带空格（class = 'color_bonus'），与真实页面一致。
const hdareaIndexFixture = `<html><body>
<table id="info_block" cellpadding="0" cellspacing="0" border="0" width="100%"><tr>
	<td><table width="100%" cellspacing="0" cellpadding="4" border="0"><tr>
		<td class="bottom12" align="left"><span class="medium">欢迎回来, <span class="nowrap"><a  href="userdetails.php?id=77001" class='VeteranUser_Name'><b>fixture_user</b></a></span>  [<a href="logout.php">退出</a>] <font class = 'color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 2,747,702.4 <font class = 'color_invite'>邀请 </font>[<a href="invite.php?id=77001">发送</a>]: 0<br />
	<font class="color_ratio">分享率：</font> 5.603  <font class='color_uploaded'>上传量：</font> 5.607 TB<font class='color_downloaded'> 下载量：</font> 1.001 TB  <font class='color_active'>当前活动：</font> <img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />487  <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />0&nbsp;&nbsp;<font class='color_connectable'>可连接：</font><b><font color="green">是</font></b></span></td>
	</tr></table></td>
</tr></table>
</body></html>`

// userdetails.php 正文表格：「传输」行是标准的
// <strong>分享率</strong> / <strong>上传量</strong> / <strong>下载量</strong> 合并单元格；
// 「最近动向」只出现一次，不存在同名的「(地点)」行。
const hdareaUserdetailsFixture = `<html><body>
<h1 style='margin:0px'><span class="nowrap"><b>fixture_target</b></span></h1>
<table width="100%" border="1" cellspacing="0" cellpadding="5">
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">邀请</td><td width="99%" class="rowfollow" valign="top" align="left">0</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">加入日期</td><td width="99%" class="rowfollow" valign="top" align="left">2025-12-13 21:58:57 (<span title="2025-12-13 21:58:57">7月15天前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-07-27 15:10:35 (<span title="2026-07-27 15:10:35">&lt; 1分前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">BT客户端</td><td width="99%" class="rowfollow" valign="top" align="left">qBittorrent/5.1.2</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">传输</td><td width="99%" class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>:  <font color="">5.603</font></td><td class="embedded">&nbsp;&nbsp;<img src="pic/smilies/5.gif" alt="" /></td></tr><tr><td class="embedded"><strong>上传量</strong>:  5.607 TB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>:  1.001 TB</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">BT时间</td><td width="99%" class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>做种时间</strong>:  1015天05:12:11</td><td class="embedded">&nbsp;&nbsp;<strong>下载时间</strong>:  61天03:44:02</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">等级</td><td width="99%" class="rowfollow" valign="top" align="left"><img alt="Veteran User" title="Veteran User" src="pic/veteran.gif" /> </td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">魔力值</td><td width="99%" class="rowfollow" valign="top" align="left">2,747,702.4</td></tr>
</table>
</body></html>`

// --- Helpers ---

func getHdareaDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("hdarea")
	require.True(t, ok, "hdarea definition not found")
	return def
}

// --- Suite: Search ---

func testHdareaSearch(t *testing.T) {
	def := getHdareaDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(hdareaSearchFixture))
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
	// 关键断言：fixture 里有 4 条 table.torrentname（预告区 1 条 + 真实列表 3 条），
	// TableRows 必须只命中真实列表的 3 条。
	require.Len(t, items, 3, "TableRows must skip the 预告区 table (expect 3, not 4)")
	for _, it := range items {
		require.NotEqual(t, "88109", it.ID, "预告区条目不应出现在搜索结果中")
	}

	free := items[0]
	assert.Equal(t, "88101", free.ID)
	assert.Equal(t, "Fixture.Show.S01E03.2026.2160p.WEB-DL.H265-FIXTURE", free.Title)
	// 副标题是名称单元格内带 title 的第二个 div
	assert.Equal(t, "虚构副标题一 第03集 | 国语 | 中字 | 官方", free.Subtitle)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	// 大小列为 "482.76<br />MB"，parseSize 去空白后按 1024 进制换算
	assert.Equal(t, int64(506210549), free.SizeBytes)
	assert.Equal(t, 1, free.Seeders)
	assert.Equal(t, 2, free.Leechers)
	assert.Equal(t, 3, free.Snatched)
	assert.Equal(t, "TV Series", free.Category)
	// 促销结束时间取 (<font color>剩余时间：<span title>)，
	// 不能误取第 5 列的存活时间 span，也不能误取豆瓣评分 span[title="豆瓣评分"]
	require.False(t, free.DiscountEndTime.IsZero(), "discount end time should be parsed")
	assert.Equal(t, "2026-07-28 15:09:57", free.DiscountEndTime.Format("2006-01-02 15:04:05"))
	assert.Equal(t, int64(1785164997), free.UploadedAt)

	// 2X 免费：内置分支先判 free2up，不会退化成 DiscountFree；font color 与免费不同
	twoXFree := items[1]
	assert.Equal(t, "88102", twoXFree.ID)
	assert.Equal(t, v2.Discount2xFree, twoXFree.DiscountLevel)
	assert.Equal(t, "虚构副标题二 第06集 | 中字 | 官方", twoXFree.Subtitle)
	assert.Equal(t, int64(2201170739), twoXFree.SizeBytes)
	assert.Equal(t, 12, twoXFree.Seeders)
	assert.Equal(t, 4, twoXFree.Leechers)
	assert.Equal(t, 6, twoXFree.Snatched)
	require.False(t, twoXFree.DiscountEndTime.IsZero())
	assert.Equal(t, "2026-07-28 14:08:49", twoXFree.DiscountEndTime.Format("2006-01-02 15:04:05"))

	// 无促销行：即便名称子表里有豆瓣评分 span[title="豆瓣评分"]，结束时间也必须是零值
	normal := items[2]
	assert.Equal(t, "88103", normal.ID)
	assert.Equal(t, "Fixture.Movie.2026.BluRay.1080p.x264-FIXTURE", normal.Title)
	assert.Equal(t, "虚构副标题三 | 国语 | 中字", normal.Subtitle)
	assert.Equal(t, v2.DiscountNone, normal.DiscountLevel)
	assert.True(t, normal.DiscountEndTime.IsZero(), "no discount -> no end time (豆瓣评分 span must not leak)")
	assert.Equal(t, int64(18114024570), normal.SizeBytes)
	assert.Equal(t, 88, normal.Seeders)
	assert.Equal(t, 0, normal.Leechers)
	assert.Equal(t, 120, normal.Snatched)
	assert.Equal(t, "Movies", normal.Category)
}

// --- Suite: Detail ---

func testHdareaDetail(t *testing.T) {
	def := getHdareaDef(t)

	t.Run("Free", func(t *testing.T) {
		doc := FixtureDoc(t, "hdarea_detail", hdareaDetailFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "88101", info.TorrentID)
		assert.Equal(t, "Fixture.Show.S01E03.2026.2160p.WEB-DL.H265-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		require.False(t, info.DiscountEnd.IsZero(), "discount end time should be parsed")
		assert.Equal(t, "2026-07-28 15:09:57", info.DiscountEnd.Format("2006-01-02 15:04:05"))
		// 「基本信息」标签单元格 + .Next() 兄弟节点；真实抓包体积单位是 MB，不做换算
		assert.InDelta(t, 482.76, info.SizeMB, 0.01)
		assert.False(t, info.HasHR)
	})

	t.Run("NoDiscountWithHR", func(t *testing.T) {
		doc := FixtureDoc(t, "hdarea_detail_hr", hdareaDetailWithHRFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "88103", info.TorrentID)
		assert.Equal(t, "Fixture.Movie.2026.BluRay.1080p.x264-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel)
		assert.True(t, info.DiscountEnd.IsZero())
		assert.InDelta(t, 16.87*1024, info.SizeMB, 0.1)
		assert.True(t, info.HasHR, "should detect HR from hit_run.gif")
	})
}

// --- Suite: UserInfo ---

func testHdareaUserInfo(t *testing.T) {
	def := getHdareaDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "hdarea_index", hdareaIndexFixture)
		fields := map[string]string{
			"id":       "77001",
			"name":     "fixture_user",
			"seeding":  "487",
			"leeching": "0",
			// parseNumber 对大数值返回科学计数法字符串
			"bonus": "2.7477024e+06",
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
		doc := FixtureDoc(t, "hdarea_userdetails", hdareaUserdetailsFixture)
		fields := map[string]string{
			// 5.607 TB / 1.001 TB 按 1024 进制换算为字节
			"uploaded":   "6164961696940",
			"downloaded": "1100611139403",
			"ratio":      "5.603",
			"levelName":  "Veteran User",
			// parseTime 按站点 TimezoneOffset(+0800) 解析
			"joinTime":     "1765634337",
			"lastAccessAt": "1785136235",
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

func TestHdarea_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      hdareaSearchFixture,
		"detail":      hdareaDetailFixture,
		"detail_hr":   hdareaDetailWithHRFixture,
		"index":       hdareaIndexFixture,
		"userdetails": hdareaUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
