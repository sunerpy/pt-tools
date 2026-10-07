package cmd

import (
	"context"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/web"
)

// newSubscribeService 构造并启动订阅服务：搜索用 Web 搜索同一个 CachedSearchOrchestrator，下载种子文件用 UserInfoService
// 里注册的共享站点实例（共用限速），推送走 internal.PushTorrentToDownloader（磁盘与站点容量闸门不变），下载器取
// DownloaderManager 的共享实例，电影入库问整理入库配的媒体服务器，洗版替换交给整理入库；通知写进监控通知日志、由现有投递器发出。
// RSS 取到详情的种子经 internal.SetSubscriptionOffer 交给它。
func newSubscribeService(ctx context.Context, mgr *scheduler.Manager, rec *recognize.Service, organizer *organize.Service, sites *v2.UserInfoService, notifier *scheduler.MonitorNotifier) *subscribe.Service {
	if global.GlobalDB == nil || global.GlobalDB.DB == nil || mgr == nil || rec == nil {
		global.GetSlogger().Warn("订阅跳过初始化：数据库或媒体识别服务未就绪")
		return nil
	}
	cfg := subscribe.Config{
		DB:         global.GlobalDB.DB,
		Recognizer: rec,
		Sites: func(name string) (v2.Site, bool) {
			if sites == nil {
				return nil, false
			}
			return sites.GetSite(name)
		},
		SiteNames: func() []string {
			if sites == nil {
				return nil
			}
			return sites.ListSites()
		},
		Push:        internal.PushTorrentToDownloader,
		Downloaders: organizeDownloaders{mgr: mgr},
		Notify:      subscribeNotifier(notifier),
		Logger:      global.GetSlogger(),
		DoubanBase:  qaDoubanURL(),
	}
	if o := web.GetSearchOrchestrator(); o != nil {
		cfg.Search = o
	}
	if organizer != nil {
		cfg.Organizer, cfg.Library = organizer, organizer
	}
	svc := subscribe.New(cfg)
	svc.Start(ctx)
	internal.SetSubscriptionOffer(svc.Offer)
	global.GetSlogger().Info("订阅服务已启动")
	return svc
}

// stopSubscribeService 先断开 RSS 的入口再停后台。
func stopSubscribeService(svc *subscribe.Service) {
	internal.SetSubscriptionOffer(nil)
	if svc != nil {
		svc.Stop()
	}
}

// subscribeNotifier 把订阅的通知写进监控通知日志（source=media），不直接调用 Push、也不自建 Router。
func subscribeNotifier(n *scheduler.MonitorNotifier) subscribe.Notifier {
	if !n.Enabled() {
		return nil
	}
	return func(ctx context.Context, no subscribe.Notice) error {
		subject := no.Subject
		if len(subject) > 64 {
			subject = subject[:64]
		}
		text := no.Body
		if no.ImageURL != "" {
			text += "\n" + no.ImageURL
		}
		_, err := n.Enqueue(ctx, scheduler.MonitorNotifyEntry{
			Source: "media", Subject: subject, Kind: no.Kind, EventKey: no.EventKey,
			Title: no.Title, Text: text, ConfIDs: no.Channels,
		})
		return err
	}
}
