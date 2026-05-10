package core

import "context"

// ProviderInfo contains metadata about a scraper provider.
type ProviderInfo struct {
	Name        string // machine name
	DisplayName string // human-readable name
	Version     string
	Priority    int // lower = higher priority
	Summary     string
	LogoURL     string
	Kind        string // "movie", "tv", "artwork", "all"
}

// MediaScraper is the root interface for all scraper providers.
type MediaScraper interface {
	// Info returns metadata about this provider.
	Info() ProviderInfo

	// IsActive returns whether this provider is currently active.
	IsActive() bool
}

// MovieMetadataScraper scrapes movie metadata from a provider.
type MovieMetadataScraper interface {
	MediaScraper

	// SearchMovie searches for movies matching the given options.
	SearchMovie(ctx context.Context, opts MovieSearchOptions) ([]MediaSearchCandidate, error)

	// GetMovieMetadata retrieves full metadata for a movie.
	GetMovieMetadata(ctx context.Context, opts MovieSearchOptions) (*Movie, error)
}

// TvShowMetadataScraper scrapes TV show and episode metadata from a provider.
type TvShowMetadataScraper interface {
	MediaScraper

	// SearchTvShow searches for TV shows matching the given options.
	SearchTvShow(ctx context.Context, opts TvShowSearchOptions) ([]MediaSearchCandidate, error)

	// GetTvShowMetadata retrieves full metadata for a TV show.
	GetTvShowMetadata(ctx context.Context, opts TvShowSearchOptions) (*TvShow, error)

	// GetEpisodeList retrieves the full episode list for a TV show.
	GetEpisodeList(ctx context.Context, opts TvShowSearchOptions) ([]TvShowEpisode, error)

	// GetEpisodeMetadata retrieves metadata for a specific episode.
	GetEpisodeMetadata(ctx context.Context, opts TvShowEpisodeSearchOptions) (*TvShowEpisode, error)
}

// ArtworkScraper scrapes artwork and images from a provider.
type ArtworkScraper interface {
	MediaScraper

	// GetArtwork retrieves artwork matching the given options.
	GetArtwork(ctx context.Context, opts ArtworkSearchOptions) ([]MediaArtwork, error)
}

// NfoWriter writes metadata to NFO (XML) files.
type NfoWriter interface {
	// WriteMovieNfo writes movie metadata to NFO files.
	WriteMovieNfo(ctx context.Context, m *Movie, paths []string) error

	// WriteTvShowNfo writes TV show metadata to NFO file.
	WriteTvShowNfo(ctx context.Context, s *TvShow, showDir string) error

	// WriteSeasonNfo writes season metadata to NFO file.
	WriteSeasonNfo(ctx context.Context, s *TvShowSeason, seasonDir string) error

	// WriteEpisodeNfo writes episode metadata to NFO file.
	WriteEpisodeNfo(ctx context.Context, e *TvShowEpisode, path string) error

	// Dialect returns the NFO dialect name.
	Dialect() string // "kodi", "jellyfin", "emby", "universal"
}

// Library represents a media library.
type Library struct {
	ID             string
	Name           string
	CollectionType string // "movies", "tvshows", etc.
	Paths          []string
}

// ServerInfo contains information about a media server.
type ServerInfo struct {
	Product  string
	Version  string
	ServerID string
	Name     string
}

// ScanStatus represents the current status of a library scan.
type ScanStatus struct {
	Running  bool
	Percent  float64
	TaskName string
}

// MediaServerConnector manages connections to media servers (Jellyfin, Emby, etc.).
type MediaServerConnector interface {
	// Name returns the connector type identifier ("jellyfin", "emby", etc.).
	Name() string

	// Ping tests connectivity to the server.
	Ping(ctx context.Context) (*ServerInfo, error)

	// Authenticate authenticates with the server.
	Authenticate(ctx context.Context) (*ServerInfo, error)

	// ListLibraries lists all libraries on the server.
	ListLibraries(ctx context.Context) ([]Library, error)

	// RefreshLibrary triggers a library refresh/scan. Empty libraryID means all libraries.
	RefreshLibrary(ctx context.Context, libraryID string) error

	// ScanStatus returns the current scan status.
	ScanStatus(ctx context.Context) (*ScanStatus, error)
}

// RawMediaInfo represents raw metadata from a source provider.
type RawMediaInfo struct {
	Provider     string
	Data         any // source-native struct
	SearchResult MediaSearchCandidate
}

// Fuser merges metadata from multiple providers.
type Fuser interface {
	// Merge merges multiple movie metadata sources into a single Movie.
	Merge(ctx context.Context, sources map[string]*RawMediaInfo) (*Movie, error)

	// MergeTv merges multiple TV show metadata sources into a single TvShow.
	MergeTv(ctx context.Context, sources map[string]*RawMediaInfo) (*TvShow, error)

	// MergeEpisode merges multiple episode metadata sources into a single TvShowEpisode.
	MergeEpisode(ctx context.Context, sources map[string]*RawMediaInfo) (*TvShowEpisode, error)
}
