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
	RegisterFixtureSuite(FixtureSuite{SiteID: "gamegamept", Search: testGameGamePTSearch, Detail: testGameGamePTDetail, UserInfo: testGameGamePTUserInfo})
}

const gamegameptSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="电影/Movies" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=16295">GamePT.Demo.2026-CarPT</a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" />
    <img class="hitandrun" src="pic/trans.gif" alt="H&amp;R" title="H&amp;R" />
    <br/><span>演示标题 / Demo</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-03-28 13:01:22">3天2时</span></td>
  <td class="rowfollow">4.00 GB</td>
  <td class="rowfollow">53</td>
  <td class="rowfollow">3</td>
  <td class="rowfollow">69</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="音乐/Music" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=162957">GamePT.Demo2.FLAC-CarPT</a>
    <img class="pro_50pctdown" src="pic/trans.gif" alt="50%" />
    <br/><span>演示标题2 / Demo2</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-03-27 08:00:00">4天</span></td>
  <td class="rowfollow">186.95 MB</td>
  <td class="rowfollow">8</td>
  <td class="rowfollow">1</td>
  <td class="rowfollow">21</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table>
</body></html>`

// index.php 顶部 #info_block：本站币种叫「G值」（不是魔力值），真实值形如 97,146.5。
// 站内其它地方（公告、说明页）仍会出现「魔力值」字样，这里刻意保留作干扰项。
const gamegameptIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td>
欢迎回来, <span class="nowrap"><a href="https://www.gamegamept.com/userdetails.php?id=15270" class='User_Name'><b>gamegamept_user</b></a></span>
<font class = 'color_bonus'>G值 </font>[<a href="mybonus.php">使用</a>]: 1,234.5 <a href="attendance.php" class="faqlink">[签到得G值]</a> <a href="luckywheel.php">[幸运大转盘]</a>
<font class="color_ratio">分享率:</font> --- <font class='color_uploaded'>上传量:</font> 0.00 KB <font class='color_downloaded'> 下载量:</font> 0.00 KB <font class='color_active'>当前活动:</font> <img class="arrowup" src="pic/trans.gif" />2 <img class="arrowdown" src="pic/trans.gif" />1
</td></tr></table>
<div class="notice">本站魔力值（即 G值）可用于兑换邀请与勋章。</div>
</body></html>`

const gamegameptUserdetailsFixture = `<html><body>
<table>
  <tr><td class="rowhead nowrap" valign="top" align="right">用户ID/UID</td><td class="rowfollow">44933</td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">加入日期</td><td class="rowfollow">2026-03-20 08:30:00 (<span title="2026-03-20 08:30:00">7天前</span>)</td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">传输</td><td class="rowfollow"><table><tr><td class="embedded"><strong>分享率</strong>: <font color="">4.000</font></td></tr><tr><td class="embedded"><strong>上传量</strong>: 1.00 TB</td><td class="embedded"><strong>下载量</strong>: 256.00 GB</td></tr><tr><td class="embedded"><strong>实际上传量</strong>: 50.00 TB</td><td class="embedded"><strong>实际下载量</strong>: 1.00 TB</td></tr></table></td></tr>
  <tr><td class="rowhead nowrap" valign="top" align="right">等级</td><td class="rowfollow"><img title="User" src="pic/user.gif" /></td></tr>
</table>
</body></html>`

// mybonus.php：时魔要取「每小时获得的合计魔力值」表格末列（基本奖励 + 勋章加成 + 官种加成 + 后宫加成）。
// 页面上方的「你当前每小时能获取」只给基本奖励，且出现在合计表格**之前**，因此不能用
// 「A|B」交替（正则取最左匹配会命中上方那句），必须锚定合计表头。真实页面该表格为单行。
const gamegameptMyBonusFixture = `<html><body><div id="outer">
<h1>魔力值到底是什么玩意儿，我怎样才能得到它？</h1>
<ul><li>简言之，为做种人数少、文件体积大的种子做种能获得更多魔力值。</li></ul>
<div align="center">你当前每小时能获取299.172个魔力值 (A = 1469.9)</div>
<h1>每小时获得的合计魔力值</h1>
<div style="display: flex;justify-content: center;margin-top: 20px;"><table cellpadding="5" style="width: 50%"><tr><td class="colhead">奖励类型</td><td class="colhead">数量</td><td class="colhead">体积</td><td class="colhead">A 值</td><td class="colhead">基础魔力</td><td class="colhead">系数</td><td class="colhead">获得魔力</td><td class="colhead">合计</td></tr><tr><td>基本奖励</td><td>58</td><td>3.145 TB</td><td>1,469.869</td><td>299.172</td><td>1</td><td>299.172</td><td rowspan="4">581.337</td></tr><tr><td>勋章加成</td><td>58</td><td>3.145 TB</td><td>1,469.869</td><td>274.172</td><td>1.000</td><td>274.172</td></tr><tr><td>官种加成</td><td>1</td><td>16.78 GB</td><td>16.781</td><td>15.987</td><td>0.500</td><td>7.994</td></tr><tr><td>后宫加成</td><td>--</td><td>--</td><td>--</td><td>0.000</td><td>0.100</td><td>0.000</td></tr></table></div>
</div></body></html>`

const gamegameptDetailFixture = `<html><body>
<h1 align="center" id="top" value="GamePT.Demo.2026-CarPT"><input name="torrent_name" type="hidden" value="GamePT.Demo.2026-CarPT" /><input name="detail_torrent_id" type="hidden" value="16295" />GamePT.Demo.2026-CarPT <b>[<font class='free'>免费</font>]</b> <font color='#0000FF'>剩余时间：<span title="2026-04-04 13:01:22">3天21时</span></font><img class="hitandrun" src="pic/trans.gif" alt="H&amp;R" title="H&amp;R" /></h1>
<table>
  <tr><td class="rowhead">下载</td><td class="rowfollow"><a href="download.php?id=154097">demo.torrent</a></td></tr>
  <tr><td class="rowhead">副标题</td><td class="rowfollow">演示标题 / Demo</td></tr>
  <tr><td class="rowhead">基本信息</td><td class="rowfollow">大小</td><td class="rowfollow">大小：4.00 GB</td></tr>
</table>
</body></html>`

func getGameGamePTDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("gamegamept")
	require.True(t, ok)
	return def
}

func testGameGamePTSearch(t *testing.T) {
	def := getGameGamePTDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(gamegameptSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: http.MethodGet})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "16295", items[0].ID)
	assert.Equal(t, "GamePT.Demo.2026-CarPT", items[0].Title)
	assert.Equal(t, "演示标题 / Demo", items[0].Subtitle)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 53, items[0].Seeders)
	assert.Equal(t, 3, items[0].Leechers)
	assert.Equal(t, 69, items[0].Snatched)

	assert.Equal(t, "162957", items[1].ID)
	assert.Equal(t, v2.DiscountPercent50, items[1].DiscountLevel)
}

func testGameGamePTDetail(t *testing.T) {
	def := getGameGamePTDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "gamegamept_detail", gamegameptDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "16295", info.TorrentID)
	assert.Equal(t, "GamePT.Demo.2026-CarPT", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.True(t, info.HasHR)
	assert.InDelta(t, 4096, info.SizeMB, 0.1)
}

func testGameGamePTUserInfo(t *testing.T) {
	def := getGameGamePTDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "gamegamept_index", gamegameptIndexFixture)
	assert.Equal(t, "15270", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "gamegamept_user", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "1234.5", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "2", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "1", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "gamegamept_userdetails", gamegameptUserdetailsFixture)
	assert.Equal(t, "1099511627776", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "274877906944", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "4", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "1773966600", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["joinTime"]))
	assert.Equal(t, "54975581388800", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["trueUploaded"]))
	assert.Equal(t, "1099511627776", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["trueDownloaded"]))

	bonusDoc := FixtureDoc(t, "gamegamept_mybonus", gamegameptMyBonusFixture)
	// 取合计（299.172 + 274.172 + 7.994 + 0），而不是上方「你当前每小时能获取」的基本奖励 299.172
	assert.Equal(t, "581.337", driver.ExtractFieldValuePublic(bonusDoc, def.UserInfo.Selectors["bonusPerHour"]))
}

func TestGameGamePT_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      gamegameptSearchFixture,
		"index":       gamegameptIndexFixture,
		"userdetails": gamegameptUserdetailsFixture,
		"mybonus":     gamegameptMyBonusFixture,
		"detail":      gamegameptDetailFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) { RequireNoSecrets(t, name, data) })
	}
}

// #727：GGPT 开了整站 H&R；站点规则里没有做种时长，由用户在站点设置里填。
func TestGameGamePT_SiteWideHR(t *testing.T) {
	def, ok := v2.GetDefinitionRegistry().Get("gamegamept")
	require.True(t, ok)
	assert.True(t, def.HREnabled, "整站 H&R")
	assert.False(t, def.HasHRSeedTimeRule(), "做种时长由站点设置补")
}
