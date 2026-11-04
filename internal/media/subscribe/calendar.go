package subscribe

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/serverindex"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/schedule"
	"github.com/sunerpy/pt-tools/models"
)

// 订阅日历与今日更新：按订阅（暂停的、待确认的不算）列出一段日子里播出的集与上映的电影。集的状态读追剧台账（不重建，入库按整理记录），
// 今天还没到播出时刻的集写还没播；每天定时把今天播出的集推送到通知通道，每日战报里写一行那天播出、入库了几集。

const (
	// calendarMaxDays 是日历一次最多查的天数（含两头）
	calendarMaxDays = 62
	// defaultTodayTime 是每天推送今日更新的默认时刻
	defaultTodayTime = "09:00"
	// todayRetry 是今日更新没取成（TMDB 读不到）以后多久再试
	todayRetry = 10 * time.Minute
)

// NoticeCalendarToday 是每天推送的今日更新。
const NoticeCalendarToday = "calendar_today"

// CalendarEntry 是日历上的一项：剧集的一集，或电影的上映。
type CalendarEntry struct {
	SubscriptionID uint   `json:"subscription_id"`
	MediaType      string `json:"media_type"`
	TMDBID         int    `json:"tmdb_id"`
	Title          string `json:"title"`
	Year           int    `json:"year,omitempty"`
	// Season、Episode 是季与集（电影是 0）；Name 是集名
	Season  int    `json:"season,omitempty"`
	Episode int    `json:"episode,omitempty"`
	Name    string `json:"name,omitempty"`
	AirDate string `json:"air_date"`
	// AirAt、SearchAt 是播出时刻与开始搜的时刻：按播出时间的追剧才有
	AirAt    *time.Time `json:"air_at,omitempty"`
	SearchAt *time.Time `json:"search_at,omitempty"`
	// State 同追剧台账：library、downloading、waiting_version、missing、upcoming、skipped
	State      string `json:"state"`
	PosterPath string `json:"poster_path,omitempty"`
	StillPath  string `json:"still_path,omitempty"`
}

// Calendar 是一段日子（From 到 To，含两头）的日历。Errors 是没读到 TMDB 的订阅（别的照样列出）。
type Calendar struct {
	From    string          `json:"from"`
	To      string          `json:"to"`
	Today   string          `json:"today"`
	Entries []CalendarEntry `json:"entries"`
	Errors  []string        `json:"errors"`
}

// Calendar 列出 from 到 to（YYYY-MM-DD，含两头，最多 62 天）里订阅的剧集播出的集与电影的上映。from 为空时从今天起，to 为空时列 7 天。
func (s *Service) Calendar(ctx context.Context, from, to string) (*Calendar, error) {
	today := s.cfg.Now().In(time.Local).Format(time.DateOnly)
	if from == "" {
		from = today
	}
	start, err := time.Parse(time.DateOnly, from)
	if err != nil {
		return nil, fmt.Errorf("%w: 开始日期要写成 2026-11-03 这样", ErrInvalid)
	}
	end := start.AddDate(0, 0, 6)
	if to != "" {
		if end, err = time.Parse(time.DateOnly, to); err != nil {
			return nil, fmt.Errorf("%w: 结束日期要写成 2026-11-03 这样", ErrInvalid)
		}
	}
	if end.Before(start) {
		return nil, fmt.Errorf("%w: 结束日期不能早于开始日期", ErrInvalid)
	}
	if int(end.Sub(start)/(24*time.Hour))+1 > calendarMaxDays {
		return nil, fmt.Errorf("%w: 一次最多查 %d 天", ErrInvalid, calendarMaxDays)
	}
	out := &Calendar{From: start.Format(time.DateOnly), To: end.Format(time.DateOnly), Today: today}
	if out.Entries, out.Errors, err = s.calendarEntries(ctx, out.From, out.To); err != nil {
		return nil, err
	}
	return out, nil
}

// calKey 是一集：TMDB 编号、季、集。
type calKey struct{ tmdbID, season, episode int }

func (s *Service) calendarEntries(ctx context.Context, from, to string) ([]CalendarEntry, []string, error) {
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if err != nil {
		return nil, nil, err
	}
	db := s.cfg.DB.WithContext(ctx)
	var subs []models.MediaSubscription
	// 暂停的不找资源，待确认的（豆瓣想看建的）确认以前也不找：都不列
	if err = db.Where("status NOT IN ?", []string{models.MediaSubPaused, models.MediaSubPending}).Order("id").Find(&subs).Error; err != nil {
		return nil, nil, fmt.Errorf("读取订阅失败: %w", err)
	}
	var subIDs []uint
	var tv, movies []int
	for _, sub := range subs {
		if sub.MediaType == models.MediaKindTV {
			subIDs, tv = append(subIDs, sub.ID), append(tv, sub.TMDBID)
		} else {
			movies = append(movies, sub.TMDBID)
		}
	}
	ledgers := map[uint]map[int]models.MediaSubscriptionEpisode{}
	if len(subIDs) > 0 {
		var rows []models.MediaSubscriptionEpisode
		if err = db.Select("subscription_id", "episode", "state").Where("subscription_id IN ?", subIDs).Find(&rows).Error; err != nil {
			return nil, nil, fmt.Errorf("读取追剧台账失败: %w", err)
		}
		for _, r := range rows {
			if ledgers[r.SubscriptionID] == nil {
				ledgers[r.SubscriptionID] = map[int]models.MediaSubscriptionEpisode{}
			}
			ledgers[r.SubscriptionID][r.Episode] = r
		}
	}
	inLib, movieLib, err := s.calendarLibrary(ctx, tv, movies)
	if err != nil {
		return nil, nil, err
	}
	now := s.cfg.Now()
	today := now.In(time.Local).Format(time.DateOnly)
	entries := []CalendarEntry{}
	errs := []string{}
	for i := range subs {
		sub := &subs[i]
		base := CalendarEntry{SubscriptionID: sub.ID, MediaType: sub.MediaType, TMDBID: sub.TMDBID, Title: sub.Title, Year: sub.Year, PosterPath: sub.PosterPath}
		if sub.MediaType != models.MediaKindTV {
			d, derr := c.Details(ctx, models.MediaKindMovie, sub.TMDBID)
			if derr != nil {
				errs = append(errs, fmt.Sprintf("%s：%v", subTitle(sub), derr))
				continue
			}
			if d.Date == "" || d.Date < from || d.Date > to {
				continue
			}
			e := base
			e.AirDate, e.State = d.Date, dateState(d.Date, today)
			if movieLib[sub.TMDBID] {
				e.State = models.MediaEpisodeLibrary
			}
			entries = append(entries, e)
			continue
		}
		season, serr := s.seasonFor(ctx, sub, false)
		if serr != nil {
			errs = append(errs, fmt.Sprintf("%s：%v", subTitle(sub), serr))
			continue
		}
		for _, ep := range season.Episodes {
			if ep.AirDate == "" || ep.AirDate < from || ep.AirDate > to {
				continue
			}
			e := base
			e.Season, e.Episode, e.Name, e.AirDate, e.StillPath = sub.Season, ep.Number, ep.Name, ep.AirDate, ep.StillPath
			if sub.ScheduleKind == models.MediaScheduleAirtime {
				if at, ok := airTime(sub, ep.AirDate); ok {
					search := at.Add(hoursOr(sub.AirDelayHours, defaultAirDelay))
					e.AirAt, e.SearchAt = &at, &search
				}
			}
			row, hasRow := ledgers[sub.ID][ep.Number]
			e.State = calendarState(sub, ep, row, hasRow, inLib[calKey{sub.TMDBID, sub.Season, ep.Number}], today)
			e.State = beforeAir(e.State, e.AirAt, now)
			entries = append(entries, e)
		}
	}
	slices.SortStableFunc(entries, func(a, b CalendarEntry) int {
		return cmp.Or(cmp.Compare(a.AirDate, b.AirDate), cmp.Compare(clock(a.AirAt), clock(b.AirAt)),
			cmp.Compare(a.Title, b.Title), cmp.Compare(a.Season, b.Season), cmp.Compare(a.Episode, b.Episode))
	})
	return entries, errs, nil
}

// calendarLibrary 是整理记录里已入库的：剧集的各集（按 TMDB 编号、季、集；开了「算作已入库」的媒体服务器里有的集也算），
// 电影（按 TMDB 编号）。
func (s *Service) calendarLibrary(ctx context.Context, tv, movies []int) (map[calKey]bool, map[int]bool, error) {
	eps, films := map[calKey]bool{}, map[int]bool{}
	db := s.cfg.DB.WithContext(ctx)
	if len(tv) > 0 {
		var rows []models.MediaTransferHistory
		if err := db.Select("tmdb_id", "season", "episode", "episode_end").
			Where("status = ? AND media_type = ? AND tmdb_id IN ?", models.MediaTransferDone, models.MediaKindTV, tv).Find(&rows).Error; err != nil {
			return nil, nil, fmt.Errorf("读取整理记录失败: %w", err)
		}
		for _, r := range rows {
			for n := r.Episode; n > 0 && n <= max(r.EpisodeEnd, r.Episode); n++ {
				eps[calKey{r.TMDBID, r.Season, n}] = true
			}
		}
		// 开了「算作已入库」的媒体服务器里有的集也算（读不到索引时不算，不挡日历）
		counted, err := serverindex.CountedEpisodes(ctx, s.cfg.DB, tv)
		if err != nil {
			s.cfg.Logger.Debugf("[订阅] 读取媒体服务器的索引失败（日历不算服务器里的集）: %v", err)
		}
		for k := range counted {
			eps[calKey{k.TMDBID, k.Season, k.Episode}] = true
		}
	}
	if len(movies) > 0 {
		var ids []int
		if err := db.Model(&models.MediaTransferHistory{}).Where("status = ? AND media_type = ? AND tmdb_id IN ?",
			models.MediaTransferDone, models.MediaKindMovie, movies).Distinct().Pluck("tmdb_id", &ids).Error; err != nil {
			return nil, nil, fmt.Errorf("读取整理记录失败: %w", err)
		}
		for _, id := range ids {
			films[id] = true
		}
	}
	return eps, films, nil
}

// calendarState 是日历上一集的状态：入库按整理记录；下载中、等版本、不追按台账；其余按播出日期（今天以后的还没播，其余缺）。
func calendarState(sub *models.MediaSubscription, ep tmdb.Episode, row models.MediaSubscriptionEpisode, hasRow, inLib bool, today string) string {
	switch {
	case inLib:
		return models.MediaEpisodeLibrary
	case hasRow && (row.State == models.MediaEpisodeDownloading || row.State == models.MediaEpisodeWaitingVersion || row.State == models.MediaEpisodeSkipped):
		return row.State
	case !hasRow && sub.StartEpisode > 0 && ep.Number < sub.StartEpisode:
		return models.MediaEpisodeSkipped
	}
	return dateState(ep.AirDate, today)
}

func dateState(date, today string) string {
	if date > today {
		return models.MediaEpisodeUpcoming
	}
	return models.MediaEpisodeMissing
}

// beforeAir 把还没到播出时刻的缺的集写成还没播（台账按日期算，今天播出的一早就算缺）。
func beforeAir(state string, airAt *time.Time, now time.Time) string {
	if state == models.MediaEpisodeMissing && airAt != nil && now.Before(*airAt) {
		return models.MediaEpisodeUpcoming
	}
	return state
}

// clock 是排序用的播出时刻（没有的排在一天的最后）。
func clock(at *time.Time) string {
	if at == nil {
		return "99:99"
	}
	return at.In(time.Local).Format("15:04")
}

// Today 是今天的日历。
func (s *Service) Today(ctx context.Context) (*Calendar, error) {
	today := s.cfg.Now().In(time.Local).Format(time.DateOnly)
	return s.Calendar(ctx, today, today)
}

// stateLabels 是给人看的状态（同网页上的台账）。
var stateLabels = map[string]string{
	models.MediaEpisodeLibrary:        "已入库",
	models.MediaEpisodeDownloading:    "下载中",
	models.MediaEpisodeWaitingVersion: "等版本",
	models.MediaEpisodeMissing:        "缺",
	models.MediaEpisodeUpcoming:       "还没播",
	models.MediaEpisodeSkipped:        "不追",
}

// StateLabel 是状态的中文说明。
func StateLabel(state string) string {
	if l, ok := stateLabels[state]; ok {
		return l
	}
	return state
}

// EntryTitle 是给人看的一项：订阅名（年份、季）加集号与集名，电影写上映。
func EntryTitle(e CalendarEntry) string {
	title := e.Title
	if e.Year > 0 {
		title = fmt.Sprintf("%s (%d)", title, e.Year)
	}
	if e.MediaType != models.MediaKindTV {
		return title + " 上映"
	}
	title += fmt.Sprintf(" 第 %d 季 E%02d", e.Season, e.Episode)
	if e.Name != "" {
		title += "「" + e.Name + "」"
	}
	return title
}

// AirClock 是按播出时间追剧的播出时刻（HH:MM）；不按播出时间的为空。
func AirClock(e CalendarEntry) string {
	if e.AirAt == nil {
		return ""
	}
	return e.AirAt.In(time.Local).Format("15:04")
}

// todayText 是今日更新的正文：一行一集（播出时刻、状态）。
func todayText(entries []CalendarEntry) string {
	var b strings.Builder
	episodes := episodesOf(entries)
	switch movies := len(entries) - episodes; {
	case movies == 0:
		fmt.Fprintf(&b, "今天播出 %d 集：", episodes)
	case episodes == 0:
		fmt.Fprintf(&b, "今天上映 %d 部电影：", movies)
	default:
		fmt.Fprintf(&b, "今天播出 %d 集、上映 %d 部电影：", episodes, movies)
	}
	for _, e := range entries {
		b.WriteString("\n· " + EntryTitle(e))
		if at := AirClock(e); at != "" {
			b.WriteString(" " + at)
		}
		b.WriteString(" · " + StateLabel(e.State))
	}
	return b.String()
}

func episodesOf(entries []CalendarEntry) int {
	n := 0
	for _, e := range entries {
		if e.MediaType == models.MediaKindTV {
			n++
		}
	}
	return n
}

// todayFields 是今日更新的事件变量。
func todayFields(day string, entries []CalendarEntry) map[string]any {
	items := make([]map[string]any, 0, len(entries))
	inLib := 0
	for _, e := range entries {
		if e.State == models.MediaEpisodeLibrary {
			inLib++
		}
		items = append(items, map[string]any{
			"title": EntryTitle(e), "media_type": e.MediaType, "season": e.Season, "episode": e.Episode, "name": e.Name,
			"air_time": AirClock(e), "state": e.State, "state_label": StateLabel(e.State),
		})
	}
	return map[string]any{"day": day, "count": len(entries), "in_library": inLib, "items": items}
}

// todayState 记下今日更新今天处理过没有（同一天只处理一次；重启以后再处理一次，通知按日期去重不会重发）。
type todayState struct {
	day   string
	tryAt time.Time
}

// todayDue 到了设置的时刻、今天还没处理过时，把今天播出的集推送到通知通道（没有时不发）。
func (s *Service) todayDue(ctx context.Context, set Settings) {
	if !set.TodayEnabled || s.cfg.Notify == nil {
		return
	}
	channels := set.TodayChannels
	if len(channels) == 0 {
		channels = set.NotifyChannels
	}
	if len(channels) == 0 {
		return
	}
	now := s.cfg.Now().In(time.Local)
	day := now.Format(time.DateOnly)
	h, m, err := schedule.Clock(set.TodayTime)
	if err != nil {
		h, m, _ = schedule.Clock(defaultTodayTime)
	}
	if now.Before(time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, time.Local)) {
		return
	}
	s.todayMu.Lock()
	if s.today.day == day || now.Sub(s.today.tryAt) < todayRetry {
		s.todayMu.Unlock()
		return
	}
	s.today.tryAt = now
	s.todayMu.Unlock()
	cal, err := s.Calendar(ctx, day, day)
	if err != nil {
		s.cfg.Logger.Warnf("[订阅] 今日更新没取成，%s 以后再试: %v", todayRetry, err)
		return
	}
	s.todayMu.Lock()
	s.today.day = day
	s.todayMu.Unlock()
	if len(cal.Entries) == 0 {
		return
	}
	n := Notice{
		Channels: channels, Kind: NoticeCalendarToday, Subject: "calendar", EventKey: "today-" + day,
		Title: "📺 今日更新 " + day, Body: todayText(cal.Entries),
	}
	n.Event = notify.NewEvent(notify.EventCalendarToday, s.cfg.Now(), todayFields(day, cal.Entries))
	if err := s.cfg.Notify(ctx, n); err != nil {
		s.cfg.Logger.Warnf("[订阅] 发今日更新失败: %v", err)
	}
}

// FollowLine 是每日战报里追剧的那一行：day 那天播出的集（还没到播出时刻的不算）与其中已入库的，label（「今日」「昨日」）写在行首。
// 订阅总开关关着、TMDB 读不到或那天没有播出的集时为空串。
func (s *Service) FollowLine(ctx context.Context, day, label string) string {
	set, err := s.Settings(ctx)
	if err != nil || !set.Enabled {
		return ""
	}
	cal, err := s.Calendar(ctx, day, day)
	if err != nil {
		s.cfg.Logger.Warnf("[订阅] 每日战报的追剧一行没取成: %v", err)
		return ""
	}
	aired, inLib := 0, 0
	for _, e := range cal.Entries {
		if e.MediaType != models.MediaKindTV || e.State == models.MediaEpisodeSkipped || e.State == models.MediaEpisodeUpcoming {
			continue
		}
		aired++
		if e.State == models.MediaEpisodeLibrary {
			inLib++
		}
	}
	if aired == 0 {
		return ""
	}
	return fmt.Sprintf("📺 %s追剧：播出 %d 集，入库 %d 集", label, aired, inLib)
}
