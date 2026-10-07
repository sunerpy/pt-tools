package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/iyuu"
	"github.com/sunerpy/pt-tools/internal/reseed"
	"github.com/sunerpy/pt-tools/internal/transfer"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

type apiTestCipher struct{}

func (apiTestCipher) Encrypt(p string) (string, error) { return "enc:" + p, nil }
func (apiTestCipher) Decrypt(c string) (string, error) {
	if !strings.HasPrefix(c, "enc:") {
		return "", errors.New("bad")
	}
	return strings.TrimPrefix(c, "enc:"), nil
}

// newReseedServer 注册辅种路由，带一个假 IYUU（只回站点列表）和没启动的后台。
func newReseedServer(t *testing.T) (*Server, *http.ServeMux) {
	t.Helper()
	srv := setupServer(t)
	srv.mgr.StopAll()
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.TorrentTransferJob{}, &models.DownloaderPathMap{}, &models.TransferRule{},
		&models.ReseedSetting{}, &models.ReseedRecord{}))
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"sites": []map[string]any{
			{"id": 1, "site": "hdsky", "nickname": "天空", "base_url": "hdsky.me"},
		}}})
	}))
	t.Cleanup(fake.Close)
	tr := transfer.New(transfer.Config{DB: db})
	tw := scheduler.NewTransferWorker(scheduler.TransferWorkerConfig{Service: tr, DB: db})
	srv.mgr.SetTransferWorker(tw)
	svc := reseed.New(reseed.Config{
		DB: db, Cipher: apiTestCipher{}, Transfer: tr,
		SiteIDs: func() []string { return []string{"hdsky"} },
		NewClient: func(token string) *iyuu.Client {
			c := iyuu.New(token)
			c.BaseURL = fake.URL
			return c
		},
	})
	srv.mgr.SetReseedWorker(scheduler.NewReseedWorker(scheduler.ReseedWorkerConfig{Service: svc, Transfer: tw}))
	mux := http.NewServeMux()
	srv.registerReseedRoutes(mux)
	srv.sessions.put("sess-test", "admin")
	return srv, mux
}

func TestReseedAPI_RequiresSession(t *testing.T) {
	_, mux := newReseedServer(t)
	for _, path := range []string{"/api/reseed/settings", "/api/reseed/sites", "/api/reseed/run", "/api/reseed/records", "/api/reseed/jobs"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusFound}, w.Code, path)
	}
}

func TestReseedAPI_NotStarted(t *testing.T) {
	srv := setupServer(t)
	srv.mgr.StopAll()
	mux := http.NewServeMux()
	srv.registerReseedRoutes(mux)
	srv.sessions.put("sess-test", "admin")
	assert.Equal(t, http.StatusServiceUnavailable, serveAuthed(mux, http.MethodGet, "/api/reseed/settings", "").Code)
	assert.Equal(t, http.StatusServiceUnavailable, serveAuthed(mux, http.MethodGet, "/api/reseed/jobs", "").Code)
}

func TestReseedAPI_SettingsSitesRun(t *testing.T) {
	_, mux := newReseedServer(t)
	w := serveAuthed(mux, http.MethodGet, "/api/reseed/settings", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got ReseedSettingsView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.False(t, got.Enabled)
	assert.False(t, got.HasToken)
	assert.False(t, got.Running)

	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodGet, "/api/reseed/sites", "").Code, "没有 token")
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPut, "/api/reseed/settings", `{"enabled":true}`).Code, "开启要先填 token")
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPut, "/api/reseed/settings", `{"interval_hours":999}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPut, "/api/reseed/settings", `{`).Code)

	w = serveAuthed(mux, http.MethodPut, "/api/reseed/settings", `{"enabled":true,"token":"secret-token","interval_hours":6,"site_names":["hdsky"]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "secret-token", "token 只写不读")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.True(t, got.HasToken)
	assert.Equal(t, 6, got.IntervalHours)
	assert.NotContains(t, serveAuthed(mux, http.MethodGet, "/api/reseed/settings", "").Body.String(), "secret-token")

	w = serveAuthed(mux, http.MethodGet, "/api/reseed/sites", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var sites struct {
		Items []reseed.SiteMapItem `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sites))
	require.Len(t, sites.Items, 1)
	assert.Equal(t, "hdsky", sites.Items[0].SiteName)

	w = serveAuthed(mux, http.MethodPost, "/api/reseed/run", "")
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	require.Eventually(t, func() bool {
		resp := serveAuthed(mux, http.MethodGet, "/api/reseed/settings", "")
		var v ReseedSettingsView
		_ = json.Unmarshal(resp.Body.Bytes(), &v)
		return v.LastResult != "" && !v.Running
	}, 5*time.Second, 20*time.Millisecond, "后台跑完一轮并记下结果")

	assert.Equal(t, http.StatusOK, serveAuthed(mux, http.MethodGet, "/api/reseed/records", "").Code)
	w = serveAuthed(mux, http.MethodGet, "/api/reseed/jobs?status=active", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"items":[]}`, w.Body.String())
	w = serveAuthed(mux, http.MethodDelete, "/api/reseed/jobs", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"deleted":0}`, w.Body.String())

	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/api/reseed/settings"},
		{http.MethodPost, "/api/reseed/sites"},
		{http.MethodGet, "/api/reseed/run"},
		{http.MethodPost, "/api/reseed/records"},
		{http.MethodPut, "/api/reseed/jobs"},
	} {
		assert.Equal(t, http.StatusMethodNotAllowed, serveAuthed(mux, c.method, c.path, "").Code, c.path)
	}
}

func TestWriteReseedError(t *testing.T) {
	for err, want := range map[error]int{
		reseed.ErrInvalid:                   http.StatusBadRequest,
		iyuu.ErrNoToken:                     http.StatusBadRequest,
		reseed.ErrNoSites:                   http.StatusBadRequest,
		scheduler.ErrReseedBusy:             http.StatusConflict,
		&iyuu.RateLimitError{Msg: "x"}:      http.StatusTooManyRequests,
		&iyuu.APIError{Code: 500, Msg: "x"}: http.StatusBadGateway,
		errors.New("boom"):                  http.StatusInternalServerError,
	} {
		w := httptest.NewRecorder()
		writeReseedError(w, err)
		assert.Equal(t, want, w.Code, err.Error())
	}
}
