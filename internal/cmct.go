package internal

import (
    "context"
    "os"
    "path/filepath"
    "time"

	"github.com/gocolly/colly"
	"github.com/mmcdole/gofeed"
	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/site"
	"github.com/sunerpy/pt-tools/thirdpart/qbit"
	"github.com/sunerpy/pt-tools/utils"
	"go.uber.org/zap"
)

type CmctImpl struct {
	ctx        context.Context
	maxRetries int
	retryDelay time.Duration
	Collector  *colly.Collector
	SiteConf   *site.SiteMapConfig
	qbitClient *qbit.QbitClient
}

func NewCmctImpl(ctx context.Context) *CmctImpl {
	store := core.NewConfigStore(global.GlobalDB)
	qbc, _ := store.GetQbitOnly()
	client, err := qbit.NewQbitClient(qbc.URL, qbc.User, qbc.Password, time.Second*10)
	if err != nil {
		sLogger().Fatal("qbit认证失败", err)
	}
	co := site.NewCollectorWithTransport()
	parser := site.NewCMCTParser()
	cfg, _ := core.NewConfigStore(global.GlobalDB).Load()
	siteCfg := site.NewSiteMapConfig(models.CMCT, cfg.Sites[models.CMCT].Cookie, cfg.Sites[models.CMCT], parser)
	return &CmctImpl{
		ctx:        ctx,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
		Collector:  co,
		SiteConf:   siteCfg,
		qbitClient: client,
	}
}

func (h *CmctImpl) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	url := item.Link
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cfg, _ := core.NewConfigStore(global.GlobalDB).Load()
	siteCfg := site.NewSiteMapConfig(models.CMCT, cfg.Sites[models.CMCT].Cookie, cfg.Sites[models.CMCT], site.NewCMCTParser())
	info, err := site.CommonFetchTorrentInfo(ctx, h.Collector, siteCfg, url)
	if err != nil {
		return nil, err
	}
	res := models.APIResponse[models.PHPTorrentInfo]{
		Code:    "success",
		Data:    *info,
		Message: "success",
	}
	return &res, nil
}

func (h *CmctImpl) IsEnabled() bool {
	cfg, _ := core.NewConfigStore(global.GlobalDB).Load()
	if cfg.Sites[models.CMCT].Enabled != nil {
		return *cfg.Sites[models.CMCT].Enabled
	}
	return false
}

func (h *CmctImpl) CanbeFinished(detail models.PHPTorrentInfo) bool {
	gl, _ := core.NewConfigStore(global.GlobalDB).GetGlobalOnly()
	if !gl.DownloadLimitEnabled {
		return true
	} else {
		duration := time.Until(detail.EndTime)
		secondsDiff := int(duration.Seconds())
		if float64(secondsDiff)*float64(gl.DownloadSpeedLimit) < (detail.SizeMB / 1024 / 1024) {
			sLogger().Warn("种子免费时间不足以完成下载,跳过...", zap.String("torrent_id", detail.TorrentID))
			return false
		}
		return true
	}
}

func (h *CmctImpl) DownloadTorrent(url, title, downloadDir string) (string, error) {
	return downloadTorrent(url, title, downloadDir, h.maxRetries, h.retryDelay)
}

func (h *CmctImpl) MaxRetries() int {
	return h.maxRetries
}

func (h *CmctImpl) RetryDelay() time.Duration {
	return h.retryDelay
}

func (h *CmctImpl) SendTorrentToQbit(ctx context.Context, rssCfg models.RSSConfig) error {
	if h.qbitClient == nil {
		sLogger().Fatal("qbit client is nil")
	}
    homeDir, _ := os.UserHomeDir()
    store := core.NewConfigStore(global.GlobalDB)
    gl, _ := store.GetGlobalOnly()
    dirPath := filepath.Join(homeDir, models.WorkDir, gl.DownloadDir, rssCfg.DownloadSubPath)
	// 检查目录
    // 启动时若目录不存在则尝试创建，以免误判为空
    if _, err := os.Stat(dirPath); os.IsNotExist(err) {
        _ = os.MkdirAll(dirPath, 0o755)
    }
    exists, empty, err := utils.CheckDirectory(dirPath)
	if err != nil {
		sLogger().Errorf("检查目录失败: %v", err)
		return err
	}
	if !exists {
		sLogger().Infof("下载目录不存在(未下载种子,跳过): %s", dirPath)
		return nil
	}
	if empty {
		sLogger().Infof("下载目录为空(未下载种子,跳过): %s", dirPath)
		return nil
	}
	// 处理种子并更新数据库（包含清理与重试逻辑在 ProcessTorrentsWithDBUpdate 内部）
	err = ProcessTorrentsWithDBUpdate(ctx, h.qbitClient, dirPath, rssCfg.Category, rssCfg.Tag, models.CMCT)
	if err != nil {
		sLogger().Errorf("发送种子到 qBittorrent 失败: %v", err)
		return err
	}
	sLogger().Infof("种子处理完成并更新数据库记录, 路径: %s, 分类: %s, 标签: %s", dirPath, rssCfg.Category, rssCfg.Tag)
	return nil
}

func (h *CmctImpl) Context() context.Context {
	return h.ctx
}
