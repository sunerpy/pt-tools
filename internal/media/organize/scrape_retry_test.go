package organize

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

func TestScrapeRetryDue(t *testing.T) {
	created := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { v := created.Add(d); return &v }
	day := 24 * time.Hour
	cases := []struct {
		name    string
		scraped *time.Time
		now     time.Duration
		want    bool
	}{
		{"整理后不到一天", at(0), 23 * time.Hour, false},
		{"满一天", at(0), day, true},
		{"没刮削过、满一天", nil, day, true},
		{"一天的那次补过了", at(day + time.Hour), 2 * day, false},
		{"满三天", at(day + time.Hour), 3*day + time.Hour, true},
		{"满七天", at(3*day + time.Hour), 7 * day, true},
		{"满十四天", at(7*day + time.Hour), 14 * day, true},
		{"十四天的那次补过了", at(14*day + time.Hour), 20 * day, false},
		{"之后不再补", at(14*day + time.Hour), 60 * day, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, scrapeRetryDue(created, c.scraped, created.Add(c.now)))
		})
	}
}

// 部分刮削与刮削失败的记录按整理后 1、3、7、14 天自动补刮：不看「每周补一次缺的刮削」开关；一部排一次，
// 先补刮削得最早的；关着刮削的媒体库、刮削好了的、不到时候的、过了最后一次的不补。
func TestScrapeRetry(t *testing.T) {
	e := newEnv(t)
	tv := e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Scrape: true})
	off := e.library(LibraryInput{Name: "关着刮削", Kind: models.MediaKindMovie, Path: e.movies})
	require.False(t, tv.ScrapeFill)
	now := e.Now()
	ago := func(d time.Duration) *time.Time { v := now.Add(-d); return &v }
	day := 24 * time.Hour
	mk := func(lib uint, tmdbID int, status, src string, created time.Time, scraped *time.Time) models.MediaTransferHistory {
		r := models.MediaTransferHistory{
			LibraryID: lib, SourcePath: src, TargetPath: src + ".lib", Status: models.MediaTransferDone, MediaType: tmdb.KindTV, TMDBID: tmdbID,
			ScrapeStatus: status, CreatedAt: created, UpdatedAt: created, ScrapedAt: scraped,
		}
		require.NoError(t, e.db.Create(&r).Error)
		return r
	}
	e1 := mk(tv.ID, 1, models.MediaScrapePartial, "/e1", now.Add(-26*time.Hour), ago(26*time.Hour))
	e2 := mk(tv.ID, 1, models.MediaScrapePartial, "/e2", now.Add(-25*time.Hour), ago(25*time.Hour))
	other := mk(tv.ID, 2, models.MediaScrapeFailed, "/o", now.Add(-4*day), ago(2*day))
	mk(tv.ID, 3, models.MediaScrapeOK, "/ok", now.Add(-2*day), ago(2*day))
	fresh := mk(tv.ID, 4, models.MediaScrapePartial, "/new", now.Add(-time.Hour), ago(time.Hour))
	mk(tv.ID, 5, models.MediaScrapePartial, "/old", now.Add(-30*day), ago(15*day))
	mk(off.ID, 6, models.MediaScrapePartial, "/off", now.Add(-2*day), ago(2*day))

	e.svc.scrapeRetry(e.ctx)
	jobs := e.drainJobs()
	require.Len(t, jobs, 2)
	assert.Equal(t, other.ID, jobs[0].rescrape, "先补刮削得最早的")
	assert.Equal(t, e1.ID, jobs[1].rescrape, "一部只排一次（第 2 集的记录不再排）")
	assert.True(t, jobs[0].fill && jobs[1].fill, "只补没有的文件")

	// 补过了（刮削时间是现在）：到下一个时间点以前不再补
	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("id IN ?", []uint{e1.ID, e2.ID, other.ID}).Update("scraped_at", now).Error)
	e.svc.scrapeRetry(e.ctx)
	assert.Empty(t, e.drainJobs())
	e.advance(2 * day)
	e.svc.scrapeRetry(e.ctx)
	jobs = e.drainJobs()
	require.Len(t, jobs, 2, "第 1 部到了第三天，刚整理的那部到了第一天；失败的那部下一次在第七天")
	assert.ElementsMatch(t, []uint{e1.ID, fresh.ID}, []uint{jobs[0].rescrape, jobs[1].rescrape})

	// 队列满了：不排，下次再补
	for range jobQueueSize {
		e.svc.jobs <- job{rescrape: 9999}
	}
	e.svc.scrapeRetry(e.ctx)
	assert.Len(t, e.drainJobs(), jobQueueSize)
}

// 刚播出的一集整理入库时 TMDB 还没有标题、简介与截图：NFO 照样写（标题写「第 N 集」），记成部分刮削并写明缺什么；
// 第二天自动补刮时来源有了，补上截图、换掉 pt-tools 写的 NFO，记成刮削好了。
func TestScrapeRetryFillsLateEpisodeData(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Scrape: true, ScrapeItems: []string{"nfo", "thumb"}})
	e.tmdb.mu.Lock()
	e.tmdb.bareEpisodes = true
	e.tmdb.mu.Unlock()
	e.lastOfUs()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	season := filepath.Join(e.tv, "最后生还者 (2023)", "Season 1")
	rows := e.history()
	require.Len(t, rows, 2)
	for _, r := range rows {
		assert.Equal(t, models.MediaScrapePartial, r.ScrapeStatus, filepath.Base(r.TargetPath))
		assert.Contains(t, r.ScrapeNote, "集标题")
		assert.Contains(t, r.ScrapeNote, "集简介")
		assert.Contains(t, r.ScrapeNote, "集截图")
	}
	nfos, err := filepath.Glob(filepath.Join(season, "*S01E01*.nfo"))
	require.NoError(t, err)
	require.Len(t, nfos, 1)
	assert.Contains(t, readFile(t, nfos[0]), "<title>第 1 集</title>")

	// 第二天：TMDB 补上了，自动补刮
	e.tmdb.mu.Lock()
	e.tmdb.bareEpisodes = false
	e.tmdb.mu.Unlock()
	e.advance(25 * time.Hour)
	e.svc.scrapeRetry(e.ctx)
	jobs := e.drainJobs()
	require.Len(t, jobs, 1, "一部剧排一次")
	_, err = e.svc.handleRescrape(e.ctx, jobs[0])
	require.NoError(t, err)
	for _, r := range e.history() {
		assert.Equal(t, models.MediaScrapeOK, r.ScrapeStatus, r.ScrapeNote)
		assert.Empty(t, r.ScrapeNote)
	}
	nfo := readFile(t, nfos[0])
	assert.Contains(t, nfo, "<title>当你迷失在黑暗中</title>")
	assert.Contains(t, nfo, "<plot>乔尔遇到了艾莉。</plot>")
	thumbs, err := filepath.Glob(filepath.Join(season, "*S01E01*-thumb.jpg"))
	require.NoError(t, err)
	require.Len(t, thumbs, 1)
	_, err = os.Stat(thumbs[0])
	require.NoError(t, err)

	// 刮削好了：不再补
	e.advance(3 * 24 * time.Hour)
	e.svc.scrapeRetry(e.ctx)
	assert.Empty(t, e.drainJobs())
}
