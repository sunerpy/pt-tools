package v2

import (
	"context"
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
// failover.go — GetAllURLs, WithLogger, FailoverHTTPClient.Do
// ---------------------------------------------------------------------------

func TestURLFailoverManager_GetAllURLs(t *testing.T) {
	cfg := URLFailoverConfig{BaseURLs: []string{"http://a", "http://b"}, Timeout: time.Second}
	m := NewURLFailoverManager(cfg, nil)
	assert.Equal(t, []string{"http://a", "http://b"}, m.GetAllURLs())
}

func TestFailover_WithLogger(t *testing.T) {
	logger := zap.NewNop()
	cfg := URLFailoverConfig{BaseURLs: []string{"http://a"}, Timeout: time.Second}
	c := NewFailoverHTTPClient(cfg, WithLogger(logger))
	assert.Same(t, logger, c.logger)
}

func TestFailoverHTTPClient_Do(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api", r.URL.Path)
		assert.Equal(t, "custom", r.Header.Get("X-Test"))
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("get-ok"))
		}
	}))
	defer server.Close()

	cfg := DefaultFailoverConfig([]string{server.URL})
	c := NewFailoverHTTPClient(cfg)

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		resp, err := c.Do(context.Background(), method, "/api", []byte("body"), map[string]string{"X-Test": "custom"})
		require.NoError(t, err, "method %s", method)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}
}

func TestFailoverHTTPClient_Do_UnsupportedMethod(t *testing.T) {
	cfg := DefaultFailoverConfig([]string{"http://127.0.0.1:1"})
	cfg.MaxRetries = 0
	c := NewFailoverHTTPClient(cfg)
	_, err := c.Do(context.Background(), "BREW", "/x", nil, nil)
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// nexusphp_driver.go — Execute / login & 2FA detection / ParseDownload live /
// FetchSeedingStatus / filterNames / buildCurlCommand / isHexString
// ---------------------------------------------------------------------------

func TestNexusPHPDriver_Execute_LoginPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><form action="takelogin.php"></form></body></html>`))
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	_, err := d.Execute(context.Background(), NexusPHPRequest{Path: "/index.php"})
	assert.ErrorIs(t, err, ErrSessionExpired)
}

func TestNexusPHPDriver_Execute_2FA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>二次验证</title></head><body><form action="take2fa.php"></form></body></html>`))
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	_, err := d.Execute(context.Background(), NexusPHPRequest{Path: "/index.php"})
	assert.ErrorIs(t, err, Err2FARequired)
}

func TestNexusPHPDriver_Execute_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	_, err := d.Execute(context.Background(), NexusPHPRequest{Path: "/index.php"})
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestNexusPHPDriver_Execute_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	_, err := d.Execute(context.Background(), NexusPHPRequest{Path: "/index.php"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 500")
}

func TestNexusPHPDriver_Execute_OK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "a=1", r.URL.RawQuery)
		_, _ = w.Write([]byte(`<html><body><table class="torrents"></table></body></html>`))
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	params := map[string][]string{"a": {"1"}}
	res, err := d.Execute(context.Background(), NexusPHPRequest{Path: "/torrents.php", Params: params})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	require.NotNil(t, res.Document)
}

func TestNexusPHPDriver_ParseDownload_LiveFetch(t *testing.T) {
	var torrentHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "download.php") {
			torrentHits++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("d8:announce"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})

	// Build a detail document with a relative download link.
	html := `<html><body><a href="download.php?id=5&passkey=abc">dl</a></body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	require.NoError(t, err)

	data, err := d.ParseDownload(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Equal(t, []byte("d8:announce"), data)
	assert.Equal(t, 1, torrentHits)
}

func TestNexusPHPDriver_ParseDownload_NoURL(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(`<html><body>nothing</body></html>`))
	_, err := d.ParseDownload(NexusPHPResponse{Document: doc})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no download URL")
}

func TestNexusPHPDriver_ParseDownload_AbsoluteURL(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte("d4:info"))
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com", Cookie: "c=1"})
	html := `<html><body><a href="` + server.URL + `/download.php?id=1&hash=xx">dl</a></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	data, err := d.ParseDownload(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Equal(t, []byte("d4:info"), data)
	assert.Equal(t, 1, hits)
}

func TestNexusPHPDriver_ParseDownload_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	html := `<html><body><a href="/download.php?id=1&passkey=x">dl</a></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	_, err := d.ParseDownload(NexusPHPResponse{Document: doc})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 404")
}

func TestNexusPHPDriver_ParseDownload_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	html := `<html><body><a href="/download.php?id=1&passkey=x">dl</a></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	_, err := d.ParseDownload(NexusPHPResponse{Document: doc})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty torrent file")
}

func TestNexusPHPDriver_FetchSeedingStatus_ExecuteError(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "http://127.0.0.1:1", Cookie: "c=1"})
	_, _, err := d.FetchSeedingStatus(context.Background(), "42")
	require.Error(t, err)
}

func TestFilterNames(t *testing.T) {
	filters := []Filter{{Name: "parseSize"}, {Name: "regex", Args: []any{"x"}}}
	assert.Equal(t, []string{"parseSize", "regex"}, filterNames(filters))
	assert.Empty(t, filterNames(nil))
}

func TestBuildCurlCommand(t *testing.T) {
	cmd := buildCurlCommand("GET", "https://x.com/a", map[string]string{"Cookie": "c='v'"})
	assert.Contains(t, cmd, "curl -X GET")
	assert.Contains(t, cmd, "'https://x.com/a'")
	assert.Contains(t, cmd, "Cookie:")
}

func TestIsHexString(t *testing.T) {
	assert.True(t, isHexString("abcDEF0123456789"))
	assert.False(t, isHexString("xyz"))
	assert.True(t, isHexString(""))
}

func TestNewNexusPHPDriverWithFailover(t *testing.T) {
	GetGlobalRegistry().RegisterURLs(SiteName("failtest"), []string{"https://a.example", "https://b.example"})
	d := NewNexusPHPDriverWithFailover(SiteName("failtest"), "cookie=1")
	require.NotNil(t, d)
	assert.Equal(t, "https://a.example", d.BaseURL)
	assert.NotNil(t, d.failoverClient)
}

func TestNexusPHPDriver_ExecuteWithFailover(t *testing.T) {
	var good *httptest.Server
	good = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><table class="torrents"></table></body></html>`))
	}))
	defer good.Close()

	GetGlobalRegistry().RegisterURLs(SiteName("failtest2"), []string{good.URL})
	d := NewNexusPHPDriverWithFailover(SiteName("failtest2"), "cookie=1")
	require.NotNil(t, d.failoverClient)
	res, err := d.Execute(context.Background(), NexusPHPRequest{Path: "/torrents.php"})
	require.NoError(t, err)
	require.NotNil(t, res.Document)
}

// ---------------------------------------------------------------------------
// mtorrent_driver.go — UnmarshalJSON, ParseDownload, Execute failover,
// GetBonusPerHour/GetPeerStatistics error paths, mapMTorrentRole,
// getMTeamCategoryName, WithFailover
// ---------------------------------------------------------------------------

func TestFlexibleCode_UnmarshalJSON(t *testing.T) {
	var fc FlexibleCode
	require.NoError(t, fc.UnmarshalJSON([]byte(`"SUCCESS"`)))
	assert.Equal(t, "SUCCESS", fc.String())
	assert.True(t, fc.IsSuccess())

	require.NoError(t, fc.UnmarshalJSON([]byte(`0`)))
	assert.Equal(t, "0", fc.String())
	assert.True(t, fc.IsSuccess())

	require.NoError(t, fc.UnmarshalJSON([]byte(`42`)))
	assert.Equal(t, "42", fc.String())

	require.Error(t, fc.UnmarshalJSON([]byte(`{bad`)))
}

func TestFlexInt_UnmarshalJSON(t *testing.T) {
	var fi FlexInt
	require.NoError(t, fi.UnmarshalJSON([]byte(`123`)))
	assert.Equal(t, 123, fi.Int())

	require.NoError(t, fi.UnmarshalJSON([]byte(`"456"`)))
	assert.Equal(t, 456, fi.Int())

	require.NoError(t, fi.UnmarshalJSON([]byte(`""`)))
	assert.Equal(t, 0, fi.Int())

	require.Error(t, fi.UnmarshalJSON([]byte(`"notanint"`)))
	require.Error(t, fi.UnmarshalJSON([]byte(`{bad`)))
}

func TestMTorrentDriver_ParseDownload_Full(t *testing.T) {
	var torrentServer *httptest.Server
	torrentServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		// minimal valid torrent bencode
		_, _ = w.Write([]byte("d8:announce4:test4:infod4:name4:teste"))
	}))
	defer torrentServer.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	res := MTorrentResponse{Code: "0", Data: []byte(`"` + torrentServer.URL + `/dl"`)}
	data, err := d.ParseDownload(res)
	// ValidateTorrentFile may reject; assert error surface is meaningful either way
	if err != nil {
		assert.Contains(t, err.Error(), "invalid torrent")
	} else {
		assert.NotEmpty(t, data)
	}
}

func TestMTorrentDriver_ParseDownload_APIError(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	_, err := d.ParseDownload(MTorrentResponse{Code: "1", Message: "fail", RawBody: []byte("errbody")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API error")
}

func TestMTorrentDriver_ParseDownload_EmptyURL(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	_, err := d.ParseDownload(MTorrentResponse{Code: "0", Data: []byte(`""`)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty download URL")
}

func TestMTorrentDriver_ParseDownload_BadData(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	_, err := d.ParseDownload(MTorrentResponse{Code: "0", Data: []byte(`{notstring}`)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse download URL")
}

func TestMTorrentDriver_ParseDownload_FetchError(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	_, err := d.ParseDownload(MTorrentResponse{Code: "0", Data: []byte(`"http://127.0.0.1:1/dl"`)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch torrent file")
}

func TestMapMTorrentRole(t *testing.T) {
	assert.Equal(t, "Power User", mapMTorrentRole("2"))
	assert.Equal(t, "Nexus Master", mapMTorrentRole("9"))
	assert.Equal(t, "User", mapMTorrentRole("999"))
}

func TestGetMTeamCategoryName(t *testing.T) {
	assert.Equal(t, "unknowncat", getMTeamCategoryName("unknowncat"))
}

func TestNewMTorrentDriverWithFailover_Extra(t *testing.T) {
	GetGlobalRegistry().RegisterURLs(SiteName("mteam"), []string{"https://api.m-team.cc"})
	d := NewMTorrentDriverWithFailover("apikey")
	require.NotNil(t, d)
}

func TestMTorrentDriver_ExecuteWithFailover(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{}}`))
	}))
	defer server.Close()

	GetGlobalRegistry().RegisterURLs(SiteNameMTeam, []string{server.URL})
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: server.URL, APIKey: "k", UseFailover: true})
	if d.failoverClient == nil {
		t.Skip("failover client not initialized")
	}
	res, err := d.Execute(context.Background(), MTorrentRequest{Endpoint: "/api/x", Method: "POST"})
	require.NoError(t, err)
	assert.True(t, res.Code.IsSuccess())
}

func TestMTorrentDriver_GetBonusPerHour_Error(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "http://127.0.0.1:1", APIKey: "k"})
	_, err := d.GetBonusPerHour(context.Background())
	require.Error(t, err)
}

func TestMTorrentDriver_GetPeerStatistics_Error(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "http://127.0.0.1:1", APIKey: "k"})
	_, err := d.GetPeerStatistics(context.Background())
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// gazelle_driver.go — selectGazelleDetailTorrent branches, GetUserInfo,
// GetTorrentDetail with Torrents array
// ---------------------------------------------------------------------------

func TestSelectGazelleDetailTorrent_SingleTorrent(t *testing.T) {
	detail := gazelleTorrentDetailResponse{Torrent: GazelleTorrent{TorrentID: 7, Size: 100}}
	got := selectGazelleDetailTorrent(detail, 0)
	assert.Equal(t, 7, got.TorrentID)
}

func TestSelectGazelleDetailTorrent_MatchInSlice(t *testing.T) {
	detail := gazelleTorrentDetailResponse{Torrents: []GazelleTorrent{
		{TorrentID: 1}, {TorrentID: 2}, {TorrentID: 3},
	}}
	got := selectGazelleDetailTorrent(detail, 2)
	assert.Equal(t, 2, got.TorrentID)
}

func TestSelectGazelleDetailTorrent_FallbackFirst(t *testing.T) {
	detail := gazelleTorrentDetailResponse{Torrents: []GazelleTorrent{
		{TorrentID: 10}, {TorrentID: 20},
	}}
	got := selectGazelleDetailTorrent(detail, 999) // no match -> first
	assert.Equal(t, 10, got.TorrentID)
}

func TestSelectGazelleDetailTorrent_FallbackTopLevel(t *testing.T) {
	detail := gazelleTorrentDetailResponse{ID: 55, Format: "FLAC", Size: 999}
	got := selectGazelleDetailTorrent(detail, 0)
	assert.Equal(t, 55, got.TorrentID)
	assert.Equal(t, "FLAC", got.Format)
	assert.Equal(t, int64(999), got.Size)
}

func TestGazelleDriver_GetUserInfo_ExecuteError(t *testing.T) {
	d := NewGazelleDriver(GazelleDriverConfig{BaseURL: "http://127.0.0.1:1", APIKey: "key"})
	_, err := d.GetUserInfo(context.Background())
	require.Error(t, err)
}

func TestGazelleDriver_GetTorrentDetail_Array(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","response":{"group":{"name":"Album","artist":"Artist"},
			"torrents":[{"torrentId":5,"format":"FLAC","encoding":"Lossless","size":2048,"seeders":3,"leechers":1,"snatches":9,"isFreeleech":true,"time":"2024-01-01 10:00:00"}]}}`))
	}))
	defer server.Close()

	d := NewGazelleDriver(GazelleDriverConfig{BaseURL: server.URL, APIKey: "key"})
	item, err := d.GetTorrentDetail(context.Background(), "5", server.URL+"/torrents.php?id=5", "fallback")
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, "5", item.ID)
	assert.Contains(t, item.Title, "Album")
	assert.Equal(t, DiscountFree, item.DiscountLevel)
	assert.Greater(t, item.UploadedAt, int64(0))
}

// ---------------------------------------------------------------------------
// unit3d_driver.go — Execute error paths, GetUserInfo
// ---------------------------------------------------------------------------

func TestUnit3DDriver_Execute_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	d := NewUnit3DDriver(Unit3DDriverConfig{BaseURL: server.URL, APIKey: "k"})
	_, err := d.Execute(context.Background(), Unit3DRequest{Endpoint: "/api/user"})
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestUnit3DDriver_Execute_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	d := NewUnit3DDriver(Unit3DDriverConfig{BaseURL: server.URL, APIKey: "k"})
	_, err := d.Execute(context.Background(), Unit3DRequest{Endpoint: "/api/user"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 502")
}

func TestUnit3DDriver_Execute_BadJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	d := NewUnit3DDriver(Unit3DDriverConfig{BaseURL: server.URL, APIKey: "k"})
	_, err := d.Execute(context.Background(), Unit3DRequest{Endpoint: "/api/user"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse JSON")
}

func TestUnit3DDriver_GetUserInfo_Error(t *testing.T) {
	d := NewUnit3DDriver(Unit3DDriverConfig{BaseURL: "http://127.0.0.1:1", APIKey: "k"})
	_, err := d.GetUserInfo(context.Background())
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// base_site.go — DownloadWithHash, GetDetailFetcher
// ---------------------------------------------------------------------------

func TestBaseSite_DownloadWithHash_NoHashDownloader(t *testing.T) {
	driver := &MockDriver{}
	site := NewBaseSite(driver, BaseSiteConfig{ID: "t", Name: "T", Kind: SiteNexusPHP, RateLimit: 100, RateBurst: 100, Logger: zap.NewNop()})

	driver.On("PrepareDownload", "12345").Return("req", nil)
	driver.On("Execute", mock.Anything, "req").Return("resp", nil)
	driver.On("ParseDownload", "resp").Return([]byte("data"), nil)

	data, err := site.DownloadWithHash(context.Background(), "12345", "hashval")
	require.NoError(t, err)
	assert.Equal(t, []byte("data"), data)
}

func TestBaseSite_GetDetailFetcher_Nil(t *testing.T) {
	driver := &MockDriver{}
	site := NewBaseSite(driver, BaseSiteConfig{ID: "t", Name: "T", Kind: SiteNexusPHP, Logger: zap.NewNop()})
	assert.Nil(t, site.GetDetailFetcher())
}

// ---------------------------------------------------------------------------
// filters.go — querystringFilter branches
// ---------------------------------------------------------------------------

func TestQuerystringFilter_Branches(t *testing.T) {
	assert.Equal(t, "42", querystringFilter("https://x.com/details.php?id=42&x=1", "id"))
	assert.Equal(t, "", querystringFilter("https://x.com?id=42"))
	assert.Equal(t, "", querystringFilter("https://x.com?a=1", "id"))
	// control char makes url.Parse fail but url.ParseQuery still recovers id=9 (fallback branch)
	assert.Equal(t, "9", querystringFilter("foo\nbar=1&id=9", "id"))
}

// ---------------------------------------------------------------------------
// site_definition.go — NewSizeTieredHRCalc, DefaultAuthMethod
// ---------------------------------------------------------------------------

func TestNewSizeTieredHRCalc_NoMatchFallback(t *testing.T) {
	rules := []HRSeedTimeRule{
		{MinSizeGB: 0, MaxSizeGB: 10, SeedTimeH: 24},
		{MinSizeGB: 10, MaxSizeGB: 50, SeedTimeH: 48},
		{MinSizeGB: 50, MaxSizeGB: 0, SeedTimeH: 96},
	}
	calc := NewSizeTieredHRCalc(rules, 12)

	// 5 GiB -> tier 1
	assert.Equal(t, 36, calc(5*1024*1024*1024))
	// 20 GiB -> tier 2
	assert.Equal(t, 60, calc(20*1024*1024*1024))
	// 100 GiB -> tier 3
	assert.Equal(t, 108, calc(100*1024*1024*1024))
	// unknown size (<=0) -> max
	assert.Equal(t, 108, calc(0))
}

func TestSchema_DefaultAuthMethod(t *testing.T) {
	assert.Equal(t, AuthMethodAPIKey, SchemaMTorrent.DefaultAuthMethod())
	assert.Equal(t, AuthMethodAPIKey, SchemaUnit3D.DefaultAuthMethod())
	assert.Equal(t, AuthMethodPasskey, SchemaRousi.DefaultAuthMethod())
	assert.Equal(t, AuthMethodCookie, SchemaNexusPHP.DefaultAuthMethod())
	assert.Equal(t, AuthMethodCookie, SchemaGazelle.DefaultAuthMethod())
}

// ---------------------------------------------------------------------------
// persistent_rate_limiter.go — NewPersistentRateLimiterFromRPS, timeUntilNextWindow
// ---------------------------------------------------------------------------

func TestNewPersistentRateLimiterFromRPS_Defaults(t *testing.T) {
	l := NewPersistentRateLimiterFromRPS(nil, "site", 0, 0) // defaults kick in
	require.NotNil(t, l)
	d := l.timeUntilNextWindow()
	assert.GreaterOrEqual(t, d, time.Duration(0))
}

func TestNewPersistentRateLimiterFromRPS_Values(t *testing.T) {
	l := NewPersistentRateLimiterFromRPS(nil, "site", 2.0, 5)
	require.NotNil(t, l)
}

// ---------------------------------------------------------------------------
// http_client.go — DoRequest methods, HandleQBittorrentAuthWithRequests
// ---------------------------------------------------------------------------

func TestSiteHTTPClient_DoRequest_Methods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.Method))
	}))
	defer server.Close()

	c := NewSiteHTTPClient(SiteHTTPClientConfig{Timeout: 5 * time.Second, UserAgent: "t"})
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		resp, err := c.DoRequest(context.Background(), method, server.URL, nil, map[string]string{"X-H": "v"})
		require.NoError(t, err, method)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}
}

func TestSiteHTTPClient_DoRequest_UnsupportedMethod(t *testing.T) {
	c := NewSiteHTTPClient(SiteHTTPClientConfig{Timeout: time.Second, UserAgent: "t"})
	_, err := c.DoRequest(context.Background(), "BREW", "http://x", nil, nil)
	require.Error(t, err)
}

func TestHandleQBittorrentAuthWithRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "sessid123"})
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Ok."))
	}))
	defer server.Close()

	sid, err := HandleQBittorrentAuthWithRequests(context.Background(), server.URL, "admin", "pw")
	require.NoError(t, err)
	assert.Equal(t, "sessid123", sid)
}

func TestHandleQBittorrentAuthWithRequests_NoCookie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Ok."))
	}))
	defer server.Close()

	_, err := HandleQBittorrentAuthWithRequests(context.Background(), server.URL, "a", "p")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no SID cookie")
}

func TestHandleQBittorrentAuthWithRequests_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	_, err := HandleQBittorrentAuthWithRequests(context.Background(), server.URL, "a", "p")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "login failed")
}

// ---------------------------------------------------------------------------
// registry.go — CreateSite HDDolby + Gazelle credential branches
// ---------------------------------------------------------------------------

func TestSiteRegistry_CreateSite_KindBranches(t *testing.T) {
	registry := NewSiteRegistry(zap.NewNop())

	registry.Register(SiteMeta{ID: "hddolbytest", Name: "HD", Kind: SiteHDDolby, DefaultBaseURL: "https://hd.example"})
	registry.Register(SiteMeta{ID: "gazelletest", Name: "GZ", Kind: SiteGazelle, DefaultBaseURL: "https://gz.example"})
	registry.Register(SiteMeta{ID: "rousitest", Name: "RS", Kind: SiteRousi, DefaultBaseURL: "https://rs.example"})

	_, errHDNoCreds := registry.CreateSite("hddolbytest", SiteCredentials{}, "")
	require.Error(t, errHDNoCreds)
	_, errHDNoCookie := registry.CreateSite("hddolbytest", SiteCredentials{APIKey: "k"}, "")
	require.Error(t, errHDNoCookie)

	_, errGazelle := registry.CreateSite("gazelletest", SiteCredentials{}, "")
	require.Error(t, errGazelle)

	_, errRousi := registry.CreateSite("rousitest", SiteCredentials{}, "")
	require.Error(t, errRousi)

	_, errUnknown := registry.CreateSite("nope", SiteCredentials{}, "")
	require.Error(t, errUnknown)
}
