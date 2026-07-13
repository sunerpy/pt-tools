// MIT License
// Copyright (c) 2025 pt-tools
//
// Package maintenance 提供 ~/.pt-tools 工作目录的共享清理服务（特性 D）。
// 它只在三个固定白名单子目录（logs / downloads / backups）内操作，并叠加
// 硬编码红线 denylist（torrents.db(+wal/shm)、secret.key、日志 base 文件），
// 绝不触碰工作目录之外或红线文件。
package maintenance

import (
	"errors"
	"os"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

// shouldSweepDecision 是暂存 .torrent 清理的唯一判定规则（保守，避免误删还有用的）。
// torrent 为 nil 表示 DB 无记录（孤立）。删除条件（任一命中即删）：
//   - torrent == nil（孤立种子）
//   - torrent.IsPushed == true（已推送残留）
//   - maxRetry > 0 且 RetryCount >= maxRetry（已达最大重试）
//   - 文件 mtime 早于 now-retainHours（超过保留期仍未推送）
//
// 用 mtime 而非被 RSS 每轮刷新的 last_check_time，避免"反复被看到但推不出去"的种子永不老化。
func shouldSweepDecision(torrent *models.TorrentInfo, filePath string, retainHours, maxRetry int) bool {
	if torrent == nil {
		return true
	}
	if torrent.IsPushed != nil && *torrent.IsPushed {
		return true
	}
	if maxRetry > 0 && torrent.RetryCount >= maxRetry {
		return true
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return false
	}
	cutoff := time.Now().Add(-time.Duration(retainHours) * time.Hour)
	return info.ModTime().Before(cutoff)
}

// ShouldSweepStaging 判定一个暂存 .torrent 是否应被清理（按 site+hash 定位 DB 记录）。
//
// 这是从 internal/common.go 的 shouldSweep（Issue #450）提取出来的可复用决策，使 RSS
// 推送循环（internal.sweepStagingDir）与特性 D 的目录清理共享同一规则，避免规则分叉。
// db 为 nil 或哈希计算/查询失败时返回 false（保守不删）。
func ShouldSweepStaging(db *models.TorrentDB, filePath string, siteName models.SiteGroup, retainHours, maxRetry int) bool {
	if db == nil {
		return false
	}
	hash, err := qbit.ComputeTorrentHashWithPath(filePath)
	if err != nil {
		return false
	}
	torrent, err := db.GetTorrentBySiteAndHash(string(siteName), hash)
	if err != nil {
		return false
	}
	return shouldSweepDecision(torrent, filePath, retainHours, maxRetry)
}

// shouldSweepAnySite 是特性 D 目录清理用的判定：不依赖文件名推断站点，按 hash 在
// 所有站点中查 DB 记录（查不到即视为孤立）。复用与 #450 相同的 shouldSweepDecision 规则。
// 哈希计算失败时返回 false（保守不删）。
func shouldSweepAnySite(db *models.TorrentDB, filePath string, retainHours, maxRetry int) bool {
	if db == nil {
		return false
	}
	hash, err := qbit.ComputeTorrentHashWithPath(filePath)
	if err != nil {
		return false
	}
	var ti models.TorrentInfo
	err = db.DB.Where("torrent_hash = ?", hash).First(&ti).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return shouldSweepDecision(nil, filePath, retainHours, maxRetry)
	}
	if err != nil {
		return false
	}
	return shouldSweepDecision(&ti, filePath, retainHours, maxRetry)
}
