package v2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// 请求站点遇到一时的错误时按指数退避重试（所有站点、所有入口）：
//   - 读种子列表与搜索（BaseSite.Search：刷流、网页搜索、订阅与追剧）；
//   - 取用户信息：走 API 的站点（M-Team、Rousi 等）与没有站点定义的 NexusPHP 站点整次重试（BaseSite.GetUserInfo）；
//     有站点定义的 NexusPHP 站点按页重试（fetchUserInfoPage，退避见 userInfoRetryBase）；
//   - 下载种子文件（RSS、订阅与追剧、手动推送、App、qB 兼容入口、转种、辅种）：BaseSite 重试取种子的那次请求，
//     两段式下载（先取下载地址、再拉种子文件，M-Team 与 NexusPHP）的第二段在驱动里重试，只重拉拿到的下载地址。
//
// 只重试网络错误（连接被重置、超时、连不上）与服务端出错（5xx）。调用方取消、凭证无效、登录过期、二次验证、
// Cloudflare 质询、限流（429：要等更久，交给调用方稍后再来）、内容不对、域名解析不了与证书不对的不重试。
//
// 退避是指数的：第 n 次重试前等 siteRetryBase·2^(n-1)（2 秒、4 秒），各另加最多一半的随机时长，几个任务不会同时重试；
// 最多重试 siteRetryMax 次。每次重试前还要先等站点限速的令牌（wait）：重试也算进站点的请求频率，不会因为重试而
// 把请求变密、触发站点的限流。剩下的时间不够一次像样的请求（siteRetryMinAttempt）时不再试。
// 2026-10-27 生产到 Cloudflare 一部分地址丢包五到七成，HDSky 刷流读列表、OurBits 与 Rousi 的登录探测时常超时或被重置。
var (
	siteRetryBase       = 2 * time.Second
	siteRetryMax        = 2
	siteRetryMinAttempt = 10 * time.Second
)

// transientSiteError 报告请求站点的这个错误是不是一时的、值得隔一会儿再试：网络错误（域名解析不了、证书不对除外）与 5xx。
// 只取到一部分用户信息的不算（每一页已经自己重试过了，取到的部分交回去由 UserInfoService 补齐）。
func transientSiteError(err error) bool {
	var partial *PartialUserInfoError
	if errors.As(err, &partial) {
		return false
	}
	if isTransientNetError(err) {
		return true
	}
	var se *HTTPStatusError
	return errors.As(err, &se) && se.Code >= http.StatusInternalServerError
}

// siteRetryDelay 是第 n 次重试（从 1 起）前要等多久：siteRetryBase 翻 n-1 倍，再加最多一半的随机时长。
func siteRetryDelay(n int) time.Duration {
	return jitter(siteRetryBase << (n - 1))
}

// retryTransient 跑 fn，一时的错误按指数退避再跑（最多重试 siteRetryMax 次）；ctx 结束了、剩下的时间不够了就不再试。
// wait 不为空时每次重试前先调它（等站点限速的令牌）。重试过的错误带上一共试了几次。
func retryTransient[T any](ctx context.Context, wait func(context.Context) error, fn func() (T, error)) (T, error) {
	for attempt := 1; ; attempt++ {
		v, err := fn()
		if err == nil {
			return v, nil
		}
		if !transientSiteError(err) || attempt > siteRetryMax || ctx.Err() != nil {
			return v, triedErr(err, attempt)
		}
		delay := siteRetryDelay(attempt)
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < delay+siteRetryMinAttempt {
			return v, triedErr(err, attempt)
		}
		if !sleepContext(ctx, delay) {
			return v, triedErr(err, attempt)
		}
		if wait != nil {
			if werr := wait(ctx); werr != nil {
				return v, triedErr(err, attempt)
			}
		}
	}
}

func triedErr(err error, attempts int) error {
	if attempts <= 1 {
		return err
	}
	return fmt.Errorf("%w (%d attempts)", err, attempts)
}
