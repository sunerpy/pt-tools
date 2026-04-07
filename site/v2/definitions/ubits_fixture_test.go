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
		SiteID:   "ubits",
		Search:   testUBitsSearch,
		Detail:   testUBitsDetail,
		UserInfo: testUBitsUserInfo,
	})
}

const ubitsSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="电影(Movie)" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=317082"><b>Hamnet 2025 UHD BluRay 2160p REMUX DV HDR HEVC Atmos.TrueHD.7.1-UBits</b></a>
    <img class="pro_free2up" src="pic/trans.gif" alt="2X Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;twoupfree&quot;&gt;2X免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-03-21 15:13:45&quot;&gt;20时59分钟&lt;/span&gt;&lt;/b&gt;', 'trail', false)" />
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow nowrap"><span title="2026-03-20 15:13:45">3时<br/>0分钟</span></td>
  <td class="rowfollow">71.76<br/>GB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=317082&amp;dllist=1#seeders">32</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=317082&amp;dllist=1#leechers">22</a></b></td>
  <td class="rowfollow"><a href="viewsnatches.php?id=317082"><b>33</b></a></td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="电视剧(TV Series)" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=317117"><b>Her Blaze 2026 S01E08 2160p 60fps WEB-DL HEVC 10bit HDR Vivid</b></a>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow nowrap"><span title="2026-03-20 18:12:45">1分钟</span></td>
  <td class="rowfollow">405.71<br/>MB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=317117&amp;dllist=1#seeders">2</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=317117&amp;dllist=1#leechers">9</a></b></td>
  <td class="rowfollow"><a href="viewsnatches.php?id=317117"><b>1</b></a></td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table>
</body></html>`

const ubitsIndexFixture = `<html><body>
<div id="info_block">
  欢迎回来, <a href="https://ubits.club/userdetails.php?id=29325" class="Uploader_Name"><b>tester</b></a>
  <font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 34,335,472.0
  <font class="color_ratio">分享率:</font> 7.337
  <font class='color_uploaded'>上传量:</font> 101.908 TB
  <font class='color_downloaded'> 下载量:</font> 13.890 TB
  <font class='color_active'>当前活动:</font> <img class="arrowup" src="pic/trans.gif" />1490 <img class="arrowdown" src="pic/trans.gif" />0
</div>
</body></html>`

const ubitsUserdetailsFixture = `<html><body>
<table>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2025-01-27 21:01:09 (<span title="2025-01-27 21:01:09">59周</span>)</td></tr>
  <tr><td class="rowhead">传输</td><td class="rowfollow"><table><tr><td class="embedded"><strong>分享率</strong>: 7.337</td></tr><tr><td class="embedded"><strong>上传量</strong>: 101.908 TB</td><td class="embedded"><strong>下载量</strong>: 13.890 TB</td></tr></table></td></tr>
  <tr><td class="rowhead">等级</td><td class="rowfollow"><img title="Extreme User" src="pic/extreme.gif" /></td></tr>
</table>
</body></html>`

const ubitsDetailFixture = `<html><body>
<h1 align="center" id="top" value="Hamnet 2025 UHD BluRay 2160p REMUX DV HDR HEVC Atmos.TrueHD.7.1-UBits">
  <input name="torrent_name" type="hidden" value="Hamnet 2025 UHD BluRay 2160p REMUX DV HDR HEVC Atmos.TrueHD.7.1-UBits" />
  <input name="detail_torrent_id" type="hidden" value="317082" />
  <font class='twoupfree'>2X免费</font> <span title="2026-03-21 15:13:45">20时59分钟</span>
</h1>
<table>
  <tr><td class="rowhead">副标题</td><td class="rowfollow">哈姆奈特 4K UHD原盘 REMUX 简体/繁体/简英双语/繁英双语 保留杜比视界</td></tr>
  <tr><td class="rowhead">基本信息</td><td class="rowfollow">大小</td><td class="rowfollow">大小：71.76 GB 类型：电影</td></tr>
</table>
</body></html>`

const ubitsDetailWithHRFixture = `<html><body>
<h1 value="Hamnet 2025 UHD BluRay 2160p REMUX HR">
  <input name="torrent_name" type="hidden" value="Hamnet 2025 UHD BluRay 2160p REMUX HR" />
  <input name="detail_torrent_id" type="hidden" value="317083" />
</h1>
<table><tr><td class="rowhead">基本信息</td><td class="rowfollow">大小</td><td class="rowfollow">大小：10.00 GB 类型：电影</td></tr></table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

func getUBitsDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("ubits")
	require.True(t, ok)
	return def
}

func testUBitsSearch(t *testing.T) {
	def := getUBitsDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(ubitsSearchFixture))
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
	assert.Equal(t, "317082", first.ID)
	assert.Equal(t, "Hamnet 2025 UHD BluRay 2160p REMUX DV HDR HEVC Atmos.TrueHD.7.1-UBits", first.Title)
	assert.Equal(t, v2.Discount2xFree, first.DiscountLevel)
	assert.Equal(t, 32, first.Seeders)
	assert.Equal(t, 22, first.Leechers)
	assert.Equal(t, 33, first.Snatched)

	second := items[1]
	assert.Equal(t, "317117", second.ID)
	assert.Equal(t, v2.DiscountNone, second.DiscountLevel)
	assert.Equal(t, 2, second.Seeders)
	assert.Equal(t, 9, second.Leechers)
}

func testUBitsDetail(t *testing.T) {
	def := getUBitsDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "ubits_detail", ubitsDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "317082", info.TorrentID)
	assert.Equal(t, "Hamnet 2025 UHD BluRay 2160p REMUX DV HDR HEVC Atmos.TrueHD.7.1-UBits", info.Title)
	assert.Equal(t, v2.Discount2xFree, info.DiscountLevel)
	assert.InDelta(t, 71.76*1024, info.SizeMB, 0.1)

	hrDoc := FixtureDoc(t, "ubits_detail_hr", ubitsDetailWithHRFixture)
	hrInfo := parser.ParseAll(hrDoc.Selection)
	assert.Equal(t, "317083", hrInfo.TorrentID)
	assert.True(t, hrInfo.HasHR)
}

func testUBitsUserInfo(t *testing.T) {
	def := getUBitsDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "ubits_index", ubitsIndexFixture)
	assert.Equal(t, "29325", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "tester", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "112049030963396", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "15272216509808", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "7.337", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "3.4335472e+07", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "1490", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "ubits_userdetails", ubitsUserdetailsFixture)
	assert.Equal(t, "Extreme User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "1738011669", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["joinTime"]))
	assert.Equal(t, "112049030963396", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "15272216509808", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "7.337", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["ratio"]))
}

func TestUBits_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      ubitsSearchFixture,
		"index":       ubitsIndexFixture,
		"userdetails": ubitsUserdetailsFixture,
		"detail":      ubitsDetailFixture,
		"detail_hr":   ubitsDetailWithHRFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
