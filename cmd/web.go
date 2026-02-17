package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/version"
	"github.com/sunerpy/pt-tools/web"
)

var (
	host string
	port int
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "启动 Web 管理界面（默认）",
	Run: func(cmd *cobra.Command, args []string) {
		version.CleanupOldBinary()

		// 初始化配置与数据库
		if _, err := core.InitRuntime(); err != nil {
			color.Red("初始化失败: %v", err)
			return
		}

		global.GetSlogger().Infof("=== pt-tools 启动 === 版本: %s, 构建时间: %s", version.Version, version.BuildTime)

		// 创建 SiteRegistry 并同步到数据库
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

		// 初始化 UserInfoService
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

			// 初始化 SearchOrchestrator
			searchOrchestrator := v2.NewSearchOrchestrator(v2.SearchOrchestratorConfig{
				Logger: global.GetLogger(),
			})
			cachedSearchOrchestrator := v2.NewCachedSearchOrchestrator(searchOrchestrator, v2.SearchCacheConfig{
				TTL:     10 * time.Minute,
				MaxSize: 500,
			})
			web.InitSearchOrchestrator(cachedSearchOrchestrator)
			global.GetSlogger().Info("SearchOrchestrator 初始化成功")

			// 注册已启用的站点到 UserInfoService 和 SearchOrchestrator
			web.InitSiteRegistry(siteRegistry) // 保存 registry 供后续动态注册使用
			sites, siteErr := store.ListSites()
			if siteErr != nil {
				global.GetSlogger().Warnf("读取站点配置失败: %v", siteErr)
			} else {
				for siteGroup, siteConfig := range sites {
					if siteConfig.Enabled == nil || !*siteConfig.Enabled {
						continue
					}

					// 使用 SiteRegistry 创建站点实例
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

		srv := web.NewServer(store, mgr)
		if cfg, _ := store.Load(); cfg != nil {
			if cfg.Global.AutoStart && strings.TrimSpace(cfg.Global.DownloadDir) != "" {
				global.GetSlogger().Info("检测到自动启动配置，加载并启动任务")
				mgr.Reload(cfg)
			} else {
				global.GetSlogger().Info("自动启动未开启或下载目录为空，等待手动启动")
			}
		}
		global.GetSlogger().Infof("Web 服务启动于 %s", addr)
		go startVersionChecker()
		if err := srv.Serve(addr); err != nil {
			global.GetSlogger().Fatalf("Web 启动失败: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(webCmd)
	webCmd.Flags().StringVar(&host, "host", "0.0.0.0", "服务绑定主机")
	webCmd.Flags().IntVar(&port, "port", 8080, "服务监听端口")
}

// getRegisteredSitesFromRegistry 从 SiteRegistry 获取所有注册的站点信息
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
