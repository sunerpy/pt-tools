package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/sunerpy/pt-tools/internal/remote"
)

func TestVersionCmd_RunDoesNotPanic(t *testing.T) {
	c := &cobra.Command{}
	versionCmd.Run(c, []string{})
}

// 默认 relay：只说有没有（不打印地址），注入了不合法的地址时是 invalid
func TestDefaultRelayState(t *testing.T) {
	old := remote.DefaultRelayURL
	t.Cleanup(func() { remote.DefaultRelayURL = old })
	remote.DefaultRelayURL = ""
	assert.Equal(t, "none", defaultRelayState())
	remote.DefaultRelayURL = "wss://relay.example.com"
	assert.Equal(t, "configured", defaultRelayState())
	remote.DefaultRelayURL = "https://relay.example.com"
	assert.Equal(t, "invalid", defaultRelayState())
}
