package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func TestSetQAHook(t *testing.T) {
	s := &Server{}
	called := false
	s.SetQAHook(func(_ *http.ServeMux) { called = true })
	require.NotNil(t, s.qaHook)
	s.qaHook(http.NewServeMux())
	assert.True(t, called)
}

func TestShutdown(t *testing.T) {
	t.Run("nil server", func(t *testing.T) {
		var s *Server
		assert.NoError(t, s.Shutdown(context.Background()))
	})

	t.Run("no http server", func(t *testing.T) {
		s := &Server{}
		assert.NoError(t, s.Shutdown(context.Background()))
	})

	t.Run("with http server", func(t *testing.T) {
		s := &Server{httpServer: &http.Server{}}
		assert.NoError(t, s.Shutdown(context.Background()))
	})
}

func TestVerifyLegacyPassword(t *testing.T) {
	tests := []struct {
		name   string
		stored string
		pw     string
		want   bool
	}{
		{"has pipe rejected", "salt|sum|100", "x", false},
		{"plaintext match", "secret", "secret", true},
		{"plaintext mismatch", "secret", "other", false},
		{"sha256 hex match", "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", "hello", true},
		{"sha256 mismatch", "abcdef", "hello", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, verifyLegacyPassword(tt.stored, tt.pw))
		})
	}
}

func TestDisableUnavailableSites(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)

	enabled := true
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "cookie", Cookie: "c=1", APIUrl: "https://hdsky.me",
	}))

	srv.disableUnavailableSites([]models.SiteGroup{"hdsky"})

	sc, err := srv.store.GetSiteConf(models.SiteGroup("hdsky"))
	require.NoError(t, err)
	require.NotNil(t, sc.Enabled)
	assert.False(t, *sc.Enabled)
}

func TestHashAndVerifyPassword_Roundtrip(t *testing.T) {
	hashed := hashPassword("mypassword")
	assert.True(t, verifyPassword(hashed, "mypassword"))
	assert.False(t, verifyPassword(hashed, "wrong"))
	assert.False(t, verifyPassword("badformat", "mypassword"))
}

func TestApiSiteDefinitions(t *testing.T) {
	s := &Server{}

	t.Run("GET returns definitions", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/sites/definitions", nil)
		w := httptest.NewRecorder()
		s.apiSiteDefinitions(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.NotEmpty(t, w.Body.Bytes())
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/sites/definitions", nil)
		w := httptest.NewRecorder()
		s.apiSiteDefinitions(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestGetEnabledSiteIDs(t *testing.T) {
	t.Run("nil store", func(t *testing.T) {
		s := &Server{}
		assert.Nil(t, s.getEnabledSiteIDs())
	})

	t.Run("with enabled sites", func(t *testing.T) {
		writeWebTestSecretKey(t)
		srv := setupServer(t)
		enabled := true
		require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{
			Enabled: &enabled, AuthMethod: "cookie", Cookie: "c=1", APIUrl: "https://hdsky.me",
		}))
		ids := srv.getEnabledSiteIDs()
		assert.True(t, ids["hdsky"])
	})
}

func TestFilterEnabledSites(t *testing.T) {
	t.Run("nil enabled returns requested", func(t *testing.T) {
		got := filterEnabledSites([]string{"a", "b"}, nil)
		assert.Equal(t, []string{"a", "b"}, got)
	})

	t.Run("empty requested returns all enabled", func(t *testing.T) {
		got := filterEnabledSites(nil, map[string]bool{"a": true})
		assert.Equal(t, []string{"a"}, got)
	})

	t.Run("filters requested by enabled", func(t *testing.T) {
		got := filterEnabledSites([]string{"a", "b", "c"}, map[string]bool{"a": true, "c": true})
		assert.ElementsMatch(t, []string{"a", "c"}, got)
	})
}
