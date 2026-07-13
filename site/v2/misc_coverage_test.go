package v2

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthMethod_IsValidString(t *testing.T) {
	assert.True(t, AuthMethodCookie.IsValid())
	assert.True(t, AuthMethodAPIKey.IsValid())
	assert.True(t, AuthMethodCookieAndAPIKey.IsValid())
	assert.True(t, AuthMethodPasskey.IsValid())
	assert.False(t, AuthMethod("bogus").IsValid())
	assert.Equal(t, "cookie", AuthMethodCookie.String())
}

func TestTorrentItem_Getters(t *testing.T) {
	end := time.Now().Add(time.Hour)
	item := TorrentItem{
		Title:           "My Torrent",
		Tags:            []string{"a", "b"},
		DiscountLevel:   DiscountFree,
		DiscountEndTime: end,
	}
	assert.Equal(t, "My Torrent", item.GetName())
	assert.Equal(t, "a b", item.GetSubTitle())
	assert.Equal(t, string(DiscountFree), item.GetFreeLevel())
	require.NotNil(t, item.GetFreeEndTime())
	assert.Equal(t, end, *item.GetFreeEndTime())

	noEnd := TorrentItem{DiscountLevel: DiscountFree}
	assert.Nil(t, noEnd.GetFreeEndTime())
	assert.Equal(t, "", noEnd.GetSubTitle())
}

func TestTorrentItem_CanbeFinished(t *testing.T) {
	// Size over limit -> false
	big := TorrentItem{SizeBytes: 100 * 1024 * 1024 * 1024}
	assert.False(t, big.CanbeFinished(true, 10, 50))

	// disabled -> true
	item := TorrentItem{SizeBytes: 1024 * 1024 * 1024}
	assert.True(t, item.CanbeFinished(false, 0, 0))

	// enabled, no end time -> true
	assert.True(t, item.CanbeFinished(true, 100, 0))

	// enabled, end passed -> false
	past := TorrentItem{SizeBytes: 1024 * 1024 * 1024, DiscountEndTime: time.Now().Add(-time.Hour)}
	assert.False(t, past.CanbeFinished(true, 100, 0))

	// enabled, plenty of time -> true
	future := TorrentItem{SizeBytes: 1024 * 1024, DiscountEndTime: time.Now().Add(10 * time.Hour)}
	assert.True(t, future.CanbeFinished(true, 100, 0))
}

func TestDefaultSiteHTTPClientConfig(t *testing.T) {
	cfg := DefaultSiteHTTPClientConfig()
	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.Equal(t, 100, cfg.MaxIdleConns)
	assert.NotEmpty(t, cfg.UserAgent)
}

func TestHTTPResponse_IsSuccessIsError(t *testing.T) {
	assert.True(t, (&HTTPResponse{StatusCode: 200}).IsSuccess())
	assert.True(t, (&HTTPResponse{StatusCode: 299}).IsSuccess())
	assert.False(t, (&HTTPResponse{StatusCode: 404}).IsSuccess())
	assert.True(t, (&HTTPResponse{StatusCode: 404}).IsError())
	assert.True(t, (&HTTPResponse{StatusCode: 500}).IsError())
	assert.False(t, (&HTTPResponse{StatusCode: 200}).IsError())
}

func TestSiteHTTPClient_PostJSONAndClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewSiteHTTPClient(SiteHTTPClientConfig{Timeout: 5 * time.Second})
	resp, err := client.PostJSON(context.Background(), server.URL, []byte(`{"a":1}`), nil)
	require.NoError(t, err)
	assert.True(t, resp.IsSuccess())
	require.NoError(t, client.Close())
}

func TestFilters_IntFloatCase(t *testing.T) {
	assert.Equal(t, int64(1234), ApplyFilters("1,234", []Filter{{Name: "parseInt"}}))
	assert.InDelta(t, 12.5, ApplyFilters("12.5", []Filter{{Name: "parseFloat"}}).(float64), 0.01)
	assert.Equal(t, "hello", ApplyFilters("HELLO", []Filter{{Name: "toLowerCase"}}))
	assert.Equal(t, "HELLO", ApplyFilters("hello", []Filter{{Name: "toUpperCase"}}))
}

func TestListRegisteredSchemas(t *testing.T) {
	schemas := ListRegisteredSchemas()
	assert.NotEmpty(t, schemas)
	found := false
	for _, s := range schemas {
		if s == "NexusPHP" {
			found = true
		}
	}
	assert.True(t, found)
}

func TestDefaultDetailParserConfig(t *testing.T) {
	cfg := DefaultDetailParserConfig()
	require.NotNil(t, cfg)
	assert.Equal(t, "2006-01-02 15:04:05", cfg.TimeLayout)
	assert.Equal(t, DiscountFree, cfg.DiscountMapping["free"])
	assert.NotEmpty(t, cfg.HRKeywords)
}

func TestNexusPHPParser_Options(t *testing.T) {
	p := NewNexusPHPParser(
		WithDiscountMapping(map[string]DiscountLevel{"foo": DiscountFree}),
		WithHRKeywords([]string{"kw"}),
		WithParserTimeLayout("2006-01-02"),
	)
	require.NotNil(t, p)
	assert.Equal(t, DiscountFree, p.config.DiscountMapping["foo"])
	assert.Equal(t, []string{"kw"}, p.config.HRKeywords)
	assert.Equal(t, "2006-01-02", p.config.TimeLayout)
}

func TestNexusPHPParserFromDefinition_Default(t *testing.T) {
	p := NewNexusPHPParserFromDefinition(nil)
	require.NotNil(t, p)

	def := &SiteDefinition{DetailParser: &DetailParserConfig{TimeLayout: "2006-01-02"}}
	p2 := NewNexusPHPParserFromDefinition(def)
	assert.Equal(t, "2006-01-02", p2.config.TimeLayout)
}

func TestNexusPHPDriver_ExtractFieldValuePublic(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	doc := mustDoc(t, `<div id="v">hello</div>`)
	sel := FieldSelector{Selector: []string{"#v"}}
	assert.Equal(t, "hello", d.ExtractFieldValuePublic(doc, sel))
}

func TestSearchOrchestrator_GetSite(t *testing.T) {
	o := NewSearchOrchestrator(SearchOrchestratorConfig{})
	site := &mockSearchSite{id: "s1", name: "Site 1"}
	o.RegisterSite(site)
	assert.Equal(t, site, o.GetSite("s1"))
	assert.Nil(t, o.GetSite("missing"))
}

func TestSiteURLRegistry_GetFailoverConfig(t *testing.T) {
	reg := GetGlobalRegistry()
	// A registered site should have URLs
	sites := reg.ListSites()
	if len(sites) > 0 {
		cfg, err := reg.GetFailoverConfig(sites[0])
		require.NoError(t, err)
		assert.NotEmpty(t, cfg.BaseURLs)
	}
	// Unknown site -> error
	_, err := reg.GetFailoverConfig(SiteName("nonexistent-xyz"))
	assert.Error(t, err)
}

func TestGetSiteURLsForKind(t *testing.T) {
	result := GetSiteURLsForKind(SiteNexusPHP)
	assert.NotNil(t, result)
}

func TestComputeTorrentHashFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.torrent")
	// Minimal valid torrent with info dict
	content := []byte("d4:infod6:lengthi100e4:name4:teste8:announce3:urle")
	require.NoError(t, os.WriteFile(path, content, 0o644))

	hash, err := ComputeTorrentHashFromFile(path)
	require.NoError(t, err)
	assert.Len(t, hash, 40)

	_, err = ComputeTorrentHashFromFile(filepath.Join(dir, "missing.torrent"))
	assert.Error(t, err)
}

func TestParseTorrentFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.torrent")
	content := []byte("d8:announce3:url4:infod6:lengthi100e4:name8:test.mkv12:piece lengthi16384eee")
	require.NoError(t, os.WriteFile(path, content, 0o644))

	parsed, err := ParseTorrentFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, "test.mkv", parsed.Name)
	assert.Equal(t, int64(100), parsed.Size)

	_, err = ParseTorrentFromFile(filepath.Join(dir, "missing.torrent"))
	assert.Error(t, err)
}

func TestGetSiteCategoryConfig(t *testing.T) {
	cfg := GetSiteCategoryConfig("mteam")
	require.NotNil(t, cfg)
	assert.Equal(t, "mteam", cfg.SiteID)
	assert.NotEmpty(t, cfg.Categories)

	assert.Nil(t, GetSiteCategoryConfig("nonexistent-site"))

	all := GetAllSiteCategoryConfigs()
	assert.NotEmpty(t, all)
}

func TestGazelleDriver_GetUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","response":{
			"id":42,"username":"gztester",
			"stats":{"uploaded":10737418240,"downloaded":1073741824,"ratio":10.0,"LastAccess":"2024-06-01 12:00:00"},
			"ranks":{"class":"Elite"},
			"personal":{"bonus":5000},
			"community":{"seeding":15,"leeching":1}
		}}`))
	}))
	defer server.Close()

	d := NewGazelleDriver(GazelleDriverConfig{BaseURL: server.URL, APIKey: "k"})
	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "42", info.UserID)
	assert.Equal(t, "gztester", info.Username)
	assert.Equal(t, int64(10737418240), info.Uploaded)
	assert.Equal(t, "Elite", info.Rank)
	assert.Equal(t, 15, info.Seeding)
	assert.Greater(t, info.LastAccess, int64(0))
}

func TestGazelleDriver_ParseDownload(t *testing.T) {
	d := NewGazelleDriver(GazelleDriverConfig{BaseURL: "https://x.com"})
	data, err := d.ParseDownload(GazelleResponse{RawBody: []byte("torrent")})
	require.NoError(t, err)
	assert.Equal(t, []byte("torrent"), data)

	_, err = d.ParseDownload(GazelleResponse{})
	assert.ErrorIs(t, err, ErrParseError)
}

func TestUnit3DDriver_GetUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{
			"id":7,"username":"u3duser","uploaded":10737418240,"downloaded":1073741824,
			"ratio":10.0,"seedbonus":5000,"seeding":20,"leeching":2,
			"group":{"name":"Uploader"},"created_at":"2020-01-01T00:00:00Z",
			"last_login":"2024-06-01T12:00:00Z","last_action":"2024-06-02T09:00:00Z"
		}}`))
	}))
	defer server.Close()

	d := NewUnit3DDriver(Unit3DDriverConfig{BaseURL: server.URL, APIKey: "k"})
	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "7", info.UserID)
	assert.Equal(t, "u3duser", info.Username)
	assert.Equal(t, "Uploader", info.Rank)
	assert.Greater(t, info.JoinDate, int64(0))
	assert.Greater(t, info.LastLogin, int64(0))
	assert.Greater(t, info.LastAccess, int64(0))
}

func TestUnit3DDriver_ParseDownload(t *testing.T) {
	d := NewUnit3DDriver(Unit3DDriverConfig{BaseURL: "https://x.com", APIKey: "k"})
	data, err := d.ParseDownload(Unit3DResponse{RawBody: []byte("torrent")})
	require.NoError(t, err)
	assert.Equal(t, []byte("torrent"), data)

	_, err = d.ParseDownload(Unit3DResponse{})
	assert.ErrorIs(t, err, ErrParseError)
}

func TestParseUnit3DTimestamp(t *testing.T) {
	assert.Equal(t, int64(0), parseUnit3DTimestamp(""))
	assert.Equal(t, int64(0), parseUnit3DTimestamp("garbage"))
	assert.Greater(t, parseUnit3DTimestamp("2024-06-01T12:00:00Z"), int64(0))
	assert.Greater(t, parseUnit3DTimestamp("2024-06-01 12:00:00"), int64(0))
}

func TestJSONMarshalMetricsSnapshot(t *testing.T) {
	// exercise json tags to avoid unused import concerns; harmless coverage of struct
	snap := MetricsSnapshot{Counters: map[string]int64{"a": 1}}
	b, err := json.Marshal(snap)
	require.NoError(t, err)
	assert.Contains(t, string(b), "\"a\":1")
}
