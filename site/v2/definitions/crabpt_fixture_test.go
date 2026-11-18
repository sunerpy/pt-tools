package definitions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func init() {
	RegisterFixtureSuite(FixtureSuite{SiteID: "crabpt", Search: testCrabPTSearch, Detail: testCrabPTDetail, UserInfo: testCrabPTUserInfo})
}

const crabptSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="电影/Movies" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=1811">CrabPT.Demo.2026-CarPT</a>
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
    <a href="details.php?id=18117">CrabPT.Demo2.FLAC-CarPT</a>
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

// 索引页：CrabPT 把「魔力值」改名为「蟹币值」；分享率为无穷时站点写的是「无限」而不是「∞」。
// 站内公告里同样会出现「魔力值」字样，这里刻意保留，确保解析不会被公告内容带偏。
const crabptIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td>
欢迎回来, <span class="nowrap"><a href="https://crabpt.vip/userdetails.php?id=19967" class='User_Name'><b>crabpt_user</b></a></span>
<font class = 'color_bonus'>蟹币值 </font>[<a href="mybonus.php">使用</a>]: 1,234.5 <a href="attendance.php" class="faqlink">[签到得蟹币]</a>
<font class="color_ratio">分享率:</font> 无限 <font class='color_uploaded'>上传量:</font> 0.00 KB <font class='color_downloaded'> 下载量:</font> 0.00 KB <font class='color_active'>当前活动:</font> <img class="arrowup" src="pic/trans.gif" />2 <img class="arrowdown" src="pic/trans.gif" />1
</td></tr></table>
<div class="news">入选作品将有机会展示，并获得丰厚的上传量/魔力值补偿。</div>
</body></html>`

// 用户详情页：分享率只出现在顶部 #info_block 的用户栏里；
// 「传输」单元格没有任何分享率数值（只有一句「不参与分享率计算」的说明）。
const crabptUserdetailsFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td>
欢迎回来, <span class="nowrap"><a href="https://crabpt.vip/userdetails.php?id=19967" class='User_Name'><b>crabpt_user</b></a></span>
<font class = 'color_bonus'>蟹币值 </font>[<a href="mybonus.php">使用</a>]: 1,234.5
<font class="color_ratio">分享率:</font> 4.000 <font class='color_uploaded'>上传量:</font> 1.00 TB <font class='color_downloaded'> 下载量:</font> 256.00 GB
</td></tr></table>
<table>
  <tr><td class="rowhead">用户ID/UID</td><td class="rowfollow">44933</td></tr>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2026-03-20 08:30:00 (<span title="2026-03-20 08:30:00">7天前</span>)</td></tr>
  <tr><td class="rowhead">传输</td><td class="rowfollow"><table><tr><td class="embedded"><strong>上传量</strong>: 1.00 TB</td><td class="embedded"><strong>下载量</strong>: 256.00 GB</td></tr><tr><td class="embedded"><strong>实际上传量</strong>: 900.00 GB</td><td class="embedded"><strong>实际下载量</strong>: 200.00 GB</td><td class="embedded text-muted">实际上传/下载量 (仅用于记录, 不参与分享率计算)</td></tr></table></td></tr>
  <tr><td class="rowhead">等级</td><td class="rowfollow"><img title="User" src="pic/user.gif" /></td></tr>
</table>
</body></html>`

// 蟹币值页：句子里的数值是「基础」时魔，表格「每小时获得的合计蟹币值」才是含勋章/官种/后宫加成的合计值。
const crabptMyBonusFixture = `<html><body><div id="outer"><table border="1"><tr><td class="text" align="left">
<h1>做种每小时将得到如下的蟹币值</h1>
<ul><li>0.5个蟹币值 * <i>你的做种数</i> (做种数最多计200个)</li></ul>
<div align="center">你当前每小时能获取183.606个蟹币值 (A = 1139.1)</div>
</td></tr></table>
<h1>每小时获得的合计蟹币值</h1><div style="display: flex;justify-content: center;margin-top: 20px;"><table cellpadding="5" style="width: 50%"><tr><td class="colhead">奖励类型</td><td class="colhead">数量</td><td class="colhead">体积</td><td class="colhead">A 值</td><td class="colhead">基础蟹币</td><td class="colhead">系数</td><td class="colhead">获得蟹币</td><td class="colhead">合计</td></tr><tr><td>基本奖励</td><td>598</td><td>2.147 TB</td><td>1,139.130</td><td>183.606</td><td>1</td><td>183.606</td><td rowspan="4">184.442</td></tr><tr><td>勋章加成</td><td>598</td><td>2.147 TB</td><td>1,139.130</td><td>83.606</td><td>0.010</td><td>0.836</td></tr></table></div>
</div></body></html>`

const crabptDetailFixture = `<html><body>
<h1 align="center" id="top" value="CrabPT.Demo.2026-CarPT"><input name="torrent_name" type="hidden" value="CrabPT.Demo.2026-CarPT" /><input name="detail_torrent_id" type="hidden" value="1811" />CrabPT.Demo.2026-CarPT <b>[<font class='free'>免费</font>]</b> <font color='#0000FF'>剩余时间：<span title="2026-04-04 13:01:22">3天21时</span></font><img class="hitandrun" src="pic/trans.gif" alt="H&amp;R" title="H&amp;R" /></h1>
<table>
  <tr><td class="rowhead">下载</td><td class="rowfollow"><a href="download.php?id=154097">demo.torrent</a></td></tr>
  <tr><td class="rowhead">副标题</td><td class="rowfollow">演示标题 / Demo</td></tr>
  <tr><td class="rowhead">基本信息</td><td class="rowfollow">大小</td><td class="rowfollow">大小：4.00 GB</td></tr>
</table>
</body></html>`

func getCrabPTDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("crabpt")
	require.True(t, ok)
	return def
}

func testCrabPTSearch(t *testing.T) {
	def := getCrabPTDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(crabptSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: http.MethodGet})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "1811", items[0].ID)
	assert.Equal(t, "CrabPT.Demo.2026-CarPT", items[0].Title)
	assert.Equal(t, "演示标题 / Demo", items[0].Subtitle)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 53, items[0].Seeders)
	assert.Equal(t, 3, items[0].Leechers)
	assert.Equal(t, 69, items[0].Snatched)

	assert.Equal(t, "18117", items[1].ID)
	assert.Equal(t, v2.DiscountPercent50, items[1].DiscountLevel)
}

func testCrabPTDetail(t *testing.T) {
	def := getCrabPTDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "crabpt_detail", crabptDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "1811", info.TorrentID)
	assert.Equal(t, "CrabPT.Demo.2026-CarPT", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.True(t, info.HasHR)
	assert.InDelta(t, 4096, info.SizeMB, 0.1)
}

func testCrabPTUserInfo(t *testing.T) {
	def := getCrabPTDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "crabpt_index", crabptIndexFixture)
		fields := map[string]string{
			"id":       "19967",
			"name":     "crabpt_user",
			"bonus":    "1234.5",
			"seeding":  "2",
			"leeching": "1",
			// 分享率为「无限」时统一折算成一个远大于 1 的有限值，
			// 保证分享率门槛判断与前端展示（> 999999 显示 ∞）都正确。
			"ratio": "9999999",
		}
		for field, expected := range fields {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				if !assert.True(t, ok, "selector %q not found", field) {
					return
				}
				assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel))
			})
		}
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "crabpt_userdetails", crabptUserdetailsFixture)
		fields := map[string]string{
			"uploaded":   "1099511627776",
			"downloaded": "274877906944",
			"ratio":      "4.000",
			"levelName":  "User",
			// 2026-03-20 08:30:00 +0800
			"joinTime": "1773966600",
		}
		for field, expected := range fields {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				if !assert.True(t, ok, "selector %q not found", field) {
					return
				}
				assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel))
			})
		}
	})

	t.Run("MyBonusPage", func(t *testing.T) {
		doc := FixtureDoc(t, "crabpt_mybonus", crabptMyBonusFixture)
		sel, ok := def.UserInfo.Selectors["bonusPerHour"]
		if !assert.True(t, ok, "selector %q not found", "bonusPerHour") {
			return
		}
		// 取「每小时获得的合计蟹币值」，而不是句子里的基础值 183.606
		assert.Equal(t, "184.442", driver.ExtractFieldValuePublic(doc, sel))
	})
}

func TestCrabPT_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      crabptSearchFixture,
		"index":       crabptIndexFixture,
		"userdetails": crabptUserdetailsFixture,
		"mybonus":     crabptMyBonusFixture,
		"detail":      crabptDetailFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) { RequireNoSecrets(t, name, data) })
	}
}

// 繁体、英文界面与分享率不到 1 的写法：「无限」的繁体与英文取自上游 NexusPHP 的 lang_functions.php（無限 / Inf.），
// 表头取自 resources/lang/*/bonus.php（合计 / 合計 / Total），分享率不到 1 时数值包在 <font color="#880000"> 里
func TestCrabPT_UserInfoVariants(t *testing.T) {
	def := getCrabPTDef(t)
	driver := newTestNexusPHPDriver(def)

	for _, total := range []string{"合計", "Total"} {
		t.Run("bonusPerHour/"+total, func(t *testing.T) {
			html := strings.ReplaceAll(crabptMyBonusFixture, "合计</td>", total+"</td>")
			require.NotEqual(t, crabptMyBonusFixture, html)
			doc := FixtureDoc(t, "crabpt_mybonus_"+total, html)
			assert.Equal(t, "184.442", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["bonusPerHour"]))
		})
	}

	t.Run("bonus/蟹幣值", func(t *testing.T) {
		html := strings.Replace(crabptIndexFixture, "蟹币值 </font>", "蟹幣值 </font>", 1)
		require.NotEqual(t, crabptIndexFixture, html)
		doc := FixtureDoc(t, "crabpt_index_cht", html)
		assert.Equal(t, "1234.5", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["bonus"]))
	})

	// 无穷分享率不管写成哪种，都和「无限」一样折算成 9999999
	infinite := map[string]string{
		"無限":   `<font class="color_ratio">分享率：</font> 無限`,
		"Inf.": `<font class="color_ratio">Ratio:</font> Inf.`,
		"∞":    `<font class="color_ratio">分享率:</font> ∞`,
	}
	for name, ratio := range infinite {
		t.Run("ratio/"+name, func(t *testing.T) {
			html := strings.Replace(crabptIndexFixture, `<font class="color_ratio">分享率:</font> 无限`, ratio, 1)
			require.NotEqual(t, crabptIndexFixture, html)
			doc := FixtureDoc(t, "crabpt_index_ratio", html)
			assert.Equal(t, "9999999", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["ratio"]))
		})
	}

	t.Run("ratio below 1", func(t *testing.T) {
		html := strings.Replace(crabptUserdetailsFixture, "分享率:</font> 4.000", `分享率:</font> <font color="#880000">0.512</font>`, 1)
		require.NotEqual(t, crabptUserdetailsFixture, html)
		doc := FixtureDoc(t, "crabpt_userdetails_low_ratio", html)
		assert.Equal(t, "0.512", driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["ratio"]))
	})
}
