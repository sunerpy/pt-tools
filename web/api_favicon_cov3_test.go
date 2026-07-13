package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestRefreshExpiredFavicons_ExpiredCacheLoop(t *testing.T) {
	setupFaviconServer(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47, 42})
	}))
	defer ts.Close()

	registerOrRefreshFaviconDef(t, "covexpiredloop", "CovExpiredLoop", ts.URL)

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "covexpiredloop", Enabled: true}).Error)
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, global.GlobalDB.DB.Create(&models.FaviconCache{
		SiteID: "covexpiredloop", SiteName: "CovExpiredLoop", FaviconURL: ts.URL + "/favicon.ico",
		Data: []byte{1}, LastFetched: old,
	}).Error)

	fs := &FaviconService{refreshInterval: time.Nanosecond}
	fs.refreshExpiredFavicons()

	cache, err := fs.GetFavicon("covexpiredloop")
	require.NoError(t, err)
	assert.NotEmpty(t, cache.Data)
}

func TestRefreshExpiredFavicons_EnabledButNoDef(t *testing.T) {
	setupFaviconServer(t)

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "no-def-site-xyz", Enabled: true}).Error)
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, global.GlobalDB.DB.Create(&models.FaviconCache{
		SiteID: "no-def-site-xyz", SiteName: "X", FaviconURL: "http://127.0.0.1:1/f.ico",
		Data: []byte{1}, LastFetched: old,
	}).Error)

	fs := &FaviconService{refreshInterval: time.Nanosecond}
	fs.refreshExpiredFavicons()
}
