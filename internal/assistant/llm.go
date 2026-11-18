package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/internal/redact"
	"github.com/sunerpy/pt-tools/utils/httpclient"
)

// 调模型的时限。
const (
	// stepTimeout 是一次模型调用（含流式读完）的上限
	stepTimeout = 5 * time.Minute
	// headerTimeout 是等模型服务回应头的上限（思考模式开头要想一会儿）
	headerTimeout = 2 * time.Minute
	pingTimeout   = time.Minute
	// maxRetryWait 是 429 时最多等多久再试（测试连接与获取模型列表总共只有 pingTimeout）
	maxRetryWait = 20 * time.Second
)

// newHTTPClient 建调模型用的 http.Client：走 proxy，为空时用环境变量里的代理。不设整体超时（流式回答可能很久），
// 连接、TLS 握手与等回应头分别限时。
func newHTTPClient(proxy string) (*http.Client, error) {
	var pf func(*http.Request) (*url.URL, error)
	if p := strings.TrimSpace(proxy); p != "" {
		u, err := tmdb.ParseProxyURL(p)
		if err != nil {
			return nil, err
		}
		pf = http.ProxyURL(u)
	} else {
		pf = func(r *http.Request) (*url.URL, error) {
			if s := httpclient.ResolveProxyFromEnvironment(r.URL.String()); s != "" {
				return url.Parse(s)
			}
			return nil, nil
		}
	}
	return &http.Client{Transport: &http.Transport{
		Proxy:                 pf,
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: headerTimeout,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	}}, nil
}

// chatService 建 Chat Completions 客户端。不经 openai.NewClient：它会读 OPENAI_API_KEY、OPENAI_BASE_URL 等环境变量，
// 这里只用设置里的地址与 Key。
func (s *Service) chatService(c llmConfig) (openai.ChatCompletionService, error) {
	opts, err := s.clientOptions(c)
	if err != nil {
		return openai.ChatCompletionService{}, err
	}
	return openai.NewChatCompletionService(opts...), nil
}

// modelService 建读模型列表（GET /models）的客户端，地址、代理与 Key 同 chatService。
func (s *Service) modelService(c llmConfig) (openai.ModelService, error) {
	opts, err := s.clientOptions(c)
	if err != nil {
		return openai.ModelService{}, err
	}
	return openai.NewModelService(opts...), nil
}

// clientOptions 是调模型服务的公共选项：设置里的地址、代理（没填用环境变量）与 Key（没有就不带）。
func (s *Service) clientOptions(c llmConfig) ([]option.RequestOption, error) {
	hc := s.cfg.HTTPClient
	if hc == nil {
		var err error
		if hc, err = newHTTPClient(c.Proxy); err != nil {
			return nil, err
		}
	}
	// 重试两次：免费层（例如 Groq 每分钟 8K token）调工具多时容易 429，SDK 按服务商给的 Retry-After 等；
	// 服务商要等的超过 maxRetryWait 时不在这里干等，直接回 429（describeError 说要等多久）
	opts := []option.RequestOption{
		option.WithBaseURL(c.BaseURL), option.WithHTTPClient(hc),
		option.WithMaxRetries(2), option.WithMaxRetryDelay(maxRetryWait),
		option.WithMiddleware(normalizeErrorResponse),
	}
	if c.APIKey != "" {
		opts = append(opts, option.WithAPIKey(c.APIKey))
	}
	return opts, nil
}

// describeError 把调模型的错误换成给人看的一句话：不带 Key、请求头与完整地址。
func describeError(err error, key string) string {
	var msg string
	var apiErr *openai.Error
	var urlErr *url.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		msg = "模型服务响应超时"
	case errors.As(err, &apiErr):
		detail := strings.TrimSpace(apiErr.Message)
		if detail == "" {
			var body struct {
				Error   any    `json:"error"`
				Message string `json:"message"`
			}
			if raw := apiErr.RawJSON(); raw != "" && json.Unmarshal([]byte(raw), &body) == nil {
				detail = body.Message
			}
		}
		if detail == "" {
			detail = bodyMessage(apiErr.Response)
		}
		if detail == "" {
			detail = http.StatusText(apiErr.StatusCode)
		}
		msg = fmt.Sprintf("模型服务回 %d：%s", apiErr.StatusCode, detail)
		switch {
		case locationBlocked(detail):
			// Google AI Studio 对中国大陆、香港的 IP 回 400 User location is not supported
			msg += "（这个服务商不对你所在的地区开放：在「代理地址」里填一个能访问它的代理）"
		case apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden || keyRejected(detail):
			// Google AI Studio 对不对的 Key 回 400 Please pass a valid API key
			msg += "（检查 API Key）"
		case apiErr.StatusCode == http.StatusNotFound:
			msg += "（检查接口地址与模型名）"
		case apiErr.StatusCode == http.StatusServiceUnavailable:
			// Google AI Studio：This model is currently experiencing high demand
			msg += "（服务商这会儿太忙，过一会儿再试，或者换个模型）"
		case apiErr.StatusCode == http.StatusTooManyRequests:
			msg += "（请求太快或者免费额度用完了" + retryAfter(apiErr.Response) + "；也可以换个模型或服务商）"
		}
	case errors.As(err, &urlErr):
		msg = "连不上模型服务：" + urlErr.Err.Error()
	default:
		msg = "调用模型失败：" + err.Error()
	}
	if key != "" {
		msg = strings.ReplaceAll(msg, key, "***")
	}
	return redact.Secrets(msg)
}

// normalizeErrorResponse 让 SDK 认得出 Google AI Studio 的错误回应：错误体包在数组里（[{"error":{…}}]）时取第一个；
// 429 的头里没有 SDK 认得的等待时间、但正文写了要等多久（RetryInfo 的 retryDelay，或者「retry in 48.4s」）时补上 Retry-After，
// SDK 按它决定重不重试（超过 maxRetryWait 就不重试），describeError 按它说要等多久。成功的回应原样放过（不读流式的正文）。
func normalizeErrorResponse(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
	res, err := next(req)
	if err != nil || res == nil || res.StatusCode < 400 || res.Body == nil {
		return res, err
	}
	raw, rerr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	_ = res.Body.Close()
	if rerr == nil {
		if t := bytes.TrimSpace(raw); len(t) > 0 && t[0] == '[' {
			var items []json.RawMessage
			if json.Unmarshal(t, &items) == nil && len(items) > 0 {
				raw = items[0]
			}
		}
		if _, said := retryWait(res.Header); res.StatusCode == http.StatusTooManyRequests && !said {
			if secs := retryDelayIn(raw); secs > 0 {
				res.Header.Set("Retry-After", strconv.Itoa(secs))
			}
		}
	}
	res.Body = io.NopCloser(bytes.NewReader(raw))
	res.ContentLength = int64(len(raw))
	return res, nil
}

// retryInText 是错误正文里写的要等多久（Gemini：Please retry in 48.434155456s.）
var retryInText = regexp.MustCompile(`(?i)retry in ([0-9]+(?:\.[0-9]+)?)\s*s`)

// retryDelayIn 是错误体里写的要等多久（秒，向上取整）：Google 的 RetryInfo（"retryDelay":"48s"），或者正文里的 retry in。
func retryDelayIn(raw []byte) int {
	var body struct {
		Error struct {
			Message string `json:"message"`
			Details []struct {
				RetryDelay string `json:"retryDelay"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return 0
	}
	secs := func(v string) int {
		f, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(v), "s"), 64)
		if err != nil || f <= 0 {
			return 0
		}
		return int(math.Ceil(f))
	}
	for _, d := range body.Error.Details {
		if n := secs(d.RetryDelay); n > 0 {
			return n
		}
	}
	if m := retryInText.FindStringSubmatch(body.Error.Message); m != nil {
		return secs(m[1])
	}
	return 0
}

// bodyMessage 读错误体顶层的 message：SDK 只认 {"error":{"message"}}，硅基流动写成 {"code":30014,"message":"Token is invalid."}
// （2026-10-12 实测）。包在数组里的已经由 normalizeErrorResponse 拆开了。读不出来时为空。读完把正文放回去，可以再读。
func bodyMessage(res *http.Response) string {
	if res == nil || res.Body == nil {
		return ""
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	res.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	var body struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return ""
	}
	return strings.TrimSpace(body.Message)
}

// locationBlocked 是服务商说所在地区不能用（Gemini API：User location is not supported for the API use）。
func locationBlocked(detail string) bool {
	d := strings.ToLower(detail)
	return strings.Contains(d, "location is not supported") || strings.Contains(d, "unsupported_country") ||
		strings.Contains(d, "country, region, or territory not supported")
}

// keyRejected 是服务商说 Key 不对（有的不回 401，例如 Google AI Studio 回 400）。
func keyRejected(detail string) bool {
	d := strings.ToLower(detail)
	return strings.Contains(d, "api key") && (strings.Contains(d, "valid") || strings.Contains(d, "incorrect")) ||
		strings.Contains(d, "api_key_invalid")
}

// maxReportedWait 是提示里最多说要等多久：服务商给的数大得离谱时不溢出
const maxReportedWait = 24 * time.Hour

// retryWait 是回应头要求等多久，读法和 SDK 决定重不重试时一样（openai-go v3.76.0 的 parseRetryAfterHeader）：
// 先 Retry-After-Ms，再 Retry-After（秒数或 RFC 1123 日期）；空的、读不懂的、负数的跳过、看下一个；0 也算数（SDK 马上重试）。
// said 为 false 是没说。
func retryWait(h http.Header) (wait time.Duration, said bool) {
	for _, r := range [...]struct {
		name string
		unit time.Duration
	}{{"Retry-After-Ms", time.Millisecond}, {"Retry-After", time.Second}} {
		v := h.Get(r.name)
		if v == "" {
			continue
		}
		f, err := strconv.ParseFloat(v, 64)
		if errors.Is(err, strconv.ErrRange) && math.IsInf(f, 1) {
			return maxReportedWait, true
		}
		if err == nil {
			if math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
				continue
			}
			return time.Duration(math.Min(f*float64(r.unit), float64(maxReportedWait))), true
		}
		if r.unit == time.Second {
			if at, err := time.Parse(time.RFC1123, v); err == nil {
				return min(max(time.Until(at), 0), maxReportedWait), true
			}
		}
	}
	return 0, false
}

// retryAfter 是 429 回应要等多久，说成「，约 N 秒后再试」（retryWait 读的）；没说、或者说不用等时为空。
func retryAfter(res *http.Response) string {
	if res == nil {
		return ""
	}
	wait, said := retryWait(res.Header)
	n := int(math.Ceil(wait.Seconds()))
	if !said || n <= 0 {
		return ""
	}
	if n >= 120 {
		return fmt.Sprintf("，约 %d 分钟后再试", (n+59)/60)
	}
	return fmt.Sprintf("，约 %d 秒后再试", n)
}

// completion 是模型一次回答：文字、推理内容、工具调用与用掉的 token。
type completion struct {
	content   string
	reasoning string
	// reasoningField 是推理内容从哪儿来的（见 models.AssistantMessage.ReasoningField）
	reasoningField string
	calls          []rawCall
	usage          tokenUsage
}

// tokenUsage 是服务商报的一次调用用掉的 token；都是 0 时是没报。
type tokenUsage struct {
	prompt, completion int64
}

func (u tokenUsage) reported() bool { return u.prompt > 0 || u.completion > 0 }

// usageOf 取分片里的用量：stream_options.include_usage 时最后一片的 usage（choices 是空的）；
// Groq 早先只写在 x_groq.usage 里。没有时 ok 为假。
func usageOf(chunk openai.ChatCompletionChunk) (tokenUsage, bool) {
	if chunk.JSON.Usage.Valid() {
		u := tokenUsage{prompt: chunk.Usage.PromptTokens, completion: chunk.Usage.CompletionTokens}
		if u.reported() {
			return u, true
		}
	}
	if f, ok := chunk.JSON.ExtraFields["x_groq"]; ok {
		var x struct {
			Usage *struct {
				Prompt     int64 `json:"prompt_tokens"`
				Completion int64 `json:"completion_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(f.Raw()), &x) == nil && x.Usage != nil {
			u := tokenUsage{prompt: x.Usage.Prompt, completion: x.Usage.Completion}
			if u.reported() {
				return u, true
			}
		}
	}
	return tokenUsage{}, false
}

// 推理内容的来处（models.AssistantMessage.ReasoningField）。
const (
	fieldReasoningContent = ""
	fieldReasoning        = "reasoning"
	fieldThink            = "think"
)

type rawCall struct {
	id, name, args string
	// extra 是这个调用带的 extra_content（原样 JSON）：Gemini 3 的思考签名在这里，之后的请求要原样带回去
	extra string
}

// maxExtraBytes 是一个工具调用的 extra_content 最多存多大
const maxExtraBytes = 16 << 10

// accumulator 拼流式回答。不用 openai.ChatCompletionAccumulator：它碰到各片 id 不一致、工具调用没有 index 的
// OpenAI 兼容服务会丢内容。推理内容认 reasoning_content（DeepSeek、通义、Kimi 等）、reasoning（OpenRouter、Groq、Ollama 等），
// 以及写在正文开头的 <think>…</think>。
type accumulator struct {
	content, reasoning strings.Builder
	calls              []*rawCall
	byIndex            map[int64]int
	think              thinkSplitter
	// sawContent、sawReasoning 是有过 reasoning_content、reasoning 的推理内容
	sawContent, sawReasoning, sawThink bool
	// usage 是最后一次报的用量（有的服务每一片都报累计的）
	usage tokenUsage
}

// extraString 是分片里一个字符串类的扩展字段（null、不是字符串时为空）。
func extraString(d openai.ChatCompletionChunkChoiceDelta, key string) string {
	f, ok := d.JSON.ExtraFields[key]
	if !ok {
		return ""
	}
	var t string
	if json.Unmarshal([]byte(f.Raw()), &t) != nil {
		return ""
	}
	return t
}

// add 拼一个分片，返回这一片新增的回答文字与推理内容。同一片里 reasoning_content 与 reasoning 都有时只认前者
// （有的服务两个字段写一样的内容）。
func (a *accumulator) add(chunk openai.ChatCompletionChunk) (text, reasoning string) {
	if u, ok := usageOf(chunk); ok {
		a.usage = u
	}
	if len(chunk.Choices) == 0 {
		return "", ""
	}
	d := chunk.Choices[0].Delta
	if t := extraString(d, "reasoning_content"); t != "" {
		reasoning, a.sawContent = t, true
	} else if t := extraString(d, "reasoning"); t != "" {
		reasoning, a.sawReasoning = t, true
	}
	text, thought := a.think.add(d.Content)
	if thought != "" {
		reasoning += thought
		a.sawThink = true
	}
	a.content.WriteString(text)
	a.reasoning.WriteString(reasoning)
	if a.byIndex == nil {
		a.byIndex = map[int64]int{}
	}
	for _, tc := range d.ToolCalls {
		pos, ok := a.byIndex[tc.Index]
		if !ok || (tc.ID != "" && a.calls[pos].id != "" && a.calls[pos].id != tc.ID) {
			a.calls = append(a.calls, &rawCall{})
			pos = len(a.calls) - 1
			a.byIndex[tc.Index] = pos
		}
		c := a.calls[pos]
		if c.id == "" {
			c.id = tc.ID
		}
		if f, ok := tc.JSON.ExtraFields["extra_content"]; ok && c.extra == "" {
			if raw := strings.TrimSpace(f.Raw()); raw != "" && raw != "null" && len(raw) <= maxExtraBytes && json.Valid([]byte(raw)) {
				c.extra = raw
			}
		}
		if n := tc.Function.Name; n != "" && n != c.name {
			c.name += n
		}
		c.args += tc.Function.Arguments
	}
	return text, reasoning
}

// finish 是流结束时攒着没交出去的文字与推理内容（还没判断完是不是 <think> 的那一段）。
func (a *accumulator) finish() (text, reasoning string) {
	text, reasoning = a.think.flush()
	if reasoning != "" {
		a.sawThink = true
	}
	a.content.WriteString(text)
	a.reasoning.WriteString(reasoning)
	return text, reasoning
}

func (a *accumulator) result() completion {
	out := completion{content: a.content.String(), reasoning: a.reasoning.String(), usage: a.usage}
	switch {
	case a.sawContent:
		out.reasoningField = fieldReasoningContent
	case a.sawReasoning:
		out.reasoningField = fieldReasoning
	case a.sawThink:
		out.reasoningField = fieldThink
	}
	for i, c := range a.calls {
		if c.name == "" {
			continue
		}
		if c.id == "" {
			c.id = "call_" + strconv.Itoa(i+1)
		}
		out.calls = append(out.calls, *c)
	}
	return out
}

// deltaFlusher 把文字增量攒一攒再发事件（每 32 个字节或 80 毫秒一次），事件不至于一个字一个。
type deltaFlusher struct {
	buf  strings.Builder
	last time.Time
	emit func(string)
}

func (f *deltaFlusher) add(s string, now time.Time) {
	f.buf.WriteString(s)
	if f.buf.Len() >= 32 || now.Sub(f.last) >= 80*time.Millisecond {
		f.flush(now)
	}
}

func (f *deltaFlusher) flush(now time.Time) {
	if f.buf.Len() == 0 {
		return
	}
	f.emit(f.buf.String())
	f.buf.Reset()
	f.last = now
}

// step 是一次模型调用的参数：模型、推理等级（为空不发）、消息与工具。noUsage 为真时不要 stream_options
// （不认这个参数的服务会拒绝整个请求）。
type step struct {
	model   string
	effort  string
	msgs    []openai.ChatCompletionMessageParamUnion
	tools   []openai.ChatCompletionToolUnionParam
	noUsage bool
}

// complete 流式调一次模型，文字增量经 emit、推理内容的增量经 emitReasoning 发出去。
func (s *Service) complete(ctx context.Context, chat openai.ChatCompletionService, st step, emit, emitReasoning func(string)) (completion, error) {
	ctx, cancel := context.WithTimeout(ctx, stepTimeout)
	defer cancel()
	params := openai.ChatCompletionNewParams{Model: st.model, Messages: st.msgs}
	if len(st.tools) > 0 {
		params.Tools = st.tools
	}
	if st.effort != "" {
		params.ReasoningEffort = shared.ReasoningEffort(st.effort)
	}
	if !st.noUsage {
		// 最后一片报这次用掉的 token（上下文用量与自动压缩靠它）
		params.StreamOptions = openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)}
	}
	stream := chat.NewStreaming(ctx, params)
	defer stream.Close()
	var acc accumulator
	now := time.Now()
	fl := &deltaFlusher{emit: emit, last: now}
	rf := &deltaFlusher{emit: emitReasoning, last: now}
	for stream.Next() {
		text, reasoning := acc.add(stream.Current())
		at := time.Now()
		if reasoning != "" {
			rf.add(reasoning, at)
		}
		if text != "" {
			// 回答开始了：推理内容先发完
			rf.flush(at)
			fl.add(text, at)
		}
	}
	text, reasoning := acc.finish()
	now = time.Now()
	rf.add(reasoning, now)
	rf.flush(now)
	fl.add(text, now)
	fl.flush(now)
	if err := stream.Err(); err != nil {
		return completion{}, err
	}
	return acc.result(), nil
}

// thinkSplitter 把正文开头的 <think>…</think> 分出来当推理内容（有的模型与网关把思考写在正文里）。只认开头的（前面只有空白）；
// 流式的分片可能把标签切开，切开的那一段先攒着。
type thinkSplitter struct {
	// state：0 还不知道开头是不是 <think>，1 在 <think> 里，2 刚出 </think>（跳过后面的空白），3 正文
	state int
	buf   string
}

const (
	thinkOpen  = "<think>"
	thinkClose = "</think>"
)

// heldPrefix 是 s 末尾可能是 tag 开头的那一段的长度。
func heldPrefix(s, tag string) int {
	for n := min(len(tag)-1, len(s)); n > 0; n-- {
		if strings.HasSuffix(s, tag[:n]) {
			return n
		}
	}
	return 0
}

// add 收一段正文，返回可以交出去的回答文字与推理内容。
func (t *thinkSplitter) add(s string) (text, thought string) {
	t.buf += s
	for {
		switch t.state {
		case 0:
			trimmed := strings.TrimLeftFunc(t.buf, unicode.IsSpace)
			switch {
			case strings.HasPrefix(trimmed, thinkOpen):
				t.state, t.buf = 1, trimmed[len(thinkOpen):]
			case strings.HasPrefix(thinkOpen, trimmed):
				// 只有空白，或者可能是 <think> 的开头：接着攒
				return text, thought
			default:
				t.state = 3
			}
		case 1:
			if i := strings.Index(t.buf, thinkClose); i >= 0 {
				thought += t.buf[:i]
				t.state, t.buf = 2, t.buf[i+len(thinkClose):]
				continue
			}
			keep := heldPrefix(t.buf, thinkClose)
			thought += t.buf[:len(t.buf)-keep]
			t.buf = t.buf[len(t.buf)-keep:]
			return text, thought
		case 2:
			t.buf = strings.TrimLeftFunc(t.buf, unicode.IsSpace)
			if t.buf == "" {
				return text, thought
			}
			t.state = 3
		default:
			text += t.buf
			t.buf = ""
			return text, thought
		}
	}
}

// flush 交出攒着的：还在 <think> 里的算推理内容，没判断完的算正文。
func (t *thinkSplitter) flush() (text, thought string) {
	buf := t.buf
	t.buf = ""
	switch t.state {
	case 1:
		return "", buf
	case 2:
		return "", ""
	default:
		t.state = 3
		return buf, ""
	}
}

// effortRejected 报告模型服务是不是因为推理等级拒绝了请求（不认这个参数或者这个值）：400、422，错误里提到 effort。
func effortRejected(err error) bool {
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) || (apiErr.StatusCode != http.StatusBadRequest && apiErr.StatusCode != http.StatusUnprocessableEntity) {
		return false
	}
	text := apiErr.Message + " " + apiErr.RawJSON()
	if apiErr.Message == "" {
		text += " " + bodyMessage(apiErr.Response)
	}
	return strings.Contains(strings.ToLower(text), "effort")
}

// streamOptionsRejected 报告模型服务是不是因为 stream_options 拒绝了请求：400、422，错误里提到 stream_options 或 include_usage。
func streamOptionsRejected(err error) bool {
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) || (apiErr.StatusCode != http.StatusBadRequest && apiErr.StatusCode != http.StatusUnprocessableEntity) {
		return false
	}
	text := strings.ToLower(apiErr.Message + " " + apiErr.RawJSON() + " " + bodyMessage(apiErr.Response))
	return strings.Contains(text, "stream_options") || strings.Contains(text, "include_usage")
}

// contextExceeded 报告请求是不是因为超过了模型的上下文窗口被拒绝：400、413、422，错误里说上下文或 token 超了。
func contextExceeded(err error) bool {
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.StatusCode {
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity:
	default:
		return false
	}
	text := strings.ToLower(apiErr.Message + " " + apiErr.RawJSON() + " " + bodyMessage(apiErr.Response))
	return strings.Contains(text, "context_length_exceeded") || strings.Contains(text, "context length") ||
		strings.Contains(text, "context window") || strings.Contains(text, "maximum context") ||
		strings.Contains(text, "too many tokens") || strings.Contains(text, "prompt is too long") ||
		strings.Contains(text, "input is too long") || strings.Contains(text, "上下文")
}

// imageRejected 报告带图片的请求是不是被拒绝了（多半是模型不能看图）：400、415、422。
func imageRejected(err error) bool {
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.StatusCode {
	case http.StatusBadRequest, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity:
		return true
	}
	return false
}

// ping 发一次很短的对话，测试设置能不能用。
func (s *Service) ping(ctx context.Context, c llmConfig) (TestResult, error) {
	chat, err := s.chatService(c)
	if err != nil {
		return TestResult{}, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	start := time.Now()
	res, err := chat.New(ctx, openai.ChatCompletionNewParams{
		Model:    c.Model,
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Reply with the single word OK.")},
	})
	if err != nil {
		return TestResult{}, &TestError{Message: describeError(err, c.APIKey)}
	}
	out := TestResult{Model: res.Model, LatencyMs: time.Since(start).Milliseconds()}
	if out.Model == "" {
		out.Model = c.Model
	}
	if len(res.Choices) > 0 {
		// 先按原文换掉 Key，再通用脱敏：通用规则先改写了 Key 的一部分，原文就对不上了
		out.Reply = truncate(redact.Text(keyScrubber(c.APIKey)(strings.TrimSpace(res.Choices[0].Message.Content))), 200)
	}
	return out, nil
}

// TestError 是测试连接没通：Message 是给人看的原因（不带 Key）。
type TestError struct{ Message string }

func (e *TestError) Error() string { return e.Message }
