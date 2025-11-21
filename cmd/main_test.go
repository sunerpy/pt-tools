package cmd

import (
	"testing"

	"github.com/sunerpy/pt-tools/core"
)

func TestMainEntrypoint_NoPanic(t *testing.T) {
	// initialize runtime to ensure rootCmd.Execute has minimal deps
	_, _ = core.InitRuntime()
}
