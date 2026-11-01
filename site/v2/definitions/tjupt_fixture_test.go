package definitions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func init() {
	RegisterFixtureSuite(FixtureSuite{
		SiteID:   "tjupt",
		Search:   testTJUPTSearch,
		Detail:   testTJUPTDetail,
		UserInfo: testTJUPTUserInfo,
	})
}

// 下面的页面按 2026-11-01 在站点上看到的结构手写（标题、人名、编号都换成了示例值）。

// 列表：第一行是表头，最后一行是「全选」那一行；标题格里套了两行的 table.torrentname，第二行是标签与促销图标。
// 第一行是驻站置顶种子；第二行是 2X、被救活的种子，标签格里有救活者的 userdetails.php 链接；第三行是新种，副标题里有 & 和引号。
const tjuptSearchFixture = `<html><body>
<table id="info_block"><tr><td class="bottom">欢迎, <span class="nowrap"><a href="userdetails.php?id=140001" class="InsaneUser_Name"><b><span>demo</span></b></a></span></td></tr></table>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%">
<tr><td class="colhead" style="padding: 0">类型</td><td class="colhead"><a href="?sort=1&amp;type=asc">标题</a></td><td class="colhead"><img class="comments" src="pic/trans.gif" alt="comments" title="评论数" /></td><td class="colhead"><img class="time" src="pic/trans.gif" alt="time" title="添加时间" /></td><td class="colhead"><img class="size" src="pic/trans.gif" alt="size" title="大小" /></td><td class="colhead"><img class="seeders" src="pic/trans.gif" alt="seeders" title="种子数" /></td><td class="colhead"><img class="leechers" src="pic/trans.gif" alt="leechers" title="下载数" /></td><td class="colhead"><img class="snatched" src="pic/trans.gif" alt="snatched" title="完成数" /></td><td class="colhead">发布者</td></tr>
<tr class="double_sticky_bg"><td class="rowfollow nowrap" valign="middle" style="padding: 0"><a href="?cat=409"><img class="c_game" src="pic/cattrans.gif" alt="游戏" title="游戏" /></a></td><td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr class="double_sticky_bg"><td class="embedded" width="16px"><div><img class="sticky_2" src="pic/trans.gif" alt="Sticky" title="置顶 至 2026-11-01T20:35:49 " /><input type="checkbox" name="checked_torrent[]" value="644629" /></div></td><td class="embedded"><a title="[PC][RPG][示例游戏][DemoGame v1.9.638-DEMO][压缩包]" href="details.php?id=644629&amp;hit=1" target="_blank"><b>[PC][RPG][示例游戏][DemoGame v1.9.638-DEMO][压缩包]</b></a><br />示例游戏 | v1.6.15 | 压缩包</td><td width="40" class="embedded"><a href="/download.php?id=644629"><img class="download" src="pic/trans.gif" alt="download" title="下载种子文件" /></a> <a id="bookmark0" href="javascript: bookmark(644629,0);"><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td></tr><tr><td class="embedded" align="left"></td><td class="embedded" align="center"><div id="tag" class="tag-group"><a href="?tag[]=internal_team"><span title="驻站小组发布的作品" class="tag tag-internal-team">驻站</span></a></div></td><td class="embedded" align="right"></td></tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=644629&amp;type=1" title="添加评论">0</a></td><td class="rowfollow nowrap">2026-10-31<br />20:35:49</td><td class="rowfollow">1.06<br />GiB</td><td class="rowfollow" align="center"><b><a href="details.php?id=644629&amp;hit=1&amp;dllist=1#seeders" target="_blank">7</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><a href="viewsnatches.php?id=644629" target="_blank"><b>11</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr class="normal_bg"><td class="rowfollow nowrap" valign="middle" style="padding: 0"><a href="?cat=401"><img class="c_movies" src="pic/cattrans.gif" alt="电影" title="电影" /></a></td><td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr class="normal_bg"><td class="embedded" width="16px"><div><input type="checkbox" name="checked_torrent[]" value="179385" /></div></td><td class="embedded"><a title="[美国][示例电影][Demo Movie 1941 1080p BluRay x264 DTS-DEMO]" href="details.php?id=179385&amp;hit=1" target="_blank"><b>[美国][示例电影][Demo Movie 1941 1080p BluRay x264 DTS-DEMO]</b></a><br />示例电影/又名</td><td width="40" class="embedded"><a href="/download.php?id=179385"><img class="download" src="pic/trans.gif" alt="download" title="下载种子文件" /></a></td></tr><tr><td class="embedded" align="left"></td><td class="embedded" align="center"><div id="tag" class="tag-group"><span title="由reviver于2026-11-01T10:47:38救活" class="tag tag-reseed">救活</span>&nbsp;由 <span class="nowrap"><a href="userdetails.php?id=124001" target="_blank" class="EliteUser_Name"><b><span>reviver</span></b></a></span> 救活 <img class="pro_2up" src="pic/trans.gif" alt="2X" title="2X" /></div></td><td class="embedded" align="right"><a href="https://www.imdb.com/title/tt0033870" target="_blank"><img class="imdb" src="pic/trans.gif" alt="imdb" title="IMDb评分" /> <span style="font-size: 8pt">NA</span></a></td></tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=179385&amp;type=1" title="添加评论">0</a></td><td class="rowfollow nowrap">2019-04-21<br />08:28:25</td><td class="rowfollow">11.11<br />GiB</td><td class="rowfollow" align="center"><b><a href="details.php?id=179385&amp;hit=1&amp;dllist=1#seeders" target="_blank">1</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=179385&amp;hit=1&amp;dllist=1#leechers" target="_blank">2</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=179385" target="_blank"><b>5</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr class="normal_bg"><td class="rowfollow nowrap" valign="middle" style="padding: 0"><a href="?cat=402"><img class="c_tvseries" src="pic/cattrans.gif" alt="剧集" title="剧集" /></a></td><td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr class="normal_bg"><td class="embedded" width="16px"><div><input type="checkbox" name="checked_torrent[]" value="644867" /></div></td><td class="embedded"><a title="[日剧][示例][DEMO 2026 S01E03 1080p NF WEB-DL H264 AAC-DEMO]" href="details.php?id=644867&amp;hit=1" target="_blank"><b>[日剧][示例][DEMO 2026 S01E03 1080p NF WEB-DL H264 AAC-DEMO]</b></a><b> (<font class="new">新</font>)</b><br />示例 | 第03集 | Tom &amp; Jerry &quot;特别篇&quot; [日语/简繁中字]</td><td width="40" class="embedded"><a href="/download.php?id=644867"><img class="download" src="pic/trans.gif" alt="download" title="下载种子文件" /></a></td></tr><tr><td class="embedded" align="left"></td><td class="embedded" align="center"><div id="tag" class="tag-group"><a href="?tag[]=chinese"><span title="该种子包含中文字幕" class="tag tag-chinese">中字</span></a></div></td><td class="embedded" align="right"></td></tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=644867&amp;type=1" title="添加评论">0</a></td><td class="rowfollow nowrap">2026-11-01<br />12:00:01</td><td class="rowfollow">1.58<br />GiB</td><td class="rowfollow" align="center"><b><a href="details.php?id=644867&amp;hit=1&amp;dllist=1#seeders" target="_blank">1,234</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=644867&amp;hit=1&amp;dllist=1#leechers" target="_blank">16</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><span class="nowrap"><a href="userdetails.php?id=86001" target="_blank" class="InsaneUser_Name"><b><span>uploader</span></b></a></span></td>
</tr>
<tr><td class="rowfollow" colspan="10"><a href="#" onclick="set_checked_torrent(true); return false;"><b>全选</b></a> 选中 <span id="numChecked">0</span> / 100 项</td></tr>
</table>
</body></html>`

// 详情页：标题与编号在隐藏的 input 里；「基本信息」一格有大小（冒号后是 &nbsp;）与本种的促销图标；
// 「其它版本」表里另一个种子是免费的，不能算到本种头上。
const tjuptDetailFixture = `<html><body>
<h1 style="text-align: center" id="top" class="pagetitle">[美国][示例电影][Demo Movie 1941 1080p BluRay x264 DTS-DEMO]</h1>
<input type="hidden" name="torrent_name" value="[美国][示例电影][Demo Movie 1941 1080p BluRay x264 DTS-DEMO]" />
<input type="hidden" name="detail_torrent_id" value="179385" />
<table width="97%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead nowrap" valign="top" align="right">种子名称</td><td class="rowfollow" width="87%" align="left"><a class="index" href="/download.php?id=179385">[TJUPT].Demo.Movie.1941.1080p.BluRay.x264.DTS-DEMO.torrent</a></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right"><font color="red">Hit&amp;Run</font></td><td class="rowfollow" valign="top" align="left">当下载完成后，你需要在<b>20天内</b>累计做种约<b>2天</b>(期间可间断)，否则你将失去<b>2.22</b>点H&amp;R积分。</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b>种子ID:</b>&nbsp;<a href="javascript:;" id="copy_torrent_id">179385</a><span style="margin:0 4px;">|</span><b><b>大小</b>:</b>&nbsp;11.11 GiB<span style="margin:0 4px;">|</span><b>类型:</b>&nbsp;电影<span style="margin:0 4px;">|</span><b>特性：</b><span id="tag" class="tag-group"><span title="由reviver于2026-11-01 10:47:38救活" class="tag tag-reseed">救活</span>&nbsp;由 <span class="nowrap"><a href="userdetails.php?id=124001" class="EliteUser_Name"><b><span>reviver</span></b></a></span> 救活</span> <img class="pro_2up" src="pic/trans.gif" alt="2X" title="2X" /></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">示例电影/又名</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">其它版本</td><td class="rowfollow" valign="top" align="left"><table class="coltable table-left"><tr><td class="colhead">类型</td><td class="colhead">标题</td></tr><tr><td class="rowfollow">电影</td><td class="rowfollow"><a href="details.php?id=620473&amp;hit=1"><strong>[美国][示例电影][Demo Movie 1941 1080p WEBRip-DEMO]</strong></a> <img class="pro_free" src="pic/trans.gif" alt="Free" title="免费" /></td></tr></table></td></tr>
</table>
</body></html>`

// 首页顶栏（#info_block）：等级、魔力值、做种积分、H&R 积分、上传量与做种下载数；没有下载量和分享率。
const tjuptIndexFixture = `<html><body>
<table id="info_block"><tr>
<td class="bottom" style="text-align: left; padding-left:5px">
欢迎, <span class="nowrap"><a style="color:#17e2c6;" href="userdetails.php?id=140001" target="_blank" class="InsaneUser_Name"><b><span style="text-decoration: none;">demo</span></b></a></span> [<a href="logout.php">退出</a>] [<a href="torrents.php?inclbookmarked=1&amp;allsec=1&amp;incldead=0">收藏</a>]
<span class="color_bonus"> 等级: </span>[<a href="/classes.php#5"><span class="InsaneUser_Name" style="font-weight: bold">小有名气</span></a>]
<span class="color_bonus"> 魔力值  </span>[<a href="mybonus.php">使用</a>|<a href="mybonusapps.php">茉莉园</a>]: <a href="bonus.php?show=log">36,218.4</a>
<span class="color_bonus">做种积分: </span><a href="bonus.php?show=description">1,864,215.6</a>
<span class="color_invite"> 邀请: </span> <a href="invite.php?id=140001" title="邀请码数量">2</a><br />
<span class="color_bonus">H&amp;R积分 </span>[<a href="/hnr_bonus.php">日志</a>|<a href="/hnr_details.php">详情</a>]: <a href="/hnr_bonus.php">100</a>(<a href="/hnr_details.php">0</a>)
<span class="color_uploaded">上传量:</span> 5.312 TiB <span class="color_active"> 上传排名:</span> <a href="/topten.php">8137</a>&nbsp;
<span class="color_active">活动种子:</span>
<img class="arrowup" alt="Torrents seeding" title="当前做种数" src="pic/trans.gif" />93 <img class="arrowdown" alt="Torrents leeching" title="当前下载数" src="pic/trans.gif" />0&nbsp;
<span class="color_connectable">种子网络: </span><a href="userdetails.php?id=140001" title="IPv4 / IPv6">ok</a>
</td>
<td class="bottom" style="text-align: right">[<a href="attendance.php" class="faqlink">签到得魔力</a>]&nbsp;</td>
</tr></table>
</body></html>`

const tjuptUserdetailsFixture = `<html><body>
<table width="100%" border="1" cellspacing="0" cellpadding="5">
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">加入日期</td><td width="99%" class="rowfollow" valign="top" align="left">2022-09-05 08:30:12 (<span title="2022-09-05 08:30:12">4年1月前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-10-30 21:15:40 (<span title="2026-10-30 21:15:40">1天前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">上传量</td><td width="99%" class="rowfollow" valign="top" align="left">5.312 TiB</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">H&amp;R积分</td><td width="99%" class="rowfollow" valign="top" align="left">100 <a href="hnr_details.php?uid=140001" target="_blank"><b>[查看H&amp;R记录]</b></a></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">等级</td><td width="99%" class="rowfollow" valign="top" align="left"><img alt="小有名气" title="小有名气" src="pic/insane.gif" /> &nbsp;&nbsp;<a href="classes.php#6"><b>等级详情/提升等级</b></a></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">签到天数</td><td width="99%" class="rowfollow" valign="top" align="left">连续5天(共108天)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">魔力值</td><td width="99%" class="rowfollow" valign="top" align="left"><a href="bonus.php?show=log">36230.9</a></td></tr>
</table>
</body></html>`

// 时魔在「做种积分&魔力值说明」页
const tjuptBonusFixture = `<html><body><table><tr><td id="outer">
<h1>做种每小时将得到如下的做种积分&amp;魔力值</h1><ul><li><b>L</b>为参数。<b>L</b> = 300</li></ul>
<div align="center">你当前每小时能获取112.58个魔力值 (A = 6533.9)</div>
<h1>其他获取方式</h1><ul><li>发布新种子 = 10个魔力值</li></ul>
</td></tr></table></body></html>`

// 做种列表（getusertorrentlistajax.php?type=seeding）开头的汇总
const tjuptSeedingFixture = `<b>93</b>条记录 - 总大小4.215 TiB<br /><table class='coltable table-left torrentlist'><tr><td class="colhead">类型</td><td class="colhead">标题</td><td class="colhead"><img class="size" src="pic/trans.gif" alt="size" title="大小" /></td></tr>
<tr><td class="rowfollow">软件</td><td class="rowfollow"><a href="details.php?id=1&amp;hit=1"><b>[Windows][示例][2.21GiB][安装包]</b></a></td><td class="rowfollow">3.00<br />GiB</td></tr></table>`

func getTJUPTDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("tjupt")
	require.True(t, ok, "tjupt definition not found")
	return def
}

func testTJUPTSearch(t *testing.T) {
	def := getTJUPTDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(tjuptSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 3, "表头与「全选」那一行不算种子")

	sticky := items[0]
	assert.Equal(t, "644629", sticky.ID)
	assert.Equal(t, "[PC][RPG][示例游戏][DemoGame v1.9.638-DEMO][压缩包]", sticky.Title)
	assert.Equal(t, "示例游戏 | v1.6.15 | 压缩包", sticky.Subtitle)
	assert.Equal(t, v2.DiscountNone, sticky.DiscountLevel)
	size := 1.06
	assert.Equal(t, int64(size*1024*1024*1024), sticky.SizeBytes)
	assert.Equal(t, 7, sticky.Seeders)
	assert.Equal(t, 0, sticky.Leechers)
	assert.Equal(t, 11, sticky.Snatched)
	assert.Equal(t, "游戏", sticky.Category)
	// 时间格是「2026-10-31<br />20:35:49」，取文字时两段连在一起；与别的站一样按 UTC 记
	assert.Equal(t, int64(1793478949), sticky.UploadedAt)

	reseeded := items[1]
	assert.Equal(t, "179385", reseeded.ID)
	assert.Equal(t, "[美国][示例电影][Demo Movie 1941 1080p BluRay x264 DTS-DEMO]", reseeded.Title, "救活者的 userdetails.php 链接不能拼进标题")
	assert.Equal(t, "示例电影/又名", reseeded.Subtitle)
	assert.Equal(t, v2.Discount2xUp, reseeded.DiscountLevel, "促销图标在标题格第二行的标签里")
	assert.True(t, reseeded.DiscountEndTime.IsZero(), "2X 没有结束时间")
	assert.Equal(t, 2, reseeded.Leechers)

	fresh := items[2]
	assert.Equal(t, "644867", fresh.ID)
	assert.Equal(t, `示例 | 第03集 | Tom & Jerry "特别篇" [日语/简繁中字]`, fresh.Subtitle, "实体要还原，标题后的（新）不算副标题")
	assert.Equal(t, 1234, fresh.Seeders)
	assert.Equal(t, 16, fresh.Leechers)
	assert.Equal(t, 0, fresh.Snatched)
}

func testTJUPTDetail(t *testing.T) {
	def := getTJUPTDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "tjupt_detail", tjuptDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "179385", info.TorrentID)
	assert.Equal(t, "[美国][示例电影][Demo Movie 1941 1080p BluRay x264 DTS-DEMO]", info.Title)
	assert.Equal(t, v2.Discount2xUp, info.DiscountLevel, "只看本种「基本信息」里的图标，不看其它版本的免费")
	assert.True(t, info.DiscountEnd.IsZero())
	assert.Empty(t, info.DiscountErr)
	assert.InDelta(t, 11.11*1024, info.SizeMB, 0.1)
	assert.True(t, info.HasHR)
}

func testTJUPTUserInfo(t *testing.T) {
	def := getTJUPTDef(t)
	driver := newTestNexusPHPDriver(def)

	pages := []struct {
		name    string
		fixture string
		want    map[string]string
	}{
		{"index", tjuptIndexFixture, map[string]string{
			"id":           "140001",
			"name":         "demo",
			"levelName":    "小有名气",
			"bonus":        "36218.4",
			"seedingBonus": "1.8642156e+06",
			"uploaded":     "5840605766746",
			"seeding":      "93",
			"leeching":     "0",
		}},
		{"userdetails", tjuptUserdetailsFixture, map[string]string{
			"joinTime":     "1662337812",
			"lastAccessAt": "1793366140",
		}},
		{"bonus", tjuptBonusFixture, map[string]string{
			"bonusPerHour": "112.58",
		}},
	}
	for _, page := range pages {
		t.Run(page.name, func(t *testing.T) {
			doc := FixtureDoc(t, "tjupt_"+page.name, page.fixture)
			for field, want := range page.want {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q missing", field)
				assert.Equal(t, want, driver.ExtractFieldValuePublic(doc, sel), "field %q", field)
			}
		})
	}
}

// 走 GetUserInfo 全流程：三页各取各的字段，时魔那一页带着 ?show=description，做种体积用做种列表开头的汇总。
func TestTJUPT_GetUserInfo(t *testing.T) {
	def := getTJUPTDef(t)

	var mu sync.Mutex
	queries := map[string]string{}
	page := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			queries[r.URL.Path] = r.URL.RawQuery
			mu.Unlock()
			_, _ = w.Write([]byte(body))
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/index.php", page(tjuptIndexFixture))
	mux.HandleFunc("/userdetails.php", page(tjuptUserdetailsFixture))
	mux.HandleFunc("/bonus.php", page(tjuptBonusFixture))
	mux.HandleFunc("/getusertorrentlistajax.php", page(tjuptSeedingFixture))
	server := httptest.NewServer(mux)
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	info, err := driver.GetUserInfo(context.Background())
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "id=140001", queries["/userdetails.php"])
	assert.Equal(t, "show=description", queries["/bonus.php"])
	assert.Contains(t, queries["/getusertorrentlistajax.php"], "type=seeding")

	tib := 4.215
	assert.Equal(t, "140001", info.UserID)
	assert.Equal(t, "demo", info.Username)
	assert.Equal(t, "小有名气", info.LevelName)
	assert.Equal(t, int64(5840605766746), info.Uploaded)
	assert.Zero(t, info.Downloaded, "北洋园不记下载量")
	assert.InDelta(t, 36218.4, info.Bonus, 0.01)
	assert.InDelta(t, 1864215.6, info.SeedingBonus, 0.01)
	assert.InDelta(t, 112.58, info.BonusPerHour, 0.001)
	assert.Equal(t, 93, info.Seeding)
	assert.Equal(t, int64(tib*1024*1024*1024*1024), info.SeederSize)
	assert.Equal(t, int64(1662337812), info.JoinDate)
	assert.Equal(t, int64(1793366140), info.LastAccess)
}

// 全站 H&R，按体积分档：不超过 10GiB 1 天，往上每 10GiB 加 1 天，超过 50GiB 7 天。
func TestTJUPT_HRSeedTime(t *testing.T) {
	def := getTJUPTDef(t)
	gib := int64(1024 * 1024 * 1024)
	cases := []struct {
		size  int64
		hours int
	}{
		{gib, 24},
		{15 * gib, 48},
		{25 * gib, 72},
		{35 * gib, 96},
		{45 * gib, 120},
		{80 * gib, 168},
	}
	assert.True(t, def.HREnabled)
	for _, c := range cases {
		assert.Equal(t, c.hours, def.CalcHRSeedTimeH(c.size), "size %d GiB", c.size/gib)
	}
}

func TestTJUPT_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      tjuptSearchFixture,
		"detail":      tjuptDetailFixture,
		"index":       tjuptIndexFixture,
		"userdetails": tjuptUserdetailsFixture,
		"bonus":       tjuptBonusFixture,
		"seeding":     tjuptSeedingFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
