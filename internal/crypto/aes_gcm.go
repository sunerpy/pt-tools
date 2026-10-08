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
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

var (
	encryptor *AESGCMEncryptor
	errNoKey  = errors.New("encryption key not initialized")

	// keyFilePath 是本次读取或生成的密钥文件路径；用 PT_TOOLS_SECRET_KEY 时为空。
	keyFilePath string
	// keyGenerated 为 true 表示 secret.key 原本不存在，是本进程启动时新生成的。
	keyGenerated bool
	// keyLoadErr 记录 secret.key 存在却无法使用的原因。这时不生成新密钥、也不覆盖原文件。
	keyLoadErr error

	// keyMu 与 keyLoaded：密钥在第一次用到时才读取或生成（Encrypt、Decrypt、ExportKey、KeyStatus），
	// 不在包初始化时做。pt-tools mcp 这类用不到密钥的命令因此不碰 ~/.pt-tools，也不会生成一个和服务端无关的密钥。
	keyMu     sync.Mutex
	keyLoaded bool
)

type AESGCMEncryptor struct {
	key []byte
}

// ensureKey 第一次调用时读取或生成密钥，之后什么都不做。
func ensureKey() {
	keyMu.Lock()
	defer keyMu.Unlock()
	if !keyLoaded {
		loadKeyLocked()
	}
}

// initKey 重新读取或生成密钥（测试与 ResetForTest 用）。
func initKey() {
	keyMu.Lock()
	defer keyMu.Unlock()
	loadKeyLocked()
}

func loadKeyLocked() {
	encryptor, keyFilePath, keyGenerated, keyLoadErr = nil, "", false, nil
	keyLoaded = true

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
	keyFilePath = keyFile
	// 只有文件确实不存在时才生成新密钥。文件在却读不出或内容不对（被截断、写成了 base64、挂错了文件……）时，
	// 覆盖它会让原密钥永久丢失，已加密的 Cookie 与通知凭证再也解不开，所以只记录错误，由启动检查拒绝启动。
	if _, err := os.Stat(keyFile); !errors.Is(err, fs.ErrNotExist) {
		loadExistingKey(keyFile)
		return
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

	createErr := writeNewKeyFile(keyFile, hex.EncodeToString(key))
	if errors.Is(createErr, fs.ErrExist) {
		// 检查与创建之间别处写入了密钥文件：读它，不覆盖
		loadExistingKey(keyFile)
		return
	}
	if createErr != nil {
		panic(fmt.Sprintf("failed to write secret key to %s: %v", keyFile, createErr))
	}

	encryptor = &AESGCMEncryptor{key: key}
	keyGenerated = true
}

// writeNewKeyFile 以 O_EXCL 创建密钥文件：文件（含悬空的符号链接）已存在时返回 fs.ErrExist，绝不覆盖。
func writeNewKeyFile(keyFile, keyHex string) error {
	f, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.WriteString(keyHex)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(keyFile)
	}
	return werr
}

// loadExistingKey 读取已存在的密钥文件；读不出或格式不对时只记录错误，不覆盖。
func loadExistingKey(keyFile string) {
	keyData, err := os.ReadFile(keyFile)
	if err != nil {
		keyLoadErr = fmt.Errorf("读取 %s 失败: %w", keyFile, err)
		return
	}
	key, err := hex.DecodeString(string(bytes.TrimSpace(keyData)))
	if err != nil || len(key) != 32 {
		keyLoadErr = fmt.Errorf("%s 不是 64 位十六进制的 AES-256 密钥", keyFile)
		return
	}
	encryptor = &AESGCMEncryptor{key: key}
}

// KeyStatus 报告启动时密钥的来源，供运行时初始化做安全检查：
// path 是密钥文件路径（用 PT_TOOLS_SECRET_KEY 时为空），generated 表示本次启动新生成了密钥文件，
// err 非空表示密钥文件存在却无法使用。
func KeyStatus() (path string, generated bool, err error) {
	ensureKey()
	return keyFilePath, keyGenerated, keyLoadErr
}

// DiscardGeneratedKey 删除本次启动新生成的密钥文件并停用它。启动检查发现库里已有旧密文时调用：
// 新密钥解不开旧数据，留着它，下次启动就会被当成正常密钥使用。
func DiscardGeneratedKey() error {
	keyMu.Lock()
	defer keyMu.Unlock()
	if !keyGenerated || keyFilePath == "" {
		return nil
	}
	encryptor, keyGenerated = nil, false
	return os.Remove(keyFilePath)
}

// Encrypt encrypts plaintext and returns base64-encoded "nonce|ciphertext|authtag"
// All three components are packed into the base64 string for transport.
func Encrypt(plain []byte) (cipherStr string, err error) {
	ensureKey()
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
	ensureKey()
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

func ExportKey() ([]byte, error) {
	ensureKey()
	if encryptor == nil || len(encryptor.key) == 0 {
		return nil, errNoKey
	}
	key := make([]byte, len(encryptor.key))
	copy(key, encryptor.key)
	return key, nil
}
