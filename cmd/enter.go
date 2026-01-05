package cmd

import (
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/global"
)

func sLogger() *zap.SugaredLogger {
	return global.GetSlogger()
}
