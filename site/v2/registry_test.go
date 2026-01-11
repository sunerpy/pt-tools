package v2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSiteRegistry(t *testing.T) {
	registry := NewSiteRegistry(nil)
	require.NotNil(t, registry)

	// Should have default sites registered
	sites := registry.List()
	assert.GreaterOrEqual(t, len(sites), 3, "should have at least 3 default sites")

	// Check mteam
	meta, ok := registry.Get("mteam")
	assert.True(t, ok, "mteam should be registered")
	assert.Equal(t, SiteMTorrent, meta.Kind)
	assert.Equal(t, "api_key", meta.AuthMethod)
	assert.NotEmpty(t, meta.DefaultBaseURL)

	// Check hdsky
	meta, ok = registry.Get("hdsky")
	assert.True(t, ok, "hdsky should be registered")
	assert.Equal(t, SiteNexusPHP, meta.Kind)
	assert.Equal(t, "cookie", meta.AuthMethod)

	// Check springsunday
	meta, ok = registry.Get("springsunday")
	assert.True(t, ok, "springsunday should be registered")
	assert.Equal(t, SiteNexusPHP, meta.Kind)
	assert.Equal(t, "cookie", meta.AuthMethod)
}

func TestSiteRegistry_Register(t *testing.T) {
	registry := NewSiteRegistry(nil)

	// Register a new site
	registry.Register(SiteMeta{
		ID:             "testsite",
		Name:           "Test Site",
		Kind:           SiteUnit3D,
		DefaultBaseURL: "https://test.example.com",
		AuthMethod:     "api_key",
	})

	meta, ok := registry.Get("testsite")
	assert.True(t, ok)
	assert.Equal(t, "Test Site", meta.Name)
	assert.Equal(t, SiteUnit3D, meta.Kind)
}

func TestSiteRegistry_Get(t *testing.T) {
	registry := NewSiteRegistry(nil)

	// Existing site
	meta, ok := registry.Get("mteam")
	assert.True(t, ok)
	assert.Equal(t, "mteam", meta.ID)

	// Non-existing site
	_, ok = registry.Get("nonexistent")
	assert.False(t, ok)
}

func TestSiteRegistry_GetSiteKind(t *testing.T) {
	registry := NewSiteRegistry(nil)

	kind, ok := registry.GetSiteKind("mteam")
	assert.True(t, ok)
	assert.Equal(t, SiteMTorrent, kind)

	kind, ok = registry.GetSiteKind("hdsky")
	assert.True(t, ok)
	assert.Equal(t, SiteNexusPHP, kind)

	_, ok = registry.GetSiteKind("nonexistent")
	assert.False(t, ok)
}

func TestSiteRegistry_GetDefaultBaseURL(t *testing.T) {
	registry := NewSiteRegistry(nil)

	url, ok := registry.GetDefaultBaseURL("mteam")
	assert.True(t, ok)
	assert.Equal(t, "https://api.m-team.cc", url)

	url, ok = registry.GetDefaultBaseURL("hdsky")
	assert.True(t, ok)
	assert.Equal(t, "https://hdsky.me", url)

	_, ok = registry.GetDefaultBaseURL("nonexistent")
	assert.False(t, ok)
}

func TestSiteRegistry_CreateSite_MTorrent(t *testing.T) {
	registry := NewSiteRegistry(nil)

	// Missing API key
	_, err := registry.CreateSite("mteam", SiteCredentials{}, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires API key")

	// With API key
	site, err := registry.CreateSite("mteam", SiteCredentials{APIKey: "test-api-key"}, "")
	assert.NoError(t, err)
	assert.NotNil(t, site)
	assert.Equal(t, "mteam", site.ID())
	assert.Equal(t, SiteMTorrent, site.Kind())
}

func TestSiteRegistry_CreateSite_NexusPHP(t *testing.T) {
	registry := NewSiteRegistry(nil)

	// Missing cookie
	_, err := registry.CreateSite("hdsky", SiteCredentials{}, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires cookie")

	// With cookie
	site, err := registry.CreateSite("hdsky", SiteCredentials{Cookie: "test-cookie"}, "")
	assert.NoError(t, err)
	assert.NotNil(t, site)
	assert.Equal(t, "hdsky", site.ID())
	assert.Equal(t, SiteNexusPHP, site.Kind())
}

func TestSiteRegistry_CreateSite_CustomBaseURL(t *testing.T) {
	registry := NewSiteRegistry(nil)

	customURL := "https://custom.example.com"
	site, err := registry.CreateSite("mteam", SiteCredentials{APIKey: "test-api-key"}, customURL)
	assert.NoError(t, err)
	assert.NotNil(t, site)
}

func TestSiteRegistry_CreateSite_NotFound(t *testing.T) {
	registry := NewSiteRegistry(nil)

	_, err := registry.CreateSite("nonexistent", SiteCredentials{}, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in registry")
}

func TestSiteCredentials(t *testing.T) {
	creds := SiteCredentials{
		Cookie: "my-cookie",
		APIKey: "my-api-key",
	}

	assert.Equal(t, "my-cookie", creds.Cookie)
	assert.Equal(t, "my-api-key", creds.APIKey)
}
