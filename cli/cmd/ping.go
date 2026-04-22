package cmd

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var pingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Check server health and authentication",
	RunE:  runPing,
}

func init() {
	rootCmd.AddCommand(pingCmd)
}

func runPing(cmd *cobra.Command, args []string) error {
	if err := initClient(); err != nil {
		return err
	}
	if err := requireAuth(); err != nil {
		return err
	}

	if err := apiClient.Ping(); err != nil {
		return err
	}

	color.Green("✓ Server is reachable and authenticated at %s", cfg.URL)
	return nil
}
