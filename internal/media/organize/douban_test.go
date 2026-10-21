package organize

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/douban"
	"github.com/sunerpy/pt-tools/internal/media/scrape"
	"github.com/sunerpy/pt-tools/models"
)

// fakeDouban 是假的豆瓣：按名字与季回条目，记下找过什么。
type fakeDouban struct {
	mu       sync.Mutex
	finds    []douban.Query
	subjects []string
	err      error
}

var (
	doubanOpp  = douban.Subject{ID: "35593344", Title: "奥本海默", Year: 2023, Intro: "豆瓣：原子弹之父。", Rating: 8.8, Votes: 850981}
	doubanTLOU = douban.Subject{ID: "25848328", Title: "最后生还者 第一季", Year: 2023, Intro: "豆瓣：第一季的简介。", Rating: 9.0, Votes: 211009}
)

func (f *fakeDouban) Find(_ context.Context, q douban.Query) (*douban.Subject, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finds = append(f.finds, q)
	if f.err != nil {
		return nil, f.err
	}
	switch {
	case q.Kind == douban.KindMovie && q.Title == "奥本海默" && q.Year == 2023:
		s := doubanOpp
		return &s, nil
	case q.Kind == douban.KindTV && q.Title == "最后生还者" && q.Season <= 1:
		s := doubanTLOU
		return &s, nil
	}
	return nil, nil
}

func (f *fakeDouban) Subject(_ context.Context, _, id string) (*douban.Subject, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subjects = append(f.subjects, id)
	if f.err != nil {
		return nil, f.err
	}
	if id == doubanOpp.ID {
		s := doubanOpp
		return &s, nil
	}
	return &douban.Subject{ID: id, Title: "别的电影", Year: 2023, Intro: "别的电影"}, nil
}

func doubanEnv(t *testing.T) (*env, *fakeDouban) {
	t.Helper()
	e := newEnv(t)
	f := &fakeDouban{}
	e.svc.cfg.Douban = f
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Scrape: true, ScrapeSource: scrape.SourceDouban})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Scrape: true, ScrapeSource: scrape.SourceDouban})
	e.settings(SettingsInput{MinVideoMB: 1})
	return e, f
}

// 媒体库选了豆瓣：电影的 NFO 用豆瓣的简介与评分（TMDB 的评分留着），记下豆瓣编号
func TestScrapeDoubanMovie(t *testing.T) {
	e, f := doubanEnv(t)
	e.oppenheimer()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)
	nfo := readFile(t, filepath.Join(e.movies, "奥本海默 (2023)", "奥本海默 (2023) - 2160p BluRay HDR H.265.nfo"))
	assert.Contains(t, nfo, "<plot>豆瓣：原子弹之父。</plot>")
	assert.Contains(t, nfo, `<rating name="douban" max="10" default="true">`)
	assert.Contains(t, nfo, `<rating name="themoviedb" max="10" default="false">`)
	assert.Contains(t, nfo, `<uniqueid type="douban">35593344</uniqueid>`)
	require.Len(t, f.finds, 1)
	assert.Equal(t, douban.Query{Kind: douban.KindMovie, Title: "奥本海默", OriginalTitle: "Oppenheimer", Year: 2023}, f.finds[0])
	assert.NotContains(t, e.history()[0].Message, "豆瓣")
}

// 站点给了豆瓣编号：按编号取，不搜；编号对应的是别的电影（名字对不上）时还是按名字找
func TestScrapeDoubanSiteID(t *testing.T) {
	for _, c := range []struct {
		id       string
		subjects []string
		finds    int
	}{{"35593344", []string{"35593344"}, 0}, {"1", []string{"1"}, 1}} {
		e, f := doubanEnv(t)
		e.oppenheimer()
		hash := oppHash
		require.NoError(t, e.db.Create(&models.TorrentInfo{SiteName: "s", TorrentID: "1", TorrentHash: &hash, DoubanID: c.id}).Error)
		_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
		require.NoError(t, err)
		assert.Equal(t, c.subjects, f.subjects)
		assert.Len(t, f.finds, c.finds)
		nfo := readFile(t, filepath.Join(e.movies, "奥本海默 (2023)", "奥本海默 (2023) - 2160p BluRay HDR H.265.nfo"))
		assert.Contains(t, nfo, "<plot>豆瓣：原子弹之父。</plot>")
	}
}

// 豆瓣不能访问或找不到：NFO 用 TMDB 的，整理记录里写明
func TestScrapeDoubanFallback(t *testing.T) {
	e, f := doubanEnv(t)
	f.err = errors.New("豆瓣拒绝了请求（HTTP 403）")
	e.oppenheimer()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	nfo := readFile(t, filepath.Join(e.movies, "奥本海默 (2023)", "奥本海默 (2023) - 2160p BluRay HDR H.265.nfo"))
	assert.Contains(t, nfo, "<plot>原子弹之父的故事。</plot>")
	assert.NotContains(t, nfo, "douban")
	assert.Contains(t, strings.Join(res.Messages, "\n"), "豆瓣取不到")

	// 剧集：第 1 季找到了（写进 tvshow.nfo 与 season.nfo）
	f.err = nil
	e.lastOfUs()
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	show := filepath.Join(e.tv, "最后生还者 (2023)")
	assert.Contains(t, readFile(t, filepath.Join(show, "tvshow.nfo")), "<plot>豆瓣：第一季的简介。</plot>")
	assert.Contains(t, readFile(t, filepath.Join(show, "Season 1", "season.nfo")), "<plot>豆瓣：第一季的简介。</plot>")
	assert.NotContains(t, readFile(t, filepath.Join(show, "Season 1", "最后生还者 - S01E01 - 当你迷失在黑暗中.nfo")), "豆瓣", "每一集的 NFO 还是 TMDB 的")
}

// 已经有、又不覆盖的 NFO 用不上豆瓣的内容：不去找；开着覆盖时照样找
func TestScrapeDoubanOnlyWhenNeeded(t *testing.T) {
	e, f := doubanEnv(t)
	show := filepath.Join(e.tv, "最后生还者 (2023)")
	require.NoError(t, os.MkdirAll(filepath.Join(show, "Season 1"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(show, "tvshow.nfo"), []byte("<tvshow/>"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(show, "Season 1", "season.nfo"), []byte("<season/>"), 0o644))
	e.lastOfUs()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	require.Equal(t, 2, res.Created)
	assert.Empty(t, f.finds, "NFO 都有了、不覆盖：不找")
	assert.Equal(t, "<tvshow/>", readFile(t, filepath.Join(show, "tvshow.nfo")))

	// 电影：NFO 已经有了、不覆盖时也不找
	movie := filepath.Join(e.movies, "奥本海默 (2023)", "奥本海默 (2023) - 2160p BluRay HDR H.265.nfo")
	require.NoError(t, os.MkdirAll(filepath.Dir(movie), 0o755))
	require.NoError(t, os.WriteFile(movie, []byte("<movie/>"), 0o644))
	e.oppenheimer()
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Empty(t, f.finds)
}

// 媒体库的「简介与评分」只认留空与 douban
func TestLibraryScrapeSource(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.SaveLibrary(e.ctx, 0, LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, ScrapeSource: "imdb"})
	require.ErrorIs(t, err, ErrInvalid)
	v := e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, ScrapeSource: " douban "})
	assert.Equal(t, scrape.SourceDouban, v.ScrapeSource)
	v2, err := e.svc.SaveLibrary(e.ctx, v.ID, LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Enabled: true})
	require.NoError(t, err)
	assert.Empty(t, v2.ScrapeSource)
}
