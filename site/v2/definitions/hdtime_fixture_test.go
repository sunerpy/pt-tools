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
		SiteID:   "hdtime",
		Search:   testHDTimeSearch,
		Detail:   testHDTimeDetail,
		UserInfo: testHDTimeUserInfo,
	})
}

const hdtimeSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=401"><img alt="电影" title="电影" /></a></td>
  <td class="rowfollow"><table class="torrentname" width="100%"><tr><td class="embedded"><a href="details.php?id=173135&amp;hit=1"><b>Sample.Movie.2026.1080p.WEB-DL</b></a> <img class="pro_twouphalfdown" src="pic/trans.gif" alt="promo" /><img class="pro_free2up" src="pic/trans.gif" alt="2X Free" /><br /><span>示例电影 / Sample Movie</span></td></tr></table></td>
  <td class="rowfollow"><a href="comment.php?action=add&amp;pid=173135&amp;type=torrent">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-01-30 08:48:12">1月<br />26天</span></td>
  <td class="rowfollow">7.73<br />GB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=173135&amp;hit=1&amp;dllist=1#seeders">1</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=173135&amp;hit=1&amp;dllist=1#leechers">34</a></b></td>
  <td class="rowfollow"><a href="viewsnatches.php?id=173135"><b>16</b></a></td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=405"><img alt="动漫" title="动漫" /></a></td>
  <td class="rowfollow"><table class="torrentname" width="100%"><tr><td class="embedded"><a href="details.php?id=187003&amp;hit=1"><b>Sample.Animation.2026.S01E01</b></a> <img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this,event,'content','&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-03-28 11:39:47&quot;&gt;23时57分钟&lt;/span&gt;&lt;/b&gt;')" /><br /><span>示例动画</span></td></tr></table></td>
  <td class="rowfollow"><a href="comment.php?action=add&amp;pid=187003&amp;type=torrent">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-03-27 11:39:47">2分钟</span></td>
  <td class="rowfollow">523.84<br />MB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=187003&amp;hit=1&amp;dllist=1#seeders">1</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=187003&amp;hit=1&amp;dllist=1#leechers">12</a></b></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table>
</body></html>`

const hdtimeUserinfoFixture = `<html><body>
<table><tr>
<td class="bottom" align="left"><span class="medium">
欢迎回来, <span class="nowrap"><a href="https://hdtime.org/userdetails.php?id=106612" class="User_Name"><b>alpha</b></a></span>
<font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 4567.8
<font class="color_ratio">分享率:</font> 12.34
<font class='color_uploaded'>上传量:</font> 10.50 GB
<font class='color_downloaded'> 下载量:</font> 2.00 GB
<font class='color_active'>当前活动:</font> <img class="arrowup" src="pic/trans.gif" />8 <img class="arrowdown" src="pic/trans.gif" />1
</span></td>
</tr></table>
<table width="100%" border="1" cellspacing="0" cellpadding="5">
<tr><td class="rowhead nowrap">用户ID/UID</td><td class="rowfollow">106612</td></tr>
<tr><td class="rowhead nowrap">加入日期</td><td class="rowfollow">2025-05-03 13:26:17 (<span title="2025-05-03 13:26:17">10月前</span>)</td></tr>
<tr><td class="rowhead nowrap">传输</td><td class="rowfollow"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>: 12.34</td></tr><tr><td class="embedded"><strong>上传量</strong>: 10.50 GB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>: 2.00 GB</td></tr></table></td></tr>
<tr><td class="rowhead nowrap">等级</td><td class="rowfollow"><img title="Veteran User" src="pic/veteran.gif" /></td></tr>
</table>
</body></html>`

const hdtimeDetailFixture = `<html><body>
<h1 align="center" id="top">Sample.Movie.2026.1080p.WEB-DL <b>[<font class='twoupfree'>2X免费</font>]</b> <font color='#00CC66'>剩余时间：<span title="2026-03-28 22:39:54">1天10时</span></font></h1>
<table width="97%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead">下载</td><td class="rowfollow"><a class="index" href="download.php?id=173135">[HDTime].sample.torrent</a></td></tr>
<tr><td class="rowhead nowrap">副标题</td><td class="rowfollow">示例副标题</td></tr>
<tr><td class="rowhead nowrap">基本信息</td><td class="rowfollow"><b><b>大小：</b></b>7.73 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;电影</td></tr>
</table>
</body></html>`

const hdtimeDetailWithHRFixture = `<html><body>
<h1 align="center" id="top">Sample.Movie.HR</h1>
<table width="97%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead">下载</td><td class="rowfollow"><a class="index" href="download.php?id=173136">[HDTime].sample.hr.torrent</a></td></tr>
<tr><td class="rowhead nowrap">基本信息</td><td class="rowfollow"><b><b>大小：</b></b>2.00 GB</td></tr>
</table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

func getHDTimeDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("hdtime")
	require.True(t, ok)
	return def
}

func testHDTimeSearch(t *testing.T) {
	def := getHDTimeDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(hdtimeSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "173135", items[0].ID)
	assert.Equal(t, "Sample.Movie.2026.1080p.WEB-DL", items[0].Title)
	assert.Equal(t, "示例电影 / Sample Movie", items[0].Subtitle)
	assert.Equal(t, v2.Discount2xFree, items[0].DiscountLevel)
	assert.Equal(t, 1, items[0].Seeders)
	assert.Equal(t, 34, items[0].Leechers)
	assert.Equal(t, 16, items[0].Snatched)

	assert.Equal(t, "187003", items[1].ID)
	assert.Equal(t, v2.DiscountFree, items[1].DiscountLevel)
}

func testHDTimeDetail(t *testing.T) {
	def := getHDTimeDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	info := parser.ParseAll(FixtureDoc(t, "hdtime_detail", hdtimeDetailFixture).Selection)
	assert.Equal(t, v2.Discount2xFree, info.DiscountLevel)
	assert.InDelta(t, 7.73*1024, info.SizeMB, 0.1)

	hrInfo := parser.ParseAll(FixtureDoc(t, "hdtime_detail_hr", hdtimeDetailWithHRFixture).Selection)
	assert.True(t, hrInfo.HasHR)
	assert.InDelta(t, 2.0*1024, hrInfo.SizeMB, 0.1)
}

func testHDTimeUserInfo(t *testing.T) {
	def := getHDTimeDef(t)
	driver := newTestNexusPHPDriver(def)

	doc := FixtureDoc(t, "hdtime_userinfo", hdtimeUserinfoFixture)
	assert.Equal(t, "106612", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "alpha", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "4567.8", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "8", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "1", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["leeching"]))
	assert.Equal(t, "11274289152", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "2147483648", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "12.34", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "Veteran User", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "1746278777", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["joinTime"]))
}

func TestHDTime_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":    hdtimeSearchFixture,
		"userinfo":  hdtimeUserinfoFixture,
		"detail":    hdtimeDetailFixture,
		"detail_hr": hdtimeDetailWithHRFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
