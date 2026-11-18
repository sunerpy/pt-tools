package models

import "time"

// BackupSetting 是全局设置里的「备份」与最近一次备份、恢复的结果。只有一行（id=1），没有这一行时按默认
// （开着、每天 04:30、保留 7 份、默认目录、带上密钥）。读写只经 core.ConfigStore。
//
// 布尔列故意不带 gorm 的 default：gorm 建行时会跳过零值字段、改用数据库默认值，用户关掉的开关会被存成开着。
type BackupSetting struct {
	ID      uint `gorm:"primaryKey"`
	Enabled bool `gorm:"not null"`
	// Time 是每天定时备份的时刻（HH:MM，本地时间）
	Time string `gorm:"size:5;not null"`
	// Keep 是定时备份保留几份（1–60）
	Keep int `gorm:"not null"`
	// Dir 是备份目录，空串表示默认的 <数据目录>/backups
	Dir           string `gorm:"type:text"`
	IncludeSecret bool   `gorm:"not null"`
	// ScheduledDay 是最近一次做过定时备份（不论成败）的日期（YYYY-MM-DD），同一天只做一次
	ScheduledDay string `gorm:"size:10"`

	// 最近一次备份（任何原因）的结果：LastError 为空表示成功
	LastAt     *time.Time
	LastReason string `gorm:"size:16"`
	LastName   string `gorm:"type:text"`
	LastError  string `gorm:"type:text"`
	// LastOKAt 是最近一次成功备份的时间（首页「备份几天没成功」用）
	LastOKAt *time.Time

	// 最近一次启动时恢复的结果：RestoreError 为空表示换上了，换下来的原文件在 RestoreDir
	RestoreAt    *time.Time
	RestoreName  string `gorm:"type:text"`
	RestoreDir   string `gorm:"type:text"`
	RestoreError string `gorm:"type:text"`

	UpdatedAt time.Time
}

func (BackupSetting) TableName() string { return "backup_settings" }
