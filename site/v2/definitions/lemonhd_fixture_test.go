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
		SiteID:   "lemonhd",
		Search:   testLemonHDSearch,
		Detail:   testLemonHDDetail,
		UserInfo: testLemonHDUserInfo,
	})
}

const lemonhdSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=406"><img class="c_tvseries" alt="TV Series" title="TV Series" /></a></td>
  <td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style='padding-left: 5px'><a title="The Tycoon S01 2025 2160p WEB-DL" href="details.php?id=19685&amp;hit=1"><b>The Tycoon S01 2025 2160p WEB-DL</b></a> <img class="pro_free" src="pic/trans.gif" alt="Free" title="免费" /><br /><span>大生意人 第一季</span></td><td class="embedded" style="text-align: right; width: 40px;padding: 4px"><a href="download.php?id=19685"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td></tr></table></td>
  <td class="rowfollow"><a href="comment.php?action=add&amp;pid=19685&amp;type=torrent" title="添加评论">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-01-12 09:40:07">2月<br />9天</span></td>
  <td class="rowfollow">62.62<br />GB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=19685&amp;hit=1&amp;dllist=1#seeders">37</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=19685&amp;hit=1&amp;dllist=1#leechers">1</a></b></td>
  <td class="rowfollow"><a href="viewsnatches.php?id=19685"><b>80</b></a></td>
  <td class="rowfollow"><span class="nowrap"><a href="https://lemonhd.net/userdetails.php?id=5868" class='InsaneUser_Name'><b>tester</b></a></span></td>
</tr>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=401"><img class="c_movies" alt="Movies" title="Movies" /></a></td>
  <td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style='padding-left: 5px'><a title="Lock Up 1989 UHD Blu-ray" href="details.php?id=12461&amp;hit=1"><b>Lock Up 1989 UHD Blu-ray</b></a> <img class="pro_free" src="pic/trans.gif" alt="Free" title="免费" /><br /><span>破茧威龙</span></td><td class="embedded" style="text-align: right; width: 40px;padding: 4px"><a href="download.php?id=12461"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td></tr></table></td>
  <td class="rowfollow"><a href="comment.php?action=add&amp;pid=12461&amp;type=torrent" title="添加评论">0</a></td>
  <td class="rowfollow nowrap"><span title="2025-08-01 08:59:06">7月<br />23天</span></td>
  <td class="rowfollow">66.68<br />GB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=12461&amp;hit=1&amp;dllist=1#seeders">49</a></b></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><a href="viewsnatches.php?id=12461"><b>164</b></a></td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table>
</body></html>`

const lemonhdUserinfoFixture = `<html><body>
<table><tr><td class="bottom" align="left"><span class="medium">
欢迎回来, <span class="nowrap"><a href="https://lemonhd.net/userdetails.php?id=6552" class='User_Name'><b>linkaaa</b></a></span>
<font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 4562.9
<font class="color_ratio">分享率:</font> 无限
<font class='color_uploaded'>上传量:</font> 39.77 GB
<font class='color_downloaded'> 下载量:</font> 0.00 KB
<font class='color_active'>当前活动:</font> <img class="arrowup" src="pic/trans.gif" />5 <img class="arrowdown" src="pic/trans.gif" />3
</span></td></tr></table>
<table width="100%" border="1" cellspacing="0" cellpadding="5">
<tr><td class="rowhead nowrap">用户ID/UID</td><td class="rowfollow">6086</td></tr>
<tr><td class="rowhead nowrap">加入日期</td><td class="rowfollow">2026-01-09 13:27:49 (<span title="2026-01-09 13:27:49">2月前</span>)</td></tr>
<tr><td class="rowhead nowrap">传输</td><td class="rowfollow"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>: ∞</td></tr><tr><td class="embedded"><strong>上传量</strong>: 39.77 GB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>: 0.00 KB</td></tr></table></td></tr>
<tr><td class="rowhead nowrap">等级</td><td class="rowfollow"><img title="User" src="pic/user.gif" /></td></tr>
</table>
</body></html>`

const lemonhdDetailFixture = `<html><body>
<h1 align="center" id="top">The Tycoon S01 2025 2160p WEB-DL H.265 DDP 5.1 Atmos-ADWeb <b>[<font class='free'>免费</font>]</b></h1>
<table width="97%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead">下载</td><td class="rowfollow"><a class="index" href="download.php?id=19685">[LemonHD].sample.torrent</a></td></tr>
<tr><td class="rowhead nowrap">副标题</td><td class="rowfollow">大生意人 第一季</td></tr>
<tr><td class="rowhead nowrap">基本信息</td><td class="rowfollow"><b><b>大小：</b></b>62.62 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;TV Series</td></tr>
</table>
</body></html>`

const lemonhdDetailWithHRFixture = `<html><body>
<h1 align="center" id="top">Lock Up 1989 UHD Blu-ray</h1>
<table width="97%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead">下载</td><td class="rowfollow"><a class="index" href="download.php?id=12461">[LemonHD].hr.torrent</a></td></tr>
<tr><td class="rowhead nowrap">基本信息</td><td class="rowfollow"><b><b>大小：</b></b>66.68 GB</td></tr>
</table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

func getLemonHDDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("lemonhd")
	require.True(t, ok)
	return def
}

func testLemonHDSearch(t *testing.T) {
	def := getLemonHDDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(lemonhdSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "19685", items[0].ID)
	assert.Equal(t, "The Tycoon S01 2025 2160p WEB-DL", items[0].Title)
	assert.Equal(t, "大生意人 第一季", items[0].Subtitle)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 37, items[0].Seeders)
	assert.Equal(t, 1, items[0].Leechers)
	assert.Equal(t, 80, items[0].Snatched)

	assert.Equal(t, "12461", items[1].ID)
	assert.Equal(t, v2.DiscountFree, items[1].DiscountLevel)
}

func testLemonHDDetail(t *testing.T) {
	def := getLemonHDDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	info := parser.ParseAll(FixtureDoc(t, "lemonhd_detail", lemonhdDetailFixture).Selection)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 62.62*1024, info.SizeMB, 0.1)

	hrInfo := parser.ParseAll(FixtureDoc(t, "lemonhd_detail_hr", lemonhdDetailWithHRFixture).Selection)
	assert.True(t, hrInfo.HasHR)
	assert.InDelta(t, 66.68*1024, hrInfo.SizeMB, 0.1)
}

func testLemonHDUserInfo(t *testing.T) {
	def := getLemonHDDef(t)
	driver := newTestNexusPHPDriver(def)

	doc := FixtureDoc(t, "lemonhd_userinfo", lemonhdUserinfoFixture)
	assert.Equal(t, "6552", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "linkaaa", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "4562.9", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "5", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "3", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["leeching"]))
	assert.Equal(t, "42702712340", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "User", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "1767965269", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["joinTime"]))
}

func TestLemonHD_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":    lemonhdSearchFixture,
		"userinfo":  lemonhdUserinfoFixture,
		"detail":    lemonhdDetailFixture,
		"detail_hr": lemonhdDetailWithHRFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
