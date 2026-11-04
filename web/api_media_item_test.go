package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

var mediaItemRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/media/items/movie/278"},
	{http.MethodGet, "/api/media/items/tv/1001/seasons/1"},
	{http.MethodGet, "/api/media/items/tv/1001/episode-groups"},
	{http.MethodPost, "/api/media/items/delete/preview"},
	{http.MethodPost, "/api/media/items/delete"},
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

	w = serveAuthed(mux, http.MethodGet, "/api/media/items/tv/1001/episode-groups", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var groups struct {
		Groups []tmdb.EpisodeGroupSummary `json:"groups"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &groups))
	require.Len(t, groups.Groups, 1)
	assert.Equal(t, tmdb.EpisodeGroupSummary{ID: "5b11ba820e0a265a53000ce9", Name: "Absolute Order", Type: tmdb.EpisodeGroupAbsolute, GroupCount: 1, EpisodeCount: 2}, groups.Groups[0])
	w = serveAuthed(mux, http.MethodGet, "/api/media/items/tv/278/episode-groups", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"groups":[]}`, w.Body.String(), "没有剧集组时给 []")
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodGet, "/api/media/items/tv/x/episode-groups", "").Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodGet, "/api/media/items/tv/0/episode-groups", "").Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodGet, "/api/media/items/tv/999/episode-groups", "").Code)

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

// noGuard 是什么都不保护的删种规则（这里只测接口）。
type noGuard struct{}

func (noGuard) Check(downloader.Torrent) subscribe.Protection { return subscribe.Protection{} }
func (noGuard) Rules() []string                               { return []string{"保留标签"} }

// 删除一部片：预览列出库里的文件，执行逐条回结果；请求不对时 400。
func TestMediaItemAPI_Delete(t *testing.T) {
	srv, mux := newSubscribeEnv(t, func(c *subscribe.Config) {
		c.Deleter = organize.New(organize.Config{DB: c.DB})
		c.Guard = func(context.Context) (subscribe.Guard, error) { return noGuard{}, nil }
	})
	srv.registerMediaItemRoutes(mux)
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.TorrentInfoArchive{}))
	target := filepath.Join(t.TempDir(), "肖申克的救赎 (1994).mkv")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o644))
	id, err := transfer.FileID(target)
	require.NoError(t, err)
	row := models.MediaTransferHistory{
		SourcePath: "/dl/s.mkv", TargetPath: target, TargetFileID: id, Mode: models.MediaModeCopy, MediaType: models.MediaKindMovie, TMDBID: 278,
		Status: models.MediaTransferDone, Size: 1,
	}
	require.NoError(t, db.Create(&row).Error)

	w := serveAuthed(mux, http.MethodPost, "/api/media/items/delete/preview", `{"media_type":"movie","tmdb_id":278}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var p subscribe.DeletePreview
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &p))
	require.Len(t, p.Files, 1)
	assert.Equal(t, row.ID, p.Files[0].ID)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	for _, k := range []string{"files", "torrents", "subscriptions", "issues", "errors"} {
		assert.IsType(t, []any{}, raw[k], "%s 给 [] 不给 null", k)
	}

	w = serveAuthed(mux, http.MethodPost, "/api/media/items/delete", fmt.Sprintf(`{"media_type":"movie","tmdb_id":278,"history_ids":[%d],"subscription":"keep"}`, row.ID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var res subscribe.DeleteResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	require.Len(t, res.Steps, 1)
	assert.True(t, res.Steps[0].OK, res.Steps[0].Message)
	assert.NoFileExists(t, target)

	for _, c := range []struct{ path, body string }{
		{"/api/media/items/delete/preview", `{"media_type":"book","tmdb_id":1}`},
		{"/api/media/items/delete/preview", `{"media_type":"movie","tmdb_id":278,"x":1}`},
		{"/api/media/items/delete/preview", `{`},
		{"/api/media/items/delete", `{"media_type":"movie","tmdb_id":278,"subscription":"keep"}`},
		{"/api/media/items/delete", `{"media_type":"movie","tmdb_id":278,"torrents":[{"downloader_id":0,"hash":"x"}]}`},
	} {
		assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, c.path, c.body).Code, c.body)
	}
}
