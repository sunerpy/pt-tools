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
		SiteID:   "ptlgs",
		Search:   testPtlgsSearch,
		Detail:   testPtlgsDetail,
		UserInfo: testPtlgsUserInfo,
	})
}

// --- Fixtures (anonymized, modeled on ptlgs.org real HTML — issue #377) ---

const ptlgsSearchFixture = `<html><body>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%">
<tr>
	<td class="colhead">类型</td><td class="colhead">标题</td><td class="colhead">评论</td>
	<td class="colhead">存活时间</td><td class="colhead">大小</td><td class="colhead">种子数</td>
	<td class="colhead">下载数</td><td class="colhead">完成数</td><td class="colhead">发布者</td>
</tr>
<tr>
	<td class="rowfollow nowrap"><a href="?cat=405"><img class="c_anime" alt="动漫" title="动漫" /></a></td>
	<td class="rowfollow" width="100%" align="left">
		<table class="torrentname" width="100%"><tr><td class="embedded">
			<a title="Test.Anime.S01.2014.1080p.WEB-DL.H265-GRP" href="details.php?id=73163&amp;hit=1"><b>Test.Anime.S01.2014.1080p.WEB-DL.H265-GRP</b></a>
			<img class="pro_free" alt="Free" title="免费" />
			<br />
			<span title="">中字</span><span title="">HDR</span>测试动画 / Test Anime / 第1季
		</td>
		<td class="embedded"><a href="download.php?id=73163"><img class="download" alt="download" title="下载本种" /></a></td>
		</tr></table>
	</td>
	<td class="rowfollow"><a href="comment.php?action=add&amp;pid=73163&amp;type=torrent">0</a></td>
	<td class="rowfollow nowrap"><span title="2025-12-12 16:41:39">5月<br />18天</span></td>
	<td class="rowfollow">4.75<br />GB</td>
	<td class="rowfollow" align="center"><b><a href="details.php?id=73163&amp;dllist=1#seeders">4</a></b></td>
	<td class="rowfollow">0</td>
	<td class="rowfollow"><a href="viewsnatches.php?id=73163"><b>11</b></a></td>
	<td class="rowfollow"><i>匿名</i></td>
</tr>
</table>
</body></html>`

const ptlgsDetailFixture = `<html><body>
<h1 align="center" id="top"><span class="name">Test.Movie.2012.S01.BDrip.1080p.HEVC-GRP</span>&nbsp;&nbsp;&nbsp; <b>[<font class='free' >免费</font>]</b><img class="hitandrun" alt="H&R" title="H&R" />
	<input name="torrent_name" type="hidden" value="Test.Movie.2012.S01.BDrip.1080p.HEVC-GRP" />
	<input name="detail_torrent_id" type="hidden" value="31850" />
</h1>
<table>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td>
	<td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>16.87 GB&nbsp;&nbsp;&nbsp;<b title='类型'>类型:</b>&nbsp;<span>动漫</span></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td>
	<td class="rowfollow" valign="top" align="left">测试电影 / Test Movie / 中日特效字幕</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">下载</td>
	<td class="rowfollow" valign="top" align="left"><a href="download.php?id=31850">下载种子</a></td></tr>
</table>
</body></html>`

const ptlgsUserInfoFixture = `<html><body>
<h1 style='margin:0px'><span class="nowrap"><b>testuser</b></span></h1>
<div id="info_block"><table><tr><td>
	<a href="userdetails.php?id=24292" class="User_Name"><b>testuser</b></a>
	<font class="color_ratio">分享率:</font> </font>3.50
	<font class='color_uploaded'>上传量:</font> 87.71 MB
	<font class='color_downloaded'> 下载量:</font> 0.00 KB
	<font class='color_active'>当前活动:</font> <img class="arrowup" alt="Torrents seeding" title="当前做种" />8 <img class="arrowdown" alt="Torrents leeching" title="当前下载" />0
</td></tr></table></div>
<table>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">用户ID/UID</td>
	<td width="99%" class="rowfollow" valign="top" align="left">24292</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">加入日期</td>
	<td width="99%" class="rowfollow" valign="top" align="left">2026-05-29 20:06:07 (<span title="2026-05-29 20:06:07">6时44分钟前</span>, 0周)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td>
	<td width="99%" class="rowfollow" valign="top" align="left">2026-05-30 02:50:39 (<span title="2026-05-30 02:50:39">&lt; 1分钟前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">传输</td>
	<td width="99%" class="rowfollow" valign="top" align="left"><table border="0"><tr>
		<td class="embedded"><strong>上传量</strong>: 87.71 MB</td>
		<td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>: 0.00 KB</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">工分</td>
	<td width="99%" class="rowfollow" valign="top" align="left">95.5</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">做种积分</td>
	<td width="99%" class="rowfollow" valign="top" align="left">30.7&nbsp;&nbsp;<span class='text-muted'>(更新时间: 2026-05-30 02:39:59)</span></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">等级</td>
	<td width="99%" class="rowfollow" valign="top" align="left"><img alt="User" title="User" src="pic/user.gif" /> </td></tr>
</table>
</body></html>`

// --- Tests ---

func getPtlgsDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("ptlgs")
	require.True(t, ok, "ptlgs definition not found")
	return def
}

func testPtlgsSearch(t *testing.T) {
	def := getPtlgsDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(ptlgsSearchFixture))
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
	require.Len(t, items, 1, "should parse 1 torrent row")

	item := items[0]
	assert.Equal(t, "73163", item.ID)
	assert.Equal(t, "Test.Anime.S01.2014.1080p.WEB-DL.H265-GRP", item.Title)
	assert.Equal(t, v2.DiscountFree, item.DiscountLevel)
	assert.Equal(t, 4, item.Seeders)
	assert.Equal(t, 0, item.Leechers)
	assert.Equal(t, 11, item.Snatched)
	assert.True(t, item.SizeBytes > 0, "size should be parsed")
}

func testPtlgsDetail(t *testing.T) {
	def := getPtlgsDef(t)

	doc := FixtureDoc(t, "ptlgs_detail", ptlgsDetailFixture)
	parser := v2.NewNexusPHPParserFromDefinition(def)
	info := parser.ParseAll(doc.Selection)

	assert.Equal(t, "Test.Movie.2012.S01.BDrip.1080p.HEVC-GRP", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 16.87*1024, info.SizeMB, 0.1)
	assert.True(t, info.HasHR, "should detect H&R from hitandrun icon")
}

func testPtlgsUserInfo(t *testing.T) {
	def := getPtlgsDef(t)
	driver := newTestNexusPHPDriver(def)

	doc := FixtureDoc(t, "ptlgs_userinfo", ptlgsUserInfoFixture)
	exact := map[string]string{
		"uploaded":     "91970600",
		"downloaded":   "0",
		"levelName":    "User",
		"joinTime":     "1780056367",
		"lastAccessAt": "1780080639",
	}
	for field, expected := range exact {
		t.Run(field, func(t *testing.T) {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok, "selector %q not found", field)
			got := driver.ExtractFieldValuePublic(doc, sel)
			assert.Equal(t, expected, got)
		})
	}

	t.Run("lastAccessNonZero", func(t *testing.T) {
		sel := def.UserInfo.Selectors["lastAccessAt"]
		got := driver.ExtractFieldValuePublic(doc, sel)
		assert.NotEmpty(t, got, "lastAccessAt must extract (保号 probe depends on UserInfo.LastAccess)")
		assert.NotEqual(t, "0", got)
	})
}

func TestPtlgs_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":   ptlgsSearchFixture,
		"detail":   ptlgsDetailFixture,
		"userinfo": ptlgsUserInfoFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
