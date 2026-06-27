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
		SiteID:   "pthome",
		Search:   testPTHomeSearch,
		Detail:   testPTHomeDetail,
		UserInfo: testPTHomeUserInfo,
	})
}

const pthomeSearchFixture = `<html><body>
<table class="torrents torrents-table"><tbody>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=405"><img class="c_anime" src="pic/cattrans.gif" alt="动漫" title="动漫" /></a></td>
  <td class="rowfollow torrents-box"><div class="torrents-progress"></div><div class="torrents-name"><table class="torrentname"><tr><td class="embedded">
    <a title="Teepee Time S03 2017 1080p CBC WEB-DL" href="details.php?id=266721&amp;hit=1"><b>Teepee Time S03 2017 1080p CBC WEB-DL</b></a>
    <b> (<font class='new'>新</font>)</b>
    <img class="pro_free" src="pic/trans.gif" alt="Free" title="免费" />
    <br /><span>Teepee Time 第三季 全26集</span>
  </td></tr></table></div></td>
  <td class="rowfollow"><a href="comment.php?action=add&amp;pid=266721&amp;type=torrent">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-06-27 10:28:11">10分</span></td>
  <td class="rowfollow">12.13<br />GB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=266721&amp;dllist=1#seeders">1</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=266721&amp;dllist=1#leechers">1</a></b></td>
  <td class="rowfollow">0</td>
  <td>-</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=401"><img class="c_movie" src="pic/cattrans.gif" alt="电影" title="电影" /></a></td>
  <td class="rowfollow torrents-box"><div class="torrents-progress"></div><div class="torrents-name"><table class="torrentname"><tr><td class="embedded">
    <a title="La fille de d'Artagnan 1994 1080p Blu-ray" href="details.php?id=266706&amp;hit=1"><b>La fille de d'Artagnan 1994 1080p Blu-ray</b></a>
    <img class="pro_50pctdown" src="pic/trans.gif" alt="50%" title="50%" />
    <br /><span>豪情玫瑰</span>
  </td></tr></table></div></td>
  <td class="rowfollow"><a href="comment.php?action=add&amp;pid=266706&amp;type=torrent">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-06-27 05:21:18">5时</span></td>
  <td class="rowfollow">32.24<br />GB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=266706&amp;dllist=1#seeders">5</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=266706&amp;dllist=1#leechers">2</a></b></td>
  <td class="rowfollow">3</td>
  <td>-</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table>
</body></html>`

const pthomeDetailFixture = `<html><body>
<h1>
  <input type="hidden" name="torrent_name" value="La fille de d'Artagnan 1994 1080p Blu-ray AVC DTS-HD MA 5.1-FULLSiZE" />
  <input type="hidden" name="detail_torrent_id" value="266706" />
  La fille de d'Artagnan 1994 1080p Blu-ray AVC DTS-HD MA 5.1-FULLSiZE&nbsp;&nbsp;&nbsp;
  <b>[<font class='free' onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-06-29 05:21:18&quot;&gt;1天18时&lt;/span&gt;&lt;/b&gt;', 'trail', false);">免费</font>]</b>
</h1>
<table>
  <tr><td class="rowhead nowrap">基本信息</td><td class="rowfollow" colspan=""><b><b>大小：</b></b>32.24 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;电影</td></tr>
  <tr><td class="rowhead nowrap">副标题</td><td class="rowfollow" colspan="">豪情玫瑰 | La fille de d'Artagnan | 1994</td></tr>
</table>
</body></html>`

const pthomeIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td>
<font class="medium">欢迎回来</font>, <span class="nowrap"><a href="userdetails.php?id=131529" class='InsaneUser_Name'><b>testuser</b></a></span>
[<a href="logout.php">退出</a>] [<a href="torrents.php?inclbookmarked=1&amp;allsec=1&amp;incldead=0">收藏</a>]
<font class="color_bonus">魔力值 </font>[<a href="mybonus.php">使用</a>]: 1,986,609.6&nbsp;(签到已得20)
<a href="/forums.php?action=viewtopic&amp;topicid=8153"><font style="color: #1900d1">做种积分：</font></a>2,232,801.6
<font class="color_invite">邀请 </font>[<a href="invite.php?id=131529">发送</a>]: 0/0<br/>
<font class="color_ratio">分享率：</font> 13.703
<font class='color_uploaded'>上传量：</font> 17.801 TB<font class='color_downloaded'> 下载量：</font> 1.299 TB
<font class="color_active">H&amp;R：</font><a href="myhr.php"> 0/<font style="color: red"> 0 </font></a>
<a href="/peerlist.php?userid=131529"><font class='color_active'>当前活动：</font></a> <img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif"/>2 <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif"/>0
</td></tr></table>
</body></html>`

// pthomeUserdetailsFixture models a user's OWN userdetails.php page. The real captured
// page was privacy-denied (target user hid their profile), so this fixture reconstructs
// the standard NexusPHP rows present on one's own page to validate the join/lastAccess/level
// selectors. The #info_block block is identical to index.php.
const pthomeUserdetailsFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td>
<font class="medium">欢迎回来</font>, <span class="nowrap"><a href="userdetails.php?id=131529" class='InsaneUser_Name'><b>testuser</b></a></span>
<font class="color_bonus">魔力值 </font>[<a href="mybonus.php">使用</a>]: 1,986,609.6&nbsp;(签到已得20)<br/>
<font class="color_ratio">分享率：</font> 13.703
<font class='color_uploaded'>上传量：</font> 17.801 TB<font class='color_downloaded'> 下载量：</font> 1.299 TB
<img class="arrowup" src="pic/trans.gif"/>2 <img class="arrowdown" src="pic/trans.gif"/>0
</td></tr></table>
<table>
  <tr><td class="rowhead nowrap">加入日期</td><td class="rowfollow">2024-05-10 08:30:00 (<span title="2024-05-10 08:30:00">2年前</span>)</td></tr>
  <tr><td class="rowhead nowrap">最近动向</td><td class="rowfollow">2026-06-27 10:39:07 (<span title="2026-06-27 10:39:07">&lt; 1分前</span>)</td></tr>
  <tr><td class="rowhead nowrap">等级</td><td class="rowfollow"><img alt="Insane User" title="Insane User" src="pic/insane.gif" /> </td></tr>
</table>
</body></html>`

func getPTHomeDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("pthome")
	require.True(t, ok, "pthome definition not found")
	return def
}

func testPTHomeSearch(t *testing.T) {
	def := getPTHomeDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(pthomeSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "266721", items[0].ID)
	assert.Equal(t, "Teepee Time S03 2017 1080p CBC WEB-DL", items[0].Title)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 1, items[0].Seeders)
	assert.Equal(t, 1, items[0].Leechers)
	assert.Equal(t, 0, items[0].Snatched)

	assert.Equal(t, "266706", items[1].ID)
	assert.Equal(t, v2.DiscountPercent50, items[1].DiscountLevel)
}

func testPTHomeDetail(t *testing.T) {
	def := getPTHomeDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "pthome_detail", pthomeDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "266706", info.TorrentID)
	assert.Equal(t, "La fille de d'Artagnan 1994 1080p Blu-ray AVC DTS-HD MA 5.1-FULLSiZE", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 32.24*1024, info.SizeMB, 0.1)
	assert.False(t, info.HasHR)
}

func testPTHomeUserInfo(t *testing.T) {
	def := getPTHomeDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "pthome_index", pthomeIndexFixture)
		fields := map[string]string{
			"id":           "131529",
			"name":         "testuser",
			"bonus":        "1.9866096e+06",
			"seedingBonus": "2.2328016e+06",
			"ratio":        "13.703",
			"uploaded":     "19572406486040",
			"downloaded":   "1428265604481",
			"seeding":      "2",
			"leeching":     "0",
		}
		for field, expected := range fields {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok, "selector %q missing", field)
			assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel), "field %q", field)
		}
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "pthome_userdetails", pthomeUserdetailsFixture)
		expected := map[string]string{
			"levelName":    "Insane User",
			"joinTime":     "1715301000",
			"lastAccessAt": "1782527947",
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

func TestPTHome_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      pthomeSearchFixture,
		"detail":      pthomeDetailFixture,
		"index":       pthomeIndexFixture,
		"userdetails": pthomeUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
