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
		SiteID:   "novahd",
		Search:   testNovaHDSearch,
		Detail:   testNovaHDDetail,
		UserInfo: testNovaHDUserInfo,
	})
}

// --- Fixtures ---

const novahdSearchFixture = `<html><body>
<table class="torrents">
<tbody>
<tr>
	<td class="rowfollow"><img alt="Movie" /></td>
	<td class="rowfollow">
		<table class="torrentname"><tr><td class="embedded">
			<a href="details.php?id=20001">NovaHD.Test.Movie.2025.BluRay.1080p</a>
			<img class="pro_free" src="pic/trans.gif" alt="Free"
				onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;优惠剩余时间：&lt;b&gt;&lt;span title=&quot;2026-03-01 12:00:00&quot;&gt;29天23时&lt;/span&gt;&lt;/b&gt;', 'trail', false)" />
			<br/><span>测试电影 / Test Movie / 2025</span>
		</td></tr></table>
	</td>
	<td class="rowfollow"></td>
	<td class="rowfollow"><span title="2025-01-15 08:30:00">1天前</span></td>
	<td class="rowfollow">42.5 GB</td>
	<td class="rowfollow">150</td>
	<td class="rowfollow">10</td>
	<td class="rowfollow">500</td>
</tr>
<tr>
	<td class="rowfollow"><img alt="TV" /></td>
	<td class="rowfollow">
		<table class="torrentname"><tr><td class="embedded">
			<a href="details.php?id=20002">NovaHD.Test.Show.S01E01.WEB-DL.1080p</a>
			<br/><span>测试剧集</span>
		</td></tr></table>
	</td>
	<td class="rowfollow"></td>
	<td class="rowfollow"><span title="2025-01-14 20:00:00">2天前</span></td>
	<td class="rowfollow">2.0 GB</td>
	<td class="rowfollow">50</td>
	<td class="rowfollow">5</td>
	<td class="rowfollow">200</td>
</tr>
</tbody>
</table>
</body></html>`

const novahdIndexFixture = `<html><body>
<div id="info_block">
  <a href="userdetails.php?id=12345" class="User_Name">TestUser</a>
  上传: <img class="arrowup" alt="up" />42
  下载: <img class="arrowdown" alt="down" />3
</div>
</body></html>`

const novahdUserdetailsFixture = `<html><body>
<table>
  <tr>
    <td class="rowhead">上传量</td>
    <td>17.020 TB</td>
  </tr>
  <tr>
    <td class="rowhead">下载量</td>
    <td>1.499 TB</td>
  </tr>
  <tr>
    <td class="rowhead">分享率</td>
    <td><font color="">11.356</font></td>
  </tr>
  <tr>
    <td class="rowhead">等级</td>
    <td><img alt="Extreme User" title="Extreme User" src="pic/extreme.gif" /></td>
  </tr>
  <tr>
    <td class="rowhead">魔力值</td>
    <td>1,234,567.89</td>
  </tr>
  <tr>
    <td class="rowhead">加入日期</td>
    <td>2020-01-15 12:30:00 (5年前)</td>
  </tr>
</table>
<table>
  <tr>
    <td style="background: red"><a href="messages.php">7</a></td>
  </tr>
</table>
</body></html>`

const novahdMybonusFixture = `<html><body>
<div id="outer">
  <table><tr><td rowspan="2">2.5678</td></tr></table>
</div>
</body></html>`

const novahdDetailFixture = `<html><body>
<h1 value="[Test] NovaHD.Example.Movie.2025.BluRay.1080p">
  <input name="torrent_name" type="hidden" value="[Test] NovaHD.Example.Movie.2025.BluRay.1080p" />
  <input name="detail_torrent_id" type="hidden" value="54321" />
  <font class="free">免费</font>
  <span title="2026-03-01 12:00:00">29天23时</span>
</h1>
<table>
  <tr>
    <td class="rowhead">基本信息</td>
    <td class="rowfollow">大小</td>
    <td class="rowfollow">大小：42.5 GB | 类型：电影</td>
  </tr>
</table>
</body></html>`

const novahdDetailWithHRFixture = `<html><body>
<h1 value="[Test] NovaHD.HR.Movie.2025.WEB-DL">
  <input name="torrent_name" type="hidden" value="[Test] NovaHD.HR.Movie.2025.WEB-DL" />
  <input name="detail_torrent_id" type="hidden" value="54322" />
</h1>
<table>
  <tr>
    <td class="rowhead">基本信息</td>
    <td class="rowfollow">大小</td>
    <td class="rowfollow">大小：8.5 GB | 类型：电影</td>
  </tr>
</table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

// --- Helpers ---

func getNovaHDDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("novahd")
	require.True(t, ok, "novahd definition not found")
	return def
}

// --- Suite: Search ---

func testNovaHDSearch(t *testing.T) {
	def := getNovaHDDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(novahdSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{
		BaseURL:   server.URL,
		Cookie:    "test_cookie=1",
		Selectors: def.Selectors,
	})
	driver.SetSiteDefinition(def)

	req := v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"}
	res, err := driver.Execute(context.Background(), req)
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2, "should parse 2 torrent rows")

	free := items[0]
	assert.Equal(t, "20001", free.ID)
	assert.Equal(t, "NovaHD.Test.Movie.2025.BluRay.1080p", free.Title)
	assert.Equal(t, "测试电影 / Test Movie / 2025", free.Subtitle)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	assert.False(t, free.DiscountEndTime.IsZero(), "discount end time should be parsed from onmouseover")
	assert.Equal(t, 2026, free.DiscountEndTime.Year())
	assert.Equal(t, 3, int(free.DiscountEndTime.Month()))
	assert.Equal(t, 1, free.DiscountEndTime.Day())
	assert.Equal(t, 150, free.Seeders)
	assert.Equal(t, 10, free.Leechers)
	assert.Equal(t, 500, free.Snatched)
	assert.True(t, free.SizeBytes > 0, "size should be parsed")

	normal := items[1]
	assert.Equal(t, "20002", normal.ID)
	assert.Equal(t, v2.DiscountNone, normal.DiscountLevel)
}

// --- Suite: Detail ---

func testNovaHDDetail(t *testing.T) {
	def := getNovaHDDef(t)

	t.Run("Free", func(t *testing.T) {
		doc := FixtureDoc(t, "novahd_detail", novahdDetailFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "54321", info.TorrentID)
		assert.Equal(t, "[Test] NovaHD.Example.Movie.2025.BluRay.1080p", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		assert.False(t, info.DiscountEnd.IsZero(), "discount end time should be parsed")
		assert.InDelta(t, 42.5*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})

	t.Run("WithHR", func(t *testing.T) {
		doc := FixtureDoc(t, "novahd_detail_hr", novahdDetailWithHRFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "54322", info.TorrentID)
		assert.True(t, info.HasHR, "should detect HR from hit_run.gif")
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel)
		assert.InDelta(t, 8.5*1024, info.SizeMB, 0.1)
	})
}

// --- Suite: UserInfo ---

func testNovaHDUserInfo(t *testing.T) {
	def := getNovaHDDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "novahd_index", novahdIndexFixture)
		fields := map[string]string{
			"id":       "12345",
			"name":     "TestUser",
			"seeding":  "42",
			"leeching": "3",
		}
		for field, expected := range fields {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q not found", field)
				got := driver.ExtractFieldValuePublic(doc, sel)
				assert.Equal(t, expected, got)
			})
		}
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "novahd_userdetails", novahdUserdetailsFixture)
		exact := map[string]string{
			"uploaded":     "18713687904747",
			"downloaded":   "1648167930036",
			"ratio":        "11.356",
			"levelName":    "Extreme User",
			"bonus":        "1.23456789e+06",
			"joinTime":     "1579091400",
			"messageCount": "7",
		}
		for field, expected := range exact {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q not found", field)
				got := driver.ExtractFieldValuePublic(doc, sel)
				assert.Equal(t, expected, got)
			})
		}
	})

	t.Run("MybonusPage", func(t *testing.T) {
		doc := FixtureDoc(t, "novahd_mybonus", novahdMybonusFixture)
		sel, ok := def.UserInfo.Selectors["bonusPerHour"]
		require.True(t, ok)
		got := driver.ExtractFieldValuePublic(doc, sel)
		assert.Equal(t, "2.5678", got)
	})
}

// --- Standalone Tests (edge cases beyond suite scope) ---

func TestNovaHD_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      novahdSearchFixture,
		"index":       novahdIndexFixture,
		"userdetails": novahdUserdetailsFixture,
		"mybonus":     novahdMybonusFixture,
		"detail":      novahdDetailFixture,
		"detail_hr":   novahdDetailWithHRFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
