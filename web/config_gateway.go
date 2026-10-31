package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/sunerpy/pt-tools/internal/redact"
)

// 配置网关（第五批）：App API 的 /config 接口与 MCP 的 get_config、change_config 都经它读、改 pt-tools 的配置。
// 每一类配置（全局设置、下载器、站点、RSS、过滤规则、通知通道……）登记成一个资源（config_resources.go）：
// 读的时候在进程内调网页用的同一个接口（configCall），密钥换成占位；改的时候先读现状，把要改的几项并进去，
// 再交给同一个接口保存，所以校验、加密与热重载和在网页上保存时一样。改以前可以预览：改以前、改以后与可能的影响。
// 密钥只写不读：读出来只有「有没有设置」，给新值才会改，原样交回占位表示不改。
// 不在这里的（configExcluded）只能在网页上改。

// ConfigSecretSet 是读配置时有值的密钥显示成的占位；没有值时是空串。改的时候交回占位表示不改。
const ConfigSecretSet = "<已设置>"

// configExcluded 是不能经配置网关读改的：登录与授权的边界、加密密钥、AI 助手自己的接口设置。
var configExcluded = []string{
	"登录密码",
	"API 令牌",
	"远程访问（设备与配对）",
	"加密密钥",
	"AI 助手自己的设置（服务商、接口地址、API Key、模型、代理、ChatOps 开关、语音输入）",
	"ChatOps 绑定与管理员",
	"qB 兼容入口（另开的监听端口与权限）",
	"MCP 接入（开关与地址）",
	"站点代理（代理地址与走代理的站点）",
}

const (
	configObject = "object"
	configList   = "list"

	configOpSet    = "set"
	configOpAdd    = "add"
	configOpUpdate = "update"
	configOpDelete = "delete"
)

// configMaxValueText 是差异里一个值最多显示多少个字。
const configMaxValueText = 200

// ConfigResourceView 是配置目录里的一项：资源的名字、说明、能做的操作、能改的字段与哪些是密钥。
type ConfigResourceView struct {
	Name        string            `json:"name"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Kind        string            `json:"kind"`
	IDField     string            `json:"id_field,omitempty"`
	Ops         []string          `json:"ops"`
	Fields      []string          `json:"fields"`
	Required    []string          `json:"required,omitempty"`
	Secrets     []string          `json:"secrets,omitempty"`
	Hints       map[string]string `json:"hints,omitempty"`
}

// ConfigCatalog 是配置目录：能经网关读改的资源，以及不能的那些。
type ConfigCatalog struct {
	Resources         []ConfigResourceView `json:"resources"`
	Excluded          []string             `json:"excluded"`
	SecretPlaceholder string               `json:"secret_placeholder"`
}

// ConfigState 是一个资源的现状（密钥已遮住）：对象类是 item，列表类是 items（读一项时只有那一项）。
type ConfigState struct {
	Resource string           `json:"resource"`
	Kind     string           `json:"kind"`
	IDField  string           `json:"id_field,omitempty"`
	Item     map[string]any   `json:"item,omitempty"`
	Items    []map[string]any `json:"items,omitempty"`
}

// ConfigChange 是一次改动：对象类用 set；列表类用 add（新增）、update、delete（要 id）。values 只写要改的字段。
type ConfigChange struct {
	Op     string         `json:"op"`
	ID     string         `json:"id,omitempty"`
	Values map[string]any `json:"values,omitempty"`
}

// ConfigDiff 是一项改动：Label 是给人看的名字，Field 是字段；新增的没有 Before，删掉的没有 After。
// Secret 为真时是密钥，两边都不写原文。
type ConfigDiff struct {
	Label  string `json:"label"`
	Field  string `json:"field,omitempty"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Secret bool   `json:"secret,omitempty"`
}

// ConfigChangeResult 是预览或执行的结果。Applied 为假时没有改任何东西；为真时 Item 是改完的样子（删除时没有）。
type ConfigChangeResult struct {
	Resource string         `json:"resource"`
	Op       string         `json:"op"`
	ID       string         `json:"id,omitempty"`
	Summary  string         `json:"summary"`
	Changes  []ConfigDiff   `json:"changes"`
	Impact   []string       `json:"impact"`
	Applied  bool           `json:"applied"`
	Item     map[string]any `json:"item,omitempty"`
}

// configError 是网关的错误：Status 是要回的 HTTP 状态码，Message 是给人（与 AI）看的原因。
type configError struct {
	Status  int
	Message string
}

func (e *configError) Error() string { return e.Message }

func configBad(format string, args ...any) error {
	return &configError{Status: http.StatusBadRequest, Message: fmt.Sprintf(format, args...)}
}

func configNotFound(format string, args ...any) error {
	return &configError{Status: http.StatusNotFound, Message: fmt.Sprintf(format, args...)}
}

// configInternalKey 标记配置网关在进程内发出的请求：只有进程里的代码能把它放进 context，HTTP 请求伪造不了。
// s.auth 认它（见 server.go），所以网关能调只认网页登录的接口；网关只调 config_resources.go 里登记的那些。
type configInternalKey struct{}

func configInternal(ctx context.Context) bool {
	v, _ := ctx.Value(configInternalKey{}).(bool)
	return v
}

// configCall 在进程内调一次网页用的接口（buildHandler 建出的路由），回状态码与回应体。
func (s *Server) configCall(ctx context.Context, method, path string, body any) (int, []byte, error) {
	mux := s.configMux.Load()
	if mux == nil {
		return 0, nil, &configError{Status: http.StatusServiceUnavailable, Message: "pt-tools 还没有启动完，稍后再试"}
	}
	var rd io.Reader = http.NoBody
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rd = bytes.NewReader(buf)
	}
	r, err := http.NewRequestWithContext(context.WithValue(ctx, configInternalKey{}, true), method, path, rd)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	w := &bufferWriter{header: http.Header{}}
	mux.ServeHTTP(w, r)
	return w.statusCode(), w.body.Bytes(), nil
}

// configDo 调接口，不是 2xx 时换成 configError（原因取自回应：JSON 的 error/message/detail，或者纯文本）。
func (s *Server) configDo(ctx context.Context, method, path string, body any) ([]byte, error) {
	status, out, err := s.configCall(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	if status >= 200 && status < 300 {
		return out, nil
	}
	if status >= 300 && status < 400 {
		// 落到了网页的兜底路由（跳转到登录页）：这个接口没有挂上
		return nil, &configError{Status: http.StatusServiceUnavailable, Message: "这项配置的接口没有启用"}
	}
	msg := configErrorText(out)
	if msg == "" {
		msg = http.StatusText(status)
	}
	switch status {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable:
	case http.StatusUnprocessableEntity, http.StatusRequestEntityTooLarge:
		status = http.StatusBadRequest
	default:
		status = http.StatusBadGateway
	}
	return nil, &configError{Status: status, Message: msg}
}

// configErrorText 从接口的错误回应里取原因（各接口写法不一：纯文本、{"error"}、{"error","message"}、{"error","detail"}）。
func configErrorText(body []byte) string {
	text := strings.TrimSpace(string(body))
	var m map[string]any
	if json.Unmarshal(body, &m) == nil {
		var parts []string
		for _, k := range []string{"message", "detail", "error"} {
			if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
				parts = append(parts, strings.TrimSpace(v))
				break
			}
		}
		text = strings.Join(parts, "；")
	}
	text = redact.Secrets(strings.Join(strings.Fields(text), " "))
	if r := []rune(text); len(r) > 300 {
		text = string(r[:300]) + "…"
	}
	return text
}

// configDecode 解析 JSON（数字保留成 json.Number，交回接口时原样不变）。
func configDecode(body []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	return dec.Decode(out)
}

// configResource 是一类配置。默认的读写是：GET Path 读；对象 SetMethod Path 保存；列表 POST AddPath 新增、
// UpdateMethod ItemPath 修改、DELETE ItemPath 删除（ItemPath 里的 {id} 换成编号）。站点、RSS 这类不合这个样子的
// 用 load、save 换掉。
type configResource struct {
	Name, Title, Desc string
	Kind              string
	Path              string
	// Wrap 是列表（或者对象）在回应里的字段（回应本身就是那个数组或对象时为空）
	Wrap      string
	SetMethod string
	AddPath   string
	ItemPath  string
	// UpdateMethod 默认 PUT
	UpdateMethod string
	// IDField 是列表项的编号字段，默认 id；NameField 是给人看的名字，默认 name
	IDField, NameField string
	// Fields 是能改的字段：保存时只交这几项（读出来的别的字段只给看）
	Fields []string
	// AddOnly 是只在新增时能填的字段（比如通知通道的类型）
	AddOnly []string
	// Required 是新增时必须给的字段
	Required []string
	// Secrets 是密钥字段 → 读出来时表示有没有设置的字段。空串表示读出来的就是原文（要遮住）；
	// 读出来根本没有这个字段的也写空串。密钥只在给了新值时交给接口
	Secrets map[string]string
	// Resend 是没改时也要把原值交回去的密钥（接口整份保存，不交就清掉；只对读出来就是原文的密钥有用）
	Resend []string
	// Show 不为空时只给看这些字段（加上编号与能改的字段）；Hidden 是不给看的字段
	Show, Hidden []string
	// Partial 为真时修改只交改了的几项（接口按「没给不改」处理）
	Partial bool
	// Prefix 不为空时，以它开头的字段也能改（通知通道的 config.<键>：各种通道的键不一样）；
	// secretKey 判断这样的字段是不是密钥
	Prefix    string
	secretKey func(field string) bool
	// DeleteVerb、DeleteImpact 换掉删除时的说法（消息模板的删除是恢复默认）
	DeleteVerb   string
	DeleteImpact []string
	Hints        map[string]string
	Labels       map[string]string
	Impact       []string
	Ops          []string

	// load 读全部现状（原文，没遮）：对象类回一个 map，列表类回 []map
	load func(ctx context.Context) (any, error)
	// save 保存：cur 是改以前的（新增时为 nil），next 是并进改动以后的，changed 是改了的几项。回新增出来的编号（可以为空）
	save func(ctx context.Context, op, id string, cur, next, changed map[string]any) (string, error)
	// view 在默认的遮法之后再改一改给看的样子（比如通知通道配置里的密钥）
	view func(raw, out map[string]any)
	// before 在合并改动以前检查或者改写 values（比如通知通道的配置并进现有的配置）
	before func(op string, cur, values map[string]any) error
}

func (r *configResource) idField() string {
	if r.IDField != "" {
		return r.IDField
	}
	return "id"
}

func (r *configResource) nameField() string {
	if r.NameField != "" {
		return r.NameField
	}
	return "name"
}

func (r *configResource) writable(field, op string) bool {
	if slices.Contains(r.Fields, field) {
		return true
	}
	if r.Prefix != "" && strings.HasPrefix(field, r.Prefix) && len(field) > len(r.Prefix) {
		return true
	}
	if _, ok := r.Secrets[field]; ok {
		return true
	}
	return op == configOpAdd && slices.Contains(r.AddOnly, field)
}

func (r *configResource) secret(field string) bool {
	if _, ok := r.Secrets[field]; ok {
		return true
	}
	return r.secretKey != nil && r.secretKey(field)
}

// staticSecret 报告是不是登记在 Secrets 里的密钥（读出来时可能另有表示有没有设置的字段）。
func (r *configResource) staticSecret(field string) bool {
	_, ok := r.Secrets[field]
	return ok
}

// fieldList 是能改的全部字段（目录里给看），排好序。
func (r *configResource) fieldList() []string {
	out := slices.Clone(r.Fields)
	for k := range r.Secrets {
		if !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	for _, k := range r.AddOnly {
		if !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	if r.Prefix != "" {
		out = append(out, r.Prefix+"<键>")
	}
	sort.Strings(out)
	return out
}

func (r *configResource) catalogView() ConfigResourceView {
	v := ConfigResourceView{
		Name: r.Name, Title: r.Title, Description: r.Desc, Kind: r.Kind, Ops: slices.Clone(r.Ops),
		Fields: r.fieldList(), Required: slices.Clone(r.Required), Hints: r.Hints,
	}
	if r.Kind == configList {
		v.IDField = r.idField()
	}
	for k := range r.Secrets {
		v.Secrets = append(v.Secrets, k)
	}
	sort.Strings(v.Secrets)
	return v
}

// configLoad 读全部现状（原文）。
func (s *Server) configLoad(ctx context.Context, r *configResource) (any, error) {
	if r.load != nil {
		return r.load(ctx)
	}
	body, err := s.configDo(ctx, http.MethodGet, r.Path, nil)
	if err != nil {
		return nil, err
	}
	if r.Kind == configObject {
		var m map[string]any
		if err := configDecode(body, &m); err != nil {
			return nil, fmt.Errorf("读 %s 失败: %w", r.Title, err)
		}
		if r.Wrap != "" {
			inner, _ := m[r.Wrap].(map[string]any)
			if inner == nil {
				inner = map[string]any{}
			}
			return inner, nil
		}
		return m, nil
	}
	var raw any
	if err := configDecode(body, &raw); err != nil {
		return nil, fmt.Errorf("读 %s 失败: %w", r.Title, err)
	}
	if r.Wrap != "" {
		if m, ok := raw.(map[string]any); ok {
			raw = m[r.Wrap]
		}
	}
	arr, _ := raw.([]any)
	items := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			items = append(items, m)
		}
	}
	return items, nil
}

// configSave 按默认的接口保存。
func (s *Server) configSave(ctx context.Context, r *configResource, op, id string, cur, next, changed map[string]any) (string, error) {
	if r.save != nil {
		return r.save(ctx, op, id, cur, next, changed)
	}
	itemPath := strings.ReplaceAll(r.ItemPath, "{id}", url.PathEscape(id))
	switch op {
	case configOpSet:
		_, err := s.configDo(ctx, r.SetMethod, r.Path, r.body(next, changed))
		return "", err
	case configOpUpdate:
		method := r.UpdateMethod
		if method == "" {
			method = http.MethodPut
		}
		_, err := s.configDo(ctx, method, itemPath, r.body(next, changed))
		return "", err
	case configOpAdd:
		path := r.AddPath
		if path == "" {
			path = r.Path
		}
		out, err := s.configDo(ctx, http.MethodPost, path, r.body(next, changed))
		if err != nil {
			return "", err
		}
		var m map[string]any
		if configDecode(out, &m) == nil {
			if v, ok := m[r.idField()]; ok {
				return configIDText(v), nil
			}
		}
		return "", nil
	case configOpDelete:
		_, err := s.configDo(ctx, http.MethodDelete, itemPath, nil)
		return "", err
	}
	return "", configBad("不认识的操作 %s", op)
}

// body 是交给接口的内容：能改的字段取并进改动以后的值，密钥只在改了时带上；Partial 时只交改了的几项。
func (r *configResource) body(next, changed map[string]any) map[string]any {
	out := map[string]any{}
	if r.Partial {
		for k, v := range changed {
			out[k] = v
		}
		return out
	}
	for _, k := range r.Fields {
		if r.secret(k) {
			continue
		}
		if v, ok := next[k]; ok {
			out[k] = v
		}
	}
	for _, k := range r.AddOnly {
		if v, ok := changed[k]; ok {
			out[k] = v
		}
	}
	for k := range r.Secrets {
		if v, ok := changed[k]; ok {
			out[k] = v
			continue
		}
		if slices.Contains(r.Resend, k) {
			if v, ok := next[k]; ok {
				out[k] = v
			}
		}
	}
	return out
}

// show 是给看的样子：去掉不给看的字段，密钥换成占位，别的字符串遮掉地址里的凭证（见 redact.Credentials）。
func (r *configResource) show(raw map[string]any) map[string]any {
	out := map[string]any{}
	id := r.idField()
	for k, v := range raw {
		if slices.Contains(r.Hidden, k) {
			continue
		}
		if len(r.Show) > 0 && !slices.Contains(r.Show, k) && !slices.Contains(r.Fields, k) && k != id {
			continue
		}
		if r.staticSecret(k) {
			continue
		}
		if r.secret(k) {
			out[k] = configSecretMark(!configEmpty(v))
			continue
		}
		out[k] = configRedact(v)
	}
	for k, flag := range r.Secrets {
		switch {
		case flag != "":
			if b, ok := raw[flag].(bool); ok {
				out[k] = configSecretMark(b)
			}
		default:
			if v, ok := raw[k]; ok {
				out[k] = configSecretMark(!configEmpty(v))
			}
		}
	}
	if r.view != nil {
		r.view(raw, out)
	}
	return out
}

func configSecretMark(set bool) string {
	if set {
		return ConfigSecretSet
	}
	return ""
}

// configRedact 遮掉字符串里地址的用户信息、查询串与 JSON、key=value 形式的凭证（嵌套的也遮）。
func configRedact(v any) any {
	switch t := v.(type) {
	case string:
		return redact.Credentials(t)
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = configRedact(x)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			out[k] = configRedact(x)
		}
		return out
	}
	return v
}

func configEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	case bool:
		return !t
	}
	return false
}

// configIDText 把编号写成字符串（数字编号与字符串编号都行）。
func configIDText(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case nil:
		return ""
	}
	return fmt.Sprint(v)
}

// configCanon 把值换成可以比较的样子（数字都是 float64）。
func configCanon(v any) any {
	buf, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if json.Unmarshal(buf, &out) != nil {
		return v
	}
	return out
}

func configEqual(a, b any) bool { return reflect.DeepEqual(configCanon(a), configCanon(b)) }

// configCoerce 按现在的值的类型规整 AI 给的值：现在是数字、给的是数字字符串时换成数字，现在是开关、给的是 "true"/"false" 时换成布尔，
// 现在是字符串、给的是数组或对象时写成 JSON（RSS 的 notify_conf_ids 这类字段存的是 JSON 字符串）。
func configCoerce(v, like any) any {
	switch like.(type) {
	case json.Number, float64:
		if s, ok := v.(string); ok {
			if _, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
				return json.Number(strings.TrimSpace(s))
			}
		}
	case bool:
		if s, ok := v.(string); ok {
			switch strings.ToLower(strings.TrimSpace(s)) {
			case "true", "是", "开":
				return true
			case "false", "否", "关":
				return false
			}
		}
	case string:
		switch v.(type) {
		case []any, map[string]any:
			if buf, err := json.Marshal(v); err == nil {
				return string(buf)
			}
		}
	}
	return v
}

// configText 是差异里显示的值。
func configText(v any) string {
	var s string
	switch t := v.(type) {
	case nil:
		return "（空）"
	case string:
		if strings.TrimSpace(t) == "" {
			return "（空）"
		}
		s = t
	case bool:
		if t {
			return "是"
		}
		return "否"
	case json.Number:
		return t.String()
	default:
		buf, err := json.Marshal(t)
		if err != nil {
			s = fmt.Sprint(t)
		} else {
			s = string(buf)
		}
	}
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > configMaxValueText {
		s = string(r[:configMaxValueText]) + "…"
	}
	return s
}

// configLabels 是常见字段给人看的名字（差异与预览里用）；资源自己的 Labels 优先。
var configLabels = map[string]string{
	"enabled": "启用", "name": "名称", "url": "地址", "username": "用户名", "password": "密码", "type": "类型",
	"is_default": "默认", "auto_start": "推送后自动开始", "cookie": "Cookie", "api_key": "API Key", "passkey": "Passkey",
	"token": "令牌", "category": "分类", "tag": "标签", "tags": "标签", "interval_minutes": "间隔（分钟）",
	"downloader_id": "下载器", "download_path": "下载路径", "filter_rule_ids": "过滤规则", "notify_mode": "通知方式",
	"notify_conf_ids": "通知通道", "pattern": "匹配内容", "pattern_type": "匹配方式", "priority": "优先级",
	"title": "标题", "text": "正文", "path": "路径", "kind": "种类", "site_name": "站点", "site": "站点",
}

func (r *configResource) label(field string) string {
	if l, ok := r.Labels[field]; ok {
		return l + "（" + field + "）"
	}
	if l, ok := configLabels[field]; ok {
		return l + "（" + field + "）"
	}
	return field
}

// itemName 是列表项给人看的名字：名字字段，没有时是编号。
func (r *configResource) itemName(item map[string]any) string {
	if item == nil {
		return ""
	}
	if v, ok := item[r.nameField()].(string); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return configIDText(item[r.idField()])
}

// configResource 按名字找资源。
func (s *Server) configResource(name string) (*configResource, error) {
	for _, r := range s.configResources() {
		if r.Name == name {
			return r, nil
		}
	}
	return nil, configNotFound("没有这类配置：%s（GET /config 列出全部）", name)
}

// ConfigCatalog 是配置目录。
func (s *Server) ConfigCatalog() ConfigCatalog {
	out := ConfigCatalog{Excluded: slices.Clone(configExcluded), SecretPlaceholder: ConfigSecretSet}
	for _, r := range s.configResources() {
		out.Resources = append(out.Resources, r.catalogView())
	}
	return out
}

// ConfigRead 读一类配置的现状（密钥遮住）；id 不为空时只读列表里的那一项。
func (s *Server) ConfigRead(ctx context.Context, name, id string) (ConfigState, error) {
	r, err := s.configResource(name)
	if err != nil {
		return ConfigState{}, err
	}
	raw, err := s.configLoad(ctx, r)
	if err != nil {
		return ConfigState{}, err
	}
	out := ConfigState{Resource: r.Name, Kind: r.Kind}
	if r.Kind == configObject {
		m, _ := raw.(map[string]any)
		out.Item = r.show(m)
		return out, nil
	}
	out.IDField = r.idField()
	items, _ := raw.([]map[string]any)
	out.Items = []map[string]any{}
	for _, it := range items {
		if id != "" && configIDText(it[r.idField()]) != id {
			continue
		}
		out.Items = append(out.Items, r.show(it))
	}
	if id != "" && len(out.Items) == 0 {
		return ConfigState{}, configNotFound("%s里没有 %s=%s", r.Title, r.idField(), id)
	}
	return out, nil
}

// configPlan 是算好的一次改动：改以前、改以后、改了的几项与给人看的结果。
type configPlan struct {
	r       *configResource
	op, id  string
	cur     map[string]any
	next    map[string]any
	changed map[string]any
	result  ConfigChangeResult
}

// ConfigPreview 算出一次改动会改什么（不保存）。
func (s *Server) ConfigPreview(ctx context.Context, name string, in ConfigChange) (ConfigChangeResult, error) {
	p, err := s.configPlanFor(ctx, name, in)
	if err != nil {
		return ConfigChangeResult{}, err
	}
	return p.result, nil
}

// ConfigApply 按现状重新算一遍改动并保存，回改完的样子。
func (s *Server) ConfigApply(ctx context.Context, name string, in ConfigChange) (ConfigChangeResult, error) {
	p, err := s.configPlanFor(ctx, name, in)
	if err != nil {
		return ConfigChangeResult{}, err
	}
	newID, err := s.configSave(ctx, p.r, p.op, p.id, p.cur, p.next, p.changed)
	if err != nil {
		return ConfigChangeResult{}, err
	}
	res := p.result
	res.Applied = true
	if p.op == configOpAdd && newID != "" {
		res.ID = newID
	}
	if p.op != configOpDelete {
		res.Item = s.configReadBack(ctx, p, res.ID)
	}
	return res, nil
}

// configReadBack 读回改完的那一项（读不到时为空：已经保存了，不算失败）。
func (s *Server) configReadBack(ctx context.Context, p *configPlan, id string) map[string]any {
	raw, err := s.configLoad(ctx, p.r)
	if err != nil {
		return nil
	}
	if p.r.Kind == configObject {
		m, _ := raw.(map[string]any)
		return p.r.show(m)
	}
	items, _ := raw.([]map[string]any)
	if id != "" {
		for _, it := range items {
			if configIDText(it[p.r.idField()]) == id {
				return p.r.show(it)
			}
		}
	}
	if p.op == configOpAdd {
		// 接口没回编号：按名字找新加的那一项（同名的取编号最大的）
		name := p.r.itemName(p.changed)
		var found map[string]any
		for _, it := range items {
			if p.r.itemName(it) == name {
				found = it
			}
		}
		if found != nil {
			return p.r.show(found)
		}
	}
	return nil
}

func (s *Server) configPlanFor(ctx context.Context, name string, in ConfigChange) (*configPlan, error) {
	r, err := s.configResource(name)
	if err != nil {
		return nil, err
	}
	op := strings.TrimSpace(in.Op)
	if !slices.Contains(r.Ops, op) {
		return nil, configBad("%s只能 %s（不能 %s）", r.Title, strings.Join(r.Ops, "、"), op)
	}
	p := &configPlan{r: r, op: op, id: strings.TrimSpace(in.ID), changed: map[string]any{}}
	raw, err := s.configLoad(ctx, r)
	if err != nil {
		return nil, err
	}
	values := in.Values
	if values == nil {
		values = map[string]any{}
	}
	switch op {
	case configOpSet:
		p.cur, _ = raw.(map[string]any)
		if p.cur == nil {
			p.cur = map[string]any{}
		}
	case configOpUpdate, configOpDelete:
		if p.id == "" {
			return nil, configBad("%s要给 id（%s 字段的值）", map[string]string{configOpUpdate: "修改", configOpDelete: "删除"}[op], r.idField())
		}
		items, _ := raw.([]map[string]any)
		for _, it := range items {
			if configIDText(it[r.idField()]) == p.id {
				p.cur = it
				break
			}
		}
		if p.cur == nil {
			return nil, configNotFound("%s里没有 %s=%s：先用 get_config 看现有的", r.Title, r.idField(), p.id)
		}
	case configOpAdd:
		if p.id != "" {
			return nil, configBad("新增不用给 id")
		}
	}
	if op == configOpDelete {
		if len(values) > 0 {
			return nil, configBad("删除不用给 values")
		}
		verb, impact := "删除", append(slices.Clone(r.Impact), "删掉以后不能恢复，要用得重新添加")
		if r.DeleteVerb != "" {
			verb, impact = r.DeleteVerb, slices.Clone(r.DeleteImpact)
		}
		p.result = ConfigChangeResult{
			Resource: r.Name, Op: op, ID: p.id, Summary: fmt.Sprintf("%s%s「%s」", verb, r.Title, r.itemName(p.cur)),
			Changes: []ConfigDiff{{Label: r.Title, Field: r.idField(), Before: r.itemName(p.cur), After: map[bool]string{true: verb}[r.DeleteVerb != ""]}},
			Impact:  impact,
		}
		if p.result.Impact == nil {
			p.result.Impact = []string{}
		}
		return p, nil
	}
	if r.before != nil {
		if err := r.before(op, p.cur, values); err != nil {
			return nil, err
		}
	}
	var unknown []string
	for k, v := range values {
		if !r.writable(k, op) {
			unknown = append(unknown, k)
			continue
		}
		if r.secret(k) {
			text, ok := v.(string)
			if !ok {
				return nil, configBad("%s 是密钥，要给字符串", k)
			}
			if text == ConfigSecretSet {
				continue
			}
			if op != configOpAdd || text != "" {
				p.changed[k] = text
			}
			continue
		}
		if op != configOpAdd {
			v = configCoerce(v, p.cur[k])
			if configEqual(v, p.cur[k]) {
				continue
			}
			// 交回的是给看的样子（原值里地址的凭证被遮掉了）：当作不改，免得把遮过的值存回去
			if red := configRedact(p.cur[k]); !configEqual(red, p.cur[k]) && configEqual(v, red) {
				continue
			}
		}
		p.changed[k] = v
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, configBad("%s不能改 %s；能改的有：%s", r.Title, strings.Join(unknown, "、"), strings.Join(r.fieldList(), "、"))
	}
	if op == configOpAdd {
		var missing []string
		for _, k := range r.Required {
			if configEmpty(p.changed[k]) {
				missing = append(missing, k)
			}
		}
		if len(missing) > 0 {
			return nil, configBad("新增%s要给 %s", r.Title, strings.Join(missing, "、"))
		}
	}
	if len(p.changed) == 0 {
		return nil, configBad("没有要改的：给的值和现在一样（密钥给占位 %s 表示不改）", ConfigSecretSet)
	}
	p.next = map[string]any{}
	for k, v := range p.cur {
		p.next[k] = v
	}
	for k, v := range p.changed {
		p.next[k] = v
	}
	p.result = ConfigChangeResult{Resource: r.Name, Op: op, ID: p.id, Changes: r.diffs(op, p.cur, p.changed), Impact: slices.Clone(r.Impact)}
	switch op {
	case configOpSet:
		p.result.Summary = fmt.Sprintf("改%s的 %d 项", r.Title, len(p.changed))
	case configOpUpdate:
		p.result.Summary = fmt.Sprintf("改%s「%s」的 %d 项", r.Title, r.itemName(p.cur), len(p.changed))
	case configOpAdd:
		p.result.Summary = fmt.Sprintf("新增%s「%s」", r.Title, r.itemName(p.changed))
	}
	if p.result.Impact == nil {
		p.result.Impact = []string{}
	}
	return p, nil
}

// diffs 是改了的几项（按字段排好）。密钥两边都不写原文。
func (r *configResource) diffs(op string, cur, changed map[string]any) []ConfigDiff {
	keys := make([]string, 0, len(changed))
	for k := range changed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]ConfigDiff, 0, len(keys))
	for _, k := range keys {
		d := ConfigDiff{Label: r.label(k), Field: k}
		if r.secret(k) {
			d.Secret = true
			if op != configOpAdd {
				d.Before = configSecretBefore(r, cur, k)
			}
			if s, _ := changed[k].(string); s == "" {
				d.After = "清除"
			} else {
				d.After = "已更新（不显示）"
			}
			out = append(out, d)
			continue
		}
		if op != configOpAdd {
			d.Before = configText(configRedact(cur[k]))
		}
		d.After = configText(configRedact(changed[k]))
		out = append(out, d)
	}
	return out
}

func configSecretBefore(r *configResource, cur map[string]any, k string) string {
	set := false
	if flag := r.Secrets[k]; r.staticSecret(k) && flag != "" {
		set, _ = cur[flag].(bool)
	} else if v, ok := cur[k]; ok {
		set = !configEmpty(v)
	} else {
		return "（不显示）"
	}
	if set {
		return "已设置"
	}
	return "未设置"
}

// configWriteError 把网关的错误写成 App API 的错误回应。
func configWriteError(w http.ResponseWriter, err error) {
	var ce *configError
	if errors.As(err, &ce) {
		code := map[int]string{
			http.StatusBadRequest: "invalid_argument", http.StatusNotFound: "not_found", http.StatusConflict: "conflict",
			http.StatusServiceUnavailable: "unavailable", http.StatusBadGateway: "upstream",
		}[ce.Status]
		if code == "" {
			code = "error"
		}
		appError(w, ce.Status, code, ce.Message)
		return
	}
	appError(w, http.StatusInternalServerError, "internal", redact.Secrets(err.Error()))
}
