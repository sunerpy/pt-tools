package web

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// entrySearchFake 回固定的种子，记下搜过的查询；delay 不为零时隔这么久才回（站点慢）。
type entrySearchFake struct {
	mu      sync.Mutex
	items   []v2.TorrentItem
	queries []v2.MultiSiteSearchQuery
	delay   time.Duration
}

func (f *entrySearchFake) Search(ctx context.Context, q v2.MultiSiteSearchQuery) (*v2.MultiSiteSearchResult, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return &v2.MultiSiteSearchResult{Errors: []v2.SearchError{{Site: "hdsky", Error: ctx.Err().Error()}}}, nil
		}
	}
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

// 按条目搜索带 draft（试搜索）：标题包含与排除按表单来，不改订阅；draft 与 profile_id 同时给时回 400。
// 会话接口不设站点搜索的截止时间：站点晚一点回来照样有结果、没有站点错误。
func TestMediaSearchAPIDraft(t *testing.T) {
	writeWebTestSecretKey(t)
	fake := &entrySearchFake{delay: 50 * time.Millisecond, items: []v2.TorrentItem{{
		ID: "1", SourceSite: "hdsky", Title: "The.Shawshank.Redemption.1994.1080p.BluRay.x264-WiKi", SizeBytes: 10 << 30, Seeders: 50,
	}}}
	srv, mux := newSubscribeEnv(t, func(c *subscribe.Config) { c.Search = fake })
	enabled := true
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{Enabled: &enabled, AuthMethod: "cookie", Cookie: "uid=1"}))
	search := func(body string) subscribe.MediaSearchResult {
		t.Helper()
		w := serveAuthed(mux, http.MethodPost, "/api/media/search", body)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var got subscribe.MediaSearchResult
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		return got
	}

	got := search(`{"media_type":"movie","tmdb_id":278,"draft":{"profile_id":0,"include_keywords":"WiKi","exclude_keywords":""}}`)
	assert.Empty(t, got.Errors, "站点晚 50 毫秒回来：没有截止时间，不算出错")
	require.Len(t, got.Groups, 1)
	require.NotNil(t, got.Groups[0].Members[0].Verdict)
	assert.Empty(t, got.Groups[0].Members[0].Verdict.Reject)

	got = search(`{"media_type":"movie","tmdb_id":278,"draft":{"exclude_keywords":"wiki"}}`)
	require.Len(t, got.Groups, 1)
	assert.Equal(t, "标题含排除词 wiki", got.Groups[0].Members[0].Verdict.Reject)

	w := serveAuthed(mux, http.MethodPost, "/api/media/search", `{"media_type":"movie","tmdb_id":278,"profile_id":3,"draft":{}}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "draft 与 profile_id 不能同时给")
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/search", `{"media_type":"movie","tmdb_id":278,"draft":{"bogus":1}}`).Code, "draft 里拼错的字段")
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/search", `{"media_type":"movie","tmdb_id":278,"deadline":"2026-01-01T00:00:00Z"}`).Code, "请求体设不了截止时间")
}
