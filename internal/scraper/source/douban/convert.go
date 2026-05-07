package douban

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

func toMovie(detail *subjectDetailResponse, celebs *celebritiesResponse, photos *photosResponse) *core.Movie {
	if detail == nil {
		return nil
	}

	movie := &core.Movie{
		MediaEntity: baseEntity(detail),
		ReleaseDate: parseDate(detail.Pubdate),
	}
	movie.Provider = "douban"
	movie.ScrapedAt = time.Now()
	movie.Top250 = parseTop250(detail.CardSubtitle)

	applyCelebrities(movie, celebs)
	applyPhotos(&movie.MediaEntity, photos)
	return movie
}

func toTVShow(detail *subjectDetailResponse) *core.TvShow {
	if detail == nil {
		return nil
	}

	show := &core.TvShow{
		MediaEntity:      baseEntity(detail),
		FirstAired:       parseDate(detail.Pubdate),
		EpisodeGroupKind: core.EpisodeGroupAired,
		SeasonNames:      map[int]string{},
		SeasonPlots:      map[int]string{},
	}
	show.Provider = "douban"
	show.ScrapedAt = time.Now()
	return show
}

func htmlToMovie(detail *htmlDetail) *core.Movie {
	if detail == nil {
		return nil
	}
	return &core.Movie{
		MediaEntity: core.MediaEntity{
			Title:         detail.Title,
			OriginalTitle: firstNonEmpty(detail.OriginalTitle, detail.Title),
			Year:          detail.Year,
			Plot:          detail.Plot,
			Outline:       detail.Plot,
			IDs:           buildIDs(detail.ID, detail.IMDBID),
			Ratings:       buildRatings(detail.Rating, 0),
			Actors:        peopleFromNames(detail.Actors, core.PersonTypeActor),
			Directors:     peopleFromNames(detail.Directors, core.PersonTypeDirector),
			Provider:      "douban",
			ScrapedAt:     time.Now(),
		},
	}
}

func htmlToTVShow(detail *htmlDetail) *core.TvShow {
	if detail == nil {
		return nil
	}
	return &core.TvShow{
		MediaEntity: core.MediaEntity{
			Title:         detail.Title,
			OriginalTitle: firstNonEmpty(detail.OriginalTitle, detail.Title),
			Year:          detail.Year,
			Plot:          detail.Plot,
			Outline:       detail.Plot,
			IDs:           buildIDs(detail.ID, detail.IMDBID),
			Ratings:       buildRatings(detail.Rating, 0),
			Actors:        peopleFromNames(detail.Actors, core.PersonTypeActor),
			Directors:     peopleFromNames(detail.Directors, core.PersonTypeDirector),
			Provider:      "douban",
			ScrapedAt:     time.Now(),
		},
		EpisodeGroupKind: core.EpisodeGroupAired,
		SeasonNames:      map[int]string{},
		SeasonPlots:      map[int]string{},
	}
}

func searchCandidateFromItem(item searchItem) core.MediaSearchCandidate {
	targetItem := item
	if item.Target != nil {
		targetItem = searchItem{
			ID:            item.Target.ID,
			Type:          item.Target.Type,
			Title:         item.Target.Title,
			OriginalTitle: item.Target.OriginalTitle,
			Year:          item.Target.Year,
			Abstract:      item.Target.Abstract,
			CardSubtitle:  item.Target.CardSubtitle,
			Pic:           item.Target.Pic,
			URL:           item.Target.URL,
			URI:           item.Target.URI,
		}
	}

	// 豆瓣 Frodo 2025+ 响应开始将 type 字段留空，只在 URI 里保留 /movie/ /tv/
	// 路径。上层 scraper.SearchMovie / SearchTvShow 按 MediaType 过滤，空 type
	// 会导致所有结果被丢弃（Unknown）。这里从 URI/URL 推断类型作为 fallback。
	mediaType := mapMediaType(targetItem.Type)
	if mediaType == core.MediaTypeUnknown {
		mediaType = mapMediaType(mediaTypeFromURI(targetItem.URI, targetItem.URL))
	}

	return core.MediaSearchCandidate{
		ID:        targetItem.ID,
		Title:     firstNonEmpty(targetItem.Title, targetItem.OriginalTitle),
		Year:      parseYear(targetItem.Year, targetItem.CardSubtitle),
		MediaType: mediaType,
		Provider:  "douban",
		PosterURL: bestImage(targetItem.Pic),
		Overview:  targetItem.Abstract,
		Score:     normalizedScore(item.Rating.Value),
	}
}

// mediaTypeFromURI 从 Frodo URI 或 Web URL 中提取 /movie/ 或 /tv/ 片段。
// 已知格式：
//   - douban://douban.com/movie/3541415  （Frodo target.uri）
//   - douban://douban.com/tv/26794435    （同上）
//   - https://movie.douban.com/subject/N （Web URL —— 域名含 movie 即判电影）
func mediaTypeFromURI(uri, url string) string {
	for _, s := range []string{uri, url} {
		lower := strings.ToLower(s)
		if lower == "" {
			continue
		}
		if strings.Contains(lower, "/movie/") || strings.Contains(lower, "movie.douban.com") {
			return "movie"
		}
		if strings.Contains(lower, "/tv/") || strings.Contains(lower, "/tv_show/") {
			return "tv"
		}
	}
	return ""
}

func baseEntity(detail *subjectDetailResponse) core.MediaEntity {
	title := firstNonEmpty(detail.Title, detail.OriginalTitle)
	return core.MediaEntity{
		Title:           title,
		OriginalTitle:   firstNonEmpty(detail.OriginalTitle, title),
		Year:            parseYear(detail.Year, detail.CardSubtitle),
		Plot:            detail.Intro,
		Outline:         detail.Intro,
		IDs:             buildIDs(detail.ID, firstNonEmpty(detail.IMDBID, detail.IMDB)),
		Ratings:         buildRatings(detail.Rating.Value, detail.Rating.Count),
		Genres:          append([]string(nil), detail.Genres...),
		Countries:       append([]string(nil), detail.Countries...),
		SpokenLanguages: append([]string(nil), detail.Languages...),
		ArtworkURLs:     buildArtworkURLs(detail.Pic),
		Actors:          peopleFromNames(detail.Actors, core.PersonTypeActor),
		Directors:       peopleFromNames(detail.Directors, core.PersonTypeDirector),
		Runtime:         parseRuntime(detail.Durations),
	}
}

func applyCelebrities(movie *core.Movie, celebs *celebritiesResponse) {
	if movie == nil || celebs == nil {
		return
	}
	actors := make([]core.Person, 0)
	directors := make([]core.Person, 0)
	for idx, celeb := range celebs.Celebrities {
		person := core.Person{
			Name:       firstNonEmpty(celeb.Name, celeb.LatinName),
			Role:       celeb.Character,
			Order:      idx,
			ThumbURL:   firstNonEmpty(bestImage(celeb.Avatar), celeb.CoverURL),
			ProfileURL: celeb.URL,
		}
		switch normalizeType(celeb.Role, celeb.Type) {
		case "director":
			person.Type = core.PersonTypeDirector
			directors = append(directors, person)
		case "writer":
			person.Type = core.PersonTypeWriter
			movie.Writers = append(movie.Writers, person)
		default:
			person.Type = core.PersonTypeActor
			actors = append(actors, person)
		}
	}
	if len(actors) > 0 {
		movie.Actors = actors
	}
	if len(directors) > 0 {
		movie.Directors = directors
	}
}

func applyPhotos(entity *core.MediaEntity, photos *photosResponse) {
	if entity == nil || photos == nil || len(photos.Photos) == 0 {
		return
	}
	if entity.ArtworkURLs == nil {
		entity.ArtworkURLs = map[core.ArtworkType]string{}
	}
	for _, photo := range photos.Photos {
		url := firstNonEmpty(photo.Image.Large, photo.Cover, photo.Thumb)
		if url == "" {
			continue
		}
		if entity.ArtworkURLs[core.ArtworkTypeBackground] == "" {
			entity.ArtworkURLs[core.ArtworkTypeBackground] = url
		}
		break
	}
}

func buildIDs(doubanID, imdbID string) map[string]string {
	ids := map[string]string{}
	if doubanID != "" {
		ids["douban"] = doubanID
	}
	if imdbID != "" {
		ids["imdb"] = imdbID
	}
	return ids
}

func buildRatings(value float64, votes int) map[string]core.MediaRating {
	if value <= 0 {
		return nil
	}
	return map[string]core.MediaRating{
		"douban": {
			ID:    "douban",
			Value: value,
			Votes: votes,
			Max:   10,
		},
	}
}

func buildArtworkURLs(pic image) map[core.ArtworkType]string {
	poster := bestImage(pic)
	if poster == "" {
		return nil
	}
	return map[core.ArtworkType]string{core.ArtworkTypePoster: poster}
}

func bestImage(pic image) string {
	return firstNonEmpty(pic.Large, pic.Normal, pic.Small)
}

func peopleFromNames(names []string, personType core.PersonType) []core.Person {
	people := make([]core.Person, 0, len(names))
	for idx, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		people = append(people, core.Person{Type: personType, Name: trimmed, Order: idx})
	}
	return people
}

func parseDate(values []string) time.Time {
	for _, value := range values {
		value = strings.TrimSpace(value)
		for _, layout := range []string{"2006-01-02", "2006-1-2", "2006/01/02", "2006"} {
			if ts, err := time.Parse(layout, firstDateToken(value)); err == nil {
				return ts
			}
		}
	}
	return time.Time{}
}

func firstDateToken(value string) string {
	if idx := strings.IndexAny(value, "(（ "); idx >= 0 {
		return value[:idx]
	}
	return value
}

func parseRuntime(durations []string) int {
	for _, duration := range durations {
		match := regexp.MustCompile(`\d+`).FindString(duration)
		if match == "" {
			continue
		}
		minutes, err := strconv.Atoi(match)
		if err == nil {
			return minutes
		}
	}
	return 0
}

func parseYear(values ...string) int {
	for _, value := range values {
		if match := yearRegexp.FindString(value); match != "" {
			year, err := strconv.Atoi(match)
			if err == nil {
				return year
			}
		}
	}
	return 0
}

func normalizedScore(value float64) float64 {
	if value <= 0 {
		return 0
	}
	if value > 1 {
		return value / 10
	}
	return value
}

func mapMediaType(kind string) core.MediaType {
	switch strings.ToLower(kind) {
	case "movie":
		return core.MediaTypeMovie
	case "tv", "tv_show", "tvshow":
		return core.MediaTypeTvShow
	default:
		return core.MediaTypeUnknown
	}
}

func normalizeType(values ...string) string {
	for _, value := range values {
		lower := strings.ToLower(strings.TrimSpace(value))
		switch {
		case strings.Contains(lower, "director") || strings.Contains(lower, "导演"):
			return "director"
		case strings.Contains(lower, "writer") || strings.Contains(lower, "编剧"):
			return "writer"
		case strings.Contains(lower, "actor") || strings.Contains(lower, "cast") || strings.Contains(lower, "主演"):
			return "actor"
		}
	}
	return "actor"
}

func parseTop250(value string) int {
	match := regexp.MustCompile(`TOP\s*(\d+)`).FindStringSubmatch(strings.ToUpper(value))
	if len(match) != 2 {
		return 0
	}
	result, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
