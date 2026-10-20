package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sunerpy/pt-tools/internal/apitoken"
)

// 配置工具（第五批）：get_config 读、change_config 改 pt-tools 的配置，get_skill 给出怎么用它们的说明（内置技能）。
// 都经 App API 的 /config 接口（web 的配置网关）：令牌另外要有 app:config。change_config 是写工具：
// 不带 confirm=true 时回预览（改以前、改以后与可能的影响），密钥只写不读。

// ConfigTools 是要 app:config 的工具：内置 AI 助手在入口没有配置权限时不把它们交给模型。
var ConfigTools = []string{"get_config", "change_config"}

// SkillConfig 是配置技能的名字；SkillTool 是取技能的工具。
const (
	SkillConfig = "pt-tools-config"
	SkillTool   = "get_skill"
)

type getConfigIn struct {
	Resource string `json:"resource,omitempty" jsonschema:"configuration resource name from the catalog; omit it to get the catalog (every resource, its fields, operations and secrets)"`
	ID       string `json:"id,omitempty" jsonschema:"for list resources: only this item (the value of its id_field)"`
}

type changeConfigIn struct {
	Resource string         `json:"resource" jsonschema:"configuration resource name from get_config"`
	Op       string         `json:"op" jsonschema:"set for object resources; add, update or delete for list resources"`
	ID       string         `json:"id,omitempty" jsonschema:"for update and delete: the item's id_field value from get_config"`
	Values   map[string]any `json:"values,omitempty" jsonschema:"only the fields to change (field names from the catalog); secrets: a new value changes it, an empty string clears it, the placeholder <已设置> keeps it"`
	Confirm  bool           `json:"confirm,omitempty" jsonschema:"true to run it; without it nothing changes and you get a preview (changes and impact) to show the user first"`
}

type getSkillIn struct {
	Name string `json:"name,omitempty" jsonschema:"skill name; omit it to list the built-in skills"`
}

// configPath 是 App API 里一类配置的地址。
func configPath(resource, suffix string) string {
	return "/config/" + url.PathEscape(strings.TrimSpace(resource)) + suffix
}

func registerConfigTools(s *sdk.Server, d *Deps) {
	register(s, d, spec{
		name: "get_config", title: "Read pt-tools configuration",
		description: "Read pt-tools configuration (needs the app:config scope). Without resource: the catalog of resources " +
			"(global settings, downloaders, sites, RSS, filter rules, notification channels, templates, sign-in, media library and more), " +
			"with each one's operations, writable fields, required fields and secret fields. With resource: its current values. " +
			"Secrets are never returned: they read as <已设置> or an empty string. Read get_skill pt-tools-config first for the rules.",
	}, func(in getConfigIn) (call, error) {
		if strings.TrimSpace(in.Resource) == "" {
			return call{req: Request{Method: http.MethodGet, Path: "/config"}}, nil
		}
		return call{req: Request{Method: http.MethodGet, Path: withQuery(configPath(in.Resource, ""), "id", in.ID)}}, nil
	})

	registerWrite(s, d, spec{
		name: "change_config", title: "Change pt-tools configuration", destructive: true,
		enums: map[string][]any{"op": {"set", "add", "update", "delete"}},
		description: "Change pt-tools configuration (needs the app:config scope): op=set for object resources, add/update/delete for list resources; " +
			"pass only the fields to change and take ids from get_config. A call without confirm changes nothing and returns a preview " +
			"(each field's before and after, and the possible impact) to show the user. Changes the user's configuration: ask first and pass confirm=true.",
	}, func(ctx context.Context, d *Deps, c Caller, in changeConfigIn) (Preview, error) {
		var res configResult
		body := map[string]any{"op": in.Op, "id": in.ID, "values": in.Values}
		if err := readJSON(ctx, d, c, http.MethodPost, configPath(in.Resource, "/preview"), body, &res); err != nil {
			return Preview{}, err
		}
		return res.preview(), nil
	}, func(in changeConfigIn) (call, error) {
		if !in.Confirm {
			return call{}, errConfirm
		}
		fields := make([]string, 0, len(in.Values))
		for k := range in.Values {
			fields = append(fields, k)
		}
		sort.Strings(fields)
		return call{
			req:   Request{Method: http.MethodPost, Path: configPath(in.Resource, "/changes"), Body: map[string]any{"op": in.Op, "id": in.ID, "values": in.Values}},
			audit: map[string]any{"resource": in.Resource, "op": in.Op, "id": in.ID, "fields": fields},
		}, nil
	})

	s.AddTool(&sdk.Tool{
		Name: SkillTool, Title: "Built-in skill",
		Description: "Get a built-in skill: instructions for doing a kind of task with these tools. Without name: the list of skills. " +
			SkillConfig + " explains how to read and change pt-tools configuration with get_config and change_config, with the catalog of resources.",
		InputSchema: toolSchema[getSkillIn](spec{name: SkillTool}),
		Annotations: &sdk.ToolAnnotations{Title: "Built-in skill", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: new(bool)},
	}, func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		c, ok := d.Caller(req)
		if !ok {
			return toolError(fmt.Errorf("没有 API 令牌")), nil
		}
		if !c.Has(apitoken.ScopeMCPRead) && !c.Has(apitoken.ScopeMCPWrite) {
			return toolError(fmt.Errorf("令牌没有 %s 权限", apitoken.ScopeMCPRead)), nil
		}
		var in getSkillIn
		if req.Params != nil && len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &in); err != nil {
				return toolError(fmt.Errorf("参数不对: %w", err)), nil
			}
		}
		name := strings.TrimSpace(in.Name)
		if name == "" {
			out := map[string]any{"skills": []map[string]string{{"name": SkillConfig, "description": "读、改 pt-tools 的配置（get_config、change_config）：步骤、规则、例子与配置目录"}}}
			text, _ := json.Marshal(out)
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(text)}}, StructuredContent: out}, nil
		}
		if name != SkillConfig {
			return toolError(fmt.Errorf("没有这个技能：%s（不带 name 调用列出全部）", name)), nil
		}
		text := configSkill(ctx, d, c)
		return &sdk.CallToolResult{
			Content:           []sdk.Content{&sdk.TextContent{Text: text}},
			StructuredContent: map[string]any{"name": SkillConfig, "content": text},
		}, nil
	})
}

// configResult 是 App API 配置预览与执行的回应。
type configResult struct {
	Summary string `json:"summary"`
	Changes []struct {
		Label  string `json:"label"`
		Before string `json:"before"`
		After  string `json:"after"`
		Secret bool   `json:"secret"`
	} `json:"changes"`
	Impact []string `json:"impact"`
}

func (r configResult) preview() Preview {
	pv := Preview{Summary: r.Summary, Impact: r.Impact}
	for _, c := range r.Changes {
		pv.Changes = append(pv.Changes, Change{Label: c.Label, Before: c.Before, After: c.After, Secret: c.Secret})
	}
	return pv
}

// configCatalog 是 App API GET /config 的回应（只取技能里要的）。
type configCatalog struct {
	Resources []struct {
		Name        string            `json:"name"`
		Title       string            `json:"title"`
		Description string            `json:"description"`
		Kind        string            `json:"kind"`
		IDField     string            `json:"id_field"`
		Ops         []string          `json:"ops"`
		Fields      []string          `json:"fields"`
		Required    []string          `json:"required"`
		Secrets     []string          `json:"secrets"`
		Hints       map[string]string `json:"hints"`
	} `json:"resources"`
	Excluded []string `json:"excluded"`
}

// configSkillGuide 是配置技能里不随配置目录变的部分。
const configSkillGuide = `# pt-tools 配置技能

用 get_config 读、change_config 改 pt-tools 的配置。令牌要有 app:config（另外读要 mcp:read，改要 mcp:write）。

## 步骤

1. 不知道是哪一类配置时，先看下面的配置目录（也可以 get_config 不带 resource）：每类配置的 name、能做的操作、能改的字段、新增时必填的字段、密钥字段与字段说明。
2. get_config 带上 resource 看现状（列表类可以再带 id 只看一项）。编号（id）一定从这里读，不要猜。
3. change_config 先不带 confirm：pt-tools 回一份预览，列出每个字段改以前、改以后，以及可能的影响。把预览照实告诉用户。
4. 用户同意以后带 confirm=true 再调一次才会保存（内置 AI 助手里由 pt-tools 弹出确认，用户点了「确认执行」才执行）。结果里的 item 是改完的样子，新增出来的编号在 id。

## 规则

- 对象类（kind=object）用 op=set；列表类（kind=list）用 add（新增）、update（改）、delete（删），update 与 delete 要给 id。
- values 只写要改的字段，没写的保持原样；数字、开关、数组照 get_config 读出来的类型写。
- 密钥只写不读：读出来是 <已设置>（有值）或空串（没值）。要改就给新值；交回 <已设置> 表示不改；给空串表示清除。
- 用户在对话里贴的长串（Cookie、passkey、令牌、API Key）交给你以前已经被 pt-tools 遮住：这类密钥请用户到网页上填，不要自己编一个。
- 关联到别的配置的字段填编号：下载器填 downloaders 的 id，通知通道填 notify_channels 的 id，过滤规则填 filter_rules 的 id，站点填 sites 的 name。
- 删除不能恢复，先确认用户确实要删。改动失败时 pt-tools 会回原因（比如字段不对、必填没填），照原因改了再试，不要换个字段硬改。
- 一次 change_config 只改一类配置的一项（对象类就是那一份设置）；要改几项就调几次，每次都先预览。

## 例子

- 停用编号 2 的下载器：{"resource":"downloaders","op":"update","id":"2","values":{"enabled":false}}
- 全局下载目录改成 /data/pt：{"resource":"global","op":"set","values":{"download_dir":"/data/pt"}}
- 打开 hdsky 的每日自动签到：{"resource":"sites","op":"update","id":"hdsky","values":{"attendance_enabled":true}}
- 给 hdsky 加一条 RSS，推到编号 1 的下载器：{"resource":"rss","op":"add","values":{"site":"hdsky","name":"电影","url":"<用户给的 RSS 地址>","downloader_id":1}}
- Telegram 通道换一个接收的群：{"resource":"notify_channels","op":"update","id":"3","values":{"config.default_chat_id":"-100123"}}
`

// configSkill 是配置技能的全文：固定的说明，加上从 App API 读到的配置目录。读不到目录（比如令牌没有 app:config）时写明原因。
func configSkill(ctx context.Context, d *Deps, c Caller) string {
	var b strings.Builder
	b.WriteString(configSkillGuide)
	var cat configCatalog
	err := readJSON(ctx, d, c, http.MethodGet, "/config", nil, &cat)
	if err != nil {
		fmt.Fprintf(&b, "\n## 配置目录\n\n读不到配置目录：%s。\n", oneLine(err.Error(), 200))
		return b.String()
	}
	b.WriteString("\n## 不能经配置工具改的\n\n请用户到 pt-tools 网页上改：")
	b.WriteString(strings.Join(cat.Excluded, "、"))
	b.WriteString("。\n\n## 配置目录\n")
	for _, r := range cat.Resources {
		fmt.Fprintf(&b, "\n### %s（%s，%s）\n\n%s\n\n", r.Name, r.Title, r.Kind, r.Description)
		fmt.Fprintf(&b, "- 操作：%s\n", strings.Join(r.Ops, "、"))
		if r.Kind == "list" {
			fmt.Fprintf(&b, "- 编号字段：%s\n", r.IDField)
		}
		fmt.Fprintf(&b, "- 能改的字段：%s\n", strings.Join(r.Fields, "、"))
		if len(r.Required) > 0 {
			fmt.Fprintf(&b, "- 新增时必填：%s\n", strings.Join(r.Required, "、"))
		}
		if len(r.Secrets) > 0 {
			fmt.Fprintf(&b, "- 密钥（只写不读）：%s\n", strings.Join(r.Secrets, "、"))
		}
		keys := make([]string, 0, len(r.Hints))
		for k := range r.Hints {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "- %s：%s\n", k, r.Hints[k])
		}
	}
	return b.String()
}
