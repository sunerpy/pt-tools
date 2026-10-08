package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/version"
)

// pt-tools mcp（路线图 M14）：stdio 到 HTTP 的 MCP 桥，给只支持 stdio 的客户端用。它连上正在运行的 pt-tools 的 /mcp，
// 把那里的工具原样挂到本地 stdio 服务上转发。不打开数据库，也不启动调度器：pt-tools 只有一个进程在写。

var (
	mcpBridgeURL   string
	mcpBridgeToken string
)

// mcpDiscardKey 删掉本进程启动时新生成的密钥文件（测试里换掉，不动测试进程的密钥）。
var mcpDiscardKey = crypto.DiscardGeneratedKey

// mcpDefaultURL 是没给地址时连的 pt-tools。
const mcpDefaultURL = "http://127.0.0.1:8080"

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "把运行中的 pt-tools 的 MCP 工具接到标准输入输出",
	Long: `连上 --url 指定的 pt-tools（http://主机:端口，或者带路径的完整 MCP 地址），用 --token 的 API 令牌（要有 mcp:read 或 mcp:write）
调用它的 MCP 工具，在标准输入输出上提供同样的工具，给只支持 stdio 的 MCP 客户端用。
地址也可以用环境变量 PT_TOOLS_MCP_URL 给（默认 ` + mcpDefaultURL + `），令牌用 PT_TOOLS_MCP_TOKEN（放在命令行里会被同一台机器上的其他用户从进程列表看到）。
这个命令不打开数据库，也不启动调度器。`,
	Example: `  PT_TOOLS_MCP_TOKEN=ptt_… pt-tools mcp --url http://192.168.1.10:8080`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		// 桥用不到加密密钥：进程启动时 ~/.pt-tools 里没有 secret.key 而新生成了一个的，删掉它，
		// 客户端那台机器上不留一个和服务端无关的密钥（以后在这里启动 pt-tools 时它会被当成正常密钥）
		if err := mcpDiscardKey(); err != nil {
			fmt.Fprintf(os.Stderr, "pt-tools MCP：删除用不到的密钥文件失败: %v\n", err)
		}
		endpoint, err := mcpEndpoint(firstNonEmpty(mcpBridgeURL, os.Getenv("PT_TOOLS_MCP_URL"), mcpDefaultURL))
		if err != nil {
			return err
		}
		token := firstNonEmpty(mcpBridgeToken, os.Getenv("PT_TOOLS_MCP_TOKEN"))
		if token == "" {
			return errors.New("要给 API 令牌：--token 或者环境变量 PT_TOOLS_MCP_TOKEN（在网页「系统 → API 令牌」新建，选 MCP 读取或 MCP 操作）")
		}
		return runMCPBridge(cmd.Context(), endpoint, token, &sdk.StdioTransport{})
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
	mcpCmd.Flags().StringVar(&mcpBridgeURL, "url", "", "pt-tools 的地址（默认环境变量 PT_TOOLS_MCP_URL，再默认 "+mcpDefaultURL+"）")
	mcpCmd.Flags().StringVar(&mcpBridgeToken, "token", "", "API 令牌（默认环境变量 PT_TOOLS_MCP_TOKEN）")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// mcpEndpoint 检查地址并补上 /mcp（只给了主机与端口时）。
func mcpEndpoint(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("地址要以 http:// 或 https:// 开头，带主机名: %q", raw)
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/mcp"
	}
	return u.String(), nil
}

// bearerRoundTripper 给发往 pt-tools 的每个请求带上令牌。
type bearerRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (b bearerRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}

// runMCPBridge 连上远端的 MCP，读出工具清单挂到本地服务上（调用原样转发），在 local 上跑到客户端断开或 ctx 结束。
// 提示信息写到标准错误：标准输出是协议用的。
func runMCPBridge(ctx context.Context, endpoint, token string, local sdk.Transport) error {
	remote := &sdk.StreamableClientTransport{
		Endpoint: endpoint, HTTPClient: &http.Client{Transport: bearerRoundTripper{token: token, base: http.DefaultTransport}},
		DisableStandaloneSSE: true,
	}
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "pt-tools-mcp-bridge", Version: version.GetVersionInfo().Version}, nil).Connect(ctx, remote, nil)
	if err != nil {
		return fmt.Errorf("连不上 %s（地址、令牌对吗？令牌要有 mcp:read 或 mcp:write）: %w", endpoint, err)
	}
	defer func() { _ = cs.Close() }()
	info := cs.InitializeResult()
	server := sdk.NewServer(&sdk.Implementation{Name: info.ServerInfo.Name, Title: info.ServerInfo.Title, Version: info.ServerInfo.Version},
		&sdk.ServerOptions{Instructions: info.Instructions})
	n := 0
	for tool, terr := range cs.Tools(ctx, nil) {
		if terr != nil {
			return fmt.Errorf("读取 %s 的工具清单失败: %w", endpoint, terr)
		}
		server.AddTool(tool, func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return cs.CallTool(ctx, &sdk.CallToolParams{Name: req.Params.Name, Arguments: req.Params.Arguments})
		})
		n++
	}
	fmt.Fprintf(os.Stderr, "pt-tools MCP：已连上 %s，%d 个工具\n", endpoint, n)
	return server.Run(ctx, local)
}
