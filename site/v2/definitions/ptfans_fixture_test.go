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
		SiteID:   "ptfans",
		Search:   testPTFansSearch,
		Detail:   testPTFansDetail,
		UserInfo: testPTFansUserInfo,
	})
}

const ptfansSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=412"><span class="c_shukan" title="Book">.::</span></a></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded"><img src="pic/misc/spinner.svg" class="nexus-lazy-load" /></td>
    <td class="embedded">
      <img class="sticky" src="pic/trans.gif" alt="Sticky" title="一级置顶" />&nbsp;
      <a title="从零开始玩PT入门到精通 V1.0 PDF" href="details.php?id=68&amp;hit=1"><b>从零开始玩PT入门到精通 V1.0 PDF</b></a>
      <b>[<font class='hot'>热门</font>]</b>
      <img class="pro_free" src="pic/trans.gif" alt="Free" title="免费" />
      <img class="hitandrun" src="pic/trans.gif" alt="H&amp;R" title="H&amp;R" />
      <br />本站资源开启7天HR，请在35天内完成做种7天
    </td></tr></table></td>
  <td class="rowfollow"><a href="comment.php?action=add&amp;pid=68&amp;type=torrent">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-06-16 08:39:42">1天<br />14时</span></td>
  <td class="rowfollow">7.96<br />MB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=68&amp;dllist=1#seeders">42</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=68&amp;dllist=1#leechers">3</a></b></td>
  <td class="rowfollow"><a href="viewsnatches.php?id=68"><b>88</b></a></td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=401"><span class="c_movie" title="Movie">.::</span></a></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded"><img src="pic/misc/spinner.svg" class="nexus-lazy-load" /></td>
    <td class="embedded">
      <a title="命运注定 2026 1080p" href="details.php?id=7989&amp;hit=1"><b>命运注定 2026 1080p</b></a>
      <img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-06-23 08:39:42&quot;&gt;5天9时&lt;/span&gt;&lt;/b&gt;', 'trail', false);" />
      <br />命中注定 野性
    </td></tr></table></td>
  <td class="rowfollow"><a href="comment.php?action=add&amp;pid=7989&amp;type=torrent">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-06-16 08:39:42">1天<br />14时</span></td>
  <td class="rowfollow">7.96<br />GB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=7989&amp;dllist=1#seeders">12</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=7989&amp;dllist=1#leechers">0</a></b></td>
  <td class="rowfollow"><a href="viewsnatches.php?id=7989"><b>20</b></a></td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=404"><span class="c_dianshiju" alt="TV Series(电视剧)" title="TV Series(电视剧)">.::</span></a></td>
  <td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="https://img.example.invalid/poster-b.jpg" class="nexus-lazy-load" style="max-height: 46px;max-width: 46px" /></td><td class="embedded" style='padding-left: 5px'><a title="Sample Series S01 2026 2160p WEB-DL H.265 AAC 2.0-DEMO"  href="details.php?id=20001&amp;hit=1"><b>Sample Series S01 2026 2160p WEB-DL H.265 AAC 2.0-DEMO</b></a> <img class="pro_free" src="pic/trans.gif" alt="Free"  onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-11-18 21:21:48&quot;&gt;6天10时&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000,'fade','both','styleClass','niceTitle', 'fadeMax',87, 'maxWidth', 300);" /> <font color='#0000FF'>剩余时间：<span title="2026-11-18 21:21:48">6天10时</span></font><img class="hitandrun" src="pic/trans.gif" alt="H&R" title="H&R" /><br /><span style="background-color:#6a3906;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">国语</span><span style="background-color:#006400;color:#ffffff;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">中字</span>示例剧集 第一季 全30集 | 类型：剧情 &amp; 古装 | 主演：甲乙 丙丁</td><td class="embedded" style="text-align: right; width: 40px;padding: 4px"><div style="display: flex;flex-direction: column"><div style="display: flex;align-content: center;justify-content: space-between;padding: 2px 0"><img src="pic/douban2.png" alt="douban" title="douban" style="max-width: 16px;max-height: 16px"/><span>7.0</span></div></div></td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=20001"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a><br /><a id="bookmark2"  href="javascript: bookmark(20001,2);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=20001&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-11-11 21:21:48">13时<br />45分钟</span></td><td class="rowfollow">164.71<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=20001&amp;hit=1&amp;dllist=1#seeders">2</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=20001&amp;hit=1&amp;dllist=1#leechers">5</a></b></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table>
</body></html>`

const ptfansDetailFixture = `<html><body>
<h1>
  <input type="hidden" name="torrent_name" value="从零开始玩PT入门到精通 V1.0 PDF" />
  <input type="hidden" name="detail_torrent_id" value="68" />
  从零开始玩PT入门到精通 V1.0 PDF&nbsp;&nbsp;&nbsp; <b>[<font class='free' >免费</font>]</b>
  <img class="hitandrun" src="pic/trans.gif" alt="H&amp;R" title="H&amp;R" />
</h1>
<table>
  <tr><td class="rowhead nowrap">基本信息</td><td class="rowfollow"><b><b>大小：</b></b>2.26 MB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;Book</td></tr>
  <tr><td class="rowhead nowrap">副标题</td><td class="rowfollow">本站资源开启7天HR，请在35天内完成做种7天</td></tr>
</table>
</body></html>`

const ptfansIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td>
<span class="medium">
欢迎回来, <span class="nowrap"><a href="https://ptfans.cc/userdetails.php?id=17879" class='PowerUser_Name'><b>testuser</b></a></span>
[<a href="logout.php">退出</a>]
<font class = 'color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 166,546.4
<font class = 'color_invite'>邀请 </font>[<a href="invite.php?id=17879">发送</a>]: 12(0)
<br />
<font class="color_ratio">分享率:</font> 3.983
<font class='color_uploaded'>上传量:</font> 465.39 GB
<font class='color_downloaded'> 下载量:</font> 116.85 GB
<font class='color_active'>当前活动:</font>
<img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />3
<img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />0&nbsp;&nbsp;
<font class='color_connectable'>可连接:</font><b><font color="green">是</font></b>
<font class='color_bonus'>H&R: </font> [<a href="myhr.php">1/<font color="red">2</font>/3</a>]
<font class='color_bonus'>认领: </font> [<a href="claim.php?uid=17879">0/1000</a>]
</span>
</td></tr></table>
</body></html>`

const ptfansUserdetailsFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td>
<span class="medium">
欢迎回来, <span class="nowrap"><a href="https://ptfans.cc/userdetails.php?id=17879" class='PowerUser_Name'><b>testuser</b></a></span>
<font class = 'color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 166,546.4
<br />
<font class="color_ratio">分享率:</font> 3.983
<font class='color_uploaded'>上传量:</font> 465.39 GB
<font class='color_downloaded'> 下载量:</font> 116.85 GB
<font class='color_active'>当前活动:</font>
<img class="arrowup" src="pic/trans.gif" />3
<img class="arrowdown" src="pic/trans.gif" />0
</span>
</td></tr></table>
<table>
  <tr><td class="rowhead nowrap">加入日期</td><td class="rowfollow">2026-01-21 16:33:50 (<span title="2026-01-21 16:33:50">4月27天前</span>, 21周)</td></tr>
  <tr><td class="rowhead nowrap">最近动向</td><td class="rowfollow">2026-06-17 22:19:08 (<span title="2026-06-17 22:19:08">22分钟前</span>)</td></tr>
  <tr><td class="rowhead nowrap">等级</td><td class="rowfollow"><img alt="Power User" title="Power User" src="pic/poweruser.gif" /> </td></tr>
</table>
</body></html>`

func getPTFansDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("ptfans")
	require.True(t, ok, "ptfans definition not found")
	return def
}

func testPTFansSearch(t *testing.T) {
	def := getPTFansDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(ptfansSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 3)

	assert.Equal(t, "68", items[0].ID)
	assert.Equal(t, "从零开始玩PT入门到精通 V1.0 PDF", items[0].Title)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 42, items[0].Seeders)
	assert.Equal(t, 3, items[0].Leechers)
	assert.Equal(t, 88, items[0].Snatched)
	// 副标题是 <br /> 之后的裸文本
	assert.Equal(t, "本站资源开启7天HR，请在35天内完成做种7天", items[0].Subtitle)
	assert.True(t, items[0].HasHR)

	assert.Equal(t, "7989", items[1].ID)
	assert.Equal(t, v2.DiscountFree, items[1].DiscountLevel)
	// Discount end time embedded in onmouseover tooltip
	assert.False(t, items[1].DiscountEndTime.IsZero(), "ptfans onmouseover end time should parse")
	assert.Equal(t, "命中注定 野性", items[1].Subtitle)

	// 2026-11 的列表：副标题前面有「国语」「中字」这类 <span> 标签，副标题本身是标签后面的裸文本
	tagged := items[2]
	assert.Equal(t, "20001", tagged.ID)
	assert.Equal(t, "示例剧集 第一季 全30集 | 类型：剧情 & 古装 | 主演：甲乙 丙丁", tagged.Subtitle)
	assert.Equal(t, v2.DiscountFree, tagged.DiscountLevel)
	assert.Equal(t, "2026-11-18 21:21:48", tagged.DiscountEndTime.Format("2006-01-02 15:04:05"))
	assert.True(t, tagged.HasHR)
	assert.Equal(t, int64(176856015831), tagged.SizeBytes) // 164.71 GiB
	assert.Equal(t, 2, tagged.Seeders)
	assert.Equal(t, 5, tagged.Leechers)
	assert.Equal(t, "TV Series(电视剧)", tagged.Category)
}

func testPTFansDetail(t *testing.T) {
	def := getPTFansDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "ptfans_detail", ptfansDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "68", info.TorrentID)
	assert.Equal(t, "从零开始玩PT入门到精通 V1.0 PDF", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 2.26, info.SizeMB, 0.01)
	assert.True(t, info.HasHR)
}

func testPTFansUserInfo(t *testing.T) {
	def := getPTFansDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "ptfans_index", ptfansIndexFixture)
		fields := map[string]string{
			"id":         "17879",
			"name":       "testuser",
			"bonus":      "166546.4",
			"ratio":      "3.983",
			"uploaded":   "499708707471",
			"downloaded": "125466732134",
			"seeding":    "3",
			"leeching":   "0",
			// 「H&R: [1/2/3]」：考察中 / 未达标 / 封号上限，未达标是中间那个
			"hnrUnsatisfied": "2",
		}
		for field, expected := range fields {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok, "selector %q missing", field)
			assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel), "field %q", field)
		}
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "ptfans_userdetails", ptfansUserdetailsFixture)
		expected := map[string]string{
			"levelName":    "Power User",
			"joinTime":     "1768984430",
			"lastAccessAt": "1781705948",
		}
		for field, want := range expected {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok, "selector %q missing", field)
			assert.Equal(t, want, driver.ExtractFieldValuePublic(doc, sel), "field %q", field)
		}
		laSel := def.UserInfo.Selectors["lastAccessAt"]
		assert.NotEmpty(t, driver.ExtractFieldValuePublic(doc, laSel))
	})
}

// 走真实的 GetUserInfo：首页取顶栏（含 H&R 未达标数），再按顶栏里的用户 ID 取用户详情页。
func TestPTFans_GetUserInfo(t *testing.T) {
	def := getPTFansDef(t)
	var detailQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(ptfansIndexFixture))
		case "/userdetails.php":
			detailQuery = r.URL.RawQuery
			_, _ = w.Write([]byte(ptfansUserdetailsFixture))
		case "/getusertorrentlistajax.php":
			_, _ = w.Write([]byte(`<b>3</b> 条记录 | 总大小：30.00 GB<table><tr><td>大小</td></tr><tr><td>30.00<br />GB</td></tr></table>`))
		default:
			_, _ = w.Write([]byte(`<html><body></body></html>`))
		}
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1"})
	driver.SetSiteDefinition(def)
	info, err := driver.GetUserInfo(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "id=17879", detailQuery)
	assert.Equal(t, "17879", info.UserID)
	assert.Equal(t, "testuser", info.Username)
	assert.Equal(t, int64(499708707471), info.Uploaded)
	assert.Equal(t, int64(125466732134), info.Downloaded)
	assert.InDelta(t, 3.983, info.Ratio, 0.0001)
	assert.Equal(t, 3, info.Seeding)
	assert.Equal(t, 2, info.HnRUnsatisfied)
	assert.Equal(t, "Power User", info.LevelName)
	assert.Equal(t, int64(1781705948), info.LastAccess)
}

func TestPTFans_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      ptfansSearchFixture,
		"detail":      ptfansDetailFixture,
		"index":       ptfansIndexFixture,
		"userdetails": ptfansUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
