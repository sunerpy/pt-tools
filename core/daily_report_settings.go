package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
)

// DailyReportSettings 是每日战报的配置。
type DailyReportSettings struct {
	Enabled bool `json:"enabled"`
	// Time 是每天发送的时刻（HH:MM，进程时区）。
	Time string `json:"time"`
	// ChannelIDs 是接收战报的通知通道。开启时至少一个：不沿用「空列表即全部通道」。
	ChannelIDs []uint `json:"channel_ids"`
}

// DefaultDailyReportTime 是战报默认的发送时刻。
const DefaultDailyReportTime = "22:00"

// dailyReportMaxOffsetSeconds 是本安装固定偏移的上限（10 分钟）。
const dailyReportMaxOffsetSeconds = 600

// ErrDailyReportNoChannel 表示开启了战报却没有选通道。
var ErrDailyReportNoChannel = errors.New("开启每日战报至少要选一个通知通道")

// ValidateDailyReportTime 校验 HH:MM。
func ValidateDailyReportTime(v string) error {
	if len(v) != 5 {
		return fmt.Errorf("战报时间应为 HH:MM，收到 %q", v)
	}
	if _, err := time.Parse("15:04", v); err != nil {
		return fmt.Errorf("战报时间应为 HH:MM，收到 %q", v)
	}
	return nil
}

func parseChannelIDs(raw string) []uint {
	ids := []uint{}
	if strings.TrimSpace(raw) == "" {
		return ids
	}
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return []uint{}
	}
	return ids
}

// DailyReportSettings 读出战报配置；全局设置行还不存在时返回默认值（关闭、22:00、无通道）。
func (s *ConfigStore) DailyReportSettings() (DailyReportSettings, error) {
	out := DailyReportSettings{Time: DefaultDailyReportTime, ChannelIDs: []uint{}}
	var gs models.SettingsGlobal
	err := s.db.DB.Select("daily_report_enabled", "daily_report_time", "daily_report_channel_ids").First(&gs).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("读取每日战报设置失败: %w", err)
	}
	out.Enabled = gs.DailyReportEnabled
	if ValidateDailyReportTime(gs.DailyReportTime) == nil {
		out.Time = gs.DailyReportTime
	}
	out.ChannelIDs = parseChannelIDs(gs.DailyReportChannelIDs)
	return out, nil
}

// SaveDailyReportSettings 保存战报配置，只写这三列。开启时必须选了通道，且通道都存在。
func (s *ConfigStore) SaveDailyReportSettings(in DailyReportSettings) error {
	if err := ValidateDailyReportTime(in.Time); err != nil {
		return err
	}
	if in.ChannelIDs == nil {
		in.ChannelIDs = []uint{}
	}
	if in.Enabled && len(in.ChannelIDs) == 0 {
		return ErrDailyReportNoChannel
	}
	if len(in.ChannelIDs) > 0 {
		var n int64
		if err := s.db.DB.Model(&models.NotificationConf{}).Where("id IN ?", in.ChannelIDs).Count(&n).Error; err != nil {
			return fmt.Errorf("检查通知通道失败: %w", err)
		}
		if int(n) != len(dedupUint(in.ChannelIDs)) {
			return errors.New("选中的通知通道有的已经不存在，请重新选择")
		}
	}
	raw, err := json.Marshal(dedupUint(in.ChannelIDs))
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
		"daily_report_enabled":     in.Enabled,
		"daily_report_time":        in.Time,
		"daily_report_channel_ids": string(raw),
	}).Error; err != nil {
		return fmt.Errorf("保存每日战报设置失败: %w", err)
	}
	events.Publish(events.Event{Type: events.ConfigChanged, Version: time.Now().UnixNano(), Source: "daily_report", At: time.Now()})
	return nil
}

// DailyReportOffset 返回本安装固定的发送偏移（1–600 秒）。还没分配时用 randN（返回 [0, n)）分配一个并保存；
// 全局设置行还不存在时返回 0，不分配。
func (s *ConfigStore) DailyReportOffset(randN func(n int64) int64) (time.Duration, error) {
	var gs models.SettingsGlobal
	err := s.db.DB.Select("id", "daily_report_offset_seconds").First(&gs).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("读取战报偏移失败: %w", err)
	}
	if gs.DailyReportOffsetSeconds > 0 {
		return time.Duration(gs.DailyReportOffsetSeconds) * time.Second, nil
	}
	offset := int(randN(dailyReportMaxOffsetSeconds)) + 1
	if err := s.db.DB.Model(&models.SettingsGlobal{}).Where("id = ?", gs.ID).
		Update("daily_report_offset_seconds", offset).Error; err != nil {
		return 0, fmt.Errorf("保存战报偏移失败: %w", err)
	}
	return time.Duration(offset) * time.Second, nil
}

func dedupUint(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
