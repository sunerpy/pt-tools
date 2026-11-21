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
		SiteID:   "tangpt",
		Search:   testTangPTSearch,
		Detail:   testTangPTDetail,
		UserInfo: testTangPTUserInfo,
	})
}

// --- Fixtures (anonymized, modeled on real www.tangpt.top HTML) ---

const tangptSearchFixture = `<html><body>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%">
<tr>
	<td class="colhead">类型</td>
	<td class="colhead">标题</td>
	<td class="colhead"><img alt="comments" title="评论数" /></td>
	<td class="colhead"><img alt="time" title="存活时间" /></td>
	<td class="colhead"><img alt="size" title="大小" /></td>
	<td class="colhead"><img alt="seeders" title="种子数" /></td>
	<td class="colhead"><img alt="leechers" title="下载数" /></td>
	<td class="colhead"><img alt="snatched" title="完成数" /></td>
	<td class="colhead">发布者</td>
</tr>
<tr>
	<td class="rowfollow nowrap" valign="middle"><a href="?cat=404"><img class="c_doc" alt="纪录片" title="纪录片" /></a></td>
	<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr><td class="embedded"><img class="nexus-lazy-load" /></td><td class="embedded"><a title="Sample.Doc.2026.1080p.WEB-DL.H264.AAC-TPWEB" href="details.php?id=20986&amp;hit=1"><b>Sample.Doc.2026.1080p.WEB-DL.H264.AAC-TPWEB</b></a> <b>[<font class='hot'>热门</font>]</b> <img class="pro_free" src="pic/trans.gif" alt="Free" title="免费" /><br /><span title="示例纪录片副标题">示例纪录片副标题</span></td></tr></table></td>
	<td class="rowfollow"><b><a href="details.php?id=20986&amp;hit=1&amp;cmtpage=1#startcomments">1</a></b></td>
	<td class="rowfollow nowrap"><span title="2026-01-17 22:01:11">5月<br />26天</span></td>
	<td class="rowfollow">506.78<br />MB</td>
	<td class="rowfollow" align="center"><b><a href="details.php?id=20986&amp;hit=1&amp;dllist=1#seeders">119</a></b></td>
	<td class="rowfollow">0</td>
	<td class="rowfollow"><a href="viewsnatches.php?id=20986"><b>313</b></a></td>
	<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
	<td class="rowfollow nowrap" valign="middle"><a href="?cat=409"><img class="c_hqaudio" alt="音乐" title="音乐" /></a></td>
	<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr><td class="embedded"><img class="nexus-lazy-load" /></td><td class="embedded"><a title="Sample.Audio.Pack.MP3-TPAudio" href="details.php?id=34917&amp;hit=1"><b>Sample.Audio.Pack.MP3-TPAudio</b></a> <img class="pro_50pctdown2up" src="pic/trans.gif" alt="2X 50%" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;twouphalfdown&quot;&gt;2X 50%&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-08-22 13:27:58&quot;&gt;1月9天&lt;/span&gt;&lt;/b&gt;', 'trail', false);" /> 剩余时间：<span title="2026-08-22 13:27:58">1月9天</span><br /><span title="示例音乐副标题">示例音乐副标题</span></td></tr></table></td>
	<td class="rowfollow"><b>0</b></td>
	<td class="rowfollow nowrap"><span title="2026-05-01 10:00:00">2月<br />1天</span></td>
	<td class="rowfollow">1.20<br />GB</td>
	<td class="rowfollow" align="center"><b>50</b></td>
	<td class="rowfollow">3</td>
	<td class="rowfollow"><b>8</b></td>
	<td class="rowfollow"><i>匿名</i></td>
</tr>
</table>
</body></html>`

const tangptIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td>
	<span class="medium"> 欢迎回来, <span class="nowrap"><a href="https://www.tangpt.top/userdetails.php?id=13199" class='User_Name'><b>sampleuser</b></a></span> [<a href="logout.php">退出</a>]
	<font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 10,100.0
	<font class="color_ratio">分享率:</font> 无限
	<font class='color_uploaded'>上传量:</font> 100.00 GB
	<font class='color_active'>当前活动:</font> <img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />7 <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />2
</td></tr></table>
</body></html>`

const tangptUserdetailsFixture = `<html><body>
<table>
	<tr>
		<td class="rowhead">用户ID/UID</td>
		<td class="rowfollow">13199</td>
	</tr>
	<tr>
		<td class="rowhead">加入日期</td>
		<td class="rowfollow">2026-07-13 16:05:24 (<span title="2026-07-13 16:05:24">16分钟前</span>, 0周)</td>
	</tr>
	<tr>
		<td class="rowhead">最近动向</td>
		<td class="rowfollow">2026-07-13 16:21:37 (<span title="2026-07-13 16:21:37">&lt; 1分钟前</span>)</td>
	</tr>
	<tr>
		<td class="rowhead">传输</td>
		<td width="99%" class="rowfollow" valign="top" align="left"><table border="0"><tr><td class="embedded"><strong>上传量</strong>: 100.00 GB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>: 512.00 MB</td></tr></table></td>
	</tr>
	<tr>
		<td class="rowhead">等级</td>
		<td class="rowfollow"><img alt="User" title="User" src="pic/user.gif" /></td>
	</tr>
	<tr>
		<td class="rowhead">魔力值</td>
		<td class="rowfollow">10,100.0</td>
	</tr>
</table>
</body></html>`

const tangptDetailFixture = `<html><body>
<h1>
	<input name="torrent_name" type="hidden" value="Sample.Doc.2026.1080p.WEB-DL.H264.AAC-TPWEB" />
	<input name="detail_torrent_id" type="hidden" value="20986" />
	<b>[<font class='free'>免费</font>]</b>
</h1>
<table>
	<tr>
		<td class="rowhead">基本信息</td>
		<td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>506.78 MB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;纪录片&nbsp;&nbsp;&nbsp;<b>媒介: </b>WEB-DL</td>
	</tr>
</table>
</body></html>`

const tangptDetailWithHRFixture = `<html><body>
<h1>
	<input name="torrent_name" type="hidden" value="Sample.HR.2026.WEB-DL" />
	<input name="detail_torrent_id" type="hidden" value="20987" />
</h1>
<table>
	<tr>
		<td class="rowhead">基本信息</td>
		<td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>8.50 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;电影</td>
	</tr>
</table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

// mybonus.php 的结构照 2026-11-21 站点页面（上游 NexusPHP 的魔力值页），数字是示例值：
// 上方「你当前每小时能获取」只是基本奖励的基础魔力，时魔要取后面「每小时获得的合计魔力值」表格的合计
// （601.000 + 626.250 + 7.317 + 0.000 = 1,234.567）。表格在真实页面上是一整行。
const tangptMyBonusFixture = `<html><body><table id="outer"><tr><td id="outer">
<ul><li><b>B0</b>为参数，代表用户1小时获得魔力值的上限。<b>B0</b> = 500</li><li><b>L</b>为参数。<b>L</b> = 300</li></ul>简言之，为做种人数少、文件体积大的种子做种能获得更多魔力值。</li><li>捐赠者每次将得到2倍的魔力值</li></ul><div align="center">你当前每小时能获取300.500个魔力值 (A = 456.8)</div><table align="center" border="0" width="400"><tr><td class="loadbarbg" style="border: none; padding: 0px;"><img class="loadbargreen" src="pic/trans.gif" style="width: 240.4px;" alt="60.1%" /></td></tr></table><h1>勋章加成每小时将额外得到如下的魔力值</h1><ul><li>勋章奖励计算公式同上，不考虑低保</li><li>最终奖励为计算所得勋章奖励乘以勋章加成系数，当前勋章加成系数为: 2.500</li></ul><h1>官种加成每小时将额外得到如下的魔力值</h1><ul><li>最终奖励为计算所得官种奖励乘以官种系数，当前官种系数为: 10</li></ul><h1>后宫加成每小时将额外得到如下的魔力值</h1><ul><li>所得奖励为全部后宫的时魔（不考虑加成）之和，乘以后宫加成系数，当前值为：0.1</li></ul><h1>每小时获得的合计魔力值</h1><div style="display: flex;justify-content: center;margin-top: 20px;"><table cellpadding="5" style="width: 50%"><tr><td class="colhead">奖励类型</td><td class="colhead">数量</td><td class="colhead">体积</td><td class="colhead">A 值</td><td class="colhead">基础魔力</td><td class="colhead">系数</td><td class="colhead">获得魔力</td><td class="colhead">合计</td></tr><tr><td>基本奖励</td><td>12</td><td>1.234 TB</td><td>456.789</td><td>300.500</td><td>2</td><td>601.000</td><td rowspan="4">1,234.567</td></tr><tr><td>勋章加成</td><td>12</td><td>1.234 TB</td><td>456.789</td><td>250.500</td><td>2.500</td><td>626.250</td></tr><tr><td>官种加成</td><td>1</td><td>4.00 GB</td><td>0.712</td><td>0.732</td><td>10.000</td><td>7.317</td></tr><tr><td>后宫加成</td><td>--</td><td>--</td><td>--</td><td>0.000</td><td>0.100</td><td>0.000</td></tr></table></div><h1>其他获取方式</h1><ul><li>发布新种子 = 15000个魔力值</li></ul>
</td></tr></table></body></html>`

// --- Helpers ---

func getTangPTDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("tangpt")
	require.True(t, ok, "tangpt definition not found")
	return def
}

// --- Suite: Search ---

func testTangPTSearch(t *testing.T) {
	def := getTangPTDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(tangptSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{
		BaseURL:   server.URL,
		Cookie:    "test_cookie=1",
		Selectors: def.Selectors,
	})
	driver.SetSiteDefinition(def)

	req := v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"}
	res, err := driver.Execute(context.Background(), req)
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2, "should parse 2 torrent rows")

	free := items[0]
	assert.Equal(t, "20986", free.ID)
	assert.Equal(t, "Sample.Doc.2026.1080p.WEB-DL.H264.AAC-TPWEB", free.Title)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	assert.Equal(t, 119, free.Seeders)
	assert.Equal(t, 0, free.Leechers)
	assert.Equal(t, 313, free.Snatched)
	assert.True(t, free.SizeBytes > 0, "size should be parsed")

	second := items[1]
	assert.Equal(t, "34917", second.ID)
	// pro_50pctdown2up 是 2X 50%（以前驱动的通用对照先命中「50pct」，读成了 50%）
	assert.Equal(t, v2.Discount2x50, second.DiscountLevel)
	assert.False(t, second.DiscountEndTime.IsZero(), "discount end time should be parsed from onmouseover")
	assert.Equal(t, 2026, second.DiscountEndTime.Year())
	assert.Equal(t, 8, int(second.DiscountEndTime.Month()))
	assert.Equal(t, 50, second.Seeders)
	assert.Equal(t, 3, second.Leechers)
	assert.Equal(t, 8, second.Snatched)
}

// --- Suite: Detail ---

func testTangPTDetail(t *testing.T) {
	def := getTangPTDef(t)

	t.Run("Free", func(t *testing.T) {
		doc := FixtureDoc(t, "tangpt_detail", tangptDetailFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "20986", info.TorrentID)
		assert.Equal(t, "Sample.Doc.2026.1080p.WEB-DL.H264.AAC-TPWEB", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		assert.InDelta(t, 506.78, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})

	t.Run("WithHR", func(t *testing.T) {
		doc := FixtureDoc(t, "tangpt_detail_hr", tangptDetailWithHRFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "20987", info.TorrentID)
		assert.True(t, info.HasHR, "should detect HR from hit_run.gif")
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel)
		assert.InDelta(t, 8.5*1024, info.SizeMB, 0.1)
	})
}

// --- Suite: UserInfo ---

func testTangPTUserInfo(t *testing.T) {
	def := getTangPTDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "tangpt_index", tangptIndexFixture)
		fields := map[string]string{
			"id":       "13199",
			"name":     "sampleuser",
			"seeding":  "7",
			"leeching": "2",
			"bonus":    "10100",
		}
		for field, expected := range fields {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q not found", field)
				got := driver.ExtractFieldValuePublic(doc, sel)
				assert.Equal(t, expected, got)
			})
		}
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "tangpt_userdetails", tangptUserdetailsFixture)
		exact := map[string]string{
			"uploaded":   "107374182400",
			"downloaded": "536870912",
			"levelName":  "User",
		}
		for field, expected := range exact {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q not found", field)
				got := driver.ExtractFieldValuePublic(doc, sel)
				assert.Equal(t, expected, got)
			})
		}
	})

	t.Run("LastAccess", func(t *testing.T) {
		doc := FixtureDoc(t, "tangpt_userdetails_la", tangptUserdetailsFixture)
		sel, ok := def.UserInfo.Selectors["lastAccessAt"]
		require.True(t, ok, "lastAccessAt selector not found")
		got := driver.ExtractFieldValuePublic(doc, sel)
		// 2026-07-13 16:21:37 +0800 => unix 1783930897; must be > 0 so 保号 probe gets LastAccess
		assert.Equal(t, "1783930897", got)
	})

	t.Run("BonusPerHour", func(t *testing.T) {
		doc := FixtureDoc(t, "tangpt_mybonus", tangptMyBonusFixture)
		sel, ok := def.UserInfo.Selectors["bonusPerHour"]
		require.True(t, ok, "bonusPerHour selector not found")
		// 合计 1,234.567，不是上方「你当前每小时能获取」的 300.500
		assert.Equal(t, "1234.567", driver.ExtractFieldValuePublic(doc, sel))
	})
}

// 走 GetUserInfo 全流程：时魔读魔力值页的合计，三页各请求一次，用户 ID 带到个人详情页。
func TestTangPT_GetUserInfo(t *testing.T) {
	def := getTangPTDef(t)

	var mu sync.Mutex
	hits := map[string]int{}
	var detailID string
	page := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			hits[r.URL.Path]++
			if r.URL.Path == "/userdetails.php" {
				detailID = r.URL.Query().Get("id")
			}
			mu.Unlock()
			_, _ = w.Write([]byte(body))
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/index.php", page(tangptIndexFixture))
	mux.HandleFunc("/userdetails.php", page(tangptUserdetailsFixture))
	mux.HandleFunc("/mybonus.php", page(tangptMyBonusFixture))
	server := httptest.NewServer(mux)
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	info, err := driver.GetUserInfo(context.Background())
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, hits["/index.php"])
	assert.Equal(t, 1, hits["/userdetails.php"])
	assert.Equal(t, 1, hits["/mybonus.php"])
	assert.Equal(t, "13199", detailID)

	assert.Equal(t, "13199", info.UserID)
	assert.Equal(t, "sampleuser", info.Username)
	assert.InDelta(t, 1234.567, info.BonusPerHour, 0.0001)
	assert.InDelta(t, 10100.0, info.Bonus, 0.0001)
	assert.Equal(t, int64(107374182400), info.Uploaded)
	assert.Equal(t, 7, info.Seeding)
}

// --- Standalone: Secret leak guard ---

func TestTangPT_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      tangptSearchFixture,
		"index":       tangptIndexFixture,
		"userdetails": tangptUserdetailsFixture,
		"detail":      tangptDetailFixture,
		"detail_hr":   tangptDetailWithHRFixture,
		"mybonus":     tangptMyBonusFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
