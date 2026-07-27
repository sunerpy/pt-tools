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
		SiteID:   "keepfrds",
		Search:   testKeepFRDSSearch,
		Detail:   testKeepFRDSDetail,
		UserInfo: testKeepFRDSUserInfo,
	})
}

// --- Fixtures ---
//
// 以下片段按 pt.keepfrds.com 真实页面结构手写并脱敏（用户名、用户 ID、种子 ID、头像哈希均为虚构）。
// 关键结构特征：
//   - 副标题是 <br /> 之后的裸文本节点，没有包裹元素（部分行被 <span style="color: Red;"> 包裹）
//   - 大小单元格中数字与单位被 <br /> 分隔：8.47<br />GiB
//   - 促销结束时间只存在于促销图标的 onmouseover 提示中
const keepfrdsSearchFixture = `<html><body>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%">
<tr><td class="colhead" style="padding: 0px">类型</td><td class="colhead">标题</td><td class="colhead">评论</td><td class="colhead">存活</td><td class="colhead">大小</td><td class="colhead">做种</td><td class="colhead">下载</td><td class="colhead">完成</td><td class="colhead">发布者</td></tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style="padding: 0px"><a href="?cat=401"><img class="c_movies" src="/static/pic/cattrans.gif" alt="电影" title="电影" /></a></td>
<td class="rowfollow" width="100%" align="left">
	<table class="torrentname" width="100%" cellspacing="0" cellpadding="0"><tr>
		<td class="embedded browse_td_name_cell">
			<div class="browse_imdb_poster" style="display: none;"><img loading="lazy" src="https://static.example.test/poster/w185/111111.webp" alt=""></div>
			<a title="【示例电影 / Sample Movie】10bit HEVC版本 简繁英双语字幕" href="details.php?id=1000001&amp;hit=1"><b>【示例电影 / Sample Movie】10bit HEVC版本 简繁英双语字幕</b></a> <img class='tag imdb_111111'></img><br />Sample.Movie.2026.BluRay.1080p.x265.10bit.DDP5.1-DEMO<b>[<div class='famfamfam-silk cup' style='display:inline-block;' title='热门'></div>]</b> <b>[<font class='recommended'>限时禁转</font>]</b>
		</td>
		<td width="110" class="embedded" style="text-align: right;" valign="top">
			<div><a href="download.php?id=1000001"><img class="download my-0" src="/static/pic/download.png" alt="download" title="下载本种" /></a></div>
			<div><img class="pro_free" src="/static/pic/freeicon.gif" alt="Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-07-15 14:03:04&quot;&gt;4时6分&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000,'maxWidth', 300);" /></div>
		</td>
	</tr></table>
</td>
<td class="rowfollow"><a href="comment.php?action=add&amp;pid=1000001&amp;type=torrent" title="添加评论">0</a></td>
<td class="rowfollow nowrap"><span title="2026-07-14 14:03:04">19时<br />53分</span></td>
<td class="rowfollow">8.47<br />GiB</td>
<td class="rowfollow" align="center"><b><a href="details.php?id=1000001&amp;hit=1&amp;dllist=1#seeders">334</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=1000001&amp;hit=1&amp;dllist=1#leechers">6</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=1000001"><b>494</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style="padding: 0px"><a href="?cat=402"><img class="c_tvplay" src="/static/pic/cattrans.gif" alt="剧集" title="剧集" /></a></td>
<td class="rowfollow" width="100%" align="left">
	<table class="torrentname" width="100%" cellspacing="0" cellpadding="0"><tr>
		<td class="embedded browse_td_name_cell">
			<a title="【示例剧集 第一季 / Sample Show S01】中英字幕" href="details.php?id=1000002&amp;hit=1"><b><img class="sticky" src="/static/pic/up.png" alt="Sticky" title="置顶" />&nbsp;【示例剧集 第一季 / Sample Show S01】中英字幕</b></a> <img class='tag imdb_222222'></img><br /><span style="color: Red;">Sample.Show.S01.2026.2160p.WEBRip.x265.HEVC-DEMO<b>[<div class='famfamfam-silk information' style='display:inline-block;' title='包含媒体信息'></div>]</b></span>
		</td>
		<td width="110" class="embedded" style="text-align: right;" valign="top">
			<div><a href="download.php?id=1000002"><img class="download my-0" src="/static/pic/download.png" alt="download" title="下载本种" /></a></div>
			<div><img class="pro_50pctdown" src="/static/pic/50pctdown.gif" alt="50%" /></div>
		</td>
	</tr></table>
</td>
<td class="rowfollow"><a href="comment.php?action=add&amp;pid=1000002&amp;type=torrent" title="添加评论">3</a></td>
<td class="rowfollow nowrap"><span title="2026-07-10 08:12:00">5天<br />1时</span></td>
<td class="rowfollow">36.5<br />GiB</td>
<td class="rowfollow" align="center"><b><a href="details.php?id=1000002&amp;hit=1&amp;dllist=1#seeders">68</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=1000002&amp;hit=1&amp;dllist=1#leechers">1</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=1000002"><b>744</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style="padding: 0px"><a href="?cat=401"><img class="c_movies" src="/static/pic/cattrans.gif" alt="电影" title="电影" /></a></td>
<td class="rowfollow" width="100%" align="left">
	<table class="torrentname" width="100%" cellspacing="0" cellpadding="0"><tr>
		<td class="embedded browse_td_name_cell">
			<a title="【普通示例片 / Plain Sample】英语" href="details.php?id=1000003&amp;hit=1"><b>【普通示例片 / Plain Sample】英语</b></a><br />Plain.Sample&#39;s.1999.720p.BluRay.x264-DEMO
		</td>
		<td width="110" class="embedded" style="text-align: right;" valign="top">
			<div><a href="download.php?id=1000003"><img class="download my-0" src="/static/pic/download.png" alt="download" title="下载本种" /></a></div>
		</td>
	</tr></table>
</td>
<td class="rowfollow"><a href="comment.php?action=add&amp;pid=1000003&amp;type=torrent" title="添加评论">0</a></td>
<td class="rowfollow nowrap"><span title="2026-06-01 20:00:00">1月<br />14天</span></td>
<td class="rowfollow">4.37<br />GiB</td>
<td class="rowfollow" align="center"><b><a href="details.php?id=1000003&amp;hit=1&amp;dllist=1#seeders">12</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=1000003&amp;hit=1&amp;dllist=1#leechers">0</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=1000003"><b>88</b></a></td>
<td class="rowfollow"><i>匿名</i></td>
</tr>
</table>
</body></html>`

// 详情页无 input[name='torrent_name'] / input[name='detail_torrent_id']，
// 标题在 h1#top 文本中并带尾部 [免费] 促销标记；大小单位为 GiB，标签使用全角冒号。
const keepfrdsDetailFixture = `<html><body>
<h1 align="center" id="top">【示例电影 / Sample Movie】10bit HEVC版本 国英双语 简繁英双语字幕 带章节名&nbsp;&nbsp;&nbsp; <b>[<font class='free' >免费</font>]</b></h1>
<table id="tdetail" class="outerMost" cellspacing="0" cellpadding="5">
<tr><td class="rowhead">下载</td><td class="rowfollow" align="left"><a class="index" href="download.php?id=1000001">[DEMO].Sample.Movie.2026.BluRay.1080p.x265.10bit.DDP5.1-DEMO.torrent</a>&nbsp;&nbsp;&nbsp;由&nbsp;<i>匿名</i>发布于<span title="2026-07-14 14:03:04">19时53分前</span></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">Sample Movie 2026 BluRay 1080p x265 10bit DDP5.1 DEMO</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>8.47 GiB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;电影&nbsp;&nbsp;&nbsp;<b>制作组:&nbsp;</b>DEMO</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">行为</td><td class="rowfollow" valign="top" align="left"><a title="下载种子" href="download.php?id=1000001"><img class="dt_download" src="/static/pic/trans.gif" alt="download" /></a></td></tr>
</table>
<form id="compose" name="comment" method="post" action="comment.php?action=add&amp;type=torrent"><input type="hidden" name="pid" value="1000001" /></form>
</body></html>`

// 无促销的详情页：标题尾部是合法的规格/制作组方括号，必须保留而不能被促销剥离逻辑吃掉。
const keepfrdsDetailPlainFixture = `<html><body>
<h1 align="center" id="top">【示例剧集 第一季 / Sample Show S01】中英字幕  [1080p BluRay]</h1>
<table id="tdetail" class="outerMost" cellspacing="0" cellpadding="5">
<tr><td class="rowhead">下载</td><td class="rowfollow" align="left"><a class="index" href="download.php?id=1000002">[DEMO].Sample.Show.S01.torrent</a></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>512.5 MiB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;剧集</td></tr>
</table>
<form id="compose" name="comment" method="post" action="comment.php?action=add&amp;type=torrent"><input type="hidden" name="pid" value="1000002" /></form>
</body></html>`

// #info_block：魔力值在 span#totalBonus，做种/下载数使用 Font Awesome <i> 图标而非 arrowup/arrowdown 图片。
const keepfrdsIndexFixture = `<html><body>
<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%">
<tr><td class="bottom" align="left"><span class="medium">
	欢迎回来,
	<span class="nowrap"><a href="/userdetails.php?id=20001" class='VeteranUser_Name'><b>frds_demo_user</b></a> (<b class='VeteranUser_Name'>Veteran User</b>)</span>
	<span class='color_bonus'>魔力值 </span>
	[<a href="/mybonus.php">使用</a>]: <span id="totalBonus">1,503,781.4</span> <i class="fab fa-btc"></i> [<span id="perBonus">54.7</span> <i class="fab fa-btc"></i>/h]
	<br />
	<span class="color_ratio">分享率：</span>2.893
	<span class='color_uploaded'>上传量：</span><span class="uploaded">6.714 TiB</span>
	<span class='color_downloaded'>下载量：</span><span class="downloaded">2.321 TiB</span>
	<span> | </span>
	<a href="/torrents.php?option-torrents=3"><i class="fas fa-seedling beta-symbol text-green-500" alt="Torrents seeding" title="当前做种"></i> 85</a>
	<a href="/torrents.php?option-torrents=5"><i class="fas fa-chevron-down beta-symbol text-red-500" alt="Torrents leeching" title="当前下载"></i> 0</a>
</span></td></tr>
</table>
</body></html>`

// userdetails.php：加入日期 / 最近动向 各自带专用 class，传输行是内嵌 table 的合并单元格。
const keepfrdsUserdetailsFixture = `<html><body>
<table width="100%">
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">加入日期</td><td width="99%" class="rowfollow join_date" valign="top" align="left">2019-04-01 03:23:03 (<span title="2019-04-01 03:23:03">7年4月前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow last_seen" valign="top" align="left">2026-07-15 09:56:41 (<span title="2026-07-15 09:56:41">&lt; 1分前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">等级</td><td width="99%" class="rowfollow " valign="top" align="left"><img alt="Veteran User" title="Veteran User" src="/static/pic/veteran.gif" />  <br>下一等级: Extreme User<br>需要:
	<span style="color: green"> 账号时间 90周</span>
	<span style="color: green"> 下载 2048GB </span>
	<span> 分享 4.0 </span>
	<span style="color: green"> 魔力 1280000</span>
	<span> 做种率 640</span>
	</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">传输</td><td width="99%" class="rowfollow " valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>:  <font color="">2.893</font></td><td class="embedded">&nbsp;&nbsp;<img src="/static/pic/smilies/3.gif" alt="" /></td></tr><tr><td class="embedded"><strong>上传量</strong>:  6.714 TiB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>:  2.321 TiB</td></tr><tr><td class="embedded"><strong>实际上传量</strong>:  1.993 TiB</td><td class="embedded">&nbsp;&nbsp;<strong>实际下载量</strong>:  3.567 TiB</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">魔力值</td><td width="99%" class="rowfollow " valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>做种率（魔力值/下载量）</strong>: 633</td></tr><tr><td class="embedded"><strong>魔力值</strong>:  1,503,781.41 <i class='fab fa-btc'></i>&nbsp;&nbsp;&nbsp;&nbsp;[54.7 <i class='fab fa-btc'></i>/h]</td></tr></table></td></tr>
</table>
</body></html>`

// --- Helpers ---

func getKeepFRDSDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("keepfrds")
	require.True(t, ok, "keepfrds definition not found")
	return def
}

// --- Suite: Search ---

func testKeepFRDSSearch(t *testing.T) {
	def := getKeepFRDSDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(keepfrdsSearchFixture))
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
	require.Len(t, items, 3, "should parse 3 torrent rows")

	free := items[0]
	assert.Equal(t, "1000001", free.ID)
	assert.Equal(t, "【示例电影 / Sample Movie】10bit HEVC版本 简繁英双语字幕", free.Title)
	assert.Equal(t, "Sample.Movie.2026.BluRay.1080p.x265.10bit.DDP5.1-DEMO", free.Subtitle,
		"subtitle is a bare text node after <br />")
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	// 促销结束时间只在促销图标的 onmouseover 提示里，靠驱动的 onmouseover 兜底解析
	require.False(t, free.DiscountEndTime.IsZero(), "discount end time should come from onmouseover tooltip")
	assert.Equal(t, 2026, free.DiscountEndTime.Year())
	assert.Equal(t, 7, int(free.DiscountEndTime.Month()))
	assert.Equal(t, 15, free.DiscountEndTime.Day())
	assert.Equal(t, 334, free.Seeders)
	assert.Equal(t, 6, free.Leechers)
	assert.Equal(t, 494, free.Snatched)
	// 大小单元格中数字与单位被 <br /> 分隔（8.47<br />GiB），仍需得到正确字节数
	// 8.47 * 1024^3 = 9094593249
	assert.Equal(t, int64(9094593249), free.SizeBytes)
	assert.Equal(t, "电影", free.Category)
	assert.False(t, free.UploadedAt == 0, "upload time should be parsed from span[title]")

	half := items[1]
	assert.Equal(t, "1000002", half.ID)
	assert.Equal(t, "【示例剧集 第一季 / Sample Show S01】中英字幕", half.Title,
		"sticky icon leading nbsp should be trimmed")
	assert.Equal(t, "Sample.Show.S01.2026.2160p.WEBRip.x265.HEVC-DEMO", half.Subtitle,
		"subtitle wrapped in <span style=color:Red> should still be extracted")
	assert.Equal(t, v2.DiscountPercent50, half.DiscountLevel)
	assert.True(t, half.DiscountEndTime.IsZero(), "50% icon has no tooltip in this fixture")

	plain := items[2]
	assert.Equal(t, "1000003", plain.ID)
	assert.Equal(t, "Plain.Sample's.1999.720p.BluRay.x264-DEMO", plain.Subtitle,
		"HTML entities in the bare text node must be decoded")
	assert.Equal(t, v2.DiscountNone, plain.DiscountLevel)
	assert.Equal(t, int64(4692251770), plain.SizeBytes)
}

// --- Suite: Detail ---

func testKeepFRDSDetail(t *testing.T) {
	def := getKeepFRDSDef(t)

	t.Run("Free", func(t *testing.T) {
		doc := FixtureDoc(t, "keepfrds_detail", keepfrdsDetailFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "1000001", info.TorrentID)
		// 标题来自 h1#top 文本，尾部 [免费] 促销标记应被剥离
		assert.Equal(t, "【示例电影 / Sample Movie】10bit HEVC版本 国英双语 简繁英双语字幕 带章节名", info.Title)
		assert.NotEmpty(t, info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		// 8.47 GiB → 8673.28 MB（二进制单位必须被识别，否则会得到 0 或 8.47）
		assert.InDelta(t, 8.47*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR, "site has no H&R marker")
	})

	t.Run("PlainTitleKeepsBracket", func(t *testing.T) {
		doc := FixtureDoc(t, "keepfrds_detail_plain", keepfrdsDetailPlainFixture)
		parser := v2.NewNexusPHPParserFromDefinition(def)
		info := parser.ParseAll(doc.Selection)

		assert.Equal(t, "1000002", info.TorrentID)
		// 尾部方括号是规格信息而非促销标记，不能被剥离
		assert.Equal(t, "【示例剧集 第一季 / Sample Show S01】中英字幕  [1080p BluRay]", info.Title)
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel)
		assert.InDelta(t, 512.5, info.SizeMB, 0.001)
		assert.False(t, info.HasHR)
	})
}

// --- Suite: UserInfo ---

func testKeepFRDSUserInfo(t *testing.T) {
	def := getKeepFRDSDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "keepfrds_index", keepfrdsIndexFixture)
		fields := map[string]string{
			"id":           "20001",
			"name":         "frds_demo_user",
			"bonus":        "1.5037814e+06",
			"bonusPerHour": "54.7",
			"seeding":      "85",
			"leeching":     "0",
		}
		for field, expected := range fields {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q not found", field)
				assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel))
			})
		}
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "keepfrds_userdetails", keepfrdsUserdetailsFixture)
		fields := map[string]string{
			// 传输行为合并单元格，且单位是二进制 TiB（6.714 * 1024^4 = 7382121068888）
			"uploaded":       "7382121068888",
			"downloaded":     "2551966488068",
			"ratio":          "2.893",
			"trueUploaded":   "2191326674157",
			"trueDownloaded": "3921957976276",
			"levelName":      "Veteran User",
			// 2019-04-01 03:23:03 +0800
			"joinTime": "1554060183",
		}
		for field, expected := range fields {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q not found", field)
				assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel))
			})
		}

		// 保号探测只读取 LastAccess，必须非零
		t.Run("lastAccessAt", func(t *testing.T) {
			sel, ok := def.UserInfo.Selectors["lastAccessAt"]
			require.True(t, ok)
			got := driver.ExtractFieldValuePublic(doc, sel)
			assert.Equal(t, "1784080601", got, "2026-07-15 09:56:41 +0800")
			assert.NotEqual(t, "0", got, "probe (保号) reads only UserInfo.LastAccess")
		})
	})
}

// --- Standalone Tests ---

func TestKeepfrds_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":       keepfrdsSearchFixture,
		"detail":       keepfrdsDetailFixture,
		"detail_plain": keepfrdsDetailPlainFixture,
		"index":        keepfrdsIndexFixture,
		"userdetails":  keepfrdsUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
