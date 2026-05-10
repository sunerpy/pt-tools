// Package sources aggregates side-effect imports for all scraper source providers.
// Usage: import _ "github.com/sunerpy/pt-tools/internal/scraper/sources"
// This triggers all source init() functions, registering providers with their respective registries.
package sources

// Blank imports will be added as source packages come online:
// _ "github.com/sunerpy/pt-tools/internal/scraper/source/tmdb"
// _ "github.com/sunerpy/pt-tools/internal/scraper/source/douban"
// _ "github.com/sunerpy/pt-tools/internal/scraper/source/imdb"
