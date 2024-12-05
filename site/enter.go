package site

import (
	"github.com/sunerpy/pt-tools/global"
	"go.uber.org/zap"
)

func sLogger() *zap.SugaredLogger {
	return global.GetSlogger()
}
