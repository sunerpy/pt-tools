package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
)

// DeadTorrentScanSettings 是下载器助手「失效种子」的定时扫描配置：只发通知，不删种。
type DeadTorrentScanSettings struct {
	Enabled bool `json:"enabled"`
	// IntervalHours 是两次扫描的间隔（6–168 小时）。
	IntervalHours int `json:"interval_hours"`
	// ChannelIDs 是接收通知的通道；开启时至少一个。
	ChannelIDs []uint `json:"channel_ids"`
}

// DefaultDeadTorrentScanIntervalH 是默认的扫描间隔。
const DefaultDeadTorrentScanIntervalH = 24

const (
	minDeadTorrentScanIntervalH = 6
	maxDeadTorrentScanIntervalH = 168
)

// ErrDeadTorrentScanNoChannel 表示开启了定时扫描却没有选通道。
var ErrDeadTorrentScanNoChannel = errors.New("开启失效种子定时扫描至少要选一个通知通道")

// ErrDeadTorrentScanInvalid 标记用户输入有误（与存储故障区分）。
var ErrDeadTorrentScanInvalid = errors.New("失效种子扫描设置有误")

// DeadTorrentScanSettings 读出定时扫描配置；全局设置行还不存在时返回默认值（关闭、24 小时、无通道）。
func (s *ConfigStore) DeadTorrentScanSettings() (DeadTorrentScanSettings, error) {
	out := DeadTorrentScanSettings{IntervalHours: DefaultDeadTorrentScanIntervalH, ChannelIDs: []uint{}}
	var gs models.SettingsGlobal
	err := s.db.DB.Select("dead_torrent_scan_enabled", "dead_torrent_scan_interval_h", "dead_torrent_scan_channel_ids").First(&gs).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("读取失效种子扫描设置失败: %w", err)
	}
	out.Enabled = gs.DeadTorrentScanEnabled
	if gs.DeadTorrentScanIntervalH >= minDeadTorrentScanIntervalH && gs.DeadTorrentScanIntervalH <= maxDeadTorrentScanIntervalH {
		out.IntervalHours = gs.DeadTorrentScanIntervalH
	}
	out.ChannelIDs = parseChannelIDs(gs.DeadTorrentScanChannelIDs)
	return out, nil
}

// SaveDeadTorrentScanSettings 保存定时扫描配置，只写这三列。开启时必须选了通道，且通道都存在。
func (s *ConfigStore) SaveDeadTorrentScanSettings(in DeadTorrentScanSettings) error {
	if in.IntervalHours < minDeadTorrentScanIntervalH || in.IntervalHours > maxDeadTorrentScanIntervalH {
		return fmt.Errorf("%w：扫描间隔应为 %d–%d 小时", ErrDeadTorrentScanInvalid, minDeadTorrentScanIntervalH, maxDeadTorrentScanIntervalH)
	}
	ids := dedupUint(in.ChannelIDs)
	if in.Enabled && len(ids) == 0 {
		return ErrDeadTorrentScanNoChannel
	}
	if len(ids) > 0 {
		var confs []models.NotificationConf
		if err := s.db.DB.Select("id", "enabled").Where("id IN ?", ids).Find(&confs).Error; err != nil {
			return fmt.Errorf("检查通知通道失败: %w", err)
		}
		if len(confs) != len(ids) {
			return fmt.Errorf("%w：选中的通知通道有的已经不存在，请重新选择", ErrDeadTorrentScanInvalid)
		}
		enabled := 0
		for _, c := range confs {
			if c.Enabled {
				enabled++
			}
		}
		if in.Enabled && enabled == 0 {
			return fmt.Errorf("%w：选中的通知通道都已停用，至少要有一个启用的通道", ErrDeadTorrentScanInvalid)
		}
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	var gs models.SettingsGlobal
	if err := s.db.DB.Select("id").First(&gs).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w：全局设置尚未初始化，请先保存一次全局设置", ErrDeadTorrentScanInvalid)
		}
		return fmt.Errorf("读取全局设置失败: %w", err)
	}
	if err := s.db.DB.Model(&models.SettingsGlobal{}).Where("id = ?", gs.ID).Updates(map[string]any{
		"dead_torrent_scan_enabled":     in.Enabled,
		"dead_torrent_scan_interval_h":  in.IntervalHours,
		"dead_torrent_scan_channel_ids": string(raw),
	}).Error; err != nil {
		return fmt.Errorf("保存失效种子扫描设置失败: %w", err)
	}
	events.Publish(events.Event{Type: events.ConfigChanged, Version: time.Now().UnixNano(), Source: "dead_torrent_scan", At: time.Now()})
	return nil
}
