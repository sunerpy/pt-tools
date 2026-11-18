// Package feishuapp 是飞书（Lark）企业自建应用机器人通道（类型 feishu_app）：经官方 Go SDK 的长连接收
// 「接收消息」事件，经 im/v1 接口发文字消息。pt-tools 主动连出，不需要公网地址。
//
// 与 adapter/feishu（群自定义机器人 Webhook，只出站）是两种通道，配置与行为都不同。
//
// 长连接是集群模式：同一个应用的多个连接里，每条事件只推给随机一个。所以 Init 只解析配置、建好发消息用的
// client，不连长连接；第一次 OnInbound 时才起监督协程去连。保存前的 checkConf 与 outbox 的回退只 Init 不
// OnInbound，它们不会变成同一个应用的第二个客户端。
package feishuapp

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

// ChannelType 是通道类型（注册表的键，也写进绑定与审计的 channel_type）。
const ChannelType = "feishu_app"

// 长连接的状态。
const (
	wsWaiting int32 = iota
	wsConnected
	wsFailed
)

// Channel 实现 notify.Channel、notify.ConfigChecker 与 notify.LinkStater。
type Channel struct {
	mu      sync.RWMutex
	cfg     *Config
	confID  uint
	api     apiSender
	handler notify.InboundHandler
	red     redactor
	// log 是通道自己的 logger（测试换成记录用的）；为空时用全局 logger
	log atomic.Pointer[zap.SugaredLogger]

	newAPI    apiFactory
	newRunner runnerFactory
	now       func() time.Time

	limiters *limiterSet
	queues   *userQueues

	inited    atomic.Bool
	closed    atomic.Bool
	ws        atomic.Int32
	conflict  atomic.Bool // app_id 被同一进程里的另一个通道占着
	botFailed atomic.Bool // 取不到本应用机器人的信息
	botID     atomic.Pointer[string]
	closeOnce sync.Once

	// 生命周期：Init 时建好，Close 时取消（监督循环、队列工作协程、进行中的处理、后台提示都收到取消）
	ctx     context.Context
	cancel  context.CancelFunc
	started bool // 监督循环已经起了（c.mu 保护）

	runnerMu sync.Mutex
	runner   wsRunner

	// goMu 保护 stopping 与 wg.Add：Close 开始以后不再起新协程
	goMu     sync.Mutex
	stopping bool
	wg       sync.WaitGroup

	// 退避与重试的时长，0 取默认值（测试里调小）
	backoffBase   time.Duration
	backoffMax    time.Duration
	resetAfter    time.Duration
	clientErrWait time.Duration
	claimRetry    time.Duration
	queueIdle     time.Duration
	noticeTimeout time.Duration
}

// New 新建一个通道实例。不起协程：checkConf 经注册表新建实例只为检查配置。
func New() *Channel {
	return &Channel{
		newAPI:    newLarkAPI,
		newRunner: newLarkRunner,
		now:       time.Now,
		limiters:  newLimiterSet(sendRate, sendBurst),
		queues:    newUserQueues(),
	}
}

// Type 返回通道类型。
func (c *Channel) Type() string { return ChannelType }

// SupportsInbound 恒为 true：私聊或在群里 @ 机器人可以发命令、和 AI 助手对话。
func (c *Channel) SupportsInbound() bool { return true }

// CheckConfig 只检查配置、不发请求：保存配置前调用。ConfigJSON 是明文。
func (c *Channel) CheckConfig(conf *models.NotificationConf) error {
	if conf == nil {
		return errors.New("NotificationConf 为空")
	}
	_, err := parseConfig(conf.ConfigJSON)
	return err
}

// Init 解析明文配置、建好发消息用的 client。不连网，也不起协程（见包注释）。
func (c *Channel) Init(_ context.Context, conf *models.NotificationConf) error {
	if conf == nil {
		return errors.New("NotificationConf 为空")
	}
	cfg, err := parseConfig(conf.ConfigJSON)
	if err != nil {
		return err
	}
	red := redactor{secret: cfg.AppSecret}
	// 在锁外建 client：不在持有 c.mu 时调用外部的构造函数（它拿到的 logger 会回头读通道状态）
	api := c.newAPI(cfg, sdkLogger{log: c.logger, red: red})
	ctx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.cfg, c.confID, c.red, c.api = cfg, conf.ID, red, api
	c.ctx, c.cancel = ctx, cancel
	c.mu.Unlock()
	c.ws.Store(wsWaiting)
	c.inited.Store(true)
	return nil
}

// OnInbound 登记收到消息以后交给谁（MessageChain.Process）。第一次传入非空的 handler 时才起监督协程去连长连接
// 并取机器人信息（见包注释）；生产里 initEnabledChannels 在 Init 成功后马上调它，热重载也一样。
func (c *Channel) OnInbound(handler notify.InboundHandler) {
	c.mu.Lock()
	c.handler = handler
	start := handler != nil && c.cfg != nil && !c.started && !c.closed.Load()
	if start {
		c.started = true
	}
	c.mu.Unlock()
	if start {
		c.spawn(c.supervise)
		c.spawn(c.loadBotInfo)
	}
}

// Healthy 报告通道能不能正常工作：已经初始化、没有关闭、长连接没有报错、app_id 没有被别的通道占着、
// 取得到机器人信息。为 false 时界面显示「异常」。
func (c *Channel) Healthy() bool {
	return c.inited.Load() && !c.closed.Load() && c.ws.Load() != wsFailed &&
		!c.conflict.Load() && !c.botFailed.Load()
}

// LinkState 报告长连接是不是真的连上了：连上以后是 connected，其余时候是 waiting。
func (c *Channel) LinkState() string {
	if c.ws.Load() == wsConnected && !c.closed.Load() {
		return notify.LinkConnected
	}
	return notify.LinkWaiting
}

func (c *Channel) setWS(state int32) {
	if !c.closed.Load() {
		c.ws.Store(state)
	}
}

// Close 只执行一次：标记关闭，取消生命周期，释放 app_id 登记（热重载时新实例马上能接手），
// 再等 SDK 客户端与自己的协程退出。ctx 到期就返回 ctx.Err()，不卡关机。
func (c *Channel) Close(ctx context.Context) error {
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		c.goMu.Lock()
		c.stopping = true
		c.goMu.Unlock()
		c.mu.RLock()
		cancel, cfg := c.cancel, c.cfg
		c.mu.RUnlock()
		if cancel != nil {
			cancel()
		}
		if cfg != nil {
			apps.release(cfg.AppID, c)
		}
	})
	if r := c.currentRunner(); r != nil {
		if err := r.CloseAndWait(ctx); err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
	}
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Channel) logger() *zap.SugaredLogger {
	if l := c.log.Load(); l != nil {
		return l
	}
	return sLogger()
}

// sdkLogger 是交给 SDK 的 logger：Debug 丢掉，其余脱敏后转给通道的 logger。
func (c *Channel) sdkLogger() larkcore.Logger {
	c.mu.RLock()
	red := c.red
	c.mu.RUnlock()
	return sdkLogger{log: c.logger, red: red}
}

func init() {
	notify.RegisterChannel(ChannelType, func() notify.Channel { return New() })
}
