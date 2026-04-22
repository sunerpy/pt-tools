package cmd

import "github.com/spf13/cobra"

var downloaderCmd = &cobra.Command{
	Use:   "downloader",
	Short: "Manage downloaders",
	Long:  `View downloader status and statistics.`,
}

func init() {
	rootCmd.AddCommand(downloaderCmd)
}
