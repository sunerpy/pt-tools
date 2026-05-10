package nfo

import (
	"context"
	"fmt"
	"strings"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

// EmbyNfoWriter extends KodiNfoWriter by injecting <set> tag with tmdbcolid attribute.
// This allows Emby to recognize and manage TMDB collections with proper metadata.
type EmbyNfoWriter struct {
	base *KodiNfoWriter
}

// NewEmbyNfoWriter creates a new Emby NFO writer.
func NewEmbyNfoWriter() *EmbyNfoWriter {
	return &EmbyNfoWriter{base: NewKodiNfoWriter()}
}

// Dialect returns the dialect identifier.
func (w *EmbyNfoWriter) Dialect() string {
	return "emby"
}

// WriteMovieNfo writes a movie NFO file with Emby extensions.
// Injects <set tmdbcolid="{id}"><name>...</name></set> if Movie.TmdbCollection is non-zero.
func (w *EmbyNfoWriter) WriteMovieNfo(ctx context.Context, m *core.Movie, paths []string) error {
	if err := w.base.WriteMovieNfo(ctx, m, paths); err != nil {
		return err
	}

	// Only add extension if collection ID is present
	if m == nil || m.TmdbCollection == 0 {
		return nil
	}

	// Build the <set tmdbcolid="..."> block
	var injection strings.Builder
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
			return fmt.Errorf("emby inject set: %w", err)
		}
	}

	return nil
}

// WriteTvShowNfo delegates to Kodi base (no Emby-specific extensions).
func (w *EmbyNfoWriter) WriteTvShowNfo(ctx context.Context, s *core.TvShow, path string) error {
	return w.base.WriteTvShowNfo(ctx, s, path)
}

// WriteSeasonNfo delegates to Kodi base (no Emby-specific extensions).
func (w *EmbyNfoWriter) WriteSeasonNfo(ctx context.Context, s *core.TvShowSeason, path string) error {
	return w.base.WriteSeasonNfo(ctx, s, path)
}

// WriteEpisodeNfo delegates to Kodi base (no Emby-specific extensions).
func (w *EmbyNfoWriter) WriteEpisodeNfo(ctx context.Context, e *core.TvShowEpisode, path string) error {
	return w.base.WriteEpisodeNfo(ctx, e, path)
}
