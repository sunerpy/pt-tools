package scheduler

import (
	"context"
	"encoding/json"
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
	loginProbeInterval     = 6 * time.Hour
	loginProbeStartupDelay = 15 * time.Second
	loginReminderTickEvery = 1 * time.Minute
	probeFallbackThreshold = 12 * time.Hour
	probeFailureWarnAfter  = 24 * time.Hour

	tierNone     = "none"
	tierPreWarn  = "pre-warn"
	tier30d      = "30d"
	tier14d      = "14d"
	tier7d       = "7d"
	tier3d       = "3d"
	tier1d       = "1d"
	tierImminent = "banned-imminent"

	ProbeModeAuto     = "auto"
	ProbeModeManual   = "manual"
	ProbeModeDisabled = "disabled"
)

// QuietHours represents the system-wide quiet window for non-critical
// notifications. Time strings use 24h "HH:MM" format. Empty start/end
// disables the window. The banned-imminent tier overrides quiet hours.
type QuietHours struct {
	Start string
	End   string
}

// SiteResolver wires a SiteSetting row into a v2.Site instance plus its
// SiteDefinition. It is injected so that tests can substitute fake sites
// without touching the global site registry or HTTP layer.
type SiteResolver interface {
	Resolve(setting models.SiteSetting) (*v2.SiteDefinition, v2.Site, error)
}

// CredentialDecryptor returns a usable cookie/api-key for a site. Real
// implementations decrypt SiteSetting.CookieEncrypted via core.ConfigStore;
// tests inject deterministic values.
type CredentialDecryptor interface {
	Decrypt(setting models.SiteSetting) (cookie string, err error)
}

// LoginReminderMonitor runs two cooperating loops:
//
//  1. probe loop (every 6h, per-site jitter): calls sitelogin.Probe() and
//     persists last_login/last_access into models.SiteLoginState. On
//     consecutive failure it backs off and after 24h emits a "cookie may
//     have expired" warning notification.
//
//  2. reminder loop (every minute): for each enabled site whose effective
//     last-active time is within (BanThresholdDays - RemindBeforeDays) of
//     today, it computes a tier and fires a notification through
//     notify.Router. Dedup is window-based (see CronExpr.WindowStart):
//     within the same cron window only one reminder is sent, but a tier
//     escalation immediately re-fires regardless of window.
//
// The monitor never issues raw HTTP itself; all site I/O passes through the
// site/v2 driver layer (which inherits the project's circuit breaker and
// rate limiter).
type LoginReminderMonitor struct {
	mu                   sync.Mutex
	ctx                  context.Context
	cancel               context.CancelFunc
	wg                   sync.WaitGroup
	running              bool
	db                   *gorm.DB
	router               *notify.Router
	resolver             SiteResolver
	decryptor            CredentialDecryptor
	clock                sitelogin.Clock
	logger               *zap.SugaredLogger
	quietHours           QuietHours
	probeEvery           time.Duration
	reminderTick         time.Duration
	migrationCompletedAt time.Time

	// probeLockMu guards probeLocks (the registry of per-site mutexes). The
	// per-site mutex itself protects the actual probe execution. R23: a single
	// shared map across cron + REST + extension push paths so that no two
	// trigger sources can probe the same site concurrently.
	probeLockMu sync.Mutex
	probeLocks  map[string]*probeSlot
}

// probeSlot wraps a per-site mutex with a TryLock-style flag. We do not use
// sync.Mutex.TryLock directly because Go's runtime explicitly discourages it
// for this kind of contention pattern; an explicit boolean flag plus a tiny
// guard mutex gives deterministic, race-free behavior under -race.
type probeSlot struct {
	mu    sync.Mutex
	inUse bool
}

// LoginReminderConfig holds the dependencies needed to construct a
// LoginReminderMonitor. All fields are required except QuietHours, which
// defaults to "no quiet window".
type LoginReminderConfig struct {
	DB           *gorm.DB
	Router       *notify.Router
	Resolver     SiteResolver
	Decryptor    CredentialDecryptor
	Clock        sitelogin.Clock
	Logger       *zap.SugaredLogger
	QuietHours   QuietHours
	ProbeEvery   time.Duration
	ReminderTick time.Duration
}

// NewLoginReminderMonitor builds a LoginReminderMonitor. It does not start
// the loops; call Start to begin processing.
func NewLoginReminderMonitor(cfg LoginReminderConfig) *LoginReminderMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	if cfg.Clock == nil {
		cfg.Clock = sitelogin.NewRealClock()
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop().Sugar()
	}
	if cfg.ProbeEvery == 0 {
		cfg.ProbeEvery = loginProbeInterval
	}
	if cfg.ReminderTick == 0 {
		cfg.ReminderTick = loginReminderTickEvery
	}
	return &LoginReminderMonitor{
		ctx:          ctx,
		cancel:       cancel,
		db:           cfg.DB,
		router:       cfg.Router,
		resolver:     cfg.Resolver,
		decryptor:    cfg.Decryptor,
		clock:        cfg.Clock,
		logger:       cfg.Logger,
		quietHours:   cfg.QuietHours,
		probeEvery:   cfg.ProbeEvery,
		reminderTick: cfg.ReminderTick,
		probeLocks:   make(map[string]*probeSlot),
	}
}

// Start launches the probe and reminder loops. Calling Start twice is a
// no-op.
func (m *LoginReminderMonitor) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

	m.wg.Add(2)
	go m.probeLoop()
	go m.reminderLoop()
}

// Stop signals both loops to exit and waits for them to drain.
func (m *LoginReminderMonitor) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	m.mu.Unlock()
	m.cancel()
	m.wg.Wait()
}

// TryAcquireProbeLock attempts to acquire the per-site probe mutex without
// blocking. Returns (release, true) on success — caller MUST invoke release()
// in defer to free the slot. Returns (nil, false) if another caller (cron,
// REST, or extension) already holds the mutex; the caller should surface a
// 409-style "probe in progress" response. R23: this single map is the shared
// single-flight registry across all probe trigger sources.
func (m *LoginReminderMonitor) TryAcquireProbeLock(siteName string) (release func(), ok bool) {
	slot := m.getProbeSlot(siteName)
	slot.mu.Lock()
	if slot.inUse {
		slot.mu.Unlock()
		return nil, false
	}
	slot.inUse = true
	slot.mu.Unlock()
	return func() {
		slot.mu.Lock()
		slot.inUse = false
		slot.mu.Unlock()
	}, true
}

func (m *LoginReminderMonitor) getProbeSlot(siteName string) *probeSlot {
	m.probeLockMu.Lock()
	defer m.probeLockMu.Unlock()
	if m.probeLocks == nil {
		m.probeLocks = make(map[string]*probeSlot)
	}
	slot, ok := m.probeLocks[siteName]
	if !ok {
		slot = &probeSlot{}
		m.probeLocks[siteName] = slot
	}
	return slot
}

func (m *LoginReminderMonitor) probeLoop() {
	defer m.wg.Done()
	// 延迟初探：避免重启后等整个周期，同时让 DB / 站点注册表先就绪。
	select {
	case <-m.ctx.Done():
		return
	case <-time.After(loginProbeStartupDelay):
		m.RunProbeOnce(m.ctx)
	}
	ticker := time.NewTicker(m.probeEvery)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.RunProbeOnce(m.ctx)
		}
	}
}

func (m *LoginReminderMonitor) reminderLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.reminderTick)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.RunReminderOnce(m.ctx)
		}
	}
}

// RunProbeOnce iterates enabled sites, probes each, and persists the
// result. It is exported so tests can drive the probe phase directly with
// a fake clock. This is cron-driven (manualTrigger=false). Sites whose
// probe slot is held by another trigger source (manual REST / extension
// push) are skipped this iteration with a debug log — R23 shared
// single-flight.
func (m *LoginReminderMonitor) RunProbeOnce(ctx context.Context) {
	if m.db == nil {
		return
	}
	sites, err := m.listEnabledSites()
	if err != nil {
		m.logger.Warnw("login_probe_list_sites_failed", "err", err)
		return
	}
	for _, site := range sites {
		select {
		case <-ctx.Done():
			return
		default:
		}
		release, ok := m.TryAcquireProbeLock(site.Name)
		if !ok {
			m.logger.Debugw("probe_skipped_singleflight_busy", "site", site.Name, "trigger", "cron")
			continue
		}
		m.probeSiteInternal(ctx, site, false)
		release()
	}
}

// RunProbeOnceForSite probes a single site by name. Used for manual endpoint
// triggering (manualTrigger=true), bypassing mode restrictions. Acquires
// the shared probe lock; returns false if another caller already holds it.
func (m *LoginReminderMonitor) RunProbeOnceForSite(ctx context.Context, siteName string) bool {
	if m.db == nil {
		return false
	}
	release, ok := m.TryAcquireProbeLock(siteName)
	if !ok {
		m.logger.Infow("probe_rejected_singleflight_busy", "site", siteName, "trigger", "manual")
		return false
	}
	defer release()
	m.RunProbeOnceForSiteLocked(ctx, siteName)
	return true
}

// RunProbeOnceForSiteLocked runs the probe assuming the caller already holds
// the per-site probe lock (via TryAcquireProbeLock). The web handler uses this
// split so the HTTP layer can shape its 409 response from TryAcquireProbeLock
// before dispatching the actual probe work.
func (m *LoginReminderMonitor) RunProbeOnceForSiteLocked(ctx context.Context, siteName string) {
	if m.db == nil {
		return
	}
	repo := models.NewSiteRepository(m.db)
	site, err := repo.GetSiteByName(siteName)
	if err != nil {
		m.logger.Warnw("login_probe_get_site_failed", "site", siteName, "err", err)
		return
	}
	m.probeSiteInternal(ctx, *site, true)
}

func (m *LoginReminderMonitor) probeSite(ctx context.Context, setting models.SiteSetting) {
	m.probeSiteInternal(ctx, setting, false)
}

func (m *LoginReminderMonitor) probeSiteInternal(ctx context.Context, setting models.SiteSetting, manualTrigger bool) {
	state, err := m.loadOrInitState(setting.Name)
	if err != nil {
		m.logger.Warnw("login_probe_load_state_failed", "site", setting.Name, "err", err)
		return
	}

	// ProbeMode check: respect mode unless manual trigger
	if !manualTrigger {
		mode := state.ProbeMode
		if mode == "" {
			mode = ProbeModeAuto
		}
		switch mode {
		case ProbeModeDisabled:
			m.logger.Debugw("probe_skipped_mode_disabled", "site", setting.Name)
			return
		case ProbeModeManual:
			m.logger.Debugw("probe_skipped_mode_manual", "site", setting.Name)
			return
		case ProbeModeAuto:
			// proceed normally
		default:
			m.logger.Warnw("probe_unknown_mode", "site", setting.Name, "mode", mode)
			return
		}
	}

	if state.ProbeJitterSeconds == 0 {
		state.ProbeJitterSeconds = rand.Intn(int(m.probeEvery / time.Second))
	}

	def, site, err := m.resolver.Resolve(setting)
	if err != nil || def == nil || site == nil {
		m.logger.Warnw("login_probe_resolve_failed", "site", setting.Name, "err", err)
		return
	}
	defer site.Close()

	result, _ := sitelogin.Probe(ctx, def, site, m.clock)
	if result == nil {
		result = &sitelogin.ProbeResult{Status: sitelogin.UNKNOWN, Diagnostic: "nil result"}
	}
	now := m.clock.Now()
	state.LastProbeAt = &now
	state.LastProbeStatus = string(result.Status)
	if result.RawError != nil {
		state.LastProbeError = result.RawError.Error()
	} else {
		state.LastProbeError = ""
	}

	if result.Status == sitelogin.OK {
		dispatchProbeTimestamps(state, result)
		state.LastConsistencyCheck = sitelogin.CheckConsistency(state.ApiLastLoginAt, state.CookieLastLoginAt)
		state.ConsecutiveProbeFailures = 0
	} else {
		state.ConsecutiveProbeFailures++
	}

	if err := m.saveState(state); err != nil {
		m.logger.Warnw("login_probe_save_failed", "site", setting.Name, "err", err)
		return
	}

	effective := EffectiveLastActive(state, now)
	var daysRemainingLog any
	tier := "unknown"
	if !effective.IsZero() {
		daysRemaining := DaysRemaining(state, effective, now)
		daysRemainingLog = daysRemaining
		tier = ComputeTier(state, effective, now)
	}
	m.logger.Infow(
		"login_probe_state_updated",
		"site", setting.Name,
		"status", string(result.Status),
		"source", string(result.Source),
		"last_access_at", timeForLog(state.LastAccessAt),
		"api_last_login_at", timeForLog(state.ApiLastLoginAt),
		"cookie_last_login_at", timeForLog(state.CookieLastLoginAt),
		"effective_last_active_at", timeForLogValue(effective),
		"effective_source", effectiveSourceForLog(state, effective),
		"days_remaining", daysRemainingLog,
		"tier", tier,
		"consecutive_probe_failures", state.ConsecutiveProbeFailures,
	)

	m.maybeNotifyProbeFailure(ctx, setting, state, result)
}

// dispatchProbeTimestamps writes the probe-derived last-login timestamp into
// the appropriate column based on which auth path produced it. last-access
// always goes to LastAccessAt regardless of source. The legacy LastLoginAt
// column is kept in sync for v1 read-path compatibility (max(api, cookie)).
func dispatchProbeTimestamps(state *models.SiteLoginState, result *sitelogin.ProbeResult) {
	if result.LastAccessAt != nil {
		t := result.LastAccessAt.UTC()
		state.LastAccessAt = &t
	}
	if result.LastLoginAt != nil {
		t := result.LastLoginAt.UTC()
		switch result.Source {
		case sitelogin.ProbeSourceHTTPAPIKey:
			state.ApiLastLoginAt = &t
		case sitelogin.ProbeSourceHTTPCookie, sitelogin.ProbeSourceCloak:
			state.CookieLastLoginAt = &t
		default:
			state.CookieLastLoginAt = &t
		}
		state.LastLoginAt = &t
	}
}

func timeForLog(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func timeForLogValue(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func effectiveSourceForLog(state *models.SiteLoginState, effective time.Time) string {
	if effective.IsZero() || state == nil {
		return "none"
	}
	if sameInstant(state.LastAccessAt, effective) {
		return "last_access"
	}
	if sameInstant(state.ApiLastLoginAt, effective) {
		return "api_last_login"
	}
	if sameInstant(state.CookieLastLoginAt, effective) {
		return "cookie_last_login"
	}
	if sameInstant(state.LastLoginAt, effective) {
		return "last_login"
	}
	if sameInstant(state.LastVisitAt, effective) {
		return "last_visit"
	}
	return "unknown"
}

func sameInstant(candidate *time.Time, effective time.Time) bool {
	return candidate != nil && candidate.Equal(effective)
}

func (m *LoginReminderMonitor) maybeNotifyProbeFailure(ctx context.Context, setting models.SiteSetting, state *models.SiteLoginState, result *sitelogin.ProbeResult) {
	if result.Status == sitelogin.OK {
		return
	}
	if result.Status != sitelogin.SESSION_EXPIRED {
		if state.LastProbeAt == nil {
			return
		}
		if m.clock.Now().Sub(*state.LastProbeAt) < probeFailureWarnAfter {
			return
		}
	}
	if m.router == nil {
		return
	}
	channelIDs := decodeChannelIDs(state.NotificationChannelIDs)
	notification := notify.Notification{
		Title: fmt.Sprintf("[pt-tools] 站点 %s 探测失败", setting.Name),
		Text:  fmt.Sprintf("状态：%s。请检查 cookie / API key 是否仍然有效。", result.Status),
	}
	scope := notify.RouteScope{ConfIDs: channelIDs, EventType: "site_login_probe_failure", PrimaryID: setting.Name}
	if err := m.router.Route(ctx, notification, scope); err != nil {
		m.logger.Warnw("login_probe_failure_route_failed", "site", setting.Name, "err", err)
	}
}

// RunReminderOnce iterates enabled sites, computes each site's tier, and
// fires reminders that are due. Exported for testability.
func (m *LoginReminderMonitor) RunReminderOnce(ctx context.Context) {
	if m.db == nil {
		return
	}
	sites, err := m.listEnabledSites()
	if err != nil {
		m.logger.Warnw("login_reminder_list_sites_failed", "err", err)
		return
	}
	now := m.clock.Now()
	m.refreshMigrationCompletedAt()
	for _, setting := range sites {
		select {
		case <-ctx.Done():
			return
		default:
		}
		m.evaluateReminder(ctx, setting, now)
	}
}

// SendTestReminder immediately sends a test login-reminder notification for one site.
func (m *LoginReminderMonitor) SendTestReminder(ctx context.Context, siteName string) error {
	if m.db == nil {
		return errors.New("数据库未初始化")
	}
	repo := models.NewSiteRepository(m.db)
	site, err := repo.GetSiteByName(siteName)
	if err != nil {
		return fmt.Errorf("站点 %s 不存在: %w", siteName, err)
	}
	state, err := m.loadOrInitState(site.Name)
	if err != nil {
		return fmt.Errorf("加载登录状态失败: %w", err)
	}
	now := m.clock.Now()
	effective := EffectiveLastActive(state, now)
	daysRemaining := DaysRemaining(state, effective, now)
	tier := ComputeTier(state, effective, now)
	if m.router == nil {
		return errors.New("通知路由未初始化")
	}
	effectiveStr := "未知"
	if !effective.IsZero() {
		effectiveStr = effective.UTC().Format(time.RFC3339)
	}
	notification := notify.Notification{
		Title: fmt.Sprintf("[pt-tools][测试] 站点 %s 登录提醒", site.Name),
		Text:  fmt.Sprintf("这是一条测试提醒。当前判定活跃 %s，剩余 %d 天 (tier=%s)。若你收到此消息，说明该站点的通知通道配置正常。", effectiveStr, daysRemaining, tier),
	}
	scope := notify.RouteScope{
		ConfIDs:    decodeChannelIDs(state.NotificationChannelIDs),
		EventType:  "site_login_reminder_test",
		PrimaryID:  site.Name,
		SkipDedupe: true,
	}
	return m.router.Route(ctx, notification, scope)
}

func (m *LoginReminderMonitor) evaluateReminder(ctx context.Context, setting models.SiteSetting, now time.Time) {
	state, err := m.loadOrInitState(setting.Name)
	if err != nil {
		m.logger.Warnw("login_reminder_load_state_failed", "site", setting.Name, "err", err)
		return
	}
	cron := state.ReminderCron
	if cron == "" {
		cron = "0 10,22 * * *"
	}
	expr, err := ParseCron(cron)
	if err != nil {
		m.logger.Warnw("login_reminder_invalid_cron", "site", setting.Name, "cron", cron, "err", err)
		return
	}
	effective := EffectiveLastActive(state, now)
	if effective.IsZero() {
		return
	}
	tier := ComputeTier(state, effective, now)
	if tier == tierNone {
		return
	}
	if m.inMigrationSilenceWindow(now) && tier != tierImminent {
		m.logger.Debugw("login_reminder_migration_silence_skip", "site", setting.Name, "tier", tier)
		return
	}

	tierEscalated := tierIsHigher(tier, state.LastReminderTier)
	if !tierEscalated {
		if !expr.Match(now) {
			m.logger.Debugw("login_reminder_window_skip", "site", setting.Name, "tier", tier, "reason", "cron_not_matched")
			return
		}
		windowStart := expr.WindowStart(now)
		if state.LastReminderSentAt != nil && !state.LastReminderSentAt.Before(windowStart) {
			m.logger.Debugw("login_reminder_window_skip", "site", setting.Name, "tier", tier, "reason", "already_sent_in_window")
			return
		}
	}

	if !tierAllowsImmediateOverride(tier) && notify.IsQuietNow(now, m.quietHours.Start, m.quietHours.End) {
		m.logger.Debugw("login_reminder_quiet_skip", "site", setting.Name, "tier", tier)
		return
	}

	channelIDs := decodeChannelIDs(state.NotificationChannelIDs)
	daysRemaining := DaysRemaining(state, effective, now)
	daysSinceActive := int(now.Sub(effective) / (24 * time.Hour))
	notification := notify.Notification{
		Title: fmt.Sprintf("[pt-tools] 站点 %s 即将被封号", setting.Name),
		Text:  fmt.Sprintf("上次活跃 %s（%d 天前）；建议访问站点并浏览页面以刷新活跃时间（多数站点仅靠登录不更新 last_access）。剩余 %d 天 (tier=%s)。", effective.Format(time.RFC3339), daysSinceActive, daysRemaining, tier),
	}
	scope := notify.RouteScope{ConfIDs: channelIDs, EventType: "site_login_reminder", PrimaryID: fmt.Sprintf("%s/%s", setting.Name, tier)}

	m.logger.Infow("login_reminder_route_started",
		"site", setting.Name, "tier", tier,
		"days_remaining", daysRemaining, "escalated", tierEscalated,
		"channels", len(channelIDs))

	if m.router != nil {
		if err := m.router.Route(ctx, notification, scope); err != nil {
			m.logger.Warnw("login_reminder_route_failed", "site", setting.Name, "err", err)
			return
		}
		m.logger.Infow("login_reminder_route_succeeded", "site", setting.Name, "tier", tier)
	} else {
		m.logger.Debugw("login_reminder_router_nil_proceed", "site", setting.Name, "tier", tier)
	}

	previousTier := state.LastReminderTier
	state.LastReminderTier = tier
	t := now
	state.LastReminderSentAt = &t
	if saveErr := m.saveState(state); saveErr != nil {
		m.logger.Warnw("login_reminder_save_failed", "site", setting.Name, "err", saveErr)
	}

	if tierEscalated {
		m.logger.Infow("login_reminder_tier_escalated", "site", setting.Name, "tier", tier, "previous", previousTier)
	} else {
		m.logger.Infow("login_reminder_fired", "site", setting.Name, "tier", tier)
	}
}

// EffectiveLastActive returns the most recent confirmed activity timestamp for a site.
//
// Per research: site cleanup.php scripts predominantly check `last_access` (NexusPHP)
// / `last_action` (Unit3D) / `LastAccess` (Gazelle) / `lastModifiedDate` (mTorrent)
// — NOT `last_login`. So v2 prefers last_access-class timestamps and treats
// last_login-class timestamps (ApiLastLoginAt, CookieLastLoginAt) as
// supplementary signals, used only when last_access is missing.
//
// Precedence:
//  1. state.LastAccessAt  (NexusPHP last_access; primary)
//  2. max(state.ApiLastLoginAt, state.CookieLastLoginAt)  (R-Q-A5: take newer of dual timestamps)
//  3. state.LastLoginAt   (legacy v1 field)
//  4. state.LastVisitAt   (extension visit; only when probes failing >12h)
//  5. zero value          (no signal yet)
func EffectiveLastActive(state *models.SiteLoginState, now time.Time) time.Time {
	if state == nil {
		return time.Time{}
	}
	if state.LastAccessAt != nil {
		return *state.LastAccessAt
	}
	lastLoginCandidate := newest(state.ApiLastLoginAt, state.CookieLastLoginAt)
	if !lastLoginCandidate.IsZero() {
		return lastLoginCandidate
	}
	if state.LastLoginAt != nil {
		return *state.LastLoginAt
	}
	probeStale := state.ConsecutiveProbeFailures > 0 &&
		state.LastProbeAt != nil &&
		now.Sub(*state.LastProbeAt) >= probeFallbackThreshold
	if probeStale && state.LastVisitAt != nil {
		return *state.LastVisitAt
	}
	return time.Time{}
}

// DaysRemaining returns how many full days remain until the site's
// configured ban threshold elapses since the effective last-active time.
// Negative values mean the threshold has already been crossed.
func DaysRemaining(state *models.SiteLoginState, effective, now time.Time) int {
	threshold := state.BanThresholdDays
	if threshold <= 0 {
		threshold = 30
	}
	deadline := effective.Add(time.Duration(threshold) * 24 * time.Hour)
	diff := deadline.Sub(now)
	return int(diff / (24 * time.Hour))
}

// ComputeTier maps DaysRemaining into one of the predefined tier strings.
// The tier defines both the reminder urgency and whether quiet hours can
// be overridden (only banned-imminent overrides).
func ComputeTier(state *models.SiteLoginState, effective, now time.Time) string {
	remaining := DaysRemaining(state, effective, now)
	preWarn := state.RemindBeforeDays
	if preWarn <= 0 {
		preWarn = 10
	}
	if remaining > preWarn {
		return tierNone
	}
	switch {
	case remaining <= 1:
		return tierImminent
	case remaining <= 3:
		return tier3d
	case remaining <= 7:
		return tier7d
	case remaining <= 14:
		return tier14d
	default:
		return tier30d
	}
}

func tierAllowsImmediateOverride(tier string) bool {
	return tier == tierImminent
}

func (m *LoginReminderMonitor) refreshMigrationCompletedAt() {
	completedAt, ok := models.GetLatestMigrationCompletedAt(m.db)
	if !ok {
		m.migrationCompletedAt = time.Time{}
		return
	}
	m.migrationCompletedAt = completedAt
}

func (m *LoginReminderMonitor) inMigrationSilenceWindow(now time.Time) bool {
	return !m.migrationCompletedAt.IsZero() && now.Sub(m.migrationCompletedAt) < 24*time.Hour
}

var tierOrder = map[string]int{
	tierNone:     0,
	tierPreWarn:  1,
	tier30d:      2,
	tier14d:      3,
	tier7d:       4,
	tier3d:       5,
	tier1d:       6,
	tierImminent: 7,
}

func tierIsHigher(current, previous string) bool {
	return tierOrder[current] > tierOrder[previous]
}

func decodeChannelIDs(raw string) []uint {
	if raw == "" {
		return nil
	}
	var out []uint
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func newest(a, b *time.Time) time.Time {
	if a == nil && b == nil {
		return time.Time{}
	}
	if a == nil {
		return *b
	}
	if b == nil {
		return *a
	}
	if a.After(*b) {
		return *a
	}
	return *b
}

func (m *LoginReminderMonitor) listEnabledSites() ([]models.SiteSetting, error) {
	repo := models.NewSiteRepository(m.db)
	return repo.ListEnabledSites()
}

func (m *LoginReminderMonitor) loadOrInitState(siteName string) (*models.SiteLoginState, error) {
	repo := models.NewSiteLoginStateRepository(m.db)
	state, err := repo.GetLoginState(siteName)
	if err == nil {
		return state, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) &&
		!strings.Contains(err.Error(), "record not found") &&
		!strings.Contains(err.Error(), "登录状态不存在") {
		return nil, err
	}
	banDays, remindDays, _ := models.ApplyPresetIfMissing(siteName)
	fresh := &models.SiteLoginState{
		SiteName:         siteName,
		BanThresholdDays: banDays,
		RemindBeforeDays: remindDays,
		ReminderCron:     "0 10,22 * * *",
		LastReminderTier: tierNone,
		ProbeMode:        "auto",
	}
	if err := m.db.Create(fresh).Error; err != nil {
		return nil, err
	}
	return fresh, nil
}

func (m *LoginReminderMonitor) saveState(state *models.SiteLoginState) error {
	return m.db.Save(state).Error
}
