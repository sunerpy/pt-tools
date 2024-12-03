package global

import (
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
)

func GetGlobalConfig() *config.Config {
	return GlobalCfg
}
