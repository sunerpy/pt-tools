package web

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// entrySearchFake 回固定的种子，记下搜过的查询。
type entrySearchFake struct {
	mu      sync.Mutex
	items   []v2.TorrentItem
	queries []v2.MultiSiteSearchQuery
}

func (f *entrySearchFake) Search(_ context.Context, q v2.MultiSiteSearchQuery) (*v2.MultiSiteSearchResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, q)
	return &v2.MultiSiteSearchResult{Items: append([]v2.TorrentItem(nil), f.items...)}, nil
}

// 按条目搜索：只认会话、请求体严格解析；没指定站点时搜启用的站点，指定了没启用的写进 errors，一个都没启用时报错。
func TestMediaSearchAPI(t *testing.T) {
	writeWebTestSecretKey(t)
	fake := &entrySearchFake{items: []v2.TorrentItem{{
		ID: "1", SourceSite: "hdsky", Title: "The.Shawshank.Redemption.1994.1080p.BluRay.x264-WiKi", SizeBytes: 10 << 30, Seeders: 50,
		DownloadURL: "/api/site/hdsky/torrent/1/download",
	}}}
	srv, mux := newSubscribeEnv(t, func(c *subscribe.Config) { c.Search = fake })
	enabled := true
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{Enabled: &enabled, AuthMethod: "cookie", Cookie: "uid=1"}))

	w := serveAuthed(mux, http.MethodPost, "/api/media/search", `{"media_type":"movie","tmdb_id":278,"profile_id":-1}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got subscribe.MediaSearchResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "肖申克的救赎", got.Item.Title)
	require.Len(t, got.Groups, 1)
	assert.Equal(t, "hdsky", got.Groups[0].Members[0].Site)
	assert.Equal(t, "/api/site/hdsky/torrent/1/download", got.Groups[0].Members[0].DownloadURL)
	assert.Equal(t, 1, got.Total)
	require.NotEmpty(t, fake.queries)
	assert.Equal(t, []string{"hdsky"}, fake.queries[0].Sites, "没指定站点时搜启用的站点")
	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.Contains(t, raw, "episodes", "数组字段给 [] 不给 null")
	assert.Contains(t, raw, "errors")

	w = serveAuthed(mux, http.MethodPost, "/api/media/search", `{"media_type":"movie","tmdb_id":278,"sites":["hdsky","nope"]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Contains(t, got.Errors, subscribe.MediaSearchError{Site: "nope", Error: "站点没有启用，没有搜索"})

	w = serveAuthed(mux, http.MethodPost, "/api/media/search", `{"media_type":"movie","tmdb_id":278,"sites":["nope"]}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "指定的站点都没有启用")
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/search", `{"media_type":"movie","tmdb_id":278,"bogus":1}`).Code, "拼错的字段")
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/search", `{"media_type":"book","tmdb_id":278}`).Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodPost, "/api/media/search", `{"media_type":"movie","tmdb_id":999}`).Code, "TMDB 上没有这个条目")
}
