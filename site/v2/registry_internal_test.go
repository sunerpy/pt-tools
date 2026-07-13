package v2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSiteRegistry_CreateSite_KindBranches(t *testing.T) {
	registry := NewSiteRegistry(zap.NewNop())

	registry.Register(SiteMeta{ID: "hddolbytest", Name: "HD", Kind: SiteHDDolby, DefaultBaseURL: "https://hd.example"})
	registry.Register(SiteMeta{ID: "gazelletest", Name: "GZ", Kind: SiteGazelle, DefaultBaseURL: "https://gz.example"})
	registry.Register(SiteMeta{ID: "rousitest", Name: "RS", Kind: SiteRousi, DefaultBaseURL: "https://rs.example"})

	_, errHDNoCreds := registry.CreateSite("hddolbytest", SiteCredentials{}, "")
	require.Error(t, errHDNoCreds)
	_, errHDNoCookie := registry.CreateSite("hddolbytest", SiteCredentials{APIKey: "k"}, "")
	require.Error(t, errHDNoCookie)

	_, errGazelle := registry.CreateSite("gazelletest", SiteCredentials{}, "")
	require.Error(t, errGazelle)

	_, errRousi := registry.CreateSite("rousitest", SiteCredentials{}, "")
	require.Error(t, errRousi)

	_, errUnknown := registry.CreateSite("nope", SiteCredentials{}, "")
	require.Error(t, errUnknown)
}

func TestSiteRegistry_CreateSite_Success(t *testing.T) {
	registry := NewSiteRegistry(zap.NewNop())
	registry.Register(SiteMeta{ID: "hd", Name: "HD", Kind: SiteHDDolby, DefaultBaseURL: "https://hd.example"})

	site, err := registry.CreateSite("hd", SiteCredentials{APIKey: "k", Cookie: "c=1"}, "")
	require.NoError(t, err)
	require.NotNil(t, site)

	registry.Register(SiteMeta{ID: "gz", Name: "GZ", Kind: SiteGazelle, DefaultBaseURL: "https://gz.example"})
	site2, err := registry.CreateSite("gz", SiteCredentials{APIKey: "k"}, "")
	require.NoError(t, err)
	require.NotNil(t, site2)
}

func TestSiteRegistry_CreateSite_NoBaseURL(t *testing.T) {
	registry := NewSiteRegistry(zap.NewNop())
	registry.Register(SiteMeta{ID: "nb", Name: "NB", Kind: SiteNexusPHP})
	_, err := registry.CreateSite("nb", SiteCredentials{Cookie: "c=1"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no base URL")
}
