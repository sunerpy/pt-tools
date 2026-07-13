package cmd

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// feedStdin writes s to a pipe wired as os.Stdin for the duration of fn.
func feedStdin(t *testing.T, s string, fn func()) {
	t.Helper()
	old := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old })
	go func() { _, _ = w.WriteString(s); _ = w.Close() }()
	fn()
}

func TestRunSecretImport_SuccessForce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	enc := base64.StdEncoding.EncodeToString(key)

	var got error
	feedStdin(t, enc+"\n", func() {
		captureStdout(t, func() { got = runSecretImport(true) })
	})
	require.NoError(t, got)

	written, err := os.ReadFile(filepath.Join(home, ".pt-tools", "secret.key"))
	require.NoError(t, err)
	assert.Equal(t, key, written)
}

func TestRunSecretImport_RefusesOverwrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".pt-tools")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	existing := []byte("existing-key-content-do-not-clobber")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret.key"), existing, 0o600))

	key := make([]byte, 32)
	enc := base64.StdEncoding.EncodeToString(key)

	var got error
	feedStdin(t, enc+"\n", func() {
		captureStdout(t, func() { got = runSecretImport(false) })
	})
	require.Error(t, got)
	after, err := os.ReadFile(filepath.Join(dir, "secret.key"))
	require.NoError(t, err)
	assert.Equal(t, existing, after, "must not overwrite without force")
}

func TestRunSecretImport_InvalidBase64(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var got error
	feedStdin(t, "@@@not-base64@@@\n", func() {
		captureStdout(t, func() { got = runSecretImport(true) })
	})
	require.Error(t, got)
}

func TestRunSecretImport_WrongLength(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	short := base64.StdEncoding.EncodeToString(make([]byte, 16))
	var got error
	feedStdin(t, short+"\n", func() {
		captureStdout(t, func() { got = runSecretImport(true) })
	})
	require.Error(t, got)
	assert.Contains(t, got.Error(), "invalid key length")
}

var (
	_ = cobra.Command{}
	_ = errors.New
)

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
