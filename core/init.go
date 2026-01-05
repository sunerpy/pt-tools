package core

import (
	"fmt"
	"sync"

	"go.uber.org/zap"
	glogger "gorm.io/gorm/logger"
	"moul.io/zapgorm2"

	"github.com/sunerpy/pt-tools/config"
	"github.com/sunerpy/pt-tools/core/migration"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/version"
)

var once sync.Once

// 取消 viper 文件读取，统一改为 DB 初始化与迁移（保留 once 初始化）
// InitViper 改造为：初始化日志、数据库，然后优先从 DB 加载全局配置；若 DB 为空且提供了文件，则允许后续迁移命令将文件写入 DB
func InitRuntime() (*zap.Logger, error) {
	var initErr error // 用于捕获 `once.Do` 内部的错误
	once.Do(func() {
		// removed filesystem dir cache
		// 不再读取文件配置，保留 cfgFile 参数用于兼容（未来可触发迁移命令）
		// 初始化日志
		var err error
		logger, err := config.DefaultZapConfig.InitLogger()
		if err != nil {
			initErr = fmt.Errorf("初始化日志失败: %w", err)
			return
		}
		global.GlobalLogger = logger
		// 配置 GORM 日志
		gormLg := zapgorm2.Logger{
			ZapLogger:     global.GlobalLogger,
			LogLevel:      glogger.Silent,
			SlowThreshold: 0,
		}
		global.GlobalDB, err = models.NewDBWithVersion(gormLg, version.Version)
		if err != nil {
			initErr = fmt.Errorf("初始化数据库失败: %w", err)
			return
		}

		// 执行配置迁移（v1 -> v2）
		migrationService := migration.NewMigrationService(global.GlobalDB.DB)
		if migrationService.IsMigrationNeeded() {
			global.GetSlogger().Info("检测到需要迁移配置，开始执行迁移...")
			result := migrationService.MigrateV1ToV2()
			if result.Success {
				global.GetSlogger().Infof("配置迁移成功: 迁移了 %d 个下载器, %d 个站点", result.DownloadersMigrated, result.SitesMigrated)
			} else {
				global.GetSlogger().Errorf("配置迁移失败: %s", result.Message)
				for _, e := range result.Errors {
					global.GetSlogger().Errorf("迁移错误: %s", e)
				}
			}
		}

		// 禁用默认预设的自动写入，防止与用户 DB 配置冲突
		// 优先从 DB 加载配置：仅设置目录缓存
		// removed dir cache update
	})
	// 返回捕获的错误
	return global.GlobalLogger, initErr
}
func GetLogger() *zap.Logger { return global.GlobalLogger }
