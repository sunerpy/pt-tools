package tmdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	groupAbsolute = "5b11ba820e0a265a53000ce9"
	groupDVD      = "5c8a1ef80e0a2625f5a4f1b0"
)

func TestEpisodeGroups(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/3/tv/37854/episode_groups" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		assert.Equal(t, "zh-CN", r.URL.Query().Get("language"))
		_, _ = w.Write([]byte(`{"id":37854,"results":[
			{"id":"` + groupAbsolute + `","name":" Absolute Order ","type":2,"description":"绝对集号","group_count":1,"episode_count":1100,"network":null},
			{"id":"` + groupDVD + `","name":"DVD","type":3,"group_count":20,"episode_count":1080,"network":{"id":1,"name":" Fuji TV ","origin_country":"JP"}},
			{"id":"bad","name":"坏编号","type":1}]}`))
	}))
	t.Cleanup(srv.Close)
	c, err := New(Options{APIKey: "k", BaseURL: srv.URL + "/3", RatePerSecond: 1000, Cache: NewDBCache(newDB(t))})
	require.NoError(t, err)
	ctx := context.Background()

	list, err := c.EpisodeGroups(ctx, 37854)
	require.NoError(t, err)
	assert.Equal(t, []EpisodeGroupSummary{
		{ID: groupAbsolute, Name: "Absolute Order", Type: EpisodeGroupAbsolute, Description: "绝对集号", GroupCount: 1, EpisodeCount: 1100},
		{ID: groupDVD, Name: "DVD", Type: EpisodeGroupDVD, GroupCount: 20, EpisodeCount: 1080, Network: "Fuji TV"},
	}, list, "编号不是剧集组写法的不要，名字与平台去掉空白")

	again, err := c.EpisodeGroups(ctx, 37854)
	require.NoError(t, err)
	assert.Equal(t, list, again)
	assert.EqualValues(t, 1, hits.Load(), "第二次读缓存")

	_, err = c.EpisodeGroups(ctx, 1)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = c.EpisodeGroups(ctx, 0)
	require.ErrorIs(t, err, ErrNotFound)
	assert.EqualValues(t, 2, hits.Load(), "编号是 0 时不发请求")
}

func TestEpisodeGroupsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":1399,"results":[]}`))
	}))
	t.Cleanup(srv.Close)
	c, err := New(Options{APIKey: "k", BaseURL: srv.URL + "/3", RatePerSecond: 1000})
	require.NoError(t, err)
	list, err := c.EpisodeGroups(context.Background(), 1399)
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestEpisodeGroup(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/3/tv/episode_group/"+groupDVD {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		assert.Equal(t, "zh-CN", r.URL.Query().Get("language"))
		// 组与集都故意打乱顺序：要按 order 排
		_, _ = w.Write([]byte(`{"id":"` + groupDVD + `","name":"DVD","type":3,"group_count":2,"episode_count":4,
			"network":{"name":"Fuji TV"},
			"groups":[
				{"id":"g2","name":"Disc 2","order":2,"episodes":[
					{"season_number":1,"episode_number":4,"order":1,"name":"第四集"},
					{"season_number":1,"episode_number":3,"order":0,"name":"第三集","air_date":"2020-01-15"}]},
				{"id":"g1","name":"Disc 1","order":1,"episodes":[
					{"season_number":1,"episode_number":1,"order":0},
					{"season_number":1,"episode_number":2,"order":1}]}]}`))
	}))
	t.Cleanup(srv.Close)
	c, err := New(Options{APIKey: "k", BaseURL: srv.URL + "/3", RatePerSecond: 1000, Cache: NewDBCache(newDB(t))})
	require.NoError(t, err)
	ctx := context.Background()

	g, err := c.EpisodeGroup(ctx, groupDVD)
	require.NoError(t, err)
	assert.Equal(t, EpisodeGroupSummary{ID: groupDVD, Name: "DVD", Type: EpisodeGroupDVD, GroupCount: 2, EpisodeCount: 4, Network: "Fuji TV"}, g.EpisodeGroupSummary)
	require.Len(t, g.Groups, 2)
	assert.Equal(t, "Disc 1", g.Groups[0].Name)
	assert.Equal(t, []EpisodeGroupEpisode{{Season: 1, Episode: 1, Order: 0}, {Season: 1, Episode: 2, Order: 1}}, g.Groups[0].Episodes)
	assert.Equal(t, "Disc 2", g.Groups[1].Name)
	assert.Equal(t, []EpisodeGroupEpisode{
		{Season: 1, Episode: 3, Order: 0, Name: "第三集", AirDate: "2020-01-15"},
		{Season: 1, Episode: 4, Order: 1, Name: "第四集"},
	}, g.Groups[1].Episodes)

	again, err := c.EpisodeGroup(ctx, groupDVD)
	require.NoError(t, err)
	assert.Equal(t, g, again)
	assert.EqualValues(t, 1, hits.Load(), "第二次读缓存")

	_, err = c.EpisodeGroup(ctx, groupAbsolute)
	require.ErrorIs(t, err, ErrNotFound)
	assert.EqualValues(t, 2, hits.Load())
	for _, id := range []string{"", "../movie/1", "5B11BA820E0A265A53000CE9", groupDVD + "0"} {
		_, err = c.EpisodeGroup(ctx, id)
		require.ErrorIs(t, err, ErrNotFound, id)
	}
	assert.EqualValues(t, 2, hits.Load(), "编号不是剧集组写法时不发请求")
}

func TestValidEpisodeGroupID(t *testing.T) {
	assert.True(t, ValidEpisodeGroupID(groupAbsolute))
	assert.False(t, ValidEpisodeGroupID(""))
	assert.False(t, ValidEpisodeGroupID("5b11ba820e0a265a53000ce"))
	assert.False(t, ValidEpisodeGroupID("5b11ba820e0a265a53000cez"))
}
