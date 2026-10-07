package transfer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

var (
	// ErrRuleNotFound 表示规则不存在。
	ErrRuleNotFound = errors.New("转移规则不存在")
	// ErrRuleNameTaken 表示规则名称重复。
	ErrRuleNameTaken = errors.New("已经有同名的转移规则")
	// ErrPathMapInvalid 表示路径映射的设置不对。
	ErrPathMapInvalid = errors.New("路径映射无效")
)

// 任务列表的筛选。
const (
	JobsActive   = "active"
	JobsFinished = "finished"
	// MaxJobsListed 是列表最多返回的任务数（最新的在前）。
	MaxJobsListed = 200
	// MaxPathMaps 是一对下载器之间最多的路径映射条数。
	MaxPathMaps = 50
)

// ListJobs 返回最新的这一种任务；status 为 active 只列未结束的，finished 只列已结束的，其他值列全部。
func (s *Service) ListJobs(ctx context.Context, status, kind string) ([]models.TorrentTransferJob, error) {
	q := s.cfg.DB.WithContext(ctx).Omit("torrent_data").Where("kind = ?", kind).Order("id DESC").Limit(MaxJobsListed)
	switch status {
	case JobsActive:
		q = q.Where("state IN ?", models.TransferActiveStates)
	case JobsFinished:
		q = q.Where("state NOT IN ?", models.TransferActiveStates)
	}
	var jobs []models.TorrentTransferJob
	if err := q.Find(&jobs).Error; err != nil {
		return nil, fmt.Errorf("读取转移任务失败: %w", err)
	}
	return jobs, nil
}

// PathMapEntry 是一条路径映射。
type PathMapEntry struct {
	SourcePrefix string `json:"source_prefix"`
	TargetPrefix string `json:"target_prefix"`
}

// ListPathMaps 返回路径映射；sourceID、targetID 都不为 0 时只返回这一对下载器的。
func (s *Service) ListPathMaps(ctx context.Context, sourceID, targetID uint) ([]models.DownloaderPathMap, error) {
	q := s.cfg.DB.WithContext(ctx).Order("source_downloader_id, target_downloader_id, source_prefix")
	if sourceID != 0 && targetID != 0 {
		q = q.Where("source_downloader_id = ? AND target_downloader_id = ?", sourceID, targetID)
	}
	var maps []models.DownloaderPathMap
	if err := q.Find(&maps).Error; err != nil {
		return nil, fmt.Errorf("读取路径映射失败: %w", err)
	}
	return maps, nil
}

// SavePathMaps 用 entries 整体替换一对下载器之间的路径映射；前缀前后的空白去掉，两边都不能为空，源前缀不能重复。
func (s *Service) SavePathMaps(ctx context.Context, sourceID, targetID uint, entries []PathMapEntry) ([]models.DownloaderPathMap, error) {
	switch {
	case sourceID == 0 || targetID == 0:
		return nil, fmt.Errorf("%w：要选源下载器和目标下载器", ErrPathMapInvalid)
	case sourceID == targetID:
		return nil, fmt.Errorf("%w：源和目标不能是同一台下载器", ErrPathMapInvalid)
	case len(entries) > MaxPathMaps:
		return nil, fmt.Errorf("%w：最多 %d 条", ErrPathMapInvalid, MaxPathMaps)
	}
	seen := map[string]bool{}
	rows := make([]models.DownloaderPathMap, 0, len(entries))
	for _, e := range entries {
		src, dst := strings.TrimSpace(e.SourcePrefix), strings.TrimSpace(e.TargetPrefix)
		if src == "" || dst == "" {
			return nil, fmt.Errorf("%w：源路径和目标路径都不能为空", ErrPathMapInvalid)
		}
		key := strings.TrimRight(src, "/\\")
		if seen[key] {
			return nil, fmt.Errorf("%w：源路径 %s 重复了", ErrPathMapInvalid, src)
		}
		seen[key] = true
		rows = append(rows, models.DownloaderPathMap{SourceDownloaderID: sourceID, TargetDownloaderID: targetID, SourcePrefix: src, TargetPrefix: dst})
	}
	err := s.cfg.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("source_downloader_id = ? AND target_downloader_id = ?", sourceID, targetID).
			Delete(&models.DownloaderPathMap{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		return nil, fmt.Errorf("保存路径映射失败: %w", err)
	}
	return s.ListPathMaps(ctx, sourceID, targetID)
}

// ListRules 返回全部规则。
func (s *Service) ListRules(ctx context.Context) ([]models.TransferRule, error) {
	var rules []models.TransferRule
	if err := s.cfg.DB.WithContext(ctx).Order("id").Find(&rules).Error; err != nil {
		return nil, fmt.Errorf("读取转移规则失败: %w", err)
	}
	return rules, nil
}

// GetRule 按 ID 取规则。
func (s *Service) GetRule(ctx context.Context, id uint) (models.TransferRule, error) {
	var r models.TransferRule
	err := s.cfg.DB.WithContext(ctx).First(&r, id).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return r, ErrRuleNotFound
	case err != nil:
		return r, fmt.Errorf("读取转移规则失败: %w", err)
	}
	return r, nil
}

// SaveRule 新建（ID 为 0）或修改规则；运行时间与结果保持原样。
func (s *Service) SaveRule(ctx context.Context, r models.TransferRule) (models.TransferRule, error) {
	if err := NormalizeRule(&r); err != nil {
		return r, err
	}
	var dup int64
	if err := s.cfg.DB.WithContext(ctx).Model(&models.TransferRule{}).Where("name = ? AND id <> ?", r.Name, r.ID).Count(&dup).Error; err != nil {
		return r, fmt.Errorf("检查规则名称失败: %w", err)
	}
	if dup > 0 {
		return r, ErrRuleNameTaken
	}
	if r.ID == 0 {
		r.LastRunAt, r.LastResult = nil, ""
		if err := s.cfg.DB.WithContext(ctx).Create(&r).Error; err != nil {
			return r, fmt.Errorf("保存转移规则失败: %w", err)
		}
		return r, nil
	}
	// 只写配置列：运行时间与结果由运行那一方写，修改规则不碰它们（读写之间刚记下的结果不会被盖掉）
	res := s.cfg.DB.WithContext(ctx).Model(&models.TransferRule{}).Where("id = ?", r.ID).
		Select(ruleConfigColumns).Updates(&r)
	if res.Error != nil {
		return r, fmt.Errorf("保存转移规则失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return r, ErrRuleNotFound
	}
	return s.GetRule(ctx, r.ID)
}

// ruleConfigColumns 是修改规则时写的列。
var ruleConfigColumns = []string{
	"name", "enabled", "source_downloader_id", "target_downloader_id", "category", "tag", "site_name",
	"min_seeding_hours", "max_per_run", "interval_min",
}

// DeleteRule 删除规则；它建过的任务保留。
func (s *Service) DeleteRule(ctx context.Context, id uint) error {
	res := s.cfg.DB.WithContext(ctx).Delete(&models.TransferRule{}, id)
	if res.Error != nil {
		return fmt.Errorf("删除转移规则失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrRuleNotFound
	}
	return nil
}

// RecordRuleRun 记下规则的运行时间与结果。
func (s *Service) RecordRuleRun(ctx context.Context, id uint, at time.Time, summary string) error {
	if err := s.cfg.DB.WithContext(context.WithoutCancel(ctx)).Model(&models.TransferRule{}).Where("id = ?", id).
		Updates(map[string]any{"last_run_at": at, "last_result": summary}).Error; err != nil {
		return fmt.Errorf("记录规则运行结果失败: %w", err)
	}
	return nil
}

// Now 返回服务的当前时间（测试里可以替换）。
func (s *Service) Now() time.Time { return s.cfg.Now() }
