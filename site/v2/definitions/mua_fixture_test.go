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
		SiteID:   "mua",
		Search:   testMuaSearch,
		Detail:   testMuaDetail,
		UserInfo: testMuaUserInfo,
	})
}

// 站点真实 HTML 结构（来自 issue #339 附件 mua.xloli.cc-*.zip）的脱敏版本：
//   - 9 列 NexusPHP 表格（无"进度"列）：cat / title / 评论 / 时间 / 大小 / 种子 / 下载 / 完成 / 发布者
//   - userdetails URL 使用 uuid= 而非 id=（id 必须从 'td.rowhead:contains 用户ID/UID' 提取）
//   - 详情页 h1 嵌入 free 折扣
//   - 二次元主题站点，副标题在 td.embedded > span（不带 optiontag/tag class）
const muaSearchFixture = `<html><body>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%">
<tr><td class="colhead">类型</td><td class="colhead">标题</td><td class="colhead">评论</td><td class="colhead">时间</td><td class="colhead">大小</td><td class="colhead">做种</td><td class="colhead">下载</td><td class="colhead">完成</td><td class="colhead">发布者</td></tr>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=401"><img class="m_animation" alt="Animation" title="Animation" /></a></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a title="Demo.Anime.S01E06.2026.1080p" href="details.php?id=20001"><b>Demo.Anime.S01E06.2026.1080p</b></a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" />
    <font color="#0000FF">剩余时间：<span title="2026-06-15 20:49:40">29天23时</span></font>
    <br/>
    <span>精选</span><span>中字</span><span>特效</span><span>分集</span>
    演示动画副标题
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow nowrap"><span title="2026-05-16 18:30:00">2时0分钟</span></td>
  <td class="rowfollow">1.37 GB</td>
  <td class="rowfollow">18</td>
  <td class="rowfollow">2</td>
  <td class="rowfollow">22</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow nowrap"><a href="?cat=410"><img class="m_music" alt="Music" title="Music" /></a></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a title="Demo.Album.2026.FLAC" href="details.php?id=20002"><b>Demo.Album.2026.FLAC</b></a>
    <img class="pro_50pctdown" src="pic/trans.gif" alt="50%" title="50%" />
    <br/>演示音乐副标题
  </td></tr></table></td>
  <td class="rowfollow">5</td>
  <td class="rowfollow nowrap"><span title="2026-05-15 12:00:00">1天</span></td>
  <td class="rowfollow">450 MB</td>
  <td class="rowfollow">12</td>
  <td class="rowfollow">3</td>
  <td class="rowfollow">9</td>
  <td class="rowfollow">DemoUploader</td>
</tr>
</table>
</body></html>`

const muaIndexFixture = `<html><body>
<table id="info_block"><tr><td><span class="medium">
欢迎回来, <span class="nowrap"><a href="https://mua.xloli.cc/userdetails.php?uuid=demo-uuid-0000-0000-0000-000000000000" target="_blank" class='User_Name'><b>mua_demo_user</b></a></span>
[<a href="logout.php?token=REDACTED">退出</a>]
<span class="color_bonus">魔力值 </span>[<a href="mybonus.php">使用</a>]: 105.5
[已签到，获得70魔力，补签卡：0张]
<span class="color_invite">邀请 </span>[<a href="invite.php">发送</a>]: 0(0)<br/>
<span class="color_ratio">分享率：</span> 无限
<font class='color_uploaded'>上传量：</font> 125.80 GB
<font class='color_downloaded'> 下载量：</font> 0.00 KB
<font class='color_active'>当前活动：</font>
<img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />5
<img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />1
&nbsp;<font class="color_connectable">可连接：</font>是
</span></td></tr></table>
</body></html>`

const muaUserdetailsFixture = `<html><body>
<table>
  <tr><td class="rowhead nowrap" valign="top" align="right">用户ID/UID</td><td class="rowfollow">100901</td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">用户UUID</td><td class="rowfollow">demo-uuid-0000-0000-0000-000000000000</td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">加入日期</td><td class="rowfollow">2026-05-15 14:53:02 (<span title="2026-05-15 14:53:02">1天6时前, 0.2周</span>)</td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">传输</td><td class="rowfollow">
    <table>
      <tr><td>分享率: 无限</td></tr>
      <tr><td>上传量: 125.80 GB</td><td>下载量: 0.00 KB</td></tr>
    </table>
  </td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">等级</td><td class="rowfollow"><img title="User" src="pic/user.gif" /></td></tr>
</table>
</body></html>`

const muaMyBonusFixture = `<html><body>
<div id="outer">
您当前每小时能获取 1.20 个魔力值
</div>
</body></html>`

const muaDetailFixture = `<html><body>
<h1 align="center" id="top">Demo.Anime.S01E06.2026.1080p&nbsp; <b>[<font class='free'>免费</font>]</b> <font color="#0000FF">剩余时间：<span title="2026-06-15 20:49:40">29天23时</span></font></h1>
<table>
  <tr><td class="rowhead">下载</td><td class="rowfollow"><a href="download.php?id=20001">demo.torrent</a></td></tr>
  <tr><td class="rowhead">副标题</td><td class="rowfollow">演示动画副标题</td></tr>
  <tr><td class="rowhead">基本信息</td><td class="rowfollow">大小：1.37 GB 类型: 动画 (Animation) 来源: 日本 (JPN) 媒介: WEB-DL 编码: AVC/x264 音频编码: AAC 分辨率: 1080p 制作组: Other</td></tr>
</table>
</body></html>`

func getMuaDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("mua")
	require.True(t, ok)
	return def
}

func testMuaSearch(t *testing.T) {
	def := getMuaDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(muaSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: http.MethodGet})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "20001", items[0].ID)
	assert.Equal(t, "Demo.Anime.S01E06.2026.1080p", items[0].Title)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 18, items[0].Seeders)
	assert.Equal(t, 2, items[0].Leechers)
	assert.Equal(t, 22, items[0].Snatched)

	assert.Equal(t, "20002", items[1].ID)
	assert.Equal(t, v2.DiscountPercent50, items[1].DiscountLevel)
}

func testMuaDetail(t *testing.T) {
	def := getMuaDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "mua_detail", muaDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 1402.88, info.SizeMB, 5.0)
}

func testMuaUserInfo(t *testing.T) {
	def := getMuaDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "mua_index", muaIndexFixture)
	assert.Equal(t, "mua_demo_user", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "105.5", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "5", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "1", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "mua_userdetails", muaUserdetailsFixture)
	assert.Equal(t, "100901", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["id"]),
		"id 必须从 td.rowhead:contains('用户ID/UID') + td 提取（mua 的 userdetails URL 使用 uuid 而非整数 id）")
	assert.NotEmpty(t, driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))

	bonusDoc := FixtureDoc(t, "mua_mybonus", muaMyBonusFixture)
	assert.Equal(t, "1.2", driver.ExtractFieldValuePublic(bonusDoc, def.UserInfo.Selectors["bonusPerHour"]))
}

func TestMua_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      muaSearchFixture,
		"index":       muaIndexFixture,
		"userdetails": muaUserdetailsFixture,
		"mybonus":     muaMyBonusFixture,
		"detail":      muaDetailFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) { RequireNoSecrets(t, name, data) })
	}
}
