package mcp

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

func TestNewMCPServer_Constructs(t *testing.T) {
	t.Parallel()
	srv := NewMCPServer(Deps{SourceReg: core.NewRegistry[core.MediaScraper]()})
	require.NotNil(t, srv)
}
