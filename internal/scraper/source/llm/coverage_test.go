package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

func TestPresets_AllConfigsValid(t *testing.T) {
	t.Parallel()
	require.NotEmpty(t, Presets)
	require.GreaterOrEqual(t, len(Presets), 10)

	for name, preset := range Presets {
		require.NotEmpty(t, preset.BaseURL, "preset %s missing BaseURL", name)
		require.NotEmpty(t, preset.DefaultModels, "preset %s missing DefaultModels", name)
		for _, model := range preset.DefaultModels {
			require.NotEmpty(t, model, "preset %s has empty model", name)
		}
	}
}

func TestPresets_ExpectedProviders(t *testing.T) {
	t.Parallel()
	wanted := []string{"openai", "kimi", "glm", "qwen", "deepseek", "doubao", "yi", "baichuan", "groq", "ollama"}
	for _, name := range wanted {
		_, ok := Presets[name]
		require.True(t, ok, "missing preset %s", name)
	}
}

func TestAnthropicProvider_Name_Kind_Close(t *testing.T) {
	t.Parallel()
	p, err := NewAnthropicProvider(AnthropicConfig{APIKey: "sk-test"})
	require.NoError(t, err)
	require.Equal(t, "anthropic", p.Name())
	require.Equal(t, KindAnthropic, p.Kind())
	require.NoError(t, p.Close())
}

func TestNewAnthropicProvider_NoAPIKey(t *testing.T) {
	t.Parallel()
	_, err := NewAnthropicProvider(AnthropicConfig{})
	require.Error(t, err)
}

func TestNewAnthropicProvider_WithCustomModel(t *testing.T) {
	t.Parallel()
	p, err := NewAnthropicProvider(AnthropicConfig{APIKey: "sk-test", Model: "claude-custom"})
	require.NoError(t, err)
	require.Equal(t, "claude-custom", p.model)
}

func TestAnthropicProvider_Extract_StructuredSuccess(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.URL.Path, "/messages")
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"id":   "msg_1",
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{"type": "text", "text": `{"title":"Inception","year":2010,"type":"movie"}`},
			},
			"model":       "claude",
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 10, "output_tokens": 20},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	transport := &hostRewriteTransport{target: srv.URL, inner: srv.Client().Transport}
	httpClient := &http.Client{Transport: transport}
	p2, err := NewAnthropicProvider(AnthropicConfig{APIKey: "sk-test", HTTPClient: httpClient})
	require.NoError(t, err)

	result, err := p2.Extract(context.Background(), ExtractRequest{RawTitle: "Inception 2010"})
	require.NoError(t, err)
	require.Equal(t, "Inception", result.Title)
	require.Equal(t, 2010, result.Year)
	require.Equal(t, "anthropic", result.GeneratedBy)
	require.False(t, result.GeneratedAt.IsZero())
}

func TestAnthropicProvider_Extract_FallbackAfterBetaFail(t *testing.T) {
	t.Parallel()
	var betaCalls, stdCalls int
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("anthropic-beta"), "structured-outputs") {
			betaCalls++
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"beta unsupported"}}`))
			return
		}
		stdCalls++
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"id":   "msg_1",
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{"type": "text", "text": `{"title":"Inception","year":2010}`},
			},
			"model":       "claude",
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 10, "output_tokens": 20},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	transport := &hostRewriteTransport{target: srv.URL, inner: srv.Client().Transport}
	httpClient := &http.Client{Transport: transport}
	p, err := NewAnthropicProvider(AnthropicConfig{APIKey: "sk-test", HTTPClient: httpClient})
	require.NoError(t, err)

	result, err := p.Extract(context.Background(), ExtractRequest{RawTitle: "Inception"})
	require.NoError(t, err)
	require.Equal(t, "Inception", result.Title)
	require.Greater(t, betaCalls, 0)
	require.Greater(t, stdCalls, 0)
}

func TestAnthropicProvider_Extract_BothFail(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"server","message":"boom"}}`))
	}))
	defer srv.Close()

	transport := &hostRewriteTransport{target: srv.URL, inner: srv.Client().Transport}
	httpClient := &http.Client{Transport: transport}
	p, err := NewAnthropicProvider(AnthropicConfig{APIKey: "sk-test", HTTPClient: httpClient})
	require.NoError(t, err)

	_, err = p.Extract(context.Background(), ExtractRequest{RawTitle: "X"})
	require.Error(t, err)
}

type hostRewriteTransport struct {
	target string
	inner  http.RoundTripper
}

func (h *hostRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	targetURL, err := parseTargetURL(h.target)
	if err != nil {
		return nil, err
	}
	req.URL.Scheme = targetURL.Scheme
	req.URL.Host = targetURL.Host
	req.Host = targetURL.Host
	if h.inner == nil {
		return http.DefaultTransport.RoundTrip(req)
	}
	return h.inner.RoundTrip(req)
}

func parseTargetURL(target string) (*httpURL, error) {
	if !strings.HasPrefix(target, "http") {
		return nil, fmt.Errorf("bad target: %s", target)
	}
	parts := strings.SplitN(strings.TrimPrefix(strings.TrimPrefix(target, "https://"), "http://"), "/", 2)
	scheme := "http"
	if strings.HasPrefix(target, "https://") {
		scheme = "https"
	}
	return &httpURL{Scheme: scheme, Host: parts[0]}, nil
}

type httpURL struct {
	Scheme string
	Host   string
}

func TestOllamaNativeProvider_Name_Kind_Close(t *testing.T) {
	t.Parallel()
	p, err := NewOllamaNativeProvider(OllamaConfig{Model: "qwen2.5:7b"})
	require.NoError(t, err)
	require.Equal(t, "ollama-native", p.Name())
	require.Equal(t, KindOllamaNative, p.Kind())
	require.NoError(t, p.Close())
}

func TestNewOllamaNativeProvider_DefaultModel(t *testing.T) {
	t.Parallel()
	p, err := NewOllamaNativeProvider(OllamaConfig{})
	require.NoError(t, err)
	require.NotEmpty(t, p.model)
}

func TestNewOllamaNativeProvider_WithHTTPClient(t *testing.T) {
	t.Parallel()
	p, err := NewOllamaNativeProvider(OllamaConfig{
		BaseURL:    "http://x",
		HTTPClient: &http.Client{},
	})
	require.NoError(t, err)
	require.NotEmpty(t, p.schema)
}

func TestOllamaNativeProvider_Extract(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/chat", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"model": "qwen2.5:7b",
			"message": map[string]any{
				"role":    "assistant",
				"content": `{"title":"Inception","year":2010,"type":"movie"}`,
			},
			"done": true,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p, err := NewOllamaNativeProvider(OllamaConfig{
		BaseURL:    srv.URL,
		Model:      "qwen2.5:7b",
		HTTPClient: srv.Client(),
	})
	require.NoError(t, err)

	result, err := p.Extract(context.Background(), ExtractRequest{RawTitle: "Inception"})
	require.NoError(t, err)
	require.Equal(t, "Inception", result.Title)
	require.Equal(t, 2010, result.Year)
	require.Equal(t, "ollama-native", result.GeneratedBy)
}

func TestOllamaNativeProvider_Extract_HTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p, err := NewOllamaNativeProvider(OllamaConfig{
		BaseURL:    srv.URL,
		Model:      "qwen2.5:7b",
		HTTPClient: srv.Client(),
	})
	require.NoError(t, err)

	_, err = p.Extract(context.Background(), ExtractRequest{RawTitle: "X"})
	require.Error(t, err)
}

func TestOllamaNativeProvider_Extract_InvalidJSONResponse(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{"role": "assistant", "content": "not-json"},
			"done":    true,
		})
	}))
	defer srv.Close()

	p, err := NewOllamaNativeProvider(OllamaConfig{
		BaseURL:    srv.URL,
		Model:      "qwen2.5:7b",
		HTTPClient: srv.Client(),
	})
	require.NoError(t, err)

	_, err = p.Extract(context.Background(), ExtractRequest{RawTitle: "X"})
	require.Error(t, err)
}

func TestChainedProvider_Fallback(t *testing.T) {
	t.Parallel()
	first := &stubSourceProvider{err: errors.New("first failed")}
	second := &stubSourceProvider{result: &NFOResult{Title: "second wins"}}

	chain := NewChainedProvider(first, second)
	require.Equal(t, "chained", chain.Name())
	require.Equal(t, KindChained, chain.Kind())

	result, err := chain.Extract(context.Background(), ExtractRequest{RawTitle: "X"})
	require.NoError(t, err)
	require.Equal(t, "second wins", result.Title)
	require.NoError(t, chain.Close())
}

func TestChainedProvider_AllFail(t *testing.T) {
	t.Parallel()
	first := &stubSourceProvider{err: errors.New("first failed")}
	second := &stubSourceProvider{err: errors.New("second failed")}

	chain := NewChainedProvider(first, second)
	_, err := chain.Extract(context.Background(), ExtractRequest{RawTitle: "X"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "all providers failed")
}

func TestChainedProvider_Close_WithErrors(t *testing.T) {
	t.Parallel()
	first := &closeErrProvider{name: "first", closeErr: errors.New("close-1")}
	second := &closeErrProvider{name: "second"}
	chain := NewChainedProvider(first, second)
	err := chain.Close()
	require.Error(t, err)
}

type closeErrProvider struct {
	name     string
	closeErr error
}

func (c *closeErrProvider) Name() string       { return c.name }
func (c *closeErrProvider) Kind() ProviderKind { return KindOpenAICompat }
func (c *closeErrProvider) Extract(context.Context, ExtractRequest) (*NFOResult, error) {
	return nil, nil
}
func (c *closeErrProvider) Close() error { return c.closeErr }

func TestValidateAgainstTMDB_Nil(t *testing.T) {
	t.Parallel()
	res, w := ValidateAgainstTMDB(context.Background(), nil, nil, "movie")
	require.Nil(t, res)
	require.Empty(t, w)
}

func TestValidateAgainstTMDB_ZeroID(t *testing.T) {
	t.Parallel()
	validator := stubTMDBValidator{}
	res, w := ValidateAgainstTMDB(context.Background(), &NFOResult{Title: "T", TMDBID: 0}, validator, "movie")
	require.Empty(t, w)
	require.Equal(t, 0, res.TMDBID)
}

func TestValidateAgainstTMDB_UnknownMediaType(t *testing.T) {
	t.Parallel()
	validator := stubTMDBValidator{}
	res, w := ValidateAgainstTMDB(context.Background(), &NFOResult{Title: "T", TMDBID: 1}, validator, "podcast")
	require.Empty(t, w)
	require.Equal(t, 1, res.TMDBID)
}

func TestValidateAgainstTMDB_TVSuccess(t *testing.T) {
	t.Parallel()
	validator := stubTMDBValidator{tv: map[int]*ValidationInfo{100: {Title: "Breaking Bad", Year: 2008}}}
	res, _ := ValidateAgainstTMDB(context.Background(), &NFOResult{Title: "Breaking Bad", Year: 2008, TMDBID: 100}, validator, "tv")
	require.Equal(t, 100, res.TMDBID)
}

func TestValidateAgainstTMDB_TVNilInfo(t *testing.T) {
	t.Parallel()
	validator := stubTMDBValidator{tv: map[int]*ValidationInfo{}}
	res, w := ValidateAgainstTMDB(context.Background(), &NFOResult{Title: "X", TMDBID: 100}, validator, "tv")
	require.Equal(t, 0, res.TMDBID)
	require.Equal(t, "TMDB 验证失败，清空 ID", w)
}

func TestValidateAgainstTMDB_YearMismatch(t *testing.T) {
	t.Parallel()
	validator := stubTMDBValidator{movie: map[int]*ValidationInfo{1: {Title: "Inception", Year: 2010}}}
	res, w := ValidateAgainstTMDB(context.Background(), &NFOResult{Title: "Inception", Year: 2020, TMDBID: 1}, validator, "movie")
	require.Equal(t, 0, res.TMDBID)
	require.Contains(t, w, "年份")
}

func TestValidateAgainstTMDB_YearCloseMatch(t *testing.T) {
	t.Parallel()
	validator := stubTMDBValidator{movie: map[int]*ValidationInfo{1: {Title: "Inception", Year: 2010}}}
	res, _ := ValidateAgainstTMDB(context.Background(), &NFOResult{Title: "Inception", Year: 2011, TMDBID: 1}, validator, "movie")
	require.Equal(t, 1, res.TMDBID)
}

func TestValidateFieldFormat_Nil(t *testing.T) {
	t.Parallel()
	require.Empty(t, ValidateFieldFormat(nil))
}

func TestValidateFieldFormat_ValidData(t *testing.T) {
	t.Parallel()
	require.Empty(t, ValidateFieldFormat(&NFOResult{Year: 2020, TMDBID: 100, IMDBID: "tt1234567"}))
}

func TestValidateFieldFormat_OldYear(t *testing.T) {
	t.Parallel()
	w := ValidateFieldFormat(&NFOResult{Year: 1800})
	require.Contains(t, w, "year < 1900")
}

func TestAbs(t *testing.T) {
	t.Parallel()
	require.Equal(t, 5, abs(-5))
	require.Equal(t, 5, abs(5))
	require.Equal(t, 0, abs(0))
}

func TestLLMSource_IsActive(t *testing.T) {
	t.Parallel()
	src, err := NewLLMSource(SourceConfig{Provider: &stubSourceProvider{}})
	require.NoError(t, err)
	require.True(t, src.IsActive())

	var nilSrc *LLMSource
	require.False(t, nilSrc.IsActive())
}

func TestLLMSource_SearchTvShow(t *testing.T) {
	t.Parallel()
	src, _ := NewLLMSource(SourceConfig{Provider: &stubSourceProvider{}})
	results, err := src.SearchTvShow(context.Background(), core.TvShowSearchOptions{Query: "Breaking Bad", Year: 2008})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "Breaking Bad", results[0].Title)
	require.Equal(t, 2008, results[0].Year)

	noResults, err := src.SearchTvShow(context.Background(), core.TvShowSearchOptions{})
	require.NoError(t, err)
	require.Nil(t, noResults)
}

func TestLLMSource_SearchTvShow_FirstAirYearPrefers(t *testing.T) {
	t.Parallel()
	src, _ := NewLLMSource(SourceConfig{Provider: &stubSourceProvider{}})
	results, err := src.SearchTvShow(context.Background(), core.TvShowSearchOptions{Query: "Show", FirstAirYear: 2020, Year: 2010})
	require.NoError(t, err)
	require.Equal(t, 2020, results[0].Year)
}

func TestBuildMovieHints_EmptyReturnsNil(t *testing.T) {
	t.Parallel()
	require.Nil(t, buildMovieHints(core.MovieSearchOptions{}))
	hints := buildMovieHints(core.MovieSearchOptions{TMDBID: 1, IMDBID: "tt1", Year: 2020})
	require.Equal(t, "1", hints["tmdb_id"])
	require.Equal(t, "tt1", hints["imdb_id"])
	require.Equal(t, "2020", hints["year"])
}

func TestBuildTVShowHints_Variants(t *testing.T) {
	t.Parallel()
	require.Nil(t, buildTVShowHints(core.TvShowSearchOptions{}))
	hints := buildTVShowHints(core.TvShowSearchOptions{TMDBID: 1, TVDBID: 2, IMDBID: "tt1", FirstAirYear: 2020})
	require.Equal(t, "1", hints["tmdb_id"])
	require.Equal(t, "2", hints["tvdb_id"])
	require.Equal(t, "2020", hints["first_air_year"])

	hintsYear := buildTVShowHints(core.TvShowSearchOptions{Year: 2019})
	require.Equal(t, "2019", hintsYear["year"])
}

func TestCloneStrings_Full(t *testing.T) {
	t.Parallel()
	require.Nil(t, cloneStrings(nil))
	require.Nil(t, cloneStrings([]string{}))
	original := []string{"a", "b"}
	cloned := cloneStrings(original)
	cloned[0] = "changed"
	require.Equal(t, "a", original[0])
}

func TestBuildPeople_Empty(t *testing.T) {
	t.Parallel()
	require.Nil(t, buildPeople(nil, core.PersonTypeActor))
	require.Nil(t, buildPeople([]string{}, core.PersonTypeActor))
	require.Nil(t, buildPeople([]string{"", "  "}, core.PersonTypeActor))
}

func TestBuildIDs_LLMScraper(t *testing.T) {
	t.Parallel()
	ids := buildIDs(nil)
	require.Empty(t, ids)

	full := buildIDs(&NFOResult{TMDBID: 100, IMDBID: "tt1"})
	require.Equal(t, "100", full["tmdb"])
	require.Equal(t, "tt1", full["imdb"])
}

func TestSamplingProvider_Metadata(t *testing.T) {
	t.Parallel()
	p := NewSamplingProvider()
	require.Equal(t, "mcp-sampling", p.Name())
	require.Equal(t, KindMCPSampling, p.Kind())
	require.NoError(t, p.Close())
}

func TestSamplingProvider_Extract_NoSession(t *testing.T) {
	t.Parallel()
	p := NewSamplingProvider()
	_, err := p.Extract(context.Background(), ExtractRequest{RawTitle: "X"})
	require.ErrorIs(t, err, ErrNoMCPSession)
}

func TestOpenAICompat_Name_Kind_Close(t *testing.T) {
	t.Parallel()
	p, err := NewOpenAICompatProvider(Config{PresetName: "openai", APIKey: "sk"})
	require.NoError(t, err)
	require.Equal(t, "openai", p.Name())
	require.Equal(t, KindOpenAICompat, p.Kind())
	require.NoError(t, p.Close())
}

func TestNewOpenAICompatProvider_NoAPIKey(t *testing.T) {
	t.Parallel()
	_, err := NewOpenAICompatProvider(Config{PresetName: "openai"})
	require.Error(t, err)
}

func TestNewOpenAICompatProvider_OllamaNoAPIKey(t *testing.T) {
	t.Parallel()
	p, err := NewOpenAICompatProvider(Config{PresetName: "ollama", Model: "test"})
	require.NoError(t, err)
	require.Equal(t, "ollama", p.Name())
}

func TestNewOpenAICompatProvider_NoModel(t *testing.T) {
	t.Parallel()
	_, err := NewOpenAICompatProvider(Config{APIKey: "sk"})
	require.Error(t, err)
}

func TestNewOpenAICompatProvider_StrictOverride(t *testing.T) {
	t.Parallel()
	strict := false
	p, err := NewOpenAICompatProvider(Config{PresetName: "openai", APIKey: "sk", SupportsStrictSchema: &strict})
	require.NoError(t, err)
	require.False(t, p.strict)
}

func TestGenerateNFOSchema_HasProperties(t *testing.T) {
	t.Parallel()
	schema := generateNFOSchema()
	require.NotEmpty(t, schema)
	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, props, "title")
	require.Contains(t, props, "year")
}

func TestDecodeResultPayload_CodeFence(t *testing.T) {
	t.Parallel()
	result, err := decodeResultPayload("```json\n{\"title\":\"X\"}\n```")
	require.NoError(t, err)
	require.Equal(t, "X", result.Title)
}

func TestDecodeResultPayload_BadJSON(t *testing.T) {
	t.Parallel()
	_, err := decodeResultPayload("not-json")
	require.Error(t, err)
}
