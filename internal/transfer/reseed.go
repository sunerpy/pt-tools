package transfer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/models"
)

// ErrJobActive 表示这个 info hash 已经有进行中的任务。
var ErrJobActive = errors.New("这个种子已经有进行中的任务")

// EnqueueReseed 建一个辅种任务：种子文件已经下好并核对过（TorrentData），直接从「等待加入」开始，
// 加入下载器时暂停、保存路径用原种子的路径，校验到 100% 才开始做种，不到就移除；不移除任何原有的种子。
func (s *Service) EnqueueReseed(ctx context.Context, j *models.TorrentTransferJob) error {
	j.InfoHash = strings.ToLower(strings.TrimSpace(j.InfoHash))
	switch {
	case j.InfoHash == "" || len(j.TorrentData) == 0:
		return fmt.Errorf("%w：辅种任务缺少 info hash 或种子文件", ErrInvalid)
	case j.TargetDownloaderID == 0:
		return fmt.Errorf("%w：辅种任务缺少下载器", ErrInvalid)
	}
	j.Kind = models.JobKindReseed
	j.SourceDownloaderID = j.TargetDownloaderID
	j.State = models.TransferExported
	return s.cfg.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Model(&models.TorrentTransferJob{}).
			Where("info_hash = ? AND state IN ?", j.InfoHash, models.TransferActiveStates).Count(&n).Error; err != nil {
			return fmt.Errorf("检查进行中的任务失败: %w", err)
		}
		if n > 0 {
			return ErrJobActive
		}
		if err := tx.Create(j).Error; err != nil {
			return fmt.Errorf("保存辅种任务失败: %w", err)
		}
		return nil
	})
}

// recordReseed 为辅种成功的新种子写一条种子记录（站点 + 站内 ID），站点做种容量、免费到期与清理都按它认种子。
func (s *Service) recordReseed(ctx context.Context, j *models.TorrentTransferJob, target models.DownloaderSetting) {
	if j.SiteName == "" || j.TorrentID == "" {
		return
	}
	now := s.cfg.Now()
	pushed := true
	hash := j.InfoHash
	dlID := target.ID
	row := models.TorrentInfo{
		SiteName: j.SiteName, TorrentID: j.TorrentID, TorrentHash: &hash, Title: j.Name, Category: j.Category, Tag: j.Tags,
		IsDownloaded: true, IsPushed: &pushed, PushTime: &now, LastCheckTime: &now, DownloadSource: ReseedSource,
		DownloaderID: &dlID, DownloaderName: target.Name, DownloaderTaskID: hash, TorrentSize: j.TotalSize,
		IsCompleted: true, Progress: 100,
	}
	cols := []string{
		"torrent_hash", "title", "is_downloaded", "is_pushed", "push_time", "last_check_time", "download_source",
		"downloader_id", "downloader_name", "downloader_task_id", "torrent_size", "is_completed", "progress",
	}
	if err := s.cfg.DB.WithContext(context.WithoutCancel(ctx)).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "site_name"}, {Name: "torrent_id"}},
		DoUpdates: clause.AssignmentColumns(cols),
	}).Create(&row).Error; err != nil {
		s.cfg.Logger.Warnf("[辅种] 任务 %d 已完成，写种子记录失败: %v", j.ID, err)
	}
}
