//go:build linux

package transfer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/sunerpy/pt-tools/models"
)

func TestCopyCloneErrnoFallback(t *testing.T) {
	for _, errno := range []unix.Errno{unix.EOPNOTSUPP, unix.EXDEV, unix.EINVAL, unix.ENOTTY, unix.ENOSYS} {
		t.Run(errno.Error(), func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "a.mkv")
			writeFile(t, src, "video data")
			fakeClone(t, func(*os.File, *os.File) error { return errno })
			dst := filepath.Join(dir, "lib", "A.mkv")
			_, d, err := TransferDetail(src, dst, models.MediaModeCopy)
			require.NoError(t, err)
			assert.False(t, d.Cloned)
			b, err := os.ReadFile(dst)
			require.NoError(t, err)
			assert.Equal(t, "video data", string(b))
		})
	}
	t.Run("other errno", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "a.mkv")
		writeFile(t, src, "video data")
		fakeClone(t, func(*os.File, *os.File) error { return unix.EIO })
		_, _, err := TransferDetail(src, filepath.Join(dir, "lib", "A.mkv"), models.MediaModeCopy)
		require.ErrorIs(t, err, unix.EIO)
	})
}

// TestRealCloneClassified 在这台机器的临时目录上真的调一次 FICLONE：支持写时复制的文件系统（Btrfs、XFS）成功，
// 不支持的（ext4、tmpfs、overlay 等）返回的错误要认成「不支持」，不然在这些文件系统上复制会失败。
func TestRealCloneClassified(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.mkv")
	writeFile(t, src, "video data")
	in, err := os.Open(src)
	require.NoError(t, err)
	defer in.Close()
	out, err := os.Create(filepath.Join(dir, "b.mkv"))
	require.NoError(t, err)
	defer out.Close()
	if cerr := cloneFile(out, in); cerr != nil {
		assert.True(t, cloneUnsupported(cerr), "FICLONE 的错误 %v 要认成不支持", cerr)
		return
	}
	b, err := os.ReadFile(out.Name())
	require.NoError(t, err)
	assert.Equal(t, "video data", string(b))
}
