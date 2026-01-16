package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/utils"
)

var fixTimezoneCmd = &cobra.Command{
	Use:   "fix-timezone",
	Short: "修复数据库中的免费结束时间时区问题",
	Long: `修复因时区解析错误导致的 free_end_time 时间偏移问题。

该命令将数据库中所有 free_end_time 字段的时间从 UTC 修正为 CST (UTC+8)。

问题原因：
  之前解析 discountEndTime 时使用 time.Parse 而非 time.ParseInLocation，
  导致 CST 时间被错误地当作 UTC 存储，实际存储的时间比正确时间早了8小时。

修复逻辑：
  将 free_end_time 减去 8 小时，使其回到正确的 CST 时间点。

使用示例：
  # 预览将要修复的记录（不实际修改）
  pt-tools db fix-timezone --dry-run

  # 执行修复
  pt-tools db fix-timezone`,
	RunE: runFixTimezone,
}

var fixTimezoneDryRun bool

func init() {
	dbCmd.AddCommand(fixTimezoneCmd)
	fixTimezoneCmd.Flags().BoolVar(&fixTimezoneDryRun, "dry-run", false, "只预览，不实际修改")
}

func runFixTimezone(_ *cobra.Command, _ []string) error {
	if err := initTools(); err != nil {
		return fmt.Errorf("初始化失败: %w", err)
	}

	db := global.GlobalDB
	if db == nil {
		return fmt.Errorf("数据库未初始化")
	}

	offset := 8 * time.Hour

	var torrents []models.TorrentInfo
	if err := db.DB.Where("free_end_time IS NOT NULL").Find(&torrents).Error; err != nil {
		return fmt.Errorf("查询种子信息失败: %w", err)
	}

	fmt.Printf("\n=== 修复 free_end_time 时区 ===\n")
	fmt.Printf("当前时区: %s\n", utils.CSTLocation.String())
	fmt.Printf("发现 %d 条有 free_end_time 的记录\n\n", len(torrents))

	if len(torrents) == 0 {
		fmt.Println("没有需要修复的记录")
		return nil
	}

	if fixTimezoneDryRun {
		fmt.Println("【预览模式】以下是将要修复的记录：")
		fmt.Println()
	}

	fixedCount := 0
	for _, t := range torrents {
		if t.FreeEndTime == nil {
			continue
		}

		oldTime := *t.FreeEndTime
		newTime := oldTime.Add(-offset)

		if fixTimezoneDryRun {
			fmt.Printf("ID: %d, 站点: %s\n", t.ID, t.SiteName)
			fmt.Printf("  标题: %s\n", truncateTitle(t.Title, 60))
			fmt.Printf("  原时间: %s (UTC)\n", oldTime.Format("2006-01-02 15:04:05"))
			fmt.Printf("  新时间: %s (CST)\n", newTime.In(utils.CSTLocation).Format("2006-01-02 15:04:05"))
			fmt.Println()
		} else {
			if err := db.DB.Model(&t).Update("free_end_time", newTime).Error; err != nil {
				fmt.Printf("更新失败 ID=%d: %v\n", t.ID, err)
				continue
			}
			fixedCount++
		}
	}

	var archives []models.TorrentInfoArchive
	if err := db.DB.Where("free_end_time IS NOT NULL").Find(&archives).Error; err != nil {
		sLogger().Warnf("查询归档记录失败: %v", err)
	} else if len(archives) > 0 {
		fmt.Printf("\n发现 %d 条有 free_end_time 的归档记录\n", len(archives))

		for _, a := range archives {
			if a.FreeEndTime == nil {
				continue
			}

			oldTime := *a.FreeEndTime
			newTime := oldTime.Add(-offset)

			if fixTimezoneDryRun {
				fmt.Printf("归档 ID: %d, 站点: %s\n", a.ID, a.SiteName)
				fmt.Printf("  原时间: %s -> 新时间: %s\n",
					oldTime.Format("2006-01-02 15:04:05"),
					newTime.In(utils.CSTLocation).Format("2006-01-02 15:04:05"))
			} else {
				if err := db.DB.Model(&a).Update("free_end_time", newTime).Error; err != nil {
					fmt.Printf("更新归档失败 ID=%d: %v\n", a.ID, err)
					continue
				}
				fixedCount++
			}
		}
	}

	fmt.Println()
	if fixTimezoneDryRun {
		fmt.Println("【预览完成】使用不带 --dry-run 参数执行实际修复")
	} else {
		fmt.Printf("修复完成，共更新 %d 条记录\n", fixedCount)
	}

	return nil
}

func truncateTitle(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}
