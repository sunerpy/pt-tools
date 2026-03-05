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
		SiteID:   "1ptba",
		Search:   testOnePTBASearch,
		Detail:   testOnePTBADetail,
		UserInfo: testOnePTBAUserInfo,
	})
}

const oneptbaSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="Movie(電影)" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=69978">Weapons.Sword.and.Shadow.2026.2160p</a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this,event,'content','&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-03-07 12:18:23&quot;&gt;2天23时&lt;/span&gt;&lt;/b&gt;','trail',false)" />
    <br /><span>武器之刀光剑影</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-03-04 12:18:23">55分钟</span></td>
  <td class="rowfollow">3.73 GB</td>
  <td class="rowfollow">1</td>
  <td class="rowfollow">15</td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="Movie(電影)" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=69977">Weapons.Sword.and.Shadow.2026.2160p.HDRVivid</a>
    <br /><span>武器之刀光剑影 HDR</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-03-04 11:59:22">1时14分钟</span></td>
  <td class="rowfollow">6.91 GB</td>
  <td class="rowfollow">1</td>
  <td class="rowfollow">20</td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table>
</body></html>`

const oneptbaDetailFixture = `<html><body>
<h1>
  <input name="torrent_name" type="hidden" value="Weapons.Sword.and.Shadow.2026.2160p" />
  <input name="detail_torrent_id" type="hidden" value="69978" />
  <font class="free">免费</font>
  <span title="2026-03-07 12:18:23">2天23时</span>
</h1>
<table><tr><td class="rowhead">基本信息</td><td class="rowfollow">大小：3.73 GB | 类型：Movie</td></tr></table>
</body></html>`

const oneptbaDetailHRFixture = `<html><body>
<h1>
  <input name="torrent_name" type="hidden" value="OnePTBA.HR.Test" />
  <input name="detail_torrent_id" type="hidden" value="69979" />
</h1>
<table><tr><td class="rowhead">基本信息</td><td class="rowfollow">大小：9.10 GB | 类型：Movie</td></tr></table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

const oneptbaIndexFixture = `<html><body>
<table id="info_block"><tr><td>
欢迎回来, <a href="https://1ptba.com/userdetails.php?id=118296" class="PowerUser_Name"><b>Jccc0201</b></a>
<font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 12724.2
<img class="arrowup" src="pic/trans.gif" />42
<img class="arrowdown" src="pic/trans.gif" />1
</td></tr></table>
</body></html>`

const oneptbaUserdetailsFixture = `<html><body>
<table>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2020-01-01 00:00:00 (<span title="2020-01-01 00:00:00">久以前</span>)</td></tr>
  <tr><td class="rowhead">传输</td><td class="rowfollow">
    <table><tr><td class="embedded"><strong>分享率</strong>: <font>2.382</font></td></tr>
    <tr><td class="embedded"><strong>上传量</strong>: 2.00 TB</td><td class="embedded"><strong>下载量</strong>: 100.00 GB</td><td class="embedded"><strong>做种积分</strong>: 4000</td></tr></table>
  </td></tr>
  <tr><td class="rowhead">等级</td><td class="rowfollow"><img title="Power User" src="pic/power.gif" /></td></tr>
</table>
</body></html>`

func getOnePTBADef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("1ptba")
	require.True(t, ok, "1ptba definition not found")
	return def
}

func testOnePTBASearch(t *testing.T) {
	def := getOnePTBADef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(oneptbaSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "69978", items[0].ID)
	assert.Equal(t, "Weapons.Sword.and.Shadow.2026.2160p", items[0].Title)
	assert.Equal(t, "武器之刀光剑影", items[0].Subtitle)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 1, items[0].Seeders)
	assert.Equal(t, 15, items[0].Leechers)
	assert.Equal(t, 0, items[0].Snatched)

	assert.Equal(t, "69977", items[1].ID)
	assert.Equal(t, v2.DiscountNone, items[1].DiscountLevel)
}

func testOnePTBADetail(t *testing.T) {
	def := getOnePTBADef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	t.Run("Free", func(t *testing.T) {
		doc := FixtureDoc(t, "oneptba_detail", oneptbaDetailFixture)
		info := parser.ParseAll(doc.Selection)
		assert.Equal(t, "69978", info.TorrentID)
		assert.Equal(t, "Weapons.Sword.and.Shadow.2026.2160p", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		assert.InDelta(t, 3.73*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})

	t.Run("HR", func(t *testing.T) {
		doc := FixtureDoc(t, "oneptba_detail_hr", oneptbaDetailHRFixture)
		info := parser.ParseAll(doc.Selection)
		assert.Equal(t, "69979", info.TorrentID)
		assert.True(t, info.HasHR)
	})
}

func testOnePTBAUserInfo(t *testing.T) {
	def := getOnePTBADef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "oneptba_index", oneptbaIndexFixture)
		expected := map[string]string{
			"id":       "118296",
			"name":     "Jccc0201",
			"bonus":    "12724.2",
			"seeding":  "42",
			"leeching": "1",
		}
		for field, want := range expected {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok)
			assert.Equal(t, want, driver.ExtractFieldValuePublic(doc, sel))
		}
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "oneptba_userdetails", oneptbaUserdetailsFixture)
		expected := map[string]string{
			"uploaded":   "2199023255552",
			"downloaded": "107374182400",
			"ratio":      "2.382",
			"levelName":  "Power User",
			"joinTime":   "1577836800",
		}
		for field, want := range expected {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok)
			assert.Equal(t, want, driver.ExtractFieldValuePublic(doc, sel))
		}
	})
}

func TestOnePTBA_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      oneptbaSearchFixture,
		"detail":      oneptbaDetailFixture,
		"detail_hr":   oneptbaDetailHRFixture,
		"index":       oneptbaIndexFixture,
		"userdetails": oneptbaUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
