package feishuapp

import (
	"context"
	"errors"
	"math/rand/v2"
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
)

func (c *Channel) timing(v, def time.Duration) time.Duration {
	if v > 0 {
		return v
	}
	return def
}

/*
 * supervise 是长连接的监督循环，参照 Telegram 的 runInbound：
 *  1. 先登记 app_id，被同一进程里的另一个通道占着时不连、标异常，每分钟重试；
 *  2. 每一轮都新建 SDK 客户端（停下以后不能再 Start），Start 阻塞运行，断线重连由 SDK 自己做；
 *  3. Start 只在 ctx 取消、不可重试的错误或服务端下发的重试次数用完时返回：后两种标异常、退避、重建。
 */
func (c *Channel) supervise() {
	ctx := c.ctx
	c.mu.RLock()
	cfg := c.cfg
	c.mu.RUnlock()
	log := c.logger()
	for warned := false; ; warned = true {
		holder, ok := apps.claim(cfg.AppID, c)
		if ok {
			break
		}
		c.conflict.Store(true)
		if !warned {
			log.Warnf("飞书应用机器人(%d): 同一个飞书应用已被通道 #%d 使用，这个通道不连长连接（一个应用只能给一个通道用），每分钟重试一次", c.confID, holder)
		}
		if !sleepCtx(ctx, c.timing(c.claimRetry, defaultClaimRetry)) {
			return
		}
	}
	c.conflict.Store(false)
	defer apps.release(cfg.AppID, c)

	wait := c.timing(c.backoffBase, defaultBackoffBase)
	for {
		started := time.Now()
		r := c.newRunner(cfg, c.hooks(), c.sdkLogger())
		c.setRunner(r)
		err := r.Start(ctx)
		c.setRunner(nil)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			err = errors.New("长连接意外结束")
		}
		c.setWS(wsFailed)
		var delay time.Duration
		delay, wait = c.nextDelay(err, time.Since(started), wait)
		if isClientError(err) {
			log.Warnf("飞书应用机器人(%d): 长连接被飞书拒绝: %v；%s 后重试（改对 App ID、App Secret 并保存以后会马上重连）", c.confID, c.red.wrap(err), delay)
		} else {
			log.Warnf("飞书应用机器人(%d): 长连接中断: %v；%s 后重连", c.confID, c.red.wrap(err), delay.Round(time.Second))
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

// loadBotInfo 取本应用机器人的 open_id（群里判断有没有 @ 它要用）。取不到时标异常、按退避重试；
// 取到以前群消息一律忽略，私聊照常。
func (c *Channel) loadBotInfo() {
	ctx := c.ctx
	wait := c.timing(c.backoffBase, defaultBackoffBase)
	maxWait := c.timing(c.backoffMax, defaultBackoffMax)
	for {
		reqCtx, cancel := context.WithTimeout(ctx, httpTimeout+5*time.Second)
		info, _, err := c.api.BotInfo(reqCtx)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			id := info.OpenID
			c.botID.Store(&id)
			c.botFailed.Store(false)
			c.logger().Infof("飞书应用机器人(%d): 机器人 open_id=%s", c.confID, id)
			return
		}
		c.botFailed.Store(true)
		c.logger().Warnf("飞书应用机器人(%d): 获取机器人信息失败（取到以前群里的消息一律忽略）: %v；%s 后重试",
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
