package wecom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/notify/adapter/outbound"
	"github.com/sunerpy/pt-tools/models"
)

type WeComChannel struct {
	webhookKey string
	msgType    string
	client     *outbound.Client // 为空时用 outbound.Default()
}

func (w *WeComChannel) Type() string {
	return "wecom_webhook"
}

func (w *WeComChannel) Init(ctx context.Context, conf *models.NotificationConf) error {
	if conf == nil {
		return errors.New("notification conf is nil")
	}

	var cfg struct {
		WebhookKey string `json:"webhook_key"`
		MsgType    string `json:"msg_type"`
	}

	if err := json.Unmarshal([]byte(conf.ConfigJSON), &cfg); err != nil {
		return fmt.Errorf("解析 wecom webhook 配置失败: %w", err)
	}

	w.webhookKey = strings.TrimSpace(cfg.WebhookKey)
	if w.webhookKey == "" {
		return errors.New("wecom webhook_key 为空")
	}
	w.msgType = cfg.MsgType
	if w.msgType == "" {
		w.msgType = "markdown"
	}
	if w.msgType != "markdown" && w.msgType != "text" {
		return fmt.Errorf("wecom msg_type 无效: %s", w.msgType)
	}

	return nil
}

// CheckConfig 只检查配置、不发请求：保存配置前调用。
func (w *WeComChannel) CheckConfig(conf *models.NotificationConf) error {
	return (&WeComChannel{}).Init(context.Background(), conf)
}

func (w *WeComChannel) SupportsInbound() bool {
	return false
}

func (w *WeComChannel) Send(ctx context.Context, n notify.Notification) error {
	return w.sendNotification(ctx, n)
}

func (w *WeComChannel) OnInbound(handler notify.InboundHandler) {
}

func (w *WeComChannel) Close(ctx context.Context) error {
	return nil
}

func (w *WeComChannel) Healthy() bool {
	return true
}

func init() {
	notify.RegisterChannel("wecom_webhook", func() notify.Channel {
		return &WeComChannel{}
	})
}
