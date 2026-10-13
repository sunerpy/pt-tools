package webhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/notify/tmpl"
	"github.com/sunerpy/pt-tools/internal/redact"
)

// 请求体的格式（Config.ContentType）
const (
	formatJSON   = "json"
	formatText   = "text"
	formatForm   = "form"
	formatCustom = "custom"
)

var formatHeader = map[string]string{
	formatJSON: "application/json",
	formatText: "text/plain; charset=utf-8",
	formatForm: "application/x-www-form-urlencoded",
}

// sampleChannel 是检查与预览时 channel 变量的值
const sampleChannel = "我的 Webhook"

// ErrInvalid 包着「配置写得不对」的错误（方法、格式、地址、请求头）；模板本身的错误包着 tmpl.ErrInvalid。
var ErrInvalid = errors.New("Webhook 配置不对")

type invalidError struct{ msg string }

func (e *invalidError) Error() string { return e.msg }
func (e *invalidError) Unwrap() error { return ErrInvalid }

func invalid(format string, args ...any) error {
	return &invalidError{msg: fmt.Sprintf(format, args...)}
}

// request 是编译好的请求：发送、保存前的检查与预览共用。
type request struct {
	method  string
	format  string
	headers Headers
	// url 是不带模板的地址；urlTmpl 不为空时地址按它渲染，origin 是模板前面那段固定文字里的协议与主机
	url     string
	urlTmpl *tmpl.Template
	origin  *url.URL
	// body 为空时发固定 JSON
	body *tmpl.Template
	// success 不为空时按响应内容判断成败
	success *tmpl.Template
}

// rendered 是渲染好的一次请求。ContentType 是要发的 Content-Type 头（GET 且请求头里没写时为空）。
type rendered struct {
	Method      string
	URL         string
	ContentType string
	Body        []byte
}

// compile 检查配置并编译地址与请求体模板。
func compile(cfg *Config) (*request, error) {
	r := &request{headers: cfg.Headers}

	r.method = strings.ToUpper(strings.TrimSpace(cfg.Method))
	if r.method == "" {
		r.method = http.MethodPost
	}
	switch r.method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodGet:
	default:
		return nil, invalid("请求方法只能是 POST、PUT、PATCH、GET")
	}

	r.format = strings.ToLower(strings.TrimSpace(cfg.ContentType))
	if r.format == "" {
		r.format = formatJSON
	}
	if _, ok := formatHeader[r.format]; !ok && r.format != formatCustom {
		return nil, invalid("请求体格式只能是 json、text、form、custom")
	}

	if src := cfg.BodyTemplate; strings.TrimSpace(src) != "" {
		if r.method == http.MethodGet {
			return nil, invalid("GET 请求不带请求体：清空请求体模板，或者换成 POST")
		}
		p, err := tmpl.Parse(src, tmpl.Body, notify.ListVars())
		if err != nil {
			return nil, fmt.Errorf("请求体模板：%w", err)
		}
		r.body = p
	} else if r.method != http.MethodGet && r.format != formatJSON {
		return nil, invalid("请求体格式选了 %s 就要写请求体模板（留空时发的是固定 JSON）", r.format)
	}
	if r.method != http.MethodGet && r.format == formatCustom && strings.TrimSpace(r.headers.get("Content-Type")) == "" {
		return nil, invalid("请求体格式选了 custom 就要在请求头里写 Content-Type")
	}

	if err := r.compileURL(strings.TrimSpace(cfg.EndpointURL)); err != nil {
		return nil, err
	}
	if src := cfg.SuccessTemplate; strings.TrimSpace(src) != "" {
		p, err := tmpl.Parse(src, tmpl.Title, nil)
		if err != nil {
			return nil, fmt.Errorf("成功条件：%w", err)
		}
		r.success = p
	}
	return r, nil
}

// maxResponseBody 是判断成败时看的响应长度上限：再长的只看前面这些
const maxResponseBody = 64 << 10

// errResponseRejected 包着「响应不满足成功条件」：和 HTTP 状态码不对一样算发送失败、按通知的重试规则再试
var errResponseRejected = errors.New("webhook 响应不满足成功条件")

// checkResponse 按成功条件判断一次响应：没写成功条件时只看状态码（调用方已经判过 2xx）。
// 模板的结果去掉首尾空白以后是 true（不分大小写）才算成功；false、空串算失败，别的内容当作失败的原因。
func (r *request) checkResponse(status int, body []byte) error {
	if r.success == nil {
		return nil
	}
	out, err := r.success.Execute(responseData(status, body))
	if err != nil {
		return fmt.Errorf("%w：成功条件算不出来（%v）", errResponseRejected, err)
	}
	out = strings.TrimSpace(out)
	switch {
	case strings.EqualFold(out, "true"):
		return nil
	case out == "" || strings.EqualFold(out, "false"):
		return errResponseRejected
	}
	if runes := []rune(out); len(runes) > 200 {
		out = string(runes[:200]) + "…"
	}
	return fmt.Errorf("%w：%s", errResponseRejected, out)
}

// responseData 是成功条件的数据：status、body（最多 maxResponseBody 字节），以及响应是 JSON 时解出来的 json
// （数字是整数时为 int64，否则 float64，和事件变量的写法一样；不是 JSON 时是空对象，取字段得零值）。
func responseData(status int, body []byte) map[string]any {
	if len(body) > maxResponseBody {
		body = body[:maxResponseBody]
	}
	var parsed any = map[string]any{}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err == nil && !dec.More() {
		parsed = normalizeJSON(v)
	}
	return map[string]any{"status": int64(status), "body": string(body), "json": parsed}
}

// credentialKey 认出请求体里放凭证的字段名（token、key、secret、password、sign 一类）
var credentialKey = regexp.MustCompile(`(?i)(token|key|secret|passw|pwd|auth|sign|credential|cookie|session)`)

// secretValue 是这次请求里可能是凭证的一个值。credential 表示它来自凭证的位置（请求头、HMAC Secret、
// 字段名像凭证的查询参数与请求体字段、较长的路径段），不是普通的查询参数。
type secretValue struct {
	value      string
	credential bool
}

// sensitiveValues 是这次请求里可能是凭证的值：地址查询的每个值与较长的路径段（Discord 这类把 token 放在路径里）、
// 自定义请求头的值、HMAC Secret，以及请求体里字段名像凭证的值（JSON 的任意一层与表单）。对方的响应把它们回显时要去掉。
func sensitiveValues(r *rendered, cfg *Config) []secretValue {
	var out []secretValue
	add := func(v string, credential bool) {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, secretValue{value: v, credential: credential})
		}
	}
	if u, err := url.Parse(r.URL); err == nil {
		for k, vs := range u.Query() {
			for _, v := range vs {
				add(v, credentialKey.MatchString(k))
			}
		}
		for _, seg := range strings.Split(u.Path, "/") {
			if len(seg) >= 16 {
				add(seg, true)
			}
		}
	}
	for _, v := range cfg.Headers {
		add(v, true)
		// 「Bearer xxx」「Basic xxx」：对方多半只回显后面那一段
		for _, f := range strings.Fields(v) {
			switch strings.ToLower(f) {
			case "bearer", "basic", "token", "digest":
				continue
			}
			add(f, true)
		}
	}
	add(cfg.HMACSecret, true)
	var v any
	if json.Unmarshal(r.Body, &v) == nil {
		collectCredentials(v, "", func(s string) { add(s, true) })
	} else if form, err := url.ParseQuery(string(r.Body)); err == nil {
		for k, vs := range form {
			if credentialKey.MatchString(k) {
				for _, x := range vs {
					add(x, true)
				}
			}
		}
	}
	return out
}

func collectCredentials(v any, key string, add func(string)) {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			collectCredentials(e, k, add)
		}
	case []any:
		for _, e := range x {
			collectCredentials(e, key, add)
		}
	case string:
		if credentialKey.MatchString(key) {
			add(x)
		}
	}
}

// errReasonHidden 是失败原因里认不准有没有凭证时给的固定说法
var errReasonHidden = fmt.Errorf("%w（对方返回的原因里可能有凭证，不显示）", errResponseRejected)

// scrubError 把错误里出现的凭证值换成 ***，再按通用规则（redact.Secrets）去掉看起来像凭证的内容。
// 太短的凭证（不到 4 个字符）没法放心地替换（会误伤原因里的普通文字），原因里出现它时整段不显示。
func scrubError(err error, secrets []secretValue) error {
	msg := err.Error()
	for _, s := range secrets {
		switch {
		case len(s.value) >= 4:
			msg = strings.ReplaceAll(msg, s.value, "***")
		case s.credential && strings.Contains(msg, s.value):
			return errReasonHidden
		}
	}
	msg = redact.Secrets(msg)
	if errors.Is(err, errResponseRejected) {
		return &scrubbedError{msg: msg, err: errResponseRejected}
	}
	return errors.New(msg)
}

type scrubbedError struct {
	msg string
	err error
}

func (e *scrubbedError) Error() string { return e.msg }
func (e *scrubbedError) Unwrap() error { return e.err }

// normalizeJSON 把 json.Number 换成 int64（整数）或 float64，其余原样（逐层处理对象与数组）。
func normalizeJSON(v any) any {
	switch x := v.(type) {
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		f, _ := x.Float64()
		return f
	case map[string]any:
		for k, e := range x {
			x[k] = normalizeJSON(e)
		}
		return x
	case []any:
		for i, e := range x {
			x[i] = normalizeJSON(e)
		}
		return x
	}
	return v
}

// compileURL 检查地址。地址里可以写模板，但只能写在路径与查询里：第一个 {{ 要在主机名后面的 / 之后，
// 渲染出来的地址协议与主机名要和模板前面那段固定文字里的一样。
func (r *request) compileURL(raw string) error {
	i := strings.Index(raw, "{{")
	if i < 0 {
		if _, err := parseHTTPURL(raw); err != nil {
			return err
		}
		r.url = raw
		return nil
	}
	head := raw[:i]
	scheme := strings.Index(head, "://")
	slash := -1
	if scheme >= 0 {
		slash = strings.Index(head[scheme+3:], "/")
	}
	if slash < 0 {
		return invalid("地址里的模板只能写在路径和查询里（主机名后面的 / 之后）")
	}
	origin, err := parseHTTPURL(head[:scheme+3+slash+1])
	if err != nil {
		return err
	}
	p, err := tmpl.Parse(raw, tmpl.URL, notify.ListVars())
	if err != nil {
		return fmt.Errorf("地址模板：%w", err)
	}
	r.origin, r.urlTmpl = origin, p
	return nil
}

// parseHTTPURL 解析 http(s) 地址。出错时不带地址原文：查询部分可能有 token。
func parseHTTPURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, invalid("地址要是 http:// 或 https:// 开头的完整地址")
	}
	return u, nil
}

// data 是渲染地址与请求体用的数据：事件的变量（没有事件时只有零值）、套过消息模板的标题与正文、通道名称。
func data(n notify.Notification, channel string) map[string]any {
	ev := n.Event
	if ev == nil {
		ev = notify.NewEvent("", time.Now(), nil)
	}
	d := ev.Data(n.Title, n.Text)
	d["channel"] = channel
	return d
}

// render 生成一次请求。strict 时变量名写错要报错（保存前的检查与预览）。
func (r *request) render(n notify.Notification, channel string, strict bool) (*rendered, error) {
	exec := (*tmpl.Template).Execute
	if strict {
		exec = (*tmpl.Template).Check
	}
	d := data(n, channel)
	out := &rendered{Method: r.method, URL: r.url}

	if r.urlTmpl != nil {
		s, err := exec(r.urlTmpl, d)
		if err != nil {
			return nil, fmt.Errorf("地址模板：%w", err)
		}
		s = strings.TrimSpace(s)
		u, err := parseHTTPURL(s)
		if err != nil {
			return nil, invalid("地址模板渲染出来的不是 http:// 或 https:// 开头的完整地址")
		}
		if u.Scheme != r.origin.Scheme || u.Host != r.origin.Host {
			return nil, invalid("地址模板不能改协议和主机名")
		}
		out.URL = s
	}

	switch {
	case r.method == http.MethodGet:
	case r.body == nil:
		body, err := json.Marshal(WebhookPayload{
			EventType: "notification",
			Title:     n.Title,
			Text:      n.Text,
			Link:      n.Link,
			Payload: map[string]interface{}{
				"channel_type":   n.ChannelType,
				"source_conf_id": n.SourceConfID,
			},
			Event: n.Event,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal payload failed: %w", err)
		}
		out.Body = body
	default:
		s, err := exec(r.body, d)
		if err != nil {
			return nil, fmt.Errorf("请求体模板：%w", err)
		}
		if r.format == formatJSON && !json.Valid([]byte(s)) {
			return nil, invalid("请求体不是合法的 JSON（文字要用 json 函数写，例如 {{json .title}}）")
		}
		out.Body = []byte(s)
	}

	// 请求头里写了 Content-Type 的以它为准；GET 没有请求体，不按格式加
	out.ContentType = strings.TrimSpace(r.headers.get("Content-Type"))
	if out.ContentType == "" && r.method != http.MethodGet {
		out.ContentType = formatHeader[r.format]
	}
	return out, nil
}

// checkSamples 用每种事件的示例数据渲染一遍：Webhook 会收到所有事件，每一种都要渲染得出来。
func (r *request) checkSamples() error {
	if r.urlTmpl == nil && r.body == nil {
		return nil
	}
	for _, spec := range notify.Events {
		if _, err := r.render(sampleNotification(spec.Type), sampleChannel, true); err != nil {
			return fmt.Errorf("%s（%s）：%w", spec.Name, spec.Type, err)
		}
	}
	return nil
}

func sampleNotification(eventType string) notify.Notification {
	ev, title, text := notify.SampleEvent(eventType)
	return notify.Notification{Event: ev, Title: title, Text: text, ChannelType: "webhook"}
}

// PreviewInput 是预览的输入，字段和配置里的同名；Type 是用哪种事件的示例数据。
type PreviewInput struct {
	Type         string  `json:"type"`
	Method       string  `json:"method"`
	EndpointURL  string  `json:"endpoint_url"`
	ContentType  string  `json:"content_type"`
	Headers      Headers `json:"headers"`
	BodyTemplate string  `json:"body_template"`
	// SuccessTemplate 是成功条件；SampleResponse、SampleStatus（默认 200）是拿来试它的一份响应
	SuccessTemplate string `json:"success_template"`
	SampleResponse  string `json:"sample_response"`
	SampleStatus    int    `json:"sample_status"`
}

// PreviewResult 是按示例数据渲染出来的请求（只渲染、不发送，不含签名）。ContentType 按发送时的规则算。
type PreviewResult struct {
	Method      string `json:"method"`
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	Body        string `json:"body"`
	// Success 是按示例响应算出的成败（写了成功条件时才有），SuccessReason 是失败的原因
	Success       *bool  `json:"success,omitempty"`
	SuccessReason string `json:"success_reason,omitempty"`
}

// ErrUnknownEvent 是预览时给了不认识的事件类型
var ErrUnknownEvent = errors.New("没有这种事件")

// Preview 按一种事件的示例数据渲染请求。配置不对时返回的错误包着 ErrInvalid 或 tmpl.ErrInvalid。
func Preview(in PreviewInput) (*PreviewResult, error) {
	if notify.Spec(in.Type) == nil {
		return nil, fmt.Errorf("%w：%s", ErrUnknownEvent, in.Type)
	}
	if strings.TrimSpace(in.EndpointURL) == "" {
		return nil, invalid("请先填地址")
	}
	r, err := compile(&Config{
		EndpointURL:     in.EndpointURL,
		Method:          in.Method,
		ContentType:     in.ContentType,
		Headers:         in.Headers,
		BodyTemplate:    in.BodyTemplate,
		SuccessTemplate: in.SuccessTemplate,
	})
	if err != nil {
		return nil, err
	}
	out, err := r.render(sampleNotification(in.Type), sampleChannel, true)
	if err != nil {
		return nil, err
	}
	res := &PreviewResult{Method: out.Method, URL: out.URL, ContentType: out.ContentType, Body: string(out.Body)}
	if r.success != nil {
		status := in.SampleStatus
		if status == 0 {
			status = 200
		}
		// 和发送时一样：先看状态码，2xx 才用成功条件
		var err error
		if status < 200 || status >= 300 {
			err = fmt.Errorf("webhook returned status %d", status)
		} else {
			err = r.checkResponse(status, []byte(in.SampleResponse))
		}
		ok := err == nil
		res.Success = &ok
		if !ok {
			res.SuccessReason = err.Error()
		}
	}
	return res, nil
}
