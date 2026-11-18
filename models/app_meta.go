package models

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AppMeta 是程序自己的键值记录（app_meta 表），现在只有「上次正常启动的程序版本」一项。
// SchemaVersion.AppVersion 只在跑编号迁移时写，代表不了上次运行的版本，升级前备份靠这一项判断换没换版本。
type AppMeta struct {
	Key       string `gorm:"primaryKey;size:64"`
	Value     string `gorm:"type:text"`
	UpdatedAt time.Time
}

func (AppMeta) TableName() string { return "app_meta" }

// AppMetaLastStartedVersion 是上次正常启动（运行时初始化走完）的程序版本。
const AppMetaLastStartedVersion = "last_started_version"

// ReadAppMeta 用原生 SQL 读一项。表还不存在（升级前备份在 AutoMigrate 之前调用）或没有这一项时返回空串。
func ReadAppMeta(db *gorm.DB, key string) (string, error) {
	ok, err := TableExists(db, "app_meta")
	if err != nil || !ok {
		return "", err
	}
	var values []string
	if err := db.Raw("SELECT value FROM app_meta WHERE key = ?", key).Scan(&values).Error; err != nil {
		return "", fmt.Errorf("读取 app_meta 失败: %w", err)
	}
	if len(values) == 0 {
		return "", nil
	}
	return values[0], nil
}

// WriteAppMeta 写一项，已有就更新。
func WriteAppMeta(db *gorm.DB, key, value string) error {
	if db == nil {
		return errors.New("数据库未初始化")
	}
	row := AppMeta{Key: key, Value: value, UpdatedAt: time.Now()}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&row).Error
}

// TableExists 用 sqlite_master 查一张表在不在（AutoMigrate 之前也能用）。
func TableExists(db *gorm.DB, table string) (bool, error) {
	var n int64
	if err := db.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&n).Error; err != nil {
		return false, fmt.Errorf("查询表 %s 失败: %w", table, err)
	}
	return n > 0, nil
}
