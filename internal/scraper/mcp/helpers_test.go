package mcp

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
	"github.com/sunerpy/pt-tools/internal/scraper/store"
)

func TestNormalizeMediaType(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"movie":   "movie",
		"MOVIE":   "movie",
		"tv":      "tv",
		"tvshow":  "tv",
		"tv_show": "tv",
		"show":    "tv",
		"series":  "tv",
		"episode": "episode",
		"":        "",
		"garbage": "",
	}
	for in, want := range cases {
		assert.Equal(t, want, normalizeMediaType(in), "input=%q", in)
	}
}

func TestToCoreMediaType(t *testing.T) {
	t.Parallel()
	assert.Equal(t, core.MediaTypeMovie, toCoreMediaType("movie"))
	assert.Equal(t, core.MediaTypeTvShow, toCoreMediaType("tv"))
	assert.Equal(t, core.MediaTypeEpisode, toCoreMediaType("episode"))
	assert.Equal(t, core.MediaTypeUnknown, toCoreMediaType("invalid"))
}

func TestParseArtworkTypes(t *testing.T) {
	t.Parallel()
	assert.Nil(t, parseArtworkTypes(nil))
	got := parseArtworkTypes([]string{"poster", "fanart", "background", "banner", "clearlogo", "thumb", "landscape", "unknown"})
	assert.Len(t, got, 7)
	assert.Contains(t, got, core.ArtworkTypePoster)
	assert.Contains(t, got, core.ArtworkTypeBackground)
	assert.Contains(t, got, core.ArtworkTypeBanner)
	assert.Contains(t, got, core.ArtworkTypeClearlogo)
	assert.Contains(t, got, core.ArtworkTypeThumb)
}

func TestDedupeStrings(t *testing.T) {
	t.Parallel()
	got := dedupeStrings([]string{" a ", "a", "", "b", "b", "c"})
	assert.Equal(t, []string{"a", "b", "c"}, got)
}

func TestSelectedProviders(t *testing.T) {
	t.Parallel()
	assert.Nil(t, selectedProviders(nil, nil))

	reg := core.NewRegistry[core.MediaScraper]()
	require.NoError(t, reg.Register("alpha", func() core.MediaScraper { return nil }))
	require.NoError(t, reg.Register("beta", func() core.MediaScraper { return nil }))

	all := selectedProviders(reg, nil)
	assert.ElementsMatch(t, []string{"alpha", "beta"}, all)

	got := selectedProviders(reg, []string{" alpha ", "alpha", "gamma"})
	assert.Equal(t, []string{"alpha", "gamma"}, got)
}

func TestToSearchCandidate(t *testing.T) {
	t.Parallel()
	cand := core.MediaSearchCandidate{
		ID:        "42",
		Title:     "Answer",
		Year:      2024,
		MediaType: core.MediaTypeMovie,
		Provider:  "tmdb",
		PosterURL: "https://p/1.jpg",
		Overview:  "A summary",
		Score:     0.8,
	}
	got := toSearchCandidate(cand)
	assert.Equal(t, "42", got.ID)
	assert.Equal(t, "movie", got.MediaType)
	assert.Equal(t, 0.8, got.Score)
}

func TestToLibraryAndTaskInfo(t *testing.T) {
	t.Parallel()
	lib := store.MediaLibraryConfig{ID: 1, Name: "Movies", Type: "movie", Path: "/m"}
	info := toLibraryInfo(lib)
	assert.Equal(t, uint(1), info.ID)
	assert.Equal(t, "Movies", info.Name)

	task := store.ScrapeTask{ID: 7, TaskType: "movie", State: "running", CurrentStage: "searching", Progress: 42}
	ti := toTaskInfo(task)
	assert.Equal(t, uint(7), ti.ID)
	assert.Equal(t, "running", ti.State)
	assert.InDelta(t, 42.0, ti.Progress, 0.001)
}

func TestMetadataToMap(t *testing.T) {
	t.Parallel()
	assert.Equal(t, map[string]any{}, metadataToMap(nil))

	m := metadataToMap(struct {
		Title string `json:"title"`
	}{Title: "ok"})
	assert.Equal(t, "ok", m["title"])

	type bad struct{ C chan int }
	m2 := metadataToMap(bad{C: make(chan int)})
	_, hasRaw := m2["raw"]
	assert.True(t, hasRaw)
}

func TestValidateMetadata(t *testing.T) {
	t.Parallel()
	warnings := validateMetadata(MetadataResult{})
	assert.Contains(t, warnings, "title is empty")

	warnings = validateMetadata(MetadataResult{
		Title:   "OK",
		Year:    1800,
		TMDBID:  -1,
		IMDBID:  "nope",
		Type:    "badtype",
		Season:  -1,
		Episode: -1,
	})
	assert.Contains(t, warnings, "year out of range")
	assert.Contains(t, warnings, "tmdb_id out of range")
	assert.Contains(t, warnings, "imdb_id format invalid")
	assert.Contains(t, warnings, "type must be one of movie, tv, unknown")
	assert.Contains(t, warnings, "season must be >= 0")
	assert.Contains(t, warnings, "episode must be >= 0")

	warnings = validateMetadata(MetadataResult{Title: "OK", Year: 2020, IMDBID: "tt1234567", Type: "movie"})
	assert.Empty(t, warnings)
}

func TestMergeMetadata(t *testing.T) {
	t.Parallel()
	base := MetadataResult{Title: "Base", Year: 2000}
	result := mergeMetadata(base, nil)
	assert.Equal(t, base, result)

	patch := &MetadataResult{
		Title:         "New",
		OriginalTitle: "NewO",
		Year:          2024,
		Type:          "movie",
		TMDBID:        1,
		IMDBID:        "tt9999999",
		Season:        2,
		Episode:       3,
		Genres:        []string{"Drama"},
		Language:      "en",
		Plot:          "plot",
		Directors:     []string{"A"},
		Cast:          []string{"B"},
		Runtime:       120,
		GeneratedBy:   "llm",
	}
	merged := mergeMetadata(base, patch)
	assert.Equal(t, "New", merged.Title)
	assert.Equal(t, "NewO", merged.OriginalTitle)
	assert.Equal(t, 2024, merged.Year)
	assert.Equal(t, "movie", merged.Type)
	assert.Equal(t, 1, merged.TMDBID)
	assert.Equal(t, "tt9999999", merged.IMDBID)
	assert.Equal(t, 2, merged.Season)
	assert.Equal(t, 3, merged.Episode)
	assert.Equal(t, []string{"Drama"}, merged.Genres)
	assert.Equal(t, "en", merged.Language)
	assert.Equal(t, "plot", merged.Plot)
	assert.Equal(t, []string{"A"}, merged.Directors)
	assert.Equal(t, []string{"B"}, merged.Cast)
	assert.Equal(t, 120, merged.Runtime)
	assert.Equal(t, "llm", merged.GeneratedBy)
}

func TestLLMRequestFromMetadata(t *testing.T) {
	t.Parallel()
	out := llmRequestFromMetadata(MetadataResult{Title: "T", Language: "en", Type: "movie"}, "/a/b.mkv", "extra")
	assert.Equal(t, "T", out.RawTitle)
	assert.Equal(t, "b.mkv", out.Filename)
	assert.Contains(t, out.UserContext, "extra")
	assert.Contains(t, out.UserContext, "Partial metadata JSON")
	assert.Equal(t, "en", out.Language)
	assert.Equal(t, "movie", out.MediaType)

	out = llmRequestFromMetadata(MetadataResult{}, "/x/y.mkv", "")
	assert.Equal(t, "y.mkv", out.RawTitle)
}

func TestBuildNFOPaths(t *testing.T) {
	t.Parallel()
	nfo, movieNfo := buildMovieNFOPaths("/a/b/c.mkv")
	assert.Equal(t, "/a/b/c.nfo", nfo)
	assert.Equal(t, filepath.Join("/a/b", "movie.nfo"), movieNfo)

	assert.Equal(t, "/a/b/c.nfo", buildEpisodeNFOPath("/a/b/c.mkv"))
	assert.Equal(t, filepath.Join("/a/b", "tvshow.nfo"), buildTvShowNFOPath("/a/b/c.mkv"))
}

func TestCoreFromMetadata(t *testing.T) {
	t.Parallel()
	meta := MetadataResult{
		Title: "T", OriginalTitle: "OT", Year: 2020, Plot: "p",
		Genres: []string{"g"}, IMDBID: "tt1234567", TMDBID: 1,
		Runtime: 100, GeneratedBy: "llm", Season: 2, Episode: 3,
	}
	movie := coreMovieFromMetadata(meta, "/a/b.mkv")
	require.NotNil(t, movie)
	assert.Equal(t, "T", movie.Title)
	assert.Equal(t, "/a/b.mkv", movie.Path)
	assert.Equal(t, "tt1234567", movie.IDs["imdb"])
	assert.Equal(t, "1", movie.IDs["tmdb"])

	show := coreTvShowFromMetadata(meta, "/a/b.mkv")
	require.NotNil(t, show)
	assert.Equal(t, "T", show.Title)
	assert.Equal(t, "/a", show.Path)
	assert.NotNil(t, show.SeasonNames)

	ep := coreEpisodeFromMetadata(meta, "/a/b.mkv")
	require.NotNil(t, ep)
	assert.Equal(t, 2, ep.Season)
	assert.Equal(t, 3, ep.Episode)
}

func TestToolError(t *testing.T) {
	t.Parallel()
	res := toolError(errors.New("boom"))
	require.NotNil(t, res)
	assert.True(t, res.IsError)
	require.Len(t, res.Content, 1)
}

func TestRequireFns(t *testing.T) {
	t.Parallel()
	assert.Error(t, requireDB(nil))
	assert.Error(t, requireSourceReg(nil))
	assert.Error(t, requireWriterReg(nil))
	assert.Error(t, requireConnectorReg(nil))

	assert.NoError(t, requireSourceReg(core.NewRegistry[core.MediaScraper]()))
	assert.NoError(t, requireWriterReg(core.NewRegistry[core.NfoWriter]()))
	assert.NoError(t, requireConnectorReg(core.NewRegistry[core.MediaServerConnector]()))
}

func TestRequireLLMProvider(t *testing.T) {
	t.Parallel()
	_, err := requireLLMProvider(Deps{}, "")
	assert.Error(t, err)

	p := &fakeLLMProvider{name: "p1"}
	deps := Deps{LLMProviders: map[string]LLMProvider{"p1": p, "p2": &fakeLLMProvider{name: "p2"}}}

	got, err := requireLLMProvider(deps, "p1")
	require.NoError(t, err)
	assert.Equal(t, "p1", got.Name())

	_, err = requireLLMProvider(deps, "missing")
	assert.Error(t, err)

	got, err = requireLLMProvider(deps, "")
	require.NoError(t, err)
	assert.Equal(t, "p1", got.Name())
}

type fakeLLMProvider struct {
	name    string
	kind    string
	result  *MetadataResult
	err     error
	closed  bool
	lastReq LLMExtractRequest
}

func (f *fakeLLMProvider) Name() string { return f.name }
func (f *fakeLLMProvider) Kind() string {
	if f.kind == "" {
		return "stub"
	}
	return f.kind
}

func (f *fakeLLMProvider) Extract(_ context.Context, req LLMExtractRequest) (*MetadataResult, error) {
	f.lastReq = req
	if f.err != nil {
		return nil, f.err
	}
	if f.result != nil {
		r := *f.result
		return &r, nil
	}
	return &MetadataResult{Title: "stubbed", Type: "movie"}, nil
}

func (f *fakeLLMProvider) Close() error { f.closed = true; return nil }

func TestContext_WithSession_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	assert.Nil(t, SessionFromContext(ctx))
	ctx2 := WithSession(ctx, nil)
	assert.Nil(t, SessionFromContext(ctx2))
}
