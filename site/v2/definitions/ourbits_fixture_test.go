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
		SiteID:   "ourbits",
		Search:   testOurbitsSearch,
		Detail:   testOurbitsDetail,
		UserInfo: testOurbitsUserInfo,
	})
}

// 站点真实 HTML 结构（来自 issue #329 附件 ourbits.club-*.zip）的脱敏版本：
//   - 10 列 NexusPHP 标准表格：cat / title / 评论 / 时间 / 大小 / 种子 / 下载 / 完成 / 进度 / 发布者
//   - 详情页 h1 嵌入 free 折扣 + onmouseover 含剩余时间
//   - userinfo: index 含 #info_block、userdetails 标准 NexusPHP 行
const ourbitsSearchFixture = `<html><body>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%" id="torrenttable">
<tr><td class="colhead">类型</td><td class="colhead">标题</td><td class="colhead">评论</td><td class="colhead">时间</td><td class="colhead">大小</td><td class="colhead">种子数</td><td class="colhead">下载数</td><td class="colhead">完成数</td><td class="colhead">进度</td><td class="colhead">发布者</td></tr>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=401"><img class="c2_movies" alt="Movies" title="Movies" /></a></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a title="Demo.Movie.2026.UHD.BluRay.2160p" href="details.php?id=10001&amp;hit=1"><b>Demo.Movie.2026.UHD.BluRay.2160p</b></a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-05-15 19:26:44&quot;&gt;9时35分&lt;/span&gt;&lt;/b&gt;', 'trail', false);" />
    &nbsp;剩余时间：<b><span title="2026-05-15 19:26:44">9时35分</span></b>
    <br/>演示电影副标题
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow nowrap"><span title="2026-05-14 22:00:00">14时24分</span></td>
  <td class="rowfollow">86.63 GB</td>
  <td class="rowfollow">233</td>
  <td class="rowfollow">14</td>
  <td class="rowfollow">492</td>
  <td>-</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=402"><img class="c2_tv" alt="TV" title="TV" /></a></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a title="Demo.TV.S01E01.2026.1080p" href="details.php?id=10002"><b>Demo.TV.S01E01.2026.1080p</b></a>
    <img class="pro_50pctdown" src="pic/trans.gif" alt="50%" title="50%" />
    <img class="hitandrun" src="pic/trans.gif" alt="H&amp;R" title="H&amp;R" />
    <br/>演示剧集副标题
  </td></tr></table></td>
  <td class="rowfollow">2</td>
  <td class="rowfollow nowrap"><span title="2026-05-13 12:00:00">1天</span></td>
  <td class="rowfollow">2.34 GB</td>
  <td class="rowfollow">42</td>
  <td class="rowfollow">8</td>
  <td class="rowfollow">17</td>
  <td>-</td>
  <td class="rowfollow">DemoUploader</td>
</tr>
</table>
</body></html>`

const ourbitsIndexFixture = `<html><body>
<table id="info_block"><tr><td><span class="medium">
欢迎回来, <span class="nowrap"><a href="userdetails.php?id=70001" target="_blank" class='User_Name'><b>ob_demo_user</b></a></span>
[<a href="logout.php?token=REDACTED">退出</a>]
<span class="color_bonus">魔力值 </span>[<a href="mybonus.php">使用</a>]: 33,739.5
&nbsp;(签到已得85)
<span class="color_invite">邀请 </span>[<a href="invite.php?id=70001">发送</a>]: 0/0<br/>
<span class="color_ratio">分享率：</span> 9.258
<font class='color_uploaded'>上传量：</font> 337.04 GB
<font class='color_downloaded'> 下载量：</font> 36.41 GB
<font class='color_active'>当前活动：</font>
<img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />22
<img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />0
&nbsp;<font class="color_connectable">H&R: </font>0/10
</span></td></tr></table>
</body></html>`

const ourbitsUserdetailsFixture = `<html><body>
<table>
  <tr><td class="rowhead nowrap" valign="top" align="right">加入日期</td><td class="rowfollow">2025-11-20 20:26:31 (<span title="2025-11-20 20:26:31">5月25天前</span>)</td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">最近动向</td><td class="rowfollow">网站访问: 2026-05-15 02:49:47 (7时1分前)</td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">传输</td><td class="rowfollow">
    <table>
      <tr><td>分享率: <font color="">1,930.915</font></td></tr>
      <tr><td>上传量: 195.215 TB</td><td>下载量: 103.53 GB</td></tr>
      <tr><td>真实分享率: 145.107</td></tr>
      <tr><td>真实上传量: 99.104 TB</td><td>真实下载量: 699.36 GB</td></tr>
    </table>
  </td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">等级</td><td class="rowfollow"><img title="Power User" src="pic/poweruser.gif" /></td></tr>
</table>
</body></html>`

const ourbitsMyBonusFixture = `<html><body>
<div id="outer">
您当前每小时能获取 18.50 个魔力值
</div>
</body></html>`

const ourbitsDetailFixture = `<html><body>
<h1 align="center" id="top">Demo.Movie.2026.UHD.BluRay.2160p&nbsp; <b>[<font class='free' onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-05-15 19:26:44&quot;&gt;9时35分&lt;/span&gt;&lt;/b&gt;', 'trail', false);">免费</font>]</b> 剩余时间：<b><span title="2026-05-15 19:26:44">9时35分</span></b></h1>
<table>
  <tr><td class="rowhead">下载</td><td class="rowfollow"><a href="download.php?id=10001">demo.torrent</a></td></tr>
  <tr><td class="rowhead">副标题</td><td class="rowfollow">演示电影副标题</td></tr>
  <tr><td class="rowhead">基本信息</td><td class="rowfollow">大小：86.63 GB 类型: Movies 媒介: UHD Blu-ray 编码: HEVC 音频编码: TrueHD 分辨率: 2160p 制作组: OurBits</td></tr>
</table>
</body></html>`

func getOurbitsDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("ourbits")
	require.True(t, ok)
	return def
}

func testOurbitsSearch(t *testing.T) {
	def := getOurbitsDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(ourbitsSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: http.MethodGet})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "10001", items[0].ID)
	assert.Equal(t, "Demo.Movie.2026.UHD.BluRay.2160p", items[0].Title)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 233, items[0].Seeders)
	assert.Equal(t, 14, items[0].Leechers)
	assert.Equal(t, 492, items[0].Snatched)

	assert.Equal(t, "10002", items[1].ID)
	assert.Equal(t, v2.DiscountPercent50, items[1].DiscountLevel)
}

func testOurbitsDetail(t *testing.T) {
	def := getOurbitsDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "ourbits_detail", ourbitsDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 88708.92, info.SizeMB, 5.0)
}

func testOurbitsUserInfo(t *testing.T) {
	def := getOurbitsDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "ourbits_index", ourbitsIndexFixture)
	assert.Equal(t, "70001", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "ob_demo_user", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "33739.5", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "22", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "ourbits_userdetails", ourbitsUserdetailsFixture)
	assert.Equal(t, "214641162416291", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "111164491038", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "1930.915", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "Power User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "108966000359112", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["trueUploaded"]))
	assert.Equal(t, "750932082032", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["trueDownloaded"]))

	bonusDoc := FixtureDoc(t, "ourbits_mybonus", ourbitsMyBonusFixture)
	assert.Equal(t, "18.5", driver.ExtractFieldValuePublic(bonusDoc, def.UserInfo.Selectors["bonusPerHour"]))
}

func TestOurbits_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      ourbitsSearchFixture,
		"index":       ourbitsIndexFixture,
		"userdetails": ourbitsUserdetailsFixture,
		"mybonus":     ourbitsMyBonusFixture,
		"detail":      ourbitsDetailFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) { RequireNoSecrets(t, name, data) })
	}
}
