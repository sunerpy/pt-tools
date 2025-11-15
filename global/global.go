package global

import (
	"sync"

	"github.com/sunerpy/pt-tools/models"
	"go.uber.org/zap"
)

var (
	GlobalLogger *zap.Logger
	GlobalDB     *models.TorrentDB
	once         sync.Once
)

// Deprecated: GlobalCfg removed; use ConfigStore.Load() when needed
func InitLogger(logger *zap.Logger) {
	once.Do(func() {
		GlobalLogger = logger
	})
}

func GetLogger() *zap.Logger {
	return GlobalLogger
}

func GetSlogger() *zap.SugaredLogger {
	return GlobalLogger.Sugar()
}
