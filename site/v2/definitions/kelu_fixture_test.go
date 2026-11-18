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
	RegisterFixtureSuite(FixtureSuite{
		SiteID:   "kelu",
		Search:   testKeluSearch,
		Detail:   testKeluDetail,
		UserInfo: testKeluUserInfo,
	})
}

// --- Fixtures ---
//
// 结构照搬 issue #861 附带的真实页面（CHD/scenetorrents 分类图标皮肤）。站点内容不适合放进仓库，
// 标题、副标题、分类名、种子 ID、用户名、用户 ID、勋章与海报地址、统计数值全部换成了中性的虚构值。
//
// 搜索页覆盖的真实特征：
//   - 名称子表第一个 td.embedded 是 46x46 海报缩略图，名称单元格排在它后面
//   - 抓取到的促销都是长期促销（pro_free / pro_50pctdown / pro_50pctdown2up），图标只有 title，没有剩余时间
//   - 三种名称单元格：<br /> 后先有标签再有副标题；<br /> 后直接是副标题；根本没有 <br />（没有副标题）
//   - 分类图标的 <img> 没有 class，发布者是 <span class="nowrap"><i>匿名</i></span>

const keluSearchFixture = `<html><body>
<table class="torrents" cellspacing="0" cellpadding="5" width="100%">
<tr>
<td class="colhead" style="padding: 0px">类型</td>
<td class="colhead"><a href="?sort=1&amp;type=asc">标题</a></td>
<td class="colhead"><a href="?sort=3&amp;type=desc"><img class="comments" src="pic/trans.gif" alt="comments" title="评论数" /></a></td>
<td class="colhead"><a href="?sort=4&amp;type=desc"><img class="time" src="pic/trans.gif" alt="time" title="存活时间" /></a></td>
<td class="colhead"><a href="?sort=5&amp;type=desc"><img class="size" src="pic/trans.gif" alt="size" title="大小" /></a></td>
<td class="colhead"><a href="?sort=7&amp;type=desc"><img class="seeders" src="pic/trans.gif" alt="seeders" title="种子数" /></a></td>
<td class="colhead"><a href="?sort=8&amp;type=desc"><img class="leechers" src="pic/trans.gif" alt="leechers" title="下载数" /></a></td>
<td class="colhead"><a href="?sort=6&amp;type=desc"><img class="snatched" src="pic/trans.gif" alt="snatched" title="完成数" /></a></td>
<td class="colhead"><a href="?sort=9&amp;type=desc">发布者</a></td>
</tr>
<tr style="background-color: #89c9e6">
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=405"><img src="pic/cattrans.gif" alt="合集" title="合集" style="background-image: url(pic/category/chd/scenetorrents/fixture-a.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr style="background-color: #89c9e6"><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="pic/attachments/fixture-poster-1.jpg" class="nexus-lazy-load" style="max-height: 46px;max-width: 46px" /></td><td class="embedded" style='padding-left: 5px'><img class="sticky" src="pic/trans.gif" alt="Sticky" title="一级置顶" />&nbsp;<img class="sticky" src="pic/trans.gif" alt="Sticky" title="一级置顶" />&nbsp;<a title="Fixture.Collection.Vol.01-FIXTURE" href="details.php?id=99501&amp;hit=1"><b>Fixture.Collection.Vol.01-FIXTURE</b></a> <b>[<font class='hot'>热门</font>]</b> <img class="pro_50pctdown" src="pic/trans.gif" alt="50%" title="50%" /><br /><span style="background-color:#460046;color:#FFFFFF;border-radius:0;font-size:12px;margin:0 4px 0 0;padding:1px 2px" title="">合集</span>虚构合集 第一辑</td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=99501"><img class="download" src="pic/trans.gif" style='padding-bottom: 2px;' alt="download" title="下载本种" /></a><br /><a id="bookmark0" href="javascript: bookmark(99501,0);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=99501&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2024-03-05 06:07:08">2年<br />4月</span></td><td class="rowfollow">269.82<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99501&amp;hit=1&amp;dllist=1#seeders">42</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><a href="viewsnatches.php?id=99501"><b>130</b></a></td>
<td class="rowfollow"><span class="nowrap"><i>匿名</i></span></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=406"><img src="pic/cattrans.gif" alt="短片" title="短片" style="background-image: url(pic/category/chd/scenetorrents/fixture-b.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="pic/attachments/fixture-poster-2.jpg" class="nexus-lazy-load" style="max-height: 46px;max-width: 46px" /></td><td class="embedded" style='padding-left: 5px'><a title="Fixture.Short.Film.No.02-FIXTURE" href="details.php?id=99502&amp;hit=1"><b>Fixture.Short.Film.No.02-FIXTURE</b></a> <b>[<font class='hot'>热门</font>]</b> <img class="pro_free" src="pic/trans.gif" alt="Free" title="免费" /></td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=99502"><img class="download" src="pic/trans.gif" style='padding-bottom: 2px;' alt="download" title="下载本种" /></a><br /><a id="bookmark1" href="javascript: bookmark(99502,1);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><b><a href="details.php?id=99502&amp;hit=1&amp;cmtpage=1#startcomments" >1</a></b></td><td class="rowfollow nowrap"><span title="2025-06-07 10:20:30">1年<br />1月</span></td><td class="rowfollow">4.41<br />GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99502&amp;hit=1&amp;dllist=1#seeders">18</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=99502&amp;hit=1&amp;dllist=1#leechers">1</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=99502"><b>77</b></a></td>
<td class="rowfollow"><span class="nowrap"><i>匿名</i></span></td>
</tr>
<tr>
<td class="rowfollow nowrap" valign="middle" style='padding: 0px'><a href="?cat=406"><img src="pic/cattrans.gif" alt="短片" title="短片" style="background-image: url(pic/category/chd/scenetorrents/fixture-b.png);" /></a></td>
<td class="rowfollow" width="100%" align="left" style='padding: 0px'><table class="torrentname" width="100%"><tr><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/misc/spinner.svg" data-src="pic/attachments/fixture-poster-3.jpg" class="nexus-lazy-load" style="max-height: 46px;max-width: 46px" /></td><td class="embedded" style='padding-left: 5px'><a title="Fixture.Short.Film.No.03-FIXTURE" href="details.php?id=99503&amp;hit=1"><b>Fixture.Short.Film.No.03-FIXTURE</b></a> <img class="pro_50pctdown2up" src="pic/trans.gif" alt="2X 50%" title="2X 50%" /><br />虚构短片 第三辑</td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=99503"><img class="download" src="pic/trans.gif" style='padding-bottom: 2px;' alt="download" title="下载本种" /></a><br /><a id="bookmark2" href="javascript: bookmark(99503,2);" ><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=99503&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-07-01 21:00:00">3天<br />2时</span></td><td class="rowfollow">272.99<br />MB</td><td class="rowfollow" align="center"><b><a href="details.php?id=99503&amp;hit=1&amp;dllist=1#seeders">6</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><a href="viewsnatches.php?id=99503"><b>12</b></a></td>
<td class="rowfollow"><span class="nowrap"><i>匿名</i></span></td>
</tr>
</table>
</body></html>`

// keluInfoBlock 是每个页面顶部的 #info_block。页头有 H&R 计数器「H&R: [0/0/10]」，
// 它不是种子的 H&R 标记，详情页不能因此判成 H&R 种子。
const keluInfoBlock = `<table id="info_block" cellpadding="4" cellspacing="0" border="0" width="100%"><tr>
	<td><table width="100%" cellspacing="0" cellpadding="0" border="0"><tr>
		<td class="bottom" align="left">
            <span class="medium">
                欢迎回来, <span class="nowrap"><a href="https://our.kelu.one/userdetails.php?id=90001" class='User_Name'><b>fixture_user</b></a><img src="pic/medals/fixture-medal.png" title="虚构勋章" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"/></span>                [<a href="logout.php">退出</a>]
                [<a href="usercp.php">控制面板</a>]
                [<a href="torrents.php?inclbookmarked=1&amp;allsec=1&amp;incldead=0">收藏</a>]
                <font class = 'color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 1,234,567.8                 <a href="attendance.php" class="">[签到已得10, 补签卡: 2]</a>                <a href="medal.php">[勋章]</a>
                <a href="task.php">[任务]</a>
                <font class = 'color_invite'>邀请 </font>[<a href="invite.php?id=90001">发送</a>]: 0(0)                                <br />
	            <font class="color_ratio">分享率:</font> 1,234.567                <font class='color_uploaded'>上传量:</font> 3.793 TB                <font class='color_downloaded'> 下载量:</font> 3.15 GB                <font class='color_active'>当前活动:</font> <img class="arrowup" alt="Torrents seeding" title="当前做种" src="pic/trans.gif" />64  <img class="arrowdown" alt="Torrents leeching" title="当前下载" src="pic/trans.gif" />0&nbsp;&nbsp;
                <font class='color_connectable'>可连接:</font><b><font color="green">是</font></b>                <font class='color_slots'>连接数：</font>无限制                <font class='color_bonus'>H&R: </font> [<a href="myhr.php">0/<font color="red">0</font>/10</a>]                <font class='color_bonus'>认领: </font> [<a href="claim.php?uid=90001">0/1000</a>]            </span>
        </td>
	</tr></table></td>
</tr></table>`

// 详情页（长期免费）：h1 只有 <font class='free' >免费</font>，没有剩余时间。
const keluDetailFixture = `<html><body>` + keluInfoBlock + `
<h1 align="center" id="top">Fixture.Collection.Vol.01-FIXTURE&nbsp;&nbsp;&nbsp; <b>[<font class='free' >免费</font>]</b></h1>
<table class="main" width="100%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead" width="13%">下载</td><td class="rowfollow" width="87%" align="left"><a class="index" href="download.php?id=99501">[Kelu].Fixture.Collection.Vol.01-FIXTURE.torrent</a>&nbsp;&nbsp;&nbsp;由&nbsp;<i>匿名</i>发布于<span title="2024-03-05 06:07:08">2年4月前</span></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">虚构合集 第一辑</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>269.82 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;合集</td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">认领种子</td><td class="rowfollow" valign="top" align="left"><input type="button" value="认领种子" /></td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.Collection.Vol.01-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="99501" /></form></td></tr>
</table>
</body></html>`

// 详情页（长期 50%）：h1 只有 <font class='halfdown' >50%</font>，大小是 MB。
const keluDetailHalfDownFixture = `<html><body>` + keluInfoBlock + `
<h1 align="center" id="top">Fixture.Short.Film.No.03-FIXTURE&nbsp;&nbsp;&nbsp; <b>[<font class='halfdown' >50%</font>]</b></h1>
<table class="main" width="100%" cellspacing="0" cellpadding="5">
<tr><td class="rowhead" width="13%">下载</td><td class="rowfollow" width="87%" align="left"><a class="index" href="download.php?id=99504">[Kelu].Fixture.Short.Film.No.04-FIXTURE.torrent</a>&nbsp;&nbsp;&nbsp;由&nbsp;<i>匿名</i>发布于<span title="2026-07-01 21:00:00">3天前</span></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>272.99 MB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;短片</td></tr>
<tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.Short.Film.No.04-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="99504" /></form></td></tr>
</table>
</body></html>`

// index.php：id / name / 做种下载数取自 #info_block，用户链接是绝对地址，用户名后面有勋章图片。
const keluIndexFixture = `<html><body>` + keluInfoBlock + `</body></html>`

// userdetails.php：分享率可以是四位数带千分位逗号（「1,234.567」），还有一行「H&R」。
const keluUserdetailsFixture = `<html><body>
<table class="main" width="100%" border="1" cellspacing="0" cellpadding="5">
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">用户ID/UID</td><td width="99%" class="rowfollow" valign="top" align="left">90001</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">加入日期</td><td width="99%" class="rowfollow" valign="top" align="left">2024-03-05 06:07:08 (<span title="2024-03-05 06:07:08">2年4月前</span>, 121.3周)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-07-05 07:08:09 (<span title="2026-07-05 07:08:09">&lt; 1分钟前</span>)</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">传输</td><td width="99%" class="rowfollow" valign="top" align="left"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>: <font color="">1,234.567</font>（<strong>实际分享率</strong>：70.386）</td><td class="embedded">&nbsp;&nbsp;<img src="pic/smilies/163.gif" alt="" /></td></tr><tr><td class="embedded"><strong>上传量</strong>: 3.793 TB</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>: 3.15 GB</td></tr><tr><td class="embedded"><strong>实际上传量</strong>: 1.948 TB</td><td class="embedded">&nbsp;&nbsp;<strong>实际下载量</strong>: 28.34 GB</td><td class="embedded text-muted">&nbsp;&nbsp;实际上传/下载量 (仅用于记录, 不参与分享率计算)</td></tr></table></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">等级</td><td width="99%" class="rowfollow" valign="top" align="left"><img alt="User" title="User" src="pic/user.gif" /> </td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">H&amp;R</td><td width="99%" class="rowfollow" valign="top" align="left"><a href="myhr.php?uid=90001">0/0/10</a></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">魔力值</td><td width="99%" class="rowfollow" valign="top" align="left">1,234,567.8&nbsp;&nbsp;<a href="bonus-log.php?uid=90001" target="_blank" class="altlink">[明细]</a></td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">做种积分</td><td width="99%" class="rowfollow" valign="top" align="left">345,678.9&nbsp;&nbsp;<span class='text-muted'>(更新时间: 2026-07-05 06:00:00)</span></td></tr>
</table>
</body></html>`

// --- Helpers ---

func getKeluDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("kelu")
	require.True(t, ok, "kelu definition not found")
	return def
}

// --- Suite: Search ---

func testKeluSearch(t *testing.T) {
	def := getKeluDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(keluSearchFixture))
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

	halfDown := items[0]
	assert.Equal(t, "99501", halfDown.ID)
	assert.Equal(t, "Fixture.Collection.Vol.01-FIXTURE", halfDown.Title)
	assert.Equal(t, "虚构合集 第一辑", halfDown.Subtitle)
	assert.Equal(t, v2.DiscountPercent50, halfDown.DiscountLevel)
	// 长期促销的图标没有 onmouseover，也没有剩余时间
	assert.True(t, halfDown.DiscountEndTime.IsZero(), "long-term promotion has no end time")
	assert.Equal(t, int64(289717018951), halfDown.SizeBytes)
	assert.Equal(t, 42, halfDown.Seeders)
	assert.Equal(t, 0, halfDown.Leechers)
	assert.Equal(t, 130, halfDown.Snatched)
	assert.Equal(t, "合集", halfDown.Category)
	assert.Positive(t, halfDown.UploadedAt)
	assert.False(t, halfDown.HasHR)

	free := items[1]
	assert.Equal(t, "99502", free.ID)
	assert.Equal(t, "Fixture.Short.Film.No.02-FIXTURE", free.Title)
	// 名称单元格里没有 <br />，副标题为空
	assert.Empty(t, free.Subtitle)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	assert.True(t, free.DiscountEndTime.IsZero())
	assert.Equal(t, int64(4735201443), free.SizeBytes)
	assert.Equal(t, 18, free.Seeders)
	assert.Equal(t, 1, free.Leechers)
	assert.Equal(t, 77, free.Snatched)

	twoUpHalf := items[2]
	assert.Equal(t, "99503", twoUpHalf.ID)
	// 没有标签时副标题紧跟在 <br /> 后面
	assert.Equal(t, "虚构短片 第三辑", twoUpHalf.Subtitle)
	assert.Equal(t, v2.Discount2x50, twoUpHalf.DiscountLevel)
	assert.True(t, twoUpHalf.DiscountEndTime.IsZero())
	assert.Equal(t, int64(286250762), twoUpHalf.SizeBytes)
	assert.Equal(t, 6, twoUpHalf.Seeders)
	assert.Equal(t, 12, twoUpHalf.Snatched)
	assert.Equal(t, "短片", twoUpHalf.Category)
}

// --- Suite: Detail ---

func testKeluDetail(t *testing.T) {
	def := getKeluDef(t)

	t.Run("LongTermFree", func(t *testing.T) {
		doc := FixtureDoc(t, "kelu_detail", keluDetailFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "99501", info.TorrentID)
		assert.Equal(t, "Fixture.Collection.Vol.01-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		assert.True(t, info.DiscountEnd.IsZero(), "long-term promotion has no end time")
		assert.Empty(t, info.DiscountErr)
		assert.InDelta(t, 269.82*1024, info.SizeMB, 0.1)
		// 页头「H&R: [0/0/10]」计数器不是 H&R 标记
		assert.False(t, info.HasHR, "header H&R counter must not mark the torrent as H&R")

		assert.Equal(t, "download.php?id=99501",
			doc.Find(def.Selectors.DetailDownloadLink).First().AttrOr("href", ""))
		assert.Equal(t, "虚构合集 第一辑",
			strings.TrimSpace(doc.Find(def.Selectors.DetailSubtitle).First().Text()))
	})

	t.Run("LongTermHalfDown", func(t *testing.T) {
		doc := FixtureDoc(t, "kelu_detail_halfdown", keluDetailHalfDownFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "99504", info.TorrentID)
		assert.Equal(t, "Fixture.Short.Film.No.04-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountPercent50, info.DiscountLevel)
		assert.True(t, info.DiscountEnd.IsZero())
		assert.Empty(t, info.DiscountErr)
		assert.InDelta(t, 272.99, info.SizeMB, 0.01)
		assert.False(t, info.HasHR)
	})
}

// --- Suite: UserInfo ---

func testKeluUserInfo(t *testing.T) {
	def := getKeluDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("IndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "kelu_index", keluIndexFixture)
		fields := map[string]string{
			"id":       "90001",
			"name":     "fixture_user",
			"seeding":  "64",
			"leeching": "0",
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
		doc := FixtureDoc(t, "kelu_userdetails", keluUserdetailsFixture)
		fields := map[string]string{
			// 3.793 TB / 3.15 GB / 1.948 TB / 28.34 GB 按 1024 进制换算为字节
			"uploaded":       "4170447604154",
			"downloaded":     "3382286745",
			"trueUploaded":   "2141848650907",
			"trueDownloaded": "30429843292",
			// 正则保留千分位逗号，parseRatio 会去掉它
			"ratio":     "1,234.567",
			"levelName": "User",
			// parseNumber 对百万以上的数输出科学计数法，与其他站点的测试一致
			"bonus":        "1.2345678e+06",
			"seedingBonus": "345678.9",
			"joinTime":     "1709590028",
			"lastAccessAt": "1783206489",
		}
		for field, expected := range fields {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q not found", field)
				assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel))
			})
		}
	})
}

// --- Standalone Tests ---

func TestKelu_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":          keluSearchFixture,
		"detail":          keluDetailFixture,
		"detail_halfdown": keluDetailHalfDownFixture,
		"index":           keluIndexFixture,
		"userdetails":     keluUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
