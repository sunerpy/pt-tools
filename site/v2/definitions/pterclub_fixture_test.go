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
		SiteID:   "pterclub",
		Search:   testPTerClubSearch,
		Detail:   testPTerClubDetail,
		UserInfo: testPTerClubUserInfo,
	})
}

const pterclubSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=402"><img class="c_tvseries" src="pic/cattrans.gif" alt="电视剧" title="电视剧" /></a></td>
  <td class="rowfollow"><table class="torrentname"><tr>
    <td class="embedded"><img class="lozad" src="/pic/spinner.svg" alt="preview" /></td>
    <td class="embedded"><div><a title="The First Jasmine 2026 S01E34 2160p WEB-DL" href="details.php?id=844790"><b>&nbsp;The First Jasmine 2026 S01E34 2160p WEB-DL</b></a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" title="免费" />
    <br /><span class="torrents-subtitle">莫离/盛世嫡妃 第34集</span></div></td>
  </tr></table></td>
  <td class="rowfollow"><a href="comment.php?action=add&amp;pid=844790&amp;type=torrent">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-06-26 22:15:05">12时</span></td>
  <td class="rowfollow">4.66<br />GB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=844790&amp;dllist=1#seeders">30</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=844790&amp;dllist=1#leechers">5</a></b></td>
  <td class="rowfollow"><a href="viewsnatches.php?id=844790"><b>12</b></a></td>
  <td class="rowfollow">-</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=401"><img class="c_movie" src="pic/cattrans.gif" alt="电影" title="电影" /></a></td>
  <td class="rowfollow"><table class="torrentname"><tr>
    <td class="embedded"><img class="lozad" src="/pic/spinner.svg" alt="preview" /></td>
    <td class="embedded"><div><a title="Sample Movie 2025 1080p" href="details.php?id=844700"><b>&nbsp;Sample Movie 2025 1080p</b></a>
    <br /><span class="torrents-subtitle">样品电影</span></div></td>
  </tr></table></td>
  <td class="rowfollow"><a href="comment.php?action=add&amp;pid=844700&amp;type=torrent">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-06-25 22:15:05">1天</span></td>
  <td class="rowfollow">8.20<br />GB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=844700&amp;dllist=1#seeders">10</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=844700&amp;dllist=1#leechers">0</a></b></td>
  <td class="rowfollow"><a href="viewsnatches.php?id=844700"><b>3</b></a></td>
  <td class="rowfollow">-</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table>
</body></html>`

const pterclubDetailFixture = `<html><body>
<h1>
  <input type="hidden" name="torrent_name" value="The First Jasmine 2026 S01E34 2160p WEB-DL" />
  <input type="hidden" name="detail_torrent_id" value="844790" />
  The First Jasmine 2026 S01E34 2160p WEB-DL&nbsp;&nbsp;&nbsp;
  <b>[<font class='free' onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-06-27 22:15:05&quot;&gt;11时38分&lt;/span&gt;&lt;/b&gt;', 'trail', false);">免费</font>]</b>
</h1>
<table>
  <tr><td class="rowhead nowrap">基本信息</td><td class="rowfollow"><b><b>大小：</b></b>4.66 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;电视剧 (TV Series)</td></tr>
  <tr><td class="rowhead nowrap">副标题</td><td class="rowfollow">莫离/盛世嫡妃 第34集 | 导演: 林玉芬 [国语/中字]</td></tr>
</table>
</body></html>`

const pterclubIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td>
<span class="medium">欢迎回来, <span class="nowrap"><a href="userdetails.php?id=19860" class='EliteUser_Name'><b>testuser</b></a></span>
[<a href="#" data-url="logout.php">退出</a>] [<a href="torrents.php?inclbookmarked=1&amp;allsec=1&amp;incldead=0">收藏</a>]
<a title="点击查看每小时收益预测" href="mybonus.php#bonus-sum"> <span class="color_bonus">猫粮 </span> </a> [<a href="mybonus.php">使用</a> | <a href="sitefreepool.php">站免池</a>]: 1,452,469.7
<font class="color_invite">邀请 </font> [<a href="invite.php?id=19860">发送</a>]: 0/0<br />
<font class="color_ratio">分享率：</font> 15.802
<font class='color_uploaded'>上传量：</font> 8.785 TB<font class='color_downloaded'> 下载量：</font> 569.27 GB
<span class="color_bonus">做种积分：</span> 1,333,648.2
<font class='color_active'>当前活动：</font> <a href="getusertorrentlist.php?userid=19860&amp;type=seeding"> <img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" /> 4</a> <a href="getusertorrentlist.php?userid=19860&amp;type=leeching"> <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" /> 0</a>
</span>
</td></tr></table>
</body></html>`

const pterclubUserdetailsFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td>
<span class="medium">欢迎回来, <span class="nowrap"><a href="userdetails.php?id=19860" class='EliteUser_Name'><b>testuser</b></a></span>
<a href="mybonus.php#bonus-sum"> <span class="color_bonus">猫粮 </span> </a> [<a href="mybonus.php">使用</a> | <a href="sitefreepool.php">站免池</a>]: 1,452,469.7<br />
<font class="color_ratio">分享率：</font> 15.802
<font class='color_uploaded'>上传量：</font> 8.785 TB<font class='color_downloaded'> 下载量：</font> 569.27 GB
<font class='color_active'>当前活动：</font> <a href="x"> <img class="arrowup" src="pic/trans.gif" /> 4</a> <a href="y"> <img class="arrowdown" src="pic/trans.gif" /> 0</a></span>
</td></tr></table>
<table>
  <tr><td class="rowhead nowrap">加入日期</td><td class="rowfollow">2023-03-08 13:07:05 (<span title="2023-03-08 13:07:05">3年4月前</span>)</td></tr>
  <tr><td class="rowhead nowrap">最近动向</td><td class="rowfollow">2026-06-27 10:36:57 (<span title="2026-06-27 10:36:57">&lt; 1分前</span>)</td></tr>
  <tr><td class="rowhead nowrap"><a href='https://wiki.pterclub.net/wiki/x'>等级</a></td><td class="rowfollow"><img alt="布偶猫 ELITE USER" title="布偶猫 ELITE USER" src="pic/user_class/elite.png" /> </td></tr>
</table>
</body></html>`

func getPTerClubDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("pterclub")
	require.True(t, ok, "pterclub definition not found")
	return def
}

func testPTerClubSearch(t *testing.T) {
	def := getPTerClubDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(pterclubSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "844790", items[0].ID)
	assert.Equal(t, "The First Jasmine 2026 S01E34 2160p WEB-DL", items[0].Title)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 30, items[0].Seeders)
	assert.Equal(t, 5, items[0].Leechers)
	assert.Equal(t, 12, items[0].Snatched)

	assert.Equal(t, "844700", items[1].ID)
	assert.Equal(t, v2.DiscountNone, items[1].DiscountLevel)
}

func testPTerClubDetail(t *testing.T) {
	def := getPTerClubDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "pterclub_detail", pterclubDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "844790", info.TorrentID)
	assert.Equal(t, "The First Jasmine 2026 S01E34 2160p WEB-DL", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 4.66*1024, info.SizeMB, 0.1)
	assert.False(t, info.HasHR)
}

func testPTerClubUserInfo(t *testing.T) {
	def := getPTerClubDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "pterclub_index", pterclubIndexFixture)
		fields := map[string]string{
			"id":           "19860",
			"name":         "testuser",
			"bonus":        "1.4524697e+06",
			"seedingBonus": "1.3336482e+06",
			"ratio":        "15.802",
			"uploaded":     "9659209650012",
			"downloaded":   "611249008148",
			"seeding":      "4",
			"leeching":     "0",
		}
		for field, expected := range fields {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok, "selector %q missing", field)
			assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel), "field %q", field)
		}
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "pterclub_userdetails", pterclubUserdetailsFixture)
		expected := map[string]string{
			"levelName":    "布偶猫 ELITE USER",
			"joinTime":     "1678252025",
			"lastAccessAt": "1782527817",
		}
		for field, want := range expected {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok, "selector %q missing", field)
			assert.Equal(t, want, driver.ExtractFieldValuePublic(doc, sel), "field %q", field)
		}
		laSel := def.UserInfo.Selectors["lastAccessAt"]
		assert.NotEmpty(t, driver.ExtractFieldValuePublic(doc, laSel))
	})
}

func TestPTerClub_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      pterclubSearchFixture,
		"detail":      pterclubDetailFixture,
		"index":       pterclubIndexFixture,
		"userdetails": pterclubUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
