package organize

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func rootPaths(roots []FSRoot) []string {
	var out []string
	for _, r := range roots {
		out = append(out, r.Path)
	}
	return out
}

func entryNames(l FSList) []string {
	var out []string
	for _, e := range l.Entries {
		out = append(out, e.Name)
	}
	return out
}

// 能浏览的根目录：媒体库、监控目录、路径映射、下载器的下载目录（按映射换成本地路径）；同一个目录合成一项，不存在的不回
func TestFSRoots(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	gone := filepath.Join(e.root, "gone")
	require.NoError(t, os.MkdirAll(gone, 0o755))
	e.library(LibraryInput{Name: "没了", Kind: models.MediaKindMovie, Path: gone})
	require.NoError(t, os.Remove(gone))
	incoming := filepath.Join(e.root, "incoming")
	require.NoError(t, os.MkdirAll(incoming, 0o755))
	_, err := e.svc.SaveWatchDir(e.ctx, 0, WatchDirInput{Path: incoming})
	require.NoError(t, err)
	require.NoError(t, e.db.Create(&[]models.DownloaderDirectory{
		{DownloaderID: 1, Path: "/downloads", Alias: "默认", IsDefault: true},
		{DownloaderID: 1, Path: "/nowhere"},
		{DownloaderID: 9, Path: e.root},
	}).Error)

	roots, err := e.svc.FSRoots(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{e.dlDir, incoming, e.movies, e.tv}, rootPaths(roots), "按路径排；删掉的下载器的目录不回")
	assert.Equal(t, FSRoot{Path: e.dlDir, Kinds: []string{fsRootPathMap, fsRootDownloader}, Labels: []string{"qb 默认"}}, roots[0])
	assert.Equal(t, []string{fsRootWatch}, roots[1].Kinds)
	assert.Equal(t, FSRoot{Path: e.movies, Kinds: []string{fsRootLibrary}, Labels: []string{"电影"}}, roots[2])
}

// 下载器的默认保存路径按路径映射换成本地路径列进根目录；等不到下载器回答时不列，也不等太久
func TestFSRootsDownloaderSavePath(t *testing.T) {
	e := newEnv(t)
	require.NoError(t, e.db.Where("1 = 1").Delete(&models.MediaPathMap{}).Error)
	_, err := e.svc.SavePathMap(e.ctx, 0, PathMapInput{DownloaderID: 1, DownloaderPrefix: "/downloads", LocalPrefix: e.root})
	require.NoError(t, err)
	e.dl.mu.Lock()
	e.dl.diskPath = "/downloads/downloads"
	e.dl.mu.Unlock()
	roots, err := e.svc.FSRoots(e.ctx)
	require.NoError(t, err)
	require.Len(t, roots, 2)
	assert.Equal(t, FSRoot{Path: e.root, Kinds: []string{fsRootPathMap}}, roots[0])
	assert.Equal(t, FSRoot{Path: e.dlDir, Kinds: []string{fsRootDownloader}, Labels: []string{"qb 默认"}}, roots[1])

	old := fsRootTimeout
	t.Cleanup(func() { fsRootTimeout = old })
	fsRootTimeout = 50 * time.Millisecond
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	e.dl.mu.Lock()
	e.dl.diskBlock = block
	e.dl.mu.Unlock()
	start := time.Now()
	roots, err = e.svc.FSRoots(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{e.root}, rootPaths(roots), "等不到下载器时不列它的保存路径")
	assert.Less(t, time.Since(start), 5*time.Second)
}

// 一层目录：目录在前、按名字排（不分大小写），隐藏的、系统目录与没下载完的不列；根目录没有上一级；根目录以外的不让看
func TestFSList(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	movie := filepath.Join(e.dlDir, "Movie.2023")
	for _, d := range []string{movie, filepath.Join(e.dlDir, "alpha"), filepath.Join(e.dlDir, ".hidden"), filepath.Join(e.dlDir, "@eaDir")} {
		require.NoError(t, os.MkdirAll(d, 0o755))
	}
	for name, size := range map[string]int{"b.mkv": 3, "A.mkv": 1, "c.mkv.part": 1, ".DS_Store": 1} {
		require.NoError(t, os.WriteFile(filepath.Join(e.dlDir, name), make([]byte, size), 0o644))
	}

	l, err := e.svc.FSList(e.ctx, e.dlDir+string(filepath.Separator))
	require.NoError(t, err)
	assert.Equal(t, e.dlDir, l.Path)
	assert.Equal(t, e.dlDir, l.Root)
	assert.Empty(t, l.Parent, "根目录没有上一级")
	assert.Equal(t, []string{"alpha", "Movie.2023", "A.mkv", "b.mkv"}, entryNames(l))
	assert.True(t, l.Entries[0].Dir)
	assert.Equal(t, int64(3), l.Entries[3].Size)
	assert.Equal(t, filepath.Join(e.dlDir, "b.mkv"), l.Entries[3].Path)
	assert.False(t, l.Truncated)

	l, err = e.svc.FSList(e.ctx, movie)
	require.NoError(t, err)
	assert.Equal(t, e.dlDir, l.Parent)
	assert.Empty(t, l.Entries)

	for _, tc := range []struct {
		dir  string
		want error
	}{
		{"", ErrInvalid},
		{"relative/dir", ErrInvalid},
		{e.root, ErrForbidden},
		{filepath.Join(e.dlDir, "nope"), ErrNotFound},
		{filepath.Join(e.dlDir, "A.mkv"), ErrInvalid},
	} {
		_, err := e.svc.FSList(e.ctx, tc.dir)
		require.ErrorIs(t, err, tc.want, tc.dir)
	}
}

// 一层最多列 maxFSEntries 项，多了写明
func TestFSListTruncated(t *testing.T) {
	e := newEnv(t)
	for i := range maxFSEntries + 1 {
		require.NoError(t, os.WriteFile(filepath.Join(e.dlDir, fmt.Sprintf("f%05d.mkv", i)), nil, 0o644))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(e.dlDir, "zzz"), 0o755))
	l, err := e.svc.FSList(e.ctx, e.dlDir)
	require.NoError(t, err)
	assert.True(t, l.Truncated)
	require.Len(t, l.Entries, maxFSEntries)
	assert.Equal(t, "zzz", l.Entries[0].Name, "目录排在前面，不会被截掉")
}

// 软链接按指向的算：指向根目录里的列出来、能进；指向根目录以外的不列、进不去
func TestFSListSymlinks(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	outside := filepath.Join(e.root, "secret")
	require.NoError(t, os.MkdirAll(outside, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "key.txt"), []byte("x"), 0o644))
	if err := os.Symlink(outside, filepath.Join(e.dlDir, "out")); err != nil {
		t.Skipf("建不了软链接：%v", err)
	}
	require.NoError(t, os.Symlink(e.movies, filepath.Join(e.dlDir, "lib")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "key.txt"), filepath.Join(e.dlDir, "key.txt")))
	require.NoError(t, os.Symlink(filepath.Join(e.root, "dangling"), filepath.Join(e.dlDir, "dangling")))

	l, err := e.svc.FSList(e.ctx, e.dlDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"lib"}, entryNames(l))
	assert.True(t, l.Entries[0].Dir)

	l, err = e.svc.FSList(e.ctx, filepath.Join(e.dlDir, "lib"))
	require.NoError(t, err, "指向另一个根目录")
	assert.Equal(t, e.movies, l.Root)
	assert.Empty(t, l.Parent, "就是那个根目录")
	_, err = e.svc.FSList(e.ctx, filepath.Join(e.dlDir, "out"))
	require.ErrorIs(t, err, ErrForbidden)
}

// 读不了库表或下载器时报错
func TestFSRootsErrors(t *testing.T) {
	e := newEnv(t)
	e.svc.cfg.Downloaders = failingDLs{fakeDLs: e.dls}
	_, err := e.svc.FSRoots(e.ctx)
	require.ErrorContains(t, err, "读取下载器失败")
	_, err = e.svc.FSList(e.ctx, e.dlDir)
	require.Error(t, err)

	e = newEnv(t)
	require.NoError(t, e.db.Migrator().DropTable(&models.DownloaderDirectory{}))
	_, err = e.svc.FSRoots(e.ctx)
	require.ErrorContains(t, err, "读取下载器的下载目录失败")
	for _, table := range []any{&models.MediaPathMap{}, &models.MediaWatchDir{}, &models.MediaLibrary{}} {
		require.NoError(t, e.db.Migrator().DropTable(table))
		_, err = e.svc.FSRoots(e.ctx)
		require.Error(t, err)
	}
}

type failingDLs struct{ *fakeDLs }

func (failingDLs) List(context.Context) ([]models.DownloaderSetting, error) {
	return nil, errors.New("数据库忙")
}
