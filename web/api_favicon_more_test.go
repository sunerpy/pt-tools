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

func TestApiFavicon_FetchOnMiss(t *testing.T) {
	server := setupFaviconServer(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47, 9})
	}))
	defer ts.Close()

	require.NoError(t, global.GlobalDB.DB.Create(&models.FaviconCache{
		SiteID: "hdsky", SiteName: "HDSky", FaviconURL: ts.URL, Data: []byte{1, 2}, ContentType: "image/png", ETag: "e", LastFetched: time.Now(),
	}).Error)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicon/hdsky", nil)
	server.apiFavicon(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Body.Bytes())
}

func TestApiFavicon_RefreshDispatchViaRouter(t *testing.T) {
	server := setupFaviconServer(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/favicon/hdsky/refresh", nil)
	server.apiFavicon(w, req)
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
}

func TestApiUserInfoSiteDetail_PostSync(t *testing.T) {
	svc := setupTestUserInfoService()
	InitUserInfoService(svc)
	t.Cleanup(func() { InitUserInfoService(nil) })

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/sites/site1", nil)
	s.apiUserInfoSiteDetail(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "site1")
}
