package internal

import (
	"time"
)

const (
	enableError       = "当前站点未启用"
	torrentDetailPath = "/torrent/detail"
	mteamContentType  = "application/x-www-form-urlencoded"
	maxRetries        = 3
	retryDelay        = 5 * time.Second
	maxGoroutine      = 3
)
