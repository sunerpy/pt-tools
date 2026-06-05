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
	RegisterFixtureSuite(FixtureSuite{SiteID: "duckboobee", Search: testDuckBoobeeSearch, Detail: testDuckBoobeeDetail, UserInfo: testDuckBoobeeUserInfo})
}

const duckboobeeSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="电影/Movies" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=73414">Marty.Life.Is.Short.2026.2160p-DEMO</a>
    <img class="pro_free2up" src="pic/trans.gif" alt="2X Free" />
    <br/><span>演示标题 / Demo</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-05-13 06:43:22">16时53分</span></td>
  <td class="rowfollow">14.26 GB</td>
  <td class="rowfollow">4</td>
  <td class="rowfollow">0</td>
  <td class="rowfollow">6</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="剧集/TV" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=73415">DuckBoobee.Demo2.Series.S01-DBB</a>
    <img class="pro_50pctdown" src="pic/trans.gif" alt="50%" />
    <br/><span>演示标题2 / Demo2</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-05-12 12:00:00">1天</span></td>
  <td class="rowfollow">3.21 GB</td>
  <td class="rowfollow">12</td>
  <td class="rowfollow">2</td>
  <td class="rowfollow">30</td>
  <td class="rowfollow">DBB-Team</td>
</tr>
</tbody></table>
</body></html>`

const duckboobeeIndexFixture = `<html><body>
<table id="info_block"><tr><td>
欢迎回来, <span class="nowrap"><a href="https://duckboobee.org/userdetails.php?id=22392" class='User_Name'><b>duckboobee_user</b></a></span>
<font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 8,765.4<br/>
<font class="color_ratio">分享率:</font> 5.123
<font class='color_uploaded'>上传量:</font> 2.50 TB
<font class='color_downloaded'> 下载量:</font> 500.00 GB
<font class='color_active'>当前活动:</font> <img class="arrowup" src="pic/trans.gif" />8 <img class="arrowdown" src="pic/trans.gif" />2
</td></tr></table>
</body></html>`

const duckboobeeUserdetailsFixture = `<html><body>
<table>
  <tr><td class="rowhead">用户ID/UID</td><td class="rowfollow">22392</td></tr>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2026-01-15 10:30:00 (<span title="2026-01-15 10:30:00">3月前</span>)</td></tr>
  <tr><td class="rowhead">传输</td><td class="rowfollow"><table><tr><td class="embedded"><strong>分享率</strong>: <font color="">5.123</font></td></tr><tr><td class="embedded"><strong>上传量</strong>: 2.50 TB</td><td class="embedded"><strong>下载量</strong>: 500.00 GB</td></tr></table></td></tr>
  <tr><td class="rowhead">等级</td><td class="rowfollow"><img title="Power User" src="pic/poweruser.gif" /></td></tr>
</table>
</body></html>`

const duckboobeeDetailFixture = `<html><body>
<input type="hidden" name="torrent_name" value="Marty.Life.Is.Short.2026.2160p-DEMO" />
<input type="hidden" name="detail_torrent_id" value="73414" />
<h1 align="center" id="top">Marty.Life.Is.Short.2026.2160p-DEMO&nbsp;&nbsp;&nbsp; <b>[<font class='twoupfree'>2X免费</font>]</b> <font color='#00CC66'>剩余时间：<span title="2026-06-11 22:43:22">29天7时</span></font></h1>
<table>
  <tr><td class="rowhead">下载</td><td class="rowfollow"><a href="download.php?id=73414">demo.torrent</a></td></tr>
  <tr><td class="rowhead">副标题</td><td class="rowfollow">演示标题 / Demo</td></tr>
  <tr><td class="rowhead">基本信息</td><td class="rowfollow">大小</td><td class="rowfollow">大小：14.26 GB</td></tr>
</table>
</body></html>`

func getDuckBoobeeDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("duckboobee")
	require.True(t, ok)
	return def
}

func testDuckBoobeeSearch(t *testing.T) {
	def := getDuckBoobeeDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(duckboobeeSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: http.MethodGet})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "73414", items[0].ID)
	assert.Equal(t, "Marty.Life.Is.Short.2026.2160p-DEMO", items[0].Title)
	assert.Equal(t, "演示标题 / Demo", items[0].Subtitle)
	assert.Equal(t, v2.Discount2xFree, items[0].DiscountLevel)
	assert.Equal(t, 4, items[0].Seeders)
	assert.Equal(t, 0, items[0].Leechers)
	assert.Equal(t, 6, items[0].Snatched)

	assert.Equal(t, "73415", items[1].ID)
	assert.Equal(t, v2.DiscountPercent50, items[1].DiscountLevel)
}

func testDuckBoobeeDetail(t *testing.T) {
	def := getDuckBoobeeDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "duckboobee_detail", duckboobeeDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "73414", info.TorrentID)
	assert.Equal(t, "Marty.Life.Is.Short.2026.2160p-DEMO", info.Title)
	assert.Equal(t, v2.Discount2xFree, info.DiscountLevel)
	assert.InDelta(t, 14602.24, info.SizeMB, 0.5)
}

func testDuckBoobeeUserInfo(t *testing.T) {
	def := getDuckBoobeeDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "duckboobee_index", duckboobeeIndexFixture)
	assert.Equal(t, "22392", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "duckboobee_user", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "8765.4", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "8", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "2", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "duckboobee_userdetails", duckboobeeUserdetailsFixture)
	assert.Equal(t, "2748779069440", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "536870912000", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "5.123", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "Power User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "1768444200", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["joinTime"]))
}

func TestDuckBoobee_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      duckboobeeSearchFixture,
		"index":       duckboobeeIndexFixture,
		"userdetails": duckboobeeUserdetailsFixture,
		"detail":      duckboobeeDetailFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) { RequireNoSecrets(t, name, data) })
	}
}
