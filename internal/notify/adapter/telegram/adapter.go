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
	"strconv"
	"strings"
	"sync"

	"github.com/mymmrac/telego"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

// ChannelType is the registry key for the Telegram adapter.
const ChannelType = "telegram"

// Config is the JSON shape stored (encrypted) in NotificationConf.ConfigJSON.
//
// AllowedUsers / AdminUsers / DefaultChatID are stored as json.RawMessage so
// that the adapter tolerates both numeric and string-quoted values that users
// commonly type into the Web UI form (e.g. `"8576996727"` or `"@channel"`).
// Use the helper methods (DefaultChatIDInt, DefaultChatIDUsername,
// AdminUsersList, AllowedUsersList) to access typed values.
type Config struct {
	BotToken              string          `json:"bot_token"`
	AllowedUsers          json.RawMessage `json:"allowed_users,omitempty"`
	AdminUsers            json.RawMessage `json:"admin_users,omitempty"`
	DefaultChatID         json.RawMessage `json:"default_chat_id,omitempty"`
	PollingTimeoutSeconds int             `json:"polling_timeout_seconds"`
	APIServer             string          `json:"api_server,omitempty"`
	ProxyURL              string          `json:"proxy_url,omitempty"`
}

// DefaultChatIDInt returns the integer form of default_chat_id. Accepts both
// raw JSON integers and string-quoted integers ("123456"). Returns (0, false)
// when the value is absent or is a non-numeric string (e.g. "@channelusername"),
// in which case the caller should fall back to DefaultChatIDUsername.
func (c *Config) DefaultChatIDInt() (int64, bool) {
	if len(c.DefaultChatID) == 0 {
		return 0, false
	}
	var n int64
	if err := json.Unmarshal(c.DefaultChatID, &n); err == nil {
		return n, true
	}
	var s string
	if err := json.Unmarshal(c.DefaultChatID, &s); err == nil {
		s = strings.TrimSpace(s)
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n, true
		}
	}
	return 0, false
}

// DefaultChatIDUsername returns the @channelusername form. Only returns a
// non-empty string when the stored value is a string that does NOT parse as
// an integer (so DefaultChatIDInt and DefaultChatIDUsername never both
// return a usable value for the same config).
func (c *Config) DefaultChatIDUsername() (string, bool) {
	if len(c.DefaultChatID) == 0 {
		return "", false
	}
	var s string
	if err := json.Unmarshal(c.DefaultChatID, &s); err != nil {
		return "", false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		return "", false
	}
	if !strings.HasPrefix(s, "@") {
		s = "@" + s
	}
	return s, true
}

// AdminUsersList returns admin_users as []int64. Tolerates both raw integer
// and string-quoted integers in the JSON array; bad entries are silently
// skipped to avoid taking down the whole channel for one typo.
func (c *Config) AdminUsersList() []int64 {
	return parseUserIDList(c.AdminUsers)
}

// AllowedUsersList returns allowed_users as []int64 with the same tolerance
// as AdminUsersList.
func (c *Config) AllowedUsersList() []int64 {
	return parseUserIDList(c.AllowedUsers)
}

// parseUserIDList tolerates [123, "456", "bad"] mixed input. Returns nil on
// invalid JSON or empty input. Bad entries (non-integer strings) are skipped
// without error so a single bad row doesn't disable the channel.
func parseUserIDList(raw json.RawMessage) []int64 {
	if len(raw) == 0 {
		return nil
	}
	var rawList []json.RawMessage
	if err := json.Unmarshal(raw, &rawList); err != nil {
		return nil
	}
	out := make([]int64, 0, len(rawList))
	for _, item := range rawList {
		var n int64
		if err := json.Unmarshal(item, &n); err == nil {
			out = append(out, n)
			continue
		}
		var s string
		if err := json.Unmarshal(item, &s); err == nil && s != "" {
			if n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
				out = append(out, n)
			}
		}
	}
	return out
}

// botAPI is the minimal telego.Bot surface used by the adapter, kept narrow
// so tests can substitute a fake without standing up an HTTPS server.
type botAPI interface {
	GetMe(ctx context.Context) (*telego.User, error)
	SendMessage(ctx context.Context, params *telego.SendMessageParams) (*telego.Message, error)
	AnswerCallbackQuery(ctx context.Context, params *telego.AnswerCallbackQueryParams) error
	EditMessageReplyMarkup(ctx context.Context, params *telego.EditMessageReplyMarkupParams) (*telego.Message, error)
}

// CallbackActionHandler dispatches RSS notification button actions parsed
// from the inline keyboard payload. Set on the channel via
// SetCallbackActionHandler. If nil, callbacks are acknowledged but no
// downstream action is taken (stub mode for testing / pre-S5 boots).
type CallbackActionHandler interface {
	OnRSSDownload(ctx context.Context, logID uint, userID int64) error
	OnRSSIgnore(ctx context.Context, logID uint, userID int64) error
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

	factory       botFactory
	actionHandler CallbackActionHandler
}

// New constructs a fresh TelegramChannel.
func New() *TelegramChannel {
	return &TelegramChannel{
		factory: defaultBotFactory,
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

// SetCallbackActionHandler installs the dispatcher for RSS notification
// inline-button actions. Safe to call before or after Init. Pass nil to
// revert to stub mode.
func (c *TelegramChannel) SetCallbackActionHandler(h CallbackActionHandler) {
	c.mu.Lock()
	c.actionHandler = h
	c.mu.Unlock()
}

// Init parses the plaintext ConfigJSON (already decrypted by cmd/web.go),
// builds a per-conf telego bot, and launches the long-poll goroutine.
func (c *TelegramChannel) Init(ctx context.Context, conf *models.NotificationConf) error {
	if conf == nil {
		return errors.New("telegram: NotificationConf is nil")
	}

	if c.factory == nil {
		c.factory = defaultBotFactory
	}

	c.mu.Lock()
	c.logger = sLogger()
	c.confID = conf.ID
	c.mu.Unlock()

	cfg := &Config{}
	if unmarshalErr := json.Unmarshal([]byte(conf.ConfigJSON), cfg); unmarshalErr != nil {
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
