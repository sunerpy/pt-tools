package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitize_InvalidYear(t *testing.T) {
	t.Parallel()
	r := &NFOResult{Year: 99999999}
	sanitize(r)
	require.Equal(t, 0, r.Year)
}

func TestSanitize_InvalidTMDBID(t *testing.T) {
	t.Parallel()
	r := &NFOResult{TMDBID: 999999999}
	sanitize(r)
	require.Equal(t, 0, r.TMDBID)
}

func TestSanitize_InvalidIMDBID(t *testing.T) {
	t.Parallel()
	r := &NFOResult{IMDBID: "xxxxx"}
	sanitize(r)
	require.Equal(t, "", r.IMDBID)
}

func TestSanitize_StripReleaseTags(t *testing.T) {
	t.Parallel()
	r := &NFOResult{Title: "Inception 1080p BluRay x265"}
	sanitize(r)
	require.Equal(t, "Inception", r.Title)
}

func TestSanitize_TypeNormalization(t *testing.T) {
	t.Parallel()
	r := &NFOResult{Type: "MOVIE"}
	sanitize(r)
	require.Equal(t, "movie", r.Type)
}

func TestSampling_NoSession(t *testing.T) {
	t.Parallel()
	p := NewSamplingProvider()
	_, err := p.Extract(context.Background(), ExtractRequest{})
	require.ErrorIs(t, err, ErrNoMCPSession)
}

func TestChained_Fallback(t *testing.T) {
	t.Parallel()
	failing := &mockProvider{err: errors.New("boom")}
	succeeding := &mockProvider{result: &NFOResult{Title: "T"}}
	c := NewChainedProvider(failing, succeeding)

	result, err := c.Extract(context.Background(), ExtractRequest{})
	require.NoError(t, err)
	require.Equal(t, "T", result.Title)
}

type mockProvider struct {
	result *NFOResult
	err    error
}

func (m *mockProvider) Name() string {
	return "mock"
}

func (m *mockProvider) Kind() ProviderKind {
	return KindOpenAICompat
}

func (m *mockProvider) Extract(ctx context.Context, req ExtractRequest) (*NFOResult, error) {
	_ = ctx
	_ = req
	return m.result, m.err
}

func (m *mockProvider) Close() error {
	return nil
}
