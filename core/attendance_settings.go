package core

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
)

// 每日签到时间窗的默认值（进程时区）。
const (
	DefaultAttendanceWindowStart = "08:00"
	DefaultAttendanceWindowEnd   = "10:00"
)

var attendanceTimePattern = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// ValidateAttendanceWindow 校验签到时间窗：HH:MM，开始早于结束，不跨午夜。
func ValidateAttendanceWindow(start, end string) error {
	if !attendanceTimePattern.MatchString(start) || !attendanceTimePattern.MatchString(end) {
		return errors.New("签到时间窗的格式应为 HH:MM，例如 08:00")
	}
	if start >= end {
		return errors.New("签到时间窗的开始时间必须早于结束时间")
	}
	return nil
}

// SetSiteAttendanceEnabled 打开或关闭站点的每日自动签到，只写这一列，不影响站点的其他配置。
func (s *ConfigStore) SetSiteAttendanceEnabled(siteName string, enabled bool) error {
	res := s.db.DB.Model(&models.SiteSetting{}).Where("name = ?", siteName).Update("attendance_enabled", enabled)
	if res.Error != nil {
		return fmt.Errorf("保存签到开关失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("站点不存在: %s", siteName)
	}
	events.Publish(events.Event{Type: events.ConfigChanged, Version: time.Now().UnixNano(), Source: "attendance", At: time.Now()})
	return nil
}

// AttendanceWindow 返回签到时间窗；还没有全局设置或存的值不合法时返回默认的 08:00–10:00。
func (s *ConfigStore) AttendanceWindow() (string, string, error) {
	var gs models.SettingsGlobal
	err := s.db.DB.Select("attendance_window_start", "attendance_window_end").First(&gs).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DefaultAttendanceWindowStart, DefaultAttendanceWindowEnd, nil
	}
	if err != nil {
		return DefaultAttendanceWindowStart, DefaultAttendanceWindowEnd, fmt.Errorf("读取签到时间窗失败: %w", err)
	}
	if ValidateAttendanceWindow(gs.AttendanceWindowStart, gs.AttendanceWindowEnd) != nil {
		return DefaultAttendanceWindowStart, DefaultAttendanceWindowEnd, nil
	}
	return gs.AttendanceWindowStart, gs.AttendanceWindowEnd, nil
}

// SaveAttendanceWindow 保存签到时间窗，只写这两列。全局设置行要先存在（首次启动时保存全局设置建行）。
func (s *ConfigStore) SaveAttendanceWindow(start, end string) error {
	if err := ValidateAttendanceWindow(start, end); err != nil {
		return err
	}
	var gs models.SettingsGlobal
	if err := s.db.DB.Select("id").First(&gs).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("全局设置尚未初始化，请先保存一次全局设置")
		}
		return fmt.Errorf("读取全局设置失败: %w", err)
	}
	if err := s.db.DB.Model(&models.SettingsGlobal{}).Where("id = ?", gs.ID).Updates(map[string]any{
		"attendance_window_start": start,
		"attendance_window_end":   end,
	}).Error; err != nil {
		return fmt.Errorf("保存签到时间窗失败: %w", err)
	}
	events.Publish(events.Event{Type: events.ConfigChanged, Version: time.Now().UnixNano(), Source: "attendance", At: time.Now()})
	return nil
}
