package v2

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// parseDiscountFromElement — all discount branches + custom mapping
// ---------------------------------------------------------------------------

func discountElem(t *testing.T, class, src, alt string) *goquery.Selection {
	t.Helper()
	html := `<html><body><img class="` + class + `" src="` + src + `" alt="` + alt + `"></body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	require.NoError(t, err)
	return doc.Find("img")
}

func TestParseDiscountFromElement_Branches(t *testing.T) {
	assert.Equal(t, Discount2xFree, parseDiscountFromElement(discountElem(t, "pro_free2up", "", ""), nil))
	assert.Equal(t, DiscountFree, parseDiscountFromElement(discountElem(t, "pro_free", "", ""), nil))
	assert.Equal(t, DiscountPercent50, parseDiscountFromElement(discountElem(t, "pro_50pctdown", "", ""), nil))
	assert.Equal(t, DiscountPercent30, parseDiscountFromElement(discountElem(t, "pro_30pctdown", "", ""), nil))
	assert.Equal(t, DiscountPercent70, parseDiscountFromElement(discountElem(t, "pro_70pctdown", "", ""), nil))
	assert.Equal(t, Discount2xUp, parseDiscountFromElement(discountElem(t, "pro_2up", "", ""), nil))
	assert.Equal(t, DiscountNone, parseDiscountFromElement(discountElem(t, "normal", "", ""), nil))

	// custom mapping wins
	custom := map[string]DiscountLevel{"specialtag": Discount2x50}
	assert.Equal(t, Discount2x50, parseDiscountFromElement(discountElem(t, "specialtag", "", ""), custom))

	// match via src/alt
	assert.Equal(t, DiscountFree, parseDiscountFromElement(discountElem(t, "", "free.gif", ""), nil))
	assert.Equal(t, DiscountFree, parseDiscountFromElement(discountElem(t, "", "", "FREE"), nil))
}

// ---------------------------------------------------------------------------
// ParseDetail — subtitle, info hash, custom selector, form-action strategies
// ---------------------------------------------------------------------------

func TestNexusPHPDriver_ParseDetail_SubtitleAndHash(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	html := `<html><body><table>
		<tr><td class="rowhead">下载链接</td><td><a href="download.php?id=9&passkey=k">dl</a></td></tr>
		<tr><td class="rowhead">副标题</td><td>My Subtitle</td></tr>
		<tr><td class="no_border_wide">Hash码: 303a850dedc19e60bd7cc814f60e0e28d7f2c202</td></tr>
	</table></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	detail, err := d.ParseDetail(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Contains(t, detail.DownloadURL, "download.php")
	assert.Equal(t, "My Subtitle", detail.Subtitle)
	assert.Equal(t, "303a850dedc19e60bd7cc814f60e0e28d7f2c202", detail.InfoHash)
}

func TestNexusPHPDriver_ParseDetail_FormAction_Cov4(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	html := `<html><body>
		<form action="download.php?id=55"></form>
	</body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	detail, err := d.ParseDetail(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Contains(t, detail.DownloadURL, "download.php?id=55")
}

func TestNexusPHPDriver_ParseDetail_NilDoc(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	_, err := d.ParseDetail(NexusPHPResponse{})
	assert.ErrorIs(t, err, ErrParseError)
}

// ---------------------------------------------------------------------------
// base_site.go — Download error paths, GetUserInfo error
// ---------------------------------------------------------------------------

func TestBaseSite_Download_PrepareError(t *testing.T) {
	driver := &MockDriver{}
	site := NewBaseSite(driver, BaseSiteConfig{ID: "t", Name: "T", Kind: SiteNexusPHP, RateLimit: 100, RateBurst: 100, Logger: zap.NewNop()})
	driver.On("PrepareDownload", "1").Return("", errors.New("prep err"))
	_, err := site.Download(context.Background(), "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prepare download")
}

func TestBaseSite_Download_ExecuteError(t *testing.T) {
	driver := &MockDriver{}
	site := NewBaseSite(driver, BaseSiteConfig{ID: "t", Name: "T", Kind: SiteNexusPHP, RateLimit: 100, RateBurst: 100, Logger: zap.NewNop()})
	driver.On("PrepareDownload", "1").Return("req", nil)
	driver.On("Execute", mock.Anything, "req").Return("", errors.New("exec err"))
	_, err := site.Download(context.Background(), "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "execute download")
}

func TestBaseSite_Download_ParseError(t *testing.T) {
	driver := &MockDriver{}
	site := NewBaseSite(driver, BaseSiteConfig{ID: "t", Name: "T", Kind: SiteNexusPHP, RateLimit: 100, RateBurst: 100, Logger: zap.NewNop()})
	driver.On("PrepareDownload", "1").Return("req", nil)
	driver.On("Execute", mock.Anything, "req").Return("resp", nil)
	driver.On("ParseDownload", "resp").Return(nil, errors.New("parse err"))
	_, err := site.Download(context.Background(), "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse download")
}

func TestBaseSite_GetUserInfo_Error(t *testing.T) {
	driver := &MockDriver{}
	site := NewBaseSite(driver, BaseSiteConfig{ID: "t", Name: "T", Kind: SiteNexusPHP, RateLimit: 100, RateBurst: 100, Logger: zap.NewNop()})
	driver.On("GetUserInfo", mock.Anything).Return(UserInfo{}, errors.New("ui err"))
	_, err := site.GetUserInfo(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get user info")
}

// ---------------------------------------------------------------------------
// isLoginPage / is2FAPage — additional branches
// ---------------------------------------------------------------------------

func TestIsLoginPage_Branches(t *testing.T) {
	panel, _ := goquery.NewDocumentFromReader(strings.NewReader(`<div class="login-panel"></div>`))
	assert.True(t, isLoginPage(panel))

	title, _ := goquery.NewDocumentFromReader(strings.NewReader(`<title>登录</title><input name="username"><input name="password">`))
	assert.True(t, isLoginPage(title))

	meta, _ := goquery.NewDocumentFromReader(strings.NewReader(`<meta http-equiv="refresh" content="0;url=login.php">`))
	assert.True(t, isLoginPage(meta))

	normal, _ := goquery.NewDocumentFromReader(strings.NewReader(`<div>hello</div>`))
	assert.False(t, isLoginPage(normal))
}

func TestIs2FAPage_Branches(t *testing.T) {
	script, _ := goquery.NewDocumentFromReader(strings.NewReader(`<script>window.location='take2fa.php'</script>`))
	assert.True(t, is2FAPage(script))

	form, _ := goquery.NewDocumentFromReader(strings.NewReader(`<form action="/take2fa"></form>`))
	assert.True(t, is2FAPage(form))

	title, _ := goquery.NewDocumentFromReader(strings.NewReader(`<title>两步验证</title>`))
	assert.True(t, is2FAPage(title))

	normal, _ := goquery.NewDocumentFromReader(strings.NewReader(`<div>hi</div>`))
	assert.False(t, is2FAPage(normal))
}

// ---------------------------------------------------------------------------
// ParseUserInfo / ParseUserDetails — NexusPHP transfer row parsing
// ---------------------------------------------------------------------------

func TestNexusPHPDriver_ParseUserInfo_InfoBlock(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	html := `<html><body>
		<div id="info_block"><a class="User_Name" href="userdetails.php?id=88">Alice</a></div>
		<table><tr><td class="rowhead">上传量</td><td>2.00 TB</td></tr>
		<tr><td class="rowhead">下载量</td><td>1.00 TB</td></tr>
		<tr><td class="rowhead">分享率</td><td>2.00</td></tr>
		<tr><td class="rowhead">魔力值</td><td>5000</td></tr>
		<tr><td class="rowhead">等级</td><td>Power User</td></tr></table>
	</body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	info, err := d.ParseUserInfo(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Equal(t, "Alice", info.Username)
	assert.Equal(t, "88", info.UserID)
	assert.Greater(t, info.Uploaded, int64(0))
	assert.Greater(t, info.Downloaded, int64(0))
}

func TestNexusPHPDriver_ParseUserDetails_TransferRow_Cov4(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	html := `<html><body><table>
		<tr><td class="rowhead">用户名</td><td class="rowfollow">Bob</td></tr>
		<tr><td class="rowhead">传输</td><td class="rowfollow">上传量: 1.5 TB 下载量: 500 GB 分享率: 3.0</td></tr>
		<tr><td class="rowhead">魔力值</td><td class="rowfollow">123,456 (详情)</td></tr>
		<tr><td class="rowhead">等级</td><td class="rowfollow">Elite User</td></tr>
		<tr><td class="rowhead">上次访问</td><td class="rowfollow">2024-06-01 12:00:00</td></tr>
		<tr><td class="rowhead">上次登录</td><td class="rowfollow">2024-05-30 09:00:00</td></tr>
	</table></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	info, err := d.ParseUserDetails(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Greater(t, info.Uploaded, int64(0))
	assert.Greater(t, info.Downloaded, int64(0))
	assert.InDelta(t, 3.0, info.Ratio, 0.01)
	assert.Equal(t, "Elite User", info.Rank)
	assert.Greater(t, info.LastAccess, int64(0))
	assert.Greater(t, info.LastLogin, int64(0))
}

// ---------------------------------------------------------------------------
// unit3d_driver.go — ParseSearch full item mapping
// ---------------------------------------------------------------------------

func TestUnit3DDriver_ParseSearch_Full(t *testing.T) {
	d := NewUnit3DDriver(Unit3DDriverConfig{BaseURL: "https://u.example", APIKey: "k"})
	data := []byte(`[{"id":5,"name":"Film","info_hash":"abc","size":1024,"seeders":10,"leechers":2,
		"times_completed":50,"category":{"name":"Movies"},"freeleech":"100","double_upload":true,
		"created_at":"2024-06-01T12:00:00Z","freeleech_ends":"2024-07-01T12:00:00Z","download_link":"https://dl"}]`)
	items, err := d.ParseSearch(Unit3DResponse{Data: data})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "5", items[0].ID)
	assert.Equal(t, "Film", items[0].Title)
	assert.Equal(t, Discount2xFree, items[0].DiscountLevel)
	assert.Greater(t, items[0].UploadedAt, int64(0))
	assert.False(t, items[0].DiscountEndTime.IsZero())
}

func TestUnit3DDriver_ParseSearch_BadJSON(t *testing.T) {
	d := NewUnit3DDriver(Unit3DDriverConfig{BaseURL: "https://u.example", APIKey: "k"})
	_, err := d.ParseSearch(Unit3DResponse{Data: []byte("notjson")})
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// driver_registry.go — CreateSiteFromDefinition custom driver + no factory
// ---------------------------------------------------------------------------

func TestCreateSiteFromDefinition_CustomDriver(t *testing.T) {
	called := false
	def := &SiteDefinition{
		ID:     "customdef",
		Schema: SchemaRousi,
		CreateDriver: func(config SiteConfig, logger *zap.Logger) (Site, error) {
			called = true
			return nil, errors.New("custom invoked")
		},
	}
	_, err := CreateSiteFromDefinition(def, SiteConfig{ID: "customdef"}, zap.NewNop())
	require.Error(t, err)
	assert.True(t, called)
}

func TestCreateSiteFromDefinition_NoFactory(t *testing.T) {
	def := &SiteDefinition{ID: "x", Schema: Schema("nonexistent-schema")}
	_, err := CreateSiteFromDefinition(def, SiteConfig{ID: "x"}, zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no driver registered")
}

func TestCreateSiteFromDefinition_FactoryDispatch(t *testing.T) {
	def := &SiteDefinition{ID: "hdsky", Schema: SchemaNexusPHP}
	opts, _ := json.Marshal(NexusPHPOptions{Cookie: "c=1"})
	site, err := CreateSiteFromDefinition(def, SiteConfig{ID: "hdsky", Name: "HDSky", BaseURL: "https://hdsky.me", Options: opts}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, site)
}

// ---------------------------------------------------------------------------
// failover.go — GetCurrentURL empty, GetCurrentBaseURL
// ---------------------------------------------------------------------------

func TestURLFailoverManager_GetCurrentURL(t *testing.T) {
	m := NewURLFailoverManager(URLFailoverConfig{BaseURLs: []string{"http://a", "http://b"}}, nil)
	assert.Equal(t, "http://a", m.GetCurrentURL())

	empty := NewURLFailoverManager(URLFailoverConfig{}, nil)
	assert.Equal(t, "", empty.GetCurrentURL())
}

func TestFailoverHTTPClient_GetCurrentBaseURL(t *testing.T) {
	c := NewFailoverHTTPClient(URLFailoverConfig{BaseURLs: []string{"http://z"}, Timeout: time.Second})
	assert.Equal(t, "http://z", c.GetCurrentBaseURL())
}

// ---------------------------------------------------------------------------
// http_client.go — RequestsClient doWithRetry max exceeded
// ---------------------------------------------------------------------------

func TestRequestsClient_DoWithRetry_MaxExceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cfg := DefaultRetryConfig()
	cfg.MaxRetries = 1
	cfg.InitialBackoff = 5 * time.Millisecond
	cfg.MaxBackoff = 10 * time.Millisecond
	client := NewRequestsClient(DefaultHTTPClientConfig(), cfg, nil)
	defer client.Close()

	resp, err := client.Get(context.Background(), server.URL)
	// Retryable status returns the last response and an error after retries exhausted
	require.Error(t, err)
	if resp != nil {
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode())
	}
}

func TestRequestsClient_DoWithRetry_NetworkError(t *testing.T) {
	cfg := DefaultRetryConfig()
	cfg.MaxRetries = 1
	cfg.InitialBackoff = 5 * time.Millisecond
	cfg.MaxBackoff = 10 * time.Millisecond
	client := NewRequestsClient(DefaultHTTPClientConfig(), cfg, nil)
	defer client.Close()

	_, err := client.Get(context.Background(), "http://127.0.0.1:1")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// NewSizeTieredHRCalc — no-match fallback returns max
// ---------------------------------------------------------------------------

func TestNewSizeTieredHRCalc_NoMatchReturnsMax(t *testing.T) {
	rules := []HRSeedTimeRule{
		{MinSizeGB: 100, MaxSizeGB: 200, SeedTimeH: 48},
	}
	calc := NewSizeTieredHRCalc(rules, 10)
	// 5 GiB matches no rule -> max (48+10)
	assert.Equal(t, 58, calc(5*1024*1024*1024))
}
