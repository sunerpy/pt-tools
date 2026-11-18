package v2

import (
	"fmt"
	"net/http"
)

// 下载种子文件的重试与请求站点的其它重试是同一套（指数退避，见 site_retry.go 的 retryTransient）。

// HTTPStatusError 是站点回了不是 200 的状态码（文本与以前一样，例如 HTTP 502: Bad Gateway）。
type HTTPStatusError struct {
	Code int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.Code, http.StatusText(e.Code))
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
