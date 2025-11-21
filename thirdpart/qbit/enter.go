package qbit

import (
	"github.com/sunerpy/pt-tools/global"
	"go.uber.org/zap"
)

func sLogger() *zap.SugaredLogger {
	if global.GetLogger() == nil {
		return zap.NewNop().Sugar()
	}
	return global.GetSlogger()
}
