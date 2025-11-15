package cmd

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/scheduler"
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
		// 初始化配置与数据库
		if _, err := core.InitRuntime(); err != nil {
			color.Red("初始化失败: %v", err)
			return
		}
		// 从 DB 读取配置，若为空允许后续通过 Web 初始化
		store := core.NewConfigStore(global.GlobalDB)
		gl, _ := store.GetGlobalOnly()
		if strings.TrimSpace(gl.DownloadDir) == "" {
			color.Yellow("当前未检测到 DB 配置，可通过 Web 进行初始化")
		}
        addr := fmt.Sprintf("%s:%d", host, port)
        mgr := scheduler.NewManager()
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
        if err := srv.Serve(addr); err != nil {
            color.Red("Web 启动失败: %v", err)
        }
    },
}

func init() {
	rootCmd.AddCommand(webCmd)
	webCmd.Flags().StringVar(&host, "host", "0.0.0.0", "服务绑定主机")
	webCmd.Flags().IntVar(&port, "port", 8080, "服务监听端口")
}
