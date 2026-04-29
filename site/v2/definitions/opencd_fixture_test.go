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
		SiteID:   "opencd",
		Search:   testOpenCDSearch,
		Detail:   testOpenCDDetail,
		UserInfo: testOpenCDUserInfo,
	})
}

const opencdSearchFixture = `<html><body>
<table class="torrents">
<tbody>
<tr>
  <td class="rowfollow"><img alt="Music" title="音樂(Music)" /></td>
  <td class="rowfollow"><img src="pic/cover.jpg" /></td>
  <td class="rowfollow">
    <table class="torrentname"><tr>
      <td class="embedded">
        <a href="plugin_details.php?id=193816&hit=1" class="index"><b>Test.Artist - Test.Album.2025.FLAC</b></a>
        <br/><font color='#888888'>測試專輯 / Test Album / 2025</font>
      </td>
      <td class="nowrap embedded" style="text-align:right;">
        <img class="pro_free" src="pic/trans.gif" alt="Free"
             onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免費&lt;/font&gt;&lt;/b&gt;剩餘時間：&lt;b&gt;&lt;span title=&quot;2026-04-12 19:34:57&quot;&gt;2天7時&lt;/span&gt;&lt;/b&gt;', 'trail', false)" />
        <span title="2026-04-12 19:34:57">2天7時</span>
      </td>
    </tr></table>
  </td>
  <td class="rowfollow"><b>100</b></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-04-09 19:34:57">3天前</span></td>
  <td class="rowfollow">3.34&nbsp;GB</td>
  <td class="rowfollow"><b><a href="#seeders">304</a></b></td>
  <td class="rowfollow"><b><a href="#leechers">5</a></b></td>
  <td class="rowfollow"><a href="viewsnatches.php"><b>805</b></a></td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="Music" title="音樂(Music)" /></td>
  <td class="rowfollow"><img src="pic/cover2.jpg" /></td>
  <td class="rowfollow">
    <table class="torrentname"><tr>
      <td class="embedded">
        <a href="plugin_details.php?id=193817&hit=1" class="index"><b>Normal.Album.2025.FLAC</b></a>
        <br/><font color='#888888'>普通專輯</font>
      </td>
      <td class="nowrap embedded" style="text-align:right;"></td>
    </tr></table>
  </td>
  <td class="rowfollow"><b>50</b></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-04-08 10:00:00">5天前</span></td>
  <td class="rowfollow">1.20&nbsp;GB</td>
  <td class="rowfollow"><b><a href="#seeders">100</a></b></td>
  <td class="rowfollow"><b><a href="#leechers">2</a></b></td>
  <td class="rowfollow"><a href="viewsnatches.php"><b>250</b></a></td>
  <td class="rowfollow"><i>TestUser</i></td>
</tr>
</tbody>
</table>
</body></html>`

const opencdIndexFixture = `<html><body>
<div id="info_block">
  歡迎回來, <a href="userdetails.php?id=99102" class="User_Name"><b>TestUser</b></a>
  <img class="arrowup" src="pic/arrowup.gif" />571
  <img class="arrowdown" src="pic/arrowdown.gif" />0
</div>
</body></html>`

const opencdUserdetailsFixture = `<html><body>
<table width="100%" border="1">
<tr>
  <td class="rowhead">加入日期</td>
  <td class="rowfollow">2026-01-22 20:16:33 (<span title="2026-01-22 20:16:33">18天15時前</span>,2周)</td>
</tr>
<tr>
  <td class="rowhead">傳送</td>
  <td class="rowfollow">
    <strong>分享率</strong>:  <font color="">20.123</font><br/>
    <strong>上傳量</strong>:  928.41 GB(本月30.18 GB)<br/>
    <strong>下載量</strong>:  46.14 GB(本月3.18 GB)
  </td>
</tr>
<tr>
  <td class="rowhead">等級</td>
  <td class="rowfollow"><img alt="User" title="User" src="pic/user.gif" /></td>
</tr>
<tr>
  <td class="rowhead">魔力值</td>
  <td class="rowfollow">246,729.8</td>
</tr>
</table>
</body></html>`

const opencdDetailFixture = `<html><body>
<div class="title">周杰倫 - Jay 環球日版 2000 - WAV 整軌
  <img class="pro_free2up" src="pic/trans.gif" alt="Free 2xUp"
       onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;twoupfree&quot;&gt;免費2xUp&lt;/font&gt;&lt;/b&gt;剩餘時間：&lt;b&gt;&lt;span title=&quot;2026-04-15 18:00:00&quot;&gt;5天前&lt;/span&gt;&lt;/b&gt;', 'trail', false)" />
</div>
<div class="smalltitle">Jay 首張專輯 / 環球日版</div>
<table>
  <tr>
    <td class="rowtitle">大小</td>
    <td class="rowfollow">大小：3.34 GB</td>
  </tr>
  <tr>
    <td class="rowtitle">下載鏈接</td>
    <td class="rowfollow"><a href="download.php?id=193816">下載種子</a></td>
  </tr>
</table>
</body></html>`

func getOpenCDDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("opencd")
	require.True(t, ok, "opencd definition not found")
	return def
}

func testOpenCDSearch(t *testing.T) {
	def := getOpenCDDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(opencdSearchFixture))
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
	require.Len(t, items, 2, "should parse 2 torrent rows")

	free := items[0]
	assert.Equal(t, "193816", free.ID)
	assert.Equal(t, "Test.Artist - Test.Album.2025.FLAC", free.Title)
	assert.Equal(t, "測試專輯 / Test Album / 2025", free.Subtitle)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	assert.Equal(t, 304, free.Seeders)
	assert.Equal(t, 5, free.Leechers)
	assert.Equal(t, 805, free.Snatched)
	assert.True(t, free.SizeBytes > 0, "size should be parsed")

	normal := items[1]
	assert.Equal(t, "193817", normal.ID)
	assert.Equal(t, v2.DiscountNone, normal.DiscountLevel)
}

func testOpenCDDetail(t *testing.T) {
	def := getOpenCDDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "opencd_detail", opencdDetailFixture)
	info := parser.ParseAll(doc.Selection)

	assert.NotEqual(t, v2.DiscountNone, info.DiscountLevel, "should parse discount from div.title font class")
	assert.False(t, info.HasHR)
}

func testOpenCDUserInfo(t *testing.T) {
	def := getOpenCDDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "opencd_index", opencdIndexFixture)
		assert.Equal(t, "99102", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["id"]))
		assert.Equal(t, "TestUser", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["name"]))
		assert.Equal(t, "571", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["seeding"]))
		assert.Equal(t, "0", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["leeching"]))
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "opencd_userdetails", opencdUserdetailsFixture)
		assert.Equal(t, "User", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["levelName"]))
		exactPositive := []string{"uploaded", "downloaded", "ratio", "bonus", "joinTime"}
		for _, field := range exactPositive {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q not found", field)
				got := driver.ExtractFieldValuePublic(doc, sel)
				assert.NotEmpty(t, got, "field %q should be parsed", field)
			})
		}
	})
}

func TestOpenCD_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      opencdSearchFixture,
		"index":       opencdIndexFixture,
		"userdetails": opencdUserdetailsFixture,
		"detail":      opencdDetailFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
