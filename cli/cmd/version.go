package cmd

import (
	"context"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show server version information",
	RunE:  runVersion,
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

func runVersion(cmd *cobra.Command, args []string) error {
	if err := initClient(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()

	resp, err := apiClient.GetVersion(ctx)
	if err != nil {
		return err
	}

	color.Cyan("Version:")
	color.White("  Version:     %s", resp.Version)
	color.White("  Build Time:  %s", resp.BuildTime)
	color.White("  Commit:      %s", resp.CommitID)
	if resp.Runtime != "" {
		color.White("  Runtime:     %s", resp.Runtime)
	}
	if resp.GoVersion != "" {
		color.White("  Go:          %s", resp.GoVersion)
	}
	if resp.HasUpdate {
		color.Yellow("\n  New version available: %s", resp.Latest)
	}
	return nil
}
