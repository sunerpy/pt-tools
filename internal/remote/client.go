package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/flynn/noise"
)

// 设备端（Go 实现）：给测试、验收与跨语言互通测试用；App 用 Dart 实现同一套协议。
const (
	// ClientPingInterval 是设备给主机发 PING 的间隔（主机 90 秒收不到任何帧就断开）
	ClientPingInterval = 30 * time.Second
)

// ErrGoAway 是主机发了 GOAWAY，会话结束（原因见 Client.GoAway）。
var ErrGoAway = errors.New("主机关闭了会话")

// ClientConfig 是设备连主机的参数。
type ClientConfig struct {
	HostID  string
	HostKey []byte
	// Device 是这台设备的 X25519 密钥
	Device noise.DHKey
	// Client 是 App 的名字与版本，握手时告诉主机
	Client string
	// Random 是握手临时密钥的随机源，nil 时用 crypto/rand（测试向量用固定的）
	Random io.Reader
}

// Client 是设备端的一条会话，实现 http.RoundTripper：请求经隧道发给主机。
type Client struct {
	conn   *secureConn
	raw    MsgConn
	hello  HostHello
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	mu      sync.Mutex
	nextID  uint32
	pending map[uint32]*clientCall
	goAway  string
	err     error
}

type clientCall struct {
	head chan *http.Response
	body *callBody
	req  *http.Request
}

// Connect 在 raw 上握手；主机不认识这台设备时返回 ErrNotPaired。
func Connect(ctx context.Context, raw MsgConn, cfg ClientConfig) (*Client, error) {
	conn, hello, err := dialHandshake(ctx, raw, cfg.HostID, cfg.HostKey, cfg.Device, cfg.Random, ClientHello{Client: cfg.Client})
	if err != nil {
		raw.Close(1000, "")
		return nil, err
	}
	cctx, cancel := context.WithCancel(context.Background())
	c := &Client{conn: conn, raw: raw, hello: hello, ctx: cctx, cancel: cancel, done: make(chan struct{}), pending: map[uint32]*clientCall{}}
	go c.readLoop()
	go c.pingLoop()
	return c, nil
}

// Mode 是会话的种类：device 或 pairing。
func (c *Client) Mode() string { return c.hello.Mode }

// HostVersion 是主机报的 pt-tools 版本。
func (c *Client) HostVersion() string { return c.hello.Host }

// Done 在会话结束时关闭。
func (c *Client) Done() <-chan struct{} { return c.done }

// GoAway 是主机关会话时给的原因（没有时为空）。
func (c *Client) GoAway() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.goAway
}

// Err 是会话结束的原因。
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Close 关掉会话。
func (c *Client) Close() {
	c.fail(errors.New("会话已经关闭"))
	c.raw.Close(1000, "")
}

// Ping 发一个 PING（回的 PONG 不等）。
func (c *Client) Ping(ctx context.Context) error {
	return c.conn.WriteFrame(ctx, Frame{Type: FramePing})
}

func (c *Client) fail(err error) {
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return
	}
	c.err = err
	calls := c.pending
	c.pending = map[uint32]*clientCall{}
	c.mu.Unlock()
	c.cancel()
	for _, call := range calls {
		call.body.finish(err)
		select {
		case call.head <- nil:
		default:
		}
	}
	close(c.done)
}

func (c *Client) pingLoop() {
	t := time.NewTicker(ClientPingInterval)
	defer t.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-t.C:
			ctx, cancel := context.WithTimeout(c.ctx, writeTimeout)
			err := c.Ping(ctx)
			cancel()
			if err != nil {
				c.fail(err)
				return
			}
		}
	}
}

func (c *Client) readLoop() {
	for {
		f, err := c.conn.ReadFrame(c.ctx)
		if err != nil {
			c.fail(err)
			return
		}
		if herr := c.handle(f); herr != nil {
			c.fail(herr)
			c.raw.Close(1000, "")
			return
		}
	}
}

func (c *Client) call(id uint32) *clientCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pending[id]
}

func (c *Client) drop(id uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, id)
}

func (c *Client) handle(f Frame) error {
	switch f.Type {
	case FrameRespHead:
		call := c.call(f.ID)
		if call == nil {
			return nil
		}
		var head ResponseHead
		if err := json.Unmarshal(f.Payload, &head); err != nil {
			return fmt.Errorf("%w: 回应头格式不对", errProtocol)
		}
		resp := &http.Response{
			Status: fmt.Sprintf("%d %s", head.Status, http.StatusText(head.Status)), StatusCode: head.Status,
			Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1, Header: http.Header{}, Body: call.body, Request: call.req, ContentLength: -1,
		}
		for k, v := range head.Headers {
			resp.Header.Set(k, v)
		}
		select {
		case call.head <- resp:
		default:
		}
	case FrameRespBody:
		if call := c.call(f.ID); call != nil {
			call.body.append(bytes.Clone(f.Payload))
		}
	case FrameRespEnd:
		if call := c.call(f.ID); call != nil {
			c.drop(f.ID)
			call.body.finish(nil)
		}
	case FrameCancel:
		if call := c.call(f.ID); call != nil {
			c.drop(f.ID)
			err := fmt.Errorf("主机中止了回应: %s", f.Payload)
			call.body.finish(err)
			select {
			case call.head <- nil:
			default:
			}
		}
	case FramePing:
		ctx, cancel := context.WithTimeout(c.ctx, writeTimeout)
		defer cancel()
		return c.conn.WriteFrame(ctx, Frame{Type: FramePong, Payload: f.Payload})
	case FramePong:
	case FrameGoAway:
		// 主机随后会断开；这边也不再等，直接结束会话（经 relay 时主机的关闭不一定传得过来）
		var g GoAway
		_ = json.Unmarshal(f.Payload, &g)
		c.mu.Lock()
		c.goAway = g.Reason
		c.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrGoAway, g.Reason)
	default:
		return fmt.Errorf("%w: 主机不该发类型 0x%02x", errProtocol, byte(f.Type))
	}
	return nil
}

// RoundTrip 经隧道发一个请求。请求的 URL 只用路径与查询串；回应体读完或关掉之前，这个请求一直占着一个编号。
func (c *Client) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	var body []byte
	if req.Body != nil {
		b, err := io.ReadAll(io.LimitReader(req.Body, MaxRequestBody+1))
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
		if len(b) > MaxRequestBody {
			return nil, fmt.Errorf("请求体超过 %d MiB", MaxRequestBody>>20)
		}
		body = b
	}
	head := RequestHead{Method: req.Method, Path: req.URL.RequestURI(), Headers: map[string]string{}}
	for _, k := range requestHeaderAllow {
		if v := req.Header.Get(k); v != "" {
			head.Headers[k] = v
		}
	}
	headJSON, err := json.Marshal(head)
	if err != nil {
		return nil, err
	}
	call := &clientCall{head: make(chan *http.Response, 1), req: req}
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return nil, err
	}
	c.nextID++
	if c.nextID == 0 {
		c.nextID = 1
	}
	id := c.nextID
	call.body = &callBody{notify: make(chan struct{}, 1), cancel: func() { c.cancelCall(id) }}
	c.pending[id] = call
	c.mu.Unlock()

	frames := []Frame{{Type: FrameReqHead, ID: id, Payload: headJSON}}
	for len(body) > 0 {
		n := min(len(body), MaxFramePayload)
		frames = append(frames, Frame{Type: FrameReqBody, ID: id, Payload: body[:n]})
		body = body[n:]
	}
	frames = append(frames, Frame{Type: FrameReqEnd, ID: id})
	for _, f := range frames {
		if err := c.conn.WriteFrame(ctx, f); err != nil {
			c.drop(id)
			return nil, err
		}
	}
	select {
	case resp := <-call.head:
		if resp == nil {
			return nil, c.callErr(call)
		}
		return resp, nil
	case <-ctx.Done():
		c.cancelCall(id)
		return nil, ctx.Err()
	}
}

func (c *Client) callErr(call *clientCall) error {
	if err := call.body.failure(); err != nil {
		return err
	}
	if err := c.Err(); err != nil {
		return err
	}
	return errAborted
}

// cancelCall 取消一个还没收完回应的请求：告诉主机不用再发，本地的回应体结束。
func (c *Client) cancelCall(id uint32) {
	call := c.call(id)
	if call == nil {
		return
	}
	c.drop(id)
	call.body.finish(context.Canceled)
	ctx, cancel := context.WithTimeout(c.ctx, writeTimeout)
	defer cancel()
	_ = c.conn.WriteFrame(ctx, Frame{Type: FrameCancel, ID: id})
}

// Pair 在配对会话里提交配对密钥与设备名，返回主机记下的设备。之后主机会用 GOAWAY paired 关掉这条会话。
func (c *Client) Pair(ctx context.Context, secret []byte, name string) (Device, error) {
	b, err := json.Marshal(pairRequest{Secret: EncodeKey(secret), Name: name})
	if err != nil {
		return Device{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, PairPath, bytes.NewReader(b))
	if err != nil {
		return Device{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.RoundTrip(req)
	if err != nil {
		return Device{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Device{}, err
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(data, &e)
		return Device{}, &PairError{Status: resp.StatusCode, Code: e.Error, Message: e.Message}
	}
	var out struct {
		Device Device `json:"device"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return Device{}, fmt.Errorf("配对回应格式不对: %w", err)
	}
	return out.Device, nil
}

// PairError 是配对被拒绝。
type PairError struct {
	Status  int
	Code    string
	Message string
}

func (e *PairError) Error() string {
	return fmt.Sprintf("配对失败（%d %s）: %s", e.Status, e.Code, strings.TrimSpace(e.Message))
}

// callBody 是回应体：读者循环往里追加，调用方从里面读。
type callBody struct {
	mu     sync.Mutex
	chunks [][]byte
	done   bool
	err    error
	closed bool
	notify chan struct{}
	cancel func()
}

func (b *callBody) signal() {
	select {
	case b.notify <- struct{}{}:
	default:
	}
}

func (b *callBody) append(p []byte) {
	b.mu.Lock()
	if !b.done && !b.closed {
		b.chunks = append(b.chunks, p)
	}
	b.mu.Unlock()
	b.signal()
}

func (b *callBody) finish(err error) {
	b.mu.Lock()
	if !b.done {
		b.done, b.err = true, err
	}
	b.mu.Unlock()
	b.signal()
}

func (b *callBody) failure() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.err
}

func (b *callBody) Read(p []byte) (int, error) {
	for {
		b.mu.Lock()
		if len(b.chunks) > 0 {
			n := copy(p, b.chunks[0])
			if n == len(b.chunks[0]) {
				b.chunks = b.chunks[1:]
			} else {
				b.chunks[0] = b.chunks[0][n:]
			}
			b.mu.Unlock()
			return n, nil
		}
		if b.closed {
			b.mu.Unlock()
			return 0, errors.New("回应体已经关闭")
		}
		if b.done {
			err := b.err
			b.mu.Unlock()
			if err == nil {
				err = io.EOF
			}
			return 0, err
		}
		b.mu.Unlock()
		<-b.notify
	}
}

// Close 没读完就关掉时取消这个请求。
func (b *callBody) Close() error {
	b.mu.Lock()
	wasDone := b.done
	b.closed = true
	b.chunks = nil
	b.mu.Unlock()
	if !wasDone && b.cancel != nil {
		b.cancel()
	}
	return nil
}
