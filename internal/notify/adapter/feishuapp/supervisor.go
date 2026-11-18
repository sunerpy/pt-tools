package feishuapp

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"
)

// 监督循环的退避（测试里改小，见 Channel 的字段）。
const (
	defaultBackoffBase = 5 * time.Second
	defaultBackoffMax  = 5 * time.Minute
	// defaultResetAfter：上一轮跑过这么久，就不是连续失败，退避从头算
	defaultResetAfter = 10 * time.Minute
	// defaultClientErrWait：不可重试的错误（凭据不对、握手被拒、超过连接数）固定等这么久；用户改对凭据保存以后会热重载、马上重连
	defaultClientErrWait = 10 * time.Minute
	// defaultClaimRetry：app_id 被另一个通道占着时多久重试一次登记
	defaultClaimRetry = time.Minute
	// defaultHandoffWait：关闭时等正在处理的消息收尾多久再交接（处理函数理会取消时马上就收尾）
	defaultHandoffWait = 10 * time.Second
)

func (c *Channel) timing(v, def time.Duration) time.Duration {
	if v > 0 {
		return v
	}
	return def
}

/*
 * supervise 是长连接的监督循环，参照 Telegram 的 runInbound：
 *  1. 先登记 app_id，并取走同一个通道上一个实例留下的排队消息（热重载交接，见 state.go 的 appRegistry）。
 *     被另一个实例占着时不连：占着的是这个通道的上一个实例（热重载时旧连接还没断开）就等它断开；
 *     是别的通道就标异常。对方释放的那一刻被叫醒，另外每分钟重试一次兜底；
 *  2. Start 阻塞运行，断线重连由 SDK 自己做；
 *  3. Start 只在 ctx 取消、不可重试的错误或服务端下发的重试次数用完时返回：后两种标异常、退避、再 Start；
 *  4. 退出时（ctx 取消，SDK 的 Start 已经返回，旧连接已经断开）交出排队、还没开始处理的消息并释放登记，
 *     这是一步：新实例要么还没登记，要么登记的同时拿到这些消息。所以 Close 超时返回时登记仍然占着，
 *     直到旧连接真正结束。
 *
 * 客户端只在必要时新建。SDK 每新建一个客户端就留下一个停不掉的定时协程（上游缺陷：ws/client.go:227 的
 * larkcache.New(30s) 在 cache/cache.go:53 起协程、:12 靠 finalizer 停，可协程一直引用着 cache，finalizer 不会跑）。
 * SDK 只在 ctx 取消、Close，或者连上过以后失败时把客户端标成终态（ws/client_lifecycle.go:125-128），
 * 所以这一轮没连上过（凭据错误、网络不通）就下一轮接着用它，凭据错误时不会每 10 分钟多漏一个；
 * 热重载（每个新实例一个）与连上过以后结束的那一轮仍各新建一个，数量跟着改设置、断线重建的次数增长，不随时间自己涨。
 * 「连上过」不只看 onReady：连接刚建好就失效时 SDK 已经记下 everConnected（client_session.go:53-55），
 * 却不调回调（:71-72）。所以复用的客户端 Start 回「cannot be restarted」时，丢掉它、马上新建一个重试。
 */
func (c *Channel) supervise() {
	ctx := c.ctx
	c.mu.RLock()
	cfg := c.cfg
	c.mu.RUnlock()
	claimed := false
	defer func() {
		// 先等本实例的队列工作协程退出，再交出排队的消息：取出了还没交给 MessageChain 的那条已经放回队首，
		// 正在处理的通常也已经收尾。处理函数不理会取消、过了 handoffWait 还没收尾时照常交接，同时带上那几个人的
		// 完成信号：新实例马上连上、别人的消息照常处理，这个人的下一条等那条收尾（最多 previousWait）
		if wait := c.timing(c.handoffWait, defaultHandoffWait); !waitGroupTimeout(&c.drains, wait) {
			c.logger().Warnf("飞书应用机器人(%d): 正在处理的消息 %s内没收尾，排着队的照常交接（这个人的下一条等它收尾）",
				c.confID, durationText(wait))
		}
		left := c.queues.seal()
		if !claimed {
			return // 没登记过就没连过长连接，也就收不到消息
		}
		if n := countQueued(left.items); n > 0 {
			c.logger().Infof("飞书应用机器人(%d): 关闭时还有 %d 条排队的消息没开始处理，交给重建以后的通道接着处理"+
				"（%s内没有接手就丢掉；进程退出时不处理）", c.confID, n, durationText(apps.ttl()))
		}
		apps.release(cfg.AppID, c, left)
	}()
	for warned := false; ; warned = true {
		holder, ok, wake, items := apps.claim(cfg.AppID, c)
		if ok {
			claimed = true
			c.adopt(items)
			break
		}
		retry := c.timing(c.claimRetry, defaultClaimRetry)
		if holder == c.confID {
			// 这个通道的上一个实例：热重载时旧连接还没断开，断开的那一刻接手（不算异常）
			if !warned {
				c.logger().Infof("飞书应用机器人(%d): 上一个实例的长连接还没断开，断开以后马上接手", c.confID)
			}
		} else {
			c.conflict.Store(true)
			if !warned {
				c.logger().Warnf("飞书应用机器人(%d): 同一个飞书应用已被通道 #%d 使用，这个通道不连长连接（一个应用只能给一个通道用），"+
					"那个通道停下以后马上接手（另外每分钟重试一次）", c.confID, holder)
			}
		}
		if !waitClaim(ctx, wake, retry) {
			return
		}
	}
	c.conflict.Store(false)

	wait := c.timing(c.backoffBase, defaultBackoffBase)
	var r wsRunner
	reused := false // r 已经 Start 过（这一轮是复用）
	for {
		if r == nil {
			r, reused = c.newRunner(cfg, c.hooks(), c.sdkLogger()), false
		}
		started := time.Now()
		c.linked.Store(false)
		c.setRunner(r)
		err := r.Start(ctx)
		c.setRunner(nil)
		if ctx.Err() != nil {
			return
		}
		if reused && isTerminalClient(err) {
			// SDK 认为它连上过（见上面的注释）：换一个新的马上重试，这一轮不算失败
			r = nil
			continue
		}
		if c.linked.Load() {
			r = nil // 连上过：SDK 已经把它标成终态
		} else {
			reused = true
		}
		if err == nil {
			err = errors.New("长连接意外结束")
		}
		c.setWS(wsFailed)
		var delay time.Duration
		delay, wait = c.nextDelay(err, time.Since(started), wait)
		if isClientError(err) {
			c.logger().Warnf("飞书应用机器人(%d): 长连接被飞书拒绝: %v；%s 后重试（改对 App ID、App Secret 并保存以后会马上重连）", c.confID, c.red.wrap(err), delay)
		} else {
			c.logger().Warnf("飞书应用机器人(%d): 长连接中断: %v；%s 后重连", c.confID, c.red.wrap(err), delay.Round(time.Second))
		}
		if !sleepCtx(ctx, jitter(delay)) {
			return
		}
	}
}

// nextDelay 是这一轮失败以后等多久，以及下一轮的退避起点：不可重试的错误固定等 clientErrWait；
// 其余从 backoffBase 起翻倍、最长 backoffMax，上一轮跑过 resetAfter 就从头算。
func (c *Channel) nextDelay(err error, ranFor, wait time.Duration) (time.Duration, time.Duration) {
	base := c.timing(c.backoffBase, defaultBackoffBase)
	maxWait := c.timing(c.backoffMax, defaultBackoffMax)
	if isClientError(err) {
		return c.timing(c.clientErrWait, defaultClientErrWait), base
	}
	if ranFor >= c.timing(c.resetAfter, defaultResetAfter) {
		wait = base
	}
	return wait, min(wait*2, maxWait)
}

// durationText 把整分钟的时长写成「N 分钟」，其余照 Go 的写法（测试里的毫秒）。
func durationText(d time.Duration) string {
	if d >= time.Minute && d%time.Minute == 0 {
		return fmt.Sprintf("%d 分钟", int(d/time.Minute))
	}
	return d.String()
}

// waitGroupTimeout 等 wg 归零，最多 d；超时返回 false（等的协程在 wg 归零以后自己退出）。
func waitGroupTimeout(wg *sync.WaitGroup, d time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

// waitClaim 等占着登记的实例释放（wake 关闭）或 retry 到期；ctx 取消时返回 false。
func waitClaim(ctx context.Context, wake <-chan struct{}, retry time.Duration) bool {
	timer := time.NewTimer(retry)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-wake:
	case <-timer.C:
	}
	return ctx.Err() == nil
}

// jitter 加上 0–50% 的抖动，避免多个实例同时重连。
func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	return d + rand.N(d/2+1)
}

// hooks 是交给 SDK 的回调：只改原子变量，不阻塞，也不在里面调 CloseAndWait（会等自己）。
func (c *Channel) hooks() wsHooks {
	connected := func() {
		c.linked.Store(true)
		c.setWS(wsConnected)
		c.logger().Infof("飞书应用机器人(%d): 长连接已连上", c.confID)
	}
	return wsHooks{
		onMessage:      c.onMessage,
		onReady:        connected,
		onReconnected:  connected,
		onReconnecting: func() { c.setWS(wsWaiting) },
		onDisconnected: func() { c.setWS(wsWaiting) },
		onError: func(err error) {
			c.setWS(wsFailed)
			if c.ctx.Err() == nil {
				c.logger().Warnf("飞书应用机器人(%d): 长连接出错: %v", c.confID, c.red.wrap(err))
			}
		},
	}
}

// loadBotInfo 取本应用机器人的 open_id（群里判断有没有 @ 它要用），取到以后写进进程级缓存。取不到时标异常、
// 按退避重试；取到以前群里 @ 了人的消息请平台稍后重推（见 onMessage），私聊照常。
func (c *Channel) loadBotInfo() {
	ctx := c.ctx
	c.mu.RLock()
	key := botCacheKey(c.cfg)
	c.mu.RUnlock()
	wait := c.timing(c.backoffBase, defaultBackoffBase)
	maxWait := c.timing(c.backoffMax, defaultBackoffMax)
	for {
		reqCtx, cancel := context.WithTimeout(ctx, httpTimeout+5*time.Second)
		info, _, err := c.api.BotInfo(reqCtx)
		cancel()
		err = withTokenHint(err)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			id := info.OpenID
			botIDs.set(key, id)
			c.botID.Store(&id)
			c.botFailed.Store(false)
			c.logger().Infof("飞书应用机器人(%d): 机器人 open_id=%s", c.confID, id)
			return
		}
		c.botFailed.Store(true)
		c.logger().Warnf("飞书应用机器人(%d): 获取机器人信息失败（取到以前群里 @ 机器人的消息会请飞书稍后重推）: %v；%s 后重试",
			c.confID, c.red.wrap(err), wait)
		if !sleepCtx(ctx, jitter(wait)) {
			return
		}
		wait = min(wait*2, maxWait)
	}
}

func (c *Channel) setRunner(r wsRunner) {
	c.runnerMu.Lock()
	c.runner = r
	c.runnerMu.Unlock()
}

func (c *Channel) currentRunner() wsRunner {
	c.runnerMu.Lock()
	defer c.runnerMu.Unlock()
	return c.runner
}

// spawn 起一个受 Close 等待的协程；Close 开始以后不再起，返回 false。
func (c *Channel) spawn(fn func()) bool {
	c.goMu.Lock()
	defer c.goMu.Unlock()
	if c.stopping {
		return false
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		fn()
	}()
	return true
}
