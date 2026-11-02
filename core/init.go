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
	"github.com/sunerpy/pt-tools/internal/backup"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/version"
)

var once sync.Once

// pendingRestoreEnabled 为真时，InitRuntime 在打开数据库以前换上待恢复的备份。只有 Web 服务进程打开它
// （EnablePendingRestore）：别的子命令也走 InitRuntime，服务还在跑的时候不能去挪它正在用的数据库文件。
var pendingRestoreEnabled bool

// EnablePendingRestore 让这个进程在 InitRuntime 里换上待恢复的备份（Web 服务启动时调用）。
func EnablePendingRestore() { pendingRestoreEnabled = true }

// preUpgrade 记下这次启动的升级前备份：钩子在数据库打开以前跑，结果等数据库打开以后再写进备份设置。
var preUpgrade struct {
	ran  bool
	at   time.Time
	name string
	err  error
}

// backupMigrationTable 是迁移改表前的备份：整张表写到 ~/.pt-tools/backups，文件名带这一步迁移的版本。
func backupMigrationTable(db *gorm.DB, table string, fromVer, toVer int) (string, error) {
	return migration.DumpTableJSON(db, table, "", fromVer, toVer)
}

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

		restored, restoreErr := applyPendingRestore()
		if restoreErr != nil {
			initErr = restoreErr
			return
		}

		if keyErr := checkSecretKeyFile(); keyErr != nil {
			initErr = keyErr
			return
		}

		gormLg := zapgorm2.Logger{
			ZapLogger:     global.GlobalLogger,
			LogLevel:      glogger.Silent,
			SlowThreshold: 0,
		}
		cookieStore := NewConfigStore(&models.TorrentDB{})
		global.GlobalDB, err = models.NewDBWithVersionAndHooks(
			gormLg,
			version.Version,
			backupMigrationTable,
			cookieStore.EncryptCookie,
			cookieStore.DecryptCookie,
			preUpgradeBackup,
		)
		if err != nil {
			initErr = fmt.Errorf("初始化数据库失败: %w", err)
			return
		}
		global.GetSlogger().Info("数据库初始化完成")

		if keyErr := checkGeneratedSecretKey(global.GlobalDB.DB); keyErr != nil {
			initErr = keyErr
			return
		}
		recordStartupBackups(NewConfigStore(global.GlobalDB), restored)

		migrationService := migration.NewMigrationService(global.GlobalDB.DB)
		if migrationService.IsMigrationNeeded() {
			global.GetSlogger().Info("检测到需要迁移配置，开始执行迁移...")
			logMigrationResult(global.GetSlogger(), migrationService.MigrateV1ToV2())
		}

		dispatchV2BroadcastIfReady(global.GlobalDB.DB, global.GetSlogger())

		// 升级前备份按这一项判断换没换版本：只在初始化走完以后写
		if err := models.WriteAppMeta(global.GlobalDB.DB, models.AppMetaLastStartedVersion, version.Version); err != nil {
			global.GetSlogger().Warnf("记录这次启动的版本失败（下次启动可能会多做一次升级前备份）: %v", err)
		}
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

// logMigrationResult 记录 v1→v2 迁移结果。迁移成功时 Errors 里也可能有单个站点更新失败，
// 两种情况都逐条打印，原来只在失败分支打印，成功分支只说「配置迁移成功」。
func logMigrationResult(log *zap.SugaredLogger, result *migration.MigrationResult) {
	if result == nil {
		return
	}
	if result.Success {
		log.Infof("配置迁移完成: 迁移了 %d 个下载器, %d 个站点, %d 处出错", result.DownloadersMigrated, result.SitesMigrated, len(result.Errors))
	} else {
		log.Errorf("配置迁移失败: %s", result.Message)
	}
	for _, e := range result.Errors {
		log.Errorf("迁移错误: %s", e)
	}
}

// applyPendingRestore 换上待恢复的备份（只在 Web 服务进程里）。换不上时文件已经放回原位，按原来的数据继续启动；
// 放不回去时返回错误，停止启动。
func applyPendingRestore() (backup.ApplyResult, error) {
	if !pendingRestoreEnabled {
		return backup.ApplyResult{}, nil
	}
	data, err := DataDir()
	if err != nil {
		return backup.ApplyResult{}, err
	}
	res, err := backup.ApplyPending(data, keyFromEnv(), time.Now())
	if err != nil {
		return res, err
	}
	log := global.GetSlogger()
	switch {
	case res.Applied:
		log.Infof("已换上备份 %s 的数据；换下来的原文件在 %s", res.Name, res.PreRestoreDir)
	case res.Err != nil:
		log.Errorf("恢复备份 %s 没做成，按原来的数据继续启动: %v", res.Name, res.Err)
	}
	return res, nil
}

// preUpgradeBackup 是打开数据库时的迁移前钩子：上次正常启动的版本和这次不同、库里已经有业务表时，
// 在 AutoMigrate 与编号迁移以前先做一份备份（原因 pre-upgrade）。全新的库不备份。失败只记下来，不阻止启动。
func preUpgradeBackup(db *gorm.DB) error {
	has, err := models.TableExists(db, "torrent_infos")
	if err != nil || !has {
		return err
	}
	last, err := models.ReadAppMeta(db, models.AppMetaLastStartedVersion)
	if err != nil {
		return err
	}
	if last == version.Version {
		return nil
	}
	target, err := rawBackupTarget(db)
	at := time.Now()
	preUpgrade.ran, preUpgrade.at = true, at
	if err != nil {
		preUpgrade.err = err
		return fmt.Errorf("升级前备份没做成: %w", err)
	}
	dataVersion := last
	if dataVersion == "" {
		dataVersion = backup.UnknownAppVersion
	}
	withKey := target.includeSecret
	if withKey {
		// 这时还没做启动检查：secret.key 丢了、启动时新生成了一把时，带进去的不是解得开数据的那把，这一份只备份数据库
		if kerr := keyUsableForBackup(); kerr != nil {
			withKey = false
			global.GetSlogger().Warnf("升级前备份不带密钥：%v", kerr)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	info, err := backup.Create(ctx, db, backup.Options{
		Dir: target.dir, Reason: backup.ReasonPreUpgrade, IncludeSecret: withKey, AppVersion: dataVersion,
		Key: CurrentBackupKey, Now: func() time.Time { return at },
	})
	preUpgrade.name, preUpgrade.err = info.Name, err
	if err != nil {
		return fmt.Errorf("升级前备份没做成: %w", err)
	}
	if _, perr := backup.Prune(target.dir, target.keep); perr != nil {
		global.GetSlogger().Warnf("清理旧的升级前备份失败: %v", perr)
	}
	if last == "" {
		global.GetSlogger().Infof("第一次用 %s 打开这份数据（更早的版本不记版本号），升级以前先备份了数据：%s", version.Version, info.Path)
	} else {
		global.GetSlogger().Infof("版本从 %s 换成 %s，升级以前先备份了数据：%s", last, version.Version, info.Path)
	}
	return nil
}

// recordStartupBackups 把这次启动的升级前备份与恢复的结果写进备份设置（设置页与首页「待处理」看它）。
func recordStartupBackups(store *ConfigStore, restored backup.ApplyResult) {
	log := global.GetSlogger()
	if preUpgrade.ran {
		if err := store.RecordBackupResult(backup.ReasonPreUpgrade, preUpgrade.name, preUpgrade.err, preUpgrade.at); err != nil {
			log.Warnf("记录升级前备份的结果失败: %v", err)
		}
	}
	if restored.Name != "" {
		if err := store.RecordRestoreResult(restored.Name, restored.PreRestoreDir, restored.Err, time.Now()); err != nil {
			log.Warnf("记录恢复结果失败: %v", err)
		}
	}
}
