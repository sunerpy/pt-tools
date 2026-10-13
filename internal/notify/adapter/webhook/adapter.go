package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/utils/httpclient"
)

// Config 是 Webhook 通道的配置。Method、ContentType、BodyTemplate 都空着时发原来的固定 JSON（多一个 event 字段）。
// 请求体模板里可能写着 token，和其余配置一样只存在加密的 ConfigJSON 里。
type Config struct {
	EndpointURL    string  `json:"endpoint_url"`
	TimeoutSeconds int     `json:"timeout_seconds"`
	HMACSecret     string  `json:"hmac_secret,omitempty"`
	Headers        Headers `json:"headers,omitempty"`
	// Method 是请求方法：POST（默认）、PUT、PATCH、GET（GET 不带请求体）
	Method string `json:"method,omitempty"`
	// ContentType 是请求体的格式：json（默认）、text、form、custom（Content-Type 写在 Headers 里）
	ContentType string `json:"content_type,omitempty"`
	// BodyTemplate 是请求体模板（Go 模板，变量见消息模板的事件目录）；空着发原来的固定 JSON
	BodyTemplate string `json:"body_template,omitempty"`
}

// Headers 是自定义请求头。旧版编辑页把它存成了 JSON 文本（"{\"A\":\"b\"}"），两种写法都认。
type Headers map[string]string

func (h *Headers) UnmarshalJSON(b []byte) error {
	var m map[string]string
	if err := json.Unmarshal(b, &m); err == nil {
		*h = m
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return errors.New("headers 要是 JSON 对象，键和值都是文字")
	}
	if strings.TrimSpace(s) == "" {
		*h = nil
		return nil
	}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return errors.New("headers 要是 JSON 对象，键和值都是文字")
	}
	*h = m
	return nil
}

// get 按名字取请求头（不分大小写）。
func (h Headers) get(name string) string {
	for k, v := range h {
		if strings.EqualFold(strings.TrimSpace(k), name) {
			return v
		}
	}
	return ""
}

// WebhookPayload 是不写请求体模板时发的固定 JSON。Event 是后来加的：原有字段不变。
type WebhookPayload struct {
	EventType string                 `json:"event_type"`
	Title     string                 `json:"title"`
	Text      string                 `json:"text"`
	Link      string                 `json:"link,omitempty"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
	Event     *notify.Event          `json:"event,omitempty"`
}

type WebhookChannel struct {
	config *Config
	req    *request
	name   string
	logger *zap.SugaredLogger
}

func NewWebhookChannel() notify.Channel {
	return &WebhookChannel{}
}

func (w *WebhookChannel) Type() string {
	return "webhook"
}

func parseConfig(raw string) (*Config, error) {
	cfg := &Config{}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config_json failed: %w", err)
	}
	if strings.TrimSpace(cfg.EndpointURL) == "" {
		return nil, fmt.Errorf("endpoint_url is required")
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 5
	}
	return cfg, nil
}

func (w *WebhookChannel) Init(ctx context.Context, conf *models.NotificationConf) error {
	w.logger = sLogger()

	cfg, err := parseConfig(conf.ConfigJSON)
	if err != nil {
		return err
	}
	req, err := compile(cfg)
	if err != nil {
		return err
	}
	w.config, w.req, w.name = cfg, req, conf.Name
	return nil
}

// CheckConfig 在保存前检查配置（不发请求）：方法、格式、请求头，地址与请求体模板要能用每种事件的示例数据渲染出来，
// JSON 格式的请求体还要是合法的 JSON。
func (w *WebhookChannel) CheckConfig(conf *models.NotificationConf) error {
	cfg, err := parseConfig(conf.ConfigJSON)
	if err != nil {
		return err
	}
	req, err := compile(cfg)
	if err != nil {
		return err
	}
	return req.checkSamples()
}

func (w *WebhookChannel) SupportsInbound() bool {
	return false
}

func (w *WebhookChannel) Send(ctx context.Context, n notify.Notification) error {
	if w.config == nil || w.req == nil {
		return fmt.Errorf("webhook channel not initialized")
	}

	r, err := w.req.render(n, w.name, false)
	if err != nil {
		w.logger.Warnf("webhook 请求没有生成出来: %v", err)
		return fmt.Errorf("webhook 请求没有生成出来: %w", err)
	}

	// 先放 Content-Type，再放自定义请求头（写了的以它为准），签名最后放
	headers := http.Header{}
	if r.ContentType != "" {
		headers.Set("Content-Type", r.ContentType)
	}
	keys := make([]string, 0, len(w.config.Headers))
	for k := range w.config.Headers {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		headers.Set(strings.TrimSpace(k), w.config.Headers[k])
	}
	if w.config.HMACSecret != "" {
		sig := hmac.New(sha256.New, []byte(w.config.HMACSecret))
		sig.Write(r.Body)
		headers.Set("X-PT-Signature", base64.StdEncoding.EncodeToString(sig.Sum(nil)))
	}

	timeout := time.Duration(w.config.TimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	opts := []httpclient.RequestOption{
		httpclient.WithTimeout(timeout),
		httpclient.WithContext(ctx),
	}
	for k := range headers {
		opts = append(opts, httpclient.WithHeader(k, headers.Get(k)))
	}

	resp, err := do(r.Method, r.URL, r.Body, opts...)
	if err != nil {
		err = reason(err)
		w.logger.Warnf("webhook send failed: %v", err)
		return fmt.Errorf("webhook send failed: %w", err)
	}

	// 只有 2xx 算成功：3xx 没有跟到新地址时也是没送到
	if !resp.IsSuccess() {
		w.logger.Warnf("webhook returned status: %d", resp.StatusCode())
		return fmt.Errorf("webhook returned status %d", resp.StatusCode())
	}

	return nil
}

func do(method, url string, body []byte, opts ...httpclient.RequestOption) (httpclient.Response, error) {
	switch method {
	case http.MethodPut:
		return httpclient.Put(url, body, opts...)
	case http.MethodPatch:
		return httpclient.Patch(url, body, opts...)
	case http.MethodGet:
		return httpclient.Get(url, opts...)
	}
	return httpclient.Post(url, body, opts...)
}

// reason 是请求错误里不带地址的那一层：net/http 与 requests 包的外层错误都写着完整的地址，
// 地址的查询部分可能有 token 或渲染进去的内容。往里找到第一层不含地址的；找不到就只说没发出去。
func reason(err error) error {
	for e := err; e != nil; e = errors.Unwrap(e) {
		if !strings.Contains(e.Error(), "://") {
			return e
		}
	}
	return errors.New("请求没有发出去")
}

func (w *WebhookChannel) OnInbound(handler notify.InboundHandler) {
	w.logger.Warn("webhook adapter does not support inbound messages")
}

func (w *WebhookChannel) Close(ctx context.Context) error {
	return nil
}

func (w *WebhookChannel) Healthy() bool {
	return w.config != nil
}
