package core

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
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
	var initErr error
	once.Do(func() {
		var err error
		zapCfg := config.DefaultZapConfig
		zapCfg.ApplyEnvOverrides()
		if pruneErr := zapCfg.PruneOldLogs(); pruneErr != nil {
			fmt.Fprintf(os.Stderr, "清理历史日志失败: %v\n", pruneErr)
		}
		logger, err := zapCfg.InitLogger()
		if err != nil {
			initErr = fmt.Errorf("初始化日志失败: %w", err)
			return
		}
		global.GlobalLogger = logger
		global.GetSlogger().Info("日志系统初始化完成")

		gormLg := zapgorm2.Logger{
			ZapLogger:     global.GlobalLogger,
			LogLevel:      glogger.Silent,
			SlowThreshold: 0,
		}
		cookieStore := NewConfigStore(&models.TorrentDB{})
		backupTable := func(db *gorm.DB, table string) (string, error) {
			return migration.DumpTableJSON(db, table, "", 8, 9)
		}
		global.GlobalDB, err = models.NewDBWithVersionAndHooks(
			gormLg,
			version.Version,
			backupTable,
			cookieStore.EncryptCookie,
			cookieStore.DecryptCookie,
		)
		if err != nil {
			initErr = fmt.Errorf("初始化数据库失败: %w", err)
			return
		}
		global.GetSlogger().Info("数据库初始化完成")

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

		dispatchV2BroadcastIfReady(global.GlobalDB.DB, global.GetSlogger())

		global.GetSlogger().Info("运行时初始化完成")
	})
	// 返回捕获的错误
	return global.GlobalLogger, initErr
}
func GetLogger() *zap.Logger { return global.GlobalLogger }

// dispatchV2BroadcastIfReady wires the registered V2Broadcaster into
// MaybeSendV2Broadcast. Skipped silently when no broadcaster is configured
// (e.g. during early startup before notify.Router exists).
func dispatchV2BroadcastIfReady(db *gorm.DB, logger *zap.SugaredLogger) {
	bc := currentV2Broadcaster()
	if bc == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = MaybeSendV2Broadcast(ctx, db, bc, logger, time.Now().UTC())
}
