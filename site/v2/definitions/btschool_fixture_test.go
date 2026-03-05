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
		SiteID:   "btschool",
		Search:   testBTSchoolSearch,
		Detail:   testBTSchoolDetail,
		UserInfo: testBTSchoolUserInfo,
	})
}

const btschoolSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="电影/Movies" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=260153">First.Man.2018.BluRay.1080p.BTSCHOOL</a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this,event,'content','&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-03-05 22:19:28&quot;&gt;3天1时&lt;/span&gt;&lt;/b&gt;','trail',false)" />
    <br /><span>登月第一人</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-02-28 22:19:28">1天22时</span></td>
  <td class="rowfollow">34.32 GB</td>
  <td class="rowfollow">149</td>
  <td class="rowfollow">4</td>
  <td class="rowfollow">155</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="电影/Movies" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=258217">Sisu.Road.to.Revenge.2025.Remux</a>
    <br /><span>永生战士2</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-02-22 08:14:46">8天12时</span></td>
  <td class="rowfollow">22.50 GB</td>
  <td class="rowfollow">141</td>
  <td class="rowfollow">2</td>
  <td class="rowfollow">244</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table>
</body></html>`

const btschoolDetailFixture = `<html><body>
<h1>
  <input name="torrent_name" type="hidden" value="First.Man.2018.BluRay.1080p.BTSCHOOL" />
  <input name="detail_torrent_id" type="hidden" value="260153" />
  <font class="free">免费</font>
  <span title="2026-03-05 22:19:28">3天1时</span>
</h1>
<table>
  <tr>
    <td class="rowhead">基本信息</td>
    <td class="rowfollow">大小：34.32 GB | 类型：电影/Movies</td>
  </tr>
</table>
</body></html>`

const btschoolDetailHRFixture = `<html><body>
<h1>
  <input name="torrent_name" type="hidden" value="BTSchool.HR.Test" />
  <input name="detail_torrent_id" type="hidden" value="260154" />
</h1>
<table><tr><td class="rowhead">基本信息</td><td class="rowfollow">大小：8.50 GB | 类型：电影</td></tr></table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

const btschoolIndexFixture = `<html><body>
<table id="info_block"><tr><td>
欢迎回来, <a href="userdetails.php?id=93012" class="User_Name"><b>tmdbs</b></a>
<font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 12724.2
<font class='color_active'>当前活动：</font>
<img class="arrowup" src="pic/trans.gif" />42
<img class="arrowdown" src="pic/trans.gif" />1
</td></tr></table>
</body></html>`

const btschoolUserdetailsFixture = `<html><body>
<table>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2020-01-01 00:00:00 (<span title="2020-01-01 00:00:00">很久前</span>)</td></tr>
  <tr><td class="rowhead">传输</td><td class="rowfollow">
    <table><tr><td class="embedded"><strong>分享率</strong>: <font>20.289</font></td></tr>
    <tr><td class="embedded"><strong>上传量</strong>: 2.00 TB</td><td class="embedded"><strong>下载量</strong>: 100.00 GB</td><td class="embedded"><strong>做种积分</strong>: 5514.5</td></tr></table>
  </td></tr>
  <tr><td class="rowhead">等级</td><td class="rowfollow"><img title="User" src="pic/user.gif" /></td></tr>
</table>
</body></html>`

func getBTSchoolDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("btschool")
	require.True(t, ok, "btschool definition not found")
	return def
}

func testBTSchoolSearch(t *testing.T) {
	def := getBTSchoolDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(btschoolSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "260153", items[0].ID)
	assert.Equal(t, "First.Man.2018.BluRay.1080p.BTSCHOOL", items[0].Title)
	assert.Equal(t, "登月第一人", items[0].Subtitle)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 149, items[0].Seeders)
	assert.Equal(t, 4, items[0].Leechers)
	assert.Equal(t, 155, items[0].Snatched)

	assert.Equal(t, "258217", items[1].ID)
	assert.Equal(t, v2.DiscountNone, items[1].DiscountLevel)
}

func testBTSchoolDetail(t *testing.T) {
	def := getBTSchoolDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	t.Run("Free", func(t *testing.T) {
		doc := FixtureDoc(t, "btschool_detail", btschoolDetailFixture)
		info := parser.ParseAll(doc.Selection)
		assert.Equal(t, "260153", info.TorrentID)
		assert.Equal(t, "First.Man.2018.BluRay.1080p.BTSCHOOL", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		assert.InDelta(t, 34.32*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})

	t.Run("HR", func(t *testing.T) {
		doc := FixtureDoc(t, "btschool_detail_hr", btschoolDetailHRFixture)
		info := parser.ParseAll(doc.Selection)
		assert.Equal(t, "260154", info.TorrentID)
		assert.True(t, info.HasHR)
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel)
	})
}

func testBTSchoolUserInfo(t *testing.T) {
	def := getBTSchoolDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "btschool_index", btschoolIndexFixture)
		fields := map[string]string{
			"id":       "93012",
			"name":     "tmdbs",
			"bonus":    "12724.2",
			"seeding":  "42",
			"leeching": "1",
		}
		for field, expected := range fields {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok)
			assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel))
		}
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "btschool_userdetails", btschoolUserdetailsFixture)
		expected := map[string]string{
			"uploaded":   "2199023255552",
			"downloaded": "107374182400",
			"ratio":      "20.289",
			"levelName":  "User",
			"joinTime":   "1577836800",
		}
		for field, want := range expected {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok)
			assert.Equal(t, want, driver.ExtractFieldValuePublic(doc, sel))
		}
	})
}

func TestBTSchool_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      btschoolSearchFixture,
		"detail":      btschoolDetailFixture,
		"detail_hr":   btschoolDetailHRFixture,
		"index":       btschoolIndexFixture,
		"userdetails": btschoolUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
