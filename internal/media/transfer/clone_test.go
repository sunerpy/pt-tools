package transfer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// fakeClone 把 clone 换成 f，测试结束换回来。
func fakeClone(t *testing.T, f func(dst, src *os.File) error) {
	t.Helper()
	old := clone
	t.Cleanup(func() { clone = old })
	clone = f
}

// leftovers 是目录里 copyFile 留下的临时文件。
func leftovers(t *testing.T, dir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, "*.pt-tools-*.part"))
	require.NoError(t, err)
	return m
}

func TestCopyUsesClone(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.mkv")
	writeFile(t, src, "video data")
	old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	require.NoError(t, os.Chtimes(src, old, old))
	dst := filepath.Join(dir, "lib", "A", "A.mkv")

	calls := 0
	fakeClone(t, func(out, in *os.File) error {
		calls++
		assert.Equal(t, src, in.Name(), "从源文件克隆")
		// 写一段和源文件不同的内容：克隆以后还逐字节复制的话会被盖掉
		_, err := out.WriteString("cloned")
		return err
	})
	out, d, err := TransferDetail(src, dst, models.MediaModeCopy)
	require.NoError(t, err)
	assert.Equal(t, Created, out)
	assert.True(t, d.Cloned, "克隆成功时报告写时复制")
	assert.Equal(t, 1, calls)
	b, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "cloned", string(b), "克隆成功以后不再逐字节复制")
	info, err := os.Stat(dst)
	require.NoError(t, err)
	assert.True(t, info.ModTime().Equal(old), "修改时间跟源文件一样")
	assert.Empty(t, leftovers(t, filepath.Dir(dst)))

	// 硬链接、软链接不试克隆
	for _, mode := range []string{models.MediaModeHardlink, models.MediaModeSymlink} {
		_, d, err := TransferDetail(src, filepath.Join(dir, "lib", mode, "A.mkv"), mode)
		require.NoError(t, err)
		assert.False(t, d.Cloned, mode)
	}
	assert.Equal(t, 1, calls, "只有复制试克隆")
}

func TestCopyCloneFailsOtherwise(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.mkv")
	writeFile(t, src, "video data")
	dst := filepath.Join(dir, "lib", "A.mkv")
	boom := errors.New("磁盘坏了")
	fakeClone(t, func(*os.File, *os.File) error { return boom })

	_, d, err := TransferDetail(src, dst, models.MediaModeCopy)
	require.ErrorIs(t, err, boom, "不是「不支持」的错误时当作复制失败")
	assert.Contains(t, err.Error(), "复制失败")
	assert.False(t, d.Cloned)
	_, serr := os.Lstat(dst)
	assert.ErrorIs(t, serr, os.ErrNotExist, "失败时库里不留文件")
	assert.Empty(t, leftovers(t, filepath.Dir(dst)), "临时文件删掉")
}

func TestCopyCloneUnsupportedFallsBack(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.mkv")
	writeFile(t, src, "video data")
	fakeClone(t, func(out, _ *os.File) error {
		// 克隆失败时目标多半没动；以防万一写进一段，回退以后要从头复制
		_, _ = out.WriteString("partial garbage that is longer than the source")
		return errCloneUnsupported
	})
	dst := filepath.Join(dir, "lib", "A.mkv")
	out, d, err := TransferDetail(src, dst, models.MediaModeCopy)
	require.NoError(t, err)
	assert.Equal(t, Created, out)
	assert.False(t, d.Cloned)
	b, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "video data", string(b), "回退时清空、从头逐字节复制")
}

func TestTransferKeepsSignature(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.mkv")
	writeFile(t, src, "video data")
	fakeClone(t, func(out, _ *os.File) error {
		_, err := out.WriteString("cloned")
		return err
	})
	out, err := Transfer(src, filepath.Join(dir, "lib", "A.mkv"), models.MediaModeCopy)
	require.NoError(t, err)
	assert.Equal(t, Created, out, "Transfer 照旧只回结果")
}

// 文件系统不支持硬链接时放到目标位置的另外两条路（不覆盖的改名、独占新建）照样报告写时复制。
func TestCopyClonedReportedOnEveryPlacement(t *testing.T) {
	oldLink, oldNR := linkTmp, noReplace
	t.Cleanup(func() { linkTmp, noReplace = oldLink, oldNR })
	linkTmp = func(string, string) error { return &os.LinkError{Op: "link", Err: errors.New("不支持硬链接")} }
	cloned := 0
	fakeClone(t, func(out, in *os.File) error {
		cloned++
		_, err := out.WriteString("cloned:" + filepath.Ext(in.Name()))
		return err
	})
	dir := t.TempDir()
	src := filepath.Join(dir, "a.mkv")
	writeFile(t, src, "video data")

	// Linux 上用不覆盖的改名放过去（别的系统改名不支持，独占新建）
	noReplace = renameNoReplace
	_, d, err := TransferDetail(src, filepath.Join(dir, "lib1", "a.mkv"), models.MediaModeCopy)
	require.NoError(t, err)
	assert.True(t, d.Cloned)
	assert.Positive(t, cloned)

	noReplace = func(string, string) error { return errNoReplaceUnsupported }
	_, d, err = TransferDetail(src, filepath.Join(dir, "lib2", "a.mkv"), models.MediaModeCopy)
	require.NoError(t, err)
	assert.True(t, d.Cloned, "独占新建时从临时文件克隆")
	assert.Equal(t, "cloned:.part", readFileT(t, filepath.Join(dir, "lib2", "a.mkv")), "独占新建时克隆的是写好的临时文件")
}
