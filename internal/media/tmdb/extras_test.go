package tmdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/cachectl"
)

func TestExtras(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/3/movie/872585":
			assert.Equal(t, "credits,external_ids,release_dates", r.URL.Query().Get("append_to_response"))
			_, _ = w.Write([]byte(`{"tagline":" 世界因他而改变 ","status":"Released","vote_count":9000,"imdb_id":"tt15398776",
				"production_companies":[{"name":"Syncopy"},{"name":"Atlas"},{"name":"Syncopy"}],
				"belongs_to_collection":{"id":1,"name":"系列"},
				"credits":{"cast":[{"name":"Emily Blunt","character":"Kitty","order":1,"profile_path":"bad"},
					{"name":"Cillian Murphy","character":"Oppenheimer","order":0,"profile_path":"/c.jpg"},{"name":" ","order":2}],
					"crew":[{"name":"Christopher Nolan","job":"Director","department":"Directing"},
					{"name":"Christopher Nolan","job":"Screenplay","department":"Writing"},
					{"name":"Kai Bird","job":"Novel","department":"Writing"},{"name":"X","job":"Editor","department":"Editing"}]},
				"release_dates":{"results":[{"iso_3166_1":"US","release_dates":[{"certification":""},{"certification":"R"}]},
					{"iso_3166_1":"CN","release_dates":[{"certification":""}]}]}}`))
		case "/3/tv/100088":
			assert.Equal(t, "credits,external_ids,content_ratings", r.URL.Query().Get("append_to_response"))
			_, _ = w.Write([]byte(`{"status":"Returning Series","networks":[{"name":"HBO"}],"production_companies":[{"name":"Sony"}],
				"credits":{"cast":[{"name":"Pedro Pascal","character":"Joel","order":0}]},
				"content_ratings":{"results":[{"iso_3166_1":"US","rating":"TV-MA"}]},
				"external_ids":{"imdb_id":"tt3581920","tvdb_id":392256}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := New(Options{
		APIKey: "k", BaseURL: srv.URL + "/3", ImageBaseURL: srv.URL + "/t/p/", RatePerSecond: 1000, Cache: NewDBCache(newDB(t)),
	})
	require.NoError(t, err)
	ctx := context.Background()

	m, err := c.Extras(ctx, KindMovie, 872585)
	require.NoError(t, err)
	assert.Equal(t, "世界因他而改变", m.Tagline)
	assert.Equal(t, []string{"Syncopy", "Atlas"}, m.Studios, "去掉重复")
	assert.Equal(t, &Collection{ID: 1, Name: "系列"}, m.Collection)
	assert.Equal(t, []string{"Christopher Nolan"}, m.Directors)
	assert.Equal(t, []string{"Christopher Nolan", "Kai Bird"}, m.Writers, "编剧是 Writing 部门的")
	require.Len(t, m.Cast, 2, "没有名字的演员不要")
	assert.Equal(t, "Cillian Murphy", m.Cast[0].Name, "按 order 排")
	assert.Equal(t, srv.URL+"/t/p/w185/c.jpg", c.ProfileURL(m.Cast[0].ProfilePath))
	assert.Empty(t, m.Cast[1].ProfilePath, "头像路径格式不对时不要")
	assert.Empty(t, c.ProfileURL(""))
	assert.Equal(t, map[string]string{"US": "R"}, m.Certifications, "空的分级不要")
	assert.Equal(t, "tt15398776", m.IMDbID)
	assert.Empty(t, m.TVDBIDString())

	_, err = c.Extras(ctx, KindMovie, 872585)
	require.NoError(t, err)
	assert.Equal(t, int32(1), hits.Load(), "第二次读缓存")
	_, err = c.Extras(cachectl.Bypass(ctx), KindMovie, 872585)
	require.NoError(t, err)
	assert.Equal(t, int32(2), hits.Load(), "重新刮削时不读缓存")
	_, err = c.Extras(ctx, KindMovie, 872585)
	require.NoError(t, err)
	assert.Equal(t, int32(2), hits.Load(), "取到的新响应写回缓存")

	tv, err := c.Extras(ctx, KindTV, 100088)
	require.NoError(t, err)
	assert.Equal(t, []string{"HBO"}, tv.Studios, "剧集用电视网")
	assert.Equal(t, "Returning Series", tv.Status)
	assert.Equal(t, map[string]string{"US": "TV-MA"}, tv.Certifications)
	assert.Equal(t, "392256", tv.TVDBIDString())
	assert.Equal(t, "tt3581920", tv.IMDbID)

	_, err = c.Extras(ctx, KindTV, 7)
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = c.Extras(ctx, "book", 1)
	assert.Error(t, err)
}

func TestSeasonEpisodeCrew(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"season_number":1,"air_date":"2023-01-15","episodes":[{"id":11,"episode_number":1,"name":"一","vote_average":8.6,"vote_count":120,
			"crew":[{"name":"Craig Mazin","job":"Director","department":"Directing"},{"name":"Craig Mazin","job":"Writer","department":"Writing"},
			{"name":"Neil Druckmann","job":"Writer","department":"Writing"}]}]}`))
	}))
	t.Cleanup(srv.Close)
	c, err := New(Options{APIKey: "k", BaseURL: srv.URL + "/3", RatePerSecond: 1000})
	require.NoError(t, err)
	s, err := c.Season(context.Background(), 100088, 1)
	require.NoError(t, err)
	e := s.Episode(1)
	require.NotNil(t, e)
	assert.Equal(t, []string{"Craig Mazin"}, e.Directors)
	assert.Equal(t, []string{"Craig Mazin", "Neil Druckmann"}, e.Writers)
	assert.InDelta(t, 8.6, e.VoteAverage, 0.001)
	assert.Equal(t, 120, e.VoteCount)
	assert.Equal(t, "2023-01-15", s.AirDate)
}
