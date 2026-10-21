package v2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// 下载种子文件遇到一时的错误时重试（所有站点、所有入口：RSS、订阅与追剧、手动推送、App、qB 兼容入口、转种、辅种）。
// 网络错误（连接被重置、超时、连不上）与服务端出错（5xx）隔几秒再试；调用方取消、凭证无效、登录过期、Cloudflare 质询、
// 限流（429：要等更久，交给调用方稍后再来）与内容不对的不重试。BaseSite 重试取种子的那次请求（每次重试先等站点限速的令牌）；
// 两段式下载（先取下载地址、再拉种子文件，M-Team 与 NexusPHP）的第二段在驱动里重试，只重拉拿到的下载地址。

// downloadBackoff 是每次重试前等的时间，重试的次数是它的长度；测试里会调小。
var downloadBackoff = []time.Duration{2 * time.Second, 5 * time.Second}

// HTTPStatusError 是站点回了不是 200 的状态码（文本与以前一样，例如 HTTP 502: Bad Gateway）。
type HTTPStatusError struct {
	Code int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.Code, http.StatusText(e.Code))
}

// transientDownloadError 报告下载种子文件的这个错误是不是一时的、值得隔几秒再试。
func transientDownloadError(err error) bool {
	if errors.Is(err, ErrNetworkError) {
		return true
	}
	var se *HTTPStatusError
	return errors.As(err, &se) && se.Code >= http.StatusInternalServerError
}

// retryDownload 跑 fn，一时的错误隔一会儿再跑（最多重试 len(downloadBackoff) 次），ctx 结束时不再试；wait 不为空时
// 每次重试前先调它（等站点限速的令牌）。重试过的错误带上一共试了几次。
func retryDownload[T any](ctx context.Context, wait func(context.Context) error, fn func() (T, error)) (T, error) {
	for attempt := 1; ; attempt++ {
		v, err := fn()
		if err == nil {
			return v, nil
		}
		if !transientDownloadError(err) || attempt > len(downloadBackoff) || ctx.Err() != nil {
			return v, triedErr(err, attempt)
		}
		timer := time.NewTimer(downloadBackoff[attempt-1])
		select {
		case <-ctx.Done():
			timer.Stop()
			return v, triedErr(err, attempt)
		case <-timer.C:
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

// redactURLsInError 把错误文本里的每个地址都脱敏（下载地址带 passkey、sign，跳转后的下载节点地址也带签名），
// 保留原错误链供 errors.Is 判断。net/http 的传输错误会把完整请求地址写进错误文本。
func redactURLsInError(err error) error {
	if err == nil || !rawURLRe.MatchString(err.Error()) {
		return err
	}
	return &urlRedactedError{err: err}
}

type urlRedactedError struct {
	err error
}

func (e *urlRedactedError) Error() string {
	return sanitizeRawURLs(e.err.Error())
}

func (e *urlRedactedError) Unwrap() error { return e.err }
