package organize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func TestHistoryListRetryDelete(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	e.lastOfUs()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	e.tmdb.setDown(true)
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	e.tmdb.setDown(false)

	page, err := e.svc.History(e.ctx, HistoryQuery{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), page.Total)
	page, err = e.svc.History(e.ctx, HistoryQuery{Status: models.MediaTransferFailed})
	require.NoError(t, err)
	require.Equal(t, int64(2), page.Total)
	failedID := page.Items[0].ID
	page, err = e.svc.History(e.ctx, HistoryQuery{Keyword: "奥本海默"})
	require.NoError(t, err)
	require.Equal(t, int64(1), page.Total)
	assert.Equal(t, "电影", page.Items[0].LibraryName)
	assert.Equal(t, 1, page.Items[0].Subtitles)
	page, err = e.svc.History(e.ctx, HistoryQuery{Keyword: "100%_"})
	require.NoError(t, err)
	assert.Zero(t, page.Total, "% 与 _ 按字面匹配")
	page, err = e.svc.History(e.ctx, HistoryQuery{Limit: 1, Offset: 1})
	require.NoError(t, err)
	assert.Len(t, page.Items, 1)
	_, err = e.svc.History(e.ctx, HistoryQuery{Status: "bogus"})
	require.ErrorIs(t, err, ErrInvalid)

	res, err := e.svc.Retry(e.ctx, failedID)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Created, "重试整理整个种子")
	_, err = e.svc.Retry(e.ctx, 999)
	require.ErrorIs(t, err, ErrNotFound)

	// 删除记录并删除库里的文件：视频、字幕、NFO、目录里的图片与空目录
	movie := e.history()[0]
	require.Equal(t, models.MediaTransferDone, movie.Status)
	require.NoError(t, e.svc.DeleteHistory(e.ctx, movie.ID, true))
	assert.False(t, exists(movie.TargetPath))
	assert.False(t, exists(filepath.Dir(movie.TargetPath)), "电影目录里没别的视频了，连目录一起删")
	assert.True(t, exists(movie.SourcePath), "下载目录里的源文件不动")
	assert.True(t, exists(e.movies))
	require.ErrorIs(t, e.svc.DeleteHistory(e.ctx, movie.ID, true), ErrNotFound)

	// 只删记录
	var ep models.MediaTransferHistory
	require.NoError(t, e.db.Where("episode = ?", 1).First(&ep).Error)
	require.NoError(t, e.svc.DeleteHistory(e.ctx, ep.ID, false))
	assert.True(t, exists(ep.TargetPath))

	// 剧集目录里还有别的视频：只删这一集
	var ep2 models.MediaTransferHistory
	require.NoError(t, e.db.Where("episode = ?", 2).First(&ep2).Error)
	require.NoError(t, e.svc.DeleteHistory(e.ctx, ep2.ID, true))
	assert.False(t, exists(ep2.TargetPath))
	assert.False(t, exists(strings.TrimSuffix(ep2.TargetPath, ".mkv")+".nfo"))
	assert.False(t, exists(strings.TrimSuffix(ep2.TargetPath, ".mkv")+".en.srt"))
	show := filepath.Join(e.tv, "最后生还者 (2023)")
	assert.True(t, exists(filepath.Join(show, "tvshow.nfo")), "剧集目录里还有第 1 集")
}

func TestDeleteHistoryRefusesReplacedFileAndMove(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	row := e.history()[0]
	require.NoError(t, os.Remove(row.TargetPath))
	require.NoError(t, os.WriteFile(row.TargetPath, []byte("replaced by user"), 0o644))
	err = e.svc.DeleteHistory(e.ctx, row.ID, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "换成了别的文件")
	assert.Equal(t, "replaced by user", readFile(t, row.TargetPath))
	assert.Len(t, e.history(), 1, "没删成时记录也留着")

	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("id = ?", row.ID).Update("mode", models.MediaModeMove).Error)
	err = e.svc.DeleteHistory(e.ctx, row.ID, true)
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "唯一的一份")
	require.NoError(t, e.svc.DeleteHistory(e.ctx, row.ID, false))
}

func TestReconcileDeletesLinksAfterTorrentRemoved(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1, DeleteLinksOnRemove: true})
	e.oppenheimer()
	e.lastOfUs()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)

	assert.Zero(t, e.svc.Reconcile(e.ctx), "种子都还在")

	// 只删了种子、数据还在：不动
	e.dl.remove(tlouHash)
	assert.Zero(t, e.svc.Reconcile(e.ctx))

	// 连数据删掉：删库里的链接
	e.dl.remove(oppHash)
	movie := e.history()[0]
	require.NoError(t, os.RemoveAll(filepath.Join(e.dlDir, oppName)))
	assert.Equal(t, 1, e.svc.Reconcile(e.ctx))
	assert.False(t, exists(movie.TargetPath))
	var row models.MediaTransferHistory
	require.NoError(t, e.db.First(&row, movie.ID).Error)
	assert.Equal(t, models.MediaTransferRemoved, row.Status)
	assert.Contains(t, row.Message, "一并删除")
	assert.Zero(t, e.svc.Reconcile(e.ctx), "已经清理过的不再处理")

	// 下载器连不上时不动
	require.NoError(t, os.RemoveAll(filepath.Join(e.dlDir, tlouName)))
	e.dl.listErr = assert.AnError
	assert.Zero(t, e.svc.Reconcile(e.ctx))
	e.dl.listErr = nil

	// 库目录不在（例如没挂载）时不动
	require.NoError(t, os.Rename(e.tv, e.tv+".off"))
	assert.Zero(t, e.svc.Reconcile(e.ctx))
	require.NoError(t, os.Rename(e.tv+".off", e.tv))
	assert.Equal(t, 2, e.svc.Reconcile(e.ctx))
	assert.False(t, exists(filepath.Join(e.tv, "最后生还者 (2023)")), "剧集目录里没有视频了，刮削的文件与空目录一起删")
}

func TestReconcileLeavesCopies(t *testing.T) {
	e := newEnv(t)
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Mode: models.MediaModeCopy})
	e.settings(SettingsInput{MinVideoMB: 1, DeleteLinksOnRemove: true})
	e.oppenheimer()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	e.dl.remove(oppHash)
	require.NoError(t, os.RemoveAll(filepath.Join(e.dlDir, oppName)))
	assert.Zero(t, e.svc.Reconcile(e.ctx), "复制的文件不删")
	assert.True(t, exists(e.history()[0].TargetPath))
}

// 目录名里带 [tmdbid-…] 时也能删干净：方括号不能当成通配符
func TestDeleteHistoryBracketedShowDir(t *testing.T) {
	e := newEnv(t)
	e.library(LibraryInput{
		Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Scrape: true,
		Template: "{{.Title}} ({{.Year}}) [tmdbid-{{.TMDBID}}]/Season {{pad .Season 2}}/{{.Title}} - {{.SeasonEpisode}}",
	})
	e.settings(SettingsInput{MinVideoMB: 1})
	e.addTorrent(tlouHash, tlouName, map[string]int{tlouName + "/The.Last.of.Us.S01E01.mkv": 2}, nil)
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created, res.Plan.Problem)
	show := filepath.Join(e.tv, "最后生还者 (2023) [tmdbid-100088]")
	require.True(t, exists(filepath.Join(show, "season01-poster.jpg")))
	require.NoError(t, e.svc.DeleteHistory(e.ctx, e.history()[0].ID, true))
	assert.False(t, exists(show), "季海报也删掉，剧集目录删干净")
	assert.True(t, exists(e.tv))
}
