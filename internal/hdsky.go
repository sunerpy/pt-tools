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
	"go.uber.org/zap"
)

type HdskyImpl struct {
	ctx        context.Context
	maxRetries int
	retryDelay time.Duration
	Collector  *colly.Collector
	SiteConf   *site.SiteMapConfig
	qbitClient *qbit.QbitClient
}

func NewHdskyImpl(ctx context.Context) *HdskyImpl {
	client, err := qbit.NewQbitClient(global.GetGlobalConfig().Qbit.URL, global.GetGlobalConfig().Qbit.User, global.GetGlobalConfig().Qbit.Password, time.Second*10)
	if err != nil {
		global.GlobalLogger.Fatal("认证失败", zap.Error(err))
	}
	co := site.NewCollectorWithTransport()
	parser := site.NewHDSkyParser()
	siteCfg := site.NewSiteMapConfig(string(models.HDSKY), global.GetGlobalConfig().Sites[models.HDSKY].Cookie, global.GetGlobalConfig().Sites[models.HDSKY], parser)
	return &HdskyImpl{
		ctx:        ctx,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
		Collector:  co,
		SiteConf:   siteCfg,
		qbitClient: client,
	}
}

func (h *HdskyImpl) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
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

func (h *HdskyImpl) IsEnabled() bool {
	if global.GetGlobalConfig().Sites[models.HDSKY].Enabled != nil {
		return *global.GetGlobalConfig().Sites[models.HDSKY].Enabled
	}
	return false
}

func (h *HdskyImpl) CanbeFinished(detail models.PHPTorrentInfo) bool {
	if !global.GetGlobalConfig().Global.DownloadLimitEnabled {
		return true
	} else {
		duration := detail.EndTime.Sub(time.Now())
		secondsDiff := int(duration.Seconds())
		if float64(secondsDiff)*float64(global.GetGlobalConfig().Global.DownloadSpeedLimit) < (detail.SizeMB / 1024 / 1024) {
			global.GlobalLogger.Warn("种子免费时间不足以完成下载,跳过...", zap.String("torrent_id", detail.TorrentID))
			return false
		}
		return true
	}
}

func (h *HdskyImpl) DownloadTorrent(url, title, downloadDir string) (string, error) {
	return downloadTorrent(url, title, downloadDir, h.maxRetries, h.retryDelay)
}

func (h *HdskyImpl) MaxRetries() int {
	return h.maxRetries
}

func (h *HdskyImpl) RetryDelay() time.Duration {
	return h.retryDelay
}

func (h *HdskyImpl) SendTorrentToQbit(ctx context.Context, rssCfg config.RSSConfig) error {
	if h.qbitClient == nil {
		global.GlobalLogger.Fatal("qbit client is nil")
	}
	dirPath := filepath.Join(global.GlobalDirCfg.DownloadDir, rssCfg.DownloadSubPath)
	err := ProcessTorrentsWithDBUpdate(ctx, h.qbitClient, dirPath, rssCfg.Category, rssCfg.Tag, global.GetGlobalConfig().Sites[models.HDSKY].Name)
	if err != nil {
		global.GlobalLogger.Fatal("发送种子到 qBittorrent 失败", zap.Error(err))
		return err
	}
	global.GlobalLogger.Info("种子处理完成并更新数据库记录")
	return nil
}

func (h *HdskyImpl) Context() context.Context {
	return h.ctx
}
