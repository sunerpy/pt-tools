package v2

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// BaseSite wraps a Driver and provides common functionality like rate limiting and logging.
// It implements the Site interface by delegating to the underlying Driver.
type BaseSite[Req any, Res any] struct {
	id       string
	name     string
	kind     SiteKind
	driver   Driver[Req, Res]
	limiter  *rate.Limiter
	logger   *zap.Logger
	creds    Credentials
	loggedIn bool
	mu       sync.RWMutex
}

// BaseSiteConfig holds configuration for creating a BaseSite
type BaseSiteConfig struct {
	ID        string
	Name      string
	Kind      SiteKind
	RateLimit float64 // Requests per second
	RateBurst int     // Maximum burst size
	Logger    *zap.Logger
}

// NewBaseSite creates a new BaseSite with the given driver and configuration
func NewBaseSite[Req, Res any](driver Driver[Req, Res], config BaseSiteConfig) *BaseSite[Req, Res] {
	// Default rate limit: 1 request per second with burst of 3
	rateLimit := config.RateLimit
	if rateLimit <= 0 {
		rateLimit = 1.0
	}
	rateBurst := config.RateBurst
	if rateBurst <= 0 {
		rateBurst = 3
	}

	logger := config.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	return &BaseSite[Req, Res]{
		id:      config.ID,
		name:    config.Name,
		kind:    config.Kind,
		driver:  driver,
		limiter: rate.NewLimiter(rate.Limit(rateLimit), rateBurst),
		logger:  logger,
	}
}

// ID returns the unique site identifier
func (b *BaseSite[Req, Res]) ID() string {
	return b.id
}

// Name returns the human-readable site name
func (b *BaseSite[Req, Res]) Name() string {
	return b.name
}

// Kind returns the site architecture type
func (b *BaseSite[Req, Res]) Kind() SiteKind {
	return b.kind
}

// Login authenticates with the site
func (b *BaseSite[Req, Res]) Login(ctx context.Context, creds Credentials) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.creds = creds
	b.loggedIn = true
	b.logger.Info("Logged in to site", zap.String("site", b.name))
	return nil
}

// IsLoggedIn returns whether the site is currently logged in
func (b *BaseSite[Req, Res]) IsLoggedIn() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.loggedIn
}

// Search searches for torrents on the site
func (b *BaseSite[Req, Res]) Search(ctx context.Context, query SearchQuery) ([]TorrentItem, error) {
	// Validate query
	if err := query.Validate(); err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}

	// Rate limiting
	if err := b.limiter.Wait(ctx); err != nil {
		b.logger.Warn("Rate limit wait failed", zap.Error(err))
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	startTime := time.Now()
	b.logger.Debug(
		"Starting search",
		zap.String("site", b.name),
		zap.String("keyword", query.Keyword),
		zap.Bool("freeOnly", query.FreeOnly),
	)

	// Prepare request using driver
	req, err := b.driver.PrepareSearch(query)
	if err != nil {
		b.logger.Error("Failed to prepare search request", zap.Error(err))
		return nil, fmt.Errorf("prepare search: %w", err)
	}

	// Execute request：一时的错误（网络、5xx）按指数退避再发，每次重试先等限速的令牌（见 retryTransient）
	res, err := retryTransient(ctx, b.limiter.Wait, func() (Res, error) { return b.driver.Execute(ctx, req) })
	if err != nil {
		b.logger.Error("Failed to execute search request", zap.Error(err))
		return nil, fmt.Errorf("execute search: %w", err)
	}

	// Parse response
	items, err := b.driver.ParseSearch(res)
	if err != nil {
		b.logger.Error("Failed to parse search response", zap.Error(err))
		return nil, fmt.Errorf("parse search: %w", err)
	}

	// Set source site for all items
	for i := range items {
		items[i].SourceSite = b.id
	}

	b.logger.Info(
		"Search completed",
		zap.String("site", b.name),
		zap.Int("results", len(items)),
		zap.Duration("duration", time.Since(startTime)),
	)

	return items, nil
}

// GetUserInfo fetches the current user's information
func (b *BaseSite[Req, Res]) GetUserInfo(ctx context.Context) (UserInfo, error) {
	// Rate limiting
	if err := b.limiter.Wait(ctx); err != nil {
		return UserInfo{}, fmt.Errorf("rate limit: %w", err)
	}

	startTime := time.Now()
	b.logger.Debug("Fetching user info", zap.String("site", b.name))

	// Delegate to driver which handles all API calls
	info, err := b.getUserInfo(ctx)
	// 只取到一部分（*PartialUserInfoError）时把取到的部分一起交回去，由 UserInfoService 补上缺的字段再保存；
	// 失败的原因由调用方记（UserInfoService 一次失败只记一行）
	var partial *PartialUserInfoError
	if err != nil && !errors.As(err, &partial) {
		b.logger.Debug("Failed to fetch user info", zap.Error(err))
		return UserInfo{}, fmt.Errorf("get user info: %w", err)
	}

	// Set site and update time
	info.Site = b.id
	info.LastUpdate = time.Now().Unix()

	b.logger.Info(
		"User info fetched",
		zap.String("site", b.name),
		zap.String("username", info.Username),
		zap.Bool("partial", partial != nil),
		zap.Duration("duration", time.Since(startTime)),
	)

	if partial != nil {
		return info, fmt.Errorf("get user info: %w", err)
	}
	return info, nil
}

// userInfoPageRetrier 是取用户信息时自己按页重试的驱动（有站点定义的 NexusPHP 站点，见 fetchUserInfoPage）。
type userInfoPageRetrier interface {
	retriesUserInfoPages() bool
}

// getUserInfo 交给驱动取用户信息。自己按页重试的驱动直接取；别的驱动（M-Team、Rousi 等走 API 的站点）一次取完，
// 遇到一时的错误时整次按指数退避重试，每次重试先等限速的令牌（见 retryTransient）。
func (b *BaseSite[Req, Res]) getUserInfo(ctx context.Context) (UserInfo, error) {
	if r, ok := any(b.driver).(userInfoPageRetrier); ok && r.retriesUserInfoPages() {
		return b.driver.GetUserInfo(ctx)
	}
	return retryTransient(ctx, b.limiter.Wait, func() (UserInfo, error) { return b.driver.GetUserInfo(ctx) })
}

// Download downloads a torrent file by ID
func (b *BaseSite[Req, Res]) Download(ctx context.Context, torrentID string) ([]byte, error) {
	// Rate limiting
	if err := b.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	return b.download(ctx, torrentID)
}

// download 执行一次下载，不取限速令牌；调用方负责先 limiter.Wait，保证一次下载只消耗一个令牌。
func (b *BaseSite[Req, Res]) download(ctx context.Context, torrentID string) ([]byte, error) {
	startTime := time.Now()
	b.logger.Debug(
		"Downloading torrent",
		zap.String("site", b.name),
		zap.String("torrentID", torrentID),
	)

	// Prepare request
	req, err := b.driver.PrepareDownload(torrentID)
	if err != nil {
		b.logger.Error("Failed to prepare download request", zap.Error(err))
		return nil, fmt.Errorf("prepare download: %w", err)
	}

	// Execute request：一时的错误（网络、5xx）按指数退避再发，每次重试先等限速的令牌（见 retryTransient）
	res, err := retryTransient(ctx, b.limiter.Wait, func() (Res, error) { return b.driver.Execute(ctx, req) })
	if err != nil {
		b.logger.Error("Failed to execute download request", zap.Error(err))
		return nil, fmt.Errorf("execute download: %w", err)
	}

	// Parse response（两段式下载的驱动用带 ctx 的版本，第二段请求跟着调用方取消）
	var data []byte
	if p, ok := any(b.driver).(ContextDownloadParser[Res]); ok {
		data, err = p.ParseDownloadContext(ctx, res)
	} else {
		data, err = b.driver.ParseDownload(res)
	}
	if err != nil {
		// 站点上已经没有这个种子（删了；IYUU 辅种常遇到）：是站点的状态不是故障，记警告
		if errors.Is(err, ErrTorrentNotFound) {
			b.logger.Warn("Torrent not found on site", zap.String("torrentID", torrentID), zap.Error(err))
		} else {
			b.logger.Error("Failed to parse download response", zap.String("torrentID", torrentID), zap.Error(err))
		}
		return nil, fmt.Errorf("parse download: %w", err)
	}

	b.logger.Info(
		"Torrent downloaded",
		zap.String("site", b.name),
		zap.String("torrentID", torrentID),
		zap.Int("size", len(data)),
		zap.Duration("duration", time.Since(startTime)),
	)

	return data, nil
}

// Close releases any resources held by the site
func (b *BaseSite[Req, Res]) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.loggedIn = false
	b.logger.Info("Site closed", zap.String("site", b.name))
	return nil
}

// CurrentCookie 是驱动现在用的那组 Cookie（续期过就是换发后的，见 session_cookies.go）；驱动没有续期会话时 ok 为假。
func (b *BaseSite[Req, Res]) CurrentCookie() (string, bool) {
	if h, ok := any(b.driver).(SessionCookieHolder); ok {
		return h.CurrentCookie()
	}
	return "", false
}

// GetDriver returns the underlying driver (for testing purposes)
func (b *BaseSite[Req, Res]) GetDriver() Driver[Req, Res] {
	return b.driver
}

// GetRateLimiter returns the rate limiter (for testing purposes)
func (b *BaseSite[Req, Res]) GetRateLimiter() *rate.Limiter {
	return b.limiter
}

// DownloadWithHash downloads a torrent using hash if driver supports it
func (b *BaseSite[Req, Res]) DownloadWithHash(ctx context.Context, torrentID, hash string) ([]byte, error) {
	if err := b.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	if hd, ok := any(b.driver).(HashDownloader); ok {
		// 一时的错误（网络、5xx）按指数退避再下，每次重试先等限速的令牌
		return retryTransient(ctx, b.limiter.Wait, func() ([]byte, error) { return hd.DownloadWithHash(ctx, torrentID, hash) })
	}
	// 上面已取过令牌，回退路径不能再走 Download，否则一次下载会等两次限速。
	return b.download(ctx, torrentID)
}

func (b *BaseSite[Req, Res]) GetDetailFetcher() TorrentDetailFetcher {
	if fetcher, ok := any(b.driver).(TorrentDetailFetcher); ok {
		return fetcher
	}
	return nil
}
