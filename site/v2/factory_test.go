package v2

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewSiteFactory(t *testing.T) {
	factory := NewSiteFactory(nil)
	assert.NotNil(t, factory)

	logger := zap.NewNop()
	factory = NewSiteFactory(logger)
	assert.NotNil(t, factory)
}

func TestSiteFactory_CreateSite_NexusPHP(t *testing.T) {
	factory := NewSiteFactory(nil)

	opts := NexusPHPOptions{
		Cookie: "test-cookie",
	}
	optsBytes, _ := json.Marshal(opts)

	config := SiteConfig{
		Type:    "nexusphp",
		ID:      "hdsky",
		Name:    "HDSky",
		BaseURL: "https://hdsky.me",
		Options: optsBytes,
	}

	site, err := factory.CreateSite(config)
	require.NoError(t, err)
	require.NotNil(t, site)

	assert.Equal(t, "hdsky", site.ID())
	assert.Equal(t, "HDSky", site.Name())
	assert.Equal(t, SiteNexusPHP, site.Kind())
}

func TestSiteFactory_CreateSite_MTorrent(t *testing.T) {
	factory := NewSiteFactory(nil)

	opts := MTorrentOptions{
		APIKey: "test-api-key",
	}
	optsBytes, _ := json.Marshal(opts)

	config := SiteConfig{
		Type:    "mtorrent",
		ID:      "mteam",
		Name:    "M-Team",
		BaseURL: "https://api.m-team.cc",
		Options: optsBytes,
	}

	site, err := factory.CreateSite(config)
	require.NoError(t, err)
	require.NotNil(t, site)

	assert.Equal(t, "mteam", site.ID())
	assert.Equal(t, "M-Team", site.Name())
	assert.Equal(t, SiteMTorrent, site.Kind())
}

func TestSiteFactory_CreateSite_Unit3D(t *testing.T) {
	factory := NewSiteFactory(nil)

	opts := Unit3DOptions{
		APIKey: "test-api-key",
	}
	optsBytes, _ := json.Marshal(opts)

	config := SiteConfig{
		Type:    "unit3d",
		ID:      "blutopia",
		Name:    "Blutopia",
		BaseURL: "https://blutopia.cc",
		Options: optsBytes,
	}

	site, err := factory.CreateSite(config)
	require.NoError(t, err)
	require.NotNil(t, site)

	assert.Equal(t, "blutopia", site.ID())
	assert.Equal(t, "Blutopia", site.Name())
	assert.Equal(t, SiteUnit3D, site.Kind())
}

func TestSiteFactory_CreateSite_Gazelle(t *testing.T) {
	factory := NewSiteFactory(nil)

	opts := GazelleOptions{
		APIKey: "test-api-key",
	}
	optsBytes, _ := json.Marshal(opts)

	config := SiteConfig{
		Type:    "gazelle",
		ID:      "redacted",
		Name:    "Redacted",
		BaseURL: "https://redacted.ch",
		Options: optsBytes,
	}

	site, err := factory.CreateSite(config)
	require.NoError(t, err)
	require.NotNil(t, site)

	assert.Equal(t, "redacted", site.ID())
	assert.Equal(t, "Redacted", site.Name())
	assert.Equal(t, SiteGazelle, site.Kind())
}

func TestSiteFactory_CreateSite_GazelleWithCookie(t *testing.T) {
	factory := NewSiteFactory(nil)

	opts := GazelleOptions{
		Cookie: "test-cookie",
	}
	optsBytes, _ := json.Marshal(opts)

	config := SiteConfig{
		Type:    "gazelle",
		ID:      "orpheus",
		Name:    "Orpheus",
		BaseURL: "https://orpheus.network",
		Options: optsBytes,
	}

	site, err := factory.CreateSite(config)
	require.NoError(t, err)
	require.NotNil(t, site)

	assert.Equal(t, SiteGazelle, site.Kind())
}

func TestSiteFactory_CreateSite_MissingID(t *testing.T) {
	factory := NewSiteFactory(nil)

	config := SiteConfig{
		Type:    "nexusphp",
		BaseURL: "https://example.com",
	}

	_, err := factory.CreateSite(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ID is required")
}

func TestSiteFactory_CreateSite_MissingBaseURL(t *testing.T) {
	factory := NewSiteFactory(nil)

	config := SiteConfig{
		Type: "nexusphp",
		ID:   "test",
	}

	_, err := factory.CreateSite(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "baseUrl is required")
}

func TestSiteFactory_CreateSite_UnsupportedType(t *testing.T) {
	factory := NewSiteFactory(nil)

	config := SiteConfig{
		Type:    "unknown",
		ID:      "test",
		BaseURL: "https://example.com",
	}

	_, err := factory.CreateSite(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported site type")
}

func TestSiteFactory_CreateSite_NexusPHP_MissingCookie(t *testing.T) {
	factory := NewSiteFactory(nil)

	config := SiteConfig{
		Type:    "nexusphp",
		ID:      "test",
		BaseURL: "https://example.com",
		Options: json.RawMessage(`{}`),
	}

	_, err := factory.CreateSite(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires cookie")
}

func TestSiteFactory_CreateSite_MTorrent_MissingAPIKey(t *testing.T) {
	factory := NewSiteFactory(nil)

	config := SiteConfig{
		Type:    "mtorrent",
		ID:      "test",
		BaseURL: "https://example.com",
		Options: json.RawMessage(`{}`),
	}

	_, err := factory.CreateSite(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires apiKey")
}

func TestSiteFactory_CreateSite_Unit3D_MissingAPIKey(t *testing.T) {
	factory := NewSiteFactory(nil)

	config := SiteConfig{
		Type:    "unit3d",
		ID:      "test",
		BaseURL: "https://example.com",
		Options: json.RawMessage(`{}`),
	}

	_, err := factory.CreateSite(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires apiKey")
}

func TestSiteFactory_CreateSite_Gazelle_MissingAuth(t *testing.T) {
	factory := NewSiteFactory(nil)

	config := SiteConfig{
		Type:    "gazelle",
		ID:      "test",
		BaseURL: "https://example.com",
		Options: json.RawMessage(`{}`),
	}

	_, err := factory.CreateSite(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires apiKey or cookie")
}

func TestSiteFactory_CreateSite_DefaultName(t *testing.T) {
	factory := NewSiteFactory(nil)

	opts := NexusPHPOptions{Cookie: "test"}
	optsBytes, _ := json.Marshal(opts)

	config := SiteConfig{
		Type:    "nexusphp",
		ID:      "test-site",
		BaseURL: "https://example.com",
		Options: optsBytes,
		// Name is empty
	}

	site, err := factory.CreateSite(config)
	require.NoError(t, err)
	assert.Equal(t, "test-site", site.Name()) // Should default to ID
}

func TestSiteFactory_CreateSite_WithRateLimits(t *testing.T) {
	factory := NewSiteFactory(nil)

	opts := NexusPHPOptions{Cookie: "test"}
	optsBytes, _ := json.Marshal(opts)

	config := SiteConfig{
		Type:      "nexusphp",
		ID:        "test",
		Name:      "Test",
		BaseURL:   "https://example.com",
		Options:   optsBytes,
		RateLimit: 2.0,
		RateBurst: 5,
	}

	site, err := factory.CreateSite(config)
	require.NoError(t, err)
	require.NotNil(t, site)

	// Verify rate limiter is configured (through BaseSite)
	baseSite, ok := site.(*BaseSite[NexusPHPRequest, NexusPHPResponse])
	require.True(t, ok)
	assert.NotNil(t, baseSite.GetRateLimiter())
}

func TestSiteFactory_CreateSiteFromJSON(t *testing.T) {
	factory := NewSiteFactory(nil)

	jsonData := `{
		"type": "nexusphp",
		"id": "hdsky",
		"name": "HDSky",
		"baseUrl": "https://hdsky.me",
		"options": {"cookie": "test-cookie"}
	}`

	site, err := factory.CreateSiteFromJSON([]byte(jsonData))
	require.NoError(t, err)
	require.NotNil(t, site)

	assert.Equal(t, "hdsky", site.ID())
	assert.Equal(t, "HDSky", site.Name())
}

func TestSiteFactory_CreateSiteFromJSON_InvalidJSON(t *testing.T) {
	factory := NewSiteFactory(nil)

	_, err := factory.CreateSiteFromJSON([]byte(`invalid json`))
	assert.Error(t, err)
}

func TestSiteFactory_CreateSitesFromJSON(t *testing.T) {
	factory := NewSiteFactory(nil)

	jsonData := `[
		{
			"type": "nexusphp",
			"id": "hdsky",
			"name": "HDSky",
			"baseUrl": "https://hdsky.me",
			"options": {"cookie": "cookie1"}
		},
		{
			"type": "mtorrent",
			"id": "mteam",
			"name": "M-Team",
			"baseUrl": "https://api.m-team.cc",
			"options": {"apiKey": "key1"}
		}
	]`

	sites, err := factory.CreateSitesFromJSON([]byte(jsonData))
	require.NoError(t, err)
	require.Len(t, sites, 2)

	assert.Equal(t, "hdsky", sites[0].ID())
	assert.Equal(t, "mteam", sites[1].ID())
}

func TestSiteFactory_CreateSitesFromJSON_PartialFailure(t *testing.T) {
	factory := NewSiteFactory(zap.NewNop())

	jsonData := `[
		{
			"type": "nexusphp",
			"id": "valid",
			"name": "Valid Site",
			"baseUrl": "https://example.com",
			"options": {"cookie": "test"}
		},
		{
			"type": "nexusphp",
			"id": "invalid",
			"name": "Invalid Site",
			"baseUrl": "https://example.com",
			"options": {}
		}
	]`

	sites, err := factory.CreateSitesFromJSON([]byte(jsonData))
	require.NoError(t, err)
	require.Len(t, sites, 1) // Only valid site should be created

	assert.Equal(t, "valid", sites[0].ID())
}

func TestSiteFactory_CreateSitesFromJSON_InvalidJSON(t *testing.T) {
	factory := NewSiteFactory(nil)

	_, err := factory.CreateSitesFromJSON([]byte(`invalid json`))
	assert.Error(t, err)
}

func TestSiteFactory_CreateSite_NexusPHP_CustomSelectors(t *testing.T) {
	factory := NewSiteFactory(nil)

	opts := NexusPHPOptions{
		Cookie: "test-cookie",
		Selectors: &SiteSelectors{
			TableRows: "table.custom > tr",
			Title:     "td.title a",
		},
	}
	optsBytes, _ := json.Marshal(opts)

	config := SiteConfig{
		Type:    "nexusphp",
		ID:      "custom",
		Name:    "Custom Site",
		BaseURL: "https://example.com",
		Options: optsBytes,
	}

	site, err := factory.CreateSite(config)
	require.NoError(t, err)
	require.NotNil(t, site)

	// Verify custom selectors are applied
	baseSite, ok := site.(*BaseSite[NexusPHPRequest, NexusPHPResponse])
	require.True(t, ok)

	driver := baseSite.GetDriver().(*NexusPHPDriver)
	assert.Equal(t, "table.custom > tr", driver.Selectors.TableRows)
	assert.Equal(t, "td.title a", driver.Selectors.Title)
}
