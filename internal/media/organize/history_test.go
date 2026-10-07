package organize

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

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
	require.NoError(t, e.del(movie.ID, true))
	assert.False(t, exists(movie.TargetPath))
	assert.False(t, exists(filepath.Dir(movie.TargetPath)), "电影目录里没别的视频了，连目录一起删")
	assert.True(t, exists(movie.SourcePath), "下载目录里的源文件不动")
	assert.True(t, exists(e.movies))
	require.ErrorIs(t, e.del(movie.ID, true), ErrNotFound)

	// 只删记录
	var ep models.MediaTransferHistory
	require.NoError(t, e.db.Where("episode = ?", 1).First(&ep).Error)
	require.NoError(t, e.del(ep.ID, false))
	assert.True(t, exists(ep.TargetPath))

	// 剧集目录里还有别的视频：只删这一集
	var ep2 models.MediaTransferHistory
	require.NoError(t, e.db.Where("episode = ?", 2).First(&ep2).Error)
	require.NoError(t, e.del(ep2.ID, true))
	assert.False(t, exists(ep2.TargetPath))
	assert.False(t, exists(strings.TrimSuffix(ep2.TargetPath, ".mkv")+".nfo"))
	assert.False(t, exists(strings.TrimSuffix(ep2.TargetPath, ".mkv")+".en.srt"))
	show := filepath.Join(e.tv, "最后生还者 (2023)")
	assert.True(t, exists(filepath.Join(show, "tvshow.nfo")), "剧集目录里还有第 1 集")
}

// 库里的视频被换掉了：连文件删除时视频留着并写明，pt-tools 整理的字幕与刮削写的 NFO 照删；移动整理的不连文件删
func TestDeleteHistoryKeepsReplacedVideoRefusesMove(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	row := e.history()[0]

	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("id = ?", row.ID).Update("mode", models.MediaModeMove).Error)
	_, err = e.svc.DeleteHistory(e.ctx, row.ID, true)
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "唯一的一份")
	assert.True(t, exists(row.TargetPath), "移动整理的不删")
	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("id = ?", row.ID).Update("mode", models.MediaModeHardlink).Error)

	require.NoError(t, os.Remove(row.TargetPath))
	require.NoError(t, os.WriteFile(row.TargetPath, []byte("replaced by user"), 0o644))
	kept, err := e.svc.DeleteHistory(e.ctx, row.ID, true)
	require.NoError(t, err)
	require.Len(t, kept, 1)
	assert.True(t, strings.HasPrefix(kept[0], "库里的 "), kept[0])
	assert.Contains(t, kept[0], "换成了别的文件")
	assert.Equal(t, "replaced by user", readFile(t, row.TargetPath), "换掉的视频留着")
	stem := strings.TrimSuffix(row.TargetPath, ".mkv")
	assert.False(t, exists(stem+".zh-CN.ass"), "整理的字幕删掉")
	assert.False(t, exists(stem+".nfo"), "刮削写的 NFO 删掉")
	assert.True(t, exists(filepath.Join(filepath.Dir(row.TargetPath), "poster.jpg")), "目录里还有视频，海报留着")
	assert.Empty(t, e.history())
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
	require.NoError(t, e.del(e.history()[0].ID, true))
	assert.False(t, exists(show), "季海报也删掉，剧集目录删干净")
	assert.True(t, exists(e.tv))
}

// 删除记录并删除库里的文件时，只删 pt-tools 刮削时写的 NFO 与图片：用户自己放的海报留着，目录也留着
func TestDeleteHistoryKeepsUserFiles(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	dir := filepath.Join(e.movies, "奥本海默 (2023)")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "poster.jpg"), []byte("my poster"), 0o644))
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, "my poster", readFile(t, filepath.Join(dir, "poster.jpg")), "不覆盖时不动用户的海报")
	row := e.history()[0]
	require.NoError(t, e.del(row.ID, true))
	assert.False(t, exists(row.TargetPath))
	assert.False(t, exists(filepath.Join(dir, "fanart.jpg")), "刮削写的背景删掉")
	assert.False(t, exists(strings.TrimSuffix(row.TargetPath, ".mkv")+".nfo"), "刮削写的 NFO 删掉")
	assert.Equal(t, "my poster", readFile(t, filepath.Join(dir, "poster.jpg")), "用户的海报留着")

	// 刮削写的文件之后被换掉了：编号对不上，不删
	e.oppenheimer()
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	row = e.history()[0]
	nfo := strings.TrimSuffix(row.TargetPath, ".mkv") + ".nfo"
	require.NoError(t, os.Remove(nfo))
	require.NoError(t, os.WriteFile(nfo, []byte("<movie>mine</movie>"), 0o644))
	require.NoError(t, e.del(row.ID, true))
	assert.Equal(t, "<movie>mine</movie>", readFile(t, nfo))
}

func TestReconcileNeedsSaveRoot(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1, DeleteLinksOnRemove: true})
	e.oppenheimer()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	row := e.history()[0]
	assert.Equal(t, e.dlDir, sql1(t, e, "SELECT save_root FROM media_transfer_histories"))
	e.dl.remove(oppHash)
	// 下载目录整个不在了（没挂载）：认不出数据是不是真的删了，不清理
	require.NoError(t, os.Rename(e.dlDir, e.dlDir+".off"))
	assert.Zero(t, e.svc.Reconcile(e.ctx))
	assert.True(t, exists(row.TargetPath))
	require.NoError(t, os.Rename(e.dlDir+".off", e.dlDir))
	require.NoError(t, os.RemoveAll(filepath.Join(e.dlDir, oppName)))
	assert.Equal(t, 1, e.svc.Reconcile(e.ctx))
	assert.False(t, exists(row.TargetPath))
}

// del 删除一条整理记录，只看错误（留着的文件的说明不看）。
func (e *env) del(id uint, files bool) error {
	_, err := e.svc.DeleteHistory(e.ctx, id, files)
	return err
}

func sql1(t *testing.T, e *env, q string) string {
	t.Helper()
	var out string
	require.NoError(t, e.db.Raw(q).Scan(&out).Error)
	return out
}

// 两批整理到同一个剧集目录：先删第一批时目录级的刮削文件转给第二批，删到最后一批时一起删掉
func TestDeleteHistoryHandsOverSharedFiles(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.addTorrent("8888888888888888888888888888888888888888", "The.Last.of.Us.S01E01.2160p.mkv", map[string]int{"The.Last.of.Us.S01E01.2160p.mkv": 2}, nil)
	e.addTorrent("9999999999999999999999999999999999999999", "The.Last.of.Us.S01E02.2160p.mkv", map[string]int{"The.Last.of.Us.S01E02.2160p.mkv": 2}, nil)
	for _, h := range []string{"8888888888888888888888888888888888888888", "9999999999999999999999999999999999999999"} {
		res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: h})
		require.NoError(t, err)
		require.Equal(t, 1, res.Created, res.Plan.Problem)
	}
	show := filepath.Join(e.tv, "最后生还者 (2023)")
	require.True(t, exists(filepath.Join(show, "tvshow.nfo")))
	rows := e.history()
	require.Len(t, rows, 2)
	require.NoError(t, e.del(rows[0].ID, true))
	assert.True(t, exists(filepath.Join(show, "tvshow.nfo")), "还有第 2 集，目录级的文件留着")
	require.NoError(t, e.del(rows[1].ID, true))
	assert.False(t, exists(show), "最后一集删掉时，转过来的目录级文件一起删，目录删干净")
}

// twoEpisodes 把两集分别作为两个种子整理进同一个剧集目录，返回剧集目录。
func (e *env) twoEpisodes() (string, []string) {
	e.t.Helper()
	hashes := []string{"8888888888888888888888888888888888888888", "9999999999999999999999999999999999999999"}
	for i, h := range hashes {
		name := fmt.Sprintf("The.Last.of.Us.S01E0%d.2160p.mkv", i+1)
		e.addTorrent(h, name, map[string]int{name: 2}, nil)
		res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: h})
		require.NoError(e.t, err)
		require.Equal(e.t, 1, res.Created, res.Plan.Problem)
	}
	show := filepath.Join(e.tv, "最后生还者 (2023)")
	require.True(e.t, exists(filepath.Join(show, "tvshow.nfo")))
	return show, hashes
}

// 两个种子连数据一起删掉、同一轮清理里处理：前一条转过来的目录级文件，后一条删最后一集时一起删掉
func TestReconcileHandsOverWithinOnePass(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1, DeleteLinksOnRemove: true})
	show, hashes := e.twoEpisodes()
	for i, h := range hashes {
		e.dl.remove(h)
		require.NoError(t, os.Remove(filepath.Join(e.dlDir, fmt.Sprintf("The.Last.of.Us.S01E0%d.2160p.mkv", i+1))))
	}
	assert.Equal(t, 2, e.svc.Reconcile(e.ctx))
	assert.False(t, exists(show), "目录级文件与空目录都删掉")
}

// 把目录级文件转给另一条记录时写数据库失败：返回错误、记录留着；之后再删还能转交，删到最后一集时删干净
func TestDeleteHistoryHandOverFailureKeepsRecord(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	show, _ := e.twoEpisodes()
	rows := e.history()
	boom := errors.New("database is locked")
	cb := e.db.Callback().Update()
	require.NoError(t, cb.Before("gorm:update").Register("test:fail_extras", func(tx *gorm.DB) {
		if m, ok := tx.Statement.Dest.(map[string]any); ok {
			if _, ok := m["extras"]; ok {
				_ = tx.AddError(boom)
			}
		}
	}))
	_, err := e.svc.DeleteHistory(e.ctx, rows[0].ID, true)
	require.ErrorIs(t, err, boom)
	assert.Len(t, e.history(), 2, "转交没成，记录留着")
	require.NoError(t, cb.Remove("test:fail_extras"))

	require.NoError(t, e.del(rows[0].ID, true))
	assert.Contains(t, sql1(t, e, fmt.Sprintf("SELECT extras FROM media_transfer_histories WHERE id = %d", rows[1].ID)), "tvshow.nfo")
	require.NoError(t, e.del(rows[1].ID, true))
	assert.False(t, exists(show))
}

// 不持锁检查过以后、清理前，这条记录被重新整理过（这里模拟成改成移动、换了目标）：不删新的目标，记录也不改成已删除
func TestReconcileRowSkipsRowChangedSinceCheck(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1, DeleteLinksOnRemove: true})
	e.oppenheimer()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	snap := e.history()[0]
	require.NoError(t, os.RemoveAll(filepath.Join(e.dlDir, oppName)))

	moved := filepath.Join(filepath.Dir(snap.TargetPath), "moved.mkv")
	require.NoError(t, os.WriteFile(moved, []byte("the only copy"), 0o644))
	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("id = ?", snap.ID).
		Updates(map[string]any{"mode": models.MediaModeMove, "target_path": moved}).Error)
	e.svc.mu.Lock()
	ok, err := e.svc.reconcileRow(e.ctx, snap)
	e.svc.mu.Unlock()
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, "the only copy", readFile(t, moved), "移动过去的唯一一份不删")
	assert.Equal(t, models.MediaTransferDone, e.history()[0].Status)

	// 只是目标换了（方式没变）也不动；记录恢复成检查时那样、源文件确实不在时照常清理
	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("id = ?", snap.ID).
		Updates(map[string]any{"mode": snap.Mode, "target_path": moved}).Error)
	e.svc.mu.Lock()
	ok, err = e.svc.reconcileRow(e.ctx, snap)
	e.svc.mu.Unlock()
	require.NoError(t, err)
	assert.False(t, ok)
	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("id = ?", snap.ID).Update("target_path", snap.TargetPath).Error)
	e.svc.mu.Lock()
	ok, err = e.svc.reconcileRow(e.ctx, snap)
	e.svc.mu.Unlock()
	require.NoError(t, err)
	assert.True(t, ok)
	assert.False(t, exists(snap.TargetPath))
	assert.True(t, exists(moved), "不是这条记录的文件不动")
}

// 洗版：换成更好的版本以后删掉旧版本库里的文件，记录改成已删除并写明原因；移动整理的不删
func TestRetire(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	row := e.history()[0]
	kept, err := e.svc.Retire(e.ctx, row.ID, "洗版：换成了 X")
	require.NoError(t, err)
	assert.Empty(t, kept)
	assert.False(t, exists(row.TargetPath))
	got := e.history()[0]
	assert.Equal(t, models.MediaTransferRemoved, got.Status)
	assert.Contains(t, got.Message, "洗版")
	kept, err = e.svc.Retire(e.ctx, row.ID, "again")
	require.NoError(t, err, "不是已整理的不动")
	assert.Nil(t, kept)

	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("id = ?", row.ID).Updates(map[string]any{"status": models.MediaTransferDone, "mode": models.MediaModeMove}).Error)
	_, err = e.svc.Retire(e.ctx, row.ID, "x")
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.Retire(e.ctx, 999, "x")
	require.ErrorIs(t, err, ErrNotFound)
}
