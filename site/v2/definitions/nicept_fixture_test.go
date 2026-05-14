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
	RegisterFixtureSuite(FixtureSuite{SiteID: "nicept", Search: testNicePTSearch, Detail: testNicePTDetail, UserInfo: testNicePTUserInfo})
}

const niceptSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="电影/Movies" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=51242">Demo.Title.2011.2160p.HQ.WEB-DL-DEMO</a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" />
    <img class="hitandrun" src="pic/trans.gif" alt="H&amp;R" title="H&amp;R" />
    <br/><span>演示副标题 / Demo Subtitle</span>
  </td></tr></table></td>
  <td class="rowfollow">59</td>
  <td class="rowfollow"><span title="2024-01-14 04:00:00">2年4月</span></td>
  <td class="rowfollow">26.07 GB</td>
  <td class="rowfollow">560</td>
  <td class="rowfollow">1</td>
  <td class="rowfollow">6717</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="动漫/Anime" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=51243">NicePT.Demo2.Anime-DEMO</a>
    <img class="pro_50pctdown" src="pic/trans.gif" alt="50%" />
    <br/><span>演示副标题2 / Demo Subtitle 2</span>
  </td></tr></table></td>
  <td class="rowfollow">3</td>
  <td class="rowfollow"><span title="2026-05-13 12:00:00">1天</span></td>
  <td class="rowfollow">1.50 GB</td>
  <td class="rowfollow">42</td>
  <td class="rowfollow">2</td>
  <td class="rowfollow">128</td>
  <td class="rowfollow">DemoTeam</td>
</tr>
</tbody></table>
</body></html>`

const niceptIndexFixture = `<html><body>
<table id="info_block"><tr><td>
歡迎回來, <span class="nowrap"><a href="https://www.nicept.net/userdetails.php?id=1188761" class='User_Name'><b>nicept_user</b></a></span>
<font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 23,456.7<br/>
<font class="color_ratio">分享率:</font> 7.890
<font class='color_uploaded'>上傳量:</font> 5.00 TB
<font class='color_downloaded'> 下載量:</font> 650.00 GB
<font class='color_active'>當前活動:</font> <img class="arrowup" src="pic/trans.gif" />25 <img class="arrowdown" src="pic/trans.gif" />0
</td></tr></table>
</body></html>`

const niceptUserdetailsFixture = `<html><body>
<table>
  <tr><td class="rowhead">用戶ID/UID</td><td class="rowfollow">1188761</td></tr>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2025-06-15 10:00:00 (<span title="2025-06-15 10:00:00">11月前</span>)</td></tr>
  <tr><td class="rowhead">傳送</td><td class="rowfollow"><table><tr><td class="embedded"><strong>分享率</strong>: <font color="">7.890</font></td></tr><tr><td class="embedded"><strong>上傳量</strong>: 5.00 TB</td><td class="embedded"><strong>下載量</strong>: 650.00 GB</td></tr></table></td></tr>
  <tr><td class="rowhead">等級</td><td class="rowfollow"><img title="Veteran User" src="pic/veteran.gif" /></td></tr>
</table>
</body></html>`

const niceptDetailFixture = `<html><body>
<input type="hidden" name="torrent_name" value="Demo.Title.2011.2160p.HQ.WEB-DL-DEMO" />
<input type="hidden" name="detail_torrent_id" value="51242" />
<h1 align="center" id="top">Demo.Title.2011.2160p.HQ.WEB-DL-DEMO&nbsp;&nbsp;&nbsp; <b>[<font class='free'>免費</font>]</b><img class="hitandrun" src="pic/trans.gif" alt="H&amp;R" title="H&amp;R" /></h1>
<table>
  <tr><td class="rowhead">下載連結</td><td class="rowfollow"><a href="download.php?id=51242">demo.torrent</a></td></tr>
  <tr><td class="rowhead">副標題</td><td class="rowfollow">演示副標題 / Demo Subtitle</td></tr>
  <tr><td class="rowhead">基本資訊</td><td class="rowfollow">大小：26.07 GB</td></tr>
</table>
</body></html>`

func getNicePTDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("nicept")
	require.True(t, ok)
	return def
}

func testNicePTSearch(t *testing.T) {
	def := getNicePTDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(niceptSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: http.MethodGet})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "51242", items[0].ID)
	assert.Equal(t, "Demo.Title.2011.2160p.HQ.WEB-DL-DEMO", items[0].Title)
	assert.Equal(t, "演示副标题 / Demo Subtitle", items[0].Subtitle)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 560, items[0].Seeders)
	assert.Equal(t, 1, items[0].Leechers)
	assert.Equal(t, 6717, items[0].Snatched)

	assert.Equal(t, "51243", items[1].ID)
	assert.Equal(t, v2.DiscountPercent50, items[1].DiscountLevel)
}

func testNicePTDetail(t *testing.T) {
	def := getNicePTDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "nicept_detail", niceptDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "51242", info.TorrentID)
	assert.Equal(t, "Demo.Title.2011.2160p.HQ.WEB-DL-DEMO", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.True(t, info.HasHR)
	assert.InDelta(t, 26695.68, info.SizeMB, 1.0)
}

func testNicePTUserInfo(t *testing.T) {
	def := getNicePTDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "nicept_index", niceptIndexFixture)
	assert.Equal(t, "1188761", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "nicept_user", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "23456.7", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "25", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "nicept_userdetails", niceptUserdetailsFixture)
	assert.Equal(t, "5497558138880", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "697932185600", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "7.89", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "Veteran User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
}

func TestNicePT_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      niceptSearchFixture,
		"index":       niceptIndexFixture,
		"userdetails": niceptUserdetailsFixture,
		"detail":      niceptDetailFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) { RequireNoSecrets(t, name, data) })
	}
}
