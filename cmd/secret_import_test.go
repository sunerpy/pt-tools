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
