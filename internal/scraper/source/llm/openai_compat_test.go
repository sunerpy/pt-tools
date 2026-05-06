package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAICompat_Extract_MockServer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer testkey", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"id":"cmpl-1","choices":[{"index":0,"message":{"role":"assistant","content":"{\"title\":\"Inception\",\"year\":2010,\"type\":\"movie\"}"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	p, err := NewOpenAICompatProvider(Config{
		PresetName: "openai",
		BaseURL:    srv.URL + "/v1",
		APIKey:     "testkey",
		Model:      "gpt-4o",
		HTTPClient: srv.Client(),
	})
	require.NoError(t, err)

	result, err := p.Extract(ctx, ExtractRequest{RawTitle: "Inception 2010"})
	require.NoError(t, err)
	require.Equal(t, "Inception", result.Title)
	require.Equal(t, 2010, result.Year)
	require.Equal(t, "movie", result.Type)
	require.Equal(t, "openai", result.GeneratedBy)
	require.False(t, result.GeneratedAt.IsZero())
}

func TestOpenAICompat_PresetAutoFillBaseURL(t *testing.T) {
	t.Parallel()

	transport := &captureTransport{t: t}
	httpClient := &http.Client{Transport: transport}
	p, err := NewOpenAICompatProvider(Config{
		PresetName: "kimi",
		APIKey:     "testkey",
		HTTPClient: httpClient,
	})
	require.NoError(t, err)
	require.Equal(t, "moonshot-v1-8k", p.model)

	_, err = p.Extract(context.Background(), ExtractRequest{RawTitle: "Inception"})
	require.NoError(t, err)
	require.Equal(t, "https://api.moonshot.cn/v1/chat/completions", transport.lastURL)
}

func TestOpenAICompat_StrictSchemaVsJSONMode(t *testing.T) {
	t.Parallel()

	t.Run("strict schema", func(t *testing.T) {
		transport := &captureTransport{t: t}
		client := &http.Client{Transport: transport}
		p, err := NewOpenAICompatProvider(Config{PresetName: "openai", APIKey: "testkey", HTTPClient: client})
		require.NoError(t, err)
		_, err = p.Extract(context.Background(), ExtractRequest{RawTitle: "Inception"})
		require.NoError(t, err)
		require.Equal(t, "json_schema", nestedString(t, transport.lastBody, "response_format", "type"))
		require.Equal(t, "nfo_metadata", nestedString(t, transport.lastBody, "response_format", "json_schema", "name"))
	})

	t.Run("json object mode", func(t *testing.T) {
		transport := &captureTransport{t: t}
		client := &http.Client{Transport: transport}
		p, err := NewOpenAICompatProvider(Config{PresetName: "kimi", APIKey: "testkey", HTTPClient: client})
		require.NoError(t, err)
		_, err = p.Extract(context.Background(), ExtractRequest{RawTitle: "Inception"})
		require.NoError(t, err)
		require.Equal(t, "json_object", nestedString(t, transport.lastBody, "response_format", "type"))
	})
}

type captureTransport struct {
	t        *testing.T
	lastURL  string
	lastBody map[string]any
}

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	require.NoError(c.t, err)
	require.NoError(c.t, json.Unmarshal(body, &c.lastBody))
	c.lastURL = req.URL.String()

	resp := `{"id":"cmpl-1","choices":[{"index":0,"message":{"role":"assistant","content":"{\"title\":\"Inception\",\"year\":2010,\"type\":\"movie\"}"},"finish_reason":"stop"}]}`
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(resp)),
		Request:    req,
	}, nil
}

func nestedString(t *testing.T, data map[string]any, path ...string) string {
	t.Helper()
	var current any = data
	for _, key := range path {
		m, ok := current.(map[string]any)
		require.True(t, ok)
		current = m[key]
	}
	value, ok := current.(string)
	require.True(t, ok)
	return value
}
