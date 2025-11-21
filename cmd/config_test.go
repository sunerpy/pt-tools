package cmd

import (
	"testing"
)

func TestConfigCmd_UsageRun(t *testing.T) {
	// call config command Run path; it should print usage and not panic
	configCmd.Run(configCmd, []string{})
}
