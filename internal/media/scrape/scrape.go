package scrape

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
)

// Images 下载 TMDB 图片（生产环境是 *tmdb.Client）。
type Images interface {
	Image(ctx context.Context, path, size string) ([]byte, error)
}

// Writer 写 NFO 与图片。Overwrite 为假时已经存在的文件不动。
type Writer struct {
	Images    Images
	Overwrite bool
}

// Report 是一次刮削写了哪些文件、哪些失败了。
type Report struct {
	Written []string
	Errors  []string
}

func (r *Report) add(path string, err error) {
	switch {
	case err == nil:
		r.Written = append(r.Written, path)
	case errors.Is(err, errExists):
	default:
		r.Errors = append(r.Errors, fmt.Sprintf("%s: %v", filepath.Base(path), err))
	}
}

// Merge 把另一次的结果并进来。
func (r *Report) Merge(o Report) {
	r.Written = append(r.Written, o.Written...)
	r.Errors = append(r.Errors, o.Errors...)
}

var errExists = errors.New("已存在")

// write 原子地写一个文件：先写同目录的临时文件再改名。不覆盖时已经存在的文件返回 errExists。
func (w Writer) write(path string, data []byte) error {
	if !w.Overwrite {
		if _, err := os.Lstat(path); err == nil {
			return errExists
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var rnd [6]byte
	_, _ = rand.Read(rnd[:])
	tmp := path + ".pt-tools-" + hex.EncodeToString(rnd[:]) + ".part"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (w Writer) nfo(path string, data []byte, err error, rep *Report) {
	if err != nil {
		rep.add(path, err)
		return
	}
	rep.add(path, w.write(path, data))
}

func (w Writer) image(ctx context.Context, path, tmdbPath, size string, rep *Report) {
	if tmdbPath == "" || w.Images == nil {
		return
	}
	if !w.Overwrite {
		if _, err := os.Lstat(path); err == nil {
			return
		}
	}
	data, err := w.Images.Image(ctx, tmdbPath, size)
	if err != nil {
		rep.add(path, err)
		return
	}
	rep.add(path, w.write(path, data))
}

func stem(videoPath string) string {
	return strings.TrimSuffix(videoPath, filepath.Ext(videoPath))
}

// Movie 为一个电影文件写 <文件名>.nfo；电影有自己的目录（ownDir）时再写 poster.jpg 与 fanart.jpg，
// 放在库目录根下时写成 <文件名>-poster.jpg 与 <文件名>-fanart.jpg。
func (w Writer) Movie(ctx context.Context, videoPath string, ownDir bool, r tmdb.Result) Report {
	var rep Report
	data, err := MovieNFO(r)
	w.nfo(stem(videoPath)+".nfo", data, err, &rep)
	poster, fanart := stem(videoPath)+"-poster.jpg", stem(videoPath)+"-fanart.jpg"
	if ownDir {
		dir := filepath.Dir(videoPath)
		poster, fanart = filepath.Join(dir, "poster.jpg"), filepath.Join(dir, "fanart.jpg")
	}
	w.image(ctx, poster, r.PosterPath, tmdb.SizePoster, &rep)
	w.image(ctx, fanart, r.BackdropPath, tmdb.SizeBackdrop, &rep)
	return rep
}

// Show 在剧集目录里写 tvshow.nfo、poster.jpg 与 fanart.jpg。
func (w Writer) Show(ctx context.Context, showDir string, r tmdb.Result) Report {
	var rep Report
	data, err := TVShowNFO(r)
	w.nfo(filepath.Join(showDir, "tvshow.nfo"), data, err, &rep)
	w.image(ctx, filepath.Join(showDir, "poster.jpg"), r.PosterPath, tmdb.SizePoster, &rep)
	w.image(ctx, filepath.Join(showDir, "fanart.jpg"), r.BackdropPath, tmdb.SizeBackdrop, &rep)
	return rep
}

// SeasonPosterName 是季海报放在剧集目录里的文件名：season01-poster.jpg，第 0 季是 season-specials-poster.jpg。
func SeasonPosterName(n int) string {
	if n == 0 {
		return "season-specials-poster.jpg"
	}
	return fmt.Sprintf("season%02d-poster.jpg", n)
}

// Season 在剧集目录里写季海报；季目录（seasonDir）不是剧集目录时在季目录里写 season.nfo。
func (w Writer) Season(ctx context.Context, showDir, seasonDir string, number int, s *tmdb.Season) Report {
	var rep Report
	if seasonDir != "" && filepath.Clean(seasonDir) != filepath.Clean(showDir) {
		data, err := SeasonNFO(number, s)
		w.nfo(filepath.Join(seasonDir, "season.nfo"), data, err, &rep)
	}
	if s != nil {
		w.image(ctx, filepath.Join(showDir, SeasonPosterName(number)), s.PosterPath, tmdb.SizePoster, &rep)
	}
	return rep
}

// Episode 为一个剧集文件写 <文件名>.nfo 与 <文件名>-thumb.jpg（第一集的截图）。
func (w Writer) Episode(ctx context.Context, videoPath string, show tmdb.Result, season, episode, end int, s *tmdb.Season) Report {
	var rep Report
	data, err := EpisodeNFO(show, season, episode, end, s)
	w.nfo(stem(videoPath)+".nfo", data, err, &rep)
	if ep := s.Episode(episode); ep != nil {
		w.image(ctx, stem(videoPath)+"-thumb.jpg", ep.StillPath, tmdb.SizeStill, &rep)
	}
	return rep
}
