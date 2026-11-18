package feishuapp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/sunerpy/pt-tools/internal/notify"
)

func TestSupervisor_StartsOnceOnFirstInbound(t *testing.T) {
	c, _, runners := newTestChannel(t, testConfig)
	c.OnInbound(nil)
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 0, runners.count(), "handler 为空时不连")

	rec := &chainRecorder{}
	c.OnInbound(rec.handle)
	c.OnInbound(rec.handle)
	runners.waitRunner(t, 1)
	time.Sleep(30 * time.Millisecond)
	assert.Equal(t, 1, runners.count(), "只起一次监督循环")
}

func TestSupervisor_HealthFollowsCallbacks(t *testing.T) {
	c, _, runners := newTestChannel(t, testConfig)
	c.OnInbound((&chainRecorder{}).handle)
	r := runners.waitRunner(t, 1)
	assert.True(t, c.Healthy())
	assert.Equal(t, notify.LinkWaiting, c.LinkState(), "还没连上")

	r.Ready()
	assert.Equal(t, notify.LinkConnected, c.LinkState())
	r.hooks.onDisconnected()
	assert.Equal(t, notify.LinkWaiting, c.LinkState())
	assert.True(t, c.Healthy())
	r.hooks.onReconnecting()
	assert.True(t, c.Healthy())
	r.hooks.onError(errors.New("dial tcp: i/o timeout"))
	assert.False(t, c.Healthy(), "可重试的连接失败也标异常")
	r.hooks.onReconnected()
	assert.True(t, c.Healthy())
	assert.Equal(t, notify.LinkConnected, c.LinkState())
}

// 连上过的客户端停下以后不能再 Start（SDK 把它标成终态）：下一轮新建。
func TestSupervisor_RestartsWithNewRunner(t *testing.T) {
	c, _, runners := newTestChannel(t, testConfig)
	c.OnInbound((&chainRecorder{}).handle)
	r1 := runners.waitRunner(t, 1)
	r1.Ready()
	r1.Fail(errors.New("unable to connect to server after 3 retries"))
	r2 := runners.waitRunner(t, 2)
	assert.NotSame(t, r1, r2)
	assert.Equal(t, 1, r1.startCount(), "连上过的不再 Start")
	require.Eventually(t, func() bool { return !c.Healthy() }, time.Second, time.Millisecond, "重连以前标异常")
	r2.Ready()
	assert.True(t, c.Healthy())

	r2.Fail(nil) // Start 在 ctx 还活着时返回 nil 也算意外结束
	runners.waitRunner(t, 3)
	assert.Equal(t, 1, r2.startCount())
}

// 不可重试的错误（凭据不对、握手被拒、超过连接数）固定等 10 分钟（测试里调成 300ms）。
// 这一轮没连上过：下一轮还用同一个客户端。
func TestSupervisor_ClientErrorWaitsLonger(t *testing.T) {
	c, _, runners := newTestChannel(t, testConfig)
	c.clientErrWait = 300 * time.Millisecond
	c.OnInbound((&chainRecorder{}).handle)
	r1 := runners.waitRunner(t, 1)
	failedAt := time.Now()
	r1.Fail(larkws.NewClientError(1000040346, "app_id is invalid"))
	r1.waitStart(t, 2)
	assert.GreaterOrEqual(t, time.Since(failedAt), 300*time.Millisecond)
	assert.Equal(t, 1, runners.count())
}

// SDK 每新建一个客户端会留下一个停不掉的定时协程（ws/client.go 的 larkcache.New）：凭据错误、网络不通这类
// 没连上过的失败，下一轮用同一个客户端再 Start，不新建。
func TestSupervisor_ReusesNeverConnectedRunner(t *testing.T) {
	c, _, runners := newTestChannel(t, testConfig)
	c.OnInbound((&chainRecorder{}).handle)
	r := runners.waitRunner(t, 1)
	for i := 1; i <= 3; i++ {
		r.waitStart(t, i)
		if i%2 == 1 {
			r.Fail(larkws.NewClientError(1000040346, "app_id is invalid"))
		} else {
			r.Fail(errors.New("dial tcp: connection refused"))
		}
	}
	r.waitStart(t, 4)
	assert.Equal(t, 1, runners.count(), "一直没连上过，只建一个客户端")
	assert.False(t, c.Healthy())
}

// 复用的客户端被 SDK 判成连上过（记下了 everConnected，连接马上失效所以没调 onReady）：
// 再 Start 回「cannot be restarted」，丢掉它、马上新建一个重试。
func TestSupervisor_ReplacesTerminalRunner(t *testing.T) {
	c, _, runners := newTestChannel(t, testConfig)
	// ClientError 以后等 clientErrWait（测试里 30ms）再复用；换成新客户端以后马上 Start，不该再等退避（这里设成一小时）
	c.backoffBase = time.Hour
	c.backoffMax = time.Hour
	c.OnInbound((&chainRecorder{}).handle)
	r1 := runners.waitRunner(t, 1)
	r1.connectSilently()
	r1.Fail(larkws.NewClientError(1000040350, "exceed connection limit"))
	r2 := runners.waitRunner(t, 2)
	assert.Equal(t, 2, r1.startCount(), "先复用一次，回「cannot be restarted」")
	assert.NotSame(t, r1, r2)
	r2.Ready()
	assert.True(t, c.Healthy())
}

// 只对复用的客户端这样做：新建的客户端第一次 Start 就回「cannot be restarted」时照常退避，不空转。
func TestSupervisor_FreshTerminalRunnerBacksOff(t *testing.T) {
	c, _, runners := newTestChannel(t, testConfig)
	runners.terminalFromStart = true
	c.backoffBase = 200 * time.Millisecond
	c.backoffMax = 200 * time.Millisecond
	c.OnInbound((&chainRecorder{}).handle)
	r1 := runners.waitRunner(t, 1)
	start := time.Now()
	r1.waitStart(t, 2)
	assert.GreaterOrEqual(t, time.Since(start), 150*time.Millisecond, "复用前照常退避")
	runners.waitRunner(t, 2)
	time.Sleep(50 * time.Millisecond)
	assert.LessOrEqual(t, runners.count(), 3, "不空转")
}

// isTerminalClient 认的是 SDK 原话：真的 larkws.Client 因 ctx 停下以后再 Start。
func TestIsTerminalClient_MatchesSDK(t *testing.T) {
	cli := larkws.NewClient("cli_terminal_probe", "s", larkws.WithLogger(sdkLogger{log: func() *zap.SugaredLogger { return zap.NewNop().Sugar() }}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Error(t, cli.Start(ctx))
	err := cli.Start(context.Background())
	require.Error(t, err)
	assert.True(t, isTerminalClient(err), err.Error())
	assert.False(t, isTerminalClient(larkws.NewClientError(1000040346, "app_id is invalid")))
	assert.False(t, isTerminalClient(nil))
}

func TestSupervisor_BackoffDoublesAndResets(t *testing.T) {
	c := New()
	c.backoffBase, c.backoffMax, c.resetAfter = time.Second, 4*time.Second, time.Minute
	wait := c.backoffBase
	delay, wait := c.nextDelay(errors.New("x"), time.Second, wait)
	assert.Equal(t, time.Second, delay)
	delay, wait = c.nextDelay(errors.New("x"), time.Second, wait)
	assert.Equal(t, 2*time.Second, delay)
	delay, wait = c.nextDelay(errors.New("x"), time.Second, wait)
	assert.Equal(t, 4*time.Second, delay)
	delay, wait = c.nextDelay(errors.New("x"), time.Second, wait)
	assert.Equal(t, 4*time.Second, delay, "最长 backoffMax")
	delay, _ = c.nextDelay(errors.New("x"), 2*time.Minute, wait)
	assert.Equal(t, time.Second, delay, "上一轮跑得够久就从头算")
	c.clientErrWait = 10 * time.Minute
	delay, _ = c.nextDelay(larkws.NewClientError(403, "forbidden"), time.Second, wait)
	assert.Equal(t, 10*time.Minute, delay)

	for range 50 {
		j := jitter(time.Second)
		assert.GreaterOrEqual(t, j, time.Second)
		assert.LessOrEqual(t, j, 1500*time.Millisecond)
	}
	assert.Equal(t, time.Duration(0), jitter(0))
}

func TestDurationText(t *testing.T) {
	assert.Equal(t, "2 分钟", durationText(defaultLeftoverTTL))
	assert.Equal(t, "200ms", durationText(200*time.Millisecond))
}

func TestSupervisor_DefaultTimings(t *testing.T) {
	c := New()
	assert.Equal(t, 5*time.Second, c.timing(c.backoffBase, defaultBackoffBase))
	assert.Equal(t, 5*time.Minute, defaultBackoffMax)
	assert.Equal(t, 10*time.Minute, defaultClientErrWait)
	assert.Equal(t, 10*time.Minute, defaultResetAfter)
	assert.Equal(t, time.Minute, defaultClaimRetry)
	assert.Equal(t, time.Minute, defaultQueueIdle)
	assert.Equal(t, 200*time.Millisecond, c.timing(c.gather, defaultGather))
	assert.Equal(t, 2*time.Minute, defaultLeftoverTTL)
}

// runner 卡住（不理会 ctx、CloseAndWait 也不返回）时，Close 按 ctx 返回，不卡关机。
func TestClose_StuckRunnerReturnsOnDeadline(t *testing.T) {
	c, _, runners := newTestChannel(t, testConfig)
	runners.stuck = true
	c.OnInbound((&chainRecorder{}).handle)
	r := runners.waitRunner(t, 1)
	defer r.release()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := c.Close(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), time.Second)
}

func TestClose_StopsSupervisor(t *testing.T) {
	c, _, runners := newTestChannel(t, testConfig)
	c.OnInbound((&chainRecorder{}).handle)
	runners.waitRunner(t, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, c.Close(ctx))
	assert.False(t, c.Healthy())
	c.OnInbound((&chainRecorder{}).handle)
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 1, runners.count(), "关闭以后不再连")
}

// 同一进程里两条配置用同一个应用：第二条不连长连接、标异常，第一条关掉以后接手。
func TestSupervisor_AppIDTakenByAnotherChannel(t *testing.T) {
	first, _, runners1 := newTestChannel(t, testConfig)
	first.OnInbound((&chainRecorder{}).handle)
	runners1.waitRunner(t, 1)

	second := New()
	api2 := &fakeAPI{bot: botInfo{OpenID: "ou_bot"}}
	runners2 := &fakeRunners{}
	second.newAPI = func(*Config, larkcore.Logger) apiSender { return api2 }
	second.newRunner = runners2.factory
	shortenTimings(second)
	logs := observedLogger(second)
	require.NoError(t, second.Init(context.Background(), confOf(8, testConfig)))
	defer func() { _ = second.Close(context.Background()) }()
	second.OnInbound((&chainRecorder{}).handle)

	require.Eventually(t, func() bool { return !second.Healthy() }, 5*time.Second, time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	assert.Equal(t, 0, runners2.count(), "不连长连接")
	assert.True(t, strings.Contains(logJoined(logs.All()), "已被通道 #7 使用"))

	require.NoError(t, first.Close(context.Background()))
	runners2.waitRunner(t, 1)
	require.Eventually(t, second.Healthy, 5*time.Second, time.Millisecond, "第一条关掉以后接手")
}

func TestBotInfo_RetriesUntilSuccess(t *testing.T) {
	c, api, runners := newTestChannel(t, testConfig)
	api.botErr = errors.New("获取机器人信息失败: code=99991663, msg=invalid token t-g1044ghJRUIJJ5ELPZNLNRIHCAAXQ1")
	api.botFails = 2
	logs := observedLogger(c)
	c.OnInbound((&chainRecorder{}).handle)
	runners.waitRunner(t, 1).Ready()
	require.Eventually(t, func() bool { return c.botOpenID() == "ou_bot" }, 5*time.Second, time.Millisecond)
	assert.Equal(t, 3, api.botCallCount())
	assert.True(t, c.Healthy(), "取到以后恢复")
	joined := logJoined(logs.All())
	assert.Contains(t, joined, "99991663")
	assert.NotContains(t, joined, "t-g1044ghJRUIJJ5ELPZNLNRIHCAAXQ1")
}

// competitor 建一个用同一个应用、通道编号 confID 的实例并接上 chain（假 API、假长连接）。
func competitor(t *testing.T, confID uint, setup func(*Channel)) (*Channel, *fakeRunners, *chainRecorder) {
	t.Helper()
	c := New()
	api := &fakeAPI{bot: botInfo{OpenID: "ou_bot"}}
	runners := &fakeRunners{}
	c.newAPI = func(*Config, larkcore.Logger) apiSender { return api }
	c.newRunner = runners.factory
	shortenTimings(c)
	if setup != nil {
		setup(c)
	}
	require.NoError(t, c.Init(context.Background(), confOf(confID, testConfig)))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.Close(ctx)
	})
	rec := &chainRecorder{}
	c.OnInbound(rec.handle)
	return c, runners, rec
}

// 旧连接卡着没断开（Close 已经超时返回）时登记仍然占着：另一个通道重试多少次都拿不到，
// 等旧连接真正结束才释放，同一个应用任何时候只有一条连接。
func TestSupervisor_ReleaseWaitsForOldConnection(t *testing.T) {
	first, _, runners1 := newTestChannel(t, testConfig)
	runners1.stuck = true
	first.OnInbound((&chainRecorder{}).handle)
	stuck := runners1.waitRunner(t, 1)
	defer stuck.release()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, first.Close(ctx), context.DeadlineExceeded)

	second, runners2, _ := competitor(t, 8, nil) // claimRetry 10ms：等的这段时间里重试了很多次
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, 0, runners2.count(), "旧连接还没断开，不能连")
	assert.False(t, second.Healthy(), "被别的通道占着时标异常")

	stuck.release()
	runners2.waitRunner(t, 1)
	require.Eventually(t, second.Healthy, 5*time.Second, time.Millisecond)
}

// 等登记的实例在释放的那一刻被叫醒，不等定时重试（这里设成一小时）。
func TestSupervisor_WaiterWakesOnRelease(t *testing.T) {
	first, _, runners1 := newTestChannel(t, testConfig)
	first.OnInbound((&chainRecorder{}).handle)
	runners1.waitRunner(t, 1)

	second, runners2, _ := competitor(t, 8, func(c *Channel) { c.claimRetry = time.Hour })
	require.Eventually(t, func() bool { return !second.Healthy() }, 5*time.Second, time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	require.Equal(t, 0, runners2.count())

	require.NoError(t, first.Close(context.Background()))
	start := time.Now()
	runners2.waitRunner(t, 1)
	assert.Less(t, time.Since(start), 2*time.Second, "释放以后马上接手")
}

// 热重载时旧实例 Close 超时（旧连接卡着）：同一个通道的新实例不标异常，等旧连接真正断开的那一刻登记，
// 同时接手排着队的消息。
func TestReload_CloseTimeoutHandsOffWhenOldConnectionEnds(t *testing.T) {
	c1, _, runners1 := newTestChannel(t, testConfig)
	runners1.stuck = true
	rec1 := &chainRecorder{block: make(chan struct{})}
	c1.OnInbound(rec1.handle)
	stuck := runners1.waitRunner(t, 1)
	defer stuck.release()
	require.NoError(t, stuck.Emit(context.Background(), msgEvent("om_t1", "ou_admin", "/status")))
	require.Eventually(t, func() bool { return len(rec1.got()) == 1 }, 5*time.Second, time.Millisecond)
	require.NoError(t, stuck.Emit(context.Background(), msgEvent("om_t2", "ou_admin", "/sites")))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, c1.Close(ctx), context.DeadlineExceeded)

	var logs *observer.ObservedLogs
	c2, runners2, rec2 := competitor(t, 7, func(c *Channel) {
		c.claimRetry = time.Hour
		logs = observedLogger(c)
	})
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 0, runners2.count(), "旧连接还没断开")
	assert.True(t, c2.Healthy(), "等的是自己的上一个实例，不算异常")
	assert.Empty(t, rec2.got())
	assert.Contains(t, logJoined(logs.All()), "上一个实例的长连接还没断开")

	stuck.release()
	runners2.waitRunner(t, 1)
	require.Eventually(t, func() bool { return len(rec2.got()) == 1 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, "/sites", rec2.got()[0].Text)
	assert.Len(t, rec1.got(), 1)
}
