package remote

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/flynn/noise"
)

const (
	// HostIDLen 是 hostId 的长度：sha256(Ed25519 公钥) 的 base32 取前 26 个字符（130 位）
	HostIDLen = 26
	// KeyLen 是 X25519 公钥、私钥与配对密钥的长度
	KeyLen = 32
	// hostKeysVersion 是落库的主机密钥 JSON 的版本
	hostKeysVersion = 1
)

var (
	// b64 是协议里所有二进制值的文本写法：base64url，无填充
	b64            = base64.RawURLEncoding.Strict()
	hostIDEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

	errKeyLength = errors.New("密钥长度不对")
)

// HostKeys 是这台 pt-tools 的主机密钥。两把都只在内存与加密以后的库里出现，轮换以后所有设备要重新配对。
type HostKeys struct {
	// Sign 向 relay 证明这台主机拥有 hostId
	Sign ed25519.PrivateKey
	// Noise 是 Noise 握手里主机的静态密钥（X25519）
	Noise noise.DHKey
}

// GenerateHostKeys 生成一套新的主机密钥。rng 为 nil 时用 crypto/rand。
func GenerateHostKeys(rng io.Reader) (*HostKeys, error) {
	if rng == nil {
		rng = rand.Reader
	}
	_, sign, err := ed25519.GenerateKey(rng)
	if err != nil {
		return nil, fmt.Errorf("生成签名密钥失败: %w", err)
	}
	dh, err := GenerateKeypair(rng)
	if err != nil {
		return nil, err
	}
	return &HostKeys{Sign: sign, Noise: dh}, nil
}

// GenerateKeypair 生成一把 X25519 密钥（主机的 Noise 密钥与设备密钥都用它）。rng 为 nil 时用 crypto/rand。
func GenerateKeypair(rng io.Reader) (noise.DHKey, error) {
	if rng == nil {
		rng = rand.Reader
	}
	k, err := noise.DH25519.GenerateKeypair(rng)
	if err != nil {
		return noise.DHKey{}, fmt.Errorf("生成 X25519 密钥失败: %w", err)
	}
	return k, nil
}

// KeypairFromPrivate 由 X25519 私钥算出公钥。
func KeypairFromPrivate(priv []byte) (noise.DHKey, error) {
	if len(priv) != KeyLen {
		return noise.DHKey{}, errKeyLength
	}
	k, err := ecdh.X25519().NewPrivateKey(priv)
	if err != nil {
		return noise.DHKey{}, fmt.Errorf("X25519 私钥不对: %w", err)
	}
	return noise.DHKey{Private: append([]byte(nil), priv...), Public: k.PublicKey().Bytes()}, nil
}

// HostIDOf 由 Ed25519 公钥推导 hostId：lower(base32(sha256(pub)))[:26]，标准字母表、无填充。
func HostIDOf(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return strings.ToLower(hostIDEncoding.EncodeToString(sum[:]))[:HostIDLen]
}

// ValidHostID 报告 s 是不是格式正确的 hostId（26 个 a–z、2–7）。
func ValidHostID(s string) bool {
	if len(s) != HostIDLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < '2' || c > '7') {
			return false
		}
	}
	return true
}

// SignPublic 是签名密钥的公钥。
func (k *HostKeys) SignPublic() ed25519.PublicKey { return k.Sign.Public().(ed25519.PublicKey) }

// HostID 是这套密钥的 hostId。
func (k *HostKeys) HostID() string { return HostIDOf(k.SignPublic()) }

// storedHostKeys 是主机密钥落库前的 JSON（只有私钥，公钥读回时重新算）。
type storedHostKeys struct {
	V           int    `json:"v"`
	Ed25519Seed string `json:"ed25519_seed"`
	X25519      string `json:"x25519"`
}

// Marshal 把主机密钥写成 JSON。结果是私钥，调用方加密以后才能落库。
func (k *HostKeys) Marshal() ([]byte, error) {
	if len(k.Sign) != ed25519.PrivateKeySize || len(k.Noise.Private) != KeyLen {
		return nil, errKeyLength
	}
	return json.Marshal(storedHostKeys{V: hostKeysVersion, Ed25519Seed: b64.EncodeToString(k.Sign.Seed()), X25519: b64.EncodeToString(k.Noise.Private)})
}

// ParseHostKeys 读回 Marshal 的结果。
func ParseHostKeys(b []byte) (*HostKeys, error) {
	var s storedHostKeys
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("主机密钥格式不对: %w", err)
	}
	if s.V != hostKeysVersion {
		return nil, fmt.Errorf("不认识的主机密钥版本 %d", s.V)
	}
	seed, err := b64.DecodeString(s.Ed25519Seed)
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("主机签名密钥不对: %w", errKeyLength)
	}
	priv, err := b64.DecodeString(s.X25519)
	if err != nil {
		return nil, fmt.Errorf("主机 Noise 密钥不对: %w", err)
	}
	dh, err := KeypairFromPrivate(priv)
	if err != nil {
		return nil, err
	}
	return &HostKeys{Sign: ed25519.NewKeyFromSeed(seed), Noise: dh}, nil
}

// EncodeKey 是 32 字节的值（公钥、配对密钥）在链接与接口里的写法：base64url，无填充。
func EncodeKey(b []byte) string { return b64.EncodeToString(b) }

// DecodeKey 读回 EncodeKey 的结果，长度必须是 32 字节。
func DecodeKey(s string) ([]byte, error) {
	b, err := b64.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("不是 base64url: %w", err)
	}
	if len(b) != KeyLen {
		return nil, errKeyLength
	}
	return b, nil
}
