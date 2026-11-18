package telegram

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// pollErrorLogInterval 是长轮询持续失败时两条日志之间的最短间隔。
const pollErrorLogInterval = 10 * time.Minute

// pollHealthTransport 观察 getUpdates 请求的结果，让 Healthy() 反映长轮询是否真的在工作。
//
// telego 的长轮询失败后自己每 8 秒重试一次，既不关闭 updates channel，也不把错误交给调用方，
// 错误日志又被 WithDiscardLogger 丢掉：令牌失效、网络断开、同一个 bot 被别处轮询（409）时，
// 通道一直显示「运行中」，日志里什么都没有。这些情况只有在 HTTP 这一层看得到。
type pollHealthTransport struct {
	next   http.RoundTripper
	report func(error) // nil 表示这一轮 getUpdates 成功
}

func (t pollHealthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if t.report == nil || !strings.HasSuffix(req.URL.Path, "/getUpdates") {
		return resp, err
	}
	switch {
	case err != nil:
		// Close 取消的那一轮不算失败。传输层错误不含请求 URL，不会带出路径里的令牌
		if req.Context().Err() == nil {
			t.report(err)
		}
	case resp.StatusCode != http.StatusOK:
		t.report(fmt.Errorf("getUpdates 返回 HTTP %d", resp.StatusCode))
	default:
		t.report(nil)
	}
	return resp, err
}

// reportPoll 接收 pollHealthTransport 回报的 getUpdates 结果：失败标记不健康，成功恢复健康。
// 日志限频：开始失败时记一条，持续失败时每 pollErrorLogInterval 记一条，恢复时记一条。
// Close 之后迟到的结果直接丢弃，不会把已关闭的通道标回健康。
func (c *TelegramChannel) reportPoll(err error) {
	c.mu.Lock()
	if c.pollCtx == nil || c.pollCtx.Err() != nil {
		c.mu.Unlock()
		return
	}
	c.healthy = err == nil
	wasFailing := c.pollFailing
	c.pollFailing = err != nil
	logFailure := err != nil && (!wasFailing || time.Since(c.pollErrLoggedAt) >= pollErrorLogInterval)
	if logFailure {
		c.pollErrLoggedAt = time.Now()
	}
	logger, confID := c.logger, c.confID
	token := ""
	if c.cfg != nil {
		token = c.cfg.BotToken
	}
	c.mu.Unlock()

	if logger == nil {
		return
	}
	switch {
	case logFailure:
		msg := err.Error()
		if token != "" {
			msg = strings.ReplaceAll(msg, token, "<bot_token>")
		}
		logger.Warnf("telegram: long-poll 失败 conf=%d: %s（telego 会自动重试）", confID, msg)
	case err == nil && wasFailing:
		logger.Infof("telegram: long-poll 已恢复 conf=%d", confID)
	}
}
