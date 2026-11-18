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

// SubscriptionOfferFunc 把 RSS 取到详情的种子交给订阅（实现不能阻塞：订阅自己排队处理）。
type SubscriptionOfferFunc func(site string, item v2.TorrentItem)

var (
	subscriptionOfferMu sync.RWMutex
	subscriptionOfferF  SubscriptionOfferFunc
)

// SetSubscriptionOffer 登记订阅的入口（启动时由 cmd 接上；为 nil 时 RSS 不交给订阅）。
func SetSubscriptionOffer(f SubscriptionOfferFunc) {
	subscriptionOfferMu.Lock()
	defer subscriptionOfferMu.Unlock()
	subscriptionOfferF = f
}

// offerToSubscriptions 把 RSS 里取到详情的种子交给订阅。详情里没有种子编号时从 RSS 条目的链接里取（取不到用 GUID）。
func offerToSubscriptions(site string, detail *v2.TorrentItem, feedItem *gofeed.Item) {
	subscriptionOfferMu.RLock()
	f := subscriptionOfferF
	subscriptionOfferMu.RUnlock()
	if f == nil || detail == nil {
		return
	}
	it := *detail
	if it.SourceSite == "" {
		it.SourceSite = site
	}
	if it.ID == "" && feedItem != nil {
		if _, ref := extractTorrentRef(feedItem); ref != "" {
			it.ID = ref
		} else {
			it.ID = feedItem.GUID
		}
	}
	if it.Title == "" && feedItem != nil {
		it.Title = feedItem.Title
	}
	f(site, it)
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
