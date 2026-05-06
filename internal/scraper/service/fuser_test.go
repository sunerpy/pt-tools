package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

func TestFuser_MergeMovie(t *testing.T) {
	ctx := context.Background()
	releaseDate := time.Date(2010, time.July, 16, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		fuser   *DefaultFuser
		sources map[string]*core.RawMediaInfo
		check   func(t *testing.T, got *core.Movie, err error)
	}{
		{
			name:  "tmdb only",
			fuser: NewDefaultFuser(),
			sources: map[string]*core.RawMediaInfo{
				"tmdb": {Provider: "tmdb", Data: &core.Movie{MediaEntity: core.MediaEntity{Title: "Inception", OriginalTitle: "Inception", Year: 2010, Genres: []string{"Sci-Fi"}, IDs: map[string]string{"tmdb": "27205"}, Provider: "tmdb"}, ReleaseDate: releaseDate}},
			},
			check: func(t *testing.T, got *core.Movie, err error) {
				require.NoError(t, err)
				require.Equal(t, "Inception", got.Title)
				require.Equal(t, "Inception", got.OriginalTitle)
				require.Equal(t, 2010, got.Year)
				require.Equal(t, "tmdb", got.Sources["title"])
			},
		},
		{
			name:  "douban only",
			fuser: NewDefaultFuser(),
			sources: map[string]*core.RawMediaInfo{
				"douban": {Provider: "douban", Data: &core.Movie{MediaEntity: core.MediaEntity{Title: "霸王别姬", OriginalTitle: "Farewell My Concubine", Year: 1993, Plot: "豆瓣简介", IDs: map[string]string{"douban": "1291546"}, Provider: "douban"}, Top250: 2}},
			},
			check: func(t *testing.T, got *core.Movie, err error) {
				require.NoError(t, err)
				require.Equal(t, "霸王别姬", got.Title)
				require.Equal(t, 2, got.Top250)
				require.Equal(t, "douban", got.Sources["title"])
			},
		},
		{
			name:  "both no conflict and chinese title preferred",
			fuser: NewDefaultFuser(),
			sources: map[string]*core.RawMediaInfo{
				"tmdb":   {Provider: "tmdb", Data: &core.Movie{MediaEntity: core.MediaEntity{Title: "Inception", OriginalTitle: "Inception", Plot: "tmdb", Provider: "tmdb"}, ReleaseDate: releaseDate}},
				"douban": {Provider: "douban", Data: &core.Movie{MediaEntity: core.MediaEntity{Title: "盗梦空间", OriginalTitle: "Inception", Plot: "豆瓣", Provider: "douban"}}},
			},
			check: func(t *testing.T, got *core.Movie, err error) {
				require.NoError(t, err)
				require.Equal(t, "盗梦空间", got.Title)
				require.Equal(t, "Inception", got.OriginalTitle)
				require.Equal(t, "douban", got.Sources["title"])
				require.Equal(t, "tmdb", got.Sources["original_title"])
			},
		},
		{
			name:  "both conflict chinese title preferred",
			fuser: NewDefaultFuser(),
			sources: map[string]*core.RawMediaInfo{
				"tmdb":   {Provider: "tmdb", Data: &core.Movie{MediaEntity: core.MediaEntity{Title: "刺激1995", OriginalTitle: "The Shawshank Redemption", Provider: "tmdb"}}},
				"douban": {Provider: "douban", Data: &core.Movie{MediaEntity: core.MediaEntity{Title: "肖申克的救赎", Provider: "douban"}}},
			},
			check: func(t *testing.T, got *core.Movie, err error) {
				require.NoError(t, err)
				require.Equal(t, "肖申克的救赎", got.Title)
				require.Equal(t, "tmdb", got.Sources["original_title"])
			},
		},
		{
			name:  "genres ids ratings merged and llm artwork dropped",
			fuser: NewDefaultFuser(),
			sources: map[string]*core.RawMediaInfo{
				"tmdb":   {Provider: "tmdb", Data: &core.Movie{MediaEntity: core.MediaEntity{Title: "Inception", OriginalTitle: "Inception", Genres: []string{"Sci-Fi", "Action"}, IDs: map[string]string{"imdb": "tt1375666", "tmdb": "27205"}, Ratings: map[string]core.MediaRating{"tmdb": {ID: "tmdb", Value: 8.3, Votes: 10, Max: 10}}, ArtworkURLs: map[core.ArtworkType]string{core.ArtworkTypePoster: "https://tmdb/poster.jpg"}, Provider: "tmdb"}}},
				"douban": {Provider: "douban", Data: &core.Movie{MediaEntity: core.MediaEntity{Title: "盗梦空间", Genres: []string{"动作", "剧情"}, IDs: map[string]string{"douban": "3541415"}, Ratings: map[string]core.MediaRating{"douban": {ID: "douban", Value: 9.4, Votes: 10, Max: 10}}, ArtworkURLs: map[core.ArtworkType]string{core.ArtworkTypePoster: "https://douban/poster.jpg"}, Provider: "douban"}}},
				"llm":    {Provider: "llm", Data: &core.Movie{MediaEntity: core.MediaEntity{Title: "LLM Title", ArtworkURLs: map[core.ArtworkType]string{core.ArtworkTypePoster: "https://llm/poster.jpg", core.ArtworkTypeBackground: "https://llm/fanart.jpg"}, Provider: "llm"}}},
			},
			check: func(t *testing.T, got *core.Movie, err error) {
				require.NoError(t, err)
				require.Equal(t, []string{"Sci-Fi", "Action", "动作", "剧情"}, got.Genres)
				require.Equal(t, map[string]string{"imdb": "tt1375666", "tmdb": "27205", "douban": "3541415"}, got.IDs)
				require.Contains(t, got.Ratings, "tmdb")
				require.Contains(t, got.Ratings, "douban")
				require.Equal(t, "https://tmdb/poster.jpg", got.ArtworkURLs[core.ArtworkTypePoster])
				require.NotContains(t, got.ArtworkURLs, core.ArtworkTypeBackground)
				require.Equal(t, "tmdb,douban", got.Sources["ratings"])
			},
		},
		{
			name:  "user override",
			fuser: NewDefaultFuserWithOverrides(map[string]string{"title": "自定义"}),
			sources: map[string]*core.RawMediaInfo{
				"tmdb":   {Provider: "tmdb", Data: &core.Movie{MediaEntity: core.MediaEntity{Title: "Inception", OriginalTitle: "Inception", Provider: "tmdb"}}},
				"douban": {Provider: "douban", Data: &core.Movie{MediaEntity: core.MediaEntity{Title: "盗梦空间", Provider: "douban"}}},
			},
			check: func(t *testing.T, got *core.Movie, err error) {
				require.NoError(t, err)
				require.Equal(t, "自定义", got.Title)
				require.Equal(t, "override", got.Sources["title"])
			},
		},
		{
			name:    "neither available",
			fuser:   NewDefaultFuser(),
			sources: map[string]*core.RawMediaInfo{},
			check: func(t *testing.T, got *core.Movie, err error) {
				require.Error(t, err)
				require.Nil(t, got)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fuser.Merge(ctx, tt.sources)
			tt.check(t, got, err)
			if err == nil {
				require.Equal(t, "fused", got.Provider)
				require.False(t, got.ScrapedAt.IsZero())
			}
		})
	}
}

func TestFuser_MergeTv(t *testing.T) {
	ctx := context.Background()
	f := NewDefaultFuser()
	firstAired := time.Date(2016, time.July, 15, 0, 0, 0, 0, time.UTC)

	got, err := f.MergeTv(ctx, map[string]*core.RawMediaInfo{
		"tmdb":   {Provider: "tmdb", Data: &core.TvShow{MediaEntity: core.MediaEntity{Title: "Stranger Things", OriginalTitle: "Stranger Things", Tags: []string{"80s"}, Provider: "tmdb"}, FirstAired: firstAired, Status: core.ShowStatusReturning, EpisodeGroupKind: core.EpisodeGroupAired, SeasonNames: map[int]string{1: "Season 1"}, SeasonPlots: map[int]string{1: "Plot 1"}}},
		"douban": {Provider: "douban", Data: &core.TvShow{MediaEntity: core.MediaEntity{Title: "怪奇物语", Plot: "豆瓣简介", Provider: "douban"}, SeasonNames: map[int]string{2: "第二季"}, SeasonPlots: map[int]string{2: "剧情 2"}}},
	})
	require.NoError(t, err)
	require.Equal(t, "怪奇物语", got.Title)
	require.Equal(t, firstAired, got.FirstAired)
	require.Equal(t, core.ShowStatusReturning, got.Status)
	require.Equal(t, map[int]string{1: "Season 1", 2: "第二季"}, got.SeasonNames)
	require.Equal(t, map[int]string{1: "Plot 1", 2: "剧情 2"}, got.SeasonPlots)
	require.Equal(t, "tmdb,douban", got.Sources["season_names"])
	require.Equal(t, "fused", got.Provider)
}

func TestFuser_MergeEpisode(t *testing.T) {
	ctx := context.Background()
	f := NewDefaultFuser()
	firstAired := time.Date(2023, time.March, 4, 0, 0, 0, 0, time.UTC)

	got, err := f.MergeEpisode(ctx, map[string]*core.RawMediaInfo{
		"tmdb":   {Provider: "tmdb", Data: &core.TvShowEpisode{MediaEntity: core.MediaEntity{Title: "Episode 1", Plot: "TMDB", ArtworkURLs: map[core.ArtworkType]string{core.ArtworkTypeThumb: "https://tmdb/thumb.jpg"}, Provider: "tmdb"}, Season: 1, Episode: 1, FirstAired: firstAired}},
		"douban": {Provider: "douban", Data: &core.TvShowEpisode{MediaEntity: core.MediaEntity{Title: "第一集", Plot: "豆瓣", Provider: "douban"}}},
		"llm":    {Provider: "llm", Data: &core.TvShowEpisode{MediaEntity: core.MediaEntity{Title: "LLM Episode", ArtworkURLs: map[core.ArtworkType]string{core.ArtworkTypeThumb: "https://llm/thumb.jpg"}, Provider: "llm"}}},
	})
	require.NoError(t, err)
	require.Equal(t, "第一集", got.Title)
	require.Equal(t, 1, got.Season)
	require.Equal(t, 1, got.Episode)
	require.Equal(t, firstAired, got.FirstAired)
	require.Equal(t, "https://tmdb/thumb.jpg", got.ArtworkURLs[core.ArtworkTypeThumb])
	require.Equal(t, "tmdb", got.Sources["first_aired"])
	require.Equal(t, "fused", got.Provider)
}
