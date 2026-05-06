package core

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaType_String(t *testing.T) {
	tests := []struct {
		val  MediaType
		want string
	}{
		{MediaTypeUnknown, "unknown"},
		{MediaTypeMovie, "movie"},
		{MediaTypeTvShow, "tv_show"},
		{MediaTypeSeason, "season"},
		{MediaTypeEpisode, "episode"},
		{MediaType(999), "unknown"},
		{MediaType(-1), "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.val.String())
		})
	}
}

func TestArtworkType_String(t *testing.T) {
	tests := []struct {
		val  ArtworkType
		want string
	}{
		{ArtworkTypeUnknown, "unknown"},
		{ArtworkTypePoster, "poster"},
		{ArtworkTypeBackground, "fanart"},
		{ArtworkTypeBanner, "banner"},
		{ArtworkTypeClearlogo, "clearlogo"},
		{ArtworkTypeClearart, "clearart"},
		{ArtworkTypeDisc, "disc"},
		{ArtworkTypeKeyart, "keyart"},
		{ArtworkTypeThumb, "landscape"},
		{ArtworkTypeCharacterart, "characterart"},
		{ArtworkTypeSeasonPoster, "season_poster"},
		{ArtworkTypeSeasonFanart, "season_fanart"},
		{ArtworkTypeSeasonBanner, "season_banner"},
		{ArtworkTypeSeasonThumb, "season_thumb"},
		{ArtworkTypeActor, "actor"},
		{ArtworkType(999), "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.val.String())
		})
	}
}

func TestPersonType_String(t *testing.T) {
	tests := []struct {
		val  PersonType
		want string
	}{
		{PersonTypeUnknown, "unknown"},
		{PersonTypeActor, "actor"},
		{PersonTypeDirector, "director"},
		{PersonTypeWriter, "writer"},
		{PersonTypeProducer, "producer"},
		{PersonTypeGuest, "guest"},
		{PersonTypeComposer, "composer"},
		{PersonTypeEditor, "editor"},
		{PersonTypeCamera, "camera"},
		{PersonTypeOther, "other"},
		{PersonType(42), "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.val.String())
		})
	}
}

func TestMediaFileType_String(t *testing.T) {
	tests := []struct {
		val  MediaFileType
		want string
	}{
		{MediaFileTypeUnknown, "unknown"},
		{MediaFileTypeVideo, "video"},
		{MediaFileTypeAudio, "audio"},
		{MediaFileTypeSubtitle, "subtitle"},
		{MediaFileTypePoster, "poster"},
		{MediaFileTypeFanart, "fanart"},
		{MediaFileTypeBanner, "banner"},
		{MediaFileTypeClearart, "clearart"},
		{MediaFileTypeClearlogo, "clearlogo"},
		{MediaFileTypeDisc, "disc"},
		{MediaFileTypeKeyart, "keyart"},
		{MediaFileTypeThumb, "thumb"},
		{MediaFileTypeNfo, "nfo"},
		{MediaFileTypeTrailer, "trailer"},
		{MediaFileTypeExtra, "extra"},
		{MediaFileTypeSample, "sample"},
		{MediaFileType(999), "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.val.String())
		})
	}
}

func TestMediaSource_String(t *testing.T) {
	tests := []struct {
		val  MediaSource
		want string
	}{
		{MediaSourceUnknown, "unknown"},
		{MediaSourceDVD, "dvd"},
		{MediaSourceBluRay, "bluray"},
		{MediaSourceHDRip, "hdrip"},
		{MediaSourceWEBDL, "webdl"},
		{MediaSourceWEBRip, "webrip"},
		{MediaSourceTV, "tv"},
		{MediaSource(999), "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.val.String())
		})
	}
}

func TestShowStatus_String(t *testing.T) {
	tests := []struct {
		val  ShowStatus
		want string
	}{
		{ShowStatusUnknown, "unknown"},
		{ShowStatusReturning, "returning"},
		{ShowStatusEnded, "ended"},
		{ShowStatusCanceled, "canceled"},
		{ShowStatusPilot, "pilot"},
		{ShowStatusInProduction, "in_production"},
		{ShowStatus(999), "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.val.String())
		})
	}
}

func TestEpisodeGroup_String(t *testing.T) {
	tests := []struct {
		val  EpisodeGroup
		want string
	}{
		{EpisodeGroupUnknown, "unknown"},
		{EpisodeGroupAired, "aired"},
		{EpisodeGroupDVD, "dvd"},
		{EpisodeGroupAbsolute, "absolute"},
		{EpisodeGroupAlternate, "alternate"},
		{EpisodeGroup(999), "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.val.String())
		})
	}
}

func TestMediaEntity_EmbeddedFieldsAccess(t *testing.T) {
	m := &Movie{MediaEntity: MediaEntity{Title: "Inception", Year: 2010}}
	assert.Equal(t, "Inception", m.Title)
	assert.Equal(t, 2010, m.Year)

	tv := &TvShow{MediaEntity: MediaEntity{Title: "Breaking Bad"}}
	assert.Equal(t, "Breaking Bad", tv.Title)

	ep := &TvShowEpisode{MediaEntity: MediaEntity{Title: "Pilot"}, Season: 1, Episode: 1}
	assert.Equal(t, "Pilot", ep.Title)
	assert.Equal(t, 1, ep.Season)
	assert.Equal(t, 1, ep.Episode)
}

func TestMovie_JSONRoundtrip(t *testing.T) {
	id := uuid.New()
	original := Movie{
		MediaEntity: MediaEntity{
			DBID:     id,
			Title:    "Interstellar",
			Year:     2014,
			Plot:     "A team of explorers travel through a wormhole.",
			IDs:      map[string]string{"imdb": "tt0816692", "tmdb": "157336"},
			Genres:   []string{"Sci-Fi", "Drama"},
			Runtime:  169,
			Provider: "tmdb",
		},
		ReleaseDate: time.Date(2014, 11, 7, 0, 0, 0, 0, time.UTC),
		MediaSource: MediaSourceBluRay,
		Top250:      29,
		Watched:     true,
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Movie
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, original.Title, decoded.Title)
	assert.Equal(t, original.Year, decoded.Year)
	assert.Equal(t, original.Plot, decoded.Plot)
	assert.Equal(t, original.IDs, decoded.IDs)
	assert.Equal(t, original.Genres, decoded.Genres)
	assert.Equal(t, original.Runtime, decoded.Runtime)
	assert.Equal(t, original.Provider, decoded.Provider)
	assert.Equal(t, original.MediaSource, decoded.MediaSource)
	assert.Equal(t, original.Top250, decoded.Top250)
	assert.Equal(t, original.Watched, decoded.Watched)
	assert.True(t, original.ReleaseDate.Equal(decoded.ReleaseDate))
	assert.Equal(t, id, decoded.DBID)
}

func TestTvShow_JSONRoundtrip(t *testing.T) {
	original := TvShow{
		MediaEntity: MediaEntity{
			Title:  "Breaking Bad",
			Year:   2008,
			Genres: []string{"Drama", "Crime"},
		},
		FirstAired:       time.Date(2008, 1, 20, 0, 0, 0, 0, time.UTC),
		Status:           ShowStatusEnded,
		EpisodeGroupKind: EpisodeGroupAired,
		SeasonNames:      map[int]string{1: "Season 1", 2: "Season 2"},
		SeasonPlots:      map[int]string{1: "Walt's descent begins."},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded TvShow
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, original.Title, decoded.Title)
	assert.Equal(t, original.Year, decoded.Year)
	assert.Equal(t, original.Status, decoded.Status)
	assert.Equal(t, original.EpisodeGroupKind, decoded.EpisodeGroupKind)
	assert.Equal(t, original.SeasonNames, decoded.SeasonNames)
	assert.Equal(t, original.SeasonPlots, decoded.SeasonPlots)
	assert.True(t, original.FirstAired.Equal(decoded.FirstAired))
}

func TestTvShowEpisode_JSONRoundtrip(t *testing.T) {
	showID := uuid.New()
	displaySeason := 1
	displayEpisode := 2
	original := TvShowEpisode{
		MediaEntity: MediaEntity{
			Title: "Ozymandias",
			Plot:  "Everyone copes with radically changed circumstances.",
		},
		TvShowID:       showID,
		Season:         5,
		Episode:        14,
		DisplaySeason:  &displaySeason,
		DisplayEpisode: &displayEpisode,
		FirstAired:     time.Date(2013, 9, 15, 0, 0, 0, 0, time.UTC),
		Watched:        true,
		Playcount:      3,
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded TvShowEpisode
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, original.Title, decoded.Title)
	assert.Equal(t, original.Plot, decoded.Plot)
	assert.Equal(t, original.TvShowID, decoded.TvShowID)
	assert.Equal(t, original.Season, decoded.Season)
	assert.Equal(t, original.Episode, decoded.Episode)
	require.NotNil(t, decoded.DisplaySeason)
	require.NotNil(t, decoded.DisplayEpisode)
	assert.Equal(t, displaySeason, *decoded.DisplaySeason)
	assert.Equal(t, displayEpisode, *decoded.DisplayEpisode)
	assert.Equal(t, original.Watched, decoded.Watched)
	assert.Equal(t, original.Playcount, decoded.Playcount)
	assert.True(t, original.FirstAired.Equal(decoded.FirstAired))
}

func TestMediaSearchCandidate_JSONRoundtrip(t *testing.T) {
	orig := MediaSearchCandidate{
		ID:        "tmdb:157336",
		Title:     "Interstellar",
		Year:      2014,
		MediaType: MediaTypeMovie,
		Provider:  "tmdb",
		PosterURL: "https://example.com/poster.jpg",
		Overview:  "Space travel.",
		Score:     0.98,
	}
	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var got MediaSearchCandidate
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, orig, got)
}

func TestOptionsStructsZeroValues(t *testing.T) {
	mo := MovieSearchOptions{}
	assert.Zero(t, mo.Year)
	assert.Empty(t, mo.Query)

	to := TvShowSearchOptions{}
	assert.Zero(t, to.FirstAirYear)

	eo := TvShowEpisodeSearchOptions{TvShowID: 42, Season: 1, Episode: 3}
	assert.Equal(t, 42, eo.TvShowID)

	ao := ArtworkSearchOptions{
		EntityID:     "x",
		Type:         MediaTypeMovie,
		ArtworkTypes: []ArtworkType{ArtworkTypePoster},
		Language:     "en",
	}
	assert.Equal(t, "x", ao.EntityID)
	assert.Equal(t, MediaTypeMovie, ao.Type)
	require.Len(t, ao.ArtworkTypes, 1)
	assert.Equal(t, ArtworkTypePoster, ao.ArtworkTypes[0])
}
