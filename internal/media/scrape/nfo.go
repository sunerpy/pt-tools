// Package scrape 为整理进媒体库的电影与剧集写 Kodi 格式的 NFO（Emby、Jellyfin、Plex 的 NFO 插件都认），
// 并下载海报、背景、季海报与集截图。数据来自 TMDB。
package scrape

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
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
}

type ratings struct {
	Rating []rating `xml:"rating"`
}

// common 是电影与剧集共有的字段（顺序就是写出的顺序）。
type common struct {
	Title         string     `xml:"title"`
	OriginalTitle string     `xml:"originaltitle,omitempty"`
	Year          int        `xml:"year,omitempty"`
	Premiered     string     `xml:"premiered,omitempty"`
	Plot          string     `xml:"plot,omitempty"`
	Runtime       int        `xml:"runtime,omitempty"`
	Ratings       *ratings   `xml:"ratings,omitempty"`
	UniqueIDs     []uniqueID `xml:"uniqueid"`
	TMDBID        string     `xml:"tmdbid,omitempty"`
	IMDbID        string     `xml:"imdbid,omitempty"`
	Genres        []string   `xml:"genre"`
}

func commonOf(r tmdb.Result) common {
	c := common{
		Title: r.Title, OriginalTitle: r.OriginalTitle, Year: r.Year, Premiered: r.Date, Plot: r.Overview,
		Runtime: r.Runtime, Genres: r.Genres,
	}
	if c.OriginalTitle == c.Title {
		c.OriginalTitle = ""
	}
	if r.VoteAverage > 0 {
		c.Ratings = &ratings{Rating: []rating{{
			Name: "themoviedb", Max: 10, Default: true, Value: strconv.FormatFloat(r.VoteAverage, 'f', 1, 64),
		}}}
	}
	if r.ID > 0 {
		c.UniqueIDs = append(c.UniqueIDs, uniqueID{Type: "tmdb", Default: true, Value: strconv.Itoa(r.ID)})
		c.TMDBID = strconv.Itoa(r.ID)
	}
	if r.IMDbID != "" {
		c.UniqueIDs = append(c.UniqueIDs, uniqueID{Type: "imdb", Value: r.IMDbID})
		c.IMDbID = r.IMDbID
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
	XMLName      xml.Name `xml:"season"`
	Title        string   `xml:"title,omitempty"`
	SeasonNumber int      `xml:"seasonnumber"`
	Plot         string   `xml:"plot,omitempty"`
	Premiered    string   `xml:"premiered,omitempty"`
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
	UniqueIDs []uniqueID `xml:"uniqueid"`
}

const header = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

func marshal(vs ...any) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(header)
	for _, v := range vs {
		out, err := xml.MarshalIndent(v, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("生成 NFO 失败: %w", err)
		}
		b.Write(out)
		b.WriteByte('\n')
	}
	return b.Bytes(), nil
}

// MovieNFO 是电影的 NFO（<movie>）。
func MovieNFO(r tmdb.Result) ([]byte, error) {
	return marshal(movieNFO{common: commonOf(r)})
}

// TVShowNFO 是剧集的 NFO（<tvshow>，放在剧集目录里，文件名 tvshow.nfo）。
func TVShowNFO(r tmdb.Result) ([]byte, error) {
	return marshal(tvshowNFO{common: commonOf(r)})
}

// SeasonNFO 是一季的 NFO（<season>，放在季目录里，文件名 season.nfo）。s 为空时只写季号。
func SeasonNFO(number int, s *tmdb.Season) ([]byte, error) {
	n := seasonNFO{SeasonNumber: number}
	if s != nil {
		n.Title, n.Plot, n.Premiered = s.Name, s.Overview, s.AirDate
	}
	return marshal(n)
}

// EpisodeNFO 是一个剧集文件的 NFO（<episodedetails>）。一个文件里有多集（episode 到 end）时每集写一段，
// Kodi 按多集文件处理。季详情里没有的集只写季号与集号，标题用「第 N 集」。
func EpisodeNFO(show tmdb.Result, season, episode, end int, s *tmdb.Season) ([]byte, error) {
	if end < episode {
		end = episode
	}
	var parts []any
	for n := episode; n <= end; n++ {
		e := episodeNFO{Title: fmt.Sprintf("第 %d 集", n), ShowTitle: show.Title, Season: season, Episode: n}
		if ep := s.Episode(n); ep != nil {
			if ep.Name != "" {
				e.Title = ep.Name
			}
			e.Plot, e.Aired, e.Runtime = ep.Overview, ep.AirDate, ep.Runtime
			if ep.ID > 0 {
				e.UniqueIDs = []uniqueID{{Type: "tmdb", Default: true, Value: strconv.Itoa(ep.ID)}}
			}
		}
		parts = append(parts, e)
	}
	return marshal(parts...)
}
