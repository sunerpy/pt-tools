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
	Attendance AttendanceService
}

// AttendanceService 是 /signin 用到的每日签到能力。
type AttendanceService interface {
	// SignIn 立即签到一个站点。
	SignIn(ctx context.Context, site string) (AttendanceOutcome, error)
	// SignInAll 对开启了自动签到、当天还没有结果的站点各签到一次，返回这些站点当天的结果。
	SignInAll(ctx context.Context) ([]AttendanceOutcome, error)
}

// AttendanceOutcome 是一个站点当天的签到结果；Status 取 signed、already、failed、unsupported、pending。
type AttendanceOutcome struct {
	Site    string
	Status  string
	Message string
	Error   string
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
	FindByChannelUser(ctx context.Context, confID uint, channelType, channelUserID string) (uint, bool, error)
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
