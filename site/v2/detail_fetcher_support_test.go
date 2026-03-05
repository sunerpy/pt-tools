package v2_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
	_ "github.com/sunerpy/pt-tools/site/v2/definitions"
)

func TestSiteDetailFetcherSupportForNewSites(t *testing.T) {
	registry := v2.GetGlobalSiteRegistry()

	tests := []struct {
		name  string
		site  string
		creds v2.SiteCredentials
	}{
		{
			name: "xingyunge supports detail fetcher",
			site: "xingyunge",
			creds: v2.SiteCredentials{
				Cookie: "uid=1; pass=mock",
			},
		},
		{
			name: "agsvpt supports detail fetcher",
			site: "agsvpt",
			creds: v2.SiteCredentials{
				Cookie: "uid=1; pass=mock",
			},
		},
		{
			name: "mooko supports detail fetcher",
			site: "mooko",
			creds: v2.SiteCredentials{
				Cookie: "uid=1; pass=mock",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			site, err := registry.CreateSite(tt.site, tt.creds, "")
			require.NoError(t, err)
			t.Cleanup(func() { _ = site.Close() })

			provider, ok := site.(v2.DetailFetcherProvider)
			require.True(t, ok)
			assert.NotNil(t, provider.GetDetailFetcher())
		})
	}
}
