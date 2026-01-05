package downloader

import (
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/global"
)

func sLogger() *zap.SugaredLogger {
	if global.GetLogger() == nil {
		return zap.NewNop().Sugar()
	}
	return global.GetSlogger()
}
