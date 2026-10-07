package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
	chatopscmds "github.com/sunerpy/pt-tools/internal/chatops/commands"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
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
	chatopscmds.SetSubscribeService(chatopsSubscribe{svc: svc})
	global.GetSlogger().Info("订阅服务已启动")
	return svc
}

// stopSubscribeService 先断开 RSS 与 ChatOps 的入口再停后台。
func stopSubscribeService(svc *subscribe.Service) {
	internal.SetSubscriptionOffer(nil)
	chatopscmds.SetSubscribeService(nil)
	if svc != nil {
		svc.Stop()
	}
}

// chatopsSubscribe 把订阅服务接到 /sub、/subs。
type chatopsSubscribe struct{ svc *subscribe.Service }

func (c chatopsSubscribe) Search(ctx context.Context, keyword string) ([]chatopscmds.SubscribeCandidate, error) {
	var items []subscribe.ExploreItem
	for _, kind := range []string{models.MediaKindMovie, models.MediaKindTV} {
		page, err := c.svc.Explore(ctx, kind, subscribe.ExploreSearch, 1, keyword)
		if err != nil {
			return nil, err
		}
		items = append(items, page.Items...)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Popularity > items[j].Popularity })
	out := make([]chatopscmds.SubscribeCandidate, 0, len(items))
	for _, it := range items {
		out = append(out, chatopscmds.SubscribeCandidate{Kind: it.MediaType, TMDBID: it.ID, Title: it.Title, Year: it.Year, Subscribed: it.Subscribed})
	}
	return out, nil
}

func (c chatopsSubscribe) Subscribe(ctx context.Context, kind string, tmdbID, season int) (string, error) {
	if kind != tmdb.KindMovie && kind != tmdb.KindTV {
		return "", fmt.Errorf("条目类型无效: %q", kind)
	}
	sub, err := c.svc.CreateSubscription(ctx, subscribe.SubscriptionInput{MediaType: kind, TMDBID: tmdbID, Season: season}, models.MediaSubFromChatOps)
	if err != nil {
		return "", err
	}
	return subscriptionName(sub), nil
}

func (c chatopsSubscribe) List(ctx context.Context) ([]chatopscmds.SubscribeLine, error) {
	subs, err := c.svc.Subscriptions(ctx, subscribe.SubscriptionQuery{})
	if err != nil {
		return nil, err
	}
	out := make([]chatopscmds.SubscribeLine, 0, len(subs))
	for i := range subs {
		out = append(out, chatopscmds.SubscribeLine{
			Title: subscriptionName(&subs[i].MediaSubscription), Status: subscriptionStatus(subs[i].Status), Progress: progressText(&subs[i]),
		})
	}
	return out, nil
}

func subscriptionName(sub *models.MediaSubscription) string {
	name := sub.Title
	if sub.Year > 0 {
		name += fmt.Sprintf(" (%d)", sub.Year)
	}
	if sub.MediaType == models.MediaKindTV {
		name += fmt.Sprintf(" 第 %d 季", sub.Season)
	}
	return name
}

func subscriptionStatus(status string) string {
	switch status {
	case models.MediaSubActive:
		return "在找"
	case models.MediaSubPaused:
		return "暂停"
	case models.MediaSubPending:
		return "待确认"
	case models.MediaSubDone:
		return "完成"
	}
	return status
}

func progressText(v *subscribe.SubscriptionView) string {
	p := v.Progress
	if p == nil {
		return ""
	}
	if v.MediaType == models.MediaKindMovie {
		switch {
		case p.InLibrary > 0:
			return "已入库"
		case p.Downloading > 0:
			return "下载中"
		}
		return "还没下载"
	}
	parts := []string{fmt.Sprintf("已入库 %d/%d", p.InLibrary, p.Total)}
	if p.Downloading > 0 {
		parts = append(parts, fmt.Sprintf("下载中 %d", p.Downloading))
	}
	if len(p.Missing) > 0 {
		parts = append(parts, fmt.Sprintf("缺 %d", len(p.Missing)))
	}
	return strings.Join(parts, "，")
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
