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
	RegisterFixtureSuite(FixtureSuite{SiteID: "hxpt", Search: testHXPTSearch, Detail: testHXPTDetail, UserInfo: testHXPTUserInfo})
}

const hxptSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="纪录片" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <div class='torrent-title-line1'><a href="details.php?id=3363"><b>Animal Impossible 2022 不可思议的动物 2160p 四音轨</b></a> <img class="pro_free" src="pic/trans.gif" alt="Free" /></div>
    <div class='torrent-subtitle' title='不可思议的动物 / 自然科学'>不可思议的动物 / 自然科学</div>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-03-24 12:05:14">3天</span></td>
  <td class="rowfollow">8.00 GB</td>
  <td class="rowfollow">16</td>
  <td class="rowfollow">1</td>
  <td class="rowfollow">28</td>
  <td class="rowfollow"><a href="download.php?id=3363">下载</a></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="教育" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <div class='torrent-title-line1'><a href="details.php?id=3357"><b>Piano Course Collection Data Pack</b></a> <b>[<font class='free'>保种</font>]</b></div>
    <div class='torrent-subtitle' title='钢琴教程合集'>钢琴教程合集</div>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-03-24 10:00:00">3天2时</span></td>
  <td class="rowfollow">2.00 GB</td>
  <td class="rowfollow">12</td>
  <td class="rowfollow">1</td>
  <td class="rowfollow">13</td>
  <td class="rowfollow"><a href="download.php?id=3357">下载</a></td>
</tr>
</tbody></table>
</body></html>`

const hxptIndexFixture = `<html><body>
<table id="info_block"><tr><td>
<span class="nowrap"><a href="https://www.hxpt.org/userdetails.php?id=13088" class='User_Name'><b>hxpt_user</b></a></span>
<font class='color_bonus'>火花 </font>[<a href="mybonus.php">使用</a>]: 40,902.2<br/>
<font class="color_ratio">分享率:</font> 4.000
<font class='color_uploaded'>上传量:</font> 0.00 KB
<font class='color_downloaded'> 下载量:</font> 0.00 KB
<font class='color_active'>当前活动:</font> <img class="arrowup" src="pic/trans.gif" />0 <img class="arrowdown" src="pic/trans.gif" />0
</td></tr></table>
</body></html>`

const hxptUserdetailsFixture = `<html><body>
<table>
  <tr><td class="rowhead">用户ID/UID</td><td class="rowfollow">13999</td></tr>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2026-03-18 17:37:49 (<span title="2026-03-18 17:37:49">8天前</span>)</td></tr>
  <tr><td class="rowhead">传输</td><td class="rowfollow"><table><tr><td class="embedded"><strong>分享率</strong>: <font color="">4.000</font></td></tr><tr><td class="embedded"><strong>上传量</strong>: 256.00 GB</td><td class="embedded"><strong>下载量</strong>: 64.00 GB</td></tr></table></td></tr>
  <tr><td class="rowhead">等级</td><td class="rowfollow"><img title="User" src="pic/user.gif" /></td></tr>
</table>
</body></html>`

const hxptDetailFixture = `<html><body>
<h1 align="center" id="top">Animal Impossible 2022 不可思议的动物 2160p 四音轨 <b>[<font class='free'>免费</font>]</b> <font color='#0000FF'>剩余时间：<span title="2026-03-31 12:05:14">3天23时</span></font></h1>
<input name="torrent_name" type="hidden" value="Animal Impossible 2022 不可思议的动物 2160p 四音轨" />
<input name="detail_torrent_id" type="hidden" value="3363" />
<table>
  <tr><td class="rowhead">下载</td><td class="rowfollow"><a href="download.php?id=3363">demo.torrent</a></td></tr>
  <tr><td class="rowhead">副标题</td><td class="rowfollow">不可思议的动物</td></tr>
  <tr><td class="rowhead">基本信息</td><td class="rowfollow"><b>大小：</b>8.00 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;纪录片&nbsp;&nbsp;&nbsp;<b>媒介:</b>&nbsp;视频</td></tr>
</table>
</body></html>`

func getHXPTDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("hxpt")
	require.True(t, ok)
	return def
}

func testHXPTSearch(t *testing.T) {
	def := getHXPTDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(hxptSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: http.MethodGet})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "3363", items[0].ID)
	assert.Equal(t, "Animal Impossible 2022 不可思议的动物 2160p 四音轨", items[0].Title)
	assert.Equal(t, "不可思议的动物 / 自然科学", items[0].Subtitle)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 16, items[0].Seeders)
	assert.Equal(t, 1, items[0].Leechers)
	assert.Equal(t, 28, items[0].Snatched)

	assert.Equal(t, "3357", items[1].ID)
	assert.Equal(t, v2.DiscountFree, items[1].DiscountLevel)
}

func testHXPTDetail(t *testing.T) {
	def := getHXPTDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "hxpt_detail", hxptDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "3363", info.TorrentID)
	assert.Equal(t, "Animal Impossible 2022 不可思议的动物 2160p 四音轨", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 8192, info.SizeMB, 0.1)
}

func testHXPTUserInfo(t *testing.T) {
	def := getHXPTDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "hxpt_index", hxptIndexFixture)
	assert.Equal(t, "13088", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "hxpt_user", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "40902.2", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "hxpt_userdetails", hxptUserdetailsFixture)
	assert.Equal(t, "274877906944", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "68719476736", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "4", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "1773855469", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["joinTime"]))
}

func TestHXPT_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      hxptSearchFixture,
		"index":       hxptIndexFixture,
		"userdetails": hxptUserdetailsFixture,
		"detail":      hxptDetailFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) { RequireNoSecrets(t, name, data) })
	}
}
