package definitions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func init() {
	RegisterFixtureSuite(FixtureSuite{
		SiteID:   "pttime",
		Search:   testPTTimeSearch,
		Detail:   testPTTimeDetail,
		UserInfo: testPTTimeUserInfo,
	})
}

const pttimeSearchFixture = `<html><body>
<table class="torrents">
<tbody>
<tr>
  <td class="rowfollow"><span class="category dib c_movies" title="电影">Movies</span></td>
  <td class="rowfollow">
    <table class="torrentname"><tr>
      <td class="embedded">
        <a class="torrentname_title" href="details.php?id=50639"><b title="Test.Movie.2025.2160p.UHD.BluRay-50639">Test.Movie.2025.2160p.UHD.BluRay</b></a>
        <font class="promotion halfdown">50%</font> <span title="2025-01-12 08:00:00">2天</span>
        <br/><a class="dib cp" href="/torrents.php?tags=zz"><span class="tags zz">中字</span></a><font title="测试电影">测试电影 / Test Movie / 2025</font>
      </td>
      <td class="embedded"><a href="download.php?id=50639"><img src="pic/dl.gif" alt="dl"/></a></td>
    </tr></table>
  </td>
  <td class="rowfollow dn" style="display:none">PTR</td>
  <td class="rowfollow dn" style="display:none">0</td>
  <td class="rowfollow"><span title="2025-01-10 08:00:00">3天前</span></td>
  <td class="rowfollow">45.08 GB</td>
  <td class="rowfollow"><a href="#seeders">150</a></td>
  <td class="rowfollow"><a href="#leechers">10</a></td>
  <td class="rowfollow"><a href="viewsnatches.php">500</a></td>
  <td class="rowfollow">-</td>
  <td class="rowfollow">TestUploader</td>
  <td class="rowfollow"></td>
</tr>
<tr>
  <td class="rowfollow"><span class="category dib c_tv" title="剧集">TV</span></td>
  <td class="rowfollow">
    <table class="torrentname"><tr>
      <td class="embedded">
        <a class="torrentname_title" href="details.php?id=50640"><b title="Test.Show.S01.1080p-50640">Test.Show.S01.1080p</b></a>
        <font class="promotion free">免费</font> <span title="2025-01-11 20:00:00">1天</span><font class="promotion bb">字幕/附件(1)</font>
        <br/><font title="测试剧集">测试剧集</font>
      </td>
      <td class="embedded"><span title="豆瓣id:1001">7.0</span> <span title="IMDb:tt0000001">7.1</span></td>
      <td class="embedded"><a href="download.php?id=50640"><img src="pic/dl.gif" alt="dl"/></a></td>
    </tr></table>
  </td>
  <td class="rowfollow dn" style="display:none">PTR</td>
  <td class="rowfollow dn" style="display:none">0</td>
  <td class="rowfollow"><span title="2025-01-09 20:00:00">4天前</span></td>
  <td class="rowfollow">12.50 GB</td>
  <td class="rowfollow"><a href="#seeders">80</a></td>
  <td class="rowfollow"><a href="#leechers">5</a></td>
  <td class="rowfollow"><a href="viewsnatches.php">200</a></td>
  <td class="rowfollow">-</td>
  <td class="rowfollow">-</td>
  <td class="rowfollow"></td>
</tr>
<tr>
  <td class="rowfollow"><span class="category dib c_movies" title="电影">Movies</span></td>
  <td class="rowfollow">
    <table class="torrentname"><tr>
      <td class="embedded">
        <a class="torrentname_title" href="details.php?id=50641"><b title="Test.Twoupfree.2025.1080p-50641">Test.Twoupfree.2025.1080p</b></a>
        <font class="promotion twoupfree">2X免费</font> <span title="2025-01-13 09:30:00">3天</span>
        <br/><font title="测试 2X 免费">测试 2X 免费</font>
      </td>
    </tr></table>
  </td>
  <td class="rowfollow dn" style="display:none">PTR</td>
  <td class="rowfollow dn" style="display:none">0</td>
  <td class="rowfollow"><span title="2025-01-09 10:00:00">4天前</span></td>
  <td class="rowfollow">8.00 GB</td>
  <td class="rowfollow"><a href="#seeders">3</a></td>
  <td class="rowfollow"><a href="#leechers">9</a></td>
  <td class="rowfollow"><a href="viewsnatches.php">20</a></td>
  <td class="rowfollow">-</td>
  <td class="rowfollow">-</td>
  <td class="rowfollow"></td>
</tr>
<tr>
  <td class="rowfollow"><span class="category dib c_movies" title="电影">Movies</span></td>
  <td class="rowfollow">
    <table class="torrentname"><tr>
      <td class="embedded">
        <a class="torrentname_title" href="details.php?id=50642"><b title="Test.Plain.2025.1080p-50642">Test.Plain.2025.1080p</b></a>
        <font class="promotion bb">字幕/附件(1)</font> <span title="豆瓣id:1002">6.6</span>
        <br/><font title="没有优惠">没有优惠</font>
      </td>
    </tr></table>
  </td>
  <td class="rowfollow dn" style="display:none">PTR</td>
  <td class="rowfollow dn" style="display:none">0</td>
  <td class="rowfollow"><span title="2025-01-08 10:00:00">5天前</span></td>
  <td class="rowfollow">4.00 GB</td>
  <td class="rowfollow"><a href="#seeders">30</a></td>
  <td class="rowfollow"><a href="#leechers">1</a></td>
  <td class="rowfollow"><a href="viewsnatches.php">40</a></td>
  <td class="rowfollow">-</td>
  <td class="rowfollow">-</td>
  <td class="rowfollow"></td>
</tr>
</tbody>
</table>
</body></html>`

const pttimeIndexFixture = `<html><body>
<div id="info_block">
  欢迎, <a href="userdetails.php?id=50639" class="EliteUser_Name"><b>TestUser</b></a>
  <span class="medium">[UID=50639][(初中)Elite User]</span>
  <font class="color_ratio">分享率:</font>6.37
  <font class="fcg">上传:</font>24.23 TB
  <font class="fcr">下载:</font>3.81 TB
  <font title="当前做种" class="fcg">⬆</font>8
  <font title="当前下载" class="fcr">⬇</font>0
  魔力值(84.36魔力/小时) [<a href="mybonus.php">使用</a>]: 1889134.7
</div>
</body></html>`

const pttimeUserdetailsFixture = `<html><body>
<div id="outer">
  魔力值(84.36魔力/小时) [<a href="mybonus.php">使用</a>]: 1889134.7
  <table>
    <tr>
      <td class="rowhead">加入日期</td>
      <td class="rowfollow">2023-01-15 10:00:00 (<span title="2023-01-15 10:00:00">3年前</span>)</td>
    </tr>
  </table>
</div>
</body></html>`

const pttimeDetailFixture = `<html><body>
<h1 align='center' class='m0' id='top'>The Listener 2020 4K WEB-DL HEVC 10bits
  [<font class='halfdown'>50%免费</font>][2个做种者]
</h1>
<table>
  <tr>
    <td class="rowhead">基本信息</td>
    <td class="rowfollow">
      <b><b>大小：</b></b>136.99 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>HEVC
      <span title="2023-06-24 13:36:54">2年前</span>
    </td>
  </tr>
  <tr>
    <td class="rowhead">副标题</td>
    <td class="rowfollow">心灵法医 全36集</td>
  </tr>
  <tr>
    <td class="rowhead">下载</td>
    <td class="rowfollow fcs"><a class="index fcs" href="download.php?id=50639">下载种子</a></td>
  </tr>
</table>
</body></html>`

const pttimeDetailFreeFixture = `<html><body>
<h1 align='center' class='m0' id='top'>Free.Movie.2025.1080p
  [<font class='free'>免费</font>]
</h1>
<table>
  <tr>
    <td class="rowhead">基本信息</td>
    <td class="rowfollow">
      <b><b>大小：</b></b>8.50 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>Movies
    </td>
  </tr>
</table>
</body></html>`

func getPTTimeDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("pttime")
	require.True(t, ok, "pttime definition not found")
	return def
}

func testPTTimeSearch(t *testing.T) {
	def := getPTTimeDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(pttimeSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{
		BaseURL:   server.URL,
		Cookie:    "test_cookie=1",
		Selectors: def.Selectors,
	})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 4, "should parse 4 torrent rows")
	cst := time.FixedZone("CST", 8*3600)

	half := items[0]
	assert.Equal(t, "50639", half.ID)
	assert.Equal(t, "Test.Movie.2025.2160p.UHD.BluRay", half.Title)
	assert.Equal(t, v2.DiscountPercent50, half.DiscountLevel, "halfdown should map to Percent50")
	assert.Equal(t, 150, half.Seeders)
	assert.Equal(t, 10, half.Leechers)
	assert.Equal(t, 500, half.Snatched)

	// 结束时间在优惠角标后面的 span[title] 里（文字是剩余时间）：以前没读到，刷流全按「免费结束时间未知」拒掉（2026-10-26）
	assert.True(t, half.DiscountEndTime.Equal(time.Date(2025, 1, 12, 8, 0, 0, 0, cst)), half.DiscountEndTime)

	free := items[1]
	assert.Equal(t, "50640", free.ID)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel, "free class should map to Free")
	assert.True(t, free.DiscountEndTime.Equal(time.Date(2025, 1, 11, 20, 0, 0, 0, cst)), free.DiscountEndTime)

	two := items[2]
	assert.Equal(t, v2.Discount2xFree, two.DiscountLevel, "twoupfree 不能被 free / twoup 先匹配走")
	assert.True(t, two.DiscountEndTime.Equal(time.Date(2025, 1, 13, 9, 30, 0, 0, cst)), two.DiscountEndTime)

	plain := items[3]
	assert.Equal(t, v2.DiscountNone, plain.DiscountLevel, "字幕/附件角标（promotion bb）不是优惠")
	assert.True(t, plain.DiscountEndTime.IsZero(), "评分的 span[title] 不是结束时间")
}

func testPTTimeDetail(t *testing.T) {
	def := getPTTimeDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	t.Run("HalfDown", func(t *testing.T) {
		doc := FixtureDoc(t, "pttime_detail", pttimeDetailFixture)
		info := parser.ParseAll(doc.Selection)
		assert.Equal(t, v2.DiscountPercent50, info.DiscountLevel)
		assert.InDelta(t, 136.99*1024, info.SizeMB, 1.0)
		assert.False(t, info.HasHR)
	})

	t.Run("Free", func(t *testing.T) {
		doc := FixtureDoc(t, "pttime_detail_free", pttimeDetailFreeFixture)
		info := parser.ParseAll(doc.Selection)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		assert.InDelta(t, 8.50*1024, info.SizeMB, 1.0)
	})
}

func testPTTimeUserInfo(t *testing.T) {
	def := getPTTimeDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "pttime_index", pttimeIndexFixture)
		assert.Equal(t, "50639", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["id"]))
		assert.Equal(t, "TestUser", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["name"]))
		notEmpty := []string{"uploaded", "downloaded", "ratio", "seeding", "leeching", "bonus"}
		for _, field := range notEmpty {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q not found", field)
				got := driver.ExtractFieldValuePublic(doc, sel)
				assert.NotEmpty(t, got, "field %q should be parsed", field)
			})
		}
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "pttime_userdetails", pttimeUserdetailsFixture)
		sel, ok := def.UserInfo.Selectors["joinTime"]
		require.True(t, ok)
		got := driver.ExtractFieldValuePublic(doc, sel)
		assert.NotEmpty(t, got, "joinTime should be parsed")

		bonusSel, ok := def.UserInfo.Selectors["bonusPerHour"]
		require.True(t, ok, "bonusPerHour selector not found")
		assert.Equal(t, "84.36", driver.ExtractFieldValuePublic(doc, bonusSel))
	})
}

func TestPTTime_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      pttimeSearchFixture,
		"index":       pttimeIndexFixture,
		"userdetails": pttimeUserdetailsFixture,
		"detail":      pttimeDetailFixture,
		"detail_free": pttimeDetailFreeFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
