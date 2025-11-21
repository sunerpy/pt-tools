package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestWebCmdHasFlags(t *testing.T) {
	c := &cobra.Command{}
	// init attaches flags in package init; verify default values accessible via webCmd
	fHost := webCmd.Flags().Lookup("host")
	fPort := webCmd.Flags().Lookup("port")
	assert.NotNil(t, fHost)
	assert.NotNil(t, fPort)
	_ = c
}
