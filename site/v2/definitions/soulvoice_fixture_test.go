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
		SiteID:   "soulvoice",
		Search:   testSoulVoiceSearch,
		Detail:   testSoulVoiceDetail,
		UserInfo: testSoulVoiceUserInfo,
	})
}

const soulvoiceSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="电影" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=210481">Whistle.2026.1080p.iTunes.WEB-DL</a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this,event,'content','&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-03-07 09:48:39&quot;&gt;2天23时&lt;/span&gt;&lt;/b&gt;','trail',false)" />
    <br /><span>索命哨</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-03-04 09:48:39">1分钟</span></td>
  <td class="rowfollow">5.29 GB</td>
  <td class="rowfollow">1</td>
  <td class="rowfollow">16</td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="动漫" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=210480">Fireman.Sam.S08.2012</a>
    <br /><span>消防员山姆 第八季</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-03-04 08:51:59">58分钟</span></td>
  <td class="rowfollow">10.35 GB</td>
  <td class="rowfollow">22</td>
  <td class="rowfollow">18</td>
  <td class="rowfollow">22</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table>
</body></html>`

const soulvoiceDetailFixture = `<html><body>
<h1>
  <input name="torrent_name" type="hidden" value="Whistle.2026.1080p.iTunes.WEB-DL" />
  <input name="detail_torrent_id" type="hidden" value="210481" />
  <font class="free">免费</font>
  <span title="2026-03-07 09:48:39">2天23时</span>
</h1>
<table><tr><td class="rowhead">基本信息</td><td class="rowfollow">大小：5.29 GB | 类型：电影</td></tr></table>
</body></html>`

const soulvoiceDetailHRFixture = `<html><body>
<h1>
  <input name="torrent_name" type="hidden" value="SoulVoice.HR.Test" />
  <input name="detail_torrent_id" type="hidden" value="210499" />
</h1>
<table><tr><td class="rowhead">基本信息</td><td class="rowfollow">大小：12.34 GB | 类型：电影</td></tr></table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

const soulvoiceIndexFixture = `<html><body>
<table id="info_block"><tr><td>
欢迎回来, <a href="https://pt.soulvoice.club/userdetails.php?id=150574" class="User_Name"><b>qinmina</b></a>
<font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 18461.6
<img class="arrowup" src="pic/trans.gif" />199
<img class="arrowdown" src="pic/trans.gif" />0
</td></tr></table>
</body></html>`

const soulvoiceUserdetailsFixture = `<html><body>
<table>
  <tr><td class="rowhead">加入日期</td><td class="rowfollow">2020-01-01 00:00:00 (<span title="2020-01-01 00:00:00">很久前</span>)</td></tr>
  <tr><td class="rowhead">传输</td><td class="rowfollow">
    <table><tr><td class="embedded"><strong>分享率</strong>: <font>8.183</font></td></tr>
    <tr><td class="embedded"><strong>上传量</strong>: 64.00 GB</td><td class="embedded"><strong>下载量</strong>: 8.00 GB</td><td class="embedded"><strong>做种积分</strong>: 3000</td></tr></table>
  </td></tr>
  <tr><td class="rowhead">等级</td><td class="rowfollow"><img title="User" src="pic/user.gif" /></td></tr>
</table>
</body></html>`

func getSoulVoiceDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("soulvoice")
	require.True(t, ok, "soulvoice definition not found")
	return def
}

func testSoulVoiceSearch(t *testing.T) {
	def := getSoulVoiceDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(soulvoiceSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "210481", items[0].ID)
	assert.Equal(t, "Whistle.2026.1080p.iTunes.WEB-DL", items[0].Title)
	assert.Equal(t, "索命哨", items[0].Subtitle)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 1, items[0].Seeders)
	assert.Equal(t, 16, items[0].Leechers)
	assert.Equal(t, 0, items[0].Snatched)

	assert.Equal(t, "210480", items[1].ID)
	assert.Equal(t, v2.DiscountNone, items[1].DiscountLevel)
}

func testSoulVoiceDetail(t *testing.T) {
	def := getSoulVoiceDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	t.Run("Free", func(t *testing.T) {
		doc := FixtureDoc(t, "soulvoice_detail", soulvoiceDetailFixture)
		info := parser.ParseAll(doc.Selection)
		assert.Equal(t, "210481", info.TorrentID)
		assert.Equal(t, "Whistle.2026.1080p.iTunes.WEB-DL", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		assert.InDelta(t, 5.29*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})

	t.Run("HR", func(t *testing.T) {
		doc := FixtureDoc(t, "soulvoice_detail_hr", soulvoiceDetailHRFixture)
		info := parser.ParseAll(doc.Selection)
		assert.Equal(t, "210499", info.TorrentID)
		assert.True(t, info.HasHR)
	})
}

func testSoulVoiceUserInfo(t *testing.T) {
	def := getSoulVoiceDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "soulvoice_index", soulvoiceIndexFixture)
		expected := map[string]string{
			"id":       "150574",
			"name":     "qinmina",
			"bonus":    "18461.6",
			"seeding":  "199",
			"leeching": "0",
		}
		for field, want := range expected {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok)
			assert.Equal(t, want, driver.ExtractFieldValuePublic(doc, sel))
		}
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "soulvoice_userdetails", soulvoiceUserdetailsFixture)
		expected := map[string]string{
			"uploaded":   "68719476736",
			"downloaded": "8589934592",
			"ratio":      "8.183",
			"levelName":  "User",
			"joinTime":   "1577836800",
		}
		for field, want := range expected {
			sel, ok := def.UserInfo.Selectors[field]
			require.True(t, ok)
			assert.Equal(t, want, driver.ExtractFieldValuePublic(doc, sel))
		}
	})
}

func TestSoulVoice_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      soulvoiceSearchFixture,
		"detail":      soulvoiceDetailFixture,
		"detail_hr":   soulvoiceDetailHRFixture,
		"index":       soulvoiceIndexFixture,
		"userdetails": soulvoiceUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
