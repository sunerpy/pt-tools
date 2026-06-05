package commands

import (
	"context"
	"sync"

	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// DownloaderStatusSource 抽象 *downloader.DownloaderManager.GetAllDownloaderStatus()。
type DownloaderStatusSource interface {
	GetAllDownloaderStatus() []downloader.DownloaderStatus
}

// Services 是 chatops 命令访问的业务依赖集合。
type Services struct {
	Task       app.TaskService
	Torrent    app.TorrentService
	Site       app.SiteService
	Binding    app.BindingService
	Downloader DownloaderStatusSource
	RSSWizard  RSSWizardService
	Bindings   BindingResolver
	Sessions   chatops.SessionStoreAPI
}

type RSSWizardService interface {
	AppendRSSToSite(siteName string, entry models.RSSConfig) (models.RSSConfig, error)
	ListRSSForSite(siteName string) ([]models.RSSConfig, error)
	DeleteRSSFromSite(siteName string, rssID uint) (models.RSSConfig, error)
	ListDownloaders(ctx context.Context) ([]DownloaderOption, error)
	ListFilterRules(ctx context.Context) ([]IDNameOption, error)
	ListNotificationChannels(ctx context.Context) ([]IDNameOption, error)
}

type DownloaderOption struct {
	ID        uint
	Name      string
	IsDefault bool
}

type IDNameOption struct {
	ID   uint
	Name string
}

// BindingResolver 用于 /unbind 命令解析自身 binding ID。
type BindingResolver interface {
	FindByChannelUser(ctx context.Context, channelType, channelUserID string) (uint, bool, error)
}

var (
	servicesMu sync.RWMutex
	current    *Services
)

func SetServices(s *Services) {
	servicesMu.Lock()
	defer servicesMu.Unlock()
	current = s
}

func getServices() *Services {
	servicesMu.RLock()
	defer servicesMu.RUnlock()
	return current
}
