package subscribe

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

// 版本锁（同组同分辨率）：追剧开着版本锁时，一季里各集只要同一个制作组、同一个分辨率的版本——更新时间不一定，
// 宁可等，也不自动换版本。锁住的版本来自已有的版本（库里这一季的文件优先，其次订阅推送或认领的种子）；都没有时
// 这一轮按第一个会推送的候选定一个「本轮锁」，只用来过滤这一轮，第一次推送成功以后才写进库：一个也没推成时
// 库里不会留下没有实际版本的锁。例外是「从这个种子开始追」选中的种子没推成（lockChosen）：用户选的就是那个版本，
// 锁按它定，重新搜索时还要它。用户改了锁时以库里的为准（每次推送前重读）。

// lockOf 是订阅现在锁住的版本：没开版本锁或还没定下来时 ok 为假。
func lockOf(sub *models.MediaSubscription) (group, res string, ok bool) {
	if !sub.LockVersion || (sub.LockGroup == "" && sub.LockResolution == "") {
		return "", "", false
	}
	return sub.LockGroup, sub.LockResolution, true
}

// versionOf 是一个版本的制作组与分辨率；都认不出来时 ok 为假（解析不出组时只锁分辨率）。
func versionOf(m meta.Meta) (group, res string, ok bool) {
	group, res = truncate(strings.TrimSpace(m.Group), 64), strings.TrimSpace(m.Resolution)
	return group, res, group != "" || res != ""
}

// lockText 是给人看的锁：「HHWEB 2160p」；认不出组时写明只锁分辨率。
func lockText(group, res string) string {
	switch {
	case group != "" && res != "":
		return group + " " + res
	case res != "":
		return res + "（认不出制作组，只锁分辨率）"
	}
	return group + "（认不出分辨率，只锁制作组）"
}

// lockAllows 报告一个候选是不是锁住的版本。
func lockAllows(group, res string, m meta.Meta) bool {
	return (group == "" || strings.EqualFold(strings.TrimSpace(m.Group), group)) &&
		(res == "" || strings.EqualFold(strings.TrimSpace(m.Resolution), res))
}

// existingVersion 是已有的版本：库里这一季的文件（最早整理的那个）优先，其次订阅推送或认领的、没失败也没被换掉的种子。
func (s *Service) existingVersion(ctx context.Context, sub *models.MediaSubscription) (group, res string, ok bool) {
	rows, err := s.sameMedia(ctx, sub)
	if err == nil {
		for _, r := range rows {
			title := r.TorrentName
			if title == "" {
				title = r.SourcePath
			}
			if g, rs, ok := versionOf(s.parse(ctx, title, "")); ok {
				return g, rs, true
			}
		}
	}
	linked, err := s.torrents(ctx, sub.ID, models.MediaSubTorrentDownloading, models.MediaSubTorrentDone)
	if err != nil {
		return "", "", false
	}
	for _, t := range linked {
		if g, rs, ok := versionOf(s.parse(ctx, t.Title, t.Subtitle)); ok {
			return g, rs, true
		}
	}
	return "", "", false
}

// writeLock 在库里的锁还空着时写进锁（条件更新：用户在这期间定了锁就以用户的为准），再把订阅里的锁换成库里现在的。
func (s *Service) writeLock(ctx context.Context, sub *models.MediaSubscription, group, res string) {
	res2 := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).
		Where("id = ? AND lock_version = ? AND lock_group = '' AND lock_resolution = ''", sub.ID, true).
		Updates(map[string]any{"lock_group": group, "lock_resolution": res})
	if res2.Error != nil {
		s.cfg.Logger.Warnf("[订阅] 写版本锁失败: %v", res2.Error)
	}
	if res2.RowsAffected > 0 {
		step(ctx, stepLock, "版本锁定为 %s", lockText(group, res))
	}
	s.reloadLock(ctx, sub)
}

// reloadLock 重读库里的锁（推送前：用户中途改了锁时立即生效）。
func (s *Service) reloadLock(ctx context.Context, sub *models.MediaSubscription) {
	var row models.MediaSubscription
	if err := s.cfg.DB.WithContext(ctx).Select("lock_version", "lock_group", "lock_resolution").Where("id = ?", sub.ID).Limit(1).Find(&row).Error; err == nil {
		sub.LockVersion, sub.LockGroup, sub.LockResolution = row.LockVersion, row.LockGroup, row.LockResolution
	}
}

// ensureLock 在开着版本锁、锁还空着时按已有的版本定锁（写进库）。
func (s *Service) ensureLock(ctx context.Context, sub *models.MediaSubscription) {
	if !sub.LockVersion {
		return
	}
	if _, _, ok := lockOf(sub); ok {
		return
	}
	if g, r, ok := s.existingVersion(ctx, sub); ok {
		s.writeLock(ctx, sub, g, r)
	}
}

// lockChosen 在「从这个种子开始追」选中的种子没推成（种子文件没下载下来、下载器没接收）时按它的版本定锁：开着版本锁、
// 锁还空着时才定。不定的话下次搜索的本轮锁按分数最高的候选定，用户选的版本就丢了。
func (s *Service) lockChosen(ctx context.Context, sub *models.MediaSubscription, m meta.Meta) {
	if !sub.LockVersion || sub.MediaType != models.MediaKindTV {
		return
	}
	s.reloadLock(ctx, sub)
	if _, _, ok := lockOf(sub); ok {
		return
	}
	g, r, ok := versionOf(m)
	if !ok {
		return
	}
	step(ctx, stepInfo, "选中的种子没推成：版本锁按它的版本（%s）定下，重新搜索时还要这个版本", lockText(g, r))
	s.writeLock(ctx, sub, g, r)
}

// relock 在洗版换成了更好的整季包以后把锁改成新的版本（洗版推送的整季包不受锁限制）。
func (s *Service) relock(ctx context.Context, sub *models.MediaSubscription, t *models.MediaSubscriptionTorrent) {
	if !sub.LockVersion || sub.MediaType != models.MediaKindTV {
		return
	}
	g, r, ok := versionOf(s.parse(ctx, t.Title, t.Subtitle))
	if !ok {
		return
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).
		Updates(map[string]any{"lock_group": g, "lock_resolution": r}).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 洗版后改版本锁失败: %v", err)
		return
	}
	sub.LockGroup, sub.LockResolution = g, r
}

// waitingRemind 是等版本的提醒间隔：第一次提醒以后每 3 天最多再提醒一次。
const waitingRemind = 3 * 24 * time.Hour

// remindWaiting 给等版本的集发提醒（台账重建以后调用）：有新进入等待的、或距上次提醒超过 3 天的集时发一条，记下提醒时间。
func (s *Service) remindWaiting(ctx context.Context, sub *models.MediaSubscription, lg *ledger, set Settings) {
	if lg == nil || s.cfg.Notify == nil || len(set.NotifyChannels) == 0 {
		return
	}
	g, r, ok := lockOf(sub)
	if !ok {
		return
	}
	now := s.cfg.Now()
	var due []int
	oldest := now
	for _, row := range lg.rows {
		if row.State != models.MediaEpisodeWaitingVersion {
			continue
		}
		// 开始等以后搜过一次还没有才提醒（刚建好的追剧、刚锁住版本时还没搜过）
		if row.WaitingSince == nil || sub.LastSearchAt == nil || !sub.LastSearchAt.After(*row.WaitingSince) {
			continue
		}
		if row.WaitingNotifiedAt != nil && now.Sub(*row.WaitingNotifiedAt) < waitingRemind {
			continue
		}
		due = append(due, row.Episode)
		if at, ok := airTime(sub, row.AirDate); ok && at.Before(oldest) {
			oldest = at
		}
	}
	if len(due) == 0 {
		return
	}
	title := subTitle(sub)
	days := int(now.Sub(oldest).Hours() / 24)
	version := lockText(g, r)
	body := fmt.Sprintf("%s 播出 %d 天了，还没有 %s 的版本，会接着等。要这一集换别的版本：/unlock %d %d", episodeList(due), days, version, sub.ID, due[0])
	n := Notice{
		Channels: set.NotifyChannels, Kind: NoticeFollowWaiting, Subject: fmt.Sprintf("sub-%d", sub.ID),
		EventKey: fmt.Sprintf("followshow-waiting-%d-%s-%s", sub.ID, now.Format("20060102"), episodeList(due)), Title: "追剧等版本：" + title, Body: body,
	}
	n.Event = notify.NewEvent(notify.EventFollowWaiting, now, map[string]any{
		"subscription": title, "subscription_id": int64(sub.ID), "season": sub.Season, "episodes": episodeList(due), "version": version, "days": days,
	})
	if err := s.cfg.Notify(ctx, n); err != nil {
		s.cfg.Logger.Warnf("[订阅] 发通知失败: %v", err)
		return
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscriptionEpisode{}).Where("subscription_id = ? AND episode IN ?", sub.ID, due).
		Update("waiting_notified_at", now).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 记下提醒时间失败: %v", err)
	}
}
