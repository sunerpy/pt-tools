package scheduler

import (
	"context"
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
	cancel context.CancelFunc
}
type Manager struct {
	mu                sync.Mutex
	jobs              map[string]*job
	wg                sync.WaitGroup
	lastVersion       int64
	downloaderManager *downloader.DownloaderManager
	freeEndMonitor    *FreeEndMonitor
	cleanupMonitor    *CleanupMonitor
	eventCancel       func()
	stopped           bool
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
	m.jobs[k] = &job{cancel: cancel}
	go runner(ctx)
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

func (m *Manager) Reload(cfg *models.Config) {
	if global.GlobalDB == nil {
		global.GetSlogger().Warn("配置未就绪：数据库未初始化，任务不启动")
		return
	}
	if cfg == nil || cfg.Global.DownloadDir == "" {
		global.GetSlogger().Warn("配置未就绪：下载目录为空，任务不启动")
		return
	}
	if !cfg.Global.AutoStart {
		global.GetSlogger().Info("任务设置为手动启动，跳过自动启动")
		return
	}
	// 简化：等待当前执行中的任务结束或超时后重启
	m.mu.Lock()
	for _, j := range m.jobs {
		j.cancel()
	}
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	m.mu.Unlock()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
	}
	m.mu.Lock()
	m.jobs = map[string]*job{}
	m.mu.Unlock()

	// 初始化下载器管理器
	m.initDownloaderManager()

	m.initFreeEndMonitor()
	m.initCleanupMonitor()

	// 检查是否有可用的默认下载器
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
				m.Start(site, rr, func(ctx context.Context) { m.wg.Add(1); defer m.wg.Done(); runRSSJobUnified(ctx, rr, impl) })
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

// StopAll 取消所有任务并等待当前执行结束
func (m *Manager) StopAll() {
	m.mu.Lock()
	m.stopped = true
	for _, j := range m.jobs {
		j.cancel()
	}
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	m.mu.Unlock()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
	}
	m.mu.Lock()
	m.jobs = map[string]*job{}
	if m.freeEndMonitor != nil {
		m.freeEndMonitor.Stop()
		m.freeEndMonitor = nil
	}
	if m.cleanupMonitor != nil {
		m.cleanupMonitor.Stop()
		m.cleanupMonitor = nil
	}
	if m.eventCancel != nil {
		m.eventCancel()
		m.eventCancel = nil
	}
	m.mu.Unlock()
}

// StartAll 按配置启动所有任务（不做停止）
func (m *Manager) StartAll(cfg *models.Config) {
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
				m.Start(site, rr, func(ctx context.Context) { m.wg.Add(1); defer m.wg.Done(); runRSSJobUnified(ctx, rr, impl) })
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
