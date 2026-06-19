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
		SiteID:   "kamept",
		Search:   testKamePTSearch,
		Detail:   testKamePTDetail,
		UserInfo: testKamePTUserInfo,
	})
}

const kameptSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=410"><img class="c_0" src="pic/cattrans.gif" alt="同人AV" title="同人AV" /></a></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded"><img src="pic/misc/spinner.svg" class="nexus-lazy-load" /></td>
    <td class="embedded">
      <img class="sticky" src="pic/trans.gif" alt="Sticky" title="一级置顶" />&nbsp;
      <a title="Sample Cosplay Release 01" href="details.php?id=42507&amp;hit=1"><b>Sample Cosplay Release 01</b></a>
      <b> (<font class='new'>新</font>)</b>
      <img class="pro_free" src="pic/trans.gif" alt="Free" title="免费" />
      <br /><span style="background-color:#483d8b" title="">原盘</span><span style="background-color:#ff0000" title="">禁转</span>样品 写真集
    </td></tr></table></td>
  <td class="rowfollow"><a href="comment.php?action=add&amp;pid=42507&amp;type=torrent">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-06-18 02:38:11">22时<br />5分钟</span></td>
  <td class="rowfollow">6.55<br />GB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=42507&amp;dllist=1#seeders">95</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=42507&amp;dllist=1#leechers">2</a></b></td>
  <td class="rowfollow"><a href="viewsnatches.php?id=42507"><b>141</b></a></td>
</tr>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=410"><img class="c_0" src="pic/cattrans.gif" alt="同人AV" title="同人AV" /></a></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded"><img src="pic/misc/spinner.svg" class="nexus-lazy-load" /></td>
    <td class="embedded">
      <a title="Sample Cosplay Release 02" href="details.php?id=42506&amp;hit=1"><b>Sample Cosplay Release 02</b></a>
      <b> (<font class='new'>新</font>)</b>
      <br />样品 写真集
    </td></tr></table></td>
  <td class="rowfollow"><a href="comment.php?action=add&amp;pid=42506&amp;type=torrent">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-06-18 02:38:11">22时<br />5分钟</span></td>
  <td class="rowfollow">2.76<br />GB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=42506&amp;dllist=1#seeders">95</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=42506&amp;dllist=1#leechers">2</a></b></td>
  <td class="rowfollow"><a href="viewsnatches.php?id=42506"><b>141</b></a></td>
</tr>
</tbody></table>
</body></html>`

const kameptDetailFixture = `<html><body>
<h1>
  <input type="hidden" name="torrent_name" value="Sample Cosplay Release 01" />
  <input type="hidden" name="detail_torrent_id" value="42507" />
  Sample Cosplay Release 01&nbsp;&nbsp;&nbsp; <b>[<font class='free' >免费</font>]</b>
</h1>
<table>
  <tr><td class="rowhead nowrap">基本信息</td><td class="rowfollow"><b><b>大小：</b></b>6.55 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;同人AV</td></tr>
  <tr><td class="rowhead nowrap">副标题</td><td class="rowfollow">样品 写真集</td></tr>
</table>
</body></html>`

const kameptIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td>
<span class="medium">
欢迎回来, <span class="nowrap"><a href="https://kamept.com/userdetails.php?id=23379" class='User_Name'><b>testuser</b></a></span>
[<a href="usercp.php">控制面板</a>]
<font class = 'color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 9,371.4
<font class = 'color_bonus'>做种积分</font>: 24,496.5
<font class = 'color_invite'>邀请 </font>[<a href="invite.php?id=23379">发送</a>]: 0(0)
<br />
<font class="color_ratio">分享率:</font> 2.110
<font class='color_uploaded'>上传量:</font> 211.03 GB
<font class='color_downloaded'> 下载量:</font> 100.02 GB
<font class='color_active'>当前活动:</font>
<img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />6
<img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />0
</span>
</td></tr></table>
</body></html>`

const kameptUserdetailsFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td>
<span class="medium">
欢迎回来, <span class="nowrap"><a href="https://kamept.com/userdetails.php?id=23379" class='User_Name'><b>testuser</b></a></span>
<font class = 'color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 9,371.4
<font class = 'color_bonus'>做种积分</font>: 24,496.5
<br />
<font class="color_ratio">分享率:</font> 2.110
<font class='color_uploaded'>上传量:</font> 211.03 GB
<font class='color_downloaded'> 下载量:</font> 100.02 GB
<font class='color_active'>当前活动:</font>
<img class="arrowup" src="pic/trans.gif" />6
<img class="arrowdown" src="pic/trans.gif" />0
</span>
</td></tr></table>
<table>
  <tr><td class="rowhead nowrap">加入日期</td><td class="rowfollow">2026-02-16 09:09:17 (<span title="2026-02-16 09:09:17">4月2天前</span>, 18周)</td></tr>
  <tr><td class="rowhead nowrap">最近动向</td><td class="rowfollow">2026-06-19 00:43:25 (<span title="2026-06-19 00:43:25">&lt; 1分钟前</span>)</td></tr>
  <tr><td class="rowhead nowrap">等级</td><td class="rowfollow"><img alt="User" title="User" src="pic/user.gif" /> </td></tr>
</table>
</body></html>`

func getKamePTDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("kamept")
	require.True(t, ok, "kamept definition not found")
	return def
}

func testKamePTSearch(t *testing.T) {
	def := getKamePTDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(kameptSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "42507", items[0].ID)
	assert.Equal(t, "Sample Cosplay Release 01", items[0].Title)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 95, items[0].Seeders)
	assert.Equal(t, 2, items[0].Leechers)
	assert.Equal(t, 141, items[0].Snatched)

	assert.Equal(t, "42506", items[1].ID)
	assert.Equal(t, v2.DiscountNone, items[1].DiscountLevel)
}

func testKamePTDetail(t *testing.T) {
	def := getKamePTDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "kamept_detail", kameptDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "42507", info.TorrentID)
	assert.Equal(t, "Sample Cosplay Release 01", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 6.55*1024, info.SizeMB, 0.1)
	assert.False(t, info.HasHR)
}

func testKamePTUserInfo(t *testing.T) {
	def := getKamePTDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "kamept_index", kameptIndexFixture)
		fields := map[string]string{
			"id":           "23379",
			"name":         "testuser",
			"bonus":        "9371.4",
			"seedingBonus": "24496.5",
			"ratio":        "2.11",
			"uploaded":     "226591737118",
			"downloaded":   "107395657236",
			"seeding":      "6",
			"leeching":     "0",
		}
		for field, expected := range fields {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok, "selector %q missing", field)
			assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel), "field %q", field)
		}
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "kamept_userdetails", kameptUserdetailsFixture)
		expected := map[string]string{
			"levelName":    "User",
			"joinTime":     "1771204157",
			"lastAccessAt": "1781801005",
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

func TestKamePT_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      kameptSearchFixture,
		"detail":      kameptDetailFixture,
		"index":       kameptIndexFixture,
		"userdetails": kameptUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
