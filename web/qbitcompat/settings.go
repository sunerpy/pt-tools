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

// bindWindow 是还没记下添加时间的所有权第一次被用到时，下载器里的添加时间离记下所有权的时间最多差多少（两边的时钟差、下载器入队的延迟）。
const bindWindow = 10 * time.Minute

// owned 是经兼容入口加进这台下载器的种子（小写的 info hash → 所有权）。
func (s *Server) owned(ctx context.Context, downloaderID uint) (map[string]models.QbitCompatTorrent, error) {
	var rows []models.QbitCompatTorrent
	if err := s.deps.DB.WithContext(ctx).Where("downloader_id = ?", downloaderID).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取兼容入口加的种子失败: %w", err)
	}
	out := make(map[string]models.QbitCompatTorrent, len(rows))
	for _, r := range rows {
		out[strings.ToLower(r.InfoHash)] = r
	}
	return out, nil
}

// isOwned 判断下载器里的这个种子是不是兼容入口加的那一个：记下了添加时间时要一样；还没记下时，添加时间要在记下所有权的时间前后
// bindWindow 之内（这时 bind 为真，调用方把添加时间补上）。下载器不给添加时间时不认 —— 宁可不动，也不错删别人的。
func isOwned(owned map[string]models.QbitCompatTorrent, hash string, t downloader.Torrent) (ok, bind bool) {
	row, found := owned[hash]
	if !found || t.DateAdded <= 0 {
		return false, false
	}
	if row.AddedAt > 0 {
		return t.DateAdded == row.AddedAt, false
	}
	d := time.Unix(t.DateAdded, 0).Sub(row.CreatedAt)
	if d < -bindWindow || d > bindWindow {
		return false, false
	}
	return true, true
}

// bindAdded 补上所有权的添加时间（只补还没记的）。
func (s *Server) bindAdded(ctx context.Context, row models.QbitCompatTorrent, addedAt int64) {
	if err := s.deps.DB.WithContext(ctx).Model(&models.QbitCompatTorrent{}).
		Where("id = ? AND added_at = 0", row.ID).Update("added_at", addedAt).Error; err != nil {
		global.GetSlogger().Warnf("[qB 兼容] 记下种子 %s 的添加时间失败: %v", row.InfoHash, err)
	}
}

// own 记下这台下载器上经兼容入口加进去的种子。下载器已经列出它时同时记下添加时间，否则留到第一次用到时补（见 isOwned）。
func (s *Server) own(ctx context.Context, b *backend, hash string) error {
	row := models.QbitCompatTorrent{DownloaderID: b.setting.ID, InfoHash: strings.ToLower(hash), CreatedAt: s.deps.Now()}
	if t, err := b.dl.GetTorrent(row.InfoHash); err == nil && strings.EqualFold(t.InfoHash, row.InfoHash) && t.DateAdded > 0 {
		row.AddedAt = t.DateAdded
	}
	return s.deps.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "downloader_id"}, {Name: "info_hash"}},
		DoUpdates: clause.AssignmentColumns([]string{"added_at", "created_at"}),
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
