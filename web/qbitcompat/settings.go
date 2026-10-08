package qbitcompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
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

// observeTimeout 是一次添加以后一共最多等多久让下载器列出加进去的种子（qBittorrent 是异步加的，一般几百毫秒内就能看到）。
// 整批一起查，每次一个请求，间隔从 observeFirst 起翻倍、最多 observeMaxStep。
const (
	observeTimeout = 5 * time.Second
	observeFirst   = 100 * time.Millisecond
	observeMaxStep = time.Second
)

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

// isOwned 判断下载器里的这个种子是不是兼容入口加的那一个：所有权表里有它，并且下载器给的添加时间和记下的一样
// （删掉以后从别处加回来的时间不一样）。下载器不给添加时间、记录里没有添加时间时都不认 —— 宁可不动，也不错删别人的。
// 两种下载器的添加时间都只到秒，qBittorrent 也没有别的实例编号：在兼容入口以外删掉、又在同一秒里被别处加回来的，这里分不出来。
func isOwned(owned map[string]models.QbitCompatTorrent, hash string, t downloader.Torrent) bool {
	row, found := owned[hash]
	return found && row.AddedAt > 0 && t.DateAdded == row.AddedAt
}

// ownAdded 记下这次确实加进去的种子的所有权：要在下载器里看到它、拿到下载器给的添加时间才记，之后只认这个添加时间的那一个。
// 整批一起等，一共最多 s.observe；到时还看不到、或者下载器不给添加时间的不记 —— 没打开完全控制时客户端改不了它们，
// 但也不会错认之后从别处加的同一个种子。
func (s *Server) ownAdded(ctx context.Context, b *backend, hashes []string) {
	pending := map[string]bool{}
	for _, h := range hashes {
		pending[strings.ToLower(h)] = true
	}
	deadline := time.Now().Add(s.observe)
	step := observeFirst
	for len(pending) > 0 {
		if found, err := b.dl.GetTorrentsBy(downloader.TorrentFilter{Hashes: slices.Sorted(maps.Keys(pending))}); err == nil {
			for _, t := range found {
				h := strings.ToLower(t.InfoHash)
				if !pending[h] {
					continue
				}
				delete(pending, h)
				if t.DateAdded <= 0 {
					global.GetSlogger().Warnf("[qB 兼容] 下载器不给种子 %s 的添加时间，不记所有权（没打开完全控制时客户端改不了它）", h)
					continue
				}
				if oerr := s.own(ctx, b.setting.ID, h, t.DateAdded); oerr != nil {
					global.GetSlogger().Warnf("[qB 兼容] 记下种子 %s 的所有权失败: %v", h, oerr)
				}
			}
		}
		wait := min(step, time.Until(deadline))
		if len(pending) == 0 || wait <= 0 || !pause(ctx, wait) {
			break
		}
		step = min(step*2, observeMaxStep)
	}
	if len(pending) > 0 {
		global.GetSlogger().Warnf("[qB 兼容] 加完以后 %s 内下载器里看不到 %d 个种子，不记所有权（没打开完全控制时客户端改不了它们）: %s",
			s.observe, len(pending), strings.Join(slices.Sorted(maps.Keys(pending)), ","))
	}
}

// pause 等 d；ctx 先结束时回 false。
func pause(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// own 记下一个种子的所有权与下载器给它的添加时间（同一个种子再经兼容入口加时更新）。
func (s *Server) own(ctx context.Context, downloaderID uint, hash string, addedAt int64) error {
	row := models.QbitCompatTorrent{DownloaderID: downloaderID, InfoHash: hash, AddedAt: addedAt, CreatedAt: s.deps.Now()}
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
