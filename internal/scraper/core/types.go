package core

import (
	"time"

	"github.com/google/uuid"
)

// MediaType represents the type of media.
type MediaType int

const (
	MediaTypeUnknown MediaType = iota
	MediaTypeMovie
	MediaTypeTvShow
	MediaTypeSeason
	MediaTypeEpisode
)

// String returns the string representation of MediaType.
func (m MediaType) String() string {
	switch m {
	case MediaTypeMovie:
		return "movie"
	case MediaTypeTvShow:
		return "tv_show"
	case MediaTypeSeason:
		return "season"
	case MediaTypeEpisode:
		return "episode"
	default:
		return "unknown"
	}
}

// ArtworkType represents the type of artwork/image.
type ArtworkType int

const (
	ArtworkTypeUnknown ArtworkType = iota
	ArtworkTypePoster
	ArtworkTypeBackground // fanart
	ArtworkTypeBanner
	ArtworkTypeClearlogo
	ArtworkTypeClearart
	ArtworkTypeDisc
	ArtworkTypeKeyart
	ArtworkTypeThumb // landscape
	ArtworkTypeCharacterart
	ArtworkTypeSeasonPoster
	ArtworkTypeSeasonFanart
	ArtworkTypeSeasonBanner
	ArtworkTypeSeasonThumb
	ArtworkTypeActor
)

// String returns the string representation of ArtworkType.
func (a ArtworkType) String() string {
	switch a {
	case ArtworkTypePoster:
		return "poster"
	case ArtworkTypeBackground:
		return "fanart"
	case ArtworkTypeBanner:
		return "banner"
	case ArtworkTypeClearlogo:
		return "clearlogo"
	case ArtworkTypeClearart:
		return "clearart"
	case ArtworkTypeDisc:
		return "disc"
	case ArtworkTypeKeyart:
		return "keyart"
	case ArtworkTypeThumb:
		return "landscape"
	case ArtworkTypeCharacterart:
		return "characterart"
	case ArtworkTypeSeasonPoster:
		return "season_poster"
	case ArtworkTypeSeasonFanart:
		return "season_fanart"
	case ArtworkTypeSeasonBanner:
		return "season_banner"
	case ArtworkTypeSeasonThumb:
		return "season_thumb"
	case ArtworkTypeActor:
		return "actor"
	default:
		return "unknown"
	}
}

// PersonType represents the type/role of a person.
type PersonType int

const (
	PersonTypeUnknown PersonType = iota
	PersonTypeActor
	PersonTypeDirector
	PersonTypeWriter
	PersonTypeProducer
	PersonTypeGuest
	PersonTypeComposer
	PersonTypeEditor
	PersonTypeCamera
	PersonTypeOther
)

// String returns the string representation of PersonType.
func (p PersonType) String() string {
	switch p {
	case PersonTypeActor:
		return "actor"
	case PersonTypeDirector:
		return "director"
	case PersonTypeWriter:
		return "writer"
	case PersonTypeProducer:
		return "producer"
	case PersonTypeGuest:
		return "guest"
	case PersonTypeComposer:
		return "composer"
	case PersonTypeEditor:
		return "editor"
	case PersonTypeCamera:
		return "camera"
	case PersonTypeOther:
		return "other"
	default:
		return "unknown"
	}
}

// MediaFileType represents the type of media file.
type MediaFileType int

const (
	MediaFileTypeUnknown MediaFileType = iota
	MediaFileTypeVideo
	MediaFileTypeAudio
	MediaFileTypeSubtitle
	MediaFileTypePoster
	MediaFileTypeFanart
	MediaFileTypeBanner
	MediaFileTypeClearart
	MediaFileTypeClearlogo
	MediaFileTypeDisc
	MediaFileTypeKeyart
	MediaFileTypeThumb
	MediaFileTypeNfo
	MediaFileTypeTrailer
	MediaFileTypeExtra
	MediaFileTypeSample
)

// String returns the string representation of MediaFileType.
func (m MediaFileType) String() string {
	switch m {
	case MediaFileTypeVideo:
		return "video"
	case MediaFileTypeAudio:
		return "audio"
	case MediaFileTypeSubtitle:
		return "subtitle"
	case MediaFileTypePoster:
		return "poster"
	case MediaFileTypeFanart:
		return "fanart"
	case MediaFileTypeBanner:
		return "banner"
	case MediaFileTypeClearart:
		return "clearart"
	case MediaFileTypeClearlogo:
		return "clearlogo"
	case MediaFileTypeDisc:
		return "disc"
	case MediaFileTypeKeyart:
		return "keyart"
	case MediaFileTypeThumb:
		return "thumb"
	case MediaFileTypeNfo:
		return "nfo"
	case MediaFileTypeTrailer:
		return "trailer"
	case MediaFileTypeExtra:
		return "extra"
	case MediaFileTypeSample:
		return "sample"
	default:
		return "unknown"
	}
}

// MediaSource represents the source/format of the media.
type MediaSource int

const (
	MediaSourceUnknown MediaSource = iota
	MediaSourceDVD
	MediaSourceBluRay
	MediaSourceHDRip
	MediaSourceWEBDL
	MediaSourceWEBRip
	MediaSourceTV
)

// String returns the string representation of MediaSource.
func (s MediaSource) String() string {
	switch s {
	case MediaSourceDVD:
		return "dvd"
	case MediaSourceBluRay:
		return "bluray"
	case MediaSourceHDRip:
		return "hdrip"
	case MediaSourceWEBDL:
		return "webdl"
	case MediaSourceWEBRip:
		return "webrip"
	case MediaSourceTV:
		return "tv"
	default:
		return "unknown"
	}
}

// ShowStatus represents the status of a TV show.
type ShowStatus int

const (
	ShowStatusUnknown ShowStatus = iota
	ShowStatusReturning
	ShowStatusEnded
	ShowStatusCanceled
	ShowStatusPilot
	ShowStatusInProduction
)

// String returns the string representation of ShowStatus.
func (s ShowStatus) String() string {
	switch s {
	case ShowStatusReturning:
		return "returning"
	case ShowStatusEnded:
		return "ended"
	case ShowStatusCanceled:
		return "canceled"
	case ShowStatusPilot:
		return "pilot"
	case ShowStatusInProduction:
		return "in_production"
	default:
		return "unknown"
	}
}

// EpisodeGroup represents the grouping method for episodes.
type EpisodeGroup int

const (
	EpisodeGroupUnknown EpisodeGroup = iota
	EpisodeGroupAired
	EpisodeGroupDVD
	EpisodeGroupAbsolute
	EpisodeGroupAlternate
)

// String returns the string representation of EpisodeGroup.
func (e EpisodeGroup) String() string {
	switch e {
	case EpisodeGroupAired:
		return "aired"
	case EpisodeGroupDVD:
		return "dvd"
	case EpisodeGroupAbsolute:
		return "absolute"
	case EpisodeGroupAlternate:
		return "alternate"
	default:
		return "unknown"
	}
}

// MediaRating represents a rating for media.
type MediaRating struct {
	ID    string  // "imdb", "tmdb", "trakt", "douban", "user"
	Value float64 // 0.0-10.0 typically
	Votes int
	Max   int // usually 10
}

// AudioStream represents an audio stream in a media file.
type AudioStream struct {
	Language string
	Codec    string
	Channels int
	BitRate  int
}

// Subtitle represents a subtitle in a media file.
type Subtitle struct {
	Language string
	Codec    string
	Forced   bool
	SDH      bool
	Title    string
}

// Person represents a person (actor, director, etc.).
type Person struct {
	Type       PersonType
	Name       string
	Role       string
	Order      int
	ThumbURL   string
	ProfileURL string
	IDs        map[string]string // "imdb", "tmdb", etc.
}

// MediaArtwork represents artwork/image metadata.
type MediaArtwork struct {
	Type       ArtworkType
	URL        string
	PreviewURL string
	Language   string
	Width      int
	Height     int
	SizeOrder  int
	Likes      int
	Provider   string
	Season     int // -1 for all seasons (Season artwork)
}

// MediaTrailer represents trailer information.
type MediaTrailer struct {
	Name     string
	URL      string
	Provider string
	Quality  string // "360p", "720p", "1080p", etc.
	InNfo    bool   // whether to include in NFO
}

// MediaFile represents a file associated with media.
type MediaFile struct {
	Type           MediaFileType
	Path           string
	Filename       string
	FileSize       int64
	FileDate       time.Time
	VideoCodec     string
	Container      string
	Width          int
	Height         int
	AspectRatio    float64
	FrameRate      float64
	Duration       int // seconds
	BitDepth       int
	HDRFormat      string
	Stacking       int
	StackingMarker string
	AudioStreams   []AudioStream
	Subtitles      []Subtitle
}

// MediaEntity is the base embedded structure for all media types.
type MediaEntity struct {
	DBID            uuid.UUID
	Title           string
	OriginalTitle   string
	SortTitle       string
	Year            int
	Plot            string
	Outline         string
	Tagline         string
	Path            string
	DateAdded       time.Time
	IDs             map[string]string // "imdb", "tmdb", "tvdb", "douban"
	Ratings         map[string]MediaRating
	Genres          []string
	Tags            []string
	Studios         []string
	Countries       []string
	SpokenLanguages []string
	ArtworkURLs     map[ArtworkType]string
	MediaFiles      []MediaFile
	Actors          []Person
	Directors       []Person
	Writers         []Person
	Producers       []Person
	Trailers        []MediaTrailer
	Certification   string // "G", "PG", "PG-13", "R", etc.
	Runtime         int    // minutes
	Locked          bool
	LockedFields    []string
	Sources         map[string]string // field provenance: field name -> provider(s)
	Provider        string            // "tmdb", "douban", "llm", "fused"
	ScrapedAt       time.Time
}

// Movie represents movie metadata.
type Movie struct {
	MediaEntity
	ReleaseDate    time.Time
	MediaSource    MediaSource
	Edition        string
	Top250         int
	MovieSetID     uuid.UUID
	MovieSetName   string
	MovieSetPlot   string
	TmdbCollection int
	Playcount      int
	Watched        bool
}

// TvShow represents TV show metadata.
type TvShow struct {
	MediaEntity
	FirstAired       time.Time
	Status           ShowStatus
	EpisodeGroupKind EpisodeGroup
	SeasonNames      map[int]string // keyed by season number
	SeasonPlots      map[int]string // keyed by season number
}

// TvShowSeason represents season metadata.
type TvShowSeason struct {
	TvShowID    uuid.UUID
	Number      int // 0 = Specials
	Name        string
	Plot        string
	ArtworkURLs map[ArtworkType]string
}

// TvShowEpisode represents episode metadata.
type TvShowEpisode struct {
	MediaEntity
	TvShowID          uuid.UUID
	Season            int
	Episode           int
	DisplaySeason     *int // for DVD ordering
	DisplayEpisode    *int // for DVD ordering
	AirsBeforeSeason  *int
	AirsBeforeEpisode *int
	AirsAfterSeason   *int
	FirstAired        time.Time
	Watched           bool
	Playcount         int
}

// MediaSearchCandidate represents a search result candidate.
type MediaSearchCandidate struct {
	ID        string
	Title     string
	Year      int
	MediaType MediaType
	Provider  string
	PosterURL string
	Overview  string
	Score     float64 // 0.0-1.0, relevance score
}

// SearchResult is a generic search result wrapper.
type SearchResult[T any] struct {
	Items      []T
	TotalCount int
	Provider   string
}
