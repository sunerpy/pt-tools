package models

import (
	"errors"
	"fmt"
	"time"
)

// 一次签到尝试是怎么触发的。
const (
	AttendanceTriggerAuto   = "auto"
	AttendanceTriggerManual = "manual"
)

// 一次签到尝试的结果。retry 是这次没成、之后还会再试；failed 是当天不再试（或者手动签到失败）。
const (
	AttendanceResultSigned      = "signed"
	AttendanceResultAlready     = "already"
	AttendanceResultRetry       = "retry"
	AttendanceResultFailed      = "failed"
	AttendanceResultUnsupported = "unsupported"
)

// 签到用的方式：直接签到，或者借助 AI 识别验证码、答题。
const (
	AttendanceMethodDirect    = "direct"
	AttendanceMethodAICaptcha = "ai_captcha"
	AttendanceMethodAIQuiz    = "ai_quiz"
)

// AttendanceEventRetention 是签到事件留多久，更早的每天清一次。
const AttendanceEventRetention = 90 * 24 * time.Hour

// SiteAttendanceEvent 是一次签到尝试（定时或手动各一行），给「签到详情」看当天每个站点怎么签的：
// 结果、直接签还是借助 AI、AI 调了几次、用掉多少 token。Day 与 SiteAttendanceLog 的同一天对应；时间一律是 UTC。
type SiteAttendanceEvent struct {
	ID       uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	SiteName string `gorm:"size:64;not null;index:idx_attendance_event_day,priority:2" json:"site_name"`
	Day      string `gorm:"size:10;not null;index:idx_attendance_event_day,priority:1" json:"day"`
	Trigger  string `gorm:"size:16;not null;default:'auto'" json:"trigger"`
	Result   string `gorm:"size:16;not null;default:''" json:"result"`
	Method   string `gorm:"size:16;not null;default:'direct'" json:"method"`
	// AIModel 是这次签到调的模型（没调 AI 时为空）
	AIModel string `gorm:"size:128;not null;default:''" json:"ai_model,omitempty"`
	// AICalls 是模型回了几次（认错验证码换一张再认时不止一次）
	AICalls          int       `gorm:"not null;default:0" json:"ai_calls"`
	PromptTokens     int64     `gorm:"not null;default:0" json:"prompt_tokens"`
	CompletionTokens int64     `gorm:"not null;default:0" json:"completion_tokens"`
	DurationMs       int64     `gorm:"not null;default:0" json:"duration_ms"`
	Message          string    `gorm:"size:512;not null;default:''" json:"message,omitempty"`
	Error            string    `gorm:"size:1024;not null;default:''" json:"error,omitempty"`
	StartedAt        time.Time `gorm:"index" json:"started_at"`
	FinishedAt       time.Time `json:"finished_at"`
}

// TableName 指定表名。
func (SiteAttendanceEvent) TableName() string { return "site_attendance_events" }

// AddEvent 记一次签到尝试。
func (r *SiteAttendanceRepository) AddEvent(ev SiteAttendanceEvent) error {
	if ev.SiteName == "" || ev.Day == "" {
		return errors.New("站点名称和日期不能为空")
	}
	ev.ID = 0
	if err := r.db.Create(&ev).Error; err != nil {
		return fmt.Errorf("记录签到事件失败: %w", err)
	}
	return nil
}

// ListEvents 返回某天所有站点的签到尝试，按站点名、开始时间排序。
func (r *SiteAttendanceRepository) ListEvents(day string) ([]SiteAttendanceEvent, error) {
	var rows []SiteAttendanceEvent
	if err := r.db.Where("day = ?", day).Order("site_name, started_at, id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("加载签到事件失败: %w", err)
	}
	return rows, nil
}

// PurgeEventsBefore 删掉 before 以前开始的签到事件，返回删了几行。按时刻比较（julianday），不按文本。
func (r *SiteAttendanceRepository) PurgeEventsBefore(before time.Time) (int64, error) {
	res := r.db.Where("julianday(started_at) < julianday(?)", before.UTC()).Delete(&SiteAttendanceEvent{})
	if res.Error != nil {
		return 0, fmt.Errorf("清理签到事件失败: %w", res.Error)
	}
	return res.RowsAffected, nil
}
