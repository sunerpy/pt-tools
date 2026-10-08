package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/mcp"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/version"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// MCP（路线图 M14）：主端口的 /mcp，streamable HTTP（无状态，回 JSON）。只认 API 令牌：没有令牌或令牌不对回 401，
// 令牌既没有 mcp:read 也没有 mcp:write 回 403；网页登录的 session 不收。工具在 internal/mcp，
// 它们经 mcpBackend 在进程内调用 App API v1，所以字段与脱敏和 App API 完全一样。

// mcpPath 是 MCP 的地址。
const mcpPath = "/mcp"

// registerMCPRoutes 挂上 /mcp。
func (s *Server) registerMCPRoutes(mux *http.ServeMux) {
	mux.Handle(mcpPath, s.mcpHandler())
}

// mcpHandler 校验令牌，再交给 go-sdk 的 streamable HTTP 处理。
func (s *Server) mcpHandler() http.Handler {
	resolver := v2.NewTrackerResolver()
	server := mcp.NewServer(mcp.Deps{
		Backend: &mcpBackend{routes: s.mcpRoutes()}, Audit: s.appAudit, ResolveURL: resolver.ResolveDownloadURL,
		Version: version.GetVersionInfo().Version,
	})
	streamable := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &sdk.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true,
		// 每个请求都要令牌，DNS rebinding 拿不到令牌：关掉「经 localhost 进来、Host 却不是 localhost 就拒绝」这层保护，
		// 同一台机器上的反向代理（Host 是自己的域名）照样能用
		DisableLocalhostProtection: true,
	})
	return auth.RequireBearerToken(s.verifyMCPToken, &auth.RequireBearerTokenOptions{AllowMissingExpiration: true})(requireMCPScope(streamable))
}

// verifyMCPToken 校验 Bearer 令牌：不对回 auth.ErrInvalidToken（401）。
func (s *Server) verifyMCPToken(ctx context.Context, plain string, _ *http.Request) (*auth.TokenInfo, error) {
	if s.tokens == nil {
		return nil, auth.ErrInvalidToken
	}
	tok, err := s.tokens.Verify(ctx, plain)
	if errors.Is(err, apitoken.ErrUnauthorized) {
		return nil, auth.ErrInvalidToken
	}
	if err != nil {
		global.GetSlogger().Warnf("[MCP] 校验令牌失败: %v", err)
		return nil, errors.New("暂时不能校验令牌")
	}
	var expires time.Time
	if tok.ExpiresAt != nil {
		expires = *tok.ExpiresAt
	}
	return mcp.TokenInfo(mcp.Caller{TokenID: tok.ID, Name: tok.Name, Scopes: tok.Scopes}, expires), nil
}

// requireMCPScope 只放行有 mcp:read 或 mcp:write 的令牌（每个工具要哪一个在工具里查）。
func requireMCPScope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ti := auth.TokenInfoFromContext(r.Context())
		if ti == nil || (!slices.Contains(ti.Scopes, apitoken.ScopeMCPRead) && !slices.Contains(ti.Scopes, apitoken.ScopeMCPWrite)) {
			http.Error(w, "令牌没有 mcp:read 或 mcp:write 权限", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// mcpRoutes 是 MCP 工具在进程内调用的 App API 路由（不带 /api/app/v1 前缀）：每条先按路由声明的权限范围检查主体，
// 不经 appHandler 的主体解析与审计（写工具的审计由 internal/mcp 记，命令是工具名）。
func (s *Server) mcpRoutes() *http.ServeMux {
	mux := http.NewServeMux()
	for _, rt := range s.appRoutes() {
		mux.Handle(rt.Method+" "+rt.Path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !middleware.PrincipalFrom(r.Context()).Has(rt.Scope) {
				appError(w, http.StatusForbidden, "forbidden", "令牌没有调用这个接口的权限")
				return
			}
			rt.Handler(w, r)
		}))
	}
	return mux
}

// mcpBackend 实现 mcp.Backend：在进程内调用 App API。主体是这个令牌（种类 mcp）：mcp:read 当 app:read，mcp:write 当 app:write。
type mcpBackend struct {
	routes *http.ServeMux
}

func (b *mcpBackend) Call(ctx context.Context, c mcp.Caller, req mcp.Request) (mcp.Response, error) {
	var body io.Reader = http.NoBody
	if req.Body != nil {
		buf, err := json.Marshal(req.Body)
		if err != nil {
			return mcp.Response{}, err
		}
		body = bytes.NewReader(buf)
	}
	var scopes []string
	if c.Has(apitoken.ScopeMCPRead) {
		scopes = append(scopes, apitoken.ScopeAppRead)
	}
	if c.Has(apitoken.ScopeMCPWrite) {
		scopes = append(scopes, apitoken.ScopeAppWrite)
	}
	p := &middleware.Principal{Kind: middleware.KindMCP, ID: strconv.FormatUint(uint64(c.TokenID), 10), Name: c.Name, Scopes: scopes}
	outcome := new(string)
	ctx = context.WithValue(middleware.WithPrincipal(ctx, p), appOutcomeKey{}, outcome)
	r, err := http.NewRequestWithContext(ctx, req.Method, req.Path, body)
	if err != nil {
		return mcp.Response{}, err
	}
	if req.Body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	w := &bufferWriter{header: http.Header{}}
	b.routes.ServeHTTP(w, r)
	return mcp.Response{Status: w.statusCode(), Body: w.body.Bytes(), Outcome: *outcome}, nil
}

// bufferWriter 收下进程内调用的回应。
type bufferWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *bufferWriter) Header() http.Header { return w.header }

func (w *bufferWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
}

func (w *bufferWriter) Write(p []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	return w.body.Write(p)
}

func (w *bufferWriter) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}
