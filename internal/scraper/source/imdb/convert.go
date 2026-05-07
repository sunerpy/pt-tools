package imdb

import (
	"time"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

// detailToMovie 把 titleDetail 映射到 core.Movie。IMDb 提供的字段比 TMDB 少
// 很多（没有 collection / production companies / backdrops），我们只填充
// 能可靠抽取的部分；其余字段留零值由 Fuser 在多源合并时补齐。
func detailToMovie(d *titleDetail) *core.Movie {
	if d == nil {
		return nil
	}
	m := &core.Movie{MediaEntity: buildEntity(d)}
	if d.Year > 0 {
		// IMDb 只有年份，没有精确发行日期。用 YYYY-01-01 作为占位，保持
		// ReleaseDate 字段非零方便下游比较。Fuser 会让 TMDB/Douban 的精确
		// 日期覆盖此占位值。
		m.ReleaseDate = time.Date(d.Year, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
	return m
}

// detailToTvShow 把 titleDetail 映射到 core.TvShow。
func detailToTvShow(d *titleDetail) *core.TvShow {
	if d == nil {
		return nil
	}
	show := &core.TvShow{MediaEntity: buildEntity(d)}
	if d.Year > 0 {
		show.FirstAired = time.Date(d.Year, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
	return show
}

func buildEntity(d *titleDetail) core.MediaEntity {
	entity := core.MediaEntity{
		Title:         d.Title,
		OriginalTitle: firstNonEmpty(d.OriginalTitle, d.Title),
		Year:          d.Year,
		Plot:          d.Plot,
		Outline:       d.Plot,
		Genres:        append([]string(nil), d.Genres...),
		Runtime:       d.Runtime,
		IDs: map[string]string{
			"imdb": d.IMDBID,
		},
	}
	if d.Rating > 0 || d.RatingCount > 0 {
		entity.Ratings = map[string]core.MediaRating{
			"imdb": {
				ID:    "imdb",
				Value: d.Rating,
				Votes: d.RatingCount,
				Max:   10,
			},
		}
	}
	if d.PosterURL != "" {
		entity.ArtworkURLs = map[core.ArtworkType]string{
			core.ArtworkTypePoster: d.PosterURL,
		}
	}
	entity.Directors = peopleFromNames(d.Directors, core.PersonTypeDirector)
	entity.Actors = peopleFromNames(d.Actors, core.PersonTypeActor)
	return entity
}

// peopleFromNames 把字符串名字列表转为 core.Person 列表。
// 与 douban.peopleFromNames 保持一致的 Person 字段填充策略。
func peopleFromNames(names []string, personType core.PersonType) []core.Person {
	if len(names) == 0 {
		return nil
	}
	people := make([]core.Person, 0, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		people = append(people, core.Person{
			Name: name,
			Type: personType,
		})
	}
	return people
}
