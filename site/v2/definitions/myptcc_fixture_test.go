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
		SiteID:   "myptcc",
		Search:   testMyptccSearch,
		Detail:   testMyptccDetail,
		UserInfo: testMyptccUserInfo,
	})
}

const myptccSearchFixture = `<html><body><table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="Movies/电影" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=27470"><b>Ying Han Hong Niang 2026 2160p WEB-DL DDP2.0 H265-HDSWEB</b></a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" title="免费" />
    <img class="hitandrun" src="pic/trans.gif" alt="H&R" title="H&R" />
    <br />硬汉红娘 | 主演: 张三 李四 | ARDTU
  </td></tr></table></td>
  <td class="rowfollow"><a href="comment.php">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-03-08 14:47:50">10分钟</span></td>
  <td class="rowfollow">2.21<br />GB</td>
  <td class="rowfollow" align="center"><b><a href="#seeders">1</a></b></td>
  <td class="rowfollow"><b><a href="#leechers">3</a></b></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="Movies/电影" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=27469"><b>Escape from the Outland 2025 2160p WEB-DL H265 10bit DTS5.1-UBWEB</b></a>
    <img class="pro_free2up" src="pic/trans.gif" alt="2X Free" title="2X免费" />
    <br />用武之地 | 导演: 申奥
  </td></tr></table></td>
  <td class="rowfollow"><a href="comment.php">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-03-08 14:16:14">41分钟</span></td>
  <td class="rowfollow">3.90<br />GB</td>
  <td class="rowfollow"><span class="red">0</span></td>
  <td class="rowfollow"><b><a href="#leechers">2</a></b></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table></body></html>`

const myptccIndexFixture = `<html><body>
<table id="info_block"><tr><td>
欢迎回来, <span class="nowrap"><a href="userdetails.php?id=11695" class="User_Name"><b>shmt86</b></a></span>
<font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 10,081.3
<font class="color_ratio">分享率:</font> 372.832
<font class='color_uploaded'>上传量:</font> 14.525 TB
<font class='color_downloaded'>下载量:</font> 39.89 GB
<img class="arrowup" src="pic/trans.gif" />33
<img class="arrowdown" src="pic/trans.gif" />0
</td></tr></table>
</body></html>`

const myptccUserdetailsFixture = `<html><body>
<table id="info_block"><tr><td>
欢迎回来, <span class="nowrap"><a href="userdetails.php?id=11695" class="User_Name"><b>shmt86</b></a></span>
<font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 10,081.3
<font class="color_ratio">分享率:</font> 372.832
<img class="arrowup" src="pic/trans.gif" />33
<img class="arrowdown" src="pic/trans.gif" />0
</td></tr></table>
<h1><span class="nowrap"><b>shmt86</b></span></h1>
<table>
  <tr><td class="rowhead">用户ID/UID</td><td class="rowfollow">11695</td></tr>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2025-06-21 22:29:23 (<span title="2025-06-21 22:29:23">8月19天前</span>)</td></tr>
  <tr><td class="rowhead">传输</td><td class="rowfollow"><table><tr><td class="embedded"><strong>分享率</strong>: <font>372.832</font></td></tr><tr><td class="embedded"><strong>上传量</strong>: 14.525 TB</td><td class="embedded"><strong>下载量</strong>: 39.89 GB</td></tr></table></td></tr>
  <tr><td class="rowhead">等级</td><td class="rowfollow"><img title="养老族" src="pic/retiree.gif" /></td></tr>
</table>
</body></html>`

const myptccDetailFixture = `<html><body>
<h1 align="center"><input name="torrent_name" value="Ying Han Hong Niang 2026 2160p WEB-DL DDP2.0 H265-HDSWEB" /><input name="detail_torrent_id" value="27470" /><font class="free">免费</font><img class="hitandrun" src="pic/trans.gif" alt="H&R" title="H&R" /></h1>
<table>
  <tr><td class="rowhead">下载</td><td class="rowfollow"><a class="index" href="download.php?id=27470">download</a></td></tr>
  <tr><td class="rowhead">副标题</td><td class="rowfollow">硬汉红娘 | 主演: 张三 李四 | ARDTU</td></tr>
  <tr><td class="rowhead">基本信息</td><td class="rowfollow"><b>大小：</b>2.21 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;Movies/电影</td></tr>
</table>
</body></html>`

func getMyptccDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("myptcc")
	require.True(t, ok)
	return def
}

func testMyptccSearch(t *testing.T) {
	def := getMyptccDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(myptccSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "27470", items[0].ID)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.True(t, items[0].HasHR)
	assert.Equal(t, 1, items[0].Seeders)
	assert.Equal(t, 3, items[0].Leechers)
	assert.Equal(t, v2.Discount2xFree, items[1].DiscountLevel)
}

func testMyptccDetail(t *testing.T) {
	parser := v2.NewNexusPHPParserFromDefinition(getMyptccDef(t))
	doc := FixtureDoc(t, "myptcc_detail", myptccDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "27470", info.TorrentID)
	assert.Equal(t, "Ying Han Hong Niang 2026 2160p WEB-DL DDP2.0 H265-HDSWEB", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.True(t, info.HasHR)
	assert.InDelta(t, 2.21*1024, info.SizeMB, 0.1)
}

func testMyptccUserInfo(t *testing.T) {
	def := getMyptccDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "myptcc_index", myptccIndexFixture)
	assert.Equal(t, "11695", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "shmt86", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "10081.3", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "33", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "myptcc_userdetails", myptccUserdetailsFixture)
	assert.Equal(t, "15970406393446", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "42831561359", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "372.832", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "养老族", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "10081.3", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "1750544963", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["joinTime"]))
}

func TestMyptcc_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      myptccSearchFixture,
		"index":       myptccIndexFixture,
		"userdetails": myptccUserdetailsFixture,
		"detail":      myptccDetailFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) { RequireNoSecrets(t, name, data) })
	}
}
