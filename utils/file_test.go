package utils

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveDownloadBase(t *testing.T) {
	home := t.TempDir()
	base, err := ResolveDownloadBase(home, ".pt-tools", "downloads")
	require.NoError(t, err)
	require.DirExists(t, base)
}

func TestCheckDirectory(t *testing.T) {
	dir := t.TempDir()
	exists, empty, err := CheckDirectory(dir)
	require.NoError(t, err)
	require.True(t, exists)
	require.True(t, empty)
	f := filepath.Join(dir, "a.torrent")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))
	exists2, empty2, err := CheckDirectory(dir)
	require.NoError(t, err)
	require.True(t, exists2)
	require.False(t, empty2)
}

func TestCheckDirectory_OnFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	exists, empty, err := CheckDirectory(p)
	require.Error(t, err)
	require.False(t, exists)
	require.False(t, empty)
}

func TestDirectoryExists_OnFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	ok, err := DirectoryExists(p)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestDirectoryHelpers(t *testing.T) {
	dir := t.TempDir()
	exists, empty, err := CheckDirectory(dir)
	require.NoError(t, err)
	require.True(t, exists)
	require.True(t, empty)
	f := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))
	exists, empty, err = CheckDirectory(dir)
	require.NoError(t, err)
	require.True(t, exists)
	require.False(t, empty)
	ok, err := DirectoryExists(dir)
	require.NoError(t, err)
	require.True(t, ok)
	isEmpty, err := IsDirectoryEmpty(dir)
	require.NoError(t, err)
	require.False(t, isEmpty)
	abs, err := ResolveDownloadBase(dir, "work", dir)
	require.NoError(t, err)
	require.Equal(t, dir, abs)
}

func TestSubPathFromTag(t *testing.T) {
	got := SubPathFromTag("  tag ")
	require.Equal(t, "tag", got)
}

func TestIsDirectoryEmpty_OnUnreadable(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(p, 0o700))
	require.NoError(t, os.Chmod(p, 0))
	_, err := IsDirectoryEmpty(p)
	if err == nil {
		t.Skip("filesystem allows reading 000 perms; skip")
	}
	if _, ok := err.(*fs.PathError); !ok {
		t.Fatalf("expected *fs.PathError, got %T", err)
	}
}
