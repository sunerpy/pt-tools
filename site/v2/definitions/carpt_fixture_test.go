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
	RegisterFixtureSuite(FixtureSuite{SiteID: "carpt", Search: testCarPTSearch, Detail: testCarPTDetail, UserInfo: testCarPTUserInfo})
}

const carptSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="电影/Movies" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=154097">Legends.of.The.Condor.Heroes.2025.1080p.x265-CarPT</a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" />
    <img class="hitandrun" src="pic/trans.gif" alt="H&amp;R" title="H&amp;R" />
    <br/><span>侠之大者 / The Gallants</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-03-28 13:01:22">3天2时</span></td>
  <td class="rowfollow">4.00 GB</td>
  <td class="rowfollow">53</td>
  <td class="rowfollow">3</td>
  <td class="rowfollow">69</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="音乐/Music" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=37755">Matthew.Shiff.Piano.Sutras.2013.FLAC-CarPT</a>
    <img class="pro_50pctdown" src="pic/trans.gif" alt="50%" />
    <br/><span>钢琴经 / Piano Sutras</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-03-27 08:00:00">4天</span></td>
  <td class="rowfollow">186.95 MB</td>
  <td class="rowfollow">8</td>
  <td class="rowfollow">1</td>
  <td class="rowfollow">21</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table>
</body></html>`

const carptIndexFixture = `<html><body>
<table id="info_block"><tr><td>
欢迎回来, <span class="nowrap"><a href="https://carpt.net/userdetails.php?id=51478" class='User_Name'><b>carpt_user</b></a></span>
<font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 1,234.5<br/>
<font class="color_ratio">分享率:</font> ---
<font class='color_uploaded'>上传量:</font> 0.00 KB
<font class='color_downloaded'> 下载量:</font> 0.00 KB
<font class='color_active'>当前活动:</font> <img class="arrowup" src="pic/trans.gif" />2 <img class="arrowdown" src="pic/trans.gif" />1
</td></tr></table>
</body></html>`

const carptUserdetailsFixture = `<html><body>
<table>
  <tr><td class="rowhead">用户ID/UID</td><td class="rowfollow">44933</td></tr>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2026-03-20 08:30:00 (<span title="2026-03-20 08:30:00">7天前</span>)</td></tr>
  <tr><td class="rowhead">传输</td><td class="rowfollow"><table><tr><td class="embedded"><strong>分享率</strong>: <font color="">4.000</font></td></tr><tr><td class="embedded"><strong>上传量</strong>: 1.00 TB</td><td class="embedded"><strong>下载量</strong>: 256.00 GB</td></tr></table></td></tr>
  <tr><td class="rowhead">等级</td><td class="rowfollow"><img title="User" src="pic/user.gif" /></td></tr>
</table>
</body></html>`

const carptDetailFixture = `<html><body>
<h1 align="center" id="top" value="Legends.of.The.Condor.Heroes.2025.1080p.x265-CarPT"><input name="torrent_name" type="hidden" value="Legends.of.The.Condor.Heroes.2025.1080p.x265-CarPT" /><input name="detail_torrent_id" type="hidden" value="154097" />Legends.of.The.Condor.Heroes.2025.1080p.x265-CarPT <b>[<font class='free'>免费</font>]</b> <font color='#0000FF'>剩余时间：<span title="2026-04-04 13:01:22">3天21时</span></font><img class="hitandrun" src="pic/trans.gif" alt="H&amp;R" title="H&amp;R" /></h1>
<table>
  <tr><td class="rowhead">下载</td><td class="rowfollow"><a href="download.php?id=154097">demo.torrent</a></td></tr>
  <tr><td class="rowhead">副标题</td><td class="rowfollow">侠之大者 / The Gallants</td></tr>
  <tr><td class="rowhead">基本信息</td><td class="rowfollow">大小</td><td class="rowfollow">大小：4.00 GB</td></tr>
</table>
</body></html>`

func getCarPTDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("carpt")
	require.True(t, ok)
	return def
}

func testCarPTSearch(t *testing.T) {
	def := getCarPTDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(carptSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: http.MethodGet})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "154097", items[0].ID)
	assert.Equal(t, "Legends.of.The.Condor.Heroes.2025.1080p.x265-CarPT", items[0].Title)
	assert.Equal(t, "侠之大者 / The Gallants", items[0].Subtitle)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 53, items[0].Seeders)
	assert.Equal(t, 3, items[0].Leechers)
	assert.Equal(t, 69, items[0].Snatched)

	assert.Equal(t, "37755", items[1].ID)
	assert.Equal(t, v2.DiscountPercent50, items[1].DiscountLevel)
}

func testCarPTDetail(t *testing.T) {
	def := getCarPTDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "carpt_detail", carptDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "154097", info.TorrentID)
	assert.Equal(t, "Legends.of.The.Condor.Heroes.2025.1080p.x265-CarPT", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.True(t, info.HasHR)
	assert.InDelta(t, 4096, info.SizeMB, 0.1)
}

func testCarPTUserInfo(t *testing.T) {
	def := getCarPTDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "carpt_index", carptIndexFixture)
	assert.Equal(t, "51478", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "carpt_user", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "1234.5", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "2", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "1", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "carpt_userdetails", carptUserdetailsFixture)
	assert.Equal(t, "1099511627776", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "274877906944", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "4", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "1773995400", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["joinTime"]))
}

func TestCarPT_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      carptSearchFixture,
		"index":       carptIndexFixture,
		"userdetails": carptUserdetailsFixture,
		"detail":      carptDetailFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) { RequireNoSecrets(t, name, data) })
	}
}
