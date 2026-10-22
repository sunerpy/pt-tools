package internal

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/utils"
)

// 取 RSS：RSS 任务与过滤规则的试跑共用。带浏览器的 User-Agent（Cloudflare 后面的站点会掐掉通用 UA 的握手），
// 失败时说清楚是哪一步出的问题（域名查不到、连不上、被断开、超时、站点回的状态码、回的不是 RSS），
// 错误文本里不带地址的查询串（RSS 链接带 passkey）。

// rssFetchTimeout 是取一次 RSS 的时限。
const rssFetchTimeout = 30 * time.Second

// maxRSSBody 是 RSS 内容的上限（再大的不读，免得一个出错的地址把内存吃光）。
const maxRSSBody = 32 << 20

// ErrRSSPlaceholder 是 RSS 地址还是示例里的占位地址（hdsky.xxx 这类）：它永远取不到，不去请求。
var ErrRSSPlaceholder = errors.New("RSS 地址是示例里的占位地址")

// ErrRSSInvalidURL 是 RSS 链接写得不对（不是 http(s) 链接）。
var ErrRSSInvalidURL = errors.New("RSS 链接不对")

// RSSFetchError 是取 RSS 失败：Message 是给人看的说明（不带地址的查询串），Err 是原来的错误（errors.Is 用）。
type RSSFetchError struct {
	Message string
	Err     error
}

func (e *RSSFetchError) Error() string { return e.Message }
func (e *RSSFetchError) Unwrap() error { return e.Err }

// FetchRSSFeed 取一个 RSS 并解析。
func FetchRSSFeed(ctx context.Context, rawURL string) (*gofeed.Feed, error) {
	return fetchRSS(ctx, rawURL, &http.Client{Timeout: rssFetchTimeout})
}

func fetchRSS(ctx context.Context, rawURL string, client *http.Client) (*gofeed.Feed, error) {
	rawURL = strings.TrimSpace(rawURL)
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, &RSSFetchError{Message: "RSS 链接不对：要以 http:// 或 https:// 开头，后面是站点的域名", Err: errors.Join(ErrRSSInvalidURL, err)}
	}
	host := u.Hostname()
	if models.IsPlaceholderRSSHost(host) {
		return nil, &RSSFetchError{
			Message: fmt.Sprintf("RSS 地址是示例里的占位地址（%s），取不到任何种子：到站点页把这条 RSS 的链接换成站点 RSS 页上给你的链接", host),
			Err:     ErrRSSPlaceholder,
		}
	}

	reqCtx, cancel := context.WithTimeout(ctx, rssFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, &RSSFetchError{Message: "构造 RSS 请求失败: " + utils.RedactURLError(err).Error(), Err: err}
	}
	req.Header.Set("User-Agent", rssUserAgent)
	req.Header.Set("Accept", "application/rss+xml, application/xml;q=0.9, text/xml;q=0.8, */*;q=0.5")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, rssTransportError(host, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRSSBody+1))
	if err != nil {
		return nil, rssTransportError(host, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, rssStatusError(host, resp.StatusCode, resp.Header, body)
	}
	if len(body) > maxRSSBody {
		return nil, &RSSFetchError{Message: fmt.Sprintf("%s 回的内容太大（超过 %d MB），不像是 RSS", host, maxRSSBody>>20)}
	}
	feed, err := gofeed.NewParser().Parse(bytes.NewReader(body))
	if err != nil {
		if looksLikeHTML(body) {
			return nil, &RSSFetchError{
				Message: fmt.Sprintf("%s 回的是网页不是 RSS：链接可能不是 RSS 地址、passkey 失效要重新登录，或者被 Cloudflare 拦了", host),
				Err:     err,
			}
		}
		return nil, &RSSFetchError{Message: "解析 RSS 失败: " + err.Error(), Err: err}
	}
	return feed, nil
}

// rssTransportError 把传输层的错误说清楚（DNS、拒绝连接、被断开、超时、TLS）。
func rssTransportError(host string, err error) error {
	redacted := utils.RedactURLError(err)
	msg := func(format string, args ...any) error {
		return &RSSFetchError{Message: fmt.Sprintf(format, args...), Err: redacted}
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsNotFound {
			return msg("找不到域名 %s（DNS 查不到这个域名）：检查 RSS 链接里的域名有没有写错；示例里 xxx 的部分要换成站点给你的地址", host)
		}
		return msg("解析域名 %s 失败（%s）：检查这台机器的网络与 DNS", host, dnsErr.Err)
	}
	var certErr *tls.CertificateVerificationError
	var unknownAuth x509.UnknownAuthorityError
	var hostnameErr x509.HostnameError
	if errors.As(err, &certErr) || errors.As(err, &unknownAuth) || errors.As(err, &hostnameErr) {
		return msg("%s 的证书校验不通过：检查 RSS 链接的域名，或这台机器的时间与证书库", host)
	}
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return msg("连不上 %s（连接被拒绝）：检查 RSS 链接的域名与端口", host)
	case errors.Is(err, syscall.ECONNRESET), strings.Contains(err.Error(), "connection reset"):
		return msg("%s 断开了连接（connection reset）：站点或它前面的 Cloudflare 拒绝了这条线路，稍后再试；一直这样时给 pt-tools 设代理（环境变量 HTTPS_PROXY）", host)
	case errors.Is(err, context.DeadlineExceeded), isTimeout(err):
		return msg("连接 %s 超时（%d 秒没有回应）：站点慢或这条线路不通，稍后再试；一直这样时给 pt-tools 设代理（环境变量 HTTPS_PROXY）", host, int(rssFetchTimeout/time.Second))
	case errors.Is(err, context.Canceled):
		return msg("取 RSS 被取消了")
	case errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		return msg("%s 没回完就断开了：稍后再试", host)
	}
	return msg("请求 %s 失败: %v", host, redacted)
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// rssStatusError 是站点回了不是 2xx 的状态码。
func rssStatusError(host string, code int, header http.Header, body []byte) error {
	base := fmt.Sprintf("%s 回了 HTTP %d %s", host, code, http.StatusText(code))
	hint := ""
	cloudflare := strings.EqualFold(strings.TrimSpace(header.Get("Cf-Mitigated")), "challenge") ||
		bytes.Contains(body, []byte("cf_chl_opt")) || bytes.Contains(body, []byte("/cdn-cgi/challenge-platform/"))
	switch {
	case cloudflare:
		hint = "：被 Cloudflare 的质询拦了，稍后再试；一直这样时给 pt-tools 设代理（环境变量 HTTPS_PROXY）"
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		hint = "：站点拒绝了这个链接，RSS 链接里的 passkey 可能失效了，去站点的 RSS 页重新生成一个"
	case code == http.StatusNotFound:
		hint = "：RSS 地址不对，去站点的 RSS 页重新复制链接"
	case code == http.StatusTooManyRequests:
		hint = "：请求太频繁，把这条 RSS 的间隔调大一些"
	case code >= 500:
		hint = "：站点那边出错了，稍后再试"
	}
	return &RSSFetchError{Message: base + hint}
}

// looksLikeHTML 判断内容是不是网页（登录页、质询页、站点首页）。
func looksLikeHTML(body []byte) bool {
	head := bytes.ToLower(bytes.TrimSpace(body[:min(len(body), 1024)]))
	return bytes.HasPrefix(head, []byte("<!doctype html")) || bytes.Contains(head, []byte("<html"))
}
