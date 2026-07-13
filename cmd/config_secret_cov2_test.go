package cmd

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChekcAndInitDownloadPath_MkdirConfigError drives the first MkdirAll error
// branch: the config dir does not exist yet, but its parent is read-only so the
// MkdirAll fails with EACCES.
func TestChekcAndInitDownloadPath_MkdirConfigError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission bits are bypassed")
	}
	parent := t.TempDir()
	require.NoError(t, os.Chmod(parent, 0o555))
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	err := chekcAndInitDownloadPath(filepath.Join(parent, "cfg"))
	if err == nil {
		t.Skip("filesystem permitted mkdir despite read-only parent; skip")
	}
	assert.Contains(t, err.Error(), "无法创建工作目录")
}

// TestChekcAndInitDownloadPath_MkdirDownloadError drives the second MkdirAll
// error branch: the config dir already exists but is read-only, so creating the
// downloads subdir fails with EACCES.
func TestChekcAndInitDownloadPath_MkdirDownloadError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission bits are bypassed")
	}
	dir := filepath.Join(t.TempDir(), "cfg")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := chekcAndInitDownloadPath(dir)
	if err == nil {
		t.Skip("filesystem permitted mkdir despite read-only dir; skip")
	}
	assert.Contains(t, err.Error(), "无法创建下载目录")
}

// TestRunSecretImport_RenameError covers the os.Rename(tmpPath,keyPath) failure
// branch: keyPath already exists as a directory, so the atomic rename of the
// temp key file over it fails.
func TestRunSecretImport_RenameError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	keyDir := filepath.Join(home, ".pt-tools")
	require.NoError(t, os.MkdirAll(filepath.Join(keyDir, "secret.key"), 0o755))

	key := make([]byte, 32)
	enc := base64.StdEncoding.EncodeToString(key)

	var got error
	feedStdin(t, enc+"\n", func() {
		captureStdout(t, func() { got = runSecretImport(true) })
	})
	require.Error(t, got)
}

// TestRunSecretImport_StdinReadError covers the stdin-read-error branch by
// pointing os.Stdin at a directory, whose Read returns a non-EOF error.
func TestRunSecretImport_StdinReadError(t *testing.T) {
	d, err := os.Open(t.TempDir())
	require.NoError(t, err)
	old := os.Stdin
	os.Stdin = d
	t.Cleanup(func() { os.Stdin = old; _ = d.Close() })

	var got error
	captureStdout(t, func() { got = runSecretImport(true) })
	require.Error(t, got)
}

// TestRunSecretImport_HomeError covers runSecretImport's UserHomeDir error
// branch by clearing HOME so os.UserHomeDir fails after a valid 32-byte key is
// decoded.
func TestRunSecretImport_HomeError(t *testing.T) {
	t.Setenv("HOME", "")
	key := make([]byte, 32)
	enc := base64.StdEncoding.EncodeToString(key)

	var got error
	feedStdin(t, enc+"\n", func() {
		captureStdout(t, func() { got = runSecretImport(true) })
	})
	require.Error(t, got)
}

// TestRunSecretImport_WriteTempError covers the WriteFile(tmpPath) failure
// branch: ~/.pt-tools already exists but is read-only, so MkdirAll succeeds
// (dir present) yet writing the temp key file into it fails with EACCES.
func TestRunSecretImport_WriteTempError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission bits are bypassed")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	keyDir := filepath.Join(home, ".pt-tools")
	require.NoError(t, os.MkdirAll(keyDir, 0o755))
	require.NoError(t, os.Chmod(keyDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(keyDir, 0o755) })

	key := make([]byte, 32)
	enc := base64.StdEncoding.EncodeToString(key)

	var got error
	feedStdin(t, enc+"\n", func() {
		captureStdout(t, func() { got = runSecretImport(true) })
	})
	if got == nil {
		t.Skip("filesystem permitted write despite read-only dir; skip")
	}
	require.Error(t, got)
}

// TestRunSecretImport_MkdirError covers the MkdirAll(keyDir) failure branch:
// HOME points at a regular file so ~/.pt-tools cannot be created.
func TestRunSecretImport_MkdirError(t *testing.T) {
	dir := t.TempDir()
	homeFile := filepath.Join(dir, "home-as-file")
	require.NoError(t, os.WriteFile(homeFile, []byte("x"), 0o644))
	t.Setenv("HOME", homeFile)

	key := make([]byte, 32)
	enc := base64.StdEncoding.EncodeToString(key)

	var got error
	feedStdin(t, enc+"\n", func() {
		captureStdout(t, func() { got = runSecretImport(true) })
	})
	require.Error(t, got)
}
