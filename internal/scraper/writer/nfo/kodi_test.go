package nfo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

func TestKodi_Dialect(t *testing.T) {
	assert.Equal(t, "kodi", NewKodiNfoWriter().Dialect())
}

func TestKodi_WriteMovieNfo_Basic(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	moviePath := filepath.Join(tmpDir, "movie.nfo")
	w := NewKodiNfoWriter()

	require.NoError(t, w.WriteMovieNfo(context.Background(), newMovieFixture(), []string{moviePath}))

	content := readTestFile(t, moviePath)
	assert.Contains(t, content, "<movie>")
	assert.Contains(t, content, "<title>Inception</title>")
	assert.Contains(t, content, "<year>2010</year>")
	assert.Contains(t, content, "<plot>A dream within a dream</plot>")
	assert.Contains(t, content, "<genre>Sci-Fi</genre>")
	assert.Contains(t, content, "<studio>Warner Bros.</studio>")
	assert.Contains(t, content, "<original_filename>Inception.mkv</original_filename>")
	assert.Contains(t, content, "<streamdetails>")
	assertOrdered(
		t, content,
		"<title>Inception</title>",
		"<originaltitle>盗梦空间</originaltitle>",
		"<sorttitle>Inception</sorttitle>",
		"<year>2010</year>",
		"<ratings>",
		"<userrating>9.1</userrating>",
		"<outline>Dream infiltration</outline>",
		"<plot>A dream within a dream</plot>",
	)
	assertXMLLintValid(t, moviePath)
}

func TestKodi_WriteMovieNfo_DualPaths(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	paths := []string{filepath.Join(tmpDir, "movie.nfo"), filepath.Join(tmpDir, "Inception.nfo")}
	w := NewKodiNfoWriter()

	require.NoError(t, w.WriteMovieNfo(context.Background(), newMovieFixture(), paths))

	first := readTestFile(t, paths[0])
	second := readTestFile(t, paths[1])
	assert.Equal(t, first, second)
	assertXMLLintValid(t, paths[0])
	assertXMLLintValid(t, paths[1])
}

func TestKodi_WriteTvShowNfo_Basic(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	w := NewKodiNfoWriter()
	show := &core.TvShow{
		MediaEntity: core.MediaEntity{
			Title:         "Dark",
			OriginalTitle: "Dark",
			SortTitle:     "Dark",
			Year:          2017,
			Plot:          "A time travel mystery",
			Outline:       "Mystery in Winden",
			Genres:        []string{"Sci-Fi"},
			Studios:       []string{"Netflix"},
			IDs:           map[string]string{"imdb": "tt5753856", "tvdb": "328724"},
			Ratings: map[string]core.MediaRating{
				"imdb": {Value: 8.7, Votes: 500000, Max: 10},
			},
			ArtworkURLs: map[core.ArtworkType]string{
				core.ArtworkTypePoster:     "https://img.example/dark-poster.jpg",
				core.ArtworkTypeBackground: "https://img.example/dark-fanart.jpg",
			},
			Actors:    []core.Person{{Name: "Louis Hofmann", Role: "Jonas"}},
			Trailers:  []core.MediaTrailer{{URL: "https://trailers.example/dark", InNfo: true}},
			DateAdded: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		},
		FirstAired:  time.Date(2017, 12, 1, 0, 0, 0, 0, time.UTC),
		Status:      core.ShowStatusEnded,
		SeasonNames: map[int]string{1: "Season 1"},
	}

	require.NoError(t, w.WriteTvShowNfo(context.Background(), show, tmpDir))

	path := filepath.Join(tmpDir, "tvshow.nfo")
	content := readTestFile(t, path)
	assert.Contains(t, content, "<tvshow>")
	assert.Contains(t, content, "<showtitle>Dark</showtitle>")
	assert.Contains(t, content, `<uniqueid type="imdb" default="true">tt5753856</uniqueid>`)
	assert.Contains(t, content, `<namedseason number="1">Season 1</namedseason>`)
	assertOrdered(
		t, content,
		"<title>Dark</title>",
		"<originaltitle>Dark</originaltitle>",
		"<showtitle>Dark</showtitle>",
		"<sorttitle>Dark</sorttitle>",
		"<year>2017</year>",
	)
	assertXMLLintValid(t, path)
}

func TestKodi_WriteSeasonNfo_Basic(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	w := NewKodiNfoWriter()
	season := &core.TvShowSeason{
		TvShowID: uuid.New(),
		Number:   1,
		Name:     "Season 1",
		Plot:     "The first season",
		ArtworkURLs: map[core.ArtworkType]string{
			core.ArtworkTypeSeasonPoster: "https://img.example/season1.jpg",
		},
	}

	require.NoError(t, w.WriteSeasonNfo(context.Background(), season, tmpDir))

	path := filepath.Join(tmpDir, "season.nfo")
	content := readTestFile(t, path)
	assert.Contains(t, content, "<season>")
	assert.Contains(t, content, "<title>Season 1</title>")
	assert.Contains(t, content, "<seasonnumber>1</seasonnumber>")
	assert.Contains(t, content, "<plot>The first season</plot>")
	assertXMLLintValid(t, path)
}

func TestKodi_WriteEpisodeNfo_Basic(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	videoPath := filepath.Join(tmpDir, "Dark.S01E01.mkv")
	w := NewKodiNfoWriter()
	episode := &core.TvShowEpisode{
		MediaEntity: core.MediaEntity{
			Title:         "Secrets",
			OriginalTitle: "Secrets",
			Plot:          "Children disappear in Winden.",
			Tagline:       "Everything is connected",
			Runtime:       53,
			Certification: "TV-MA",
			Genres:        []string{"Sci-Fi"},
			Studios:       []string{"Netflix"},
			IDs:           map[string]string{"tvdb": "610001", "imdb": "tt5753856"},
			Ratings: map[string]core.MediaRating{
				"user": {Value: 8.2, Max: 10},
				"imdb": {Value: 8.1, Votes: 1200, Max: 10},
			},
			ArtworkURLs: map[core.ArtworkType]string{core.ArtworkTypeThumb: "https://img.example/ep1.jpg"},
			Directors:   []core.Person{{Name: "Baran bo Odar"}},
			Writers:     []core.Person{{Name: "Jantje Friese"}},
			Actors:      []core.Person{{Name: "Louis Hofmann", Role: "Jonas"}},
			MediaFiles: []core.MediaFile{{
				Type:       core.MediaFileTypeVideo,
				Filename:   "Dark.S01E01.mkv",
				VideoCodec: "h264",
				Width:      1920,
				Height:     1080,
				Duration:   3180,
				AudioStreams: []core.AudioStream{{
					Language: "deu",
					Codec:    "aac",
					Channels: 6,
				}},
				Subtitles: []core.Subtitle{{Language: "eng", Codec: "srt"}},
			}},
			DateAdded: time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC),
		},
		Season:     1,
		Episode:    1,
		FirstAired: time.Date(2017, 12, 1, 0, 0, 0, 0, time.UTC),
		Playcount:  1,
		Watched:    true,
	}

	require.NoError(t, w.WriteEpisodeNfo(context.Background(), episode, videoPath))

	path := filepath.Join(tmpDir, "Dark.S01E01.nfo")
	content := readTestFile(t, path)
	assert.Contains(t, content, "<episodedetails>")
	assert.Contains(t, content, "<season>1</season>")
	assert.Contains(t, content, "<episode>1</episode>")
	assert.Contains(t, content, "<streamdetails>")
	assert.Contains(t, content, "<subtitle>")
	assertXMLLintValid(t, path)
}

func TestKodi_WriteMovieNfo_MultipleUniqueIDs(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "movie.nfo")
	movie := newMovieFixture()
	movie.IDs = map[string]string{"douban": "3541415", "tmdb": "27205", "imdb": "tt1375666"}

	require.NoError(t, NewKodiNfoWriter().WriteMovieNfo(context.Background(), movie, []string{path}))

	content := readTestFile(t, path)
	assert.Contains(t, content, `<uniqueid type="imdb" default="true">tt1375666</uniqueid>`)
	assert.Contains(t, content, `<uniqueid type="tmdb">27205</uniqueid>`)
	assert.Contains(t, content, `<uniqueid type="douban">3541415</uniqueid>`)
	assert.Equal(t, 1, strings.Count(content, `default="true"`))
}

func TestKodi_WriteMovieNfo_DualRatingStyle(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "movie.nfo")

	require.NoError(t, NewKodiNfoWriter().WriteMovieNfo(context.Background(), newMovieFixture(), []string{path}))

	content := readTestFile(t, path)
	assert.Contains(t, content, `<ratings>`)
	assert.Contains(t, content, `<rating name="imdb" max="10">`)
	assert.Contains(t, content, `<value>8.5</value>`)
	assert.Contains(t, content, `<votes>123</votes>`)
	assert.Contains(t, content, `<rating>8.5</rating>`)
	assertOrdered(t, content, `<ratings>`, `</ratings>`, `<userrating>9.1</userrating>`, `<rating>8.5</rating>`)
}

func TestKodi_WriteMovieNfo_XmllintValid(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "movie.nfo")

	require.NoError(t, NewKodiNfoWriter().WriteMovieNfo(context.Background(), newMovieFixture(), []string{path}))
	assertXMLLintValid(t, path)
}

func newMovieFixture() *core.Movie {
	return &core.Movie{
		MediaEntity: core.MediaEntity{
			Title:         "Inception",
			OriginalTitle: "盗梦空间",
			SortTitle:     "Inception",
			Year:          2010,
			Plot:          "A dream within a dream",
			Outline:       "Dream infiltration",
			Tagline:       "Your mind is the scene of the crime",
			IDs:           map[string]string{"imdb": "tt1375666", "tmdb": "27205"},
			Ratings: map[string]core.MediaRating{
				"imdb": {Value: 8.5, Votes: 123, Max: 10},
				"user": {Value: 9.1, Max: 10},
			},
			Genres:    []string{"Sci-Fi", "Action"},
			Tags:      []string{"mind-bending"},
			Studios:   []string{"Warner Bros."},
			Countries: []string{"USA"},
			ArtworkURLs: map[core.ArtworkType]string{
				core.ArtworkTypePoster:     "https://img.example/poster.jpg",
				core.ArtworkTypeBackground: "https://img.example/fanart.jpg",
			},
			Actors: []core.Person{{Name: "Leonardo DiCaprio", Role: "Cobb", Order: 1}},
			Directors: []core.Person{{
				Name: "Christopher Nolan",
			}},
			Writers: []core.Person{{
				Name: "Christopher Nolan",
			}},
			Trailers: []core.MediaTrailer{{URL: "https://trailers.example/inception", InNfo: true}},
			MediaFiles: []core.MediaFile{{
				Type:        core.MediaFileTypeVideo,
				Filename:    "Inception.mkv",
				VideoCodec:  "hevc",
				Width:       3840,
				Height:      2160,
				AspectRatio: 1.778,
				Duration:    8880,
				AudioStreams: []core.AudioStream{{
					Language: "eng",
					Codec:    "dts",
					Channels: 6,
				}},
				Subtitles: []core.Subtitle{{Language: "eng", Codec: "srt"}},
			}},
			Certification: "PG-13",
			Runtime:       148,
			DateAdded:     time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		},
		ReleaseDate:  time.Date(2010, 7, 16, 0, 0, 0, 0, time.UTC),
		Top250:       13,
		MovieSetName: "The Dream Collection",
		Playcount:    1,
		Watched:      true,
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

func assertOrdered(t *testing.T, content string, parts ...string) {
	t.Helper()

	last := -1
	for _, part := range parts {
		idx := strings.Index(content, part)
		require.NotEqualf(t, -1, idx, "missing part %q", part)
		assert.Greater(t, idx, last, "expected %q after previous part", part)
		last = idx
	}
}

func assertXMLLintValid(t *testing.T, path string) {
	t.Helper()

	xmllintPath, err := exec.LookPath("xmllint")
	if err != nil {
		t.Log("xmllint not found, skipping lint")
		return
	}

	cmd := exec.Command(xmllintPath, "--noout", path)
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "xmllint failed: %s", string(output))
}
