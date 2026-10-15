package organize

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/douban"
	"github.com/sunerpy/pt-tools/internal/media/scrape"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
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

// 站点给了豆瓣编号：按编号取，不搜；编号对应的年份对不上时还是按名字找
func TestScrapeDoubanSiteID(t *testing.T) {
	e, f := doubanEnv(t)
	e.oppenheimer()
	hash := oppHash
	require.NoError(t, e.db.Create(&models.TorrentInfo{SiteName: "s", TorrentID: "1", TorrentHash: &hash, DoubanID: "35593344"}).Error)
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, []string{"35593344"}, f.subjects)
	assert.Empty(t, f.finds)

	opp := &tmdb.Result{ID: 872585, MediaType: tmdb.KindMovie, Title: "奥本海默", OriginalTitle: "Oppenheimer", Year: 2023}
	got, err := e.svc.doubanSubject(e.ctx, &Plan{Match: opp, doubanID: "1"}, 0, 2023)
	require.NoError(t, err)
	assert.Equal(t, doubanOpp.ID, got.ID, "编号是同一年的别的电影（名字对不上）：按名字找")
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

// 剧集按季找（第 2 季用这一季的年份），没找到的季只写说明，第 0 季不找；已经有、又不覆盖的 NFO 用不上，不找
func TestDoubanInfoSeasons(t *testing.T) {
	e, f := doubanEnv(t)
	show := filepath.Join(e.tv, "最后生还者 (2023)")
	p := &Plan{Library: &models.MediaLibrary{Path: e.tv}, Match: &tmdb.Result{ID: 100088, MediaType: tmdb.KindTV, Title: "最后生还者", OriginalTitle: "The Last of Us", Year: 2023}}
	item := func(season int) *PlanItem {
		return &PlanItem{Season: season, Target: filepath.Join(show, fmt.Sprintf("Season %d", season), "x.mkv"), stillFor: &tmdb.Season{AirDate: "2025-04-13"}}
	}
	got, seasons, msgs := e.svc.doubanInfo(e.ctx, p, []*PlanItem{item(1), item(2), item(0), item(2)})
	require.NotNil(t, got)
	assert.Equal(t, doubanTLOU.ID, got.ID)
	assert.Equal(t, got, seasons[1])
	assert.Nil(t, seasons[2])
	_, has0 := seasons[0]
	assert.False(t, has0)
	assert.Equal(t, []string{"刮削：豆瓣上没找到第 2 季（按名字与年份），简介与评分用 TMDB 的"}, msgs)
	require.Len(t, f.finds, 2, "第 1 季、第 2 季各找一次")
	assert.Equal(t, 2025, f.finds[1].Year, "第 2 季用这一季的年份")

	require.NoError(t, os.MkdirAll(filepath.Join(show, "Season 1"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(show, "tvshow.nfo"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(show, "Season 1", "season.nfo"), nil, 0o644))
	f.finds = nil
	got, _, _ = e.svc.doubanInfo(e.ctx, p, []*PlanItem{item(1)})
	assert.Nil(t, got)
	assert.Empty(t, f.finds, "NFO 都有了、不覆盖：不找")
	p.Library.ScrapeOverwrite = true
	got, _, _ = e.svc.doubanInfo(e.ctx, p, []*PlanItem{item(1)})
	assert.NotNil(t, got)
	assert.Len(t, f.finds, 1, "开着覆盖：照样找")

	// 电影：NFO 已经有了、不覆盖时也不找
	movie := filepath.Join(e.movies, "奥本海默 (2023)", "奥本海默 (2023).mkv")
	require.NoError(t, os.MkdirAll(filepath.Dir(movie), 0o755))
	require.NoError(t, os.WriteFile(strings.TrimSuffix(movie, ".mkv")+".nfo", nil, 0o644))
	f.finds = nil
	mp := &Plan{Library: &models.MediaLibrary{Path: e.movies}, Match: &tmdb.Result{ID: 872585, MediaType: tmdb.KindMovie, Title: "奥本海默", Year: 2023}}
	got, _, _ = e.svc.doubanInfo(e.ctx, mp, []*PlanItem{{Target: movie}})
	assert.Nil(t, got)
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
