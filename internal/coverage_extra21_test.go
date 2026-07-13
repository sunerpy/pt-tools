// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

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
