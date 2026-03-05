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
		SiteID:   "hdfans",
		Search:   testHDFansSearch,
		Detail:   testHDFansDetail,
		UserInfo: testHDFansUserInfo,
	})
}

const hdfansSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="Movies/电影" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=212555">The.Judge.Returns.S01.1080p.WEB-DL</a>
    <img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-03-05 09:16:57&quot;&gt;23时46分钟&lt;/span&gt;&lt;/b&gt;', 'trail', false)" />
    <br/><span>法官李汉英法官 / 全14集</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-03-04 09:16:57">13分钟</span></td>
  <td class="rowfollow">10.57 GB</td>
  <td class="rowfollow">2</td>
  <td class="rowfollow">25</td>
  <td class="rowfollow">1</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
  <td class="rowfollow"><img alt="TV Series/电视剧" /></td>
  <td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
    <a href="details.php?id=212554">Side.Beat.S01.1080p.WEB-DL</a>
    <br/><span>法医报告</span>
  </td></tr></table></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><span title="2026-03-04 08:53:04">37分钟</span></td>
  <td class="rowfollow">13.69 GB</td>
  <td class="rowfollow">0</td>
  <td class="rowfollow">27</td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><i>匿名</i></td>
</tr>
</tbody></table>
</body></html>`

const hdfansIndexFixture = `<html><body>
<div id="info_block">
  欢迎回来, <a href="https://hdfans.org/userdetails.php?id=61994" class="User_Name"><b>qinmina</b></a>
  <font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 14,964.0
  <font class="color_ratio">分享率:</font> 11.356 <font class='color_uploaded'>上传量:</font> 17.020 TB <font class='color_downloaded'> 下载量:</font> 1.499 TB
  当前活动: <img class="arrowup" src="pic/trans.gif" />113 <img class="arrowdown" src="pic/trans.gif" />0
</div>
</body></html>`

const hdfansUserdetailsFixture = `<html><body>
<table>
  <tr><td class="rowhead">等级</td><td><img title="Extreme User" src="pic/extreme.gif" /></td></tr>
  <tr><td class="rowhead">加入日期</td><td>2020-01-15 12:30:00 (5年前)</td></tr>
</table>
</body></html>`

const hdfansDetailFixture = `<html><body>
<h1 value="Counterpunch.2017.1080p.WEB-DL">
  <input name="torrent_name" type="hidden" value="Counterpunch.2017.1080p.WEB-DL" />
  <input name="detail_torrent_id" type="hidden" value="44336" />
  <font class="free">免费</font>
  <span title="2026-03-14 11:56:51">9天23时</span>
</h1>
<table><tr><td class="rowhead">基本信息</td><td class="rowfollow">大小</td><td class="rowfollow">大小：3.55 GB | 类型：纪录片</td></tr></table>
</body></html>`

const hdfansDetailWithHRFixture = `<html><body>
<h1 value="Counterpunch.2017.1080p.WEB-DL.HR">
  <input name="torrent_name" type="hidden" value="Counterpunch.2017.1080p.WEB-DL.HR" />
  <input name="detail_torrent_id" type="hidden" value="44337" />
</h1>
<table><tr><td class="rowhead">基本信息</td><td class="rowfollow">大小</td><td class="rowfollow">大小：1.00 GB | 类型：纪录片</td></tr></table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

func getHDFansDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("hdfans")
	require.True(t, ok)
	return def
}

func testHDFansSearch(t *testing.T) {
	def := getHDFansDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(hdfansSearchFixture))
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
	assert.Equal(t, "212555", first.ID)
	assert.Equal(t, "The.Judge.Returns.S01.1080p.WEB-DL", first.Title)
	assert.Equal(t, "法官李汉英法官 / 全14集", first.Subtitle)
	assert.Equal(t, v2.DiscountFree, first.DiscountLevel)
	assert.Equal(t, 2, first.Seeders)
	assert.Equal(t, 25, first.Leechers)
	assert.Equal(t, 1, first.Snatched)

	second := items[1]
	assert.Equal(t, "212554", second.ID)
	assert.Equal(t, v2.DiscountNone, second.DiscountLevel)
}

func testHDFansDetail(t *testing.T) {
	def := getHDFansDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	doc := FixtureDoc(t, "hdfans_detail", hdfansDetailFixture)
	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "44336", info.TorrentID)
	assert.Equal(t, "Counterpunch.2017.1080p.WEB-DL", info.Title)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 3.55*1024, info.SizeMB, 0.1)
	assert.False(t, info.HasHR)

	hrDoc := FixtureDoc(t, "hdfans_detail_hr", hdfansDetailWithHRFixture)
	hrInfo := parser.ParseAll(hrDoc.Selection)
	assert.Equal(t, "44337", hrInfo.TorrentID)
	assert.Equal(t, v2.DiscountNone, hrInfo.DiscountLevel)
	assert.True(t, hrInfo.HasHR)
}

func testHDFansUserInfo(t *testing.T) {
	def := getHDFansDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "hdfans_index", hdfansIndexFixture)
	assert.Equal(t, "61994", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "qinmina", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "18713687904747", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "1648167930036", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "11.356", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "14964", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "113", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "hdfans_userdetails", hdfansUserdetailsFixture)
	assert.Equal(t, "Extreme User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "1579091400", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["joinTime"]))
}

func TestHDFans_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      hdfansSearchFixture,
		"index":       hdfansIndexFixture,
		"userdetails": hdfansUserdetailsFixture,
		"detail":      hdfansDetailFixture,
		"detail_hr":   hdfansDetailWithHRFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
