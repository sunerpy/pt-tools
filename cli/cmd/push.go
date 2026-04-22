package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sunerpy/pt-tools/cli/internal/output"
	"github.com/sunerpy/pt-tools/cli/internal/types"
	"github.com/sunerpy/pt-tools/cli/internal/ui"
)

var (
	pushDownloaders string
	pushCategory    string
	pushTags        string
	pushSavePath    string
)

var pushCmd = &cobra.Command{
	Use:   "push <downloadUrl>",
	Short: "Push a torrent to downloaders",
	Long: `Push a single torrent to one or more downloaders.

Examples:
  pt-tools-cli push "/api/site/hdfans/torrent/12345/download"
  pt-tools-cli push <url> --downloaders 1,2 --category movies`,
	Args: cobra.ExactArgs(1),
	RunE: runPush,
}

func init() {
	pushCmd.Flags().StringVar(&pushDownloaders, "downloaders", "", "Comma-separated downloader IDs (e.g., 1,2)")
	pushCmd.Flags().StringVar(&pushCategory, "category", "", "qBittorrent category")
	pushCmd.Flags().StringVar(&pushTags, "tags", "", "qBittorrent tags")
	pushCmd.Flags().StringVar(&pushSavePath, "save-path", "", "Custom save path")
	rootCmd.AddCommand(pushCmd)
}

func runPush(cmd *cobra.Command, args []string) error {
	if err := initClient(); err != nil {
		return err
	}
	if err := requireAuth(); err != nil {
		return err
	}

	downloadURL := args[0]

	// Parse downloader IDs
	var dlIDs []uint
	if pushDownloaders != "" {
		for _, s := range strings.Split(pushDownloaders, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			var id uint
			if _, err := fmt.Sscanf(s, "%d", &id); err == nil {
				dlIDs = append(dlIDs, id)
			}
		}
	}

	if len(dlIDs) == 0 {
		// Get available downloaders and let user select
		ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
		defer cancel()

		downloaders, err := apiClient.ListDownloaders(ctx)
		if err != nil {
			return fmt.Errorf("failed to list downloaders: %w", err)
		}

		if len(downloaders) == 0 {
			return fmt.Errorf("no downloaders configured")
		}

		options := make([]string, 0, len(downloaders))
		for _, dl := range downloaders {
			options = append(options, fmt.Sprintf("%s (%s)", dl.Name, dl.Type))
		}

		selected, err := ui.SelectMulti(options, "Select target downloader(s):")
		if err != nil {
			return err
		}

		for _, s := range selected {
			for _, dl := range downloaders {
				opt := fmt.Sprintf("%s (%s)", dl.Name, dl.Type)
				if opt == s {
					dlIDs = append(dlIDs, dl.ID)
					break
				}
			}
		}
	}

	if len(dlIDs) == 0 {
		return fmt.Errorf("no downloaders selected")
	}

	req := &types.TorrentPushRequest{
		DownloadURL:   downloadURL,
		DownloaderIDs: dlIDs,
		Category:      pushCategory,
		Tags:          pushTags,
		SavePath:      pushSavePath,
	}

	output.Spinner("Pushing torrent...")

	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	resp, err := apiClient.PushTorrent(ctx, req)
	if err != nil {
		output.Done()
		return fmt.Errorf("push failed: %w", err)
	}
	output.Done()

	if !resp.Success {
		return fmt.Errorf("push failed: %s", resp.Message)
	}

	output.Success("Push: %s", resp.Message)
	for _, r := range resp.Results {
		if r.Success {
			output.Success("  ✓ %s: %s", r.DownloaderName, r.Message)
		} else {
			output.Error("  ✗ %s: %s", r.DownloaderName, r.Message)
		}
	}
	return nil
}
