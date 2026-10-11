package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
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
	hc := s.cfg.HTTPClient
	if hc == nil {
		var err error
		if hc, err = newHTTPClient(c.Proxy); err != nil {
			return openai.ChatCompletionService{}, err
		}
	}
	opts := []option.RequestOption{option.WithBaseURL(c.BaseURL), option.WithHTTPClient(hc), option.WithMaxRetries(1)}
	if c.APIKey != "" {
		opts = append(opts, option.WithAPIKey(c.APIKey))
	}
	return openai.NewChatCompletionService(opts...), nil
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
			detail = http.StatusText(apiErr.StatusCode)
		}
		msg = fmt.Sprintf("模型服务回 %d：%s", apiErr.StatusCode, detail)
		switch apiErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			msg += "（检查 API Key）"
		case http.StatusNotFound:
			msg += "（检查接口地址与模型名）"
		case http.StatusTooManyRequests:
			msg += "（请求太多或额度用完了）"
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

// completion 是模型一次回答：文字、推理内容与工具调用。
type completion struct {
	content   string
	reasoning string
	calls     []rawCall
}

type rawCall struct {
	id, name, args string
}

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
		out.Reply = truncate(keyScrubber(c.APIKey)(redact.Text(strings.TrimSpace(res.Choices[0].Message.Content))), 200)
	}
	return out, nil
}

// TestError 是测试连接没通：Message 是给人看的原因（不带 Key）。
type TestError struct{ Message string }

func (e *TestError) Error() string { return e.Message }
