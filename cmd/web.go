package cmd

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/chatops"
	chatopscmds "github.com/sunerpy/pt-tools/internal/chatops/commands"
	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/version"
	"github.com/sunerpy/pt-tools/web"

	// Side-effect imports register notify channel adapters into the notify
	// default registry during process init so that bootstrapChatOps can
	// instantiate per-conf channel implementations.
	_ "github.com/sunerpy/pt-tools/internal/notify/adapter/qq"
	telegramadapter "github.com/sunerpy/pt-tools/internal/notify/adapter/telegram"
	_ "github.com/sunerpy/pt-tools/internal/notify/adapter/wecom"
)

var (
	host string
	port int
)

const (
	chatopsBindingCreator  = "system"
	chatopsOutboxInterval  = 10 * time.Second
	chatopsPushTimeout     = 5 * time.Second
	chatopsShutdownPerStep = 5 * time.Second
	chatopsShutdownBudget  = 15 * time.Second
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "启动 Web 管理界面（默认）",
	Run: func(cmd *cobra.Command, args []string) {
		version.CleanupOldBinary()

		if _, err := core.InitRuntime(); err != nil {
			color.Red("初始化失败: %v", err)
			return
		}

		global.GetSlogger().Infof("=== pt-tools 启动 === 版本: %s, 构建时间: %s", version.Version, version.BuildTime)

		siteRegistry := v2.NewSiteRegistry(global.GetLogger())
		store := core.NewConfigStore(global.GlobalDB)

		registeredSites := getRegisteredSitesFromRegistry(siteRegistry)
		global.GetSlogger().Infof("注册站点数量: %d", len(registeredSites))
		if err := store.SyncSites(registeredSites); err != nil {
			global.GetSlogger().Warnf("同步站点到数据库失败: %v", err)
		}

		gl, _ := store.GetGlobalOnly()
		if strings.TrimSpace(gl.DownloadDir) == "" {
			color.Yellow("当前未检测到 DB 配置，可通过 Web 进行初始化")
		} else {
			global.GetSlogger().Infof("配置加载完成: 下载目录=%s, 自动启动=%v, 下载限速=%v",
				gl.DownloadDir, gl.AutoStart, gl.DownloadLimitEnabled)
		}
		addr := fmt.Sprintf("%s:%d", host, port)
		mgr := scheduler.NewManager()
		mgr.InitFreeEndMonitor()

		var userInfoService *v2.UserInfoService
		userInfoRepo, err := v2.NewDBUserInfoRepo(global.GlobalDB.DB)
		if err != nil {
			global.GetSlogger().Warnf("初始化 UserInfoRepo 失败: %v", err)
		} else {
			userInfoService = v2.NewUserInfoService(v2.UserInfoServiceConfig{
				Repo:     userInfoRepo,
				CacheTTL: 5 * time.Minute,
				Logger:   global.GetLogger(),
			})
			web.InitUserInfoService(userInfoService)
			global.GetSlogger().Info("UserInfoService 初始化成功")

			searchOrchestrator := v2.NewSearchOrchestrator(v2.SearchOrchestratorConfig{
				Logger: global.GetLogger(),
			})
			cachedSearchOrchestrator := v2.NewCachedSearchOrchestrator(searchOrchestrator, v2.SearchCacheConfig{
				TTL:     10 * time.Minute,
				MaxSize: 500,
			})
			web.InitSearchOrchestrator(cachedSearchOrchestrator)
			global.GetSlogger().Info("SearchOrchestrator 初始化成功")

			web.InitSiteRegistry(siteRegistry)
			sites, siteErr := store.ListSites()
			if siteErr != nil {
				global.GetSlogger().Warnf("读取站点配置失败: %v", siteErr)
			} else {
				for siteGroup, siteConfig := range sites {
					if siteConfig.Enabled == nil || !*siteConfig.Enabled {
						continue
					}

					site, createErr := siteRegistry.CreateSite(
						string(siteGroup),
						v2.SiteCredentials{
							Cookie:  siteConfig.Cookie,
							APIKey:  siteConfig.APIKey,
							Passkey: siteConfig.Passkey,
						},
						siteConfig.APIUrl,
					)
					if createErr != nil {
						global.GetSlogger().Warnf("创建站点 %s 失败: %v", siteGroup, createErr)
						continue
					}

					userInfoService.RegisterSite(site)
					searchOrchestrator.RegisterSite(site)
					global.GetSlogger().Infof("站点 %s 已注册到 UserInfoService 和 SearchOrchestrator", siteGroup)
				}
			}
		}

		bootCtx, bootCancel := context.WithCancel(context.Background())
		defer bootCancel()
		bs, err := bootstrapChatOps(bootCtx, global.GlobalDB, mgr, store)
		if err != nil {
			global.GetSlogger().Warnf("ChatOps 子系统接线失败，跳过：%v", err)
			bs = nil
		}

		runtimeCtx, runtimeCancel := context.WithCancel(context.Background())
		defer runtimeCancel()
		// background 跟踪依赖 runtimeCtx 的后台 goroutine（RSS 重试、通道热重载）：
		// 关闭时先取消 context 并等它们退出，之后就不会再有人重建通知通道
		var background sync.WaitGroup

		if bs != nil {
			rssNotifier := app.NewRSSNotifier(global.GlobalDB.DB, bs.Deps().NotificationSvc)

			quietFn := func(confID uint) (string, string, error) {
				var conf models.NotificationConf
				if err := global.GlobalDB.DB.WithContext(runtimeCtx).
					First(&conf, confID).Error; err != nil {
					return "", "", err
				}
				return conf.QuietHoursStart, conf.QuietHoursEnd, nil
			}

			notifySvc := bs.Deps().NotificationSvc
			// 摘要刷写只发送仍是 pending 的行：被 filtered 抑制或已发出的不再发，只剩一条时保留按钮
			digestFlush := app.NewRSSDigestFlush(global.GlobalDB.DB, notifySvc, chatopsLogger().Infof)
			digestBuf := notify.NewDigestBuffer(runtimeCtx, digestFlush)

			if rn, ok := rssNotifier.(interface {
				SetDigestBuffer(*notify.DigestBuffer)
				SetQuietFn(app.QuietLookupFunc)
			}); ok {
				rn.SetDigestBuffer(digestBuf)
				rn.SetQuietFn(quietFn)
			}

			retryWorker := app.NewRSSRetryWorker(global.GlobalDB.DB, notifySvc)
			retryWorker.SetLogf(chatopsLogger().Warnf)
			background.Go(func() { retryWorker.Run(runtimeCtx) })

			fetcher := func(ctx context.Context, siteName, torrentID string) ([]byte, error) {
				orchestrator := web.GetSearchOrchestrator()
				if orchestrator == nil {
					return nil, errors.New("搜索编排器未初始化")
				}
				site := orchestrator.GetSite(siteName)
				if site == nil {
					return nil, fmt.Errorf("站点 %s 未注册", siteName)
				}
				return site.Download(ctx, torrentID)
			}
			callbackActions := app.NewRSSCallbackActions(global.GlobalDB.DB, fetcher)
			registeredCallbackChannels := 0
			bs.eachChannel(func(ch notify.Channel) {
				if setter, ok := ch.(interface {
					SetCallbackActionHandler(telegramadapter.CallbackActionHandler)
				}); ok {
					setter.SetCallbackActionHandler(callbackActions)
					registeredCallbackChannels++
				}
			})

			internal.SetRSSNotifier(&rssNotifierAdapter{inner: rssNotifier})
			chatopsLogger().Infof("RSS notifier 已就绪")
			chatopsLogger().Infof("RSS retry worker 已启动")
			chatopsLogger().Infof("RSS callback actions 已注册 channels=%d", registeredCallbackChannels)

			background.Go(func() { runChatOpsChannelReloader(runtimeCtx, global.GlobalDB.DB, bs, callbackActions) })
		}

		wireLoginReminderMonitor(mgr, store, siteRegistry, bs, userInfoService)
		wireBrushMonitor(mgr, userInfoService)
		wireTransferWorker(mgr, userInfoService)
		wireReseedWorker(mgr, userInfoService, store)

		srv := web.NewServer(store, mgr)
		if bs != nil {
			srv.SetChatOpsDeps(bs.Deps())
		}
		wireQATestHooks(srv, bs)
		if cfg, _ := store.Load(); cfg != nil {
			maybeAutoStartReload(mgr, cfg)
		}

		plan := shutdownPlan{
			stopBackground: func() {
				runtimeCancel()
				bootCancel()
				background.Wait()
			},
			scheduler: mgr,
			bs:        bs,
			srv:       srv,
		}
		if dm := mgr.GetDownloaderManager(); dm != nil {
			plan.downloaders = dm
		}
		shutdownDone := installShutdownHandler(plan)

		global.GetSlogger().Infof("Web 服务启动于 %s", addr)
		go startVersionChecker()
		if err := srv.Serve(addr); err != nil {
			global.GetSlogger().Fatalf("Web 启动失败: %v", err)
		}
		<-shutdownDone
	},
}

// startupReloader 抽象出启动重载能力，*scheduler.Manager 已满足；
// 抽出接口便于测试注入阻塞实现，验证重载是异步派发不阻塞 srv.Serve。
type startupReloader interface {
	Reload(*models.Config)
}

// maybeAutoStartReload 承载 Web 启动时「自动启动」的判定与派发。
// 采用 go mgr.Reload 异步派发以修复下载器不可达时的死锁：Reload 经
// createWithRetry 可阻塞约 3.5 分钟，同步执行会拖住 srv.Serve 使端口不可用。
func maybeAutoStartReload(mgr startupReloader, cfg *models.Config) (dispatched bool) {
	if cfg == nil {
		return false
	}
	if cfg.Global.AutoStart && strings.TrimSpace(cfg.Global.DownloadDir) != "" {
		global.GetSlogger().Info("检测到自动启动配置，异步加载并启动任务")
		go mgr.Reload(cfg)
		return true
	}
	global.GetSlogger().Info("自动启动未开启或下载目录为空，等待手动启动")
	return false
}

func init() {
	rootCmd.AddCommand(webCmd)
	webCmd.Flags().StringVar(&host, "host", "0.0.0.0", "服务绑定主机")
	webCmd.Flags().IntVar(&port, "port", 8080, "服务监听端口")
}

func getRegisteredSitesFromRegistry(registry *v2.SiteRegistry) []models.RegisteredSite {
	siteIDs := registry.List()
	defRegistry := v2.GetDefinitionRegistry()
	result := make([]models.RegisteredSite, 0, len(siteIDs))
	for _, id := range siteIDs {
		meta, ok := registry.Get(id)
		if !ok {
			continue
		}
		regSite := models.RegisteredSite{
			ID:             meta.ID,
			Name:           meta.Name,
			AuthMethod:     meta.AuthMethod.String(),
			DefaultBaseURL: meta.DefaultBaseURL,
		}
		if def, found := defRegistry.Get(id); found && def.Schema == v2.SchemaMTorrent {
			regSite.APIUrls = def.URLs
		}
		result = append(result, regSite)
	}
	return result
}

func startVersionChecker() {
	checker := version.GetChecker()
	logger := global.GetSlogger()

	checkVersion := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := checker.CheckForUpdates(ctx, version.CheckOptions{})
		if err != nil {
			logger.Warnf("版本检查失败: %v", err)
			return
		}
		if result.HasUpdate && len(result.NewReleases) > 0 {
			latest := result.NewReleases[0]
			logger.Infof("发现新版本 %s，当前版本 %s，请访问 %s 更新",
				latest.Version, result.CurrentVersion, latest.URL)
		}
	}

	checkVersion()

	ticker := time.NewTicker(version.CheckInterval)
	defer ticker.Stop()
	for range ticker.C {
		checkVersion()
	}
}

// shutdownPlan 是收到 SIGINT/SIGTERM 之后依次执行的关闭步骤，每一步都有时限：
//  1. stopBackground：取消 runtime 与 boot context，等 RSS 重试、通道热重载这些后台 goroutine 退出，
//     之后不会再有人往通知通道里发、也不会再把通道重建起来
//  2. scheduler：停掉 RSS 任务与各个监控器
//  3. downloaders：关闭下载器实例
//  4. bs：关闭通知通道、outbox 与会话
//  5. srv：HTTP 最后关，前面几步执行期间进行中的请求还能拿到响应
//
// 原来只做第 4、5 步：RSS 任务、监控器和下载器都没停，热重载还可能在关闭途中把通道建回来。
type shutdownPlan struct {
	stopBackground func()
	scheduler      interface{ StopAll() }
	downloaders    interface{ CloseAll() }
	bs             *chatopsBootstrap
	srv            interface {
		Shutdown(ctx context.Context) error
	}
	// stepTimeout 是单步的等待上限，0 取 chatopsShutdownPerStep。
	stepTimeout time.Duration
}

// installShutdownHandler 注册 SIGINT/SIGTERM，收到后按 plan 关闭，返回的 channel 在关闭完成后关闭；
// main 在 Serve 返回后等它，保证关闭日志写完。
// signal.Notify 在返回之前注册：信号若在 Serve 起来之前到达，同样会被这里接住，
// 而 web.Server.Shutdown 会让之后的 Serve 不再监听。
func installShutdownHandler(plan shutdownPlan) <-chan struct{} {
	done := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		defer close(done)
		<-sigCh
		ctx, cancel := context.WithTimeout(context.Background(), chatopsShutdownBudget)
		defer cancel()
		runShutdown(ctx, plan)
	}()
	return done
}

func runShutdown(ctx context.Context, plan shutdownPlan) {
	log := global.GetSlogger()
	step := plan.stepTimeout
	if step <= 0 {
		step = chatopsShutdownPerStep
	}
	if plan.stopBackground != nil {
		if !runBounded(ctx, step, plan.stopBackground) {
			log.Warnf("等待后台任务退出超时（%s），继续关闭", step)
		}
	}
	if plan.scheduler != nil {
		if runBounded(ctx, step, plan.scheduler.StopAll) {
			log.Info("调度器已停止")
		} else {
			log.Warnf("调度器停止超时（%s），RSS 任务或监控器仍在收尾，继续关闭", step)
		}
	}
	if plan.downloaders != nil {
		if !runBounded(ctx, step, plan.downloaders.CloseAll) {
			log.Warnf("关闭下载器超时（%s），继续关闭", step)
		}
	}
	if plan.bs != nil {
		if err := plan.bs.Shutdown(ctx); err != nil {
			log.Warnf("ChatOps 子系统关闭出现错误: %v", err)
		} else {
			log.Info("ChatOps 子系统已优雅关闭")
		}
	}
	if plan.srv != nil {
		if err := plan.srv.Shutdown(ctx); err != nil {
			log.Warnf("Web 服务关闭出现错误: %v", err)
		} else {
			log.Info("Web 服务已优雅关闭")
		}
	}
}

// runBounded 执行 fn，最多等 timeout 或 ctx 结束；等不到时返回 false，fn 留在后台跑完。
func runBounded(ctx context.Context, timeout time.Duration, fn func()) bool {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	case <-ctx.Done():
		return false
	}
}

// chatopsBootstrap holds wired ChatOps + Notify subsystem handles so the web
// command can inject them into web.Server and unwind them in reverse order
// during graceful shutdown. Per-channel Init failures are non-fatal: the rest
// of the system must remain available even when a single notification channel
// is misconfigured.
type chatopsBootstrap struct {
	deps     *web.ChatOpsDeps
	registry *notify.Registry
	outbox   *notify.OutboxWorker
	manager  *liveNotifyManager
	sessions *chatops.SessionStore
	chain    *chatops.MessageChain

	// mu 保护 channels 与 closed：热重载在后台 goroutine 里关旧建新、整表替换，
	// Shutdown 在信号处理 goroutine 里遍历关闭，原来两边都不加锁。
	mu       sync.Mutex
	channels map[uint]notify.Channel
	closed   bool // Shutdown 之后为真：热重载不再建通道

	closeOnce sync.Once
}

// errChatOpsShutDown 表示 ChatOps 已关闭，热重载不再执行。
var errChatOpsShutDown = errors.New("ChatOps 已关闭，跳过通道热重载")

func (b *chatopsBootstrap) Deps() *web.ChatOpsDeps {
	if b == nil {
		return nil
	}
	return b.deps
}

func (b *chatopsBootstrap) Chain() *chatops.MessageChain {
	if b == nil {
		return nil
	}
	return b.chain
}

func (b *chatopsBootstrap) ChannelCount() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.channels)
}

// eachChannel 在锁内逐个访问当前的通道实例。
func (b *chatopsBootstrap) eachChannel(fn func(notify.Channel)) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.channels {
		fn(ch)
	}
}

// Shutdown closes adapters first, then stops the outbox worker, then the
// session store. Each adapter Close is bounded by chatopsShutdownPerStep so a
// hung adapter cannot block process exit indefinitely.
func (b *chatopsBootstrap) Shutdown(ctx context.Context) error {
	if b == nil {
		return nil
	}
	var firstErr error
	b.closeOnce.Do(func() {
		log := chatopsLogger()
		// 先在锁内摘下通道表并标记关闭：之后的热重载直接返回，live 投递也不再拿到这些实例
		b.mu.Lock()
		b.closed = true
		channels := b.channels
		b.channels = map[uint]notify.Channel{}
		if b.manager != nil {
			b.manager.SetChannels(nil)
		}
		b.mu.Unlock()
		for confID, ch := range channels {
			stepCtx, cancel := context.WithTimeout(ctx, chatopsShutdownPerStep)
			if err := ch.Close(stepCtx); err != nil {
				log.Warnf("ChatOps 通道关闭失败 conf_id=%d type=%s: %v", confID, ch.Type(), err)
				if firstErr == nil {
					firstErr = err
				}
			}
			cancel()
		}
		if b.outbox != nil {
			b.outbox.Stop()
		}
		if b.sessions != nil {
			b.sessions.Stop()
		}
	})
	return firstErr
}

func bootstrapChatOps(
	ctx context.Context,
	tdb *models.TorrentDB,
	mgr *scheduler.Manager,
	store *core.ConfigStore,
) (*chatopsBootstrap, error) {
	if tdb == nil || tdb.DB == nil {
		return nil, errors.New("bootstrapChatOps: 数据库未初始化")
	}
	if mgr == nil {
		return nil, errors.New("bootstrapChatOps: scheduler.Manager 不能为空")
	}
	if store == nil {
		return nil, errors.New("bootstrapChatOps: ConfigStore 不能为空")
	}

	log := chatopsLogger()
	db := tdb.DB
	registry := notify.DefaultRegistry()

	auditSvc := app.NewAuditService(db)
	bindingSvc := app.NewBindingService(db, chatopsBindingCreator)
	liveManager := newLiveNotifyManager(nil)
	notifSvc := app.NewNotificationService(db, liveManager, chatopsPushTimeout)
	taskSvc := app.NewTaskService(mgr)
	siteSvc := app.NewSiteService(store, nil)
	torrentSvc := app.NewTorrentService(mgr.GetDownloaderManager())

	outbox := notify.NewOutboxWorker(db, registry, chatopsOutboxInterval)
	// 重试经运行中的通道投递：按配置另建实例会抢 QQ 端口、和 Telegram 长轮询抢更新，库里的配置也是密文
	outbox.SetLiveSender(outboxLiveSender{m: liveManager})
	outbox.Start(ctx)

	rateLimiter := chatops.NewRateLimiter()
	sessionStore := chatops.NewSessionStore()
	bindings := &dbBindingLookup{db: db}
	bindCoder := &bindingConsumerAdapter{svc: bindingSvc}
	auditRecorder := &auditRecorderAdapter{svc: auditSvc}

	chatopscmds.SetServices(&chatopscmds.Services{
		Task:       taskSvc,
		Torrent:    torrentSvc,
		Site:       siteSvc,
		Binding:    bindingSvc,
		Downloader: mgr.GetDownloaderManager(),
		RSSWizard:  &chatopsRSSWizardService{store: store, db: db},
		Bindings:   &commandsBindingResolver{lookup: bindings},
		Sessions:   sessionStore,
		Attendance: attendanceCommands{mgr: mgr},
	})

	chain := chatops.NewMessageChain(
		chatops.DefaultRegistry(),
		bindings,
		bindCoder,
		auditRecorder,
		rateLimiter,
		sessionStore,
		liveManager,
	)
	chain.SetLogf(log.Warnf)

	channels, err := initEnabledChannels(ctx, db, registry, chain.Process, log)
	if err != nil {
		return nil, fmt.Errorf("初始化通知通道失败: %w", err)
	}
	liveManager.SetChannels(channels)

	deps := &web.ChatOpsDeps{
		NotificationSvc: notifSvc,
		BindingSvc:      bindingSvc,
		AuditSvc:        auditSvc,
	}

	return &chatopsBootstrap{
		deps:     deps,
		registry: registry,
		outbox:   outbox,
		manager:  liveManager,
		channels: channels,
		sessions: sessionStore,
		chain:    chain,
	}, nil
}

// initEnabledChannels iterates enabled NotificationConf rows, calls Init, and
// wires the inbound handler. Per-channel error is logged and skipped so a
// misconfigured channel cannot block boot.
func initEnabledChannels(
	ctx context.Context,
	db *gorm.DB,
	registry *notify.Registry,
	inbound notify.InboundHandler,
	log loggerLike,
) (map[uint]notify.Channel, error) {
	out := make(map[uint]notify.Channel)

	var confs []models.NotificationConf
	if err := db.WithContext(ctx).Where("enabled = ?", true).Find(&confs).Error; err != nil {
		return out, fmt.Errorf("查询启用的通知通道失败: %w", err)
	}

	for i := range confs {
		conf := confs[i]
		ch, makeErr := registry.Make(conf.ChannelType)
		if makeErr != nil {
			log.Warnf("ChatOps 通道工厂未知 conf_id=%d type=%s: %v", conf.ID, conf.ChannelType, makeErr)
			continue
		}
		// Decrypt ConfigJSON (stored as base64 AES-GCM ciphertext) before
		// passing to the adapter, which expects plaintext JSON.
		if conf.ConfigJSON != "" {
			plain, derr := crypto.Decrypt(conf.ConfigJSON)
			if derr != nil {
				log.Warnf("ChatOps 通道配置解密失败 conf_id=%d type=%s: %v", conf.ID, conf.ChannelType, derr)
				continue
			}
			conf.ConfigJSON = string(plain)
		}
		if err := ch.Init(ctx, &conf); err != nil {
			log.Warnf("ChatOps 通道初始化失败 conf_id=%d type=%s: %v", conf.ID, conf.ChannelType, err)
			continue
		}
		if ch.SupportsInbound() && inbound != nil {
			ch.OnInbound(inbound)
		}
		out[conf.ID] = ch
		log.Infof("ChatOps 通道已就绪 conf_id=%d type=%s name=%s", conf.ID, conf.ChannelType, conf.Name)
	}
	return out, nil
}

func chatopsLogger() loggerLike {
	if global.GetLogger() == nil {
		return nopLogger{}
	}
	return global.GetSlogger()
}

type loggerLike interface {
	Infof(template string, args ...any)
	Warnf(template string, args ...any)
}

type nopLogger struct{}

func (nopLogger) Infof(string, ...any) {}
func (nopLogger) Warnf(string, ...any) {}

type chatopsRSSWizardService struct {
	store *core.ConfigStore
	db    *gorm.DB
}

func (s *chatopsRSSWizardService) AppendRSSToSite(siteName string, entry models.RSSConfig) (models.RSSConfig, error) {
	return s.store.AppendRSSToSite(siteName, entry)
}

func (s *chatopsRSSWizardService) ListRSSForSite(siteName string) ([]models.RSSConfig, error) {
	return s.store.ListRSSForSite(siteName)
}

func (s *chatopsRSSWizardService) DeleteRSSFromSite(siteName string, rssID uint) (models.RSSConfig, error) {
	return s.store.DeleteRSSFromSite(siteName, rssID)
}

func (s *chatopsRSSWizardService) ListDownloaders(ctx context.Context) ([]chatopscmds.DownloaderOption, error) {
	var rows []models.DownloaderSetting
	if err := s.db.WithContext(ctx).Order("is_default DESC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]chatopscmds.DownloaderOption, 0, len(rows))
	for _, row := range rows {
		out = append(out, chatopscmds.DownloaderOption{ID: row.ID, Name: row.Name, IsDefault: row.IsDefault})
	}
	return out, nil
}

func (s *chatopsRSSWizardService) ListFilterRules(ctx context.Context) ([]chatopscmds.IDNameOption, error) {
	var rows []models.FilterRule
	if err := s.db.WithContext(ctx).Where("enabled = ?", true).Order("priority ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]chatopscmds.IDNameOption, 0, len(rows))
	for _, row := range rows {
		out = append(out, chatopscmds.IDNameOption{ID: row.ID, Name: row.Name})
	}
	return out, nil
}

func (s *chatopsRSSWizardService) ListNotificationChannels(ctx context.Context) ([]chatopscmds.IDNameOption, error) {
	var rows []models.NotificationConf
	if err := s.db.WithContext(ctx).Where("enabled = ?", true).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]chatopscmds.IDNameOption, 0, len(rows))
	for _, row := range rows {
		out = append(out, chatopscmds.IDNameOption{ID: row.ID, Name: row.Name})
	}
	return out, nil
}

type liveNotifyManager struct {
	mu       sync.RWMutex
	channels map[uint]notify.Channel
}

func newLiveNotifyManager(channels map[uint]notify.Channel) *liveNotifyManager {
	m := &liveNotifyManager{}
	m.SetChannels(channels)
	return m
}

// SetChannels 换上一张新的通道表。存的是**副本**，不是调用方传进来的那张 map。
//
// 这里的读者（ChannelState / Send / Reply）只拿读锁：它防得住 SetChannels 的整表替换，
// 防不住别人不拿锁地原地改同一个 map。bootstrapChatOps 曾把同一张 map 同时交给
// chatopsBootstrap.channels 和这里，热重载又在那张 map 上原地 delete / insert ——
// 与读者并发就是 fatal error: concurrent map read and map write。存副本之后，
// 调用方手里那张 map 怎么改都碰不到这里。
func (m *liveNotifyManager) SetChannels(channels map[uint]notify.Channel) {
	next := make(map[uint]notify.Channel, len(channels))
	maps.Copy(next, channels)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels = next
}

// ChannelState 实现 app.ChannelStater：把这一条通道的实例状态告诉上层。
//
// **不把 Healthy() 当「已连接」**：这四个适配器的 Healthy() 含义大多只是构造/启动成功
// （QQ 绑上端口就 true，而 NapCat 没握手时发送会明确失败；Telegram 跟着最近一次 getUpdates 的结果，
// 令牌失效、网络断开时为 false；Webhook 只判 config != nil；WeCom 恒 true）。所以 Healthy() 只够说「运行中」。
// 只有实现了 notify.LinkStater 且确认对端接上的通道，才报「已连接」。
//
// map 里没有这一条时返回空串，由 app 层结合「配置启不启用」判成异常还是停用。
func (m *liveNotifyManager) ChannelState(confID uint) string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	ch, ok := m.channels[confID]
	m.mu.RUnlock()
	if !ok || ch == nil {
		return ""
	}
	if !ch.Healthy() {
		return app.ChannelStateError
	}
	if stater, ok := ch.(notify.LinkStater); ok {
		if stater.LinkState() == notify.LinkConnected {
			return app.ChannelStateConnected
		}
		/* 在跑、但对端还没接上（QQ 等 NapCat 握手）—— 不是故障，也还不是「已连接」 */
		return app.ChannelStateRunning
	}
	return app.ChannelStateRunning
}

func (m *liveNotifyManager) Send(ctx context.Context, confID uint, n app.Notification) error {
	if m == nil {
		return errors.New("live notify manager 未初始化")
	}
	m.mu.RLock()
	ch, ok := m.channels[confID]
	m.mu.RUnlock()
	if !ok || ch == nil {
		return fmt.Errorf("通知通道未运行 conf_id=%d", confID)
	}
	if err := ch.Send(ctx, notify.Notification{
		Title:        n.Title,
		Text:         n.Text,
		SourceConfID: n.SourceConfID,
		UserID:       n.UserID,
		Targets:      n.Targets,
		Buttons:      n.Buttons,
	}); err != nil {
		chatopsLogger().Warnf("实时通知投递失败 conf_id=%d type=%s: %v", confID, ch.Type(), err)
		return err
	}
	chatopsLogger().Infof("实时通知投递成功 conf_id=%d type=%s", confID, ch.Type())
	return nil
}

// outboxLiveSender 让 notify.OutboxWorker 经 liveNotifyManager 投递重试。
type outboxLiveSender struct{ m *liveNotifyManager }

func (s outboxLiveSender) Send(ctx context.Context, confID uint, n notify.Notification) error {
	return s.m.Send(ctx, confID, app.Notification{
		Title:        n.Title,
		Text:         n.Text,
		SourceConfID: confID,
		UserID:       n.UserID,
		Targets:      n.Targets,
		Buttons:      n.Buttons,
	})
}

// Reply implements chatops.Replier — sends a reply to the inbound user via the
// live channel that received the message. Used by MessageChain.tryReply.
func (m *liveNotifyManager) Reply(ctx context.Context, msg notify.InboundMessage, reply chatops.Reply) error {
	if m == nil {
		return errors.New("live notify manager 未初始化")
	}
	if reply.SilentDrop {
		return nil
	}
	if reply.Text == "" && len(reply.Buttons) == 0 {
		return nil
	}
	m.mu.RLock()
	ch, ok := m.channels[msg.SourceConfID]
	m.mu.RUnlock()
	if !ok || ch == nil {
		chatopsLogger().Warnf("ChatOps 回复失败：通道未运行 conf_id=%d type=%s", msg.SourceConfID, msg.ChannelType)
		return fmt.Errorf("通知通道未运行 conf_id=%d", msg.SourceConfID)
	}
	targets := map[string]string{"chat_id": msg.ChatID}
	if msg.MessageType != "" {
		targets["message_type"] = msg.MessageType
	}
	if err := ch.Send(ctx, notify.Notification{
		Text:         reply.Text,
		ChannelType:  msg.ChannelType,
		SourceConfID: msg.SourceConfID,
		UserID:       msg.ChannelUserID,
		Targets:      targets,
	}); err != nil {
		chatopsLogger().Warnf("ChatOps 回复发送失败 conf_id=%d user=%s: %v", msg.SourceConfID, msg.ChannelUserID, err)
		return err
	}
	chatopsLogger().Infof("ChatOps 回复已发送 conf_id=%d user=%s text=%q", msg.SourceConfID, msg.ChannelUserID, truncateLog(reply.Text, 80))
	return nil
}

func truncateLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// dbBindingLookup satisfies chatops.BindingLookup by querying ChannelBinding rows.
type dbBindingLookup struct {
	db *gorm.DB
}

func (l *dbBindingLookup) FindByChannelUser(ctx context.Context, confID uint, channelType, channelUserID string) (chatops.BindingInfo, bool, error) {
	row, ok, err := l.findRow(ctx, confID, channelType, channelUserID)
	if err != nil || !ok {
		return chatops.BindingInfo{}, ok, err
	}
	return chatops.BindingInfo{
		ID:            row.ID,
		ConfID:        row.NotificationConfID,
		ChannelType:   row.ChannelType,
		ChannelUserID: row.ChannelUserID,
		ReplyLang:     row.ReplyLang,
		PtAdmin:       row.PtAdmin,
		Allowed:       row.Allowed,
	}, true, nil
}

func (l *dbBindingLookup) findRow(ctx context.Context, confID uint, channelType, channelUserID string) (models.ChannelBinding, bool, error) {
	var row models.ChannelBinding
	err := l.db.WithContext(ctx).
		Where("notification_conf_id = ? AND channel_type = ? AND channel_user_id = ?", confID, channelType, channelUserID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.ChannelBinding{}, false, nil
	}
	if err != nil {
		return models.ChannelBinding{}, false, fmt.Errorf("查询 channel_binding 失败: %w", err)
	}
	return row, true, nil
}

// commandsBindingResolver adapts dbBindingLookup to the
// chatops/commands.BindingResolver shape (uint return for /unbind).
type commandsBindingResolver struct {
	lookup *dbBindingLookup
}

func (r *commandsBindingResolver) FindByChannelUser(ctx context.Context, confID uint, channelType, channelUserID string) (uint, bool, error) {
	row, ok, err := r.lookup.findRow(ctx, confID, channelType, channelUserID)
	if err != nil || !ok {
		return 0, ok, err
	}
	return row.ID, true, nil
}

// bindingConsumerAdapter narrows app.BindingService.ConsumeCode (DTO return) to
// chatops.BindCodeConsumer (error only).
type bindingConsumerAdapter struct {
	svc app.BindingService
}

func (a *bindingConsumerAdapter) ConsumeCode(ctx context.Context, code string, confID uint, channelType, channelUserID string) error {
	_, err := a.svc.ConsumeCode(ctx, code, confID, channelType, channelUserID)
	return err
}

// auditRecorderAdapter forwards the field-compatible chatops.AuditEntry to
// app.AuditService.Record so the two packages stay decoupled.
type auditRecorderAdapter struct {
	svc app.AuditService
}

func (a *auditRecorderAdapter) Record(ctx context.Context, e chatops.AuditEntry) error {
	return a.svc.Record(ctx, app.AuditEntry{
		NotificationConfID: e.NotificationConfID,
		ChannelType:        e.ChannelType,
		ChannelUserID:      e.ChannelUserID,
		Command:            e.Command,
		Args:               e.Args,
		Result:             e.Result,
		LatencyMs:          e.LatencyMs,
	})
}

type rssNotifierAdapter struct {
	inner app.RSSNotifier
}

func (a *rssNotifierAdapter) NotifyNewItem(ctx context.Context, ev internal.RSSItemNotice) error {
	return a.inner.NotifyNewItem(ctx, app.RSSItemEvent{
		RSS:       ev.RSS,
		FeedItem:  ev.FeedItem,
		SiteName:  ev.SiteName,
		TorrentID: ev.TorrentID,
	})
}

func (a *rssNotifierAdapter) NotifyFilteredItem(ctx context.Context, ev internal.RSSFilteredNotice) error {
	return a.inner.NotifyFilteredItem(ctx, app.RSSFilteredEvent{
		RSS:       ev.RSS,
		Torrent:   ev.Torrent,
		Rule:      ev.Rule,
		SiteName:  ev.SiteName,
		TorrentID: ev.TorrentID,
	})
}

// runChatOpsChannelReloader 订阅 events.ConfigChanged{Source:"notification"}，
// 收到事件后重建 bs.channels：先逐个 Close 旧实例（带超时避免阻塞），
// 再调用 initEnabledChannels 从 DB 重新加载 enabled=true 的所有通道并 Init，
// 最后重新注册 RSS callback action handler。事件冒泡式 fan-out 由 events.Publish
// 保证：单条事件触发一次完整重建。
func runChatOpsChannelReloader(
	ctx context.Context,
	db *gorm.DB,
	bs *chatopsBootstrap,
	callbackActions telegramadapter.CallbackActionHandler,
) {
	if bs == nil || db == nil {
		return
	}
	id, eventCh, cancel := events.Subscribe(64)
	defer cancel()
	log := chatopsLogger()
	log.Infof("ChatOps 通道热重载订阅已就绪 sub=%s", id)
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-eventCh:
			if !ok {
				return
			}
			if ev.Type != events.ConfigChanged || ev.Source != "notification" {
				continue
			}
			if err := reloadChatOpsChannels(ctx, db, bs, callbackActions); err != nil {
				log.Warnf("ChatOps 通道热重载失败: %v", err)
				continue
			}
			log.Infof("ChatOps 通道已热重载")
		}
	}
}

// reloadChatOpsChannels 关闭所有旧通道实例并从 DB 重新加载所有 enabled=true 的通道。
// 采用全量重建策略以匹配 core/config_store.go 的全局 reload 模式：避免逐通道 dirty 状态追踪，
// 且通道 Init 是轻量操作（TG 仅打开新 HTTPS client，QQ 仅重绑 WS 端口）。
func reloadChatOpsChannels(
	ctx context.Context,
	db *gorm.DB,
	bs *chatopsBootstrap,
	callbackActions telegramadapter.CallbackActionHandler,
) error {
	log := chatopsLogger()

	// 整个「关旧、建新、换表」在锁内完成，不和 Shutdown 交错；Shutdown 之后不再建通道
	bs.mu.Lock()
	defer bs.mu.Unlock()
	if bs.closed {
		return errChatOpsShutDown
	}

	for confID, ch := range bs.channels {
		stepCtx, stepCancel := context.WithTimeout(ctx, chatopsShutdownPerStep)
		if err := ch.Close(stepCtx); err != nil {
			log.Warnf("ChatOps 通道关闭失败 conf_id=%d type=%s: %v", confID, ch.Type(), err)
		} else {
			log.Infof("ChatOps 通道已下线 conf_id=%d type=%s", confID, ch.Type())
		}
		stepCancel()
	}

	// 不在旧 map 上原地 delete / insert，而是整张换新：旧 map 此刻可能仍被别处读着。
	// initEnabledChannels 每次都返回一张新建的 map，填好之后再一次性换上去。
	var inbound notify.InboundHandler
	if bs.chain != nil {
		inbound = bs.chain.Process
	}
	next, err := initEnabledChannels(ctx, db, bs.registry, inbound, log)
	if err != nil {
		// 旧实例上面已经全部关掉：换上一张空表，不能再让 manager 往已关闭的通道投递
		empty := make(map[uint]notify.Channel)
		bs.channels = empty
		bs.manager.SetChannels(empty)
		return err
	}

	if callbackActions != nil {
		for _, ch := range next {
			if setter, ok := ch.(interface {
				SetCallbackActionHandler(telegramadapter.CallbackActionHandler)
			}); ok {
				setter.SetCallbackActionHandler(callbackActions)
			}
		}
	}

	bs.channels = next
	bs.manager.SetChannels(next)
	return nil
}
