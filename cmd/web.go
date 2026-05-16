package cmd

import (
	"context"
	"errors"
	"fmt"
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

		userInfoRepo, err := v2.NewDBUserInfoRepo(global.GlobalDB.DB)
		if err != nil {
			global.GetSlogger().Warnf("初始化 UserInfoRepo 失败: %v", err)
		} else {
			userInfoService := v2.NewUserInfoService(v2.UserInfoServiceConfig{
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
			db := global.GlobalDB.DB
			digestFlush := func(ctx context.Context, confID uint, items []notify.DigestItem) {
				title, text := notify.CombineDigest(items)
				err := notifySvc.Push(ctx, app.Notification{
					Title: title, Text: text, SourceConfID: confID,
				})
				now := time.Now()
				ids := make([]uint, len(items))
				for i, it := range items {
					ids[i] = it.LogID
				}
				if err == nil {
					db.WithContext(ctx).Model(&models.RSSNotificationLog{}).
						Where("id IN ?", ids).
						Updates(map[string]any{
							"result":       "sent",
							"delivered_at": now,
							"updated_at":   now,
							"attempts":     gorm.Expr("attempts + 1"),
						})
					chatopsLogger().Infof("RSS digest 已投递 conf_id=%d items=%d", confID, len(items))
					return
				}
				nextRetry := now.Add(5 * time.Second)
				db.WithContext(ctx).Model(&models.RSSNotificationLog{}).
					Where("id IN ?", ids).
					Updates(map[string]any{
						"attempts":      gorm.Expr("attempts + 1"),
						"next_retry_at": nextRetry,
						"last_error":    err.Error(),
						"updated_at":    now,
					})
				chatopsLogger().Warnf("RSS digest 投递失败 conf_id=%d items=%d err=%v", confID, len(items), err)
			}
			digestBuf := notify.NewDigestBuffer(runtimeCtx, digestFlush)

			if rn, ok := rssNotifier.(interface {
				SetDigestBuffer(*notify.DigestBuffer)
				SetQuietFn(app.QuietLookupFunc)
			}); ok {
				rn.SetDigestBuffer(digestBuf)
				rn.SetQuietFn(quietFn)
			}

			retryWorker := app.NewRSSRetryWorker(global.GlobalDB.DB, notifySvc)
			go retryWorker.Run(runtimeCtx)

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
			for _, ch := range bs.channels {
				if setter, ok := ch.(interface {
					SetCallbackActionHandler(telegramadapter.CallbackActionHandler)
				}); ok {
					setter.SetCallbackActionHandler(callbackActions)
					registeredCallbackChannels++
				}
			}

			internal.SetRSSNotifier(&rssNotifierAdapter{inner: rssNotifier})
			chatopsLogger().Infof("RSS notifier 已就绪")
			chatopsLogger().Infof("RSS retry worker 已启动")
			chatopsLogger().Infof("RSS callback actions 已注册 channels=%d", registeredCallbackChannels)
		}

		srv := web.NewServer(store, mgr)
		if bs != nil {
			srv.SetChatOpsDeps(bs.Deps())
		}
		wireQATestHooks(srv, bs)
		if cfg, _ := store.Load(); cfg != nil {
			if cfg.Global.AutoStart && strings.TrimSpace(cfg.Global.DownloadDir) != "" {
				global.GetSlogger().Info("检测到自动启动配置，加载并启动任务")
				mgr.Reload(cfg)
			} else {
				global.GetSlogger().Info("自动启动未开启或下载目录为空，等待手动启动")
			}
		}

		shutdownDone := installShutdownHandler(srv, bs)

		global.GetSlogger().Infof("Web 服务启动于 %s", addr)
		go startVersionChecker()
		if err := srv.Serve(addr); err != nil {
			global.GetSlogger().Fatalf("Web 启动失败: %v", err)
		}
		<-shutdownDone
	},
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

// installShutdownHandler traps SIGINT/SIGTERM and tears down the ChatOps
// subsystem in reverse-dependency order, then shuts down the main HTTP server
// so Serve returns and the process exits cleanly without os.Exit.
// Returns a done channel closed after both shutdowns complete; main should
// block on it after Serve returns to ensure shutdown logs are flushed.
func installShutdownHandler(srv *web.Server, bs *chatopsBootstrap) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		ctx, cancel := context.WithTimeout(context.Background(), chatopsShutdownBudget)
		defer cancel()
		if bs != nil {
			if err := bs.Shutdown(ctx); err != nil {
				global.GetSlogger().Warnf("ChatOps 子系统关闭出现错误: %v", err)
			} else {
				global.GetSlogger().Info("ChatOps 子系统已优雅关闭")
			}
		}
		if srv != nil {
			if err := srv.Shutdown(ctx); err != nil {
				global.GetSlogger().Warnf("Web 服务关闭出现错误: %v", err)
			} else {
				global.GetSlogger().Info("Web 服务已优雅关闭")
			}
		}
	}()
	return done
}

// chatopsBootstrap holds wired ChatOps + Notify subsystem handles so the web
// command can inject them into web.Server and unwind them in reverse order
// during graceful shutdown. Per-channel Init failures are non-fatal: the rest
// of the system must remain available even when a single notification channel
// is misconfigured.
type chatopsBootstrap struct {
	deps      *web.ChatOpsDeps
	registry  *notify.Registry
	outbox    *notify.OutboxWorker
	manager   *liveNotifyManager
	channels  map[uint]notify.Channel
	sessions  *chatops.SessionStore
	chain     *chatops.MessageChain
	closeOnce sync.Once
}

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
	return len(b.channels)
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
		for confID, ch := range b.channels {
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
		Bindings:   &commandsBindingResolver{lookup: bindings},
		Sessions:   sessionStore,
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

type liveNotifyManager struct {
	mu       sync.RWMutex
	channels map[uint]notify.Channel
}

func newLiveNotifyManager(channels map[uint]notify.Channel) *liveNotifyManager {
	if channels == nil {
		channels = make(map[uint]notify.Channel)
	}
	return &liveNotifyManager{channels: channels}
}

func (m *liveNotifyManager) SetChannels(channels map[uint]notify.Channel) {
	if channels == nil {
		channels = make(map[uint]notify.Channel)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels = channels
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

func (l *dbBindingLookup) FindByChannelUser(ctx context.Context, channelType, channelUserID string) (chatops.BindingInfo, bool, error) {
	row, ok, err := l.findRow(ctx, channelType, channelUserID)
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

func (l *dbBindingLookup) findRow(ctx context.Context, channelType, channelUserID string) (models.ChannelBinding, bool, error) {
	var row models.ChannelBinding
	err := l.db.WithContext(ctx).
		Where("channel_type = ? AND channel_user_id = ?", channelType, channelUserID).
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

func (r *commandsBindingResolver) FindByChannelUser(ctx context.Context, channelType, channelUserID string) (uint, bool, error) {
	row, ok, err := r.lookup.findRow(ctx, channelType, channelUserID)
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

func (a *bindingConsumerAdapter) ConsumeCode(ctx context.Context, code, channelType, channelUserID string) error {
	_, err := a.svc.ConsumeCode(ctx, code, channelType, channelUserID)
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
