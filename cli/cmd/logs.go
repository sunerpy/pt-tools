package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var logsTail int

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View recent logs",
	Long: `View recent application logs from the server.

Examples:
  pt-tools-cli logs
  pt-tools-cli logs --tail 500`,
	RunE: runLogs,
}

func init() {
	logsCmd.Flags().IntVar(&logsTail, "tail", 100, "Number of recent log lines to show")
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) error {
	if err := initClient(); err != nil {
		return err
	}
	if err := requireAuth(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()

	resp, err := apiClient.GetLogs(ctx)
	if err != nil {
		return err
	}

	lines := resp.Lines
	if len(lines) > logsTail {
		lines = lines[len(lines)-logsTail:]
	}

	fmt.Printf("Log file: %s", resp.Path)
	if resp.Truncated {
		fmt.Printf(" (truncated)")
	}
	fmt.Println()
	fmt.Println("---")
	for _, line := range lines {
		fmt.Println(line)
	}
	fmt.Println("---")
	fmt.Printf("\nShowing %d of %d lines\n", len(lines), len(resp.Lines))
	return nil
}
