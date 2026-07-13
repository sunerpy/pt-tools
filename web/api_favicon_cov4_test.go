package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestApiFavicon_InitsServiceWhenNil(t *testing.T) {
	setupFaviconServer(t)
	prev := faviconService
	faviconService = nil
	t.Cleanup(func() {
		if faviconService != nil {
			close(faviconService.stopCh)
		}
		faviconService = prev
	})

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicon/unknownxyz?nofetch=1", nil)
	s.apiFavicon(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, faviconService)
}

func TestApiFavicon_DefinitionURLsFallback(t *testing.T) {
	server := setupFaviconServer(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47, 11})
	}))
	defer ts.Close()

	if def := v2.GetDefinitionRegistry().GetOrDefault("covurlsonly"); def != nil {
		def.URLs = []string{ts.URL}
	} else {
		v2.RegisterSiteDefinition(&v2.SiteDefinition{
			ID:   "covurlsonly",
			Name: "CovURLsOnly",
			URLs: []string{ts.URL},
		})
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicon/covurlsonly", nil)
	server.apiFavicon(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Body.Bytes())
}
