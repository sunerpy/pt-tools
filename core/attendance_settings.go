package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
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

// maxVisionModelLen 与 AI 助手设置里模型名的上限一致。
const maxVisionModelLen = 128

// ValidateVisionModel 校验识别验证码用的模型名：可以为空（用 AI 助手的模型），不能有空白，不超过 128 字。
func ValidateVisionModel(model string) error {
	if len(model) > maxVisionModelLen {
		return fmt.Errorf("模型名不能超过 %d 个字符", maxVisionModelLen)
	}
	if strings.ContainsAny(model, " \t\r\n") {
		return errors.New("模型名里不能有空白")
	}
	return nil
}

// AttendanceAI 返回 AI 签到的开关和识别验证码用的模型；还没有全局设置时是关闭。
func (s *ConfigStore) AttendanceAI() (bool, string, error) {
	var gs models.SettingsGlobal
	err := s.db.DB.Select("attendance_ai_enabled", "attendance_vision_model").First(&gs).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("读取 AI 签到设置失败: %w", err)
	}
	return gs.AttendanceAIEnabled, gs.AttendanceVisionModel, nil
}

// SaveAttendanceAI 保存 AI 签到的开关与识别验证码用的模型，只写这两列。
func (s *ConfigStore) SaveAttendanceAI(enabled bool, visionModel string) error {
	visionModel = strings.TrimSpace(visionModel)
	if err := ValidateVisionModel(visionModel); err != nil {
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
		"attendance_ai_enabled":   enabled,
		"attendance_vision_model": visionModel,
	}).Error; err != nil {
		return fmt.Errorf("保存 AI 签到设置失败: %w", err)
	}
	events.Publish(events.Event{Type: events.ConfigChanged, Version: time.Now().UnixNano(), Source: "attendance", At: time.Now()})
	return nil
}

// AttendanceNotify 返回签到结果推送：开没开、推到哪些通道（空表示所有启用的通道）。还没有全局设置时是开、所有通道。
func (s *ConfigStore) AttendanceNotify() (bool, []uint, error) {
	var gs models.SettingsGlobal
	err := s.db.DB.Select("attendance_notify_enabled", "attendance_notify_channel_ids").First(&gs).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, []uint{}, nil
	}
	if err != nil {
		return true, []uint{}, fmt.Errorf("读取签到推送设置失败: %w", err)
	}
	return gs.AttendanceNotifyEnabled, parseChannelIDs(gs.AttendanceNotifyChannelIDs), nil
}

// SaveAttendanceNotify 保存签到结果推送，只写这两列。选了通道时通道都要存在；不选表示所有启用的通道。
func (s *ConfigStore) SaveAttendanceNotify(enabled bool, channelIDs []uint) error {
	ids := dedupUint(channelIDs)
	if ids == nil {
		ids = []uint{}
	}
	if len(ids) > 0 {
		var n int64
		if err := s.db.DB.Model(&models.NotificationConf{}).Where("id IN ?", ids).Count(&n).Error; err != nil {
			return fmt.Errorf("检查通知通道失败: %w", err)
		}
		if int(n) != len(ids) {
			return errors.New("选中的通知通道有的已经不存在，请重新选择")
		}
	}
	raw, err := json.Marshal(ids)
	if err != nil {
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
		"attendance_notify_enabled":     enabled,
		"attendance_notify_channel_ids": string(raw),
	}).Error; err != nil {
		return fmt.Errorf("保存签到推送设置失败: %w", err)
	}
	events.Publish(events.Event{Type: events.ConfigChanged, Version: time.Now().UnixNano(), Source: "attendance", At: time.Now()})
	return nil
}
