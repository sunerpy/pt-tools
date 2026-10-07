// Package cookiecloud 从自建的 CookieCloud 服务取回加密的 Cookie，在本地解密，按站点地址挑出 pt-tools 已配置站点的 Cookie。
//
// 协议按 CookieCloud 的公开说明实现（GET {server}/get/{uuid} 取回 {encrypted, crypto_type}），只在本地解密，
// 密码不发给服务端。两种加密：
//   - legacy：CryptoJS 口令模式。口令是 md5(uuid + "-" + password) 十六进制的前 16 个字符；密文是 OpenSSL 格式
//     base64("Salted__" + 8 字节 salt + 密文)，key 与 iv 由 EVP_BytesToKey(MD5, 口令, salt) 派生，AES-256-CBC + PKCS7。
//   - aes-128-cbc-fixed（CookieCloud 0.3.0 起）：key 是上面那 16 个字符本身（AES-128），iv 是 16 个 0 字节，
//     密文是标准 base64，CBC + PKCS7。
package cookiecloud

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// 加密方式。
const (
	CryptoLegacy = "legacy"
	CryptoFixed  = "aes-128-cbc-fixed"
)

const maxBodyBytes = 32 << 20

var (
	// ErrDecrypt 表示解不开：多半是 UUID 或密码不对。
	ErrDecrypt = errors.New("解密失败：请检查 UUID 和密码")
	// ErrNotFound 表示服务端没有这个 UUID 的数据。
	ErrNotFound = errors.New("CookieCloud 服务上没有这个 UUID 的数据")
)

// Cookie 是 CookieCloud 存的一条 Cookie（浏览器 cookies 接口给的字段）。
type Cookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
	Path   string `json:"path"`
	// HostOnly 为 true 时只对 Domain 这个主机名本身有效；没有这个字段时按 Domain 有没有前导点判断。
	HostOnly *bool `json:"hostOnly,omitempty"`
	// ExpirationDate 是过期时间（Unix 秒，可以带小数）；会话 Cookie 没有。
	ExpirationDate *float64 `json:"expirationDate,omitempty"`
}

// Data 是解密后的内容：按域名分组的 Cookie。
type Data struct {
	CookieData map[string][]Cookie `json:"cookie_data"`
}

// Payload 是 /get/{uuid} 返回的密文。
type Payload struct {
	Encrypted  string `json:"encrypted"`
	CryptoType string `json:"crypto_type"`
}

// Fetch 取回 uuid 的密文（不带密码，服务端不解密）。
func Fetch(ctx context.Context, httpc *http.Client, server, uuid string) (Payload, error) {
	server = strings.TrimRight(strings.TrimSpace(server), "/")
	u, err := url.Parse(server)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Payload{}, errors.New("CookieCloud 服务地址要以 http:// 或 https:// 开头")
	}
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return Payload{}, errors.New("UUID 不能为空")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server+"/get/"+url.PathEscape(uuid), nil)
	if err != nil {
		return Payload{}, err
	}
	req.Header.Set("Accept", "application/json")
	if httpc == nil {
		httpc = &http.Client{Timeout: 30 * time.Second}
	}
	// 只跟随同一主机内的重定向（如 http 升到 https）：地址里带着 UUID，不带去别的主机
	c := *httpc
	c.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("重定向次数太多")
		}
		if !strings.EqualFold(r.URL.Hostname(), via[0].URL.Hostname()) {
			return errors.New("CookieCloud 服务把请求重定向到了别的主机，没有跟随")
		}
		return nil
	}
	resp, err := c.Do(req)
	if err != nil {
		// 错误里不带请求地址：地址里有 UUID
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return Payload{}, fmt.Errorf("连不上 CookieCloud 服务: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return Payload{}, fmt.Errorf("读取 CookieCloud 响应失败: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return Payload{}, ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return Payload{}, fmt.Errorf("CookieCloud 服务返回 HTTP %d", resp.StatusCode)
	}
	var p Payload
	if err := json.Unmarshal(body, &p); err != nil || p.Encrypted == "" {
		return Payload{}, errors.New("CookieCloud 的响应里没有密文")
	}
	return p, nil
}

// passphrase 是 md5(uuid + "-" + password) 十六进制的前 16 个字符。
func passphrase(uuid, password string) string {
	sum := md5.Sum([]byte(uuid + "-" + password))
	return hex.EncodeToString(sum[:])[:16]
}

// Decrypt 解密密文。cryptoType 为空时按 legacy；legacy 解不开、密文又不是 OpenSSL 格式时再按 fixed 试一次。
func Decrypt(p Payload, uuid, password string) (Data, error) {
	key := passphrase(strings.TrimSpace(uuid), password)
	var plain []byte
	var err error
	switch p.CryptoType {
	case CryptoFixed:
		plain, err = decryptFixed(p.Encrypted, key)
	case "", CryptoLegacy:
		plain, err = decryptLegacy(p.Encrypted, key)
		if errors.Is(err, errNotSalted) {
			plain, err = decryptFixed(p.Encrypted, key)
		}
	default:
		return Data{}, fmt.Errorf("不认识的加密方式 %q", p.CryptoType)
	}
	if err != nil {
		return Data{}, ErrDecrypt
	}
	var d Data
	if err := json.Unmarshal(plain, &d); err != nil {
		return Data{}, ErrDecrypt
	}
	return d, nil
}

var errNotSalted = errors.New("not openssl salted format")

func decryptLegacy(encrypted, pass string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encrypted))
	if err != nil {
		return nil, err
	}
	if len(raw) < 16 || !bytes.Equal(raw[:8], []byte("Salted__")) {
		return nil, errNotSalted
	}
	salt, data := raw[8:16], raw[16:]
	key, iv := evpBytesToKey([]byte(pass), salt, 32, aes.BlockSize)
	return cbcDecrypt(key, iv, data)
}

func decryptFixed(encrypted, pass string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encrypted))
	if err != nil {
		return nil, err
	}
	return cbcDecrypt([]byte(pass), make([]byte, aes.BlockSize), raw)
}

// evpBytesToKey 是 OpenSSL 的 EVP_BytesToKey（MD5，迭代 1 次）：D_i = MD5(D_{i-1} || password || salt)，拼到够长。
func evpBytesToKey(password, salt []byte, keyLen, ivLen int) ([]byte, []byte) {
	var out, prev []byte
	for len(out) < keyLen+ivLen {
		h := md5.New()
		h.Write(prev)
		h.Write(password)
		h.Write(salt)
		prev = h.Sum(nil)
		out = append(out, prev...)
	}
	return out[:keyLen], out[keyLen : keyLen+ivLen]
}

func cbcDecrypt(key, iv, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data)%aes.BlockSize != 0 {
		return nil, errors.New("密文长度不对")
	}
	out := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, data)
	n := int(out[len(out)-1])
	if n == 0 || n > aes.BlockSize || n > len(out) {
		return nil, errors.New("填充不对")
	}
	for _, b := range out[len(out)-n:] {
		if int(b) != n {
			return nil, errors.New("填充不对")
		}
	}
	return out[:len(out)-n], nil
}
