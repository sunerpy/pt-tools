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
		SiteID:   "piggo",
		Search:   testPigGoSearch,
		Detail:   testPigGoDetail,
		UserInfo: testPigGoUserInfo,
	})
}

// piggoSearchFixture mirrors the real torrents.php structure from issue #532:
// nine columns, the torrentname table with no explicit <tr>, the promotion icon carrying
// both an onmouseover tooltip and a plain 剩余时间 span, coloured tag badges, and the real
// subtitle as a bare text node after the last badge.
const piggoSearchFixture = `<html><body>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%"><tbody>
<tr>
  <td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=402"><img class="c_tvseries" src="pic/cattrans.gif" alt="电视剧" title="电视剧" /></a><img class="c_webdl4ksdr" src="pic/cattrans.gif" alt="WEB-DL" title="WEB-DL" /></td>
  <td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><td class="embedded" style="text-align: center;width: 76px"><img src="pic/piggo404.png" class="nexus-lazy-load" style="max-height: 70px" /></td><td class="embedded" style='padding-left: 5px'><a title="Sample.Show.S01.2026.2160p.WEB-DL.H.265.HDR.AAC-Demo" href="details.php?id=57052&amp;hit=1"><b>Sample.Show.S01.2026.2160p.WEB-DL.H.265.HDR.AAC-Demo</b></a> <img class="pro_50pctdown" src="pic/trans.gif" alt="50%" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;halfdown&quot;&gt;50%&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2027-03-01 15:09:28&quot;&gt;5月25天&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500);" /> 剩余时间：<span title="2027-03-01 15:09:28">5月25天</span><span style="margin-left: 6px" title="通过"></span><br /><span style="background-color:#1e0330;color:#ffffff" title="">完结</span><span style="background-color:#6a3906;color:#ffffff" title="">国语</span><span style="background-color:#006400;color:#ffffff" title="">中字</span>示例剧集 第一季 全30集 | 类型：剧情 | 主演：演员甲 演员乙</td><td class="embedded" style="text-align: right"><span data-doubanid="00000000">N/A</span></td><td width="20" class="embedded" valign="middle"><a href="download.php?id=57052"><img class="download" src="pic/trans.gif" alt="download" /></a></td></table></td>
  <td class="rowfollow"><a href="comment.php?action=add&amp;pid=57052&amp;type=torrent">0</a></td>
  <td class="rowfollow nowrap"><span title="2026-09-02 15:09:28">4天 19时</span></td>
  <td class="rowfollow">64.31 GB</td>
  <td class="rowfollow" align="center">5</td>
  <td class="rowfollow">0</td>
  <td class="rowfollow">3</td>
  <td class="rowfollow">uploader_demo</td>
</tr>
</tbody></table>
</body></html>`

// piggoDetailFixture is a 50%-promotion detail page. Note the two title-bearing spans in
// h1: the promotion countdown (no style attribute) and the review status (styled).
const piggoDetailFixture = `<html><body>
<h1 align="center" id="top">Sample.Guesthouse.Rules.S01.2026.1080p.WEB-DL.H.264.DDP5.1-Demo&nbsp;&nbsp;&nbsp; <b>[<font class='halfdown' >50%</font>]</b> 剩余时间：<span title="2026-11-30 10:41:37">2月24天</span><span style="margin-left: 6px" title="通过"></span></h1>
<input type="hidden" name="torrent_name" value="Sample.Guesthouse.Rules.S01.2026.1080p.WEB-DL.H.264.DDP5.1-Demo" />
<input type="hidden" name="detail_torrent_id" value="56692" />
<table width="97%" cellspacing="0" cellpadding="5">
  <tr><td class="rowhead" width="13%">下载</td><td class="rowfollow" width="87%" align="left"><a class="index" href="download.php?id=56692">[Sample].torrent</a></td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">示例民宿法则 第1季 10集全 | 类型：真人秀 喜剧 | 主演：演员甲 演员乙</td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>25.20 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;综艺&nbsp;&nbsp;&nbsp;<b>来源: </b>WEB-DL</td></tr>
</table>
</body></html>`

// piggoDetailNoPromoFixture pins the risk described in PigGoDefinition: with no promotion,
// the only title-bearing span in h1 is the review status, which must NOT be parsed as a
// discount end time.
const piggoDetailNoPromoFixture = `<html><body>
<h1 align="center" id="top">Sample.NoPromo.2026.1080p.WEB-DL.H.264-Demo<span style="margin-left: 6px" title="通过"></span></h1>
<input type="hidden" name="torrent_name" value="Sample.NoPromo.2026.1080p.WEB-DL.H.264-Demo" />
<input type="hidden" name="detail_torrent_id" value="56700" />
<table width="97%" cellspacing="0" cellpadding="5">
  <tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>10.00 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;综艺</td></tr>
</table>
</body></html>`

// piggoIndexFixture mirrors the real #info_block table, including the nested badge links
// inside the username <b> element and the half-width colons this site uses.
const piggoIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td><table width="100%"><tr><td class="bottom" align="left">
<img src="pic/default_avatar.png" class="avatar">
<span class="medium"> 欢迎回来, <span class="nowrap"><a href="https://piggo.me/userdetails.php?id=21646" class='User_Name'><b>demouser<a href="/usercp.php" target="_blank"><img src="/pic/misc/iyuu.png" title="已绑定IYUU" /></a></b></a></span>[<b>ID:</b>21646]
[<a href="javascript:;" onclick="logout_normal(2);">退出</a>] [<a href="getrss.php">获取RSS</a>]
<font class = 'color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 1,351,847.4 <a href="attendance.php" class="faqlink">[签到得魔力]</a>
<font class = 'color_invite'>邀请 </font>[<a href="invite.php?id=21646">发送</a>]: 0(0) <br />
<font class="color_ratio">分享率:</font> 1.969 <font class='color_uploaded'>上传量:</font> 1.045 TB <font class='color_downloaded'> 下载量:</font> 543.38 GB
<font class='color_active'>当前活动:</font> <img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />2 <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />0&nbsp;&nbsp;
<font class='color_connectable'>可连接:</font>未知 <font class='color_slots'>连接数：</font>无限制
<font class='color_bonus'>H&R: </font> [<a href="myhr.php">种子区: 0/0/10 特别区: 0/7/10</a>]
</span></td></tr></table></td></tr></table>
</body></html>`

// piggoUserdetailsFixture follows the generic NexusPHP userdetails layout with this site's
// `rowhead nowrap` cells. The attachment on issue #532 captured another user's privacy-
// protected profile (no rowhead rows at all), so this structure is not yet confirmed
// against a real piggo.me page — see the follow-up item on that issue.
const piggoUserdetailsFixture = `<html><body>
<table width="100%">
  <tr><td class="rowhead nowrap" valign="top" align="right">加入日期</td><td class="rowfollow" valign="top" align="left">2024-03-15 20:11:09 (<span title="2024-03-15 20:11:09">1年6月前</span>)</td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">最近动向</td><td class="rowfollow" valign="top" align="left">2026-09-07 10:24:31 (<span title="2026-09-07 10:24:31">&lt; 1分前</span>)</td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">等级</td><td class="rowfollow" valign="top" align="left"><img alt="Power User" title="Power User" src="pic/class/power.gif" /></td></tr>
</table>
</body></html>`

func getPigGoDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("piggo")
	require.True(t, ok, "piggo definition not found")
	return def
}

func testPigGoSearch(t *testing.T) {
	def := getPigGoDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(piggoSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 1)

	item := items[0]
	assert.Equal(t, "57052", item.ID)
	assert.Equal(t, "Sample.Show.S01.2026.2160p.WEB-DL.H.265.HDR.AAC-Demo", item.Title)
	// The real subtitle is the bare text node after the tag badges.
	assert.Equal(t, "示例剧集 第一季 全30集 | 类型：剧情 | 主演：演员甲 演员乙", item.Subtitle)
	assert.Equal(t, v2.DiscountPercent50, item.DiscountLevel)
	// 剩余时间 span, not the styled 通过 status span.
	assert.Equal(t, "2027-03-01 15:09:28", item.DiscountEndTime.Format("2006-01-02 15:04:05"))
	assert.Equal(t, 5, item.Seeders)
	assert.Equal(t, 0, item.Leechers)
	assert.Equal(t, 3, item.Snatched)
	assert.Equal(t, int64(69052336701), item.SizeBytes)
}

func testPigGoDetail(t *testing.T) {
	def := getPigGoDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	t.Run("HalfDown", func(t *testing.T) {
		doc := FixtureDoc(t, "piggo_detail", piggoDetailFixture)
		info := parser.ParseAll(doc.Selection)
		assert.Equal(t, "56692", info.TorrentID)
		assert.Equal(t, "Sample.Guesthouse.Rules.S01.2026.1080p.WEB-DL.H.264.DDP5.1-Demo", info.Title)
		assert.Equal(t, v2.DiscountPercent50, info.DiscountLevel)
		assert.InDelta(t, 25.20*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
		assert.Equal(t, "2026-11-30 10:41:37", info.DiscountEnd.Format("2006-01-02 15:04:05"))
	})

	t.Run("NoPromotion", func(t *testing.T) {
		doc := FixtureDoc(t, "piggo_detail_nopromo", piggoDetailNoPromoFixture)
		info := parser.ParseAll(doc.Selection)
		assert.Equal(t, "56700", info.TorrentID)
		assert.InDelta(t, 10.00*1024, info.SizeMB, 0.1)
		// The styled 通过 status span must not leak in as a discount end time.
		assert.True(t, info.DiscountEnd.IsZero(), "DiscountEnd should stay zero, got %v", info.DiscountEnd)
	})
}

func testPigGoUserInfo(t *testing.T) {
	def := getPigGoDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "piggo_index", piggoIndexFixture)
		fields := map[string]string{
			"id":         "21646",
			"name":       "demouser",
			"bonus":      "1.3518474e+06",
			"ratio":      "1.969",
			"uploaded":   "1148989651025",
			"downloaded": "583449832325",
			"seeding":    "2",
			"leeching":   "0",
		}
		for field, expected := range fields {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok, "selector %q missing", field)
			assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel), "field %q", field)
		}
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "piggo_userdetails", piggoUserdetailsFixture)
		expected := map[string]string{
			"levelName":    "Power User",
			"joinTime":     "1710504669",
			"lastAccessAt": "1788747871",
		}
		for field, want := range expected {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok, "selector %q missing", field)
			assert.Equal(t, want, driver.ExtractFieldValuePublic(doc, sel), "field %q", field)
		}
		// The 保号 probe reads only LastAccess; keep it non-empty.
		assert.NotEmpty(t, driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["lastAccessAt"]))
	})
}

func TestPigGo_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":         piggoSearchFixture,
		"detail":         piggoDetailFixture,
		"detail_nopromo": piggoDetailNoPromoFixture,
		"index":          piggoIndexFixture,
		"userdetails":    piggoUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
