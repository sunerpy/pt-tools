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
		{name: "用户换了库里的文件", mode: OverwriteAlways, oldMB: 2, newMB: 3, prepare: func(e *env, target string) {
			require.NoError(e.t, os.Remove(target))
			require.NoError(e.t, os.WriteFile(target, []byte("mine"), 0o644))
		}, want: "不会覆盖"},
		{name: "旧版本是移动整理的", mode: OverwriteAlways, oldMB: 2, newMB: 3, prepare: func(e *env, _ string) {
			require.NoError(e.t, e.db.Model(&models.MediaTransferHistory{}).Where("info_hash = ?", ep1Old).Update("mode", models.MediaModeMove).Error)
		}, want: "库里唯一的一份"},
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

// 总是替换：换下来的旧种子自动整理（扫描）时不再换回去，记录保持已删除；手动整理才换回去
func TestOverwriteNoPingPong(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, OverwriteMode: OverwriteAlways})
	e.episodeTorrents(2, 3)
	for _, h := range []string{ep1Old, ep1New} {
		res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: h})
		require.NoError(t, err)
		require.Equal(t, 1, res.Created)
	}
	target := e.rowOf(ep1New).TargetPath
	require.Equal(t, models.MediaTransferRemoved, e.rowOf(ep1Old).Status)

	for range 2 {
		e.svc.mu.Lock()
		res, err := e.svc.organize(e.ctx, Request{DownloaderID: 1, Hash: ep1Old}, models.MediaTriggerScan)
		e.svc.mu.Unlock()
		require.NoError(t, err)
		assert.Equal(t, 0, res.Created)
		assert.Equal(t, models.MediaTransferRemoved, e.rowOf(ep1Old).Status, "记录不改，下次扫描也不换")
		assert.True(t, sameFile(t, e.rowOf(ep1New).SourcePath, target))
	}

	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: ep1Old})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created, "手动整理换回去")
	assert.True(t, sameFile(t, e.rowOf(ep1Old).SourcePath, target))
	assert.Equal(t, models.MediaTransferRemoved, e.rowOf(ep1New).Status)
}

func TestOverwriteModeValidation(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.SaveLibrary(e.ctx, 0, LibraryInput{Name: "x", Kind: "tv", Path: e.tv, OverwriteMode: "bigger"})
	require.ErrorIs(t, err, ErrInvalid)
	v := e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, OverwriteMode: " always "})
	assert.Equal(t, OverwriteAlways, v.OverwriteMode)
}
