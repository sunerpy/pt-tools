package models

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 每日签到的状态。pending 之外都是当天的最终结果，不再请求站点。
const (
	AttendancePending     = "pending"
	AttendanceSigned      = "signed"
	AttendanceAlready     = "already"
	AttendanceFailed      = "failed"
	AttendanceUnsupported = "unsupported"
)

// SiteAttendanceLog 记录每个站点每天的签到，(site_name, day) 唯一，一天一行。
// Day 是进程时区（Docker 的 TZ）的日期 YYYY-MM-DD；时间列一律以 UTC 写入。
type SiteAttendanceLog struct {
	ID            uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	SiteName      string     `gorm:"size:64;not null;uniqueIndex:idx_site_attendance_day,priority:1" json:"site_name"`
	Day           string     `gorm:"size:10;not null;uniqueIndex:idx_site_attendance_day,priority:2" json:"day"`
	Status        string     `gorm:"size:16;not null;default:'pending'" json:"status"`
	Attempts      int        `gorm:"not null;default:0" json:"attempts"`
	ScheduledAt   time.Time  `json:"scheduled_at"`
	NextAttemptAt *time.Time `json:"next_attempt_at,omitempty"`
	LastAttemptAt *time.Time `json:"last_attempt_at,omitempty"`
	Message       string     `gorm:"size:512;default:''" json:"message,omitempty"`
	LastError     string     `gorm:"size:1024;default:''" json:"last_error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// Done 报告当天的签到是否已有最终结果。
func (l SiteAttendanceLog) Done() bool {
	switch l.Status {
	case AttendanceSigned, AttendanceAlready, AttendanceFailed, AttendanceUnsupported:
		return true
	default:
		return false
	}
}

type SiteAttendanceRepository struct {
	db *gorm.DB
}

func NewSiteAttendanceRepository(db *gorm.DB) *SiteAttendanceRepository {
	return &SiteAttendanceRepository{db: db}
}

// EnsureDay 在当天还没有记录时按 row 建行；已有行时什么也不改，所以一天的计划时间只定一次。
func (r *SiteAttendanceRepository) EnsureDay(row SiteAttendanceLog) error {
	if row.SiteName == "" || row.Day == "" {
		return errors.New("站点名称和日期不能为空")
	}
	row.ID = 0
	if row.Status == "" {
		row.Status = AttendancePending
	}
	if err := r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "site_name"}, {Name: "day"}},
		DoNothing: true,
	}).Create(&row).Error; err != nil {
		return fmt.Errorf("初始化签到记录失败: %w", err)
	}
	return nil
}

// GetDay 返回某站某天的签到记录，没有时返回 gorm.ErrRecordNotFound。
func (r *SiteAttendanceRepository) GetDay(siteName, day string) (*SiteAttendanceLog, error) {
	var row SiteAttendanceLog
	if err := r.db.Where("site_name = ? AND day = ?", siteName, day).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// UpdateDay 只写给出的列。
func (r *SiteAttendanceRepository) UpdateDay(siteName, day string, columns map[string]any) error {
	if len(columns) == 0 {
		return nil
	}
	if err := r.db.Model(&SiteAttendanceLog{}).Where("site_name = ? AND day = ?", siteName, day).Updates(columns).Error; err != nil {
		return fmt.Errorf("更新签到记录失败: %w", err)
	}
	return nil
}

// ListDay 返回某天所有站点的签到记录，按站点名排序。
func (r *SiteAttendanceRepository) ListDay(day string) ([]SiteAttendanceLog, error) {
	var rows []SiteAttendanceLog
	if err := r.db.Where("day = ?", day).Order("site_name").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
