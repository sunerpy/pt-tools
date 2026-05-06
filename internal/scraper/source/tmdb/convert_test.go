package tmdb

import (
	"encoding/json"
	"testing"

	tmdbsdk "github.com/cyruzin/golang-tmdb"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

func TestTmdbToMovie_NilReturnsNil(t *testing.T) {
	t.Parallel()
	require.Nil(t, tmdbToMovie(nil))
}

func TestTmdbToTvShow_NilReturnsNil(t *testing.T) {
	t.Parallel()
	require.Nil(t, tmdbToTvShow(nil))
}

func TestTmdbToEpisode_NilReturnsNil(t *testing.T) {
	t.Parallel()
	require.Nil(t, tmdbToEpisode(nil))
}

func unmarshalTV(t *testing.T, payload string) *tmdbsdk.TVDetails {
	t.Helper()
	var out tmdbsdk.TVDetails
	require.NoError(t, json.Unmarshal([]byte(payload), &out))

	var extras struct {
		ExternalIDs *tmdbsdk.TVExternalIDs `json:"external_ids"`
		Images      *tmdbsdk.TVImages      `json:"images"`
		Videos      *tmdbsdk.VideoResults  `json:"videos"`
		Credits     *tmdbsdk.TVCredits     `json:"credits"`
	}
	require.NoError(t, json.Unmarshal([]byte(payload), &extras))
	if extras.ExternalIDs != nil {
		out.TVExternalIDsAppend = &tmdbsdk.TVExternalIDsAppend{TVExternalIDs: extras.ExternalIDs}
	}
	if extras.Images != nil {
		out.TVImagesAppend = &tmdbsdk.TVImagesAppend{Images: extras.Images}
	}
	if extras.Videos != nil {
		out.TVVideosAppend = &tmdbsdk.TVVideosAppend{Videos: extras.Videos}
	}
	if extras.Credits != nil {
		out.TVCreditsAppend = &tmdbsdk.TVCreditsAppend{Credits: struct{ *tmdbsdk.TVCredits }{TVCredits: extras.Credits}}
	}
	return &out
}

func unmarshalEpisode(t *testing.T, payload string) *tmdbsdk.TVEpisodeDetails {
	t.Helper()
	var out tmdbsdk.TVEpisodeDetails
	require.NoError(t, json.Unmarshal([]byte(payload), &out))

	var extras struct {
		ExternalIDs *tmdbsdk.TVEpisodeExternalIDs `json:"external_ids"`
		Images      *tmdbsdk.TVEpisodeImages      `json:"images"`
		Videos      *tmdbsdk.VideoResults         `json:"videos"`
	}
	require.NoError(t, json.Unmarshal([]byte(payload), &extras))
	if extras.ExternalIDs != nil {
		out.TVEpisodeExternalIDsAppend = &tmdbsdk.TVEpisodeExternalIDsAppend{ExternalIDs: extras.ExternalIDs}
	}
	if extras.Images != nil {
		out.TVEpisodeImagesAppend = &tmdbsdk.TVEpisodeImagesAppend{Images: extras.Images}
	}
	if extras.Videos != nil {
		out.TVEpisodeVideosAppend = &tmdbsdk.TVEpisodeVideosAppend{Videos: extras.Videos}
	}
	return &out
}

func TestTmdbToTvShow_Full(t *testing.T) {
	setImageBaseURL("https://image.tmdb.org/t/p/")
	defer setImageBaseURL("https://image.tmdb.org/t/p/")

	payload := `{
		"id": 1396,
		"name": "绝命毒师",
		"original_name": "Breaking Bad",
		"overview": "A chemistry teacher turns to crime.",
		"tagline": "All Hail the King",
		"first_air_date": "2008-01-20",
		"episode_run_time": [45, 50],
		"status": "Ended",
		"in_production": false,
		"languages": ["en", "es"],
		"genres": [{"id": 1, "name": "剧情"}, {"id": 2, "name": "犯罪"}],
		"production_companies": [{"id": 1, "name": "AMC"}],
		"production_countries": [{"iso_3166_1": "US", "name": "United States"}],
		"seasons": [
			{"season_number": 1, "name": "Season 1", "overview": "First season"},
			{"season_number": 2, "name": "Season 2", "overview": "Second season"}
		],
		"poster_path": "/poster.jpg",
		"backdrop_path": "/back.jpg",
		"vote_average": 9.5,
		"vote_count": 1000,
		"external_ids": {"tvdb_id": 81189, "imdb_id": "tt0903747"},
		"images": {
			"posters": [{"file_path": "/p1.jpg", "width": 100, "height": 200, "vote_count": 5, "iso_639_1": "zh"}],
			"backdrops": [{"file_path": "/b1.jpg", "width": 1920, "height": 1080, "vote_count": 3, "iso_639_1": "en"}],
			"logos": [{"file_path": "/l1.png", "width": 500, "height": 200, "vote_count": 1, "iso_639_1": "en"}]
		},
		"videos": {
			"results": [
				{"id": "1", "key": "abc", "name": "Trailer", "site": "YouTube", "size": 1080, "type": "Trailer"},
				{"id": "2", "key": "def", "name": "Teaser", "site": "Vimeo", "type": "Teaser"}
			]
		},
		"credits": {
			"cast": [
				{"id": 10, "name": "Bryan Cranston", "character": "Walter", "order": 0, "profile_path": "/bc.jpg"},
				{"id": 11, "name": "Aaron Paul", "character": "Jesse", "order": 1, "profile_path": "/ap.jpg"}
			],
			"crew": [
				{"id": 20, "name": "Vince Gilligan", "job": "Director", "department": "Directing"},
				{"id": 21, "name": "Writer X", "job": "Writer", "department": "Writing"},
				{"id": 22, "name": "Prod Y", "job": "Producer", "department": "Production"}
			]
		}
	}`

	show := tmdbToTvShow(unmarshalTV(t, payload))
	require.NotNil(t, show)
	require.Equal(t, "绝命毒师", show.Title)
	require.Equal(t, "Breaking Bad", show.OriginalTitle)
	require.Equal(t, 2008, show.Year)
	require.Equal(t, []string{"剧情", "犯罪"}, show.Genres)
	require.Equal(t, []string{"United States"}, show.Countries)
	require.Equal(t, []string{"en", "es"}, show.SpokenLanguages)
	require.Equal(t, 45, show.Runtime)
	require.Equal(t, "", show.IDs["imdb"])
	require.Equal(t, "81189", show.IDs["tvdb"])
	require.Equal(t, core.ShowStatusEnded, show.Status)
	require.Equal(t, "Season 1", show.SeasonNames[1])
	require.Equal(t, "Second season", show.SeasonPlots[2])
	require.Equal(t, "https://image.tmdb.org/t/p/original/poster.jpg", show.ArtworkURLs[core.ArtworkTypePoster])
	require.Equal(t, "https://image.tmdb.org/t/p/original/back.jpg", show.ArtworkURLs[core.ArtworkTypeBackground])
	require.Len(t, show.Trailers, 1)
	require.Len(t, show.Actors, 2)
	require.Equal(t, "Bryan Cranston", show.Actors[0].Name)
	require.Len(t, show.Directors, 1)
	require.Len(t, show.Writers, 1)
	require.Len(t, show.Producers, 1)
}

func TestTmdbToEpisode_WithCreditsAndArtworks(t *testing.T) {
	setImageBaseURL("https://image.tmdb.org/t/p/")

	payload := `{
		"id": 62085,
		"name": "Pilot",
		"overview": "Beginning",
		"air_date": "2008-01-20",
		"season_number": 1,
		"episode_number": 1,
		"runtime": 58,
		"still_path": "/still.jpg",
		"vote_average": 8.5,
		"vote_count": 100,
		"guest_stars": [
			{"id": 10, "name": "Guest A", "character": "Role A", "order": 2, "profile_path": "/ga.jpg"},
			{"id": 11, "name": "Guest B", "character": "Role B", "order": 0, "profile_path": "/gb.jpg"}
		],
		"crew": [
			{"id": 20, "name": "Ep Director", "job": "Director", "department": "Directing"}
		],
		"external_ids": {"imdb_id": "tt0959621", "tvdb_id": 349232},
		"images": {"stills": [{"file_path": "/s1.jpg", "width": 1280, "height": 720, "vote_count": 1, "iso_639_1": "en"}]},
		"videos": {"results": [{"id": "1", "key": "xyz", "name": "Trailer", "site": "YouTube", "size": 720, "type": "Trailer"}]}
	}`

	ep := tmdbToEpisode(unmarshalEpisode(t, payload))
	require.NotNil(t, ep)
	require.Equal(t, "Pilot", ep.Title)
	require.Equal(t, 1, ep.Season)
	require.Equal(t, 1, ep.Episode)
	require.Equal(t, "tt0959621", ep.IDs["imdb"])
	require.Equal(t, "349232", ep.IDs["tvdb"])
	require.Len(t, ep.Actors, 2)
	require.Equal(t, "Guest B", ep.Actors[0].Name)
	require.Equal(t, "Guest A", ep.Actors[1].Name)
	require.Len(t, ep.Directors, 1)
	require.Equal(t, "https://image.tmdb.org/t/p/original/still.jpg", ep.ArtworkURLs[core.ArtworkTypeThumb])
	require.Len(t, ep.Trailers, 1)
}

func TestMapCrewType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		job, dept string
		want      core.PersonType
	}{
		{"Director", "Directing", core.PersonTypeDirector},
		{"Writer", "Writing", core.PersonTypeWriter},
		{"Screenplay", "Writing", core.PersonTypeWriter},
		{"Author", "", core.PersonTypeWriter},
		{"Producer", "Production", core.PersonTypeProducer},
		{"Composer", "Sound", core.PersonTypeComposer},
		{"Editor", "Editing", core.PersonTypeEditor},
		{"Cameraman", "Camera", core.PersonTypeCamera},
		{"", "Camera", core.PersonTypeCamera},
		{"", "Writing", core.PersonTypeWriter},
		{"", "Production", core.PersonTypeProducer},
		{"", "Directing", core.PersonTypeDirector},
		{"Unknown", "Unknown", core.PersonTypeOther},
	}
	for _, c := range cases {
		require.Equal(t, c.want, mapCrewType(c.job, c.dept), "job=%q dept=%q", c.job, c.dept)
	}
}

func TestMapShowStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status       string
		inProduction bool
		want         core.ShowStatus
	}{
		{"Returning Series", false, core.ShowStatusReturning},
		{"Ended", false, core.ShowStatusEnded},
		{"Canceled", false, core.ShowStatusCanceled},
		{"Pilot", false, core.ShowStatusPilot},
		{"In Production", false, core.ShowStatusInProduction},
		{"Other", true, core.ShowStatusInProduction},
		{"Unknown", false, core.ShowStatusUnknown},
	}
	for _, c := range cases {
		require.Equal(t, c.want, mapShowStatus(c.status, c.inProduction), "status=%q ip=%v", c.status, c.inProduction)
	}
}

func TestFullImageURL(t *testing.T) {
	t.Parallel()
	setImageBaseURL("https://image.tmdb.org/t/p/")
	defer setImageBaseURL("https://image.tmdb.org/t/p/")

	require.Equal(t, "", fullImageURL(""))
	require.Equal(t, "https://image.tmdb.org/t/p/original/path.jpg", fullImageURL("/path.jpg"))
	require.Equal(t, "http://example.com/img.jpg", fullImageURL("http://example.com/img.jpg"))
	require.Equal(t, "https://example.com/img.jpg", fullImageURL("https://example.com/img.jpg"))
}

func TestSetImageBaseURL(t *testing.T) {
	setImageBaseURL("")
	require.Equal(t, defaultImageBaseURL, imageBaseURL)
	setImageBaseURL("https://custom.example.com/img/")
	require.Equal(t, "https://custom.example.com/img/original", imageBaseURL)
	setImageBaseURL("https://image.tmdb.org/t/p/")
}

func TestParseDate(t *testing.T) {
	t.Parallel()
	require.True(t, parseDate("").IsZero())
	require.True(t, parseDate("bad").IsZero())
	d := parseDate("2010-07-15")
	require.Equal(t, 2010, d.Year())
	require.Equal(t, 7, int(d.Month()))
}

func TestFirstPositive(t *testing.T) {
	t.Parallel()
	require.Equal(t, 0, firstPositive(nil))
	require.Equal(t, 0, firstPositive([]int{0, -1}))
	require.Equal(t, 50, firstPositive([]int{0, 50, 60}))
}

func TestTvLanguageNames(t *testing.T) {
	t.Parallel()
	require.Empty(t, tvLanguageNames(nil))
	require.Equal(t, []string{"en", "zh"}, tvLanguageNames([]string{"en", "", "zh"}))
}

func TestTvdbID(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", tvdbID(nil))
	require.Equal(t, "", tvdbID(&tmdbsdk.TVExternalIDsAppend{}))
	require.Equal(t, "", tvdbID(&tmdbsdk.TVExternalIDsAppend{TVExternalIDs: &tmdbsdk.TVExternalIDs{TVDBID: 0}}))
	require.Equal(t, "123", tvdbID(&tmdbsdk.TVExternalIDsAppend{TVExternalIDs: &tmdbsdk.TVExternalIDs{TVDBID: 123}}))
}

func TestEpisodeExternalIDs(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", episodeIMDbID(nil))
	require.Equal(t, "", episodeIMDbID(&tmdbsdk.TVEpisodeExternalIDsAppend{}))
	require.Equal(t, "tt1", episodeIMDbID(&tmdbsdk.TVEpisodeExternalIDsAppend{ExternalIDs: &tmdbsdk.TVEpisodeExternalIDs{IMDbID: "tt1"}}))

	require.Equal(t, "", episodeTVDBID(nil))
	require.Equal(t, "", episodeTVDBID(&tmdbsdk.TVEpisodeExternalIDsAppend{ExternalIDs: &tmdbsdk.TVEpisodeExternalIDs{TVDBID: 0}}))
	require.Equal(t, "99", episodeTVDBID(&tmdbsdk.TVEpisodeExternalIDsAppend{ExternalIDs: &tmdbsdk.TVEpisodeExternalIDs{TVDBID: 99}}))
}

func TestLanguageString(t *testing.T) {
	t.Parallel()
	require.Equal(t, "en", languageString("en"))
	require.Equal(t, "", languageString(nil))
	require.Equal(t, "", languageString(123))
}

func TestFilterArtworkTypes(t *testing.T) {
	t.Parallel()
	input := []core.MediaArtwork{
		{Type: core.ArtworkTypePoster, URL: "a"},
		{Type: core.ArtworkTypeBackground, URL: "b"},
		{Type: core.ArtworkTypeThumb, URL: "c"},
	}
	require.Len(t, filterArtworkTypes(input, nil), 3)
	got := filterArtworkTypes(input, []core.ArtworkType{core.ArtworkTypePoster, core.ArtworkTypeThumb})
	require.Len(t, got, 2)
}

func TestCompactStrings(t *testing.T) {
	t.Parallel()
	require.Equal(t, []string{"en", "zh"}, compactStrings("en", "", "zh", "en"))
	require.Empty(t, compactStrings())
	require.Empty(t, compactStrings("", "  "))
}

func TestApplyArtworks(t *testing.T) {
	t.Parallel()
	target := map[core.ArtworkType]string{}
	applyArtworks(target, []core.MediaArtwork{
		{Type: core.ArtworkTypePoster, URL: ""},
		{Type: core.ArtworkTypePoster, URL: "url1"},
		{Type: core.ArtworkTypePoster, URL: "url2"},
	})
	require.Equal(t, "url1", target[core.ArtworkTypePoster])
}

func TestBuildTVImageArtworks_Empty(t *testing.T) {
	t.Parallel()
	require.Empty(t, buildTVImageArtworks(nil, core.ArtworkTypePoster))
}

func TestBuildTVTrailers_Nil(t *testing.T) {
	t.Parallel()
	require.Nil(t, buildTVTrailers(nil))
	require.Nil(t, buildTVTrailers(&tmdbsdk.TVVideosAppend{}))
}

func TestBuildEpisodeTrailers_Nil(t *testing.T) {
	t.Parallel()
	require.Nil(t, buildEpisodeTrailers(nil))
	require.Nil(t, buildEpisodeTrailers(&tmdbsdk.TVEpisodeVideosAppend{}))
}

func TestBuildTrailers_Nil(t *testing.T) {
	t.Parallel()
	require.Nil(t, buildTrailers(nil))
	require.Nil(t, buildTrailers(&tmdbsdk.MovieVideosAppend{}))
}

func TestOverlayMovieText(t *testing.T) {
	t.Parallel()
	overlayMovieText(nil, nil)
	dst := &tmdbsdk.MovieDetails{Title: "", Overview: "", Tagline: ""}
	src := &tmdbsdk.MovieDetails{Title: "T", Overview: "O", Tagline: "TL"}
	overlayMovieText(dst, src)
	require.Equal(t, "T", dst.Title)
	require.Equal(t, "O", dst.Overview)
	require.Equal(t, "TL", dst.Tagline)
}

func TestOverlayTVText(t *testing.T) {
	t.Parallel()
	overlayTVText(nil, nil)
	dst := &tmdbsdk.TVDetails{Name: "", Overview: "", Tagline: "", Seasons: []tmdbsdk.Season{{SeasonNumber: 1, Name: "", Overview: ""}}}
	src := &tmdbsdk.TVDetails{Name: "N", Overview: "O", Tagline: "TL", Seasons: []tmdbsdk.Season{{SeasonNumber: 1, Name: "S1", Overview: "OV1"}}}
	overlayTVText(dst, src)
	require.Equal(t, "N", dst.Name)
	require.Equal(t, "O", dst.Overview)
	require.Equal(t, "TL", dst.Tagline)
	require.Equal(t, "S1", dst.Seasons[0].Name)
	require.Equal(t, "OV1", dst.Seasons[0].Overview)
}

func TestOverlayEpisodeText(t *testing.T) {
	t.Parallel()
	overlayEpisodeText(nil, nil)
	dst := &tmdbsdk.TVEpisodeDetails{Name: "", Overview: ""}
	src := &tmdbsdk.TVEpisodeDetails{Name: "N", Overview: "O"}
	overlayEpisodeText(dst, src)
	require.Equal(t, "N", dst.Name)
	require.Equal(t, "O", dst.Overview)
}

func TestShouldRetryWithFallback(t *testing.T) {
	t.Parallel()
	require.True(t, shouldRetryWithFallback("", "o"))
	require.True(t, shouldRetryWithFallback("t", ""))
	require.False(t, shouldRetryWithFallback("t", "o"))
}

func TestNeedsSearchFallback(t *testing.T) {
	t.Parallel()
	require.False(t, needsSearchFallback([]core.MediaSearchCandidate{{Title: "a", Overview: "b"}}))
	require.True(t, needsSearchFallback([]core.MediaSearchCandidate{{Title: "", Overview: "b"}}))
}

func TestMergeCandidates(t *testing.T) {
	t.Parallel()
	primary := []core.MediaSearchCandidate{{ID: "1", Title: "", Overview: "", PosterURL: ""}}
	fallback := []core.MediaSearchCandidate{{ID: "1", Title: "T", Overview: "O", PosterURL: "P"}}
	merged := mergeCandidates(primary, fallback)
	require.Equal(t, "T", merged[0].Title)
	require.Equal(t, "O", merged[0].Overview)
	require.Equal(t, "P", merged[0].PosterURL)
}

func TestParseEpisodeEntityID(t *testing.T) {
	t.Parallel()
	s, se, ep, err := parseEpisodeEntityID("1396:1:2")
	require.NoError(t, err)
	require.Equal(t, 1396, s)
	require.Equal(t, 1, se)
	require.Equal(t, 2, ep)

	_, _, _, err = parseEpisodeEntityID("bad")
	require.Error(t, err)

	_, _, _, err = parseEpisodeEntityID("x:1:2")
	require.Error(t, err)

	_, _, _, err = parseEpisodeEntityID("1:x:2")
	require.Error(t, err)

	_, _, _, err = parseEpisodeEntityID("1:1:x")
	require.Error(t, err)
}

func TestMovieIMDbID(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", movieIMDbID(nil))
	require.Equal(t, "tt1", movieIMDbID(&tmdbsdk.MovieDetails{IMDbID: "tt1"}))
	require.Equal(t, "tt2", movieIMDbID(&tmdbsdk.MovieDetails{
		MovieExternalIDsAppend: &tmdbsdk.MovieExternalIDsAppend{MovieExternalIDs: &tmdbsdk.MovieExternalIDs{IMDbID: "tt2"}},
	}))
	require.Equal(t, "", movieIMDbID(&tmdbsdk.MovieDetails{}))
}
