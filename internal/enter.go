package internal

import (
	"context"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

const (
	enableError       = "当前站点未启用"
	torrentDetailPath = "/api/torrent/detail"
	mteamContentType  = "application/x-www-form-urlencoded"
	maxRetries        = 3
	retryDelay        = 5 * time.Second
)

func sLogger() *zap.SugaredLogger {
	if global.GetLogger() == nil {
		return zap.NewNop().Sugar()
	}
	return global.GetSlogger()
}

type TorrentScheduleFunc func(torrent models.TorrentInfo)

var (
	scheduleFuncMu   sync.RWMutex
	scheduleTorrentF TorrentScheduleFunc

	dlManagerMu         sync.RWMutex
	globalDownloaderMgr *downloader.DownloaderManager
)

func RegisterTorrentScheduler(f TorrentScheduleFunc) {
	scheduleFuncMu.Lock()
	defer scheduleFuncMu.Unlock()
	scheduleTorrentF = f
}

func ScheduleTorrentForMonitoring(torrent models.TorrentInfo) {
	scheduleFuncMu.RLock()
	f := scheduleTorrentF
	scheduleFuncMu.RUnlock()
	if f != nil {
		f(torrent)
	}
}

func SetGlobalDownloaderManager(dm *downloader.DownloaderManager) {
	dlManagerMu.Lock()
	defer dlManagerMu.Unlock()
	globalDownloaderMgr = dm
}

func GetGlobalDownloaderManager() *downloader.DownloaderManager {
	dlManagerMu.RLock()
	defer dlManagerMu.RUnlock()
	return globalDownloaderMgr
}

var (
	rssNotifierMu sync.RWMutex
	rssNotifier   RSSNotifier
)

// RSSItemNotice is the minimal payload the RSS pipeline needs to fire an
// "all" notification — purposely dependency-light so internal does not import
// internal/app (which would create a cycle via scheduler).
type RSSItemNotice struct {
	RSS       *models.RSSConfig
	FeedItem  *gofeed.Item
	SiteName  string
	TorrentID string
}

// RSSFilteredNotice is the payload for the 'filtered' RSS notification path.
// Mirror of app.RSSFilteredEvent but defined here to avoid the
// internal → internal/app import cycle. Bridged by rssNotifierAdapter
// in cmd/web.go.
type RSSFilteredNotice struct {
	RSS       *models.RSSConfig
	Torrent   *v2.TorrentItem
	Rule      *models.FilterRule
	SiteName  string
	TorrentID string
}

// RSSNotifier is the structural contract internal/app/rssNotifier satisfies.
// The concrete adapter lives in cmd/web.go and bridges this minimal type to
// app.RSSItemEvent before delegating to the real app.RSSNotifier.
type RSSNotifier interface {
	NotifyNewItem(ctx context.Context, ev RSSItemNotice) error
	NotifyFilteredItem(ctx context.Context, ev RSSFilteredNotice) error
}

func SetRSSNotifier(n RSSNotifier) {
	rssNotifierMu.Lock()
	rssNotifier = n
	rssNotifierMu.Unlock()
}

func getRSSNotifier() RSSNotifier {
	rssNotifierMu.RLock()
	defer rssNotifierMu.RUnlock()
	return rssNotifier
}
