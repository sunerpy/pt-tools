package crypto

import (
	"bytes"
	"crypto/aes"
	cipherPkg "crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var (
	encryptor *AESGCMEncryptor
	errNoKey  = errors.New("encryption key not initialized")
)

type AESGCMEncryptor struct {
	key []byte
}

func init() {
	initKey()
}

func initKey() {
	keyB64 := os.Getenv("PT_TOOLS_SECRET_KEY")
	if keyB64 != "" {
		key, err := base64.StdEncoding.DecodeString(keyB64)
		if err != nil || len(key) != 32 {
			panic(fmt.Sprintf("invalid PT_TOOLS_SECRET_KEY: must be base64-encoded 32 bytes, got: %v", err))
		}
		encryptor = &AESGCMEncryptor{key: key}
		return
	}

	// No env key: try to load from ~/.pt-tools/secret.key
	home, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Sprintf("failed to get home directory: %v", err))
	}

	keyFile := filepath.Join(home, ".pt-tools", "secret.key")
	keyData, err := os.ReadFile(keyFile)
	if err == nil {
		key, err := hex.DecodeString(string(bytes.TrimSpace(keyData)))
		if err == nil && len(key) == 32 {
			encryptor = &AESGCMEncryptor{key: key}
			return
		}
	}

	// Generate new random key and save to file
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(fmt.Sprintf("failed to generate random key: %v", err))
	}

	dir := filepath.Dir(keyFile)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		panic(fmt.Sprintf("failed to create .pt-tools directory: %v", err))
	}

	keyHex := hex.EncodeToString(key)
	if err := os.WriteFile(keyFile, []byte(keyHex), 0o600); err != nil {
		panic(fmt.Sprintf("failed to write secret key to %s: %v", keyFile, err))
	}

	encryptor = &AESGCMEncryptor{key: key}
}

// Encrypt encrypts plaintext and returns base64-encoded "nonce|ciphertext|authtag"
// All three components are packed into the base64 string for transport.
func Encrypt(plain []byte) (cipherStr string, err error) {
	if encryptor == nil || len(encryptor.key) == 0 {
		return "", errNoKey
	}

	block, err := aes.NewCipher(encryptor.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipherPkg.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Generate 12-byte nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// Seal returns ciphertext with authtag appended (16 bytes)
	ciphertext := gcm.Seal(nil, nonce, plain, nil)

	// Pack: nonce | ciphertext | authtag all together, then base64
	// gcm.Seal already appends authtag, so ciphertext is [ciphertext_data | authtag]
	result := append(nonce, ciphertext...)
	cipherStr = base64.StdEncoding.EncodeToString(result)

	return cipherStr, nil
}

// Decrypt decodes base64 string and decrypts to plaintext.
// Expects format: base64(nonce | ciphertext | authtag)
func Decrypt(cipherStr string) (plain []byte, err error) {
	if encryptor == nil || len(encryptor.key) == 0 {
		return nil, errNoKey
	}

	block, err := aes.NewCipher(encryptor.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipherPkg.NewGCM(block)
	if err != nil {
		return nil, err
	}

	cipherBytes, err := base64.StdEncoding.DecodeString(cipherStr)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(cipherBytes) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce := cipherBytes[:nonceSize]
	ciphertext := cipherBytes[nonceSize:]

	plain, err = gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plain, nil
}
