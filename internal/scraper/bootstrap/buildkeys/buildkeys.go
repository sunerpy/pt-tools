// Package buildkeys holds compile-time API credentials that are injected via
// `-ldflags "-X github.com/sunerpy/pt-tools/internal/scraper/bootstrap/buildkeys.TmdbApiKey=..."`
// at build time.
//
// ⚠️  NOT FOR PUBLIC RELEASE BINARIES — For private / self-hosted builds ONLY.
//
// Why this is DANGEROUS for public distribution:
//   - Any user can run `strings ./pt-tools | grep <prefix>` and extract the key
//   - All users share the same quota → one heavy user can exhaust it for everyone
//   - TMDB may REVOKE the key if it detects abuse → every released binary breaks
//   - Rotating the key requires re-releasing the binary
//
// Correct usage patterns:
//   - ✅ Private internal deployment: inject your own team's key via CI, binary
//     never leaves your org
//   - ✅ Development / CI integration tests: set TMDB_BEARER_TOKEN env in
//     GitHub Actions secrets for running the integration test suite
//   - ❌ Public Releases on GitHub / Docker Hub: leave blank, let users BYOK
//     via the Web UI (Scraper Settings → Provider Credentials)
//
// Runtime precedence (highest → lowest) — see internal/scraper/bootstrap/
// providers.go#resolveTMDBConfig:
//  1. User-configured ProviderCredential row in DB (Web UI) — RECOMMENDED
//  2. PT_SCRAPER_{TMDB_BEARER,TMDB_APIKEY} environment variable at process start
//  3. Build-time ldflags-injected variables in this package (this file)
//  4. Provider-specific defaults (e.g. douban's bundled Frodo app key, IMDb's
//     HTML scraping with no key required)
package buildkeys

// TmdbBearerToken is the TMDB v4 Bearer token injected at build time.
// Left empty in source (and in all public release builds); set via:
//
//	go build -ldflags "-X github.com/sunerpy/pt-tools/internal/scraper/bootstrap/buildkeys.TmdbBearerToken=eyJhbGciOi..."
//
// ⚠️ See package doc — this MUST stay empty in public OSS release binaries.
var TmdbBearerToken string

// TmdbApiKey is the TMDB v3 API key injected at build time. Fallback if
// TmdbBearerToken is empty. ⚠️ See package doc — public releases must leave blank.
var TmdbApiKey string

// OmdbApiKey is the OMDb API key injected at build time (optional).
// ⚠️ See package doc — public releases must leave blank.
var OmdbApiKey string

// ImdbBaseURL overrides the default IMDb HTML scraping base URL. Unlike the
// provider API keys above, this is safe to set in public releases since IMDb
// does not require authentication — the base URL can be pointed at a mirror
// or local proxy for testing. Leave empty to use https://www.imdb.com.
var ImdbBaseURL string
