package definitions

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/internal/sitelogin"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func init() {
	RegisterFixtureSuite(FixtureSuite{
		SiteID:   "iptorrents",
		Search:   testIPTorrentsSearch,
		Detail:   testIPTorrentsDetail,
		UserInfo: testIPTorrentsUserInfo,
	})
}

// --- Fixtures ---
//
// 结构照搬 2026-11-21 登录后取到的真实页面（英文界面）：服务端原样输出的 HTML 不写 </td>、</th>，表头的 <thead><tr>
// 也不闭合，这里照样保留。用户名、用户 ID、各项统计、种子名与编号、IMDb 编号都换成了虚构的值；页面上的 passkey、
// 客户端与 IP、邀请人、感谢名单、站内信与论坛链接没有放进来。

// iptFixtureNow 是换算列表上相对时间用的「现在」（UTC）。
var iptFixtureNow = time.Date(2026, 11, 21, 11, 30, 0, 0, time.UTC)

const iptFixtureCookie = "uid=1234567; pass=FAKE_TEST_PASS"

// iptHeaderFixture 是每一页页头的统计栏（.stats）：自己的用户链接、退出登录表单、分享率、上传下载、活动种子数、积分。
const iptHeaderFixture = `<div id="iptStart"><div id="Head"><div class="stats"><div><div><a href="/u/1234567" class="tTipWrap uname"><div class="tTip ttop">Profile</div>fixture_user</a><form action="/lout.php" method="post"><button type="submit" class="tTipWrap"><div class="tTip ttop">Log out</div><i class="fa fa-power-off ico"></i></button></form><a href="/invite.php" class="tTipWrap"><div class="tTip ttop">Invites</div><b style="color:yellow;font-weight:bold">( 0 )</b></a><span class="c_ratio tTipWrap"><div class="tTip ttop">Ratio</div><i class="fa fa-battery-three-quarters"></i> 20.48</span><span class="tTipWrap"><div class="tTip ttop">Uploaded</div><i class="fa fa-upload grn"></i> 1 TB</span><span class="tTipWrap"><div class="tTip ttop">Downloaded</div><i class="fa fa-download red"></i> 50 GB</span><a href="/peers?u=1234567" class="tTipWrap"><div class="tTip ttop">Active Torrents</div><i class="fa fa-angle-double-up ico grn"></i> 12&nbsp;<i class="fa fa-angle-double-down ico red"></i> 3</a><a href="/mybonus.php" class="tTipWrap"><div class="tTip ttop">Bonus Points</div><i class="fa fa-gift ico"></i> 1,234.5</a></div></div><div class="rCol"><!--<i class="fa fa-clock-o"></i> 2026-11-21 11:30 UTC --><a href="/inbox" class="tTipWrap"><div class="tTip ttop">No new messages</div><i class="fa fa-envelope-o ico"></i></a></div></div></div></div>`

const iptTableHead = `<table id="torrents" class="t1" align="center"><thead><tr><th class="ac">Type<th class="al"><a href="?o=name">Name</a><th class="ac bok"><a class="tTipWrap c0" href="/t?bookmarks#torrents"><i class="fa fa-star" alt="Bookmark"></i><div class="tTip ttop">Go to Bookmarks</div></a><th class="ac"><i class="fa fa-download" alt="Download"></i><th class="ac"><a class="tTipWrap" href="?o=comments#torrents"><i class="fa fa-comments-o" alt="Comments"></i><div class="tTip ttop">Sort by comments</div></a><th class="ac"><a class="tTipWrap" href="?o=size#torrents"><i class="fa fa-file-o" alt="Size"></i><div class="tTip ttop">Sort by size</div></a><th class="ac"><a class="tTipWrap" href="?o=completed#torrents"><i class="fa fa-files-o" alt="Snatches"></i><div class="tTip ttop">Sort by snatches</div></a><th class="ac"><a class="tTipWrap" href="?o=seeders#torrents"><i class="fa fa-arrow-up grn" alt="Seeders"></i><div class="tTip ttop">Sort by seeders</div></a><th class="ac"><a class="tTipWrap" href="?o=leechers#torrents"><i class="fa fa-arrow-down red" alt="Leechers"></i><div class="tTip ttop">Sort by leechers</div></a><tbody>`

// 四行：普通种（标签 + 分钟前）、免费种（FreeLeech + 小时前）、没有标签的音乐（周前）、月前的教程（评论数 1）。
const iptSearchFixture = `<!DOCTYPE html><html class=""><head><title>Torrents - IPTorrents - #1 Private Tracker</title><meta http-equiv="Content-Type" content="text/html; charset=utf-8"></head><body class="Brtness">` +
	iptHeaderFixture + iptTableHead +
	`<tr><td class="i p72"><a href="?48"><img src="/img/cats/Movies-BluRayBDRip.png" alt="Movie/HD/Bluray"></a><td class="al"><a class=" hv" href="/t/9900001">Fixture Movie 2026 1080p AMZN WEB-DL DDP 5 1 H 264-FIXTURE</a> <span class="tag">New</span><div class="sub">2026  1080p | 2.8 minutes ago</div><td><a href="/t/9900001?bookmark" class="tTipWrap"><i class="fa fa-star fa-2x" alt="Bookmark"></i><div class="tTip ttop">Add to Bookmark</div></a><td><a href="/download.php/9900001/Fixture Movie 2026 1080p AMZN WEB-DL DDP 5 1 H 264-FIXTURE.torrent" class="tTipWrap"><i class="fa fa-download fa-2x grn"></i><div class="tTip ttop">Download Torrent</div></a><td><a href="/t/9900001?page=0#startcomments" class="tTipWrap">0<div class="tTip ttop">Go to comments</div></a><td>2.5 GB<td>0<td>1<td>14</tr>` +
	`<tr><td class="i p73"><a href="?99"><img src="/img/cats/TV-x265.png" alt="TV/x265"></a><td class="al"><a class=" hv" href="/t/9900002">Fixture Show S01 2160p NF WEB-DL DDP5 1 Atmos H 265-FIXTURE</a> <span class="tag">New</span> <span class="free">FreeLeech</span><div class="sub">7.2 2026 Action Crime Thriller 2160p | 3.2 hours ago</div><td><a href="/t/9900002?bookmark" class="tTipWrap"><i class="fa fa-star fa-2x" alt="Bookmark"></i><div class="tTip ttop">Add to Bookmark</div></a><td><a href="/download.php/9900002/Fixture Show S01 2160p NF WEB-DL DDP5 1 Atmos H 265-FIXTURE.torrent" class="tTipWrap"><i class="fa fa-download fa-2x grn"></i><div class="tTip ttop">Download Torrent</div></a><td><a href="/t/9900002?page=0#startcomments" class="tTipWrap">0<div class="tTip ttop">Go to comments</div></a><td>19.25 GB<td>26<td>25<td>4</tr>` +
	`<tr><td class="i p75"><a href="?80"><img src="/img/cats/Music-Flac.png" alt="Music/Flac"></a><td class="al"><a class=" hv" href="/t/9900003">Fixture Artist-Fixture Album-CD-FLAC-2026-FIXTURE</a> <span class="tag">New</span><div class="sub">1.5 weeks ago</div><td><a href="/t/9900003?bookmark" class="tTipWrap"><i class="fa fa-star fa-2x" alt="Bookmark"></i><div class="tTip ttop">Add to Bookmark</div></a><td><a href="/download.php/9900003/Fixture_Artist-Fixture_Album-CD-FLAC-2026-FIXTURE.torrent" class="tTipWrap"><i class="fa fa-download fa-2x grn"></i><div class="tTip ttop">Download Torrent</div></a><td><a href="/t/9900003?page=0#startcomments" class="tTipWrap">0<div class="tTip ttop">Go to comments</div></a><td>381 MB<td>2<td>3<td>1</tr>` +
	`<tr><td class="i p76"><a href="?95"><img alt="Educational" src="/img/cats/Educational.png"/></a><td class="al"><a class="hv" href="/t/9900004">Fixture Course 2026 BOOKWARE-FIXTURE</a><div class="sub">1.3 months ago</div><td><a class="tTipWrap" href="/t/9900004?bookmark"><i alt="Bookmark" class="fa fa-star fa-2x"></i><div class="tTip ttop">Add to Bookmark</div></a><td><a class="tTipWrap" href="/download.php/9900004/Fixture.Course.2026.BOOKWARE-FIXTURE.torrent"><i class="fa fa-download fa-2x grn"></i><div class="tTip ttop">Download Torrent</div></a><td><a class="tTipWrap" href="/t/9900004?page=0#startcomments">1<div class="tTip ttop">Go to comments</div></a><td>256 MB<td>1,045<td>16<td>0</tr>` +
	`</table><div class="pagination"><div class="paginate"><span class="page_nav"><a href="?q=fixture;p=2#torrents">2</a></span></div></div></body></html>`

// 搜不到时表格还在、tbody 是空的，下面另有一段「No results for …」。
const iptEmptySearchFixture = `<!DOCTYPE html><html class=""><head><title>Torrents - IPTorrents - #1 Private Tracker</title></head><body class="Brtness">` +
	iptHeaderFixture + iptTableHead +
	`</table><!-- A search that found nothing. ipt-noresults.css hides the empty table and the pagers that follow this block. --><section class="ipx nr" aria-labelledby="nr-title"><div class="ipx-card nr-card"><header class="nr-head"><div class="nr-headtext"><h1 class="nr-title" id="nr-title">No results for <span class="nr-q">zzzznotexist</span></h1><p class="nr-lead">Nothing matches those exact words.</p></div></header></div></section></body></html>`

// 没登录（或 Cookie 失效）时任何页面都跳到 /login.php。
const iptLoginFixture = `<html lang="en"><head><meta charset="UTF-8"><title>iPT – Sign In</title><link rel="shortcut icon" href="/favicon.ico" type="image/x-icon"></head><body><div class="container"><div class="login-card"><div class="card-header"><h1>Welcome Back</h1><p>Sign in to access your account</p></div><form action="do-login.php" method="post"><div class="input-group"><label for="username">Email or Username</label><div class="input-wrapper"><input type="text" id="username" name="username" placeholder="Enter your email or username" required="" autofocus=""></div></div><div class="input-group"><label for="password">Password</label><div class="input-wrapper"><input type="password" id="password" name="password" placeholder="Enter your password" required=""></div></div><button type="submit" class="submit-btn">Sign In</button></form><div class="links"><a href="/signup.php">Create Account</a><a href="/recover.php">Forgot Password?</a></div></div></div></body></html>`

// 免费种的详情页（/t/9900002 跳到的 /torrent.php?id=9900002）：免费标签在 div.tags，IMDb 链接在 #media。
const iptFreeDetailFixture = `<!DOCTYPE html><html class=""><head><title>Fixture Show S01 2160p NF WEB-DL DDP5 1 Atmos H 265-FIXTURE - IPTorrents - #1 Private Tracker</title></head><body class="Brtness">` +
	iptHeaderFixture +
	`<div class="dBox"><h2>Fixture Show S01 2160p NF WEB-DL DDP5 1 Atmos H 265-FIXTURE</h2><div class="info"><div><a href="download.php/9900002/Fixture%20Show%20S01%202160p%20NF%20WEB-DL%20DDP5%201%20Atmos%20H%20265-FIXTURE.torrent"><i class="fa fa-download dlBtn" title="Download" alt="Download"></i></a></div><div><table><tr><td><div><a class="peer"><span><i class="fa fa-angle-double-up grn"></i> 10</span><span><i class="fa fa-angle-double-down red"></i> 7</span></span></a></div><div class="sub">Size: 19.25 GB in <a href="/t/9900002/files">2 files</a></div><div class="sub">Uploaded: <span class="elapsedDate" title="Saturday, November 21, 2026 at 11:23am">7 minutes ago</span><span class="ipx tq"><a class="tq-tip" href="/tip.php?t=9900002" aria-label="Tip the uploader">Tip</a></span></div></td><td><a class="opt" href="/t/9900002?bookmark"><i class="fa fa-bookmark"></i>Bookmark</a><a class="opt" href="/issues/report?type=Torrent;id=9900002"><i class="fa fa-flag"></i>Report</a><a class="opt" href="/t/9900002?subscribe"><i class="fa fa-eye"></i>Subscribe</a></td></tr></table><div class="tags sub ipx tz"><span class="tz-g"><span class="tz-free">Freeleech</span><a class="tz-cat" href="/t?99">TV/x265</a></span><span class="tz-g"><a href="/t?q=2160p">2160p</a><a href="/t?q=WEB-DL">WEB-DL</a></span><span class="tz-g"><span class="tz-lab">Group</span><a href="/t?q=FIXTURE">FIXTURE</a></span><span class="tz-end"><a class="tz-all" href="/t?q=tt0999002" title="Find other formats / episodes">All releases</a></span></div></div></div>` +
	`<div id="media"><div><div class="main"><div class="tt">Fixture Show (2026)<div class="imdb-voters rbox" title="IMDb rating 7.2 / 10"><a class="rsrc rsrc-imdb" href="https://www.imdb.com/title/tt0999002/" target="_blank" rel="noreferrer nofollow">IMDb</a>7.2 / 10</div></div></div></div></div></div></body></html>`

// 不免费的详情页：没有 tz-free，没有 #media，上传时间是下午（3:32pm）。服务端原样输出，a.peer 后面多一个 </span>。
const iptDetailFixture = `<!DOCTYPE html><html class=""><head><title>Fixture Course 2026 BOOKWARE-FIXTURE - IPTorrents - #1 Private Tracker</title></head><body class="Brtness">` +
	iptHeaderFixture +
	`<div class="dBox"><h2>Fixture Course 2026 BOOKWARE-FIXTURE</h2><div class="info"><div><a href="download.php/9900004/Fixture.Course.2026.BOOKWARE-FIXTURE.torrent"><i class="fa fa-download dlBtn" title="Download" alt="Download"></i></a></div><div><table><tr><td><div><a class="peer"><span><i class="fa fa-angle-double-up grn"></i> 16</span><span><i class="fa fa-angle-double-down red"></i> 0</span></span></a></div><div class="sub">Size: 256 MB in <a href="/t/9900004/files">8 files</a></div><div class="sub">Uploaded: <span class="elapsedDate" title="Sunday, November 8, 2026 at 3:32pm">1.8 weeks ago</span><span class="ipx tq"><a class="tq-tip" href="/tip.php?t=9900004" aria-label="Tip the uploader">Tip</a></span></div></td><td><a class="opt" href="/t/9900004?bookmark"><i class="fa fa-bookmark"></i>Bookmark</a><a class="opt" href="/issues/report?type=Torrent;id=9900004"><i class="fa fa-flag"></i>Report</a></td></tr></table><div class="tags sub ipx tz"><span class="tz-g"><a class="tz-cat" href="/t?95">Educational</a></span><span class="tz-g"><span class="tz-lab">Group</span><a href="/t?q=FIXTURE">FIXTURE</a></span></div></div></div></div></body></html>`

// 种子不存在时 /t/{id} 直接回 404，页面上只有页头和「404 - Not Found」。
const iptNotFoundFixture = `<!DOCTYPE html><html class=""><head><title>IPTorrents - #1 Private Tracker</title></head><body class="Brtness">` +
	iptHeaderFixture + `<table id="body"><tr><td><h1>404 - Not Found</h1></td></tr></table></body></html>`

// 积分页：余额与「Earning now」。
const iptBonusFixture = `<!DOCTYPE html><html class=""><head><title>Bonus Points - IPTorrents - #1 Private Tracker</title></head><body class="Brtness">` +
	iptHeaderFixture +
	`<div class="ipx bx">
<header class="bx-hero">
<h1 class="bx-title">Bonus points</h1>
<p class="bx-lead">You earn these by seeding. Spend them on upload credit and on making your name your own.</p>
</header>
<section class="ipx-card bx-summary" aria-label="Your points">
<div class="bx-balance">
<span class="bx-balance__label">Your balance</span>
<b class="bx-balance__n">1,234.5</b>
<span class="bx-balance__label">points</span>
</div>
<div class="ipx-stat">
<span class="ipx-stat__label">Earning now</span>
<span class="ipx-stat__value">+2.6 an hour</span>
<span class="ipx-stat__sub">62.4 points a day</span>
</div>
<div class="ipx-stat">
<span class="ipx-stat__label">Torrents counted</span>
<span class="ipx-stat__value">13 of 15</span>
<span class="ipx-stat__sub">12 seeding, 15 is the most that count</span>
</div>
</section>
</div></body></html>`

// 资料页（2026-11 改版的 upf-* 布局）：精确到字节的上传下载在 .ipx-stat__sub，加入时间与最近访问是 <time datetime>。
const iptProfileFixture = `<!DOCTYPE html><html class=""><head><title>fixture_user's profile - IPTorrents - #1 Private Tracker</title></head><body class="Brtness">` +
	iptHeaderFixture +
	`<div class="ipx upf" id="upf">
	<header class="ipx-card upf-hero">
		<div class="upf-avatar upf-avatar--empty" data-initial="F"></div>
		<div class="upf-id">
			<h1 class="upf-name ">fixture_user</h1>
			<div class="upf-tags">
				<span class="ipx-pill upf-class" style="--c:var(--ipx-text-2)">Power User</span>
			</div>
			<p class="upf-meta">
				<span>Joined <time datetime="2024-03-15T08:30:00+00:00" title="2024-03-15 08:30 UTC">15 Mar 2024</time> <span class="ipx-muted">&middot; 2.7 years ago</span></span>
				<span>Last seen <time datetime="2026-11-20T10:15:30+00:00" title="2026-11-20 10:15 UTC">1 day ago</time></span>
			</p>
		</div>
	</header>
	<section class="upf-stats" aria-label="Transfer">
		<div class="ipx-card ipx-stat upf-stat upf-stat--up">
			<span class="ipx-stat__label">Uploaded</span>
			<span class="ipx-stat__value">1.00 TB</span>
			<span class="ipx-stat__sub">1,099,511,627,776 B</span>
		</div>
		<div class="ipx-card ipx-stat upf-stat upf-stat--down">
			<span class="ipx-stat__label">Downloaded</span>
			<span class="ipx-stat__value">50.00 GB</span>
			<span class="ipx-stat__sub">53,687,091,200 B</span>
		</div>
		<div class="ipx-card ipx-stat upf-stat upf-stat--bal">
			<span class="ipx-stat__label">Balance</span>
			<span class="ipx-stat__value">974.00 GB</span>
			<span class="ipx-stat__sub">1,045,824,536,576 B</span>
		</div>
		<div class="ipx-card ipx-stat upf-stat upf-stat--ratio" data-tone="">
			<span class="ipx-stat__label">Share ratio</span>
			<span class="ipx-stat__value">20.48</span>
		</div>
	</section>
	<div class="upf-grid"><div class="upf-main"><section class="ipx-card">
		<dl class="ipx-kv">
			<dt>Torrents</dt>
			<dd><a class="ipx-num" href="/t?u=1234567"><b>2</b> uploaded</a></dd>
			<dt>Peers now</dt>
			<dd class="upf-peers">
				<a href="/peers?u=1234567"><span class="ipx-good"></span><b class="ipx-num">12</b> seeding</a>
				<a href="/peers?u=1234567"><span class="ipx-bad"></span><b class="ipx-num">3</b> leeching</a>
			</dd>
		</dl>
	</section></div></div>
</div></body></html>`

// 做种页顶上的统计：Seeding 的数目与体积、Leeching、上传下载速度、To seed。
const iptPeersFixture = `<!DOCTYPE html><html class=""><head><title>Peers - IPTorrents - #1 Private Tracker</title></head><body class="Brtness">` +
	iptHeaderFixture +
	`<div class="ipx pr" id="pr"><header class="pr-hero"><div class="pr-hero__text"><h1 class="pr-title">Active torrents</h1></div></header><dl class="pr-stats"><div><dt>Seeding</dt><dd>12</dd><dd class="pr-stats__sub">1.5 TB</dd></div><div><dt>Leeching</dt><dd>3</dd></div><div><dt>Uploading</dt><dd class="pr-speed">1.2 MB/s</dd></div><div><dt>Downloading</dt><dd class="pr-speed">0 B/s</dd></div><div><dt>To seed</dt><dd>7</dd><dd class="pr-stats__sub">Not yet 1:1 or 14 days</dd></div></dl></div></body></html>`

// iptFakeTorrent 是一份最小的种子文件（bencode 字典，带 info）。
const iptFakeTorrent = "d8:announce22:https://example.invalid4:infod6:lengthi1e4:name4:fake12:piece lengthi16384e6:pieces20:aaaaaaaaaaaaaaaaaaaaee"

// --- Helpers ---

type iptRecorded struct {
	path, query, cookie string
}

// iptServer 按路径回页面，记下每个请求的路径、查询串与 Cookie；没写的路径回 404。
func iptServer(t *testing.T, pages map[string]func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, func() []iptRecorded) {
	t.Helper()
	var mu sync.Mutex
	var got []iptRecorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, iptRecorded{path: r.URL.Path, query: r.URL.RawQuery, cookie: r.Header.Get("Cookie")})
		mu.Unlock()
		if h, ok := pages[r.URL.Path]; ok {
			h(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(iptNotFoundFixture))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []iptRecorded {
		mu.Lock()
		defer mu.Unlock()
		return append([]iptRecorded(nil), got...)
	}
}

func iptPage(t *testing.T, name, html string) func(w http.ResponseWriter, r *http.Request) {
	t.Helper()
	RequireNoSecrets(t, name, html)
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
	}
}

func iptStatus(code int, body string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}
}

// newIPTTestDriver 直接建驱动（固定「现在」），包上 BaseSite 走和生产一样的限速与重试。
func newIPTTestDriver(baseURL string) (*iptDriver, v2.Site) {
	d := newIPTDriver(iptDriverConfig{BaseURL: baseURL, Cookie: iptFixtureCookie, SiteID: "iptorrents", Now: func() time.Time { return iptFixtureNow }})
	return d, v2.NewBaseSite(d, v2.BaseSiteConfig{ID: "iptorrents", Name: "IPTorrents", Kind: v2.SiteIPTorrents, RateLimit: 100, RateBurst: 100})
}

// newIPTFactorySite 用站点定义的 CreateDriver 建站点（生产的工厂与凭据入口）。
func newIPTFactorySite(t *testing.T, baseURL string) v2.Site {
	t.Helper()
	opts, err := json.Marshal(v2.IPTorrentsOptions{Cookie: iptFixtureCookie})
	require.NoError(t, err)
	site, err := IPTorrentsDefinition.CreateDriver(v2.SiteConfig{
		ID: "iptorrents", Name: "IPTorrents", BaseURL: baseURL, Options: opts, RateLimit: 100, RateBurst: 100,
	}, zap.NewNop())
	require.NoError(t, err)
	return site
}

func iptNoPageDelay(t *testing.T) {
	t.Helper()
	old := iptorrentsPageDelay
	iptorrentsPageDelay = 0
	t.Cleanup(func() { iptorrentsPageDelay = old })
}

// --- Suite: Search ---

func testIPTorrentsSearch(t *testing.T) {
	srv, requests := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
		"/t": iptPage(t, "iptorrents_search", iptSearchFixture),
	})
	_, site := newIPTTestDriver(srv.URL)

	items, err := site.Search(context.Background(), v2.SearchQuery{Keyword: " fixture ", FreeOnly: true, Page: 2})
	require.NoError(t, err)
	require.Len(t, items, 4)

	reqs := requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "/t", reqs[0].path)
	q, err := url.ParseQuery(reqs[0].query)
	require.NoError(t, err)
	assert.Equal(t, "fixture", q.Get("q"))
	assert.Equal(t, "2", q.Get("p"), "页码从 1 起，第 2 页写 p=2")
	_, free := q["free"]
	assert.True(t, free, "只看免费写 free")
	assert.Equal(t, iptFixtureCookie, reqs[0].cookie)

	normal := items[0]
	assert.Equal(t, "9900001", normal.ID)
	assert.Equal(t, "Fixture Movie 2026 1080p AMZN WEB-DL DDP 5 1 H 264-FIXTURE", normal.Title)
	assert.Equal(t, "2026 1080p", normal.Subtitle, "标签部分合并空白，不带发布时间")
	assert.Equal(t, srv.URL+"/t/9900001", normal.URL)
	assert.Equal(t, "/api/site/iptorrents/torrent/9900001/download", normal.DownloadURL)
	assert.Equal(t, int64(5)<<29, normal.SizeBytes, "2.5 GB 按 1024 进位")
	assert.Equal(t, 0, normal.Snatched)
	assert.Equal(t, 1, normal.Seeders)
	assert.Equal(t, 14, normal.Leechers)
	assert.Equal(t, iptFixtureNow.Add(-168*time.Second).Unix(), normal.UploadedAt, "2.8 minutes ago")
	assert.Equal(t, "Movie/HD/Bluray", normal.Category)
	assert.Equal(t, v2.DiscountNone, normal.DiscountLevel)
	assert.True(t, normal.HasHR, "整站 H&R")
	assert.Equal(t, "iptorrents", normal.SourceSite)

	freeItem := items[1]
	assert.Equal(t, "9900002", freeItem.ID)
	assert.Equal(t, v2.DiscountFree, freeItem.DiscountLevel)
	assert.True(t, freeItem.DiscountEndTime.IsZero(), "站点不写免费的结束时间")
	assert.Equal(t, v2.FreeUnknown, freeItem.Promotion(iptFixtureNow).Validity, "免费但结束时间未知，不能当长期免费")
	assert.Equal(t, "7.2 2026 Action Crime Thriller 2160p", freeItem.Subtitle)
	assert.Equal(t, int64(77)<<28, freeItem.SizeBytes, "19.25 GB")
	assert.Equal(t, 26, freeItem.Snatched)
	assert.Equal(t, 25, freeItem.Seeders)
	assert.Equal(t, 4, freeItem.Leechers)
	assert.Equal(t, iptFixtureNow.Add(-11520*time.Second).Unix(), freeItem.UploadedAt, "3.2 hours ago")
	assert.Equal(t, "TV/x265", freeItem.Category)

	music := items[2]
	assert.Equal(t, "Fixture Artist-Fixture Album-CD-FLAC-2026-FIXTURE", music.Title)
	assert.Empty(t, music.Subtitle, "没有标签时整行都是发布时间")
	assert.Equal(t, int64(381)<<20, music.SizeBytes)
	assert.Equal(t, iptFixtureNow.Add(-252*time.Hour).Unix(), music.UploadedAt, "1.5 weeks = 252 小时")

	course := items[3]
	assert.Equal(t, "9900004", course.ID)
	assert.Equal(t, 1045, course.Snatched, "千分位逗号")
	assert.Equal(t, 16, course.Seeders)
	assert.Equal(t, iptFixtureNow.Add(-39*24*time.Hour).Unix(), course.UploadedAt, "1.3 months = 39 天（月按 30 天）")

	t.Run("EmptyResult", func(t *testing.T) {
		srv, _ := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"/t": iptPage(t, "iptorrents_search_empty", iptEmptySearchFixture),
		})
		_, site := newIPTTestDriver(srv.URL)
		items, err := site.Search(context.Background(), v2.SearchQuery{Keyword: "zzzznotexist"})
		require.NoError(t, err)
		assert.Empty(t, items)
	})

	t.Run("LoginPage", func(t *testing.T) {
		srv, _ := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"/t": iptPage(t, "iptorrents_login", iptLoginFixture),
		})
		_, site := newIPTTestDriver(srv.URL)
		_, err := site.Search(context.Background(), v2.SearchQuery{Keyword: "x"})
		require.Error(t, err)
		assert.ErrorIs(t, err, v2.ErrSessionExpired)
		assert.Contains(t, err.Error(), "iPT – Sign In")
	})

	t.Run("CloudflareChallenge", func(t *testing.T) {
		srv, _ := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"/t": func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Cf-Mitigated", "challenge")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`<html><head><title>Just a moment...</title></head><body><script src="/cdn-cgi/challenge-platform/h/b/orchestrate/chl_page/v1"></script></body></html>`))
			},
		})
		_, site := newIPTTestDriver(srv.URL)
		_, err := site.Search(context.Background(), v2.SearchQuery{Keyword: "x"})
		require.Error(t, err)
		assert.ErrorIs(t, err, v2.ErrCloudflareChallenge)
		assert.NotErrorIs(t, err, v2.ErrInvalidCredentials, "质询不是凭证无效")
	})

	t.Run("Forbidden", func(t *testing.T) {
		srv, _ := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"/t": iptStatus(http.StatusForbidden, "forbidden"),
		})
		_, site := newIPTTestDriver(srv.URL)
		_, err := site.Search(context.Background(), v2.SearchQuery{Keyword: "x"})
		assert.ErrorIs(t, err, v2.ErrInvalidCredentials)
	})

	t.Run("RateLimited", func(t *testing.T) {
		srv, _ := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"/t": iptStatus(http.StatusTooManyRequests, "slow down"),
		})
		_, site := newIPTTestDriver(srv.URL)
		_, err := site.Search(context.Background(), v2.SearchQuery{Keyword: "x"})
		assert.ErrorIs(t, err, v2.ErrRateLimited)
	})

	t.Run("NotTorrentList", func(t *testing.T) {
		// 登录着、却不是种子列表（站点改了页面、被跳到别的页）：不能当成没有结果
		srv, _ := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"/t": iptPage(t, "iptorrents_bonus", iptBonusFixture),
		})
		_, site := newIPTTestDriver(srv.URL)
		_, err := site.Search(context.Background(), v2.SearchQuery{Keyword: "x"})
		assert.ErrorIs(t, err, v2.ErrUnexpectedPage)
	})

	t.Run("RowsNotRecognised", func(t *testing.T) {
		broken := strings.ReplaceAll(iptSearchFixture, `href="/t/99`, `href="/torrent-gone/99`)
		srv, _ := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"/t": iptPage(t, "iptorrents_search_broken", broken),
		})
		_, site := newIPTTestDriver(srv.URL)
		_, err := site.Search(context.Background(), v2.SearchQuery{Keyword: "x"})
		assert.ErrorIs(t, err, v2.ErrParseError)
	})

	t.Run("PrepareSearch", func(t *testing.T) {
		d, _ := newIPTTestDriver("https://iptorrents.com/")
		req, err := d.PrepareSearch(v2.SearchQuery{Category: "72, 73,bogus;x", Page: 1})
		require.NoError(t, err)
		assert.Equal(t, "/t", req.Path)
		_, movies := req.Params["72"]
		_, tv := req.Params["73"]
		assert.True(t, movies)
		assert.True(t, tv)
		assert.Len(t, req.Params, 2, "非数字的分类不进参数，第 1 页不写 p，没写关键词不写 q")
	})
}

// --- Suite: Detail ---

func testIPTorrentsDetail(t *testing.T) {
	srv, requests := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
		"/t/9900002": iptPage(t, "iptorrents_detail_free", iptFreeDetailFixture),
		"/t/9900004": iptPage(t, "iptorrents_detail", iptDetailFixture),
	})
	d, site := newIPTTestDriver(srv.URL)
	provider, ok := site.(v2.DetailFetcherProvider)
	require.True(t, ok)
	fetcher := provider.GetDetailFetcher()
	require.NotNil(t, fetcher, "RSS 取详情要用到 TorrentDetailFetcher")

	item, err := fetcher.GetTorrentDetail(context.Background(), "", srv.URL+"/t/9900002", "")
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, "9900002", item.ID)
	assert.Equal(t, "Fixture Show S01 2160p NF WEB-DL DDP5 1 Atmos H 265-FIXTURE", item.Title)
	assert.Equal(t, int64(77)<<28, item.SizeBytes, "Size: 19.25 GB")
	assert.Equal(t, 10, item.Seeders)
	assert.Equal(t, 7, item.Leechers)
	assert.Equal(t, time.Date(2026, 11, 21, 11, 23, 0, 0, time.UTC).Unix(), item.UploadedAt)
	assert.Equal(t, v2.DiscountFree, item.DiscountLevel)
	assert.True(t, item.DiscountEndTime.IsZero())
	assert.Equal(t, v2.FreeUnknown, item.Promotion(iptFixtureNow).Validity)
	assert.True(t, item.HasHR)
	assert.Equal(t, "TV/x265", item.Category)
	assert.Equal(t, "tt0999002", item.IMDbID)
	assert.Equal(t, "iptorrents", item.SourceSite)
	assert.Equal(t, iptFixtureNow, item.FetchedAt)
	assert.Equal(t, "/api/site/iptorrents/torrent/9900002/download", item.DownloadURL)

	// 不免费、下午的上传时间、没有 IMDb
	plain, err := d.GetTorrentDetail(context.Background(), "9900004", "", "")
	require.NoError(t, err)
	assert.Equal(t, "Fixture Course 2026 BOOKWARE-FIXTURE", plain.Title)
	assert.Equal(t, v2.DiscountNone, plain.DiscountLevel)
	assert.Equal(t, int64(256)<<20, plain.SizeBytes)
	assert.Equal(t, 16, plain.Seeders)
	assert.Equal(t, 0, plain.Leechers)
	assert.Equal(t, time.Date(2026, 11, 8, 15, 32, 0, 0, time.UTC).Unix(), plain.UploadedAt, "3:32pm 是 15:32 UTC")
	assert.Equal(t, "Educational", plain.Category)
	assert.Empty(t, plain.IMDbID)

	// RSS 链接、推送前重新验证拼的 details.php?id=、下载地址都认得出编号
	for _, link := range []string{
		"https://iptorrents.com/torrent.php?id=9900004",
		"details.php?id=9900004",
		"https://iptorrents.com/download.php/9900004/Fixture.Course.2026.BOOKWARE-FIXTURE.torrent?torrent_pass=x",
		"/t/9900004?page=0#startcomments",
	} {
		got, err := d.GetTorrentDetail(context.Background(), "", link, "")
		require.NoError(t, err, link)
		assert.Equal(t, "9900004", got.ID, link)
	}
	for _, r := range requests() {
		assert.Contains(t, []string{"/t/9900002", "/t/9900004"}, r.path)
		assert.Equal(t, iptFixtureCookie, r.cookie)
	}

	t.Run("NotFound", func(t *testing.T) {
		_, err := d.GetTorrentDetail(context.Background(), "9900099", "", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, v2.ErrTorrentNotFound)
		assert.ErrorIs(t, err, v2.ErrTorrentDetailUnavailable)
	})

	t.Run("InvalidID", func(t *testing.T) {
		before := len(requests())
		_, err := d.GetTorrentDetail(context.Background(), "not-a-number", "https://iptorrents.com/rss?x=1", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, v2.ErrInvalidTorrentID)
		assert.ErrorIs(t, err, v2.ErrTorrentDetailUnavailable)
		assert.Len(t, requests(), before, "认不出编号时不发请求")
	})

	t.Run("LoginPage", func(t *testing.T) {
		srv, _ := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"/t/9900002": iptPage(t, "iptorrents_login", iptLoginFixture),
		})
		d, _ := newIPTTestDriver(srv.URL)
		_, err := d.GetTorrentDetail(context.Background(), "9900002", "", "")
		assert.ErrorIs(t, err, v2.ErrSessionExpired)
	})

	t.Run("NotDetailPage", func(t *testing.T) {
		srv, _ := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"/t/9900002": iptPage(t, "iptorrents_bonus", iptBonusFixture),
		})
		d, _ := newIPTTestDriver(srv.URL)
		_, err := d.GetTorrentDetail(context.Background(), "9900002", "", "")
		assert.ErrorIs(t, err, v2.ErrTorrentDetailUnavailable)
		assert.ErrorIs(t, err, v2.ErrParseError)
	})
}

// --- Suite: UserInfo ---

func testIPTorrentsUserInfo(t *testing.T) {
	iptNoPageDelay(t)
	srv, requests := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
		"/mybonus.php": iptPage(t, "iptorrents_bonus", iptBonusFixture),
		"/user.php":    iptPage(t, "iptorrents_profile", iptProfileFixture),
		"/peers":       iptPage(t, "iptorrents_peers", iptPeersFixture),
	})
	site := newIPTFactorySite(t, srv.URL)
	assert.Equal(t, v2.SiteIPTorrents, site.Kind())

	info, err := site.GetUserInfo(context.Background())
	require.NoError(t, err)

	reqs := requests()
	require.Len(t, reqs, 3)
	assert.Equal(t, iptRecorded{path: "/mybonus.php", cookie: iptFixtureCookie}, reqs[0])
	assert.Equal(t, iptRecorded{path: "/user.php", query: "u=1234567", cookie: iptFixtureCookie}, reqs[1])
	assert.Equal(t, iptRecorded{path: "/peers", query: "u=1234567", cookie: iptFixtureCookie}, reqs[2])

	assert.Equal(t, "iptorrents", info.Site)
	assert.Equal(t, "1234567", info.UserID)
	assert.Equal(t, "fixture_user", info.Username)
	assert.Equal(t, "Power User", info.LevelName)
	assert.Equal(t, "Power User", info.Rank)
	assert.Equal(t, int64(1)<<40, info.Uploaded, "1,099,511,627,776 B")
	assert.Equal(t, int64(50)<<30, info.Downloaded, "53,687,091,200 B")
	assert.InDelta(t, 20.48, info.Ratio, 1e-9)
	assert.InDelta(t, 1234.5, info.Bonus, 1e-9)
	assert.InDelta(t, 2.6, info.BonusPerHour, 1e-9)
	assert.Equal(t, time.Date(2024, 3, 15, 8, 30, 0, 0, time.UTC).Unix(), info.JoinDate)
	assert.Equal(t, time.Date(2026, 11, 20, 10, 15, 30, 0, time.UTC).Unix(), info.LastAccess, "资料页的 Last seen")
	assert.Zero(t, info.LastLogin)
	assert.Equal(t, 12, info.Seeding)
	assert.Equal(t, 12, info.SeederCount)
	assert.Equal(t, int64(3)<<39, info.SeederSize, "1.5 TB")
	assert.Equal(t, 3, info.Leeching)
	assert.Equal(t, 3, info.LeecherCount)
	assert.Equal(t, 7, info.HnRUnsatisfied, "To seed：还没做种到 1:1 或 14 天")
	assert.Equal(t, 2, info.Uploads)

	// 登录探测：用真实的分类与 HTTP 主通道判断，最近访问取站点记的时间，不按探测时间兜底
	clock := sitelogin.NewFakeClock(iptFixtureNow)
	res := sitelogin.ClassifyUserInfo(IPTorrentsDefinition, info, nil, clock)
	assert.Equal(t, sitelogin.OK, res.Status)
	require.NotNil(t, res.LastAccessAt)
	assert.Equal(t, time.Date(2026, 11, 20, 10, 15, 30, 0, time.UTC), *res.LastAccessAt)
	probe, err := sitelogin.HTTPTransport{}.FetchUserInfo(context.Background(), IPTorrentsDefinition, site, clock)
	require.NoError(t, err)
	assert.Equal(t, sitelogin.OK, probe.Status)
	assert.Equal(t, sitelogin.ProbeSourceHTTPCookie, probe.Source)

	t.Run("NoDownloadsIsInfiniteRatio", func(t *testing.T) {
		profile := strings.NewReplacer(
			`<span class="ipx-stat__value">50.00 GB</span>`, `<span class="ipx-stat__value">0.00 kB</span>`,
			`<span class="ipx-stat__sub">53,687,091,200 B</span>`, `<span class="ipx-stat__sub">0 B</span>`,
			`<span class="ipx-stat__value">20.48</span>`, `<span class="ipx-stat__value">&mdash;</span><span class="ipx-stat__sub">nothing downloaded yet</span>`,
		).Replace(iptProfileFixture)
		srv, _ := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"/mybonus.php": iptPage(t, "iptorrents_bonus", iptBonusFixture),
			"/user.php":    iptPage(t, "iptorrents_profile_nodl", profile),
			"/peers":       iptPage(t, "iptorrents_peers", iptPeersFixture),
		})
		info, err := newIPTFactorySite(t, srv.URL).GetUserInfo(context.Background())
		require.NoError(t, err)
		assert.Zero(t, info.Downloaded)
		assert.Equal(t, float64(v2.InfiniteRatio), info.Ratio)
	})

	t.Run("PeersPageDown", func(t *testing.T) {
		// 做种页一时打不开（503）：用户名与资料照样回，做种体积与未达标 H&R 数按没取到算
		srv, _ := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"/mybonus.php": iptPage(t, "iptorrents_bonus", iptBonusFixture),
			"/user.php":    iptPage(t, "iptorrents_profile", iptProfileFixture),
			"/peers":       iptStatus(http.StatusServiceUnavailable, "busy"),
		})
		d, _ := newIPTTestDriver(srv.URL)
		info, err := d.GetUserInfo(context.Background())
		var partial *v2.PartialUserInfoError
		require.ErrorAs(t, err, &partial)
		assert.ElementsMatch(t, []string{"seederSize", "hnrUnsatisfied"}, partial.Missing)
		assert.Equal(t, "fixture_user", info.Username)
		assert.Equal(t, 12, info.Seeding, "做种数目资料页上有")
		assert.NoError(t, v2.IgnorePartial(info, err))
		assert.Equal(t, sitelogin.OK, sitelogin.ClassifyUserInfo(IPTorrentsDefinition, info, err, sitelogin.NewFakeClock(iptFixtureNow)).Status)
	})

	t.Run("PeersPageLoggedOut", func(t *testing.T) {
		srv, _ := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"/mybonus.php": iptPage(t, "iptorrents_bonus", iptBonusFixture),
			"/user.php":    iptPage(t, "iptorrents_profile", iptProfileFixture),
			"/peers":       iptPage(t, "iptorrents_login", iptLoginFixture),
		})
		d, _ := newIPTTestDriver(srv.URL)
		_, err := d.GetUserInfo(context.Background())
		var partial *v2.PartialUserInfoError
		assert.False(t, errors.As(err, &partial), "登录失效不按「只取到一部分」算")
		assert.ErrorIs(t, err, v2.ErrSessionExpired)
	})

	t.Run("LoginPage", func(t *testing.T) {
		srv, reqs := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"/mybonus.php": iptPage(t, "iptorrents_login", iptLoginFixture),
		})
		site := newIPTFactorySite(t, srv.URL)
		_, err := site.GetUserInfo(context.Background())
		assert.ErrorIs(t, err, v2.ErrSessionExpired)
		assert.Len(t, reqs(), 1, "登录页之后不再取别的页")
		probe, perr := sitelogin.HTTPTransport{}.FetchUserInfo(context.Background(), IPTorrentsDefinition, site, sitelogin.NewFakeClock(iptFixtureNow))
		require.NoError(t, perr)
		assert.Equal(t, sitelogin.SESSION_EXPIRED, probe.Status)
	})

	t.Run("NoUserLink", func(t *testing.T) {
		page := strings.Replace(iptBonusFixture, `<a href="/u/1234567" class="tTipWrap uname">`, `<a href="/profile" class="tTipWrap">`, 1)
		srv, _ := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"/mybonus.php": iptPage(t, "iptorrents_bonus_nouser", page),
		})
		d, _ := newIPTTestDriver(srv.URL)
		_, err := d.GetUserInfo(context.Background())
		assert.ErrorIs(t, err, v2.ErrParseError)
	})

	t.Run("ProfileChanged", func(t *testing.T) {
		srv, _ := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"/mybonus.php": iptPage(t, "iptorrents_bonus", iptBonusFixture),
			"/user.php":    iptPage(t, "iptorrents_bonus_as_profile", iptBonusFixture),
		})
		d, _ := newIPTTestDriver(srv.URL)
		_, err := d.GetUserInfo(context.Background())
		assert.ErrorIs(t, err, v2.ErrParseError)
	})

	t.Run("Cancelled", func(t *testing.T) {
		old := iptorrentsPageDelay
		iptorrentsPageDelay = time.Hour
		t.Cleanup(func() { iptorrentsPageDelay = old })
		srv, _ := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"/mybonus.php": iptPage(t, "iptorrents_bonus", iptBonusFixture),
		})
		d, _ := newIPTTestDriver(srv.URL)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, err := d.GetUserInfo(ctx)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

// --- 下载 ---

func TestIPTorrentsDownload(t *testing.T) {
	const torrentPath = "/download.php/9900004/Fixture.Course.2026.BOOKWARE-FIXTURE.torrent"
	srv, requests := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
		"/t/9900004": iptPage(t, "iptorrents_detail", iptDetailFixture),
		torrentPath: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/x-bittorrent")
			_, _ = w.Write([]byte(iptFakeTorrent))
		},
	})
	_, site := newIPTTestDriver(srv.URL)

	data, err := site.Download(context.Background(), "9900004")
	require.NoError(t, err)
	assert.Equal(t, iptFakeTorrent, string(data))
	reqs := requests()
	require.Len(t, reqs, 2, "先打开详情页取下载链接，再下载种子文件")
	assert.Equal(t, "/t/9900004", reqs[0].path)
	assert.Equal(t, torrentPath, reqs[1].path, "详情页的相对链接按站点根目录补全")
	assert.Equal(t, iptFixtureCookie, reqs[1].cookie)

	t.Run("ParseDownloadWithoutContext", func(t *testing.T) {
		d, _ := newIPTTestDriver(srv.URL)
		req, err := d.PrepareDownload("9900004")
		require.NoError(t, err)
		res, err := d.Execute(context.Background(), req)
		require.NoError(t, err)
		data, err := d.ParseDownload(res)
		require.NoError(t, err)
		assert.Equal(t, iptFakeTorrent, string(data))
	})

	t.Run("LoginPageInsteadOfTorrent", func(t *testing.T) {
		srv, _ := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"/t/9900004": iptPage(t, "iptorrents_detail", iptDetailFixture),
			torrentPath:  iptPage(t, "iptorrents_login", iptLoginFixture),
		})
		_, other := newIPTTestDriver(srv.URL)
		_, err := other.Download(context.Background(), "9900004")
		require.Error(t, err)
		assert.ErrorIs(t, err, v2.ErrSessionExpired)
		assert.ErrorIs(t, err, v2.ErrNotTorrentFile, "登录页不能当种子")
	})

	t.Run("OtherPageInsteadOfTorrent", func(t *testing.T) {
		srv, _ := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"/t/9900004": iptPage(t, "iptorrents_detail", iptDetailFixture),
			torrentPath:  iptPage(t, "iptorrents_bonus", iptBonusFixture),
		})
		_, other := newIPTTestDriver(srv.URL)
		_, err := other.Download(context.Background(), "9900004")
		assert.ErrorIs(t, err, v2.ErrNotTorrentFile)
	})

	t.Run("TorrentGone", func(t *testing.T) {
		before := len(requests())
		_, err := site.Download(context.Background(), "9900099")
		assert.ErrorIs(t, err, v2.ErrTorrentNotFound)
		assert.Len(t, requests(), before+1, "详情页 404 就不再去下载")
	})

	t.Run("InvalidID", func(t *testing.T) {
		before := len(requests())
		_, err := site.Download(context.Background(), "../etc")
		assert.ErrorIs(t, err, v2.ErrInvalidTorrentID)
		assert.Len(t, requests(), before)
	})

	t.Run("NoDownloadLink", func(t *testing.T) {
		page := strings.Replace(iptDetailFixture, `<a href="download.php/9900004/Fixture.Course.2026.BOOKWARE-FIXTURE.torrent">`, `<a href="#">`, 1)
		srv, reqs := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"/t/9900004": iptPage(t, "iptorrents_detail_nolink", page),
		})
		_, other := newIPTTestDriver(srv.URL)
		_, err := other.Download(context.Background(), "9900004")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "没有下载链接")
		assert.Len(t, reqs(), 1)
	})

	t.Run("OffSiteLinkNotFetched", func(t *testing.T) {
		// 链接指向别的域名时不去请求（请求要带 Cookie）；.invalid 域名解析不了，真去请求也只会报网络错误
		page := strings.Replace(iptDetailFixture, `href="download.php/9900004/`, `href="https://elsewhere.invalid/download.php/9900004/`, 1)
		srv, reqs := iptServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"/t/9900004": iptPage(t, "iptorrents_detail_offsite", page),
		})
		_, other := newIPTTestDriver(srv.URL)
		_, err := other.Download(context.Background(), "9900004")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "不在站点的域名上")
		assert.NotErrorIs(t, err, v2.ErrNetworkError, "没有发出请求")
		assert.Len(t, reqs(), 1)
	})
}

// --- 定义、工厂、签到、H&R、等级 ---

func TestIPTorrentsDefinition(t *testing.T) {
	def, ok := v2.GetDefinitionRegistry().Get("iptorrents")
	require.True(t, ok)
	assert.Same(t, IPTorrentsDefinition, def)
	require.NoError(t, def.Validate())
	assert.Equal(t, v2.SchemaIPTorrents, def.Schema)
	assert.Equal(t, v2.AuthMethodCookie, def.AuthMethod)
	assert.Equal(t, []string{"https://iptorrents.com/"}, def.URLs)
	assert.Equal(t, "+0000", def.TimezoneOffset)
	assert.Empty(t, def.TrackerHosts, "tracker 域名还没核实，不登记")

	// 签到：站点没有每日签到，定义与实例都报不支持，也不会去请求
	supported, reason := v2.AttendanceSupport(def)
	assert.False(t, supported)
	assert.Contains(t, reason, "没有每日签到")
	site := newIPTFactorySite(t, "https://iptorrents.com/")
	capable, ok := site.(v2.AttendanceCapable)
	require.True(t, ok)
	assert.False(t, capable.SupportsAttendance())
	_, err := capable.Attend(context.Background())
	assert.ErrorIs(t, err, v2.ErrAttendanceUnsupported)

	// H&R：整站，所有种子都按 336 小时，站点设置里填的时长不覆盖定义
	assert.True(t, def.HREnabled)
	for _, size := range []int64{0, 1, 512 << 20, 1 << 40} {
		assert.Equal(t, 336, v2.ResolveHRSeedTimeH(def, size, 0), size)
		assert.Equal(t, 336, v2.ResolveHRSeedTimeH(def, size, 72), size)
	}

	// 等级：Power User 是 4 周、上传 50GB、下载 5GB、分享率 1.05 四条都要满足；VIP、Uploader 是特殊组
	levels := def.LevelRequirements
	require.Len(t, levels, 5)
	pu := levels[2]
	assert.Equal(t, "Power User", pu.Name)
	assert.Equal(t, "P4W", pu.Interval)
	assert.Equal(t, "50GB", pu.Uploaded)
	assert.Equal(t, "5GB", pu.Downloaded)
	assert.InDelta(t, 1.05, pu.Ratio, 1e-9)
	assert.Empty(t, pu.Alternative)
	for name, want := range map[string]int{"Leecher": 1, "User": 2, "Power User": 3, "VIP": 100, "Uploader": 200} {
		assert.Equal(t, want, v2.GuessUserLevelID(&v2.UserInfo{LevelName: name}, levels), name)
	}
}

func TestIPTorrentsFactory(t *testing.T) {
	_, err := IPTorrentsDefinition.CreateDriver(v2.SiteConfig{ID: "iptorrents", Options: json.RawMessage(`{}`)}, zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Cookie")

	_, err = IPTorrentsDefinition.CreateDriver(v2.SiteConfig{ID: "iptorrents", Options: json.RawMessage(`{bad`)}, zap.NewNop())
	require.Error(t, err)

	site, err := IPTorrentsDefinition.CreateDriver(v2.SiteConfig{
		ID: "iptorrents", Name: "IPTorrents", Options: json.RawMessage(`{"cookie":"uid=1; pass=FAKE_TEST_PASS"}`),
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "iptorrents", site.ID())
	assert.Equal(t, v2.SiteIPTorrents, site.Kind())
	base, ok := site.(*v2.BaseSite[iptRequest, iptResponse])
	require.True(t, ok)
	d, ok := base.GetDriver().(*iptDriver)
	require.True(t, ok)
	assert.Equal(t, "https://iptorrents.com", d.baseURL, "没给地址时用定义里的")
	assert.Equal(t, "iptorrents", d.siteID)

	// 生产的凭据入口：站点注册表按 Cookie 建实例
	reg, err := v2.NewSiteRegistry(nil).CreateSite("iptorrents", v2.SiteCredentials{Cookie: "uid=1; pass=FAKE_TEST_PASS"}, "https://iptorrents.com/")
	require.NoError(t, err)
	assert.Equal(t, v2.SiteIPTorrents, reg.Kind())
}

func TestIPTorrentsTimeAndSize(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := map[string]int64{
		"2.8 minutes ago":                     now.Add(-168 * time.Second).Unix(),
		"1 minute ago":                        now.Add(-time.Minute).Unix(),
		"1.1 hours ago":                       now.Add(-66 * time.Minute).Unix(),
		"2 days ago":                          now.Add(-48 * time.Hour).Unix(),
		"1.8 weeks ago":                       now.Add(-1088640 * time.Second).Unix(),
		"8.2 months ago":                      now.Add(-246 * 24 * time.Hour).Unix(),
		"1 year ago":                          now.Add(-365 * 24 * time.Hour).Unix(),
		"3.2 minutes ago by fixture_uploader": now.Add(-192 * time.Second).Unix(),
		"no time here":                        0,
		"":                                    0,
	}
	for in, want := range cases {
		assert.Equal(t, want, iptParseAgo(in, now), in)
	}

	sizes := map[string]int64{
		"256 MB":                    256 << 20,
		"2.5 GB":                    5 << 29,
		"1,099,511,627,776 B":       1 << 40,
		"0.00 kB":                   0,
		"0 B":                       0,
		"Size: 19.25 GB in 2 files": 77 << 28,
		"1.5 TB":                    3 << 39,
		"nothing":                   0,
	}
	for in, want := range sizes {
		assert.Equal(t, want, iptParseSize(in), in)
	}

	for link, want := range map[string]string{
		"https://iptorrents.com/t/123":                          "123",
		"/t/123?bookmark":                                       "123",
		"https://iptorrents.com/torrent.php?id=123":             "123",
		"details.php?id=123":                                    "123",
		"download.php/123/Name.torrent":                         "123",
		"https://iptorrents.com/download.php/123/A%20B.torrent": "123",
		"https://iptorrents.com/t?q=123":                        "",
		"https://iptorrents.com/torrent.php?id=abc":             "",
		"": "",
	} {
		assert.Equal(t, want, iptTorrentIDFromLink(link), link)
	}
}

func TestIPTorrentsPageChecks(t *testing.T) {
	doc := func(html string) *goquery.Document {
		d, err := goquery.NewDocumentFromReader(strings.NewReader(html))
		require.NoError(t, err)
		return d
	}
	assert.True(t, iptIsLoginPage(doc(iptLoginFixture)), "登录页")
	assert.False(t, iptIsLoginPage(doc(iptSearchFixture)), "登录以后的列表页")
	// 有密码框、又没有退出登录表单的页面按登录页算；登录以后带密码框的页面（改密码这类）不算
	assert.True(t, iptIsLoginPage(doc(`<html><body><form action="/takelogin"><input type="password" name="p"></form></body></html>`)))
	assert.False(t, iptIsLoginPage(doc(`<html><body><form action="/lout.php" method="post"></form><input type="password" name="p"></body></html>`)))
	assert.True(t, iptIsLoginBody([]byte(iptLoginFixture)))
	assert.False(t, iptIsLoginBody([]byte(iptSearchFixture)))

	// 分享率：页面写的数优先；写「—」「Inf.」时按上传、下载算，没下载过有上传记成无穷
	assert.InDelta(t, 1.25, iptRatio("1.25", 5, 4), 1e-9)
	assert.InDelta(t, 2.5, iptRatio("—", 5, 2), 1e-9)
	assert.Equal(t, float64(v2.InfiniteRatio), iptRatio("Inf.", 5, 0))
	assert.Zero(t, iptRatio("—", 0, 0))
}
