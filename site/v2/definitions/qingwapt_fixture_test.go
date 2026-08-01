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
		SiteID:   "qingwapt",
		Search:   testQingwaptSearch,
		Detail:   testQingwaptDetail,
		UserInfo: testQingwaptUserInfo,
	})
}

// --- Fixtures ---
//
// 结构照搬真实采集页面（qw 皮肤），但所有标题、副标题、种子 ID、用户名、用户 ID
// 与图片地址均已替换为虚构值。
//
// 搜索页需要覆盖的真实特征：
//   - 促销标记不是 img.pro_free，而是自绘文字徽章
//     <span style="..." alt="Free" onmouseover="...">Free</span>
//   - 剩余时间渲染为 <font color='#0000FF'>剩余时间：<span title="...">…</span></font>
//   - 名称单元格内还有 title="置顶促销" / title="自动审核通过" 等非时间 span
//   - 名称单元格前缀可能带多个 img.sticky、<b>(新)</b>、<b>[热门]</b>
//   - 副标题是若干自绘标签 span（官方/中字/分集）之后的裸文本节点
//   - 大小列是 "496.43<br />MB"（数字与单位被 <br /> 分开）
//   - 名称子表内还有海报单元格、评分单元格与下载单元格（均为 td.embedded）
//
// 注意：采集样本 52 行全部处于免费状态（站点当时开放全站促销），
// 因此下面第二行「无促销」是人工构造的合成行，用于验证 DiscountNone 以及
// font[color] 限定确实能避开 title="置顶促销" 这类非时间 span，
// 真实抓取数据未观测到非免费行。
const qingwaptSearchFixture = `<html><body>
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
<tr style="background-color: #75CAF4">
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=405"><img class="c_anime" src="pic/cattrans.gif" alt="动漫" title="动漫" style="background-image: url(pic/category/qw/catsprites.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr style="background-color: #75CAF4"><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="pic/poster/fixture-anime.jpg" class="nexus-lazy-load" style="max-height: 46px;max-width: 46px" /></td><td class="embedded" style='padding-left: 5px'><img class="sticky" src="pic/trans.gif" alt="Sticky" title="一级置顶" />&nbsp;<img class="sticky" src="pic/trans.gif" alt="Sticky" title="一级置顶" />&nbsp;<a title="Fixture.Anime.S01E04.2026.1080p.WEB-DL.H.264.AAC-FIXTURE"  href="details.php?id=195684&amp;hit=1"><b>Fixture.Anime.S01E04.2026.1080p.WEB-DL.H.264.AAC-FIXTURE</b></a><b> (<font class='new'>新</font>)</b> <b>[<font class='hot'>热门</font>]</b> <span style="display: inline-block; margin: 2px; font-size:10px; color: rgb(255, 255, 255); background-color: #0034EF; padding: 0px 6px; border-radius: 3px;" alt="Free"  onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-07-29 00:47:50&quot;&gt;1天9时&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000);">Free</span> <font color='#0000FF'>剩余时间：<span title="2026-07-29 00:47:50">1天9时</span></font><span style="margin-left: 6px" title="自动审核通过"><svg viewBox="0 0 1024 1024" width="16" height="16"></svg></span><br /><span style="color:#fff;border-radius:4px;font-size:12px;margin:0 4px 0 0;padding:1px 4px;white-space:nowrap;" title="">官方</span><span style="color:#fff;border-radius:4px;font-size:12px;margin:0 4px 0 0;padding:1px 4px;white-space:nowrap;" title="">中字</span><span style="color:#fff;border-radius:4px;font-size:12px;margin:0 4px 0 0;padding:1px 4px;white-space:nowrap;" title="">分集</span>虚构副标题一 | 类型：动画 喜剧 Sci-Fi &amp; Fantasy | 语言：日本語 | *内封简繁英多国软字幕*  第1季 第04集</td><td class="embedded" style="text-align: right; width: 40px;padding: 4px"><div style="display: flex;flex-direction: column"><img src="pic/imdb2.png" alt="imdb" title="imdb" style="max-width: 16px;max-height: 16px"/><span>7.1</span></div></td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=195684"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a><br /><a id="bookmark0"  href="javascript: bookmark(195684,0);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=195684&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-07-27 00:47:50">14时<br />27分钟</span></td><td class="rowfollow">496.43<br />MB</td><td class="rowfollow" align="center"><b><a href="details.php?id=195684&amp;hit=1&amp;dllist=1#seeders">19</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><a href="viewsnatches.php?id=195684"><b>10</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=401"><img class="c_movie" src="pic/cattrans.gif" alt="电影" title="电影" style="background-image: url(pic/category/qw/catsprites.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="pic/poster/fixture-movie.jpg" class="nexus-lazy-load" style="max-height: 46px;max-width: 46px" /></td><td class="embedded" style='padding-left: 5px'>&nbsp;<span title="置顶促销"><svg viewBox="0 0 1024 1024" width="16" height="16"></svg></span>&nbsp;<a title="Fixture.Movie.2026.2160p.BluRay.x265.DTS-FIXTURE"  href="details.php?id=195705&amp;hit=1"><b>Fixture.Movie.2026.2160p.BluRay.x265.DTS-FIXTURE</b></a><br /><span style="color:#fff;border-radius:4px;font-size:12px;margin:0 4px 0 0;padding:1px 4px;white-space:nowrap;" title="">中字</span>虚构副标题二 | 类型：动作冒险 | 语言：英語</td><td class="embedded" style="text-align: right; width: 40px;padding: 4px"><img src="pic/douban.png" alt="douban" title="douban" style="max-width: 16px;max-height: 16px"/><span>8.2</span></td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=195705"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=195705&amp;type=torrent" title="添加评论">4</a></td><td class="rowfollow nowrap"><span title="2026-05-01 12:00:00">2月<br />26天</span></td><td class="rowfollow">12.35<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=195705&amp;hit=1&amp;dllist=1#seeders">203</a></b></td>
<td class="rowfollow">2</td>
<td class="rowfollow"><a href="viewsnatches.php?id=195705"><b>88</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
</table>
</body></html>`

// 详情页：标题/ID 来自 <form> 内的隐藏域（本站保留）；h1 里的促销标记是
// <font class='free' >（单引号 + 尾部空格），随后是 <font color='#0000FF'>剩余时间：<span title>，
// 再之后还有一个 title="自动审核通过" 的 span（不能被当成结束时间）；
// 「基本信息」行使用全角冒号与十进制单位。
const qingwaptDetailFixture = `<html><body>
<h1 align="center" id="top">Fixture.Anime.S01E15-E17.2026.1080p.WEB-DL.H.264.AAC-FIXTURE&nbsp;&nbsp;&nbsp; <b>[<font class='free' >免费</font>]</b> <font color='#0000FF'>剩余时间：<span title="2026-07-29 12:01:20">1天20时</span></font><span style="margin-left: 6px" title="自动审核通过"><svg viewBox="0 0 1024 1024" width="16" height="16"></svg></span></h1>
<table width="97%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead" width="13%">下载</td><td class="rowfollow" width="87%" align="left"><a class="index" href="download.php?id=195705">[Fixture].Anime.S01.2026.1080p.WEB-DL.H.264.AAC-FIXTURE.torrent</a>&nbsp;&nbsp;&nbsp;由&nbsp;<i>匿名</i>发布于<span title="2026-07-27 12:01:20">3时14分钟前</span></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">虚构副标题一 | 类型：动作冒险 动画 | 语言：日本語 | *内嵌繁中*  第1季 第15-17集</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">标签</td><td class="rowfollow" valign="top" align="left"><span style="color:#fff;border-radius:4px;padding:1px 4px" title="">官方</span></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>1.44 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;动漫&nbsp;&nbsp;&nbsp;<b>媒介: </b>WEB-DL&nbsp;&nbsp;&nbsp;<b>视频编码: </b>H.264/AVC&nbsp;&nbsp;&nbsp;<b>音频编码: </b>AAC&nbsp;&nbsp;&nbsp;<b>分辨率: </b>1080p&nbsp;&nbsp;&nbsp;<b>制作组: </b>FIXTURE</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">行为</td><td class="rowfollow" valign="top" align="left"><a title="下载种子" href="download.php?id=195705"><img class="dt_download" src="pic/trans.gif" alt="download" />&nbsp;<b><font class="small">下载种子</font></b></a></td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><table border="0" cellspacing="0"><tr><td class="embedded">该种子暂无字幕</td></tr></table><table border="0" cellspacing="0"><tr><td class="embedded"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.Anime.S01E15-E17.2026.1080p.WEB-DL.H.264.AAC-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="195705" /><input type="hidden" name="in_detail" value="in_detail" /><input type="submit" value="上传字幕" /></form></td></tr></table></td></tr>
</table>
</body></html>`

// 无促销 + 带 H&R 标记的详情页（合成：采集样本未观测到 H&R 种子，
// 站点也未配置 H&R 规则，此处仅验证解析器的关键字兜底不误伤）。
const qingwaptDetailWithHRFixture = `<html><body>
<h1 align="center" id="top">Fixture.HR.Show.S01.WEB-DL.1080p-FIXTURE</h1>
<table width="97%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>8.50 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;剧集</td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.HR.Show.S01.WEB-DL.1080p-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="195999" /></form></td></tr>
</table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

// #info_block：id / name / 蝌蚪（本站魔力值别名）/ 做种下载数均取自这里。
// 注意 class 属性带空格（class = 'color_bonus'），且「认领」也用 color_bonus，
// 蝌蚪标签后连着两个方括号链接再跟冒号数值。
const qingwaptIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr>
	<td><table width="100%" cellspacing="0" cellpadding="0" border="0"><tr>
		<td class="bottom" align="left">
            <span class="medium">
                欢迎回来, <span class="nowrap"><a  href="https://www.qingwapt.com/userdetails.php?id=708159" class='User_Name'><b>fixture_user</b></a></span>                [<a href="logout.php">退出</a>]
                [<a href="usercp.php">&nbsp;控制面板&nbsp;</a>]
                <font class = 'color_bonus'>蝌蚪 </font>[<a href="mybonus.php">详情</a>][<a href="bonusshop.php">使用</a>]: 260,200.3                 <a href="attendance.php" class="">[签到已得100, 补签卡: 0]</a>
                <font class = 'color_invite'>邀请 </font>[<a href="invite.php?id=708159">发送</a>]: 0(0)                                <br />
	            <font class="color_ratio">分享率:</font> 3.563                <font class='color_uploaded'>上传量:</font> 285.34 GB                <font class='color_downloaded'> 下载量:</font> 80.09 GB                <font class='color_active'>当前活动</font> [<a href="/clean.php">清除</a>] : <img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />370  <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />0&nbsp;
                <font class='color_slots'>连接数：</font>无限制                                <font class='color_bonus'>认领: </font> [<a href="claim.php?uid=708159">250/5000</a>]            </span>
        </td>
	</tr></table></td>
</tr></table>
</body></html>`

// userdetails.php 正文表格：
//   - 「传输」行是经典的 <strong>上传量</strong> 合并单元格，并额外带
//     <strong>实际上传量</strong>/<strong>实际下载量</strong>，
//     以及「（<strong>实际分享率</strong>：0.550）」尾巴
//   - 「最近动向」之后紧跟同样命中 :contains('最近动向') 的「最近动向(地点)」行，
//     该行是非时间文本（torrents），必须靠 First() 取到前一行
const qingwaptUserdetailsFixture = `<html><body>
<h1 style='margin:0px'><span class="nowrap"><b>fixture_target</b></span></h1>
<table width="100%" border="1" cellspacing="0" cellpadding="5">
<tr><td class="rowhead nowrap" valign="top" align="right">ID</td><td class="rowfollow" valign="top" align="left">703646</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">邀请</td><td class="rowfollow" valign="top" align="left">6</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">加入日期</td><td width="99%" class="rowfollow" valign="top" align="left">2024-12-17 23:11:39 (<span title="2024-12-17 23:11:39">1年7月前</span>, 83周)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-07-27 14:49:58 (<span title="2026-07-27 14:49:58">25分钟前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向(地点)</td><td width="99%" class="rowfollow" valign="top" align="left">torrents</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">传输</td><td width="99%" class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>:  <font color="">2.035</font>（<strong>实际分享率</strong>：0.550）</td><td class="embedded">&nbsp;&nbsp;<img src="pic/smilies/3.gif" alt="" /></td></tr><tr><td class="embedded"><strong>上传量</strong>:  3.131 TB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>:  1.538 TB</td></tr><tr><td class="embedded"><strong>实际上传量</strong>:  2.070 TB</td><td class="embedded">&nbsp;&nbsp;<strong>实际下载量</strong>:  3.758 TB</td><td class="embedded text-muted">&nbsp;&nbsp;实际上传/下载量 (仅用于记录, 不参与分享率计算)</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">BT时间</td><td width="99%" class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>做种时间</strong>:  14043天10:29:54</td><td class="embedded">&nbsp;&nbsp;<strong>下载时间</strong>:  105天03:40:01</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">等级</td><td width="99%" class="rowfollow" valign="top" align="left"><img alt="Crazy User" title="Crazy User" src="pic/crazy.gif" /> </td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">种子评论</td><td width="99%" class="rowfollow" valign="top" align="left">0</td></tr>
</table>
</body></html>`

// --- Helpers ---

func getQingwaptDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("qingwapt")
	require.True(t, ok, "qingwapt definition not found")
	return def
}

// --- Suite: Search ---

func testQingwaptSearch(t *testing.T) {
	def := getQingwaptDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(qingwaptSearchFixture))
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
	require.Len(t, items, 2, "should parse 2 torrent rows")

	free := items[0]
	assert.Equal(t, "195684", free.ID)
	assert.Equal(t, "Fixture.Anime.S01E04.2026.1080p.WEB-DL.H.264.AAC-FIXTURE", free.Title)
	// 副标题是自绘标签 span 之后的裸文本，需要 SubtitleSelector 才能取到
	assert.Equal(t, "虚构副标题一 | 类型：动画 喜剧 Sci-Fi & Fantasy | 语言：日本語 | *内封简繁英多国软字幕*  第1季 第04集", free.Subtitle)
	// 促销标记是 span[alt='Free']，不是 img.pro_free
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	// 大小列为 "496.43<br />MB"，parseSize 去空白后按 1024 进制换算
	assert.Equal(t, int64(520544583), free.SizeBytes)
	assert.Equal(t, 19, free.Seeders)
	assert.Equal(t, 0, free.Leechers)
	assert.Equal(t, 10, free.Snatched)
	assert.Equal(t, "动漫", free.Category)
	// 促销结束时间取 <font color='#0000FF'> 内的 span[title]，
	// 不能误取 title="自动审核通过" 或第 4 列的发布时间
	require.False(t, free.DiscountEndTime.IsZero(), "discount end time should be parsed")
	assert.Equal(t, 2026, free.DiscountEndTime.Year())
	assert.Equal(t, 7, int(free.DiscountEndTime.Month()))
	assert.Equal(t, 29, free.DiscountEndTime.Day())
	assert.Positive(t, free.UploadedAt)

	// 合成的无促销行：验证 title="置顶促销" 不会被当成促销结束时间
	normal := items[1]
	assert.Equal(t, "195705", normal.ID)
	assert.Equal(t, "Fixture.Movie.2026.2160p.BluRay.x265.DTS-FIXTURE", normal.Title)
	assert.Equal(t, "虚构副标题二 | 类型：动作冒险 | 语言：英語", normal.Subtitle)
	assert.Equal(t, v2.DiscountNone, normal.DiscountLevel)
	assert.True(t, normal.DiscountEndTime.IsZero(), "no discount -> no end time")
	assert.Equal(t, int64(13260711526), normal.SizeBytes)
	assert.Equal(t, 203, normal.Seeders)
	assert.Equal(t, 2, normal.Leechers)
	assert.Equal(t, 88, normal.Snatched)
}

// --- Suite: Detail ---

func testQingwaptDetail(t *testing.T) {
	def := getQingwaptDef(t)

	t.Run("Free", func(t *testing.T) {
		doc := FixtureDoc(t, "qingwapt_detail", qingwaptDetailFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "195705", info.TorrentID)
		assert.Equal(t, "Fixture.Anime.S01E15-E17.2026.1080p.WEB-DL.H.264.AAC-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		require.False(t, info.DiscountEnd.IsZero(), "discount end time should be parsed")
		assert.Equal(t, 2026, info.DiscountEnd.Year())
		assert.Equal(t, 7, int(info.DiscountEnd.Month()))
		assert.Equal(t, 29, info.DiscountEnd.Day())
		// 「基本信息」标签单元格 + .Next() 兄弟节点，全角冒号 + 十进制 GB
		assert.InDelta(t, 1.44*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})

	t.Run("WithHR", func(t *testing.T) {
		doc := FixtureDoc(t, "qingwapt_detail_hr", qingwaptDetailWithHRFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "195999", info.TorrentID)
		assert.Equal(t, "Fixture.HR.Show.S01.WEB-DL.1080p-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel)
		assert.True(t, info.DiscountEnd.IsZero())
		assert.InDelta(t, 8.5*1024, info.SizeMB, 0.1)
		assert.True(t, info.HasHR, "should detect HR from hit_run.gif")
	})
}

// --- Suite: UserInfo ---

func testQingwaptUserInfo(t *testing.T) {
	def := getQingwaptDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "qingwapt_index", qingwaptIndexFixture)
		fields := map[string]string{
			"id":       "708159",
			"name":     "fixture_user",
			"seeding":  "370",
			"leeching": "0",
			// 蝌蚪 260,200.3 —— parseNumber 去掉千分位
			"bonus": "260200.3",
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
		doc := FixtureDoc(t, "qingwapt_userdetails", qingwaptUserdetailsFixture)
		fields := map[string]string{
			// 3.131 TB / 1.538 TB / 2.070 TB / 3.758 TB 按 1024 进制换算为字节
			"uploaded":       "3442570906566",
			"downloaded":     "1691048883519",
			"trueUploaded":   "2275989069496",
			"trueDownloaded": "4131964697182",
			"ratio":          "2.035",
			"levelName":      "Crazy User",
			// parseTime 按站点 TimezoneOffset(+0800) 解析：
			// 2024-12-17 23:11:39 +0800 / 2026-07-27 14:49:58 +0800
			"joinTime":     "1734448299",
			"lastAccessAt": "1785134998",
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

func TestQingwapt_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      qingwaptSearchFixture,
		"detail":      qingwaptDetailFixture,
		"detail_hr":   qingwaptDetailWithHRFixture,
		"index":       qingwaptIndexFixture,
		"userdetails": qingwaptUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
