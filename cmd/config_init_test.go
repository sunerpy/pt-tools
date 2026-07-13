package cmd

import (
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
