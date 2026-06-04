package models

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type SiteLoginState struct {
	ID                       uint       `gorm:"primaryKey" json:"id"`
	SiteName                 string     `gorm:"uniqueIndex;size:64;not null" json:"site_name"`
	LastLoginAt              *time.Time `json:"last_login_at"`
	LastAccessAt             *time.Time `json:"last_access_at"`
	LastVisitAt              *time.Time `json:"last_visit_at"`
	LastProbeAt              *time.Time `json:"last_probe_at"`
	LastProbeStatus          string     `gorm:"size:32" json:"last_probe_status"`
	LastProbeError           string     `gorm:"type:text" json:"last_probe_error"`
	ConsecutiveProbeFailures int        `json:"consecutive_probe_failures"`
	ProbeJitterSeconds       int        `json:"probe_jitter_seconds"`
	BanThresholdDays         int        `gorm:"default:30" json:"ban_threshold_days"`
	RemindBeforeDays         int        `gorm:"default:10" json:"remind_before_days"`
	ReminderCron             string     `gorm:"size:64;default:'0 10,22 * * *'" json:"reminder_cron"`
	NotificationChannelIDs   string     `gorm:"type:text" json:"notification_channel_ids"`
	LastReminderTier         string     `gorm:"size:16;default:'none'" json:"last_reminder_tier"`
	LastReminderSentAt       *time.Time `json:"last_reminder_sent_at"`
	ApiLastLoginAt           *time.Time `json:"api_last_login_at,omitempty"`
	CookieLastLoginAt        *time.Time `json:"cookie_last_login_at,omitempty"`
	ProbeMode                string     `gorm:"size:16;default:'auto'" json:"probe_mode"`
	LastConsistencyCheck     string     `gorm:"size:32;default:''" json:"last_consistency_check"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
}

type SiteLoginStateRepository struct {
	db *gorm.DB
}

func NewSiteLoginStateRepository(db *gorm.DB) *SiteLoginStateRepository {
	return &SiteLoginStateRepository{db: db}
}

func (r *SiteLoginStateRepository) UpsertLoginState(siteName string, fields map[string]any) error {
	if siteName == "" {
		return errors.New("站点名称不能为空")
	}

	state := SiteLoginState{SiteName: siteName}
	if err := r.db.Where("site_name = ?", siteName).First(&state).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("查询站点登录状态失败: %w", err)
		}
		state = SiteLoginState{
			SiteName:         siteName,
			BanThresholdDays: 30,
			RemindBeforeDays: 10,
			ReminderCron:     "0 10,22 * * *",
			LastReminderTier: "none",
		}
	}

	for key, val := range fields {
		switch key {
		case "BanThresholdDays":
			if v, ok := val.(int); ok {
				state.BanThresholdDays = v
			}
		case "RemindBeforeDays":
			if v, ok := val.(int); ok {
				state.RemindBeforeDays = v
			}
		case "ReminderCron":
			if v, ok := val.(string); ok {
				state.ReminderCron = v
			}
		case "NotificationChannelIDs":
			if v, ok := val.(string); ok {
				state.NotificationChannelIDs = v
			}
		case "LastProbeStatus":
			if v, ok := val.(string); ok {
				state.LastProbeStatus = v
			}
		case "ProbeJitterSeconds":
			if v, ok := val.(int); ok {
				state.ProbeJitterSeconds = v
			}
		}
	}

	if err := r.db.Save(&state).Error; err != nil {
		return fmt.Errorf("保存站点登录状态失败: %w", err)
	}
	return nil
}

func (r *SiteLoginStateRepository) GetLoginState(siteName string) (*SiteLoginState, error) {
	if siteName == "" {
		return nil, errors.New("站点名称不能为空")
	}

	var state SiteLoginState
	if err := r.db.Where("site_name = ?", siteName).First(&state).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("站点 %s 登录状态不存在", siteName)
		}
		return nil, fmt.Errorf("查询站点登录状态失败: %w", err)
	}
	return &state, nil
}

func (r *SiteLoginStateRepository) ListLoginStates(enabledOnly bool) ([]SiteLoginState, error) {
	var states []SiteLoginState
	query := r.db
	if enabledOnly {
		query = query.Where("last_probe_status = ?", "OK")
	}
	if err := query.Find(&states).Error; err != nil {
		return nil, fmt.Errorf("查询站点登录状态列表失败: %w", err)
	}
	return states, nil
}

func (r *SiteLoginStateRepository) UpdateProbeResult(siteName, status string, lastLogin, lastAccess *time.Time, probeErr error) error {
	if siteName == "" {
		return errors.New("站点名称不能为空")
	}

	updates := map[string]any{
		"last_probe_status": status,
		"last_probe_at":     time.Now(),
	}

	if lastLogin != nil {
		updates["last_login_at"] = lastLogin
	}
	if lastAccess != nil {
		updates["last_access_at"] = lastAccess
	}

	errMsg := ""
	if probeErr != nil {
		errMsg = probeErr.Error()
	}
	updates["last_probe_error"] = errMsg

	if err := r.db.Model(&SiteLoginState{}).Where("site_name = ?", siteName).Updates(updates).Error; err != nil {
		return fmt.Errorf("更新探测结果失败: %w", err)
	}
	return nil
}

func (r *SiteLoginStateRepository) ClampLastVisit(siteName string, ts, now time.Time) error {
	if siteName == "" {
		return errors.New("站点名称不能为空")
	}

	effective := ts
	if ts.After(now) {
		effective = now
	}

	if err := r.db.Model(&SiteLoginState{}).Where("site_name = ?", siteName).Update("last_visit_at", effective).Error; err != nil {
		return fmt.Errorf("更新最后访问时间失败: %w", err)
	}
	return nil
}

func (r *SiteLoginStateRepository) IncrProbeFailures(siteName string) error {
	if siteName == "" {
		return errors.New("站点名称不能为空")
	}

	if err := r.db.Model(&SiteLoginState{}).Where("site_name = ?", siteName).Update("consecutive_probe_failures", gorm.Expr("consecutive_probe_failures + 1")).Error; err != nil {
		return fmt.Errorf("增加探测失败计数失败: %w", err)
	}
	return nil
}

func (r *SiteLoginStateRepository) ResetProbeFailures(siteName string) error {
	if siteName == "" {
		return errors.New("站点名称不能为空")
	}

	if err := r.db.Model(&SiteLoginState{}).Where("site_name = ?", siteName).Update("consecutive_probe_failures", 0).Error; err != nil {
		return fmt.Errorf("重置探测失败计数失败: %w", err)
	}
	return nil
}
