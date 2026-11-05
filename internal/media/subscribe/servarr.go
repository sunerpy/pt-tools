package subscribe

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/sunerpy/pt-tools/models"
)

// Servarr 兼容入口（媒体中心 M10）要的几样：库里有没有这部电影、一部剧每一季入库了几集，以及让订阅尽快搜一次。

// LibraryMovie 是库里一部电影的情况：有没有整理好的文件、文件一共多大、最近整理的文件所在的目录。
type LibraryMovie struct {
	Have bool
	Size int64
	Dir  string
}

// MovieLibrary 是库里这部电影的情况（按整理记录）。
func (s *Service) MovieLibrary(ctx context.Context, tmdbID int) (LibraryMovie, error) {
	var rows []models.MediaTransferHistory
	if err := s.cfg.DB.WithContext(ctx).Select("id", "size", "target_path").
		Where("status = ? AND media_type = ? AND tmdb_id = ?", models.MediaTransferDone, models.MediaKindMovie, tmdbID).
		Order("id DESC").Find(&rows).Error; err != nil {
		return LibraryMovie{}, fmt.Errorf("读取整理记录失败: %w", err)
	}
	out := LibraryMovie{Have: len(rows) > 0}
	for _, h := range rows {
		out.Size += h.Size
	}
	if len(rows) > 0 && rows[0].TargetPath != "" {
		out.Dir = filepath.Dir(rows[0].TargetPath)
	}
	return out, nil
}

// LibrarySeason 是库里一季的情况：入库了几集、这些集的文件一共多大。
type LibrarySeason struct {
	Episodes int
	Size     int64
}

// LibrarySeasons 是一部剧库里每一季的情况（按整理记录；季号 → 情况，没有入库的季不在里面）。
func (s *Service) LibrarySeasons(ctx context.Context, tmdbID int) (map[int]LibrarySeason, error) {
	hist, err := s.itemHistory(ctx, models.MediaKindTV, tmdbID)
	if err != nil {
		return nil, err
	}
	out := map[int]LibrarySeason{}
	for n, eps := range historyEpisodes(hist) {
		out[n] = LibrarySeason{Episodes: len(eps)}
	}
	for _, h := range hist {
		if ls, ok := out[h.Season]; ok && h.Episode > 0 {
			ls.Size += h.Size
			out[h.Season] = ls
		}
	}
	return out, nil
}

// SearchSoon 让一个在找的订阅尽快主动搜一次：下次搜索的时间改成现在，后台每分钟看一次到期的搜索（订阅的总开关关着时不搜）。
// 不在找（暂停、待确认、已完成）时返回 ErrInvalid。
func (s *Service) SearchSoon(ctx context.Context, id uint) error {
	row, err := s.subRow(ctx, id)
	if err != nil {
		return err
	}
	if row.Status != models.MediaSubActive {
		return fmt.Errorf("%w: 订阅没在找资源（暂停、待确认或已完成）", ErrInvalid)
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ?", id).
		Update("next_search_at", s.cfg.Now()).Error; err != nil {
		return fmt.Errorf("保存订阅失败: %w", err)
	}
	return nil
}

// SubscriptionRow 是一条订阅（不算进度）。
func (s *Service) SubscriptionRow(ctx context.Context, id uint) (models.MediaSubscription, error) {
	return s.subRow(ctx, id)
}

// SubscriptionsOf 是一类订阅（电影或剧集）的全部行，按编号排；tmdbID 大于 0 时只要这个条目的。不算进度。
func (s *Service) SubscriptionsOf(ctx context.Context, kind string, tmdbID int) ([]models.MediaSubscription, error) {
	db := s.cfg.DB.WithContext(ctx).Where("media_type = ?", kind)
	if tmdbID > 0 {
		db = db.Where("tmdb_id = ?", tmdbID)
	}
	var rows []models.MediaSubscription
	if err := db.Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取订阅失败: %w", err)
	}
	return rows, nil
}

// AiredEpisodes 是一个追剧这一季已播出的集数（按台账，没有台账时重建一次）；读不到时为 0。
func (s *Service) AiredEpisodes(ctx context.Context, row *models.MediaSubscription) int {
	if row.MediaType != models.MediaKindTV {
		return 0
	}
	p, err := s.progress(ctx, row, false)
	if err != nil || p == nil {
		return 0
	}
	return p.Aired
}

// SetProfile 改订阅的质量档案（0 是订阅设置里的默认档案，档案要存在）。改了时在找的订阅尽快搜一次（新的条件可能对得上了）。
func (s *Service) SetProfile(ctx context.Context, id, profileID uint) (*models.MediaSubscription, error) {
	row, err := s.subRow(ctx, id)
	if err != nil {
		return nil, err
	}
	if row.ProfileID == profileID {
		return &row, nil
	}
	if err = (SubscriptionInput{ProfileID: profileID}).check(s, ctx); err != nil {
		return nil, err
	}
	upd := map[string]any{"profile_id": profileID, "updated_at": s.cfg.Now()}
	if row.Status == models.MediaSubActive {
		upd["next_search_at"] = s.cfg.Now()
	}
	if err = s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ?", id).Updates(upd).Error; err != nil {
		return nil, fmt.Errorf("保存订阅失败: %w", err)
	}
	row, err = s.subRow(ctx, id)
	return &row, err
}

// SetMonitored 按 Servarr 的 monitored 打开或暂停订阅。打开：没在找的（暂停、待确认、已完成）改成在找，searchNow 为真时马上搜，
// 为假时按定时排下次搜索；已经在找的 searchNow 为真时提前到现在。关上：只暂停在找的，已完成与待确认的不动。
func (s *Service) SetMonitored(ctx context.Context, id uint, on, searchNow bool) (*models.MediaSubscription, error) {
	row, err := s.subRow(ctx, id)
	if err != nil {
		return nil, err
	}
	now := s.cfg.Now()
	upd := map[string]any{}
	switch {
	case !on && row.Status == models.MediaSubActive:
		upd["status"] = models.MediaSubPaused
	case on && searchNow:
		upd["status"], upd["next_search_at"] = models.MediaSubActive, now
	case on && row.Status != models.MediaSubActive:
		set, serr := s.Settings(ctx)
		if serr != nil {
			set.SearchIntervalHours = DefaultSearchIntervalHours
		}
		var lg *ledger
		if row.MediaType == models.MediaKindTV {
			s.mu.Lock()
			lg, _ = s.rebuildLedger(ctx, &row, rebuildOpts{set: set})
			s.mu.Unlock()
		}
		upd["status"], upd["next_search_at"] = models.MediaSubActive, s.nextSearch(&row, set, lg, now)
	}
	if len(upd) == 0 {
		return &row, nil
	}
	upd["updated_at"] = now
	if err = s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ?", id).Updates(upd).Error; err != nil {
		return nil, fmt.Errorf("保存订阅失败: %w", err)
	}
	row, err = s.subRow(ctx, id)
	return &row, err
}
