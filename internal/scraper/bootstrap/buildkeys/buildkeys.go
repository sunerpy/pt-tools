// Package buildkeys holds compile-time API credentials that are injected via
// `-ldflags "-X github.com/sunerpy/pt-tools/internal/scraper/bootstrap/buildkeys.TmdbApiKey=..."`
// at build time (typically from GitHub Actions secrets).
//
// Runtime precedence (highest → lowest):
//  1. User-configured ProviderCredential row in DB (from Web UI)
//  2. PT_SCRAPER_<PROVIDER>_<FIELD> environment variable at process start
//  3. Build-time ldflags-injected variables in this package
//  4. Provider-specific defaults (e.g. douban's bundled Frodo key)
//
// Why ldflags and not just env: distributing binaries with a functional
// out-of-the-box TMDB key is a common pattern (tinyMediaManager does this
// via a closed-source Maven JAR). Go's ldflags mechanism is the idiomatic
// equivalent — the key ends up as a string literal in the binary, visible
// via `strings` but not committed to source control.
//
// Security note: any key shipped in public release binaries CAN be extracted.
// Use a key dedicated to this distribution (not your personal TMDB key) so
// it can be revoked without affecting other services. Monitor usage and
// rotate if abuse is detected. For privacy-sensitive deployments prefer
// BYOK via the Web UI (option 1 above).
package buildkeys

// TmdbBearerToken is the TMDB v4 Bearer token injected at build time.
// Left empty in source; set via:
//
//	go build -ldflags "-X github.com/sunerpy/pt-tools/internal/scraper/bootstrap/buildkeys.TmdbBearerToken=eyJhbGciOi..."
var TmdbBearerToken string

// TmdbApiKey is the TMDB v3 API key injected at build time. Fallback if
// TmdbBearerToken is empty.
var TmdbApiKey string

// OmdbApiKey is the OMDb API key injected at build time (optional).
var OmdbApiKey string

// ImdbBaseURL overrides the default IMDb HTML scraping base URL. Left empty
// to use the default (https://www.imdb.com). Kept here so the URL can be
// swapped at build time for testing against mirrors or proxies.
var ImdbBaseURL string
