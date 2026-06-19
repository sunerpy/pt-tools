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
		SiteID:   "pandapt",
		Search:   testPandaPTSearch,
		Detail:   testPandaPTDetail,
		UserInfo: testPandaPTUserInfo,
	})
}

const pandaptSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=402"><img class="c_tvseries" src="pic/cattrans.gif" alt="电视剧" title="电视剧" /></a></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded"><img src="pic/misc/spinner.svg" class="nexus-lazy-load" /></td>
    <td class="embedded">
      <img class="sticky" src="pic/trans.gif" alt="Sticky" title="一级置顶" />&nbsp;
      <a title="Ashes to Crown 2026 S01E21 2160p WEB-DL" href="details.php?id=124750&amp;hit=1"><b>Ashes to Crown 2026 S01E21 2160p WEB-DL</b></a>
      <img class="pro_free" src="pic/trans.gif" alt="Free" title="免费" />
      <br /><span style="background-color:#0000ff" title="">官方</span><span style="background-color:#174143" title="">国语</span>翘楚 第1季 第21集
    </td></tr></table></td>
  <td class="rowfollow"><a href="comment.php?action=add&amp;pid=124750&amp;type=torrent">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-06-17 21:10:30">1时<br />28分钟</span></td>
  <td class="rowfollow">1.04<br />GB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=124750&amp;dllist=1#seeders">27</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=124750&amp;dllist=1#leechers">13</a></b></td>
  <td class="rowfollow"><a href="viewsnatches.php?id=124750"><b>29</b></a></td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=405"><img class="c_anime" src="pic/cattrans.gif" alt="动漫" title="动漫" /></a></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded"><img src="pic/misc/spinner.svg" class="nexus-lazy-load" /></td>
    <td class="embedded">
      <a title="Coiled Dragon 2026 S01E10 2160p WEB-DL" href="details.php?id=124743&amp;hit=1"><b>Coiled Dragon 2026 S01E10 2160p WEB-DL</b></a>
      <br />蟠龙 第1季 第10集
    </td></tr></table></td>
  <td class="rowfollow"><a href="comment.php?action=add&amp;pid=124743&amp;type=torrent">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-06-17 20:00:00">2时</span></td>
  <td class="rowfollow">2.50<br />GB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=124743&amp;dllist=1#seeders">10</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=124743&amp;dllist=1#leechers">2</a></b></td>
  <td class="rowfollow"><a href="viewsnatches.php?id=124743"><b>5</b></a></td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table>
</body></html>`

const pandaptDetailFixture = `<html><body>
<h1>
  <input type="hidden" name="torrent_name" value="Ashes to Crown 2026 S01E21 2160p WEB-DL" />
  <input type="hidden" name="detail_torrent_id" value="124750" />
  Ashes to Crown 2026 S01E21 2160p WEB-DL&nbsp;&nbsp;&nbsp; <b>[<font class='free' >免费</font>]</b>
</h1>
<table>
  <tr><td class="rowhead nowrap">基本信息</td><td class="rowfollow"><b><b>大小：</b></b>1.04 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;电视剧</td></tr>
  <tr><td class="rowhead nowrap">副标题</td><td class="rowfollow">翘楚 第1季 第21集</td></tr>
</table>
</body></html>`

const pandaptIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td>
<span class="medium">
欢迎回来, <span class="nowrap"><a href="https://pandapt.net/userdetails.php?id=105818" class='User_Name'><b>testuser</b></a></span>
[<a href="usercp.php">控制面板</a>]
<font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 651,449.7
<font class='color_invite'>邀请 </font>[<a href="invite.php?id=105818">发送</a>]: 0(0)
<br/>
<font class="color_ratio">分享率:</font> 2.733
<font class='color_uploaded'>上传量:</font> 293.24 GB
<font class='color_downloaded'> 下载量:</font> 107.28 GB
<font class='color_active'>当前活动:</font>
<img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif"/>11
<img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif"/>1
</span>
</td></tr></table>
</body></html>`

const pandaptUserdetailsFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td>
<span class="medium">
欢迎回来, <span class="nowrap"><a href="https://pandapt.net/userdetails.php?id=105818" class='User_Name'><b>testuser</b></a></span>
<font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 651,449.7
<br/>
<font class="color_ratio">分享率:</font> 2.733
<font class='color_uploaded'>上传量:</font> 293.24 GB
<font class='color_downloaded'> 下载量:</font> 107.28 GB
<font class='color_active'>当前活动:</font>
<img class="arrowup" src="pic/trans.gif"/>11
<img class="arrowdown" src="pic/trans.gif"/>1
</span>
</td></tr></table>
<table>
  <tr><td class="rowhead nowrap">加入日期</td><td class="rowfollow">2025-05-01 23:11:25 (<span title="2025-05-01 23:11:25">1年1月前</span>, 58周)</td></tr>
  <tr><td class="rowhead nowrap">最近动向</td><td class="rowfollow">2026-06-17 22:39:27 (<span title="2026-06-17 22:39:27">&lt; 1分钟前</span>)</td></tr>
  <tr><td class="rowhead nowrap">等级</td><td class="rowfollow"><img alt="User" title="User" src="pic/user.gif" /> </td></tr>
</table>
</body></html>`

func getPandaPTDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("pandapt")
	require.True(t, ok, "pandapt definition not found")
	return def
}

func testPandaPTSearch(t *testing.T) {
	def := getPandaPTDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(pandaptSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "124750", items[0].ID)
	assert.Equal(t, "Ashes to Crown 2026 S01E21 2160p WEB-DL", items[0].Title)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 27, items[0].Seeders)
	assert.Equal(t, 13, items[0].Leechers)
	assert.Equal(t, 29, items[0].Snatched)

	assert.Equal(t, "124743", items[1].ID)
	assert.Equal(t, v2.DiscountNone, items[1].DiscountLevel)
}

func testPandaPTDetail(t *testing.T) {
	def := getPandaPTDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "pandapt_detail", pandaptDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "124750", info.TorrentID)
	assert.Equal(t, "Ashes to Crown 2026 S01E21 2160p WEB-DL", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 1.04*1024, info.SizeMB, 0.1)
	assert.False(t, info.HasHR)
}

func testPandaPTUserInfo(t *testing.T) {
	def := getPandaPTDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "pandapt_index", pandaptIndexFixture)
		fields := map[string]string{
			"id":         "105818",
			"name":       "testuser",
			"bonus":      "651449.7",
			"ratio":      "2.733",
			"uploaded":   "314864052469",
			"downloaded": "115191022878",
			"seeding":    "11",
			"leeching":   "1",
		}
		for field, expected := range fields {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok, "selector %q missing", field)
			assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel), "field %q", field)
		}
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "pandapt_userdetails", pandaptUserdetailsFixture)
		expected := map[string]string{
			"levelName":    "User",
			"joinTime":     "1746112285",
			"lastAccessAt": "1781707167",
		}
		for field, want := range expected {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok, "selector %q missing", field)
			assert.Equal(t, want, driver.ExtractFieldValuePublic(doc, sel), "field %q", field)
		}
		// 保号 probe requires LastAccess > 0
		laSel := def.UserInfo.Selectors["lastAccessAt"]
		assert.NotEmpty(t, driver.ExtractFieldValuePublic(doc, laSel))
	})
}

func TestPandaPT_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      pandaptSearchFixture,
		"detail":      pandaptDetailFixture,
		"index":       pandaptIndexFixture,
		"userdetails": pandaptUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
