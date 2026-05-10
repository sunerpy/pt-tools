package nfo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

var ctx = context.Background()

func TestJellyfin_WriteMovieNfo_CollectionNumber(t *testing.T) {
	tmpDir := t.TempDir()
	m := &core.Movie{
		MediaEntity: core.MediaEntity{
			Title: "Inception",
			Year:  2010,
			IDs:   map[string]string{},
		},
		TmdbCollection: 86311,
	}

	w := NewJellyfinNfoWriter()
	moviePath := filepath.Join(tmpDir, "movie.nfo")
	require.NoError(t, w.WriteMovieNfo(ctx, m, []string{moviePath}))

	content, err := os.ReadFile(moviePath)
	require.NoError(t, err)

	contentStr := string(content)
	require.Contains(t, contentStr, "<collectionnumber>86311</collectionnumber>")

	// Verify collectionnumber appears before closing </movie> tag
	collectIdx := strings.Index(contentStr, "<collectionnumber>")
	movieCloseIdx := strings.LastIndex(contentStr, "</movie>")
	require.Greater(t, collectIdx, 0, "collectionnumber tag not found")
	require.Greater(t, movieCloseIdx, collectIdx, "collectionnumber must appear before </movie>")
}

func TestJellyfin_WriteMovieNfo_NoCollection(t *testing.T) {
	tmpDir := t.TempDir()
	m := &core.Movie{
		MediaEntity: core.MediaEntity{
			Title: "Inception",
			Year:  2010,
			IDs:   map[string]string{},
		},
		TmdbCollection: 0,
	}

	w := NewJellyfinNfoWriter()
	moviePath := filepath.Join(tmpDir, "movie.nfo")
	require.NoError(t, w.WriteMovieNfo(ctx, m, []string{moviePath}))

	content, err := os.ReadFile(moviePath)
	require.NoError(t, err)

	contentStr := string(content)
	require.NotContains(t, contentStr, "<collectionnumber>")
}

func TestEmby_WriteMovieNfo_SetBlock(t *testing.T) {
	tmpDir := t.TempDir()
	m := &core.Movie{
		MediaEntity: core.MediaEntity{
			Title: "Inception",
			Year:  2010,
			IDs:   map[string]string{},
		},
		TmdbCollection: 86311,
		MovieSetName:   "诺兰系列",
	}

	w := NewEmbyNfoWriter()
	moviePath := filepath.Join(tmpDir, "movie.nfo")
	require.NoError(t, w.WriteMovieNfo(ctx, m, []string{moviePath}))

	content, err := os.ReadFile(moviePath)
	require.NoError(t, err)

	contentStr := string(content)
	require.Contains(t, contentStr, "<set tmdbcolid=\"86311\">")
	require.Contains(t, contentStr, "</set>")
	require.Contains(t, contentStr, "<name>诺兰系列</name>")

	// Verify that the set block contains the name tag (more specific check)
	// Find the set tmdbcolid block and verify it contains the name
	setBlockStart := strings.Index(contentStr, "<set tmdbcolid=\"86311\">")
	setBlockEnd := strings.Index(contentStr[setBlockStart:], "</set>") + len("</set>")
	setBlock := contentStr[setBlockStart : setBlockStart+setBlockEnd]
	require.Contains(t, setBlock, "<name>诺兰系列</name>")
}

func TestEmby_WriteMovieNfo_SetBlockWithoutName(t *testing.T) {
	tmpDir := t.TempDir()
	m := &core.Movie{
		MediaEntity: core.MediaEntity{
			Title: "Inception",
			Year:  2010,
			IDs:   map[string]string{},
		},
		TmdbCollection: 86311,
		MovieSetName:   "",
	}

	w := NewEmbyNfoWriter()
	moviePath := filepath.Join(tmpDir, "movie.nfo")
	require.NoError(t, w.WriteMovieNfo(ctx, m, []string{moviePath}))

	content, err := os.ReadFile(moviePath)
	require.NoError(t, err)

	contentStr := string(content)
	require.Contains(t, contentStr, "<set tmdbcolid=\"86311\">")
	require.Contains(t, contentStr, "</set>")
	// Name tag should not be present if empty
	require.NotContains(t, contentStr, "<name></name>")
}

func TestUniversal_WriteMovieNfo_BothExtensions(t *testing.T) {
	tmpDir := t.TempDir()
	m := &core.Movie{
		MediaEntity: core.MediaEntity{
			Title: "Inception",
			Year:  2010,
			IDs:   map[string]string{},
		},
		TmdbCollection: 86311,
		MovieSetName:   "Collection",
	}

	w := NewUniversalNfoWriter()
	moviePath := filepath.Join(tmpDir, "movie.nfo")
	require.NoError(t, w.WriteMovieNfo(ctx, m, []string{moviePath}))

	content, err := os.ReadFile(moviePath)
	require.NoError(t, err)

	contentStr := string(content)
	// Verify both Jellyfin and Emby extensions present
	require.Contains(t, contentStr, "<collectionnumber>86311</collectionnumber>")
	require.Contains(t, contentStr, "<set tmdbcolid=\"86311\">")
	require.Contains(t, contentStr, "</set>")
}

func TestUniversal_Dialect(t *testing.T) {
	w := NewUniversalNfoWriter()
	require.Equal(t, "universal", w.Dialect())
}

func TestJellyfin_Dialect(t *testing.T) {
	w := NewJellyfinNfoWriter()
	require.Equal(t, "jellyfin", w.Dialect())
}

func TestEmby_Dialect(t *testing.T) {
	w := NewEmbyNfoWriter()
	require.Equal(t, "emby", w.Dialect())
}

func TestAll_TvShowDelegatesToBase(t *testing.T) {
	tmpDir := t.TempDir()
	s := &core.TvShow{
		MediaEntity: core.MediaEntity{
			Title: "Breaking Bad",
			Year:  2008,
			IDs:   map[string]string{},
		},
	}

	for _, writer := range []interface {
		WriteTvShowNfo(context.Context, *core.TvShow, string) error
	}{
		NewJellyfinNfoWriter(),
		NewEmbyNfoWriter(),
		NewUniversalNfoWriter(),
	} {
		showDir := filepath.Join(tmpDir, "show")
		require.NoError(t, os.MkdirAll(showDir, 0o755))
		require.NoError(t, writer.WriteTvShowNfo(ctx, s, showDir))
		nfoPath := filepath.Join(showDir, "tvshow.nfo")
		require.FileExists(t, nfoPath)
		os.RemoveAll(showDir)
	}
}

func TestAll_WriteMultiplePaths(t *testing.T) {
	tmpDir := t.TempDir()
	m := &core.Movie{
		MediaEntity: core.MediaEntity{
			Title: "Inception",
			Year:  2010,
			IDs:   map[string]string{},
		},
		TmdbCollection: 86311,
		MovieSetName:   "Collection",
	}

	paths := []string{
		filepath.Join(tmpDir, "movie1.nfo"),
		filepath.Join(tmpDir, "movie2.nfo"),
	}

	w := NewUniversalNfoWriter()
	require.NoError(t, w.WriteMovieNfo(ctx, m, paths))

	for _, p := range paths {
		require.FileExists(t, p)
		content, err := os.ReadFile(p)
		require.NoError(t, err)
		contentStr := string(content)
		require.Contains(t, contentStr, "<collectionnumber>86311</collectionnumber>")
		require.Contains(t, contentStr, "<set tmdbcolid=\"86311\">")
	}
}

func TestXmllint_UniversalNfo(t *testing.T) {
	// Skip test if xmllint is not available
	if _, err := exec.LookPath("xmllint"); err != nil {
		t.Skip("xmllint not available")
	}

	tmpDir := t.TempDir()
	m := &core.Movie{
		MediaEntity: core.MediaEntity{
			Title: "Inception",
			Year:  2010,
			IDs:   map[string]string{"tmdb": "27205"},
		},
		TmdbCollection: 86311,
		MovieSetName:   "诺兰系列",
	}

	w := NewUniversalNfoWriter()
	moviePath := filepath.Join(tmpDir, "movie.nfo")
	require.NoError(t, w.WriteMovieNfo(ctx, m, []string{moviePath}))

	// Run xmllint to validate XML structure
	cmd := exec.Command("xmllint", "--noout", moviePath)
	err := cmd.Run()
	require.NoError(t, err, "xmllint validation failed - NFO is not valid XML")
}

func TestEmby_XmlEscaping(t *testing.T) {
	tmpDir := t.TempDir()
	m := &core.Movie{
		MediaEntity: core.MediaEntity{
			Title: "Test & <Movie>",
			Year:  2010,
			IDs:   map[string]string{},
		},
		TmdbCollection: 100,
		MovieSetName:   "Set with & < > chars",
	}

	w := NewEmbyNfoWriter()
	moviePath := filepath.Join(tmpDir, "movie.nfo")
	require.NoError(t, w.WriteMovieNfo(ctx, m, []string{moviePath}))

	content, err := os.ReadFile(moviePath)
	require.NoError(t, err)

	contentStr := string(content)
	// Verify XML entities are properly escaped
	require.Contains(t, contentStr, "Set with &amp; &lt; &gt; chars")
	require.NotContains(t, contentStr, "Set with & < > chars") // Raw chars should not appear in set name context
}
