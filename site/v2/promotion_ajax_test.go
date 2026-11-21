package v2

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 优惠状态由页面脚本另取的站点（HDWinG）：列表、详情页的 HTML 里只有 <span class="sp_state_placeholder" id="种子ID">，
// 页面脚本带着 <meta name="x-csrf"> 的令牌 POST /ajax_promotion.php（ids[] 与 csrf），按种子 ID 取回 sp_state 与 timeout。
// 下面的页面与返回按 HDWinG 2026-11-18 的页面脚本与渲染结果写，编号、令牌都是示例值。

const promoListPage = `<html><head><meta name="x-csrf" content="csrf-demo-token"></head><body>
<a href="logout.php?csrf=csrf-demo-token">退出</a>
<table class="torrents">
<tr><th>类型</th><th>标题</th><th>优惠</th></tr>
<tr><td class="t_cat"><img alt="电影" /></td><td class="t_name"><a href="details.php?id=101&amp;hit=1">Free.Movie.2026</a></td><td class="discount"><span class="sp_state_placeholder" id="101"></span></td></tr>
<tr><td class="t_cat"><img alt="电影" /></td><td class="t_name"><a href="details.php?id=102&amp;hit=1">Plain.Movie.2026</a></td><td class="discount"><span class="sp_state_placeholder" id="102"></span></td></tr>
</table></body></html>`

const promoDetailPage = `<html><head><meta name="x-csrf" content="csrf-demo-token"></head><body>
<a href="logout.php?csrf=csrf-demo-token">退出</a>
<h2 id="top">Free.Movie.2026 <span class="sp_state_placeholder" id="101"></span></h2>
<input type="hidden" name="torrent_name" value="Free.Movie.2026" /><input type="hidden" name="detail_torrent_id" value="101" />
<table><tr><td class="rowhead">其它版本</td><td><a href="details.php?id=900&amp;hit=1">Free.Movie.2026.2160p</a> <span class="sp_state_placeholder" id="900"></span></td></tr></table>
</body></html>`

const (
	promoFreeState    = `{"sp_state":" <img class=\"pro_free\" src=\"pic/trans.gif\" alt=\"Free\" />","timeout":"<span title=\"2026-12-07 11:45:28\">18天20时</span>"}`
	promoTwoUpState   = `{"sp_state":" <img class=\"pro_2up\" src=\"pic/trans.gif\" alt=\"2X\" />","timeout":"<span title=\"2026-11-18 20:15:54\">5时27分</span>"}`
	promoNoneState    = `{"sp_state":"","timeout":""}`
	promoListResponse = `{"status":200,"message":{"101":` + promoFreeState + `,"102":` + promoNoneState + `}}`
)

func promoDefinition() *SiteDefinition {
	def := makeMinimalNexusPHP("promo")
	def.PromotionAJAX = &PromotionAJAXConfig{Path: "/ajax_promotion.php"}
	def.Selectors = &SiteSelectors{
		TableRows:       "table.torrents tr:has(td.t_name)",
		Title:           "td.t_name a",
		TitleLink:       "td.t_name a",
		DiscountIcon:    "td.discount img[class*='pro_']",
		DiscountEndTime: "td.discount span[title]",
	}
	def.DetailParser = &DetailParserConfig{
		DiscountMapping:  map[string]DiscountLevel{"pro_free": DiscountFree, "pro_2up": Discount2xUp},
		DiscountSelector: "h2#top img[class*='pro_']",
		EndTimeSelector:  "h2#top span[title]",
	}
	return def
}

// promoServer 回页面与优惠状态，记下优惠状态的请求。
type promoServer struct {
	*httptest.Server
	mu       sync.Mutex
	calls    int
	ids      []string
	csrf     string
	header   http.Header
	method   string
	response string
	status   int
}

func newPromoServer(t *testing.T, page, response string) *promoServer {
	t.Helper()
	ps := &promoServer{response: response, status: http.StatusOK}
	mux := http.NewServeMux()
	serve := func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(page)) }
	mux.HandleFunc("/torrents.php", serve)
	mux.HandleFunc("/details.php", serve)
	mux.HandleFunc("/ajax_promotion.php", func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, r.ParseForm())
		ps.mu.Lock()
		ps.calls++
		ps.method = r.Method
		ps.ids = r.PostForm["ids[]"]
		ps.csrf = r.PostForm.Get("csrf")
		ps.header = r.Header.Clone()
		status, body := ps.status, ps.response
		ps.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	ps.Server = httptest.NewServer(mux)
	t.Cleanup(ps.Close)
	return ps
}

func (ps *promoServer) driver(def *SiteDefinition) *NexusPHPDriver {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: ps.URL, Cookie: "c_secure_pass=demo", Selectors: def.Selectors})
	d.SetSiteDefinition(def)
	return d
}

func (ps *promoServer) callCount() int {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.calls
}

func TestPromotionAJAX_SearchFillsPlaceholders(t *testing.T) {
	ps := newPromoServer(t, promoListPage, promoListResponse)
	d := ps.driver(promoDefinition())

	req, err := d.PrepareSearch(SearchQuery{Keyword: "Movie"})
	require.NoError(t, err)
	assert.True(t, req.FillPromotions, "定义写了 PromotionAJAX，搜索要补优惠状态")

	res, err := d.Execute(context.Background(), req)
	require.NoError(t, err)
	items, err := d.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "101", items[0].ID)
	assert.Equal(t, DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, time.Date(2026, 12, 7, 11, 45, 28, 0, CSTLocation).Unix(), items[0].DiscountEndTime.Unix(), "结束时间按北京时间")
	assert.Equal(t, DiscountNone, items[1].DiscountLevel)
	assert.True(t, items[1].DiscountEndTime.IsZero())

	// 一页只发一次，和页面脚本一样：POST 表单，ids[] 是页面上的每个种子，带着页面里的 csrf 令牌与 Cookie
	assert.Equal(t, 1, ps.callCount())
	assert.Equal(t, http.MethodPost, ps.method)
	assert.Equal(t, []string{"101", "102"}, ps.ids)
	assert.Equal(t, "csrf-demo-token", ps.csrf)
	assert.Equal(t, "XMLHttpRequest", ps.header.Get("X-Requested-With"))
	assert.Contains(t, ps.header.Get("Content-Type"), "application/x-www-form-urlencoded")
	assert.Equal(t, "c_secure_pass=demo", ps.header.Get("Cookie"))
	assert.True(t, strings.HasPrefix(ps.header.Get("Referer"), ps.URL+"/torrents.php"), ps.header.Get("Referer"))
}

// 走 BaseSite.Search（刷流、网页搜索、订阅都走这里）：免费列表 spstate=2 读到的种子要认得出免费
func TestPromotionAJAX_BaseSiteSearch(t *testing.T) {
	ps := newPromoServer(t, promoListPage, promoListResponse)
	site := NewBaseSite(ps.driver(promoDefinition()), BaseSiteConfig{ID: "promo", Name: "Promo", Kind: SiteNexusPHP, RateLimit: 100, RateBurst: 10})

	items, err := site.Search(context.Background(), SearchQuery{FreeOnly: true})
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, 1, ps.callCount())
}

func TestPromotionAJAX_GetTorrentDetail(t *testing.T) {
	ps := newPromoServer(t, promoDetailPage, `{"status":200,"message":{"101":`+promoFreeState+`,"900":`+promoTwoUpState+`}}`)
	d := ps.driver(promoDefinition())

	item, err := d.GetTorrentDetail(context.Background(), "101", ps.URL+"/details.php?id=101&hit=1", "")
	require.NoError(t, err)
	assert.Equal(t, "101", item.ID)
	assert.Equal(t, DiscountFree, item.DiscountLevel, "只看本种标题里的图标，不看其它版本的 2X")
	assert.Equal(t, time.Date(2026, 12, 7, 11, 45, 28, 0, CSTLocation).Unix(), item.DiscountEndTime.Unix())
	assert.Empty(t, item.DiscountErr)
	// 详情页上其它版本的占位也一起取（和页面脚本一样一次取完）
	assert.ElementsMatch(t, []string{"101", "900"}, ps.ids)
}

// 别的种子缺了不要紧；本种的优惠状态没取回来时不能当成「没有优惠」（RSS 会当成不是免费），按读不到详情报错
func TestPromotionAJAX_GetTorrentDetailOwnStateMissing(t *testing.T) {
	ps := newPromoServer(t, promoDetailPage, `{"status":200,"message":{"900":`+promoTwoUpState+`}}`)
	d := ps.driver(promoDefinition())

	item, err := d.GetTorrentDetail(context.Background(), "101", "", "")
	require.Error(t, err)
	assert.Nil(t, item)
	assert.ErrorIs(t, err, ErrTorrentDetailUnavailable)

	ps.mu.Lock()
	ps.response = `{"status":200,"message":{"101":` + promoFreeState + `}}`
	ps.mu.Unlock()
	item, err = d.GetTorrentDetail(context.Background(), "101", "", "")
	require.NoError(t, err, "其它版本缺了不影响本种")
	assert.Equal(t, DiscountFree, item.DiscountLevel)
}

// 站点没回优惠状态时，搜索整次失败，不把每个种子都报成「没有优惠」（刷流、RSS 会当成不是免费）
func TestPromotionAJAX_Failures(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		wantHTTP int
		wantText string
	}{
		{name: "业务状态不是 200", status: http.StatusOK, body: `{"status":403,"message":"bad csrf"}`, wantText: "403"},
		{name: "不是 JSON", status: http.StatusOK, body: `<html><body>出错了</body></html>`, wantText: "JSON"},
		{name: "message 格式不对", status: http.StatusOK, body: `{"status":200,"message":"oops"}`, wantText: "message"},
		{name: "HTTP 502", status: http.StatusBadGateway, body: `bad gateway`, wantHTTP: http.StatusBadGateway},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ps := newPromoServer(t, promoListPage, tc.body)
			ps.status = tc.status
			d := ps.driver(promoDefinition())
			req, err := d.PrepareSearch(SearchQuery{Keyword: "Movie"})
			require.NoError(t, err)

			_, err = d.Execute(context.Background(), req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "ajax_promotion.php")
			assert.NotContains(t, err.Error(), "csrf-demo-token", "错误里不写令牌")
			if tc.wantHTTP != 0 {
				var se *HTTPStatusError
				require.True(t, errors.As(err, &se), err.Error())
				assert.Equal(t, tc.wantHTTP, se.Code)
				assert.True(t, transientSiteError(err), "5xx 交给 BaseSite 重试")
			}
			if tc.wantText != "" {
				assert.Contains(t, err.Error(), tc.wantText)
			}
		})
	}
}

// 页面里没有 csrf 令牌：发不出页面脚本那样的请求，报错，不发请求
func TestPromotionAJAX_MissingCSRF(t *testing.T) {
	page := strings.Replace(promoListPage, `<meta name="x-csrf" content="csrf-demo-token">`, "", 1)
	page = strings.ReplaceAll(page, "?csrf=csrf-demo-token", "")
	ps := newPromoServer(t, page, promoListResponse)
	d := ps.driver(promoDefinition())
	req, err := d.PrepareSearch(SearchQuery{})
	require.NoError(t, err)

	_, err = d.Execute(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "csrf")
	assert.Zero(t, ps.callCount())
}

// 表单里的 csrf（搜索框里的隐藏域）也认：和 meta 里的是同一个令牌
func TestPromotionAJAX_CSRFFromHiddenInput(t *testing.T) {
	page := strings.Replace(promoListPage, `<meta name="x-csrf" content="csrf-demo-token">`, "", 1)
	page = strings.Replace(page, "<body>", `<body><form><input type="hidden" name="csrf" value="csrf-form-token" /></form>`, 1)
	ps := newPromoServer(t, page, promoListResponse)
	d := ps.driver(promoDefinition())
	req, err := d.PrepareSearch(SearchQuery{})
	require.NoError(t, err)

	res, err := d.Execute(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "csrf-form-token", ps.csrf)
	items, err := d.ParseSearch(res)
	require.NoError(t, err)
	assert.Equal(t, DiscountFree, items[0].DiscountLevel)
}

// 页面上没有占位（搜不到种子）时不发请求；message 是空数组（PHP 的空数组）时什么也不填，不报错
func TestPromotionAJAX_NothingToFill(t *testing.T) {
	empty := `<html><head><meta name="x-csrf" content="csrf-demo-token"></head><body><a href="logout.php">退出</a><table class="torrents"><tr><th>标题</th></tr></table></body></html>`
	ps := newPromoServer(t, empty, promoListResponse)
	d := ps.driver(promoDefinition())
	req, err := d.PrepareSearch(SearchQuery{Keyword: "nothing"})
	require.NoError(t, err)
	res, err := d.Execute(context.Background(), req)
	require.NoError(t, err)
	items, err := d.ParseSearch(res)
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.Zero(t, ps.callCount())

	ps2 := newPromoServer(t, promoListPage, `{"status":200,"message":[]}`)
	d2 := ps2.driver(promoDefinition())
	req2, err := d2.PrepareSearch(SearchQuery{})
	require.NoError(t, err)
	res2, err := d2.Execute(context.Background(), req2)
	require.NoError(t, err)
	items2, err := d2.ParseSearch(res2)
	require.NoError(t, err)
	require.Len(t, items2, 2)
	assert.Equal(t, DiscountNone, items2[0].DiscountLevel)
}

// 没写 PromotionAJAX 的站点照旧：不补、不发请求（占位留着，和以前一样）
func TestPromotionAJAX_NotConfigured(t *testing.T) {
	ps := newPromoServer(t, promoListPage, promoListResponse)
	def := promoDefinition()
	def.PromotionAJAX = nil
	d := ps.driver(def)

	req, err := d.PrepareSearch(SearchQuery{Keyword: "Movie"})
	require.NoError(t, err)
	assert.False(t, req.FillPromotions)
	res, err := d.Execute(context.Background(), req)
	require.NoError(t, err)
	items, err := d.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, DiscountNone, items[0].DiscountLevel)

	item, err := d.GetTorrentDetail(context.Background(), "101", "", "")
	require.NoError(t, err)
	assert.Equal(t, DiscountNone, item.DiscountLevel)
	assert.Zero(t, ps.callCount())
}

// 下载、取用户信息、字幕这些请求不补优惠状态（用不上，不多发请求）
func TestPromotionAJAX_OtherRequestsSkip(t *testing.T) {
	ps := newPromoServer(t, promoDetailPage, promoListResponse)
	d := ps.driver(promoDefinition())

	dl, err := d.PrepareDownload("101")
	require.NoError(t, err)
	assert.False(t, dl.FillPromotions)
	detail, err := d.PrepareDetail("101")
	require.NoError(t, err)
	assert.False(t, detail.FillPromotions, "PrepareDetail 也给字幕用")
	_, err = d.Execute(context.Background(), detail)
	require.NoError(t, err)
	assert.Zero(t, ps.callCount())
}

func TestValidate_PromotionAJAX(t *testing.T) {
	ok := makeMinimalNexusPHP("promo")
	ok.PromotionAJAX = &PromotionAJAXConfig{Path: "/ajax_promotion.php"}
	assert.NoError(t, ok.Validate())

	noPath := makeMinimalNexusPHP("promo")
	noPath.PromotionAJAX = &PromotionAJAXConfig{}
	err := noPath.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PromotionAJAX.Path")

	notNexus := &SiteDefinition{
		ID: "promo-mt", Name: "Promo", Schema: SchemaMTorrent, URLs: []string{"https://api.example.com/"},
		UserInfo: &UserInfoConfig{
			Process:   []UserInfoProcess{{RequestConfig: RequestConfig{URL: "/api/profile"}, Fields: []string{"id"}}},
			Selectors: map[string]FieldSelector{"id": {Selector: []string{"data.id"}}},
		},
		PromotionAJAX: &PromotionAJAXConfig{Path: "/ajax_promotion.php"},
	}
	err = notNexus.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PromotionAJAX")
}

// 取优惠状态时遇到的限流、Cloudflare 质询、连不上，错误链里带着对应的哨兵错误（BaseSite 的重试与界面按它们分类）
func TestPromotionAJAX_ErrorClassification(t *testing.T) {
	t.Run("429", func(t *testing.T) {
		ps := newPromoServer(t, promoListPage, `slow down`)
		ps.mu.Lock()
		ps.status = http.StatusTooManyRequests
		ps.mu.Unlock()
		d := ps.driver(promoDefinition())
		req, err := d.PrepareSearch(SearchQuery{})
		require.NoError(t, err)
		_, err = d.Execute(context.Background(), req)
		assert.ErrorIs(t, err, ErrRateLimited)
	})
	t.Run("Cloudflare 质询", func(t *testing.T) {
		ps := newPromoServer(t, promoListPage, `<html><script>window._cf_chl_opt={}</script></html>`)
		ps.mu.Lock()
		ps.status = http.StatusForbidden
		ps.mu.Unlock()
		d := ps.driver(promoDefinition())
		req, err := d.PrepareSearch(SearchQuery{})
		require.NoError(t, err)
		_, err = d.Execute(context.Background(), req)
		assert.ErrorIs(t, err, ErrCloudflareChallenge)
	})
	t.Run("连不上", func(t *testing.T) {
		mux := http.NewServeMux()
		srv := httptest.NewServer(mux)
		def := promoDefinition()
		// 优惠状态的地址指向一个已经关掉的服务器：页面取到了，补优惠状态时连不上
		closed := httptest.NewServer(http.NotFoundHandler())
		closedURL := closed.URL
		closed.Close()
		mux.HandleFunc("/torrents.php", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(promoListPage)) })
		t.Cleanup(srv.Close)
		d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: srv.URL, Cookie: "c=1", Selectors: def.Selectors})
		d.SetSiteDefinition(def)
		doc, err := d.Execute(context.Background(), NexusPHPRequest{Path: "/torrents.php", Method: http.MethodGet})
		require.NoError(t, err)
		err = d.fillPromotions(context.Background(), doc.Document, closedURL, srv.URL+"/torrents.php")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNetworkError)
		assert.True(t, transientSiteError(err), "连不上交给 BaseSite 重试")
	})
}
