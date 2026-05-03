package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sunerpy/pt-tools/cli/internal/output"
)

var (
	taskSite       string
	taskQ          string
	taskSort       string
	taskDownloaded bool
	taskPushed     bool
	taskExpired    bool
	taskPage       int
	taskPageSize   int
)

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	Long: `List tasks with optional filters.

Examples:
  pt-tools-cli task list
  pt-tools-cli task list --site HDFans --page 1 --page-size 50
  pt-tools-cli task list --downloaded --q keyword`,
	RunE: runTaskList,
}

var taskStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start all tasks",
	RunE:  runTaskStart,
}

var taskStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop all tasks",
	RunE:  runTaskStop,
}

func init() {
	taskCmd.AddCommand(taskListCmd)
	taskCmd.AddCommand(taskStartCmd)
	taskCmd.AddCommand(taskStopCmd)

	taskListCmd.Flags().StringVar(&taskSite, "site", "", "Filter by site")
	taskListCmd.Flags().StringVar(&taskQ, "q", "", "Search by keyword")
	taskListCmd.Flags().StringVar(&taskSort, "sort", "", "Sort: created_at_asc")
	taskListCmd.Flags().BoolVar(&taskDownloaded, "downloaded", false, "Show downloaded only")
	taskListCmd.Flags().BoolVar(&taskPushed, "pushed", false, "Show pushed only")
	taskListCmd.Flags().BoolVar(&taskExpired, "expired", false, "Show expired only")
	taskListCmd.Flags().IntVar(&taskPage, "page", 1, "Page number")
	taskListCmd.Flags().IntVar(&taskPageSize, "page-size", 20, "Items per page (max 500)")
}

func runTaskList(cmd *cobra.Command, args []string) error {
	if err := initClient(); err != nil {
		return err
	}
	if err := requireAuth(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()

	resp, err := apiClient.ListTasks(ctx, taskSite, taskQ, taskSort, taskDownloaded, taskPushed, taskExpired, taskPage, taskPageSize)
	if err != nil {
		return err
	}

	if len(resp.Items) == 0 {
		output.Warn("No tasks found.")
		return nil
	}

	rows := make([][]any, 0, len(resp.Items))
	for i, item := range resp.Items {
		status := ""
		switch {
		case item.IsExpired:
			status = "expired"
		case item.IsDownloaded && item.IsPushed:
			status = "done"
		case item.IsDownloaded:
			status = "downloaded"
		case item.IsPushed:
			status = "pushed"
		default:
			status = "pending"
		}

		title := item.Title
		if len(title) > 40 {
			title = title[:39] + "…"
		}

		createdAt := item.CreatedAt
		if idx := strings.Index(createdAt, "T"); idx > 0 {
			createdAt = createdAt[:idx]
		}

		rows = append(rows, []any{
			i + 1,
			title,
			item.SiteName,
			status,
			createdAt,
		})
	}

	output.PrintTable([]string{"#", "Title", "Site", "Status", "Date"}, rows)
	fmt.Printf("\nTotal: %d | Page: %d/%d\n", resp.Total, resp.Page, (int(resp.Total)+resp.Size-1)/resp.Size)
	return nil
}

func runTaskStart(cmd *cobra.Command, args []string) error {
	if err := initClient(); err != nil {
		return err
	}
	if err := requireAuth(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()

	if err := apiClient.StartAllTasks(ctx); err != nil {
		return err
	}

	output.Success("All tasks started.")
	return nil
}

func runTaskStop(cmd *cobra.Command, args []string) error {
	if err := initClient(); err != nil {
		return err
	}
	if err := requireAuth(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()

	if err := apiClient.StopAllTasks(ctx); err != nil {
		return err
	}

	output.Success("All tasks stopped.")
	return nil
}
