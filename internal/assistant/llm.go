package assistant

import (
	"bytes"
	"cmp"
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

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

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
// 429 不带 Retry-After、但写了要等多久（RetryInfo 的 retryDelay，或者正文里的「retry in 48.4s」）时补上 Retry-After，
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
		if res.StatusCode == http.StatusTooManyRequests && res.Header.Get("Retry-After") == "" && res.Header.Get("Retry-After-Ms") == "" {
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

// bodyMessage 从错误回应体里读原因：SDK 只认 {"error":{"message"}}，Google AI Studio 有的错误包在数组里
// （[{"error":{"message"}}]），也有只带 {"message"} 的。读不出来时为空。
func bodyMessage(res *http.Response) string {
	if res == nil || res.Body == nil {
		return ""
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if err != nil {
		return ""
	}
	type wrapped struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	pick := func(w wrapped) string { return strings.TrimSpace(cmp.Or(w.Error.Message, w.Message)) }
	var one wrapped
	if json.Unmarshal(raw, &one) == nil {
		return pick(one)
	}
	var many []wrapped
	if json.Unmarshal(raw, &many) == nil && len(many) > 0 {
		return pick(many[0])
	}
	return ""
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

// retryAfter 是 429 回应建议等多久，说成「，约 N 秒后再试」。和 SDK 决定要不要重试时的顺序一样：先 Retry-After-Ms，
// 再 Retry-After（秒数或 HTTP 日期）；没有或读不懂时为空。
func retryAfter(res *http.Response) string {
	if res == nil {
		return ""
	}
	n := 0
	if ms, err := strconv.ParseFloat(strings.TrimSpace(res.Header.Get("Retry-After-Ms")), 64); err == nil && ms > 0 {
		n = int(math.Ceil(ms / 1000))
	} else if v := strings.TrimSpace(res.Header.Get("Retry-After")); v != "" {
		if secs, err := strconv.ParseFloat(v, 64); err == nil {
			n = int(math.Ceil(secs))
		} else if at, err := http.ParseTime(v); err == nil {
			n = int(math.Ceil(time.Until(at).Seconds()))
		}
	}
	if n <= 0 {
		return ""
	}
	if n >= 120 {
		return fmt.Sprintf("，约 %d 分钟后再试", (n+59)/60)
	}
	return fmt.Sprintf("，约 %d 秒后再试", n)
}

// completion 是模型一次回答：文字、推理内容与工具调用。
type completion struct {
	content   string
	reasoning string
	calls     []rawCall
}

type rawCall struct {
	id, name, args string
	// extra 是这个调用带的 extra_content（原样 JSON）：Gemini 3 的思考签名在这里，之后的请求要原样带回去
	extra string
}

// maxExtraBytes 是一个工具调用的 extra_content 最多存多大
const maxExtraBytes = 16 << 10

// accumulator 拼流式回答。不用 openai.ChatCompletionAccumulator：它碰到各片 id 不一致、工具调用没有 index 的
// OpenAI 兼容服务会丢内容。
type accumulator struct {
	content, reasoning strings.Builder
	calls              []*rawCall
	byIndex            map[int64]int
}

func (a *accumulator) add(chunk openai.ChatCompletionChunk) (delta string) {
	if len(chunk.Choices) == 0 {
		return ""
	}
	d := chunk.Choices[0].Delta
	if f, ok := d.JSON.ExtraFields["reasoning_content"]; ok {
		var t string
		if json.Unmarshal([]byte(f.Raw()), &t) == nil {
			a.reasoning.WriteString(t)
		}
	}
	a.content.WriteString(d.Content)
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
	return d.Content
}

func (a *accumulator) result() completion {
	out := completion{content: a.content.String(), reasoning: a.reasoning.String()}
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

// complete 流式调一次模型，文字增量经 emit 发出去。
func (s *Service) complete(ctx context.Context, chat openai.ChatCompletionService, model string,
	msgs []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolUnionParam, emit func(string),
) (completion, error) {
	ctx, cancel := context.WithTimeout(ctx, stepTimeout)
	defer cancel()
	params := openai.ChatCompletionNewParams{Model: model, Messages: msgs}
	if len(tools) > 0 {
		params.Tools = tools
	}
	stream := chat.NewStreaming(ctx, params)
	defer stream.Close()
	var acc accumulator
	fl := &deltaFlusher{emit: emit, last: time.Now()}
	for stream.Next() {
		if d := acc.add(stream.Current()); d != "" {
			fl.add(d, time.Now())
		}
	}
	fl.flush(time.Now())
	if err := stream.Err(); err != nil {
		return completion{}, err
	}
	return acc.result(), nil
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
