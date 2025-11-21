package cmd

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

type contextKey string

const modeKey contextKey = "mode"

// 单次运行模式
func genTorrentsWithRSSOnce(ctx context.Context) error {
	// 在上下文中设置模式
	ctx = context.WithValue(ctx, modeKey, "single")
	var wg sync.WaitGroup // 定义 WaitGroup，用于等待所有 goroutine 结束
	scMap, _ := core.NewConfigStore(global.GlobalDB).ListSites()
	for k, cfg := range scMap {
		if cfg.Enabled != nil && *cfg.Enabled {
			switch k {
			case models.MTEAM:
				siteImpl := internal.NewMteamImpl(ctx)
				wg.Add(1)
				go func() {
					defer wg.Done()
					runSiteJobs(ctx, k, cfg, siteImpl)
				}()
			case models.HDSKY:
				siteImpl := internal.NewHdskyImpl(ctx)
				wg.Add(1)
				go func() {
					defer wg.Done()
					runSiteJobs(ctx, k, cfg, siteImpl)
				}()
			case models.CMCT:
				siteImpl := internal.NewCmctImpl(ctx)
				wg.Add(1)
				go func() {
					defer wg.Done()
					runSiteJobs(ctx, k, cfg, siteImpl)
				}()
			default:
				sLogger().Warnf("未找到站点: %s 配置,跳过任务", string(k))
			}
		}
	}
	wg.Wait() // 等待所有 Goroutine 完成
	sLogger().Info("genTorrentsWithRSSOnce: 所有任务已完成")
	return nil
}

// 持续运行模式
func genTorrentsWithRSS(ctx context.Context) error {
	// 在上下文中设置模式
	ctx = context.WithValue(ctx, modeKey, "persistent")
	store := core.NewConfigStore(global.GlobalDB)
	gl, _ := store.GetGlobalOnly()
	if strings.TrimSpace(gl.DownloadDir) == "" {
		sLogger().Warn("下载目录为空，任务等待配置完善后再启动")
		<-ctx.Done()
		return nil
	}
	m := scheduler.NewManager()
	scMap2, _ := store.ListSites()
	cfg := &models.Config{Global: gl, Sites: scMap2}
	m.Reload(cfg)
	<-ctx.Done()
	return nil
}

// 站点任务
func runSiteJobs[T models.ResType](ctx context.Context, siteName models.SiteGroup, siteCfg models.SiteConfig, siteImpl internal.PTSiteInter[T]) {
	var siteWg sync.WaitGroup // 用于等待该站点的所有 RSS 任务结束
	for _, rss := range siteCfg.RSS {
		siteWg.Add(1)
		go func(rss models.RSSConfig) {
			defer siteWg.Done()
			runRSSJob(ctx, siteName, rss, siteImpl)
		}(rss)
	}
	siteWg.Wait() // 等待该站点的所有 RSS 任务结束
}

func getInterval(cfg models.RSSConfig) time.Duration {
	if cfg.IntervalMinutes <= 0 {
		gl, _ := core.NewConfigStore(global.GlobalDB).GetGlobalOnly()
		if gl.DefaultIntervalMinutes > 0 {
			return time.Duration(gl.DefaultIntervalMinutes) * time.Minute
		}
		return 10 * time.Minute
	}
	return time.Duration(cfg.IntervalMinutes) * time.Minute
}

// RSS 任务
func runRSSJob[T models.ResType](ctx context.Context, siteName models.SiteGroup, cfg models.RSSConfig, siteImpl internal.PTSiteInter[T]) {
	ticker := time.NewTicker(getInterval(cfg))
	defer ticker.Stop()
	executeTask(ctx, siteName, cfg, siteImpl)
	// 判断运行模式
	mode, _ := ctx.Value(modeKey).(string)
	if mode == "single" {
		sLogger().Infof("站点:%s 单次任务执行完成", cfg.Name)
		return
	}
	for {
		select {
		case <-ctx.Done(): // 上下文取消时退出
			sLogger().Infof("站点:%s 任务取消", cfg.Name)
			return
		case <-ticker.C: // 持续运行模式下的定时任务
			executeTask(ctx, siteName, cfg, siteImpl)
		}
	}
}

// 执行单次任务
func executeTask[T models.ResType](ctx context.Context, siteName models.SiteGroup, cfg models.RSSConfig, siteImpl internal.PTSiteInter[T]) {
	sLogger().Infof("开始任务: %s", cfg.Name)
	if err := processRSS(ctx, siteName, cfg, siteImpl); err != nil {
		sLogger().Errorf("站点: %s 任务执行失败, %v", cfg.Name, err)
	} else {
		sLogger().Infof("站点: %s 任务执行完成", cfg.Name)
	}
}

// 处理单个 RSS 配置
func processRSS[T models.ResType](ctx context.Context, siteName models.SiteGroup, cfg models.RSSConfig, ptSite internal.PTSiteInter[T]) error {
	if err := internal.FetchAndDownloadFreeRSS(ctx, siteName, ptSite, cfg); err != nil {
		return err
	}
	if err := ptSite.SendTorrentToQbit(ctx, cfg); err != nil {
		return err
	}
	return nil
}
