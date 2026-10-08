package qbitcompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

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

// ownedSkew 是下载器与 pt-tools 的时钟差容许值：种子的添加时间比记下所有权的时间还晚这么多，就是后来又从别处加回来的。
const ownedSkew = time.Hour

// owned 是经兼容入口加进这台下载器的种子（小写的 info hash → 记下的时间）。
func (s *Server) owned(ctx context.Context, downloaderID uint) (map[string]time.Time, error) {
	var rows []models.QbitCompatTorrent
	if err := s.deps.DB.WithContext(ctx).Where("downloader_id = ?", downloaderID).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取兼容入口加的种子失败: %w", err)
	}
	out := make(map[string]time.Time, len(rows))
	for _, r := range rows {
		out[strings.ToLower(r.InfoHash)] = r.CreatedAt
	}
	return out, nil
}

// isOwned：种子要在所有权表里，并且添加时间不晚于记下的时间（容许时钟差）；下载器不给添加时间时只看表。
func isOwned(owned map[string]time.Time, hash string, t downloader.Torrent) bool {
	at, ok := owned[hash]
	if !ok {
		return false
	}
	return t.DateAdded <= 0 || !time.Unix(t.DateAdded, 0).After(at.Add(ownedSkew))
}

// own 记下这台下载器上经兼容入口加进去的种子（再加一次时更新时间）。
func (s *Server) own(ctx context.Context, downloaderID uint, hash string) error {
	row := models.QbitCompatTorrent{DownloaderID: downloaderID, InfoHash: strings.ToLower(hash), CreatedAt: s.deps.Now()}
	return s.deps.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "downloader_id"}, {Name: "info_hash"}},
		DoUpdates: clause.AssignmentColumns([]string{"created_at"}),
	}).Create(&row).Error
}

// disown 去掉这些种子的所有权（经兼容入口删掉以后）。
func (s *Server) disown(ctx context.Context, downloaderID uint, hashes []string) error {
	if len(hashes) == 0 {
		return nil
	}
	return s.deps.DB.WithContext(ctx).Where("downloader_id = ? AND info_hash IN ?", downloaderID, hashes).Delete(&models.QbitCompatTorrent{}).Error
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
