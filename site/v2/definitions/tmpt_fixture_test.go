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
		SiteID:   "tmpt",
		Search:   testTMPTSearch,
		Detail:   testTMPTDetail,
		UserInfo: testTMPTUserInfo,
	})
}

const tmptSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="其他" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=16"><b>从零开始玩PT_V1.0 pdf</b></a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" title="免费" />
  </td></tr></table></td>
  <td class="rowfollow"><b><a href="details.php?id=16&amp;cmtpage=1#startcomments">1</a></b></td>
  <td class="rowfollow nowrap"><span title="2025-02-26 00:04:57">1年<br/>0月</span></td>
  <td class="rowfollow">2.26<br/>MB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=16&amp;dllist=1#seeders">207</a></b></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><a href="viewsnatches.php?id=16"><b>567</b></a></td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="电视剧" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=63079"><b>Her Blaze 2026 S01E08 2160p 60fps WEB-DL HEVC 10bit HDR Vivid</b></a>
    <img class="pro_free2up" src="pic/trans.gif" alt="2X Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;twoupfree&quot;&gt;2X免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-04-15 11:08:44&quot;&gt;29天23时&lt;/span&gt;&lt;/b&gt;', 'trail', false)" />
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow nowrap"><span title="2026-03-16 11:08:44">22分钟</span></td>
  <td class="rowfollow">4.65<br/>GB</td>
  <td class="rowfollow" align="center"><b><a href="details.php?id=63079&amp;dllist=1#seeders">4</a></b></td>
  <td class="rowfollow"><b><a href="details.php?id=63079&amp;dllist=1#leechers">1</a></b></td>
  <td class="rowfollow"><a href="viewsnatches.php?id=63079"><b>3</b></a></td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table>
</body></html>`

const tmptIndexFixture = `<html><body>
<div id="info_block">
  欢迎回来, <a href="https://tmpt.top/userdetails.php?id=12723" class="User_Name"><b>awa</b></a>
  <font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 10.6
  <font class="color_ratio">分享率:</font> 无限
  <font class='color_uploaded'>上传量:</font> 20.00 GB
  <font class='color_downloaded'> 下载量:</font> 0.00 KB
  <font class='color_active'>当前活动:</font> <img class="arrowup" src="pic/trans.gif" />1 <img class="arrowdown" src="pic/trans.gif" />0
</div>
</body></html>`

const tmptUserdetailsFixture = `<html><body>
<table>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2026-03-16 09:55:41 (<span title="2026-03-16 09:55:41">1时35分钟前</span>)</td></tr>
  <tr><td class="rowhead">传输</td><td class="rowfollow"><table><tr><td class="embedded"><strong>分享率</strong>: 无限</td></tr><tr><td class="embedded"><strong>上传量</strong>: 20.00 GB</td><td class="embedded"><strong>下载量</strong>: 0.00 KB</td></tr></table></td></tr>
  <tr><td class="rowhead">等级</td><td class="rowfollow"><img title="User" src="pic/user.gif" /></td></tr>
</table>
</body></html>`

const tmptDetailFixture = `<html><body>
<h1 align="center" id="top" value="从零开始玩PT_V1.0 pdf">
  <input name="torrent_name" type="hidden" value="从零开始玩PT_V1.0 pdf" />
  <input name="detail_torrent_id" type="hidden" value="16" />
  <font class='free'>免费</font>
</h1>
<table>
  <tr><td class="rowhead">副标题</td><td class="rowfollow">小萌新混 PT 界的必备入门教材 保证你学以致用</td></tr>
  <tr><td class="rowhead">基本信息</td><td class="rowfollow">大小</td><td class="rowfollow">大小：2.26 MB 类型：其他</td></tr>
</table>
</body></html>`

const tmptDetailWithHRFixture = `<html><body>
<h1 value="从零开始玩PT_V1.0 pdf HR">
  <input name="torrent_name" type="hidden" value="从零开始玩PT_V1.0 pdf HR" />
  <input name="detail_torrent_id" type="hidden" value="17" />
</h1>
<table><tr><td class="rowhead">基本信息</td><td class="rowfollow">大小</td><td class="rowfollow">大小：1.00 GB 类型：其他</td></tr></table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

func getTMPTDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("tmpt")
	require.True(t, ok)
	return def
}

func testTMPTSearch(t *testing.T) {
	def := getTMPTDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(tmptSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	first := items[0]
	assert.Equal(t, "16", first.ID)
	assert.Equal(t, "从零开始玩PT_V1.0 pdf", first.Title)
	assert.Equal(t, v2.DiscountFree, first.DiscountLevel)
	assert.Equal(t, 207, first.Seeders)
	assert.Equal(t, 0, first.Leechers)
	assert.Equal(t, 567, first.Snatched)

	second := items[1]
	assert.Equal(t, "63079", second.ID)
	assert.Equal(t, v2.Discount2xFree, second.DiscountLevel)
}

func testTMPTDetail(t *testing.T) {
	def := getTMPTDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "tmpt_detail", tmptDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "16", info.TorrentID)
	assert.Equal(t, "从零开始玩PT_V1.0 pdf", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 2.26, info.SizeMB, 0.1)

	hrDoc := FixtureDoc(t, "tmpt_detail_hr", tmptDetailWithHRFixture)
	hrInfo := parser.ParseAll(hrDoc.Selection)
	assert.Equal(t, "17", hrInfo.TorrentID)
	assert.True(t, hrInfo.HasHR)
}

func testTMPTUserInfo(t *testing.T) {
	def := getTMPTDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "tmpt_index", tmptIndexFixture)
	assert.Equal(t, "12723", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "awa", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "21474836480", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "10.6", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "1", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "tmpt_userdetails", tmptUserdetailsFixture)
	assert.Equal(t, "User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "1773654941", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["joinTime"]))
	assert.Equal(t, "21474836480", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["downloaded"]))
}

func TestTMPT_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      tmptSearchFixture,
		"index":       tmptIndexFixture,
		"userdetails": tmptUserdetailsFixture,
		"detail":      tmptDetailFixture,
		"detail_hr":   tmptDetailWithHRFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
