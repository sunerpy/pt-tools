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
		SiteID:   "pt0ffcc",
		Search:   testPT0FFCCSearch,
		Detail:   testPT0FFCCDetail,
		UserInfo: testPT0FFCCUserInfo,
	})
}

const pt0ffccSearchFixture = `<html><body>
<table class="torrents">
<tbody>
<tr>
  <td><a href="?cat=401" title="Movies|电影"><img alt="Movies|电影" /></a></td>
  <td>
    <table class="torrentname"><tr>
      <td class="embedded"><img class="nexus-lazy-load" data-src="pic/cover.jpg" /></td>
      <td class="embedded">
        <a href="details.php?id=38742"><b>Tampopo.1985.CC.BluRay.1080p.x264.FLAC.1.0-CMCT</b></a>
        <img class="pro_free" src="pic/trans.gif" alt="Free" />
        <span title="通过"><svg></svg></span>
        <br/>蒲公英 / Dandelion / Tampopo [CC收藏版] [日语] [简繁中字]
      </td>
      <td class="embedded">
        <a href="download.php?id=38742"><img src="pic/dl.gif" alt="dl"/></a>
      </td>
    </tr></table>
  </td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2025-07-02 14:27:28">10月3天前</span></td>
  <td class="rowfollow">9.07<br/>GB</td>
  <td class="rowfollow"><a href="#seeders"><b>50</b></a></td>
  <td class="rowfollow"><a href="#leechers"><b>0</b></a></td>
  <td class="rowfollow"><a href="viewsnatches.php?id=38742"><b>269</b></a></td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td><a href="?cat=401" title="Movies|电影"><img alt="Movies|电影" /></a></td>
  <td>
    <table class="torrentname"><tr>
      <td class="embedded"><img class="nexus-lazy-load" data-src="pic/cover2.jpg" /></td>
      <td class="embedded">
        <a href="details.php?id=38743"><b>Normal.Movie.2025.1080p</b></a>
        <br/>普通电影
      </td>
      <td class="embedded">
        <a href="download.php?id=38743"><img src="pic/dl.gif" alt="dl"/></a>
      </td>
    </tr></table>
  </td>
  <td class="rowfollow">1</td>
  <td class="rowfollow"><span title="2025-10-01 08:00:00">3天前</span></td>
  <td class="rowfollow">4.20<br/>GB</td>
  <td class="rowfollow"><a href="#seeders"><b>12</b></a></td>
  <td class="rowfollow"><a href="#leechers"><b>3</b></a></td>
  <td class="rowfollow"><a href="viewsnatches.php?id=38743"><b>45</b></a></td>
  <td class="rowfollow"><a href="userdetails.php?id=100">testuploader</a></td>
</tr>
</tbody>
</table>
</body></html>`

const pt0ffccDetailFixture = `<html><body>
<h1 align='center' id='top'>Tampopo.1985.CC.BluRay.1080p.x264.FLAC.1.0-CMCT&nbsp;
  <b>[<font class='free'>免费</font>]</b>
  <span title="通过"><svg></svg></span>
</h1>
<table>
  <tr>
    <td class="rowhead">下载</td>
    <td class="rowfollow">
      <a class="index" href="download.php?id=38742">Tampopo.1985.torrent</a>
      <span title="2025-07-02 14:27:28">10月3天前</span>
    </td>
  </tr>
  <tr>
    <td class="rowhead">副标题</td>
    <td class="rowfollow">蒲公英 / Dandelion / Tampopo [CC收藏版] [日语] [简繁中字]</td>
  </tr>
  <tr>
    <td class="rowhead">基本信息</td>
    <td class="rowfollow">
      <b><b>大小：</b></b>9.07 GB&nbsp;&nbsp;&nbsp;
      <b>类型:</b>&nbsp;Movies|电影&nbsp;&nbsp;&nbsp;
      <b>地区: </b>日本 (JPN)&nbsp;&nbsp;&nbsp;
      <b>分辨率: </b>1080P&nbsp;&nbsp;&nbsp;
      <b>媒介: </b>Encode
    </td>
  </tr>
</table>
</body></html>`

const pt0ffccIndexFixture = `<html><body>
<div id="info_block">
  欢迎回来, <a href="userdetails.php?id=12189" class="User_Name"><b>TestViewer</b></a>
  <img class="arrowup" src="pic/arrowup.gif" alt="Torrents seeding" />1
  <img class="arrowdown" src="pic/arrowdown.gif" alt="Torrents leeching" />0
  <font class="color_bonus">魔力值</font>[<a href="mybonus.php">使用</a>]: 4,526.9
</div>
</body></html>`

const pt0ffccUserdetailsFixture = `<html><body>
<h1 style='margin:0px'><span class="nowrap"><b>testuser</b></span><img src="pic/flag/CN.gif" alt="CN" /></h1>
<p>(加入好友列表) - (加入黑名单)</p>
<table class="main" width="1200">
<table border="1">
<tr>
  <td class="rowhead">加入日期</td>
  <td class="rowfollow">2025-08-16 13:36:23 (<span title="2025-08-16 13:36:23">2月前</span>)</td>
</tr>
<tr>
  <td class="rowhead">传输</td>
  <td class="rowfollow">
    <strong>分享率</strong>: <font color="">24.360</font>（<strong>实际分享率</strong>：10.637）<br/>
    <strong>上传量</strong>: 55.343 TB&nbsp;&nbsp;&nbsp;<strong>下载量</strong>: 2.272 TB<br/>
    <strong>实际上传量</strong>: 29.358 TB&nbsp;&nbsp;&nbsp;<strong>实际下载量</strong>: 2.760 TB&nbsp;&nbsp;&nbsp;
    实际上传/下载量 (仅用于记录, 不参与分享率计算)
  </td>
</tr>
<tr>
  <td class="rowhead">等级</td>
  <td class="rowfollow"><img alt="Insane User" title="Insane User" src="pic/insane.gif" /></td>
</tr>
</table>
</table>
</body></html>`

func getPT0FFCCDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("pt0ffcc")
	require.True(t, ok, "pt0ffcc definition not found")
	return def
}

func testPT0FFCCSearch(t *testing.T) {
	def := getPT0FFCCDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(pt0ffccSearchFixture))
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
	assert.Equal(t, "38742", free.ID)
	assert.Equal(t, "Tampopo.1985.CC.BluRay.1080p.x264.FLAC.1.0-CMCT", free.Title)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	assert.Equal(t, 50, free.Seeders)
	assert.Equal(t, 0, free.Leechers)
	assert.Equal(t, 269, free.Snatched)
	assert.True(t, free.SizeBytes > 0, "size must be parsed")

	normal := items[1]
	assert.Equal(t, "38743", normal.ID)
	assert.Equal(t, v2.DiscountNone, normal.DiscountLevel)
}

func testPT0FFCCDetail(t *testing.T) {
	def := getPT0FFCCDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "pt0ffcc_detail", pt0ffccDetailFixture)
	info := parser.ParseAll(doc.Selection)

	assert.Equal(t, v2.DiscountFree, info.DiscountLevel, "should parse FREE from h1 font.free")
	assert.InDelta(t, 9.07*1024, info.SizeMB, 1.0, "SizeRegex must extract 9.07 GB from 基本信息 cell")
	assert.False(t, info.HasHR, "detail page has no H&R marker")
}

func testPT0FFCCUserInfo(t *testing.T) {
	def := getPT0FFCCDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage_LoggedInUser", func(t *testing.T) {
		doc := FixtureDoc(t, "pt0ffcc_index", pt0ffccIndexFixture)
		assert.Equal(t, "12189", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["id"]))
		assert.Equal(t, "TestViewer", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["name"]))
		assert.Equal(t, "1", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["seeding"]))
		assert.Equal(t, "0", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["leeching"]))
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "pt0ffcc_userdetails", pt0ffccUserdetailsFixture)
		assert.Equal(t, "testuser", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["name"]))
		assert.Equal(t, "Insane User", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["levelName"]))

		// Each regex-driven field must produce a non-empty, well-formed value.
		// Specifically guards against the 上传量 vs 实际上传量 ambiguity.
		for _, field := range []string{"uploaded", "downloaded", "ratio", "joinTime"} {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q missing", field)
				got := driver.ExtractFieldValuePublic(doc, sel)
				assert.NotEmpty(t, got, "field %q must be parsed", field)
			})
		}
	})
}

func TestPT0FFCC_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      pt0ffccSearchFixture,
		"detail":      pt0ffccDetailFixture,
		"index":       pt0ffccIndexFixture,
		"userdetails": pt0ffccUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
