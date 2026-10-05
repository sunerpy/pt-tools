package models

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

	// 以下列由各自唯一的写者维护（见 scheduler/login_reminder_monitor.go 顶部的写者表）。
	// NextProbeAt 是下一次定时探测的时间；LastProbeStartedAt 是最近一次探测开始（取锁并重读配置之前）的时间，
	// 与 ProbeRequestedAt 一起判断「凭证更新后是否还欠一次探测」。
	NextProbeAt        *time.Time `json:"next_probe_at,omitempty"`
	LastProbeStartedAt *time.Time `json:"last_probe_started_at,omitempty"`
	ProbeRequestedAt   *time.Time `json:"probe_requested_at,omitempty"`
	// FirstFailureAt 是当前失败连续段的开始时间，探测成功时清空；LastSuccessAt 是最近一次成功探测的时间。
	FirstFailureAt *time.Time `json:"first_failure_at,omitempty"`
	LastSuccessAt  *time.Time `json:"last_success_at,omitempty"`
	// LastFailureNotifiedAt 是当前失败连续段最近一次发出失败提醒的时间，恢复提醒发出后清空。
	LastFailureNotifiedAt *time.Time `json:"last_failure_notified_at,omitempty"`
	// AccessStaleSince：探测成功但站点返回的 last_access 超过 48 小时没有前进时写入，前进后清空。
	AccessStaleSince *time.Time `json:"access_stale_since,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// 配置接口可写的列，键是 UpsertLoginState 接受的字段名。
var loginStateFieldColumns = map[string]string{
	"BanThresholdDays":       "ban_threshold_days",
	"RemindBeforeDays":       "remind_before_days",
	"ReminderCron":           "reminder_cron",
	"NotificationChannelIDs": "notification_channel_ids",
	"LastProbeStatus":        "last_probe_status",
	"ProbeJitterSeconds":     "probe_jitter_seconds",
	"ProbeMode":              "probe_mode",
}

// DefaultSiteLoginState 返回新站点登录状态行的默认值。
func DefaultSiteLoginState(siteName string) SiteLoginState {
	return SiteLoginState{
		SiteName:         siteName,
		BanThresholdDays: 30,
		RemindBeforeDays: 10,
		ReminderCron:     "0 10,22 * * *",
		LastReminderTier: "none",
		ProbeMode:        "auto",
	}
}

type SiteLoginStateRepository struct {
	db *gorm.DB
}

func NewSiteLoginStateRepository(db *gorm.DB) *SiteLoginStateRepository {
	return &SiteLoginStateRepository{db: db}
}

// EnsureLoginStateRow 在缺行时按 defaults 建行，已有行时什么也不改。
// 用 ON CONFLICT DO NOTHING，两个写者同时建行也不会报唯一约束冲突。
func (r *SiteLoginStateRepository) EnsureLoginStateRow(defaults SiteLoginState) error {
	if defaults.SiteName == "" {
		return errors.New("站点名称不能为空")
	}
	row := defaults
	row.ID = 0
	if err := r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "site_name"}},
		DoNothing: true,
	}).Create(&row).Error; err != nil {
		return fmt.Errorf("初始化站点登录状态失败: %w", err)
	}
	return nil
}

// UpsertLoginState 只更新 fields 里给出的列（键为字段名，见 loginStateFieldColumns），缺行时先按默认值建行。
// 不整行写回，所以不会覆盖探测、提醒循环在此期间写入的列。
func (r *SiteLoginStateRepository) UpsertLoginState(siteName string, fields map[string]any) error {
	if siteName == "" {
		return errors.New("站点名称不能为空")
	}
	if err := r.EnsureLoginStateRow(DefaultSiteLoginState(siteName)); err != nil {
		return err
	}

	updates := make(map[string]any, len(fields))
	for key, val := range fields {
		column, ok := loginStateFieldColumns[key]
		if !ok {
			continue
		}
		switch v := val.(type) {
		case int, string:
			updates[column] = v
		}
	}
	if len(updates) == 0 {
		return nil
	}
	if err := r.UpdateColumns(siteName, updates); err != nil {
		return fmt.Errorf("保存站点登录状态失败: %w", err)
	}
	return nil
}

// UpdateColumns 只写给出的列（键为列名），不读不改其他列。
func (r *SiteLoginStateRepository) UpdateColumns(siteName string, columns map[string]any) error {
	if siteName == "" {
		return errors.New("站点名称不能为空")
	}
	if len(columns) == 0 {
		return nil
	}
	if err := r.db.Model(&SiteLoginState{}).Where("site_name = ?", siteName).Updates(columns).Error; err != nil {
		return fmt.Errorf("更新站点登录状态失败: %w", err)
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

// ClampLastVisit 记录浏览器扩展上报的访问时间：晚于 now 的按 now 记，早于已记录值的忽略（只前进不后退）。
// 比较在 Go 里做：历史数据可能以本地时区存储，SQLite 按字符串比较时间会排错序。
func (r *SiteLoginStateRepository) ClampLastVisit(siteName string, ts, now time.Time) error {
	if siteName == "" {
		return errors.New("站点名称不能为空")
	}

	effective := ts
	if ts.After(now) {
		effective = now
	}
	effective = effective.UTC()

	err := r.db.Transaction(func(tx *gorm.DB) error {
		var state SiteLoginState
		if err := tx.Select("id", "last_visit_at").Where("site_name = ?", siteName).First(&state).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if state.LastVisitAt != nil && !effective.After(*state.LastVisitAt) {
			return nil
		}
		return tx.Model(&SiteLoginState{}).Where("id = ?", state.ID).Update("last_visit_at", effective).Error
	})
	if err != nil {
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
