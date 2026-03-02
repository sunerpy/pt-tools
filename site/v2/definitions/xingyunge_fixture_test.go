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
		SiteID:   "xingyunge",
		Search:   testXingYunGeSearch,
		Detail:   testXingYunGeDetail,
		UserInfo: testXingYunGeUserInfo,
	})
}

const xingyungeSearchFixture = `<html><body>
<table class="torrents">
<tbody>
<tr>
	<td class="rowfollow"><img alt="Movie" /></td>
	<td class="rowfollow">
		<table class="torrentname"><tr><td class="embedded">
			<a href="details.php?id=41001">XingYunGe.Test.Movie.2026.BluRay.1080p</a>
			<img class="pro_free" src="pic/trans.gif" alt="Free"
				onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;优惠剩余时间：&lt;b&gt;&lt;span title=&quot;2026-04-01 12:00:00&quot;&gt;20天&lt;/span&gt;&lt;/b&gt;', 'trail', false)" />
			<br/><span>测试电影 / XingYunGe Test Movie</span>
		</td></tr></table>
	</td>
	<td class="rowfollow"></td>
	<td class="rowfollow"><span title="2026-03-01 10:00:00">1天前</span></td>
	<td class="rowfollow">38.0 GB</td>
	<td class="rowfollow">132</td>
	<td class="rowfollow">8</td>
	<td class="rowfollow">421</td>
</tr>
<tr>
	<td class="rowfollow"><img alt="TV" /></td>
	<td class="rowfollow">
		<table class="torrentname"><tr><td class="embedded">
			<a href="details.php?id=41002">XingYunGe.Test.Show.S01E01.1080p.WEB-DL</a>
			<br/><span>测试剧集</span>
		</td></tr></table>
	</td>
	<td class="rowfollow"></td>
	<td class="rowfollow"><span title="2026-02-28 20:00:00">2天前</span></td>
	<td class="rowfollow">2.8 GB</td>
	<td class="rowfollow">36</td>
	<td class="rowfollow">4</td>
	<td class="rowfollow">152</td>
</tr>
</tbody>
</table>
</body></html>`

const xingyungeIndexFixture = `<html><body>
<div id="info_block">
  <a href="userdetails.php?id=12345" class="User_Name">TestUser</a>
  上传: <img class="arrowup" alt="up" />42
  下载: <img class="arrowdown" alt="down" />3
  <font class='color_bonus'>星焱 </font>[<a href="mybonus.php">使用</a>]: 150,356.9
</div>
</body></html>`

const xingyungeUserdetailsFixture = `<html><body>
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

const xingyungeMybonusFixture = `<html><body>
<div id="outer">
  <table><tr><td rowspan="2">2.5678</td></tr></table>
</div>
</body></html>`

const xingyungeDetailFixture = `<html><body>
<h1 value="[Test] XingYunGe.Example.Movie.2026.BluRay.1080p">
  <input name="torrent_name" type="hidden" value="[Test] XingYunGe.Example.Movie.2026.BluRay.1080p" />
  <input name="detail_torrent_id" type="hidden" value="71001" />
  <font class="free">免费</font>
  <span title="2026-04-01 12:00:00">20天</span>
</h1>
<table>
  <tr>
    <td class="rowhead">基本信息</td>
    <td class="rowfollow">大小</td>
    <td class="rowfollow">大小：38.0 GB | 类型：电影</td>
  </tr>
</table>
</body></html>`

const xingyungeDetailWithHRFixture = `<html><body>
<h1 value="[Test] XingYunGe.HR.Movie.2026.WEB-DL">
  <input name="torrent_name" type="hidden" value="[Test] XingYunGe.HR.Movie.2026.WEB-DL" />
  <input name="detail_torrent_id" type="hidden" value="71002" />
</h1>
<table>
  <tr>
    <td class="rowhead">基本信息</td>
    <td class="rowfollow">大小</td>
    <td class="rowfollow">大小：6.5 GB | 类型：电影</td>
  </tr>
</table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

func getXingYunGeDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("xingyunge")
	require.True(t, ok, "xingyunge definition not found")
	return def
}

func testXingYunGeSearch(t *testing.T) {
	def := getXingYunGeDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(xingyungeSearchFixture))
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
	require.Len(t, items, 2)

	free := items[0]
	assert.Equal(t, "41001", free.ID)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	assert.False(t, free.DiscountEndTime.IsZero())
	assert.Equal(t, 132, free.Seeders)
	assert.Equal(t, 8, free.Leechers)

	normal := items[1]
	assert.Equal(t, "41002", normal.ID)
	assert.Equal(t, v2.DiscountNone, normal.DiscountLevel)
}

func testXingYunGeDetail(t *testing.T) {
	def := getXingYunGeDef(t)

	t.Run("Free", func(t *testing.T) {
		doc := FixtureDoc(t, "xingyunge_detail", xingyungeDetailFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "71001", info.TorrentID)
		assert.Equal(t, "[Test] XingYunGe.Example.Movie.2026.BluRay.1080p", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		assert.False(t, info.DiscountEnd.IsZero())
		assert.InDelta(t, 38.0*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})

	t.Run("WithHR", func(t *testing.T) {
		doc := FixtureDoc(t, "xingyunge_detail_hr", xingyungeDetailWithHRFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "71002", info.TorrentID)
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel)
		assert.InDelta(t, 6.5*1024, info.SizeMB, 0.1)
		assert.True(t, info.HasHR)
	})
}

func testXingYunGeUserInfo(t *testing.T) {
	def := getXingYunGeDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "xingyunge_index", xingyungeIndexFixture)
		fields := map[string]string{
			"id":         "12345",
			"name":       "TestUser",
			"seeding":    "42",
			"leeching":   "3",
			"bonusIndex": "150356.9",
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
		doc := FixtureDoc(t, "xingyunge_userdetails", xingyungeUserdetailsFixture)
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
		doc := FixtureDoc(t, "xingyunge_mybonus", xingyungeMybonusFixture)
		sel, ok := def.UserInfo.Selectors["bonusPerHour"]
		require.True(t, ok)
		got := driver.ExtractFieldValuePublic(doc, sel)
		assert.Equal(t, "2.5678", got)
	})
}

func TestXingYunGe_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      xingyungeSearchFixture,
		"index":       xingyungeIndexFixture,
		"userdetails": xingyungeUserdetailsFixture,
		"mybonus":     xingyungeMybonusFixture,
		"detail":      xingyungeDetailFixture,
		"detail_hr":   xingyungeDetailWithHRFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
