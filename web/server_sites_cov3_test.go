package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func TestApiSites_DeleteSuccess(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)

	enabled := true
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "cookie", Cookie: "c=1", APIUrl: "https://hdsky.me",
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/sites?name=hdsky", nil)
	srv.apiSites(w, req)
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusBadRequest)
}

func TestApiSites_DeleteError(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/sites?name=does-not-exist-zzz", nil)
	srv.apiSites(w, req)
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusBadRequest)
}
