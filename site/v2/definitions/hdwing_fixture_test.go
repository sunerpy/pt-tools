package definitions

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/sitelogin"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func init() {
	RegisterFixtureSuite(FixtureSuite{
		SiteID:   "hdwing",
		Search:   testHDWinGSearch,
		Detail:   testHDWinGDetail,
		UserInfo: testHDWinGUserInfo,
	})
}

// 下面的页面按 issue #910 报告人 2026-11-18 采集的 HDWinG 页面结构手写：用户名、用户 ID、令牌、签名下载链接、IP、
// 统计数都换成了示例值，标题换成了示例标题。列表与详情页写的是站点直接回的样子（优惠位置只有 sp_state_placeholder
// 占位，首页样本就是这样）；采到的列表与详情页是页面脚本填过优惠状态以后的，填进去的内容就是下面 ajax_promotion.php
// 返回里的 sp_state 与 timeout（返回的外层结构按页面脚本还原：status 为 200，message 按种子 ID 列）。

const hdwingCSRF = "demo0csrf0token0for0fixture"

// hdwingUserBar 是每一页顶部的用户信息栏（div.userinfo），首页样本里「下载量：」后面有一个多余的 </font>
const hdwingUserBar = `<div class="headerbox"><div id="site_header">
<div class="site_logo" title="Welcome to HDChina.org ^_^"></div>
<div class="userinfo">
<p class="donate"><a href="donate.php" title="Donate 捐赠"><span class="loadbarbg"><b class="colorbar loadbarred" style="width:0px;" title="捐赠完成进度 Donate Progress：0%"></b><i class="percent">0%</i></span></a><i style='color: goldenrod; margin-left: 5px;vertical-align: 3px;' class="fas fa-star"></i></p>
<p>欢迎回来                    ，<span class="nowrap"><a  href="userdetails.php?id=10001" class='PowerUser_Name'  style='' ><b>demo</b></a></span>
<!-- [<a href="usercp.php">控制面板</a>] -->　[<a href="torrents.php?inclbookmarked=1&amp;allsec=1&amp;incldead=0">收藏</a>]　[<a href="torrents.php?rsscart=1&allsec=1&incldead=0">下载筐</a>]　魔力值                     [<a href="mybonus.php">使用</a>]：12,345.6                                        　邀请 [<a href="invite.php?id=10001">发送</a>]：0                </p>
<p>
<!-- H&R: <b style="color:green">0</b>　-->分享率：2.500                    　上传量：1.25 TB                    　下载量：</font> 512.00 GB 客户端 <a href='takeflush.php'>[清理]</a>：1                    　(<i class="fas fa-arrow-up" style="color: forestgreen"
     title="当前做种"></i> 128　<i
        class="fas fa-arrow-down" style="color: red"
        title="当前下载"></i> 3                    )
　<font>流量排名: 1000 <i style="color: green" class="fas fa-arrow-up"></i>5                        　<!-- 连接数：无限制 --></p>
</div>
<div class="userinforb"><p>当前时间：15:04　<a href="getrss.php"><img class="rss" alt="RSS" title="获取RSS" src="pic/trans.gif" /></a></p>
<p><a href="messages.php"><img class="inbox" src="pic/trans.gif" alt="inbox" title="收件箱&nbsp;(无新短讯)" /></a> 0 (0 新)　[<a href='logout.php?csrf=demo0csrf0token0for0fixture'>退出</a>]</p></div>
<div class="userinfort"><p><a class="label label-default" href="donate.php">捐赠</a>
<a class="label label-default" href="#">已签到</a>                    <a class="label label-default" href="blackjack.php">21 点</a>
<a class="label label-default" href="bets.php">菠 菜</a></p></div>
</div></div>`

func hdwingPage(title, body string) string {
	return `<!DOCTYPE HTML>
<html><head><meta charset="utf-8"><meta name="generator" content="NexusPHP"/>
<meta name="x-csrf" content="` + hdwingCSRF + `"/>
<title>HDWinG :: ` + title + `</title></head>
<body><div id="all_box">` + hdwingUserBar + `<div id="site_content">` + body + `</div>
<div id="site_footer"><p> (c)  <a href="/" target="_self">HDWinG</a> 2013-2026 Powered by <a href="aboutnexus.php">NexusPHP</a> Version de93eed</p></div>
</div></body></html>`
}

// hdwingRow 是种子列表的一行：t_name 里套着 table.tbname（置顶图标、标题与副标题、优惠占位、分享规则、下载与收藏）。
func hdwingRow(class, cat, catName, pin, id, title, subtitle, progress, comment, timeTitle, timeLeft, size, seeders, leechers, completed, uploader string) string {
	hash := map[string]string{
		"200101": "MQoyMDAxMDEKMTAwMDE", "200102": "MQoyMDAxMDIKMTAwMDE", "200103": "MQoyMDAxMDMKMTAwMDE",
		"200104": "MQoyMDAxMDQKMTAwMDE", "200105": "MQoyMDAxMDUKMTAwMDE", "200106": "MQoyMDAxMDYKMTAwMDE",
	}[id]
	return `<tr class="` + class + `">
<td class="t_cat"><a href="?cat=` + cat + `"><img class="catchs type_` + cat + `" src="pic/cattrans.gif" alt="` + catName + `" title="` + catName + `" /></a></td>
<td class="t_name"><table class="tbname"><tr><td class="t_pin">` + pin + `</td><td><h3><a title="` + title + `" href="details.php?id=` + id + `&amp;hit=1">` + title + `</a></h3><h4>` + subtitle + `</h4>` + progress + `</td><td class="discount"><span class="sp_state_placeholder" id="` + id + `"></span></td>
<td class="share_rule" valign="middle"><img src="/pic/share_rule_3.gif" title="欢迎转载" /></td>
<td class="act" valign="middle"><a class="imdb" title="IMDB评分" href="retriver.php?id=` + id + `&amp;type=1&amp;siteid=1"><em class="t icon_t i_imdb"></em>6.8</a><br /><a href="download.php?hash=` + hash + `.demo-signature"><img class="download" src="pic/trans.gif" alt="download" title="下载本种" /></a><a id="bookmark0" href="javascript: bookmark(` + id + `,0);"><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a></td>
</tr></table></td><td class="t_comment">` + comment + `</td><td class="t_time"><span title="` + timeTitle + `">` + timeLeft + `</span></td><td class="t_size">` + size + `</td><td class="t_torrents">` + seeders + `</td>
<td class="t_leech">` + leechers + `</td>
<td class="t_completed">` + completed + `</td>
<td class="t_uploader">` + uploader + `</td>
</tr>
`
}

const (
	hdwingStickz   = `<img class="stickz" src="pic/trans.gif" alt="Stickz" title="置顶中的战斗机" />&nbsp;`
	hdwingSeeding  = `<div class="progressarea" title="做种中"><img src="plugin/style/progress_seeding.gif" /><div class="progress"><div class="progress_seeding" style="width:100%;"></div></div></div>`
	hdwingAnon     = `<i>匿名</i>`
	hdwingUploader = `<span class="nowrap"><a href="userdetails.php?id=20002" class="Uploader_Name" style=''><b>uploader</b></a></span>`
)

var hdwingSearchFixture = hdwingPage("种子", `<form method="get" name="searchbox"><input type="hidden" name="csrf" value="`+hdwingCSRF+`" /></form>
<form id="form_torrent" name="form_torrent" method="post" action="">
<table class="torrent_list" border="0" cellspacing="0" cellpadding="0">
<tr class="tab_header"><th width="49">类型</th><th><a href="?sort=1&amp;type=asc">标题</a></th><th width="30"><img class="comments" src="pic/trans.gif" alt="comments" title="评论数" /></th><th width="60"><img class="time" src="pic/trans.gif" alt="time" title="存活时间" /></th><th width="40"><img class="size" src="pic/trans.gif" alt="size" title="大小" /></th><th width="30"><img class="seeders" src="pic/trans.gif" alt="seeders" title="种子数" /></th><th width="30"><img class="leechers" src="pic/trans.gif" alt="leechers" title="下载数" /></th><th width="30"><img class="snatched" src="pic/trans.gif" alt="snatched" title="完成数" /></th><th width="60">发布者</th></tr>
`+hdwingRow("stickz_bg", "17", "电影Movie(1080p)", hdwingStickz, "200101", "Demo Night 2026 Director's Cut 1080p BluRay DD+5.1 x264-DEMO",
	`示例 / 示例(导演版) | 类型: 剧情 / 喜剧 [粤语] *导演剪辑版* `, hdwingSeeding,
	`<b><a href="details.php?id=200101&amp;hit=1&amp;cmtpage=1#startcomments">3</a></b>`, "2026-11-13 15:11:50", "4天23时", "23.47&nbsp;GB",
	`<a href="details.php?id=200101&amp;hit=1&amp;dllist=1#seeders">190</a>`, `<a href="details.php?id=200101&amp;hit=1&amp;dllist=1#leechers">1</a>`,
	`<a href="viewsnatches.php?id=200101">325</a>`, hdwingAnon)+
	hdwingRow("stickz_bg", "14", "动画片(Animation)", hdwingStickz, "200102", "Demo Dragon 2021 Blu-ray 1080p AVC Atmos TrueHD 7.1-DEMO",
		`<b>[<font class="recommended">推荐</font>]</b>示例龙 DIY简/繁/双语特效字幕 菜单修改 `, "",
		`<b><a href="details.php?id=200102&amp;hit=1&amp;cmtpage=1#startcomments">12</a></b>`, "2022-10-14 18:57:28", "4年1月", "110.10&nbsp;GB",
		`<a href="details.php?id=200102&amp;hit=1&amp;dllist=1#seeders">38</a>`, `<a href="details.php?id=200102&amp;hit=1&amp;dllist=1#leechers">3</a>`,
		`<a href="viewsnatches.php?id=200102">82</a>`, hdwingAnon)+
	hdwingRow("", "20", "原盘(Full BD)", "", "200103", "Demo Sights 2008 BluRay 1080i AVC DD2.0",
		`示例纪录片 [蓝光原盘 中英双语] `, "",
		`<a href="comment.php?action=add&amp;pid=200103&amp;type=torrent" title="添加评论">0</a>`, "2026-11-18 14:38:59", "9分", "17.23&nbsp;GB",
		`<a href="details.php?id=200103&amp;hit=1&amp;dllist=1#seeders"><font color="#cc0000">1</font></a>`, `<a href="details.php?id=200103&amp;hit=1&amp;dllist=1#leechers">11</a>`,
		`0`, hdwingAnon)+
	hdwingRow("", "21", "欧美剧集包(EU/US TV series pack)", "", "200104", "Demo Women 2025 S01 1080p WEB-DL H264 AAC 2.0-DEMO",
		`示例剧 第一季 | 全6集 [英语/中英字幕] `, "",
		`<a href="comment.php?action=add&amp;pid=200104&amp;type=torrent" title="添加评论">0</a>`, "2026-11-18 14:15:54", "32分", "12.15&nbsp;GB",
		`<a href="details.php?id=200104&amp;hit=1&amp;dllist=1#seeders">7</a>`, `<a href="details.php?id=200104&amp;hit=1&amp;dllist=1#leechers">3</a>`,
		`<a href="viewsnatches.php?id=200104">5</a>`, hdwingUploader)+
	hdwingRow("", "21", "欧美剧集包(EU/US TV series pack)", "", "200105", "Demo River 2026 S01 1080p WEB-DL H264 AAC 2.0-DEMO",
		`示例河谷 第一季 | 全6集 [英语/中英字幕] `, "",
		`<a href="comment.php?action=add&amp;pid=200105&amp;type=torrent" title="添加评论">0</a>`, "2026-11-18 12:13:20", "2时34分", "11.05&nbsp;GB",
		`<a href="details.php?id=200105&amp;hit=1&amp;dllist=1#seeders">10</a>`, `<a href="details.php?id=200105&amp;hit=1&amp;dllist=1#leechers">3</a>`,
		`<a href="viewsnatches.php?id=200105">9</a>`, hdwingAnon)+
	hdwingRow("", "408", "音乐(Music)", "", "200106", "示例歌手三张专辑 FLAC",
		``, "",
		`<a href="comment.php?action=add&amp;pid=200106&amp;type=torrent" title="添加评论">0</a>`, "2026-11-18 10:43:56", "4时4分", "512.00&nbsp;MB",
		`<span class="red">0</span>`, `<a href="details.php?id=200106&amp;hit=1&amp;dllist=1#leechers">1</a>`,
		`<a href="viewsnatches.php?id=200106">2</a>`, hdwingAnon)+`</table></form>
<script>jQuery(function() {
    const torrents = jQuery('span.sp_state_placeholder').map(function() {
        return this.id;
    }).toArray();
    jQuery.post('ajax_promotion.php', {'ids': torrents, 'csrf': get_csrf()},
        function(response) {
           if(response.status === 200) {
               for (const [key, value] of Object.entries(response.message)) {
                   jQuery('span#' + key + '.sp_state_placeholder').replaceWith('<p>' + value['sp_state'] + '</p>' + value['timeout']);
                }
           }
        }
    );
});</script>`)

// hdwingPromo 是一个种子的优惠图标（ajax_promotion.php 返回的 sp_state），提示里有优惠名与剩余时间
func hdwingPromo(class, alt, fontClass, label, end, left string) string {
	return ` <img class="` + class + `" src="pic/trans.gif" alt="` + alt + `" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;` + fontClass + `&quot;&gt;` + label +
		`&lt;/font&gt;&lt;/b&gt;　剩余时间：&lt;b&gt;&lt;span title=&quot;` + end + `&quot;&gt;` + left + `&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000,'fade','both','styleClass','niceTitle', 'fadeMax',87, 'maxWidth', 300);" />`
}

func hdwingTimeout(end, left string) string {
	return `<span title="` + end + `">` + left + `</span>`
}

// hdwingPromotionJSON 是 ajax_promotion.php 的返回：没有优惠的种子 sp_state、timeout 都是空的
func hdwingPromotionJSON(t *testing.T, states map[string][2]string) string {
	t.Helper()
	msg := map[string]map[string]string{}
	for id, s := range states {
		msg[id] = map[string]string{"sp_state": s[0], "timeout": s[1]}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	require.NoError(t, enc.Encode(map[string]any{"status": 200, "message": msg}))
	return buf.String()
}

func hdwingSearchPromotions(t *testing.T) string {
	return hdwingPromotionJSON(t, map[string][2]string{
		"200101": {"", ""},
		"200102": {hdwingPromo("pro_free", "Free", "free", "免费", "2026-12-07 11:45:28", "18天20时"), hdwingTimeout("2026-12-07 11:45:28", "18天20时")},
		"200103": {hdwingPromo("pro_50pctdown", "50%", "halfdown", "50%", "2026-11-18 20:38:59", "5时50分"), hdwingTimeout("2026-11-18 20:38:59", "5时50分")},
		"200104": {hdwingPromo("pro_2up", "2X", "twoup", "2X", "2026-11-18 20:15:54", "5时27分"), hdwingTimeout("2026-11-18 20:15:54", "5时27分")},
		"200105": {hdwingPromo("pro_free2up", "2X Free", "twoupfree", "2X免费", "2026-11-18 18:13:20", "3时25分"), hdwingTimeout("2026-11-18 18:13:20", "3时25分")},
		"200106": {"", ""},
	})
}

// 详情页：标题在 h2#top（优惠占位也在这里），副标题是 h3；大小在「基本信息」的 li 里；下载链接都是签名的 download.php?hash=；
// 「其它版本」表里另一个种子也有占位（它是 2X，不能算到本种头上）；标题与编号另在字幕表单的隐藏域里。
var hdwingDetailFixture = hdwingPage("种子详情 \"Demo Dragon 2021 Blu-ray 1080p AVC Atmos TrueHD 7.1-DEMO\"", `<div class="details_box">
<table class="movie_details" border="0" cellspacing="0" cellpadding="0">
<tr><td colspan="2"><div class="m_name"><h2 id="top">Demo Dragon 2021 Blu-ray 1080p AVC Atmos TrueHD 7.1-DEMO <span class="sp_state_placeholder" id="200102"></span></h2>
<h3>示例龙 DIY简/繁/双语特效字幕 菜单修改</h3>
</div></td></tr>
<tr><td class="poster_box" width="320"><div class="imgbox"><img src="https://hdwing.org/imdb/images/tt1234567_imdb.jpg" /><span></span><b>7.0</b></div></td>
<td valign="top" class="info_box">
<ul><li class="right w710"><a id="bookmark0" href="javascript: bookmark(200102,0);"><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏" /></a><a class="torrentdown_button" title="[HDW].Demo Dragon 2021 Blu-ray 1080p AVC Atmos TrueHD7.1-DEMO.torrent" href="download.php?hash=MQoyMDAxMDIKMTAwMDE.demo-signature"> </a></li></ul>
<ul><li class="left">种子添加</li><li class="right">由&nbsp;&nbsp;<i>匿名</i>&nbsp;&nbsp;发布于<span title="2022-10-14 18:57:28">4年1月前</span></li></ul>
<ul><li class="left">基本信息</li><li class="right bspace"><i><b>类型：</b>动画片(Animation)</i><i><b>媒介：</b>Blu-ray</i><i><b>编码：</b>H.264/AVC</i><i><b>音频编码：</b>Atmos</i><i><b>格式：</b>1080P</i><br /><i><b>大小：</b>110.10 GB</i><i><b>制作组：</b>其他(Other)</i></li></ul>
<ul><li class="left">行为</li><li class="right"><a title="下载种子" href="download.php?hash=MQoyMDAxMDIKMTAwMDE.demo-signature"><img class="dt_download" src="pic/trans.gif" alt="download" />&nbsp;<b><font class="small">下载种子</font></b></a>&nbsp;|&nbsp;<a title="举报该种子违反了规则" href="report.php?torrent=200102"><img class="dt_report" src="pic/trans.gif" alt="report" /></a></li></ul>
<ul><li class="left">下载链接</li><li class="right"><a id="clip_target" href="https://hdwing.org/download.php?hash=MQoyMDAxMDIKMTAwMDE.demo-signature&amp;uid=10001">https://hdwing.org/download.php?hash=MQoyMDAxMDIKMTAwMDE.demo-signature&amp;uid=10001</a></li></ul>
<ul><li class="left">优惠历史</li><li class="right bspace"><i><strong><a href="promotion.php?id=200102">点击查询</a></strong></i></li></ul>
</td></tr></table>
<table class="table_details" align="center" cellspacing="0" cellpadding="0">
<tr><td class="rowhead">字幕</td><td class="rowfollow" valign="top"><div class="subtitle_box"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Demo Dragon 2021 Blu-ray 1080p AVC Atmos TrueHD 7.1-DEMO" /><input type="hidden" name="detail_torrent_id" value="200102" /><input type="hidden" name="in_detail" value="in_detail" /><input type="submit" value="上传字幕" /></form></div></td></tr>
<tr><td class="rowhead">简介</td><td class="rowfollow" valign="top"><div id="kdescr">◎片　　名　示例龙<br />◎IMDb链接　https://www.imdb.com/title/tt1234567/<br />◎豆瓣链接　https://movie.douban.com/subject/1234567/</div></td></tr>
<tr><td class="rowhead">IMDb信息</td><td class="rowfollow" align="left"><div id="kimdb"><a href="https://www.imdb.com/title/tt1234567/">IMDb</a></div></td></tr>
<tr><td class="rowhead">豆瓣信息</td><td class="rowfollow" align="left"><a href="https://movie.douban.com/subject/1234567/">豆瓣</a></td></tr>
<tr><td class="rowhead">其它版本</td><td class="rowfollow" valign="top"><b>1个其它版本 </b><br /><div id="kothercopy" style="display: none;"><table class="detail_moreotherver" border="1" cellspacing="0" cellpadding="0">
<tr><th width="40">类型</th><th>标题</th><th>质量</th></tr>
<tr><td class="more_cat"><a href="torrents.php?allsec=1&amp;cat=14"><img class="catchs type_14" src="pic/cattrans.gif" alt="动画片(Animation)" title="动画片(Animation)" /></a></td><td><a href="https://hdwing.org/details.php?id=200901&amp;hit=1">Demo.Dragon.2021.2160p.UHD.Blu-ray.HEVC.Atmos.TrueHD.7.1-DEMO</a> <span class="sp_state_placeholder" id="200901"></span></td><td>Blu-ray, x265/HEVC, 4K</td></tr>
</table></div></td></tr>
</table></div>`)

func hdwingDetailPromotions(t *testing.T) string {
	return hdwingPromotionJSON(t, map[string][2]string{
		"200102": {hdwingPromo("pro_free", "Free", "free", "免费", "2026-12-07 11:45:28", "18天20时"), hdwingTimeout("2026-12-07 11:45:28", "18天20时")},
		"200901": {hdwingPromo("pro_2up", "2X", "twoup", "2X", "2026-11-19 14:24:22", "23时33分"), hdwingTimeout("2026-11-19 14:24:22", "23时33分")},
	})
}

// 首页：顶栏有用户名、魔力值与做种、下载数；「热门」里的种子也有优惠占位（取用户信息时不补）
var hdwingIndexFixture = hdwingPage("首页", `<div class="indexbox"><div class="index_left">
<table class="index_new" style="margin-top:18px;" cellspacing="0" cellpadding="0"><tr><th class="left">热门</th><th width="40">做种</th><th width="40">下载</th></tr>
<tr><td><a href="details.php?id=200201&amp;hit=1">Demo.Hot.2018.1080p.BluRay.x264-DEMO<span class="sp_state_placeholder" id="200201"></span></a></td><td class="center color">37</td><td class="center">1</td></tr></table>
</div></div>`)

// 个人页：「传输」一格是分享率、上传量与下载量（括号里是本月的）；等级是图片的 title；当前 IP、客户端换成了示例地址
var hdwingUserdetailsFixture = hdwingPage("用户详情", `<div class="noraml_box" style="width:auto !important;">
<h2><span class="nowrap"><b>demo</b><img class="starbig" src="pic/trans.gif" alt="Donor" style="margin-left: 4pt" /></span><img src="pic/flag/china.gif" alt="中国" style="margin-left: 8pt" /></h2>
<table class="normal_tab" cellspacing="0" cellpadding="5">
<tr><td width="100" class="rowhead">邀请</td><td class="rowfollow"><a href="invite.php?id=10001">没有邀请资格</a></td></tr>
<tr><td width="100" class="rowhead">邀请人</td><td class="rowfollow"><span class="nowrap"><a href="userdetails.php?id=1" class="Administrator_Name" style=''><b>HDChina</b></a></span></td></tr>
<tr><td width="100" class="rowhead">加入日期</td><td class="rowfollow">2025-03-02 08:30:00 (<span title="2025-03-02 08:30:00">1年8月前</span>)</td></tr>
<tr><td width="100" class="rowhead">最近动向</td><td class="rowfollow">2026-11-18 09:15:30 (<span title="2026-11-18 09:15:30">5时前</span>)</td></tr>
<tr><td width="100" class="rowhead">最近活动(Tracker)</td><td class="rowfollow">2026-11-18 14:00:00 (<span title="2026-11-18 14:00:00">1分前</span>)</td></tr>
<tr><td width="100" class="rowhead">当前IP</td><td class="rowfollow">192.0.2.10[示例]</td></tr>
<tr><td width="100" class="rowhead">BT客户端</td><td class="rowfollow">qBittorrent/5.0.0 (192.0.2.10:6881; torrents:128)<br /></td></tr>
<tr><td width="100" class="rowhead">传输</td><td class="rowfollow"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>分享率</strong>:  <font color="">2.500</font></td><td class="embedded">&nbsp;&nbsp;<img src="pic/smilies/163.gif" alt="" /></td></tr><tr><td class="embedded"><strong>上传量</strong>:  1.25 TB(本月100.00 GB)</td><td class="embedded">&nbsp;&nbsp;<strong>下载量</strong>:  512.00 GB(本月20.00 GB)</td></tr></table></td></tr>
<tr><td width="100" class="rowhead">BT时间</td><td class="rowfollow"><table border="0" cellspacing="0" cellpadding="0"><tr><td class="embedded"><strong>做种/下载时间比率</strong>:  <font color="">60.500</font></td></tr><tr><td class="embedded"><strong>做种时间</strong>:  121天00:00:00(本月30天00:00:00)</td><td class="embedded">&nbsp;&nbsp;<strong>下载时间</strong>:  2天00:00:00(本月1天00:00:00)</td></tr></table></td></tr>
<tr><td width="100" class="rowhead">等级</td><td class="rowfollow"><img alt="客串龙套(Power User)" title="客串龙套(Power User)" src="pic/power.gif" /> </td></tr>
<tr><td width="100" class="rowhead">种子评论</td><td class="rowfollow">0</td></tr>
<tr><td width="100" class="rowhead">魔力值</td><td class="rowfollow">12345.6</td></tr>
<tr><td width="100" class="rowhead">做种积分</td><td class="rowfollow">0.0</td></tr>
<tr><td width="100" class="rowhead">任务</td><td class="rowfollow">当前考核任务名称：新手任务一(Newbie task 1)<br />上传↑35.00 GB:当前1.25 TB，已完成<br /></td></tr>
<tr><td width="100" class="rowhead">当前做种</td><td class="rowfollow"><a href="javascript: getusertorrentlistajax('10001', 'seeding', 'ka1'); klappe_news('a1')"><u>[显示/隐藏]</u></a><div id="ka1" style="display: none;"></div></td></tr>
</table></div>`)

// 魔力值页：捐赠者的那句前面先写不算捐赠加成的时魔，要取的是「你当前每小时能获取」；商店里还留着「消除H&R」一项
var hdwingBonusFixture = hdwingPage("魔力值", `<div class="noraml_box my_bonus"><table class="normal_tab mybonus" cellspacing="0" cellpadding="3">
<tr><td class="colhead" colspan="4" align="center"><font class="big">HDWinG魔力值系统</font></td></tr>
<tr><td class="text" align="center" colspan="4">用你的魔力值（当前12,345.6）换东东！</td></tr>
<tr><td class="rowhead_center"><b>13</b></td><td class="rowfollow" align="left"><h1>消除H&amp;R</h1>每消除一个H&amp;R，需要100000魔力值，请确保你有足够的魔力值！</td><td class="rowfollow" align="center">100,000</td><td class="rowfollow" align="center"><input type="submit" name="submit" value="你没有H&amp;R" disabled="disabled" /></td></tr>
</table>
<table class="normal_tab mybonus" cellspacing="0" cellpadding="3"><tr><td class="text" align="left">
<h2>做种每小时将得到如下的魔力值</h2><ul><li>0.5个魔力值 * <i>你的做种数</i> (做种数最多计20个)</li><li>捐赠者每次将得到2倍的魔力值</li></ul>
<div class="bonus_bar_box"><h3 align="center">如果没有捐赠标志，你每小时仅能获取8.250个魔力值 (A = 12.3); 你当前每小时能获取16.500个魔力值 (A = 24.6)[<a href="mybonus_detail.php"><b>查看详情</b></a>]</h3><p class="loadbarbg"><img class="loadbarred" src="pic/trans.gif" style="width: 120px;" alt="15%" /></p></div>
<h2>其他获取方式</h2><ul><li>发布新种子 = 50个魔力值</li></ul>
</td></tr></table></div>`)

// 一个最小的种子文件（bencode 字典）
const hdwingTorrentFile = "d8:announce39:https://tracker.hdwing.org/announce.php4:infod6:lengthi1e4:name4:demo12:piece lengthi16384e6:pieces20:aaaaaaaaaaaaaaaaaaaaee"

func getHDWinGDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("hdwing")
	require.True(t, ok, "hdwing definition not found")
	return def
}

// hdwingServer 回固定的页面，记下每个地址收到的查询串与优惠状态的请求。
type hdwingServer struct {
	*httptest.Server
	mu         sync.Mutex
	queries    map[string][]string
	promoIDs   []string
	promoCSRF  string
	promoCalls int
	cookies    map[string]string
}

func newHDWinGServer(t *testing.T, pages map[string]string, promotions string) *hdwingServer {
	t.Helper()
	hs := &hdwingServer{queries: map[string][]string{}, cookies: map[string]string{}}
	mux := http.NewServeMux()
	for path, body := range pages {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			hs.mu.Lock()
			hs.queries[r.URL.Path] = append(hs.queries[r.URL.Path], r.URL.RawQuery)
			hs.cookies[r.URL.Path] = r.Header.Get("Cookie")
			hs.mu.Unlock()
			if strings.HasPrefix(body, "d8:") {
				w.Header().Set("Content-Type", "application/x-bittorrent")
			}
			_, _ = w.Write([]byte(body))
		})
	}
	mux.HandleFunc("/ajax_promotion.php", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.NoError(t, r.ParseForm())
		hs.mu.Lock()
		hs.promoCalls++
		hs.promoIDs = r.PostForm["ids[]"]
		hs.promoCSRF = r.PostForm.Get("csrf")
		hs.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(promotions))
	})
	hs.Server = httptest.NewServer(mux)
	t.Cleanup(hs.Close)
	return hs
}

// site 是和生产上一样由 SiteFactory 建的站点实例（定义里的选择器并进默认值，带着 PromotionAJAX），只是地址换成测试服务器
func (hs *hdwingServer) site(t *testing.T) *v2.BaseSite[v2.NexusPHPRequest, v2.NexusPHPResponse] {
	t.Helper()
	site, err := v2.NewSiteFactory(nil).CreateSite(v2.SiteConfig{
		ID: "hdwing", BaseURL: hs.URL, Options: json.RawMessage(`{"cookie":"c_secure_pass=demo"}`), RateLimit: 100, RateBurst: 10,
	})
	require.NoError(t, err)
	base, ok := site.(*v2.BaseSite[v2.NexusPHPRequest, v2.NexusPHPResponse])
	require.True(t, ok)
	return base
}

func (hs *hdwingServer) driver(t *testing.T) *v2.NexusPHPDriver {
	t.Helper()
	d, ok := hs.site(t).GetDriver().(*v2.NexusPHPDriver)
	require.True(t, ok)
	return d
}

func testHDWinGSearch(t *testing.T) {
	hs := newHDWinGServer(t, map[string]string{"/torrents.php": hdwingSearchFixture}, hdwingSearchPromotions(t))

	items, err := hs.site(t).Search(context.Background(), v2.SearchQuery{Keyword: "Demo"})
	require.NoError(t, err)
	require.Len(t, items, 6, "表头不算种子")

	q, err := url.ParseQuery(hs.queries["/torrents.php"][0])
	require.NoError(t, err)
	assert.Equal(t, "Demo", q.Get("search"))
	// 一页一次：ids[] 是列表上每个种子，带着页面里的 csrf
	assert.Equal(t, 1, hs.promoCalls)
	assert.Equal(t, []string{"200101", "200102", "200103", "200104", "200105", "200106"}, hs.promoIDs)
	assert.Equal(t, hdwingCSRF, hs.promoCSRF)

	cst := time.FixedZone("CST", 8*3600)
	gib := float64(1024 * 1024 * 1024)

	sticky := items[0]
	assert.Equal(t, "200101", sticky.ID)
	assert.Equal(t, "Demo Night 2026 Director's Cut 1080p BluRay DD+5.1 x264-DEMO", sticky.Title)
	assert.Equal(t, "示例 / 示例(导演版) | 类型: 剧情 / 喜剧 [粤语] *导演剪辑版*", sticky.Subtitle)
	assert.Equal(t, int64(23.47*gib), sticky.SizeBytes, "大小是「23.47&nbsp;GB」")
	assert.Equal(t, 190, sticky.Seeders)
	assert.Equal(t, 1, sticky.Leechers)
	assert.Equal(t, 325, sticky.Snatched)
	assert.Equal(t, "电影Movie(1080p)", sticky.Category)
	assert.Equal(t, v2.DiscountNone, sticky.DiscountLevel, "置顶不是优惠")
	assert.True(t, sticky.DiscountEndTime.IsZero())
	// 发布时间取 span 的 title；与别的 NexusPHP 站一样按 UTC 记
	assert.Equal(t, time.Date(2026, 11, 13, 15, 11, 50, 0, time.UTC).Unix(), sticky.UploadedAt)
	assert.Equal(t, "/api/site/hdwing/torrent/200101/download", sticky.DownloadURL)
	assert.Equal(t, hs.URL+"/details.php?id=200101&hit=1", sticky.URL)

	free := items[1]
	assert.Equal(t, "200102", free.ID)
	assert.Equal(t, "示例龙 DIY简/繁/双语特效字幕 菜单修改", free.Subtitle, "副标题前面的「[推荐]」去掉")
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	assert.Equal(t, time.Date(2026, 12, 7, 11, 45, 28, 0, cst).Unix(), free.DiscountEndTime.Unix(), "结束时间按北京时间")
	assert.Equal(t, int64(110.10*gib), free.SizeBytes)

	half := items[2]
	assert.Equal(t, v2.DiscountPercent50, half.DiscountLevel)
	assert.Equal(t, time.Date(2026, 11, 18, 20, 38, 59, 0, cst).Unix(), half.DiscountEndTime.Unix())
	assert.Equal(t, 1, half.Seeders, "做种数在 font 里")
	assert.Equal(t, 11, half.Leechers)
	assert.Equal(t, 0, half.Snatched, "完成数是裸文字 0")

	twoUp := items[3]
	assert.Equal(t, v2.Discount2xUp, twoUp.DiscountLevel)
	assert.Equal(t, time.Date(2026, 11, 18, 20, 15, 54, 0, cst).Unix(), twoUp.DiscountEndTime.Unix())

	twoUpFree := items[4]
	assert.Equal(t, v2.Discount2xFree, twoUpFree.DiscountLevel, "pro_free2up 不能认成免费或 2X")
	assert.Equal(t, time.Date(2026, 11, 18, 18, 13, 20, 0, cst).Unix(), twoUpFree.DiscountEndTime.Unix())

	music := items[5]
	assert.Equal(t, "示例歌手三张专辑 FLAC", music.Title)
	assert.Empty(t, music.Subtitle)
	assert.Equal(t, int64(512*1024*1024), music.SizeBytes)
	assert.Equal(t, 0, music.Seeders, "断种是 <span class=red>0</span>")
	assert.Equal(t, v2.DiscountNone, music.DiscountLevel)

	for _, it := range items {
		assert.False(t, it.HasHR, "%s：站点没有 H&R 标记", it.ID)
		assert.Empty(t, it.DiscountErr, it.ID)
	}

	// 免费列表：spstate=2（站点的优惠筛选里「免费」是 2）
	hs2 := newHDWinGServer(t, map[string]string{"/torrents.php": hdwingSearchFixture}, hdwingSearchPromotions(t))
	_, err = hs2.site(t).Search(context.Background(), v2.SearchQuery{FreeOnly: true})
	require.NoError(t, err)
	q2, err := url.ParseQuery(hs2.queries["/torrents.php"][0])
	require.NoError(t, err)
	assert.Equal(t, "2", q2.Get("spstate"))
}

func testHDWinGDetail(t *testing.T) {
	hs := newHDWinGServer(t, map[string]string{"/details.php": hdwingDetailFixture}, hdwingDetailPromotions(t))

	item, err := hs.site(t).GetDetailFetcher().GetTorrentDetail(context.Background(), "200102", hs.URL+"/details.php?id=200102&hit=1", "")
	require.NoError(t, err)
	assert.Equal(t, "200102", item.ID)
	assert.Equal(t, "Demo Dragon 2021 Blu-ray 1080p AVC Atmos TrueHD 7.1-DEMO", item.Title)
	assert.Equal(t, v2.DiscountFree, item.DiscountLevel, "只看 h2#top 里本种的图标，其它版本的 2X 不算")
	assert.Equal(t, time.Date(2026, 12, 7, 11, 45, 28, 0, time.FixedZone("CST", 8*3600)).Unix(), item.DiscountEndTime.Unix())
	assert.Empty(t, item.DiscountErr)
	// 详情页的大小先按 MB 算（110.10 GB = 112742.4 MB）再换成字节，与按 GiB 直接算的差不到 1 字节
	gib := float64(1024 * 1024 * 1024)
	assert.InDelta(t, 110.10*gib, float64(item.SizeBytes), 1, "大小在「基本信息」的 li 里")
	assert.False(t, item.HasHR)
	assert.Equal(t, "tt1234567", item.IMDbID)
	assert.Equal(t, "1234567", item.DoubanID)
	assert.ElementsMatch(t, []string{"200102", "200901"}, hs.promoIDs, "页面上的占位一次取完")
	assertHDWinGDetailQuery(t, hs.queries["/details.php"])
}

// assertHDWinGDetailQuery 断言只取了一次详情页，参数是 id=200102&hit=1
func assertHDWinGDetailQuery(t *testing.T, queries []string) {
	t.Helper()
	require.Len(t, queries, 1)
	q, err := url.ParseQuery(queries[0])
	require.NoError(t, err)
	assert.Equal(t, url.Values{"id": {"200102"}, "hit": {"1"}}, q)
}

func testHDWinGUserInfo(t *testing.T) {
	def := getHDWinGDef(t)
	driver := newTestNexusPHPDriver(def)

	pages := []struct {
		name    string
		fixture string
		want    map[string]string
	}{
		{"index", hdwingIndexFixture, map[string]string{
			"id":       "10001",
			"name":     "demo",
			"bonus":    "12345.6",
			"seeding":  "128",
			"leeching": "3",
		}},
		{"userdetails", hdwingUserdetailsFixture, map[string]string{
			"uploaded":     "1374389534720", // 1.25 TB，不是括号里本月的 100.00 GB
			"downloaded":   "549755813888",  // 512.00 GB
			"ratio":        "2.5",
			"levelName":    "客串龙套(Power User)",
			"seedingBonus": "0",
			"joinTime":     "1740875400", // 2025-03-02 08:30:00 +0800
			"lastAccessAt": "1794964530", // 2026-11-18 09:15:30 +0800（「最近动向」，不是 Tracker 的时间）
		}},
		{"mybonus", hdwingBonusFixture, map[string]string{
			"bonusPerHour": "16.5", // 「你当前每小时能获取」，不是前面不算捐赠加成的 8.250
		}},
	}
	for _, page := range pages {
		t.Run(page.name, func(t *testing.T) {
			doc := FixtureDoc(t, "hdwing_"+page.name, page.fixture)
			for field, want := range page.want {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q missing", field)
				assert.Equal(t, want, driver.ExtractFieldValuePublic(doc, sel), "field %q", field)
			}
		})
	}
}

// 走 GetUserInfo 全流程：首页取 ID、用户名、魔力值与做种数，个人页按 ID 取流量、等级与时间，魔力值页取时魔。
// 首页的「热门」里也有优惠占位：取用户信息不补优惠状态，不发 ajax_promotion.php。
// 做种列表没核实（个人页由页面脚本另取）：测试服务器没有 getusertorrentlistajax.php，做种体积不取、也不算失败。
func TestHDWinG_GetUserInfo(t *testing.T) {
	def := getHDWinGDef(t)
	hs := newHDWinGServer(t, map[string]string{
		"/index.php":       hdwingIndexFixture,
		"/userdetails.php": hdwingUserdetailsFixture,
		"/mybonus.php":     hdwingBonusFixture,
	}, hdwingSearchPromotions(t))

	info, err := hs.site(t).GetUserInfo(context.Background())
	require.NoError(t, err)

	hs.mu.Lock()
	defer hs.mu.Unlock()
	assert.Equal(t, []string{"id=10001"}, hs.queries["/userdetails.php"])
	assert.Len(t, hs.queries["/mybonus.php"], 1)
	assert.Zero(t, hs.promoCalls)
	assert.Equal(t, "c_secure_pass=demo", hs.cookies["/userdetails.php"])

	cst := time.FixedZone("CST", 8*3600)
	assert.Equal(t, "10001", info.UserID)
	assert.Equal(t, "demo", info.Username)
	assert.Equal(t, int64(1.25*1024*1024*1024*1024), info.Uploaded)
	assert.Equal(t, int64(512*1024*1024*1024), info.Downloaded)
	assert.InDelta(t, 2.5, info.Ratio, 1e-9)
	assert.InDelta(t, 12345.6, info.Bonus, 1e-6)
	assert.InDelta(t, 16.5, info.BonusPerHour, 1e-9)
	assert.Zero(t, info.SeedingBonus)
	assert.Equal(t, 128, info.Seeding)
	assert.Equal(t, 3, info.Leeching)
	assert.Equal(t, "客串龙套(Power User)", info.LevelName)
	assert.Equal(t, time.Date(2025, 3, 2, 8, 30, 0, 0, cst).Unix(), info.JoinDate)
	assert.Equal(t, time.Date(2026, 11, 18, 9, 15, 30, 0, cst).Unix(), info.LastAccess)
	assert.Zero(t, info.TrueUploaded, "站点没有实际上传量")
	assert.Zero(t, info.SeederSize)
	assert.Zero(t, info.HnRUnsatisfied)

	// 保号探测：NexusPHP 的分类按个人页「最近动向」记最近访问，不用探测时间兜底
	clock := sitelogin.NewFakeClock(time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC))
	probe := sitelogin.ClassifyUserInfo(def, info, nil, clock)
	require.Equal(t, sitelogin.OK, probe.Status, probe.Diagnostic)
	require.NotNil(t, probe.LastAccessAt)
	assert.Equal(t, time.Date(2026, 11, 18, 9, 15, 30, 0, cst).Unix(), probe.LastAccessAt.Unix())
}

// Cookie 失效时站点回的页面没采到（PT-depiler 记的是「阻断页面」）；标准的登录页照样认成会话过期，不当成取到了空的用户信息
func TestHDWinG_GetUserInfoLoginPage(t *testing.T) {
	login := `<html><head><title>HDWinG :: 登录</title></head><body><form method="post" action="takelogin.php"><input name="username"><input name="password" type="password"></form></body></html>`
	hs := newHDWinGServer(t, map[string]string{"/index.php": login, "/mybonus.php": login}, "")

	_, err := hs.driver(t).GetUserInfo(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, v2.ErrSessionExpired)
}

// 按编号下载：先取详情页，再用页面上签名的 download.php?hash= 下载（站点没有 download.php?id=），带上 Cookie；
// 下载取详情页不补优惠状态。下载地址回的是网页时报错，不当成种子文件。
func TestHDWinG_Download(t *testing.T) {
	hs := newHDWinGServer(t, map[string]string{
		"/details.php":  hdwingDetailFixture,
		"/download.php": hdwingTorrentFile,
	}, hdwingDetailPromotions(t))

	data, err := hs.site(t).Download(context.Background(), "200102")
	require.NoError(t, err)
	assert.Equal(t, hdwingTorrentFile, string(data))
	parsed, err := v2.ParseTorrent(data)
	require.NoError(t, err, "下到的要是能解析的种子文件")
	assert.Equal(t, "demo", parsed.Name)
	assert.Equal(t, "https://tracker.hdwing.org/announce.php", parsed.Announce)

	hs.mu.Lock()
	assertHDWinGDetailQuery(t, hs.queries["/details.php"])
	require.Len(t, hs.queries["/download.php"], 1)
	assert.Equal(t, "hash=MQoyMDAxMDIKMTAwMDE.demo-signature", hs.queries["/download.php"][0], "用详情页上第一个签名链接（不带 uid 的那个）")
	assert.Equal(t, "c_secure_pass=demo", hs.cookies["/download.php"])
	assert.Zero(t, hs.promoCalls)
	hs.mu.Unlock()

	bad := newHDWinGServer(t, map[string]string{
		"/details.php":  hdwingDetailFixture,
		"/download.php": hdwingPage("出错了", `<p>签名无效</p>`),
	}, hdwingDetailPromotions(t))
	_, err = bad.site(t).Download(context.Background(), "200102")
	require.Error(t, err)
	assert.ErrorIs(t, err, v2.ErrNotTorrentFile)
}

// RSS 取详情（GetTorrentDetail）时本种的优惠状态没取回来：读不到是不是免费，报读不到详情，不当成没有优惠
func TestHDWinG_DetailPromotionMissing(t *testing.T) {
	hs := newHDWinGServer(t, map[string]string{"/details.php": hdwingDetailFixture}, hdwingPromotionJSON(t, map[string][2]string{
		"200901": {hdwingPromo("pro_2up", "2X", "twoup", "2X", "2026-11-19 14:24:22", "23时33分"), hdwingTimeout("2026-11-19 14:24:22", "23时33分")},
	}))

	item, err := hs.driver(t).GetTorrentDetail(context.Background(), "200102", "", "")
	require.Error(t, err)
	assert.Nil(t, item)
	assert.ErrorIs(t, err, v2.ErrTorrentDetailUnavailable)
}

// 签到：入口与回执没采到，定义标为不支持；站点实例也报不支持，签到不发请求
func TestHDWinG_AttendanceUnsupported(t *testing.T) {
	def := getHDWinGDef(t)
	ok, reason := v2.AttendanceSupport(def)
	assert.False(t, ok)
	assert.Contains(t, reason, "签到入口")

	hs := newHDWinGServer(t, map[string]string{"/attendance.php": "<html><body>签到成功</body></html>"}, "")
	site := hs.site(t)
	assert.False(t, site.SupportsAttendance())
	_, err := site.Attend(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, v2.ErrAttendanceUnsupported)
	hs.mu.Lock()
	assert.Empty(t, hs.queries["/attendance.php"], "不支持时不发签到请求")
	hs.mu.Unlock()
}

// 没有 H&R：不开整站 H&R，定义算不出做种要求；列表、详情页的 H&R 判断见上面的测试
func TestHDWinG_NoHR(t *testing.T) {
	def := getHDWinGDef(t)
	assert.False(t, def.HREnabled)
	assert.False(t, def.HasHRSeedTimeRule())
	assert.Zero(t, def.CalcHRSeedTimeH(50*1024*1024*1024))

	parser := v2.NewNexusPHPParserFromDefinition(def)
	assert.False(t, parser.ParseHR(FixtureDoc(t, "hdwing_bonus", hdwingBonusFixture).Selection), "魔力值商店的「消除H&R」不是种子的 H&R")
}

// 等级：个人页写「客串龙套(Power User)」；注册周数、下载量、分享率都要满足
func TestHDWinG_LevelRequirements(t *testing.T) {
	def := getHDWinGDef(t)
	reqs := def.LevelRequirements
	require.NotEmpty(t, reqs)

	assert.Equal(t, 1, v2.GuessUserLevelID(&v2.UserInfo{LevelName: "群众演员(User)"}, reqs))
	assert.Equal(t, 2, v2.GuessUserLevelID(&v2.UserInfo{LevelName: "客串龙套(Power User)"}, reqs))
	assert.Equal(t, 9, v2.GuessUserLevelID(&v2.UserInfo{LevelName: "影业帝国(Nexus Master)"}, reqs))
	assert.Equal(t, v2.MinManagerLevelID+3, v2.GuessUserLevelID(&v2.UserInfo{LevelName: "Moderator"}, reqs), "总版主不能认成论坛版主")
	assert.Equal(t, v2.MinManagerLevelID+4, v2.GuessUserLevelID(&v2.UserInfo{LevelName: "Forum Moderator"}, reqs))
	assert.Equal(t, v2.MinVipLevelID, v2.GuessUserLevelID(&v2.UserInfo{LevelName: "VIP"}, reqs))

	// 注册 6 周、下载 300 GB、分享率 1.6：够客串龙套，不够领衔主演（还差 4 周、200 GB、分享率 0.4）
	joined := time.Now().Add(-6 * 7 * 24 * time.Hour).Unix()
	info := &v2.UserInfo{LevelName: "客串龙套(Power User)", JoinDate: joined, Downloaded: 300 * 1024 * 1024 * 1024, Uploaded: 480 * 1024 * 1024 * 1024, Ratio: 1.6}
	progress := v2.CalculateSiteLevelProgress(info, reqs)
	require.NotNil(t, progress)
	require.NotNil(t, progress.NextLevel)
	assert.Equal(t, "领衔主演", progress.NextLevel.Name)
	assert.Equal(t, "P10W", progress.NextLevel.Interval)
	assert.Equal(t, "500GB", progress.NextLevel.Downloaded)
	assert.InDelta(t, 2.0, progress.NextLevel.Ratio, 1e-9)
	assert.Equal(t, int64(200*1024*1024*1024), progress.UnmetRequirements["downloaded"])
	assert.InDelta(t, 0.4, progress.UnmetRequirements["ratio"], 1e-9)
	assert.Contains(t, progress.UnmetRequirements, "interval")
}

func TestHDWinG_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search":            hdwingSearchFixture,
		"search_promotions": hdwingSearchPromotions(t),
		"detail":            hdwingDetailFixture,
		"detail_promotions": hdwingDetailPromotions(t),
		"index":             hdwingIndexFixture,
		"userdetails":       hdwingUserdetailsFixture,
		"bonus":             hdwingBonusFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
	_ = DecodeFixtureJSON[map[string]any](t, "search_promotions", hdwingSearchPromotions(t))
}
