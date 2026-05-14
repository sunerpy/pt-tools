// Package telegram implements the notify.Channel adapter for Telegram bots
// using mymmrac/telego with long-polling. Each NotificationConf gets its own
// bot instance and goroutine; Init failures are non-fatal so that application
// boot is never blocked by a misconfigured channel.
package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/mymmrac/telego"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

// ChannelType is the registry key for the Telegram adapter.
const ChannelType = "telegram"

// Config is the JSON shape stored (encrypted) in NotificationConf.ConfigJSON.
type Config struct {
	BotToken              string  `json:"bot_token"`
	AllowedUsers          []int64 `json:"allowed_users"`
	AdminUsers            []int64 `json:"admin_users"`
	DefaultChatID         int64   `json:"default_chat_id"`
	PollingTimeoutSeconds int     `json:"polling_timeout_seconds"`
	APIServer             string  `json:"api_server,omitempty"`
}

// botAPI is the minimal telego.Bot surface used by the adapter, kept narrow
// so tests can substitute a fake without standing up an HTTPS server.
type botAPI interface {
	GetMe(ctx context.Context) (*telego.User, error)
	SendMessage(ctx context.Context, params *telego.SendMessageParams) (*telego.Message, error)
}

type updateSource func(ctx context.Context) (<-chan telego.Update, error)

type botFactory func(cfg *Config) (botAPI, updateSource, error)

// TelegramChannel implements notify.Channel for Telegram.
type TelegramChannel struct {
	mu      sync.RWMutex
	cfg     *Config
	confID  uint
	bot     botAPI
	logger  *zap.SugaredLogger
	handler notify.InboundHandler
	healthy bool

	pollCtx    context.Context
	pollCancel context.CancelFunc
	pollDone   chan struct{}

	factory   botFactory
	decryptFn func(string) ([]byte, error)
}

// New constructs a fresh TelegramChannel.
func New() *TelegramChannel {
	return &TelegramChannel{
		factory:   defaultBotFactory,
		decryptFn: crypto.Decrypt,
	}
}

// Type returns the registry key.
func (c *TelegramChannel) Type() string { return ChannelType }

// SupportsInbound reports that Telegram delivers inbound chat messages.
func (c *TelegramChannel) SupportsInbound() bool { return true }

// Healthy reports whether Init succeeded and the bot is currently usable.
func (c *TelegramChannel) Healthy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.healthy
}

// OnInbound registers the handler invoked once a message passes the
// permission gate.
func (c *TelegramChannel) OnInbound(handler notify.InboundHandler) {
	c.mu.Lock()
	c.handler = handler
	c.mu.Unlock()
}

// Init parses the encrypted ConfigJSON, builds a per-conf telego bot, and
// launches the long-poll goroutine.
func (c *TelegramChannel) Init(ctx context.Context, conf *models.NotificationConf) error {
	if conf == nil {
		return errors.New("telegram: NotificationConf is nil")
	}

	if c.factory == nil {
		c.factory = defaultBotFactory
	}
	if c.decryptFn == nil {
		c.decryptFn = crypto.Decrypt
	}

	c.mu.Lock()
	c.logger = sLogger()
	c.confID = conf.ID
	c.mu.Unlock()

	plain, err := c.decryptFn(conf.ConfigJSON)
	if err != nil {
		c.markUnhealthy()
		return fmt.Errorf("telegram: 解密 config_json 失败: %w", err)
	}

	cfg := &Config{}
	if unmarshalErr := json.Unmarshal(plain, cfg); unmarshalErr != nil {
		c.markUnhealthy()
		return fmt.Errorf("telegram: 解析 config_json 失败: %w", unmarshalErr)
	}
	if cfg.BotToken == "" {
		c.markUnhealthy()
		return errors.New("telegram: bot_token 不能为空")
	}

	bot, src, err := c.factory(cfg)
	if err != nil {
		c.markUnhealthy()
		return fmt.Errorf("telegram: 创建 bot 失败: %w", err)
	}

	c.mu.Lock()
	c.cfg = cfg
	c.bot = bot
	c.healthy = true
	c.pollDone = make(chan struct{})
	pollCtx, cancel := context.WithCancel(context.Background())
	c.pollCtx = pollCtx
	c.pollCancel = cancel
	c.mu.Unlock()

	go c.runInbound(pollCtx, src)

	return nil
}

// Close stops the long-poll loop and clears state.
func (c *TelegramChannel) Close(ctx context.Context) error {
	c.mu.Lock()
	cancel := c.pollCancel
	done := c.pollDone
	c.healthy = false
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (c *TelegramChannel) markUnhealthy() {
	c.mu.Lock()
	c.healthy = false
	c.mu.Unlock()
}

func init() {
	notify.RegisterChannel(ChannelType, func() notify.Channel { return New() })
}
