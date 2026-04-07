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
		SiteID:   "raingfh",
		Search:   testRaingfhSearch,
		Detail:   testRaingfhDetail,
		UserInfo: testRaingfhUserInfo,
	})
}

const raingfhSearchFixture = `<html><body><table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="other" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=26432"><b>从零开始玩PT：从入门到精通</b></a>
    <img class="pro_free2up" src="pic/trans.gif" alt="2X Free" title="2X免费" />
    <br />LASTGL | PDF
  </td></tr></table></td>
  <td class="rowfollow"><a href="comment.php">3</a></td>
  <td class="rowfollow nowrap"><span title="2025-02-23 12:44:17">1年0月</span></td>
  <td class="rowfollow">2.26<br />MB</td>
  <td class="rowfollow" align="center"><b><a href="#seeders">413</a></b></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><a href="#snatches"><b>857</b></a></td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="Movies" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=98269"><b>Dirty Dancing: Havana Nights 2004 1080p Blu-ray AVC DTS-HD MA 7.1-NiXFLiX</b></a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" title="免费" />
    <br />辣身舞2：情迷哈瓦那
  </td></tr></table></td>
  <td class="rowfollow"><a href="comment.php">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-02-25 20:12:20">15分钟</span></td>
  <td class="rowfollow">22.71<br />GB</td>
  <td class="rowfollow" align="center"><b><a href="#seeders">2</a></b></td>
  <td class="rowfollow"><b><a href="#leechers">6</a></b></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table></body></html>`

const raingfhIndexFixture = `<html><body>
<table id="info_block"><tr><td>
欢迎回来, <span class="nowrap"><a href="userdetails.php?id=20703" class="PowerUser_Name"><b>xionghaizi</b></a></span>
<font class='color_bonus'>雨滴 </font>[<a href="mybonus.php">使用</a>]: 18,890.1
<font class="color_ratio">分享率:</font> 1.201
<font class='color_uploaded'>上传量:</font> 448.82 GB
<font class='color_downloaded'> 下载量:</font> 373.78 GB
<img class="arrowup" src="pic/trans.gif" />0
<img class="arrowdown" src="pic/trans.gif" />0
</td></tr></table>
</body></html>`

const raingfhUserdetailsFixture = `<html><body>
<table id="info_block"><tr><td>
欢迎回来, <span class="nowrap"><a href="userdetails.php?id=20703" class="PowerUser_Name"><b>xionghaizi</b></a></span>
<font class='color_bonus'>雨滴 </font>[<a href="mybonus.php">使用</a>]: 18,890.1
<font class="color_ratio">分享率:</font> 1.201
<img class="arrowup" src="pic/trans.gif" />0
<img class="arrowdown" src="pic/trans.gif" />0
</td></tr></table>
<h1><span class="nowrap"><b>xionghaizi</b></span></h1>
<table>
  <tr><td class="rowhead">用户ID/UID</td><td class="rowfollow">20703</td></tr>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2025-10-13 11:36:19 (<span title="2025-10-13 11:36:19">4月15天前</span>)</td></tr>
  <tr><td class="rowhead">传输</td><td class="rowfollow"><table><tr><td class="embedded"><strong>上传量</strong>: 80.982 TB</td><td class="embedded"><strong>下载量</strong>: 0.00 KB</td></tr><tr><td class="embedded"><strong>实际上传量</strong>: 40.491 TB</td><td class="embedded"><strong>实际下载量</strong>: 12.65 GB</td></tr></table></td></tr>
  <tr><td class="rowhead">等级</td><td class="rowfollow"><img title="User" src="pic/user.gif" /></td></tr>
</table>
</body></html>`

const raingfhDetailFixture = `<html><body>
<h1 align="center"><input name="torrent_name" value="从零开始玩PT：从入门到精通" /><input name="detail_torrent_id" value="26432" /><font class="twoupfree">2X免费</font></h1>
<table>
  <tr><td class="rowhead">下载</td><td class="rowfollow"><a class="index" href="download.php?id=26432">download</a></td></tr>
  <tr><td class="rowhead">副标题</td><td class="rowfollow">LASTGL | PDF</td></tr>
  <tr><td class="rowhead">基本信息</td><td class="rowfollow"><b>大小：</b>2.26 MB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;other</td></tr>
</table>
</body></html>`

func getRaingfhDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("raingfh")
	require.True(t, ok)
	return def
}

func testRaingfhSearch(t *testing.T) {
	def := getRaingfhDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(raingfhSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "26432", items[0].ID)
	assert.Equal(t, v2.Discount2xFree, items[0].DiscountLevel)
	assert.Equal(t, 413, items[0].Seeders)
	assert.Equal(t, 0, items[0].Leechers)
	assert.Equal(t, 857, items[0].Snatched)
	assert.Equal(t, v2.DiscountFree, items[1].DiscountLevel)
}

func testRaingfhDetail(t *testing.T) {
	parser := v2.NewNexusPHPParserFromDefinition(getRaingfhDef(t))
	doc := FixtureDoc(t, "raingfh_detail", raingfhDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "26432", info.TorrentID)
	assert.Equal(t, "从零开始玩PT：从入门到精通", info.Title)
	assert.Equal(t, v2.Discount2xFree, info.DiscountLevel)
	assert.InDelta(t, 2.26, info.SizeMB, 0.1)
}

func testRaingfhUserInfo(t *testing.T) {
	def := getRaingfhDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "raingfh_index", raingfhIndexFixture)
	assert.Equal(t, "20703", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "xionghaizi", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "18890.1", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "raingfh_userdetails", raingfhUserdetailsFixture)
	assert.Equal(t, "89040650640556", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "1.201", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "18890.1", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "1760355379", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["joinTime"]))
}

func TestRaingfh_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      raingfhSearchFixture,
		"index":       raingfhIndexFixture,
		"userdetails": raingfhUserdetailsFixture,
		"detail":      raingfhDetailFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) { RequireNoSecrets(t, name, data) })
	}
}
