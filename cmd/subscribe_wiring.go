package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
	chatopscmds "github.com/sunerpy/pt-tools/internal/chatops/commands"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/internal/redact"
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
	if organizer != nil {
		// 追剧推送或认领的种子整理进追剧指定的媒体库
		organizer.SetLibraryFor(svc.LibraryFor)
	}
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

// Follow 建追剧：只认剧集，按追剧的默认设置（与网页、App 新建的一样）。
func (c chatopsSubscribe) Follow(ctx context.Context, tmdbID, season int) (string, error) {
	in := subscribe.SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: tmdbID, Season: season}
	subscribe.FollowDefaults(&in)
	sub, err := c.svc.CreateSubscription(ctx, in, models.MediaSubFromChatOps)
	if err != nil {
		return "", err
	}
	return subscriptionName(sub) + "，" + subscribe.Describe(sub), nil
}

func (c chatopsSubscribe) List(ctx context.Context) ([]chatopscmds.SubscribeLine, error) {
	subs, err := c.svc.Subscriptions(ctx, subscribe.SubscriptionQuery{})
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := make([]chatopscmds.SubscribeLine, 0, len(subs))
	for i := range subs {
		out = append(out, chatopscmds.SubscribeLine{
			ID: subs[i].ID, Title: subscriptionName(&subs[i].MediaSubscription), Status: subscriptionStatus(subs[i].Status),
			Progress: progressText(&subs[i]), Next: nextSearchText(&subs[i].MediaSubscription, now),
		})
	}
	return out, nil
}

// Runs 是订阅的名字与最近几次运行；说明里的凭据去掉（回复会发到聊天软件）。
func (c chatopsSubscribe) Runs(ctx context.Context, id uint, limit int) (string, []chatopscmds.RunLine, error) {
	d, err := c.svc.Subscription(ctx, id)
	if err != nil {
		return "", nil, err
	}
	page, err := c.svc.Runs(ctx, id, limit, 0)
	if err != nil {
		return "", nil, err
	}
	out := make([]chatopscmds.RunLine, 0, len(page.Items))
	for _, r := range page.Items {
		line := chatopscmds.RunLine{At: r.StartedAt, Trigger: r.Trigger, Status: r.Status, Summary: redact.Text(r.Summary)}
		for _, st := range r.Steps {
			line.Steps = append(line.Steps, redact.Text(st.Text))
		}
		out = append(out, line)
	}
	return subscriptionName(&d.MediaSubscription), out, nil
}

// Unlock 让追剧的一集不受版本锁限制，并尽快搜一次。
func (c chatopsSubscribe) Unlock(ctx context.Context, id uint, episode int) (string, error) {
	if _, err := c.svc.UnlockEpisode(ctx, id, episode, true); err != nil {
		return "", err
	}
	d, err := c.svc.Subscription(ctx, id)
	if err != nil {
		return "", err
	}
	return subscriptionName(&d.MediaSubscription), nil
}

// nextSearchText 是下次主动搜索的时间（月-日 时:分）；暂停、完成、待确认的是空，已经到点的是「马上」。
func nextSearchText(sub *models.MediaSubscription, now time.Time) string {
	if sub.Status != models.MediaSubActive || sub.NextSearchAt == nil {
		return ""
	}
	if !sub.NextSearchAt.After(now) {
		return "马上"
	}
	return sub.NextSearchAt.Local().Format("01-02 15:04")
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
		miss := fmt.Sprintf("缺 %d", len(p.Missing))
		if p.Waiting > 0 {
			// 在等锁住的版本的也算缺
			miss += fmt.Sprintf("（等版本 %d）", p.Waiting)
		}
		parts = append(parts, miss)
	}
	if p.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("不追 %d", p.Skipped))
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
			Title: no.Title, Text: text, ConfIDs: no.Channels, Event: no.Event,
		})
		return err
	}
}
