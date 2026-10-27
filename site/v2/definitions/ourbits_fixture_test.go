package definitions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func init() {
	RegisterFixtureSuite(FixtureSuite{
		SiteID:   "ourbits",
		Search:   testOurbitsSearch,
		Detail:   testOurbitsDetail,
		UserInfo: testOurbitsUserInfo,
	})
}

// 站点真实 HTML 结构（来自 issue #329 附件 ourbits.club-*.zip）的脱敏版本：
//   - 10 列 NexusPHP 标准表格：cat / title / 评论 / 时间 / 大小 / 种子 / 下载 / 完成 / 进度 / 发布者
//   - 详情页 h1 嵌入 free 折扣 + onmouseover 含剩余时间
//   - userinfo: index 含 #info_block、userdetails 标准 NexusPHP 行
const ourbitsSearchFixture = `<html><body>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%" id="torrenttable">
<tr><td class="colhead">类型</td><td class="colhead">标题</td><td class="colhead">评论</td><td class="colhead">时间</td><td class="colhead">大小</td><td class="colhead">种子数</td><td class="colhead">下载数</td><td class="colhead">完成数</td><td class="colhead">进度</td><td class="colhead">发布者</td></tr>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=401"><img class="c2_movies" alt="Movies" title="Movies" /></a></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a title="Demo.Movie.2026.UHD.BluRay.2160p" href="details.php?id=10001&amp;hit=1"><b>Demo.Movie.2026.UHD.BluRay.2160p</b></a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-05-15 19:26:44&quot;&gt;9时35分&lt;/span&gt;&lt;/b&gt;', 'trail', false);" />
    &nbsp;剩余时间：<b><span title="2026-05-15 19:26:44">9时35分</span></b>
    <br/>演示电影副标题
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow nowrap"><span title="2026-05-14 22:00:00">14时24分</span></td>
  <td class="rowfollow">86.63 GB</td>
  <td class="rowfollow">233</td>
  <td class="rowfollow">14</td>
  <td class="rowfollow">492</td>
  <td>-</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=402"><img class="c2_tv" alt="TV" title="TV" /></a></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a title="Demo.TV.S01E01.2026.1080p" href="details.php?id=10002"><b>Demo.TV.S01E01.2026.1080p</b></a>
    <img class="pro_50pctdown" src="pic/trans.gif" alt="50%" title="50%" />
    <img class="hitandrun" src="pic/trans.gif" alt="H&amp;R" title="H&amp;R" />
    <br/>演示剧集副标题
  </td></tr></table></td>
  <td class="rowfollow">2</td>
  <td class="rowfollow nowrap"><span title="2026-05-13 12:00:00">1天</span></td>
  <td class="rowfollow">2.34 GB</td>
  <td class="rowfollow">42</td>
  <td class="rowfollow">8</td>
  <td class="rowfollow">17</td>
  <td>-</td>
  <td class="rowfollow">DemoUploader</td>
</tr>
</table>
</body></html>`

const ourbitsIndexFixture = `<html><body>
<table id="info_block"><tr><td><span class="medium">
欢迎回来, <span class="nowrap"><a href="userdetails.php?id=70001" target="_blank" class='User_Name'><b>ob_demo_user</b></a></span>
[<a href="logout.php?token=REDACTED">退出</a>]
<span class="color_bonus">魔力值 </span>[<a href="mybonus.php">使用</a>]: 33,739.5
&nbsp;(签到已得85)
<span class="color_invite">邀请 </span>[<a href="invite.php?id=70001">发送</a>]: 0/0<br/>
<span class="color_ratio">分享率：</span> 9.258
<font class='color_uploaded'>上传量：</font> 337.04 GB
<font class='color_downloaded'> 下载量：</font> 36.41 GB
<font class='color_active'>当前活动：</font>
<img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />22
<img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />0
&nbsp;<font class="color_connectable">H&R: </font>0/10
</span></td></tr></table>
</body></html>`

const ourbitsUserdetailsFixture = `<html><body>
<table>
  <tr><td class="rowhead nowrap" valign="top" align="right">加入日期</td><td class="rowfollow">2025-11-20 20:26:31 (<span title="2025-11-20 20:26:31">5月25天前</span>)</td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">最近动向</td><td class="rowfollow">网站访问: 2026-05-15 02:49:47 (7时1分前)</td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">传输</td><td class="rowfollow">
    <table>
      <tr><td>分享率: <font color="">1,930.915</font></td></tr>
      <tr><td>上传量: 195.215 TB</td><td>下载量: 103.53 GB</td></tr>
      <tr><td>真实分享率: 145.107</td></tr>
      <tr><td>真实上传量: 99.104 TB</td><td>真实下载量: 699.36 GB</td></tr>
    </table>
  </td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">等级</td><td class="rowfollow"><img title="Power User" src="pic/poweruser.gif" /></td></tr>
</table>
</body></html>`

const ourbitsMyBonusFixture = `<html><body>
<div id="outer">
您当前每小时能获取 18.50 个魔力值
</div>
</body></html>`

const ourbitsDetailFixture = `<html><body>
<h1 align="center" id="top">Demo.Movie.2026.UHD.BluRay.2160p&nbsp; <b>[<font class='free' onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-05-15 19:26:44&quot;&gt;9时35分&lt;/span&gt;&lt;/b&gt;', 'trail', false);">免费</font>]</b> 剩余时间：<b><span title="2026-05-15 19:26:44">9时35分</span></b></h1>
<table>
  <tr><td class="rowhead">下载</td><td class="rowfollow"><a href="download.php?id=10001">demo.torrent</a></td></tr>
  <tr><td class="rowhead">副标题</td><td class="rowfollow">演示电影副标题</td></tr>
  <tr><td class="rowhead">基本信息</td><td class="rowfollow">大小：86.63 GB 类型: Movies 媒介: UHD Blu-ray 编码: HEVC 音频编码: TrueHD 分辨率: 2160p 制作组: OurBits</td></tr>
</table>
</body></html>`

// 2026-10-27 用户开着的种子页（脱敏，标题与编号是演示用的）：促销显示方式是默认的「图标」，
// 图标后面另有一段「剩余时间：<b><span title=…>」；H&R 是 img.hitandrun；副标题是 <br> 之后的裸文本，
// 前面可能隔着一块标签 <div>。置顶的种子另带 img[alt=Sticky]。
const ourbitsListIconFixture = `<html><body>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%" id="torrenttable"><tbody>
<tr><td class="colhead">类型</td><td class="colhead">标题</td><td class="colhead">评论</td><td class="colhead">时间</td><td class="colhead">大小</td><td class="colhead">种子数</td><td class="colhead">下载数</td><td class="colhead">完成数</td><td class="colhead">进度</td><td class="colhead">发布者</td></tr>
<tr class="sticky_top_more">
  <td class="rowfollow nowrap"><a href="?cat=401"><img class="c_movies" alt="Movies" title="Movies"></a></td>
  <td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tbody><tr class="sticky_top_more"><td class="embedded"><img src="pic/sticky3.png" alt="Sticky" title="高级置顶">&nbsp;<a title="Demo.Movie.2026.2160p.UHD.Blu-ray-OurBits" href="details.php?id=20001&amp;hit=1"><b>Demo.Movie.2026.2160p.UHD.Blu-ray-OurBits</b></a><b> (<font class="new">新</font>)</b> <img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-10-28 10:02:58&quot;&gt;23时3分&lt;/span&gt;&lt;/b&gt;', 'trail', false);">&nbsp;剩余时间：<b><span title="2026-10-28 10:02:58">23时3分</span></b><img class="hitandrun" src="pic/trans.gif" alt="H&amp;R" title="H&amp;R"><br><div style="display:inline-block;height:14px;line-height: 12px;padding-right: 5px"><a href="?tags[]=gf"><span class="tag tag-gf">官方</span></a><a href="?tags[]=zz"><span class="tag tag-zz">中字</span></a></div>演示电影 [原盘中字] &amp; 双语</td><td class="embedded" width="20"><a href="download.php?id=20001"><img class="download" src="pic/trans.gif" alt="download" title="下载本种"></a></td></tr></tbody></table></td>
  <td class="rowfollow"><a href="comment.php?action=add&amp;pid=20001&amp;type=torrent">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-10-27 10:02:58">1时<br>2分</span></td>
  <td class="rowfollow">83.66<br>GB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=20001&amp;hit=1&amp;dllist=1#seeders">1</a></b></td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=20001&amp;hit=1&amp;dllist=1#leechers">87</a></b></td>
  <td class="rowfollow"><a href="viewsnatches.php?id=20001">0</a></td>
  <td class="rowfollow nowrap">-</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=402"><img class="c_tvseries" alt="TV-Series" title="TV-Series"></a></td>
  <td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tbody><tr><td class="embedded"><a title="Demo.Show.S01.2160p.WEB-DL-OurTV" href="details.php?id=20002&amp;hit=1"><b>Demo.Show.S01.2160p.WEB-DL-OurTV</b></a> <img class="pro_50pctdown" src="pic/trans.gif" alt="50%" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;halfdown&quot;&gt;50%&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-10-27 15:46:14&quot;&gt;4时43分&lt;/span&gt;&lt;/b&gt;', 'trail', false);">&nbsp;剩余时间：<b><span title="2026-10-27 15:46:14">4时43分</span></b><br>演示剧集 第一季</td></tr></tbody></table></td>
  <td class="rowfollow">2</td>
  <td class="rowfollow nowrap"><span title="2026-10-26 22:10:00">12时</span></td>
  <td class="rowfollow">34.37<br>GB</td>
  <td class="rowfollow" align="center">1</td>
  <td class="rowfollow" align="center">1</td>
  <td class="rowfollow">0</td>
  <td class="rowfollow nowrap">-</td>
  <td class="rowfollow">demo_uploader</td>
</tr>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=401"><img class="c_movies" alt="Movies" title="Movies"></a></td>
  <td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tbody><tr><td class="embedded"><a title="Demo.Remux.2019.1080p.Blu-ray-OurBits" href="details.php?id=20003&amp;hit=1"><b>Demo.Remux.2019.1080p.Blu-ray-OurBits</b></a> <img class="pro_50pctdown2up" src="pic/trans.gif" alt="2X 50%" title="2X 50%"><br>演示原盘</td></tr></tbody></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow nowrap"><span title="2026-10-20 08:00:00">7天</span></td>
  <td class="rowfollow">25.04<br>GB</td>
  <td class="rowfollow" align="center">12</td>
  <td class="rowfollow" align="center">0</td>
  <td class="rowfollow">30</td>
  <td class="rowfollow nowrap">-</td>
  <td class="rowfollow">demo_uploader</td>
</tr>
</tbody></table>
</body></html>`

// 控制面板「网站设定 → 种子页面 → 促销种子显示方式」选「文字」时，促销写成 [<font class='…'>…</font>]，
// 与详情页标题里的一样（同一个函数的文字形式）；剩余时间照样跟在后面。类名是 NexusPHP 的 free / twoup /
// twoupfree / halfdown / twouphalfdown / thirtypercent。
const ourbitsListWordFixture = `<html><body>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%" id="torrenttable"><tbody>
<tr><td class="colhead">类型</td><td class="colhead">标题</td><td class="colhead">评论</td><td class="colhead">时间</td><td class="colhead">大小</td><td class="colhead">种子数</td><td class="colhead">下载数</td><td class="colhead">完成数</td><td class="colhead">进度</td><td class="colhead">发布者</td></tr>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=401"><img class="c_movies" alt="Movies" title="Movies"></a></td>
  <td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tbody><tr><td class="embedded"><a title="Demo.Word.Free2x.2026.1080p" href="details.php?id=20011&amp;hit=1"><b>Demo.Word.Free2x.2026.1080p</b></a> [<font class='twoupfree' onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;twoupfree&quot;&gt;2X免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-10-29 08:00:00&quot;&gt;1天&lt;/span&gt;&lt;/b&gt;', 'trail', false);">2X免费</font>]&nbsp;剩余时间：<b><span title="2026-10-29 08:00:00">1天</span></b><br>文字形式 2X免费</td></tr></tbody></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow nowrap"><span title="2026-10-27 08:00:00">3时</span></td>
  <td class="rowfollow">8.00<br>GB</td>
  <td class="rowfollow" align="center">5</td>
  <td class="rowfollow" align="center">2</td>
  <td class="rowfollow">1</td>
  <td class="rowfollow nowrap">-</td>
  <td class="rowfollow">demo_uploader</td>
</tr>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=401"><img class="c_movies" alt="Movies" title="Movies"></a></td>
  <td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tbody><tr><td class="embedded"><a title="Demo.Word.Free.2026.1080p" href="details.php?id=20012&amp;hit=1"><b>Demo.Word.Free.2026.1080p</b></a> [<font class='free' onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-10-28 20:30:00&quot;&gt;20时&lt;/span&gt;&lt;/b&gt;', 'trail', false);">免费</font>]&nbsp;剩余时间：<b><span title="2026-10-28 20:30:00">20时</span></b><br>文字形式 免费</td></tr></tbody></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow nowrap"><span title="2026-10-27 08:00:00">3时</span></td>
  <td class="rowfollow">4.00<br>GB</td>
  <td class="rowfollow" align="center">5</td>
  <td class="rowfollow" align="center">2</td>
  <td class="rowfollow">1</td>
  <td class="rowfollow nowrap">-</td>
  <td class="rowfollow">demo_uploader</td>
</tr>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=401"><img class="c_movies" alt="Movies" title="Movies"></a></td>
  <td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tbody><tr><td class="embedded"><a title="Demo.Word.Half.2026.1080p" href="details.php?id=20013&amp;hit=1"><b>Demo.Word.Half.2026.1080p</b></a> [<font class='halfdown'>50%</font>]<br>文字形式 50%</td></tr></tbody></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow nowrap"><span title="2026-10-27 08:00:00">3时</span></td>
  <td class="rowfollow">2.00<br>GB</td>
  <td class="rowfollow" align="center">5</td>
  <td class="rowfollow" align="center">2</td>
  <td class="rowfollow">1</td>
  <td class="rowfollow nowrap">-</td>
  <td class="rowfollow">demo_uploader</td>
</tr>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=401"><img class="c_movies" alt="Movies" title="Movies"></a></td>
  <td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tbody><tr><td class="embedded"><a title="Demo.Word.TwoUpHalf.2026.1080p" href="details.php?id=20014&amp;hit=1"><b>Demo.Word.TwoUpHalf.2026.1080p</b></a> [<font class='twouphalfdown'>2X 50%</font>] [<font class='new'>新</font>]<br>文字形式 2X 50%</td></tr></tbody></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow nowrap"><span title="2026-10-27 08:00:00">3时</span></td>
  <td class="rowfollow">2.00<br>GB</td>
  <td class="rowfollow" align="center">5</td>
  <td class="rowfollow" align="center">2</td>
  <td class="rowfollow">1</td>
  <td class="rowfollow nowrap">-</td>
  <td class="rowfollow">demo_uploader</td>
</tr>
</tbody></table>
</body></html>`

func parseOurbitsList(t *testing.T, html string) []v2.TorrentItem {
	t.Helper()
	def := getOurbitsDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()
	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "ourbits_jwt=test", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)
	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: http.MethodGet})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	return items
}

func cst(t *testing.T, s string) time.Time {
	t.Helper()
	at, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.FixedZone("CST", 8*3600))
	require.NoError(t, err)
	return at
}

// 默认的图标形式：优惠、结束时间、H&R 与副标题都要认出来（以前 H&R 与副标题是空的，2X 50% 认成了 50%）。
func TestOurbits_ListIconMode(t *testing.T) {
	items := parseOurbitsList(t, ourbitsListIconFixture)
	require.Len(t, items, 3)

	assert.Equal(t, "20001", items[0].ID)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.True(t, items[0].DiscountEndTime.Equal(cst(t, "2026-10-28 10:02:58")), "结束时间 %v", items[0].DiscountEndTime)
	assert.True(t, items[0].HasHR)
	assert.Equal(t, "演示电影 [原盘中字] & 双语", items[0].Subtitle)
	assert.Equal(t, 1, items[0].Seeders)
	assert.Equal(t, 87, items[0].Leechers)

	assert.Equal(t, v2.DiscountPercent50, items[1].DiscountLevel)
	assert.True(t, items[1].DiscountEndTime.Equal(cst(t, "2026-10-27 15:46:14")), "结束时间 %v", items[1].DiscountEndTime)
	assert.False(t, items[1].HasHR)
	assert.Equal(t, "演示剧集 第一季", items[1].Subtitle)

	assert.Equal(t, v2.Discount2x50, items[2].DiscountLevel)
	assert.True(t, items[2].DiscountEndTime.IsZero(), "长期的促销没有结束时间")
	assert.Equal(t, "演示原盘", items[2].Subtitle)
}

// 「促销种子显示方式」选文字时也认得出（以前一个都认不出：选择器只认图标）。
func TestOurbits_ListWordMode(t *testing.T) {
	items := parseOurbitsList(t, ourbitsListWordFixture)
	require.Len(t, items, 4)

	assert.Equal(t, v2.Discount2xFree, items[0].DiscountLevel)
	assert.True(t, items[0].DiscountEndTime.Equal(cst(t, "2026-10-29 08:00:00")), "结束时间 %v", items[0].DiscountEndTime)
	assert.Equal(t, "文字形式 2X免费", items[0].Subtitle)

	assert.Equal(t, v2.DiscountFree, items[1].DiscountLevel)
	assert.True(t, items[1].DiscountEndTime.Equal(cst(t, "2026-10-28 20:30:00")), "结束时间 %v", items[1].DiscountEndTime)

	assert.Equal(t, v2.DiscountPercent50, items[2].DiscountLevel)
	assert.True(t, items[2].DiscountEndTime.IsZero())

	assert.Equal(t, v2.Discount2x50, items[3].DiscountLevel)
}

func getOurbitsDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("ourbits")
	require.True(t, ok)
	return def
}

func testOurbitsSearch(t *testing.T) {
	def := getOurbitsDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(ourbitsSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: http.MethodGet})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "10001", items[0].ID)
	assert.Equal(t, "Demo.Movie.2026.UHD.BluRay.2160p", items[0].Title)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 233, items[0].Seeders)
	assert.Equal(t, 14, items[0].Leechers)
	assert.Equal(t, 492, items[0].Snatched)

	assert.Equal(t, "10002", items[1].ID)
	assert.Equal(t, v2.DiscountPercent50, items[1].DiscountLevel)
}

func testOurbitsDetail(t *testing.T) {
	def := getOurbitsDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "ourbits_detail", ourbitsDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 88708.92, info.SizeMB, 5.0)
}

func testOurbitsUserInfo(t *testing.T) {
	def := getOurbitsDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "ourbits_index", ourbitsIndexFixture)
	assert.Equal(t, "70001", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "ob_demo_user", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "33739.5", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "22", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "ourbits_userdetails", ourbitsUserdetailsFixture)
	assert.Equal(t, "214641162416291", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "111164491038", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "1930.915", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "Power User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "108966000359112", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["trueUploaded"]))
	assert.Equal(t, "750932082032", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["trueDownloaded"]))

	bonusDoc := FixtureDoc(t, "ourbits_mybonus", ourbitsMyBonusFixture)
	assert.Equal(t, "18.5", driver.ExtractFieldValuePublic(bonusDoc, def.UserInfo.Selectors["bonusPerHour"]))
}

func TestOurbits_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      ourbitsSearchFixture,
		"index":       ourbitsIndexFixture,
		"userdetails": ourbitsUserdetailsFixture,
		"mybonus":     ourbitsMyBonusFixture,
		"detail":      ourbitsDetailFixture,
		"list_icon":   ourbitsListIconFixture,
		"list_word":   ourbitsListWordFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) { RequireNoSecrets(t, name, data) })
	}
}
