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
		SiteID:   "springsunday",
		Search:   testSpringSundaySearch,
		Detail:   testSpringSundayDetail,
		UserInfo: testSpringSundayUserInfo,
	})
}

// --- Fixtures ---

const springsundaySearchFixture = `<html><body>
<table class="torrents">
<tbody>
<tr>
	<td class="rowfollow"><img alt="Movie" /></td>
	<td class="rowfollow">
		<table class="torrentname"><tr><td class="embedded">
			<div class="torrent-title">
				<a href="details.php?id=164895">Test.Movie.2025.BluRay.1080p</a>
				<span class="torrent-pro-free">免费</span>
				<span style="color:DimGray">(限时: <span title="2026-03-01 12:00:00">29天23时</span>)</span>
			</div>
			<div class="torrent-smalldescr">
				<span>标签1</span>
				<span title="测试电影 / Test Movie / 2025">测试电影 / Test Movie / 2025</span>
			</div>
		</td></tr></table>
	</td>
	<td class="rowfollow"></td>
	<td class="rowfollow nowrap"><span title="2025-01-15 08:30:00">5时<br/>5分</span></td>
	<td class="rowfollow">42.5 GB</td>
	<td class="rowfollow">150</td>
	<td class="rowfollow">10</td>
	<td class="rowfollow">500</td>
</tr>
<tr>
	<td class="rowfollow"><img alt="TV" /></td>
	<td class="rowfollow">
		<table class="torrentname"><tr><td class="embedded">
			<div class="torrent-title">
				<a href="details.php?id=164896">Test.Show.S01E01.WEB-DL.1080p</a>
			</div>
			<div class="torrent-smalldescr">
				<span title="测试剧集 / Test Show">测试剧集 / Test Show</span>
			</div>
		</td></tr></table>
	</td>
	<td class="rowfollow"></td>
	<td class="rowfollow nowrap"><span title="2025-01-14 20:00:00">2天前</span></td>
	<td class="rowfollow">2.0 GB</td>
	<td class="rowfollow">50</td>
	<td class="rowfollow">5</td>
	<td class="rowfollow">200</td>
</tr>
</tbody>
</table>
</body></html>`

const springsundayIndexFixture = `<html><body>
<div id="info_block">
	<a href="userdetails.php?id=81583"><b><span class="UserClass_Name Elite_Name">ilwpbb1314</span></b></a>
	上传: <img class="arrowup" alt="up" />94
	下载: <img class="arrowdown" alt="down" />0
	<a href="mybonus.php" title="茉莉: 546,424.7">茉莉: 546.4K</a>
</div>
</body></html>`

const springsundayUserdetailsFixture = `<html><body>
<div>你有2条新系统短讯！</div>
<div>你有1条新私人短讯！</div>
<table>
	<tr>
		<td class="rowhead">传输</td>
		<td class="rowfollow">
			<strong>上传量</strong>: 5.392 TB<br/>
			<strong>下载量</strong>: 462.06 GB<br/>
			<strong>分享率</strong>: <font color="">11.948</font>
		</td>
	</tr>
	<tr>
		<td class="rowhead">等级</td>
		<td class="rowfollow"><img alt="精英" title="精英" src="pic/elite.gif" /></td>
	</tr>
	<tr>
		<td class="rowhead">积分</td>
		<td class="rowfollow"><b>做种积分:</b> 913,905.2</td>
	</tr>
	<tr>
		<td class="rowhead">加入日期</td>
		<td class="rowfollow">2015-07-01 00:03:08 (<span title="2015-07-01 00:03:08">10年6月前</span>)</td>
	</tr>
</table>
</body></html>`

const springsundayMybonusFixture = `<html><body>
<h3>当前每小时能获得的积分/茉莉</h3>
<table>
	<thead><tr><th>类型</th><th>...</th><th>每小时茉莉</th></tr></thead>
	<tbody>
		<tr class="nowrap"><td><b>我的数据</b></td><td>1</td><td>2</td><td>3</td><td>4</td><td>5</td><td>6</td><td>7</td><td>8</td><td>9</td><td>71.740</td></tr>
	</tbody>
</table>
</body></html>`

const springsundayDetailFixture = `<html><body>
<h1>
	<input name="torrent_name" type="hidden" value="[Test] Example.Movie.2025.BluRay.1080p.DTS-HD.MA.5.1.x264" />
	<input name="detail_torrent_id" type="hidden" value="54321" />
	<font class="free">免费</font>
	<span title="2026-03-01 12:00:00">29天23时</span>
</h1>
<table>
	<tr>
		<td class="rowhead">基本信息</td>
		<td class="rowfollow">大小：42.5 GB | 类型：电影</td>
	</tr>
</table>
</body></html>`

const springsundayDetailWithHRFixture = `<html><body>
<h1>
	<input name="torrent_name" type="hidden" value="[Test] HR.Movie.2025.WEB-DL" />
	<input name="detail_torrent_id" type="hidden" value="54322" />
</h1>
<table>
	<tr>
		<td class="rowhead">基本信息</td>
		<td class="rowfollow">大小：8.5 GB | 类型：电影</td>
	</tr>
</table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

// --- Helpers ---

func getSpringSundayDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("springsunday")
	require.True(t, ok, "springsunday definition not found")
	return def
}

// --- Suite: Search ---

func testSpringSundaySearch(t *testing.T) {
	def := getSpringSundayDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(springsundaySearchFixture))
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
	assert.Equal(t, "164895", free.ID)
	assert.Equal(t, "Test.Movie.2025.BluRay.1080p", free.Title)
	assert.Equal(t, "测试电影 / Test Movie / 2025", free.Subtitle)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	assert.False(t, free.DiscountEndTime.IsZero(), "discount end time should be parsed from title attr")
	assert.Equal(t, 2026, free.DiscountEndTime.Year())
	assert.Equal(t, 3, int(free.DiscountEndTime.Month()))
	assert.Equal(t, 1, free.DiscountEndTime.Day())
	assert.Equal(t, 150, free.Seeders)
	assert.Equal(t, 10, free.Leechers)
	assert.Equal(t, 500, free.Snatched)
	assert.True(t, free.SizeBytes > 0, "size should be parsed")

	normal := items[1]
	assert.Equal(t, "164896", normal.ID)
	assert.Equal(t, v2.DiscountNone, normal.DiscountLevel)
}

// --- Suite: Detail ---

func testSpringSundayDetail(t *testing.T) {
	def := getSpringSundayDef(t)

	t.Run("Free", func(t *testing.T) {
		doc := FixtureDoc(t, "springsunday_detail", springsundayDetailFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "54321", info.TorrentID)
		assert.Equal(t, "[Test] Example.Movie.2025.BluRay.1080p.DTS-HD.MA.5.1.x264", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		assert.False(t, info.DiscountEnd.IsZero(), "discount end time should be parsed")
		assert.InDelta(t, 42.5*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})

	t.Run("WithHR", func(t *testing.T) {
		doc := FixtureDoc(t, "springsunday_detail_hr", springsundayDetailWithHRFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "54322", info.TorrentID)
		assert.True(t, info.HasHR, "should detect HR from hit_run.gif")
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel)
		assert.InDelta(t, 8.5*1024, info.SizeMB, 0.1)
	})
}

// --- Suite: UserInfo ---

func testSpringSundayUserInfo(t *testing.T) {
	def := getSpringSundayDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "springsunday_index", springsundayIndexFixture)
		fields := map[string]string{
			"id":       "81583",
			"name":     "ilwpbb1314",
			"seeding":  "94",
			"leeching": "0",
			"bonus":    "546424.7",
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
		doc := FixtureDoc(t, "springsunday_userdetails", springsundayUserdetailsFixture)
		exact := map[string]string{
			"uploaded":     "5928566696968",
			"downloaded":   "496133147197",
			"ratio":        "11.948",
			"levelName":    "精英",
			"seedingBonus": "913905.2",
			"joinTime":     "1435708988",
			"messageCount": "3",
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
		doc := FixtureDoc(t, "springsunday_mybonus", springsundayMybonusFixture)
		sel, ok := def.UserInfo.Selectors["bonusPerHour"]
		require.True(t, ok)
		got := driver.ExtractFieldValuePublic(doc, sel)
		assert.Equal(t, "71.74", got)
	})
}

// --- Standalone Tests (edge cases beyond suite scope) ---

func TestSpringSunday_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      springsundaySearchFixture,
		"index":       springsundayIndexFixture,
		"userdetails": springsundayUserdetailsFixture,
		"mybonus":     springsundayMybonusFixture,
		"detail":      springsundayDetailFixture,
		"detail_hr":   springsundayDetailWithHRFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
