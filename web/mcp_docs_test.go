package web

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/mcp"
)

// 「系统 → MCP 接入」页（config/mcp.ts）与使用说明（中英）列出的工具要和 internal/mcp 的 ToolNames 一致：加了、改名了工具，三处一起改
func TestMCPToolListsInSync(t *testing.T) {
	for _, path := range []string{"frontend/src/config/mcp.ts", "../docs/guide/mcp.md", "../docs/en/guide/mcp.md"} {
		b, err := os.ReadFile(path)
		require.NoError(t, err, path)
		text := string(b)
		for _, name := range mcp.ToolNames {
			assert.Truef(t, strings.Contains(text, "`"+name+"`") || strings.Contains(text, `"`+name+`"`), "%s 里没有工具 %s", path, name)
		}
	}
}
