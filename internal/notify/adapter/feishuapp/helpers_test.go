package feishuapp

import (
	"context"
	"sync"
	"testing"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/sunerpy/pt-tools/models"
)

const testConfig = `{"app_id":"cli_test","app_secret":"sec-ret-value","admin_users":["ou_admin"],"allowed_users":["ou_user"]}`

// sentText 是假 API 收到的一次发消息请求。
type sentText struct {
	typ, id, content string
}

// fakeAPI 记下发出的消息，按 results 的顺序回结果（用完以后一律成功）。
type fakeAPI struct {
	mu        sync.Mutex
	sends     []sentText
	results   []sendResult
	errs      []error
	bot       botInfo
	botResult sendResult
	botErr    error
	botCalls  int
	botFails  int // 前 botFails 次 BotInfo 返回 botErr
	block     chan struct{}
}

func (f *fakeAPI) SendText(ctx context.Context, typ, id, content string) (sendResult, error) {
	f.mu.Lock()
	block := f.block
	f.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return sendResult{}, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends = append(f.sends, sentText{typ: typ, id: id, content: content})
	var res sendResult
	var err error
	if len(f.results) > 0 {
		res, f.results = f.results[0], f.results[1:]
	} else {
		res = sendResult{Status: 200}
	}
	if len(f.errs) > 0 {
		err, f.errs = f.errs[0], f.errs[1:]
	}
	return res, err
}

func (f *fakeAPI) BotInfo(context.Context) (botInfo, sendResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.botCalls++
	if f.botErr != nil && (f.botFails == 0 || f.botCalls <= f.botFails) {
		return botInfo{}, f.botResult, f.botErr
	}
	return f.bot, sendResult{Status: 200}, nil
}

func (f *fakeAPI) sent() []sentText {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sentText(nil), f.sends...)
}

func (f *fakeAPI) botCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.botCalls
}

// newTestChannel 用假 API 与假长连接建一个通道并 Init。
func newTestChannel(t *testing.T, raw string) (*Channel, *fakeAPI, *fakeRunners) {
	t.Helper()
	resetProcessState(t)
	api := &fakeAPI{bot: botInfo{OpenID: "ou_bot", AppName: "pt-tools"}}
	runners := &fakeRunners{}
	c := New()
	c.newAPI = func(*Config, larkcore.Logger) apiSender { return api }
	c.newRunner = runners.factory
	shortenTimings(c)
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ID: 7, ChannelType: ChannelType, ConfigJSON: raw}))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.Close(ctx)
	})
	return c, api, runners
}

// shortenTimings 把退避、重试与空闲时长调小，测试不用等真的几分钟。
func shortenTimings(c *Channel) {
	c.backoffBase = 5 * time.Millisecond
	c.backoffMax = 20 * time.Millisecond
	c.resetAfter = time.Hour
	c.clientErrWait = 30 * time.Millisecond
	c.claimRetry = 10 * time.Millisecond
	c.queueIdle = 50 * time.Millisecond
	c.noticeTimeout = time.Second
}

// observedLogger 换上一个记录日志的 logger，返回读日志的入口。
func observedLogger(c *Channel) *observer.ObservedLogs {
	core, logs := observer.New(zap.DebugLevel)
	c.log.Store(zap.New(core).Sugar())
	return logs
}

// resetProcessState 清空进程级的状态（去重表、app_id 登记、名单外提示的节流），测试结束后再清一次。
func resetProcessState(t *testing.T) {
	t.Helper()
	resetForTest()
	t.Cleanup(resetForTest)
}

// fakeRunner 是假长连接：记下工厂传进来的回调，Start 阻塞到 ctx 结束或 Fail。
type fakeRunner struct {
	hooks   wsHooks
	started chan struct{}
	failCh  chan error
	stuck   bool // Start 不理会 ctx，模拟卡住的 SDK
	closed  chan struct{}
	once    sync.Once
}

func (r *fakeRunner) Start(ctx context.Context) error {
	close(r.started)
	if r.stuck {
		<-r.closed
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-r.failCh:
		return err
	}
}

func (r *fakeRunner) CloseAndWait(ctx context.Context) error {
	if r.stuck {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.closed:
			return nil
		}
	}
	r.release()
	return nil
}

// release 放开卡住的 runner（测试收尾用）。
func (r *fakeRunner) release() { r.once.Do(func() { close(r.closed) }) }

// Emit 像 SDK 一样调收消息的回调，返回回调的结果（nil 时 SDK 回 200，否则回 500 让平台重推）。
func (r *fakeRunner) Emit(ctx context.Context, ev *larkim.P2MessageReceiveV1) error {
	return r.hooks.onMessage(ctx, ev)
}

// Ready 模拟连上；Fail 让 Start 返回 err。
func (r *fakeRunner) Ready()         { r.hooks.onReady() }
func (r *fakeRunner) Fail(err error) { r.failCh <- err }

// fakeRunners 是假长连接的工厂，记下每次新建的 runner。
type fakeRunners struct {
	mu      sync.Mutex
	runners []*fakeRunner
	stuck   bool
}

func (f *fakeRunners) factory(_ *Config, hooks wsHooks, _ larkcore.Logger) wsRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := &fakeRunner{hooks: hooks, started: make(chan struct{}), failCh: make(chan error, 1), stuck: f.stuck, closed: make(chan struct{})}
	f.runners = append(f.runners, r)
	return r
}

func (f *fakeRunners) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.runners)
}

func (f *fakeRunners) last() *fakeRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.runners) == 0 {
		return nil
	}
	return f.runners[len(f.runners)-1]
}

// waitRunner 等第 n 个 runner（从 1 数）开始 Start。
func (f *fakeRunners) waitRunner(t *testing.T, n int) *fakeRunner {
	t.Helper()
	require.Eventually(t, func() bool { return f.count() >= n }, 5*time.Second, time.Millisecond)
	f.mu.Lock()
	r := f.runners[n-1]
	f.mu.Unlock()
	select {
	case <-r.started:
	case <-time.After(5 * time.Second):
		t.Fatalf("第 %d 个 runner 没有 Start", n)
	}
	return r
}
