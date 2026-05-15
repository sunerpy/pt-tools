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

	"github.com/RomiChan/websocket"
	zero "github.com/wdvxdr1123/ZeroBot"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

const (
	channelType = "qq_onebot"

	// qqReadDeadline 是 WS 读取的最长沉默期：在此期间内若收不到任何 frame
	// (含 NapCat 心跳事件、ping 的 pong 回包等)，视为半开链路并触发重连。
	qqReadDeadline = 90 * time.Second
	// qqPingInterval 是后台主动 ping 的间隔。
	qqPingInterval = 30 * time.Second
	// qqPongTimeout 是单次 ping 写控制帧的超时。
	qqPongTimeout = 10 * time.Second
)

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

	caller   atomic.Pointer[napCatCaller]
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

func (q *QQChannel) wsHandshakeHandler(w http.ResponseWriter, r *http.Request) {
	// 鉴权 (mirror ZeroBot driver/wsserver.go: checkAuth)
	if status := checkAccessToken(r, q.cfg.AccessToken); status != http.StatusOK {
		warnLogger().Warnf("QQ 适配器(%d): 拒绝 %v 的 WS 请求: token 鉴权失败 (code:%d)", q.confID, r.RemoteAddr, status)
		w.WriteHeader(status)
		return
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		warnLogger().Warnf("QQ 适配器(%d): WS upgrade 失败: %v", q.confID, err)
		return
	}

	// 读取 OneBot 反向 WS 握手帧 {"self_id": ...}
	var hello struct {
		SelfID int64 `json:"self_id"`
	}
	if err := conn.ReadJSON(&hello); err != nil {
		warnLogger().Warnf("QQ 适配器(%d): 握手读 self_id 失败: %v", q.confID, err)
		_ = conn.Close()
		return
	}

	c := newNapCatCaller(conn, hello.SelfID)
	q.caller.Store(c)
	q.healthy.Store(true)
	warnLogger().Warnf("QQ 适配器(%d): [wss] 连接Websocket服务器: %s 成功, 账号: %d", q.confID, r.RemoteAddr, hello.SelfID)

	go q.listenCaller(c)
}

// checkAccessToken 复刻 ZeroBot driver.checkAuth 的逻辑。
func checkAccessToken(req *http.Request, token string) int {
	if token == "" {
		return http.StatusOK
	}
	auth := req.Header.Get("Authorization")
	if auth == "" {
		auth = req.URL.Query().Get("access_token")
	} else if _, after, ok := strings.Cut(auth, " "); ok {
		auth = after
	}
	switch auth {
	case token:
		return http.StatusOK
	case "":
		return http.StatusUnauthorized
	default:
		return http.StatusForbidden
	}
}

// listenCaller 阻塞式读取 NapCat 推送的 OneBot 事件并分发。
//
// 半开链路保护:
//   - 设置 ReadDeadline，任何 frame (含 pong) 收到时刷新；超时即返回错误退出读循环。
//   - 后台 30s 间隔主动 ping；写失败立即 Close 触发重连。
//
// 已知遗留: 若 NapCat 仅 WS 协议层活跃但内部消息 dispatcher 卡住 (仍能 pong)，
// 此机制无效，需要切换到 forward-WS 或 HTTP webhook 才能彻底解决。
func (q *QQChannel) listenCaller(c *napCatCaller) {
	defer func() {
		q.caller.Store(nil)
		q.healthy.Store(false)
		_ = c.conn.Close()
		warnLogger().Warnf("QQ 适配器(%d): [wss] WebSocket 连接断开, 账号: %d", q.confID, c.selfID)
	}()

	// 初始 read deadline。
	if err := c.conn.SetReadDeadline(time.Now().Add(qqReadDeadline)); err != nil {
		warnLogger().Warnf("QQ 适配器(%d): 设置 ReadDeadline 失败: %v", q.confID, err)
	}
	// pong 收到后刷新 deadline。
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(qqReadDeadline))
	})

	// 后台 ping pump：写失败时主动 Close 以打断 ReadMessage 阻塞。
	pingCtx, cancelPing := context.WithCancel(q.lifecycleCtx)
	defer cancelPing()
	go func() {
		ticker := time.NewTicker(qqPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-pingCtx.Done():
				return
			case <-ticker.C:
				if err := c.conn.WriteControl(
					websocket.PingMessage,
					nil,
					time.Now().Add(qqPongTimeout),
				); err != nil {
					warnLogger().Warnf("QQ 适配器(%d): WS Ping 发送失败: %v (主动关闭以触发重连)", q.confID, err)
					_ = c.conn.Close()
					return
				}
			}
		}
	}()

	for {
		t, payload, err := c.conn.ReadMessage()
		if err != nil {
			// 任何读错误（含 deadline exceeded）都触发 defer 关闭连接。
			warnLogger().Warnf("QQ 适配器(%d): WS Read 异常 (可能 NapCat 半死链路): %v", q.confID, err)
			return
		}
		// 任何收到的 frame 都刷新 deadline。
		_ = c.conn.SetReadDeadline(time.Now().Add(qqReadDeadline))
		if t != websocket.TextMessage {
			continue
		}
		// API 调用响应（含 echo 字段）走回调通道；否则视为事件。
		if c.dispatchAPIResponse(payload) {
			continue
		}
		if err := q.HandleRawEvent(payload); err != nil {
			warnLogger().Warnf("QQ 适配器(%d): 处理 OneBot 事件失败: %v", q.confID, err)
		}
	}
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
	return q.sendOutbound(ctx, chatID, body, n.Targets["message_type"])
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
	c := q.caller.Load()
	if c == nil {
		return nil
	}
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
