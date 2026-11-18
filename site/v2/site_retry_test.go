package v2

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// 退避是指数的：第 1 次重试前等 base 到 1.5 倍 base，第 2 次 2 倍到 3 倍（随机抖动最多一半）。
func TestSiteRetryDelay_Exponential(t *testing.T) {
	for range 50 {
		d1, d2 := siteRetryDelay(1), siteRetryDelay(2)
		assert.GreaterOrEqual(t, d1, siteRetryBase)
		assert.LessOrEqual(t, d1, siteRetryBase*3/2)
		assert.GreaterOrEqual(t, d2, 2*siteRetryBase)
		assert.LessOrEqual(t, d2, 3*siteRetryBase)
	}
	assert.Equal(t, 2*time.Second, siteRetryBase)
	assert.Equal(t, 2, siteRetryMax)
}

// 读种子列表与搜索遇到网络错误、5xx：按退避再发，发成了就成；每次重试前先等限速的令牌。
func TestBaseSite_Search_RetriesTransientExecute(t *testing.T) {
	quickDownloadRetry(t)
	driver := &MockDriver{}
	driver.On("PrepareSearch", mock.Anything).Return("req", nil)
	driver.On("Execute", mock.Anything, "req").Return("", fmt.Errorf("execute request: %w: read: connection reset by peer", ErrNetworkError)).Once()
	driver.On("Execute", mock.Anything, "req").Return("", &HTTPStatusError{Code: http.StatusBadGateway}).Once()
	driver.On("Execute", mock.Anything, "req").Return("resp", nil).Once()
	driver.On("ParseSearch", "resp").Return([]TorrentItem{{ID: "1", Title: "a"}}, nil)

	items, err := newRetrySite(driver).Search(context.Background(), SearchQuery{FreeOnly: true})
	require.NoError(t, err)
	assert.Len(t, items, 1)
	driver.AssertNumberOfCalls(t, "Execute", 3)
}

// 一直超时：试满三次，错误里写明试了几次；限流、凭证、登录过期、二次验证、Cloudflare 质询、4xx、域名解析不了都不重试。
func TestBaseSite_Search_RetryLimits(t *testing.T) {
	quickDownloadRetry(t)
	driver := &MockDriver{}
	driver.On("PrepareSearch", mock.Anything).Return("req", nil)
	driver.On("Execute", mock.Anything, "req").Return("", fmt.Errorf("execute request: %w: Client.Timeout exceeded while awaiting headers", ErrNetworkError))
	_, err := newRetrySite(driver).Search(context.Background(), SearchQuery{FreeOnly: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "(3 attempts)")
	assert.ErrorIs(t, err, ErrNetworkError)
	driver.AssertNumberOfCalls(t, "Execute", 3)

	for _, e := range []error{
		fmt.Errorf("HTTP 429: %w", ErrRateLimited),
		ErrInvalidCredentials,
		ErrSessionExpired,
		Err2FARequired,
		fmt.Errorf("HTTP 403: %w", ErrCloudflareChallenge),
		&HTTPStatusError{Code: http.StatusNotFound},
		fmt.Errorf("%w: %w", ErrNetworkError, &net.DNSError{Err: "no such host", Name: "x.invalid", IsNotFound: true}),
	} {
		driver := &MockDriver{}
		driver.On("PrepareSearch", mock.Anything).Return("req", nil)
		driver.On("Execute", mock.Anything, "req").Return("", e)
		_, err := newRetrySite(driver).Search(context.Background(), SearchQuery{FreeOnly: true})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "attempts", e.Error())
		driver.AssertNumberOfCalls(t, "Execute", 1)
	}
}

// 每次重试前先等站点限速的令牌：重试也算进站点的请求频率。
func TestRetryTransient_WaitsForLimiterBeforeEachRetry(t *testing.T) {
	quickDownloadRetry(t)
	waits, calls := 0, 0
	_, err := retryTransient(context.Background(), func(context.Context) error { waits++; return nil }, func() (int, error) {
		calls++
		return 0, fmt.Errorf("%w: EOF", ErrNetworkError)
	})
	require.Error(t, err)
	assert.Equal(t, 3, calls)
	assert.Equal(t, 2, waits, "第一次不等令牌（调用方已经取过），之后每次重试前各等一次")
}

// 剩下的时间不够退避加一次请求时不再试，马上返回；调用方取消了也不再试。
func TestRetryTransient_StopsWhenNoTimeLeft(t *testing.T) {
	oldBase, oldMin := siteRetryBase, siteRetryMinAttempt
	siteRetryBase, siteRetryMinAttempt = 10*time.Second, 10*time.Second
	t.Cleanup(func() { siteRetryBase, siteRetryMinAttempt = oldBase, oldMin })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	calls := 0
	start := time.Now()
	_, err := retryTransient(ctx, nil, func() (int, error) {
		calls++
		return 0, fmt.Errorf("%w: connection reset", ErrNetworkError)
	})
	require.Error(t, err)
	assert.Equal(t, 1, calls)
	assert.Less(t, time.Since(start), time.Second)

	canceled, stop := context.WithCancel(context.Background())
	stop()
	calls = 0
	_, err = retryTransient(canceled, nil, func() (int, error) {
		calls++
		return 0, fmt.Errorf("%w: %w", ErrNetworkError, context.Canceled)
	})
	require.Error(t, err)
	assert.Equal(t, 1, calls)
}

// 走 API 的站点（M-Team、Rousi 等）取用户信息一次取完：网络错误整次重试。
func TestBaseSite_GetUserInfo_RetriesAPIDriver(t *testing.T) {
	quickDownloadRetry(t)
	driver := &MockDriver{}
	driver.On("GetUserInfo", mock.Anything).Return(UserInfo{}, fmt.Errorf("execute user info: request failed: %w: context deadline exceeded (Client.Timeout or context cancellation while reading body)", ErrNetworkError)).Once()
	driver.On("GetUserInfo", mock.Anything).Return(UserInfo{Username: "u", Uploaded: 1}, nil).Once()

	info, err := newRetrySite(driver).GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "u", info.Username)
	driver.AssertNumberOfCalls(t, "GetUserInfo", 2)
}

// pageRetryingDriver 是自己按页重试的驱动（像有站点定义的 NexusPHP）：BaseSite 不再整次重试。
type pageRetryingDriver struct{ *MockDriver }

func (pageRetryingDriver) retriesUserInfoPages() bool { return true }

// 按页重试过的驱动、只取到一部分的结果都不再整次重试：页面已经各自重试过了，取到的部分交给 UserInfoService 补齐。
func TestBaseSite_GetUserInfo_NoWholeRetry(t *testing.T) {
	quickDownloadRetry(t)
	netErr := fmt.Errorf("execute request: %w: connection reset by peer", ErrNetworkError)

	paged := pageRetryingDriver{&MockDriver{}}
	paged.On("GetUserInfo", mock.Anything).Return(UserInfo{}, netErr)
	site := NewBaseSite[string, string](paged, BaseSiteConfig{ID: "t", Name: "T", Kind: SiteNexusPHP, RateLimit: 100, RateBurst: 100, Logger: zap.NewNop()})
	_, err := site.GetUserInfo(context.Background())
	require.Error(t, err)
	paged.AssertNumberOfCalls(t, "GetUserInfo", 1)

	partial := &MockDriver{}
	partial.On("GetUserInfo", mock.Anything).Return(UserInfo{Username: "u"}, &PartialUserInfoError{Missing: []string{"bonus"}, Err: netErr})
	info, err := newRetrySite(partial).GetUserInfo(context.Background())
	var pe *PartialUserInfoError
	require.True(t, errors.As(err, &pe))
	assert.Equal(t, "u", info.Username)
	partial.AssertNumberOfCalls(t, "GetUserInfo", 1)
}
