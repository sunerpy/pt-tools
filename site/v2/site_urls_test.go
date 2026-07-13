package v2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteURLRegistry_GetFailoverConfig(t *testing.T) {
	reg := GetGlobalRegistry()
	// A registered site should have URLs
	sites := reg.ListSites()
	if len(sites) > 0 {
		cfg, err := reg.GetFailoverConfig(sites[0])
		require.NoError(t, err)
		assert.NotEmpty(t, cfg.BaseURLs)
	}
	// Unknown site -> error
	_, err := reg.GetFailoverConfig(SiteName("nonexistent-xyz"))
	assert.Error(t, err)
}

func TestGetSiteURLsForKind(t *testing.T) {
	result := GetSiteURLsForKind(SiteNexusPHP)
	assert.NotNil(t, result)
}
