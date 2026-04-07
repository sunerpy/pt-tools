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
		SiteID:   "hdhome",
		Search:   testHDHomeSearch,
		Detail:   testHDHomeDetail,
		UserInfo: testHDHomeUserInfo,
	})
}

const hdhomeSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr class="sticky_top">
  <td class="rowfollow"><img alt="Movies Bluray" /></td>
  <td class="rowfollow"><table class="torrentname"><tr class="sticky_top"><td class="embedded">
    <a href="details.php?id=282983"><b>Merrily We Roll Along 2025 1080p Blu-ray AVC DTS-HD MA5.1-DiY@HDHome</b></a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-03-12 13:08:18&quot;&gt;20时50分&lt;/span&gt;&lt;/b&gt;', 'trail', false)" />
    <br /><span class='tags tgf'>官方</span><span class='tags tyc'>原创</span><span class='tags tzz'>中字</span><span class='tags tgz'>官字</span><span class='tags tdiy'>DIY</span><span style='float:left;padding: 2px;line-height: 20px;'>欢乐岁月 | DiY官译简繁字幕</span>
  </td><td class="embedded"><a href="download.php?id=282983"><img class="download" src="pic/trans.gif" alt="download" /></a></td></tr><tr><td class="embedded rss"><a data-toggle-rss="282983"><img src="pic/rss.png" alt="RSS" /></a></td></tr></table></td>
  <td class="rowfollow"><a href="comment.php?action=add&amp;pid=282983&amp;type=torrent">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-03-11 13:08:18">3时<br/>9分</span></td>
  <td class="rowfollow">39.21<br/>GB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=282983&amp;dllist=1#seeders">1</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=282983&amp;dllist=1#leechers">91</a></b></td>
  <td class="rowfollow">0</td>
  <td align="center">-</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="Movies UHD Blu-ray" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=282932"><b>Another UHD Movie 2025 2160p Blu-ray HEVC</b></a>
    <br /><span class='tags tgf'>官方</span><span style='float:left;padding: 2px;line-height: 20px;'>测试副标题</span>
  </td><td class="embedded"><a href="download.php?id=282932"><img class="download" src="pic/trans.gif" alt="download" /></a></td></tr><tr><td class="embedded rss"><a data-toggle-rss="282932"><img src="pic/rss.png" alt="RSS" /></a></td></tr></table></td>
  <td class="rowfollow"><a href="comment.php?action=add&amp;pid=282932&amp;type=torrent">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-03-10 20:56:09">19时<br/>21分</span></td>
  <td class="rowfollow">76.75<br/>GB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=282932&amp;dllist=1#seeders">220</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=282932&amp;dllist=1#leechers">22</a></b></td>
  <td class="rowfollow"><a href="viewsnatches.php?id=282932"><b>341</b></a></td>
  <td align="center">-</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table>
</body></html>`

const hdhomeIndexFixture = `<html><body>
<table id="info_block"><tr><td>
  欢迎回来, <a href="userdetails.php?id=91176" class='ExtremeUser_Name'><b>nestzhong</b></a>
  <font class="color_bonus">魔力值 </font>[<a href="mybonus.php">使用</a>]: 2,078,769.3
  <font class="color_ratio">分享率：</font> 7.316
  <font class='color_uploaded'>上传量：</font> 20.167 TB
  <font class='color_downloaded'> 下载量：</font> 2.756 TB
  <font class='color_active'>当前活动：</font><img class="arrowup" src="pic/trans.gif" />30 <img class="arrowdown" src="pic/trans.gif" />0
</td></tr></table>
</body></html>`

const hdhomeUserdetailsFixture = `<html><body>
<table>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2023-09-01 08:00:00 (<span title="2023-09-01 08:00:00">2年前</span>)</td></tr>
  <tr><td class="rowhead">等级</td><td class="rowfollow"><img title="Extreme User" src="pic/extreme.gif" /></td></tr>
</table>
</body></html>`

const hdhomeDetailFixture = `<html><body>
<h1 align="center" id="top" value="Merrily We Roll Along 2025 1080p Blu-ray AVC DTS-HD MA5.1-DiY@HDHome">
  <input name="torrent_name" type="hidden" value="Merrily We Roll Along 2025 1080p Blu-ray AVC DTS-HD MA5.1-DiY@HDHome" />
  <input name="detail_torrent_id" type="hidden" value="282983" />
  <font class='free'>免费</font> <span title="2026-03-12 13:08:18">20时50分</span>
</h1>
<table>
  <tr><td class="rowhead">副标题</td><td class="rowfollow">欢乐岁月 | DiY官译简繁字幕</td></tr>
  <tr><td class="rowhead">基本信息</td><td class="rowfollow">大小</td><td class="rowfollow">大小：39.21 GB 类型：Movies Bluray</td></tr>
</table>
</body></html>`

const hdhomeDetailWithHRFixture = `<html><body>
<h1 value="Merrily We Roll Along 2025 1080p Blu-ray HR">
  <input name="torrent_name" type="hidden" value="Merrily We Roll Along 2025 1080p Blu-ray HR" />
  <input name="detail_torrent_id" type="hidden" value="282984" />
</h1>
<table><tr><td class="rowhead">基本信息</td><td class="rowfollow">大小</td><td class="rowfollow">大小：15.00 GB 类型：Movies Bluray</td></tr></table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

func getHDHomeDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("hdhome")
	require.True(t, ok)
	return def
}

func testHDHomeSearch(t *testing.T) {
	def := getHDHomeDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(hdhomeSearchFixture))
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
	assert.Equal(t, "282983", first.ID)
	assert.Equal(t, "Merrily We Roll Along 2025 1080p Blu-ray AVC DTS-HD MA5.1-DiY@HDHome", first.Title)
	assert.Equal(t, "欢乐岁月 | DiY官译简繁字幕", first.Subtitle)
	assert.Equal(t, v2.DiscountFree, first.DiscountLevel)
	assert.Equal(t, 1, first.Seeders)
	assert.Equal(t, 91, first.Leechers)
	assert.Equal(t, 0, first.Snatched)

	second := items[1]
	assert.Equal(t, "282932", second.ID)
	assert.Equal(t, "测试副标题", second.Subtitle)
	assert.Equal(t, v2.DiscountNone, second.DiscountLevel)
}

func testHDHomeDetail(t *testing.T) {
	def := getHDHomeDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "hdhome_detail", hdhomeDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "282983", info.TorrentID)
	assert.Equal(t, "Merrily We Roll Along 2025 1080p Blu-ray AVC DTS-HD MA5.1-DiY@HDHome", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 39.21*1024, info.SizeMB, 0.1)

	hrDoc := FixtureDoc(t, "hdhome_detail_hr", hdhomeDetailWithHRFixture)
	hrInfo := parser.ParseAll(hrDoc.Selection)
	assert.Equal(t, "282984", hrInfo.TorrentID)
	assert.True(t, hrInfo.HasHR)
}

func testHDHomeUserInfo(t *testing.T) {
	def := getHDHomeDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "hdhome_index", hdhomeIndexFixture)
	assert.Equal(t, "91176", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "nestzhong", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "22173850997358", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "3030254046150", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "7.316", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "2.0787693e+06", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "30", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "hdhome_userdetails", hdhomeUserdetailsFixture)
	assert.Equal(t, "Extreme User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "1693555200", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["joinTime"]))
}

func TestHDHome_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      hdhomeSearchFixture,
		"index":       hdhomeIndexFixture,
		"userdetails": hdhomeUserdetailsFixture,
		"detail":      hdhomeDetailFixture,
		"detail_hr":   hdhomeDetailWithHRFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
