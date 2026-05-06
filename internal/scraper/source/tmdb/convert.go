package tmdb

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	tmdbsdk "github.com/cyruzin/golang-tmdb"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

const defaultImageBaseURL = "https://image.tmdb.org/t/p/original"

var imageBaseURL = defaultImageBaseURL

func setImageBaseURL(base string) {
	base = strings.TrimSpace(base)
	if base == "" {
		imageBaseURL = defaultImageBaseURL
		return
	}
	imageBaseURL = strings.TrimRight(base, "/") + "/original"
}

func tmdbToMovie(raw *tmdbsdk.MovieDetails) *core.Movie {
	if raw == nil {
		return nil
	}

	result := &core.Movie{
		MediaEntity: core.MediaEntity{
			Title:           raw.Title,
			OriginalTitle:   raw.OriginalTitle,
			Year:            parseYear(raw.ReleaseDate),
			Plot:            raw.Overview,
			Outline:         raw.Overview,
			Tagline:         raw.Tagline,
			IDs:             buildIDs(strconv.FormatInt(raw.ID, 10), movieIMDbID(raw), ""),
			Ratings:         buildRatings(float64(raw.VoteAverage), int(raw.VoteCount)),
			Genres:          genreNames(raw.Genres),
			Studios:         companyNames(raw.ProductionCompanies),
			Countries:       countryNames(raw.ProductionCountries),
			SpokenLanguages: languageNames(raw.SpokenLanguages),
			ArtworkURLs:     make(map[core.ArtworkType]string),
			Trailers:        buildTrailers(raw.MovieVideosAppend),
			Runtime:         raw.Runtime,
			Provider:        "tmdb",
			ScrapedAt:       time.Now().UTC(),
		},
		ReleaseDate: parseDate(raw.ReleaseDate),
	}

	if raw.BelongsToCollection.ID > 0 {
		result.TmdbCollection = int(raw.BelongsToCollection.ID)
		result.MovieSetName = raw.BelongsToCollection.Name
	}

	actors, directors, writers, producers := moviePeople(raw)
	result.Actors = actors
	result.Directors = directors
	result.Writers = writers
	result.Producers = producers

	artworks := movieArtworks(raw)
	applyArtworks(result.ArtworkURLs, artworks)

	return result
}

func tmdbToTvShow(raw *tmdbsdk.TVDetails) *core.TvShow {
	if raw == nil {
		return nil
	}

	result := &core.TvShow{
		MediaEntity: core.MediaEntity{
			Title:           raw.Name,
			OriginalTitle:   raw.OriginalName,
			Year:            parseYear(raw.FirstAirDate),
			Plot:            raw.Overview,
			Outline:         raw.Overview,
			Tagline:         raw.Tagline,
			IDs:             buildIDs(strconv.FormatInt(raw.ID, 10), "", tvdbID(raw.TVExternalIDsAppend)),
			Ratings:         buildRatings(float64(raw.VoteAverage), int(raw.VoteCount)),
			Genres:          genreNames(raw.Genres),
			Studios:         companyNames(raw.ProductionCompanies),
			Countries:       countryNames(raw.ProductionCountries),
			SpokenLanguages: tvLanguageNames(raw.Languages),
			ArtworkURLs:     make(map[core.ArtworkType]string),
			Trailers:        buildTVTrailers(raw.TVVideosAppend),
			Runtime:         firstPositive(raw.EpisodeRunTime),
			Provider:        "tmdb",
			ScrapedAt:       time.Now().UTC(),
		},
		FirstAired:  parseDate(raw.FirstAirDate),
		Status:      mapShowStatus(raw.Status, raw.InProduction),
		SeasonNames: make(map[int]string),
		SeasonPlots: make(map[int]string),
	}

	actors, directors, writers, producers := tvPeople(raw)
	result.Actors = actors
	result.Directors = directors
	result.Writers = writers
	result.Producers = producers

	artworks := tvArtworks(raw)
	applyArtworks(result.ArtworkURLs, artworks)

	for _, season := range raw.Seasons {
		result.SeasonNames[season.SeasonNumber] = season.Name
		result.SeasonPlots[season.SeasonNumber] = season.Overview
	}

	return result
}

func tmdbToEpisode(raw *tmdbsdk.TVEpisodeDetails) *core.TvShowEpisode {
	if raw == nil {
		return nil
	}

	result := &core.TvShowEpisode{
		MediaEntity: core.MediaEntity{
			Title:       raw.Name,
			Plot:        raw.Overview,
			Outline:     raw.Overview,
			IDs:         buildIDs(strconv.FormatInt(raw.ID, 10), episodeIMDbID(raw.TVEpisodeExternalIDsAppend), episodeTVDBID(raw.TVEpisodeExternalIDsAppend)),
			Ratings:     buildRatings(float64(raw.VoteAverage), int(raw.VoteCount)),
			ArtworkURLs: make(map[core.ArtworkType]string),
			Trailers:    buildEpisodeTrailers(raw.TVEpisodeVideosAppend),
			Runtime:     raw.Runtime,
			Provider:    "tmdb",
			ScrapedAt:   time.Now().UTC(),
		},
		Season:     raw.SeasonNumber,
		Episode:    raw.EpisodeNumber,
		FirstAired: parseDate(raw.AirDate),
	}

	actors, directors, writers, producers := episodePeople(raw)
	result.Actors = actors
	result.Directors = directors
	result.Writers = writers
	result.Producers = producers

	artworks := episodeArtworks(raw)
	applyArtworks(result.ArtworkURLs, artworks)

	return result
}

func moviePeople(raw *tmdbsdk.MovieDetails) ([]core.Person, []core.Person, []core.Person, []core.Person) {
	if raw.MovieCreditsAppend == nil || raw.Credits.MovieCredits == nil {
		return nil, nil, nil, nil
	}

	credits := raw.Credits.MovieCredits
	actors := make([]core.Person, 0, len(credits.Cast))
	for _, cast := range credits.Cast {
		actors = append(actors, core.Person{
			Type:       core.PersonTypeActor,
			Name:       cast.Name,
			Role:       cast.Character,
			Order:      cast.Order,
			ThumbURL:   fullImageURL(cast.ProfilePath),
			ProfileURL: fullImageURL(cast.ProfilePath),
			IDs:        map[string]string{"tmdb": strconv.FormatInt(cast.ID, 10)},
		})
	}
	slices.SortFunc(actors, func(a, b core.Person) int { return cmp.Compare(a.Order, b.Order) })

	directors, writers, producers := splitMovieCrew(credits.Crew)
	return actors, directors, writers, producers
}

func tvPeople(raw *tmdbsdk.TVDetails) ([]core.Person, []core.Person, []core.Person, []core.Person) {
	if raw.TVCreditsAppend == nil || raw.Credits.TVCredits == nil {
		return nil, nil, nil, nil
	}

	credits := raw.Credits.TVCredits
	actors := make([]core.Person, 0, len(credits.Cast))
	for _, cast := range credits.Cast {
		actors = append(actors, core.Person{
			Type:       core.PersonTypeActor,
			Name:       cast.Name,
			Role:       cast.Character,
			Order:      cast.Order,
			ThumbURL:   fullImageURL(cast.ProfilePath),
			ProfileURL: fullImageURL(cast.ProfilePath),
			IDs:        map[string]string{"tmdb": strconv.FormatInt(cast.ID, 10)},
		})
	}
	slices.SortFunc(actors, func(a, b core.Person) int { return cmp.Compare(a.Order, b.Order) })

	directors, writers, producers := splitTVCrew(credits.Crew)
	return actors, directors, writers, producers
}

func episodePeople(raw *tmdbsdk.TVEpisodeDetails) ([]core.Person, []core.Person, []core.Person, []core.Person) {
	actors := make([]core.Person, 0, len(raw.GuestStars))
	for _, guest := range raw.GuestStars {
		actors = append(actors, core.Person{
			Type:       core.PersonTypeGuest,
			Name:       guest.Name,
			Role:       guest.Character,
			Order:      guest.Order,
			ThumbURL:   fullImageURL(guest.ProfilePath),
			ProfileURL: fullImageURL(guest.ProfilePath),
			IDs:        map[string]string{"tmdb": strconv.FormatInt(guest.ID, 10)},
		})
	}
	slices.SortFunc(actors, func(a, b core.Person) int { return cmp.Compare(a.Order, b.Order) })

	directors, writers, producers := splitEpisodeCrew(raw.Crew)
	return actors, directors, writers, producers
}

func movieArtworks(raw *tmdbsdk.MovieDetails) []core.MediaArtwork {
	artworks := make([]core.MediaArtwork, 0)
	if raw.PosterPath != "" {
		artworks = append(artworks, core.MediaArtwork{Type: core.ArtworkTypePoster, URL: fullImageURL(raw.PosterPath), PreviewURL: fullImageURL(raw.PosterPath), Provider: "tmdb"})
	}
	if raw.BackdropPath != "" {
		artworks = append(artworks, core.MediaArtwork{Type: core.ArtworkTypeBackground, URL: fullImageURL(raw.BackdropPath), PreviewURL: fullImageURL(raw.BackdropPath), Provider: "tmdb"})
	}
	if raw.MovieImagesAppend == nil || raw.Images == nil {
		return artworks
	}
	artworks = append(artworks, buildMovieImageArtworks(raw.Images.Posters, core.ArtworkTypePoster)...)
	artworks = append(artworks, buildMovieImageArtworks(raw.Images.Backdrops, core.ArtworkTypeBackground)...)
	artworks = append(artworks, buildMovieImageArtworks(raw.Images.Logos, core.ArtworkTypeClearlogo)...)
	return artworks
}

func tvArtworks(raw *tmdbsdk.TVDetails) []core.MediaArtwork {
	artworks := make([]core.MediaArtwork, 0)
	if raw.PosterPath != "" {
		artworks = append(artworks, core.MediaArtwork{Type: core.ArtworkTypePoster, URL: fullImageURL(raw.PosterPath), PreviewURL: fullImageURL(raw.PosterPath), Provider: "tmdb"})
	}
	if raw.BackdropPath != "" {
		artworks = append(artworks, core.MediaArtwork{Type: core.ArtworkTypeBackground, URL: fullImageURL(raw.BackdropPath), PreviewURL: fullImageURL(raw.BackdropPath), Provider: "tmdb"})
	}
	if raw.TVImagesAppend == nil || raw.Images == nil {
		return artworks
	}
	artworks = append(artworks, buildTVImageArtworks(raw.Images.Posters, core.ArtworkTypePoster)...)
	artworks = append(artworks, buildTVImageArtworks(raw.Images.Backdrops, core.ArtworkTypeBackground)...)
	artworks = append(artworks, buildTVImageArtworks(raw.Images.Logos, core.ArtworkTypeClearlogo)...)
	return artworks
}

func episodeArtworks(raw *tmdbsdk.TVEpisodeDetails) []core.MediaArtwork {
	artworks := make([]core.MediaArtwork, 0)
	if raw.StillPath != "" {
		artworks = append(artworks, core.MediaArtwork{Type: core.ArtworkTypeThumb, URL: fullImageURL(raw.StillPath), PreviewURL: fullImageURL(raw.StillPath), Provider: "tmdb"})
	}
	if raw.TVEpisodeImagesAppend == nil || raw.Images == nil {
		return artworks
	}
	for idx, still := range raw.Images.Stills {
		artworks = append(artworks, core.MediaArtwork{
			Type:       core.ArtworkTypeThumb,
			URL:        fullImageURL(still.FilePath),
			PreviewURL: fullImageURL(still.FilePath),
			Language:   languageString(still.Iso6391),
			Width:      still.Width,
			Height:     still.Height,
			SizeOrder:  idx,
			Likes:      int(still.VoteCount),
			Provider:   "tmdb",
		})
	}
	return artworks
}

func buildMovieImageArtworks(images []tmdbsdk.MovieImage, artType core.ArtworkType) []core.MediaArtwork {
	artworks := make([]core.MediaArtwork, 0, len(images))
	for idx, img := range images {
		artworks = append(artworks, core.MediaArtwork{
			Type:       artType,
			URL:        fullImageURL(img.FilePath),
			PreviewURL: fullImageURL(img.FilePath),
			Language:   img.Iso639_1,
			Width:      img.Width,
			Height:     img.Height,
			SizeOrder:  idx,
			Likes:      int(img.VoteCount),
			Provider:   "tmdb",
		})
	}
	return artworks
}

func buildTVImageArtworks(images []tmdbsdk.TVImage, artType core.ArtworkType) []core.MediaArtwork {
	artworks := make([]core.MediaArtwork, 0, len(images))
	for idx, img := range images {
		artworks = append(artworks, core.MediaArtwork{
			Type:       artType,
			URL:        fullImageURL(img.FilePath),
			PreviewURL: fullImageURL(img.FilePath),
			Language:   img.Iso639_1,
			Width:      img.Width,
			Height:     img.Height,
			SizeOrder:  idx,
			Likes:      int(img.VoteCount),
			Provider:   "tmdb",
		})
	}
	return artworks
}

func buildTrailers(videos *tmdbsdk.MovieVideosAppend) []core.MediaTrailer {
	if videos == nil || videos.Videos == nil {
		return nil
	}
	return trailersFromVideos(videos.Videos.Results)
}

func buildTVTrailers(videos *tmdbsdk.TVVideosAppend) []core.MediaTrailer {
	if videos == nil || videos.Videos == nil {
		return nil
	}
	return trailersFromVideos(videos.Videos.Results)
}

func buildEpisodeTrailers(videos *tmdbsdk.TVEpisodeVideosAppend) []core.MediaTrailer {
	if videos == nil || videos.Videos == nil {
		return nil
	}
	return trailersFromVideos(videos.Videos.Results)
}

func trailersFromVideos(results []tmdbsdk.VideoResult) []core.MediaTrailer {
	trailers := make([]core.MediaTrailer, 0, len(results))
	for _, video := range results {
		if !strings.EqualFold(video.Site, "YouTube") || video.Key == "" {
			continue
		}
		trailers = append(trailers, core.MediaTrailer{
			Name:     video.Name,
			URL:      "https://www.youtube.com/watch?v=" + video.Key,
			Provider: "tmdb",
			Quality:  strconv.Itoa(video.Size) + "p",
			InNfo:    strings.EqualFold(video.Type, "Trailer"),
		})
	}
	return trailers
}

func splitMovieCrew(crew []struct {
	Adult              bool    `json:"adult"`
	CreditID           string  `json:"credit_id"`
	Department         string  `json:"department"`
	Gender             int     `json:"gender"`
	ID                 int64   `json:"id"`
	Job                string  `json:"job"`
	KnownForDepartment string  `json:"known_for_department"`
	Name               string  `json:"name"`
	OriginalName       string  `json:"original_name"`
	Popularity         float32 `json:"popularity"`
	ProfilePath        string  `json:"profile_path"`
},
) ([]core.Person, []core.Person, []core.Person) {
	directors := make([]core.Person, 0)
	writers := make([]core.Person, 0)
	producers := make([]core.Person, 0)
	for idx, item := range crew {
		person := crewPerson(item.Name, item.Job, item.Department, item.ProfilePath, item.ID, idx)
		switch person.Type {
		case core.PersonTypeDirector:
			directors = append(directors, person)
		case core.PersonTypeWriter:
			writers = append(writers, person)
		case core.PersonTypeProducer:
			producers = append(producers, person)
		}
	}
	return directors, writers, producers
}

func splitTVCrew(crew []struct {
	CreditID           string  `json:"credit_id"`
	Department         string  `json:"department"`
	Gender             int     `json:"gender"`
	ID                 int64   `json:"id"`
	Job                string  `json:"job"`
	KnownForDepartment string  `json:"known_for_department"`
	Name               string  `json:"name"`
	OriginalName       string  `json:"original_name"`
	Popularity         float32 `json:"popularity"`
	ProfilePath        string  `json:"profile_path"`
},
) ([]core.Person, []core.Person, []core.Person) {
	directors := make([]core.Person, 0)
	writers := make([]core.Person, 0)
	producers := make([]core.Person, 0)
	for idx, item := range crew {
		person := crewPerson(item.Name, item.Job, item.Department, item.ProfilePath, item.ID, idx)
		switch person.Type {
		case core.PersonTypeDirector:
			directors = append(directors, person)
		case core.PersonTypeWriter:
			writers = append(writers, person)
		case core.PersonTypeProducer:
			producers = append(producers, person)
		}
	}
	return directors, writers, producers
}

func splitEpisodeCrew(crew []struct {
	ID          int64  `json:"id"`
	CreditID    string `json:"credit_id"`
	Name        string `json:"name"`
	Department  string `json:"department"`
	Job         string `json:"job"`
	Gender      int    `json:"gender"`
	ProfilePath string `json:"profile_path"`
},
) ([]core.Person, []core.Person, []core.Person) {
	directors := make([]core.Person, 0)
	writers := make([]core.Person, 0)
	producers := make([]core.Person, 0)
	for idx, item := range crew {
		person := crewPerson(item.Name, item.Job, item.Department, item.ProfilePath, item.ID, idx)
		switch person.Type {
		case core.PersonTypeDirector:
			directors = append(directors, person)
		case core.PersonTypeWriter:
			writers = append(writers, person)
		case core.PersonTypeProducer:
			producers = append(producers, person)
		}
	}
	return directors, writers, producers
}

func crewPerson(name, job, department, profile string, id int64, order int) core.Person {
	personType := mapCrewType(job, department)
	return core.Person{
		Type:       personType,
		Name:       name,
		Role:       job,
		Order:      order,
		ThumbURL:   fullImageURL(profile),
		ProfileURL: fullImageURL(profile),
		IDs:        map[string]string{"tmdb": strconv.FormatInt(id, 10)},
	}
}

func mapCrewType(job, department string) core.PersonType {
	job = strings.ToLower(job)
	department = strings.ToLower(department)
	switch {
	case strings.Contains(job, "director"):
		return core.PersonTypeDirector
	case strings.Contains(job, "writer"), strings.Contains(job, "screenplay"), strings.Contains(job, "author"):
		return core.PersonTypeWriter
	case strings.Contains(job, "producer"):
		return core.PersonTypeProducer
	case strings.Contains(job, "composer"):
		return core.PersonTypeComposer
	case strings.Contains(job, "editor"):
		return core.PersonTypeEditor
	case strings.Contains(job, "camera"), strings.Contains(department, "camera"):
		return core.PersonTypeCamera
	case strings.Contains(department, "writing"):
		return core.PersonTypeWriter
	case strings.Contains(department, "production"):
		return core.PersonTypeProducer
	case strings.Contains(department, "directing"):
		return core.PersonTypeDirector
	default:
		return core.PersonTypeOther
	}
}

func applyArtworks(target map[core.ArtworkType]string, artworks []core.MediaArtwork) {
	for _, art := range artworks {
		if art.URL == "" {
			continue
		}
		if _, ok := target[art.Type]; !ok {
			target[art.Type] = art.URL
		}
	}
}

func fullImageURL(path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return strings.TrimRight(imageBaseURL, "/") + "/" + strings.TrimLeft(path, "/")
}

func buildIDs(tmdbID, imdbID, tvdbID string) map[string]string {
	ids := map[string]string{}
	if tmdbID != "" {
		ids["tmdb"] = tmdbID
	}
	if imdbID != "" {
		ids["imdb"] = imdbID
	}
	if tvdbID != "" {
		ids["tvdb"] = tvdbID
	}
	return ids
}

func movieIMDbID(raw *tmdbsdk.MovieDetails) string {
	if raw == nil {
		return ""
	}
	if raw.IMDbID != "" {
		return raw.IMDbID
	}
	if raw.MovieExternalIDsAppend != nil && raw.MovieExternalIDs != nil {
		return raw.MovieExternalIDs.IMDbID
	}
	return ""
}

func buildRatings(value float64, votes int) map[string]core.MediaRating {
	if value <= 0 {
		return nil
	}
	return map[string]core.MediaRating{
		"tmdb": {ID: "tmdb", Value: value, Votes: votes, Max: 10},
	}
}

func genreNames(genres []tmdbsdk.Genre) []string {
	result := make([]string, 0, len(genres))
	for _, genre := range genres {
		if genre.Name != "" {
			result = append(result, genre.Name)
		}
	}
	return result
}

func companyNames(companies []tmdbsdk.ProductionCompany) []string {
	result := make([]string, 0, len(companies))
	for _, company := range companies {
		if company.Name != "" {
			result = append(result, company.Name)
		}
	}
	return result
}

func countryNames(countries []tmdbsdk.ProductionCountry) []string {
	result := make([]string, 0, len(countries))
	for _, country := range countries {
		if country.Name != "" {
			result = append(result, country.Name)
		}
	}
	return result
}

func languageNames(languages []tmdbsdk.SpokenLanguage) []string {
	result := make([]string, 0, len(languages))
	for _, language := range languages {
		if language.Name != "" {
			result = append(result, language.Name)
		}
	}
	return result
}

func tvLanguageNames(languages []string) []string {
	result := make([]string, 0, len(languages))
	for _, language := range languages {
		if language != "" {
			result = append(result, language)
		}
	}
	return result
}

func mapShowStatus(status string, inProduction bool) core.ShowStatus {
	switch strings.ToLower(status) {
	case "returning series":
		return core.ShowStatusReturning
	case "ended":
		return core.ShowStatusEnded
	case "canceled":
		return core.ShowStatusCanceled
	case "pilot":
		return core.ShowStatusPilot
	case "in production":
		return core.ShowStatusInProduction
	default:
		if inProduction {
			return core.ShowStatusInProduction
		}
		return core.ShowStatusUnknown
	}
}

func parseDate(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}
	}
	return t
}

func parseYear(value string) int {
	return parseDate(value).Year()
}

func tvdbID(raw *tmdbsdk.TVExternalIDsAppend) string {
	if raw == nil || raw.TVExternalIDs == nil || raw.TVDBID <= 0 {
		return ""
	}
	return strconv.FormatInt(raw.TVDBID, 10)
}

func episodeIMDbID(raw *tmdbsdk.TVEpisodeExternalIDsAppend) string {
	if raw == nil || raw.ExternalIDs == nil {
		return ""
	}
	return raw.ExternalIDs.IMDbID
}

func episodeTVDBID(raw *tmdbsdk.TVEpisodeExternalIDsAppend) string {
	if raw == nil || raw.ExternalIDs == nil || raw.ExternalIDs.TVDBID <= 0 {
		return ""
	}
	return strconv.FormatInt(raw.ExternalIDs.TVDBID, 10)
}

func firstPositive(values []int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func languageString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}
