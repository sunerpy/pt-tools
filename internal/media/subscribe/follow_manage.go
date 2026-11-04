package subscribe

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 追剧的建立与按集的操作：从一个种子开始追、跳过一集、这一集换版本、集号规则。

// torrentLookup 是搜索缓存里按（站点, 种子编号）找种子（生产环境是 CachedSearchOrchestrator）。
type torrentLookup interface {
	CachedTorrent(site, id string) (v2.TorrentItem, bool)
}

// Describe 是给人看的订阅设置（定时、版本锁、起始集），ChatOps 建追剧时回给用户。
func Describe(sub *models.MediaSubscription) string { return describe(sub) }

// describe 是给人看的追剧设置。
func describe(sub *models.MediaSubscription) string {
	var parts []string
	sc := scheduleOf(sub)
	switch sc.Kind {
	case "":
		parts = append(parts, "按全局间隔搜索")
	case models.MediaScheduleAirtime:
		at := sc.At
		if at == "" {
			at = defaultAirAt
		}
		parts = append(parts, fmt.Sprintf("按播出时间搜索（%s 播出，%d 小时后开始搜）", at, int(hoursOr(sc.AirDelayHours, defaultAirDelay).Hours())))
	case models.MediaScheduleDaily:
		parts = append(parts, "每天 "+sc.At+" 搜索")
	case models.MediaScheduleEveryDays:
		parts = append(parts, fmt.Sprintf("每 %d 天 %s 搜索", sc.Every, sc.At))
	case models.MediaScheduleWeekly:
		names := []string{"日", "一", "二", "三", "四", "五", "六"}
		var days []string
		for _, d := range sc.Days {
			days = append(days, "周"+names[d])
		}
		parts = append(parts, strings.Join(days, "、")+" "+sc.At+" 搜索")
	case models.MediaScheduleMonthly:
		var days []string
		for _, d := range sc.Days {
			days = append(days, fmt.Sprintf("%d 号", d))
		}
		parts = append(parts, "每月 "+strings.Join(days, "、")+" "+sc.At+" 搜索")
	case models.MediaScheduleInterval:
		parts = append(parts, "每隔 "+intervalText(sc.IntervalMinutes)+"搜索一次")
	case models.MediaScheduleCron:
		parts = append(parts, "按 cron「"+sc.Cron+"」搜索")
	}
	if sub.MediaType == models.MediaKindTV {
		if sub.LockVersion {
			if g, r, ok := lockOf(sub); ok {
				parts = append(parts, "版本锁 "+lockText(g, r))
			} else {
				parts = append(parts, "版本锁开（按第一个下载的版本定）")
			}
		}
		if sub.StartEpisode > 1 {
			parts = append(parts, fmt.Sprintf("从第 %d 集开始", sub.StartEpisode))
		}
		if sub.FollowNextSeason {
			parts = append(parts, "完成后跟下一季")
		}
		if sub.EpisodeGroupID != "" {
			parts = append(parts, "种子的季集按 TMDB 剧集组换算")
		}
	}
	if kws := splitKeywords(sub.IncludeKeywords); len(kws) > 0 {
		parts = append(parts, "标题包含 "+strings.Join(kws, " 或 "))
	}
	if kws := splitKeywords(sub.ExcludeKeywords); len(kws) > 0 {
		parts = append(parts, "标题排除 "+strings.Join(kws, "、"))
	}
	return strings.Join(parts, "，")
}

// afterCreate 写建立的运行记录；带了 from_torrent 的剧集从那个种子开始（结果写进订阅的说明）。
func (s *Service) afterCreate(ctx context.Context, row *models.MediaSubscription, in SubscriptionInput) {
	rctx, run := s.startRun(ctx, row, models.MediaRunCreate)
	kind := "订阅"
	if row.MediaType == models.MediaKindTV {
		kind = "追剧"
	}
	summary := fmt.Sprintf("建立了%s：%s", kind, describe(row))
	step(rctx, stepInfo, "%s", summary)
	if in.FromTorrent != nil && row.MediaType == models.MediaKindTV && row.Status == models.MediaSubActive {
		summary = s.fromTorrent(rctx, row, *in.FromTorrent, in.StartEpisode == nil)
		row.Message = truncate(summary, 1024)
		s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ?", row.ID).Update("message", row.Message)
		if cur, err := s.subRow(ctx, row.ID); err == nil {
			*row = cur
		}
	}
	s.finishRun(rctx, row, run, summary, false)
}

// fromTorrent 从选中的种子开始追：先在搜索缓存里找这个种子（找不到时用 title 在这个站点搜一次，编号要完全相同），
// 认出来要和这个追剧是同一部剧、同一季；起始集默认取它的集号；然后作为第一个候选照常评估（质量档案、版本锁、台账、统一推送）。
// 返回结果说明。
func (s *Service) fromTorrent(ctx context.Context, sub *models.MediaSubscription, f FromTorrent, setStart bool) string {
	site, id := strings.TrimSpace(f.Site), strings.TrimSpace(f.TorrentID)
	var item v2.TorrentItem
	found := false
	if l, ok := s.cfg.Search.(torrentLookup); ok {
		item, found = l.CachedTorrent(site, id)
	}
	if !found && strings.TrimSpace(f.Title) != "" && s.cfg.Search != nil {
		sctx, cancel := context.WithTimeout(ctx, searchTimeout)
		res, err := s.cfg.Search.Search(sctx, v2.MultiSiteSearchQuery{SearchQuery: v2.SearchQuery{Keyword: strings.TrimSpace(f.Title)}, Sites: []string{site}, Timeout: searchTimeout, RawTitles: true})
		cancel()
		if err == nil {
			for _, it := range res.Items {
				if strings.EqualFold(it.SourceSite, site) && it.ID == id {
					item, found = it, true
					break
				}
			}
		}
	}
	if !found {
		step(ctx, stepInfo, "没能从站点 %s 取到种子 %s", site, id)
		return "没能从站点取到这个种子，按追剧设置搜索"
	}
	if item.SourceSite == "" {
		item.SourceSite = site
	}
	m := s.parse(ctx, item.Title, item.Subtitle)
	if why, ok := s.sameSeason(ctx, sub, m, item); !ok {
		step(ctx, stepReject, "%s：%s", item.Title, why)
		return fmt.Sprintf("这个种子（%s）不是这部剧的第 %d 季，没有用它；按追剧设置搜索", item.Title, sub.Season)
	}
	if res, err := s.cfg.Recognizer.Recognize(ctx, recognize.Input{Title: item.Title, Subtitle: item.Subtitle, IMDbID: item.IMDbID}); err == nil && res != nil && res.Match != nil &&
		(res.Match.ID != sub.TMDBID || res.Match.MediaType != models.MediaKindTV) {
		step(ctx, stepReject, "%s：认出来是「%s」", item.Title, res.Match.Title)
		return fmt.Sprintf("这个种子认出来是「%s」，和追剧的条目不同，没有用它；按追剧设置搜索", res.Match.Title)
	}
	if setStart {
		if season, err := s.seasonFor(ctx, sub, false); err == nil {
			if sp, ok, _ := newMatcher(*sub).withMapper(s.mapperFor(ctx, sub, season)).matchMapped(m, item); ok && !sp.Whole && sp.Start > 1 {
				s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).Update("start_episode", sp.Start)
				sub.StartEpisode = sp.Start
				step(ctx, stepInfo, "从第 %d 集开始追", sp.Start)
			}
		}
	}
	set, err := s.Settings(ctx)
	if err != nil {
		return err.Error()
	}
	hr, _ := siteHR(item.SourceSite)
	c := Candidate{Site: item.SourceSite, Item: item, Meta: m, From: fromSearch, SiteHR: hr, SeenAt: s.cfg.Now()}
	failed := pushFailures(ctx)
	s.mu.Lock()
	msg := s.evaluate(ctx, sub, []Candidate{c}, set)
	if pushFailures(ctx) > failed {
		s.lockChosen(ctx, sub, m)
	}
	s.mu.Unlock()
	return "从这个种子开始追：" + msg
}

// sameSeason 报告一键追剧选中的种子是不是这部剧的这一季：选了剧集组时先按剧集组换算季集。不是时第一个返回值写明原因。
func (s *Service) sameSeason(ctx context.Context, sub *models.MediaSubscription, m meta.Meta, item v2.TorrentItem) (string, bool) {
	mt := newMatcher(*sub)
	if mt.grouped() {
		season, err := s.seasonFor(ctx, sub, false)
		if err != nil {
			season = nil
		}
		if _, ok, why := mt.withMapper(s.mapperFor(ctx, sub, season)).matchMapped(m, item); !ok {
			if why == "" {
				why = "不是这个追剧的剧集或这一季"
			}
			return why, false
		}
		return "", true
	}
	season := m.Season
	if season == 0 && m.Episode > 0 {
		season = 1
	}
	if _, ok := mt.match(m, item); !ok || (season > 0 && season != sub.Season) {
		return "不是这个追剧的剧集或这一季", false
	}
	return "", true
}

// Episodes 是一个追剧每一集的情况（重建一次台账）。
func (s *Service) Episodes(ctx context.Context, id uint) ([]EpisodeState, error) {
	sub, err := s.subRow(ctx, id)
	if err != nil {
		return nil, err
	}
	if sub.MediaType != models.MediaKindTV {
		return nil, fmt.Errorf("%w: 电影没有分集", ErrInvalid)
	}
	set, _ := s.Settings(ctx)
	s.mu.Lock()
	lg, err := s.rebuildLedger(ctx, &sub, rebuildOpts{set: set})
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return orEmpty(progressOf(lg, true).Episodes), nil
}

// episodeRow 读（没有时建）台账里的一集；n 要在 1 到总集数之间。
func (s *Service) episodeFlag(ctx context.Context, id uint, n int, column string, on bool) (*models.MediaSubscription, error) {
	sub, err := s.subRow(ctx, id)
	if err != nil {
		return nil, err
	}
	if sub.MediaType != models.MediaKindTV {
		return nil, fmt.Errorf("%w: 电影没有分集", ErrInvalid)
	}
	if n < 1 || n > max(sub.TotalEpisodes, 1) {
		return nil, fmt.Errorf("%w: 这一季没有第 %d 集", ErrInvalid, n)
	}
	row := models.MediaSubscriptionEpisode{SubscriptionID: id, Episode: n, UpdatedAt: s.cfg.Now()}
	if err := s.cfg.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return nil, fmt.Errorf("保存追剧台账失败: %w", err)
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscriptionEpisode{}).Where("subscription_id = ? AND episode = ?", id, n).
		Updates(map[string]any{column: on, "updated_at": s.cfg.Now()}).Error; err != nil {
		return nil, fmt.Errorf("保存追剧台账失败: %w", err)
	}
	return &sub, nil
}

// SkipEpisode 跳过（skip 为真）或重新要一集：跳过的集不算缺，也不影响完成。
func (s *Service) SkipEpisode(ctx context.Context, id uint, n int, skip bool) ([]EpisodeState, error) {
	sub, err := s.episodeFlag(ctx, id, n, "manual_skip", skip)
	if err != nil {
		return nil, err
	}
	s.manualRun(ctx, sub, map[bool]string{true: fmt.Sprintf("跳过第 %d 集", n), false: fmt.Sprintf("重新要第 %d 集", n)}[skip])
	return s.Episodes(ctx, id)
}

// UnlockEpisode 让一集不受版本锁限制（unlock 为真；锁住的版本久等不到时用），并马上搜一次；unlock 为假时恢复。
func (s *Service) UnlockEpisode(ctx context.Context, id uint, n int, unlock bool) ([]EpisodeState, error) {
	sub, err := s.episodeFlag(ctx, id, n, "manual_unlock", unlock)
	if err != nil {
		return nil, err
	}
	msg := fmt.Sprintf("第 %d 集换版本（不受版本锁限制）", n)
	if !unlock {
		msg = fmt.Sprintf("第 %d 集恢复版本锁", n)
	}
	s.manualRun(ctx, sub, msg)
	if unlock && sub.Status == models.MediaSubActive {
		s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ?", id).Update("next_search_at", s.cfg.Now())
	}
	return s.Episodes(ctx, id)
}

// manualRun 记一条手动操作的运行记录。
func (s *Service) manualRun(ctx context.Context, sub *models.MediaSubscription, msg string) {
	rctx, run := s.startRun(ctx, sub, models.MediaRunManual)
	step(rctx, stepInfo, "%s", msg)
	markRun(rctx, func(rl *runLog) { rl.changed = true })
	s.finishRun(rctx, sub, run, msg, false)
}

// EpisodeRuleInput 是一条集号规则：Group 为空时对所有制作组。
type EpisodeRuleInput struct {
	Group  string `json:"group"`
	Mode   string `json:"mode"`
	Offset int    `json:"offset"`
	Note   string `json:"note"`
	// Source 是规则从哪里来的（manual、ai；为空当作 manual）
	Source string `json:"source,omitempty"`
}

// EpisodeRules 列出一个追剧的集号规则。
func (s *Service) EpisodeRules(ctx context.Context, id uint) ([]models.MediaEpisodeRule, error) {
	if _, err := s.subRow(ctx, id); err != nil {
		return nil, err
	}
	var rows []models.MediaEpisodeRule
	if err := s.cfg.DB.WithContext(ctx).Where("subscription_id = ?", id).Order("group_name").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取集号规则失败: %w", err)
	}
	return orEmpty(rows), nil
}

// SaveEpisodeRule 设一个制作组的集号规则（同一个组已有时替换），然后重建台账、马上搜一次。
func (s *Service) SaveEpisodeRule(ctx context.Context, id uint, in EpisodeRuleInput) ([]models.MediaEpisodeRule, error) {
	sub, err := s.subRow(ctx, id)
	if err != nil {
		return nil, err
	}
	if sub.MediaType != models.MediaKindTV {
		return nil, fmt.Errorf("%w: 电影没有分集", ErrInvalid)
	}
	switch in.Mode {
	case models.MediaEpisodeRuleTMDB, models.MediaEpisodeRulePeriodPart, models.MediaEpisodeRuleDate:
		in.Offset = 0
	case models.MediaEpisodeRuleOffset:
		if in.Offset == 0 || in.Offset < -maxEpisodes || in.Offset > maxEpisodes {
			return nil, fmt.Errorf("%w: 偏移要是不为 0 的整数", ErrInvalid)
		}
	default:
		return nil, fmt.Errorf("%w: 规则要选 tmdb、period_part、date 或 offset", ErrInvalid)
	}
	switch in.Source {
	case "":
		in.Source = models.MediaRuleFromManual
	case models.MediaRuleFromManual, models.MediaRuleFromAI, models.MediaRuleFromAuto:
	default:
		return nil, fmt.Errorf("%w: 来源要选 manual、ai 或 auto", ErrInvalid)
	}
	group := strings.TrimSpace(in.Group)
	if utf8.RuneCountInString(group) > 64 || utf8.RuneCountInString(in.Note) > 255 {
		return nil, fmt.Errorf("%w: 制作组或说明太长", ErrInvalid)
	}
	now := s.cfg.Now()
	row := models.MediaEpisodeRule{SubscriptionID: id, Group: group, Mode: in.Mode, Offset: in.Offset, Source: in.Source, Note: strings.TrimSpace(in.Note), CreatedAt: now, UpdatedAt: now}
	if err := s.cfg.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "subscription_id"}, {Name: "group_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"mode", "offset", "source", "note", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return nil, fmt.Errorf("保存集号规则失败: %w", err)
	}
	who := "所有制作组"
	if group != "" {
		who = group
	}
	s.manualRun(ctx, &sub, fmt.Sprintf("设了集号规则：%s 按 %s", who, ruleText(row)))
	if sub.Status == models.MediaSubActive {
		s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ?", id).Update("next_search_at", now)
	}
	return s.EpisodeRules(ctx, id)
}

// DeleteEpisodeRule 删掉一个制作组的集号规则（group 为空时是对所有组的那条）。
func (s *Service) DeleteEpisodeRule(ctx context.Context, id uint, group string) ([]models.MediaEpisodeRule, error) {
	sub, err := s.subRow(ctx, id)
	if err != nil {
		return nil, err
	}
	group = strings.TrimSpace(group)
	res := s.cfg.DB.WithContext(ctx).Where("subscription_id = ? AND group_name = ?", id, group).Delete(&models.MediaEpisodeRule{})
	if res.Error != nil {
		return nil, fmt.Errorf("删除集号规则失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	who := "所有制作组"
	if group != "" {
		who = group
	}
	// 删了规则以后按自动的方式对集号：记下来，马上按新的方式搜一次
	s.manualRun(ctx, &sub, fmt.Sprintf("删掉了集号规则：%s 改回自动", who))
	if sub.Status == models.MediaSubActive {
		s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ?", id).Update("next_search_at", s.cfg.Now())
	}
	return s.EpisodeRules(ctx, id)
}

// learnTitleNumbering 在因为副标题写错了期号、按标题的集号收下的种子推送成功以后，给这个追剧记一条这个组「按 TMDB 的集号」
// 的集号规则（来源「自动」，用户可以删）。以后这个组都按标题的集号，不再靠 TMDB 的集名与播出进度判断：副标题写的期离播出进度
// 只差一期（季初），或者 TMDB 列出了副标题写的那一期（季末补缺）时，那个判断会把这个组的种子判成集号对不上。
// 认不出组名、已经有这个组（不分大小写）的规则时不记。
func (s *Service) learnTitleNumbering(ctx context.Context, sub *models.MediaSubscription, o scored) {
	group := strings.TrimSpace(o.c.Meta.Group)
	if group == "" || utf8.RuneCountInString(group) > 64 {
		return
	}
	db := s.cfg.DB.WithContext(context.WithoutCancel(ctx))
	var n int64
	if err := db.Model(&models.MediaEpisodeRule{}).Where("subscription_id = ? AND lower(group_name) = lower(?)", sub.ID, group).
		Count(&n).Error; err != nil || n > 0 {
		return
	}
	h := parseHint(o.c.Item.Title, o.c.Item.Subtitle)
	note := fmt.Sprintf("副标题把集号写成了期号：第 %d 集写的是第 %d 期%s", o.sp.Start, h.period, partLabel(h.part))
	now := s.cfg.Now()
	row := models.MediaEpisodeRule{
		SubscriptionID: sub.ID, Group: group, Mode: models.MediaEpisodeRuleTMDB, Source: models.MediaRuleFromAuto, Note: note,
		CreatedAt: now, UpdatedAt: now,
	}
	res := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if res.Error != nil {
		s.cfg.Logger.Warnf("[订阅] 给「%s」记集号规则失败: %v", sub.Title, res.Error)
		return
	}
	if res.RowsAffected > 0 {
		step(ctx, stepInfo, "记下集号规则：%s 按 TMDB 的集号（%s；不对的话在集号规则里删掉）", group, note)
		s.cfg.Logger.Infof("[订阅] 「%s」记下集号规则：%s 按 TMDB 的集号（%s）", sub.Title, group, note)
	}
}

// ruleText 是给人看的集号规则。
func ruleText(r models.MediaEpisodeRule) string {
	switch r.Mode {
	case models.MediaEpisodeRuleTMDB:
		return "TMDB 的集号"
	case models.MediaEpisodeRulePeriodPart:
		return "期号与上下（Part）"
	case models.MediaEpisodeRuleDate:
		return "播出日期"
	case models.MediaEpisodeRuleOffset:
		return fmt.Sprintf("集号偏移 %+d", r.Offset)
	}
	return r.Mode
}

// MediaFor 是订阅的条目与媒体库（给整理入库用，见 organize.MediaFor）：按种子的编号找订阅推送或认领的种子（任何状态，
// 同一个编号在几个下载器里时先看这个下载器的，再取最新的），回那个订阅的条目与媒体库（库为 0 是没指定）。
func (s *Service) MediaFor(ctx context.Context, downloaderID uint, hash string) (organize.MediaHint, bool, error) {
	hash = strings.ToLower(strings.TrimSpace(hash))
	if hash == "" {
		return organize.MediaHint{}, false, nil
	}
	var rows []models.MediaSubscriptionTorrent
	if err := s.cfg.DB.WithContext(ctx).Select("id", "subscription_id", "downloader_id").Where("info_hash = ?", hash).Order("id DESC").Find(&rows).Error; err != nil {
		return organize.MediaHint{}, false, fmt.Errorf("读取订阅的种子失败: %w", err)
	}
	if len(rows) == 0 {
		return organize.MediaHint{}, false, nil
	}
	pick := rows[0]
	for _, r := range rows {
		if r.DownloaderID == downloaderID {
			pick = r
			break
		}
	}
	var sub models.MediaSubscription
	if err := s.cfg.DB.WithContext(ctx).Select("id", "library_id", "media_type", "tmdb_id", "episode_group_id").Where("id = ?", pick.SubscriptionID).
		Limit(1).Find(&sub).Error; err != nil {
		return organize.MediaHint{}, false, fmt.Errorf("读取订阅失败: %w", err)
	}
	if sub.ID == 0 {
		return organize.MediaHint{}, false, nil
	}
	return organize.MediaHint{LibraryID: sub.LibraryID, MediaType: sub.MediaType, TMDBID: sub.TMDBID, EpisodeGroup: sub.EpisodeGroupID}, true, nil
}

// intervalText 是间隔的说法：整小时的写小时（整天的写天），其余写分钟。
func intervalText(minutes int) string {
	switch {
	case minutes >= 24*60 && minutes%(24*60) == 0:
		return fmt.Sprintf("%d 天", minutes/(24*60))
	case minutes >= 60 && minutes%60 == 0:
		return fmt.Sprintf("%d 小时", minutes/60)
	}
	return fmt.Sprintf("%d 分钟", minutes)
}
