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
	RegisterFixtureSuite(FixtureSuite{SiteID: "longpt", Search: testLongPTSearch, Detail: testLongPTDetail, UserInfo: testLongPTUserInfo})
}

const longptSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="电影/Movies" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=38948">Dong.Bei.Qi.Miao.Demo-LongWeb</a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" />
    <br/><span>演示标题 / Demo</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-05-13 05:21:28">2时11分</span></td>
  <td class="rowfollow">5.65 GB</td>
  <td class="rowfollow">21</td>
  <td class="rowfollow">5</td>
  <td class="rowfollow">23</td>
  <td class="rowfollow">LongWeb</td>
</tr>
<tr>
  <td class="rowfollow"><img alt="动漫/Anime" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=38950">LongPT.Demo2.Anime-DEMO</a>
    <img class="pro_30pctdown" src="pic/trans.gif" alt="30%" />
    <br/><span>演示标题2 / Demo2</span>
  </td></tr></table></td>
  <td class="rowfollow">2</td>
  <td class="rowfollow"><span title="2026-05-12 12:00:00">1天">
  <td class="rowfollow">512.50 MB</td>
  <td class="rowfollow">15</td>
  <td class="rowfollow">3</td>
  <td class="rowfollow">42</td>
  <td class="rowfollow">DemoTeam</td>
</tr>
</tbody></table>
</body></html>`

const longptIndexFixture = `<html><body>
<table id="info_block"><tr><td>
欢迎回来, <span class="nowrap"><a href="https://longpt.org/userdetails.php?id=22222" class='User_Name'><b>longpt_user</b></a></span>
<font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 4,567.8<br/>
<font class="color_ratio">分享率:</font> 3.456
<font class='color_uploaded'>上传量:</font> 1.20 TB
<font class='color_downloaded'> 下载量:</font> 350.00 GB
<font class='color_active'>当前活动:</font> <img class="arrowup" src="pic/trans.gif" />5 <img class="arrowdown" src="pic/trans.gif" />0
</td></tr></table>
</body></html>`

const longptUserdetailsFixture = `<html><body>
<table>
  <tr><td class="rowhead">用户ID/UID</td><td class="rowfollow">22222</td></tr>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2026-02-10 14:00:00 (<span title="2026-02-10 14:00:00">2月前</span>)</td></tr>
  <tr><td class="rowhead">传输</td><td class="rowfollow"><table><tr><td class="embedded"><strong>分享率</strong>: <font color="">3.456</font></td></tr><tr><td class="embedded"><strong>上传量</strong>: 1.20 TB</td><td class="embedded"><strong>下载量</strong>: 350.00 GB</td></tr></table></td></tr>
  <tr><td class="rowhead">等级</td><td class="rowfollow"><img title="Elite User" src="pic/eliteuser.gif" /></td></tr>
</table>
</body></html>`

const longptDetailFixture = `<html><body>
<input type="hidden" name="torrent_name" value="Dong.Bei.Qi.Miao.Demo-LongWeb" />
<input type="hidden" name="detail_torrent_id" value="38948" />
<h1 align="center" id="top">Dong.Bei.Qi.Miao.Demo-LongWeb&nbsp;&nbsp;&nbsp; <b>[<font class='free'>免费</font>]</b> <font color='#0000FF'>剩余时间：<span title="2026-05-28 13:21:28">14天21时</span></font></h1>
<table>
  <tr><td class="rowhead">下载</td><td class="rowfollow"><a href="download.php?id=38948">demo.torrent</a></td></tr>
  <tr><td class="rowhead">副标题</td><td class="rowfollow">演示标题 / Demo</td></tr>
  <tr><td class="rowhead">基本信息</td><td class="rowfollow">大小</td><td class="rowfollow">大小：5.65 GB</td></tr>
</table>
</body></html>`

func getLongPTDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("longpt")
	require.True(t, ok)
	return def
}

func testLongPTSearch(t *testing.T) {
	def := getLongPTDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(longptSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: http.MethodGet})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(items), 1)

	assert.Equal(t, "38948", items[0].ID)
	assert.Equal(t, "Dong.Bei.Qi.Miao.Demo-LongWeb", items[0].Title)
	assert.Equal(t, "演示标题 / Demo", items[0].Subtitle)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 21, items[0].Seeders)
	assert.Equal(t, 5, items[0].Leechers)
	assert.Equal(t, 23, items[0].Snatched)
}

func testLongPTDetail(t *testing.T) {
	def := getLongPTDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "longpt_detail", longptDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "38948", info.TorrentID)
	assert.Equal(t, "Dong.Bei.Qi.Miao.Demo-LongWeb", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 5785.6, info.SizeMB, 1.0)
}

func testLongPTUserInfo(t *testing.T) {
	def := getLongPTDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "longpt_index", longptIndexFixture)
	assert.Equal(t, "22222", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "longpt_user", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "4567.8", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "5", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "longpt_userdetails", longptUserdetailsFixture)
	assert.Equal(t, "1319413953331", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "375809638400", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "3.456", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "Elite User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "1770703200", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["joinTime"]))
}

func TestLongPT_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      longptSearchFixture,
		"index":       longptIndexFixture,
		"userdetails": longptUserdetailsFixture,
		"detail":      longptDetailFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) { RequireNoSecrets(t, name, data) })
	}
}
