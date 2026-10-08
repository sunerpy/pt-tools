package qbitcompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// errNoDownloader 是没有可用的下载器（没设、被删或没启用）。
var errNoDownloader = errors.New("没有可用的下载器：在「qB 兼容入口」里选一台，或者设一台默认下载器")

// Settings 是兼容入口的设置（网页「qB 兼容入口」页读写的那部分）。
type Settings struct {
	DownloaderID uint `json:"downloader_id"`
	FullControl  bool `json:"full_control"`
}

// LoadSettings 读设置；还没存过时是零值（默认下载器、不开完全控制）。
func LoadSettings(ctx context.Context, db *gorm.DB) (models.QbitCompatSetting, error) {
	var row models.QbitCompatSetting
	if err := db.WithContext(ctx).Where("id = ?", 1).Limit(1).Find(&row).Error; err != nil {
		return row, fmt.Errorf("读取 qB 兼容入口设置失败: %w", err)
	}
	row.ID = 1
	return row, nil
}

// SaveSettings 存下载器与完全控制开关；分类与标签不动。指定的下载器要存在。
func SaveSettings(ctx context.Context, db *gorm.DB, in Settings) (models.QbitCompatSetting, error) {
	if in.DownloaderID != 0 {
		var n int64
		if err := db.WithContext(ctx).Model(&models.DownloaderSetting{}).Where("id = ?", in.DownloaderID).Count(&n).Error; err != nil {
			return models.QbitCompatSetting{}, fmt.Errorf("读取下载器失败: %w", err)
		}
		if n == 0 {
			return models.QbitCompatSetting{}, fmt.Errorf("%w: 下载器 %d 不存在", ErrInvalid, in.DownloaderID)
		}
	}
	row := models.QbitCompatSetting{ID: 1, DownloaderID: in.DownloaderID, FullControl: in.FullControl}
	if err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"downloader_id", "full_control", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return models.QbitCompatSetting{}, fmt.Errorf("保存 qB 兼容入口设置失败: %w", err)
	}
	return LoadSettings(ctx, db)
}

// ErrInvalid 是设置里的值不对。
var ErrInvalid = errors.New("参数不对")

// categoryMap 是设置里记下的分类：名字 → 保存目录。
func categoryMap(row models.QbitCompatSetting) map[string]string {
	out := map[string]string{}
	if row.Categories != "" {
		_ = json.Unmarshal([]byte(row.Categories), &out)
	}
	return out
}

func tagList(row models.QbitCompatSetting) []string {
	var out []string
	if row.Tags != "" {
		_ = json.Unmarshal([]byte(row.Tags), &out)
	}
	return out
}

// backend 是绑定的下载器：设置里指定的那台，没指定时是默认下载器，没有默认时是第一台启用的。
type backend struct {
	setting models.DownloaderSetting
	dl      downloader.Downloader
	cfg     models.QbitCompatSetting
}

func (s *Server) backend(ctx context.Context) (*backend, error) {
	cfg, err := LoadSettings(ctx, s.deps.DB)
	if err != nil {
		return nil, err
	}
	var ds models.DownloaderSetting
	q := s.deps.DB.WithContext(ctx).Where("enabled = ?", true)
	if cfg.DownloaderID != 0 {
		q = q.Where("id = ?", cfg.DownloaderID)
	} else {
		q = q.Order("is_default DESC, id")
	}
	if ferr := q.Limit(1).Find(&ds).Error; ferr != nil {
		return nil, fmt.Errorf("读取下载器失败: %w", ferr)
	}
	if ds.ID == 0 {
		return nil, errNoDownloader
	}
	dl, err := s.deps.Instance(ctx, ds.Name)
	if err != nil {
		return nil, fmt.Errorf("连接下载器 %s 失败: %w", ds.Name, err)
	}
	return &backend{setting: ds, dl: dl, cfg: cfg}, nil
}

// withBackend 取绑定的下载器，取不到时回 503（客户端会显示连接失败）。
func (s *Server) withBackend(w http.ResponseWriter, r *http.Request) (*backend, bool) {
	b, err := s.backend(r.Context())
	if err != nil {
		global.GetSlogger().Warnf("[qB 兼容] %v", err)
		text(w, http.StatusServiceUnavailable, redact(err.Error()))
		return nil, false
	}
	return b, true
}

// saveCategory 记下一个分类（已有的更新保存目录）。
func (s *Server) saveCategory(ctx context.Context, name, savePath string) error {
	return s.updateSetting(ctx, func(row *models.QbitCompatSetting) {
		m := categoryMap(*row)
		m[name] = savePath
		b, _ := json.Marshal(m)
		row.Categories = string(b)
	})
}

// saveTags 记下标签（去重，按名字排序）。
func (s *Server) saveTags(ctx context.Context, tags []string) error {
	return s.updateSetting(ctx, func(row *models.QbitCompatSetting) {
		all := tagList(*row)
		for _, t := range tags {
			if !slices.Contains(all, t) {
				all = append(all, t)
			}
		}
		slices.Sort(all)
		b, _ := json.Marshal(all)
		row.Tags = string(b)
	})
}

func (s *Server) updateSetting(ctx context.Context, mod func(*models.QbitCompatSetting)) error {
	return s.deps.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row models.QbitCompatSetting
		if err := tx.Where("id = ?", 1).Limit(1).Find(&row).Error; err != nil {
			return err
		}
		row.ID = 1
		mod(&row)
		return tx.Save(&row).Error
	})
}

// compatHashes 是经兼容入口加进这台下载器的种子（小写的 info hash）。
func (s *Server) compatHashes(ctx context.Context) (map[string]bool, error) {
	var hashes []string
	if err := s.deps.DB.WithContext(ctx).Model(&models.TorrentInfo{}).
		Where("download_source = ? AND torrent_hash IS NOT NULL AND torrent_hash <> ''", Source).
		Pluck("torrent_hash", &hashes).Error; err != nil {
		return nil, fmt.Errorf("读取兼容入口加的种子失败: %w", err)
	}
	out := make(map[string]bool, len(hashes))
	for _, h := range hashes {
		out[strings.ToLower(h)] = true
	}
	return out, nil
}

// record 写一条审计；写不进去只记日志。
func (s *Server) record(r *http.Request, e app.AuditEntry) {
	if s.deps.Audit == nil {
		return
	}
	e.ChannelType = ChannelType
	if err := s.deps.Audit.Record(context.WithoutCancel(r.Context()), e); err != nil {
		global.GetSlogger().Warnf("[qB 兼容] 记审计失败: %v", err)
	}
}
