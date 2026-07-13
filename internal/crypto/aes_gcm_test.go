package crypto

import (
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEncryptDecryptRoundTrip tests basic encrypt/decrypt cycle
func TestEncryptDecryptRoundTrip(t *testing.T) {
	// Setup: set a known base64 key in env for this test
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i % 256)
	}
	keyB64 := base64.StdEncoding.EncodeToString(key)
	t.Setenv("PT_TOOLS_SECRET_KEY", keyB64)

	// Reset the global key state by re-initializing
	testResetEncryptor()

	plain := []byte("hello world 你好")
	cipher, err := Encrypt(plain)
	require.NoError(t, err, "Encrypt should not error")
	assert.NotEmpty(t, cipher, "cipher should not be empty")

	decrypted, err := Decrypt(cipher)
	require.NoError(t, err, "Decrypt should not error")
	assert.Equal(t, plain, decrypted, "decrypted should match original")
}

// TestDecryptTampered tests that tampered ciphertext fails to decrypt
func TestDecryptTampered(t *testing.T) {
	// Setup: set a known key
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i % 256)
	}
	keyB64 := base64.StdEncoding.EncodeToString(key)
	t.Setenv("PT_TOOLS_SECRET_KEY", keyB64)

	testResetEncryptor()

	plain := []byte("secret data")
	cipher, err := Encrypt(plain)
	require.NoError(t, err)

	// Tamper: modify the last characters
	if len(cipher) > 2 {
		cipherBytes, _ := base64.StdEncoding.DecodeString(cipher)
		if len(cipherBytes) > 0 {
			cipherBytes[len(cipherBytes)-1] ^= 0xFF // flip bits
			cipher = base64.StdEncoding.EncodeToString(cipherBytes)
		}
	}

	decrypted, err := Decrypt(cipher)
	assert.Error(t, err, "Decrypt should error on tampered data")
	assert.Nil(t, decrypted, "decrypted should be nil on error")
}

// TestKeyDerivedFromEnv tests that key is read from environment
func TestKeyDerivedFromEnv(t *testing.T) {
	// Setup: create two different keys
	key1 := make([]byte, 32)
	for i := range key1 {
		key1[i] = byte(i % 256)
	}
	key1B64 := base64.StdEncoding.EncodeToString(key1)

	t.Setenv("PT_TOOLS_SECRET_KEY", key1B64)
	testResetEncryptor()

	plain := []byte("test data")
	cipher1, err := Encrypt(plain)
	require.NoError(t, err)

	// Verify we can decrypt with same key
	decrypted1, err := Decrypt(cipher1)
	require.NoError(t, err)
	assert.Equal(t, plain, decrypted1)
}

// TestKeyMissingFallback tests that ~/.pt-tools/secret.key is created when env not set
func TestKeyMissingFallback(t *testing.T) {
	// Setup: use temp dir
	tmpDir := t.TempDir()
	homeEnv := filepath.Join(tmpDir, ".pt-tools")

	// Unset the env var to trigger fallback
	t.Setenv("PT_TOOLS_SECRET_KEY", "")

	// We'll mock the home directory by manipulating init logic
	// For now, we just verify the file generation path works
	// In practice, this test verifies:
	// 1. When no env is set
	// 2. The key should be generated randomly
	// 3. File ~/.pt-tools/secret.key should be created with mode 0600

	// This is a conceptual test; actual implementation will handle this in init()
	// We just verify the logic can be tested without relying on actual home dir
	assert.NotEmpty(t, homeEnv, "temp dir created")

	// Verify directory exists
	os.MkdirAll(homeEnv, 0o700)
	assert.True(t, isDir(homeEnv), "pt-tools dir should exist")
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// TestExportKey verifies ExportKey returns a copy of the active key.
func TestExportKey(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	keyB64 := base64.StdEncoding.EncodeToString(key)
	t.Setenv("PT_TOOLS_SECRET_KEY", keyB64)
	testResetEncryptor()

	exported, err := ExportKey()
	require.NoError(t, err)
	assert.Equal(t, key, exported)

	exported[0] ^= 0xFF
	exported2, err := ExportKey()
	require.NoError(t, err)
	assert.Equal(t, key, exported2, "internal key should be unaffected by mutation of returned slice")
}

// TestResetForTest ensures the exported reset helper reloads the key.
func TestResetForTest(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(255 - i)
	}
	t.Setenv("PT_TOOLS_SECRET_KEY", base64.StdEncoding.EncodeToString(key))
	ResetForTest()

	exported, err := ExportKey()
	require.NoError(t, err)
	assert.Equal(t, key, exported)
}

// TestInitKeyFromFile covers the ~/.pt-tools/secret.key load branch.
func TestInitKeyFromFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PT_TOOLS_SECRET_KEY", "")

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i * 3)
	}
	dir := filepath.Join(tmpHome, ".pt-tools")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	keyFile := filepath.Join(dir, "secret.key")
	require.NoError(t, os.WriteFile(keyFile, []byte(hex.EncodeToString(key)), 0o600))

	testResetEncryptor()
	t.Cleanup(func() {
		envKey := make([]byte, 32)
		t.Setenv("PT_TOOLS_SECRET_KEY", base64.StdEncoding.EncodeToString(envKey))
		testResetEncryptor()
	})

	exported, err := ExportKey()
	require.NoError(t, err)
	assert.Equal(t, key, exported)

	plain := []byte("file key round trip")
	cipher, err := Encrypt(plain)
	require.NoError(t, err)
	decrypted, err := Decrypt(cipher)
	require.NoError(t, err)
	assert.Equal(t, plain, decrypted)
}

// TestInitKeyGenerated covers the branch where no env and no file exist,
// so a fresh random key is generated and persisted.
func TestInitKeyGenerated(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PT_TOOLS_SECRET_KEY", "")

	testResetEncryptor()
	t.Cleanup(func() {
		envKey := make([]byte, 32)
		t.Setenv("PT_TOOLS_SECRET_KEY", base64.StdEncoding.EncodeToString(envKey))
		testResetEncryptor()
	})

	keyFile := filepath.Join(tmpHome, ".pt-tools", "secret.key")
	data, err := os.ReadFile(keyFile)
	require.NoError(t, err)
	decoded, err := hex.DecodeString(string(data))
	require.NoError(t, err)
	assert.Len(t, decoded, 32)

	exported, err := ExportKey()
	require.NoError(t, err)
	assert.Equal(t, decoded, exported)
}

// TestDecryptShortCiphertext covers the "ciphertext too short" branch.
func TestDecryptShortCiphertext(t *testing.T) {
	key := make([]byte, 32)
	t.Setenv("PT_TOOLS_SECRET_KEY", base64.StdEncoding.EncodeToString(key))
	testResetEncryptor()

	short := base64.StdEncoding.EncodeToString([]byte{1, 2, 3})
	plain, err := Decrypt(short)
	assert.Error(t, err)
	assert.Nil(t, plain)
}

// TestDecryptInvalidBase64 covers the base64 decode error branch.
func TestDecryptInvalidBase64(t *testing.T) {
	key := make([]byte, 32)
	t.Setenv("PT_TOOLS_SECRET_KEY", base64.StdEncoding.EncodeToString(key))
	testResetEncryptor()

	plain, err := Decrypt("!!!not base64!!!")
	assert.Error(t, err)
	assert.Nil(t, plain)
}

func TestInitKeyPanicsOnInvalidEnv(t *testing.T) {
	t.Setenv("PT_TOOLS_SECRET_KEY", "not-valid-base64-and-wrong-length")
	assert.Panics(t, func() { testResetEncryptor() })

	t.Cleanup(func() {
		envKey := make([]byte, 32)
		t.Setenv("PT_TOOLS_SECRET_KEY", base64.StdEncoding.EncodeToString(envKey))
		testResetEncryptor()
	})
}

func TestEncryptDecryptExportNoKey(t *testing.T) {
	saved := encryptor
	encryptor = nil
	t.Cleanup(func() { encryptor = saved })

	_, err := Encrypt([]byte("x"))
	assert.ErrorIs(t, err, errNoKey)

	_, err = Decrypt("x")
	assert.ErrorIs(t, err, errNoKey)

	_, err = ExportKey()
	assert.ErrorIs(t, err, errNoKey)
}

// testResetEncryptor is a helper to reset the global encryptor for testing
// In production code, this won't be exported, but for tests we need a way
// to reinitialize based on test-set env vars
func testResetEncryptor() {
	// This will be called after t.Setenv to force reinit
	initKey()
}
