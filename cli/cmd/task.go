package cmd

import "github.com/spf13/cobra"

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage tasks",
	Long:  `View and control task execution.`,
}

func init() {
	rootCmd.AddCommand(taskCmd)
}
