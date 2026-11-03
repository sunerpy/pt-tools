package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/models"
)

var blockRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/media/blocks"},
	{http.MethodPost, "/api/media/blocks"},
	{http.MethodDelete, "/api/media/blocks/1"},
	{http.MethodPost, "/api/media/subscriptions/1/torrents/1/block"},
}

func newBlockEnv(t *testing.T) (*Server, *http.ServeMux) {
	t.Helper()
	srv, mux := newSubscribeEnv(t)
	srv.registerBlockRoutes(mux)
	return srv, mux
}

func TestBlockAPI_RequiresSessionAndService(t *testing.T) {
	_, mux := newBlockEnv(t)
	for _, c := range blockRoutes {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(c.method, c.path, nil))
		assert.Equal(t, http.StatusUnauthorized, w.Code, c.method+" "+c.path)
	}
	srv := setupServer(t)
	srv.mgr.StopAll()
	bare := http.NewServeMux()
	srv.registerBlockRoutes(bare)
	srv.sessions.put("sess-test", "admin")
	for _, c := range blockRoutes {
		assert.Equal(t, http.StatusServiceUnavailable, serveAuthed(bare, c.method, c.path, "{}").Code, c.method+" "+c.path)
	}
}

func TestBlockAPI_Flow(t *testing.T) {
	_, mux := newBlockEnv(t)
	w := serveAuthed(mux, http.MethodPost, "/api/media/blocks",
		`{"media_type":"movie","tmdb_id":278,"media_title":"肖申克的救赎 (1994)","reason":"音画不同步","items":[{"site":"hdsky","torrent_id":"12","title":"Shawshank.1080p"}]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var res subscribe.BlockResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	require.Len(t, res.Blocks, 1)
	assert.Equal(t, "音画不同步", res.Blocks[0].Reason)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/blocks", `{"items":[]}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/blocks", `{"item":[]}`).Code, "拼错的字段")

	w = serveAuthed(mux, http.MethodGet, "/api/media/blocks", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"media_title":"肖申克的救赎 (1994)"`)

	id := strconv.Itoa(int(res.Blocks[0].ID))
	assert.Equal(t, http.StatusOK, serveAuthed(mux, http.MethodDelete, "/api/media/blocks/"+id, "").Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodDelete, "/api/media/blocks/"+id, "").Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodDelete, "/api/media/blocks/x", "").Code)
	w = serveAuthed(mux, http.MethodGet, "/api/media/blocks", "")
	assert.Contains(t, w.Body.String(), `"items":[]`)
}

func TestBlockAPI_SubscriptionTorrent(t *testing.T) {
	_, mux := newBlockEnv(t)
	w := serveAuthed(mux, http.MethodPost, "/api/media/subscriptions", `{"media_type":"movie","tmdb_id":278}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var sub models.MediaSubscription
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sub))
	db := global.GlobalDB.DB
	row := models.MediaSubscriptionTorrent{SubscriptionID: sub.ID, SiteName: "hdsky", TorrentID: "12", Title: "Shawshank.1080p-X", Status: models.MediaSubTorrentDownloading}
	require.NoError(t, db.Create(&row).Error)
	sid, tid := strconv.Itoa(int(sub.ID)), strconv.Itoa(int(row.ID))

	w = serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/"+sid+"/torrents/"+tid+"/block", `{"reason":"假种"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"torrent_id":"12"`)
	var got models.MediaSubscriptionTorrent
	require.NoError(t, db.First(&got, row.ID).Error)
	assert.Equal(t, models.MediaSubTorrentFailed, got.Status)
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/"+sid+"/torrents/999/block", `{}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/"+sid+"/torrents/"+tid+"/block", `{"why":1}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/"+sid+"/torrents/x/block", `{}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/x/torrents/"+tid+"/block", `{}`).Code)
	w = serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/"+sid+"/torrents/"+tid+"/block", `{"exclude_group":true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"excluded_group":"X"`)
}

// 关键字搜索的结果里对所有条目拉黑的带上 block；只对一个条目拉黑的不带；没有订阅服务时不标。
func TestMarkBlocked(t *testing.T) {
	srv, _ := newBlockEnv(t)
	ctx := context.Background()
	_, err := srv.subscriber.AddBlocks(ctx, subscribe.BlockInput{Reason: "假种", Items: []subscribe.BlockItem{{Site: "hdsky", TorrentID: "1"}}})
	require.NoError(t, err)
	_, err = srv.subscriber.AddBlocks(ctx, subscribe.BlockInput{MediaType: models.MediaKindMovie, TMDBID: 278, Items: []subscribe.BlockItem{{Site: "hdsky", TorrentID: "2"}}})
	require.NoError(t, err)
	items := []TorrentItemResponse{{ID: "1", SourceSite: "HDSky"}, {ID: "2", SourceSite: "hdsky"}, {ID: "3", SourceSite: "hdsky"}}
	srv.markBlocked(ctx, items)
	require.NotNil(t, items[0].Block)
	assert.Equal(t, "假种", items[0].Block.Reason)
	assert.Nil(t, items[1].Block, "只对一个条目拉黑的不算")
	assert.Nil(t, items[2].Block)

	bare := &Server{}
	plain := []TorrentItemResponse{{ID: "1", SourceSite: "hdsky"}}
	bare.markBlocked(ctx, plain)
	assert.Nil(t, plain[0].Block)
}
