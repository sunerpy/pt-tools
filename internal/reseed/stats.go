package reseed

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/utils"
)

// ---------- 统计 ----------
//
// 统计全部从持久的数据算，不另设会漏记的计数器：
//   - 建成任务、失败、站点已删除、「已有进行中的任务」：尝试行按结局，归到开始尝试的那一天（被打断的不算）；
//   - 成功、撤回、任务失败、取消：尝试行的任务结果（清除任务时抄进来的；任务还在时读任务表），归到任务完成的那一天；
//     任务被别的途径删掉、结果不知道的不算；
//   - 下载以前就跳过的：reseed_daily_skips；IYUU 请求：reseed_iyuu_daily。
// 按进程时区分段（与刷流的统计同一套分段）；时间按时刻比较（julianday），不按文本。

// maxStatsDays 是自定义范围最长几天（尝试行保留 5 年）。
const maxStatsDays = 5 * 366

// StatsQuery 是统计的范围：Granularity（day、week、month、quarter、year，默认 day）加最近 Periods 段
// （0 是默认段数，最后一段是当前这一段、只算到今天），或者自定义 From、To（YYYY-MM-DD，含两头）。
type StatsQuery struct {
	Granularity string
	Periods     int
	From, To    string
}

// StatsCounts 是一段时间里的各项计数。
type StatsCounts struct {
	// Created 是建成的辅种任务，Failed 是下载或核对没通过，Gone 是站点上已经删了，
	// Active 是种子已经下载、核对过，但这个种子已有进行中的任务（算跳过）。按开始尝试的那一天。
	Created int `json:"created"`
	Failed  int `json:"failed"`
	Gone    int `json:"gone"`
	Active  int `json:"active"`
	// Succeeded 是校验通过、开始做种，RolledBack 是校验没到 100% 撤回了，JobFailed 是任务失败，Canceled 是任务被取消。
	// 按任务完成的那一天。
	Succeeded  int `json:"succeeded"`
	RolledBack int `json:"rolled_back"`
	JobFailed  int `json:"job_failed"`
	Canceled   int `json:"canceled"`
	// 下载以前就跳过的：下载器里已经有、已经尝试过、今天这个站点已经加满、站点不在辅种范围。每一轮都算一次。
	SkipPresent     int `json:"skip_present"`
	SkipTried       int `json:"skip_tried"`
	SkipDailyLimit  int `json:"skip_daily_limit"`
	SkipNotSelected int `json:"skip_not_selected"`
	// IYUU 请求（站点的合计里没有）
	IYUUCalls       int `json:"iyuu_calls"`
	IYUURateLimited int `json:"iyuu_rate_limited"`
	IYUUFailed      int `json:"iyuu_failed"`
	IYUUHashes      int `json:"iyuu_hashes"`
}

func (c StatsCounts) empty() bool { return c == StatsCounts{} }

// StatsPoint 是一段：Date 是第一天，End 是最后一天（自定义范围时裁到范围以内）。
type StatsPoint struct {
	Date string `json:"date"`
	End  string `json:"end"`
	StatsCounts
}

// StatsSite 是一个站点在范围内的合计（SiteName 为空串的是不在辅种范围的 IYUU 站点）。
type StatsSite struct {
	SiteName string `json:"site_name"`
	StatsCounts
}

// StatsIYUU 是范围内一个 IYUU 接口的请求合计。
type StatsIYUU struct {
	Endpoint    string `json:"endpoint"`
	Calls       int    `json:"calls"`
	OK          int    `json:"ok"`
	RateLimited int    `json:"rate_limited"`
	Failed      int    `json:"failed"`
	Hashes      int    `json:"hashes"`
}

// Stats 是一个范围的辅种统计。
type Stats struct {
	Granularity string       `json:"granularity"`
	Custom      bool         `json:"custom"`
	From        string       `json:"from"`
	To          string       `json:"to"`
	Series      []StatsPoint `json:"series"`
	Totals      StatsCounts  `json:"totals"`
	Sites       []StatsSite  `json:"sites"`
	IYUU        []StatsIYUU  `json:"iyuu"`
	// LastIYUUAt 是最近一次请求 IYUU 的时间（不限范围）
	LastIYUUAt *time.Time `json:"last_iyuu_at,omitempty"`
	// FirstDay 是最早有数据的那天（尝试、跳过或 IYUU 请求）；没有数据时为空
	FirstDay string `json:"first_day,omitempty"`
}

// statsRange 算出范围与每一段的第一天：from、to 是含两头的两天（零点，进程时区）。
func statsRange(q StatsQuery, now time.Time) (gran string, from, to time.Time, starts []time.Time, custom bool, err error) {
	loc := now.Location()
	gran = strings.TrimSpace(q.Granularity)
	if gran == "" {
		gran = utils.PeriodDay
	}
	lim, ok := utils.PeriodLimits[gran]
	if !ok {
		return "", from, to, nil, false, fmt.Errorf("%w: granularity 只能是 day、week、month、quarter、year", ErrInvalid)
	}
	custom = q.From != "" || q.To != ""
	if custom {
		if q.From == "" || q.To == "" {
			return "", from, to, nil, true, fmt.Errorf("%w: 自定义范围要同时给出开始和结束日期", ErrInvalid)
		}
		if from, err = time.ParseInLocation(time.DateOnly, q.From, loc); err != nil {
			return "", from, to, nil, true, fmt.Errorf("%w: 开始日期要写成 2026-10-30 这样", ErrInvalid)
		}
		if to, err = time.ParseInLocation(time.DateOnly, q.To, loc); err != nil {
			return "", from, to, nil, true, fmt.Errorf("%w: 结束日期要写成 2026-10-30 这样", ErrInvalid)
		}
		if to.Before(from) {
			return "", from, to, nil, true, fmt.Errorf("%w: 结束日期不能早于开始日期", ErrInvalid)
		}
		if to.Sub(from) >= maxStatsDays*24*time.Hour {
			return "", from, to, nil, true, fmt.Errorf("%w: 自定义范围最长 5 年", ErrInvalid)
		}
	} else {
		periods := lim.Default
		if q.Periods != 0 {
			if q.Periods < 1 || q.Periods > lim.Max {
				return "", from, to, nil, false, fmt.Errorf("%w: periods 应为 1–%d 的整数", ErrInvalid, lim.Max)
			}
			periods = q.Periods
		}
		to = utils.PeriodStart(now, utils.PeriodDay)
		from = utils.PeriodAdd(utils.PeriodStart(now, gran), gran, -(periods - 1))
	}
	for st := utils.PeriodStart(from, gran); !st.After(to); st = utils.PeriodAdd(st, gran, 1) {
		if len(starts) == lim.Max {
			return "", from, to, nil, custom, fmt.Errorf("%w: 这个范围按这种分段超过 %d 段，换成更大的分段", ErrInvalid, lim.Max)
		}
		starts = append(starts, st)
	}
	return gran, from, to, starts, custom, nil
}

// Stats 算一个范围的辅种统计。
func (s *Service) Stats(ctx context.Context, q StatsQuery) (Stats, error) {
	now := s.cfg.Now()
	loc := now.Location()
	gran, from, to, starts, custom, err := statsRange(q, now)
	if err != nil {
		return Stats{}, err
	}
	out := Stats{
		Granularity: gran, Custom: custom, From: from.Format(time.DateOnly), To: to.Format(time.DateOnly),
		Series: make([]StatsPoint, len(starts)), Sites: []StatsSite{}, IYUU: []StatsIYUU{},
	}
	for i, st := range starts {
		d, e := st, utils.PeriodAdd(st, gran, 1).AddDate(0, 0, -1)
		if custom {
			d, e = later(d, from), earlier(e, to)
		}
		out.Series[i].Date, out.Series[i].End = d.Format(time.DateOnly), e.Format(time.DateOnly)
	}
	hi := to.AddDate(0, 0, 1)
	index := func(t time.Time) int {
		t = t.In(loc)
		if t.Before(from) || !t.Before(hi) {
			return -1
		}
		return sort.Search(len(starts), func(k int) bool { return starts[k].After(t) }) - 1
	}
	sites := map[string]*StatsCounts{}
	// add 把一项计进第 i 段与合计；site 不为 nil 时也计进那个站点
	add := func(i int, site *string, f func(*StatsCounts)) {
		f(&out.Series[i].StatsCounts)
		f(&out.Totals)
		if site == nil {
			return
		}
		c := sites[*site]
		if c == nil {
			c = &StatsCounts{}
			sites[*site] = c
		}
		f(c)
	}
	db := s.cfg.DB.WithContext(ctx)

	// 尝试：按开始尝试的那一天
	var atts []struct {
		StartedAt time.Time
		SiteName  string
		Outcome   string
	}
	if err = db.Model(&models.ReseedAttempt{}).Select("started_at, site_name, outcome").
		Where("outcome IN ? AND julianday(started_at) >= julianday(?) AND julianday(started_at) < julianday(?)",
			[]string{models.ReseedAttemptCreated, models.ReseedAttemptFailed, models.ReseedAttemptGone, models.ReseedAttemptActive},
			from.UTC(), hi.UTC()).
		Scan(&atts).Error; err != nil {
		return Stats{}, fmt.Errorf("读取辅种尝试失败: %w", err)
	}
	for _, a := range atts {
		i := index(a.StartedAt)
		if i < 0 {
			continue
		}
		add(i, &a.SiteName, func(c *StatsCounts) {
			switch a.Outcome {
			case models.ReseedAttemptCreated:
				c.Created++
			case models.ReseedAttemptFailed:
				c.Failed++
			case models.ReseedAttemptGone:
				c.Gone++
			case models.ReseedAttemptActive:
				c.Active++
			}
		})
	}

	// 任务结果：按任务完成的那一天（清除任务时抄进尝试行的优先，任务还在时读任务表）
	var jobs []struct {
		SiteName      string
		AttemptState  string
		AttemptAt     *time.Time
		JobState      *string
		JobFinishedAt *time.Time
	}
	finished := "COALESCE(a.job_finished_at, j.finished_at)"
	if err = db.Table("reseed_attempts AS a").Joins("LEFT JOIN torrent_transfer_jobs j ON j.id = a.job_id").
		Select("a.site_name AS site_name, a.job_state AS attempt_state, a.job_finished_at AS attempt_at, "+
			"j.state AS job_state, j.finished_at AS job_finished_at").
		Where("a.outcome = ? AND a.job_id IS NOT NULL", models.ReseedAttemptCreated).
		Where("julianday("+finished+") >= julianday(?) AND julianday("+finished+") < julianday(?)", from.UTC(), hi.UTC()).
		Scan(&jobs).Error; err != nil {
		return Stats{}, fmt.Errorf("读取辅种任务的结果失败: %w", err)
	}
	for _, j := range jobs {
		state, at := j.AttemptState, j.AttemptAt
		if state == "" && j.JobState != nil {
			state = *j.JobState
		}
		if at == nil {
			at = j.JobFinishedAt
		}
		if at == nil {
			continue
		}
		i := index(*at)
		if i < 0 {
			continue
		}
		add(i, &j.SiteName, func(c *StatsCounts) {
			switch state {
			case models.TransferDone:
				c.Succeeded++
			case models.TransferRolledBack:
				c.RolledBack++
			case models.TransferFailed:
				c.JobFailed++
			case models.TransferCanceled:
				c.Canceled++
			}
		})
	}

	// 下载以前就跳过的
	var skips []models.ReseedDailySkip
	if err = db.Where("day >= ? AND day <= ?", out.From, out.To).Find(&skips).Error; err != nil {
		return Stats{}, fmt.Errorf("读取跳过的统计失败: %w", err)
	}
	for _, k := range skips {
		day, perr := time.ParseInLocation(time.DateOnly, k.Day, loc)
		if perr != nil {
			continue
		}
		i := index(day)
		if i < 0 {
			continue
		}
		add(i, &k.SiteName, func(c *StatsCounts) {
			c.SkipPresent += k.Present
			c.SkipTried += k.Tried
			c.SkipDailyLimit += k.DailyLimit
			c.SkipNotSelected += k.NotSelected
		})
	}

	// IYUU 请求（不分站点）
	var calls []models.ReseedIYUUDaily
	if err = db.Where("day >= ? AND day <= ?", out.From, out.To).Order("endpoint").Find(&calls).Error; err != nil {
		return Stats{}, fmt.Errorf("读取 IYUU 请求计数失败: %w", err)
	}
	byEndpoint := map[string]*StatsIYUU{}
	for _, c := range calls {
		day, perr := time.ParseInLocation(time.DateOnly, c.Day, loc)
		if perr != nil {
			continue
		}
		i := index(day)
		if i < 0 {
			continue
		}
		add(i, nil, func(t *StatsCounts) {
			t.IYUUCalls += c.Calls
			t.IYUURateLimited += c.RateLimited
			t.IYUUFailed += c.Failed
			t.IYUUHashes += c.Hashes
		})
		e := byEndpoint[c.Endpoint]
		if e == nil {
			e = &StatsIYUU{Endpoint: c.Endpoint}
			byEndpoint[c.Endpoint] = e
		}
		e.Calls += c.Calls
		e.OK += c.OK
		e.RateLimited += c.RateLimited
		e.Failed += c.Failed
		e.Hashes += c.Hashes
	}
	for _, e := range byEndpoint {
		out.IYUU = append(out.IYUU, *e)
	}
	sort.Slice(out.IYUU, func(i, j int) bool { return out.IYUU[i].Endpoint < out.IYUU[j].Endpoint })

	for name, c := range sites {
		if !c.empty() {
			out.Sites = append(out.Sites, StatsSite{SiteName: name, StatsCounts: *c})
		}
	}
	sort.Slice(out.Sites, func(i, j int) bool {
		a, b := out.Sites[i], out.Sites[j]
		if x, y := a.Created+a.Failed+a.Gone, b.Created+b.Failed+b.Gone; x != y {
			return x > y
		}
		return a.SiteName < b.SiteName
	})

	if out.LastIYUUAt, err = s.lastIYUUAt(ctx); err != nil {
		return Stats{}, err
	}
	if out.FirstDay, err = s.firstStatsDay(ctx, loc); err != nil {
		return Stats{}, err
	}
	return out, nil
}

func later(a, b time.Time) time.Time {
	if a.Before(b) {
		return b
	}
	return a
}

func earlier(a, b time.Time) time.Time {
	if a.After(b) {
		return b
	}
	return a
}

// lastIYUUAt 是最近一次请求 IYUU 的时间。
func (s *Service) lastIYUUAt(ctx context.Context) (*time.Time, error) {
	var rows []models.ReseedIYUUDaily
	if err := s.cfg.DB.WithContext(ctx).Where("last_at IS NOT NULL").Order("julianday(last_at) DESC").Limit(1).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取 IYUU 请求计数失败: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0].LastAt, nil
}

// firstStatsDay 是最早有数据的那天。
func (s *Service) firstStatsDay(ctx context.Context, loc *time.Location) (string, error) {
	db := s.cfg.DB.WithContext(ctx)
	first := ""
	pick := func(day string) {
		if day != "" && (first == "" || day < first) {
			first = day
		}
	}
	var atts []models.ReseedAttempt
	if err := db.Select("started_at").Order("julianday(started_at)").Limit(1).Find(&atts).Error; err != nil {
		return "", fmt.Errorf("读取辅种尝试失败: %w", err)
	}
	if len(atts) > 0 {
		pick(atts[0].StartedAt.In(loc).Format(time.DateOnly))
	}
	for _, m := range []any{&models.ReseedDailySkip{}, &models.ReseedIYUUDaily{}} {
		var days []string
		if err := db.Model(m).Order("day").Limit(1).Pluck("day", &days).Error; err != nil {
			return "", fmt.Errorf("读取辅种统计失败: %w", err)
		}
		if len(days) > 0 {
			pick(days[0])
		}
	}
	return first, nil
}
