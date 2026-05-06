package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/sunerpy/pt-tools/global"
)

// scraperCmd 启动嵌入模式的 scraper 子命令。
//
// 实际刮削路由已通过 T21 HTTP API 挂到 pt-tools web 命令下（/api/v2/scraper/*），
// 本命令为用户提供单独启动入口。用户通常直接用 `pt-tools web` 即可同时获得
// 主 UI 和 scraper 功能；此命令用于独立调试或未来分离场景。
var scraperCmd = &cobra.Command{
	Use:   "scraper",
	Short: "启动媒体刮削子系统（嵌入模式）",
	Long: `启动媒体刮削子系统（嵌入模式）。

scraper 子系统已作为 /api/v2/scraper/* 嵌入到 pt-tools web 命令，
通常直接运行 'pt-tools web' 即可同时访问主 UI 和刮削功能。

如需独立部署 scraper（自有 DB/端口），请使用 pt-scraper 二进制。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 嵌入模式下 scraper 路由已由 web 命令注册；此命令仅提醒用户。
		fmt.Fprintln(os.Stderr, "scraper 子系统已嵌入 'pt-tools web' 命令")
		fmt.Fprintln(os.Stderr, "  - Web UI: /scraper")
		fmt.Fprintln(os.Stderr, "  - HTTP API: /api/v2/scraper/*")
		fmt.Fprintln(os.Stderr, "独立部署请使用 pt-scraper 二进制。按 Ctrl+C 退出。")

		if log := global.GetLogger(); log != nil {
			log.Info("scraper 子命令启动（嵌入模式提示）")
		}

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		return nil
	},
}

func init() {
	rootCmd.AddCommand(scraperCmd)
}
