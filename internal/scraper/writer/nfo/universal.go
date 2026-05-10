package nfo

import (
	"context"
	"fmt"
	"strings"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

// UniversalNfoWriter combines both Jellyfin and Emby extensions for maximum compatibility.
// It writes both <collectionnumber> (Jellyfin) and <set tmdbcolid="..."> (Emby) tags,
// allowing the same NFO to be recognized by Kodi, Jellyfin, and Emby.
type UniversalNfoWriter struct {
	base *KodiNfoWriter
}

// NewUniversalNfoWriter creates a new Universal NFO writer.
func NewUniversalNfoWriter() *UniversalNfoWriter {
	return &UniversalNfoWriter{base: NewKodiNfoWriter()}
}

// Dialect returns the dialect identifier.
func (w *UniversalNfoWriter) Dialect() string {
	return "universal"
}

// WriteMovieNfo writes a movie NFO file compatible with Kodi, Jellyfin, and Emby.
// Injects both <collectionnumber> and <set tmdbcolid="..."> if Movie.TmdbCollection is non-zero.
func (w *UniversalNfoWriter) WriteMovieNfo(ctx context.Context, m *core.Movie, paths []string) error {
	if err := w.base.WriteMovieNfo(ctx, m, paths); err != nil {
		return err
	}

	// Only add extensions if collection ID is present
	if m == nil || m.TmdbCollection == 0 {
		return nil
	}

	// Build injection containing both Jellyfin and Emby extensions
	var injection strings.Builder

	// Jellyfin extension: <collectionnumber>
	fmt.Fprintf(&injection, "  <collectionnumber>%d</collectionnumber>\n", m.TmdbCollection)

	// Emby extension: <set tmdbcolid="..."><name>...</name></set>
	injection.WriteString("  <set tmdbcolid=\"")
	fmt.Fprintf(&injection, "%d", m.TmdbCollection)
	injection.WriteString("\">\n")
	if m.MovieSetName != "" {
		injection.WriteString("    <name>")
		injection.WriteString(escape(m.MovieSetName))
		injection.WriteString("</name>\n")
	}
	injection.WriteString("  </set>\n")

	// Inject into each NFO file
	for _, p := range paths {
		if err := injectBeforeClosing(p, "movie", injection.String()); err != nil {
			return fmt.Errorf("universal inject: %w", err)
		}
	}

	return nil
}

// WriteTvShowNfo delegates to Kodi base (no Universal-specific extensions).
func (w *UniversalNfoWriter) WriteTvShowNfo(ctx context.Context, s *core.TvShow, path string) error {
	return w.base.WriteTvShowNfo(ctx, s, path)
}

// WriteSeasonNfo delegates to Kodi base (no Universal-specific extensions).
func (w *UniversalNfoWriter) WriteSeasonNfo(ctx context.Context, s *core.TvShowSeason, path string) error {
	return w.base.WriteSeasonNfo(ctx, s, path)
}

// WriteEpisodeNfo delegates to Kodi base (no Universal-specific extensions).
func (w *UniversalNfoWriter) WriteEpisodeNfo(ctx context.Context, e *core.TvShowEpisode, path string) error {
	return w.base.WriteEpisodeNfo(ctx, e, path)
}
