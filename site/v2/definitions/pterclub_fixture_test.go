package definitions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func init() {
	RegisterFixtureSuite(FixtureSuite{
		SiteID:   "pterclub",
		Search:   testPTerClubSearch,
		Detail:   testPTerClubDetail,
		UserInfo: testPTerClubUserInfo,
	})
}

// 列表按 2026-11-01 在站点上看到的结构手写（片名、编号换成示例值）：标题格里第一个 div 是置顶图标、标题、促销图标与
// 「剩余时间」，第二个 div 是「官方」「国语」这些标签和副标题 span；评分格（IMDb/豆瓣）里的 span 不是副标题。
// 第三行是游戏种子，链接是 detailsgame.php，列表里不收；第四行是旧版写法的副标题（span.torrents-subtitle）。
const pterclubSearchFixture = `<html><body>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%"><tbody>
<tr><td class="colhead" style="padding: 0px">类型</td><td class="colhead"><a href="?sort=1&amp;type=asc">标题</a></td><td class="colhead"><img class="comments" src="pic/trans.gif" alt="comments" title="评论数" /></td><td class="colhead"><img class="time" src="pic/trans.gif" alt="time" title="存活时间" /></td><td class="colhead"><img class="size" src="pic/trans.gif" alt="size" title="大小" /></td><td class="colhead"><img class="seeders" src="pic/trans.gif" alt="seeders" title="种子数" /></td><td class="colhead"><img class="leechers" src="pic/trans.gif" alt="leechers" title="下载数" /></td><td class="colhead"><img class="snatched" src="pic/trans.gif" alt="snatched" title="完成数" /></td><td class="colhead">发布者</td></tr>
<tr class="sticky_top">
<td class="rowfollow nowrap" valign="middle" style="padding: 0px"><a href="?cat=404"><img class="c_tvseries" src="pic/cattrans.gif" alt="电视剧 (TV Series)" title="电视剧 (TV Series)" /></a><a href="?source5=1"><img class="c_wd" src="pic/cattrans.gif" alt="WEB-DL" title="WEB-DL" /></a></td>
<td class="rowfollow" width="100%" align="left" style="padding: 0px"><table class="torrentname" width="100%"><tbody><tr class="sticky_top"><td class="embedded" style="width: 75px;text-align: center;"><img class="lozad" src="/pic/spinner.svg" data-torrent="905232" alt="preview" /></td><td class="embedded"><div style="padding: 2px;"><div>&nbsp;<img class="sticky" src="pic/trans.gif" alt="Sticky" title="二级置顶 至 2026-11-01 23:26:38" />&nbsp;<a title="Demo Series 2026 S01 E14-E15 2160p WEB-DL H265 AAC-DEMOWEB" href="details.php?id=905232"><b>&nbsp;Demo Series 2026 S01 E14-E15 2160p WEB-DL H265 AAC-DEMOWEB</b></a><b> (<font class="new">新</font>)</b> <b>[<font class="hot">热门</font>]</b> <a href="torrents.php?spstate=2"><img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-11-01 23:26:38&quot;&gt;11时32分&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500);" /></a>&nbsp;剩余时间：<b><span title="2026-11-01 23:26:38">11时32分</span></b></div><div style="margin-top: 4px;"><a class="chs_tag chs_tag-gf torrents-tag" href="torrents.php?tag_internal=yes">官方</a><a class="chs_tag chs_tag-gy torrents-tag" href="torrents.php?tag_mandarin=yes">国语</a>&nbsp;<span>示例剧 第14-15集 | 导演: 甲 主演: 乙 丙  [国语/中字]</span></div></div></td><td class="embedded" width="6px"><div style="line-height: 16px"></div></td><td class="embedded" style="padding-left: 0px;" width="20" valign="middle"><a href="download.php?id=905232"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td><td class="embedded" width="44px"><a href="/torrents.php?search=35324004&amp;search_area=4" style="display: flex; align-items: center;"><img src="pic/imdb.png" title="IMDb评分" /><span data-imdbid="35324004" style="padding-left: 5px">8.3</span></a><a href="/torrents.php?search=35861007&amp;search_area=5" style="margin-top: 2px; display: flex;"><img src="pic/douban.png" title="豆瓣评分" /><span data-doubanid="35861007" style="padding-left: 5px"><font color="gray">N/A</font></span></a></td></tr></tbody></table></td>
<td class="rowfollow"><a href="comment.php?action=add&amp;pid=905232&amp;type=torrent">0</a></td><td class="rowfollow nowrap"><span title="2026-11-01 11:26:38">27分</span></td><td class="rowfollow">9.82<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=905232&amp;dllist=1#seeders">29</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=905232&amp;dllist=1#leechers">3</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=905232"><b>26</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style="padding: 0px"><a href="?cat=404"><img class="c_tvseries" src="pic/cattrans.gif" alt="电视剧 (TV Series)" title="电视剧 (TV Series)" /></a></td>
<td class="rowfollow" width="100%" align="left" style="padding: 0px"><table class="torrentname" width="100%"><tbody><tr><td class="embedded" style="width: 75px;text-align: center;"><img class="lozad" src="/pic/spinner.svg" data-torrent="905262" alt="preview" /></td><td class="embedded"><div style="padding: 2px;"><div>&nbsp;<a title="Demo Short 2026 S01 720p WEB-DL H.265 AAC-DEMO" href="details.php?id=905262"><b>&nbsp;Demo Short 2026 S01 720p WEB-DL H.265 AAC-DEMO</b></a><b> (<font class="new">新</font>)</b> <a href="torrents.php?spstate=4"><img class="pro_free2up" src="pic/trans.gif" alt="2X Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;twoupfree&quot;&gt;2X免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-11-04 11:49:08&quot;&gt;2天23时&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500);" /></a>&nbsp;剩余时间：<b><span title="2026-11-04 11:49:08">2天23时</span></b></div><div style="margin-top: 4px;"><a class="chs_tag chs_tag-jz torrents-tag" href="torrents.php?tag_exclusive=yes">禁转</a>&nbsp;<span>示例短剧 | 全77集 | 2026年</span></div></div></td><td class="embedded" style="padding-left: 0px;" width="20" valign="middle"><a href="download.php?id=905262"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td><td class="embedded" width="44px"><div style="display: flex; align-items: center;"><img src="pic/imdb.png" title="IMDb评分" /><span data-imdbid="" style="padding-left: 5px"><font color="gray">N/A</font></span></div></td></tr></tbody></table></td>
<td class="rowfollow"><a href="comment.php?action=add&amp;pid=905262&amp;type=torrent">0</a></td><td class="rowfollow nowrap"><span title="2026-11-01 11:49:08">7分</span></td><td class="rowfollow">225.81<br />MB</td><td class="rowfollow" align="center"><b><a href="details.php?id=905262&amp;dllist=1#seeders"><font color="#ee0000">1</font></a></b></td>
<td class="rowfollow"><b><a href="details.php?id=905262&amp;dllist=1#leechers">10</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style="padding: 0px"><a href="?cat=409"><img class="c_game" src="pic/cattrans.gif" alt="游戏 (Game)" title="游戏 (Game)" /></a></td>
<td class="rowfollow" width="100%" align="left" style="padding: 0px"><table class="torrentname" width="100%"><tbody><tr><td class="embedded"><div style="padding: 2px;"><div>&nbsp;<a title="Demo Game 1.9.638-DEMOGame" href="detailsgame.php?id=904947"><b>&nbsp;Demo Game 1.9.638-DEMOGame</b></a></div><div style="margin-top: 4px;">&nbsp;<span>示例游戏</span></div></div></td></tr></tbody></table></td>
<td class="rowfollow">0</td><td class="rowfollow nowrap"><span title="2026-11-01 08:31:41">3时</span></td><td class="rowfollow">1.20<br />GB</td><td class="rowfollow" align="center">8</td><td class="rowfollow">0</td><td class="rowfollow">2</td><td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
<td class="rowfollow nowrap"><a href="?cat=401"><img class="c_movie" src="pic/cattrans.gif" alt="电影" title="电影" /></a></td>
<td class="rowfollow"><table class="torrentname"><tr>
    <td class="embedded"><img class="lozad" src="/pic/spinner.svg" alt="preview" /></td>
    <td class="embedded"><div><a title="Sample Movie 2025 1080p" href="details.php?id=844700"><b>&nbsp;Sample Movie 2025 1080p</b></a>
    <br /><span class="torrents-subtitle">样品电影</span></div></td>
  </tr></table></td>
<td class="rowfollow"><a href="comment.php?action=add&amp;pid=844700&amp;type=torrent">0</a></td>
<td class="rowfollow nowrap"><span title="2026-06-25 22:15:05">1天</span></td>
<td class="rowfollow">8.20<br />GB</td>
<td class="rowfollow" align="center"><b><a href="details.php?id=844700&amp;dllist=1#seeders">10</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=844700&amp;dllist=1#leechers">0</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=844700"><b>3</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table>
</body></html>`

// 详情页：H1 里是促销（font 的 class 是 twoupfree）与「剩余时间」，大小在「基本信息」一格。
const pterclubDetailFixture = `<html><body>
<h1 align="center" id="top">Demo Short 2026 S01 720p WEB-DL H.265 AAC-DEMO&nbsp;&nbsp;&nbsp;<b>&nbsp;[<font size="3" color="red">禁转</font>]</b> <b>[<font class="twoupfree" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;twoupfree&quot;&gt;2X免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-11-04 11:49:08&quot;&gt;2天23时&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500);">2X免费</font>]</b>&nbsp;剩余时间：<b><span title="2026-11-04 11:49:08">2天23时</span></b></h1>
<input type="hidden" name="torrent_name" value="Demo Short 2026 S01 720p WEB-DL H.265 AAC-DEMO" />
<input type="hidden" name="detail_torrent_id" value="905262" />
<table>
  <tr><td class="rowhead nowrap">下载</td><td class="rowfollow"><a class="index" href="download.php?id=905262">[PTer].Demo.Short.2026.S01.720p.WEB-DL.H265.AAC-DEMO.torrent</a></td></tr>
  <tr><td class="rowhead nowrap">副标题</td><td class="rowfollow">示例短剧 | 全77集 | 2026年</td></tr>
  <tr><td class="rowhead nowrap">基本信息</td><td class="rowfollow"><b><b>大小：</b></b>225.81 MB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;电视剧 (TV Series)&nbsp;&nbsp;&nbsp;<b>质量:&nbsp;</b>WEB-DL</td></tr>
</table>
</body></html>`

const pterclubIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td>
<span class="medium">欢迎回来, <span class="nowrap"><a href="userdetails.php?id=19860" class='EliteUser_Name'><b>testuser</b></a></span>
[<a href="#" data-url="logout.php">退出</a>] [<a href="torrents.php?inclbookmarked=1&amp;allsec=1&amp;incldead=0">收藏</a>]
<a title="点击查看每小时收益预测" href="mybonus.php#bonus-sum"> <span class="color_bonus">猫粮 </span> </a> [<a href="mybonus.php">使用</a> | <a href="sitefreepool.php">站免池</a>]: 1,452,469.7
<font class="color_invite">邀请 </font> [<a href="invite.php?id=19860">发送</a>]: 0/0<br />
<font class="color_ratio">分享率：</font> 15.802
<font class='color_uploaded'>上传量：</font> 8.785 TB<font class='color_downloaded'> 下载量：</font> 569.27 GB
<span class="color_bonus">做种积分：</span> 1,333,648.2
<font class='color_active'>当前活动：</font> <a href="getusertorrentlist.php?userid=19860&amp;type=seeding"> <img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" /> 4</a> <a href="getusertorrentlist.php?userid=19860&amp;type=leeching"> <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" /> 0</a>
</span>
</td></tr></table>
</body></html>`

const pterclubUserdetailsFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td>
<span class="medium">欢迎回来, <span class="nowrap"><a href="userdetails.php?id=19860" class='EliteUser_Name'><b>testuser</b></a></span>
<a href="mybonus.php#bonus-sum"> <span class="color_bonus">猫粮 </span> </a> [<a href="mybonus.php">使用</a> | <a href="sitefreepool.php">站免池</a>]: 1,452,469.7<br />
<font class="color_ratio">分享率：</font> 15.802
<font class='color_uploaded'>上传量：</font> 8.785 TB<font class='color_downloaded'> 下载量：</font> 569.27 GB
<font class='color_active'>当前活动：</font> <a href="x"> <img class="arrowup" src="pic/trans.gif" /> 4</a> <a href="y"> <img class="arrowdown" src="pic/trans.gif" /> 0</a></span>
</td></tr></table>
<table>
  <tr><td class="rowhead nowrap">加入日期</td><td class="rowfollow">2023-03-08 13:07:05 (<span title="2023-03-08 13:07:05">3年4月前</span>)</td></tr>
  <tr><td class="rowhead nowrap">最近动向</td><td class="rowfollow">2026-06-27 10:36:57 (<span title="2026-06-27 10:36:57">&lt; 1分前</span>)</td></tr>
  <tr><td class="rowhead nowrap">做种积分</td><td class="rowfollow">1333648.2</td></tr>
  <tr><td class="rowhead nowrap"><a href='https://wiki.pterclub.net/wiki/x'>等级</a></td><td class="rowfollow"><img alt="布偶猫 ELITE USER" title="布偶猫 ELITE USER" src="pic/user_class/elite.png" /> </td></tr>
  <tr><td class="rowhead nowrap">猫粮</td><td class="rowfollow">1452469.7<a href="mybonus.php?uid=19860#bonus-sum" target="_blank" style="margin-left: 10px;">[查看每小时猫粮]</a></td></tr>
  <tr><td class="rowhead nowrap">H&amp;R</td><td class="rowfollow">0</td></tr>
  <tr><td class="rowhead nowrap">做种大小</td><td class="rowfollow">4.208 TB</td></tr>
</table>
</body></html>`

// 魔力值页：说明里有「每小时」字样，时魔是表格下面那一句（合计列的 97.452）。
const pterclubMyBonusFixture = `<html><body><table><tr><td id="outer">
<h1>做种每小时将得到如下的猫粮</h1><ul><li>基本奖励: 1克猫粮 * 你的做种数 (做种数最多计20个)</li><li><b>B0</b>为参数，代表用户1小时获得猫粮的上限。<b>B0</b> = 100</li></ul>
<table id="bonus-sum"><tr><td class="bonus-title">来源</td><td class="bonus-title">计算公式</td><td class="bonus-title">获得猫粮</td><td class="bonus-title">合计</td></tr>
<tr><td>基本奖励</td><td>1 * <span class="user-var">20</span></td><td>20</td><td rowspan="2"><span class="user-var">97.452</span></td></tr>
<tr><td>B值</td><td>-</td><td>77.452</td></tr></table>
<div>你当前每小时能获取97.452克猫粮 ( A = 2210.389， B = 77.452 )</div>
<div>如果是捐赠黄星每小时将获取双倍194.904克猫粮</div>
</td></tr></table></body></html>`

func getPTerClubDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("pterclub")
	require.True(t, ok, "pterclub definition not found")
	return def
}

func testPTerClubSearch(t *testing.T) {
	def := getPTerClubDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(pterclubSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 3, "游戏种子（detailsgame.php）不收")

	free := items[0]
	assert.Equal(t, "905232", free.ID)
	assert.Equal(t, "Demo Series 2026 S01 E14-E15 2160p WEB-DL H265 AAC-DEMOWEB", free.Title)
	assert.Equal(t, "示例剧 第14-15集 | 导演: 甲 主演: 乙 丙  [国语/中字]", free.Subtitle, "副标题不带标签，也不带评分格的分数")
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	assert.Equal(t, "2026-11-01 23:26:38", free.DiscountEndTime.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05"))
	assert.Equal(t, 29, free.Seeders)
	assert.Equal(t, 3, free.Leechers)
	assert.Equal(t, 26, free.Snatched)
	assert.Equal(t, "电视剧 (TV Series)", free.Category)

	twoUpFree := items[1]
	assert.Equal(t, "905262", twoUpFree.ID)
	assert.Equal(t, "示例短剧 | 全77集 | 2026年", twoUpFree.Subtitle)
	assert.Equal(t, v2.Discount2xFree, twoUpFree.DiscountLevel)
	assert.Equal(t, 1, twoUpFree.Seeders, "做种数包在 font 里")
	assert.Equal(t, 0, twoUpFree.Snatched)

	legacy := items[2]
	assert.Equal(t, "844700", legacy.ID)
	assert.Equal(t, "样品电影", legacy.Subtitle)
	assert.Equal(t, v2.DiscountNone, legacy.DiscountLevel)
}

func testPTerClubDetail(t *testing.T) {
	def := getPTerClubDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "pterclub_detail", pterclubDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "905262", info.TorrentID)
	assert.Equal(t, "Demo Short 2026 S01 720p WEB-DL H.265 AAC-DEMO", info.Title)
	assert.Equal(t, v2.Discount2xFree, info.DiscountLevel)
	assert.Equal(t, "2026-11-04 11:49:08", info.DiscountEnd.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05"))
	assert.InDelta(t, 225.81, info.SizeMB, 0.01)
	assert.False(t, info.HasHR, "猫站不实行 H&R")
}

func testPTerClubUserInfo(t *testing.T) {
	def := getPTerClubDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "pterclub_index", pterclubIndexFixture)
		fields := map[string]string{
			"id":           "19860",
			"name":         "testuser",
			"bonus":        "1.4524697e+06",
			"seedingBonus": "1.3336482e+06",
			"ratio":        "15.802",
			"uploaded":     "9659209650012",
			"downloaded":   "611249008148",
			"seeding":      "4",
			"leeching":     "0",
		}
		for field, expected := range fields {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok, "selector %q missing", field)
			assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel), "field %q", field)
		}
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "pterclub_userdetails", pterclubUserdetailsFixture)
		expected := map[string]string{
			"levelName":    "布偶猫 ELITE USER",
			"joinTime":     "1678252025",
			"lastAccessAt": "1782527817",
			"seedingSize":  "4626744929681",
		}
		for field, want := range expected {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok, "selector %q missing", field)
			assert.Equal(t, want, driver.ExtractFieldValuePublic(doc, sel), "field %q", field)
		}
		laSel := def.UserInfo.Selectors["lastAccessAt"]
		assert.NotEmpty(t, driver.ExtractFieldValuePublic(doc, laSel))
	})

	t.Run("MyBonusPage", func(t *testing.T) {
		doc := FixtureDoc(t, "pterclub_mybonus", pterclubMyBonusFixture)
		sel := def.UserInfo.Selectors["bonusPerHour"]
		assert.Equal(t, "97.452", driver.ExtractFieldValuePublic(doc, sel), "不能取到黄星的双倍时魔或公式里的参数")
	})
}

// 走 GetUserInfo 全流程：做种体积读个人详情页的「做种大小」，不再请求站点上没有的 getusertorrentlistajax.php；
// 时魔读魔力值页。
func TestPTerClub_GetUserInfo(t *testing.T) {
	def := getPTerClubDef(t)

	var mu sync.Mutex
	hits := map[string]int{}
	page := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			hits[r.URL.Path]++
			mu.Unlock()
			_, _ = w.Write([]byte(body))
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/index.php", page(pterclubIndexFixture))
	mux.HandleFunc("/userdetails.php", page(pterclubUserdetailsFixture))
	mux.HandleFunc("/mybonus.php", page(pterclubMyBonusFixture))
	mux.HandleFunc("/getusertorrentlistajax.php", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		http.NotFound(w, r)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	info, err := driver.GetUserInfo(context.Background())
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Zero(t, hits["/getusertorrentlistajax.php"], "做种体积已经从个人详情页读到，不取做种列表")
	assert.Equal(t, 1, hits["/mybonus.php"])

	assert.Equal(t, "19860", info.UserID)
	assert.Equal(t, "布偶猫 ELITE USER", info.LevelName)
	assert.Equal(t, int64(4626744929681), info.SeederSize)
	assert.InDelta(t, 97.452, info.BonusPerHour, 0.001)
	assert.Equal(t, 4, info.Seeding)
	assert.InDelta(t, 15.802, info.Ratio, 0.001)
}

func TestPTerClub_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      pterclubSearchFixture,
		"detail":      pterclubDetailFixture,
		"index":       pterclubIndexFixture,
		"userdetails": pterclubUserdetailsFixture,
		"mybonus":     pterclubMyBonusFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
