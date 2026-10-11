package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"

	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/mcp"
	"github.com/sunerpy/pt-tools/internal/redact"
)

// maxToolResult 是交给模型的工具结果的上限（字节），超过的截断并注明。
const maxToolResult = 16 << 10

// toolbox 是一次运行连上的 MCP 工具：进程内的服务端与客户端会话，以及给模型的工具定义。
type toolbox struct {
	cs    *sdk.ClientSession
	ss    *sdk.ServerSession
	write map[string]bool
	defs  []openai.ChatCompletionToolUnionParam
}

// caller 是这次运行在 MCP 里的调用方：属主当审计的触发用户，能写时才有 mcp:write。
func caller(a Actor) mcp.Caller {
	c := mcp.Caller{Kind: mcp.KindAssistant, Subject: a.Owner, Name: a.Name, Scopes: []string{apitoken.ScopeMCPRead}}
	if a.CanWrite {
		c.Scopes = append(c.Scopes, apitoken.ScopeMCPWrite)
	}
	return c
}

// openToolbox 建 MCP 服务，经内存传输连一个客户端，把工具定义换成模型用的（写工具去掉 confirm；不能写时不给写工具）。
func (s *Service) openToolbox(ctx context.Context, a Actor) (*toolbox, error) {
	if s.cfg.Tools == nil {
		return nil, errors.New("工具没有接上")
	}
	srvT, cliT := sdk.NewInMemoryTransports()
	ss, err := s.cfg.Tools(caller(a)).Connect(ctx, srvT, nil)
	if err != nil {
		return nil, fmt.Errorf("连接工具失败: %w", err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "pt-tools-assistant", Version: s.cfg.Version}, nil)
	cs, err := client.Connect(ctx, cliT, nil)
	if err != nil {
		_ = ss.Close()
		return nil, fmt.Errorf("连接工具失败: %w", err)
	}
	tb := &toolbox{cs: cs, ss: ss, write: map[string]bool{}}
	for tool, err := range cs.Tools(ctx, nil) {
		if err != nil {
			tb.Close()
			return nil, fmt.Errorf("读取工具列表失败: %w", err)
		}
		write := tool.Annotations == nil || !tool.Annotations.ReadOnlyHint
		if write && !a.CanWrite {
			continue
		}
		schema, err := modelSchema(tool.InputSchema, write)
		if err != nil {
			tb.Close()
			return nil, fmt.Errorf("工具 %s 的参数: %w", tool.Name, err)
		}
		desc := tool.Description
		if write {
			desc = modelDescription(desc)
		}
		tb.write[tool.Name] = write
		tb.defs = append(tb.defs, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name: tool.Name, Description: openai.String(desc), Parameters: schema,
		}))
	}
	return tb, nil
}

// Close 关掉客户端与服务端会话。
func (t *toolbox) Close() {
	_ = t.cs.Close()
	_ = t.ss.Close()
}

// known 报告这次运行有没有这个工具，以及是不是写工具。
func (t *toolbox) known(name string) (write, ok bool) {
	write, ok = t.write[name]
	return write, ok
}

// modelSchema 把 MCP 工具的参数 Schema 换成给模型的：写工具去掉 confirm（确认由 pt-tools 问用户，模型不能自己确认）。
func modelSchema(in any, write bool) (shared.FunctionParameters, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil || schema == nil {
		return nil, errors.New("不是 JSON 对象")
	}
	if write {
		if props, ok := schema["properties"].(map[string]any); ok {
			delete(props, "confirm")
		}
		if req, ok := schema["required"].([]any); ok {
			req = slices.DeleteFunc(req, func(v any) bool { return v == "confirm" })
			if len(req) == 0 {
				delete(schema, "required")
			} else {
				schema["required"] = req
			}
		}
	}
	return shared.FunctionParameters(schema), nil
}

// confirmPhrase 是 MCP 写工具说明里叫客户端自己确认的那句。
var confirmPhrase = regexp.MustCompile(`(?i)ask first and pass confirm=true\.?`)

// modelDescription 把写工具的说明改成「执行前 pt-tools 会请用户确认」。
func modelDescription(desc string) string {
	const note = "pt-tools asks the user to confirm before it runs; just call it when the user wants this."
	if confirmPhrase.MatchString(desc) {
		return confirmPhrase.ReplaceAllString(desc, note)
	}
	return strings.TrimSpace(desc + " " + note)
}

// toolResult 是一次工具调用交给模型的结果。
type toolResult struct {
	text    string
	isError bool
}

// call 调一个工具，结果是 App API 回的 JSON（本来就不带凭证，再过一遍 redact.Credentials），超过上限截断。
func (t *toolbox) call(ctx context.Context, name string, args map[string]any) toolResult {
	res, err := t.cs.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		if ctx.Err() != nil {
			return toolResult{text: "运行已停止，工具没有执行完。", isError: true}
		}
		return toolResult{text: "调用工具失败：" + redact.Secrets(err.Error()), isError: true}
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(tc.Text)
		}
	}
	text := redact.Credentials(b.String())
	if res.IsError {
		text = redact.Secrets(text)
	}
	return toolResult{text: truncate(text, maxToolResult), isError: res.IsError}
}

// truncate 把文字截到 limit 字节以内（不切开一个字），截了的注明。
func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + fmt.Sprintf("\n…（结果太长，只保留了前 %d KB）", limit>>10)
}

// pendingCall 是模型的一个工具调用，参数已经规范化：去掉了 confirm，push_torrent 的 torrent_url 换成了站点与种子编号。
type pendingCall struct {
	id, name string
	args     map[string]any
	// display 是落库与下发用的参数（规范化以后的 JSON，再过一遍脱敏）
	display string
	// invalid 不为空时不执行，直接把它当工具错误交回模型
	invalid string
}

// normalize 规范化模型给的参数。原始参数（可能带 passkey 的地址）只在这里经手，不落库、不下发。
func (s *Service) normalize(id, name, raw string) pendingCall {
	pc := pendingCall{id: id, name: name, args: map[string]any{}}
	raw = strings.TrimSpace(raw)
	if raw != "" && raw != "null" {
		if err := json.Unmarshal([]byte(raw), &pc.args); err != nil || pc.args == nil {
			pc.args = map[string]any{}
			pc.invalid = "参数不是 JSON 对象"
			pc.display = redact.Secrets(raw)
			return pc
		}
	}
	delete(pc.args, "confirm")
	if name == "push_torrent" {
		if u, ok := pc.args["torrent_url"].(string); ok && strings.TrimSpace(u) != "" {
			delete(pc.args, "torrent_url")
			site, tid, ok := "", "", false
			if s.cfg.ResolveURL != nil {
				site, tid, ok = s.cfg.ResolveURL(strings.TrimSpace(u))
			}
			switch {
			case !ok:
				pc.invalid = "认不出这个地址：只收已配置站点的种子下载地址（磁力链接不收）。用 search_torrents 找到种子，再用 site 与 torrent_id 推送"
			case pc.args["site"] != nil || pc.args["torrent_id"] != nil:
				pc.invalid = "torrent_url 与 site、torrent_id 只给一种"
			default:
				pc.args["site"], pc.args["torrent_id"] = site, tid
			}
		}
	}
	pc.display = displayArgs(pc.args)
	return pc
}

// displayArgs 是参数的 JSON（键排好序），再过一遍 redact.Credentials。
func displayArgs(args map[string]any) string {
	buf, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return redact.Credentials(string(buf))
}

// 写工具在确认卡片与 ChatOps 提示里的名字。
var toolTitles = map[string]string{
	"pause_torrent":    "暂停种子",
	"resume_torrent":   "继续种子",
	"delete_torrent":   "删除种子",
	"push_torrent":     "推送种子到下载器",
	"add_subscription": "添加订阅",
}

// 参数在摘要里的叫法（按这个顺序列出）。
var argLabels = []struct{ key, label string }{
	{"site", "站点"},
	{"torrent_id", "种子编号"},
	{"downloader_id", "下载器"},
	{"task_id", "任务"},
	{"remove_data", "同时删除文件"},
	{"category", "分类"},
	{"tags", "标签"},
	{"save_path", "保存目录"},
	{"media_type", "类型"},
	{"tmdb_id", "TMDB 编号"},
	{"season", "季"},
	{"upgrade", "洗版"},
}

// summarize 是一个写操作给人看的一句话，例如「暂停种子：下载器 1，任务 abc」。
func summarize(tool string, args map[string]any) string {
	title := toolTitles[tool]
	if title == "" {
		title = tool
	}
	var parts []string
	seen := map[string]bool{}
	for _, l := range argLabels {
		if v, ok := args[l.key]; ok {
			seen[l.key] = true
			if s := argText(v); s != "" {
				parts = append(parts, l.label+" "+s)
			}
		}
	}
	for _, k := range slices.Sorted(maps.Keys(args)) {
		if !seen[k] {
			if s := argText(args[k]); s != "" {
				parts = append(parts, k+" "+s)
			}
		}
	}
	if len(parts) == 0 {
		return title
	}
	return redact.Credentials(title + "：" + strings.Join(parts, "，"))
}

func argText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case bool:
		if x {
			return "是"
		}
		return "否"
	case string:
		return x
	case float64:
		return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%f", x), "0"), ".")
	default:
		buf, _ := json.Marshal(x)
		return string(buf)
	}
}
