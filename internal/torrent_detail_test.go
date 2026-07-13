// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	_ "github.com/sunerpy/pt-tools/site/v2/definitions"
)

func TestFetchMTorrentDetail_DecodeError(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	srv := badJSONServer(t)
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("mteam"), models.SiteConfig{
		Enabled: boolPtrIX(true), AuthMethod: "api_key", APIKey: "k", APIUrl: srv.URL,
	}))
	_, err := fetchMTorrentDetail(context.Background(), models.SiteGroup("mteam"), &gofeed.Item{GUID: "1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "解析 JSON")
}

func TestFetchMTorrentDetail_ProxyBranch(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","data":{"id":"1","name":"N","smallDescr":"S","size":"1024","status":{"discount":"FREE"}}}`))
	}))
	t.Cleanup(srv.Close)
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("mteam"), models.SiteConfig{
		Enabled: boolPtrIX(true), AuthMethod: "api_key", APIKey: "k", APIUrl: srv.URL,
	}))

	t.Setenv("HTTP_PROXY", srv.URL)
	t.Setenv("HTTPS_PROXY", srv.URL)
	detail, err := fetchMTorrentDetail(context.Background(), models.SiteGroup("mteam"), &gofeed.Item{GUID: "1"})
	if err == nil {
		require.NotNil(t, detail)
		assert.Equal(t, "N", detail.GetName())
	}
}

func TestGetTorrentDetailForTest_MTorrentRateLimitCanceled(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","data":{"id":"1","name":"N","status":{"discount":"FREE"}}}`))
	}))
	t.Cleanup(srv.Close)
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("mteam"), models.SiteConfig{
		Enabled: boolPtrIX(true), AuthMethod: "api_key", APIKey: "k", APIUrl: srv.URL,
	}))

	site := models.SiteGroup("mteam")
	require.NoError(t, waitMTorrentDetailRateLimit(context.Background(), site))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := GetTorrentDetailForTest(ctx, site, &gofeed.Item{GUID: "1", Title: "fb"})
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestFetchMTorrentDetail_MissingConfig(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "mteam", AuthMethod: "api_key"}).Error)
	_, ferr := fetchMTorrentDetail(context.Background(), models.SiteGroup("mteam"), &gofeed.Item{GUID: "1"})
	require.Error(t, ferr)
	assert.Contains(t, ferr.Error(), "API 未配置")
}

func TestFetchMTorrentDetail_APIErrorCode(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"500","message":"boom"}`))
	}))
	t.Cleanup(srv.Close)
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("mteam"), models.SiteConfig{
		Enabled: boolPtrIX(true), AuthMethod: "api_key", APIKey: "k", APIUrl: srv.URL,
	}))
	_, err := fetchMTorrentDetail(context.Background(), models.SiteGroup("mteam"), &gofeed.Item{GUID: "1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API error")
}

func TestFetchMTorrentDetail_Non200(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("mteam"), models.SiteConfig{
		Enabled: boolPtrIX(true), AuthMethod: "api_key", APIKey: "k", APIUrl: srv.URL,
	}))
	_, err := fetchMTorrentDetail(context.Background(), models.SiteGroup("mteam"), &gofeed.Item{GUID: "1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "状态码")
}

func TestFetchNexusPHPDetail_MissingCookie(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "springsunday", AuthMethod: "cookie"}).Error)
	_, err := fetchNexusPHPDetail(context.Background(), models.SiteGroup("springsunday"), &gofeed.Item{GUID: "1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Cookie 未配置")
}

func TestGetTorrentDetailForTest_NexusPHPSchema(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body>
			<input name="torrent_name" value="Nexus.Movie">
			<input name="detail_torrent_id" value="42">
			<h1><font class="free">免费</font><span title="2030-01-20 15:30:00">2天</span></h1>
			<td class="rowhead">基本信息</td><td>大小：10.00 GB</td>
		</body></html>`))
	}))
	t.Cleanup(srv.Close)

	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled: boolPtrPS(true), AuthMethod: "cookie", Cookie: "c=1", APIUrl: srv.URL,
	}))

	item := &gofeed.Item{Title: "fallback", GUID: "42", Link: srv.URL + "/details.php?id=42"}
	res, err := GetTorrentDetailForTest(context.Background(), models.SiteGroup("springsunday"), item)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "Nexus.Movie", res.Title)
	assert.True(t, res.IsFree)
}

func TestGetTorrentDetailForTest_NexusPHPMissingCookie(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled: boolPtrPS(true), AuthMethod: "cookie", Cookie: "c=1", APIUrl: "http://127.0.0.1:0",
	}))
	item := &gofeed.Item{Title: "fb", GUID: "1"}
	res, err := GetTorrentDetailForTest(context.Background(), models.SiteGroup("springsunday"), item)
	require.NoError(t, err)
	assert.Equal(t, "fb", res.Title)
}

func TestGetTorrentDetailForTest_NilItem(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	_, err := GetTorrentDetailForTest(context.Background(), models.SiteGroup("mteam"), nil)
	require.Error(t, err)
}

func TestGetTorrentDetailForTest_UnknownSiteFallsBackToRSS(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	item := &gofeed.Item{Title: "RSS Title", GUID: "g1", Categories: []string{"Movie", "HD"}}
	res, err := GetTorrentDetailForTest(context.Background(), models.SiteGroup("unknown-site-xyz"), item)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "RSS Title", res.Title)
	assert.Equal(t, "Movie,HD", res.Tag)
}

func TestGetTorrentDetailForTest_MTorrentSchema(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/torrent/detail", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","data":{"id":"123","name":"中文名","smallDescr":"副标题","size":"1024","status":{"discount":"FREE"}}}`))
	}))
	t.Cleanup(srv.Close)

	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("mteam"), models.SiteConfig{
		Enabled:    boolPtrTD(true),
		AuthMethod: "api_key",
		APIKey:     "k",
		APIUrl:     srv.URL,
	}))

	item := &gofeed.Item{Title: "fallback", GUID: "123"}
	res, err := GetTorrentDetailForTest(context.Background(), models.SiteGroup("mteam"), item)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "中文名", res.Title)
	assert.Equal(t, "副标题", res.Tag)
	assert.True(t, res.IsFree)
}

func TestGetTorrentDetailForTest_MTorrentMissingConfigSwallowsError(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	// mteam configured but the API endpoint is unreachable -> fetchMTorrentDetail
	// errors, which GetTorrentDetailForTest swallows, returning the RSS fallback.
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("mteam"), models.SiteConfig{
		Enabled:    boolPtrTD(true),
		AuthMethod: "api_key",
		APIKey:     "k",
		APIUrl:     "http://127.0.0.1:0",
	}))

	item := &gofeed.Item{Title: "fallback-title", GUID: "999"}
	res, err := GetTorrentDetailForTest(context.Background(), models.SiteGroup("mteam"), item)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "fallback-title", res.Title)
}

func TestWaitMTorrentDetailRateLimit(t *testing.T) {
	site := models.SiteGroup("mteam-rl-test")
	// first call returns immediately, second call is delayed ~1.2s but we don't
	// wait that long; just verify it returns without error under a generous ctx.
	require.NoError(t, waitMTorrentDetailRateLimit(context.Background(), site))
}

func TestWaitMTorrentDetailRateLimit_ContextCanceled(t *testing.T) {
	site := models.SiteGroup("mteam-rl-cancel")
	// Prime the limiter so the next call must wait.
	require.NoError(t, waitMTorrentDetailRateLimit(context.Background(), site))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := waitMTorrentDetailRateLimit(ctx, site)
	require.Error(t, err)
}
