package internal

import (
	"context"
	"path/filepath"
	"time"

	"github.com/gocolly/colly"
	"github.com/mmcdole/gofeed"
	"github.com/sunerpy/pt-tools/config"
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
	client, err := qbit.NewQbitClient(global.GetGlobalConfig().Qbit.URL, global.GetGlobalConfig().Qbit.User, global.GetGlobalConfig().Qbit.Password, time.Second*10)
	if err != nil {
		sLogger().Fatal("qbit认证失败", err)
	}
	co := site.NewCollectorWithTransport()
	parser := site.NewCMCTParser()
	siteCfg := site.NewSiteMapConfig(models.CMCT, global.GetGlobalConfig().Sites[models.CMCT].Cookie, global.GetGlobalConfig().Sites[models.CMCT], parser)
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
	info, err := site.CommonFetchTorrentInfo(ctx, h.Collector, h.SiteConf, url)
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
	if global.GetGlobalConfig().Sites[models.CMCT].Enabled != nil {
		return *global.GetGlobalConfig().Sites[models.CMCT].Enabled
	}
	return false
}

func (h *CmctImpl) CanbeFinished(detail models.PHPTorrentInfo) bool {
	if !global.GetGlobalConfig().Global.DownloadLimitEnabled {
		return true
	} else {
		duration := detail.EndTime.Sub(time.Now())
		secondsDiff := int(duration.Seconds())
		if float64(secondsDiff)*float64(global.GetGlobalConfig().Global.DownloadSpeedLimit) < (detail.SizeMB / 1024 / 1024) {
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

func (h *CmctImpl) SendTorrentToQbit(ctx context.Context, rssCfg config.RSSConfig) error {
	if h.qbitClient == nil {
		sLogger().Fatal("qbit client is nil")
	}
	dirPath := filepath.Join(global.GlobalDirCfg.DownloadDir, rssCfg.DownloadSubPath)
	// 检查目录
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
	// 处理种子并更新数据库
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
