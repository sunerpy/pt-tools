package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestFaviconFetchAndSave_ErrorPaths(t *testing.T) {
	setupFaviconServer(t)

	t.Run("nil db", func(t *testing.T) {
		prev := global.GlobalDB
		global.GlobalDB = nil
		t.Cleanup(func() { global.GlobalDB = prev })
		fs := &FaviconService{}
		err := fs.fetchAndSave("s", "S", "http://127.0.0.1:1/f.ico")
		require.Error(t, err)
	})

	t.Run("build error invalid url", func(t *testing.T) {
		fs := &FaviconService{}
		err := fs.fetchAndSave("s", "S", "://bad-url")
		require.Error(t, err)
	})

	t.Run("connection error", func(t *testing.T) {
		fs := &FaviconService{}
		err := fs.fetchAndSave("s", "S", "http://127.0.0.1:1/f.ico")
		require.Error(t, err)
	})
}

func TestGetFavicon_NilDB(t *testing.T) {
	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })
	fs := &FaviconService{}
	_, err := fs.GetFavicon("x")
	require.Error(t, err)
}

func TestApiSiteDetail_GetFullResponse(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)

	enabled := true
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{
		Enabled:    &enabled,
		AuthMethod: "cookie",
		Cookie:     "abc=1",
		APIUrl:     "https://hdsky.me",
		Passkey:    "pk",
		RSS:        []models.RSSConfig{{Name: "f1", URL: "http://e/rss"}},
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/hdsky", nil)
	srv.apiSiteDetail(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp SiteConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.HasCookie)
	assert.Empty(t, resp.Cookie)
	assert.True(t, resp.IsBuiltin)
}
