package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/sunerpy/pt-tools/config"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"go.uber.org/zap"
	glogger "gorm.io/gorm/logger"
	"moul.io/zapgorm2"
)

const (
	configDir  = ".pt-tools"
	configName = "config.toml"
	dbFile     = "torrents.db"
)

var once sync.Once

func initViper(cfgFile string) {
	if global.GlobalViper == nil {
		global.GlobalViper = viper.New()
	}
	v := global.GlobalViper
	home, err := os.UserHomeDir()
	global.GlobalDirCfg = &config.DirConf{
		HomeDir: home,
		WorkDir: filepath.Join(home, configDir),
	}
	cobra.CheckErr(err)
	if cfgFile != "" {
		// Use config file from the flag.
		v.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		// Search config in home directory with name ".pt-tools" (without extension).
		v.SetConfigType("toml")
		confDir := filepath.Join(home, configDir)
		v.AddConfigPath(confDir)
		v.SetConfigName(configName)
	}
	v.AutomaticEnv() // read in environment variables that match
	// If a config file is found, read it in.
	if err := v.ReadInConfig(); err != nil {
		// color.Blue("使用配置文件: %s", v.ConfigFileUsed())
		// } else {
		fmt.Fprintln(os.Stderr, "Error reading config file:", err)
		panic(err)
	}
	// 将配置解析到结构体
	if err := v.Unmarshal(&global.GlobalCfg); err != nil {
		panic("配置解析失败")
	}
	if err := global.GlobalCfg.ValidateSites(); err != nil {
		color.Red("配置文件验证失败: %v", err)
		os.Exit(1)
	}
	global.GlobalDirCfg.DownloadDir = filepath.Join(global.GlobalDirCfg.WorkDir, global.GlobalCfg.Global.DownloadDir)
}

func InitViper(cfgFile string) *config.Config {
	once.Do(func() {
		initViper(cfgFile)
		var err error
		global.GlobalLogger, err = config.DefaultZapConfig.InitLogger()
		if err != nil {
			color.Red("初始化日志失败: %v", err)
			panic(err)
		}
		gormLg := zapgorm2.Logger{
			ZapLogger:     global.GlobalLogger,
			LogLevel:      glogger.Silent,
			SlowThreshold: 0,
		}
		global.GlobalDB, err = models.NewDB(gormLg)
		if err != nil {
			color.Red("初始化数据库失败: %v", err)
			panic(err)
		}
	})
	return global.GlobalCfg
}

func GetLogger() *zap.Logger {
	return global.GlobalLogger
}
