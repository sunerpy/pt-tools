package subscribe

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/cachectl"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

// 完成以后的观察：剧集完成以后 TMDB 还可能加集（加更、特别篇编进了这一季），也可能出了下一季。

const (
	// doneCheckEvery 是每个完成的追剧多久看一次
	doneCheckEvery = 24 * time.Hour
	// reopenWithin 是完成以后多久内有了新的已播的集就重新打开
	reopenWithin = 30 * 24 * time.Hour
	// nextSeasonWithin 是完成以后多久内等下一季（之后不再看）
	nextSeasonWithin = 365 * 24 * time.Hour
)

// doneDue 看完成以后的追剧（每个每天最多一次；暂停与待确认的不参与）：完成后 30 天内 TMDB 有了新的已播的必需集就重新打开、
// 马上搜；打开了「跟下一季」的，TMDB 上下一季有了播出日期就给下一季建追剧（只建一次）。
func (s *Service) doneDue(ctx context.Context, set Settings) {
	now := s.cfg.Now()
	var subs []models.MediaSubscription
	if err := s.cfg.DB.WithContext(ctx).Where("status = ? AND media_type = ? AND completed_at IS NOT NULL AND completed_at > ?",
		models.MediaSubDone, models.MediaKindTV, now.Add(-nextSeasonWithin)).Order("id").Find(&subs).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 读取完成的追剧失败: %v", err)
		return
	}
	for i := range subs {
		sub := &subs[i]
		s.doneMu.Lock()
		last, seen := s.doneAt[sub.ID]
		if seen && now.Sub(last) < doneCheckEvery {
			s.doneMu.Unlock()
			continue
		}
		s.doneAt[sub.ID] = now
		s.doneMu.Unlock()
		if ctx.Err() != nil {
			return
		}
		if now.Sub(*sub.CompletedAt) <= reopenWithin && s.reopenIfNew(ctx, sub, set) {
			continue
		}
		if sub.FollowNextSeason && sub.NextSeasonID == 0 {
			s.followNextSeason(ctx, sub)
		}
	}
}

// reopenIfNew 在完成的追剧有了新的已播的必需集时改回在找、马上搜，返回是不是重新打开了。
func (s *Service) reopenIfNew(ctx context.Context, sub *models.MediaSubscription, set Settings) bool {
	s.mu.Lock()
	lg, err := s.rebuildLedger(ctx, sub, rebuildOpts{set: set, fresh: true})
	s.mu.Unlock()
	if err != nil || len(lg.missing) == 0 {
		return false
	}
	now := s.cfg.Now()
	msg := fmt.Sprintf("完成以后 TMDB 上有了新的集（%s），接着追", episodeList(lg.missing))
	res := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ? AND status = ?", sub.ID, models.MediaSubDone).
		Updates(map[string]any{"status": models.MediaSubActive, "completed_at": nil, "next_search_at": now, "message": msg, "updated_at": now})
	if res.Error != nil || res.RowsAffected == 0 {
		return false
	}
	rctx, run := s.startRun(ctx, sub, models.MediaRunSchedule)
	step(rctx, stepInfo, "%s", msg)
	s.finishRun(rctx, sub, run, msg, false)
	return true
}

// followNextSeason 给完成的追剧的下一季建追剧：TMDB 上下一季存在、至少一集有播出日期时才建（继承档案、站点、下载器、分类、标签、
// 保存目录、定时、版本锁（锁住的版本清空）、媒体库与「跟下一季」，从第 1 集开始）。下一季已经有订阅时记下它的编号，不重复建。
func (s *Service) followNextSeason(ctx context.Context, sub *models.MediaSubscription) {
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if err != nil {
		return
	}
	next := sub.Season + 1
	season, err := c.Season(cachectl.Bypass(ctx), sub.TMDBID, next)
	if err != nil || !slices.ContainsFunc(season.Episodes, func(e tmdb.Episode) bool { return e.AirDate != "" }) {
		return
	}
	var existing models.MediaSubscription
	if err := s.cfg.DB.WithContext(ctx).Select("id").Where("media_type = ? AND tmdb_id = ? AND season = ?", models.MediaKindTV, sub.TMDBID, next).
		Limit(1).Find(&existing).Error; err != nil {
		return
	}
	id := existing.ID
	if id == 0 {
		sc := scheduleOf(sub)
		lock, follow, start, lib := sub.LockVersion, true, 1, sub.LibraryID
		row, err := s.CreateSubscription(ctx, SubscriptionInput{
			MediaType: models.MediaKindTV, TMDBID: sub.TMDBID, Season: next, ProfileID: sub.ProfileID, Sites: decodeList(sub.Sites),
			DownloaderID: sub.DownloaderID, Category: sub.Category, Tags: sub.Tags, SavePath: sub.SavePath, Upgrade: sub.Upgrade,
			Schedule: &sc, LockVersion: &lock, FollowNextSeason: &follow, StartEpisode: &start, LibraryID: &lib,
		}, sub.Source)
		switch {
		case err == nil:
			id = row.ID
		case errors.Is(err, ErrInvalid):
			// 撞上了同时建的：用已有的那条
			if s.cfg.DB.WithContext(ctx).Select("id").Where("media_type = ? AND tmdb_id = ? AND season = ?", models.MediaKindTV, sub.TMDBID, next).
				Limit(1).Find(&existing).Error == nil {
				id = existing.ID
			}
		default:
			s.cfg.Logger.Warnf("[订阅] 给「%s」建第 %d 季的追剧失败: %v", sub.Title, next, err)
		}
	}
	if id == 0 {
		return
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ? AND next_season_id = 0", sub.ID).
		Update("next_season_id", id).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 记下下一季的追剧失败: %v", err)
		return
	}
	rctx, run := s.startRun(ctx, sub, models.MediaRunSchedule)
	msg := fmt.Sprintf("第 %d 季有了播出日期，建了它的追剧", next)
	step(rctx, stepInfo, "%s", msg)
	s.finishRun(rctx, sub, run, msg, false)
}
