package global

import (
	"sync"

	"github.com/spf13/viper"
	"github.com/sunerpy/pt-tools/config"
	"github.com/sunerpy/pt-tools/models"
	"go.uber.org/zap"
)

var (
	GlobalCfg    *config.Config
	GlobalLogger *zap.Logger
	GlobalDB     *models.TorrentDB
	GlobalDirCfg *config.DirConf
	GlobalViper  *viper.Viper
	once         sync.Once
)

func GetGlobalConfig() *config.Config {
	return GlobalCfg
}

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
