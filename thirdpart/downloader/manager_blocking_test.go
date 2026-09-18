package downloader

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 这一组测试锁定「下载器连不上时不得堵塞其它调用方」这条不变量。
//
// 回归背景：GetDownloader 曾把 createWithRetry（最长 31s 退避 + 每次尝试的连接
// 超时）跑在 dm.mu 的写锁里。实测一个连不上的下载器会让每个排队的调用方多等
// 61s 且无上界，前端 30s 一次的全局轮询把浏览器连接池占满，整个 Web UI 无法
// 加载任何数据。

// blockingFactory 模拟一个连不上的下载器：每次建连都卡住直到 release 关闭或超时。
type blockingFactory struct {
	calls    atomic.Int32
	release  chan struct{}
	failWith error
}

func newBlockingFactory() *blockingFactory {
	return &blockingFactory{
		release:  make(chan struct{}),
		failWith: errors.New("连接超时，请检查网络是否可达"),
	}
}

func (f *blockingFactory) factory(config DownloaderConfig, name string) (Downloader, error) {
	f.calls.Add(1)
	select {
	case <-f.release:
	case <-time.After(30 * time.Second):
	}
	return nil, f.failWith
}

func managerWithFactory(t *testing.T, name string, factory DownloaderFactory) *DownloaderManager {
	t.Helper()
	// 退避设成 0，测试只关心阻塞语义，不关心真实等待时长
	dm := NewDownloaderManagerWithConfig(ReconnectConfig{
		MaxRetries:     5,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1.0,
	})
	dm.RegisterFactory(DownloaderQBittorrent, factory)
	require.NoError(t, dm.RegisterConfig(name, &MockConfig{
		Type: DownloaderQBittorrent,
		URL:  "http://10.255.255.1:40909",
	}, true))
	return dm
}

// TestGetDownloaderContextRespectsDeadline 建连卡住时，调用方按自己的预算返回，
// 而不是被拖进完整的退避序列。
func TestGetDownloaderContextRespectsDeadline(t *testing.T) {
	f := newBlockingFactory()
	defer close(f.release)
	dm := managerWithFactory(t, "dead", f.factory)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := dm.GetDownloaderContext(ctx, "dead")
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded,
		"超预算返回的错误要能被 errors.Is 认出是 deadline，调用方才能和「连不上」区分开")
	assert.Less(t, elapsed, 5*time.Second, "调用方不应等待超过自己的预算，实际 %v", elapsed)
}

// TestGetDownloaderDoesNotBlockOtherNames 一个连不上的下载器不得影响另一个
// 健康下载器的获取 —— 这正是旧代码用一把全局写锁造成的串行化。
func TestGetDownloaderDoesNotBlockOtherNames(t *testing.T) {
	f := newBlockingFactory()
	defer close(f.release)

	dm := NewDownloaderManagerWithConfig(ReconnectConfig{
		MaxRetries:     5,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1.0,
	})
	dm.RegisterFactory(DownloaderQBittorrent, f.factory)
	dm.RegisterFactory(DownloaderTransmission, MockDownloaderFactory)
	require.NoError(t, dm.RegisterConfig("dead",
		&MockConfig{Type: DownloaderQBittorrent, URL: "http://10.255.255.1:40909"}, false))
	require.NoError(t, dm.RegisterConfig("alive",
		&MockConfig{Type: DownloaderTransmission, URL: "http://127.0.0.1:9091"}, true))

	// 先让 dead 卡在建连里
	deadDone := make(chan struct{})
	go func() {
		defer close(deadDone)
		_, _ = dm.GetDownloader("dead")
	}()
	waitFor(t, func() bool { return f.calls.Load() > 0 }, "dead 的工厂应已被调用")

	start := time.Now()
	dl, err := dm.GetDownloader("alive")
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, dl)
	assert.Less(t, elapsed, time.Second, "健康下载器不应被另一个下载器的建连拖住，实际 %v", elapsed)
}

// TestGetDownloaderSingleFlightPerName 同名并发调用共享一次建连尝试，
// 不是每个调用方各跑一遍完整退避序列。
func TestGetDownloaderSingleFlightPerName(t *testing.T) {
	f := newBlockingFactory()
	dm := managerWithFactory(t, "dead", f.factory)

	const callers = 8
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	errs := make([]error, callers)
	start := time.Now()
	for i := range callers {
		wg.Go(func() {
			_, errs[i] = dm.GetDownloaderContext(ctx, "dead")
		})
	}
	wg.Wait()
	elapsed := time.Since(start)
	// 先取样再放行：放行后后台那次尝试会继续跑退避序列，计数还会涨
	callsWhileWaiting := f.calls.Load()
	close(f.release)

	for i, err := range errs {
		require.Error(t, err, "第 %d 个调用方应当拿到错误", i)
	}
	// 关键断言：8 个调用方总耗时接近单个预算，而不是 8 倍 —— 旧实现是严格串行的
	assert.Less(t, elapsed, 3*time.Second, "同名并发调用不应线性累加，实际 %v", elapsed)
	assert.Equal(t, int32(1), callsWhileWaiting,
		"单飞应把并发建连收敛到一次，实际调用工厂 %d 次", callsWhileWaiting)
}

// TestGetDownloaderHealthyFastPathIsConcurrent 已建好的健康实例走只读快路径，
// 不受慢路径闸门影响。
func TestGetDownloaderHealthyFastPathIsConcurrent(t *testing.T) {
	dm := managerWithFactory(t, "alive", MockDownloaderFactory)
	_, err := dm.GetDownloader("alive")
	require.NoError(t, err)

	var wg sync.WaitGroup
	start := time.Now()
	for range 64 {
		wg.Go(func() {
			dl, err := dm.GetDownloader("alive")
			assert.NoError(t, err)
			assert.NotNil(t, dl)
		})
	}
	wg.Wait()
	assert.Less(t, time.Since(start), time.Second, "快路径不应有可观测的争用")
}

// TestReconnectDownloaderContextRespectsDeadline 显式重连同样受 ctx 约束，
// 且和普通获取共享同一道闸门。
func TestReconnectDownloaderContextRespectsDeadline(t *testing.T) {
	f := newBlockingFactory()
	defer close(f.release)
	dm := managerWithFactory(t, "dead", f.factory)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := dm.ReconnectDownloaderContext(ctx, "dead")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 5*time.Second)
}

// TestGetDownloaderReturnsBeforeBackoffCompletes 调用方按自己的预算返回，不等退避跑完。
//
// 工厂本身不接受 ctx（见 DownloaderFactory 签名），所以无法取消一次正在进行的建连；
// 能做的是不去等它。这里的退避首跳是 2s，而调用方只有 200ms 预算：它必须先返回，
// 后台那次尝试则照原策略继续跑完并缓存结果。
func TestGetDownloaderReturnsBeforeBackoffCompletes(t *testing.T) {
	var calls atomic.Int32
	dm := NewDownloaderManagerWithConfig(ReconnectConfig{
		MaxRetries:     5,
		InitialBackoff: 2 * time.Second,
		MaxBackoff:     30 * time.Second,
		Multiplier:     2.0,
	})
	dm.RegisterFactory(DownloaderQBittorrent, func(config DownloaderConfig, name string) (Downloader, error) {
		calls.Add(1)
		return nil, fmt.Errorf("连接被拒绝")
	})
	require.NoError(t, dm.RegisterConfig("dead",
		&MockConfig{Type: DownloaderQBittorrent, URL: "http://10.255.255.1:40909"}, true))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := dm.GetDownloaderContext(ctx, "dead")
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, elapsed, 2*time.Second, "第一次退避是 2s，超预算应立即返回，实际 %v", elapsed)
	assert.GreaterOrEqual(t, calls.Load(), int32(1), "应当已经发起过一次建连")
}

// TestFailureCooldownShortCircuits 创建失败后进入冷却期，期间请求直接复用上次错误，
// 不再制造新的建连 —— 否则前端 30s 一次的轮询会让一个死掉的下载器持续刷退避日志。
func TestFailureCooldownShortCircuits(t *testing.T) {
	var calls atomic.Int32
	dm := NewDownloaderManagerWithConfig(ReconnectConfig{
		MaxRetries:     1,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1.0,
	})
	dm.RegisterFactory(DownloaderQBittorrent, func(config DownloaderConfig, name string) (Downloader, error) {
		calls.Add(1)
		return nil, fmt.Errorf("连接被拒绝")
	})
	require.NoError(t, dm.RegisterConfig("dead",
		&MockConfig{Type: DownloaderQBittorrent, URL: "http://10.255.255.1:40909"}, true))

	_, err := dm.GetDownloader("dead")
	require.Error(t, err)
	first := calls.Load()
	require.Positive(t, first)

	// 冷却期内的后续请求立即失败，且不再调用工厂
	for range 5 {
		_, err = dm.GetDownloader("dead")
		require.Error(t, err)
	}
	assert.Equal(t, first, calls.Load(), "冷却期内不应再发起建连")

	// 显式重连要能穿透冷却：用户点重连就是要立刻重试
	require.Error(t, dm.ReconnectDownloader("dead"))
	assert.Greater(t, calls.Load(), first, "ReconnectDownloader 应清掉冷却并重新尝试")

	// 从 DB 同步同样清冷却：用户改完地址保存后不该再等一分钟
	before := calls.Load()
	dm.SyncFromDB([]DownloaderDBRecord{{
		Name: "dead", Type: DownloaderQBittorrent, URL: "http://10.255.255.1:40909",
		IsDefault: true, Enabled: true,
	}})
	_, err = dm.GetDownloader("dead")
	require.Error(t, err)
	assert.Greater(t, calls.Load(), before, "SyncFromDB 应清掉冷却")
}

// TestGetDownloaderRecreatesUnhealthyInstance 重建路径的行为不变：
// 实例不健康且 Ping 失败时关闭旧实例并重建。
func TestGetDownloaderRecreatesUnhealthyInstance(t *testing.T) {
	var created atomic.Int32
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, func(config DownloaderConfig, name string) (Downloader, error) {
		created.Add(1)
		return &MockDownloader{name: name, dlType: config.GetType(), healthy: true}, nil
	})
	require.NoError(t, dm.RegisterConfig("qbit",
		&MockConfig{Type: DownloaderQBittorrent, URL: "http://127.0.0.1:8080"}, true))

	first, err := dm.GetDownloader("qbit")
	require.NoError(t, err)
	require.Equal(t, int32(1), created.Load())

	// 让实例变成不健康；MockDownloader.Ping 返回失败，于是必须重建
	require.NoError(t, first.Close())
	require.False(t, first.IsHealthy())

	second, err := dm.GetDownloader("qbit")
	require.NoError(t, err)
	assert.Equal(t, int32(2), created.Load(), "不健康的实例应被重建")
	assert.NotSame(t, first, second)
	assert.Equal(t, 0, dm.GetErrorCount("qbit"))
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("等待条件超时: %s", msg)
}

// ---------------------------------------------------------------------------
// 配置代次：建连期间配置被改掉，旧结果不得入表
//
// 回归背景：把建连移出 dm.mu 之后出现了一个新窗口 —— 退避序列最长约 31s，用户
// 完全来得及在这期间改完 URL/凭据点保存。旧实现会无条件把这次建连的结果登记进
// 实例表，于是暂停、删除、加种等操作被发往用户已经改掉或删掉的旧端点。
// ---------------------------------------------------------------------------

// gatedFactory 建连时先等一个信号，用来把「配置变更」精确插进建连中间。
type gatedFactory struct {
	entered chan string // 每次进入工厂时送出它看到的 URL
	release chan struct{}
	built   atomic.Int32
	closed  atomic.Int32
}

func newGatedFactory() *gatedFactory {
	return &gatedFactory{entered: make(chan string, 8), release: make(chan struct{})}
}

func (f *gatedFactory) factory(config DownloaderConfig, name string) (Downloader, error) {
	f.entered <- config.GetURL()
	<-f.release
	f.built.Add(1)
	return &countingDownloader{MockDownloader: MockDownloader{name: name, dlType: config.GetType(), healthy: true}, url: config.GetURL(), closed: &f.closed}, nil
}

type countingDownloader struct {
	MockDownloader
	url    string
	closed *atomic.Int32
}

func (d *countingDownloader) Close() error {
	d.closed.Add(1)
	return d.MockDownloader.Close()
}

// nextURL 取下一次进入工厂时看到的 URL。有界等待：缺了这一道，
// 代次检查一旦失效，测试会挂死到整个包超时而不是给出可读的失败。
func (f *gatedFactory) nextURL(t *testing.T) string {
	t.Helper()
	select {
	case u := <-f.entered:
		return u
	case <-time.After(3 * time.Second):
		t.Fatal("等待下一次建连超时：过期配置的结果很可能被直接接受了，没有按新配置重试")
		return ""
	}
}

func gatedManager(t *testing.T, f *gatedFactory, url string) *DownloaderManager {
	t.Helper()
	dm := NewDownloaderManagerWithConfig(ReconnectConfig{
		MaxRetries:     0,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1.0,
	})
	dm.RegisterFactory(DownloaderQBittorrent, f.factory)
	require.NoError(t, dm.RegisterConfig("qbit",
		&MockConfig{Type: DownloaderQBittorrent, URL: url}, true))
	return dm
}

// TestFlightDiscardsInstanceBuiltFromStaleConfig 建连跑完时配置已变更，
// 这个按旧配置建出来的实例必须被关掉且不入表，调用方随后拿到的是按新配置建的。
func TestFlightDiscardsInstanceBuiltFromStaleConfig(t *testing.T) {
	f := newGatedFactory()
	dm := gatedManager(t, f, "http://old:8080")

	got := make(chan error, 1)
	go func() {
		_, err := dm.GetDownloader("qbit")
		got <- err
	}()

	// 第一次建连已经进到工厂里，拿到的是旧 URL
	assert.Equal(t, "http://old:8080", f.nextURL(t))

	// 就在这个窗口里保存新配置
	require.NoError(t, dm.RegisterConfig("qbit",
		&MockConfig{Type: DownloaderQBittorrent, URL: "http://new:9090"}, true))

	// 放行：第一次建连成功返回，但它依据的配置已经作废
	close(f.release)

	// reviveOrCreate 会按新配置重试，所以第二次进入工厂看到的是新 URL
	assert.Equal(t, "http://new:9090", f.nextURL(t))

	require.NoError(t, waitErr(t, got))

	dl, err := dm.GetDownloader("qbit")
	require.NoError(t, err)
	assert.Equal(t, "http://new:9090", dl.(*countingDownloader).url,
		"实例表里必须是按新配置建的那个")
	assert.Equal(t, int32(1), f.closed.Load(), "按旧配置建出来的实例必须被关掉")
}

// TestStaleInstanceNotServedAfterConfigChange 配置一变，快路径就不能再把手上那个
// 按旧配置建的健康实例交出去 —— 否则暂停/删除/加种会发到旧端点。
func TestStaleInstanceNotServedAfterConfigChange(t *testing.T) {
	var built atomic.Int32
	var closed atomic.Int32
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, func(config DownloaderConfig, name string) (Downloader, error) {
		built.Add(1)
		return &countingDownloader{
			MockDownloader: MockDownloader{name: name, dlType: config.GetType(), healthy: true},
			url:            config.GetURL(),
			closed:         &closed,
		}, nil
	})
	require.NoError(t, dm.RegisterConfig("qbit",
		&MockConfig{Type: DownloaderQBittorrent, URL: "http://old:8080"}, true))

	first, err := dm.GetDownloader("qbit")
	require.NoError(t, err)
	require.Equal(t, "http://old:8080", first.(*countingDownloader).url)
	require.True(t, first.IsHealthy(), "旧实例自报健康，正是会被快路径直接交出去的情形")

	require.NoError(t, dm.RegisterConfig("qbit",
		&MockConfig{Type: DownloaderQBittorrent, URL: "http://new:9090"}, true))

	second, err := dm.GetDownloader("qbit")
	require.NoError(t, err)
	assert.Equal(t, "http://new:9090", second.(*countingDownloader).url,
		"配置变更后必须按新配置重建，不能复用旧实例")
	assert.Equal(t, int32(2), built.Load())
}

// TestRemovedDownloaderRejectsInFlightInstance 删除期间仍在飞的建连即使成功，
// 也不能把实例塞回表里 —— 否则删掉的下载器还能继续收操作。
func TestRemovedDownloaderRejectsInFlightInstance(t *testing.T) {
	f := newGatedFactory()
	dm := gatedManager(t, f, "http://doomed:8080")

	got := make(chan error, 1)
	go func() {
		_, err := dm.GetDownloader("qbit")
		got <- err
	}()
	assert.Equal(t, "http://doomed:8080", f.nextURL(t))

	require.NoError(t, dm.RemoveDownloader("qbit"))
	close(f.release)

	err := waitErr(t, got)
	require.Error(t, err, "配置已被删除，调用方不该拿到一个可用实例")
	assert.Equal(t, int32(1), f.closed.Load(), "飞在半路的实例必须被关掉")

	dm.mu.RLock()
	_, stillThere := dm.downloaders["qbit"]
	dm.mu.RUnlock()
	assert.False(t, stillThere, "实例表里不该留下已删除下载器的实例")
}

// TestSyncFromDBKeepsUnchangedInstances SyncFromDB 每次配置事件都会整表重放，
// 配置没变的下载器不能因此被作废，否则每次保存都会引发一轮无谓的重连。
func TestSyncFromDBKeepsUnchangedInstances(t *testing.T) {
	var built atomic.Int32
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, func(config DownloaderConfig, name string) (Downloader, error) {
		built.Add(1)
		return &MockDownloader{name: name, dlType: config.GetType(), healthy: true}, nil
	})

	rec := DownloaderDBRecord{
		Name: "qbit", Type: DownloaderQBittorrent, URL: "http://same:8080",
		IsDefault: true, Enabled: true,
	}
	dm.SyncFromDB([]DownloaderDBRecord{rec})

	_, err := dm.GetDownloader("qbit")
	require.NoError(t, err)
	require.Equal(t, int32(1), built.Load())

	// 同一份记录再同步两次：实例应当被原样保留
	dm.SyncFromDB([]DownloaderDBRecord{rec})
	dm.SyncFromDB([]DownloaderDBRecord{rec})
	_, err = dm.GetDownloader("qbit")
	require.NoError(t, err)
	assert.Equal(t, int32(1), built.Load(), "配置没变不该重建实例")

	// 改掉 URL 再同步：这次必须重建
	rec.URL = "http://moved:8080"
	dm.SyncFromDB([]DownloaderDBRecord{rec})
	_, err = dm.GetDownloader("qbit")
	require.NoError(t, err)
	assert.Equal(t, int32(2), built.Load(), "配置变了必须重建")
}

// waitErr 有界地取后台调用方的返回值，避免代次检查失效时把测试挂死。
func waitErr(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("等待调用方返回超时")
		return nil
	}
}

// failingGatedFactory 和 gatedFactory 一样能把「配置变更」精确插进建连中间，
// 区别是它**建连失败**。上一版只有成功路径的夹具，于是「旧配置的失败被写进冷却」
// 这条路径一直没被覆盖到。
type failingGatedFactory struct {
	entered chan string
	release chan struct{}
	calls   atomic.Int32
}

func newFailingGatedFactory() *failingGatedFactory {
	return &failingGatedFactory{entered: make(chan string, 8), release: make(chan struct{})}
}

func (f *failingGatedFactory) factory(config DownloaderConfig, name string) (Downloader, error) {
	f.calls.Add(1)
	f.entered <- config.GetURL()
	<-f.release
	return nil, fmt.Errorf("连接被拒绝: %s", config.GetURL())
}

func (f *failingGatedFactory) nextURL(t *testing.T) string {
	t.Helper()
	select {
	case u := <-f.entered:
		return u
	case <-time.After(3 * time.Second):
		t.Fatal("等待下一次建连超时")
		return ""
	}
}

// TestStaleFailureDoesNotCooldownNewConfig 旧配置的建连失败不得进入失败冷却。
//
// 时序：对着坏地址建连 → 用户改成新地址点保存（clearCooldown 生效）→ 旧建连才失败。
// 如果这次失败被原样缓存，接下来 60s 内每个请求都直接吃这个旧错误，
// 用户刚改好的地址一次都不会被尝试。判据就是：新地址必须真的被建连过。
func TestStaleFailureDoesNotCooldownNewConfig(t *testing.T) {
	f := newFailingGatedFactory()
	dm := NewDownloaderManagerWithConfig(ReconnectConfig{
		MaxRetries:     0, // 不退避，让失败立刻发生，时序才好控
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1.0,
	})
	dm.RegisterFactory(DownloaderQBittorrent, f.factory)
	require.NoError(t, dm.RegisterConfig("qbit",
		&MockConfig{Type: DownloaderQBittorrent, URL: "http://old-bad:8080"}, true))

	got := make(chan error, 1)
	go func() {
		_, err := dm.GetDownloader("qbit")
		got <- err
	}()

	// 第一次建连已进入工厂，用的是旧地址
	assert.Equal(t, "http://old-bad:8080", f.nextURL(t))

	// 就在这个窗口里把地址改掉
	require.NoError(t, dm.RegisterConfig("qbit",
		&MockConfig{Type: DownloaderQBittorrent, URL: "http://new-good:9090"}, true))

	// 放行：旧建连此刻失败。它的错误不该被当成「当前配置连不上」
	close(f.release)

	// reviveOrCreate 会按新配置重试，所以工厂必须被新地址再进一次
	assert.Equal(t, "http://new-good:9090", f.nextURL(t),
		"旧配置的失败被写进冷却了：新地址一次都没被尝试")

	// 这次也会失败（工厂恒失败），但关键是它确实尝试过新地址
	require.Error(t, waitErr(t, got))

	// 冷却里现在记的应当是新地址的错误，而不是旧地址的
	_, err := dm.GetDownloader("qbit")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "new-good",
		"冷却里缓存的应该是当前配置的失败，实际是: %v", err)
	assert.NotContains(t, err.Error(), "old-bad")
}

// TestLateCooldownCommitFromStaleFlightIsIgnored 复现「代次检查」和「写冷却」之间那个窗口。
//
// 交错顺序（评审指出的那一条，也是上一版修复没覆盖到的）：
//  1. 旧 flight 建连失败，此刻配置还没变，genIsCurrent 检查通过；
//  2. 用户保存新配置 —— 换代，并清掉失败冷却；
//  3. 旧 flight 这时才把错误提交进冷却，正好落在清空之后。
//
// 两把锁（dm.mu 与 flightsMu）不可能把第 1 步和第 3 步做成原子，反序加锁又会造成锁序反转，
// 所以兜底放在读取侧：冷却带着它依据的代次，读到过期的就丢掉。
// 这里直接调 settleFlight 来精确摆出第 3 步的时机 —— 靠 sleep 去碰这个窗口碰不稳。
func TestLateCooldownCommitFromStaleFlightIsIgnored(t *testing.T) {
	var calls atomic.Int32
	var lastURL atomic.Value
	dm := NewDownloaderManagerWithConfig(ReconnectConfig{
		MaxRetries:     0,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		Multiplier:     1.0,
	})
	dm.RegisterFactory(DownloaderQBittorrent, func(config DownloaderConfig, name string) (Downloader, error) {
		calls.Add(1)
		lastURL.Store(config.GetURL())
		return &MockDownloader{name: name, dlType: config.GetType(), healthy: true}, nil
	})
	require.NoError(t, dm.RegisterConfig("qbit",
		&MockConfig{Type: DownloaderQBittorrent, URL: "http://old:8080"}, true))

	// 第 1 步用到的代次：旧 flight 当时读到的就是这一代
	_, _, staleGen, err := dm.resolveFactory("qbit")
	require.NoError(t, err)

	// 第 2 步：保存新配置（换代 + 清冷却）
	require.NoError(t, dm.RegisterConfig("qbit",
		&MockConfig{Type: DownloaderQBittorrent, URL: "http://new:9090"}, true))

	// 第 3 步：旧 flight 迟到的提交，带的是过期代次
	dm.settleFlight("qbit", nil, staleGen, errors.New("连接被拒绝: http://old:8080"))

	// 判据：这条迟到的冷却不得挡住新配置。请求必须真的去建连，且用的是新地址。
	before := calls.Load()
	dl, err := dm.GetDownloader("qbit")
	require.NoError(t, err, "迟到的过期冷却把新配置挡住了")
	require.NotNil(t, dl)
	assert.Greater(t, calls.Load(), before, "应当真的发起了一次建连")
	assert.Equal(t, "http://new:9090", lastURL.Load(), "建连用的应该是新地址")

	// 同代次的失败照旧生效，别把冷却整个废掉了
	_, _, curGen, err := dm.resolveFactory("qbit")
	require.NoError(t, err)
	dm.settleFlight("qbit", nil, curGen, errors.New("当前配置也连不上"))
	dm.dropCachedInstanceForTest("qbit")
	_, err = dm.GetDownloader("qbit")
	require.Error(t, err, "同代次的失败应当正常进入冷却")
	assert.Contains(t, err.Error(), "当前配置也连不上")
}

// dropCachedInstanceForTest 丢掉已缓存的实例，让下一次获取必须走慢路径。
// 只给测试用：上面那个用例要在「实例已经建好」之后再验冷却是否生效。
func (dm *DownloaderManager) dropCachedInstanceForTest(name string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	delete(dm.downloaders, name)
	delete(dm.instanceGen, name)
}
