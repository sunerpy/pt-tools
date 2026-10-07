package transfer

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

func sameFile(t *testing.T, a, b string) bool {
	t.Helper()
	ai, err := os.Stat(a)
	require.NoError(t, err)
	bi, err := os.Stat(b)
	require.NoError(t, err)
	return os.SameFile(ai, bi)
}

func TestHardlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dl", "Movie.2023.mkv")
	dst := filepath.Join(dir, "lib", "Movie (2023)", "Movie (2023).mkv")
	writeFile(t, src, "video")

	out, err := Transfer(src, dst, "")
	require.NoError(t, err)
	assert.Equal(t, Created, out)
	assert.True(t, sameFile(t, src, dst), "空串当作硬链接")

	out, err = Transfer(src, dst, models.MediaModeHardlink)
	require.NoError(t, err)
	assert.Equal(t, AlreadyDone, out, "同一个 inode 跳过")

	other := filepath.Join(dir, "dl", "Other.mkv")
	writeFile(t, other, "video")
	_, err = Transfer(other, dst, models.MediaModeHardlink)
	assert.ErrorIs(t, err, ErrTargetExists, "目标是别的文件时不覆盖")
	b, _ := os.ReadFile(dst)
	assert.Equal(t, "video", string(b))

	id1, err := FileID(src)
	require.NoError(t, err)
	id2, err := FileID(dst)
	require.NoError(t, err)
	assert.Equal(t, id1, id2)
	id3, err := FileID(other)
	require.NoError(t, err)
	assert.NotEqual(t, id1, id3)
}

func TestHardlinkCrossDevice(t *testing.T) {
	src := filepath.Join(t.TempDir(), "a.mkv")
	writeFile(t, src, "video")

	t.Run("injected", func(t *testing.T) {
		old := link
		t.Cleanup(func() { link = old })
		link = func(string, string) error { return &os.LinkError{Op: "link", Err: syscall.EXDEV} }
		_, err := Transfer(src, filepath.Join(t.TempDir(), "x", "a.mkv"), models.MediaModeHardlink)
		require.ErrorIs(t, err, ErrCrossDevice)
		assert.Contains(t, err.Error(), "同一个分区")
	})

	t.Run("real", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("只在 Linux 上用 /dev/shm 测")
		}
		other, err := os.MkdirTemp("/dev/shm", "pt-tools-xdev-")
		if err != nil {
			t.Skipf("/dev/shm 不可写: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(other) })
		si, _ := os.Stat(filepath.Dir(src))
		oi, _ := os.Stat(other)
		if si.Sys().(*syscall.Stat_t).Dev == oi.Sys().(*syscall.Stat_t).Dev {
			t.Skip("临时目录与 /dev/shm 在同一个文件系统")
		}
		_, err = Transfer(src, filepath.Join(other, "lib", "a.mkv"), models.MediaModeHardlink)
		require.ErrorIs(t, err, ErrCrossDevice)

		// 复制与移动跨文件系统照样能做
		out, err := Transfer(src, filepath.Join(other, "copy", "a.mkv"), models.MediaModeCopy)
		require.NoError(t, err)
		assert.Equal(t, Created, out)
		mv := filepath.Join(filepath.Dir(src), "mv.mkv")
		writeFile(t, mv, "move me")
		out, err = Transfer(mv, filepath.Join(other, "mv", "mv.mkv"), models.MediaModeMove)
		require.NoError(t, err)
		assert.Equal(t, Created, out)
		_, err = os.Stat(mv)
		assert.ErrorIs(t, err, os.ErrNotExist, "跨文件系统移动是复制后删掉源文件")
		b, _ := os.ReadFile(filepath.Join(other, "mv", "mv.mkv"))
		assert.Equal(t, "move me", string(b))
	})
}

func TestCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.mkv")
	writeFile(t, src, "video data")
	old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	require.NoError(t, os.Chtimes(src, old, old))
	dst := filepath.Join(dir, "lib", "A", "A.mkv")

	out, err := Transfer(src, dst, models.MediaModeCopy)
	require.NoError(t, err)
	assert.Equal(t, Created, out)
	assert.False(t, sameFile(t, src, dst), "复制出的是另一个文件")
	b, _ := os.ReadFile(dst)
	assert.Equal(t, "video data", string(b))
	info, _ := os.Stat(dst)
	assert.True(t, info.ModTime().Equal(old), "修改时间跟源文件一样")
	left, _ := filepath.Glob(filepath.Join(dir, "lib", "A", "*.part"))
	assert.Empty(t, left, "不留临时文件")

	_, err = Transfer(src, dst, models.MediaModeCopy)
	assert.ErrorIs(t, err, ErrTargetExists, "复制出的文件认不出是不是自己放的：目标已存在就不动，哪怕大小相同")
	b, _ = os.ReadFile(dst)
	assert.Equal(t, "video data", string(b))
}

func TestCopyWithoutHardlinkSupport(t *testing.T) {
	oldLink, oldNR := linkTmp, noReplace
	t.Cleanup(func() { linkTmp, noReplace = oldLink, oldNR })
	linkTmp = func(string, string) error { return &os.LinkError{Op: "link", Err: syscall.EPERM} }
	dir := t.TempDir()
	src := filepath.Join(dir, "a.mkv")
	writeFile(t, src, "video data")

	// 文件系统不支持硬链接：用不覆盖的改名放到目标位置（Linux）或独占新建
	out, err := Transfer(src, filepath.Join(dir, "lib1", "a.mkv"), models.MediaModeCopy)
	require.NoError(t, err)
	assert.Equal(t, Created, out)
	assert.Equal(t, "video data", readFileT(t, filepath.Join(dir, "lib1", "a.mkv")))

	noReplace = func(string, string) error { return errNoReplaceUnsupported }
	out, err = Transfer(src, filepath.Join(dir, "lib2", "a.mkv"), models.MediaModeCopy)
	require.NoError(t, err)
	assert.Equal(t, Created, out, "改名也不支持时独占新建再复制")
	assert.Equal(t, "video data", readFileT(t, filepath.Join(dir, "lib2", "a.mkv")))

	// 放到目标位置之前目标出现了：两种方式都不覆盖
	info, err := os.Stat(src)
	require.NoError(t, err)
	for _, nr := range []func(string, string) error{oldNR, func(string, string) error { return errNoReplaceUnsupported }} {
		noReplace = nr
		dst := filepath.Join(t.TempDir(), "a.mkv")
		writeFile(t, dst, "someone else")
		require.ErrorIs(t, copyFile(src, dst, info), ErrTargetExists)
		assert.Equal(t, "someone else", readFileT(t, dst))
		left, _ := filepath.Glob(filepath.Join(filepath.Dir(dst), "*.part"))
		assert.Empty(t, left, "不留临时文件")
	}
}

func readFileT(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	return string(b)
}

func TestCopyRefusesClobberRace(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.mkv")
	writeFile(t, src, "video")
	dst := filepath.Join(dir, "lib", "a.mkv")
	info, err := os.Stat(src)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o755))
	// 复制途中目标出现了：不覆盖
	writeFile(t, dst, "someone else")
	err = copyFile(src, dst, info)
	require.ErrorIs(t, err, ErrTargetExists)
	b, _ := os.ReadFile(dst)
	assert.Equal(t, "someone else", string(b))
	left, _ := filepath.Glob(filepath.Join(dir, "lib", "*.part"))
	assert.Empty(t, left)
}

func TestSymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.mkv")
	writeFile(t, src, "video")
	dst := filepath.Join(dir, "lib", "a.mkv")

	out, err := Transfer(src, dst, models.MediaModeSymlink)
	require.NoError(t, err)
	assert.Equal(t, Created, out)
	to, err := os.Readlink(dst)
	require.NoError(t, err)
	assert.Equal(t, src, to)

	out, err = Transfer(src, dst, models.MediaModeSymlink)
	require.NoError(t, err)
	assert.Equal(t, AlreadyDone, out)

	other := filepath.Join(dir, "b.mkv")
	writeFile(t, other, "video")
	_, err = Transfer(other, dst, models.MediaModeSymlink)
	assert.ErrorIs(t, err, ErrTargetExists, "指向别的文件的软链接不覆盖")
}

func TestMove(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dl", "a.mkv")
	writeFile(t, src, "video")
	dst := filepath.Join(dir, "lib", "a.mkv")

	out, err := Transfer(src, dst, models.MediaModeMove)
	require.NoError(t, err)
	assert.Equal(t, Created, out)
	_, err = os.Stat(src)
	assert.ErrorIs(t, err, os.ErrNotExist)

	_, err = Transfer(src, dst, models.MediaModeMove)
	require.ErrorIs(t, err, ErrSourceMissing, "源没了、目标在：认不出目标是不是自己移过去的，不当作完成")
	writeFile(t, src, "again")
	_, err = Transfer(src, dst, models.MediaModeMove)
	require.ErrorIs(t, err, ErrTargetExists, "目标已存在时不覆盖")
	assert.Equal(t, "video", readFileT(t, dst))

	_, err = Transfer(filepath.Join(dir, "nope.mkv"), filepath.Join(dir, "lib", "nope.mkv"), models.MediaModeMove)
	assert.ErrorIs(t, err, ErrSourceMissing)
}

func TestMoveCrossDeviceInjected(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.mkv")
	writeFile(t, src, "video")
	old := rename
	t.Cleanup(func() { rename = old })
	rename = func(string, string) error { return &os.LinkError{Op: "rename", Err: syscall.EXDEV} }
	out, err := Transfer(src, filepath.Join(dir, "lib", "a.mkv"), models.MediaModeMove)
	require.NoError(t, err)
	assert.Equal(t, Created, out)
	_, err = os.Stat(src)
	assert.ErrorIs(t, err, os.ErrNotExist)

	rename = func(string, string) error { return errors.New("boom") }
	writeFile(t, src, "video")
	_, err = Transfer(src, filepath.Join(dir, "lib2", "a.mkv"), models.MediaModeMove)
	require.ErrorContains(t, err, "移动失败")
}

func TestTransferErrors(t *testing.T) {
	dir := t.TempDir()
	_, err := Transfer(filepath.Join(dir, "none.mkv"), filepath.Join(dir, "x.mkv"), "")
	assert.ErrorIs(t, err, ErrSourceMissing)
	_, err = Transfer(dir, filepath.Join(dir, "lib", "x"), "")
	assert.ErrorContains(t, err, "不是普通文件")
	_, err = Transfer(filepath.Join(dir, "a"), filepath.Join(dir, "b"), "rsync")
	assert.ErrorIs(t, err, ErrBadMode)

	src := filepath.Join(dir, "a.mkv")
	writeFile(t, src, "v")
	blocker := filepath.Join(dir, "file")
	writeFile(t, blocker, "x")
	_, err = Transfer(src, filepath.Join(blocker, "sub", "a.mkv"), "")
	assert.ErrorContains(t, err, "检查目标失败", "路径中间有一级是文件")

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		return
	}
	ro := filepath.Join(dir, "ro")
	require.NoError(t, os.Mkdir(ro, 0o555))
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	_, err = Transfer(src, filepath.Join(ro, "sub", "a.mkv"), "")
	assert.ErrorContains(t, err, "建立目录失败")
}

func TestRemoveIfOurs(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dl", "a.mkv")
	writeFile(t, src, "video")
	dst := filepath.Join(dir, "lib", "Show", "Season 1", "a.mkv")
	_, err := Transfer(src, dst, models.MediaModeHardlink)
	require.NoError(t, err)
	id, err := FileID(dst)
	require.NoError(t, err)

	require.Error(t, RemoveIfOurs(dst, models.MediaModeHardlink, "", src), "没有记下文件编号时不删")
	require.Error(t, RemoveIfOurs(dst, models.MediaModeHardlink, "1:2", src), "文件编号对不上时不删")
	require.NoError(t, RemoveIfOurs(dst, models.MediaModeHardlink, id, src))
	_, err = os.Stat(dst)
	assert.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, RemoveIfOurs(dst, models.MediaModeHardlink, id, src), "已经不在了")
	_, err = os.Stat(src)
	require.NoError(t, err, "源文件不动")

	RemoveEmptyDirs(filepath.Dir(dst), filepath.Join(dir, "lib"))
	_, err = os.Stat(filepath.Join(dir, "lib", "Show"))
	assert.ErrorIs(t, err, os.ErrNotExist, "空目录往上删到库目录为止")
	_, err = os.Stat(filepath.Join(dir, "lib"))
	require.NoError(t, err, "库目录本身不删")

	ln := filepath.Join(dir, "lib", "b.mkv")
	_, err = Transfer(src, ln, models.MediaModeSymlink)
	require.NoError(t, err)
	require.Error(t, RemoveIfOurs(ln, models.MediaModeSymlink, "", filepath.Join(dir, "other.mkv")))
	require.NoError(t, RemoveIfOurs(ln, models.MediaModeSymlink, "", src))
	_, err = os.Lstat(ln)
	assert.ErrorIs(t, err, os.ErrNotExist)

	sub := filepath.Join(dir, "lib", "keep", "x.mkv")
	writeFile(t, sub, "x")
	writeFile(t, filepath.Join(dir, "lib", "keep", "other.txt"), "y")
	RemoveEmptyDirs(filepath.Dir(sub), filepath.Join(dir, "lib"))
	_, err = os.Stat(filepath.Dir(sub))
	require.NoError(t, err, "不空的目录不删")
}

func TestWithin(t *testing.T) {
	assert.True(t, Within("/media/movies", "/media/movies/A/a.mkv"))
	assert.False(t, Within("/media/movies", "/media/movies"))
	assert.False(t, Within("/media/movies", "/media/movies2/a.mkv"))
	assert.False(t, Within("/media/movies", "/media/movies/../x"))
	assert.False(t, Within("/media/movies", "/media"))
}
