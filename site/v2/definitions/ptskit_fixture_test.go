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
	RegisterFixtureSuite(FixtureSuite{SiteID: "ptskit", Search: testPTSKITSearch, Detail: testPTSKITDetail, UserInfo: testPTSKITUserInfo})
}

const ptskitSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="其他" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=3160">从零开始玩PT V1.0 PDF</a>
    <img class="pro_free2up" src="pic/trans.gif" alt="2X Free" title="2X免费" />
    <br/><span>萌新入门教材 / 新手必看</span>
  </td></tr></table></td>
  <td class="rowfollow">1</td>
  <td class="rowfollow"><span title="2025-06-11 22:15:57">9月18天</span></td>
  <td class="rowfollow">2.00 MB</td>
  <td class="rowfollow">264</td>
  <td class="rowfollow">0</td>
  <td class="rowfollow">806</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="电影" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=35826">Manjianghong.2025.1080p.GER.BluRay-PTSKIT</a>
    <img class="pro_50pctdown2up" src="pic/trans.gif" alt="2X 50%" />
    <br/><span>满江红 / 2X 50%</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-03-27 08:57:13">3时20分钟</span></td>
  <td class="rowfollow">42.32 GB</td>
  <td class="rowfollow">2</td>
  <td class="rowfollow">6</td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table>
</body></html>`

const ptskitIndexFixture = `<html><body>
<table id="info_block"><tr><td>
欢迎回来, <span class="nowrap"><a href="https://www.ptskit.org/userdetails.php?id=7528" class='User_Name'><b>ptskit_user</b></a></span>
<font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 50,876.4<br/>
<font class="color_ratio">分享率:</font> 4.000
<font class='color_uploaded'>上传量:</font> 0.00 KB
<font class='color_downloaded'> 下载量:</font> 0.00 KB
<font class='color_active'>当前活动:</font> <img class="arrowup" src="pic/trans.gif" />0 <img class="arrowdown" src="pic/trans.gif" />0
</td></tr></table>
</body></html>`

const ptskitUserdetailsFixture = `<html><body>
<table>
  <tr><td class="rowhead">用户ID/UID</td><td class="rowfollow">7528</td></tr>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2025-11-06 00:00:46 (<span title="2025-11-06 00:00:46">4月前</span>)</td></tr>
  <tr><td class="rowhead">传输</td><td class="rowfollow"><table><tr><td class="embedded"><strong>分享率</strong>: <font color="">4.000</font></td></tr><tr><td class="embedded"><strong>上传量</strong>: 512.00 GB</td><td class="embedded"><strong>下载量</strong>: 128.00 GB</td></tr></table></td></tr>
  <tr><td class="rowhead">等级</td><td class="rowfollow"><img title="Power User" src="pic/poweruser.gif" /></td></tr>
</table>
</body></html>`

const ptskitDetailFixture = `<html><body>
<h1 align="center" id="top" value="从零开始玩PT V1.0 PDF"><input name="torrent_name" type="hidden" value="从零开始玩PT V1.0 PDF" /><input name="detail_torrent_id" type="hidden" value="3160" />从零开始玩PT V1.0 PDF <b>[<font class='twoupfree'>2X免费</font>]</b></h1>
<table>
  <tr><td class="rowhead">下载</td><td class="rowfollow"><a href="download.php?id=3160">demo.torrent</a></td></tr>
  <tr><td class="rowhead">副标题</td><td class="rowfollow">萌新入门教材 / 新手必看</td></tr>
  <tr><td class="rowhead">基本信息</td><td class="rowfollow">大小</td><td class="rowfollow">大小：2.00 MB</td></tr>
</table>
</body></html>`

func getPTSKITDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("ptskit")
	require.True(t, ok)
	return def
}

func testPTSKITSearch(t *testing.T) {
	def := getPTSKITDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(ptskitSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: http.MethodGet})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "3160", items[0].ID)
	assert.Equal(t, "从零开始玩PT V1.0 PDF", items[0].Title)
	assert.Equal(t, "萌新入门教材 / 新手必看", items[0].Subtitle)
	assert.Equal(t, v2.Discount2xFree, items[0].DiscountLevel)
	assert.Equal(t, 264, items[0].Seeders)
	assert.Equal(t, 0, items[0].Leechers)
	assert.Equal(t, 806, items[0].Snatched)

	assert.Equal(t, "35826", items[1].ID)
	assert.Equal(t, v2.DiscountPercent50, items[1].DiscountLevel)
}

func testPTSKITDetail(t *testing.T) {
	def := getPTSKITDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "ptskit_detail", ptskitDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "3160", info.TorrentID)
	assert.Equal(t, "从零开始玩PT V1.0 PDF", info.Title)
	assert.Equal(t, v2.Discount2xFree, info.DiscountLevel)
	assert.InDelta(t, 2, info.SizeMB, 0.1)
}

func testPTSKITUserInfo(t *testing.T) {
	def := getPTSKITDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "ptskit_index", ptskitIndexFixture)
	assert.Equal(t, "7528", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "ptskit_user", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "50876.4", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "ptskit_userdetails", ptskitUserdetailsFixture)
	assert.Equal(t, "549755813888", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "137438953472", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "4", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "Power User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "1762387246", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["joinTime"]))
}

func TestPTSKIT_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      ptskitSearchFixture,
		"index":       ptskitIndexFixture,
		"userdetails": ptskitUserdetailsFixture,
		"detail":      ptskitDetailFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) { RequireNoSecrets(t, name, data) })
	}
}
