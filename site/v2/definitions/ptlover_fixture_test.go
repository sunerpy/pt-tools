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
		SiteID:   "ptlover",
		Search:   testPtloveSearch,
		Detail:   testPtloveDetail,
		UserInfo: testPtloveUserInfo,
	})
}

const ptloverSearchFixture = `<html><body><table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="电视剧" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=1162"><b>Marvels.Iron.Fist.S01.2160p.NF.WEB-DL.x265.10bit.HDR.DDP5.1-ABBiE</b></a>
    <img class="pro_free2up" src="pic/trans.gif" alt="2X Free" title="2X免费" />
    <br /><span style="background-color:#006400;color:#ffffff">中字</span>铁拳 第一季 / Marvel's Iron Fist 4K HDR
  </td></tr></table></td>
  <td class="rowfollow"><a href="comment.php">0</a></td>
  <td class="rowfollow nowrap"><span title="2025-02-04 15:00:56">1年1月</span></td>
  <td class="rowfollow">82.69<br />GB</td>
  <td class="rowfollow" align="center"><b><a href="#seeders">5</a></b></td>
  <td class="rowfollow"><b><a href="#leechers">3</a></b></td>
  <td class="rowfollow"><a href="#snatches"><b>17</b></a></td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="电影" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=991"><b>Burning Stars 2024 2160p 60fps WEB-DL HEVC AV3A5.1 8Audios-QHstudIo</b></a>
    <img class="pro_free2up" src="pic/trans.gif" alt="2X Free" title="2X免费" />
    <br /><span style="background-color:#006400;color:#ffffff">中字</span>孤星计划
  </td></tr></table></td>
  <td class="rowfollow"><a href="comment.php">0</a></td>
  <td class="rowfollow nowrap"><span title="2025-01-29 17:56:06">1年1月</span></td>
  <td class="rowfollow">3.67<br />GB</td>
  <td class="rowfollow" align="center"><b><a href="#seeders">6</a></b></td>
  <td class="rowfollow"><b><a href="#leechers">1</a></b></td>
  <td class="rowfollow"><a href="#snatches"><b>37</b></a></td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table></body></html>`

const ptloverIndexFixture = `<html><body>
<table id="info_block"><tr><td>
欢迎回来, <span class="nowrap"><a href="userdetails.php?id=21458" class="User_Name"><b>qinmina</b></a></span>
<font class='color_bonus'>喵饼 </font>[<a href="mybonus.php">使用</a>]: 10.0
<font class="color_ratio">分享率:</font> ---
<font class='color_uploaded'>上传量:</font> 0.00 KB
<font class='color_downloaded'> 下载量:</font> 0.00 KB
<img class="arrowup" src="pic/trans.gif" />0
<img class="arrowdown" src="pic/trans.gif" />0
</td></tr></table>
</body></html>`

const ptloverUserdetailsFixture = `<html><body>
<table id="info_block"><tr><td>
欢迎回来, <span class="nowrap"><a href="userdetails.php?id=21458" class="User_Name"><b>qinmina</b></a></span>
<font class='color_bonus'>喵饼 </font>[<a href="mybonus.php">使用</a>]: 10.0
<font class="color_ratio">分享率:</font> ---
<img class="arrowup" src="pic/trans.gif" />0
<img class="arrowdown" src="pic/trans.gif" />0
</td></tr></table>
<h1><span class="nowrap"><b>qinmina</b></span></h1>
<table>
  <tr><td class="rowhead">用户ID/UID</td><td class="rowfollow">21458</td></tr>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2026-03-07 18:24:32 (<span title="2026-03-07 18:24:32">4时11分钟前</span>)</td></tr>
  <tr><td class="rowhead">传输</td><td class="rowfollow"><table><tr><td class="embedded"><strong>上传量</strong>: 0.00 KB</td><td class="embedded"><strong>下载量</strong>: 0.00 KB</td></tr><tr><td class="embedded"><strong>实际上传量</strong>: 0.00 KB</td><td class="embedded"><strong>实际下载量</strong>: 0.00 KB</td></tr></table></td></tr>
  <tr><td class="rowhead">等级</td><td class="rowfollow"><img title="User" src="pic/user.gif" /></td></tr>
  <tr><td class="rowhead">喵饼</td><td class="rowfollow">10.0</td></tr>
</table>
</body></html>`

const ptloverDetailFixture = `<html><body>
<h1 align="center"><input name="torrent_name" value="Marvels.Iron.Fist.S01.2160p.NF.WEB-DL.x265.10bit.HDR.DDP5.1-ABBiE" /><input name="detail_torrent_id" value="1162" /><font class="twoupfree">2X免费</font></h1>
<table>
  <tr><td class="rowhead">下载</td><td class="rowfollow"><a class="index" href="download.php?id=1162">download</a></td></tr>
  <tr><td class="rowhead">副标题</td><td class="rowfollow">铁拳 第一季 / Marvel's Iron Fist 4K HDR</td></tr>
  <tr><td class="rowhead">基本信息</td><td class="rowfollow"><b>大小：</b>82.69 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;电视剧</td></tr>
</table>
</body></html>`

func getPtloveDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("ptlover")
	require.True(t, ok)
	return def
}

func testPtloveSearch(t *testing.T) {
	def := getPtloveDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(ptloverSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "1162", items[0].ID)
	assert.Equal(t, v2.Discount2xFree, items[0].DiscountLevel)
	assert.Equal(t, 5, items[0].Seeders)
	assert.Equal(t, 3, items[0].Leechers)
	assert.Equal(t, 17, items[0].Snatched)
}

func testPtloveDetail(t *testing.T) {
	parser := v2.NewNexusPHPParserFromDefinition(getPtloveDef(t))
	doc := FixtureDoc(t, "ptlover_detail", ptloverDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "1162", info.TorrentID)
	assert.Equal(t, "Marvels.Iron.Fist.S01.2160p.NF.WEB-DL.x265.10bit.HDR.DDP5.1-ABBiE", info.Title)
	assert.Equal(t, v2.Discount2xFree, info.DiscountLevel)
	assert.InDelta(t, 82.69*1024, info.SizeMB, 0.1)
}

func testPtloveUserInfo(t *testing.T) {
	def := getPtloveDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "ptlover_index", ptloverIndexFixture)
	assert.Equal(t, "21458", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "qinmina", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "ptlover_userdetails", ptloverUserdetailsFixture)
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "10", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "1772907872", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["joinTime"]))
}

func TestPtlover_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      ptloverSearchFixture,
		"index":       ptloverIndexFixture,
		"userdetails": ptloverUserdetailsFixture,
		"detail":      ptloverDetailFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) { RequireNoSecrets(t, name, data) })
	}
}
