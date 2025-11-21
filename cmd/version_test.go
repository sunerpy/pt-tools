package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestVersionCmd_RunDoesNotPanic(t *testing.T) {
	c := &cobra.Command{}
	versionCmd.Run(c, []string{})
}
