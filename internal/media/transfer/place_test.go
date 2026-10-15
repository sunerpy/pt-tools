package transfer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlace(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	require.NoError(t, os.WriteFile(src, []byte("A"), 0o644))
	require.NoError(t, Place(src, dst))
	assert.False(t, exists(src))
	b, _ := os.ReadFile(dst)
	assert.Equal(t, "A", string(b))

	// 目标已存在：不覆盖
	require.NoError(t, os.WriteFile(src, []byte("A2"), 0o644))
	require.ErrorIs(t, Place(src, dst), ErrTargetExists)
	b, _ = os.ReadFile(dst)
	assert.Equal(t, "A", string(b))
}

// 不支持不覆盖的改名时用硬链接：放好以后 src 被换掉了就不删它（删的会是别人的文件）
func TestPlaceLinkFallback(t *testing.T) {
	oldRename, oldLink := placeRename, linkTmp
	t.Cleanup(func() { placeRename, linkTmp = oldRename, oldLink })
	placeRename = func(string, string) error { return errNoReplaceUnsupported }

	dir := t.TempDir()
	src, dst := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	require.NoError(t, os.WriteFile(src, []byte("A"), 0o644))
	require.NoError(t, Place(src, dst))
	assert.False(t, exists(src))
	assert.True(t, exists(dst))

	src2, dst2 := filepath.Join(dir, "c"), filepath.Join(dir, "d")
	require.NoError(t, os.WriteFile(src2, []byte("C"), 0o644))
	linkTmp = func(a, b string) error {
		if err := os.Link(a, b); err != nil {
			return err
		}
		// 硬链接放好以后、删原来的名字以前，别人换掉了 src
		if err := os.Remove(a); err != nil {
			return err
		}
		return os.WriteFile(a, []byte("someone else"), 0o644)
	}
	require.Error(t, Place(src2, dst2))
	b, _ := os.ReadFile(src2)
	assert.Equal(t, "someone else", string(b), "别人的文件留着")
	b, _ = os.ReadFile(dst2)
	assert.Equal(t, "C", string(b))

	linkTmp = func(string, string) error { return &os.LinkError{Op: "link", Err: os.ErrPermission} }
	require.Error(t, Place(filepath.Join(dir, "d"), filepath.Join(dir, "e")), "两种都不行时不冒险覆盖")
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}
