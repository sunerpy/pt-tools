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
		SiteID:   "audiences",
		Search:   testAudiencesSearch,
		Detail:   testAudiencesDetail,
		UserInfo: testAudiencesUserInfo,
	})
}

const audiencesSearchFixture = `<html><body><table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="电影" /></td>
  <td class="rowfollow torrents-box"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=630621"><b>I Am Waiting 1957 1080p BluRay AVC LPCM 2.0-Runrun@Audies</b></a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" />
    <br /><span style="padding:2px;line-height:20px;">我在等待 / Ore wa matteru ze</span>
  </td></tr></table></td>
  <td class="rowfollow"><a href="comment.php">1</a></td>
  <td class="rowfollow nowrap"><span title="2026-03-08 22:27:53">11时32分</span></td>
  <td class="rowfollow">40.80<br />GB</td>
  <td class="rowfollow" align="center"><b><a href="#seeders">133</a></b></td>
  <td class="rowfollow"><b><a href="#leechers">11</a></b></td>
  <td class="rowfollow"><a href="#snatches"><b>217</b></a></td>
  <td>-</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="剧集" /></td>
  <td class="rowfollow torrents-box"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=630607"><b>Evangelion Broadcast 30th Anniversary Special Screening 2026 1080p WEB-DL</b></a>
    <img class="pro_free2up" src="pic/trans.gif" alt="2X Free" />
    <br /><span style="padding:2px;line-height:20px;">新世纪福音战士 30周年新作动画</span>
  </td></tr></table></td>
  <td class="rowfollow"><a href="comment.php">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-03-08 21:31:07">12时29分</span></td>
  <td class="rowfollow">1.71<br />GB</td>
  <td class="rowfollow" align="center"><b><a href="#seeders">175</a></b></td>
  <td class="rowfollow"><b><a href="#leechers">1</a></b></td>
  <td class="rowfollow"><a href="#snatches"><b>335</b></a></td>
  <td>-</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table></body></html>`

const audiencesIndexFixture = `<html><body>
<table id="info_block"><tr><td>
欢迎回来, <span class="nowrap"><a href="userdetails.php?id=31147" class="PowerUser_Name"><b>cxlhm</b></a></span>
[<a href="mybonus.php">爆米花系统</a>] : 229,786.3
<span class="color_ratio">分享率：</span> 64.821
<span class='color_uploaded'>上传量：</span> 132.270 TB
<span class='color_downloaded'>下载量：</span> 2.041 TB
<a href="peerlist.php?userid=31147"><font class='color_active'>当前活动：</font></a>
<img class="arrowup" src="pic/trans.gif" />63
<img class="arrowdown" src="pic/trans.gif" />5
</td></tr></table>
</body></html>`

const audiencesUserdetailsFixture = `<html><body>
<table id="info_block"><tr><td>
欢迎回来, <span class="nowrap"><a href="userdetails.php?id=31147" class="PowerUser_Name"><b>cxlhm</b></a></span>
[<a href="mybonus.php">爆米花系统</a>] : 229,786.3
<span class="color_ratio">分享率：</span> 64.821
<img class="arrowup" src="pic/trans.gif" />63
<img class="arrowdown" src="pic/trans.gif" />5
</td></tr></table>
<h1><span class="nowrap"><a href="userdetails.php?id=31147" class="PowerUser_Name"><b>cxlhm</b></a></span></h1>
<table>
  <tr><td class="rowhead">UID</td><td class="rowfollow">31147</td></tr>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2025-10-02 23:34:11 (<span title="2025-10-02 23:34:11">5月7天前</span>)</td></tr>
  <tr><td class="rowhead">传输</td><td class="rowfollow"><table><tr><td class="embedded"><strong>分享率</strong>: <font>64.820</font></td></tr><tr><td class="embedded"><strong>上传量</strong>: 132.270 TB</td><td class="embedded"><strong>下载量</strong>: 2.041 TB</td></tr></table></td></tr>
  <tr><td class="rowhead">等级</td><td class="rowfollow"><img title="(年轻气盛)Power User" src="pic/power.gif" /></td></tr>
  <tr><td class="rowhead">爆米花</td><td class="rowfollow">229786.3</td></tr>
</table>
</body></html>`

const audiencesDetailFixture = `<html><body>
<h1 align="center"><input name="torrent_name" value="I Am Waiting 1957 1080p BluRay AVC LPCM 2.0-Runrun@Audies" /><input name="detail_torrent_id" value="630621" /><font class="free">免费</font> <span title="2026-03-10 10:27:53">1天0时</span></h1>
<table>
  <tr><td class="rowhead">下载</td><td class="rowfollow"><a class="index" href="download.php?id=630621">download</a></td></tr>
  <tr><td class="rowhead">副标题</td><td class="rowfollow">我在等待 / Ore wa matteru ze</td></tr>
  <tr><td class="rowhead">基本信息</td><td class="rowfollow"><b>大小：</b>40.80 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;电影</td></tr>
</table>
</body></html>`

func getAudiencesDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("audiences")
	require.True(t, ok)
	return def
}

func testAudiencesSearch(t *testing.T) {
	def := getAudiencesDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(audiencesSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "630621", items[0].ID)
	assert.Equal(t, "I Am Waiting 1957 1080p BluRay AVC LPCM 2.0-Runrun@Audies", items[0].Title)
	assert.Equal(t, "我在等待 / Ore wa matteru ze", items[0].Subtitle)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 133, items[0].Seeders)
	assert.Equal(t, 11, items[0].Leechers)
	assert.Equal(t, 217, items[0].Snatched)
	assert.Equal(t, v2.Discount2xFree, items[1].DiscountLevel)
}

func testAudiencesDetail(t *testing.T) {
	parser := v2.NewNexusPHPParserFromDefinition(getAudiencesDef(t))
	doc := FixtureDoc(t, "audiences_detail", audiencesDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "630621", info.TorrentID)
	assert.Equal(t, "I Am Waiting 1957 1080p BluRay AVC LPCM 2.0-Runrun@Audies", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 40.80*1024, info.SizeMB, 0.1)
}

func testAudiencesUserInfo(t *testing.T) {
	def := getAudiencesDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "audiences_index", audiencesIndexFixture)
	assert.Equal(t, "31147", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "cxlhm", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "229786.3", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "63", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "5", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "audiences_userdetails", audiencesUserdetailsFixture)
	assert.Equal(t, "145432403005931", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "2244103232290", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "64.82", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "(年轻气盛)Power User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "229786.3", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "1759448051", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["joinTime"]))
}

func TestAudiences_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      audiencesSearchFixture,
		"index":       audiencesIndexFixture,
		"userdetails": audiencesUserdetailsFixture,
		"detail":      audiencesDetailFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) { RequireNoSecrets(t, name, data) })
	}
}
