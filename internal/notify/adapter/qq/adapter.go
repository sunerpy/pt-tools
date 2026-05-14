package qq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/driver"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

const channelType = "qq_onebot"

type qqConfig struct {
	ListenAddr     string  `json:"listen_addr"`
	Path           string  `json:"path"`
	AccessToken    string  `json:"access_token"`
	AdminQQUsers   []int64 `json:"admin_qq_users"`
	AllowedQQUsers []int64 `json:"allowed_qq_users"`
}

type inboundMessage = notify.InboundMessage

type QQChannel struct {
	confID       uint
	cfg          qqConfig
	adminUsers   map[int64]struct{}
	allowedUsers map[int64]struct{}

	pg *paginator

	lifecycleCtx    context.Context
	lifecycleCancel context.CancelFunc

	httpServer *http.Server
	listener   net.Listener
	wsServer   *driver.WSServer

	caller   atomic.Value
	healthy  atomic.Bool
	startErr error

	handlerMu      sync.RWMutex
	inboundHandler notify.InboundHandler

	closeOnce sync.Once
}

func New() *QQChannel {
	return &QQChannel{
		adminUsers:   make(map[int64]struct{}),
		allowedUsers: make(map[int64]struct{}),
		pg:           newPaginator(20, 5*time.Minute),
	}
}

func (q *QQChannel) Type() string          { return channelType }
func (q *QQChannel) SupportsInbound() bool { return true }
func (q *QQChannel) Healthy() bool         { return q.healthy.Load() }

func (q *QQChannel) Init(ctx context.Context, conf *models.NotificationConf) error {
	if conf == nil {
		return errors.New("QQ 适配器: NotificationConf 为空")
	}
	q.confID = conf.ID

	cfg, err := parseConfig(conf.ConfigJSON)
	if err != nil {
		return fmt.Errorf("QQ 适配器: 解析 config_json 失败: %w", err)
	}
	q.cfg = cfg
	q.adminUsers = toIDSet(cfg.AdminQQUsers)
	q.allowedUsers = toIDSet(cfg.AllowedQQUsers)

	q.lifecycleCtx, q.lifecycleCancel = context.WithCancel(context.Background())

	if cfg.ListenAddr == "" {
		warnLogger().Warnf("QQ 适配器(%d): listen_addr 为空，跳过 ws 启动", conf.ID)
		return nil
	}

	if err := q.startServer(); err != nil {
		warnLogger().Warnf("QQ 适配器(%d): 启动失败: %v (将稍后由调用方重试)", conf.ID, err)
		q.startErr = err
		return nil
	}
	q.healthy.Store(true)
	return nil
}

func (q *QQChannel) startServer() error {
	lc := &net.ListenConfig{}
	listener, err := lc.Listen(q.lifecycleCtx, "tcp", q.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("监听 %s 失败: %w", q.cfg.ListenAddr, err)
	}
	q.listener = listener

	wss := driver.NewWebSocketServer(16, q.cfg.ListenAddr, q.cfg.AccessToken)
	q.wsServer = wss

	mux := http.NewServeMux()
	path := q.cfg.Path
	if path == "" {
		path = "/onebot/v11/ws"
	}
	mux.HandleFunc(path, q.wsHandshakeHandler)

	q.httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := q.httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			warnLogger().Warnf("QQ 适配器(%d): http.Serve 退出: %v", q.confID, err)
			q.healthy.Store(false)
			q.pg.OnReconnect(q.confIDKey())
		}
	}()
	return nil
}

func (q *QQChannel) wsHandshakeHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (q *QQChannel) Send(ctx context.Context, n notify.Notification) error {
	chatID := n.Targets["chat_id"]
	if chatID == "" {
		chatID = n.UserID
	}
	if chatID == "" {
		return errors.New("QQ 适配器: 缺少目标 chat_id")
	}

	body := strings.TrimSpace(n.Title)
	if body != "" && n.Text != "" {
		body += "\n"
	}
	body += stripMarkdown(n.Text)
	if n.Link != "" {
		if body != "" {
			body += "\n"
		}
		body += n.Link
	}
	return q.sendOutbound(ctx, chatID, body)
}

func (q *QQChannel) OnInbound(handler notify.InboundHandler) {
	q.handlerMu.Lock()
	defer q.handlerMu.Unlock()
	q.inboundHandler = handler
}

func (q *QQChannel) Close(_ context.Context) error {
	q.closeOnce.Do(func() {
		if q.lifecycleCancel != nil {
			q.lifecycleCancel()
		}
		if q.httpServer != nil {
			_ = q.httpServer.Close()
		}
		if q.listener != nil {
			_ = q.listener.Close()
		}
		if q.pg != nil {
			q.pg.Stop()
		}
		q.healthy.Store(false)
	})
	return nil
}

func (q *QQChannel) activeCaller() zero.APICaller {
	v := q.caller.Load()
	if v == nil {
		return nil
	}
	c, _ := v.(zero.APICaller)
	return c
}

func (q *QQChannel) confIDKey() string {
	return fmt.Sprintf("conf-%d", q.confID)
}

func parseConfig(raw string) (qqConfig, error) {
	var cfg qqConfig
	if strings.TrimSpace(raw) == "" {
		return cfg, errors.New("config_json 为空")
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return cfg, err
	}
	if cfg.Path == "" {
		cfg.Path = "/onebot/v11/ws"
	}
	return cfg, nil
}

func toIDSet(ids []int64) map[int64]struct{} {
	out := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out
}

func stripMarkdown(s string) string {
	r := strings.NewReplacer(
		"**", "",
		"*", "",
		"`", "",
		"~~", "",
	)
	return r.Replace(s)
}

type warnLog interface {
	Warnf(template string, args ...interface{})
}

func warnLogger() warnLog {
	if global.GetLogger() == nil {
		return nopLogger{}
	}
	return global.GetSlogger()
}

type nopLogger struct{}

func (nopLogger) Warnf(string, ...interface{}) {}

func init() {
	notify.RegisterChannel(channelType, func() notify.Channel { return New() })
}
