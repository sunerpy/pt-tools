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
		SiteID:   "byrpt",
		Search:   testBYRPTSearch,
		Detail:   testBYRPTDetail,
		UserInfo: testBYRPTUserInfo,
	})
}

const byrptSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr class='free_bg'>
  <td class='rowfollow nowrap'><a href='upload.php?quote=361600' title='引用该种子发布'><div class='icons quote'></div></a></td>
  <td class="rowfollow nowrap" style='padding: 0'><div class="cat-icon-merge"><span class="cat-icon cat-403"><a href="torrents.php?cat=403" class="cat-link">游戏</a></span></div></td>
  <td class='rowfollow'><table class='torrentname full transparentbg'><tr><td class='embedded' style='width: 99%'><a target='_self' title='[PC][Sample Game]' href="details.php?id=361600&amp;hit=1"><span class="bold">[PC][Sample Game]</span></a>&nbsp;<img class='pro_free' src='/pic/trans.gif' title='不计下载量' alt=''> <span class='bold'>(<span style='color: blue'>剩余时间：<span title='2026-03-24 18:25:02'>1天 3时</span></span>)</span><br /><span>样例副标题</span></td><td class="embedded" style="text-align: right;"><a href='download.php?id=361600'><div class='icons download' title='下载本种'></div></a></td></tr></table></td>
  <td class="rowfollow"><span title="无新评论">0</span></td>
  <td class="rowfollow nowrap">2026-03-22<br />18:23:16</td>
  <td class="rowfollow">3.76<br />GiB</td>
  <td class="rowfollow"><b><a href="details.php?id=361600&amp;hit=1&amp;dllist=1#seeders">51</a></b></td>
  <td class="rowfollow">0</td>
  <td class="rowfollow"><a href="viewsnatches.php?id=361600"><b>63</b></a></td>
</tr>
<tr class='halfdown_bg'>
  <td class='rowfollow nowrap'><a href='upload.php?quote=356898'><div class='icons quote'></div></a></td>
  <td class="rowfollow nowrap" style='padding: 0'><div class="cat-icon-merge"><span class="cat-icon cat-403"><a href="torrents.php?cat=403" class="cat-link">游戏</a></span></div></td>
  <td class='rowfollow'><table class='torrentname full transparentbg'><tr><td class='embedded' style='width: 99%'><a target='_self' title='[PC][Another Game]' href="details.php?id=356898&amp;hit=1"><span class="bold">[PC][Another Game]</span></a><img class='pro_50pctdown' src='/pic/trans.gif' title='计50%下载量' alt=''><br /><span>半价副标题</span></td><td class="embedded" style="text-align: right;"><a href='download.php?id=356898'><div class='icons download' title='下载本种'></div></a></td></tr></table></td>
  <td class="rowfollow"><span title="无新评论">0</span></td>
  <td class="rowfollow nowrap">2025-06-09<br />22:10:08</td>
  <td class="rowfollow">64.88<br />GiB</td>
  <td class="rowfollow">3</td>
  <td class="rowfollow">0</td>
  <td class="rowfollow">97</td>
</tr>
</tbody></table>
</body></html>`

const byrptIndexFixture = `<html><body>
<td class="bottom navbar-user-data">
欢迎, <span class='nowrap'><a class='ExtremeUser_Name' href='/userdetails.php?id=308953'><span style='font-weight: bold'>nav_user</span></a></span>
<span class='color_bonus'>魔力值 </span>[<a href="mybonus.php">明细</a>]: 1424274.9
<span class="color_ratio">分享率：</span>8.26
<span class='color_uploaded'>上传量：</span>8.610 TiB
<span class='color_downloaded'> 下载量：</span>1.042 TiB
<span class='color_active'>当前活动：</span><div class='icons std_head arrowup' title="当前做种"></div>10<div class='icons std_head arrowdown' title="当前下载"></div>0
</td>
</body></html>`

const byrptUserdetailsFixture = `<html><body>
<table class="rowtable full">
<tr><td class="rowhead nowrap">用户ID/UID</td><td class="rowfollow">344402</td></tr>
<tr><td class="rowhead nowrap">加入日期</td><td class="rowfollow">2021-04-13 18:05:04 (<span title='2021-04-13 18:05:04'>4年 11月前</span>)</td></tr>
<tr><td class="rowhead nowrap">传输</td><td class="rowfollow"><table class='block-left'><tr><td class="embedded"><strong>分享率</strong>: <span>165.662</span></td></tr><tr><td class="embedded"><strong>上传量</strong>: 113.666 TiB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>: 702.60 GiB</td></tr></table></td></tr>
<tr><td class="rowhead nowrap">等级</td><td class="rowfollow"><img alt="Ultimate User" title="Ultimate User" src="pic/ultimate.gif" /></td></tr>
<tr><td class="rowhead nowrap">魔力值</td><td class="rowfollow">1192520.0</td></tr>
</table>
</body></html>`

const byrptDetailFixture = `<html><body>
<h1 style='text-align: center' id='share'>[PC][Sample Game]&nbsp;&nbsp;&nbsp;<br><font class='free'>免费</font> <span title='2026-03-24 18:25:02'>1天 3时</span></h1>
<table class='rowtable wide'>
<tr><td class='rowhead'>下载</td><td class='rowfollow'><a class='index' href='download.php?id=361600'>[BYRBT].sample.torrent</a></td></tr>
<tr><td class='rowhead nowrap'>副标题</td><td class='rowfollow'><div id='subtitle'><li style='list-style-type: none'>样例副标题</li></div></td></tr>
<tr><td class='rowhead nowrap'>基本信息</td><td class='rowfollow'><b><b>大小：</b></b>3.76 GB&nbsp;&nbsp;|&nbsp;&nbsp;<b>类型：</b><span id='type'>游戏</span></td></tr>
<tr><td class='rowhead'>字幕</td><td class='rowfollow'><form method='post' action='subtitles.php'><input type='hidden' name='torrent_name' value='[PC][Sample Game]' /><input type='hidden' name='detail_torrent_id' value='361600' /></form></td></tr>
</table>
</body></html>`

const byrptDetailWithHalfDownFixture = `<html><body>
<h1 style='text-align: center' id='share'>[PC][Another Game]&nbsp;&nbsp;&nbsp;<br><font class='halfdown'>50%</font></h1>
<table class='rowtable wide'>
<tr><td class='rowhead'>下载</td><td class='rowfollow'><a class='index' href='download.php?id=356898'>[BYRBT].half.torrent</a></td></tr>
<tr><td class='rowhead nowrap'>基本信息</td><td class='rowfollow'><b><b>大小：</b></b>64.88 GB</td></tr>
<tr><td class='rowhead'>字幕</td><td class='rowfollow'><form method='post' action='subtitles.php'><input type='hidden' name='torrent_name' value='[PC][Another Game]' /><input type='hidden' name='detail_torrent_id' value='356898' /></form></td></tr>
</table>
</body></html>`

func getBYRPTDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("byrpt")
	require.True(t, ok)
	return def
}

func testBYRPTSearch(t *testing.T) {
	def := getBYRPTDef(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(byrptSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1", Selectors: def.Selectors})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)
	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "361600", items[0].ID)
	assert.Equal(t, "[PC][Sample Game]", items[0].Title)
	assert.Equal(t, "样例副标题", items[0].Subtitle)
	assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 51, items[0].Seeders)
	assert.Equal(t, 0, items[0].Leechers)
	assert.Equal(t, 63, items[0].Snatched)

	assert.Equal(t, "356898", items[1].ID)
	assert.Equal(t, v2.DiscountPercent50, items[1].DiscountLevel)
}

func testBYRPTDetail(t *testing.T) {
	def := getBYRPTDef(t)
	parser := v2.NewNexusPHPParserFromDefinition(def)

	info := parser.ParseAll(FixtureDoc(t, "byrpt_detail", byrptDetailFixture).Selection)
	assert.Equal(t, "[PC][Sample Game]", info.Title)
	assert.Equal(t, "361600", info.TorrentID)
	assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 3.76*1024, info.SizeMB, 0.1)

	half := parser.ParseAll(FixtureDoc(t, "byrpt_detail_halfdown", byrptDetailWithHalfDownFixture).Selection)
	assert.Equal(t, "356898", half.TorrentID)
	assert.Equal(t, v2.DiscountPercent50, half.DiscountLevel)
	assert.InDelta(t, 64.88*1024, half.SizeMB, 0.1)
}

func testBYRPTUserInfo(t *testing.T) {
	def := getBYRPTDef(t)
	driver := newTestNexusPHPDriver(def)

	indexDoc := FixtureDoc(t, "byrpt_index", byrptIndexFixture)
	assert.Equal(t, "308953", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["id"]))
	assert.Equal(t, "nav_user", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["name"]))
	assert.Equal(t, "1.4242749e+06", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["bonus"]))
	assert.Equal(t, "10", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["seeding"]))
	assert.Equal(t, "0", driver.ExtractFieldValuePublic(indexDoc, def.UserInfo.Selectors["leeching"]))

	userDoc := FixtureDoc(t, "byrpt_userdetails", byrptUserdetailsFixture)
	assert.Equal(t, "124977088682786", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["uploaded"]))
	assert.Equal(t, "754411005542", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["downloaded"]))
	assert.Equal(t, "165.662", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["ratio"]))
	assert.Equal(t, "Ultimate User", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["levelName"]))
	assert.Equal(t, "1618337104", driver.ExtractFieldValuePublic(userDoc, def.UserInfo.Selectors["joinTime"]))
}

func TestBYRPT_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":   byrptSearchFixture,
		"index":    byrptIndexFixture,
		"userinfo": byrptUserdetailsFixture,
		"detail":   byrptDetailFixture,
		"halfdown": byrptDetailWithHalfDownFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
