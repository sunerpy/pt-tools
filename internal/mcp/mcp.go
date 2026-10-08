// Package mcp 是 pt-tools 的 MCP（Model Context Protocol）服务（路线图 M14）：工具定义、权限检查与写操作的审计。
//
// 工具本身不碰数据库与下载器：经 Backend 在进程内调用 App API v1（web 实现），所以返回的字段、脱敏与 App API 完全一样。
// 传输在别处接：主端口的 streamable HTTP（/mcp，web 包）与 stdio 桥（pt-tools mcp，cmd 包）。
//
// 权限分两层：HTTP 层只放行有 mcp:read 或 mcp:write 的 API 令牌；工具分派时只读工具要 mcp:read，写工具要 mcp:write，
// 写工具还要 confirm=true。写工具的每次调用（包括被拒的）记操作审计，通道是 mcp，命令是工具名。
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/app"
)

// ChannelType 是 MCP 写工具在操作审计里的通道。
const ChannelType = "mcp"

// Caller 是调用工具的 API 令牌。
type Caller struct {
	TokenID uint
	Name    string
	Scopes  []string
}

// Has 报告令牌有没有这个权限范围（精确匹配）。
func (c Caller) Has(scope string) bool { return slices.Contains(c.Scopes, scope) }

// tokenExtra 是 TokenInfo.Extra 里放令牌编号与名字的键。
const (
	extraTokenID = "token_id"
	extraName    = "name"
)

// TokenInfo 把令牌换成 go-sdk 的 TokenInfo（HTTP 层校验令牌以后放进请求，工具从 req.Extra.TokenInfo 取回来）。
// expires 为零表示不过期。
func TokenInfo(c Caller, expires time.Time) *auth.TokenInfo {
	return &auth.TokenInfo{
		Scopes: slices.Clone(c.Scopes), Expiration: expires, UserID: "token:" + strconv.FormatUint(uint64(c.TokenID), 10),
		Extra: map[string]any{extraTokenID: c.TokenID, extraName: c.Name},
	}
}

// CallerFromRequest 从工具请求里取调用的令牌（HTTP 层放进去的 TokenInfo）；没有时 ok 为假。
func CallerFromRequest(req *sdk.CallToolRequest) (Caller, bool) {
	if req == nil || req.Extra == nil || req.Extra.TokenInfo == nil {
		return Caller{}, false
	}
	ti := req.Extra.TokenInfo
	id, _ := ti.Extra[extraTokenID].(uint)
	name, _ := ti.Extra[extraName].(string)
	if id == 0 {
		return Caller{}, false
	}
	return Caller{TokenID: id, Name: name, Scopes: slices.Clone(ti.Scopes)}, true
}

// Request 是一次对 App API v1 的进程内调用。
type Request struct {
	Method string
	// Path 不带 /api/app/v1 前缀，可以带查询串
	Path string
	// Body 不为 nil 时按 JSON 编码
	Body any
	// Write 为真时按写接口调用（要 mcp:write）
	Write bool
}

// Response 是 App API 的回应。
type Response struct {
	Status int
	Body   []byte
	// Outcome 是写接口记下的业务结果（推送被拦下、批量动作全失败这类回 200 的失败），用审计的写法
	Outcome string
}

// Backend 在进程内调用 App API v1（web.Server 实现）。
type Backend interface {
	Call(ctx context.Context, c Caller, req Request) (Response, error)
}

// Auditor 记操作审计（app.AuditService 实现了它）。
type Auditor interface {
	Record(ctx context.Context, e app.AuditEntry) error
}

// Deps 是 MCP 服务的依赖。
type Deps struct {
	Backend Backend
	Audit   Auditor
	// ResolveURL 把种子下载地址解析成已知站点与种子编号（push_torrent 的 torrent_url）；不认识时 ok 为假
	ResolveURL func(rawURL string) (site, torrentID string, ok bool)
	// Version 是 pt-tools 的版本（initialize 时报给客户端）
	Version string
	// Caller 从工具请求里取令牌，默认 CallerFromRequest（测试里换掉）
	Caller func(req *sdk.CallToolRequest) (Caller, bool)
}

// NewServer 建 MCP 服务，注册全部工具。
func NewServer(d Deps) *sdk.Server {
	if d.Caller == nil {
		d.Caller = CallerFromRequest
	}
	s := sdk.NewServer(&sdk.Implementation{Name: "pt-tools", Title: "pt-tools", Version: d.Version}, &sdk.ServerOptions{
		Instructions: "pt-tools manages private-tracker (PT) sites, downloaders (qBittorrent/Transmission) and media subscriptions. " +
			"Use read tools freely. Write tools (pause/resume/delete/push torrents, add subscriptions) change the user's downloaders " +
			"and require confirm=true: ask the user before calling them.",
	})
	registerTools(s, &d)
	return s
}

// errAPI 是 App API 回的错误：{"error": 代码, "message": 说明}。
type errAPI struct {
	Status  int
	Code    string `json:"error"`
	Message string `json:"message"`
}

func (e *errAPI) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("pt-tools 回 %d", e.Status)
	}
	return fmt.Sprintf("%s（%d %s）", e.Message, e.Status, e.Code)
}

// errConfirm 是写工具没有 confirm=true。
var errConfirm = errors.New("这个工具会改动下载器或订阅：先向用户确认，再带上 confirm=true 调用")

// spec 是一个工具的定义：名字、说明、是不是写工具、参数的取值范围（枚举）。
type spec struct {
	name, title, description string
	write                    bool
	// destructive 是会删东西的写工具（客户端据此提醒用户）
	destructive bool
	enums       map[string][]any
}

// call 是工具的一次调用：要发给 App API 的请求、审计里记的参数，以及怎么处理回应（nil 时原样返回）。
type call struct {
	req   Request
	audit map[string]any
	shape func(out map[string]any) map[string]any
}

// register 注册一个工具：检查权限范围（写工具另要 confirm=true），在进程内调用 App API，把 JSON 回应原样交给客户端；
// 写工具每次调用都记审计（被拒的也记）。
func register[In any](s *sdk.Server, d *Deps, sp spec, build func(in In) (call, error)) {
	schema, err := jsonschema.For[In](nil)
	if err != nil {
		panic(fmt.Sprintf("mcp: 工具 %s 的参数: %v", sp.name, err))
	}
	for field, values := range sp.enums {
		if p := schema.Properties[field]; p != nil {
			p.Enum = values
		}
	}
	ann := &sdk.ToolAnnotations{Title: sp.title, ReadOnlyHint: !sp.write, IdempotentHint: !sp.write}
	if sp.write {
		ann.DestructiveHint = &sp.destructive
	}
	closed := false
	ann.OpenWorldHint = &closed
	tool := &sdk.Tool{Name: sp.name, Title: sp.title, Description: sp.description, InputSchema: schema, Annotations: ann}
	sdk.AddTool(s, tool, func(ctx context.Context, req *sdk.CallToolRequest, in In) (*sdk.CallToolResult, any, error) {
		start := time.Now()
		c, ok := d.Caller(req)
		if !ok {
			return nil, nil, errors.New("没有 API 令牌")
		}
		need := apitoken.ScopeMCPRead
		if sp.write {
			need = apitoken.ScopeMCPWrite
		}
		if !c.Has(need) {
			if sp.write {
				d.record(ctx, c, sp.name, "denied:scope", nil, start)
			}
			return nil, nil, fmt.Errorf("令牌没有 %s 权限", need)
		}
		cl, err := build(in)
		if err != nil {
			if sp.write {
				result := "error:invalid_argument"
				if errors.Is(err, errConfirm) {
					result = "denied:confirm"
				}
				d.record(ctx, c, sp.name, result, nil, start)
			}
			return nil, nil, err
		}
		cl.req.Write = sp.write
		resp, err := d.Backend.Call(ctx, c, cl.req)
		if err != nil {
			if sp.write {
				d.record(ctx, c, sp.name, "error:internal", cl.audit, start)
			}
			return nil, nil, err
		}
		if sp.write {
			d.record(ctx, c, sp.name, outcome(resp), cl.audit, start)
		}
		if resp.Status >= 400 {
			e := &errAPI{Status: resp.Status}
			_ = json.Unmarshal(resp.Body, e)
			return nil, nil, e
		}
		out, err := asObject(resp.Body)
		if err != nil {
			return nil, nil, err
		}
		if cl.shape != nil {
			out = cl.shape(out)
		}
		return nil, out, nil
	})
}

// asObject 把 App API 的回应换成 JSON 对象（MCP 的 structuredContent 要是对象）：本来是数组的（比如订阅列表）放进 items。
func asObject(body []byte) (map[string]any, error) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, fmt.Errorf("pt-tools 的回应不是 JSON: %w", err)
	}
	switch x := v.(type) {
	case map[string]any:
		return x, nil
	case []any:
		return map[string]any{"items": x}, nil
	}
	return nil, errors.New("pt-tools 的回应不是 JSON 对象")
}

// outcome 是写工具的审计结果：失败的状态码记 error:http_<状态码>；回 200 但业务上没成的用 App API 记下的结果。
func outcome(resp Response) string {
	if resp.Status >= 400 {
		return "error:http_" + strconv.Itoa(resp.Status)
	}
	if resp.Outcome != "" {
		return resp.Outcome
	}
	return "success"
}

// record 记一条写工具的审计；写不进去只记日志。
func (d *Deps) record(ctx context.Context, c Caller, tool, result string, args map[string]any, start time.Time) {
	if d.Audit == nil {
		return
	}
	all := map[string]any{"name": c.Name}
	for k, v := range args {
		all[k] = v
	}
	e := app.AuditEntry{
		ChannelType: ChannelType, ChannelUserID: strconv.FormatUint(uint64(c.TokenID), 10), Command: tool, Result: result,
		Args: all, LatencyMs: time.Since(start).Milliseconds(),
	}
	if err := d.Audit.Record(context.WithoutCancel(ctx), e); err != nil {
		global.GetSlogger().Warnf("[MCP] 记审计失败: %v", err)
	}
}
