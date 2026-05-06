package nfo

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

// JellyfinNfoWriter extends KodiNfoWriter by injecting <collectionnumber> tag.
// This allows Jellyfin to recognize TMDB collections.
type JellyfinNfoWriter struct {
	base *KodiNfoWriter
}

// NewJellyfinNfoWriter creates a new Jellyfin NFO writer.
func NewJellyfinNfoWriter() *JellyfinNfoWriter {
	return &JellyfinNfoWriter{base: NewKodiNfoWriter()}
}

// Dialect returns the dialect identifier.
func (w *JellyfinNfoWriter) Dialect() string {
	return "jellyfin"
}

// WriteMovieNfo writes a movie NFO file with Jellyfin extensions.
// Injects <collectionnumber> if Movie.TmdbCollection is non-zero.
func (w *JellyfinNfoWriter) WriteMovieNfo(ctx context.Context, m *core.Movie, paths []string) error {
	if err := w.base.WriteMovieNfo(ctx, m, paths); err != nil {
		return err
	}

	// Only add extension if collection ID is present
	if m == nil || m.TmdbCollection == 0 {
		return nil
	}

	// Inject <collectionnumber> into each NFO file
	injection := fmt.Sprintf("  <collectionnumber>%d</collectionnumber>\n", m.TmdbCollection)
	for _, p := range paths {
		if err := injectBeforeClosing(p, "movie", injection); err != nil {
			return fmt.Errorf("jellyfin inject collectionnumber: %w", err)
		}
	}

	return nil
}

// WriteTvShowNfo delegates to Kodi base (no Jellyfin-specific extensions).
func (w *JellyfinNfoWriter) WriteTvShowNfo(ctx context.Context, s *core.TvShow, path string) error {
	return w.base.WriteTvShowNfo(ctx, s, path)
}

// WriteSeasonNfo delegates to Kodi base (no Jellyfin-specific extensions).
func (w *JellyfinNfoWriter) WriteSeasonNfo(ctx context.Context, s *core.TvShowSeason, path string) error {
	return w.base.WriteSeasonNfo(ctx, s, path)
}

// WriteEpisodeNfo delegates to Kodi base (no Jellyfin-specific extensions).
func (w *JellyfinNfoWriter) WriteEpisodeNfo(ctx context.Context, e *core.TvShowEpisode, path string) error {
	return w.base.WriteEpisodeNfo(ctx, e, path)
}

// injectBeforeClosing inserts content into an XML file before its closing tag.
// It reads the file, finds the closing </rootTag>, and inserts the content before it.
// The file must exist and contain the closing tag.
func injectBeforeClosing(path, rootTag, content string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	marker := []byte("</" + rootTag + ">")
	idx := bytes.LastIndex(data, marker)
	if idx < 0 {
		return fmt.Errorf("closing tag </%s> not found in %s", rootTag, path)
	}

	var buf bytes.Buffer
	buf.Write(data[:idx])
	buf.WriteString(content)
	buf.Write(data[idx:])

	return os.WriteFile(path, buf.Bytes(), 0o644)
}
