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
		SiteID:   "agsvpt",
		Search:   testAGSVPTSearch,
		Detail:   testAGSVPTDetail,
		UserInfo: testAGSVPTUserInfo,
	})
}

const agsvptSearchFixture = `<html><body>
<table class="torrents">
<tbody>
<tr>
	<td class="rowfollow"><img alt="Movie" /></td>
	<td class="rowfollow">
		<table class="torrentname"><tr><td class="embedded">
			<a href="details.php?id=31001">AGSVPT.Test.Movie.2026.BluRay.1080p</a>
			<img class="pro_free" src="pic/trans.gif" alt="Free"
				onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;优惠剩余时间：&lt;b&gt;&lt;span title=&quot;2026-03-20 18:00:00&quot;&gt;10天&lt;/span&gt;&lt;/b&gt;', 'trail', false)" />
			<br/><span>测试电影 / AGSVPT Test Movie</span>
		</td></tr></table>
	</td>
	<td class="rowfollow"></td>
	<td class="rowfollow"><span title="2026-03-01 08:30:00">1天前</span></td>
	<td class="rowfollow">26.5 GB</td>
	<td class="rowfollow">98</td>
	<td class="rowfollow">12</td>
	<td class="rowfollow">345</td>
</tr>
<tr>
	<td class="rowfollow"><img alt="TV" /></td>
	<td class="rowfollow">
		<table class="torrentname"><tr><td class="embedded">
			<a href="details.php?id=31002">AGSVPT.Test.Show.S01E01.1080p.WEB-DL</a>
			<br/><span>测试剧集</span>
		</td></tr></table>
	</td>
	<td class="rowfollow"></td>
	<td class="rowfollow"><span title="2026-02-28 20:00:00">2天前</span></td>
	<td class="rowfollow">3.2 GB</td>
	<td class="rowfollow">45</td>
	<td class="rowfollow">6</td>
	<td class="rowfollow">188</td>
</tr>
</tbody>
</table>
</body></html>`

const agsvptIndexFixture = `<html><body>
<div id="info_block">
  <a href="userdetails.php?id=12345" class="User_Name">TestUser</a>
  上传: <img class="arrowup" alt="up" />42
  下载: <img class="arrowdown" alt="down" />3
</div>
</body></html>`

const agsvptUserdetailsFixture = `<html><body>
<table>
  <tr>
    <td class="rowhead">传输</td>
    <td><table border="0" cellspacing="0" cellpadding="0">
      <tr><td class="embedded"><strong>分享率</strong>:  <font color="">11.356</font></td></tr>
      <tr><td class="embedded"><strong>上传量</strong>:  17.020 TB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>:  1.499 TB</td></tr>
    </table></td>
  </tr>
  <tr>
    <td class="rowhead">等级</td>
    <td><img alt="Extreme User" title="Extreme User" src="pic/extreme.gif" /></td>
  </tr>
  <tr>
    <td class="rowhead">冰晶</td>
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

const agsvptMybonusFixture = `<html><body>
<div id="outer">
  <table><tr><td rowspan="2">2.5678</td></tr></table>
</div>
</body></html>`

const agsvptDetailFixture = `<html><body>
<h1 value="[Test] AGSVPT.Example.Movie.2026.BluRay.1080p">
  <input name="torrent_name" type="hidden" value="[Test] AGSVPT.Example.Movie.2026.BluRay.1080p" />
  <input name="detail_torrent_id" type="hidden" value="61001" />
  <font class="free">免费</font>
  <span title="2026-03-20 18:00:00">10天</span>
</h1>
<table>
  <tr>
    <td class="rowhead">基本信息</td>
    <td class="rowfollow">大小</td>
    <td class="rowfollow">大小：26.5 GB | 类型：电影</td>
  </tr>
</table>
</body></html>`

const agsvptDetailWithHRFixture = `<html><body>
<h1 value="[Test] AGSVPT.HR.Movie.2026.WEB-DL">
  <input name="torrent_name" type="hidden" value="[Test] AGSVPT.HR.Movie.2026.WEB-DL" />
  <input name="detail_torrent_id" type="hidden" value="61002" />
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

func getAGSVPTDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("agsvpt")
	require.True(t, ok, "agsvpt definition not found")
	return def
}

func testAGSVPTSearch(t *testing.T) {
	def := getAGSVPTDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(agsvptSearchFixture))
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
	assert.Equal(t, "31001", free.ID)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	assert.False(t, free.DiscountEndTime.IsZero())
	assert.Equal(t, 98, free.Seeders)
	assert.Equal(t, 12, free.Leechers)

	normal := items[1]
	assert.Equal(t, "31002", normal.ID)
	assert.Equal(t, v2.DiscountNone, normal.DiscountLevel)
}

func testAGSVPTDetail(t *testing.T) {
	def := getAGSVPTDef(t)

	t.Run("Free", func(t *testing.T) {
		doc := FixtureDoc(t, "agsvpt_detail", agsvptDetailFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "61001", info.TorrentID)
		assert.Equal(t, "[Test] AGSVPT.Example.Movie.2026.BluRay.1080p", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		assert.False(t, info.DiscountEnd.IsZero())
		assert.InDelta(t, 26.5*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})

	t.Run("WithHR", func(t *testing.T) {
		doc := FixtureDoc(t, "agsvpt_detail_hr", agsvptDetailWithHRFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "61002", info.TorrentID)
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel)
		assert.InDelta(t, 8.5*1024, info.SizeMB, 0.1)
		assert.True(t, info.HasHR)
	})
}

func testAGSVPTUserInfo(t *testing.T) {
	def := getAGSVPTDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "agsvpt_index", agsvptIndexFixture)
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
		doc := FixtureDoc(t, "agsvpt_userdetails", agsvptUserdetailsFixture)
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
		doc := FixtureDoc(t, "agsvpt_mybonus", agsvptMybonusFixture)
		sel, ok := def.UserInfo.Selectors["bonusPerHour"]
		require.True(t, ok)
		got := driver.ExtractFieldValuePublic(doc, sel)
		assert.Equal(t, "2.5678", got)
	})
}

func TestAGSVPT_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      agsvptSearchFixture,
		"index":       agsvptIndexFixture,
		"userdetails": agsvptUserdetailsFixture,
		"mybonus":     agsvptMybonusFixture,
		"detail":      agsvptDetailFixture,
		"detail_hr":   agsvptDetailWithHRFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
