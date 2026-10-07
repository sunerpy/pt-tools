package scheduler

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/transmission"
)

type job struct {
	cancel    context.CancelFunc
	startedAt time.Time
}

type JobStatus struct {
	SiteName  string
	RSSName   string
	Running   bool
	StartedAt time.Time
}

type Manager struct {
	mu                   sync.Mutex
	jobs                 map[string]*job
	wg                   sync.WaitGroup
	lastVersion          int64
	downloaderManager    *downloader.DownloaderManager
	freeEndMonitor       *FreeEndMonitor
	cleanupMonitor       *CleanupMonitor
	peerRatioMonitor     *PeerRatioMonitor
	loginReminderMonitor *LoginReminderMonitor
	attendanceMonitor    *AttendanceMonitor
	dailyReportJob       *DailyReportJob
	brushMonitor         *BrushMonitor
	deadTorrentMonitor   *DeadTorrentMonitor
	transferWorker       *TransferWorker
	reseedWorker         *ReseedWorker
	cookieCloudWorker    *CookieCloudWorker
	eventCancel          func()
	stopped              bool
	// jobsWanted / jobsPaused 记录用户在调度器里点的「启动 / 停止所有任务」：
	// 手动启动后配置变更照常重启任务；手动停止后配置变更不再把任务拉起来，直到再次手动启动。
	jobsWanted bool
	jobsPaused bool
}

func NewManager() *Manager {
	m := &Manager{
		jobs:              map[string]*job{},
		downloaderManager: downloader.NewDownloaderManager(),
	}

	id, ch, cancel := events.Subscribe(64)
	_ = id
	m.eventCancel = cancel
	go func() {
		defer cancel()
		var pendingVersion int64
		var timer *time.Timer
		for e := range ch {
			if e.Type != events.ConfigChanged {
				continue
			}
			m.mu.Lock()
			if m.stopped {
				m.mu.Unlock()
				return
			}
			if e.Version <= m.lastVersion {
				m.mu.Unlock()
				continue
			}
			pendingVersion = e.Version
			m.mu.Unlock()

			if timer == nil {
				timer = time.NewTimer(200 * time.Millisecond)
			} else {
				if !timer.Stop() {
				}
				timer.Reset(200 * time.Millisecond)
			}
			<-timer.C
			m.mu.Lock()
			if m.stopped {
				m.mu.Unlock()
				return
			}
			db := global.GlobalDB
			m.mu.Unlock()
			if db == nil {
				continue
			}
			cfg, _ := core.NewConfigStore(db).Load()
			if cfg != nil {
				m.Reload(cfg)
				m.mu.Lock()
				m.lastVersion = pendingVersion
				m.mu.Unlock()
			}
		}
	}()
	return m
}

func (m *Manager) InitFreeEndMonitor() {
	m.initDownloaderManager()
	m.initFreeEndMonitor()
	m.initCleanupMonitor()
	m.initPeerRatioMonitor()
}

// GetDownloaderManager 获取下载器管理器
func (m *Manager) GetDownloaderManager() *downloader.DownloaderManager {
	return m.downloaderManager
}

// GetFreeEndMonitor 获取免费结束监控器
func (m *Manager) GetFreeEndMonitor() *FreeEndMonitor {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.freeEndMonitor
}

func (m *Manager) LastVersion() int64 { return m.lastVersion }
func key(site models.SiteGroup, rssName string) string {
	return string(site) + "|" + rssName
}

func (m *Manager) Start(site models.SiteGroup, r models.RSSConfig, runner func(ctx context.Context)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(site, r.Name)
	if _, ok := m.jobs[k]; ok {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.jobs[k] = &job{cancel: cancel, startedAt: time.Now()}
	// 在启动协程之前登记：等待方（Reload/StopJobs/StopAll）随时 Wait 都能等到这个任务，
	// 不会出现计数还是 0、Wait 已返回而旧任务稍后才开始运行的窗口。
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		runner(ctx)
	}()
}

func (m *Manager) Stop(site models.SiteGroup, rssName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(site, rssName)
	if j, ok := m.jobs[k]; ok {
		j.cancel()
		delete(m.jobs, k)
	}
}

func (m *Manager) ListJobs() []JobStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]JobStatus, 0, len(m.jobs))
	for k, j := range m.jobs {
		parts := splitKey(k)
		if len(parts) == 2 {
			result = append(result, JobStatus{
				SiteName:  parts[0],
				RSSName:   parts[1],
				Running:   true,
				StartedAt: j.startedAt,
			})
		}
	}
	return result
}

func splitKey(k string) []string {
	for i := len(k) - 1; i >= 0; i-- {
		if k[i] == '|' {
			return []string{k[:i], k[i+1:]}
		}
	}
	return nil
}

func (m *Manager) Reload(cfg *models.Config) {
	if global.GlobalDB == nil {
		global.GetSlogger().Warn("配置未就绪：数据库未初始化，任务不启动")
		return
	}
	// 先停掉旧任务再判断新配置：旧任务带着旧配置，新配置不让运行（下载目录清空等）时它们也必须停下。
	m.mu.Lock()
	wasRunning := len(m.jobs) > 0
	m.mu.Unlock()
	m.cancelJobsAndWait()

	if cfg == nil || cfg.Global.DownloadDir == "" {
		global.GetSlogger().Warn("配置未就绪：下载目录为空，任务不启动")
		return
	}
	m.mu.Lock()
	paused, wanted := m.jobsPaused, m.jobsWanted
	m.mu.Unlock()
	if paused {
		global.GetSlogger().Info("任务已在调度器中手动停止，配置变更后不自动启动")
		return
	}
	// 自动启动只管程序启动时要不要跑；用户手动启动过（或任务正在运行）时，配置变更要按新配置重启。
	if !cfg.Global.AutoStart && !wanted && !wasRunning {
		global.GetSlogger().Info("任务设置为手动启动，跳过自动启动")
		return
	}

	// 初始化下载器管理器
	m.initDownloaderManager()

	m.initFreeEndMonitor()
	m.initCleanupMonitor()
	m.initPeerRatioMonitor()

	defaultDl, err := m.downloaderManager.GetDefaultDownloader()
	if err != nil {
		global.GetSlogger().Warnf("未配置默认下载器，任务不启动: %v", err)
		return
	}

	// 对默认下载器进行健康检查
	if ok, pingErr := defaultDl.Ping(); !ok {
		global.GetSlogger().Errorf("默认下载器健康检查失败，任务不启动: %v", pingErr)
		return
	}
	global.GetSlogger().Infof("默认下载器 %s 健康检查通过", defaultDl.GetName())

	// 重新启动：每次启动任务时从 DB 读取最新配置，保证一致性
	for site, sc := range cfg.Sites {
		if sc.Enabled != nil && *sc.Enabled {
			// 使用统一的工厂函数创建站点实现
			impl, err := internal.NewUnifiedSiteImpl(context.Background(), site)
			if err != nil {
				global.GetSlogger().Warnf("站点 %s 未注册或不支持，跳过: %v", string(site), err)
				continue
			}
			for _, r := range sc.RSS {
				if r.ShouldSkip() {
					global.GetSlogger().Debugf("跳过RSS配置: %s %s (示例或空URL)", string(site), r.Name)
					continue
				}
				if !validRSS(r.URL) {
					global.GetSlogger().Warnf("跳过无效RSS: %s %s", string(site), r.Name)
					continue
				}
				rr := r
				m.Start(site, rr, func(ctx context.Context) { runRSSJobUnified(ctx, rr, impl) })
			}
		}
	}
}

// initDownloaderManager 从数据库初始化下载器管理器
func (m *Manager) initDownloaderManager() {
	if global.GlobalDB == nil {
		return
	}

	// 注册下载器工厂
	m.downloaderManager.RegisterFactory(downloader.DownloaderQBittorrent, createQBitFactory())
	m.downloaderManager.RegisterFactory(downloader.DownloaderTransmission, createTransmissionFactory())

	// 从数据库加载下载器配置
	var downloaderSettings []models.DownloaderSetting
	if err := global.GlobalDB.DB.Find(&downloaderSettings).Error; err != nil {
		global.GetSlogger().Errorf("加载下载器配置失败: %v", err)
		return
	}

	for _, ds := range downloaderSettings {
		if !ds.Enabled {
			continue
		}

		dlType := downloader.DownloaderType(ds.Type)
		if !m.downloaderManager.HasFactory(dlType) {
			global.GetSlogger().Warnf("未知下载器类型: %s", ds.Type)
			continue
		}

		config := downloader.NewGenericConfig(dlType, ds.URL, ds.Username, ds.Password, ds.AutoStart)
		if err := m.downloaderManager.RegisterConfig(ds.Name, config, ds.IsDefault); err != nil {
			global.GetSlogger().Errorf("注册下载器配置失败: %s, %v", ds.Name, err)
			continue
		}

		m.checkDownloaderHealthAsync(ds)
	}

	// 加载站点-下载器映射
	var sites []models.SiteSetting
	if err := global.GlobalDB.DB.Find(&sites).Error; err != nil {
		global.GetSlogger().Errorf("加载站点配置失败: %v", err)
		return
	}

	for _, site := range sites {
		if site.DownloaderID != nil {
			var dlSetting models.DownloaderSetting
			if err := global.GlobalDB.DB.First(&dlSetting, *site.DownloaderID).Error; err == nil {
				m.downloaderManager.SetSiteDownloader(site.Name, dlSetting.Name)
			}
		}
	}

	global.GetSlogger().Info("下载器管理器初始化完成")

	internal.SetGlobalDownloaderManager(m.downloaderManager)
}

func (m *Manager) checkDownloaderHealthAsync(setting models.DownloaderSetting) {
	go func(ds models.DownloaderSetting) {
		dlType := downloader.DownloaderType(ds.Type)
		if !m.downloaderManager.HasFactory(dlType) {
			global.GetSlogger().Warnf("未知下载器类型: %s", ds.Type)
			return
		}

		config := downloader.NewGenericConfig(dlType, ds.URL, ds.Username, ds.Password, ds.AutoStart)
		dl, err := m.downloaderManager.CreateFromConfig(config, ds.Name)
		if err != nil {
			global.GetSlogger().Errorf("[下载器健康检查] %s 创建实例失败: %v", ds.Name, err)
			return
		}
		defer dl.Close()

		if ok, pingErr := dl.Ping(); ok {
			global.GetSlogger().Infof("[下载器健康检查] %s 连接正常 (类型=%s, 默认=%v)", ds.Name, ds.Type, ds.IsDefault)
		} else {
			global.GetSlogger().Warnf("[下载器健康检查] %s 连接失败: %v (类型=%s)", ds.Name, pingErr, ds.Type)
		}
	}(setting)
}

func (m *Manager) initFreeEndMonitor() {
	if global.GlobalDB == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.freeEndMonitor != nil {
		m.freeEndMonitor.Stop()
	}

	m.freeEndMonitor = NewFreeEndMonitor(global.GlobalDB.DB, m.downloaderManager)
	if err := m.freeEndMonitor.Start(); err != nil {
		global.GetSlogger().Errorf("启动免费结束监控器失败: %v", err)
		return
	}

	internal.RegisterTorrentScheduler(func(torrent models.TorrentInfo) {
		m.mu.Lock()
		monitor := m.freeEndMonitor
		m.mu.Unlock()
		if monitor != nil {
			monitor.ScheduleTorrent(torrent)
		}
	})
}

func (m *Manager) initCleanupMonitor() {
	if global.GlobalDB == nil {
		return
	}

	if m.cleanupMonitor != nil {
		m.cleanupMonitor.Stop()
	}

	m.cleanupMonitor = NewCleanupMonitor(global.GlobalDB.DB, m.downloaderManager)
	if err := m.cleanupMonitor.Start(); err != nil {
		global.GetSlogger().Errorf("启动自动删种监控器失败: %v", err)
	}
}

func (m *Manager) initPeerRatioMonitor() {
	if global.GlobalDB == nil {
		return
	}

	if m.peerRatioMonitor != nil {
		m.peerRatioMonitor.Stop()
	}

	m.peerRatioMonitor = NewPeerRatioMonitor(global.GlobalDB.DB, m.downloaderManager)
	if err := m.peerRatioMonitor.Start(); err != nil {
		global.GetSlogger().Errorf("启动竞争度监控器失败: %v", err)
	}
}

// createQBitFactory 创建 qBittorrent 工厂
func createQBitFactory() downloader.DownloaderFactory {
	return func(config downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
		qbitConfig := qbit.NewQBitConfigWithAutoStart(config.GetURL(), config.GetUsername(), config.GetPassword(), config.GetAutoStart())
		return qbit.NewQbitClient(qbitConfig, name)
	}
}

// createTransmissionFactory 创建 Transmission 工厂
func createTransmissionFactory() downloader.DownloaderFactory {
	return func(config downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
		transConfig := transmission.NewTransmissionConfigWithAutoStart(config.GetURL(), config.GetUsername(), config.GetPassword(), config.GetAutoStart())
		return transmission.NewTransmissionClient(transConfig, name)
	}
}

func validRSS(raw string) bool {
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	if host == "rss.m-team.xxx" {
		return false
	}
	return true
}

// cancelJobsAndWait 取消全部 RSS 任务，等它们退出（最多 30 秒）后清空任务表。
func (m *Manager) cancelJobsAndWait() {
	m.mu.Lock()
	for _, j := range m.jobs {
		j.cancel()
	}
	m.mu.Unlock()
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		global.GetSlogger().Warn("等待 RSS 任务退出超时（30 秒），继续执行")
	}
	m.mu.Lock()
	m.jobs = map[string]*job{}
	m.mu.Unlock()
}

// StopJobs 停止全部 RSS 任务（调度器里的「停止所有任务」）。免费到期、自动删种、竞争度、
// 登录提醒和签到监控继续运行，配置热重载也照常；之后的配置变更不会把任务重新拉起，直到 StartAll。
func (m *Manager) StopJobs() {
	m.mu.Lock()
	m.jobsPaused, m.jobsWanted = true, false
	m.mu.Unlock()
	m.cancelJobsAndWait()
}

// StopAll 是进程退出前的最终关闭：取消所有任务并等待结束，停掉全部监控器和配置事件订阅。
// 之后不能再用 StartAll 恢复；调度器里的「停止所有任务」用 StopJobs。
func (m *Manager) StopAll() {
	m.mu.Lock()
	m.stopped = true
	m.mu.Unlock()
	m.cancelJobsAndWait()
	m.mu.Lock()
	if m.freeEndMonitor != nil {
		m.freeEndMonitor.Stop()
		m.freeEndMonitor = nil
	}
	if m.cleanupMonitor != nil {
		m.cleanupMonitor.Stop()
		m.cleanupMonitor = nil
	}
	if m.peerRatioMonitor != nil {
		m.peerRatioMonitor.Stop()
		m.peerRatioMonitor = nil
	}
	if m.brushMonitor != nil {
		m.brushMonitor.Stop()
		m.brushMonitor = nil
	}
	if m.deadTorrentMonitor != nil {
		m.deadTorrentMonitor.Stop()
		m.deadTorrentMonitor = nil
	}
	if m.reseedWorker != nil {
		m.reseedWorker.Stop()
		m.reseedWorker = nil
	}
	if m.cookieCloudWorker != nil {
		m.cookieCloudWorker.Stop()
		m.cookieCloudWorker = nil
	}
	if m.transferWorker != nil {
		m.transferWorker.Stop()
		m.transferWorker = nil
	}
	if m.dailyReportJob != nil {
		m.dailyReportJob.Stop()
		m.dailyReportJob = nil
	}
	if m.attendanceMonitor != nil {
		m.attendanceMonitor.Stop()
		m.attendanceMonitor = nil
	}
	if m.loginReminderMonitor != nil {
		m.loginReminderMonitor.Stop()
		m.loginReminderMonitor = nil
	}
	if m.eventCancel != nil {
		m.eventCancel()
		m.eventCancel = nil
	}
	m.mu.Unlock()
}

// StartAll 按配置启动所有任务（不做停止），并记下用户要求运行任务。
func (m *Manager) StartAll(cfg *models.Config) {
	m.mu.Lock()
	m.jobsPaused, m.jobsWanted = false, true
	m.mu.Unlock()
	for site, sc := range cfg.Sites {
		if sc.Enabled != nil && *sc.Enabled {
			// 使用统一的工厂函数创建站点实现
			impl, err := internal.NewUnifiedSiteImpl(context.Background(), site)
			if err != nil {
				global.GetSlogger().Warnf("站点 %s 未注册或不支持，跳过: %v", string(site), err)
				continue
			}
			for _, r := range sc.RSS {
				if r.ShouldSkip() {
					global.GetSlogger().Debugf("跳过RSS配置: %s %s (示例或空URL)", string(site), r.Name)
					continue
				}
				rr := r
				m.Start(site, rr, func(ctx context.Context) { runRSSJobUnified(ctx, rr, impl) })
			}
		}
	}
}

// runRSSJobUnified 使用 UnifiedPTSite 接口运行 RSS 任务
func runRSSJobUnified(ctx context.Context, cfg models.RSSConfig, siteImpl internal.UnifiedPTSite) {
	ticker := time.NewTicker(getInterval(cfg))
	defer ticker.Stop()
	executeTaskUnified(ctx, cfg, siteImpl)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			executeTaskUnified(ctx, cfg, siteImpl)
		}
	}
}

func getInterval(cfg models.RSSConfig) time.Duration {
	var gl *models.SettingsGlobal
	if global.GlobalDB != nil {
		store := core.NewConfigStore(global.GlobalDB)
		if g, err := store.GetGlobalOnly(); err == nil {
			gl = &g
		}
	}
	// 使用 RSSConfig 的方法获取有效间隔时间
	intervalMinutes := cfg.GetEffectiveIntervalMinutes(gl)
	return time.Duration(intervalMinutes) * time.Minute
}

func executeTaskUnified(ctx context.Context, cfg models.RSSConfig, siteImpl internal.UnifiedPTSite) {
	if err := processRSSUnified(ctx, cfg, siteImpl); err != nil {
		global.GetSlogger().Errorf("站点: %s 任务执行失败, %v", cfg.Name, err)
	}
}

func processRSSUnified(ctx context.Context, cfg models.RSSConfig, ptSite internal.UnifiedPTSite) error {
	if err := internal.FetchAndDownloadFreeRSSUnified(ctx, ptSite, cfg); err != nil {
		return err
	}
	if err := ptSite.SendTorrentToDownloader(ctx, cfg); err != nil {
		return err
	}
	return nil
}

// runRSSJob 旧的泛型版本（已废弃，请使用 runRSSJobUnified）
// Deprecated: Use runRSSJobUnified instead for new implementations
func runRSSJob[T models.ResType](ctx context.Context, siteName models.SiteGroup, cfg models.RSSConfig, siteImpl internal.PTSiteInter[T]) {
	ticker := time.NewTicker(getInterval(cfg))
	defer ticker.Stop()
	executeTask(ctx, siteName, cfg, siteImpl)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			executeTask(ctx, siteName, cfg, siteImpl)
		}
	}
}

func executeTask[T models.ResType](ctx context.Context, siteName models.SiteGroup, cfg models.RSSConfig, siteImpl internal.PTSiteInter[T]) {
	if err := processRSS(ctx, siteName, cfg, siteImpl); err != nil {
		global.GetSlogger().Errorf("站点: %s 任务执行失败, %v", cfg.Name, err)
	}
}

func processRSS[T models.ResType](ctx context.Context, siteName models.SiteGroup, cfg models.RSSConfig, ptSite internal.PTSiteInter[T]) error {
	if err := internal.FetchAndDownloadFreeRSS(ctx, siteName, ptSite, cfg); err != nil {
		return err
	}
	if err := ptSite.SendTorrentToDownloader(ctx, cfg); err != nil {
		return err
	}
	return nil
}

// SetLoginReminderMonitor wires a fully-constructed LoginReminderMonitor
// into the manager. It is intended to be called by the web layer after the
// monitor has been built with a SiteResolver that knows how to produce
// v2.Site instances. Calling this twice replaces and stops the previous
// instance. Pass nil to detach without stopping.
func (m *Manager) SetLoginReminderMonitor(mon *LoginReminderMonitor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.loginReminderMonitor != nil && m.loginReminderMonitor != mon {
		m.loginReminderMonitor.Stop()
	}
	m.loginReminderMonitor = mon
}

// GetLoginReminderMonitor returns the registered monitor, or nil if not yet
// wired. Used by web handlers that want to trigger ad-hoc probes.
func (m *Manager) GetLoginReminderMonitor() *LoginReminderMonitor {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loginReminderMonitor
}

// SetAttendanceMonitor 登记每日签到监控；替换旧实例时先停掉旧的。
func (m *Manager) SetAttendanceMonitor(mon *AttendanceMonitor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.attendanceMonitor != nil && m.attendanceMonitor != mon {
		m.attendanceMonitor.Stop()
	}
	m.attendanceMonitor = mon
}

// SetDailyReportJob 登记每日战报任务；替换旧实例时先停掉旧的。
func (m *Manager) SetDailyReportJob(job *DailyReportJob) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dailyReportJob != nil && m.dailyReportJob != job {
		m.dailyReportJob.Stop()
	}
	m.dailyReportJob = job
}

// SetDeadTorrentMonitor 登记失效种子定时扫描；替换旧实例时先停掉旧的。
func (m *Manager) SetDeadTorrentMonitor(mon *DeadTorrentMonitor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deadTorrentMonitor != nil && m.deadTorrentMonitor != mon {
		m.deadTorrentMonitor.Stop()
	}
	m.deadTorrentMonitor = mon
}

// GetDeadTorrentMonitor 返回失效种子定时扫描（未接线时为 nil）。
func (m *Manager) GetDeadTorrentMonitor() *DeadTorrentMonitor {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deadTorrentMonitor
}

// SetTransferWorker 登记转移做种后台；替换旧实例时先停掉旧的。
func (m *Manager) SetTransferWorker(w *TransferWorker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.transferWorker != nil && m.transferWorker != w {
		m.transferWorker.Stop()
	}
	m.transferWorker = w
}

// GetTransferWorker 返回转移做种后台（未接线时为 nil）。
func (m *Manager) GetTransferWorker() *TransferWorker {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.transferWorker
}

// SetReseedWorker 登记辅种后台；替换旧实例时先停掉旧的。
func (m *Manager) SetReseedWorker(w *ReseedWorker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.reseedWorker != nil && m.reseedWorker != w {
		m.reseedWorker.Stop()
	}
	m.reseedWorker = w
}

// GetReseedWorker 返回辅种后台（未接线时为 nil）。
func (m *Manager) GetReseedWorker() *ReseedWorker {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reseedWorker
}

// SetCookieCloudWorker 登记 CookieCloud 定时同步；替换旧实例时先停掉旧的。
func (m *Manager) SetCookieCloudWorker(w *CookieCloudWorker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cookieCloudWorker != nil && m.cookieCloudWorker != w {
		m.cookieCloudWorker.Stop()
	}
	m.cookieCloudWorker = w
}

// GetCookieCloudWorker 返回 CookieCloud 定时同步（未接线时为 nil）。
func (m *Manager) GetCookieCloudWorker() *CookieCloudWorker {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cookieCloudWorker
}

// TransferDownloader 按下载器 ID 取管理器里的实例与配置（转移做种用）；下载器不存在、未启用或连不上时返回错误。
func (m *Manager) TransferDownloader(ctx context.Context, id uint) (downloader.Downloader, models.DownloaderSetting, error) {
	if global.GlobalDB == nil {
		return nil, models.DownloaderSetting{}, errors.New("数据库未初始化")
	}
	var ds models.DownloaderSetting
	if err := global.GlobalDB.DB.WithContext(ctx).First(&ds, id).Error; err != nil {
		return nil, ds, fmt.Errorf("下载器 %d 不存在: %w", id, err)
	}
	if !ds.Enabled {
		return nil, ds, fmt.Errorf("下载器 %s 未启用", ds.Name)
	}
	dl, err := m.downloaderManager.GetDownloaderContext(ctx, ds.Name)
	if err != nil {
		return nil, ds, err
	}
	return dl, ds, nil
}

// SetBrushMonitor 登记刷流监控；替换旧实例时先停掉旧的。
func (m *Manager) SetBrushMonitor(mon *BrushMonitor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.brushMonitor != nil && m.brushMonitor != mon {
		m.brushMonitor.Stop()
	}
	m.brushMonitor = mon
}

// GetBrushMonitor 返回刷流监控，没有接线时为 nil。
func (m *Manager) GetBrushMonitor() *BrushMonitor {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.brushMonitor
}

// BrushDownloader 按下载器 ID 取管理器里的实例（刷流列种子、删种用）；下载器不存在或未启用时返回错误。
func (m *Manager) BrushDownloader(id uint) (downloader.Downloader, string, error) {
	if global.GlobalDB == nil {
		return nil, "", errors.New("数据库未初始化")
	}
	var ds models.DownloaderSetting
	if err := global.GlobalDB.DB.First(&ds, id).Error; err != nil {
		return nil, "", fmt.Errorf("下载器 %d 不存在: %w", id, err)
	}
	if !ds.Enabled {
		return nil, ds.Name, fmt.Errorf("下载器 %s 未启用", ds.Name)
	}
	dl, err := m.downloaderManager.GetDownloader(ds.Name)
	if err != nil {
		return nil, ds.Name, err
	}
	return dl, ds.Name, nil
}

// GetAttendanceMonitor 返回每日签到监控，没有接线时为 nil。
func (m *Manager) GetAttendanceMonitor() *AttendanceMonitor {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.attendanceMonitor
}
