package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/sunerpy/pt-tools/cli/internal/output"
	"github.com/sunerpy/pt-tools/cli/internal/types"
	"github.com/sunerpy/pt-tools/cli/internal/ui"
)

var (
	searchSites      string
	searchMinSeeders int
	searchFreeOnly   bool
	searchTimeout    int
	searchNoInteract bool
	searchOutput     string
)

var searchCmd = &cobra.Command{
	Use:   "search <keyword>",
	Short: "Search torrents across PT sites",
	Long: `Search for torrents across configured PT sites with interactive selection and push.

Examples:
  pt-tools-cli search "Movie Name"
  pt-tools-cli search "Movie" --sites HDFans,MT --min-seeders 10
  pt-tools-cli search "Movie" --free-only
  pt-tools-cli search "Movie" --no-interactive --output json`,
	Args: cobra.ExactArgs(1),
	RunE: runSearch,
}

func init() {
	searchCmd.Flags().StringVar(&searchSites, "sites", "", "Comma-separated site IDs to search (e.g., HDFans,MT)")
	searchCmd.Flags().IntVar(&searchMinSeeders, "min-seeders", 0, "Minimum seeders")
	searchCmd.Flags().BoolVar(&searchFreeOnly, "free-only", false, "Show only free torrents")
	searchCmd.Flags().IntVar(&searchTimeout, "timeout", 30, "Search timeout in seconds")
	searchCmd.Flags().BoolVar(&searchNoInteract, "no-interactive", false, "Skip interactive selection")
	searchCmd.Flags().StringVar(&searchOutput, "output", "", "Output format: json, quiet")
	rootCmd.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	if err := initClient(); err != nil {
		return err
	}
	if err := requireAuth(); err != nil {
		return err
	}

	keyword := args[0]

	// Build search request
	req := &types.MultiSiteSearchRequest{
		Keyword:    keyword,
		FreeOnly:   searchFreeOnly,
		MinSeeders: searchMinSeeders,
		TimeoutSecs: searchTimeout,
	}

	if searchSites != "" {
		req.Sites = strings.Split(searchSites, ",")
		for i := range req.Sites {
			req.Sites[i] = strings.TrimSpace(req.Sites[i])
		}
	}

	output.Spinner(fmt.Sprintf("Searching for '%s'", keyword))

	ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(searchTimeout)*time.Second)
	defer cancel()

	resp, err := apiClient.Search(ctx, req)
	if err != nil {
		output.Done()
		return fmt.Errorf("search failed: %w", err)
	}
	output.Done()

	if len(resp.Items) == 0 {
		color.Yellow("No results found for '%s'", keyword)
		if len(resp.Errors) > 0 {
			for _, e := range resp.Errors {
				color.Red("  Error from %s: %s", e.Site, e.Error)
			}
		}
		return nil
	}

	if searchOutput == "json" {
		// JSON output
		data, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if searchOutput == "quiet" {
		for _, item := range resp.Items {
			fmt.Printf("%s\t%s\t%s\t%d\t%s\n", item.Title, item.SourceSite, output.FormatBytes(item.SizeBytes), item.Seeders, item.DownloadURL)
		}
		return nil
	}

	// Display results table
	fmt.Printf("\nFound %d results (%dms)\n\n", resp.TotalResults, resp.DurationMs)
	printSearchResults(resp.Items)

	if searchNoInteract || len(resp.Items) == 0 {
		return nil
	}

	// Interactive selection
	indices, err := ui.SelectIndices(len(resp.Items))
	if err != nil {
		if strings.Contains(err.Error(), "cancelled") {
			return nil
		}
		return err
	}

	if len(indices) == 0 {
		color.Yellow("No torrents selected.")
		return nil
	}

	// Get selected torrents
	var selected []types.TorrentItemResponse
	for _, idx := range indices {
		if idx >= 0 && idx < len(resp.Items) {
			selected = append(selected, resp.Items[idx])
		}
	}

	// Push to downloaders
	return pushSelected(cmd, selected)
}

func printSearchResults(items []types.TorrentItemResponse) {
	rows := make([][]any, 0, len(items))
	for i, item := range items {
		tags := output.TagString(item.Tags)
		hr := ""
		if item.HasHR {
			hr = "HR"
		}

		title := item.Title
		if len(title) > 50 {
			title = title[:49] + "…"
		}

		rows = append(rows, []any{
			i + 1,
			title,
			item.SourceSite,
			output.FormatBytes(item.SizeBytes),
			item.Seeders,
			output.FreeStatus(item.DiscountLevel),
			hr,
			tags,
		})
	}

	output.PrintTable(
		[]string{"#", "Title", "Site", "Size", "Seeders", "Free", "HR", "Tags"},
		rows,
	)
}

func pushSelected(cmd *cobra.Command, torrents []types.TorrentItemResponse) error {
	// Get available downloaders
	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()

	downloaders, err := apiClient.ListDownloaders(ctx)
	if err != nil {
		return fmt.Errorf("failed to list downloaders: %w", err)
	}

	if len(downloaders) == 0 {
		color.Yellow("No downloaders configured. Please configure a downloader in the web UI first.")
		return nil
	}

	// Build downloader options
	options := make([]string, 0, len(downloaders))
	for _, dl := range downloaders {
		options = append(options, fmt.Sprintf("%s (%s)", dl.Name, dl.Type))
	}

	// Select downloaders
	selected, err := ui.SelectMulti(options, "Select target downloader(s):")
	if err != nil {
		return err
	}

	if len(selected) == 0 {
		color.Yellow("No downloaders selected.")
		return nil
	}

	// Map selected to IDs
	var dlIDs []uint
	for _, s := range selected {
		// Extract ID from "Name (Type)" format
		for _, dl := range downloaders {
			opt := fmt.Sprintf("%s (%s)", dl.Name, dl.Type)
			if opt == s {
				dlIDs = append(dlIDs, dl.ID)
				break
			}
		}
	}

	// Build batch push request
	var pushItems []types.TorrentPushItem
	for _, t := range torrents {
		pushItems = append(pushItems, types.TorrentPushItem{
			DownloadURL:  t.DownloadURL,
			TorrentTitle: t.Title,
			SourceSite:   t.SourceSite,
			SizeBytes:    t.SizeBytes,
		})
	}

	batchReq := &types.BatchTorrentPushRequest{
		Torrents:      pushItems,
		DownloaderIDs: dlIDs,
	}

	output.Spinner("Pushing torrents...")

	ctx2, cancel2 := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel2()

	resp, err := apiClient.BatchPushTorrents(ctx2, batchReq)
	if err != nil {
		output.Done()
		return fmt.Errorf("push failed: %w", err)
	}
	output.Done()

	// Show results
	fmt.Printf("\nPush results: %d success, %d skipped, %d failed\n\n",
		resp.SuccessCount, resp.SkippedCount, resp.FailedCount)

	rows := make([][]any, 0, len(resp.Results))
	for _, r := range resp.Results {
		status := "✓"
		if !r.Success {
			status = "✗"
		} else if r.Skipped {
			status = "⊘"
		}
		rows = append(rows, []any{
			status,
			output.Truncate(r.TorrentTitle, 50),
			r.SourceSite,
			r.Message,
		})
	}

	output.PrintTable([]string{"Status", "Title", "Site", "Message"}, rows)
	return nil
}
