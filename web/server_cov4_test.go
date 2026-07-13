package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestApiPassword_Paths(t *testing.T) {
	srv := setupServer(t)
	require.NoError(t, srv.store.EnsureAdmin("pwuser", hashPassword("oldpass")))

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/password", nil)
		srv.apiPassword(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/password", bytes.NewBufferString(`{bad`))
		srv.apiPassword(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("wrong old password", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"Username": "pwuser", "Old": "nope", "New": "newpass"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/password", bytes.NewReader(body))
		srv.apiPassword(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"Username": "pwuser", "Old": "oldpass", "New": "newpass"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/password", bytes.NewReader(body))
		srv.apiPassword(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestLoginHandler_MultipartForm(t *testing.T) {
	srv := setupServer(t)
	require.NoError(t, srv.store.EnsureAdmin("admin", hashPassword("adminadmin")))

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("username", "admin")
	_ = mw.WriteField("password", "adminadmin")
	_ = mw.Close()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	srv.loginHandler(w, req)
	assert.Equal(t, http.StatusFound, w.Code)
}

func TestApiSiteDetail_DeleteRSS(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)

	enabled := true
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "cookie", Cookie: "c=1", APIUrl: "https://hdsky.me",
		RSS: []models.RSSConfig{{Name: "feed1", URL: "http://e/rss"}},
	}))

	var rss models.RSSSubscription
	require.NoError(t, global.GlobalDB.DB.First(&rss).Error)

	t.Run("missing id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/sites/hdsky", nil)
		srv.apiSiteDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("bad id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/sites/hdsky?id=abc", nil)
		srv.apiSiteDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("delete existing rss", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/sites/hdsky?id=1", nil)
		srv.apiSiteDetail(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestApiQbit_GetPostCov(t *testing.T) {
	srv := setupServer(t)

	t.Run("get", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/qbit", nil)
		srv.apiQbit(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("post bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/qbit", bytes.NewBufferString(`{bad`))
		srv.apiQbit(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/qbit", nil)
		srv.apiQbit(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiTasks_Filters(t *testing.T) {
	srv := setupServer(t)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}))
	pushed := true
	require.NoError(t, global.GlobalDB.DB.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "1", Title: "Alpha", IsDownloaded: true, IsPushed: &pushed,
	}).Error)
	require.NoError(t, global.GlobalDB.DB.Create(&models.TorrentInfo{
		SiteName: "mteam", TorrentID: "2", Title: "Beta", IsExpired: true,
	}).Error)

	t.Run("filter downloaded pushed site", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/tasks?downloaded=1&pushed=1&site=hdsky&page=1&page_size=2", nil)
		srv.apiTasks(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("search and expired sort asc", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/tasks?q=Beta&expired=1&sort=created_at_asc", nil)
		srv.apiTasks(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})
}
