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
		SiteID:   "tangpt",
		Search:   testTangPTSearch,
		Detail:   testTangPTDetail,
		UserInfo: testTangPTUserInfo,
	})
}

// --- Fixtures (anonymized, modeled on real www.tangpt.top HTML) ---

const tangptSearchFixture = `<html><body>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%">
<tr>
	<td class="colhead">类型</td>
	<td class="colhead">标题</td>
	<td class="colhead"><img alt="comments" title="评论数" /></td>
	<td class="colhead"><img alt="time" title="存活时间" /></td>
	<td class="colhead"><img alt="size" title="大小" /></td>
	<td class="colhead"><img alt="seeders" title="种子数" /></td>
	<td class="colhead"><img alt="leechers" title="下载数" /></td>
	<td class="colhead"><img alt="snatched" title="完成数" /></td>
	<td class="colhead">发布者</td>
</tr>
<tr>
	<td class="rowfollow nowrap" valign="middle"><a href="?cat=404"><img class="c_doc" alt="纪录片" title="纪录片" /></a></td>
	<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr><td class="embedded"><img class="nexus-lazy-load" /></td><td class="embedded"><a title="Sample.Doc.2026.1080p.WEB-DL.H264.AAC-TPWEB" href="details.php?id=20986&amp;hit=1"><b>Sample.Doc.2026.1080p.WEB-DL.H264.AAC-TPWEB</b></a> <b>[<font class='hot'>热门</font>]</b> <img class="pro_free" src="pic/trans.gif" alt="Free" title="免费" /><br /><span title="示例纪录片副标题">示例纪录片副标题</span></td></tr></table></td>
	<td class="rowfollow"><b><a href="details.php?id=20986&amp;hit=1&amp;cmtpage=1#startcomments">1</a></b></td>
	<td class="rowfollow nowrap"><span title="2026-01-17 22:01:11">5月<br />26天</span></td>
	<td class="rowfollow">506.78<br />MB</td>
	<td class="rowfollow" align="center"><b><a href="details.php?id=20986&amp;hit=1&amp;dllist=1#seeders">119</a></b></td>
	<td class="rowfollow">0</td>
	<td class="rowfollow"><a href="viewsnatches.php?id=20986"><b>313</b></a></td>
	<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
	<td class="rowfollow nowrap" valign="middle"><a href="?cat=409"><img class="c_hqaudio" alt="音乐" title="音乐" /></a></td>
	<td class="rowfollow" width="100%" align="left"><table class="torrentname" width="100%"><tr><td class="embedded"><img class="nexus-lazy-load" /></td><td class="embedded"><a title="Sample.Audio.Pack.MP3-TPAudio" href="details.php?id=34917&amp;hit=1"><b>Sample.Audio.Pack.MP3-TPAudio</b></a> <img class="pro_50pctdown2up" src="pic/trans.gif" alt="2X 50%" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;twouphalfdown&quot;&gt;2X 50%&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-08-22 13:27:58&quot;&gt;1月9天&lt;/span&gt;&lt;/b&gt;', 'trail', false);" /> 剩余时间：<span title="2026-08-22 13:27:58">1月9天</span><br /><span title="示例音乐副标题">示例音乐副标题</span></td></tr></table></td>
	<td class="rowfollow"><b>0</b></td>
	<td class="rowfollow nowrap"><span title="2026-05-01 10:00:00">2月<br />1天</span></td>
	<td class="rowfollow">1.20<br />GB</td>
	<td class="rowfollow" align="center"><b>50</b></td>
	<td class="rowfollow">3</td>
	<td class="rowfollow"><b>8</b></td>
	<td class="rowfollow"><i>匿名</i></td>
</tr>
</table>
</body></html>`

const tangptIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr><td>
	<span class="medium"> 欢迎回来, <span class="nowrap"><a href="https://www.tangpt.top/userdetails.php?id=13199" class='User_Name'><b>sampleuser</b></a></span> [<a href="logout.php">退出</a>]
	<font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 10,100.0
	<font class="color_ratio">分享率:</font> 无限
	<font class='color_uploaded'>上传量:</font> 100.00 GB
	<font class='color_active'>当前活动:</font> <img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />7 <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />2
</td></tr></table>
</body></html>`

const tangptUserdetailsFixture = `<html><body>
<table>
	<tr>
		<td class="rowhead">用户ID/UID</td>
		<td class="rowfollow">13199</td>
	</tr>
	<tr>
		<td class="rowhead">加入日期</td>
		<td class="rowfollow">2026-07-13 16:05:24 (<span title="2026-07-13 16:05:24">16分钟前</span>, 0周)</td>
	</tr>
	<tr>
		<td class="rowhead">最近动向</td>
		<td class="rowfollow">2026-07-13 16:21:37 (<span title="2026-07-13 16:21:37">&lt; 1分钟前</span>)</td>
	</tr>
	<tr>
		<td class="rowhead">传输</td>
		<td width="99%" class="rowfollow" valign="top" align="left"><table border="0"><tr><td class="embedded"><strong>上传量</strong>: 100.00 GB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>: 512.00 MB</td></tr></table></td>
	</tr>
	<tr>
		<td class="rowhead">等级</td>
		<td class="rowfollow"><img alt="User" title="User" src="pic/user.gif" /></td>
	</tr>
	<tr>
		<td class="rowhead">魔力值</td>
		<td class="rowfollow">10,100.0</td>
	</tr>
</table>
</body></html>`

const tangptDetailFixture = `<html><body>
<h1>
	<input name="torrent_name" type="hidden" value="Sample.Doc.2026.1080p.WEB-DL.H264.AAC-TPWEB" />
	<input name="detail_torrent_id" type="hidden" value="20986" />
	<b>[<font class='free'>免费</font>]</b>
</h1>
<table>
	<tr>
		<td class="rowhead">基本信息</td>
		<td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>506.78 MB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;纪录片&nbsp;&nbsp;&nbsp;<b>媒介: </b>WEB-DL</td>
	</tr>
</table>
</body></html>`

const tangptDetailWithHRFixture = `<html><body>
<h1>
	<input name="torrent_name" type="hidden" value="Sample.HR.2026.WEB-DL" />
	<input name="detail_torrent_id" type="hidden" value="20987" />
</h1>
<table>
	<tr>
		<td class="rowhead">基本信息</td>
		<td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>8.50 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;电影</td>
	</tr>
</table>
<img src="pic/hit_run.gif" alt="Hit and Run" />
</body></html>`

// --- Helpers ---

func getTangPTDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("tangpt")
	require.True(t, ok, "tangpt definition not found")
	return def
}

// --- Suite: Search ---

func testTangPTSearch(t *testing.T) {
	def := getTangPTDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(tangptSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{
		BaseURL:   server.URL,
		Cookie:    "test_cookie=1",
		Selectors: def.Selectors,
	})
	driver.SetSiteDefinition(def)

	req := v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"}
	res, err := driver.Execute(context.Background(), req)
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2, "should parse 2 torrent rows")

	free := items[0]
	assert.Equal(t, "20986", free.ID)
	assert.Equal(t, "Sample.Doc.2026.1080p.WEB-DL.H264.AAC-TPWEB", free.Title)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	assert.Equal(t, 119, free.Seeders)
	assert.Equal(t, 0, free.Leechers)
	assert.Equal(t, 313, free.Snatched)
	assert.True(t, free.SizeBytes > 0, "size should be parsed")

	second := items[1]
	assert.Equal(t, "34917", second.ID)
	assert.Equal(t, v2.DiscountPercent50, second.DiscountLevel)
	assert.False(t, second.DiscountEndTime.IsZero(), "discount end time should be parsed from onmouseover")
	assert.Equal(t, 2026, second.DiscountEndTime.Year())
	assert.Equal(t, 8, int(second.DiscountEndTime.Month()))
	assert.Equal(t, 50, second.Seeders)
	assert.Equal(t, 3, second.Leechers)
	assert.Equal(t, 8, second.Snatched)
}

// --- Suite: Detail ---

func testTangPTDetail(t *testing.T) {
	def := getTangPTDef(t)

	t.Run("Free", func(t *testing.T) {
		doc := FixtureDoc(t, "tangpt_detail", tangptDetailFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "20986", info.TorrentID)
		assert.Equal(t, "Sample.Doc.2026.1080p.WEB-DL.H264.AAC-TPWEB", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		assert.InDelta(t, 506.78, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})

	t.Run("WithHR", func(t *testing.T) {
		doc := FixtureDoc(t, "tangpt_detail_hr", tangptDetailWithHRFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "20987", info.TorrentID)
		assert.True(t, info.HasHR, "should detect HR from hit_run.gif")
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel)
		assert.InDelta(t, 8.5*1024, info.SizeMB, 0.1)
	})
}

// --- Suite: UserInfo ---

func testTangPTUserInfo(t *testing.T) {
	def := getTangPTDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "tangpt_index", tangptIndexFixture)
		fields := map[string]string{
			"id":       "13199",
			"name":     "sampleuser",
			"seeding":  "7",
			"leeching": "2",
			"bonus":    "10100",
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
		doc := FixtureDoc(t, "tangpt_userdetails", tangptUserdetailsFixture)
		exact := map[string]string{
			"uploaded":   "107374182400",
			"downloaded": "536870912",
			"levelName":  "User",
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

	t.Run("LastAccess", func(t *testing.T) {
		doc := FixtureDoc(t, "tangpt_userdetails_la", tangptUserdetailsFixture)
		sel, ok := def.UserInfo.Selectors["lastAccessAt"]
		require.True(t, ok, "lastAccessAt selector not found")
		got := driver.ExtractFieldValuePublic(doc, sel)
		// 2026-07-13 16:21:37 +0800 => unix 1783930897; must be > 0 so 保号 probe gets LastAccess
		assert.Equal(t, "1783930897", got)
	})
}

// --- Standalone: Secret leak guard ---

func TestTangPT_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":      tangptSearchFixture,
		"index":       tangptIndexFixture,
		"userdetails": tangptUserdetailsFixture,
		"detail":      tangptDetailFixture,
		"detail_hr":   tangptDetailWithHRFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
