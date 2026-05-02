package cmd

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/sunerpy/pt-tools/cli/internal/output"
)

var userinfoSites bool

var userinfoCmd = &cobra.Command{
	Use:   "userinfo",
	Short: "Show aggregated user info from PT sites",
	Long: `Display aggregated user information across all configured PT sites.

Examples:
  pt-tools-cli userinfo
  pt-tools-cli userinfo --sites`,
	RunE: runUserInfo,
}

func init() {
	userinfoCmd.Flags().BoolVar(&userinfoSites, "sites", false, "Show per-site breakdown")
	rootCmd.AddCommand(userinfoCmd)
}

func runUserInfo(cmd *cobra.Command, args []string) error {
	if err := initClient(); err != nil {
		return err
	}
	if err := requireAuth(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	resp, err := apiClient.GetUserInfoAggregated(ctx)
	if err != nil {
		return err
	}

	if len(resp.Sites) == 0 {
		output.Warn("No site data available.")
		return nil
	}

	if !userinfoSites {
		// Summary only
		var totalUploaded, totalDownloaded, totalSeedSize int64
		for _, s := range resp.Sites {
			totalUploaded += s.Uploaded
			totalDownloaded += s.Downloaded
			totalSeedSize += s.SeedSize
		}
		output.Info("Sites: %d", len(resp.Sites))
		output.Info("Total Uploaded:   %s", output.FormatBytes(totalUploaded))
		output.Info("Total Downloaded:  %s", output.FormatBytes(totalDownloaded))
		output.Info("Total Seed Size:   %s", output.FormatBytes(totalSeedSize))
		return nil
	}

	// Per-site breakdown
	rows := make([][]any, 0, len(resp.Sites))
	for _, s := range resp.Sites {
		rows = append(rows, []any{
			s.SiteName,
			s.Username,
			s.Level,
			output.FormatBytes(s.Uploaded),
			output.FormatBytes(s.Downloaded),
			s.Ratio,
			output.FormatBytes(s.SeedSize),
		})
	}

	output.PrintTable(
		[]string{"Site", "User", "Level", "Uploaded", "Downloaded", "Ratio", "Seed Size"},
		rows,
	)
	return nil
}
