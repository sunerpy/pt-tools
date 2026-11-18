package organize

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

const (
	ep1Old = "1111111111111111111111111111111111111111"
	ep1New = "3333333333333333333333333333333333333333"
)

// 同一集的两个种子：默认的剧集模板不带画质，整理出的文件名相同
func (e *env) episodeTorrents(oldMB, newMB int) {
	e.t.Helper()
	e.addTorrent(ep1Old, "The.Last.of.Us.S01E01.1080p.WEB-DL", map[string]int{
		"The.Last.of.Us.S01E01.1080p.WEB-DL/The.Last.of.Us.S01E01.1080p.WEB-DL.mkv":     oldMB,
		"The.Last.of.Us.S01E01.1080p.WEB-DL/The.Last.of.Us.S01E01.1080p.WEB-DL.chs.srt": 0,
	}, nil)
	e.addTorrent(ep1New, "The.Last.of.Us.S01E01.2160p.BluRay", map[string]int{
		"The.Last.of.Us.S01E01.2160p.BluRay/The.Last.of.Us.S01E01.2160p.BluRay.mkv":     newMB,
		"The.Last.of.Us.S01E01.2160p.BluRay/The.Last.of.Us.S01E01.2160p.BluRay.chs.srt": 0,
	}, nil)
}

func (e *env) rowOf(hash string) models.MediaTransferHistory {
	e.t.Helper()
	var r models.MediaTransferHistory
	require.NoError(e.t, e.db.Where("info_hash = ?", hash).First(&r).Error)
	return r
}

// 新文件更大时替换：新文件盖住旧的，旧记录的字幕与 NFO 换成新的，旧记录标成已删除；目录里的刮削文件归新记录
func TestOverwriteLargerReplacesOldVersion(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	lib := e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Scrape: true, OverwriteMode: OverwriteLarger})
	assert.Equal(t, OverwriteLarger, lib.OverwriteMode)
	e.episodeTorrents(2, 3)
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1Old})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)
	old := e.rowOf(ep1Old)
	target := old.TargetPath
	nfo := strings.TrimSuffix(target, ".mkv") + ".nfo"
	sub := strings.TrimSuffix(target, ".mkv") + ".zh-CN.srt"
	require.True(t, exists(nfo))
	require.True(t, exists(sub))

	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: ep1New})
	require.NoError(t, err)
	require.Len(t, p.Items, 1)
	assert.Equal(t, ItemReplace, p.Items[0].Status)
	assert.Contains(t, p.Items[0].Message, "替换库里的旧版本（The.Last.of.Us.S01E01.1080p.WEB-DL，2.00 MiB → 3.00 MiB）")
	assert.True(t, sameFile(t, old.SourcePath, target), "预览不改动文件")

	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1New})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)
	newRow := e.rowOf(ep1New)
	assert.Equal(t, models.MediaTransferDone, newRow.Status)
	assert.Equal(t, target, newRow.TargetPath)
	assert.Contains(t, newRow.Message, "替换了旧版本")
	assert.True(t, sameFile(t, newRow.SourcePath, target), "库里换成了新文件")
	assert.True(t, exists(old.SourcePath), "下载目录里的旧文件不动")
	assert.True(t, exists(sub), "字幕换成了新种子的")
	assert.True(t, exists(nfo), "NFO 重新写了")
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(target), "*.pt-tools-*"))
	assert.Empty(t, left, "不留临时文件")
	old = e.rowOf(ep1Old)
	assert.Equal(t, models.MediaTransferRemoved, old.Status)
	assert.Contains(t, old.Message, "换成了 The.Last.of.Us.S01E01.2160p.BluRay")

	// 目录里的 tvshow.nfo 归了新记录：删掉新记录（连文件）时剧集目录删干净
	show := filepath.Join(e.tv, "最后生还者 (2023)")
	require.True(t, exists(filepath.Join(show, "tvshow.nfo")))
	require.NoError(t, e.del(newRow.ID, true))
	assert.False(t, exists(show))
}

// 不替换的几种情况：默认策略、新文件不比旧的大、库里的文件不是 pt-tools 放的、旧版本是移动整理的
func TestOverwriteKeepsOldVersion(t *testing.T) {
	cases := []struct {
		name    string
		mode    string
		oldMB   int
		newMB   int
		prepare func(e *env, target string)
		want    string
	}{
		{name: "默认不替换", mode: OverwriteNever, oldMB: 2, newMB: 3, want: "没有替换：要换成新的"},
		{name: "新文件不比旧的大", mode: OverwriteLarger, oldMB: 3, newMB: 2, want: "不比它大，不替换"},
		{name: "用户换了库里的文件", mode: OverwriteLarger, oldMB: 2, newMB: 3, prepare: func(e *env, target string) {
			require.NoError(e.t, os.Remove(target))
			require.NoError(e.t, os.WriteFile(target, []byte("mine"), 0o644))
		}, want: "不会覆盖"},
		{name: "旧版本是移动整理的", mode: OverwriteLarger, oldMB: 2, newMB: 3, prepare: func(e *env, _ string) {
			require.NoError(e.t, e.db.Model(&models.MediaTransferHistory{}).Where("info_hash = ?", ep1Old).Update("mode", models.MediaModeMove).Error)
		}, want: "库里唯一的一份"},
		{name: "记录里没有文件编号", mode: OverwriteLarger, oldMB: 2, newMB: 3, prepare: func(e *env, _ string) {
			require.NoError(e.t, e.db.Model(&models.MediaTransferHistory{}).Where("info_hash = ?", ep1Old).Update("target_file_id", "").Error)
		}, want: "没有它的文件编号"},
		{name: "别的条目（模板是固定的文件名）", mode: OverwriteLarger, oldMB: 2, newMB: 3, prepare: func(e *env, _ string) {
			require.NoError(e.t, e.db.Model(&models.MediaTransferHistory{}).Where("info_hash = ?", ep1Old).Update("tmdb_id", 1).Error)
		}, want: "不会覆盖"},
		{name: "别的集", mode: OverwriteLarger, oldMB: 2, newMB: 3, prepare: func(e *env, _ string) {
			require.NoError(e.t, e.db.Model(&models.MediaTransferHistory{}).Where("info_hash = ?", ep1Old).Update("episode", 9).Error)
		}, want: "不会覆盖"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			e.settings(SettingsInput{MinVideoMB: 1})
			e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, OverwriteMode: c.mode})
			e.episodeTorrents(c.oldMB, c.newMB)
			_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1Old})
			require.NoError(t, err)
			old := e.rowOf(ep1Old)
			before := readFile(t, old.TargetPath)
			if c.prepare != nil {
				c.prepare(e, old.TargetPath)
				before = readFile(t, old.TargetPath)
			}
			res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1New})
			require.NoError(t, err)
			assert.Equal(t, 0, res.Created)
			assert.Equal(t, 1, res.Skipped)
			assert.Contains(t, e.rowOf(ep1New).Message, c.want)
			assert.Equal(t, before, readFile(t, old.TargetPath), "库里的文件不动")
			assert.NotEqual(t, models.MediaTransferRemoved, e.rowOf(ep1Old).Status)
		})
	}
}

// 只按大小换：换下来的小的旧种子再整理（扫描或手动）时不会把大的换回去，记成跳过
func TestOverwriteNoSwapBack(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, OverwriteMode: OverwriteLarger})
	e.episodeTorrents(2, 3)
	for _, h := range []string{ep1Old, ep1New} {
		res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: h})
		require.NoError(t, err)
		require.Equal(t, 1, res.Created)
	}
	target := e.rowOf(ep1New).TargetPath
	e.svc.mu.Lock()
	res, err := e.svc.organize(e.ctx, Request{DownloaderID: 1, Hash: ep1Old}, models.MediaTriggerScan)
	e.svc.mu.Unlock()
	require.NoError(t, err)
	assert.Equal(t, 0, res.Created)
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1Old})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Created)
	assert.True(t, sameFile(t, e.rowOf(ep1New).SourcePath, target), "库里还是大的那个")
	assert.Equal(t, models.MediaTransferDone, e.rowOf(ep1New).Status)
}

func TestOverwriteModeValidation(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.SaveLibrary(e.ctx, 0, LibraryInput{Name: "x", Kind: "tv", Path: e.tv, OverwriteMode: "bigger"})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.SaveLibrary(e.ctx, 0, LibraryInput{Name: "x", Kind: "tv", Path: e.tv, OverwriteMode: "always"})
	require.ErrorIs(t, err, ErrInvalid, "只按大小换")
	v := e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, OverwriteMode: " larger "})
	assert.Equal(t, OverwriteLarger, v.OverwriteMode)
}

// 计划以后、替换以前目标位置被换成了别的文件：挪开以后认出不是旧记录放的，放回原处、不替换，不留临时文件与日志
func TestReplaceForeignSwapDuringRun(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, OverwriteMode: OverwriteLarger})
	e.episodeTorrents(2, 3)
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1Old})
	require.NoError(t, err)
	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: ep1New})
	require.NoError(t, err)
	it := &p.Items[0]
	require.Equal(t, ItemReplace, it.Status)
	require.NoError(t, os.Remove(it.Target))
	require.NoError(t, os.WriteFile(it.Target, []byte("someone else's"), 0o644))

	res := &Result{Plan: p}
	e.svc.mu.Lock()
	ok := e.svc.replace(e.ctx, p, it, models.MediaTriggerManual, res)
	e.svc.mu.Unlock()
	assert.False(t, ok)
	assert.Contains(t, it.Message, "换过了")
	assert.Equal(t, "someone else's", readFile(t, it.Target), "别人的文件放回原处")
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(it.Target), "*.pt-tools-*"))
	assert.Empty(t, left)
	var n int64
	require.NoError(t, e.db.Model(&models.MediaReplaceJournal{}).Count(&n).Error)
	assert.Zero(t, n)
	assert.Equal(t, models.MediaTransferDone, e.rowOf(ep1Old).Status, "旧记录不动")
}

// pt-tools 中途退出：没提交的替换在下次启动时撤回（旧文件放回原处、新文件删掉），已提交的收尾（删掉挪开的旧文件）
func TestRecoverReplacements(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, OverwriteMode: OverwriteLarger})
	e.episodeTorrents(2, 3)
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1Old})
	require.NoError(t, err)
	old := e.rowOf(ep1Old)
	newSrc := filepath.Join(e.dlDir, "The.Last.of.Us.S01E01.2160p.BluRay", "The.Last.of.Us.S01E01.2160p.BluRay.mkv")

	// 没提交：新文件已经放到目标位置、旧文件在备份名上
	j := models.MediaReplaceJournal{
		OldID: old.ID, Source: newSrc, Mode: models.MediaModeHardlink, Target: old.TargetPath,
		Temp: sideName(old.TargetPath, "new"), Backup: sideName(old.TargetPath, "old"),
	}
	require.NoError(t, os.Rename(old.TargetPath, j.Backup))
	require.NoError(t, os.Link(newSrc, old.TargetPath))
	j.TempID, _ = fileIDOf(old.TargetPath)
	require.NoError(t, e.db.Create(&j).Error)
	e.svc.recoverReplacements(e.ctx)
	assert.True(t, sameFile(t, old.SourcePath, old.TargetPath), "旧文件放回原处")
	assert.False(t, exists(j.Backup))
	var n int64
	require.NoError(t, e.db.Model(&models.MediaReplaceJournal{}).Count(&n).Error)
	assert.Zero(t, n)

	// 没提交、只放好了临时文件：删掉临时文件
	j2 := models.MediaReplaceJournal{
		OldID: old.ID, Source: newSrc, Mode: models.MediaModeHardlink, Target: old.TargetPath,
		Temp: sideName(old.TargetPath, "new"), Backup: sideName(old.TargetPath, "old"),
	}
	require.NoError(t, os.Link(newSrc, j2.Temp))
	j2.TempID, _ = fileIDOf(j2.Temp)
	require.NoError(t, e.db.Create(&j2).Error)
	e.svc.recoverReplacements(e.ctx)
	assert.False(t, exists(j2.Temp))
	assert.True(t, sameFile(t, old.SourcePath, old.TargetPath))

	// 已提交：新文件在目标位置，删掉挪开的旧文件与旧字幕
	sub := strings.TrimSuffix(old.TargetPath, ".mkv") + ".zh-CN.srt"
	require.True(t, exists(sub))
	j3 := models.MediaReplaceJournal{
		OldID: old.ID, Source: newSrc, Mode: models.MediaModeHardlink, Target: old.TargetPath,
		Temp: sideName(old.TargetPath, "new"), Backup: sideName(old.TargetPath, "old"), Committed: true,
	}
	require.NoError(t, os.Rename(old.TargetPath, j3.Backup))
	j3.BackupID, _ = fileIDOf(j3.Backup)
	require.NoError(t, os.Link(newSrc, old.TargetPath))
	j3.TempID, _ = fileIDOf(old.TargetPath)
	require.NoError(t, e.db.Create(&j3).Error)
	e.svc.recoverReplacements(e.ctx)
	assert.False(t, exists(j3.Backup), "挪开的旧文件删掉")
	assert.False(t, exists(sub), "旧字幕删掉")
	assert.True(t, sameFile(t, newSrc, old.TargetPath))
	assert.True(t, exists(old.SourcePath), "下载目录里的不动")
	require.NoError(t, e.db.Model(&models.MediaReplaceJournal{}).Count(&n).Error)
	assert.Zero(t, n)
}

// 已提交的替换收尾时，目标位置上已经不是这次放的新文件（中途退出期间被删或被换）：挪开的旧文件就是仅剩的版本，不删，日志留着
func TestRecoverReplacementKeepsBackup(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, OverwriteMode: OverwriteLarger})
	e.episodeTorrents(2, 3)
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1Old})
	require.NoError(t, err)
	old := e.rowOf(ep1Old)
	j := models.MediaReplaceJournal{
		OldID: old.ID, Mode: models.MediaModeHardlink, Target: old.TargetPath,
		Temp: sideName(old.TargetPath, "new"), Backup: sideName(old.TargetPath, "old"), TempID: "1:2:3:4", Committed: true,
	}
	require.NoError(t, os.Rename(old.TargetPath, j.Backup))
	require.NoError(t, e.db.Create(&j).Error)
	e.svc.recoverReplacements(e.ctx)
	assert.True(t, exists(j.Backup), "目标位置上没有新文件：旧文件留着")
	var n int64
	require.NoError(t, e.db.Model(&models.MediaReplaceJournal{}).Count(&n).Error)
	assert.EqualValues(t, 1, n, "日志留着，之后再试")
}

// 收尾时旧字幕一时删不掉（被占用）：新字幕先不放、日志留着；好了以后后台恢复删掉旧字幕、放好新字幕并记进新记录
func TestReplaceSubtitlesAfterCleanupRetry(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, OverwriteMode: OverwriteLarger})
	e.episodeTorrents(2, 3)
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1Old})
	require.NoError(t, err)
	target := e.rowOf(ep1Old).TargetPath
	sub := strings.TrimSuffix(target, ".mkv") + ".zh-CN.srt"
	newSub := filepath.Join(e.dlDir, "The.Last.of.Us.S01E01.2160p.BluRay", "The.Last.of.Us.S01E01.2160p.BluRay.chs.srt")

	orig := removeIfOurs
	t.Cleanup(func() { removeIfOurs = orig })
	removeIfOurs = func(dst, mode, fileID, src string) error {
		if strings.HasSuffix(dst, ".srt") {
			return errors.New("file in use")
		}
		return orig(dst, mode, fileID, src)
	}
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1New})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)
	assert.Contains(t, strings.Join(res.Messages, "\n"), "收尾没做完")
	assert.True(t, sameFile(t, e.rowOf(ep1New).SourcePath, target), "视频已经换好")
	assert.False(t, sameFile(t, newSub, sub), "旧字幕还占着，新字幕先不放")
	var n int64
	require.NoError(t, e.db.Model(&models.MediaReplaceJournal{}).Count(&n).Error)
	require.EqualValues(t, 1, n, "日志留着")

	removeIfOurs = orig
	e.svc.recoverReplacements(e.ctx)
	assert.True(t, sameFile(t, newSub, sub), "新字幕放好了")
	assert.Contains(t, e.rowOf(ep1New).Extras, "zh-CN.srt", "记进了新记录")
	require.NoError(t, e.db.Model(&models.MediaReplaceJournal{}).Count(&n).Error)
	assert.Zero(t, n)
}

// 看不了临时文件（权限、挂载出错）时不能当成已经清理：日志留着，之后再撤回
func TestUndoReplaceKeepsJournalOnStatError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("要靠目录权限让 Lstat 出错")
	}
	e := newEnv(t)
	locked := filepath.Join(e.root, "locked")
	require.NoError(t, os.MkdirAll(locked, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(locked, "x.tmp"), []byte("x"), 0o644))
	require.NoError(t, os.Chmod(locked, 0))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	j := models.MediaReplaceJournal{
		Mode: models.MediaModeCopy, Target: filepath.Join(e.root, "t.mkv"),
		Temp: filepath.Join(locked, "x.tmp"), Backup: filepath.Join(e.root, "t.mkv.old"),
	}
	require.NoError(t, e.db.Create(&j).Error)
	e.svc.mu.Lock()
	err := e.svc.undoReplace(e.ctx, j)
	e.svc.mu.Unlock()
	require.Error(t, err)
	var n int64
	require.NoError(t, e.db.Model(&models.MediaReplaceJournal{}).Count(&n).Error)
	assert.EqualValues(t, 1, n)
}

// 已提交的替换收尾：备份名上的文件换过了（例如旧记录被删以后有人放了别的文件）就不删；
// 上一次已经删了备份、之后目标上的视频被换掉时不再放新字幕
func TestFinishReplaceVerifiesBackupAndTarget(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, OverwriteMode: OverwriteLarger})
	e.episodeTorrents(2, 3)
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1Old})
	require.NoError(t, err)
	old := e.rowOf(ep1Old)
	newDir := filepath.Join(e.dlDir, "The.Last.of.Us.S01E01.2160p.BluRay")
	newSrc := filepath.Join(newDir, "The.Last.of.Us.S01E01.2160p.BluRay.mkv")

	// 备份名上换成了别人的文件：编号对不上，不删
	j := models.MediaReplaceJournal{
		OldID: 9999, Source: newSrc, Mode: models.MediaModeHardlink, Target: old.TargetPath,
		Temp: sideName(old.TargetPath, "new"), Backup: sideName(old.TargetPath, "old"), Committed: true,
	}
	require.NoError(t, os.Rename(old.TargetPath, j.Backup))
	j.BackupID, _ = fileIDOf(j.Backup)
	require.NoError(t, os.Remove(j.Backup))
	require.NoError(t, os.WriteFile(j.Backup, []byte("someone else's"), 0o644))
	require.NoError(t, os.Link(newSrc, old.TargetPath))
	j.TempID, _ = fileIDOf(old.TargetPath)
	require.NoError(t, e.db.Create(&j).Error)
	e.svc.recoverReplacements(e.ctx)
	assert.Equal(t, "someone else's", readFile(t, j.Backup), "别人的文件留着")
	require.NoError(t, os.Remove(j.Backup))

	// 备份已经删了、目标上的视频被换掉：不放新字幕，日志删掉
	sub := strings.TrimSuffix(old.TargetPath, ".mkv") + ".zh-CN.srt"
	require.NoError(t, os.Remove(sub))
	subs, _ := json.Marshal([]SubtitlePlan{{Source: filepath.Join(newDir, "The.Last.of.Us.S01E01.2160p.BluRay.chs.srt"), Target: sub}})
	j2 := models.MediaReplaceJournal{
		OldID: old.ID, Source: newSrc, Mode: models.MediaModeHardlink, Target: old.TargetPath,
		Temp: sideName(old.TargetPath, "new"), Backup: sideName(old.TargetPath, "old"), TempID: j.TempID, Subs: string(subs), Committed: true,
	}
	require.NoError(t, os.Remove(old.TargetPath))
	require.NoError(t, os.WriteFile(old.TargetPath, []byte("another video"), 0o644))
	require.NoError(t, e.db.Create(&j2).Error)
	e.svc.recoverReplacements(e.ctx)
	assert.False(t, exists(sub), "视频对不上，不放字幕")
	var n int64
	require.NoError(t, e.db.Model(&models.MediaReplaceJournal{}).Where("id = ?", j2.ID).Count(&n).Error)
	assert.Zero(t, n)
}

// 收尾时读不出目标的文件编号（权限、网络存储一时出错）：不能当成目标没了，日志留着
func TestFinishReplaceKeepsJournalOnTargetStatError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("要靠目录权限让读编号出错")
	}
	e := newEnv(t)
	locked := filepath.Join(e.root, "locked")
	require.NoError(t, os.MkdirAll(locked, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(locked, "t.mkv"), []byte("x"), 0o644))
	require.NoError(t, os.Chmod(locked, 0))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	j := models.MediaReplaceJournal{
		Mode: models.MediaModeCopy, Target: filepath.Join(locked, "t.mkv"), TempID: "1:2:3:4",
		Temp: filepath.Join(e.root, "t.new.tmp"), Backup: filepath.Join(e.root, "t.old.tmp"), Committed: true,
	}
	require.NoError(t, e.db.Create(&j).Error)
	e.svc.mu.Lock()
	_, err := e.svc.finishReplace(e.ctx, j, models.MediaTransferHistory{})
	e.svc.mu.Unlock()
	require.Error(t, err)
	var n int64
	require.NoError(t, e.db.Model(&models.MediaReplaceJournal{}).Count(&n).Error)
	assert.EqualValues(t, 1, n)
}

// 撤回的几种情况：备份只是旧文件多出来的名字时只删备份；旧文件放不回去（目标位置被占着）时报错、日志留着
func TestUndoReplaceCases(t *testing.T) {
	e := newEnv(t)
	dir := filepath.Join(e.root, "lib")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	target := filepath.Join(dir, "a.mkv")
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o644))

	// 备份与目标是同一个文件（挪的时候原来的名字没删掉）
	j := models.MediaReplaceJournal{Mode: models.MediaModeCopy, Target: target, Temp: sideName(target, "new"), Backup: sideName(target, "old")}
	require.NoError(t, os.Link(target, j.Backup))
	require.NoError(t, e.db.Create(&j).Error)
	e.svc.mu.Lock()
	require.NoError(t, e.svc.undoReplace(e.ctx, j))
	e.svc.mu.Unlock()
	assert.False(t, exists(j.Backup))
	assert.Equal(t, "old", readFile(t, target))

	// 目标位置上是别人的文件、备份是旧文件：放不回去，报错、日志留着
	j2 := models.MediaReplaceJournal{Mode: models.MediaModeCopy, Target: target, Temp: sideName(target, "new"), Backup: sideName(target, "old")}
	require.NoError(t, os.WriteFile(j2.Backup, []byte("old2"), 0o644))
	require.NoError(t, e.db.Create(&j2).Error)
	e.svc.recoverReplacements(e.ctx)
	assert.True(t, exists(j2.Backup), "旧文件留在备份名上")
	var n int64
	require.NoError(t, e.db.Model(&models.MediaReplaceJournal{}).Where("id = ?", j2.ID).Count(&n).Error)
	assert.EqualValues(t, 1, n)

	// 临时名上不是这次放的（编号对不上）：不删它，日志照样删掉
	require.NoError(t, os.Remove(j2.Backup))
	j3 := models.MediaReplaceJournal{Mode: models.MediaModeCopy, Target: target, Temp: sideName(target, "new"), Backup: sideName(target, "old"), TempID: "x"}
	require.NoError(t, os.WriteFile(j3.Temp, []byte("?"), 0o644))
	require.NoError(t, e.db.Create(&j3).Error)
	e.svc.mu.Lock()
	require.NoError(t, e.svc.undoReplace(e.ctx, j3))
	e.svc.mu.Unlock()
	assert.True(t, exists(j3.Temp))
}

// 计划以后新文件的源文件没了：放不到临时名，撤回、记成失败，旧版本不动、不留日志
func TestReplaceSourceGone(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, OverwriteMode: OverwriteLarger})
	e.episodeTorrents(2, 3)
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1Old})
	require.NoError(t, err)
	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: ep1New})
	require.NoError(t, err)
	it := &p.Items[0]
	require.Equal(t, ItemReplace, it.Status)
	require.NoError(t, os.Remove(it.Source))
	res := &Result{Plan: p}
	e.svc.mu.Lock()
	ok := e.svc.replace(e.ctx, p, it, models.MediaTriggerManual, res)
	e.svc.mu.Unlock()
	assert.False(t, ok)
	assert.Equal(t, 1, res.Failed)
	assert.Contains(t, it.Message, "旧版本留着")
	assert.True(t, sameFile(t, e.rowOf(ep1Old).SourcePath, it.Target))
	var n int64
	require.NoError(t, e.db.Model(&models.MediaReplaceJournal{}).Count(&n).Error)
	assert.Zero(t, n)
	assert.Equal(t, models.MediaTransferFailed, e.rowOf(ep1New).Status)
}

// failUpdate 让符合 match 的更新出错（模拟写库失败），测试结束时去掉。
func failUpdate(t *testing.T, db *gorm.DB, match func(table string, dest any) bool) {
	t.Helper()
	name := "test:fail:" + t.Name()
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
		if match(tx.Statement.Table, tx.Statement.Dest) {
			_ = tx.AddError(errors.New("injected"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Update().Remove(name) })
}

func hasKey(dest any, key string) bool {
	m, ok := dest.(map[string]any)
	if !ok {
		return false
	}
	_, ok = m[key]
	return ok
}

// 替换途中写日志或写整理记录失败：每一步都撤回，旧版本原样留着，不留临时文件、备份与日志
func TestReplaceUndoOnWriteFailure(t *testing.T) {
	steps := map[string]func(table string, dest any) bool{
		"临时文件编号": func(table string, dest any) bool { return table == "media_replace_journals" && hasKey(dest, "temp_id") },
		"备份编号": func(table string, dest any) bool {
			return table == "media_replace_journals" && hasKey(dest, "backup_id")
		},
		"整理记录": func(table string, dest any) bool {
			return table == "media_transfer_histories" && hasKey(dest, "status")
		},
	}
	for name, match := range steps {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			e.settings(SettingsInput{MinVideoMB: 1})
			e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, OverwriteMode: OverwriteLarger})
			e.episodeTorrents(2, 3)
			_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1Old})
			require.NoError(t, err)
			old := e.rowOf(ep1Old)
			failUpdate(t, e.db, match)
			res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1New})
			require.NoError(t, err)
			assert.Equal(t, 0, res.Created)
			assert.Equal(t, 1, res.Failed)
			assert.True(t, sameFile(t, old.SourcePath, old.TargetPath), "旧版本原样留着")
			left, _ := filepath.Glob(filepath.Join(filepath.Dir(old.TargetPath), "*.pt-tools-*"))
			assert.Empty(t, left)
			var n int64
			require.NoError(t, e.db.Model(&models.MediaReplaceJournal{}).Count(&n).Error)
			assert.Zero(t, n)
			assert.Equal(t, models.MediaTransferDone, e.rowOf(ep1Old).Status)
		})
	}
}

// 媒体库后来改成了移动整理：不替换旧版本
func TestOverwriteMoveLibraryKeepsOld(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	lib := e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, OverwriteMode: OverwriteLarger})
	e.episodeTorrents(2, 3)
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1Old})
	require.NoError(t, err)
	in := LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Mode: models.MediaModeMove, OverwriteMode: OverwriteLarger, Enabled: true}
	_, err = e.svc.SaveLibrary(e.ctx, lib.ID, in)
	require.NoError(t, err)
	e.dl.mu.Lock()
	tr := e.dl.torrents[ep1New]
	tr.State = downloader.TorrentPaused
	e.dl.torrents[ep1New] = tr
	e.dl.mu.Unlock()
	p, err := e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: ep1New})
	require.NoError(t, err)
	require.Len(t, p.Items, 1, p.Problem)
	assert.Equal(t, ItemExists, p.Items[0].Status)
	assert.Contains(t, p.Items[0].Message, "用移动整理")
}

// 替换日志写不进去：不开始替换；撤回时删不掉日志：写明「下次再撤回」，恢复时照样撤回干净
func TestReplaceJournalFailures(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, OverwriteMode: OverwriteLarger})
	e.episodeTorrents(2, 3)
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1Old})
	require.NoError(t, err)
	old := e.rowOf(ep1Old)

	createName := "test:fail-create:" + t.Name()
	require.NoError(t, e.db.Callback().Create().Before("gorm:create").Register(createName, func(tx *gorm.DB) {
		if tx.Statement.Table == "media_replace_journals" {
			_ = tx.AddError(errors.New("injected"))
		}
	}))
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1New})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Failed)
	assert.Contains(t, e.rowOf(ep1New).Message, "替换日志写不进去")
	assert.True(t, sameFile(t, old.SourcePath, old.TargetPath))
	require.NoError(t, e.db.Callback().Create().Remove(createName))

	failUpdate(t, e.db, func(table string, dest any) bool {
		return table == "media_replace_journals" && hasKey(dest, "backup_id")
	})
	deleteName := "test:fail-delete:" + t.Name()
	require.NoError(t, e.db.Callback().Delete().Before("gorm:delete").Register(deleteName, func(tx *gorm.DB) {
		if tx.Statement.Table == "media_replace_journals" {
			_ = tx.AddError(errors.New("injected"))
		}
	}))
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1New})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Failed)
	assert.Contains(t, e.rowOf(ep1New).Message, "下次启动时再撤回")
	assert.True(t, sameFile(t, old.SourcePath, old.TargetPath), "文件已经撤回了，只是日志没删掉")
	require.NoError(t, e.db.Callback().Delete().Remove(deleteName))
	e.svc.recoverReplacements(e.ctx)
	var n int64
	require.NoError(t, e.db.Model(&models.MediaReplaceJournal{}).Count(&n).Error)
	assert.Zero(t, n)

	assert.Nil(t, e.svc.occupant(e.ctx, &Plan{}, &PlanItem{}), "没有媒体库与条目时没有可替换的")
}

// 选好临时名以后、放新文件以前，别的程序占了这个名字：放不成（目标已存在），撤回时不删那个文件
func TestReplaceTempTakenKeepsForeignFile(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, OverwriteMode: OverwriteLarger})
	e.episodeTorrents(2, 3)
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1Old})
	require.NoError(t, err)
	old := e.rowOf(ep1Old)
	var taken string
	orig := transferTemp
	t.Cleanup(func() { transferTemp = orig })
	transferTemp = func(src, dst, mode string) (transfer.Outcome, error) {
		taken = dst
		if werr := os.WriteFile(dst, []byte("someone else's"), 0o644); werr != nil {
			return 0, werr
		}
		return orig(src, dst, mode)
	}
	// 第一次撤回时日志删不掉：恢复时读回来的日志里还有临时名，也不能删别人的文件
	deleteName := "test:fail-delete:" + t.Name()
	require.NoError(t, e.db.Callback().Delete().Before("gorm:delete").Register(deleteName, func(tx *gorm.DB) {
		if tx.Statement.Table == "media_replace_journals" {
			_ = tx.AddError(errors.New("injected"))
		}
	}))
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1New})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Failed)
	require.NotEmpty(t, taken)
	assert.Equal(t, "someone else's", readFile(t, taken), "别人的文件留着")
	assert.True(t, sameFile(t, old.SourcePath, old.TargetPath))
	var n int64
	require.NoError(t, e.db.Model(&models.MediaReplaceJournal{}).Count(&n).Error)
	require.EqualValues(t, 1, n, "日志没删掉")
	require.NoError(t, e.db.Callback().Delete().Remove(deleteName))
	e.svc.recoverReplacements(e.ctx)
	assert.Equal(t, "someone else's", readFile(t, taken), "恢复时也不删")
	require.NoError(t, e.db.Model(&models.MediaReplaceJournal{}).Count(&n).Error)
	assert.Zero(t, n)
}
