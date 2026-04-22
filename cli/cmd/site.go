package cmd

import "github.com/spf13/cobra"

var siteCmd = &cobra.Command{
	Use:   "site",
	Short: "Manage sites",
	Long:  `View and manage configured PT sites.`,
}

func init() {
	rootCmd.AddCommand(siteCmd)
}
