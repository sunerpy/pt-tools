package organize

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 手动整理时选了整理方式：这次按它整理、不按媒体库的（库是硬链接）；预览也按它算；记录记下实际用的与选的方式。
// 没选（跟媒体库）时照旧用库的方式，记录里选的方式是空的
func TestChosenModeOverridesLibrary(t *testing.T) {
	for _, mode := range []string{"", models.MediaModeHardlink, models.MediaModeCopy, models.MediaModeSymlink, models.MediaModeMove} {
		t.Run("mode="+mode, func(t *testing.T) {
			e := newEnv(t)
			e.defaultLibraries()
			e.settings(SettingsInput{MinVideoMB: 1})
			src := e.putIncoming("Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv", 5, 10*time.Minute)
			want := firstNonEmpty(mode, models.MediaModeHardlink)

			p, err := e.svc.Preview(e.ctx, Request{SourcePath: src, Mode: mode})
			require.NoError(t, err)
			require.Empty(t, p.Problem)
			assert.Equal(t, want, p.Mode, "预览按选的方式算")

			res, err := e.svc.Organize(e.ctx, Request{SourcePath: src, Mode: mode})
			require.NoError(t, err)
			require.Empty(t, res.Plan.Problem)
			require.Equal(t, 1, res.Created)
			rows := e.pathRows(src)
			require.Len(t, rows, 1)
			r := rows[0]
			assert.Equal(t, models.MediaTransferDone, r.Status)
			assert.Equal(t, want, r.Mode, "实际用的方式")
			assert.Equal(t, mode, r.ChosenMode, "选的方式（跟媒体库时为空）")
			switch want {
			case models.MediaModeHardlink:
				assert.True(t, sameFile(t, src, r.TargetPath))
			case models.MediaModeCopy:
				assert.True(t, exists(src))
				assert.False(t, sameFile(t, src, r.TargetPath), "复制出的是另一个文件")
			case models.MediaModeSymlink:
				to, lerr := os.Readlink(r.TargetPath)
				require.NoError(t, lerr)
				assert.Equal(t, src, to)
			case models.MediaModeMove:
				assert.False(t, exists(src), "移动以后源文件不在原处")
				assert.True(t, exists(r.TargetPath))
			}
		})
	}
}

// 整理方式只能是硬链接、复制、软链接、移动（空着是跟媒体库）：别的在排队以前就报错，不写记录
func TestChosenModeInvalid(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	src := e.putIncoming("Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv", 5, 10*time.Minute)
	for _, bad := range []string{"rename", "Copy", "hard link"} {
		_, err := e.svc.Preview(e.ctx, Request{SourcePath: src, Mode: bad})
		require.ErrorIs(t, err, ErrInvalid, bad)
		require.ErrorContains(t, err, "整理方式", bad)
		_, err = e.svc.Organize(e.ctx, Request{SourcePath: src, Mode: bad})
		require.ErrorIs(t, err, ErrInvalid, bad)
	}
	assert.Empty(t, e.svc.jobs, "都没排进队列")
	assert.Empty(t, e.history(), "都没写记录")
	assert.True(t, exists(src))
}

// 下载器里的种子也能选：选了复制就复制；选了移动时照样检查种子还在不在做种，提示改选整理方式（不是改媒体库）
func TestChosenModeTorrent(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	src := filepath.Join(e.dlDir, oppName, oppName+".mkv")

	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash, Mode: models.MediaModeMove})
	require.NoError(t, err)
	assert.Contains(t, res.Plan.Problem, "种子还在做种，不能用移动整理")
	assert.Contains(t, res.Plan.Problem, "整理方式改选硬链接")
	assert.NotContains(t, res.Plan.Problem, "媒体库改用", "库本来就是硬链接")
	assert.True(t, exists(src), "源文件不动")
	rows := e.history()
	require.Len(t, rows, 1)
	assert.Equal(t, models.MediaTransferFailed, rows[0].Status)
	assert.Equal(t, models.MediaModeMove, rows[0].ChosenMode)

	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash, Mode: models.MediaModeCopy})
	require.NoError(t, err)
	require.Empty(t, res.Plan.Problem)
	assert.Equal(t, 1, res.Created)
	rows = e.history()
	require.Len(t, rows, 1, "还是同一条记录")
	assert.Equal(t, models.MediaTransferDone, rows[0].Status)
	assert.Equal(t, models.MediaModeCopy, rows[0].Mode)
	assert.Equal(t, models.MediaModeCopy, rows[0].ChosenMode)
	assert.False(t, sameFile(t, src, rows[0].TargetPath), "复制")
}

// 指定的路径和下载器里的种子同名时，选了移动不整理，提示改选整理方式；跟媒体库（库是移动）时提示改媒体库
func TestChosenModePathNameHitHint(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	name := "Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv"
	src := e.putIncoming(name, 5, 10*time.Minute)
	e.addTorrent("h1", name, map[string]int{name: 5}, func(tr *downloader.Torrent) {
		tr.SavePath, tr.ContentPath = `D:\Downloads`, `D:\Downloads\`+name
	})
	res, err := e.svc.Organize(e.ctx, Request{SourcePath: src, Mode: models.MediaModeMove})
	require.NoError(t, err)
	assert.Contains(t, res.Plan.Problem, "同名")
	assert.Contains(t, res.Plan.Problem, "整理方式改选硬链接")
	assert.NotContains(t, res.Plan.Problem, "媒体库改用")
	assert.True(t, exists(src))
}

// 选了整理方式的整理失败了（TMDB 暂时不能访问，还没选到媒体库）：记录记下选的方式，到期重试与「重试」都按它整理，不按媒体库的
func TestChosenModeRetry(t *testing.T) {
	for _, way := range []string{"到期重试", "重试"} {
		t.Run(way, func(t *testing.T) {
			e := newEnv(t)
			e.defaultLibraries()
			e.settings(SettingsInput{MinVideoMB: 1})
			src := e.putIncoming("Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv", 5, 10*time.Minute)
			e.tmdb.setDown(true)
			res, err := e.svc.Organize(e.ctx, Request{SourcePath: src, Mode: models.MediaModeCopy})
			require.NoError(t, err)
			require.Contains(t, res.Plan.Problem, "识别失败")
			rows := e.pathRows(src)
			require.Len(t, rows, 1)
			assert.Equal(t, models.MediaTransferFailed, rows[0].Status)
			assert.Equal(t, models.MediaModeCopy, rows[0].ChosenMode, "失败的记录也记下选的方式")
			require.NotNil(t, rows[0].NextRetryAt)

			e.tmdb.setDown(false)
			e.advance(11 * time.Minute)
			if way == "到期重试" {
				e.svc.retryDue(e.ctx, e.currentSettings())
				require.Equal(t, 1, e.drain())
			} else {
				res, err = e.svc.Retry(e.ctx, rows[0].ID)
				require.NoError(t, err)
				require.Empty(t, res.Plan.Problem)
				assert.Equal(t, 1, res.Created)
			}
			got := e.row(rows[0].ID)
			assert.Equal(t, models.MediaTransferDone, got.Status)
			assert.Equal(t, models.MediaModeCopy, got.Mode)
			assert.Equal(t, models.MediaModeCopy, got.ChosenMode)
			assert.True(t, exists(src))
			assert.False(t, sameFile(t, src, got.TargetPath), "按选的复制，不是媒体库的硬链接")
		})
	}
}

// 跟媒体库（没选整理方式）的失败了：重试按媒体库现在的方式，不按上次用的。库从移动改成硬链接以后「重试」就是硬链接
func TestFollowLibraryRetryUsesCurrentMode(t *testing.T) {
	e := newEnv(t)
	lib := e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Mode: models.MediaModeMove})
	e.settings(SettingsInput{MinVideoMB: 1})
	name := "Oppenheimer.2023.2160p.BluRay.x265-FRDS.mkv"
	src := e.putIncoming(name, 5, 10*time.Minute)
	e.addTorrent("h1", name, map[string]int{name: 5}, func(tr *downloader.Torrent) {
		tr.SavePath, tr.ContentPath = `D:\Downloads`, `D:\Downloads\`+name
	})
	res, err := e.svc.Organize(e.ctx, Request{SourcePath: src})
	require.NoError(t, err)
	require.Contains(t, res.Plan.Problem, "同名")
	require.Contains(t, res.Plan.Problem, "把媒体库改用硬链接")
	rows := e.pathRows(src)
	require.Len(t, rows, 1)
	assert.Equal(t, models.MediaModeMove, rows[0].Mode, "上次用的是库的移动")
	assert.Empty(t, rows[0].ChosenMode)

	_, err = e.svc.SaveLibrary(e.ctx, lib.ID, LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Mode: models.MediaModeHardlink, Enabled: true})
	require.NoError(t, err)
	res, err = e.svc.Retry(e.ctx, rows[0].ID)
	require.NoError(t, err)
	require.Empty(t, res.Plan.Problem)
	assert.Equal(t, 1, res.Created)
	got := e.row(rows[0].ID)
	assert.Equal(t, models.MediaTransferDone, got.Status)
	assert.Equal(t, models.MediaModeHardlink, got.Mode)
	assert.Empty(t, got.ChosenMode)
	assert.True(t, sameFile(t, src, got.TargetPath))
	assert.True(t, transfer.Within(e.movies, got.TargetPath), got.TargetPath)
}
