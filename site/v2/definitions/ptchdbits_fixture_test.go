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
		SiteID:   "ptchdbits",
		Search:   testPTCHDBitsSearch,
		Detail:   testPTCHDBitsDetail,
		UserInfo: testPTCHDBitsUserInfo,
	})
}

const ptchdbitsSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=401"><img class="c_movie" src="pic/cattrans.gif" alt="Movies" title="Movies" /></a></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a title="Wasteman.2025.REMUX.Bluray.1080p.AVC.DTS-HDMA5.1-CHD" href="details.php?id=559886&amp;hit=1"><b>Wasteman.2025.REMUX.Bluray.1080p.AVC.DTS-HDMA5.1-CHD</b></a>
    <b> (<font class='new'>新</font>)</b>
    <img class="pro_free" src="pic/trans.gif" alt="Free" title="免费" /> (限时<span title="2026-06-28 09:11:50">22时38分</span>)
    <br /><font class='subtitle'>废人 Wasteman</font>
  </td></tr></table></td>
  <td class="rowfollow"><a href="comment.php?action=add&amp;pid=559886&amp;type=torrent">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-06-26 09:11:50">1天</span></td>
  <td class="rowfollow">28.35<br />GB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=559886&amp;dllist=1#seeders">15</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=559886&amp;dllist=1#leechers">3</a></b></td>
  <td class="rowfollow"><a href="viewsnatches.php?id=559886"><b>22</b></a></td>
  <td class="rowfollow"><i>匿名</i></td>
  <td class="rowfollow">-</td>
</tr>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=401"><img class="c_movie" src="pic/cattrans.gif" alt="Movies" title="Movies" /></a></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a title="Sample.2024.1080p.BluRay-CHD" href="details.php?id=559800&amp;hit=1"><b>Sample.2024.1080p.BluRay-CHD</b></a>
    <img class="pro_50pctdown" src="pic/trans.gif" alt="50%" title="50%" />
    <br /><font class='subtitle'>样品</font>
  </td></tr></table></td>
  <td class="rowfollow"><a href="comment.php?action=add&amp;pid=559800&amp;type=torrent">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-06-25 09:11:50">2天</span></td>
  <td class="rowfollow">10.00<br />GB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=559800&amp;dllist=1#seeders">8</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=559800&amp;dllist=1#leechers">1</a></b></td>
  <td class="rowfollow"><a href="viewsnatches.php?id=559800"><b>5</b></a></td>
  <td class="rowfollow"><i>匿名</i></td>
  <td class="rowfollow">-</td>
</tr>
</tbody></table>
</body></html>`

const ptchdbitsDetailFixture = `<html><body>
<h1>
  <input type="hidden" name="torrent_name" value="Wasteman.2025.REMUX.Bluray.1080p.AVC.DTS-HDMA5.1-CHD" />
  <input type="hidden" name="detail_torrent_id" value="559886" />
  Wasteman.2025.REMUX.Bluray.1080p.AVC.DTS-HDMA5.1-CHD&nbsp;&nbsp;&nbsp;
  <img class="pro_free" src="pic/trans.gif" alt="Free" title="免费" /> (限时<span title="2026-06-28 09:11:50">22时38分</span>)
</h1>
<table>
  <tr><td class="rowhead nowrap">基本信息</td><td class="rowfollow"><b><b>大小：</b></b>28.35 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;Movies</td></tr>
  <tr><td class="rowhead nowrap">副标题</td><td class="rowfollow">废人 Wasteman</td></tr>
</table>
</body></html>`

const ptchdbitsIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td>
<span class="medium">欢迎回来, <span class="nowrap"><a href="userdetails.php?id=126666" class='VeteranUser_Name'><b>testuser</b></a></span>
[<a href="logout.php">退出</a>] [<a href="getrss.php">RSS下载</a>]
<font class = 'color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 2,228,351.0
<font class = 'color_invite'>邀请 </font>[<a href="invite.php?id=126666">发送</a>]: 0
<a href='hnr.php?id=126666'><font class = 'color_bonus'>H&R:</font></a>0<br />
<font class="color_ratio">分享率：</font> 36.716
<font class='color_uploaded'>上传量：</font> 55.431 TB<font class='color_downloaded'> 下载量：</font> 1.510 TB
<font class='color_active'>当前活动：</font> <img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />4 <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />0&nbsp;&nbsp;
<font class='color_connectable'>可连接：</font>未知 <font class='color_slots'>连接数：</font>无限制
<font class="color_ratio">做种积分: </font>1664472.0</span>
</td></tr></table>
</body></html>`

const ptchdbitsUserdetailsFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td>
<span class="medium">欢迎回来, <span class="nowrap"><a href="userdetails.php?id=126666" class='VeteranUser_Name'><b>testuser</b></a></span>
<font class = 'color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 2,228,351.0<br />
<font class="color_ratio">分享率：</font> 36.716
<font class='color_uploaded'>上传量：</font> 55.431 TB<font class='color_downloaded'> 下载量：</font> 1.510 TB
<font class='color_active'>当前活动：</font> <img class="arrowup" src="pic/trans.gif" />4 <img class="arrowdown" src="pic/trans.gif" />0</span>
</td></tr></table>
<table>
  <tr><td class="rowhead nowrap">加入日期</td><td class="rowfollow">2022-11-30 19:59:28 (<span title="2022-11-30 19:59:28">3年7月前</span>)</td></tr>
  <tr><td class="rowhead nowrap">最近动向</td><td class="rowfollow">2026-06-27 10:33:44 (<span title="2026-06-27 10:33:44">&lt; 1分前</span>)</td></tr>
  <tr><td class="rowhead nowrap">等级</td><td class="rowfollow"><img alt="(高烧)Veteran User" title="(高烧)Veteran User" src="pic/veteran.gif" /> </td></tr>
</table>
</body></html>`

func getPTCHDBitsDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("ptchdbits")
	require.True(t, ok, "ptchdbits definition not found")
	return def
}

func testPTCHDBitsSearch(t *testing.T) {
	def := getPTCHDBitsDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(ptchdbitsSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "559886", items[0].ID)
	assert.Equal(t, "Wasteman.2025.REMUX.Bluray.1080p.AVC.DTS-HDMA5.1-CHD", items[0].Title)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 15, items[0].Seeders)
	assert.Equal(t, 3, items[0].Leechers)
	assert.Equal(t, 22, items[0].Snatched)

	assert.Equal(t, "559800", items[1].ID)
	assert.Equal(t, v2.DiscountPercent50, items[1].DiscountLevel)
}

func testPTCHDBitsDetail(t *testing.T) {
	def := getPTCHDBitsDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "ptchdbits_detail", ptchdbitsDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "559886", info.TorrentID)
	assert.Equal(t, "Wasteman.2025.REMUX.Bluray.1080p.AVC.DTS-HDMA5.1-CHD", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 28.35*1024, info.SizeMB, 0.1)
	assert.False(t, info.HasHR)
}

func testPTCHDBitsUserInfo(t *testing.T) {
	def := getPTCHDBitsDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "ptchdbits_index", ptchdbitsIndexFixture)
		fields := map[string]string{
			"id":           "126666",
			"name":         "testuser",
			"bonus":        "2.228351e+06",
			"seedingBonus": "1.664472e+06",
			"ratio":        "36.716",
			"uploaded":     "60947029039251",
			"downloaded":   "1660262557941",
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
		doc := FixtureDoc(t, "ptchdbits_userdetails", ptchdbitsUserdetailsFixture)
		expected := map[string]string{
			"levelName":    "(高烧)Veteran User",
			"joinTime":     "1669809568",
			"lastAccessAt": "1782527624",
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

func TestPTCHDBits_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      ptchdbitsSearchFixture,
		"detail":      ptchdbitsDetailFixture,
		"index":       ptchdbitsIndexFixture,
		"userdetails": ptchdbitsUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
