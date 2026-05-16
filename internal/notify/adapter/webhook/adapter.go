package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/utils/httpclient"
)

type Config struct {
	EndpointURL    string            `json:"endpoint_url"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	HMACSecret     string            `json:"hmac_secret,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
}

type WebhookPayload struct {
	EventType string                 `json:"event_type"`
	Title     string                 `json:"title"`
	Text      string                 `json:"text"`
	Link      string                 `json:"link,omitempty"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
}

type WebhookChannel struct {
	config *Config
	logger *zap.SugaredLogger
}

func NewWebhookChannel() notify.Channel {
	return &WebhookChannel{}
}

func (w *WebhookChannel) Type() string {
	return "webhook"
}

func (w *WebhookChannel) Init(ctx context.Context, conf *models.NotificationConf) error {
	w.logger = sLogger()

	cfg := &Config{}
	err := json.Unmarshal([]byte(conf.ConfigJSON), cfg)
	if err != nil {
		return fmt.Errorf("unmarshal config_json failed: %w", err)
	}

	if cfg.EndpointURL == "" {
		return fmt.Errorf("endpoint_url is required")
	}

	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 5
	}

	w.config = cfg
	return nil
}

func (w *WebhookChannel) SupportsInbound() bool {
	return false
}

func (w *WebhookChannel) Send(ctx context.Context, n notify.Notification) error {
	if w.config == nil {
		return fmt.Errorf("webhook channel not initialized")
	}

	payload := WebhookPayload{
		EventType: "notification",
		Title:     n.Title,
		Text:      n.Text,
		Link:      n.Link,
		Payload: map[string]interface{}{
			"channel_type":   n.ChannelType,
			"source_conf_id": n.SourceConfID,
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload failed: %w", err)
	}

	headers := make(map[string]string)
	headers["Content-Type"] = "application/json"

	if w.config.Headers != nil {
		for k, v := range w.config.Headers {
			headers[k] = v
		}
	}

	if w.config.HMACSecret != "" {
		sig := hmac.New(sha256.New, []byte(w.config.HMACSecret))
		sig.Write(bodyBytes)
		headers["X-PT-Signature"] = base64.StdEncoding.EncodeToString(sig.Sum(nil))
	}

	timeout := time.Duration(w.config.TimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	opts := []httpclient.RequestOption{
		httpclient.WithTimeout(timeout),
		httpclient.WithContext(ctx),
	}

	for k, v := range headers {
		opts = append(opts, httpclient.WithHeader(k, v))
	}

	resp, err := httpclient.Post(w.config.EndpointURL, bodyBytes, opts...)
	if err != nil {
		w.logger.Warnf("webhook send failed: %v", err)
		return fmt.Errorf("webhook send failed: %w", err)
	}

	if resp.IsError() {
		w.logger.Warnf("webhook returned error status: %d", resp.StatusCode())
		return fmt.Errorf("webhook returned status %d", resp.StatusCode())
	}

	return nil
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
