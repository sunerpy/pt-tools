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
		SiteID:   "lajidui",
		Search:   testLajiduiSearch,
		Detail:   testLajiduiDetail,
		UserInfo: testLajiduiUserInfo,
	})
}

const lajiduiSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="Documentaries/纪录片" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=44336">Counterpunch.2017.1080p.WEB-DL</a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-03-14 11:56:51&quot;&gt;9天23时&lt;/span&gt;&lt;/b&gt;', 'trail', false)" />
    <br/><span>反击 / Counterpunch (2017)</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-03-04 11:56:51">13分钟</span></td>
  <td class="rowfollow">3.55 GB</td>
  <td class="rowfollow">1</td>
  <td class="rowfollow">3</td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="Movies/电影" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=44332">All.The.Way.To.The.Endless.2024</a>
    <img class="pro_free2up" src="pic/trans.gif" alt="2X Free" title="2X免费" />
    <br/><span>一路向前</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-03-04 11:01:36">1时8分钟</span></td>
  <td class="rowfollow">3.67 GB</td>
  <td class="rowfollow">0</td>
  <td class="rowfollow">4</td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table>
</body></html>`

const lajiduiIndexFixture = `<html><body>
<div id="info_block">
  欢迎回来, <a href="https://pt.lajidui.top/userdetails.php?id=12243" class="Peasant_Name"><b>Jccc0201</b></a>
  <font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 973.3
  当前活动: <img class="arrowup" src="pic/trans.gif" />16 <img class="arrowdown" src="pic/trans.gif" />6
</div>
</body></html>`

const lajiduiUserdetailsFixture = `<html><body>
<table>
  <tr><td class="rowhead">用户ID/UID</td><td class="rowfollow">12139</td></tr>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2026-01-20 14:28:57 (<span title="2026-01-20 14:28:57">1月12天前</span>)</td></tr>
  <tr><td class="rowhead">传输</td><td class="rowfollow"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>:  <font color="">4,090.374</font></td><td class="embedded">&nbsp;&nbsp;<img src="pic/smilies/163.gif" alt="" /></td></tr><tr><td class="embedded"><strong>上传量</strong>:  855.29 GB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>:  1.224 TB</td></tr></table></td></tr>
  <tr><td class="rowhead">等级</td><td class="rowfollow"><img title="User" src="pic/user.gif" /></td></tr>
</table>
</body></html>`

const lajiduiDetailFixture = `<html><body>
<h1 align="center" id="top" value="Counterpunch.2017.1080p.WEB-DL">
  <input name="torrent_name" type="hidden" value="Counterpunch.2017.1080p.WEB-DL" />
  <input name="detail_torrent_id" type="hidden" value="44336" />
  <font class='free'>免费</font> <span title="2026-03-14 11:56:51">9天23时</span>
</h1>
<table><tr><td class="rowhead">基本信息</td><td class="rowfollow">大小</td><td class="rowfollow">大小：3.55 GB</td></tr></table>
</body></html>`

const lajiduiDetailWithHRFixture = `<html><body>
<h1 value="Counterpunch.2017.1080p.WEB-DL.HR">
  <input name="torrent_name" type="hidden" value="Counterpunch.2017.1080p.WEB-DL.HR" />
  <input name="detail_torrent_id" type="hidden" value="44337" />
</h1>
<table><tr><td class="rowhead">基本信息</td><td class="rowfollow">大小</td><td class="rowfollow">大小：2.00 GB</td></tr></table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

func getLajiduiDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("lajidui")
	require.True(t, ok)
	return def
}

func testLajiduiSearch(t *testing.T) {
	def := getLajiduiDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(lajiduiSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	first := items[0]
	assert.Equal(t, "44336", first.ID)
	assert.Equal(t, "Counterpunch.2017.1080p.WEB-DL", first.Title)
	assert.Equal(t, "反击 / Counterpunch (2017)", first.Subtitle)
	assert.Equal(t, v2.DiscountFree, first.DiscountLevel)
	assert.Equal(t, 1, first.Seeders)
	assert.Equal(t, 3, first.Leechers)
	assert.Equal(t, 0, first.Snatched)

	second := items[1]
	assert.Equal(t, "44332", second.ID)
	assert.Equal(t, v2.Discount2xFree, second.DiscountLevel)
}

func testLajiduiDetail(t *testing.T) {
	def := getLajiduiDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "lajidui_detail", lajiduiDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "44336", info.TorrentID)
	assert.Equal(t, "Counterpunch.2017.1080p.WEB-DL", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 3.55*1024, info.SizeMB, 0.1)

	hrDoc := FixtureDoc(t, "lajidui_detail_hr", lajiduiDetailWithHRFixture)
	hrInfo := parser.ParseAll(hrDoc.Selection)
	assert.Equal(t, "44337", hrInfo.TorrentID)
	assert.True(t, hrInfo.HasHR)
}

func testLajiduiUserInfo(t *testing.T) {
	def := getLajiduiDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "lajidui_index", lajiduiIndexFixture)
	assert.Equal(t, "12243", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "Jccc0201", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "973.3", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "16", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "6", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "lajidui_userdetails", lajiduiUserdetailsFixture)
	assert.Equal(t, "918360644648", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "1345802232397", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "4090.374", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "1768919337", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["joinTime"]))
}

func TestLajidui_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      lajiduiSearchFixture,
		"index":       lajiduiIndexFixture,
		"userdetails": lajiduiUserdetailsFixture,
		"detail":      lajiduiDetailFixture,
		"detail_hr":   lajiduiDetailWithHRFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
