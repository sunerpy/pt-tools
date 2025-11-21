package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestRootCmd_RunDelegatesToWeb(t *testing.T) {
	called := false
	oldRun := webCmd.Run
	webCmd.Run = func(cmd *cobra.Command, args []string) { called = true }
	defer func() { webCmd.Run = oldRun }()
	rootCmd.Run(&cobra.Command{}, []string{})
	assert.True(t, called)
}

func TestExecute_DelegatesToWeb(t *testing.T) {
	called := false
	oldRun := webCmd.Run
	webCmd.Run = func(cmd *cobra.Command, args []string) { called = true }
	defer func() { webCmd.Run = oldRun }()
	Execute()
	assert.True(t, called)
}
