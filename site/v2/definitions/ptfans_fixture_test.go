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
<img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />0
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
	require.Len(t, items, 2)

	assert.Equal(t, "68", items[0].ID)
	assert.Equal(t, "从零开始玩PT入门到精通 V1.0 PDF", items[0].Title)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 42, items[0].Seeders)
	assert.Equal(t, 3, items[0].Leechers)
	assert.Equal(t, 88, items[0].Snatched)

	assert.Equal(t, "7989", items[1].ID)
	assert.Equal(t, v2.DiscountFree, items[1].DiscountLevel)
	// Discount end time embedded in onmouseover tooltip
	assert.False(t, items[1].DiscountEndTime.IsZero(), "ptfans onmouseover end time should parse")
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
