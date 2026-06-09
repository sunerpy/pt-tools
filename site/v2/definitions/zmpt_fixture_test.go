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
		SiteID:   "zmpt",
		Search:   testZmptSearch,
		Detail:   testZmptDetail,
		UserInfo: testZmptUserInfo,
	})
}

// --- Fixtures (anonymized, modeled on zmpt.cc real HTML) ---

const zmptSearchFixture = `<html><body>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%">
<tr>
	<td class="colhead">类型</td><td class="colhead">标题</td><td class="colhead">评论</td>
	<td class="colhead">存活时间</td><td class="colhead">大小</td><td class="colhead">种子数</td>
	<td class="colhead">下载数</td><td class="colhead">完成数</td><td class="colhead">发布者</td>
</tr>
<tr>
	<td class="rowfollow nowrap"><a href="?cat=404"><img class="c_anime" alt="动漫" title="动漫" /></a></td>
	<td class="rowfollow" width="100%" align="left">
		<table class="torrentname" width="100%"><tr><td class="embedded">
			<a title="Test.Anime.S01.2025.2160p.WEB-DL.H264-GRP" href="details.php?id=343859&amp;hit=1"><b>Test.Anime.S01.2025.2160p.WEB-DL.H264-GRP</b></a>
			<img class="pro_free" src="pic/trans.gif" alt="Free" />
			<font color='#0000FF'>剩余时间：<span title="2026-06-06 19:04:17">4时14分钟</span></font>
			<br />
			<span title="">国语</span><span title="">中字</span>测试动画 / Test Anime
		</td>
		<td class="embedded"><a href="download.php?id=343859"><img class="download" alt="download" title="下载" /></a></td>
		</tr></table>
	</td>
	<td class="rowfollow"><a href="comment.php?action=add&amp;pid=343859">0</a></td>
	<td class="rowfollow nowrap"><span title="2025-12-01 10:20:30">6月<br />5天</span></td>
	<td class="rowfollow">344.65<br />GB</td>
	<td class="rowfollow" align="center"><b><a href="details.php?id=343859&amp;dllist=1#seeders">42</a></b></td>
	<td class="rowfollow">3</td>
	<td class="rowfollow"><a href="viewsnatches.php?id=343859"><b>120</b></a></td>
	<td class="rowfollow"><i>匿名</i></td>
</tr>
</table>
</body></html>`

const zmptDetailFixture = `<html><body>
<h1 align="center" id="top">Test.Anime.S01.2025.2160p.WEB-DL.H264-GRP&nbsp;&nbsp;&nbsp; <b>[<font class='free' >免费</font>]</b> <font color='#0000FF'>剩余时间：<span title="2026-06-06 19:04:17">4时14分钟</span></font>
	<input name="torrent_name" type="hidden" value="Test.Anime.S01.2025.2160p.WEB-DL.H264-GRP" />
	<input name="detail_torrent_id" type="hidden" value="343859" />
</h1>
<table>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td>
	<td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>344.65 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;动漫 / Anime</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td>
	<td class="rowfollow" valign="top" align="left">测试动画 / Test Anime 国语中字</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">下载</td>
	<td class="rowfollow" valign="top" align="left"><a href="download.php?id=343859">下载种子</a></td></tr>
</table>
</body></html>`

// zmptIndexFixture models the top-of-page #info_block (logged-in user). ZmPT
// uses color-class fonts and labels bonus as「电力值」.
const zmptIndexFixture = `<html><body>
<table id="info_block"><tr><td>
	<span class="nowrap"><a href="https://zmpt.cc/userdetails.php?id=23124" class='User_Name'><b>testuser</b></a></span>
	<font class="color_ratio"> 分享率: </font> 12.197
	<font class='color_uploaded'> 上传量: </font> 248.64 GB
	<font class='color_downloaded'> 下载量: </font> 20.38 GB
	<font class='color_active'> 当前活动: </font> <img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />33 <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />0
	<a href="mybonus.php" id="self_bonus"><font class='color_bonus'> 电力值 </font> 1,474,397.2</a>
</td></tr></table>
</body></html>`

// zmptUserdetailsFixture models the SELF userdetails.php main table rows.
// (The captured ZIP's userdetails was a privacy-protected OTHER user, so these
// standard NexusPHP rows are modeled from the 织梦/NexusPHP template.)
const zmptUserdetailsFixture = `<html><body>
<table>
<tr><td class="rowhead nowrap">等级</td><td class="rowfollow"><img alt="User" title="User" src="pic/user.gif" /></td></tr>
<tr><td class="rowhead nowrap">加入日期</td><td class="rowfollow">2024-03-10 09:15:00 (<span title="2024-03-10 09:15:00">2年前</span>)</td></tr>
<tr><td class="rowhead nowrap">最近动向</td><td class="rowfollow">2026-06-06 14:30:00 (<span title="2026-06-06 14:30:00">&lt; 1分钟前</span>)</td></tr>
</table>
</body></html>`

// --- Tests ---

func getZmptDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("zmpt")
	require.True(t, ok, "zmpt definition not found")
	return def
}

func testZmptSearch(t *testing.T) {
	def := getZmptDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(zmptSearchFixture))
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
	require.Len(t, items, 1)

	item := items[0]
	assert.Equal(t, "343859", item.ID)
	assert.Equal(t, "Test.Anime.S01.2025.2160p.WEB-DL.H264-GRP", item.Title)
	assert.Equal(t, v2.DiscountFree, item.DiscountLevel)
	assert.Equal(t, 42, item.Seeders)
	assert.Equal(t, 3, item.Leechers)
	assert.Equal(t, 120, item.Snatched)
	assert.True(t, item.SizeBytes > 0, "size should be parsed")
}

func testZmptDetail(t *testing.T) {
	def := getZmptDef(t)

	doc := FixtureDoc(t, "zmpt_detail", zmptDetailFixture)
	parser := v2.NewNexusPHPParserFromDefinition(def)
	info := parser.ParseAll(doc.Selection)

	assert.Equal(t, "Test.Anime.S01.2025.2160p.WEB-DL.H264-GRP", info.Title)
	assert.Equal(t, "343859", info.TorrentID)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 344.65*1024, info.SizeMB, 0.1)
}

func testZmptUserInfo(t *testing.T) {
	def := getZmptDef(t)
	driver := newTestNexusPHPDriver(def)

	// Fields from index.php #info_block.
	indexDoc := FixtureDoc(t, "zmpt_index", zmptIndexFixture)
	indexFields := map[string]string{
		"id":         "23124",
		"name":       "testuser",
		"ratio":      "12.197",
		"seeding":    "33",
		"leeching":   "0",
		"uploaded":   "266975167119",
		"downloaded": "21882858373",
		"bonus":      "1.4743972e+06",
	}
	for field, expected := range indexFields {
		t.Run(field, func(t *testing.T) {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok, "selector %q not found", field)
			assert.Equal(t, expected, driver.ExtractFieldValuePublic(indexDoc, sel))
		})
	}

	// Fields from userdetails.php main table.
	userDoc := FixtureDoc(t, "zmpt_userdetails", zmptUserdetailsFixture)
	t.Run("levelName", func(t *testing.T) {
		assert.Equal(t, "User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	})
	t.Run("lastAccessAt", func(t *testing.T) {
		got := driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["lastAccessAt"])
		assert.NotEmpty(t, got, "lastAccessAt must parse (保号 probe depends on UserInfo.LastAccess)")
		assert.NotEqual(t, "0", got)
	})
	t.Run("joinTime", func(t *testing.T) {
		got := driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["joinTime"])
		assert.NotEmpty(t, got)
		assert.NotEqual(t, "0", got)
	})
}

func TestZmpt_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      zmptSearchFixture,
		"detail":      zmptDetailFixture,
		"index":       zmptIndexFixture,
		"userdetails": zmptUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
