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
	RegisterFixtureSuite(FixtureSuite{SiteID: "hdvideo", Search: testHDVideoSearch, Detail: testHDVideoDetail, UserInfo: testHDVideoUserInfo})
}

const hdvideoSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="电影/Movies" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=65743">Lang.Lang.Shan.Demo.2025.1080p-HDV</a>
    <img class="pro_free2up" src="pic/trans.gif" alt="2X Free" />
    <br/><span>演示标题 / Demo</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-04-26 04:58:19">18天3时</span></td>
  <td class="rowfollow">30.96 GB</td>
  <td class="rowfollow">79</td>
  <td class="rowfollow">7</td>
  <td class="rowfollow">589</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="纪录片/Documentary" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=65745">HDVideo.Demo2.Doc-DEMO</a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" />
    <br/><span>演示标题2 / Demo2</span>
  </td></tr></table></td>
  <td class="rowfollow">3</td>
  <td class="rowfollow"><span title="2026-05-12 12:00:00">1天</span></td>
  <td class="rowfollow">8.21 GB</td>
  <td class="rowfollow">42</td>
  <td class="rowfollow">1</td>
  <td class="rowfollow">128</td>
  <td class="rowfollow">DocTeam</td>
</tr>
</tbody></table>
</body></html>`

const hdvideoIndexFixture = `<html><body>
<table id="info_block"><tr><td>
欢迎回来, <span class="nowrap"><a href="https://hdvideo.top/userdetails.php?id=25339" class='User_Name'><b>hdvideo_user</b></a></span>
<font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 9,876.0
<font class="color_ratio">分享率:</font> 无限                <font class='color_uploaded'>上传量:</font> 10.00 GB                <font class='color_downloaded'> 下载量:</font> 0.00 KB                <font class='color_active'>当前活动:</font> <img class="arrowup" src="pic/trans.gif" />0  <img class="arrowdown" src="pic/trans.gif" />0
</td></tr></table>
</body></html>`

const hdvideoUserdetailsFixture = `<html><body>
<h1 style='margin:0px'><span class="nowrap"><b>hdvideo_user</b></span></h1>
<table>
  <tr><td class="rowhead">用户ID/UID</td><td class="rowfollow">25339</td></tr>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2025-12-01 09:00:00 (<span title="2025-12-01 09:00:00">5月前</span>)</td></tr>
  <tr><td class="rowhead">等级</td><td class="rowfollow"><img title="Veteran User" src="pic/veteran.gif" /></td></tr>
</table>
</body></html>`

const hdvideoDetailFixture = `<html><body>
<input type="hidden" name="torrent_name" value="Lang.Lang.Shan.Demo.2025.1080p-HDV" />
<input type="hidden" name="detail_torrent_id" value="65743" />
<h1 align="center" id="top">Lang.Lang.Shan.Demo.2025.1080p-HDV&nbsp;&nbsp;&nbsp; <b>[<font class='twoupfree'>2X免费</font>]</b> <font color='#00CC66'>剩余时间：<span title="2026-05-14 07:58:19">2天16时</span></font></h1>
<table>
  <tr><td class="rowhead">下载</td><td class="rowfollow"><a href="download.php?id=65743">demo.torrent</a></td></tr>
  <tr><td class="rowhead">副标题</td><td class="rowfollow">演示标题 / Demo</td></tr>
  <tr><td class="rowhead">基本信息</td><td class="rowfollow">大小</td><td class="rowfollow">大小：30.96 GB</td></tr>
</table>
</body></html>`

func getHDVideoDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("hdvideo")
	require.True(t, ok)
	return def
}

func testHDVideoSearch(t *testing.T) {
	def := getHDVideoDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(hdvideoSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: http.MethodGet})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "65743", items[0].ID)
	assert.Equal(t, "Lang.Lang.Shan.Demo.2025.1080p-HDV", items[0].Title)
	assert.Equal(t, "演示标题 / Demo", items[0].Subtitle)
	assert.Equal(t, v2.Discount2xFree, items[0].DiscountLevel)
	assert.Equal(t, 79, items[0].Seeders)
	assert.Equal(t, 7, items[0].Leechers)
	assert.Equal(t, 589, items[0].Snatched)

	assert.Equal(t, "65745", items[1].ID)
	assert.Equal(t, v2.DiscountFree, items[1].DiscountLevel)
}

func testHDVideoDetail(t *testing.T) {
	def := getHDVideoDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "hdvideo_detail", hdvideoDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "65743", info.TorrentID)
	assert.Equal(t, "Lang.Lang.Shan.Demo.2025.1080p-HDV", info.Title)
	assert.Equal(t, v2.Discount2xFree, info.DiscountLevel)
	assert.InDelta(t, 31703.04, info.SizeMB, 1.0)
}

func testHDVideoUserInfo(t *testing.T) {
	def := getHDVideoDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "hdvideo_index", hdvideoIndexFixture)
	assert.Equal(t, "25339", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "hdvideo_user", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "9876", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "10737418240", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["downloaded"]))

	userDoc := FixtureDoc(t, "hdvideo_userdetails", hdvideoUserdetailsFixture)
	assert.Equal(t, "Veteran User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "1764579600", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["joinTime"]))
}

func TestHDVideo_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      hdvideoSearchFixture,
		"index":       hdvideoIndexFixture,
		"userdetails": hdvideoUserdetailsFixture,
		"detail":      hdvideoDetailFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) { RequireNoSecrets(t, name, data) })
	}
}
