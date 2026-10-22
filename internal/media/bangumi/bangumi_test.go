package bangumi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/cachectl"
)

type memCache struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (c *memCache) Get(k string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[k]
	return v, ok
}

func (c *memCache) Set(k string, v []byte, _ time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[k] = v
}

// fake 是假的 Bangumi：葬送的芙莉莲第 1 季（2023-09-29）与第 2 季（2026-01-16），以及同名的特别篇。
type fake struct {
	*httptest.Server
	mu       sync.Mutex
	searches []map[string]any
	hits     int
}

func newFake(t *testing.T) *fake {
	f := &fake{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.hits++
		f.mu.Unlock()
		assert.Contains(t, r.Header.Get("User-Agent"), "pt-tools")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v0/search/subjects":
			var body map[string]any
			b, _ := io.ReadAll(r.Body)
			require.NoError(t, json.Unmarshal(b, &body))
			f.mu.Lock()
			f.searches = append(f.searches, body)
			f.mu.Unlock()
			filter, _ := body["filter"].(map[string]any)
			dates, _ := filter["air_date"].([]any)
			kw, _ := body["keyword"].(string)
			if kw != "葬送のフリーレン" {
				_, _ = w.Write([]byte(`{"data":[],"total":0}`))
				return
			}
			switch {
			case len(dates) == 2 && dates[0] == ">=2023-09-19":
				_, _ = w.Write([]byte(`{"data":[{"id":400602,"type":2,"name":"葬送のフリーレン","name_cn":"葬送的芙莉莲","date":"2023-09-29"},
					{"id":9,"type":2,"name":"葬送のフリーレン ～魔法の才能～","name_cn":"","date":"2023-10-01"}]}`))
			case len(dates) == 2 && dates[0] == ">=2026-01-06":
				_, _ = w.Write([]byte(`{"data":[{"id":500001,"type":2,"name":"葬送のフリーレン 第2期","name_cn":"葬送的芙莉莲 第二季","date":"2026-01-16"}]}`))
			default:
				_, _ = w.Write([]byte(`{"data":[{"id":400602,"type":2,"name":"葬送のフリーレン","date":"2023-09-29"},
					{"id":500001,"type":2,"name":"葬送のフリーレン 第2期","date":"2026-01-16"}]}`))
			}
		case r.URL.Path == "/v0/subjects/400602":
			_, _ = w.Write([]byte(`{"id":400602,"type":2,"name":"葬送のフリーレン","name_cn":"葬送的芙莉莲","summary":" 魔法使芙莉莲…… ",
				"date":"2023-09-29","platform":"TV","eps":28,"rating":{"score":8.5,"total":36689}}`))
		case r.URL.Path == "/v0/subjects/500001":
			_, _ = w.Write([]byte(`{"id":500001,"type":2,"name":"葬送のフリーレン 第2期","name_cn":"葬送的芙莉莲 第二季","summary":"第二季","date":"2026-01-16"}`))
		case r.URL.Path == "/v0/episodes":
			assert.Equal(t, "400602", r.URL.Query().Get("subject_id"))
			assert.Equal(t, "0", r.URL.Query().Get("type"))
			_, _ = w.Write([]byte(`{"data":[{"id":1,"ep":1,"sort":1,"name":"冒険の終わり","name_cn":"冒险结束","airdate":"2023-09-29","desc":"……"},
				{"id":2,"ep":0,"sort":2,"name":"別に魔法じゃなくたって…","name_cn":"","airdate":"2023-09-29"},
				{"id":3,"ep":13.5,"sort":13.5,"name":"总集篇"}],"total":3}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func client(f *fake) *Client {
	return New(Config{BaseURL: f.URL, Interval: time.Millisecond, Cache: &memCache{m: map[string][]byte{}}})
}

func TestFind(t *testing.T) {
	f := newFake(t)
	c := client(f)
	ctx := context.Background()

	s, err := c.Find(ctx, Query{Title: "葬送的芙莉莲", OriginalTitle: "葬送のフリーレン", Date: "2023-09-29"})
	require.NoError(t, err)
	require.NotNil(t, s, "名字对得上的有两个时取完全相同的")
	assert.Equal(t, 400602, s.ID)
	assert.Equal(t, "魔法使芙莉莲……", s.Summary)
	assert.InDelta(t, 8.5, s.Score, 0.001)
	assert.Equal(t, 36689, s.Votes)
	filter := f.searches[0]["filter"].(map[string]any)
	assert.Equal(t, []any{">=2023-09-19", "<2023-10-10"}, filter["air_date"], "首播日期前后 10 天")
	assert.Equal(t, []any{float64(SubjectAnime)}, filter["type"])

	s2, err := c.Find(ctx, Query{Title: "葬送的芙莉莲", OriginalTitle: "葬送のフリーレン", Date: "2026-01-16"})
	require.NoError(t, err)
	require.NotNil(t, s2, "第 2 季：名字以原名开头、日期对得上")
	assert.Equal(t, 500001, s2.ID)

	none, err := c.Find(ctx, Query{Title: "别的", OriginalTitle: "別の", Date: "2023-09-29"})
	require.NoError(t, err)
	assert.Nil(t, none)

	undated, err := c.Find(ctx, Query{OriginalTitle: "葬送のフリーレン"})
	require.NoError(t, err)
	require.NotNil(t, undated, "没给日期时名字完全相同的只有一个才用")
	assert.Equal(t, 400602, undated.ID)

	f.mu.Lock()
	before := f.hits
	f.mu.Unlock()
	_, err = c.Find(ctx, Query{Title: "葬送的芙莉莲", OriginalTitle: "葬送のフリーレン", Date: "2023-09-29"})
	require.NoError(t, err)
	f.mu.Lock()
	assert.Equal(t, before, f.hits, "第二次读缓存")
	f.mu.Unlock()
	_, err = c.Find(cachectl.Bypass(ctx), Query{Title: "葬送的芙莉莲", OriginalTitle: "葬送のフリーレン", Date: "2023-09-29"})
	require.NoError(t, err)
	f.mu.Lock()
	assert.Greater(t, f.hits, before, "重新刮削时不读缓存")
	f.mu.Unlock()
}

func TestEpisodes(t *testing.T) {
	f := newFake(t)
	eps, err := client(f).Episodes(context.Background(), 400602)
	require.NoError(t, err)
	require.Len(t, eps, 2, "总集篇（13.5）不要")
	assert.Equal(t, Episode{ID: 1, Ep: 1, Name: "冒険の終わり", NameCN: "冒险结束", AirDate: "2023-09-29", Desc: "……"}, eps[0])
	assert.Equal(t, 2, eps[1].Ep, "ep 为 0 时用 sort")

	_, err = client(f).Get(context.Background(), 7)
	assert.True(t, IsNotFound(err))
	_, err = client(f).Get(context.Background(), 0)
	assert.Error(t, err)
}

func TestErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "429") {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte("<html>"))
	}))
	t.Cleanup(srv.Close)
	c := New(Config{BaseURL: srv.URL, Interval: time.Millisecond})
	_, err := c.Get(context.Background(), 1)
	assert.ErrorContains(t, err, "不是预期的内容")
	c = New(Config{BaseURL: srv.URL + "/429", Interval: time.Millisecond})
	_, err = c.Get(context.Background(), 1)
	assert.ErrorContains(t, err, "HTTP 429")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	slow := New(Config{BaseURL: srv.URL, Interval: time.Hour})
	_ = slow.wait(context.Background())
	assert.ErrorIs(t, slow.wait(ctx), context.Canceled)
}
