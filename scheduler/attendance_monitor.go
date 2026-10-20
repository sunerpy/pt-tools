package scheduler

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

const (
	attendanceTick           = time.Minute
	attendanceStartupDelay   = 20 * time.Second
	attendanceAttemptTimeout = 60 * time.Second
	// attendanceChallengeTimeout 是要过验证码或答题的站点一次签到的上限：要等 AI，验证码认错了还要换几张。
	attendanceChallengeTimeout = 3 * time.Minute
	// 首次加最多 2 次重试。
	attendanceMaxAttempts = 3
	// 过了当天的时间窗才开启签到（或进程才启动）时，在接下来这么久内随机取一个时刻。
	attendanceLateSpread   = 10 * time.Minute
	attendanceNotifySource = "attendance"
)

// attendanceRetryDelays 是第 1、2 次失败之后的重试间隔。
var attendanceRetryDelays = []time.Duration{10 * time.Minute, 30 * time.Minute}

// ErrAttendanceBusy 表示该站正在被登录探测或另一次签到占用（共用单站单飞锁）。
var ErrAttendanceBusy = errors.New("站点正在探测或签到，请稍后再试")

// AttendanceSites 按站点名取能签到的站点实例；生产环境取 UserInfoService 里注册的共享实例，与搜索、探测共用限速器。
type AttendanceSites interface {
	AttendanceSite(siteName string) (v2.AttendanceCapable, bool)
}

// AttendanceSitesFunc 把函数适配为 AttendanceSites。
type AttendanceSitesFunc func(siteName string) (v2.AttendanceCapable, bool)

func (f AttendanceSitesFunc) AttendanceSite(siteName string) (v2.AttendanceCapable, bool) {
	return f(siteName)
}

// AttendanceWindowSource 返回签到时间窗（HH:MM，进程时区），即 core.ConfigStore.AttendanceWindow。
type AttendanceWindowSource interface {
	AttendanceWindow() (string, string, error)
}

// AttendanceNotifySource 返回签到结果推送的设置（开没开、推到哪些通道，空表示所有启用的通道），
// 即 core.ConfigStore.AttendanceNotify。
type AttendanceNotifySource interface {
	AttendanceNotify() (bool, []uint, error)
}

// AttendanceSolverSource 给签到要识别验证码或答题的站点取 AI（cmd 里接 AI 助手与签到设置）；
// 取不到时（没开 AI 签到、AI 助手没启用）返回的错误就是给用户看的原因。
type AttendanceSolverSource interface {
	AttendanceSolver(ctx context.Context) (v2.AttendanceSolver, error)
}

// ProbeLocker 是登录探测与签到共用的单站单飞锁，即 LoginReminderMonitor.TryAcquireProbeLock。
type ProbeLocker interface {
	TryAcquireProbeLock(siteName string) (release func(), ok bool)
}

// AttendanceMonitorConfig 是 AttendanceMonitor 的依赖。
type AttendanceMonitorConfig struct {
	DB     *gorm.DB
	Sites  AttendanceSites
	Window AttendanceWindowSource
	Locker ProbeLocker
	// Notifier 为空时不发当天的签到汇总。
	Notifier *MonitorNotifier
	// NotifySettings 为空时照旧推给所有启用的通道；设置里关掉推送时不发汇总。
	NotifySettings AttendanceNotifySource
	Clock          sitelogin.Clock
	Logger         *zap.SugaredLogger
	Tick           time.Duration
	// Location 是计算日期与时间窗的时区，默认进程时区（time.Local）。
	Location *time.Location
	// Rand 返回 [0, n) 的随机数，用于在时间窗内取时刻；测试可以注入确定的值。
	Rand func(n int64) int64
	// Solver 给要过验证码或答题的站点取 AI；为空时这些站点签到失败并说明要 AI（也可以之后 SetSolver）。
	Solver AttendanceSolverSource
}

// AttendanceMonitor 每分钟检查一次开启了自动签到的站点：每站每天在时间窗内随机取一个时刻签到，
// 失败按 10、30 分钟退避重试最多 2 次，当天成功或已签后不再请求。结果写 SiteAttendanceLog，
// 当天所有站点都有结果后合成一条通知，经 MonitorNotifier 按通道静默时段投递。
type AttendanceMonitor struct {
	db       *gorm.DB
	sites    AttendanceSites
	window   AttendanceWindowSource
	locker   ProbeLocker
	notifier *MonitorNotifier
	notify   AttendanceNotifySource
	clock    sitelogin.Clock
	logger   *zap.SugaredLogger
	tick     time.Duration
	loc      *time.Location
	Rand     func(n int64) int64

	mu          sync.Mutex
	running     bool
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	summaryDone string // 已经写过汇总通知的日期，避免每分钟重复查询
	purgedDay   string // 已经清理过旧签到事件的日期，一天清一次
	solver      AttendanceSolverSource
}

// NewAttendanceMonitor 构造签到监控，不启动；调用 Start 开始调度。
func NewAttendanceMonitor(cfg AttendanceMonitorConfig) *AttendanceMonitor {
	if cfg.Clock == nil {
		cfg.Clock = sitelogin.NewRealClock()
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop().Sugar()
	}
	if cfg.Tick <= 0 {
		cfg.Tick = attendanceTick
	}
	if cfg.Location == nil {
		cfg.Location = time.Local
	}
	if cfg.Rand == nil {
		cfg.Rand = func(n int64) int64 {
			if n <= 0 {
				return 0
			}
			return rand.Int63n(n)
		}
	}
	return &AttendanceMonitor{
		db:       cfg.DB,
		sites:    cfg.Sites,
		window:   cfg.Window,
		locker:   cfg.Locker,
		notifier: cfg.Notifier,
		notify:   cfg.NotifySettings,
		clock:    cfg.Clock,
		logger:   cfg.Logger,
		tick:     cfg.Tick,
		loc:      cfg.Location,
		Rand:     cfg.Rand,
		solver:   cfg.Solver,
	}
}

// SetSolver 接上签到用的 AI。AI 助手比签到监控晚建好，所以在 cmd 里建好以后再接。
func (m *AttendanceMonitor) SetSolver(src AttendanceSolverSource) {
	m.mu.Lock()
	m.solver = src
	m.mu.Unlock()
}

// withSolver 给要过一关的站点把 AI 放进 ctx；取不到时返回原因（不需要 AI 的站点返回 nil）。
func (m *AttendanceMonitor) withSolver(ctx context.Context, siteName string) (context.Context, time.Duration, error) {
	def, _ := v2.GetDefinitionRegistry().Get(siteName)
	if v2.AttendanceChallengeOf(def) == "" {
		return ctx, attendanceAttemptTimeout, nil
	}
	m.mu.Lock()
	src := m.solver
	m.mu.Unlock()
	if src == nil {
		return ctx, attendanceChallengeTimeout, errors.New("AI 助手没有启动，不能用 AI 签到")
	}
	solver, err := src.AttendanceSolver(ctx)
	if err != nil {
		return ctx, attendanceChallengeTimeout, err
	}
	return v2.WithAttendanceSolver(ctx, solver), attendanceChallengeTimeout, nil
}

// Start 启动调度循环；重复调用无效果。
func (m *AttendanceMonitor) Start() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.running = true
	m.wg.Go(func() { m.loop(ctx) })
}

// Stop 停止调度循环并等它退出。
func (m *AttendanceMonitor) Stop() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	cancel := m.cancel
	m.mu.Unlock()
	cancel()
	m.wg.Wait()
}

func (m *AttendanceMonitor) loop(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(attendanceStartupDelay):
		m.RunOnce(ctx)
	}
	ticker := time.NewTicker(m.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.RunOnce(ctx)
		}
	}
}

// Today 返回进程时区的当天日期（YYYY-MM-DD），签到记录按它分天。
func (m *AttendanceMonitor) Today() string {
	return m.dayOf(m.clock.Now())
}

func (m *AttendanceMonitor) dayOf(t time.Time) string {
	return t.In(m.loc).Format("2006-01-02")
}

// RunOnce 处理一轮：为当天还没有记录的站点安排时刻，签到已到时刻的站点，并在都有结果后写当天的汇总通知。
func (m *AttendanceMonitor) RunOnce(ctx context.Context) {
	if m == nil || m.db == nil {
		return
	}
	sites, err := m.enabledSites()
	if err != nil {
		m.logger.Warnw("attendance_list_sites_failed", "err", err)
		return
	}
	now := m.clock.Now()
	day := m.dayOf(now)
	repo := models.NewSiteAttendanceRepository(m.db)
	for _, site := range sites {
		if ctx.Err() != nil {
			return
		}
		row, err := m.ensureDay(repo, site.Name, day, now)
		if err != nil {
			m.logger.Warnw("attendance_schedule_failed", "site", site.Name, "err", err)
			continue
		}
		if row.Done() {
			continue
		}
		due := row.ScheduledAt
		if row.NextAttemptAt != nil {
			due = *row.NextAttemptAt
		}
		if now.Before(due) {
			continue
		}
		if _, err := m.attempt(ctx, repo, site.Name, day, false); err != nil && !errors.Is(err, ErrAttendanceBusy) {
			m.logger.Warnw("attendance_attempt_failed", "site", site.Name, "err", err)
		}
	}
	m.maybeNotifySummary(ctx, repo, day, sites)
	m.maybePurgeEvents(repo, day, now)
}

// maybePurgeEvents 每天清一次 AttendanceEventRetention 以前的签到事件。
func (m *AttendanceMonitor) maybePurgeEvents(repo *models.SiteAttendanceRepository, day string, now time.Time) {
	m.mu.Lock()
	done := m.purgedDay == day
	m.purgedDay = day
	m.mu.Unlock()
	if done {
		return
	}
	n, err := repo.PurgeEventsBefore(now.Add(-models.AttendanceEventRetention))
	if err != nil {
		m.logger.Warnw("attendance_events_purge_failed", "err", err)
		return
	}
	if n > 0 {
		m.logger.Infow("attendance_events_purged", "rows", n)
	}
}

// SignNow 立即签到一次（手动或 ChatOps），不要求站点开启了自动签到。
// 成功、今日已签或不支持时写入当天的最终结果；失败只记下错误，不占用自动重试的次数。
func (m *AttendanceMonitor) SignNow(ctx context.Context, siteName string) (*models.SiteAttendanceLog, error) {
	if m == nil || m.db == nil {
		return nil, errors.New("签到服务不可用")
	}
	if _, err := models.NewSiteRepository(m.db).GetSiteByName(siteName); err != nil {
		return nil, fmt.Errorf("站点不存在: %s", siteName)
	}
	now := m.clock.Now()
	day := m.dayOf(now)
	repo := models.NewSiteAttendanceRepository(m.db)
	if err := repo.EnsureDay(models.SiteAttendanceLog{SiteName: siteName, Day: day, ScheduledAt: now.UTC()}); err != nil {
		return nil, err
	}
	return m.attempt(ctx, repo, siteName, day, true)
}

// SignAll 对所有开启了自动签到、当天还没有结果的站点立即签到一次，返回这些站点当天的记录。
func (m *AttendanceMonitor) SignAll(ctx context.Context) ([]models.SiteAttendanceLog, error) {
	return m.SignAllWithProgress(ctx, nil)
}

// SignAllWithProgress 同 SignAll；每处理完一个站点调一次 progress(已处理, 总数)（网页的「一键签到」据此显示进度）。
func (m *AttendanceMonitor) SignAllWithProgress(ctx context.Context, progress func(done, total int)) ([]models.SiteAttendanceLog, error) {
	if m == nil || m.db == nil {
		return nil, errors.New("签到服务不可用")
	}
	sites, err := m.enabledSites()
	if err != nil {
		return nil, err
	}
	day := m.Today()
	repo := models.NewSiteAttendanceRepository(m.db)
	out := make([]models.SiteAttendanceLog, 0, len(sites))
	report := func() {
		if progress != nil {
			progress(len(out), len(sites))
		}
	}
	report()
	for _, site := range sites {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if row, err := repo.GetDay(site.Name, day); err == nil && row.Done() {
			out = append(out, *row)
			report()
			continue
		}
		row, err := m.SignNow(ctx, site.Name)
		if err != nil {
			out = append(out, models.SiteAttendanceLog{SiteName: site.Name, Day: day, Status: models.AttendancePending, LastError: err.Error()})
			report()
			continue
		}
		out = append(out, *row)
		report()
	}
	return out, nil
}

func (m *AttendanceMonitor) enabledSites() ([]models.SiteSetting, error) {
	var sites []models.SiteSetting
	err := m.db.Where("enabled = ? AND attendance_enabled = ?", true, true).Order("name").Find(&sites).Error
	return sites, err
}

// ensureDay 在当天还没有记录时安排一个时刻并建行，返回当天的记录。
func (m *AttendanceMonitor) ensureDay(repo *models.SiteAttendanceRepository, siteName, day string, now time.Time) (*models.SiteAttendanceLog, error) {
	row, err := repo.GetDay(siteName, day)
	if err == nil {
		return row, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err := repo.EnsureDay(models.SiteAttendanceLog{SiteName: siteName, Day: day, ScheduledAt: m.pickTime(now)}); err != nil {
		return nil, err
	}
	return repo.GetDay(siteName, day)
}

// pickTime 在当天的时间窗内随机取一个时刻；已经进入时间窗时在剩下的部分里取，过了时间窗时在接下来 10 分钟内取。
func (m *AttendanceMonitor) pickTime(now time.Time) time.Time {
	startText, endText := "08:00", "10:00"
	if m.window != nil {
		if s, e, err := m.window.AttendanceWindow(); err == nil && s < e {
			startText, endText = s, e
		}
	}
	local := now.In(m.loc)
	start := clockOn(local, startText)
	end := clockOn(local, endText)
	var from, span time.Time
	switch {
	case local.Before(start):
		from, span = start, end
	case local.Before(end):
		from, span = local, end
	default:
		from, span = local, local.Add(attendanceLateSpread)
	}
	offset := time.Duration(m.Rand(int64(span.Sub(from))))
	return from.Add(offset).UTC()
}

func clockOn(day time.Time, hhmm string) time.Time {
	t, err := time.ParseInLocation("15:04", hhmm, day.Location())
	if err != nil {
		return day
	}
	return time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), 0, 0, day.Location())
}

// attempt 持有单站锁签到一次并写结果。manual 为 true 时失败不计入自动重试。
func (m *AttendanceMonitor) attempt(ctx context.Context, repo *models.SiteAttendanceRepository, siteName, day string, manual bool) (*models.SiteAttendanceLog, error) {
	if m.locker != nil {
		release, ok := m.locker.TryAcquireProbeLock(siteName)
		if !ok {
			return nil, ErrAttendanceBusy
		}
		defer release()
	}
	row, err := repo.GetDay(siteName, day)
	if err != nil {
		return nil, err
	}
	if row.Done() && !manual {
		return row, nil
	}

	var (
		result v2.AttendResult
		runErr error
		usage  v2.AttendanceAIUsage
	)
	started := m.clock.Now().UTC()
	site, ok := m.lookupSite(siteName)
	switch {
	case !ok:
		runErr = errors.New("站点没有注册：凭证缺失或站点实例创建失败")
	case !site.SupportsAttendance():
		runErr = v2.ErrAttendanceUnsupported
	default:
		solverCtx, timeout, aiErr := m.withSolver(ctx, siteName)
		attemptCtx, cancel := context.WithTimeout(v2.WithAttendanceAIUsage(solverCtx, &usage), timeout)
		result, runErr = site.Attend(attemptCtx)
		cancel()
		if aiErr != nil && errors.Is(runErr, v2.ErrAttendanceNeedsAI) {
			// 站点说的是哪一关（「要识别验证码」），取不到 AI 的原因说怎么打开
			runErr = fmt.Errorf("%w: %s，%s", v2.ErrAttendanceNeedsAI, sentinelReason(runErr, v2.ErrAttendanceNeedsAI), aiErr.Error())
		}
	}

	now := m.clock.Now().UTC()
	cols := map[string]any{"last_attempt_at": now}
	switch {
	case runErr == nil:
		status := models.AttendanceSigned
		if result.Status == v2.AttendAlready {
			status = models.AttendanceAlready
		}
		cols["status"] = status
		cols["message"] = truncateRunes(result.Message, 200)
		cols["last_error"] = ""
		cols["next_attempt_at"] = nil
		if !manual {
			cols["attempts"] = row.Attempts + 1
		}
	case errors.Is(runErr, v2.ErrAttendanceUnsupported):
		cols["status"] = models.AttendanceUnsupported
		cols["last_error"] = attendanceErrorText(runErr)
		cols["next_attempt_at"] = nil
	case errors.Is(runErr, v2.ErrAttendanceNeedsAI):
		// 要 AI 而没有：不改设置重试也没用，当天直接记失败
		cols["status"] = models.AttendanceFailed
		cols["last_error"] = attendanceErrorText(runErr)
		cols["next_attempt_at"] = nil
		if !manual {
			cols["attempts"] = row.Attempts + 1
		}
	default:
		cols["last_error"] = attendanceErrorText(runErr)
		if !manual {
			attempts := row.Attempts + 1
			cols["attempts"] = attempts
			if attempts >= attendanceMaxAttempts {
				cols["status"] = models.AttendanceFailed
				cols["next_attempt_at"] = nil
			} else {
				cols["next_attempt_at"] = now.Add(attendanceRetryDelays[attempts-1])
			}
		}
	}
	if err := repo.UpdateDay(siteName, day, cols); err != nil {
		return nil, err
	}
	m.recordEvent(repo, siteName, day, manual, started, now, result, runErr, cols, &usage)
	m.logger.Infow("attendance_attempted", "site", siteName, "day", day, "manual", manual,
		"status", cols["status"], "err", cols["last_error"])
	return repo.GetDay(siteName, day)
}

// recordEvent 把这次尝试记进签到事件（签到详情看的）：结果、直接签还是借助 AI、AI 调了几次、用掉多少 token。
// 记不上只写日志，不影响签到结果。
func (m *AttendanceMonitor) recordEvent(repo *models.SiteAttendanceRepository, siteName, day string, manual bool,
	started, finished time.Time, result v2.AttendResult, runErr error, cols map[string]any, usage *v2.AttendanceAIUsage,
) {
	calls, model, prompt, completion := usage.Snapshot()
	ev := models.SiteAttendanceEvent{
		SiteName: siteName, Day: day, Trigger: models.AttendanceTriggerAuto,
		Method: AttendanceMethodOf(siteName), AIModel: model, AICalls: calls,
		PromptTokens: prompt, CompletionTokens: completion,
		DurationMs: finished.Sub(started).Milliseconds(), StartedAt: started, FinishedAt: finished,
	}
	if manual {
		ev.Trigger = models.AttendanceTriggerManual
	}
	status, _ := cols["status"].(string)
	switch {
	case runErr == nil:
		ev.Result = models.AttendanceResultSigned
		if result.Status == v2.AttendAlready {
			ev.Result = models.AttendanceResultAlready
		}
		ev.Message = truncateRunes(result.Message, 200)
	case errors.Is(runErr, v2.ErrAttendanceUnsupported):
		ev.Result = models.AttendanceResultUnsupported
		ev.Error = attendanceErrorText(runErr)
	default:
		ev.Result = models.AttendanceResultRetry
		if manual || status == models.AttendanceFailed {
			ev.Result = models.AttendanceResultFailed
		}
		ev.Error = attendanceErrorText(runErr)
	}
	if err := repo.AddEvent(ev); err != nil {
		m.logger.Warnw("attendance_event_record_failed", "site", siteName, "day", day, "err", err)
	}
}

// AttendanceMethodOf 是站点签到的方式：要过验证码或答题的借助 AI，别的直接签到。
func AttendanceMethodOf(siteName string) string {
	def, _ := v2.GetDefinitionRegistry().Get(siteName)
	switch c := v2.AttendanceChallengeOf(def); {
	case c == "":
		return models.AttendanceMethodDirect
	case c.IsCaptcha():
		return models.AttendanceMethodAICaptcha
	default:
		return models.AttendanceMethodAIQuiz
	}
}

func (m *AttendanceMonitor) lookupSite(siteName string) (v2.AttendanceCapable, bool) {
	if m.sites == nil {
		return nil, false
	}
	site, ok := m.sites.AttendanceSite(siteName)
	if !ok || site == nil {
		return nil, false
	}
	return site, true
}

// attendanceErrorText 把常见的签到错误说成用户能看懂的话。
func attendanceErrorText(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, v2.ErrAttendanceUnsupported):
		msg := err.Error()
		if _, reason, ok := strings.Cut(msg, ": "); ok && reason != "" {
			return reason
		}
		return "该站点不支持自动签到"
	case errors.Is(err, v2.ErrSessionExpired), errors.Is(err, v2.ErrInvalidCredentials):
		// 驱动写了具体原因（Rousi：API Key 没有签到权限）时用它；Cookie 站点照旧提示重新同步 Cookie
		for _, sentinel := range []error{v2.ErrInvalidCredentials, v2.ErrSessionExpired} {
			if reason := sentinelReason(err, sentinel); reason != err.Error() && reason != "" {
				return truncateRunes(reason, 300)
			}
		}
		return "Cookie 已失效或没有权限，请重新同步 Cookie"
	case errors.Is(err, v2.ErrAttendanceNeedsAI):
		return sentinelReason(err, v2.ErrAttendanceNeedsAI)
	case errors.Is(err, v2.ErrAttendanceChallengeFailed):
		return truncateRunes(sentinelReason(err, v2.ErrAttendanceChallengeFailed), 300)
	case errors.Is(err, v2.ErrAttendanceUnrecognized):
		if reason := sentinelReason(err, v2.ErrAttendanceUnrecognized); reason != err.Error() {
			return truncateRunes(reason, 300)
		}
		return "签到页面里没有签到成功或今日已签到的文字"
	case errors.Is(err, context.DeadlineExceeded):
		return "签到请求超时"
	default:
		return truncateRunes(err.Error(), 300)
	}
}

var attendanceStatusLabels = map[string]string{
	models.AttendanceSigned:      "签到成功",
	models.AttendanceAlready:     "今天已签到",
	models.AttendanceFailed:      "签到失败",
	models.AttendanceUnsupported: "不支持自动签到",
	models.AttendancePending:     "待签到",
}

// maybeNotifySummary 在当天所有开启签到的站点都有结果后写一条汇总通知；唯一键保证一天只发一次。
func (m *AttendanceMonitor) maybeNotifySummary(ctx context.Context, repo *models.SiteAttendanceRepository, day string, sites []models.SiteSetting) {
	if !m.notifier.Enabled() || len(sites) == 0 {
		return
	}
	m.mu.Lock()
	done := m.summaryDone == day
	m.mu.Unlock()
	if done {
		return
	}
	// 设置里关掉推送时不发；不记成已发，当天再打开还能补发
	notifyOn, channelIDs := true, []uint(nil)
	if m.notify != nil {
		if on, ids, err := m.notify.AttendanceNotify(); err == nil {
			notifyOn, channelIDs = on, ids
		} else {
			m.logger.Warnw("attendance_notify_settings_failed", "err", err)
		}
	}
	if !notifyOn {
		return
	}
	rows, err := repo.ListDay(day)
	if err != nil {
		m.logger.Warnw("attendance_summary_load_failed", "err", err)
		return
	}
	byName := make(map[string]models.SiteAttendanceLog, len(rows))
	for _, row := range rows {
		byName[row.SiteName] = row
	}
	var b strings.Builder
	items := make([]map[string]any, 0, len(sites))
	signed, failed := 0, 0
	for _, site := range sites {
		row, ok := byName[site.Name]
		if !ok || !row.Done() {
			return
		}
		fmt.Fprintf(&b, "· %s：%s", site.Name, attendanceStatusLabels[row.Status])
		message := ""
		switch {
		case row.Status == models.AttendanceSigned && row.Message != "":
			message = truncateRunes(row.Message, 60)
			fmt.Fprintf(&b, "（%s）", message)
		case row.LastError != "" && row.Status != models.AttendanceSigned && row.Status != models.AttendanceAlready:
			message = truncateRunes(row.LastError, 80)
			fmt.Fprintf(&b, "（%s）", message)
		}
		b.WriteString("\n")
		switch row.Status {
		case models.AttendanceSigned, models.AttendanceAlready:
			signed++
		case models.AttendanceFailed:
			failed++
		}
		items = append(items, map[string]any{
			"site": site.Name, "status": row.Status, "status_label": attendanceStatusLabels[row.Status], "message": message,
		})
	}
	entry := MonitorNotifyEntry{
		Source:   attendanceNotifySource,
		Subject:  "*",
		Kind:     "daily",
		EventKey: day,
		ConfIDs:  channelIDs,
		Title:    fmt.Sprintf("[pt-tools] %s 签到结果", day),
		Text:     strings.TrimRight(b.String(), "\n"),
		Event: notify.NewEvent(notify.EventAttendanceDaily, m.clock.Now(), map[string]any{
			"day": day, "signed": signed, "failed": failed, "items": items,
		}),
	}
	if _, err := m.notifier.Enqueue(ctx, entry); err != nil {
		m.logger.Warnw("attendance_summary_enqueue_failed", "err", err)
		return
	}
	m.mu.Lock()
	m.summaryDone = day
	m.mu.Unlock()
}

// sentinelReason 取错误里哨兵后面的那句说明（「attendance needs AI: 这个站点签到要识别验证码」取后半句）；没有时原样返回。
func sentinelReason(err, sentinel error) string {
	msg := err.Error()
	prefix := sentinel.Error() + ": "
	if i := strings.Index(msg, prefix); i >= 0 {
		return msg[i+len(prefix):]
	}
	return msg
}
