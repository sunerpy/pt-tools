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

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "passkey masked",
			input:    "https://rss.example.com/rss?passkey=abc123&other=value",
			expected: "https://rss.example.com/rss?other=value&passkey=%2A%2A%2A",
		},
		{
			name:     "sign masked",
			input:    "https://rss.m-team.cc/api/rss/fetch?sign=259dcb690912c699776d0a51a9145597",
			expected: "https://rss.m-team.cc/api/rss/fetch?sign=%2A%2A%2A",
		},
		{
			name:     "multiple sensitive params",
			input:    "https://example.com?passkey=secret&apikey=token123&normal=ok",
			expected: "https://example.com?apikey=%2A%2A%2A&normal=ok&passkey=%2A%2A%2A",
		},
		{
			name:     "no sensitive params unchanged",
			input:    "https://example.com/rss?category=movie&limit=50",
			expected: "https://example.com/rss?category=movie&limit=50",
		},
		{
			name:     "no query params unchanged",
			input:    "https://example.com/rss",
			expected: "https://example.com/rss",
		},
		{
			name:     "invalid url returns placeholder",
			input:    "://invalid",
			expected: "<invalid-url>",
		},
		{
			name:     "case insensitive matching",
			input:    "https://example.com?PassKey=secret&APIKEY=token",
			expected: "https://example.com?APIKEY=%2A%2A%2A&PassKey=%2A%2A%2A",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeURL(tt.input)
			require.Equal(t, tt.expected, got)
		})
	}
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
