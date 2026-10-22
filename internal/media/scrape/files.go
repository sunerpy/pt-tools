package scrape

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sunerpy/pt-tools/internal/media/metadata"
)

// NFO 的种类。
const (
	NFOMovie   = "movie"
	NFOShow    = "tvshow"
	NFOSeason  = "season"
	NFOEpisode = "episode"
)

// File 是刮削要写的一个文件。文件名只在这里定（见 MovieFiles 等），删除整理记录时按 IsDirArtifact 认目录里共用的文件。
type File struct {
	Path string
	// NFO 是 NFO 的种类，图片为空
	NFO string
	// Art 是图片的种类（metadata.ArtPoster 一类），NFO 为空
	Art string
	// Season 是季号（季 NFO、季海报、每一集的 NFO 与截图）；Episode 到 EpisodeEnd 是集号（一个文件里有多集时
	// EpisodeEnd 是最后一集，截图用第一集的）
	Season, Episode, EpisodeEnd int
}

// art 是一种放在目录里的图片：控制它的刮削内容与文件名（Kodi 与 Jellyfin 认的写法）。
type art struct {
	item, kind, name string
	// movieOnly 表示只有电影有（碟片图）
	movieOnly bool
}

// dirArt 是电影与剧集目录里的图片，按界面上的顺序。放在库目录根下的电影前面加上「<文件名>-」。
var dirArt = []art{
	{ItemPoster, metadata.ArtPoster, "poster.jpg", false},
	{ItemFanart, metadata.ArtFanart, "fanart.jpg", false},
	{ItemLogo, metadata.ArtLogo, "clearlogo.png", false},
	{ItemClearart, metadata.ArtClearart, "clearart.png", false},
	{ItemBanner, metadata.ArtBanner, "banner.jpg", false},
	{ItemLandscape, metadata.ArtLandscape, "landscape.jpg", false},
	{ItemDisc, metadata.ArtDisc, "discart.png", true},
}

func stem(videoPath string) string {
	return strings.TrimSuffix(videoPath, filepath.Ext(videoPath))
}

// on 报告这一项选了没有。
func (it Items) on(item string) bool {
	p := it.field(item)
	return p != nil && *p
}

// MovieFiles 是一个电影文件要写的文件：<文件名>.nfo；电影有自己的目录（ownDir）时图片放在目录里（poster.jpg 等），
// 平铺在库目录（或分类目录）里时带上文件名（<文件名>-poster.jpg 等），免得几部电影共用一张。
func MovieFiles(video string, ownDir bool, it Items) []File {
	var out []File
	if it.NFO {
		out = append(out, File{Path: stem(video) + ".nfo", NFO: NFOMovie})
	}
	for _, a := range dirArt {
		if !it.on(a.item) {
			continue
		}
		p := stem(video) + "-" + a.name
		if ownDir {
			p = filepath.Join(filepath.Dir(video), a.name)
		}
		out = append(out, File{Path: p, Art: a.kind})
	}
	return out
}

// ShowFiles 是剧集目录里的文件：tvshow.nfo 与图片（碟片图只有电影有）。
func ShowFiles(showDir string, it Items) []File {
	var out []File
	if it.NFO {
		out = append(out, File{Path: filepath.Join(showDir, "tvshow.nfo"), NFO: NFOShow})
	}
	for _, a := range dirArt {
		if it.on(a.item) && !a.movieOnly {
			out = append(out, File{Path: filepath.Join(showDir, a.name), Art: a.kind})
		}
	}
	return out
}

// SeasonPosterName 是季海报放在剧集目录里的文件名：season01-poster.jpg，第 0 季是 season-specials-poster.jpg。
func SeasonPosterName(n int) string {
	if n == 0 {
		return "season-specials-poster.jpg"
	}
	return fmt.Sprintf("season%02d-poster.jpg", n)
}

// SeasonFiles 是一季的文件：季目录（seasonDir）不是剧集目录时季目录里的 season.nfo，剧集目录里的季海报。
func SeasonFiles(showDir, seasonDir string, n int, it Items) []File {
	var out []File
	if it.NFO && seasonDir != "" && filepath.Clean(seasonDir) != filepath.Clean(showDir) {
		out = append(out, File{Path: filepath.Join(seasonDir, "season.nfo"), NFO: NFOSeason, Season: n})
	}
	if it.SeasonPoster {
		out = append(out, File{Path: filepath.Join(showDir, SeasonPosterName(n)), Art: metadata.ArtSeasonPoster, Season: n})
	}
	return out
}

// EpisodeFiles 是一个剧集文件的 <文件名>.nfo 与 <文件名>-thumb.jpg（第一集的截图）。
func EpisodeFiles(video string, season, episode, end int, it Items) []File {
	if end < episode {
		end = episode
	}
	var out []File
	if it.NFO {
		out = append(out, File{Path: stem(video) + ".nfo", NFO: NFOEpisode, Season: season, Episode: episode, EpisodeEnd: end})
	}
	if it.Thumb {
		out = append(out, File{Path: stem(video) + "-thumb.jpg", Art: metadata.ArtThumb, Season: season, Episode: episode, EpisodeEnd: end})
	}
	return out
}

var seasonPosterRe = regexp.MustCompile(`^season(\d+|-specials)-poster\.jpg$`)

// IsDirArtifact 报告文件名是不是刮削写在目录里、几个视频共用的文件（电影与剧集目录里的图片、tvshow.nfo、season.nfo、季海报）。
func IsDirArtifact(name string) bool {
	switch name {
	case "tvshow.nfo", "season.nfo":
		return true
	}
	for _, a := range dirArt {
		if name == a.name {
			return true
		}
	}
	return seasonPosterRe.MatchString(name)
}
