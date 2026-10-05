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

	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

const (
	attendanceTick           = time.Minute
	attendanceStartupDelay   = 20 * time.Second
	attendanceAttemptTimeout = 60 * time.Second
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
	Clock    sitelogin.Clock
	Logger   *zap.SugaredLogger
	Tick     time.Duration
	// Location 是计算日期与时间窗的时区，默认进程时区（time.Local）。
	Location *time.Location
	// Rand 返回 [0, n) 的随机数，用于在时间窗内取时刻；测试可以注入确定的值。
	Rand func(n int64) int64
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
		clock:    cfg.Clock,
		logger:   cfg.Logger,
		tick:     cfg.Tick,
		loc:      cfg.Location,
		Rand:     cfg.Rand,
	}
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
	for _, site := range sites {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if row, err := repo.GetDay(site.Name, day); err == nil && row.Done() {
			out = append(out, *row)
			continue
		}
		row, err := m.SignNow(ctx, site.Name)
		if err != nil {
			out = append(out, models.SiteAttendanceLog{SiteName: site.Name, Day: day, Status: models.AttendancePending, LastError: err.Error()})
			continue
		}
		out = append(out, *row)
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
	)
	site, ok := m.lookupSite(siteName)
	switch {
	case !ok:
		runErr = errors.New("站点没有注册：凭证缺失或站点实例创建失败")
	case !site.SupportsAttendance():
		runErr = v2.ErrAttendanceUnsupported
	default:
		attemptCtx, cancel := context.WithTimeout(ctx, attendanceAttemptTimeout)
		result, runErr = site.Attend(attemptCtx)
		cancel()
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
	m.logger.Infow("attendance_attempted", "site", siteName, "day", day, "manual", manual,
		"status", cols["status"], "err", cols["last_error"])
	return repo.GetDay(siteName, day)
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
		return "Cookie 已失效或没有权限，请重新同步 Cookie"
	case errors.Is(err, v2.ErrAttendanceUnrecognized):
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
	for _, site := range sites {
		row, ok := byName[site.Name]
		if !ok || !row.Done() {
			return
		}
		fmt.Fprintf(&b, "· %s：%s", site.Name, attendanceStatusLabels[row.Status])
		switch {
		case row.Status == models.AttendanceSigned && row.Message != "":
			fmt.Fprintf(&b, "（%s）", truncateRunes(row.Message, 60))
		case row.LastError != "" && row.Status != models.AttendanceSigned && row.Status != models.AttendanceAlready:
			fmt.Fprintf(&b, "（%s）", truncateRunes(row.LastError, 80))
		}
		b.WriteString("\n")
	}
	entry := MonitorNotifyEntry{
		Source:   attendanceNotifySource,
		Subject:  "*",
		Kind:     "daily",
		EventKey: day,
		Title:    fmt.Sprintf("[pt-tools] %s 签到结果", day),
		Text:     strings.TrimRight(b.String(), "\n"),
	}
	if _, err := m.notifier.Enqueue(ctx, entry); err != nil {
		m.logger.Warnw("attendance_summary_enqueue_failed", "err", err)
		return
	}
	m.mu.Lock()
	m.summaryDone = day
	m.mu.Unlock()
}
