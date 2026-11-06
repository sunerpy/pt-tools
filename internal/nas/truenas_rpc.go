package nas

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/sunerpy/pt-tools/internal/redact"
)

// trueNASPath 是 TrueNAS 的 JSON-RPC 2.0 接口（25.04 起有）。
const trueNASPath = "/api/current"

// trueNASReadLimit 是一条回复最大多大：30 分钟的报表与告警列表比 coder/websocket 默认的 32 KiB 大。
const trueNASReadLimit = 16 << 20

// TrueNAS 回复里的错误号（data.error，同 Linux 的 errno）。
const (
	errnoEACCES            = 13
	errnoENOTAUTHENTICATED = 207
)

// rpcError 是 TrueNAS 回复的错误：Code 是 JSON-RPC 的错误码，Data 里是 TrueNAS 的错误号与原因。
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    *struct {
		Error   int    `json:"error"`
		Errname string `json:"errname"`
		Reason  string `json:"reason"`
	} `json:"data"`

	method string
	secret string
}

func (e *rpcError) Error() string {
	reason := e.Message
	if e.Data != nil && strings.TrimSpace(e.Data.Reason) != "" {
		reason = e.Data.Reason
	}
	// 校验错误的原因有好几行，只要第一行
	if i := strings.IndexByte(reason, '\n'); i >= 0 {
		reason = reason[:i]
	}
	reason = strings.TrimSpace(redact.Secrets(reason))
	if len(e.secret) >= 4 {
		reason = strings.ReplaceAll(reason, e.secret, "***")
	}
	if r := []rune(reason); len(r) > 160 {
		reason = string(r[:160]) + "…"
	}
	if reason == "" {
		reason = fmt.Sprintf("错误码 %d", e.Code)
	}
	if e.method == "" {
		return "TrueNAS 返回错误：" + reason
	}
	return fmt.Sprintf("TrueNAS 的 %s 返回错误：%s", e.method, reason)
}

// denied 是账号没有调这个方法的权限（[EACCES] Not authorized）。
func (e *rpcError) denied() bool {
	return e.Data != nil && (e.Data.Error == errnoEACCES || e.Data.Errname == "EACCES")
}

// unauthenticated 是这条连接没登录（或者登录失效了）。
func (e *rpcError) unauthenticated() bool {
	return e.Data != nil && (e.Data.Error == errnoENOTAUTHENTICATED || e.Data.Errname == "ENOTAUTHENTICATED")
}

// missing 是这个版本的 TrueNAS 没有这个方法。
func (e *rpcError) missing() bool { return e.Code == -32601 }

// decodeError 是回复的结果和预期的样子对不上。
type decodeError struct {
	method string
	err    error
}

func (e *decodeError) Error() string {
	return fmt.Sprintf("TrueNAS 的 %s 回复的数据看不懂：%v", e.method, e.err)
}

func (e *decodeError) Unwrap() error { return e.err }

// transportError 报告 err 是不是连接本身的问题（断了、超时）：TrueNAS 回了错误或回复看不懂都不算。
func transportError(err error) bool {
	if err == nil {
		return false
	}
	var re *rpcError
	var de *decodeError
	return !errors.As(err, &re) && !errors.As(err, &de)
}

type rpcReply struct {
	result json.RawMessage
	err    error
}

// rpcConn 是到一台 TrueNAS 的一条 WebSocket 连接：请求按 id 对上回复，可以同时发多个；一个读循环收全部回复（顺带回应 ping）。
type rpcConn struct {
	ws     *websocket.Conn
	secret string
	nextID atomic.Int64
	cancel context.CancelFunc
	done   chan struct{}

	mu      sync.Mutex
	pending map[int64]chan rpcReply
	dead    error
}

// dialRPC 连上 u（https://主机:端口）的 /api/current。证书按 Transport 校验：不认时在握手阶段就断开，返回 *CertError，
// 不会发出 WebSocket 请求，更不会发 Key。不走代理、不跟随重定向。secret 只用来从错误原文里去掉 Key。
func dialRPC(ctx context.Context, u *url.URL, pin string, roots *x509.CertPool, secret string) (*rpcConn, error) {
	if u.Scheme != "https" {
		return nil, errTrueNASHTTP
	}
	target := url.URL{Scheme: "wss", Host: u.Host, Path: trueNASPath}
	client := &http.Client{
		Transport: Transport(u, pin, roots),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	ws, resp, err := websocket.Dial(ctx, target.String(), &websocket.DialOptions{HTTPClient: client})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		var ce *CertError
		if errors.As(err, &ce) {
			return nil, ce
		}
		if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
			return nil, handshakeError(resp.StatusCode)
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, err
		}
		// 拒绝连接、找不到主机这类：只留网络层那一段，不带 coder/websocket 套的几层前缀
		var op *net.OpError
		if errors.As(err, &op) {
			return nil, fmt.Errorf("连不上 TrueNAS：%w", op)
		}
		return nil, fmt.Errorf("连不上 TrueNAS：%w", err)
	}
	ws.SetReadLimit(trueNASReadLimit)
	rctx, cancel := context.WithCancel(context.Background())
	c := &rpcConn{ws: ws, secret: secret, cancel: cancel, done: make(chan struct{}), pending: map[int64]chan rpcReply{}}
	go c.readLoop(rctx)
	return c, nil
}

// handshakeError 是 /api/current 没有接受 WebSocket 连接时的原因。
func handshakeError(status int) error {
	switch {
	case status == http.StatusNotFound || status == http.StatusOK:
		return fmt.Errorf("这台 TrueNAS 没有 %s 接口（HTTP %d）：要 TrueNAS 25.04 或以后的版本", trueNASPath, status)
	case status >= 300 && status < 400:
		return fmt.Errorf("TrueNAS 把 %s 重定向到了别的地址（HTTP %d）：地址要写 TrueNAS 网页实际用的 https 地址与端口", trueNASPath, status)
	default:
		return fmt.Errorf("TrueNAS 没有接受 %s 的 WebSocket 连接（HTTP %d）", trueNASPath, status)
	}
}

func (c *rpcConn) readLoop(ctx context.Context) {
	defer close(c.done)
	for {
		_, data, err := c.ws.Read(ctx)
		if err != nil {
			c.fail(fmt.Errorf("和 TrueNAS 的连接断了：%w", err))
			return
		}
		var msg struct {
			ID     *int64          `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *rpcError       `json:"error"`
		}
		// 看不懂的消息与不带 id 的通知不管
		if json.Unmarshal(data, &msg) != nil || msg.ID == nil {
			continue
		}
		c.mu.Lock()
		ch, ok := c.pending[*msg.ID]
		delete(c.pending, *msg.ID)
		c.mu.Unlock()
		if !ok {
			continue
		}
		if msg.Error != nil {
			ch <- rpcReply{err: msg.Error}
		} else {
			ch <- rpcReply{result: msg.Result}
		}
	}
}

// fail 让连接不能再用：在等的请求都拿到 err。
func (c *rpcConn) fail(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dead == nil {
		c.dead = err
	}
	for id, ch := range c.pending {
		ch <- rpcReply{err: c.dead}
		delete(c.pending, id)
	}
}

// broken 报告连接是不是已经断了（读循环退出了）。
func (c *rpcConn) broken() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dead != nil
}

// close 断开连接，等读循环退出（最多 2 秒）。
func (c *rpcConn) close() {
	c.fail(errors.New("和 TrueNAS 的连接关了"))
	_ = c.ws.CloseNow()
	c.cancel()
	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
	}
}

// call 调一个方法，等到回复或 ctx 结束；out 为空时不解析结果。TrueNAS 回的错误是 *rpcError，回复看不懂是 *decodeError，
// 其余是连接的问题（连接随之不能再用）。
func (c *rpcConn) call(ctx context.Context, method string, params []any, out any) error {
	if params == nil {
		params = []any{}
	}
	id := c.nextID.Add(1)
	ch := make(chan rpcReply, 1)
	c.mu.Lock()
	if c.dead != nil {
		err := c.dead
		c.mu.Unlock()
		return err
	}
	c.pending[id] = ch
	c.mu.Unlock()
	req, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		c.forget(id)
		return fmt.Errorf("TrueNAS 的 %s 的参数写不成 JSON：%w", method, err)
	}
	if err := c.ws.Write(ctx, websocket.MessageText, req); err != nil {
		// 写失败时 coder/websocket 已经关了连接
		c.fail(fmt.Errorf("和 TrueNAS 的连接断了：%w", err))
		return err
	}
	var r rpcReply
	select {
	case r = <-ch:
	case <-ctx.Done():
		c.forget(id)
		return ctx.Err()
	}
	if r.err != nil {
		var re *rpcError
		if errors.As(r.err, &re) {
			re.method, re.secret = method, c.secret
		}
		return r.err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(r.result, out); err != nil {
		return &decodeError{method: method, err: err}
	}
	return nil
}

func (c *rpcConn) forget(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

// rpcCall 是 batch 里的一次调用。
type rpcCall struct {
	method string
	params []any
	out    any
	err    error
}

// batch 同时发出几次调用，等它们都回来。
func (c *rpcConn) batch(ctx context.Context, calls ...*rpcCall) {
	var wg sync.WaitGroup
	for _, rc := range calls {
		if rc == nil {
			continue
		}
		wg.Go(func() { rc.err = c.call(ctx, rc.method, rc.params, rc.out) })
	}
	wg.Wait()
}

// firstTransportError 是这几次调用里第一个连接本身的问题（超时排在前面，体检与列表上写「N 秒内没有回应」）。
func firstTransportError(calls ...*rpcCall) error {
	var first error
	for _, rc := range calls {
		if rc == nil || !transportError(rc.err) {
			continue
		}
		if errors.Is(rc.err, context.DeadlineExceeded) {
			return rc.err
		}
		if first == nil {
			first = rc.err
		}
	}
	return first
}

// login 用 API Key 登录这条连接：Key 不对或被吊销时 TrueNAS 回 false。
func (c *rpcConn) login(ctx context.Context, key string) error {
	var ok bool
	if err := c.call(ctx, "auth.login_with_api_key", []any{key}, &ok); err != nil {
		return err
	}
	if !ok {
		return &AuthError{Reason: "API Key 不对或已被吊销（在 TrueNAS 的「API Keys」里看它还在不在）"}
	}
	return nil
}
