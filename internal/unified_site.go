package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/sunerpy/requests"
	"go.uber.org/zap"

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
}

// newUnifiedSiteImplWithID 使用指定的 siteID 创建统一站点实现
func newUnifiedSiteImplWithID(ctx context.Context, siteGroup models.SiteGroup, siteID string) (*UnifiedSiteImpl, error) {
	siteKind, ok := SiteGroupToKind[siteGroup]
	if !ok {
		return nil, fmt.Errorf("unsupported site group: %s", siteGroup)
	}

	var logger *zap.SugaredLogger
	var zapLogger *zap.Logger
	if global.GetLogger() != nil {
		logger = global.GetSlogger()
		zapLogger = global.GetLogger()
	} else {
		// 使用 nop logger 作为后备
		zapLogger = zap.NewNop()
		logger = zapLogger.Sugar()
	}
	registry := v2.NewSiteRegistry(zapLogger)

	return &UnifiedSiteImpl{
		ctx:        ctx,
		siteGroup:  siteGroup,
		siteID:     siteID,
		siteKind:   siteKind,
		registry:   registry,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
		logger:     logger,
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

// GetTorrentDetails 获取种子详情，返回统一的 TorrentItem
func (u *UnifiedSiteImpl) GetTorrentDetails(item *gofeed.Item) (*v2.TorrentItem, error) {
	if !u.IsEnabled() {
		return nil, errors.New(enableError)
	}
	if item == nil {
		return nil, fmt.Errorf("RSS item 为空")
	}

	switch u.siteKind {
	case v2.SiteMTorrent:
		return u.getMTorrentDetails(item)
	case v2.SiteNexusPHP:
		return u.getNexusPHPDetails(item)
	default:
		return nil, fmt.Errorf("unsupported site kind: %s", u.siteKind)
	}
}

// getMTorrentDetails 获取 M-Team 种子详情
func (u *UnifiedSiteImpl) getMTorrentDetails(item *gofeed.Item) (*v2.TorrentItem, error) {
	cfg, err := core.NewConfigStore(global.GlobalDB).Load()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}
	sc := cfg.Sites[u.siteGroup]
	if sc.APIUrl == "" || sc.APIKey == "" {
		return nil, fmt.Errorf("站点 API 未配置")
	}

	data := fmt.Sprintf("id=%s", item.GUID)
	apiPath := fmt.Sprintf("%s%s", sc.APIUrl, torrentDetailPath)

	resp, err := requests.Post(apiPath, strings.NewReader(data),
		requests.WithContext(u.ctx),
		requests.WithTimeout(30*time.Second),
		requests.WithHeader("x-api-key", sc.APIKey),
		requests.WithHeader("Content-Type", mteamContentType),
	)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("请求失败，状态码: %d", resp.StatusCode)
	}

	var responseData models.APIResponse[models.MTTorrentDetail]
	if err := json.Unmarshal(resp.Bytes(), &responseData); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %w", err)
	}

	// 转换为 v2.TorrentItem
	return u.convertMTTorrentToItem(&responseData.Data), nil
}

// getNexusPHPDetails 获取 NexusPHP 种子详情
// 使用 site/v2 中的新解析器
func (u *UnifiedSiteImpl) getNexusPHPDetails(item *gofeed.Item) (*v2.TorrentItem, error) {
	url := item.Link
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg, err := core.NewConfigStore(global.GlobalDB).Load()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}

	// 使用 site/v2 的新解析器
	var v2Parser v2.NexusPHPDetailParser
	switch u.siteGroup {
	case models.HDSKY:
		v2Parser = v2.NewHDSkyParser()
	case models.SpringSunday:
		v2Parser = v2.NewSpringSundayParser()
	case models.TTG:
		v2Parser = v2.NewTTGParser()
	default:
		v2Parser = v2.NewHDSkyParser() // 默认使用 HDSky 解析器
	}

	// 使用旧的 colly 方式获取页面，但用新解析器解析
	siteCfg := NewSiteMapConfig(u.siteGroup, cfg.Sites[u.siteGroup].Cookie, cfg.Sites[u.siteGroup], NewLegacyParserAdapter(v2Parser))
	co := NewCollectorWithTransport()

	info, err := CommonFetchTorrentInfo(ctx, co, siteCfg, url)
	if err != nil {
		return nil, err
	}

	// 转换为 v2.TorrentItem
	return u.convertPHPTorrentToItem(info), nil
}

// convertMTTorrentToItem 将 MTTorrentDetail 转换为 v2.TorrentItem
func (u *UnifiedSiteImpl) convertMTTorrentToItem(detail *models.MTTorrentDetail) *v2.TorrentItem {
	item := &v2.TorrentItem{
		ID:            detail.ID,
		Title:         detail.Name,
		SourceSite:    string(u.siteGroup),
		DiscountLevel: v2.DiscountNone,
	}

	// 解析大小
	if detail.Size != "" {
		var sizeBytes int64
		if _, err := fmt.Sscanf(detail.Size, "%d", &sizeBytes); err == nil {
			item.SizeBytes = sizeBytes
		}
	}

	// 解析优惠信息
	if detail.Status != nil {
		item.DiscountLevel = u.mapMTDiscountLevel(detail.Status.Discount)

		// 解析优惠结束时间
		if detail.Status.DiscountEndTime != "" {
			if endTime, err := v2.ParseTimeInCST("2006-01-02 15:04:05", detail.Status.DiscountEndTime); err == nil {
				item.DiscountEndTime = endTime
			}
		}
	}

	// 设置标签
	if detail.SmallDescr != "" {
		item.Tags = []string{detail.SmallDescr}
	}

	return item
}

// convertPHPTorrentToItem 将 PHPTorrentInfo 转换为 v2.TorrentItem
func (u *UnifiedSiteImpl) convertPHPTorrentToItem(info *models.PHPTorrentInfo) *v2.TorrentItem {
	item := &v2.TorrentItem{
		ID:              info.TorrentID,
		Title:           info.Title,
		SizeBytes:       int64(info.SizeMB * 1024 * 1024),
		Seeders:         info.Seeders,
		Leechers:        info.Leechers,
		SourceSite:      string(u.siteGroup),
		DiscountLevel:   u.mapPHPDiscountLevel(info.Discount),
		DiscountEndTime: info.EndTime,
		HasHR:           info.HR,
	}

	// 设置标签
	if info.SubTitle != "" {
		item.Tags = []string{info.SubTitle}
	}

	return item
}

// mapMTDiscountLevel 将 M-Team 优惠类型映射到 v2.DiscountLevel
func (u *UnifiedSiteImpl) mapMTDiscountLevel(discount string) v2.DiscountLevel {
	discount = strings.ToUpper(strings.TrimSpace(discount))
	switch discount {
	case "FREE":
		return v2.DiscountFree
	case "_2X_FREE", "2XFREE":
		return v2.Discount2xFree
	case "PERCENT_50", "50%":
		return v2.DiscountPercent50
	case "PERCENT_30", "30%":
		return v2.DiscountPercent30
	case "PERCENT_70", "70%":
		return v2.DiscountPercent70
	case "_2X_UP", "2XUP":
		return v2.Discount2xUp
	case "_2X_PERCENT_50", "2X50%":
		return v2.Discount2x50
	default:
		return v2.DiscountNone
	}
}

// mapPHPDiscountLevel 将 PHP 优惠类型映射到 v2.DiscountLevel
func (u *UnifiedSiteImpl) mapPHPDiscountLevel(discount models.DiscountType) v2.DiscountLevel {
	switch discount {
	case models.DISCOUNT_FREE:
		return v2.DiscountFree
	case models.DISCOUNT_TWO_X_FREE:
		return v2.Discount2xFree
	case models.DISCOUNT_TWO_X:
		return v2.Discount2xUp
	case models.DISCOUNT_FIFTY:
		return v2.DiscountPercent50
	case models.DISCOUNT_THIRTY:
		return v2.DiscountPercent30
	case models.DISCOUNT_TWO_X_FIFTY:
		return v2.Discount2x50
	default:
		return v2.DiscountNone
	}
}

// SendTorrentToQbit 发送种子到下载器
func (u *UnifiedSiteImpl) SendTorrentToQbit(ctx context.Context, rssCfg models.RSSConfig) error {
	homeDir, _ := os.UserHomeDir()
	store := core.NewConfigStore(global.GlobalDB)
	gl, _ := store.GetGlobalOnly()
	base, berr := utils.ResolveDownloadBase(homeDir, models.WorkDir, gl.DownloadDir)
	if berr != nil {
		return berr
	}
	sub := utils.SubPathFromTag(rssCfg.Tag)
	dirPath := filepath.Join(base, sub)

	// 检查目录
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		_ = os.MkdirAll(dirPath, 0o755)
	}
	exists, empty, err := utils.CheckDirectory(dirPath)
	if err != nil {
		u.logger.Errorf("检查目录失败: %v", err)
		return err
	}
	if !exists {
		u.logger.Infof("下载目录不存在(未下载种子,跳过): %s", dirPath)
		return nil
	}
	if empty {
		u.logger.Infof("下载目录为空(未下载种子,跳过): %s", dirPath)
		return nil
	}

	// 使用下载器选择逻辑
	err = ProcessTorrentsWithDownloaderByRSS(ctx, rssCfg, dirPath, rssCfg.Category, rssCfg.Tag, u.siteGroup)
	if err != nil {
		u.logger.Errorf("发送种子到下载器失败: %v", err)
		return err
	}
	u.logger.Infof("种子处理完成并更新数据库记录, 路径: %s, 分类: %s, 标签: %s", dirPath, rssCfg.Category, rssCfg.Tag)
	return nil
}
