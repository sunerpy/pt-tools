package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sunerpy/pt-tools/internal/scraper/bootstrap/buildkeys"
	"github.com/sunerpy/pt-tools/internal/scraper/store"
)

func TestResolveTMDBConfig_Precedence(t *testing.T) {
	origBearer := buildkeys.TmdbBearerToken
	origAPIKey := buildkeys.TmdbApiKey
	t.Cleanup(func() {
		buildkeys.TmdbBearerToken = origBearer
		buildkeys.TmdbApiKey = origAPIKey
	})

	tests := []struct {
		name          string
		cred          store.ProviderCredential
		envBearer     string
		envAPIKey     string
		ldflagsBearer string
		ldflagsAPIKey string
		wantBearer    string
		wantAPIKey    string
	}{
		{
			name:          "empty everywhere returns empty",
			cred:          store.ProviderCredential{},
			ldflagsAPIKey: "",
			wantBearer:    "",
			wantAPIKey:    "",
		},
		{
			name:          "DB cred beats env beats ldflags",
			cred:          store.ProviderCredential{BearerToken: "db-bearer", APIKey: "db-key"},
			envBearer:     "env-bearer",
			envAPIKey:     "env-key",
			ldflagsBearer: "ld-bearer",
			ldflagsAPIKey: "ld-key",
			wantBearer:    "db-bearer",
			wantAPIKey:    "db-key",
		},
		{
			name:          "env beats ldflags when DB empty",
			cred:          store.ProviderCredential{},
			envBearer:     "env-bearer",
			envAPIKey:     "env-key",
			ldflagsBearer: "ld-bearer",
			ldflagsAPIKey: "ld-key",
			wantBearer:    "env-bearer",
			wantAPIKey:    "env-key",
		},
		{
			name:          "ldflags fallback when DB and env both empty",
			cred:          store.ProviderCredential{},
			ldflagsBearer: "ld-bearer",
			ldflagsAPIKey: "ld-key",
			wantBearer:    "ld-bearer",
			wantAPIKey:    "ld-key",
		},
		{
			name:          "partial DB mix — DB bearer + env key",
			cred:          store.ProviderCredential{BearerToken: "db-bearer"},
			envAPIKey:     "env-key",
			ldflagsAPIKey: "ld-key",
			wantBearer:    "db-bearer",
			wantAPIKey:    "env-key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PT_SCRAPER_TMDB_BEARER", tc.envBearer)
			t.Setenv("PT_SCRAPER_TMDB_APIKEY", tc.envAPIKey)
			buildkeys.TmdbBearerToken = tc.ldflagsBearer
			buildkeys.TmdbApiKey = tc.ldflagsAPIKey

			got := resolveTMDBConfig(tc.cred)
			assert.Equal(t, tc.wantBearer, got.BearerToken, "BearerToken")
			assert.Equal(t, tc.wantAPIKey, got.APIKey, "APIKey")
		})
	}
}
