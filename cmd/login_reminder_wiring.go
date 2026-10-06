package cmd

import (
	"context"
	"time"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/cloakdriver/transport"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// loginReminderSender 把登录监控的通知交给 ChatOps 的 live 通道同步发送（PushSync）。
//
// 不自建 notify.Router：Router 会按通道配置 registry.Make 出新实例，QQ 会在已被占用的
// listen_addr 上再开反向 WS 服务（收不到 NapCat 连接），Telegram 会用同一个 bot token
// 再起一个长轮询，与 ChatOps 争抢更新并丢掉被它取走的命令。
// 也不用 Push：它在 live 发送失败后转写 outbox 并返回 nil，重试应由 MonitorNotificationLog 负责。
func loginReminderSender(bs *chatopsBootstrap) scheduler.MonitorSender {
	if bs == nil || bs.deps == nil || bs.deps.NotificationSvc == nil {
		return nil
	}
	svc := bs.deps.NotificationSvc
	return scheduler.MonitorSenderFunc(func(ctx context.Context, confID uint, title, text string) error {
		return svc.PushSync(ctx, app.Notification{Title: title, Text: text, SourceConfID: confID})
	})
}

type loginReminderDecryptor struct {
	store *core.ConfigStore
}

func (d loginReminderDecryptor) Decrypt(setting models.SiteSetting) (string, error) {
	if setting.CookieEncrypted == "" {
		return "", nil
	}
	return d.store.DecryptCookie(setting.CookieEncrypted)
}

type loginReminderResolver struct {
	registry  *v2.SiteRegistry
	decryptor loginReminderDecryptor
}

func (r loginReminderResolver) Resolve(setting models.SiteSetting) (*v2.SiteDefinition, v2.Site, error) {
	cookie, err := r.decryptor.Decrypt(setting)
	if err != nil {
		return nil, nil, err
	}
	site, err := r.registry.CreateSite(
		setting.Name,
		v2.SiteCredentials{
			Cookie:  cookie,
			APIKey:  setting.APIKey,
			Passkey: setting.Passkey,
		},
		setting.APIUrl,
	)
	if err != nil {
		return nil, nil, err
	}
	def, _ := v2.GetDefinitionRegistry().Get(setting.Name)
	return def, site, nil
}

// loginReminderUserInfo 把可能为空的 UserInfoService 转成接口；直接赋值会把 nil 指针装进非空接口。
func loginReminderUserInfo(svc *v2.UserInfoService) sitelogin.UserInfoFetcher {
	if svc == nil {
		return nil
	}
	return svc
}

// loginReminderFallback 构造 CloakBrowser 后备：每次探测从库里读三项配置（改配置不需要重启）；
// Cookie 与主通道同一取法（ConfigStore.SiteCookiePlaintext）；身份信息取最近一次成功探测保存的用户信息。
func loginReminderFallback(store *core.ConfigStore, userInfo *v2.UserInfoService) scheduler.FallbackProvider {
	if store == nil {
		return nil
	}
	return &transport.Provider{
		Settings: func() (transport.Settings, error) {
			snap, err := store.GetCloakConfig()
			if err != nil || !snap.HasToken || snap.Endpoint == "" || snap.ProfileID == "" {
				return transport.Settings{}, err
			}
			token, err := store.GetCloakToken()
			if err != nil {
				return transport.Settings{}, err
			}
			return transport.Settings{Endpoint: snap.Endpoint, Token: token, ProfileID: snap.ProfileID}, nil
		},
		Identity: func(ctx context.Context, siteName string) (transport.Identity, bool) {
			if userInfo == nil {
				return transport.Identity{}, false
			}
			info, err := userInfo.GetUserInfo(ctx, siteName)
			if err != nil {
				return transport.Identity{}, false
			}
			return transport.Identity{UserID: info.UserID, Username: info.Username}, true
		},
		Cookie: store.SiteCookiePlaintext,
	}
}

// wireLoginReminderMonitor 构造并启动登录提醒监控。userInfo 非空时探测经它进行（共用站点实例与限速器，
// 顺带刷新用户统计）；为空（仓库初始化失败）时退回 resolver 每次新建站点实例。
func wireLoginReminderMonitor(
	mgr *scheduler.Manager,
	store *core.ConfigStore,
	siteRegistry *v2.SiteRegistry,
	bs *chatopsBootstrap,
	userInfo *v2.UserInfoService,
) {
	if global.GlobalDB == nil || global.GlobalDB.DB == nil {
		global.GetSlogger().Warn("登录提醒监控器跳过初始化：数据库未就绪")
		return
	}
	db := global.GlobalDB.DB

	notifier := scheduler.NewMonitorNotifier(db, loginReminderSender(bs), nil, global.GetSlogger())
	if !notifier.Enabled() {
		global.GetSlogger().Warn("登录提醒监控器：ChatOps 通知服务不可用，提醒只记录决策、不发送")
	}

	decryptor := loginReminderDecryptor{store: store}
	resolver := loginReminderResolver{registry: siteRegistry, decryptor: decryptor}

	mon := scheduler.NewLoginReminderMonitor(scheduler.LoginReminderConfig{
		DB:        db,
		Notifier:  notifier,
		UserInfo:  loginReminderUserInfo(userInfo),
		Resolver:  resolver,
		Decryptor: decryptor,
		Fallback:  loginReminderFallback(store, userInfo),
		Logger:    global.GetSlogger(),
	})
	mgr.SetLoginReminderMonitor(mon)
	mon.Start()
	global.GetSlogger().Info("登录提醒监控器已初始化并启动")

	// 每日签到与登录探测共用单站锁和通知投递器。
	var window scheduler.AttendanceWindowSource
	if store != nil {
		window = store
	}
	att := scheduler.NewAttendanceMonitor(scheduler.AttendanceMonitorConfig{
		DB:       db,
		Sites:    attendanceSites(userInfo),
		Window:   window,
		Locker:   mon,
		Notifier: notifier,
		Logger:   global.GetSlogger(),
	})
	mgr.SetAttendanceMonitor(att)
	att.Start()
	global.GetSlogger().Info("每日签到监控器已初始化并启动")

	wireDailyReportJob(mgr, store, userInfo, notifier)
}

// wireDailyReportJob 构造并启动每日战报任务：与登录提醒共用通知投递器；用户数据仓库没起来时不启动。
func wireDailyReportJob(mgr *scheduler.Manager, store *core.ConfigStore, userInfo *v2.UserInfoService, notifier *scheduler.MonitorNotifier) {
	history, ok := userInfo.History()
	if !ok || store == nil {
		global.GetSlogger().Warn("每日战报跳过初始化：用户数据仓库不可用")
		return
	}
	job := scheduler.NewDailyReportJob(scheduler.DailyReportJobConfig{
		DB: global.GlobalDB.DB,
		Settings: func() (bool, string, []uint, error) {
			s, err := store.DailyReportSettings()
			return s.Enabled, s.Time, s.ChannelIDs, err
		},
		Offset: func() (time.Duration, error) {
			return store.DailyReportOffset(scheduler.DailyReportRand)
		},
		History:  history,
		Notifier: notifier,
		Logger:   global.GetSlogger(),
	})
	mgr.SetDailyReportJob(job)
	job.Start()
	global.GetSlogger().Info("每日战报任务已初始化并启动")
}

// attendanceSites 从 UserInfoService 取已注册的共享站点实例签到，与搜索、登录探测共用限速器。
func attendanceSites(svc *v2.UserInfoService) scheduler.AttendanceSites {
	return scheduler.AttendanceSitesFunc(func(name string) (v2.AttendanceCapable, bool) {
		if svc == nil {
			return nil, false
		}
		site, ok := svc.GetSite(name)
		if !ok {
			return nil, false
		}
		capable, ok := site.(v2.AttendanceCapable)
		return capable, ok
	})
}
