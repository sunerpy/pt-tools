package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

// DefaultFuser 按字段级优先级融合 TMDB / Douban / LLM 数据源。
type DefaultFuser struct {
	overrides map[string]string
}

func NewDefaultFuser() *DefaultFuser {
	return &DefaultFuser{overrides: map[string]string{}}
}

func NewDefaultFuserWithOverrides(overrides map[string]string) *DefaultFuser {
	if overrides == nil {
		overrides = map[string]string{}
	}
	return &DefaultFuser{overrides: maps.Clone(overrides)}
}

var _ core.Fuser = (*DefaultFuser)(nil)

func (f *DefaultFuser) Merge(_ context.Context, sources map[string]*core.RawMediaInfo) (*core.Movie, error) {
	tmdb, _ := castMovie(sources["tmdb"])
	douban, _ := castMovie(sources["douban"])
	llm, _ := castMovie(sources["llm"])
	if tmdb == nil && douban == nil && llm == nil {
		return nil, errors.New("fuser: no sources to merge")
	}

	out := &core.Movie{}
	state := newMergeState(f.overrides, out.Sources)
	initMovie(out, tmdb, douban, llm)
	state.sources = out.Sources

	mergeMovieEntity(state, &out.MediaEntity, mediaEntityOfMovie(llm), mediaEntityOfMovie(tmdb), mediaEntityOfMovie(douban))

	if m := firstMovie(llm, tmdb, douban); m != nil {
		out.Locked = m.Locked
	}
	for _, movie := range []*core.Movie{llm, tmdb, douban} {
		if movie == nil {
			continue
		}
		out.LockedFields = unionStrings(out.LockedFields, movie.LockedFields)
	}
	setLockedMeta(state, out.Locked, out.LockedFields)

	if pick := pickMovieByPriority(func(m *core.Movie) bool { return m.MediaSource != core.MediaSourceUnknown }, tmdb, douban, llm); pick != nil {
		out.MediaSource = pick.MediaSource
		state.setSource("media_source", pick.Provider)
	}
	state.setString(&out.Edition, "edition", pickMovieString(tmdb, douban, llm, func(m *core.Movie) string { return m.Edition }))
	if pick := pickMovieByPriority(func(m *core.Movie) bool { return m.TmdbCollection > 0 }, tmdb, douban, llm); pick != nil {
		out.TmdbCollection = pick.TmdbCollection
		state.setSource("tmdb_collection", pick.Provider)
	}
	if pick := pickMovieByPriority(func(m *core.Movie) bool { return m.Top250 > 0 }, douban, tmdb, llm); pick != nil {
		out.Top250 = pick.Top250
		state.setSource("top250", pick.Provider)
	}
	if pick := pickMovieByPriority(func(m *core.Movie) bool { return !m.ReleaseDate.IsZero() }, tmdb, douban, llm); pick != nil {
		out.ReleaseDate = pick.ReleaseDate
		state.setSource("release_date", pick.Provider)
	}

	applyOverridesToMovie(state, out)
	finalizeEntity(&out.MediaEntity)
	return out, nil
}

func (f *DefaultFuser) MergeTv(_ context.Context, sources map[string]*core.RawMediaInfo) (*core.TvShow, error) {
	tmdb, _ := castTvShow(sources["tmdb"])
	douban, _ := castTvShow(sources["douban"])
	llm, _ := castTvShow(sources["llm"])
	if tmdb == nil && douban == nil && llm == nil {
		return nil, errors.New("fuser: no sources to merge")
	}

	out := &core.TvShow{}
	state := newMergeState(f.overrides, out.Sources)
	initTvShow(out, tmdb, douban, llm)
	state.sources = out.Sources

	mergeMovieEntity(state, &out.MediaEntity, mediaEntityOfShow(llm), mediaEntityOfShow(tmdb), mediaEntityOfShow(douban))

	if show := firstTvShow(llm, tmdb, douban); show != nil {
		out.Locked = show.Locked
	}
	for _, show := range []*core.TvShow{llm, tmdb, douban} {
		if show == nil {
			continue
		}
		out.LockedFields = unionStrings(out.LockedFields, show.LockedFields)
	}
	setLockedMeta(state, out.Locked, out.LockedFields)

	if pick := pickShowByPriority(func(s *core.TvShow) bool { return !s.FirstAired.IsZero() }, tmdb, douban, llm); pick != nil {
		out.FirstAired = pick.FirstAired
		state.setSource("first_aired", pick.Provider)
	}
	if pick := pickShowByPriority(func(s *core.TvShow) bool { return s.Status != core.ShowStatusUnknown }, tmdb, douban, llm); pick != nil {
		out.Status = pick.Status
		state.setSource("status", pick.Provider)
	}
	out.EpisodeGroupKind = core.EpisodeGroupAired
	state.setSource("episode_group_kind", "default")
	if pick := pickShowByPriority(func(s *core.TvShow) bool { return s.EpisodeGroupKind != core.EpisodeGroupUnknown }, tmdb, douban, llm); pick != nil {
		out.EpisodeGroupKind = pick.EpisodeGroupKind
		state.setSource("episode_group_kind", pick.Provider)
	}
	mergeSeasonMaps(state, out, llm, tmdb, douban)

	applyOverridesToShow(state, out)
	finalizeEntity(&out.MediaEntity)
	return out, nil
}

func (f *DefaultFuser) MergeEpisode(_ context.Context, sources map[string]*core.RawMediaInfo) (*core.TvShowEpisode, error) {
	tmdb, _ := castEpisode(sources["tmdb"])
	douban, _ := castEpisode(sources["douban"])
	llm, _ := castEpisode(sources["llm"])
	if tmdb == nil && douban == nil && llm == nil {
		return nil, errors.New("fuser: no sources to merge")
	}

	out := &core.TvShowEpisode{}
	state := newMergeState(f.overrides, out.Sources)
	initEpisode(out, tmdb, douban, llm)
	state.sources = out.Sources

	mergeMovieEntity(state, &out.MediaEntity, mediaEntityOfEpisode(llm), mediaEntityOfEpisode(tmdb), mediaEntityOfEpisode(douban))

	if episode := firstEpisode(llm, tmdb, douban); episode != nil {
		out.Locked = episode.Locked
	}
	for _, episode := range []*core.TvShowEpisode{llm, tmdb, douban} {
		if episode == nil {
			continue
		}
		out.LockedFields = unionStrings(out.LockedFields, episode.LockedFields)
	}
	setLockedMeta(state, out.Locked, out.LockedFields)

	if pick := pickEpisodeByPriority(func(e *core.TvShowEpisode) bool { return e.Season > 0 }, tmdb, douban, llm); pick != nil {
		out.Season = pick.Season
		state.setSource("season", pick.Provider)
	}
	if pick := pickEpisodeByPriority(func(e *core.TvShowEpisode) bool { return e.Episode > 0 }, tmdb, douban, llm); pick != nil {
		out.Episode = pick.Episode
		state.setSource("episode", pick.Provider)
	}
	if pick := pickEpisodeByPriority(func(e *core.TvShowEpisode) bool { return !e.FirstAired.IsZero() }, tmdb, douban, llm); pick != nil {
		out.FirstAired = pick.FirstAired
		state.setSource("first_aired", pick.Provider)
	}

	applyOverridesToEpisode(state, out)
	finalizeEntity(&out.MediaEntity)
	return out, nil
}

type mergeState struct {
	overrides map[string]string
	sources   map[string]string
	locked    map[string]struct{}
}

func newMergeState(overrides, sources map[string]string) *mergeState {
	if overrides == nil {
		overrides = map[string]string{}
	}
	if sources == nil {
		sources = map[string]string{}
	}
	return &mergeState{
		overrides: maps.Clone(overrides),
		sources:   sources,
		locked:    map[string]struct{}{},
	}
}

func (s *mergeState) setSource(field, provider string) {
	if provider == "" {
		return
	}
	s.sources[field] = provider
}

func (s *mergeState) isLocked(field string) bool {
	_, ok := s.locked[normalizeFieldName(field)]
	return ok
}

func (s *mergeState) markLocked(fields []string) {
	for _, field := range fields {
		norm := normalizeFieldName(field)
		if norm == "" {
			continue
		}
		s.locked[norm] = struct{}{}
	}
}

func (s *mergeState) setString(target *string, field string, candidate fieldString) {
	if target == nil || strings.TrimSpace(candidate.value) == "" {
		return
	}
	if s.isLocked(field) && strings.TrimSpace(*target) != "" {
		return
	}
	*target = strings.TrimSpace(candidate.value)
	s.setSource(field, candidate.provider)
}

func (s *mergeState) applyOverrideString(target *string, field string) {
	if target == nil {
		return
	}
	value, ok := s.overrides[normalizeFieldName(field)]
	if !ok || strings.TrimSpace(value) == "" {
		return
	}
	*target = strings.TrimSpace(value)
	s.setSource(field, "override")
}

func (s *mergeState) applyOverrideInt(target *int, field string) {
	if target == nil {
		return
	}
	value, ok := s.overrides[normalizeFieldName(field)]
	if !ok || strings.TrimSpace(value) == "" {
		return
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return
	}
	*target = parsed
	s.setSource(field, "override")
}

func (s *mergeState) applyOverrideDate(target *time.Time, field string) {
	if target == nil {
		return
	}
	value, ok := s.overrides[normalizeFieldName(field)]
	if !ok || strings.TrimSpace(value) == "" {
		return
	}
	parsed, err := parseOverrideDate(value)
	if err != nil {
		return
	}
	*target = parsed
	s.setSource(field, "override")
}

type fieldString struct {
	value    string
	provider string
}

func mergeMovieEntity(state *mergeState, out, llm, tmdb, douban *core.MediaEntity) {
	state.setString(&out.Title, "title", pickTitle(llm, tmdb, douban))
	state.setString(&out.OriginalTitle, "original_title", pickStringByEntity(func(m *core.MediaEntity) string { return m.OriginalTitle }, tmdb, douban, llm))
	state.setString(&out.SortTitle, "sort_title", pickStringByEntity(func(m *core.MediaEntity) string { return m.SortTitle }, tmdb, douban, llm))
	if pick := pickEntityByPriority(func(m *core.MediaEntity) bool { return m.Year > 0 }, tmdb, douban, llm); pick != nil {
		out.Year = pick.Year
		state.setSource("year", pick.Provider)
	}
	state.setString(&out.Plot, "plot", pickStringByEntity(func(m *core.MediaEntity) string { return m.Plot }, douban, tmdb, llm))
	state.setString(&out.Outline, "outline", pickStringByEntity(func(m *core.MediaEntity) string { return m.Outline }, douban, tmdb, llm))
	state.setString(&out.Tagline, "tagline", pickStringByEntity(func(m *core.MediaEntity) string { return m.Tagline }, tmdb, douban, llm))
	if pick := pickEntityByPriority(func(m *core.MediaEntity) bool { return m.Runtime > 0 }, tmdb, douban, llm); pick != nil {
		out.Runtime = pick.Runtime
		state.setSource("runtime", pick.Provider)
	}

	out.Genres = unionFromEntities(state, "genres", llm, tmdb, douban)
	out.Tags = unionFromEntities(state, "tags", llm, tmdb, douban)
	out.Studios = pickSliceByEntity(state, "studios", tmdb, douban, llm)
	out.Countries = pickSliceByEntity(state, "countries", tmdb, douban, llm)
	out.SpokenLanguages = pickSliceByEntity(state, "spoken_languages", tmdb, douban, llm)
	out.Ratings = mergeRatings(state, tmdb, douban, llm)
	out.IDs = mergeIDs(state, tmdb, douban, llm)
	out.ArtworkURLs = pickArtwork(state, tmdb)
	out.Actors = pickPeople(state, "actors", tmdb, func(m *core.MediaEntity) []core.Person { return m.Actors })
	out.Directors = pickPeople(state, "directors", tmdb, func(m *core.MediaEntity) []core.Person { return m.Directors })
	out.Writers = pickPeople(state, "writers", tmdb, func(m *core.MediaEntity) []core.Person { return m.Writers })
	out.Producers = pickPeople(state, "producers", tmdb, func(m *core.MediaEntity) []core.Person { return m.Producers })
	out.Trailers = pickTrailers(state, tmdb)
	state.setString(&out.Certification, "certification", pickStringByEntity(func(m *core.MediaEntity) string { return m.Certification }, tmdb, douban, llm))

	state.markLocked(out.LockedFields)
}

func initMovie(out, tmdb, douban, llm *core.Movie) {
	base := firstMovie(llm, tmdb, douban)
	if base == nil {
		return
	}
	out.MovieSetID = base.MovieSetID
	out.MovieSetName = base.MovieSetName
	out.MovieSetPlot = base.MovieSetPlot
	out.Playcount = base.Playcount
	out.Watched = base.Watched
	out.MediaFiles = cloneMediaFiles(base.MediaFiles)
	out.Locked = base.Locked
	out.LockedFields = append([]string(nil), base.LockedFields...)
	out.Sources = map[string]string{}
	if base.Sources != nil {
		out.Sources = maps.Clone(base.Sources)
	}
	if out.Sources == nil {
		out.Sources = map[string]string{}
	}
	if out.Locked {
		out.Sources["locked"] = base.Provider
	}
	if len(out.LockedFields) > 0 {
		out.Sources["locked_fields"] = base.Provider
	}
}

func initTvShow(out, tmdb, douban, llm *core.TvShow) {
	base := firstTvShow(llm, tmdb, douban)
	if base == nil {
		return
	}
	out.MediaFiles = cloneMediaFiles(base.MediaFiles)
	out.Locked = base.Locked
	out.LockedFields = append([]string(nil), base.LockedFields...)
	out.Sources = map[string]string{}
	if base.Sources != nil {
		out.Sources = maps.Clone(base.Sources)
	}
	if out.Sources == nil {
		out.Sources = map[string]string{}
	}
	out.SeasonNames = map[int]string{}
	out.SeasonPlots = map[int]string{}
	if out.Locked {
		out.Sources["locked"] = base.Provider
	}
	if len(out.LockedFields) > 0 {
		out.Sources["locked_fields"] = base.Provider
	}
}

func initEpisode(out, tmdb, douban, llm *core.TvShowEpisode) {
	base := firstEpisode(llm, tmdb, douban)
	if base == nil {
		return
	}
	out.TvShowID = base.TvShowID
	out.DisplaySeason = cloneOptionalInt(base.DisplaySeason)
	out.DisplayEpisode = cloneOptionalInt(base.DisplayEpisode)
	out.AirsBeforeSeason = cloneOptionalInt(base.AirsBeforeSeason)
	out.AirsBeforeEpisode = cloneOptionalInt(base.AirsBeforeEpisode)
	out.AirsAfterSeason = cloneOptionalInt(base.AirsAfterSeason)
	out.Watched = base.Watched
	out.Playcount = base.Playcount
	out.MediaFiles = cloneMediaFiles(base.MediaFiles)
	out.Locked = base.Locked
	out.LockedFields = append([]string(nil), base.LockedFields...)
	out.Sources = map[string]string{}
	if base.Sources != nil {
		out.Sources = maps.Clone(base.Sources)
	}
	if out.Sources == nil {
		out.Sources = map[string]string{}
	}
	if out.Locked {
		out.Sources["locked"] = base.Provider
	}
	if len(out.LockedFields) > 0 {
		out.Sources["locked_fields"] = base.Provider
	}
}

func finalizeEntity(entity *core.MediaEntity) {
	if entity == nil {
		return
	}
	entity.Provider = "fused"
	entity.ScrapedAt = time.Now()
	if entity.Sources == nil {
		entity.Sources = map[string]string{}
	}
	if entity.IDs == nil {
		entity.IDs = map[string]string{}
	}
	if entity.Ratings == nil {
		entity.Ratings = map[string]core.MediaRating{}
	}
	if entity.ArtworkURLs == nil {
		entity.ArtworkURLs = map[core.ArtworkType]string{}
	}
}

func setLockedMeta(state *mergeState, locked bool, fields []string) {
	state.markLocked(fields)
	if locked {
		state.setSource("locked", firstNonEmpty(state.sources["locked"], "external"))
	}
	if len(fields) > 0 {
		state.setSource("locked_fields", firstNonEmpty(state.sources["locked_fields"], "external"))
	}
}

func mergeSeasonMaps(state *mergeState, out, llm, tmdb, douban *core.TvShow) {
	providers := []struct {
		name string
		show *core.TvShow
	}{
		{name: "llm", show: llm},
		{name: "tmdb", show: tmdb},
		{name: "douban", show: douban},
	}
	nameProviders := make([]string, 0, len(providers))
	plotProviders := make([]string, 0, len(providers))
	for _, item := range providers {
		if item.show == nil {
			continue
		}
		for season, name := range item.show.SeasonNames {
			if strings.TrimSpace(name) == "" {
				continue
			}
			out.SeasonNames[season] = strings.TrimSpace(name)
		}
		if len(item.show.SeasonNames) > 0 {
			nameProviders = append(nameProviders, item.name)
		}
		for season, plot := range item.show.SeasonPlots {
			if strings.TrimSpace(plot) == "" {
				continue
			}
			out.SeasonPlots[season] = strings.TrimSpace(plot)
		}
		if len(item.show.SeasonPlots) > 0 {
			plotProviders = append(plotProviders, item.name)
		}
	}
	if len(out.SeasonNames) > 0 {
		state.setSource("season_names", strings.Join(nameProviders, ","))
	}
	if len(out.SeasonPlots) > 0 {
		state.setSource("season_plots", strings.Join(plotProviders, ","))
	}
}

func applyOverridesToMovie(state *mergeState, out *core.Movie) {
	state.applyOverrideString(&out.Title, "title")
	state.applyOverrideString(&out.OriginalTitle, "original_title")
	state.applyOverrideString(&out.SortTitle, "sort_title")
	state.applyOverrideString(&out.Plot, "plot")
	state.applyOverrideString(&out.Outline, "outline")
	state.applyOverrideString(&out.Tagline, "tagline")
	state.applyOverrideString(&out.Certification, "certification")
	state.applyOverrideString(&out.Edition, "edition")
	state.applyOverrideInt(&out.Year, "year")
	state.applyOverrideInt(&out.Runtime, "runtime")
	state.applyOverrideInt(&out.Top250, "top250")
	state.applyOverrideInt(&out.TmdbCollection, "tmdb_collection")
	state.applyOverrideDate(&out.ReleaseDate, "release_date")
}

func applyOverridesToShow(state *mergeState, out *core.TvShow) {
	state.applyOverrideString(&out.Title, "title")
	state.applyOverrideString(&out.OriginalTitle, "original_title")
	state.applyOverrideString(&out.SortTitle, "sort_title")
	state.applyOverrideString(&out.Plot, "plot")
	state.applyOverrideString(&out.Outline, "outline")
	state.applyOverrideString(&out.Tagline, "tagline")
	state.applyOverrideString(&out.Certification, "certification")
	state.applyOverrideInt(&out.Year, "year")
	state.applyOverrideInt(&out.Runtime, "runtime")
	state.applyOverrideDate(&out.FirstAired, "first_aired")
}

func applyOverridesToEpisode(state *mergeState, out *core.TvShowEpisode) {
	state.applyOverrideString(&out.Title, "title")
	state.applyOverrideString(&out.OriginalTitle, "original_title")
	state.applyOverrideString(&out.SortTitle, "sort_title")
	state.applyOverrideString(&out.Plot, "plot")
	state.applyOverrideString(&out.Outline, "outline")
	state.applyOverrideString(&out.Tagline, "tagline")
	state.applyOverrideString(&out.Certification, "certification")
	state.applyOverrideInt(&out.Year, "year")
	state.applyOverrideInt(&out.Runtime, "runtime")
	state.applyOverrideInt(&out.Season, "season")
	state.applyOverrideInt(&out.Episode, "episode")
	state.applyOverrideDate(&out.FirstAired, "first_aired")
}

func pickTitle(llm, tmdb, douban *core.MediaEntity) fieldString {
	if value := strings.TrimSpace(valueFromEntity(douban, func(m *core.MediaEntity) string { return m.Title })); value != "" {
		return fieldString{value: value, provider: douban.Provider}
	}
	if value := strings.TrimSpace(valueFromEntity(tmdb, func(m *core.MediaEntity) string { return m.Title })); value != "" {
		return fieldString{value: value, provider: tmdb.Provider}
	}
	if value := strings.TrimSpace(valueFromEntity(llm, func(m *core.MediaEntity) string { return m.Title })); value != "" {
		return fieldString{value: value, provider: llm.Provider}
	}
	return fieldString{}
}

func pickStringByEntity(getter func(*core.MediaEntity) string, entities ...*core.MediaEntity) fieldString {
	for _, entity := range entities {
		if entity == nil {
			continue
		}
		value := strings.TrimSpace(getter(entity))
		if value == "" {
			continue
		}
		return fieldString{value: value, provider: entity.Provider}
	}
	return fieldString{}
}

func pickSliceByEntity(state *mergeState, field string, entities ...*core.MediaEntity) []string {
	for _, entity := range entities {
		if entity == nil {
			continue
		}
		var values []string
		switch field {
		case "studios":
			values = entity.Studios
		case "countries":
			values = entity.Countries
		case "spoken_languages":
			values = entity.SpokenLanguages
		}
		trimmed := compactStrings(values)
		if len(trimmed) == 0 {
			continue
		}
		state.setSource(field, entity.Provider)
		return trimmed
	}
	return nil
}

func mergeRatings(state *mergeState, entities ...*core.MediaEntity) map[string]core.MediaRating {
	out := map[string]core.MediaRating{}
	providers := make([]string, 0, len(entities))
	for _, entity := range entities {
		if entity == nil || len(entity.Ratings) == 0 {
			continue
		}
		providers = append(providers, entity.Provider)
		keys := slices.Sorted(maps.Keys(entity.Ratings))
		for _, key := range keys {
			if _, exists := out[key]; exists {
				continue
			}
			out[key] = entity.Ratings[key]
		}
	}
	if len(out) > 0 {
		state.setSource("ratings", strings.Join(providers, ","))
	}
	return out
}

func mergeIDs(state *mergeState, tmdb, douban, llm *core.MediaEntity) map[string]string {
	out := map[string]string{}
	providers := make([]string, 0, 3)
	for _, entity := range []*core.MediaEntity{tmdb, douban, llm} {
		if entity == nil || len(entity.IDs) == 0 {
			continue
		}
		providers = append(providers, entity.Provider)
		keys := slices.Sorted(maps.Keys(entity.IDs))
		for _, key := range keys {
			if _, exists := out[key]; exists {
				continue
			}
			value := strings.TrimSpace(entity.IDs[key])
			if value == "" {
				continue
			}
			out[key] = value
		}
	}
	if len(out) > 0 {
		state.setSource("ids", strings.Join(providers, ","))
	}
	return out
}

func pickArtwork(state *mergeState, tmdb *core.MediaEntity) map[core.ArtworkType]string {
	if tmdb == nil || len(tmdb.ArtworkURLs) == 0 {
		return map[core.ArtworkType]string{}
	}
	out := map[core.ArtworkType]string{}
	for key, value := range tmdb.ArtworkURLs {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) > 0 {
		state.setSource("artwork_urls", tmdb.Provider)
	}
	return out
}

func pickPeople(state *mergeState, field string, tmdb *core.MediaEntity, getter func(*core.MediaEntity) []core.Person) []core.Person {
	if tmdb == nil {
		return nil
	}
	people := clonePeople(getter(tmdb))
	if len(people) > 0 {
		state.setSource(field, tmdb.Provider)
	}
	return people
}

func pickTrailers(state *mergeState, tmdb *core.MediaEntity) []core.MediaTrailer {
	if tmdb == nil {
		return nil
	}
	trailers := cloneTrailers(tmdb.Trailers)
	if len(trailers) > 0 {
		state.setSource("trailers", tmdb.Provider)
	}
	return trailers
}

func unionFromEntities(state *mergeState, field string, entities ...*core.MediaEntity) []string {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	providers := make([]string, 0, len(entities))
	for _, entity := range entities {
		if entity == nil {
			continue
		}
		var values []string
		switch field {
		case "genres":
			values = entity.Genres
		case "tags":
			values = entity.Tags
		}
		added := false
		for _, raw := range values {
			value := strings.TrimSpace(raw)
			if value == "" {
				continue
			}
			key := strings.ToLower(value)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, value)
			added = true
		}
		if added {
			providers = append(providers, entity.Provider)
		}
	}
	if len(out) > 0 {
		state.setSource(field, strings.Join(providers, ","))
	}
	return out
}

func pickEntityByPriority(predicate func(*core.MediaEntity) bool, entities ...*core.MediaEntity) *core.MediaEntity {
	for _, entity := range entities {
		if entity == nil {
			continue
		}
		if predicate(entity) {
			return entity
		}
	}
	return nil
}

func castMovie(raw *core.RawMediaInfo) (*core.Movie, bool) {
	if raw == nil || raw.Data == nil {
		return nil, false
	}
	movie, ok := raw.Data.(*core.Movie)
	return movie, ok && movie != nil
}

func castTvShow(raw *core.RawMediaInfo) (*core.TvShow, bool) {
	if raw == nil || raw.Data == nil {
		return nil, false
	}
	show, ok := raw.Data.(*core.TvShow)
	return show, ok && show != nil
}

func castEpisode(raw *core.RawMediaInfo) (*core.TvShowEpisode, bool) {
	if raw == nil || raw.Data == nil {
		return nil, false
	}
	episode, ok := raw.Data.(*core.TvShowEpisode)
	return episode, ok && episode != nil
}

func mediaEntityOfMovie(movie *core.Movie) *core.MediaEntity {
	if movie == nil {
		return nil
	}
	return &movie.MediaEntity
}

func mediaEntityOfShow(show *core.TvShow) *core.MediaEntity {
	if show == nil {
		return nil
	}
	return &show.MediaEntity
}

func mediaEntityOfEpisode(episode *core.TvShowEpisode) *core.MediaEntity {
	if episode == nil {
		return nil
	}
	return &episode.MediaEntity
}

func firstMovie(items ...*core.Movie) *core.Movie {
	for _, item := range items {
		if item != nil {
			return item
		}
	}
	return nil
}

func firstTvShow(items ...*core.TvShow) *core.TvShow {
	for _, item := range items {
		if item != nil {
			return item
		}
	}
	return nil
}

func firstEpisode(items ...*core.TvShowEpisode) *core.TvShowEpisode {
	for _, item := range items {
		if item != nil {
			return item
		}
	}
	return nil
}

func pickMovieByPriority(predicate func(*core.Movie) bool, items ...*core.Movie) *core.Movie {
	for _, item := range items {
		if item == nil {
			continue
		}
		if predicate(item) {
			return item
		}
	}
	return nil
}

func pickShowByPriority(predicate func(*core.TvShow) bool, items ...*core.TvShow) *core.TvShow {
	for _, item := range items {
		if item == nil {
			continue
		}
		if predicate(item) {
			return item
		}
	}
	return nil
}

func pickEpisodeByPriority(predicate func(*core.TvShowEpisode) bool, items ...*core.TvShowEpisode) *core.TvShowEpisode {
	for _, item := range items {
		if item == nil {
			continue
		}
		if predicate(item) {
			return item
		}
	}
	return nil
}

func pickMovieString(a, b, c *core.Movie, getter func(*core.Movie) string) fieldString {
	for _, movie := range []*core.Movie{a, b, c} {
		if movie == nil {
			continue
		}
		value := strings.TrimSpace(getter(movie))
		if value == "" {
			continue
		}
		return fieldString{value: value, provider: movie.Provider}
	}
	return fieldString{}
}

func valueFromEntity(entity *core.MediaEntity, getter func(*core.MediaEntity) string) string {
	if entity == nil {
		return ""
	}
	return getter(entity)
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func unionStrings(left, right []string) []string {
	out := append([]string(nil), left...)
	seen := map[string]struct{}{}
	for _, value := range out {
		seen[normalizeFieldName(value)] = struct{}{}
	}
	for _, value := range right {
		if strings.TrimSpace(value) == "" {
			continue
		}
		norm := normalizeFieldName(value)
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}
		out = append(out, strings.TrimSpace(value))
	}
	return out
}

func cloneMediaFiles(values []core.MediaFile) []core.MediaFile {
	if len(values) == 0 {
		return nil
	}
	out := make([]core.MediaFile, len(values))
	copy(out, values)
	return out
}

func clonePeople(values []core.Person) []core.Person {
	if len(values) == 0 {
		return nil
	}
	out := make([]core.Person, len(values))
	for i, value := range values {
		out[i] = value
		if value.IDs != nil {
			out[i].IDs = maps.Clone(value.IDs)
		}
	}
	return out
}

func cloneTrailers(values []core.MediaTrailer) []core.MediaTrailer {
	if len(values) == 0 {
		return nil
	}
	out := make([]core.MediaTrailer, len(values))
	copy(out, values)
	return out
}

func cloneOptionalInt(value *int) *int {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func parseOverrideDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339, "2006-01-02", "2006-01-02 15:04:05", "2006"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid override date: %s", value)
}

func normalizeFieldName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
