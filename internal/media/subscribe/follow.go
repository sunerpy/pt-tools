package subscribe

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/schedule"
	"github.com/sunerpy/pt-tools/models"
)

// 一键追剧：剧集的一季按自己的定时主动搜索（按播出时间、每隔一段时间、每天、每隔几天、每周、每月、cron 表达式）。

// Schedule 是追剧主动搜索的定时。Kind 为空时沿用设置里的全局间隔（升级以前的订阅都是它）。
type Schedule struct {
	Kind string `json:"kind"`
	// At 是钟点（HH:MM）：按播出时间时是当地的播出时刻（TMDB 只有日期），空的按 20:00
	At string `json:"at,omitempty"`
	// Every 是每隔几天（every_days，1–30）
	Every int `json:"every,omitempty"`
	// Days 是每周的星期几（weekly，0–6，0 是星期日）或每月的几号（monthly，1–31，短月按月末）
	Days []int `json:"days,omitempty"`
	// AirDelayHours、AirRetryHours、AirWindowDays 用在按播出时间：播出后多久开始搜（0 = 2 小时）、
	// 没找到时隔多久再搜（0 = 6 小时）、播出后几天里集中重试（0 = 7 天）
	AirDelayHours int `json:"air_delay_hours,omitempty"`
	AirRetryHours int `json:"air_retry_hours,omitempty"`
	AirWindowDays int `json:"air_window_days,omitempty"`
	// IntervalMinutes 是每隔几分钟搜一次（interval，30 分钟到 7 天），从上次搜索算起
	IntervalMinutes int `json:"interval_minutes,omitempty"`
	// Cron 是 cron 表达式（cron，分 时 日 月 星期；几条用分号或换行分开，到点的取最早的）
	Cron string `json:"cron,omitempty"`
}

// 按播出时间的默认值与范围。
const (
	defaultAirAt     = "20:00"
	defaultAirDelay  = 2 * time.Hour
	defaultAirRetry  = 6 * time.Hour
	defaultAirWindow = 7 * 24 * time.Hour
	maxAirDelayHours = 72
	maxAirRetryHours = 48
	maxAirWindowDays = 60
	// minIntervalMinutes、maxIntervalMinutes 是按间隔时的范围（30 分钟到 7 天）
	minIntervalMinutes  = 30
	maxIntervalMinutes  = 7 * 24 * 60
	followCheckInterval = 24 * time.Hour
	// minNextSearch 是下次搜索最早在多久以后（避免每一拍都搜）
	minNextSearch = 30 * time.Minute
	// scheduleJitter 是定时搜索的随机偏移（前后各一半）
	scheduleJitter = 10 * time.Minute
)

// ScheduleOf 读订阅的定时。
func ScheduleOf(sub *models.MediaSubscription) Schedule { return scheduleOf(sub) }

// scheduleOf 读订阅的定时。
func scheduleOf(sub *models.MediaSubscription) Schedule {
	out := Schedule{
		Kind: sub.ScheduleKind, At: sub.ScheduleAt, Every: sub.ScheduleEvery,
		AirDelayHours: sub.AirDelayHours, AirRetryHours: sub.AirRetryHours, AirWindowDays: sub.AirWindowDays,
		IntervalMinutes: sub.ScheduleIntervalMinutes, Cron: sub.ScheduleCron,
	}
	if sub.ScheduleDays != "" {
		_ = json.Unmarshal([]byte(sub.ScheduleDays), &out.Days)
	}
	if out.Days == nil {
		out.Days = []int{}
	}
	return out
}

// normalize 检查定时并规整：只保留这种定时用得上的字段；mediaType 是订阅的类型（按播出时间只用于剧集）。
func (sc Schedule) normalize(mediaType string) (Schedule, error) {
	out := Schedule{Kind: sc.Kind}
	switch sc.Kind {
	case "":
		return out, nil
	case models.MediaScheduleAirtime:
		if mediaType != models.MediaKindTV {
			return out, fmt.Errorf("%w: 按播出时间只用于剧集", ErrInvalid)
		}
		at := ""
		if sc.At != "" {
			h, m, err := schedule.Clock(sc.At)
			if err != nil {
				return out, fmt.Errorf("%w: 播出时刻要写成 HH:MM", ErrInvalid)
			}
			at = fmt.Sprintf("%02d:%02d", h, m)
		}
		switch {
		case sc.AirDelayHours < 0 || sc.AirDelayHours > maxAirDelayHours:
			return out, fmt.Errorf("%w: 播出后多久开始搜要在 0 到 %d 小时之间", ErrInvalid, maxAirDelayHours)
		case sc.AirRetryHours < 0 || sc.AirRetryHours > maxAirRetryHours:
			return out, fmt.Errorf("%w: 没找到时隔多久再搜要在 0 到 %d 小时之间", ErrInvalid, maxAirRetryHours)
		case sc.AirWindowDays < 0 || sc.AirWindowDays > maxAirWindowDays:
			return out, fmt.Errorf("%w: 集中重试的天数要在 0 到 %d 天之间", ErrInvalid, maxAirWindowDays)
		}
		out.At, out.AirDelayHours, out.AirRetryHours, out.AirWindowDays = at, sc.AirDelayHours, sc.AirRetryHours, sc.AirWindowDays
		return out, nil
	case models.MediaScheduleDaily, models.MediaScheduleEveryDays, models.MediaScheduleWeekly, models.MediaScheduleMonthly:
		out.At = sc.At
		if out.At == "" {
			out.At = defaultAirAt
		}
		switch sc.Kind {
		case models.MediaScheduleEveryDays:
			out.Every = sc.Every
		case models.MediaScheduleWeekly, models.MediaScheduleMonthly:
			out.Days = slices.Compact(slices.Sorted(slices.Values(sc.Days)))
		}
		if err := out.spec(time.Time{}).Validate(); err != nil {
			return out, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
		}
		// 存成 HH:MM（Clock 也认 8:00 与前后的空格）
		h, m, _ := schedule.Clock(out.At)
		out.At = fmt.Sprintf("%02d:%02d", h, m)
		return out, nil
	case models.MediaScheduleInterval:
		if sc.IntervalMinutes < minIntervalMinutes || sc.IntervalMinutes > maxIntervalMinutes {
			return out, fmt.Errorf("%w: 每隔多久搜一次要在 %d 分钟到 %d 天之间", ErrInvalid, minIntervalMinutes, maxIntervalMinutes/(24*60))
		}
		out.IntervalMinutes = sc.IntervalMinutes
		return out, nil
	case models.MediaScheduleCron:
		set, err := schedule.ParseCron(sc.Cron)
		if err != nil {
			return out, fmt.Errorf("%w: %s", ErrInvalid, strings.TrimPrefix(err.Error(), schedule.ErrInvalid.Error()+": "))
		}
		// 存规整后的写法（几段之间一个空格，几条之间「; 」）
		out.Cron = set.String()
		if len(out.Cron) > maxCronLen {
			return out, fmt.Errorf("%w: cron 表达式太长（最多 %d 个字符）", ErrInvalid, maxCronLen)
		}
		return out, nil
	}
	return out, fmt.Errorf("%w: 定时要选 airtime、interval、daily、every_days、weekly、monthly 或 cron（空的沿用全局间隔）", ErrInvalid)
}

// maxCronLen 是存下来的 cron 表达式最长几个字符（和订阅表的列宽一样）。
const maxCronLen = 512

// spec 是 internal/schedule 的定时（每天、每隔几天、每周、每月、cron 表达式）；anchor 是每隔几天的起点。
func (sc Schedule) spec(anchor time.Time) schedule.Spec {
	return schedule.Spec{Kind: sc.Kind, At: sc.At, Every: sc.Every, Days: sc.Days, Anchor: anchor, Expr: sc.Cron}
}

// columns 是定时在订阅表里的几列。
func (sc Schedule) columns() map[string]any {
	days := ""
	if len(sc.Days) > 0 {
		b, _ := json.Marshal(sc.Days)
		days = string(b)
	}
	return map[string]any{
		"schedule_kind": sc.Kind, "schedule_at": sc.At, "schedule_every": sc.Every, "schedule_days": days,
		"air_delay_hours": sc.AirDelayHours, "air_retry_hours": sc.AirRetryHours, "air_window_days": sc.AirWindowDays,
		"schedule_interval_minutes": sc.IntervalMinutes, "schedule_cron": sc.Cron,
	}
}

func (sc Schedule) apply(row *models.MediaSubscription) {
	row.ScheduleKind, row.ScheduleAt, row.ScheduleEvery = sc.Kind, sc.At, sc.Every
	row.ScheduleDays = ""
	if len(sc.Days) > 0 {
		b, _ := json.Marshal(sc.Days)
		row.ScheduleDays = string(b)
	}
	row.AirDelayHours, row.AirRetryHours, row.AirWindowDays = sc.AirDelayHours, sc.AirRetryHours, sc.AirWindowDays
	row.ScheduleIntervalMinutes, row.ScheduleCron = sc.IntervalMinutes, sc.Cron
}

func hoursOr(n int, def time.Duration) time.Duration {
	if n <= 0 {
		return def
	}
	return time.Duration(n) * time.Hour
}

// airWindow 是按播出时间时播出后集中重试的时长；也是「等版本」开始提醒的时长。
func airWindow(sub *models.MediaSubscription) time.Duration {
	if sub.AirWindowDays <= 0 {
		return defaultAirWindow
	}
	return time.Duration(sub.AirWindowDays) * 24 * time.Hour
}

// airTime 是一集的播出时刻：TMDB 的播出日期加订阅的播出时刻（空的按 20:00），按进程的时区。日期不对时第二个返回值为假。
func airTime(sub *models.MediaSubscription, date string) (time.Time, bool) {
	day, err := time.ParseInLocation(time.DateOnly, date, time.Local)
	if err != nil {
		return time.Time{}, false
	}
	at := sub.ScheduleAt
	if at == "" {
		at = defaultAirAt
	}
	h, m, err := schedule.Clock(at)
	if err != nil {
		h, m, _ = schedule.Clock(defaultAirAt)
	}
	return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, time.Local), true
}

// nextSearch 是下次主动搜索的时间。lg 是这个订阅现在的台账（剧集按播出时间时要用；为空时按没有缺集算）。
func (s *Service) nextSearch(sub *models.MediaSubscription, set Settings, lg *ledger, now time.Time) time.Time {
	if sub.ScheduleKind == "" {
		return now.Add(time.Duration(set.SearchIntervalHours)*time.Hour + s.cfg.Jitter(searchJitter))
	}
	var next time.Time
	switch sub.ScheduleKind {
	case models.MediaScheduleAirtime:
		next = airtimeNext(sub, lg, now)
	case models.MediaScheduleInterval:
		// 间隔从这次搜索算起；随机偏移最多前后各 5 分钟（短间隔按十分之一）
		every := time.Duration(max(sub.ScheduleIntervalMinutes, minIntervalMinutes)) * time.Minute
		jitter := min(every/10, scheduleJitter)
		next = now.Add(every + s.cfg.Jitter(jitter) - jitter/2)
	default:
		if t, ok := schedule.Next(scheduleOf(sub).spec(sub.CreatedAt), now, time.Local); ok {
			next = t.Add(s.cfg.Jitter(scheduleJitter) - scheduleJitter/2)
		} else {
			next = now.Add(followCheckInterval)
		}
	}
	if floor := now.Add(minNextSearch); next.Before(floor) {
		next = floor
	}
	return next
}

// airtimeNext 按播出时间算下次搜索：必需集里还没有的每一集（缺的、等版本的、还没播的）各算一个时间，取最早的——
// 还没到播出时刻加延迟的是那个时刻；播出以后集中重试的几天里是隔一段再搜；更早的与不知道播出日期的每天一次；
// 都没有时也每天一次（看 TMDB 有没有新的集）。
func airtimeNext(sub *models.MediaSubscription, lg *ledger, now time.Time) time.Time {
	next := now.Add(followCheckInterval)
	if lg == nil {
		return next
	}
	delay, retry, window := hoursOr(sub.AirDelayHours, defaultAirDelay), hoursOr(sub.AirRetryHours, defaultAirRetry), airWindow(sub)
	for _, n := range lg.required {
		row := lg.row(n)
		if row == nil || row.State == models.MediaEpisodeLibrary || row.State == models.MediaEpisodeDownloading {
			continue
		}
		at, ok := airTime(sub, row.AirDate)
		var t time.Time
		switch {
		case !ok:
			t = now.Add(followCheckInterval)
		case now.Before(at.Add(delay)):
			t = at.Add(delay)
		case now.Before(at.Add(window)):
			t = now.Add(retry)
		default:
			t = now.Add(followCheckInterval)
		}
		if t.Before(next) {
			next = t
		}
	}
	return next
}

// PreviewMax 是定时预览最多给几次。
const PreviewMax = 10

// SchedulePreview 是一个定时接下来几次主动搜索的时间：页面上改定时时给人看（不算随机偏移与搜索本身花的时间，
// 两次之间至少隔 30 分钟，和真正排的一样）。按播出时间的要看每一集的播出日期，这里不算，返回空；
// 没有定时的按设置里的全局间隔。anchor 是每隔几天的起点（订阅建立的时间，新建的是现在）。
func (s *Service) SchedulePreview(ctx context.Context, sc Schedule, mediaType string, anchor time.Time, n int) ([]time.Time, error) {
	if mediaType == "" {
		mediaType = models.MediaKindTV
	}
	norm, err := sc.normalize(mediaType)
	if err != nil {
		return nil, err
	}
	n = min(max(n, 1), PreviewMax)
	now := s.cfg.Now()
	if anchor.IsZero() {
		anchor = now
	}
	var step time.Duration
	switch norm.Kind {
	case models.MediaScheduleAirtime:
		return nil, nil
	case "":
		set, err := s.Settings(ctx)
		if err != nil {
			return nil, err
		}
		step = time.Duration(set.SearchIntervalHours) * time.Hour
	case models.MediaScheduleInterval:
		step = time.Duration(norm.IntervalMinutes) * time.Minute
	}
	out := make([]time.Time, 0, n)
	t := now
	for len(out) < n {
		next := t.Add(step)
		if step == 0 {
			var ok bool
			if next, ok = schedule.Next(norm.spec(anchor), t, time.Local); !ok {
				break
			}
		}
		if floor := t.Add(minNextSearch); next.Before(floor) {
			next = floor
		}
		out = append(out, next)
		t = next
	}
	return out, nil
}
