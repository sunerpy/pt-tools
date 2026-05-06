package nfo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

type KodiNfoWriter struct {
	mergeMode MergeMode
}

type MergeMode int

const (
	MergeFillEmpty MergeMode = iota
	MergeOverwrite
	MergeOnlyLocked
)

var uniqueIDPriority = []string{"imdb", "tmdb", "tvdb", "douban"}

func NewKodiNfoWriter() *KodiNfoWriter {
	return &KodiNfoWriter{mergeMode: MergeFillEmpty}
}

func NewKodiNfoWriterWithMode(m MergeMode) *KodiNfoWriter {
	return &KodiNfoWriter{mergeMode: m}
}

func (w *KodiNfoWriter) Dialect() string { return "kodi" }

func (w *KodiNfoWriter) WriteMovieNfo(ctx context.Context, m *core.Movie, paths []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m == nil {
		return errors.New("nil movie")
	}
	if len(paths) == 0 {
		return errors.New("empty movie nfo paths")
	}

	content, err := buildMovieNfo(m)
	if err != nil {
		return err
	}

	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		final, err := w.mergeWithExisting(p, content, "movie")
		if err != nil {
			return err
		}
		if err := os.WriteFile(p, final, 0o644); err != nil {
			return err
		}
	}

	return nil
}

func (w *KodiNfoWriter) WriteTvShowNfo(ctx context.Context, s *core.TvShow, showDir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return errors.New("nil tv show")
	}
	if showDir == "" {
		return errors.New("empty tv show dir")
	}

	content, err := buildTvShowNfo(s)
	if err != nil {
		return err
	}

	path := filepath.Join(showDir, "tvshow.nfo")
	final, err := w.mergeWithExisting(path, content, "tvshow")
	if err != nil {
		return err
	}

	return os.WriteFile(path, final, 0o644)
}

func (w *KodiNfoWriter) WriteSeasonNfo(ctx context.Context, s *core.TvShowSeason, seasonDir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return errors.New("nil season")
	}
	if seasonDir == "" {
		return errors.New("empty season dir")
	}

	content, err := buildSeasonNfo(s)
	if err != nil {
		return err
	}

	path := filepath.Join(seasonDir, "season.nfo")
	final, err := w.mergeWithExisting(path, content, "season")
	if err != nil {
		return err
	}

	return os.WriteFile(path, final, 0o644)
}

func (w *KodiNfoWriter) WriteEpisodeNfo(ctx context.Context, e *core.TvShowEpisode, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e == nil {
		return errors.New("nil episode")
	}
	if path == "" {
		return errors.New("empty episode path")
	}

	content, err := buildEpisodeNfo(e)
	if err != nil {
		return err
	}

	nfoPath := normalizeEpisodeNfoPath(path)
	final, err := w.mergeWithExisting(nfoPath, content, "episodedetails")
	if err != nil {
		return err
	}

	return os.WriteFile(nfoPath, final, 0o644)
}

func (w *KodiNfoWriter) mergeWithExisting(existingPath string, newContent []byte, rootTag string) ([]byte, error) {
	switch w.mergeMode {
	case MergeFillEmpty, MergeOverwrite, MergeOnlyLocked:
		return mergeWithExisting(existingPath, newContent, rootTag)
	default:
		return mergeWithExisting(existingPath, newContent, rootTag)
	}
}

func buildMovieNfo(m *core.Movie) ([]byte, error) {
	var buf bytes.Buffer
	x := newXMLWriter(&buf)

	if err := x.openBlock("movie"); err != nil {
		return nil, err
	}
	if err := writeMovieBody(x, m); err != nil {
		return nil, err
	}
	if err := x.closeBlock("movie"); err != nil {
		return nil, err
	}
	if err := x.flush(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func buildTvShowNfo(s *core.TvShow) ([]byte, error) {
	var buf bytes.Buffer
	x := newXMLWriter(&buf)

	if err := x.openBlock("tvshow"); err != nil {
		return nil, err
	}
	if err := writeTvShowBody(x, s); err != nil {
		return nil, err
	}
	if err := x.closeBlock("tvshow"); err != nil {
		return nil, err
	}
	if err := x.flush(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func buildSeasonNfo(s *core.TvShowSeason) ([]byte, error) {
	var buf bytes.Buffer
	x := newXMLWriter(&buf)

	if err := x.openBlock("season"); err != nil {
		return nil, err
	}
	if err := writeSeasonBody(x, s); err != nil {
		return nil, err
	}
	if err := x.closeBlock("season"); err != nil {
		return nil, err
	}
	if err := x.flush(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func buildEpisodeNfo(e *core.TvShowEpisode) ([]byte, error) {
	var buf bytes.Buffer
	x := newXMLWriter(&buf)

	if err := x.openBlock("episodedetails"); err != nil {
		return nil, err
	}
	if err := writeEpisodeBody(x, e); err != nil {
		return nil, err
	}
	if err := x.closeBlock("episodedetails"); err != nil {
		return nil, err
	}
	if err := x.flush(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func writeMovieBody(x *xmlWriter, m *core.Movie) error {
	if err := writeMediaEntityShared(x, m.MediaEntity, movieEntityOptions{showTitle: false}); err != nil {
		return err
	}

	if err := writeMovieSpecific(x, m); err != nil {
		return err
	}

	return nil
}

func writeTvShowBody(x *xmlWriter, s *core.TvShow) error {
	if s.Title != "" {
		if err := x.writeElement("title", s.Title); err != nil {
			return err
		}
	}
	if s.OriginalTitle != "" {
		if err := x.writeElement("originaltitle", s.OriginalTitle); err != nil {
			return err
		}
	}
	if s.Title != "" {
		if err := x.writeElement("showtitle", s.Title); err != nil {
			return err
		}
	}
	if s.SortTitle != "" {
		if err := x.writeElement("sorttitle", s.SortTitle); err != nil {
			return err
		}
	}
	if s.Year > 0 {
		if err := x.writeElementInt("year", s.Year); err != nil {
			return err
		}
	}
	if err := writeRatingsBlock(x, s.Ratings); err != nil {
		return err
	}
	if err := writeUserRating(x, s.Ratings); err != nil {
		return err
	}
	if err := writeLegacyRating(x, s.Ratings); err != nil {
		return err
	}
	if err := writeTop250(x, s.Ratings); err != nil {
		return err
	}
	if err := writeEpisodeSummaryCounts(x, s); err != nil {
		return err
	}
	if s.Outline != "" {
		if err := x.writeElement("outline", s.Outline); err != nil {
			return err
		}
	}
	if s.Plot != "" {
		if err := x.writeElement("plot", s.Plot); err != nil {
			return err
		}
	}
	if s.Tagline != "" {
		if err := x.writeElement("tagline", s.Tagline); err != nil {
			return err
		}
	}
	if s.Runtime > 0 {
		if err := x.writeElementInt("runtime", s.Runtime); err != nil {
			return err
		}
	}
	if err := writeThumbs(x, s.ArtworkURLs, false); err != nil {
		return err
	}
	if err := writeFanart(x, s.ArtworkURLs); err != nil {
		return err
	}
	if s.Certification != "" {
		if err := x.writeElement("mpaa", s.Certification); err != nil {
			return err
		}
		if err := x.writeElement("certification", s.Certification); err != nil {
			return err
		}
	}
	if err := writeEpisodeGuide(x, s); err != nil {
		return err
	}
	if err := writePrimaryID(x, s.IDs); err != nil {
		return err
	}
	if err := writeUniqueIDs(x, s.IDs); err != nil {
		return err
	}
	for _, genre := range s.Genres {
		if err := x.writeElement("genre", genre); err != nil {
			return err
		}
	}
	if !s.FirstAired.IsZero() {
		date := formatDate(s.FirstAired)
		if err := x.writeElement("premiered", date); err != nil {
			return err
		}
	}
	if status := formatShowStatus(s.Status); status != "" {
		if err := x.writeElement("status", status); err != nil {
			return err
		}
	}
	if code := primaryCode(s.IDs); code != "" {
		if err := x.writeElement("code", code); err != nil {
			return err
		}
	}
	if !s.FirstAired.IsZero() {
		if err := x.writeElement("aired", formatDate(s.FirstAired)); err != nil {
			return err
		}
	}
	for _, studio := range s.Studios {
		if err := x.writeElement("studio", studio); err != nil {
			return err
		}
	}
	if err := writeTrailer(x, s.Trailers); err != nil {
		return err
	}
	if err := writeActors(x, s.Actors); err != nil {
		return err
	}
	if err := writeNamedSeasons(x, s.SeasonNames); err != nil {
		return err
	}
	if err := writeResume(x); err != nil {
		return err
	}
	if !s.DateAdded.IsZero() {
		if err := x.writeElement("dateadded", formatDateTime(s.DateAdded)); err != nil {
			return err
		}
	}

	return nil
}

func writeSeasonBody(x *xmlWriter, s *core.TvShowSeason) error {
	if s.Name != "" {
		if err := x.writeElement("title", s.Name); err != nil {
			return err
		}
	}
	if s.Number > 0 || s.Number == 0 {
		if err := x.writeElementInt("seasonnumber", s.Number); err != nil {
			return err
		}
	}
	if s.Plot != "" {
		if err := x.writeElement("plot", s.Plot); err != nil {
			return err
		}
	}
	if err := writeThumbs(x, s.ArtworkURLs, true); err != nil {
		return err
	}

	return nil
}

func writeEpisodeBody(x *xmlWriter, e *core.TvShowEpisode) error {
	if e.Title != "" {
		if err := x.writeElement("title", e.Title); err != nil {
			return err
		}
	}
	if e.OriginalTitle != "" {
		if err := x.writeElement("originaltitle", e.OriginalTitle); err != nil {
			return err
		}
	}
	if e.Title != "" {
		if err := x.writeElement("showtitle", e.Title); err != nil {
			return err
		}
	}
	if err := x.writeElementInt("season", e.Season); err != nil {
		return err
	}
	if err := x.writeElementInt("episode", e.Episode); err != nil {
		return err
	}
	if e.DisplaySeason != nil {
		if err := x.writeElementInt("displayseason", *e.DisplaySeason); err != nil {
			return err
		}
	}
	if e.DisplayEpisode != nil {
		if err := x.writeElementInt("displayepisode", *e.DisplayEpisode); err != nil {
			return err
		}
	}
	if err := writeUserRating(x, e.Ratings); err != nil {
		return err
	}
	if err := writeRatingsBlock(x, e.Ratings); err != nil {
		return err
	}
	if err := writeLegacyRating(x, e.Ratings); err != nil {
		return err
	}
	if e.Plot != "" {
		if err := x.writeElement("plot", e.Plot); err != nil {
			return err
		}
	}
	if e.Tagline != "" {
		if err := x.writeElement("tagline", e.Tagline); err != nil {
			return err
		}
	}
	if e.Runtime > 0 {
		if err := x.writeElementInt("runtime", e.Runtime); err != nil {
			return err
		}
	}
	if err := writeThumbs(x, e.ArtworkURLs, false); err != nil {
		return err
	}
	if e.Certification != "" {
		if err := x.writeElement("mpaa", e.Certification); err != nil {
			return err
		}
	}
	if e.Playcount > 0 {
		if err := x.writeElementInt("playcount", e.Playcount); err != nil {
			return err
		}
	}
	if e.Watched {
		if err := x.writeElement("watched", "true"); err != nil {
			return err
		}
	}
	if err := writePrimaryID(x, e.IDs); err != nil {
		return err
	}
	if err := writeUniqueIDs(x, e.IDs); err != nil {
		return err
	}
	for _, genre := range e.Genres {
		if err := x.writeElement("genre", genre); err != nil {
			return err
		}
	}
	if err := writeCredits(x, e.Writers); err != nil {
		return err
	}
	if err := writeDirectors(x, e.Directors); err != nil {
		return err
	}
	if !e.FirstAired.IsZero() {
		if err := x.writeElement("premiered", formatDate(e.FirstAired)); err != nil {
			return err
		}
	}
	for _, studio := range e.Studios {
		if err := x.writeElement("studio", studio); err != nil {
			return err
		}
	}
	if err := writeActors(x, e.Actors); err != nil {
		return err
	}
	if err := writeFileInfo(x, e.MediaFiles); err != nil {
		return err
	}
	if err := writeResume(x); err != nil {
		return err
	}
	if !e.DateAdded.IsZero() {
		if err := x.writeElement("dateadded", formatDateTime(e.DateAdded)); err != nil {
			return err
		}
	}
	if !e.FirstAired.IsZero() {
		if err := x.writeElement("aired", formatDate(e.FirstAired)); err != nil {
			return err
		}
	}

	return nil
}

type movieEntityOptions struct {
	showTitle bool
}

func writeMediaEntityShared(x *xmlWriter, entity core.MediaEntity, opts movieEntityOptions) error {
	if entity.Title != "" {
		if err := x.writeElement("title", entity.Title); err != nil {
			return err
		}
	}
	if entity.OriginalTitle != "" {
		if err := x.writeElement("originaltitle", entity.OriginalTitle); err != nil {
			return err
		}
	}
	if entity.SortTitle != "" {
		if err := x.writeElement("sorttitle", entity.SortTitle); err != nil {
			return err
		}
	}
	if entity.Year > 0 {
		if err := x.writeElementInt("year", entity.Year); err != nil {
			return err
		}
	}
	if err := writeRatingsBlock(x, entity.Ratings); err != nil {
		return err
	}
	if err := writeUserRating(x, entity.Ratings); err != nil {
		return err
	}
	if err := writeLegacyRating(x, entity.Ratings); err != nil {
		return err
	}

	return nil
}

func writeMovieSpecific(x *xmlWriter, m *core.Movie) error {
	if m.Top250 > 0 {
		if err := x.writeElementInt("top250", m.Top250); err != nil {
			return err
		}
	}
	if m.Outline != "" {
		if err := x.writeElement("outline", m.Outline); err != nil {
			return err
		}
	}
	if m.Plot != "" {
		if err := x.writeElement("plot", m.Plot); err != nil {
			return err
		}
	}
	if m.Tagline != "" {
		if err := x.writeElement("tagline", m.Tagline); err != nil {
			return err
		}
	}
	if m.Runtime > 0 {
		if err := x.writeElementInt("runtime", m.Runtime); err != nil {
			return err
		}
	}
	if err := writeThumbs(x, m.ArtworkURLs, false); err != nil {
		return err
	}
	if err := writeFanart(x, m.ArtworkURLs); err != nil {
		return err
	}
	if m.Certification != "" {
		if err := x.writeElement("mpaa", m.Certification); err != nil {
			return err
		}
		if err := x.writeElement("certification", m.Certification); err != nil {
			return err
		}
	}
	if err := writePrimaryID(x, m.IDs); err != nil {
		return err
	}
	if err := writeUniqueIDs(x, m.IDs); err != nil {
		return err
	}
	for _, country := range m.Countries {
		if err := x.writeElement("country", country); err != nil {
			return err
		}
	}
	if !m.ReleaseDate.IsZero() {
		date := formatDate(m.ReleaseDate)
		if err := x.writeElement("premiered", date); err != nil {
			return err
		}
		if err := x.writeElement("aired", date); err != nil {
			return err
		}
	}
	for _, studio := range m.Studios {
		if err := x.writeElement("studio", studio); err != nil {
			return err
		}
	}
	for _, genre := range m.Genres {
		if err := x.writeElement("genre", genre); err != nil {
			return err
		}
	}
	for _, tag := range m.Tags {
		if err := x.writeElement("tag", tag); err != nil {
			return err
		}
	}
	if m.MovieSetName != "" {
		if err := x.writeElement("set", m.MovieSetName); err != nil {
			return err
		}
	}
	if err := writeTrailer(x, m.Trailers); err != nil {
		return err
	}
	if err := writeDirectors(x, m.Directors); err != nil {
		return err
	}
	if err := writeCredits(x, m.Writers); err != nil {
		return err
	}
	if err := writeActors(x, m.Actors); err != nil {
		return err
	}
	if err := writeFileInfo(x, m.MediaFiles); err != nil {
		return err
	}
	if !m.DateAdded.IsZero() {
		if err := x.writeElement("dateadded", formatDateTime(m.DateAdded)); err != nil {
			return err
		}
	}
	if m.Playcount > 0 {
		if err := x.writeElementInt("playcount", m.Playcount); err != nil {
			return err
		}
	}
	if m.Watched {
		if err := x.writeElement("watched", "true"); err != nil {
			return err
		}
	}
	if name := originalFilename(m.MediaFiles); name != "" {
		if err := x.writeElement("original_filename", name); err != nil {
			return err
		}
	}
	if err := writeResume(x); err != nil {
		return err
	}

	return nil
}

func writeRatingsBlock(x *xmlWriter, ratings map[string]core.MediaRating) error {
	ordered := orderedRatings(ratings)
	if len(ordered) == 0 {
		return nil
	}

	return x.writeBlock("ratings", func() error {
		for _, rating := range ordered {
			attrs := []string{"name", rating.ID, "max", ratingMax(rating)}
			if err := x.openBlockAttr("rating", attrs); err != nil {
				return err
			}
			if err := x.writeElement("value", fmt.Sprintf("%.1f", rating.Value)); err != nil {
				return err
			}
			if rating.Votes > 0 {
				if err := x.writeElementInt("votes", rating.Votes); err != nil {
					return err
				}
			}
			if err := x.closeBlock("rating"); err != nil {
				return err
			}
		}
		return nil
	})
}

func writeUserRating(x *xmlWriter, ratings map[string]core.MediaRating) error {
	if rating, ok := ratings["user"]; ok && rating.Value > 0 {
		return x.writeElementFloat("userrating", rating.Value)
	}

	return nil
}

func writeLegacyRating(x *xmlWriter, ratings map[string]core.MediaRating) error {
	rating, ok := preferredRating(ratings)
	if !ok {
		return nil
	}

	return x.writeElementFloat("rating", rating.Value)
}

func writeTop250(x *xmlWriter, ratings map[string]core.MediaRating) error {
	if rating, ok := ratings["top250"]; ok && rating.Votes > 0 {
		return x.writeElementInt("top250", rating.Votes)
	}

	return nil
}

func writePrimaryID(x *xmlWriter, ids map[string]string) error {
	if id := primaryCode(ids); id != "" {
		return x.writeElement("id", id)
	}

	return nil
}

func writeUniqueIDs(x *xmlWriter, ids map[string]string) error {
	ordered := orderedIDs(ids)
	if len(ordered) == 0 {
		return nil
	}

	defaultID := defaultUniqueIDType(ids)
	for _, key := range ordered {
		attrs := []string{"type", key}
		if key == defaultID {
			attrs = append(attrs, "default", "true")
		}
		if err := x.writeElementAttr("uniqueid", attrs, ids[key]); err != nil {
			return err
		}
	}

	return nil
}

func writeThumbs(x *xmlWriter, artwork map[core.ArtworkType]string, seasonOnly bool) error {
	thumbs := artworkThumbs(artwork, seasonOnly)
	for _, thumb := range thumbs {
		if err := x.writeElement("thumb", thumb); err != nil {
			return err
		}
	}

	return nil
}

func writeFanart(x *xmlWriter, artwork map[core.ArtworkType]string) error {
	url := strings.TrimSpace(artwork[core.ArtworkTypeBackground])
	if url == "" {
		return nil
	}

	return x.writeBlock("fanart", func() error {
		return x.writeElement("thumb", url)
	})
}

func writeTrailer(x *xmlWriter, trailers []core.MediaTrailer) error {
	for _, trailer := range trailers {
		if trailer.URL == "" || !trailer.InNfo {
			continue
		}
		return x.writeElement("trailer", trailer.URL)
	}

	return nil
}

func writeDirectors(x *xmlWriter, people []core.Person) error {
	for _, person := range people {
		if person.Name == "" {
			continue
		}
		if err := x.writeElement("director", person.Name); err != nil {
			return err
		}
	}

	return nil
}

func writeCredits(x *xmlWriter, people []core.Person) error {
	for _, person := range people {
		if person.Name == "" {
			continue
		}
		if err := x.writeElement("credits", person.Name); err != nil {
			return err
		}
	}

	return nil
}

func writeActors(x *xmlWriter, actors []core.Person) error {
	for _, actor := range actors {
		if actor.Name == "" {
			continue
		}
		if err := x.writeBlock("actor", func() error {
			if err := x.writeElement("name", actor.Name); err != nil {
				return err
			}
			if actor.Role != "" {
				if err := x.writeElement("role", actor.Role); err != nil {
					return err
				}
			}
			if actor.Order > 0 {
				if err := x.writeElementInt("order", actor.Order); err != nil {
					return err
				}
			}
			if actor.ThumbURL != "" {
				if err := x.writeElement("thumb", actor.ThumbURL); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

func writeFileInfo(x *xmlWriter, files []core.MediaFile) error {
	file := primaryMediaFile(files)
	if file == nil {
		return nil
	}

	return x.writeBlock("fileinfo", func() error {
		return x.writeBlock("streamdetails", func() error {
			if err := writeVideoStream(x, *file); err != nil {
				return err
			}
			if err := writeAudioStreams(x, file.AudioStreams); err != nil {
				return err
			}
			return writeSubtitleStreams(x, file.Subtitles)
		})
	})
}

func writeVideoStream(x *xmlWriter, file core.MediaFile) error {
	if file.VideoCodec == "" && file.Width == 0 && file.Height == 0 && file.Duration == 0 && file.AspectRatio == 0 {
		return nil
	}

	return x.writeBlock("video", func() error {
		if file.VideoCodec != "" {
			if err := x.writeElement("codec", file.VideoCodec); err != nil {
				return err
			}
		}
		if file.AspectRatio > 0 {
			if err := x.writeElement("aspect", fmt.Sprintf("%.3f", file.AspectRatio)); err != nil {
				return err
			}
		}
		if file.Width > 0 {
			if err := x.writeElementInt("width", file.Width); err != nil {
				return err
			}
		}
		if file.Height > 0 {
			if err := x.writeElementInt("height", file.Height); err != nil {
				return err
			}
		}
		if file.Duration > 0 {
			if err := x.writeElementInt("durationinseconds", file.Duration); err != nil {
				return err
			}
		}
		return nil
	})
}

func writeAudioStreams(x *xmlWriter, streams []core.AudioStream) error {
	for _, stream := range streams {
		if err := x.writeBlock("audio", func() error {
			if stream.Codec != "" {
				if err := x.writeElement("codec", stream.Codec); err != nil {
					return err
				}
			}
			if stream.Language != "" {
				if err := x.writeElement("language", stream.Language); err != nil {
					return err
				}
			}
			if stream.Channels > 0 {
				if err := x.writeElementInt("channels", stream.Channels); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

func writeSubtitleStreams(x *xmlWriter, subtitles []core.Subtitle) error {
	for _, subtitle := range subtitles {
		if err := x.writeBlock("subtitle", func() error {
			if subtitle.Language != "" {
				if err := x.writeElement("language", subtitle.Language); err != nil {
					return err
				}
			}
			if subtitle.Codec != "" {
				if err := x.writeElement("codec", subtitle.Codec); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

func writeNamedSeasons(x *xmlWriter, seasonNames map[int]string) error {
	keys := make([]int, 0, len(seasonNames))
	for number, name := range seasonNames {
		if strings.TrimSpace(name) == "" {
			continue
		}
		keys = append(keys, number)
	}
	sort.Ints(keys)

	for _, number := range keys {
		if err := x.writeElementAttr("namedseason", []string{"number", fmt.Sprintf("%d", number)}, seasonNames[number]); err != nil {
			return err
		}
	}

	return nil
}

func writeResume(x *xmlWriter) error {
	return nil
}

func writeEpisodeSummaryCounts(x *xmlWriter, _ *core.TvShow) error {
	return nil
}

func writeEpisodeGuide(x *xmlWriter, _ *core.TvShow) error {
	return nil
}

func orderedRatings(ratings map[string]core.MediaRating) []core.MediaRating {
	ordered := make([]core.MediaRating, 0, len(ratings))
	for id, rating := range ratings {
		if rating.Value <= 0 {
			continue
		}
		rating.ID = id
		ordered = append(ordered, rating)
	}

	sort.Slice(ordered, func(i, j int) bool {
		return uniqueIDRank(ordered[i].ID) < uniqueIDRank(ordered[j].ID)
	})

	return ordered
}

func preferredRating(ratings map[string]core.MediaRating) (core.MediaRating, bool) {
	for _, key := range uniqueIDPriority {
		if rating, ok := ratings[key]; ok && rating.Value > 0 {
			rating.ID = key
			return rating, true
		}
	}

	for key, rating := range ratings {
		if key == "user" || key == "top250" || rating.Value <= 0 {
			continue
		}
		rating.ID = key
		return rating, true
	}

	return core.MediaRating{}, false
}

func orderedIDs(ids map[string]string) []string {
	keys := make([]string, 0, len(ids))
	for key, value := range ids {
		if strings.TrimSpace(value) == "" {
			continue
		}
		keys = append(keys, key)
	}

	sort.Slice(keys, func(i, j int) bool {
		return uniqueIDRank(keys[i]) < uniqueIDRank(keys[j])
	})

	return keys
}

func defaultUniqueIDType(ids map[string]string) string {
	for _, key := range uniqueIDPriority {
		if strings.TrimSpace(ids[key]) != "" {
			return key
		}
	}

	keys := orderedIDs(ids)
	if len(keys) == 0 {
		return ""
	}

	return keys[0]
}

func primaryCode(ids map[string]string) string {
	for _, key := range uniqueIDPriority {
		if value := strings.TrimSpace(ids[key]); value != "" {
			return value
		}
	}

	for _, value := range ids {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
}

func ratingMax(rating core.MediaRating) string {
	if rating.Max > 0 {
		return fmt.Sprintf("%d", rating.Max)
	}

	return "10"
}

func artworkThumbs(artwork map[core.ArtworkType]string, seasonOnly bool) []string {
	if len(artwork) == 0 {
		return nil
	}

	priority := []core.ArtworkType{core.ArtworkTypePoster, core.ArtworkTypeThumb, core.ArtworkTypeSeasonPoster, core.ArtworkTypeSeasonThumb}
	if seasonOnly {
		priority = []core.ArtworkType{core.ArtworkTypeSeasonPoster, core.ArtworkTypeSeasonThumb, core.ArtworkTypePoster, core.ArtworkTypeThumb}
	}

	seen := make(map[string]struct{})
	thumbs := make([]string, 0, len(priority))
	for _, kind := range priority {
		url := strings.TrimSpace(artwork[kind])
		if url == "" {
			continue
		}
		if _, ok := seen[url]; ok {
			continue
		}
		seen[url] = struct{}{}
		thumbs = append(thumbs, url)
	}

	return thumbs
}

func primaryMediaFile(files []core.MediaFile) *core.MediaFile {
	for i := range files {
		if files[i].Type == core.MediaFileTypeVideo {
			return &files[i]
		}
	}
	if len(files) == 0 {
		return nil
	}

	return &files[0]
}

func originalFilename(files []core.MediaFile) string {
	file := primaryMediaFile(files)
	if file == nil {
		return ""
	}
	if file.Filename != "" {
		return file.Filename
	}
	if file.Path != "" {
		return filepath.Base(file.Path)
	}

	return ""
}

func normalizeEpisodeNfoPath(path string) string {
	if strings.EqualFold(filepath.Ext(path), ".nfo") {
		return path
	}

	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if base == "" {
		return path + ".nfo"
	}

	return filepath.Join(filepath.Dir(path), base+".nfo")
}

func uniqueIDRank(key string) int {
	for idx, candidate := range uniqueIDPriority {
		if key == candidate {
			return idx
		}
	}

	return len(uniqueIDPriority) + int(key[0])
}

func formatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

func formatDateTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

func formatShowStatus(status core.ShowStatus) string {
	switch status {
	case core.ShowStatusReturning:
		return "Continuing"
	case core.ShowStatusEnded:
		return "Ended"
	case core.ShowStatusCanceled:
		return "Canceled"
	case core.ShowStatusPilot:
		return "Pilot"
	case core.ShowStatusInProduction:
		return "In Production"
	default:
		return ""
	}
}
