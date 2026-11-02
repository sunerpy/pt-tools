package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/backup"
	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/version"
)

// 备份与恢复（媒体中心 M0）在 core 里的部分：设置、做备份、列出与删除、放待恢复；
// 启动时的升级前备份与换上待恢复的备份在 init.go。设置存在 backup_settings 那一行，没有这一行时按默认。

// BackupSettings 是备份设置与最近一次备份、恢复的结果。
type BackupSettings struct {
	Enabled bool
	// Time 是每天定时备份的时刻（HH:MM）
	Time string
	// Keep 是定时备份保留几份
	Keep int
	// Dir 是用户填的备份目录，空串表示默认目录
	Dir string
	// DefaultDir 是默认目录（<数据目录>/backups）
	DefaultDir    string
	IncludeSecret bool
	// ScheduledDay 是最近一次做过定时备份（不论成败）的日期
	ScheduledDay string
	// Last 是最近一次备份的结果，没做过时为 nil
	Last *BackupResult
	// Restore 是最近一次启动时恢复的结果，没恢复过时为 nil
	Restore *RestoreResult
}

// BackupResult 是一次备份的结果：Error 为空表示成功。
type BackupResult struct {
	At     time.Time
	Reason backup.Reason
	Name   string
	Error  string
	// LastOKAt 是最近一次成功备份的时间（不一定是这一次）
	LastOKAt *time.Time
}

// RestoreResult 是一次启动时恢复的结果：Error 为空表示换上了，换下来的原文件在 Dir。
type RestoreResult struct {
	At    time.Time
	Name  string
	Dir   string
	Error string
}

// BackupSettingsInput 是保存备份设置的参数。
type BackupSettingsInput struct {
	Enabled       bool
	Time          string
	Keep          int
	Dir           string
	IncludeSecret bool
}

// ErrBackupSettingsInvalid 是备份设置不对；错误文本里有给人看的原因。
var ErrBackupSettingsInvalid = errors.New("备份设置有误")

// DataDir 是 pt-tools 的数据目录（~/.pt-tools）：数据库、secret.key、默认的备份目录与待恢复都在这里。
func DataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取主目录失败: %w", err)
	}
	return filepath.Join(home, models.WorkDir), nil
}

func defaultBackupDir() (string, error) {
	data, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(data, backup.DefaultDirName), nil
}

func defaultBackupRow() models.BackupSetting {
	return models.BackupSetting{ID: 1, Enabled: true, Time: backup.DefaultTime, Keep: backup.DefaultKeep, IncludeSecret: true}
}

// BackupSettings 读备份设置；没有这一行时返回默认值（开着、04:30、7 份、默认目录、带上密钥）。
func (s *ConfigStore) BackupSettings() (BackupSettings, error) {
	def, err := defaultBackupDir()
	if err != nil {
		return BackupSettings{}, err
	}
	row := defaultBackupRow()
	var rows []models.BackupSetting
	if err := s.db.DB.Where("id = 1").Limit(1).Find(&rows).Error; err != nil {
		return BackupSettings{}, fmt.Errorf("读取备份设置失败: %w", err)
	}
	if len(rows) > 0 {
		row = rows[0]
	}
	out := BackupSettings{
		Enabled: row.Enabled, Time: row.Time, Keep: row.Keep, Dir: row.Dir, DefaultDir: def,
		IncludeSecret: row.IncludeSecret, ScheduledDay: row.ScheduledDay,
	}
	if out.Time == "" {
		out.Time = backup.DefaultTime
	}
	if out.Keep < backup.MinKeep || out.Keep > backup.MaxKeep {
		out.Keep = backup.DefaultKeep
	}
	if row.LastAt != nil {
		out.Last = &BackupResult{At: *row.LastAt, Reason: backup.Reason(row.LastReason), Name: row.LastName, Error: row.LastError, LastOKAt: row.LastOKAt}
	}
	if row.RestoreAt != nil {
		out.Restore = &RestoreResult{At: *row.RestoreAt, Name: row.RestoreName, Dir: row.RestoreDir, Error: row.RestoreError}
	}
	return out, nil
}

// BackupDir 是现在生效的备份目录。
func (s BackupSettings) BackupDir() string {
	if strings.TrimSpace(s.Dir) != "" {
		return s.Dir
	}
	return s.DefaultDir
}

// SaveBackupSettings 校验并保存备份设置，只改设置这几列，不动最近一次备份与恢复的结果。
// 目录要是绝对路径；不存在时建出来，并试着写一个文件确认能写。
func (s *ConfigStore) SaveBackupSettings(in BackupSettingsInput) (BackupSettings, error) {
	t := strings.TrimSpace(in.Time)
	if _, err := time.Parse("15:04", t); err != nil || len(t) != 5 {
		return BackupSettings{}, fmt.Errorf("%w：时刻要写成 HH:MM，例如 04:30", ErrBackupSettingsInvalid)
	}
	if in.Keep < backup.MinKeep || in.Keep > backup.MaxKeep {
		return BackupSettings{}, fmt.Errorf("%w：保留份数要在 %d 到 %d 之间", ErrBackupSettingsInvalid, backup.MinKeep, backup.MaxKeep)
	}
	dir := strings.TrimSpace(in.Dir)
	if dir != "" {
		if !filepath.IsAbs(dir) {
			return BackupSettings{}, fmt.Errorf("%w：备份目录要写绝对路径", ErrBackupSettingsInvalid)
		}
		dir = filepath.Clean(dir)
		if err := checkWritableDir(dir); err != nil {
			return BackupSettings{}, fmt.Errorf("%w：%v", ErrBackupSettingsInvalid, err)
		}
	}
	if err := s.ensureBackupRow(); err != nil {
		return BackupSettings{}, err
	}
	if err := s.db.DB.Model(&models.BackupSetting{}).Where("id = 1").Updates(map[string]any{
		"enabled": in.Enabled, "time": t, "keep": in.Keep, "dir": dir, "include_secret": in.IncludeSecret, "updated_at": time.Now(),
	}).Error; err != nil {
		return BackupSettings{}, fmt.Errorf("保存备份设置失败: %w", err)
	}
	return s.BackupSettings()
}

func checkWritableDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("建不了备份目录 %s：%v", dir, err)
	}
	f, err := os.CreateTemp(dir, ".pt-tools-write-test-*")
	if err != nil {
		return fmt.Errorf("备份目录 %s 不能写：%v", dir, err)
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name)
}

// ensureBackupRow 没有设置这一行时按默认值建出来（已有就不动），之后都用按列更新。
func (s *ConfigStore) ensureBackupRow() error {
	row := defaultBackupRow()
	row.UpdatedAt = time.Now()
	if err := s.db.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return fmt.Errorf("保存备份设置失败: %w", err)
	}
	return nil
}

// RecordBackupResult 记下一次备份的结果（失败时 err 非空）。定时备份同时记下做过的日期，同一天不再做。
func (s *ConfigStore) RecordBackupResult(reason backup.Reason, name string, runErr error, at time.Time) error {
	if err := s.ensureBackupRow(); err != nil {
		return err
	}
	fields := map[string]any{"last_at": at, "last_reason": string(reason), "last_name": name, "last_error": ""}
	if runErr != nil {
		fields["last_error"] = runErr.Error()
	} else {
		fields["last_ok_at"] = at
	}
	if reason == backup.ReasonScheduled {
		fields["scheduled_day"] = at.Local().Format("2006-01-02")
	}
	if err := s.db.DB.Model(&models.BackupSetting{}).Where("id = 1").Updates(fields).Error; err != nil {
		return fmt.Errorf("记录备份结果失败: %w", err)
	}
	return nil
}

// RecordRestoreResult 记下一次启动时恢复的结果：换上了时 dir 是换下来的原文件放在哪，没换上时 dir 不记。
func (s *ConfigStore) RecordRestoreResult(name, dir string, restoreErr error, at time.Time) error {
	if err := s.ensureBackupRow(); err != nil {
		return err
	}
	msg := ""
	if restoreErr != nil {
		msg, dir = restoreErr.Error(), ""
	}
	if err := s.db.DB.Model(&models.BackupSetting{}).Where("id = 1").Updates(map[string]any{
		"restore_at": at, "restore_name": name, "restore_dir": dir, "restore_error": msg,
	}).Error; err != nil {
		return fmt.Errorf("记录恢复结果失败: %w", err)
	}
	return nil
}

// RunBackup 按现在的设置做一份备份（原因是 reason），做完按保留规则删旧备份，并记下结果。
func (s *ConfigStore) RunBackup(ctx context.Context, reason backup.Reason) (backup.Info, error) {
	cfg, err := s.BackupSettings()
	if err != nil {
		return backup.Info{}, err
	}
	at := time.Now()
	dir := cfg.BackupDir()
	info, runErr := backup.Create(ctx, s.db.DB, backup.Options{
		Dir: dir, Reason: reason, IncludeSecret: cfg.IncludeSecret, AppVersion: version.Version,
		Key: CurrentBackupKey, Now: func() time.Time { return at },
	})
	if recErr := s.RecordBackupResult(reason, info.Name, runErr, at); recErr != nil && runErr == nil {
		runErr = recErr
	}
	if runErr != nil {
		return info, runErr
	}
	// 删旧备份出错不算这次备份失败（新的已经保存好了），只记日志
	if _, err := backup.Prune(dir, cfg.Keep); err != nil {
		if log := global.GetSloggerSafe(); log != nil {
			log.Warnf("备份已保存，清理旧备份时出错: %v", err)
		}
	}
	return info, nil
}

// ListBackups 列出备份目录里的备份（最新的在前）与这个目录。
func (s *ConfigStore) ListBackups() ([]backup.Info, string, error) {
	cfg, err := s.BackupSettings()
	if err != nil {
		return nil, "", err
	}
	dir := cfg.BackupDir()
	list, err := backup.List(dir)
	return list, dir, err
}

// BackupFile 是备份目录里一份备份的路径（下载用）；只认这里起的文件名。
func (s *ConfigStore) BackupFile(name string) (string, error) {
	cfg, err := s.BackupSettings()
	if err != nil {
		return "", err
	}
	return backup.Resolve(cfg.BackupDir(), name)
}

// DeleteBackup 删掉备份目录里的一份备份。
func (s *ConfigStore) DeleteBackup(name string) error {
	cfg, err := s.BackupSettings()
	if err != nil {
		return err
	}
	return backup.Delete(cfg.BackupDir(), name)
}

// StageRestore 校验 src 这份备份（name 是给人看的名字），通过后放进待恢复，下次启动时生效。
func StageRestore(src, name string) (backup.Pending, error) {
	data, err := DataDir()
	if err != nil {
		return backup.Pending{}, err
	}
	if err := os.MkdirAll(data, 0o700); err != nil {
		return backup.Pending{}, fmt.Errorf("创建数据目录失败: %w", err)
	}
	return backup.Stage(src, name, data, ActiveBackupKey(), time.Now())
}

// StageRestoreBackup 把备份目录里的一份备份放进待恢复。
func (s *ConfigStore) StageRestoreBackup(name string) (backup.Pending, error) {
	p, err := s.BackupFile(name)
	if err != nil {
		return backup.Pending{}, err
	}
	return StageRestore(p, name)
}

// PendingRestore 读已经放好、下次启动生效的恢复；没有时返回 nil。
func PendingRestore() (*backup.Pending, error) {
	data, err := DataDir()
	if err != nil {
		return nil, err
	}
	return backup.ReadPending(data)
}

// CancelRestore 取消还没生效的恢复。
func CancelRestore() error {
	data, err := DataDir()
	if err != nil {
		return err
	}
	return backup.CancelPending(data)
}

// keyFromEnv 报告密钥是不是来自环境变量 PT_TOOLS_SECRET_KEY（和 internal/crypto 读密钥的判断一样）。
func keyFromEnv() bool { return os.Getenv("PT_TOOLS_SECRET_KEY") != "" }

// ActiveBackupKey 是校验恢复时「当前生效的密钥」：环境变量 PT_TOOLS_SECRET_KEY 设了就是它，否则是现在的 secret.key。
func ActiveBackupKey() backup.ActiveKey {
	key, err := crypto.ExportKey()
	if err != nil {
		key = nil
	}
	return backup.ActiveKey{FromEnv: keyFromEnv(), Key: key}
}

// CurrentBackupKey 是带进备份的密钥与它的来源。
func CurrentBackupKey() (backup.Key, error) {
	key, err := crypto.ExportKey()
	if err != nil {
		return backup.Key{}, err
	}
	src := backup.SecretFromFile
	if keyFromEnv() {
		src = backup.SecretFromEnv
	}
	return backup.Key{Bytes: key, Source: src}, nil
}

// keyUsableForBackup 报告这个进程里的密钥能不能带进备份：secret.key 读不出来，或者这个进程启动时它不存在、
// 刚生成了一把新的（原来的密钥丢了或者没挂载进来）时，带进去的不是解得开数据的那把。
// 在运行中的服务里不用它：服务启动时新生成的密钥通过了启动检查（库里还没有密文），之后就是这个安装的密钥。
func keyUsableForBackup() error {
	path, generated, loadErr := crypto.KeyStatus()
	switch {
	case loadErr != nil:
		return fmt.Errorf("%s 读不出来：%v", path, loadErr)
	case generated:
		return fmt.Errorf("%s 原本不存在，现在的密钥是这次新生成的", path)
	}
	return nil
}

// BackupDataDir 是命令行 pt-tools db backup：不走 InitRuntime（服务在运行时也能用，不迁移数据库），
// 只读打开数据目录里的数据库做一份备份（原因 cli）。output 为空时放进备份设置里的目录；
// includeSecret 为 nil 时按备份设置决定带不带密钥。
func BackupDataDir(ctx context.Context, output string, includeSecret *bool) (backup.Info, error) {
	data, err := DataDir()
	if err != nil {
		return backup.Info{}, err
	}
	dbPath := filepath.Join(data, models.DBFile)
	if _, err = os.Stat(dbPath); err != nil {
		return backup.Info{}, fmt.Errorf("没有找到数据库 %s: %w", dbPath, err)
	}
	db, err := openDataDirReadOnly(dbPath)
	if err != nil {
		return backup.Info{}, err
	}
	defer func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	}()
	target, err := rawBackupTarget(db)
	if err != nil {
		return backup.Info{}, err
	}
	withKey := target.includeSecret
	if includeSecret != nil {
		withKey = *includeSecret
	}
	if withKey {
		if err = keyUsableForBackup(); err != nil {
			return backup.Info{}, fmt.Errorf("带不上密钥：%v。把原来的 secret.key 放回去再备份，或者加 --no-secret 只备份数据库", err)
		}
	}
	appVersion, err := models.ReadAppMeta(db, models.AppMetaLastStartedVersion)
	if err != nil || appVersion == "" {
		appVersion = version.Version
	}
	return backup.Create(ctx, db, backup.Options{
		Dir: target.dir, Output: output, Reason: backup.ReasonCLI, IncludeSecret: withKey, AppVersion: appVersion, Key: CurrentBackupKey,
	})
}

// openDataDirReadOnly 只读打开（可能正被服务使用的）数据库：不建表、不迁移，等锁最多 30 秒。
func openDataDirReadOnly(path string) (*gorm.DB, error) {
	dsn := "file:" + filepath.ToSlash(path) + "?mode=ro&_pragma=busy_timeout(30000)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	return db, nil
}

// backupTarget 是升级前备份用的几项设置。
type backupTarget struct {
	dir           string
	includeSecret bool
	keep          int
}

// rawBackupTarget 在 AutoMigrate 以前（升级前备份）读备份目录、带不带密钥与保留份数：表或这一行不存在时按默认。
func rawBackupTarget(db *gorm.DB) (backupTarget, error) {
	dir, err := defaultBackupDir()
	if err != nil {
		return backupTarget{}, err
	}
	out := backupTarget{dir: dir, includeSecret: true, keep: backup.DefaultKeep}
	ok, err := models.TableExists(db, "backup_settings")
	if err != nil || !ok {
		return out, err
	}
	var rows []struct {
		Dir           string
		IncludeSecret bool
		Keep          int
	}
	if err := db.Raw("SELECT dir, include_secret, keep FROM backup_settings WHERE id = 1").Scan(&rows).Error; err != nil {
		return backupTarget{}, fmt.Errorf("读取备份设置失败: %w", err)
	}
	if len(rows) > 0 {
		if strings.TrimSpace(rows[0].Dir) != "" {
			out.dir = rows[0].Dir
		}
		out.includeSecret = rows[0].IncludeSecret
		if rows[0].Keep >= backup.MinKeep && rows[0].Keep <= backup.MaxKeep {
			out.keep = rows[0].Keep
		}
	}
	return out, nil
}
