package internal

import (
	"time"

	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/global"
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
