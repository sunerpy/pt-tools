package internal

import (
	"time"

	"github.com/sunerpy/pt-tools/global"
	"go.uber.org/zap"
)

const (
	enableError       = "当前站点未启用"
	torrentDetailPath = "/torrent/detail"
	mteamContentType  = "application/x-www-form-urlencoded"
	maxRetries        = 3
	retryDelay        = 5 * time.Second
	maxGoroutine      = 3
)

func sLogger() *zap.SugaredLogger {
	if global.GetLogger() == nil {
		return zap.NewNop().Sugar()
	}
	return global.GetSlogger()
}
