package internal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mmcdole/gofeed"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/utils"
)

// UnifiedSiteImpl 统一站点实现
// 使用 site/v2 的 Driver 模式，无需为每个站点创建单独的实现
type UnifiedSiteImpl struct {
	ctx        context.Context
	siteGroup  models.SiteGroup
	siteID     string
	siteKind   v2.SiteKind
	registry   *v2.SiteRegistry
	maxRetries int
	retryDelay time.Duration
	logger     *zap.SugaredLogger
	limiter    *v2.PersistentRateLimiter
}

func getDBInstance() *gorm.DB {
	if global.GlobalDB != nil && global.GlobalDB.DB != nil {
		return global.GlobalDB.DB
	}
	return nil
}

func newUnifiedSiteImplWithID(ctx context.Context, siteGroup models.SiteGroup, siteID string, siteKind v2.SiteKind) (*UnifiedSiteImpl, error) {
	var logger *zap.SugaredLogger
	var zapLogger *zap.Logger
	if global.GetLogger() != nil {
		logger = global.GetSlogger()
	} else {
		zapLogger = zap.NewNop()
		logger = zapLogger.Sugar()
	}

	// 从站点定义获取速率限制配置
	rateLimit := 2.0 // 默认: 2 请求/秒
	rateBurst := 5   // 默认: burst 5
	var rateWindow time.Duration
	var rateWindowLimit int
	if def := v2.GetDefinitionRegistry().GetOrDefault(siteID); def != nil {
		if def.RateLimit > 0 {
			rateLimit = def.RateLimit
		}
		if def.RateBurst > 0 {
			rateBurst = def.RateBurst
		}
		rateWindow = def.RateWindow
		rateWindowLimit = def.RateWindowLimit
		logger.Debugf("[速率限制] 站点=%s, RateLimit=%.4f/s, Burst=%d", siteID, rateLimit, rateBurst)
	}

	var limiter *v2.PersistentRateLimiter
	db := getDBInstance()
	if rateWindow > 0 && rateWindowLimit > 0 {
		limiter = v2.NewPersistentRateLimiter(v2.PersistentRateLimiterConfig{
			DB:     db,
			SiteID: siteID,
			Limit:  rateWindowLimit,
			Window: rateWindow,
		})
		logger.Debugf("[速率限制] 站点=%s, 使用滑动窗口: %d次/%v", siteID, rateWindowLimit, rateWindow)
	} else {
		limiter = v2.NewPersistentRateLimiterFromRPS(db, siteID, rateLimit, rateBurst)
	}

	return &UnifiedSiteImpl{
		ctx:        ctx,
		siteGroup:  siteGroup,
		siteID:     siteID,
		siteKind:   siteKind,
		registry:   v2.GetGlobalSiteRegistry(),
		maxRetries: maxRetries,
		retryDelay: retryDelay,
		logger:     logger,
		limiter:    limiter,
	}, nil
}

// SiteGroup 返回站点分组标识
func (u *UnifiedSiteImpl) SiteGroup() models.SiteGroup {
	return u.siteGroup
}

// Context 返回上下文
func (u *UnifiedSiteImpl) Context() context.Context {
	return u.ctx
}

// IsEnabled 检查站点是否启用
func (u *UnifiedSiteImpl) IsEnabled() bool {
	if global.GlobalDB == nil {
		return false
	}
	cfg, err := core.NewConfigStore(global.GlobalDB).Load()
	if err != nil {
		return false
	}
	sc, ok := cfg.Sites[u.siteGroup]
	if !ok {
		return false
	}
	if sc.Enabled != nil {
		return *sc.Enabled
	}
	return false
}

// MaxRetries 返回最大重试次数
func (u *UnifiedSiteImpl) MaxRetries() int {
	return u.maxRetries
}

// RetryDelay 返回重试间隔
func (u *UnifiedSiteImpl) RetryDelay() time.Duration {
	return u.retryDelay
}

// DownloadTorrent 下载种子文件，返回 torrent hash
func (u *UnifiedSiteImpl) DownloadTorrent(url, title, downloadDir string) (string, error) {
	return downloadTorrent(url, title, downloadDir, u.maxRetries, u.retryDelay)
}

// waitForRateLimit 等待速率限制，返回等待时间
func (u *UnifiedSiteImpl) waitForRateLimit(ctx context.Context) error {
	return u.limiter.Wait(ctx)
}

// GetTorrentDetails 获取种子详情，返回统一的 TorrentItem
func (u *UnifiedSiteImpl) GetTorrentDetails(item *gofeed.Item) (*v2.TorrentItem, error) {
	if !u.IsEnabled() {
		return nil, errors.New(enableError)
	}
	if item == nil {
		return nil, fmt.Errorf("RSS item 为空")
	}

	if err := u.waitForRateLimit(u.ctx); err != nil {
		return nil, fmt.Errorf("速率限制等待失败: %w", err)
	}

	u.logger.Debugf("[获取种子详情] 站点=%s, ID=%s, 标题=%s", u.siteGroup, item.GUID, item.Title)

	cfg, err := core.NewConfigStore(global.GlobalDB).Load()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}
	sc := cfg.Sites[u.siteGroup]

	creds := v2.SiteCredentials{
		Cookie:  sc.Cookie,
		APIKey:  sc.APIKey,
		Passkey: sc.Passkey,
	}

	baseURL := ""
	if def := v2.GetDefinitionRegistry().GetOrDefault(u.siteID); def != nil && len(def.URLs) > 0 {
		baseURL = def.URLs[0]
	}
	if sc.APIUrl != "" {
		baseURL = sc.APIUrl
	}

	site, err := u.registry.CreateSite(u.siteID, creds, baseURL)
	if err != nil {
		return nil, fmt.Errorf("创建站点实例失败: %w", err)
	}
	defer site.Close()

	provider, ok := site.(v2.DetailFetcherProvider)
	if !ok {
		return nil, fmt.Errorf("站点 %s 不支持 DetailFetcherProvider 接口", u.siteID)
	}

	fetcher := provider.GetDetailFetcher()
	if fetcher == nil {
		return nil, fmt.Errorf("站点 %s 未实现 TorrentDetailFetcher", u.siteID)
	}

	return fetcher.GetTorrentDetail(u.ctx, item.GUID, item.Link, item.Title)
}

// SendTorrentToDownloader 发送种子到下载器
func (u *UnifiedSiteImpl) SendTorrentToDownloader(ctx context.Context, rssCfg models.RSSConfig) error {
	homeDir, _ := os.UserHomeDir()
	store := core.NewConfigStore(global.GlobalDB)
	gl, _ := store.GetGlobalOnly()
	base, berr := utils.ResolveDownloadBase(homeDir, models.WorkDir, gl.DownloadDir)
	if berr != nil {
		return berr
	}
	sub := utils.SubPathFromTag(rssCfg.Tag)
	dirPath := filepath.Join(base, sub)

	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		_ = os.MkdirAll(dirPath, 0o755)
	}
	exists, empty, err := utils.CheckDirectory(dirPath)
	if err != nil {
		u.logger.Errorf("检查目录失败: %v", err)
		return err
	}
	if !exists {
		u.logger.Infof("[跳过推送] 站点=%s, 下载目录不存在: %s", u.siteGroup, dirPath)
		return nil
	}
	if empty {
		u.logger.Infof("[跳过推送] 站点=%s, 下载目录为空: %s", u.siteGroup, dirPath)
		return nil
	}

	err = ProcessTorrentsWithDownloaderByRSS(ctx, rssCfg, dirPath, rssCfg.Category, rssCfg.Tag, u.siteGroup)
	if err != nil {
		u.logger.Errorf("[推送失败] 站点=%s, 错误=%v", u.siteGroup, err)
		return err
	}
	return nil
}
