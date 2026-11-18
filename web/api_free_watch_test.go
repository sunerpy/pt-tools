package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/freewatch"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// detailSite 是能读种子详情的假站点（等免费要用）。
type detailSite struct {
	mockSearchSiteForAPI
	detail bool
}

func (d *detailSite) GetDetailFetcher() v2.TorrentDetailFetcher {
	if !d.detail {
		return nil
	}
	return struct{ v2.TorrentDetailFetcher }{}
}

var freeWatchRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/free-watches"},
	{http.MethodPost, "/api/free-watches"},
	{http.MethodGet, "/api/free-watches/sites"},
	{http.MethodPatch, "/api/free-watches/1"},
	{http.MethodPost, "/api/free-watches/1/cancel"},
}

func newFreeWatchEnv(t *testing.T) (*http.ServeMux, uint) {
	t.Helper()
	srv := setupServer(t)
	srv.mgr.StopAll()
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.FreeWatch{}, &models.DownloaderSetting{}, &models.SiteSetting{}))
	dl := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://qb", Enabled: true}
	require.NoError(t, db.Create(&dl).Error)
	for _, s := range []models.SiteSetting{
		{Name: "hdtime", Enabled: true, AuthMethod: "cookie"},
		{Name: "unit3d", Enabled: true, AuthMethod: "cookie"},
		{Name: "offsite", Enabled: false, AuthMethod: "cookie"},
	} {
		require.NoError(t, db.Create(&s).Error)
	}
	require.NoError(t, db.Model(&models.SiteSetting{}).Where("name = ?", "offsite").Update("enabled", false).Error)
	sites := map[string]*detailSite{
		"hdtime":  {mockSearchSiteForAPI: mockSearchSiteForAPI{id: "hdtime", name: "HDTime", kind: v2.SiteNexusPHP}, detail: true},
		"unit3d":  {mockSearchSiteForAPI: mockSearchSiteForAPI{id: "unit3d", name: "Unit3D", kind: v2.SiteNexusPHP}},
		"offsite": {mockSearchSiteForAPI: mockSearchSiteForAPI{id: "offsite", name: "Off", kind: v2.SiteNexusPHP}, detail: true},
	}
	orch := v2.NewSearchOrchestrator(v2.SearchOrchestratorConfig{})
	for _, s := range sites {
		orch.RegisterSite(s)
	}
	prev := searchOrchestrator
	InitSearchOrchestrator(v2.NewCachedSearchOrchestrator(orch, v2.SearchCacheConfig{TTL: time.Minute}))
	t.Cleanup(func() { InitSearchOrchestrator(prev) })
	svc := freewatch.New(freewatch.Config{
		DB: db,
		Site: func(name string) v2.Site {
			if s, ok := sites[name]; ok {
				return s
			}
			return nil
		},
		Fresh: func(context.Context, v2.Site, string, string, time.Time) *v2.PromotionSnapshot { return nil },
	})
	srv.mgr.SetFreeWatchWorker(scheduler.NewFreeWatchWorker(svc, time.Hour))
	mux := http.NewServeMux()
	srv.registerFreeWatchRoutes(mux)
	srv.sessions.put("sess-test", "admin")
	return mux, dl.ID
}

func TestFreeWatchAPI_RequiresSessionAndService(t *testing.T) {
	mux, _ := newFreeWatchEnv(t)
	for _, c := range freeWatchRoutes {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(c.method, c.path, nil))
		assert.Equal(t, http.StatusUnauthorized, w.Code, c.method+" "+c.path)
	}
	srv := setupServer(t)
	srv.mgr.StopAll()
	bare := http.NewServeMux()
	srv.registerFreeWatchRoutes(bare)
	srv.sessions.put("sess-test", "admin")
	for _, c := range freeWatchRoutes {
		assert.Equal(t, http.StatusServiceUnavailable, serveAuthed(bare, c.method, c.path, "{}").Code, c.method+" "+c.path)
	}
}

func TestFreeWatchAPI_Flow(t *testing.T) {
	mux, dl := newFreeWatchEnv(t)
	w := serveAuthed(mux, http.MethodGet, "/api/free-watches/sites", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"sites":["hdtime"]}`, w.Body.String(), "启用着、能读详情的才算")

	body := `{"site":"hdtime","torrent_id":"300101","title":"Fake.Movie","size_bytes":1024,"downloader_id":` + strconv.Itoa(int(dl)) + `,"days":3}`
	w = serveAuthed(mux, http.MethodPost, "/api/free-watches", body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var row models.FreeWatch
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &row))
	assert.Equal(t, models.FreeWatchWatching, row.Status)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/free-watches", body).Code, "已经在等")
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/free-watches", `{"site":"unit3d","torrent_id":"1","downloader_id":1}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/free-watches", `{"site":"hdtime","torrent":"1"}`).Code, "拼错的字段")

	w = serveAuthed(mux, http.MethodGet, "/api/free-watches", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"counts":{"watching":1}`)
	assert.Contains(t, w.Body.String(), `"torrent_id":"300101"`)
	w = serveAuthed(mux, http.MethodGet, "/api/free-watches?status=pushed", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"items":[]`)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodGet, "/api/free-watches?status=bad", "").Code)

	id := strconv.Itoa(int(row.ID))
	at := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	w = serveAuthed(mux, http.MethodPatch, "/api/free-watches/"+id, `{"expires_at":"`+at.Format(time.RFC3339)+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var patched models.FreeWatch
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &patched))
	assert.True(t, at.Equal(patched.ExpiresAt), "到期时间按时刻比（库里存本地时区）：%s", patched.ExpiresAt)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPatch, "/api/free-watches/"+id, `{"expires_at":"2020-01-01T00:00:00Z"}`).Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodPatch, "/api/free-watches/999", `{"expires_at":"`+at.Format(time.RFC3339)+`"}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPatch, "/api/free-watches/x", `{}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPatch, "/api/free-watches/"+id, `{"days":1}`).Code)

	w = serveAuthed(mux, http.MethodPost, "/api/free-watches/"+id+"/cancel", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"status":"canceled"`)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/free-watches/"+id+"/cancel", "").Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodPost, "/api/free-watches/999/cancel", "").Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/free-watches/x/cancel", "").Code)
}

func TestWriteFreeWatchError(t *testing.T) {
	w := httptest.NewRecorder()
	writeFreeWatchError(w, context.DeadlineExceeded)
	assert.Equal(t, http.StatusGatewayTimeout, w.Code)
	w = httptest.NewRecorder()
	writeFreeWatchError(w, assert.AnError)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
