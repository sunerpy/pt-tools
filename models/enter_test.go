package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateSiteName(t *testing.T) {
	// Test all valid preset sites
	validSites := []string{"springsunday", "hdsky", "mteam", "hddolby", "ourbits"}
	for _, site := range validSites {
		g, err := ValidateSiteName(site)
		require.NoError(t, err, "site: %s", site)
		require.Equal(t, SiteGroup(site), g, "site: %s", site)
	}
	// Test invalid site
	_, err := ValidateSiteName("unknown")
	require.Error(t, err)
}
