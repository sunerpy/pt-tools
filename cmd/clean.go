package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/sunerpy/pt-tools/config"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/maintenance"
	"github.com/sunerpy/pt-tools/models"
)

var (
	cleanDryRun      bool
	cleanConfirm     bool
	cleanCategories  []string
	cleanKeepBackups int
)

var cleanCmd = &cobra.Command{
	Use:           "clean",
	SilenceUsage:  true,
	SilenceErrors: true,
	Short:         "清理 ~/.pt-tools 下的日志/暂存种子/旧备份",
	Long: `clean 在 ~/.pt-tools 的三个白名单目录（logs / downloads / backups）内安全清理：
  - logs     ：删除已轮转的日志备份（保留正在写入的 base 日志）
  - staging  ：删除 downloads/{tag} 下已推送/孤立/超保留期的 .torrent
  - backups  ：仅保留最近 N 份备份（--keep-backups）

红线文件（torrents.db、secret.key、base 日志等）在任何情况下都不会被删除。
默认以预览（dry-run）模式运行，不会删除任何文件；真实删除必须显式传入 --confirm。`,
	Example: `  预览将清理的内容（默认，安全）
  pt-tools clean
  只预览 logs 类别
  pt-tools clean --category logs
  真实删除（破坏性，需确认）
  pt-tools clean --confirm
  真实删除并保留最近 10 份备份
  pt-tools clean --confirm --keep-backups 10`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("获取用户主目录失败: %w", err)
		}
		zapCfg := config.DefaultZapConfig
		zapCfg.ApplyEnvOverrides()
		return executeClean(cmd.OutOrStdout(), home, global.GlobalDB, zapCfg,
			cleanCategories, cleanDryRun, cleanConfirm, cleanKeepBackups)
	},
}

func init() {
	rootCmd.AddCommand(cleanCmd)
	cleanCmd.Flags().BoolVar(&cleanDryRun, "dry-run", true, "预览模式，只列出将删除的文件而不实际删除")
	cleanCmd.Flags().BoolVar(&cleanConfirm, "confirm", false, "确认执行真实删除（破坏性操作，与 --dry-run=false 配合）")
	cleanCmd.Flags().StringSliceVar(&cleanCategories, "category", nil, "限定清理类别：logs,staging,backups（留空表示全部）")
	cleanCmd.Flags().IntVar(&cleanKeepBackups, "keep-backups", 5, "backups 类别保留的最近份数")
}

// executeClean 是 clean 命令的可测试核心：解析类别、执行 confirm 门禁、调用 Cleaner 并格式化输出。
// 有效 dry-run 语义：只有显式 --confirm 时才真实删除；否则一律预览。
func executeClean(w io.Writer, home string, db *models.TorrentDB, zapCfg config.Zap,
	categories []string, dryRunFlag, confirm bool, keepBackups int,
) error {
	cats, err := parseCleanCategories(categories)
	if err != nil {
		return err
	}

	// confirm 门禁：用户显式要求真实删除（--dry-run=false）却未加 --confirm → 拒绝。
	if !dryRunFlag && !confirm {
		fmt.Fprintln(w, color.RedString("这是破坏性操作，请加 --confirm 确认后再执行真实删除。"))
		return fmt.Errorf("真实删除需要 --confirm 确认")
	}

	// 有效 dry-run：仅在显式 --confirm 时才真正删除。
	effectiveDryRun := !confirm

	cleaner := maintenance.NewCleaner(home, db, zapCfg)
	res, err := cleaner.Clean(context.Background(), maintenance.CleanOptions{
		Categories:  cats,
		DryRun:      effectiveDryRun,
		KeepBackups: keepBackups,
	})
	if err != nil {
		return fmt.Errorf("清理失败: %w", err)
	}

	printCleanResult(w, res)
	return nil
}

// parseCleanCategories 将 CLI 字符串校验并映射为 maintenance.CleanCategory；非法值报错。
func parseCleanCategories(raw []string) ([]maintenance.CleanCategory, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]maintenance.CleanCategory, 0, len(raw))
	for _, r := range raw {
		switch maintenance.CleanCategory(r) {
		case maintenance.CategoryLogs, maintenance.CategoryStaging, maintenance.CategoryBackups:
			out = append(out, maintenance.CleanCategory(r))
		default:
			return nil, fmt.Errorf("未知的清理类别 %q，可选：logs,staging,backups", r)
		}
	}
	return out, nil
}

// printCleanResult 以 fatih/color 风格逐类别打印结果与合计释放空间。
func printCleanResult(w io.Writer, res *maintenance.CleanResult) {
	if res.DryRun {
		fmt.Fprintln(w, color.YellowString("预览（未删除）：以下文件将在 --confirm 后被清理"))
	} else {
		fmt.Fprintln(w, color.GreenString("清理完成："))
	}

	if len(res.Categories) == 0 {
		fmt.Fprintln(w, "  无可清理内容（对应目录不存在或已为空）")
		return
	}

	var totalFreed int64
	for _, cr := range res.Categories {
		verb := "将删除"
		if !res.DryRun {
			verb = "已删除"
		}
		fmt.Fprintf(w, "  [%s] %s %d 个文件，%s %s\n",
			color.CyanString(string(cr.Category)),
			verb, len(cr.Deleted),
			verb, humanBytes(cr.FreedBytes))
		if len(cr.Skipped) > 0 {
			fmt.Fprintf(w, "    %s %d 项（命中红线/越界保护）\n",
				color.RedString("跳过"), len(cr.Skipped))
		}
		if cr.Note != "" {
			fmt.Fprintf(w, "    %s %s\n", color.YellowString("注意:"), cr.Note)
		}
		totalFreed += cr.FreedBytes
	}

	summaryVerb := "将释放"
	if !res.DryRun {
		summaryVerb = "共释放"
	}
	fmt.Fprintf(w, "%s %s\n", color.GreenString(summaryVerb), humanBytes(totalFreed))
}

// humanBytes 以 1024 进制格式化字节数为可读字符串。
func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
