package core

// MovieSearchOptions contains options for searching movies.
type MovieSearchOptions struct {
	Query            string
	Year             int
	Language         string
	FallbackLanguage string
	IMDBID           string
	TMDBID           int
	IncludeAdult     bool
}

// TvShowSearchOptions contains options for searching TV shows.
type TvShowSearchOptions struct {
	Query            string
	Year             int
	FirstAirYear     int
	Language         string
	FallbackLanguage string
	IMDBID           string
	TMDBID           int
	TVDBID           int
	IncludeAdult     bool
}

// TvShowEpisodeSearchOptions contains options for searching episodes.
type TvShowEpisodeSearchOptions struct {
	TvShowID int // TMDB ID
	Season   int
	Episode  int
	Language string
}

// ArtworkSearchOptions contains options for searching artwork.
type ArtworkSearchOptions struct {
	EntityID     string
	Type         MediaType
	ArtworkTypes []ArtworkType
	Language     string
}
