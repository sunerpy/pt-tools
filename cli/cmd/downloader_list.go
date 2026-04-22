package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/sunerpy/pt-tools/cli/internal/output"
)

var downloaderListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all downloaders",
	RunE:  runDownloaderList,
}

var downloaderStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show transfer statistics",
	RunE:  runDownloaderStats,
}

func init() {
	downloaderCmd.AddCommand(downloaderListCmd)
	downloaderCmd.AddCommand(downloaderStatsCmd)
}

func runDownloaderList(cmd *cobra.Command, args []string) error {
	if err := initClient(); err != nil {
		return err
	}
	if err := requireAuth(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()

	downloaders, err := apiClient.ListDownloaders(ctx)
	if err != nil {
		return err
	}

	if len(downloaders) == 0 {
		output.Warn("No downloaders configured.")
		return nil
	}

	rows := make([][]any, 0, len(downloaders))
	for _, dl := range downloaders {
		status := "disabled"
		if dl.Enabled {
			status = "enabled"
		}

		rows = append(rows, []any{
			dl.ID,
			dl.Name,
			dl.Type,
			status,
			dl.URL,
		})
	}

	output.PrintTable([]string{"ID", "Name", "Type", "Status", "URL"}, rows)
	fmt.Printf("\nTotal: %d downloaders\n", len(downloaders))
	return nil
}

func runDownloaderStats(cmd *cobra.Command, args []string) error {
	if err := initClient(); err != nil {
		return err
	}
	if err := requireAuth(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()

	stats, err := apiClient.DownloaderStats(ctx)
	if err != nil {
		return err
	}

	fmt.Println("Transfer Statistics:")
	fmt.Printf("  Download Speed:  %s\n", output.FormatSpeed(stats.DownloadSpeed))
	fmt.Printf("  Upload Speed:    %s\n", output.FormatSpeed(stats.UploadSpeed))
	fmt.Printf("  Total Downloaded: %s\n", output.FormatBytes(stats.Downloaded))
	fmt.Printf("  Total Uploaded:   %s\n", output.FormatBytes(stats.Uploaded))
	fmt.Printf("  Active Torrents:  %d\n", stats.ActiveTorrents)
	return nil
}
