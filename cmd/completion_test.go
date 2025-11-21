package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestCompletionCmd_BashZsh(t *testing.T) {
	c := &cobra.Command{}
	c.AddCommand(completionCmd)
	completionCmd.Run(c, []string{"bash"})
	completionCmd.Run(c, []string{"zsh"})
}

// unsupported path intentionally not called to avoid exiting process
