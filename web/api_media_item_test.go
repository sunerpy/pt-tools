package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/subscribe"
)

var mediaItemRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/media/items/movie/278"},
	{http.MethodGet, "/api/media/items/tv/1001/seasons/1"},
	{http.MethodGet, "/api/media/calendar"},
}

func newMediaItemEnv(t *testing.T) *http.ServeMux {
	t.Helper()
	srv, mux := newSubscribeEnv(t)
	srv.registerMediaItemRoutes(mux)
	return mux
}

func TestMediaItemAPI_RequiresSessionAndService(t *testing.T) {
	mux := newMediaItemEnv(t)
	for _, c := range mediaItemRoutes {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(c.method, c.path, nil))
		assert.Equal(t, http.StatusUnauthorized, w.Code, c.method+" "+c.path)
	}
	srv := setupServer(t)
	srv.mgr.StopAll()
	bare := http.NewServeMux()
	srv.registerMediaItemRoutes(bare)
	srv.sessions.put("sess-test", "admin")
	for _, c := range mediaItemRoutes {
		assert.Equal(t, http.StatusServiceUnavailable, serveAuthed(bare, c.method, c.path, "").Code, c.method+" "+c.path)
	}
}

// 条目详情、一季的每一集与订阅日历：只读；类型、编号、日期不对时 400，TMDB 上没有时 404。
func TestMediaItemAPI_Flow(t *testing.T) {
	mux := newMediaItemEnv(t)
	w := serveAuthed(mux, http.MethodGet, "/api/media/items/movie/278", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var d subscribe.ItemDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &d))
	assert.Equal(t, "肖申克的救赎", d.Item.Title)
	assert.Equal(t, "tt0111161", d.Item.IMDbID)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	for _, k := range []string{"seasons", "subscriptions", "torrents", "recommendations", "errors"} {
		assert.IsType(t, []any{}, raw[k], "%s 给 [] 不给 null", k)
	}
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodGet, "/api/media/items/book/278", "").Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodGet, "/api/media/items/movie/x", "").Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodGet, "/api/media/items/movie/999", "").Code)

	w = serveAuthed(mux, http.MethodPost, "/api/media/subscriptions", `{"media_type":"tv","tmdb_id":1001,"season":1}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = serveAuthed(mux, http.MethodGet, "/api/media/items/tv/1001/seasons/1", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var season subscribe.SeasonDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &season))
	require.Len(t, season.Episodes, 2)
	require.NotNil(t, season.Subscription)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodGet, "/api/media/items/tv/1001/seasons/x", "").Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodGet, "/api/media/items/tv/-1/seasons/1", "").Code)

	w = serveAuthed(mux, http.MethodGet, "/api/media/calendar?from=2026-01-01&to=2026-01-31", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var cal subscribe.Calendar
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cal))
	assert.Equal(t, "2026-01-01", cal.From)
	require.Len(t, cal.Entries, 2)
	assert.Equal(t, "假综艺", cal.Entries[0].Title)
	assert.Equal(t, "第1期上", cal.Entries[0].Name)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodGet, "/api/media/calendar?from=2026-1-1", "").Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodGet, "/api/media/calendar?from=2026-01-01&to=2026-06-01", "").Code, "超过 62 天")
}
