package nas

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tlsServer 是 httptest 的 HTTPS 服务（自签证书，系统根证书不认），hits 是它收到的请求数。
func tlsServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("ok"))
	}))
	ts.Config.ErrorLog = log.New(io.Discard, "", 0) // 握手被拒是这些测试要的，不打日志
	ts.StartTLS()
	t.Cleanup(ts.Close)
	return ts, &hits
}

func getVia(t *testing.T, ts *httptest.Server, pin string, roots *x509.CertPool) error {
	t.Helper()
	u, err := url.Parse(ts.URL)
	require.NoError(t, err)
	c := &http.Client{Transport: Transport(u, pin, roots), Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/secret?key=abc", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer abc")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

func TestTLS_UntrustedStopsBeforeRequest(t *testing.T) {
	ts, hits := tlsServer(t)
	err := getVia(t, ts, "", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCertUntrusted)
	assert.NotErrorIs(t, err, ErrCertChanged)
	var ce *CertError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, Fingerprint(ts.Certificate()), ce.Cert.SHA256)
	assert.Len(t, ce.Cert.SHA256, 64)
	assert.Contains(t, ce.Cert.Subject, "Acme Co")
	assert.False(t, ce.Cert.NotAfter.IsZero())
	assert.Contains(t, ce.Cert.DNSNames, "example.com")
	assert.Equal(t, int32(0), hits.Load(), "证书不认时请求（含凭据）一个字节都没发出去")
	assert.Equal(t, ErrCertUntrusted.Error(), ce.Error())
}

func TestTLS_PinnedAccepted(t *testing.T) {
	ts, hits := tlsServer(t)
	require.NoError(t, getVia(t, ts, Fingerprint(ts.Certificate()), nil))
	assert.Equal(t, int32(1), hits.Load())
}

func TestTLS_PinChanged(t *testing.T) {
	ts, hits := tlsServer(t)
	err := getVia(t, ts, strings.Repeat("ab", 32), nil)
	assert.ErrorIs(t, err, ErrCertChanged)
	var ce *CertError
	require.ErrorAs(t, err, &ce)
	assert.True(t, ce.Changed)
	assert.Equal(t, ErrCertChanged.Error(), ce.Error())
	assert.Equal(t, int32(0), hits.Load())
}

func TestTLS_TrustedChainNeedsNoPin(t *testing.T) {
	ts, hits := tlsServer(t)
	roots := x509.NewCertPool()
	roots.AddCert(ts.Certificate())
	require.NoError(t, getVia(t, ts, "", roots), "根证书认得时不用钉住")
	assert.Equal(t, int32(1), hits.Load())
}

func TestVerifyPeer_NoCertificate(t *testing.T) {
	assert.EqualError(t, verifyPeer(nil, "nas", "", nil), "对方没有给证书")
}

func TestPins(t *testing.T) {
	good := strings.Repeat("0a", 32)
	assert.True(t, ValidPin(""))
	assert.True(t, ValidPin(good))
	assert.False(t, ValidPin(strings.ToUpper(good)), "要小写")
	assert.False(t, ValidPin(good[:62]))
	assert.Equal(t, good, NormalizePin(" "+strings.ToUpper(strings.Join(splitPairs(good), ":"))+" "))
}

func splitPairs(s string) []string {
	out := []string{}
	for i := 0; i+2 <= len(s); i += 2 {
		out = append(out, s[i:i+2])
	}
	return out
}

func TestHTTPClient_NoRedirectNoProxy(t *testing.T) {
	var hits atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer target.Close()
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/steal", http.StatusFound)
	}))
	defer ts.Close()
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	u, _ := url.Parse(ts.URL)
	c := HTTPClient(u, Fingerprint(ts.Certificate()), 5*time.Second)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, nil)
	resp, err := c.Do(req)
	require.NoError(t, err, "不走环境变量里的代理")
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusFound, resp.StatusCode, "重定向不跟随")
	assert.Equal(t, int32(0), hits.Load())
	assert.Equal(t, 5*time.Second, c.Timeout)
	tr, ok := c.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Nil(t, tr.Proxy)
	assert.True(t, errors.Is(&CertError{}, ErrCertUntrusted))
}
