package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

const (
	loginProbeInterval     = 6 * time.Hour
	loginProbeStartupDelay = 15 * time.Second
	loginProbeTick         = time.Minute
	loginReminderTickEvery = 1 * time.Minute

	// 单次探测的总预算与 HTTP 主通道的上限；剩余部分留给 CloakBrowser 后备。
	loginProbeBudget         = 150 * time.Second
	loginProbePrimaryTimeout = 60 * time.Second

	// 非凭证类失败持续满这么久才提醒一次，之后每隔这么久至多一次。
	failureNoticeAfter = 24 * time.Hour
	// 探测成功但站点的 last_access 超过这么久没有前进，记为「访问未生效」。
	accessStaleAfter = 48 * time.Hour
	// 凭证类失败（会话过期、密钥错误）后隔这么久再探，失效的凭证不必每 6 小时请求一次站点。
	credentialRetryDelay = 24 * time.Hour
	// 动态站点（没有内置定义）隔这么久复查一次，期间不发请求。
	unsupportedRecheckDelay = 24 * time.Hour

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

	loginNotifySource = "login"
)

// transientRetryDelays 是非凭证类失败连续第 1、2、3 次之后的重试间隔，再往后回到正常间隔。
var transientRetryDelays = []time.Duration{time.Hour, 2 * time.Hour, 4 * time.Hour}

// probeStatusLabels 与前端 web/frontend/src/utils/probeStatus.ts 的文案一致。
var probeStatusLabels = map[sitelogin.ProbeStatus]string{
	sitelogin.OK:              "正常",
	sitelogin.SESSION_EXPIRED: "会话已过期",
	sitelogin.KEY_ERROR:       "密钥错误",
	sitelogin.CHALLENGE:       "被反爬拦截",
	sitelogin.RATE_LIMITED:    "请求过于频繁",
	sitelogin.NETWORK_ERROR:   "网络错误",
	sitelogin.PARSE_ERROR:     "解析失败",
	sitelogin.UNKNOWN:         "未知状态",
	sitelogin.NOT_CONFIGURED:  "未配置凭证",
	sitelogin.UNSUPPORTED:     "暂不支持探测",
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
//  1. probe loop（每分钟检查一次）：逐个探测到期的 auto 模式站点。到期条件是
//     NextProbeAt 已到，或 ProbeRequestedAt 晚于 LastProbeStartedAt。下一次探测
//     时间按结果退避（见 nextProbeDelay），首轮按每站固定抖动分散，重启不补探。
//
//  2. reminder loop（每分钟）：计算封号提醒等级，并判断探测失败与恢复提醒。
//     所有通知只写 MonitorNotificationLog，由 MonitorNotifier 按通道处理静默时段、
//     经 live 通道发送并重试；监控自己不创建通道实例。
//
// SiteLoginState 的每一列只有一个写者，全部按列 Updates，不整行 Save：
//
//	探测（持有单站锁）：last_probe_at、last_probe_started_at、last_probe_status、
//	  last_probe_error、consecutive_probe_failures、first_failure_at、last_success_at、
//	  next_probe_at、probe_jitter_seconds、last_access_at、last_login_at、
//	  api_last_login_at、cookie_last_login_at、last_consistency_check、access_stale_since
//	提醒循环：last_reminder_tier、last_reminder_sent_at、last_failure_notified_at
//	配置接口：ban_threshold_days、remind_before_days、reminder_cron、
//	  notification_channel_ids、probe_mode
//	访问上报：last_visit_at
//	探测请求（RequestProbe）：probe_requested_at
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
	notifier             *MonitorNotifier
	userInfo             sitelogin.UserInfoFetcher
	resolver             SiteResolver
	decryptor            CredentialDecryptor
	clock                sitelogin.Clock
	logger               *zap.SugaredLogger
	probeEvery           time.Duration
	probeTick            time.Duration
	reminderTick         time.Duration
	probeBudget          time.Duration
	primaryTimeout       time.Duration
	lookupDefinition     func(siteName string) (*v2.SiteDefinition, bool)
	migrationCompletedAt time.Time
	testSeq              atomic.Int64

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
// LoginReminderMonitor.
type LoginReminderConfig struct {
	DB *gorm.DB
	// Notifier 为空时只记录提醒决策，不投递（没有可用的通知服务）。
	Notifier *MonitorNotifier
	// UserInfo 非空时探测经 UserInfoService.FetchAndSave 进行：用与搜索共用的站点实例和限速器，
	// 成功时顺带刷新用户统计，也不再调用 Resolver。为空（仓库初始化失败）时退回 Resolver 新建实例。
	UserInfo     sitelogin.UserInfoFetcher
	Resolver     SiteResolver
	Decryptor    CredentialDecryptor
	Clock        sitelogin.Clock
	Logger       *zap.SugaredLogger
	ProbeEvery   time.Duration
	ProbeTick    time.Duration
	ReminderTick time.Duration
	// DefinitionLookup 判断站点是否有内置定义，默认查 v2.GetDefinitionRegistry()。
	DefinitionLookup func(siteName string) (*v2.SiteDefinition, bool)
}

func defaultDefinitionLookup(siteName string) (*v2.SiteDefinition, bool) {
	return v2.GetDefinitionRegistry().Get(siteName)
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
	if cfg.ProbeTick == 0 {
		cfg.ProbeTick = loginProbeTick
	}
	if cfg.ReminderTick == 0 {
		cfg.ReminderTick = loginReminderTickEvery
	}
	if cfg.DefinitionLookup == nil {
		cfg.DefinitionLookup = defaultDefinitionLookup
	}
	return &LoginReminderMonitor{
		ctx:              ctx,
		cancel:           cancel,
		db:               cfg.DB,
		notifier:         cfg.Notifier,
		userInfo:         cfg.UserInfo,
		resolver:         cfg.Resolver,
		decryptor:        cfg.Decryptor,
		clock:            cfg.Clock,
		logger:           cfg.Logger,
		probeEvery:       cfg.ProbeEvery,
		probeTick:        cfg.ProbeTick,
		reminderTick:     cfg.ReminderTick,
		probeBudget:      loginProbeBudget,
		primaryTimeout:   loginProbePrimaryTimeout,
		lookupDefinition: cfg.DefinitionLookup,
		probeLocks:       make(map[string]*probeSlot),
	}
}

// Notifier 返回监控使用的通知投递器（可能为 nil）。
func (m *LoginReminderMonitor) Notifier() *MonitorNotifier {
	if m == nil {
		return nil
	}
	return m.notifier
}

// Start launches the probe and reminder loops and the notification delivery
// loop. Calling Start twice is a no-op.
func (m *LoginReminderMonitor) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

	m.notifier.Start()
	m.wg.Add(2)
	go m.probeLoop()
	go m.reminderLoop()
}

// Stop signals the loops to exit and waits for them to drain.
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
	m.notifier.Stop()
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
	// 延迟首轮检查，让 DB / 站点注册表先就绪。首轮只探测已经到期的站点，不会整轮补探。
	select {
	case <-m.ctx.Done():
		return
	case <-time.After(loginProbeStartupDelay):
		m.RunDueProbes(m.ctx)
	}
	tick := m.probeTick
	if tick <= 0 {
		tick = loginProbeTick
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.RunDueProbes(m.ctx)
		}
	}
}

func (m *LoginReminderMonitor) reminderLoop() {
	defer m.wg.Done()
	tick := m.reminderTick
	if tick <= 0 {
		tick = loginReminderTickEvery
	}
	ticker := time.NewTicker(tick)
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

// RunDueProbes 逐个探测到期的 auto 模式站点。NextProbeAt 为空（升级后的老数据、新建的行）
// 时只安排首轮时间「当前时间 + 固定抖动」，不立即探测，除非有人请求过探测。
func (m *LoginReminderMonitor) RunDueProbes(ctx context.Context) {
	if m.db == nil {
		return
	}
	sites, err := m.listEnabledSites()
	if err != nil {
		m.logger.Warnw("login_probe_list_sites_failed", "err", err)
		return
	}
	repo := models.NewSiteLoginStateRepository(m.db)
	for _, site := range sites {
		if ctx.Err() != nil {
			return
		}
		state, err := m.loadOrInitState(site.Name)
		if err != nil {
			m.logger.Warnw("login_probe_load_state_failed", "site", site.Name, "err", err)
			continue
		}
		if effectiveProbeMode(state.ProbeMode) != ProbeModeAuto {
			continue
		}
		now := m.clock.Now().UTC()
		needsInit := state.NextProbeAt == nil
		if !needsInit && !probeDue(state, now) {
			continue
		}
		release, ok := m.TryAcquireProbeLock(site.Name)
		if !ok {
			m.logger.Debugw("probe_skipped_singleflight_busy", "site", site.Name, "trigger", "cron")
			continue
		}
		if needsInit {
			jitter := state.ProbeJitterSeconds
			cols := map[string]any{}
			if jitter <= 0 {
				jitter = m.newJitterSeconds()
				cols["probe_jitter_seconds"] = jitter
			}
			cols["next_probe_at"] = now.Add(time.Duration(jitter) * time.Second)
			if err := repo.UpdateColumns(site.Name, cols); err != nil {
				m.logger.Warnw("login_probe_schedule_failed", "site", site.Name, "err", err)
			}
			if !probeRequested(state) {
				release()
				continue
			}
		}
		m.probeSiteInternal(ctx, site, false)
		release()
	}
}

// RunProbeOnce probes every enabled site whose mode allows cron probing,
// regardless of NextProbeAt. Exported so tests can drive the probe phase
// directly with a fake clock. Sites whose probe slot is held by another
// trigger source are skipped — R23 shared single-flight.
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

// probeSiteInternal 必须在持有该站单站锁时调用。顺序固定：先写 last_probe_started_at，
// 再重读站点配置与登录状态，再判断模式、预检、探测。凭证更新入口按「写凭证 → 刷新注册 →
// 写 probe_requested_at」执行，所以请求时间早于本次开始时间时，这里读到的一定是新配置；
// 晚于开始时间的请求仍会让该站保持到期，下一轮再探。
func (m *LoginReminderMonitor) probeSiteInternal(ctx context.Context, setting models.SiteSetting, manualTrigger bool) {
	name := setting.Name
	if _, err := m.loadOrInitState(name); err != nil {
		m.logger.Warnw("login_probe_load_state_failed", "site", name, "err", err)
		return
	}
	stateRepo := models.NewSiteLoginStateRepository(m.db)
	if err := stateRepo.UpdateColumns(name, map[string]any{"last_probe_started_at": m.clock.Now().UTC()}); err != nil {
		m.logger.Warnw("login_probe_mark_started_failed", "site", name, "err", err)
		return
	}

	fresh, err := models.NewSiteRepository(m.db).GetSiteByName(name)
	if err != nil {
		m.logger.Warnw("login_probe_get_site_failed", "site", name, "err", err)
		return
	}
	state, err := stateRepo.GetLoginState(name)
	if err != nil {
		m.logger.Warnw("login_probe_load_state_failed", "site", name, "err", err)
		return
	}

	// ProbeMode check: respect mode unless manual trigger
	if !manualTrigger {
		switch mode := effectiveProbeMode(state.ProbeMode); mode {
		case ProbeModeDisabled:
			m.logger.Debugw("probe_skipped_mode_disabled", "site", name)
			return
		case ProbeModeManual:
			m.logger.Debugw("probe_skipped_mode_manual", "site", name)
			return
		case ProbeModeAuto:
			// proceed normally
		default:
			m.logger.Warnw("probe_unknown_mode", "site", name, "mode", mode)
			return
		}
		if !fresh.Enabled {
			m.logger.Debugw("probe_skipped_site_disabled", "site", name)
			return
		}
	}

	result := m.runProbe(ctx, *fresh)
	m.recordProbeResult(name, state, result, m.clock.Now())
}

// runProbe 先做不发请求的预检，再经 site/v2 探测，并给整次探测加上时间预算。
func (m *LoginReminderMonitor) runProbe(ctx context.Context, setting models.SiteSetting) *sitelogin.ProbeResult {
	lookup := m.lookupDefinition
	if lookup == nil {
		lookup = defaultDefinitionLookup
	}
	knownDef, ok := lookup(setting.Name)
	if !ok || knownDef == nil {
		return &sitelogin.ProbeResult{
			Status:     sitelogin.UNSUPPORTED,
			Diagnostic: "站点没有内置定义（动态站点），暂不支持登录探测",
		}
	}
	if missing := missingCredential(setting); missing != "" {
		return &sitelogin.ProbeResult{Status: sitelogin.NOT_CONFIGURED, Diagnostic: missing}
	}

	budget := m.probeBudget
	if budget <= 0 {
		budget = loginProbeBudget
	}
	primaryTimeout := m.primaryTimeout
	if primaryTimeout <= 0 {
		primaryTimeout = loginProbePrimaryTimeout
	}
	probeCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	if m.userInfo != nil {
		// 站点 ID 与 RefreshSiteRegistrations 注册时一致，就是站点配置名。
		primary := timeoutTransport{
			inner:   sitelogin.UserInfoServiceTransport{Service: m.userInfo, SiteID: setting.Name},
			timeout: primaryTimeout,
		}
		result, _ := sitelogin.ProbeWithFallback(probeCtx, knownDef, nil, m.clock, primary, nil)
		if result == nil {
			result = &sitelogin.ProbeResult{Status: sitelogin.UNKNOWN, Diagnostic: "nil result"}
		}
		return result
	}

	if m.resolver == nil {
		return &sitelogin.ProbeResult{Status: sitelogin.UNKNOWN, Diagnostic: "站点解析器未初始化"}
	}
	def, site, err := m.resolver.Resolve(setting)
	if err != nil || site == nil {
		diag := "创建站点实例失败"
		if err != nil {
			diag += ": " + err.Error()
		}
		return &sitelogin.ProbeResult{Status: sitelogin.UNKNOWN, RawError: err, Diagnostic: diag}
	}
	defer site.Close()
	if def == nil {
		def = knownDef
	}

	primary := timeoutTransport{inner: sitelogin.HTTPTransport{}, timeout: primaryTimeout}
	result, _ := sitelogin.ProbeWithFallback(probeCtx, def, site, m.clock, primary, nil)
	if result == nil {
		result = &sitelogin.ProbeResult{Status: sitelogin.UNKNOWN, Diagnostic: "nil result"}
	}
	return result
}

// missingCredential 按认证方式检查必需凭证，缺失时返回说明；认证方式未知时不拦截。
func missingCredential(setting models.SiteSetting) string {
	hasCookie := strings.TrimSpace(setting.CookieEncrypted) != "" || strings.TrimSpace(setting.Cookie) != ""
	hasAPIKey := strings.TrimSpace(setting.APIKey) != ""
	switch strings.ToLower(strings.TrimSpace(setting.AuthMethod)) {
	case string(v2.AuthMethodCookie):
		if !hasCookie {
			return "未配置 Cookie"
		}
	case string(v2.AuthMethodAPIKey):
		if !hasAPIKey {
			return "未配置 API Key"
		}
	case string(v2.AuthMethodCookieAndAPIKey):
		switch {
		case !hasCookie && !hasAPIKey:
			return "未配置 Cookie 和 API Key"
		case !hasCookie:
			return "未配置 Cookie"
		case !hasAPIKey:
			return "未配置 API Key"
		}
	case string(v2.AuthMethodPasskey):
		if strings.TrimSpace(setting.Passkey) == "" {
			return "未配置 Passkey"
		}
	}
	return ""
}

// timeoutTransport 给主通道单独加一个上限，避免它吃掉整次探测的预算。
type timeoutTransport struct {
	inner   sitelogin.Transport
	timeout time.Duration
}

func (t timeoutTransport) Name() string { return t.inner.Name() }

func (t timeoutTransport) FetchUserInfo(ctx context.Context, def *v2.SiteDefinition, site v2.Site, clock sitelogin.Clock) (*sitelogin.ProbeResult, error) {
	if t.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.timeout)
		defer cancel()
	}
	return t.inner.FetchUserInfo(ctx, def, site, clock)
}

// recordProbeResult 只写探测拥有的列。prev 是取锁后重读的状态，探测列只有持锁的探测会写，所以可以据此累加。
func (m *LoginReminderMonitor) recordProbeResult(name string, prev *models.SiteLoginState, result *sitelogin.ProbeResult, now time.Time) {
	nowUTC := now.UTC()
	cols := map[string]any{
		"last_probe_at":     nowUTC,
		"last_probe_status": string(result.Status),
		"last_probe_error":  probeErrorText(result),
	}
	merged := *prev
	merged.LastProbeAt = &nowUTC
	merged.LastProbeStatus = string(result.Status)
	failures := prev.ConsecutiveProbeFailures

	switch {
	case result.Status == sitelogin.OK:
		dispatchProbeTimestamps(&merged, result)
		if result.LastAccessAt != nil {
			cols["last_access_at"] = merged.LastAccessAt
		}
		if result.LastLoginAt != nil {
			cols["last_login_at"] = merged.LastLoginAt
			cols["api_last_login_at"] = merged.ApiLastLoginAt
			cols["cookie_last_login_at"] = merged.CookieLastLoginAt
		}
		cols["last_consistency_check"] = sitelogin.CheckConsistency(merged.ApiLastLoginAt, merged.CookieLastLoginAt)
		failures = 0
		cols["consecutive_probe_failures"] = 0
		cols["first_failure_at"] = nil
		cols["last_success_at"] = nowUTC
		if result.LastAccessAt != nil {
			access := result.LastAccessAt.UTC()
			stale := prev.LastAccessAt != nil && prev.LastAccessAt.Equal(access) && nowUTC.Sub(access) >= accessStaleAfter
			switch {
			case stale && prev.AccessStaleSince == nil:
				cols["access_stale_since"] = nowUTC
			case !stale:
				cols["access_stale_since"] = nil
			}
		}
	case result.Status.IsFailure():
		failures = prev.ConsecutiveProbeFailures + 1
		cols["consecutive_probe_failures"] = failures
		if prev.FirstFailureAt == nil {
			cols["first_failure_at"] = nowUTC
		}
	}
	if prev.ProbeJitterSeconds <= 0 {
		cols["probe_jitter_seconds"] = m.newJitterSeconds()
	}
	next := nowUTC.Add(m.nextProbeDelay(result.Status, failures))
	cols["next_probe_at"] = next

	if err := models.NewSiteLoginStateRepository(m.db).UpdateColumns(name, cols); err != nil {
		m.logger.Warnw("login_probe_save_failed", "site", name, "err", err)
		return
	}

	effective := EffectiveLastActive(&merged, now)
	var daysRemainingLog any
	tier := "unknown"
	if !effective.IsZero() {
		daysRemainingLog = DaysRemaining(&merged, effective, now)
		tier = ComputeTier(&merged, effective, now)
	}
	m.logger.Infow(
		"login_probe_state_updated",
		"site", name,
		"status", string(result.Status),
		"source", string(result.Source),
		"last_access_at", timeForLog(merged.LastAccessAt),
		"api_last_login_at", timeForLog(merged.ApiLastLoginAt),
		"cookie_last_login_at", timeForLog(merged.CookieLastLoginAt),
		"effective_last_active_at", timeForLogValue(effective),
		"effective_source", effectiveSourceForLog(&merged, effective),
		"days_remaining", daysRemainingLog,
		"tier", tier,
		"consecutive_probe_failures", failures,
		"next_probe_at", timeForLogValue(next),
	)
}

func probeErrorText(result *sitelogin.ProbeResult) string {
	if result.RawError != nil {
		return result.RawError.Error()
	}
	if result.Status != sitelogin.OK {
		return result.Diagnostic
	}
	return ""
}

// nextProbeDelay 返回本次结果之后到下一次定时探测的间隔。failures 是本次之后的连续失败次数。
func (m *LoginReminderMonitor) nextProbeDelay(status sitelogin.ProbeStatus, failures int) time.Duration {
	every := m.probeEvery
	if every <= 0 {
		every = loginProbeInterval
	}
	switch {
	case status == sitelogin.OK:
		// 6 小时 ± 10%，各站的探测时刻不会逐渐对齐。
		spread := (rand.Float64()*2 - 1) * 0.1 * float64(every)
		return every + time.Duration(spread)
	case status.IsCredentialFailure():
		return credentialRetryDelay
	case status == sitelogin.NOT_CONFIGURED:
		// 不发请求，只在本地复查凭证是否已补齐。
		return every
	case status == sitelogin.UNSUPPORTED:
		return unsupportedRecheckDelay
	case failures >= 1 && failures <= len(transientRetryDelays):
		return transientRetryDelays[failures-1]
	default:
		return every
	}
}

// newJitterSeconds 返回 [1 秒, 探测间隔) 内的固定抖动；0 只表示「尚未分配」。
func (m *LoginReminderMonitor) newJitterSeconds() int {
	every := m.probeEvery
	if every <= 0 {
		every = loginProbeInterval
	}
	span := int(every / time.Second)
	if span <= 1 {
		return 1
	}
	return 1 + rand.Intn(span-1)
}

func effectiveProbeMode(mode string) string {
	if mode == "" {
		return ProbeModeAuto
	}
	return mode
}

// RequestProbe 记录一次探测请求：auto 模式下探测循环会在一分钟内探测该站，不在调用方的请求里同步探测。
// 用监控自己的时钟写 probe_requested_at，与 last_probe_started_at 可比。凭证与站点配置入口
// 按「写库 → 刷新站点注册 → RequestProbe」调用，所以请求早于某次探测开始时，那次探测一定读到新配置；
// 晚于开始时，该站在那次探测之后仍然到期。
func (m *LoginReminderMonitor) RequestProbe(siteName string) error {
	if m == nil || m.db == nil {
		return nil
	}
	if _, err := m.loadOrInitState(siteName); err != nil {
		return err
	}
	return models.NewSiteLoginStateRepository(m.db).UpdateColumns(siteName, map[string]any{
		"probe_requested_at": m.clock.Now().UTC(),
	})
}

// probeRequested 报告是否有人在最近一次探测开始之后请求过探测。
func probeRequested(state *models.SiteLoginState) bool {
	if state.ProbeRequestedAt == nil {
		return false
	}
	return state.LastProbeStartedAt == nil || state.ProbeRequestedAt.After(*state.LastProbeStartedAt)
}

func probeDue(state *models.SiteLoginState, now time.Time) bool {
	if state.NextProbeAt != nil && !state.NextProbeAt.After(now) {
		return true
	}
	return probeRequested(state)
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

// EffectiveActiveSource 返回判定活跃时间取自哪一列：last_access、api_last_login、
// cookie_last_login、last_login、last_visit；没有依据时为 none。
func EffectiveActiveSource(state *models.SiteLoginState, effective time.Time) string {
	return effectiveSourceForLog(state, effective)
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

// RunReminderOnce iterates enabled sites, computes each site's tier, fires
// reminders that are due, and decides probe failure / recovery notices.
// Exported for testability.
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
		m.evaluateProbeNotices(ctx, setting, now)
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
	if !m.notifier.Enabled() {
		return errors.New("通知路由未初始化：通知服务不可用")
	}
	effectiveStr := "未知"
	if !effective.IsZero() {
		effectiveStr = effective.UTC().Format(time.RFC3339)
	}
	entry := MonitorNotifyEntry{
		Source:      loginNotifySource,
		Subject:     site.Name,
		Kind:        "test",
		EventKey:    fmt.Sprintf("test:%d:%d", now.UnixNano(), m.testSeq.Add(1)),
		Title:       fmt.Sprintf("[pt-tools][测试] 站点 %s 登录提醒", site.Name),
		Text:        fmt.Sprintf("这是一条测试提醒。当前判定活跃 %s，剩余 %d 天 (tier=%s)。若你收到此消息，说明该站点的通知通道配置正常。", effectiveStr, daysRemaining, tier),
		ConfIDs:     decodeChannelIDs(state.NotificationChannelIDs),
		BypassQuiet: true,
	}
	inserted, err := m.notifier.Enqueue(ctx, entry)
	if err != nil {
		return err
	}
	if inserted == 0 {
		return errors.New("没有可用的通知通道")
	}
	return m.notifier.DeliverNow(ctx, entry)
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

	windowStart := expr.WindowStart(now)
	tierEscalated := tierIsHigher(tier, state.LastReminderTier)
	if !tierEscalated {
		if !expr.Match(now) {
			m.logger.Debugw("login_reminder_window_skip", "site", setting.Name, "tier", tier, "reason", "cron_not_matched")
			return
		}
		if state.LastReminderSentAt != nil && !state.LastReminderSentAt.Before(windowStart) {
			m.logger.Debugw("login_reminder_window_skip", "site", setting.Name, "tier", tier, "reason", "already_sent_in_window")
			return
		}
	}

	daysRemaining := DaysRemaining(state, effective, now)
	daysSinceActive := int(now.Sub(effective) / (24 * time.Hour))
	text := fmt.Sprintf("上次活跃 %s（%d 天前，依据：%s）；建议访问站点并浏览页面以刷新活跃时间（多数站点仅靠登录不更新 last_access）。剩余 %d 天 (tier=%s)。",
		effective.Format(time.RFC3339), daysSinceActive, activitySourceLabel(EffectiveActiveSource(state, effective)), daysRemaining, tier)
	if state.AccessStaleSince != nil {
		text += "探测成功但站点的最近访问时间没有更新，自动访问对该站无效，需要手动登录。"
	}
	entry := MonitorNotifyEntry{
		Source:      loginNotifySource,
		Subject:     setting.Name,
		Kind:        "tier",
		EventKey:    fmt.Sprintf("tier:%s:%d", tier, windowStart.Unix()),
		Title:       fmt.Sprintf("[pt-tools] 站点 %s 即将被封号", setting.Name),
		Text:        text,
		ConfIDs:     decodeChannelIDs(state.NotificationChannelIDs),
		BypassQuiet: tierAllowsImmediateOverride(tier),
	}

	m.logger.Infow("login_reminder_route_started",
		"site", setting.Name, "tier", tier,
		"days_remaining", daysRemaining, "escalated", tierEscalated,
		"channels", len(entry.ConfIDs))

	if m.notifier.Enabled() {
		if _, err := m.notifier.Enqueue(ctx, entry); err != nil {
			m.logger.Warnw("login_reminder_route_failed", "site", setting.Name, "err", err)
			return
		}
		m.logger.Infow("login_reminder_route_succeeded", "site", setting.Name, "tier", tier)
	} else {
		m.logger.Debugw("login_reminder_router_nil_proceed", "site", setting.Name, "tier", tier)
	}

	previousTier := state.LastReminderTier
	if err := models.NewSiteLoginStateRepository(m.db).UpdateColumns(setting.Name, map[string]any{
		"last_reminder_tier":    tier,
		"last_reminder_sent_at": now.UTC(),
	}); err != nil {
		m.logger.Warnw("login_reminder_save_failed", "site", setting.Name, "err", err)
	}

	if tierEscalated {
		m.logger.Infow("login_reminder_tier_escalated", "site", setting.Name, "tier", tier, "previous", previousTier)
	} else {
		m.logger.Infow("login_reminder_fired", "site", setting.Name, "tier", tier)
	}
}

// evaluateProbeNotices 判断探测失败与恢复提醒：
//   - 凭证类失败（会话过期、密钥错误）：进入失败后的第一次检查即提醒，未恢复时每 24 小时至多一次。
//   - 其他失败：自 FirstFailureAt 起满 24 小时才提醒，之后每 24 小时至多一次。
//   - 恢复：曾发过失败提醒、状态回到 OK 时提醒一次，并清空 LastFailureNotifiedAt。
func (m *LoginReminderMonitor) evaluateProbeNotices(ctx context.Context, setting models.SiteSetting, now time.Time) {
	state, err := m.loadOrInitState(setting.Name)
	if err != nil {
		m.logger.Warnw("login_notice_load_state_failed", "site", setting.Name, "err", err)
		return
	}
	status := sitelogin.ProbeStatus(state.LastProbeStatus)
	repo := models.NewSiteLoginStateRepository(m.db)
	channels := decodeChannelIDs(state.NotificationChannelIDs)

	if status == sitelogin.OK {
		if state.LastFailureNotifiedAt == nil {
			return
		}
		probedAt := "刚刚"
		if state.LastProbeAt != nil {
			probedAt = state.LastProbeAt.UTC().Format(time.RFC3339)
		}
		entry := MonitorNotifyEntry{
			Source:   loginNotifySource,
			Subject:  setting.Name,
			Kind:     "recovery",
			EventKey: fmt.Sprintf("recovery:%d", state.LastFailureNotifiedAt.Unix()),
			Title:    fmt.Sprintf("[pt-tools] 站点 %s 探测已恢复", setting.Name),
			Text:     fmt.Sprintf("最近一次探测成功（%s），登录状态已恢复正常。", probedAt),
			ConfIDs:  channels,
		}
		if m.notifier.Enabled() {
			if _, err := m.notifier.Enqueue(ctx, entry); err != nil {
				m.logger.Warnw("login_recovery_notice_failed", "site", setting.Name, "err", err)
				return
			}
		}
		if err := repo.UpdateColumns(setting.Name, map[string]any{"last_failure_notified_at": nil}); err != nil {
			m.logger.Warnw("login_notice_save_failed", "site", setting.Name, "err", err)
		}
		return
	}

	if !status.IsFailure() || state.FirstFailureAt == nil {
		return
	}
	first := *state.FirstFailureAt
	notifiedInStreak := state.LastFailureNotifiedAt != nil && !state.LastFailureNotifiedAt.Before(first)
	var due bool
	if status.IsCredentialFailure() {
		due = !notifiedInStreak || now.Sub(*state.LastFailureNotifiedAt) >= failureNoticeAfter
	} else {
		due = now.Sub(first) >= failureNoticeAfter &&
			(!notifiedInStreak || now.Sub(*state.LastFailureNotifiedAt) >= failureNoticeAfter)
	}
	if !due {
		return
	}
	var lastKey int64
	if notifiedInStreak {
		lastKey = state.LastFailureNotifiedAt.Unix()
	}
	advice := "pt-tools 会自动重试；若持续失败，请检查网络、代理或站点能否访问。"
	if status.IsCredentialFailure() {
		advice = "请重新同步 Cookie，或检查 API Key / Passkey 是否仍然有效。"
	}
	text := fmt.Sprintf("状态：%s（%s），自 %s 起持续失败。%s",
		probeStatusLabel(status), status, first.UTC().Format(time.RFC3339), advice)
	if diag := strings.TrimSpace(state.LastProbeError); diag != "" {
		text += "\n诊断：" + truncateRunes(diag, 200)
	}
	entry := MonitorNotifyEntry{
		Source:   loginNotifySource,
		Subject:  setting.Name,
		Kind:     "failure",
		EventKey: fmt.Sprintf("failure:%d:%d", first.Unix(), lastKey),
		Title:    fmt.Sprintf("[pt-tools] 站点 %s 探测失败", setting.Name),
		Text:     text,
		ConfIDs:  channels,
	}
	if m.notifier.Enabled() {
		if _, err := m.notifier.Enqueue(ctx, entry); err != nil {
			m.logger.Warnw("login_failure_notice_failed", "site", setting.Name, "err", err)
			return
		}
	}
	if err := repo.UpdateColumns(setting.Name, map[string]any{"last_failure_notified_at": now.UTC()}); err != nil {
		m.logger.Warnw("login_notice_save_failed", "site", setting.Name, "err", err)
	}
	m.logger.Infow("login_failure_notice_fired", "site", setting.Name, "status", string(status), "since", timeForLogValue(first))
}

func probeStatusLabel(status sitelogin.ProbeStatus) string {
	if label, ok := probeStatusLabels[status]; ok {
		return label
	}
	return "未知状态"
}

func activitySourceLabel(source string) string {
	switch source {
	case "last_access":
		return "站点最近访问"
	case "api_last_login", "cookie_last_login", "last_login":
		return "站点最近登录"
	case "last_visit":
		return "浏览器访问"
	default:
		return "未知"
	}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// EffectiveLastActive returns the most recent confirmed activity timestamp for a site.
//
// Per research: site cleanup.php scripts predominantly check `last_access` (NexusPHP)
// / `last_action` (Unit3D) / `LastAccess` (Gazelle) / `lastModifiedDate` (mTorrent)
// — NOT `last_login`. So the probe-derived value prefers last_access-class timestamps
// and treats last_login-class timestamps (ApiLastLoginAt, CookieLastLoginAt) as
// supplementary signals, used only when last_access is missing:
//
//  1. state.LastAccessAt
//  2. max(state.ApiLastLoginAt, state.CookieLastLoginAt)
//  3. state.LastLoginAt   (legacy v1 field)
//
// 最近一次探测为 OK 时只用上面的结果（站点的数据就是事实，探测本身也刷新了它）；
// 否则探测数据可能已过时，再与浏览器扩展上报的 LastVisitAt 取较新者，
// 避免探测坏掉后用户正常用浏览器访问，剩余天数却照样下降。
func EffectiveLastActive(state *models.SiteLoginState, _ time.Time) time.Time {
	if state == nil {
		return time.Time{}
	}
	base := probeActivity(state)
	if state.LastProbeStatus == string(sitelogin.OK) {
		return base
	}
	if state.LastVisitAt != nil && state.LastVisitAt.After(base) {
		return *state.LastVisitAt
	}
	return base
}

func probeActivity(state *models.SiteLoginState) time.Time {
	if state.LastAccessAt != nil {
		return *state.LastAccessAt
	}
	if login := newest(state.ApiLastLoginAt, state.CookieLastLoginAt); !login.IsZero() {
		return login
	}
	if state.LastLoginAt != nil {
		return *state.LastLoginAt
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
	defaults := models.DefaultSiteLoginState(siteName)
	defaults.BanThresholdDays = banDays
	defaults.RemindBeforeDays = remindDays
	if err := repo.EnsureLoginStateRow(defaults); err != nil {
		return nil, err
	}
	return repo.GetLoginState(siteName)
}
