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
	RegisterFixtureSuite(FixtureSuite{SiteID: "gtkpw", Search: testGTKPWSearch, Detail: testGTKPWDetail, UserInfo: testGTKPWUserInfo})
}

const gtkpwSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="电影/Movies" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=45429">12.Angry.Men.1957.BluRay.1080p-DEMO</a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" />
    <br/><span>演示标题 / Demo</span>
  </td></tr></table></td>
  <td class="rowfollow">1</td>
  <td class="rowfollow"><span title="2025-05-14 03:00:00">1年</span></td>
  <td class="rowfollow">11.87 GB</td>
  <td class="rowfollow">127</td>
  <td class="rowfollow">0</td>
  <td class="rowfollow">585</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="剧集/TV" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=45430">GTKPW.Demo2.Series-DEMO</a>
    <img class="pro_50pctdown" src="pic/trans.gif" alt="50%" />
    <br/><span>演示标题2 / Demo2</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-05-13 12:00:00">1天</span></td>
  <td class="rowfollow">2.34 GB</td>
  <td class="rowfollow">9</td>
  <td class="rowfollow">2</td>
  <td class="rowfollow">15</td>
  <td class="rowfollow">DemoTeam</td>
</tr>
</tbody></table>
</body></html>`

const gtkpwIndexFixture = `<html><body>
<table id="info_block"><tr><td>
欢迎回来, <span class="nowrap"><a href="https://pt.gtkpw.xyz/userdetails.php?id=7283" class='User_Name'><b>gtkpw_user</b></a></span>
<font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 12,345.6<br/>
<font class="color_ratio">分享率:</font> 6.789
<font class='color_uploaded'>上传量:</font> 3.50 TB
<font class='color_downloaded'> 下载量:</font> 600.00 GB
<font class='color_active'>当前活动:</font> <img class="arrowup" src="pic/trans.gif" />12 <img class="arrowdown" src="pic/trans.gif" />0
</td></tr></table>
</body></html>`

const gtkpwUserdetailsFixture = `<html><body>
<table>
  <tr><td class="rowhead">用户ID/UID</td><td class="rowfollow">7283</td></tr>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2025-08-20 09:00:00 (<span title="2025-08-20 09:00:00">8月前</span>)</td></tr>
  <tr><td class="rowhead">传输</td><td class="rowfollow"><table><tr><td class="embedded"><strong>分享率</strong>: <font color="">6.789</font></td></tr><tr><td class="embedded"><strong>上传量</strong>: 3.50 TB</td><td class="embedded"><strong>下载量</strong>: 600.00 GB</td></tr></table></td></tr>
  <tr><td class="rowhead">等级</td><td class="rowfollow"><img title="Crazy User" src="pic/crazyuser.gif" /></td></tr>
</table>
</body></html>`

const gtkpwDetailFixture = `<html><body>
<input type="hidden" name="torrent_name" value="The.Boys.S05E07.2026.2160p-DEMO" />
<input type="hidden" name="detail_torrent_id" value="45429" />
<h1 align="center" id="top">The.Boys.S05E07.2026.2160p-DEMO&nbsp;&nbsp;&nbsp; <b>[<font class='free'>免费</font>]</b> <font color='#0000FF'>剩余时间：<span title="2026-07-13 11:38:25">1月29天</span></font></h1>
<table>
  <tr><td class="rowhead">下载</td><td class="rowfollow"><a href="download.php?id=45429">demo.torrent</a></td></tr>
  <tr><td class="rowhead">副标题</td><td class="rowfollow">演示标题 / Demo</td></tr>
  <tr><td class="rowhead">基本信息</td><td class="rowfollow">大小</td><td class="rowfollow">大小：11.87 GB</td></tr>
</table>
</body></html>`

func getGTKPWDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("gtkpw")
	require.True(t, ok)
	return def
}

func testGTKPWSearch(t *testing.T) {
	def := getGTKPWDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(gtkpwSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: http.MethodGet})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "45429", items[0].ID)
	assert.Equal(t, "12.Angry.Men.1957.BluRay.1080p-DEMO", items[0].Title)
	assert.Equal(t, "演示标题 / Demo", items[0].Subtitle)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 127, items[0].Seeders)
	assert.Equal(t, 0, items[0].Leechers)
	assert.Equal(t, 585, items[0].Snatched)

	assert.Equal(t, "45430", items[1].ID)
	assert.Equal(t, v2.DiscountPercent50, items[1].DiscountLevel)
}

func testGTKPWDetail(t *testing.T) {
	def := getGTKPWDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "gtkpw_detail", gtkpwDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "45429", info.TorrentID)
	assert.Equal(t, "The.Boys.S05E07.2026.2160p-DEMO", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 12154.88, info.SizeMB, 1.0)
}

func testGTKPWUserInfo(t *testing.T) {
	def := getGTKPWDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "gtkpw_index", gtkpwIndexFixture)
	assert.Equal(t, "7283", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "gtkpw_user", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "12345.6", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "12", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "gtkpw_userdetails", gtkpwUserdetailsFixture)
	assert.Equal(t, "3848290697216", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "644245094400", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "6.789", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "Crazy User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
}

func TestGTKPW_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      gtkpwSearchFixture,
		"index":       gtkpwIndexFixture,
		"userdetails": gtkpwUserdetailsFixture,
		"detail":      gtkpwDetailFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) { RequireNoSecrets(t, name, data) })
	}
}
