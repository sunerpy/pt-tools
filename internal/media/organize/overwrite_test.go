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

	"github.com/sunerpy/pt-tools/models"
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
