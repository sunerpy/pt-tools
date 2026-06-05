package cmd

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/crypto"
)

func TestSecretExportImportRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)
	os.Setenv("HOME", tmpDir)

	keyDir := filepath.Join(tmpDir, ".pt-tools")
	require.NoError(t, os.MkdirAll(keyDir, 0o700))

	keyPath := filepath.Join(keyDir, "secret.key")
	testKey := make([]byte, 32)
	for i := 0; i < 32; i++ {
		testKey[i] = byte(i)
	}

	keyHex := ""
	for _, b := range testKey {
		keyHex += "0" + string('0'+rune(b%10))
	}
	require.NoError(t, os.WriteFile(keyPath, []byte(keyHex), 0o600))

	exported, err := crypto.ExportKey()
	require.NoError(t, err)
	assert.Equal(t, len(exported), 32)

	encoded := base64.StdEncoding.EncodeToString(exported)
	assert.NotEmpty(t, encoded)

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	assert.Equal(t, decoded, exported)
}

func TestSecretImportRefusesOverwriteWithoutForce(t *testing.T) {
	tmpDir := t.TempDir()
	keyDir := filepath.Join(tmpDir, ".pt-tools")
	require.NoError(t, os.MkdirAll(keyDir, 0o700))

	keyPath := filepath.Join(keyDir, "secret.key")
	existingContent := "abcdef0123456789abcdef0123456789"
	require.NoError(t, os.WriteFile(keyPath, []byte(existingContent), 0o600))

	testKey := make([]byte, 32)
	for i := 0; i < 32; i++ {
		testKey[i] = byte(i)
	}
	encoded := base64.StdEncoding.EncodeToString(testKey)

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdin = r

	go func() {
		defer w.Close()
		w.WriteString(encoded + "\n")
	}()

	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)
	os.Setenv("HOME", tmpDir)

	err = runSecretImport(false)
	assert.Error(t, err)

	content, err := os.ReadFile(keyPath)
	require.NoError(t, err)
	assert.Equal(t, string(content), existingContent)
}

func TestSecretImportInvalidLength(t *testing.T) {
	tmpDir := t.TempDir()
	keyDir := filepath.Join(tmpDir, ".pt-tools")
	require.NoError(t, os.MkdirAll(keyDir, 0o700))

	shortKey := make([]byte, 16)
	for i := 0; i < 16; i++ {
		shortKey[i] = byte(i)
	}
	encoded := base64.StdEncoding.EncodeToString(shortKey)

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdin = r

	go func() {
		defer w.Close()
		w.WriteString(encoded + "\n")
	}()

	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)
	os.Setenv("HOME", tmpDir)

	err = runSecretImport(false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key length")
}

func TestSecretExportSize(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)
	os.Setenv("HOME", tmpDir)

	keyDir := filepath.Join(tmpDir, ".pt-tools")
	require.NoError(t, os.MkdirAll(keyDir, 0o700))

	keyPath := filepath.Join(keyDir, "secret.key")
	testKey := make([]byte, 32)
	for i := 0; i < 32; i++ {
		testKey[i] = byte((i * 7) % 256)
	}

	keyHex := ""
	for _, b := range testKey {
		keyHex += "0" + string('0'+rune(b%10))
	}
	require.NoError(t, os.WriteFile(keyPath, []byte(keyHex), 0o600))

	exported, err := crypto.ExportKey()
	require.NoError(t, err)

	encoded := base64.StdEncoding.EncodeToString(exported)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)

	assert.Equal(t, len(decoded), 32)
}

func TestSecretImportAtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	keyDir := filepath.Join(tmpDir, ".pt-tools")
	require.NoError(t, os.MkdirAll(keyDir, 0o700))

	keyPath := filepath.Join(keyDir, "secret.key")

	testKey := make([]byte, 32)
	for i := 0; i < 32; i++ {
		testKey[i] = byte(i)
	}
	encoded := base64.StdEncoding.EncodeToString(testKey)

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdin = r

	go func() {
		defer w.Close()
		w.WriteString(encoded + "\n")
	}()

	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)
	os.Setenv("HOME", tmpDir)

	err = runSecretImport(true)
	require.NoError(t, err)

	content, err := os.ReadFile(keyPath)
	require.NoError(t, err)

	assert.Equal(t, content, testKey)

	stat, err := os.Stat(keyPath)
	require.NoError(t, err)
	perms := stat.Mode().Perm()
	assert.Equal(t, perms, os.FileMode(0o600))
}
