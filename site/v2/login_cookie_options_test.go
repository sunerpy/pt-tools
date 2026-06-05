package v2

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestMTorrentOptions_CarriesLoginCookie(t *testing.T) {
	opts := MTorrentOptions{APIKey: "k", Cookie: "c_secure=abc"}
	data, err := json.Marshal(opts)
	require.NoError(t, err)

	var decoded MTorrentOptions
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "k", decoded.APIKey)
	assert.Equal(t, "c_secure=abc", decoded.Cookie)
}

func TestRousiOptions_CarriesLoginCookie(t *testing.T) {
	opts := RousiOptions{Passkey: "p", Cookie: "c_secure=xyz"}
	data, err := json.Marshal(opts)
	require.NoError(t, err)

	var decoded RousiOptions
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "p", decoded.Passkey)
	assert.Equal(t, "c_secure=xyz", decoded.Cookie)
}

func TestCreateMTorrentSite_CookieDoesNotBreakConstruction(t *testing.T) {
	opts, err := json.Marshal(MTorrentOptions{APIKey: "k", Cookie: "c_secure=abc"})
	require.NoError(t, err)

	site, err := createMTorrentSite(SiteConfig{
		ID:      "mteam",
		Name:    "M-Team",
		BaseURL: "https://api.m-team.cc",
		Options: opts,
	}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, site)
	assert.Equal(t, "mteam", site.ID())
	assert.Equal(t, SiteMTorrent, site.Kind())
}

func TestNewMTorrentDriver_HoldsLoginCookieIsolated(t *testing.T) {
	driver := NewMTorrentDriver(MTorrentDriverConfig{
		BaseURL:     "https://api.m-team.cc",
		APIKey:      "k",
		LoginCookie: "c_secure=abc",
	})
	assert.Equal(t, "c_secure=abc", driver.LoginCookie)
	assert.Equal(t, "k", driver.APIKey)
}
