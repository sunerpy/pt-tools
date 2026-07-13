// MIT License
// Copyright (c) 2025 pt-tools

// Coverage for internal/torrent_detail.go: GetTorrentDetailForTest across the
// nil-item guard, unknown-site fallback, the mTorrent schema path (against a
// fake M-Team detail API), and waitMTorrentDetailRateLimit's timing +
// cancellation behavior.

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

	_ "github.com/sunerpy/pt-tools/site/v2/definitions"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

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

func boolPtrTD(b bool) *bool { return &b }
