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
		SiteID:   "baozi",
		Search:   testBaoziSearch,
		Detail:   testBaoziDetail,
		UserInfo: testBaoziUserInfo,
	})
}

// --- Fixtures (anonymized, modeled on real p.t-baozi.cc HTML) ---

const baoziSearchFixture = `<html><body>
<table class="torrents table2" cellspacing="0" cellpadding="5" width="100%">
<tr class="table2_title">
	<td class="colhead">类型</td>
	<td class="colhead">标题</td>
	<td class="colhead">评论数</td>
	<td class="colhead">存活时间</td>
	<td class="colhead">大小</td>
	<td class="colhead">种子数</td>
	<td class="colhead">下载数</td>
	<td class="colhead">完成数</td>
	<td class="colhead">发布者</td>
</tr>
<tr>
	<td class="rowfollow nowrap" valign="middle"><a href="?cat=402"><img class="c_tvseries" src="pic/cattrans.gif" alt="剧集" title="剧集" /></a></td>
	<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr><td class="embedded"><a title="Sample.Show.The.Complete.Series.1994-2003.BluRay.1080p.x265.10bit-GRP" href="details.php?id=7&amp;hit=1"><b>Sample.Show.The.Complete.Series.1994-2003.BluRay.1080p.x265.10bit-GRP</b></a> <img class="pro_free2up" src="pic/trans.gif" alt="2X Free" title="2X免费" /><br /><span title="中文字幕">中字</span><span title="">HDR</span>【样例剧集全十季】 十年朋友 一生珍藏 10bit HEVC版本</td><td width="20" class="embedded"><a href="download.php?id=7"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td></tr></table></td>
	<td class="rowfollow"><a href="comment.php?action=add&amp;pid=7&amp;type=torrent">0</a></td>
	<td class="rowfollow nowrap"><span title="2025-11-29 14:46:01">7月<br />16天</span></td>
	<td class="rowfollow">190.36<br />GB</td>
	<td class="rowfollow" align="center"><b><a href="details.php?id=7&amp;hit=1&amp;dllist=1#seeders">79</a></b></td>
	<td class="rowfollow"><b><a href="details.php?id=7&amp;hit=1&amp;dllist=1#leechers">1</a></b></td>
	<td class="rowfollow"><a href="viewsnatches.php?id=7"><b>135</b></a></td>
	<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
	<td class="rowfollow nowrap" valign="middle"><a href="?cat=402"><img class="c_tvseries" src="pic/cattrans.gif" alt="剧集" title="剧集" /></a></td>
	<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr><td class="embedded"><a title="Sample.Series.2026.S03E02.2160p.WEB-DL.HEVC-GRP" href="details.php?id=59250&amp;hit=1"><b>Sample.Series.2026.S03E02.2160p.WEB-DL.HEVC-GRP</b></a> <img class="pro_free2" src="/styles/Baozi/torrents_icon10.png" alt="Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-07-14 16:34:27&quot;&gt;23时55分钟&lt;/span&gt;&lt;/b&gt;', 'trail', false);" /> <font color='#0000FF'>剩余时间：<span title="2026-07-14 16:34:27">23时55分钟</span></font><br /><span title="中文字幕">中字</span>样例剧集</td><td width="20" class="embedded"><a href="download.php?id=59250"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a></td></tr></table></td>
	<td class="rowfollow"><a href="comment.php?action=add&amp;pid=59250&amp;type=torrent">3</a></td>
	<td class="rowfollow nowrap"><span title="2026-07-13 16:00:00">1天</span></td>
	<td class="rowfollow">5.42<br />GB</td>
	<td class="rowfollow" align="center"><b><a href="details.php?id=59250&amp;dllist=1#seeders">50</a></b></td>
	<td class="rowfollow"><b><a href="details.php?id=59250&amp;dllist=1#leechers">10</a></b></td>
	<td class="rowfollow"><a href="viewsnatches.php?id=59250"><b>200</b></a></td>
	<td class="rowfollow"><i>匿名</i></td>
</tr>
</table>
</body></html>`

const baoziIndexFixture = `<html><body>
<div id="info_block">
	<span class="nowrap"><a href="https://p.t-baozi.cc/userdetails.php?id=13143" class='User_Name'><b>sample_user</b></a></span>
	<div id="menu_lists_text">
		<div><span>分享率:</span> 无限</div>
		<div><span>上传量:</span> 100.00 GB</div>
		<div><span>下载量:</span> 0.00 KB</div>
		<div><span>当前活动:</span> <img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />7  <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />2&nbsp;&nbsp;</div>
		<div><a href="mybonus.php"><span>魔力值 ：</span></a> 12345.6 <a href="attendance.php">[签到已得1]</a></div>
	</div>
</div>
</body></html>`

const baoziUserdetailsFixture = `<html><body>
<table>
	<tr>
		<td class="rowhead nowrap">用户ID/UID</td>
		<td class="rowfollow">13143</td>
	</tr>
	<tr>
		<td class="rowhead nowrap">加入日期</td>
		<td class="rowfollow">2025-12-02 09:36:15 (<span title="2025-12-02 09:36:15">7月13天前</span>, 31.9周)</td>
	</tr>
	<tr>
		<td class="rowhead nowrap">最近动向</td>
		<td class="rowfollow">2026-07-13 16:05:55 (<span title="2026-07-13 16:05:55">32分钟前</span>)</td>
	</tr>
	<tr>
		<td class="rowhead nowrap">传输</td>
		<td class="rowfollow"><table border="0"><tr><td class="embedded"><strong>上传量</strong>: 30.610 TB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>: 4.50 GB</td></tr><tr><td class="embedded"><strong>实际上传量</strong>: 15.311 TB</td><td class="embedded">&nbsp;&nbsp;<strong>实际下载量</strong>: 2.75 GB</td></tr></table></td>
	</tr>
	<tr>
		<td class="rowhead nowrap">等级</td>
		<td class="rowfollow"><img alt="Power User" title="Power User" src="pic/power.gif" /> </td>
	</tr>
</table>
</body></html>`

const baoziDetailFixture = `<html><body>
<h1 align="center" id="top">
	<input type="hidden" name="torrent_name" value="Sample.Show.The.Complete.Series.1994-2003.BluRay.1080p.x265.10bit-GRP" />
	<input type="hidden" name="detail_torrent_id" value="7" />
	Sample.Show.The.Complete.Series.1994-2003.BluRay.1080p.x265.10bit-GRP&nbsp;&nbsp; <b>[<font class='twoupfree'>2X免费</font>]</b>
</h1>
<table>
	<tr><td class="rowhead nowrap">副标题</td><td class="rowfollow" align="left">【样例剧集全十季】 十年朋友 一生珍藏 10bit HEVC版本</td></tr>
	<tr><td class="rowhead nowrap">基本信息</td><td class="rowfollow" align="left"><b><b>大小：</b></b>190.36 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;剧集&nbsp;&nbsp;&nbsp;<b>媒介: </b>Blu-ray 原盘</td></tr>
</table>
</body></html>`

const baoziDetailHRFixture = `<html><body>
<h1 align="center" id="top">
	<input type="hidden" name="torrent_name" value="Sample.Movie.2026.WEB-DL.1080p-GRP" />
	<input type="hidden" name="detail_torrent_id" value="8" />
	Sample.Movie.2026.WEB-DL.1080p-GRP
</h1>
<table>
	<tr><td class="rowhead nowrap">基本信息</td><td class="rowfollow" align="left"><b><b>大小：</b></b>8.50 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;电影</td></tr>
</table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

// --- Helper ---

func getBaoziDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("baozi")
	require.True(t, ok, "baozi definition not found")
	return def
}

// --- Suite: Search ---

func testBaoziSearch(t *testing.T) {
	def := getBaoziDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(baoziSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{
		BaseURL:   server.URL,
		Cookie:    "test_cookie=1",
		Selectors: def.Selectors,
	})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2, "should parse 2 torrent rows")

	first := items[0]
	assert.Equal(t, "7", first.ID)
	assert.Equal(t, "Sample.Show.The.Complete.Series.1994-2003.BluRay.1080p.x265.10bit-GRP", first.Title)
	assert.Equal(t, v2.Discount2xFree, first.DiscountLevel)
	assert.Equal(t, 79, first.Seeders)
	assert.Equal(t, 1, first.Leechers)
	assert.Equal(t, 135, first.Snatched)
	assert.True(t, first.SizeBytes > 0, "size should be parsed")

	// Second row: free with end time embedded in onmouseover tooltip (Pitfall #11)
	second := items[1]
	assert.Equal(t, "59250", second.ID)
	assert.Equal(t, v2.DiscountFree, second.DiscountLevel)
	assert.False(t, second.DiscountEndTime.IsZero(), "free-end time should parse from onmouseover")
	assert.Equal(t, 2026, second.DiscountEndTime.Year())
	assert.Equal(t, 7, int(second.DiscountEndTime.Month()))
	assert.Equal(t, 14, second.DiscountEndTime.Day())
	assert.Equal(t, 50, second.Seeders)
	assert.Equal(t, 10, second.Leechers)
	assert.Equal(t, 200, second.Snatched)
}

// --- Suite: Detail ---

func testBaoziDetail(t *testing.T) {
	def := getBaoziDef(t)

	t.Run("TwoUpFree", func(t *testing.T) {
		doc := FixtureDoc(t, "baozi_detail", baoziDetailFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "7", info.TorrentID)
		assert.Equal(t, "Sample.Show.The.Complete.Series.1994-2003.BluRay.1080p.x265.10bit-GRP", info.Title)
		assert.Equal(t, v2.Discount2xFree, info.DiscountLevel)
		assert.InDelta(t, 190.36*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})

	t.Run("WithHR", func(t *testing.T) {
		doc := FixtureDoc(t, "baozi_detail_hr", baoziDetailHRFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "8", info.TorrentID)
		assert.True(t, info.HasHR, "should detect HR from hit_run.gif")
		assert.InDelta(t, 8.50*1024, info.SizeMB, 0.1)
	})
}

// --- Suite: UserInfo ---

func testBaoziUserInfo(t *testing.T) {
	def := getBaoziDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "baozi_index", baoziIndexFixture)
		fields := map[string]string{
			"id":       "13143",
			"name":     "sample_user",
			"seeding":  "7",
			"leeching": "2",
			"bonus":    "12345.6",
		}
		for field, expected := range fields {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q not found", field)
				got := driver.ExtractFieldValuePublic(doc, sel)
				assert.Equal(t, expected, got)
			})
		}
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "baozi_userdetails", baoziUserdetailsFixture)
		exact := map[string]string{
			"uploaded":   "33656050926223",
			"downloaded": "4831838208",
			"levelName":  "Power User",
		}
		for field, expected := range exact {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q not found", field)
				got := driver.ExtractFieldValuePublic(doc, sel)
				assert.Equal(t, expected, got)
			})
		}
	})

	// Login-state probe (保号) depends on lastAccessAt → LastAccess > 0 (Pitfall #16)
	t.Run("LastAccess", func(t *testing.T) {
		doc := FixtureDoc(t, "baozi_userdetails", baoziUserdetailsFixture)
		sel, ok := def.UserInfo.Selectors["lastAccessAt"]
		require.True(t, ok, "lastAccessAt selector required for 保号 probe")
		got := driver.ExtractFieldValuePublic(doc, sel)
		require.NotEmpty(t, got, "lastAccessAt must parse to a non-empty timestamp")
		assert.NotEqual(t, "0", got, "lastAccessAt must be a positive Unix timestamp")
	})
}

// --- Standalone Tests ---

func TestBaozi_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      baoziSearchFixture,
		"index":       baoziIndexFixture,
		"userdetails": baoziUserdetailsFixture,
		"detail":      baoziDetailFixture,
		"detail_hr":   baoziDetailHRFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
