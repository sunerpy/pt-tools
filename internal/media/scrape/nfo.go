// Package scrape 为整理进媒体库的电影与剧集写 Kodi 格式的 NFO（Emby、Jellyfin、Plex 的 NFO 插件都认），
// 并下载海报、背景、Logo 等图片。写什么由媒体库的刮削内容决定（见 Items），数据来自 metadata：
// 以 TMDB 为基础，媒体库的数据来源里选了别的来源的那几项用那个来源的（见 metadata.Sources）。
package scrape

import (
	"encoding/xml"
	"fmt"
	"strconv"

	"github.com/sunerpy/pt-tools/internal/media/metadata"
)

type uniqueID struct {
	Type    string `xml:"type,attr"`
	Default bool   `xml:"default,attr,omitempty"`
	Value   string `xml:",chardata"`
}

type rating struct {
	Name    string `xml:"name,attr"`
	Max     int    `xml:"max,attr"`
	Default bool   `xml:"default,attr"`
	Value   string `xml:"value"`
	Votes   int    `xml:"votes,omitempty"`
}

type ratings struct {
	Rating []rating `xml:"rating"`
}

type actor struct {
	Name  string `xml:"name"`
	Role  string `xml:"role,omitempty"`
	Order int    `xml:"order"`
	Thumb string `xml:"thumb,omitempty"`
}

// set 是电影所属的系列：Jellyfin 按 tmdbcolid 认 TMDB 的系列，Kodi 读 <name>。
type set struct {
	TMDBColID string `xml:"tmdbcolid,attr,omitempty"`
	Name      string `xml:"name"`
}

// common 是电影与剧集共有的字段（顺序就是写出的顺序）。
type common struct {
	Title         string     `xml:"title"`
	OriginalTitle string     `xml:"originaltitle,omitempty"`
	Year          int        `xml:"year,omitempty"`
	Premiered     string     `xml:"premiered,omitempty"`
	Plot          string     `xml:"plot,omitempty"`
	Tagline       string     `xml:"tagline,omitempty"`
	Runtime       int        `xml:"runtime,omitempty"`
	MPAA          string     `xml:"mpaa,omitempty"`
	Status        string     `xml:"status,omitempty"`
	Ratings       *ratings   `xml:"ratings,omitempty"`
	UniqueIDs     []uniqueID `xml:"uniqueid"`
	TMDBID        string     `xml:"tmdbid,omitempty"`
	IMDbID        string     `xml:"imdbid,omitempty"`
	Genres        []string   `xml:"genre"`
	Studios       []string   `xml:"studio"`
	Set           *set       `xml:"set,omitempty"`
	Directors     []string   `xml:"director"`
	Credits       []string   `xml:"credits"`
	Actors        []actor    `xml:"actor"`
}

// idOrder 是写 <uniqueid> 的顺序；TMDB 的是默认的。
var idOrder = []string{metadata.IDTMDB, metadata.IDIMDb, metadata.IDTVDB, metadata.IDDouban, metadata.IDBangumi}

func uniqueIDs(ids map[string]string) []uniqueID {
	var out []uniqueID
	for _, k := range idOrder {
		if v := ids[k]; v != "" {
			out = append(out, uniqueID{Type: k, Default: k == metadata.IDTMDB, Value: v})
		}
	}
	return out
}

// ratingsOf 是 <ratings>：metadata 里主要的评分排在最后，写成 default。Kodi 看 default，Jellyfin 与 Emby 按顺序读
// 每个 <rating>、最后一个生效（不看 default）。
func ratingsOf(rs []metadata.Rating) *ratings {
	if len(rs) == 0 {
		return nil
	}
	out := &ratings{}
	for i, r := range rs {
		prec := 1
		if r.Max > 10 {
			prec = 0
		}
		out.Rating = append(out.Rating, rating{
			Name: r.Name, Max: r.Max, Default: i == len(rs)-1, Value: strconv.FormatFloat(r.Value, 'f', prec, 64), Votes: r.Votes,
		})
	}
	return out
}

func commonOf(it *metadata.Item) common {
	c := common{
		Title: it.Title, OriginalTitle: it.OriginalTitle, Year: it.Year, Premiered: it.Premiered, Plot: it.Plot,
		Tagline: it.Tagline, Runtime: it.Runtime, MPAA: it.Certification, Status: it.Status, Ratings: ratingsOf(it.Ratings),
		UniqueIDs: uniqueIDs(it.IDs), TMDBID: it.ID(metadata.IDTMDB), IMDbID: it.ID(metadata.IDIMDb),
		Genres: it.Genres, Studios: it.Studios,
	}
	if c.OriginalTitle == c.Title {
		c.OriginalTitle = ""
	}
	for _, p := range it.Cast {
		c.Actors = append(c.Actors, actor{Name: p.Name, Role: p.Role, Order: p.Order, Thumb: p.Thumb})
	}
	return c
}

type movieNFO struct {
	XMLName xml.Name `xml:"movie"`
	common
}

type tvshowNFO struct {
	XMLName xml.Name `xml:"tvshow"`
	common
}

type seasonNFO struct {
	XMLName      xml.Name   `xml:"season"`
	Title        string     `xml:"title,omitempty"`
	SeasonNumber int        `xml:"seasonnumber"`
	Plot         string     `xml:"plot,omitempty"`
	Premiered    string     `xml:"premiered,omitempty"`
	UniqueIDs    []uniqueID `xml:"uniqueid"`
}

type episodeNFO struct {
	XMLName   xml.Name   `xml:"episodedetails"`
	Title     string     `xml:"title"`
	ShowTitle string     `xml:"showtitle,omitempty"`
	Season    int        `xml:"season"`
	Episode   int        `xml:"episode"`
	Plot      string     `xml:"plot,omitempty"`
	Aired     string     `xml:"aired,omitempty"`
	Runtime   int        `xml:"runtime,omitempty"`
	Ratings   *ratings   `xml:"ratings,omitempty"`
	UniqueIDs []uniqueID `xml:"uniqueid"`
	Directors []string   `xml:"director"`
	Credits   []string   `xml:"credits"`
}

func movieDoc(it *metadata.Item) []any {
	c := commonOf(it)
	c.Status = ""
	c.Directors, c.Credits = it.Directors, it.Writers
	if col := it.Collection; col != nil && col.Name != "" {
		c.Set = &set{Name: col.Name}
		if col.TMDBID > 0 {
			c.Set.TMDBColID = strconv.Itoa(col.TMDBID)
		}
	}
	return []any{movieNFO{common: c}}
}

func showDoc(it *metadata.Item) []any {
	// 剧集的导演与编剧写在每一集里
	return []any{tvshowNFO{common: commonOf(it)}}
}

func seasonDoc(s *metadata.Season) []any {
	return []any{seasonNFO{SeasonNumber: s.Number, Title: s.Title, Plot: s.Plot, Premiered: s.Premiered, UniqueIDs: uniqueIDs(s.IDs)}}
}

// episodeDoc 是一个剧集文件的 NFO：一个文件里有多集（episode 到 end）时每集写一段，Kodi 按多集文件处理。
// 季里没有的集只写季号与集号，标题用「第 N 集」。
func episodeDoc(show *metadata.Item, s *metadata.Season, season, episode, end int) []any {
	if end < episode {
		end = episode
	}
	var out []any
	for n := episode; n <= end; n++ {
		e := episodeNFO{Title: fmt.Sprintf("第 %d 集", n), ShowTitle: show.Title, Season: season, Episode: n}
		if ep := s.Episode(n); ep != nil {
			if ep.Title != "" {
				e.Title = ep.Title
			}
			e.Plot, e.Aired, e.Runtime = ep.Plot, ep.Aired, ep.Runtime
			e.Ratings, e.UniqueIDs = ratingsOf(ep.Ratings), uniqueIDs(ep.IDs)
			e.Directors, e.Credits = ep.Directors, ep.Writers
		}
		out = append(out, e)
	}
	return out
}

// MovieNFO 是电影的 NFO（<movie>）。
func MovieNFO(it *metadata.Item) ([]byte, error) { return build(movieDoc(it), nil) }

// TVShowNFO 是剧集的 NFO（<tvshow>，放在剧集目录里，文件名 tvshow.nfo）。
func TVShowNFO(it *metadata.Item) ([]byte, error) { return build(showDoc(it), nil) }

// SeasonNFO 是一季的 NFO（<season>，放在季目录里，文件名 season.nfo）。
func SeasonNFO(s *metadata.Season) ([]byte, error) { return build(seasonDoc(s), nil) }

// EpisodeNFO 是一个剧集文件的 NFO（<episodedetails>，多集的文件每集一段）。
func EpisodeNFO(show *metadata.Item, s *metadata.Season, season, episode, end int) ([]byte, error) {
	return build(episodeDoc(show, s, season, episode, end), nil)
}
