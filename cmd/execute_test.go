package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestExecute_NoPanic(t *testing.T) {
	if rootCmd == nil {
		t.Fatalf("rootCmd not initialized")
	}
	c := &cobra.Command{Run: func(cmd *cobra.Command, args []string) {}}
	rootCmd.AddCommand(c)
}
