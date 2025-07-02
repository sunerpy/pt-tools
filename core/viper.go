package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

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

func initViper(cfgFile string) error {
	if global.GlobalViper == nil {
		global.GlobalViper = viper.New()
	}
	v := global.GlobalViper
	// 获取用户主目录
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("无法获取用户主目录: %w", err)
	}
	// 初始化目录配置
	global.GlobalDirCfg = &config.DirConf{
		HomeDir: home,
		WorkDir: filepath.Join(home, configDir),
	}
	// 设置配置文件路径
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		fmt.Println("使用默认配置文件")
		v.SetConfigType("toml")
		confDir := filepath.Join(home, configDir)
		v.AddConfigPath(confDir)
		v.SetConfigName(configName)
	}
	// 读取环境变量
	v.AutomaticEnv()
	// 读取配置文件
	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}
	// 将配置解析到结构体
	if err := v.Unmarshal(&global.GlobalCfg); err != nil {
		return fmt.Errorf("配置解析失败: %w", err)
	}
	// 验证配置文件
	if err := global.GlobalCfg.ValidateSites(); err != nil {
		return fmt.Errorf("配置文件验证失败: %w", err)
	}
	// 初始化下载目录
	global.GlobalDirCfg.DownloadDir = filepath.Join(global.GlobalDirCfg.WorkDir, global.GlobalCfg.Global.DownloadDir)
	return nil
}

func InitViper(cfgFile string) (*zap.Logger, error) {
	var initErr error // 用于捕获 `once.Do` 内部的错误
	once.Do(func() {
		// 调用 initViper，并捕获可能的错误
		if err := initViper(cfgFile); err != nil {
			initErr = fmt.Errorf("初始化配置失败: %w", err)
			return
		}
		// 初始化日志
		var err error
		global.GlobalLogger, err = config.DefaultZapConfig.InitLogger()
		if err != nil {
			initErr = fmt.Errorf("初始化日志失败: %w", err)
			return // 直接返回，避免继续执行
		}
		// 配置 GORM 日志
		gormLg := zapgorm2.Logger{
			ZapLogger:     global.GlobalLogger,
			LogLevel:      glogger.Silent,
			SlowThreshold: 0,
		}
		// 初始化数据库
		global.GlobalDB, err = models.NewDB(gormLg)
		if err != nil {
			initErr = fmt.Errorf("初始化数据库失败: %w", err)
			return
		}
	})
	// 返回捕获的错误
	return global.GlobalLogger, initErr
}

func GetLogger() *zap.Logger {
	return global.GlobalLogger
}
