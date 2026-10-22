package v2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newRetrySite(d Driver[string, string]) *BaseSite[string, string] {
	return NewBaseSite(d, BaseSiteConfig{ID: "t", Name: "T", Kind: SiteNexusPHP, RateLimit: 100, RateBurst: 100, Logger: zap.NewNop()})
}

// 取种子的请求遇到网络错误、5xx：隔几秒再发，发成了就成（所有站点都走 BaseSite）。
func TestBaseSite_Download_RetriesTransientExecute(t *testing.T) {
	quickDownloadRetry(t)
	driver := &MockDriver{}
	driver.On("PrepareDownload", "9").Return("req", nil)
	driver.On("Execute", mock.Anything, "req").Return("", fmt.Errorf("execute request: %w: connection reset by peer", ErrNetworkError)).Once()
	driver.On("Execute", mock.Anything, "req").Return("", &HTTPStatusError{Code: http.StatusBadGateway}).Once()
	driver.On("Execute", mock.Anything, "req").Return("resp", nil).Once()
	driver.On("ParseDownload", "resp").Return([]byte("data"), nil)

	data, err := newRetrySite(driver).Download(context.Background(), "9")
	require.NoError(t, err)
	assert.Equal(t, []byte("data"), data)
	driver.AssertNumberOfCalls(t, "Execute", 3)
}

// 一直失败：试满三次，错误里写明试了几次；凭证无效、限流、Cloudflare 质询、4xx 不重试。
func TestBaseSite_Download_RetryLimits(t *testing.T) {
	quickDownloadRetry(t)
	driver := &MockDriver{}
	driver.On("PrepareDownload", "9").Return("req", nil)
	driver.On("Execute", mock.Anything, "req").Return("", fmt.Errorf("execute request: %w: timeout", ErrNetworkError))
	_, err := newRetrySite(driver).Download(context.Background(), "9")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "(3 attempts)")
	assert.ErrorIs(t, err, ErrNetworkError)
	driver.AssertNumberOfCalls(t, "Execute", 3)

	for _, e := range []error{
		ErrInvalidCredentials,
		ErrSessionExpired,
		fmt.Errorf("HTTP 429: %w", ErrRateLimited),
		fmt.Errorf("HTTP 403: %w", ErrCloudflareChallenge),
		&HTTPStatusError{Code: http.StatusNotFound},
	} {
		driver := &MockDriver{}
		driver.On("PrepareDownload", "9").Return("req", nil)
		driver.On("Execute", mock.Anything, "req").Return("", e)
		_, err := newRetrySite(driver).Download(context.Background(), "9")
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "attempts", e.Error())
		driver.AssertNumberOfCalls(t, "Execute", 1)
	}
}

// hashFlaky 是要 hash 才能下载的站点：前 fails 次下载断开连接。
type hashFlaky struct {
	hashDriver
	fails, calls int
}

func (h *hashFlaky) DownloadWithHash(context.Context, string, string) ([]byte, error) {
	h.calls++
	if h.calls <= h.fails {
		return nil, fmt.Errorf("download request failed: %w: EOF", ErrNetworkError)
	}
	return []byte("ok"), nil
}

func TestBaseSite_DownloadWithHash_Retries(t *testing.T) {
	quickDownloadRetry(t)
	hd := &hashFlaky{fails: 1}
	site := NewBaseSite[string, string](hd, BaseSiteConfig{ID: "t", Name: "T", Kind: SiteHDDolby, RateLimit: 100, RateBurst: 100, Logger: zap.NewNop()})
	data, err := site.DownloadWithHash(context.Background(), "5", "abc")
	require.NoError(t, err)
	assert.Equal(t, []byte("ok"), data)
	assert.Equal(t, 2, hd.calls)
}

// NexusPHP 从详情页取到下载地址以后拉种子文件：断开连接、5xx 隔几秒重拉同一个地址（不再取详情页）；
// 错误里的下载地址（带 passkey）要脱敏。
func TestNexusPHPDriver_ParseDownloadContext_RetriesFetch(t *testing.T) {
	quickDownloadRetry(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch hits.Add(1) {
		case 1:
			conn, _, err := w.(http.Hijacker).Hijack()
			require.NoError(t, err)
			_ = conn.Close()
		case 2:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			_, _ = w.Write([]byte("d8:announce"))
		}
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<html><body><a href="download.php?id=5&passkey=abc">dl</a></body></html>`))
	require.NoError(t, err)
	data, err := d.ParseDownloadContext(context.Background(), NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Equal(t, []byte("d8:announce"), data)
	assert.Equal(t, int32(3), hits.Load())

	// 连不上：试满三次，错误里没有 passkey
	d = NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "http://127.0.0.1:1", Cookie: "c=1"})
	doc, err = goquery.NewDocumentFromReader(strings.NewReader(`<html><body><a href="download.php?id=5&passkey=SECRETPASSKEY">dl</a></body></html>`))
	require.NoError(t, err)
	_, err = d.ParseDownloadContext(context.Background(), NexusPHPResponse{Document: doc})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "(3 attempts)")
	assert.NotContains(t, err.Error(), "SECRETPASSKEY")
	assert.True(t, errors.Is(err, ErrNetworkError))
}
