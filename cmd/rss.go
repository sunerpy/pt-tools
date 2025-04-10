package cmd

import (
	"context"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/config"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
)

// 单次运行模式
func genTorrentsWithRSSOnce(ctx context.Context) error {
	// 在上下文中设置模式
	ctx = context.WithValue(ctx, "mode", "single")
	var wg sync.WaitGroup // 定义 WaitGroup，用于等待所有 goroutine 结束
	for k, cfg := range global.GlobalCfg.Sites {
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
	ctx = context.WithValue(ctx, "mode", "persistent")
	var wg sync.WaitGroup // 定义 WaitGroup，用于等待所有 goroutine 结束
	for k, cfg := range global.GlobalCfg.Sites {
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
	<-ctx.Done() // 等待取消信号
	sLogger().Warn("genTorrentsWithRSS: 收到取消信号，开始清理资源")
	wg.Wait() // 等待所有 Goroutine 完成
	sLogger().Warn("genTorrentsWithRSS: 所有任务已取消")
	return nil
}

// 站点任务
func runSiteJobs[T models.ResType](ctx context.Context, siteName models.SiteGroup, siteCfg config.SiteConfig, siteImpl internal.PTSiteInter[T]) {
	var siteWg sync.WaitGroup // 用于等待该站点的所有 RSS 任务结束
	for _, rss := range siteCfg.RSS {
		siteWg.Add(1)
		go func(rss config.RSSConfig) {
			defer siteWg.Done()
			runRSSJob(ctx, siteName, rss, siteImpl)
		}(rss)
	}
	siteWg.Wait() // 等待该站点的所有 RSS 任务结束
}

func getInterval(cfg config.RSSConfig) time.Duration {
	if cfg.IntervalMinutes <= 0 {
		if global.GlobalCfg.Global.DefaultInterval > 0 {
			return global.GlobalCfg.Global.DefaultInterval
		}
		return 10 * time.Minute
	}
	// 如果 cfg.IntervalMinutes 大于 0，则使用 cfg.IntervalMinutes 转换为 time.Duration
	return time.Duration(cfg.IntervalMinutes) * time.Minute
}

// RSS 任务
func runRSSJob[T models.ResType](ctx context.Context, siteName models.SiteGroup, cfg config.RSSConfig, siteImpl internal.PTSiteInter[T]) {
	ticker := time.NewTicker(getInterval(cfg))
	defer ticker.Stop()
	executeTask(ctx, siteName, cfg, siteImpl)
	// 判断运行模式
	mode, _ := ctx.Value("mode").(string)
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
func executeTask[T models.ResType](ctx context.Context, siteName models.SiteGroup, cfg config.RSSConfig, siteImpl internal.PTSiteInter[T]) {
	sLogger().Infof("开始任务: %s", cfg.Name)
	if err := processRSS(ctx, siteName, cfg, siteImpl); err != nil {
		sLogger().Errorf("站点: %s 任务执行失败, %v", cfg.Name, err)
	} else {
		sLogger().Infof("站点: %s 任务执行完成", cfg.Name)
	}
}

// 处理单个 RSS 配置
func processRSS[T models.ResType](ctx context.Context, siteName models.SiteGroup, cfg config.RSSConfig, ptSite internal.PTSiteInter[T]) error {
	if err := internal.FetchAndDownloadFreeRSS(ctx, siteName, ptSite, cfg); err != nil {
		return err
	}
	if err := ptSite.SendTorrentToQbit(ctx, cfg); err != nil {
		return err
	}
	return nil
}
