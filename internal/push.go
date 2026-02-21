// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/transmission"
)

// PushTorrentRequest 推送种子请求参数
type PushTorrentRequest struct {
	SiteID       string // 站点ID (如 hdsky, mteam)
	TorrentID    string // 种子ID
	TorrentData  []byte // 种子文件数据
	Title        string // 种子标题
	Category     string // 分类
	Tags         string // 标签
	SavePath     string // 保存路径（可选）
	DownloaderID uint   // 下载器ID
}

// PushTorrentResult 推送种子结果
type PushTorrentResult struct {
	Success     bool
	Skipped     bool // 是否跳过（如已存在）
	TorrentHash string
	Message     string
}

// PushTorrentToDownloader 将种子推送到下载器
// 复用现有的下载和推送逻辑，同时记录到数据库
func PushTorrentToDownloader(ctx context.Context, req PushTorrentRequest) (*PushTorrentResult, error) {
	if global.GlobalDB == nil {
		return nil, errors.New("数据库未初始化")
	}

	// 获取下载器配置
	var dlSetting models.DownloaderSetting
	if err := global.GlobalDB.DB.First(&dlSetting, req.DownloaderID).Error; err != nil {
		return nil, fmt.Errorf("获取下载器失败: %w", err)
	}
	if !dlSetting.Enabled {
		return nil, fmt.Errorf("下载器 %s 未启用", dlSetting.Name)
	}

	// 创建下载器实例
	dl, err := createDownloaderInstanceForPush(dlSetting)
	if err != nil {
		return nil, fmt.Errorf("创建下载器实例失败: %w", err)
	}

	// 计算种子哈希
	torrentHash, err := qbit.ComputeTorrentHash(req.TorrentData)
	if err != nil {
		return nil, fmt.Errorf("计算种子哈希失败: %w", err)
	}

	// 检查种子是否已存在于下载器中
	exists, err := dl.CheckTorrentExists(torrentHash)
	if err != nil {
		sLogger().Warnf("检查种子存在失败: %v", err)
	}
	if exists {
		sLogger().Infof("[PushTorrent] 种子已存在于下载器中，跳过: site=%s, id=%s, hash=%s, downloader=%s",
			req.SiteID, req.TorrentID, torrentHash, dlSetting.Name)
		return &PushTorrentResult{
			Success:     true,
			Skipped:     true,
			TorrentHash: torrentHash,
			Message:     "种子已存在于下载器中",
		}, nil
	}

	// 创建或更新数据库记录
	now := time.Now()
	torrentInfo := &models.TorrentInfo{
		SiteName:       req.SiteID,
		TorrentID:      req.TorrentID,
		TorrentHash:    &torrentHash,
		Title:          req.Title,
		Tag:            req.Tags,
		Category:       req.Category,
		IsDownloaded:   true,
		LastCheckTime:  &now,
		DownloadSource: "manual_push",
	}

	err = global.GlobalDB.WithTransaction(func(tx *gorm.DB) error {
		// 使用 upsert 创建或更新记录
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "site_name"}, {Name: "torrent_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"torrent_hash", "title", "tag", "category", "is_downloaded", "last_check_time", "download_source"}),
		}).Create(torrentInfo).Error
	})
	if err != nil {
		return nil, fmt.Errorf("保存种子记录失败: %w", err)
	}

	// 构建添加选项
	opts := downloader.AddTorrentOptions{
		AddAtPaused: !dlSetting.AutoStart,
		SavePath:    req.SavePath,
		Category:    req.Category,
		Tags:        req.Tags,
	}

	glOnly, glErr := core.NewConfigStore(global.GlobalDB).GetGlobalOnly()
	if glErr == nil && glOnly.CleanupDiskProtect && glOnly.CleanupMinDiskSpaceGB > 0 {
		freeSpace, spaceErr := dl.GetClientFreeSpace(ctx)
		if spaceErr != nil {
			sLogger().Warnf("[磁盘保护] 获取磁盘空间失败，继续推送: %v", spaceErr)
		} else {
			freeGB := float64(freeSpace) / (1024 * 1024 * 1024)
			if freeGB < glOnly.CleanupMinDiskSpaceGB {
				sLogger().Warnf("[磁盘保护] %s: 磁盘空间不足 (%.1f GB < %.1f GB)，跳过推送: site=%s, id=%s",
					dlSetting.Name, freeGB, glOnly.CleanupMinDiskSpaceGB, req.SiteID, req.TorrentID)
				if glOnly.CleanupEnabled {
					events.Publish(events.Event{Type: events.DiskSpaceLow, Source: "push", At: time.Now()})
				}
				return &PushTorrentResult{
					Success:     false,
					TorrentHash: torrentHash,
					Message:     fmt.Sprintf("磁盘空间不足 (%.1f GB < %.1f GB)", freeGB, glOnly.CleanupMinDiskSpaceGB),
				}, nil
			}
		}
	}

	// 推送种子到下载器
	result, err := dl.AddTorrentFileEx(req.TorrentData, opts)
	if err != nil {
		// 更新推送失败状态
		_ = global.GlobalDB.DB.Model(&models.TorrentInfo{}).
			Where("site_name = ? AND torrent_id = ?", req.SiteID, req.TorrentID).
			Updates(map[string]any{
				"last_error":  err.Error(),
				"retry_count": gorm.Expr("retry_count + 1"),
			})
		return nil, fmt.Errorf("推送种子失败: %w", err)
	}

	if !result.Success {
		errMsg := fmt.Sprintf("%v", result.Message)
		_ = global.GlobalDB.DB.Model(&models.TorrentInfo{}).
			Where("site_name = ? AND torrent_id = ?", req.SiteID, req.TorrentID).
			Updates(map[string]any{
				"last_error":  errMsg,
				"retry_count": gorm.Expr("retry_count + 1"),
			})
		return &PushTorrentResult{
			Success:     false,
			TorrentHash: torrentHash,
			Message:     errMsg,
		}, nil
	}

	// 推送成功，更新数据库状态
	pushed := true
	err = global.GlobalDB.DB.Model(&models.TorrentInfo{}).
		Where("site_name = ? AND torrent_id = ?", req.SiteID, req.TorrentID).
		Updates(map[string]any{
			"is_pushed":  &pushed,
			"push_time":  time.Now(),
			"last_error": nil,
		}).Error
	if err != nil {
		sLogger().Warnf("更新推送状态失败: %v", err)
	}

	sLogger().Infof("[PushTorrent] 种子推送成功: site=%s, id=%s, hash=%s, downloader=%s",
		req.SiteID, req.TorrentID, torrentHash, dlSetting.Name)

	return &PushTorrentResult{
		Success:     true,
		TorrentHash: result.Hash,
	}, nil
}

// createDownloaderInstanceForPush 根据配置创建下载器实例
func createDownloaderInstanceForPush(config models.DownloaderSetting) (downloader.Downloader, error) {
	switch strings.ToLower(config.Type) {
	case "qbittorrent":
		dlConfig := qbit.NewQBitConfig(config.URL, config.Username, config.Password)
		return qbit.NewQbitClient(dlConfig, config.Name)
	case "transmission":
		dlConfig := transmission.NewTransmissionConfigWithAutoStart(config.URL, config.Username, config.Password, config.AutoStart)
		return transmission.NewTransmissionClient(dlConfig, config.Name)
	default:
		return nil, fmt.Errorf("不支持的下载器类型: %s", config.Type)
	}
}
