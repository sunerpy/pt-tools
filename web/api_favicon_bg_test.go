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
	_ "github.com/sunerpy/pt-tools/site/v2/definitions"
)

func TestInitFaviconService(t *testing.T) {
	setupFaviconServer(t)
	initFaviconService()
	require.NotNil(t, faviconService)
	close(faviconService.stopCh)
}

func TestRefreshExpiredFavicons(t *testing.T) {
	setupFaviconServer(t)

	t.Run("no enabled sites returns early", func(t *testing.T) {
		faviconService.refreshExpiredFavicons()
	})

	t.Run("with enabled site and expired cache", func(t *testing.T) {
		require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)
		old := time.Now().Add(-48 * time.Hour)
		require.NoError(t, global.GlobalDB.DB.Create(&models.FaviconCache{
			SiteID: "hdsky", SiteName: "HDSky", FaviconURL: "http://127.0.0.1:1/favicon.ico",
			Data: []byte{1}, LastFetched: old,
		}).Error)
		faviconService.refreshExpiredFavicons()
	})
}

func TestRefreshExpiredFavicons_NilDB(t *testing.T) {
	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })
	fs := &FaviconService{refreshInterval: time.Hour}
	fs.refreshExpiredFavicons()
}

func TestLoadEnabledSiteIDsLower_NilDB(t *testing.T) {
	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })
	assert.Nil(t, loadEnabledSiteIDsLower())
}

func TestApiFavicon_NilDBList(t *testing.T) {
	setupFaviconServer(t)
	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })

	server := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicons", nil)
	server.apiFaviconList(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
