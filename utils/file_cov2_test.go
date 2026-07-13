package utils

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestParseTimeInCST covers the previously-uncovered ParseTimeInCST helper.
func TestParseTimeInCST(t *testing.T) {
	got, err := ParseTimeInCST("2006-01-02 15:04:05", "2024-05-01 12:00:00")
	require.NoError(t, err)
	// CST is UTC+8, so the parsed wall-clock time offset should be 8h.
	_, offset := got.Zone()
	require.Equal(t, 8*3600, offset)
	require.Equal(t, 2024, got.Year())
	require.Equal(t, time.Month(5), got.Month())

	// Invalid layout/value should surface a parse error.
	_, err = ParseTimeInCST("2006-01-02", "not-a-date")
	require.Error(t, err)
}

// TestCheckDirectory_NotExist covers the os.IsNotExist branch.
func TestCheckDirectory_NotExist(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	exists, empty, err := CheckDirectory(missing)
	require.NoError(t, err)
	require.False(t, exists)
	require.False(t, empty)
}

// TestCheckDirectory_ReadError covers the read-fail branch (exists but not readable).
func TestCheckDirectory_ReadError(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "restricted")
	require.NoError(t, os.MkdirAll(sub, 0o700))
	require.NoError(t, os.Chmod(sub, 0))
	t.Cleanup(func() { _ = os.Chmod(sub, 0o700) })

	exists, _, err := CheckDirectory(sub)
	if err == nil {
		t.Skip("filesystem allows reading 000 perms; skip read-error branch")
	}
	require.True(t, exists)
}

// TestDirectoryExists_NotExist covers os.IsNotExist branch returning false, nil.
func TestDirectoryExists_NotExist(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	ok, err := DirectoryExists(missing)
	require.NoError(t, err)
	require.False(t, ok)
}

// TestResolveDownloadBase_Empty covers the empty-dir error branch.
func TestResolveDownloadBase_Empty(t *testing.T) {
	_, err := ResolveDownloadBase(t.TempDir(), "work", "   ")
	require.Error(t, err)
}

// TestResolveDownloadBase_Relative covers the relative-path join + MkdirAll branch.
func TestResolveDownloadBase_Relative(t *testing.T) {
	home := t.TempDir()
	base, err := ResolveDownloadBase(home, "work", "sub/downloads")
	require.NoError(t, err)
	require.DirExists(t, base)
	require.Equal(t, filepath.Join(home, "work", "sub/downloads"), base)
}

// TestResolveDownloadBase_StatError covers the non-IsNotExist stat error branch:
// a parent path component that is a file makes os.Stat return ENOTDIR.
func TestResolveDownloadBase_StatError(t *testing.T) {
	dir := t.TempDir()
	fileAsParent := filepath.Join(dir, "afile")
	require.NoError(t, os.WriteFile(fileAsParent, []byte("x"), 0o644))

	// base = <file>/child -> stat returns a non-IsNotExist error (ENOTDIR)
	_, err := ResolveDownloadBase(dir, "afile", "child")
	require.Error(t, err)
}

// TestResolveDownloadBase_ExistingAbs covers the already-exists branch.
func TestResolveDownloadBase_ExistingAbs(t *testing.T) {
	dir := t.TempDir()
	base, err := ResolveDownloadBase("/ignored-home", "ignored-work", dir)
	require.NoError(t, err)
	require.Equal(t, dir, base)
}

// TestNewLocker_Error covers the OpenFile error branch (path in a nonexistent dir).
func TestNewLocker_Error(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "no-such-dir", "lock")
	_, err := NewLocker(bad)
	require.Error(t, err)
}
