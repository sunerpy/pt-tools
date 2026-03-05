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
		SiteID:   "52pt",
		Search:   testFiveTwoPTSearch,
		Detail:   testFiveTwoPTDetail,
		UserInfo: testFiveTwoPTUserInfo,
	})
}

const fiveTwoPTSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr class='thirtypercentdown_bg'>
  <td class="rowfollow"><img alt="Movies/电影" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=36011">Hamnet.2025.2160p.BluRay.x265</a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;Free（下载量不统计）&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-03-06 01:54:11&quot;&gt;1天14时&lt;/span&gt;&lt;/b&gt;', 'trail', false)" />
    <br/><span>哈姆奈特 / 双语字幕</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-03-04 01:54:11">9时18分</span></td>
  <td class="rowfollow">21.34 GB</td>
  <td class="rowfollow">38</td>
  <td class="rowfollow">4</td>
  <td class="rowfollow">34</td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="Movies/电影" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=35889">The.Peacemaker.1997.2160p.BluRay</a>
    <img class="pro_50pctdown" src="pic/trans.gif" alt="50%" title="50%（下载量按50%统计）" />
    <br/><span>末日戒备</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-03-02 02:38:19">2天8时</span></td>
  <td class="rowfollow">35.60 GB</td>
  <td class="rowfollow">60</td>
  <td class="rowfollow">0</td>
  <td class="rowfollow">58</td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table>
</body></html>`

const fiveTwoPTIndexFixture = `<html><body>
<div id="info_block">
  欢迎回来, <a href="userdetails.php?id=37194" class="User_Name"><b>Jccc0201</b></a>
  <font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 2,213.4
  当前活动：<img class="arrowup" src="pic/trans.gif" />做种数:7 <img class="arrowdown" src="pic/trans.gif" />下载数:2
</div>
</body></html>`

const fiveTwoPTUserdetailsFixture = `<html><body>
<table>
  <tr><td class="rowhead">传输</td><td class="rowfollow"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>上传量</strong>:  2.632 TB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>:  0.00 KB</td></tr></table></td></tr>
  <tr><td class="rowhead">BT时间</td><td class="rowfollow"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>做种/下载时间比率</strong>:  <font color="">159,204.667</font></td><td class="embedded">&nbsp;&nbsp;<img src="pic/smilies/163.gif" alt="" /></td></tr></table></td></tr>
  <tr><td class="rowhead">等级</td><td class="rowfollow"><img title="(幼儿班)User" src="pic/user.gif" /></td></tr>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2023-11-30 16:03:44 (<span title="2023-11-30 16:03:44">2年3月前</span>)</td></tr>
</table>
</body></html>`

const fiveTwoPTDetailFixture = `<html><body>
<h1 align="center" id="top" value="Hamnet.2025.2160p.BluRay.x265">
  <input name="torrent_name" type="hidden" value="Hamnet.2025.2160p.BluRay.x265" />
  <input name="detail_torrent_id" type="hidden" value="36011" />
  <font class='free'>Free（下载量不统计）</font>
</h1>
<table><tr><td class="rowhead">基本信息</td><td class="rowfollow">大小</td><td class="rowfollow">大小：21.34 GB</td></tr></table>
</body></html>`

const fiveTwoPTDetailWithHRFixture = `<html><body>
<h1 value="HR.Movie.2025.1080p">
  <input name="torrent_name" type="hidden" value="HR.Movie.2025.1080p" />
  <input name="detail_torrent_id" type="hidden" value="36012" />
</h1>
<table><tr><td class="rowhead">基本信息</td><td class="rowfollow">大小</td><td class="rowfollow">大小：5.00 GB</td></tr></table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

func getFiveTwoPTDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("52pt")
	require.True(t, ok)
	return def
}

func testFiveTwoPTSearch(t *testing.T) {
	def := getFiveTwoPTDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fiveTwoPTSearchFixture))
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
	assert.Equal(t, "36011", first.ID)
	assert.Equal(t, "Hamnet.2025.2160p.BluRay.x265", first.Title)
	assert.Equal(t, "哈姆奈特 / 双语字幕", first.Subtitle)
	assert.Equal(t, v2.DiscountFree, first.DiscountLevel)
	assert.Equal(t, 38, first.Seeders)
	assert.Equal(t, 4, first.Leechers)
	assert.Equal(t, 34, first.Snatched)

	second := items[1]
	assert.Equal(t, "35889", second.ID)
	assert.Equal(t, v2.DiscountPercent50, second.DiscountLevel)
}

func testFiveTwoPTDetail(t *testing.T) {
	def := getFiveTwoPTDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "52pt_detail", fiveTwoPTDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "36011", info.TorrentID)
	assert.Equal(t, "Hamnet.2025.2160p.BluRay.x265", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 21.34*1024, info.SizeMB, 0.1)

	hrDoc := FixtureDoc(t, "52pt_detail_hr", fiveTwoPTDetailWithHRFixture)
	hrInfo := parser.ParseAll(hrDoc.Selection)
	assert.Equal(t, "36012", hrInfo.TorrentID)
	assert.True(t, hrInfo.HasHR)
}

func testFiveTwoPTUserInfo(t *testing.T) {
	def := getFiveTwoPTDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "52pt_index", fiveTwoPTIndexFixture)
	assert.Equal(t, "37194", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "Jccc0201", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "2213.4", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "7", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "2", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "52pt_userdetails", fiveTwoPTUserdetailsFixture)
	assert.Equal(t, "2893914604306", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "159204.667", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "(幼儿班)User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "1701360224", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["joinTime"]))
}

func TestFiveTwoPT_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      fiveTwoPTSearchFixture,
		"index":       fiveTwoPTIndexFixture,
		"userdetails": fiveTwoPTUserdetailsFixture,
		"detail":      fiveTwoPTDetailFixture,
		"detail_hr":   fiveTwoPTDetailWithHRFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
