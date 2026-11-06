package nas

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// CertInfo 是证书确认时给用户看的叶证书：SHA256 是整张证书（DER）的指纹，小写十六进制。
type CertInfo struct {
	SHA256    string    `json:"sha256"`
	Subject   string    `json:"subject"`
	Issuer    string    `json:"issuer"`
	NotBefore time.Time `json:"not_before"`
	NotAfter  time.Time `json:"not_after"`
	DNSNames  []string  `json:"dns_names"`
}

// CertError 是握手时证书没通过：Changed 为真是钉住了指纹、但这张证书的指纹不一样；否则是系统根证书不认、也没有钉住。
// errors.Is 认得 ErrCertChanged 与 ErrCertUntrusted。
type CertError struct {
	Changed bool
	Cert    CertInfo
}

func (e *CertError) Error() string {
	if e.Changed {
		return ErrCertChanged.Error()
	}
	return ErrCertUntrusted.Error()
}

// Is 让 errors.Is 按 Changed 认 ErrCertChanged 或 ErrCertUntrusted。
func (e *CertError) Is(target error) bool {
	if e.Changed {
		return target == ErrCertChanged
	}
	return target == ErrCertUntrusted
}

var pinRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidPin 是钉住的指纹的格式：空（不钉住）或 64 位小写十六进制。
func ValidPin(pin string) bool { return pin == "" || pinRe.MatchString(pin) }

// NormalizePin 把用户或证书确认带来的指纹规范成小写十六进制（去掉冒号与空白）。
func NormalizePin(pin string) string {
	pin = strings.ToLower(strings.TrimSpace(pin))
	return strings.NewReplacer(":", "", " ", "").Replace(pin)
}

// Fingerprint 是一张证书（DER）的 SHA-256，小写十六进制。
func Fingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}

func certInfo(cert *x509.Certificate) CertInfo {
	return CertInfo{
		SHA256:    Fingerprint(cert),
		Subject:   cert.Subject.String(),
		Issuer:    cert.Issuer.String(),
		NotBefore: cert.NotBefore,
		NotAfter:  cert.NotAfter,
		DNSNames:  append([]string(nil), cert.DNSNames...),
	}
}

// TLSConfig 是连一个来源用的 TLS 设置：先按 roots（为空时是系统根证书）校验证书链与主机名；不通过时看叶证书的
// SHA-256 是不是钉住的 pin；都不是时返回 *CertError。
//
// 校验写在 VerifyConnection 里，握手拿到证书就判：证书不认时连接在发出任何应用数据（含 API Key、密码）之前就断了。
// 为此关掉 crypto/tls 自己的链校验（InsecureSkipVerify），改成在这里做，不是跳过校验。
func TLSConfig(host, pin string, roots *x509.CertPool) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			return verifyPeer(cs.PeerCertificates, host, pin, roots)
		},
	}
}

func verifyPeer(certs []*x509.Certificate, host, pin string, roots *x509.CertPool) error {
	if len(certs) == 0 {
		return errors.New("对方没有给证书")
	}
	leaf := certs[0]
	inter := x509.NewCertPool()
	for _, c := range certs[1:] {
		inter.AddCert(c)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{DNSName: host, Roots: roots, Intermediates: inter}); err == nil {
		return nil
	}
	got := Fingerprint(leaf)
	if pin != "" && got == pin {
		return nil
	}
	return &CertError{Changed: pin != "", Cert: certInfo(leaf)}
}

// HTTPClient 是连一个来源用的 HTTP 客户端：证书按 TLSConfig 校验、不走代理（NAS 在局域网里，环境变量里的代理是给
// tracker 用的，不让 API Key 经过它），不跟随重定向（重定向可能把凭据带到别的主机），单次请求最多 timeout。
func HTTPClient(u *url.URL, pin string, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: Transport(u, pin, nil),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// Transport 是 HTTPClient 用的传输层（roots 为空时是系统根证书；测试里换成 httptest 的证书）。
func Transport(u *url.URL, pin string, roots *x509.CertPool) *http.Transport {
	return &http.Transport{
		Proxy:                 nil,
		TLSClientConfig:       TLSConfig(u.Hostname(), pin, roots),
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConnsPerHost:   2,
	}
}
