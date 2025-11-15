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
)

type job struct {
	cancel context.CancelFunc
}
type Manager struct {
	mu          sync.Mutex
	jobs        map[string]*job // key: site|rss.name
	wg          sync.WaitGroup
	lastVersion int64
}

func NewManager() *Manager {
	m := &Manager{jobs: map[string]*job{}}
	id, ch, cancel := events.Subscribe(64)
	_ = id
	go func() {
		defer cancel()
		var pendingVersion int64
		var timer *time.Timer
		for e := range ch {
			if e.Type != events.ConfigChanged {
				continue
			}
			if e.Version <= m.lastVersion {
				continue
			}
			pendingVersion = e.Version
			if timer == nil {
				timer = time.NewTimer(200 * time.Millisecond)
			} else {
				if !timer.Stop() {
				}
				timer.Reset(200 * time.Millisecond)
			}
			go func() {
				<-timer.C
				if global.GlobalDB == nil {
					return
				}
				cfg, _ := core.NewConfigStore(global.GlobalDB).Load()
				if cfg != nil {
					m.Reload(cfg)
					m.lastVersion = pendingVersion
				}
			}()
		}
	}()
	return m
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
	// 重新启动：每次启动任务时从 DB 读取最新配置，保证一致性
    for site, sc := range cfg.Sites {
        if sc.Enabled != nil && *sc.Enabled {
            switch site {
            case models.MTEAM:
                impl := internal.NewMteamImpl(context.Background())
                for _, r := range sc.RSS {
                    if !validRSS(r.URL) {
                        global.GetSlogger().Warnf("跳过无效RSS: %s %s", string(site), r.Name)
                        continue
                    }
                    rr := r
                    m.Start(site, rr, func(ctx context.Context) { m.wg.Add(1); defer m.wg.Done(); runRSSJob(ctx, site, rr, impl) })
                }
            case models.HDSKY:
                impl := internal.NewHdskyImpl(context.Background())
                for _, r := range sc.RSS {
                    if !validRSS(r.URL) {
                        global.GetSlogger().Warnf("跳过无效RSS: %s %s", string(site), r.Name)
                        continue
                    }
                    rr := r
                    m.Start(site, rr, func(ctx context.Context) { m.wg.Add(1); defer m.wg.Done(); runRSSJob(ctx, site, rr, impl) })
                }
            case models.CMCT:
                impl := internal.NewCmctImpl(context.Background())
                for _, r := range sc.RSS {
                    if !validRSS(r.URL) {
                        global.GetSlogger().Warnf("跳过无效RSS: %s %s", string(site), r.Name)
                        continue
                    }
                    rr := r
                    m.Start(site, rr, func(ctx context.Context) { m.wg.Add(1); defer m.wg.Done(); runRSSJob(ctx, site, rr, impl) })
                }
            default:
                global.GetSlogger().Warnf("未知站点: %s", string(site))
            }
        }
    }
}

func validRSS(raw string) bool {
    if raw == "" { return false }
    u, err := url.Parse(raw)
    if err != nil { return false }
    if u.Scheme != "http" && u.Scheme != "https" { return false }
    host := u.Hostname()
    if host == "" { return false }
    if host == "rss.m-team.xxx" { return false }
    return true
}

// StopAll 取消所有任务并等待当前执行结束
func (m *Manager) StopAll() {
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
}

// StartAll 按配置启动所有任务（不做停止）
func (m *Manager) StartAll(cfg *models.Config) {
	for site, sc := range cfg.Sites {
		if sc.Enabled != nil && *sc.Enabled {
			switch site {
			case models.MTEAM:
				impl := internal.NewMteamImpl(context.Background())
				for _, r := range sc.RSS {
					rr := r
					m.Start(site, rr, func(ctx context.Context) { m.wg.Add(1); defer m.wg.Done(); runRSSJob(ctx, site, rr, impl) })
				}
			case models.HDSKY:
				impl := internal.NewHdskyImpl(context.Background())
				for _, r := range sc.RSS {
					rr := r
					m.Start(site, rr, func(ctx context.Context) { m.wg.Add(1); defer m.wg.Done(); runRSSJob(ctx, site, rr, impl) })
				}
			case models.CMCT:
				impl := internal.NewCmctImpl(context.Background())
				for _, r := range sc.RSS {
					rr := r
					m.Start(site, rr, func(ctx context.Context) { m.wg.Add(1); defer m.wg.Done(); runRSSJob(ctx, site, rr, impl) })
				}
			default:
				global.GetSlogger().Warnf("未知站点: %s", string(site))
			}
		}
	}
}

// 本地复制 runRSSJob 以便独立运行（与 cmd/rss.go 同步逻辑）
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

func getInterval(cfg models.RSSConfig) time.Duration {
	if cfg.IntervalMinutes <= 0 {
		// 从 DB 读取默认间隔
		if global.GlobalDB != nil {
			store := core.NewConfigStore(global.GlobalDB)
			gl, err := store.GetGlobalOnly()
			if err == nil && gl.DefaultIntervalMinutes > 0 {
				return time.Duration(gl.DefaultIntervalMinutes) * time.Minute
			}
		}
		return 10 * time.Minute
	}
	return time.Duration(cfg.IntervalMinutes) * time.Minute
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
	if err := ptSite.SendTorrentToQbit(ctx, cfg); err != nil {
		return err
	}
	return nil
}
