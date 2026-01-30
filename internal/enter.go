package internal

import (
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
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
