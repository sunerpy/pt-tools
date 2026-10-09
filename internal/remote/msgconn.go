package remote

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/coder/websocket"
)

// MsgConn 是按消息收发的连接，每条消息是一条 Noise 消息：直连时是一条 WebSocket，经 relay 时是 relay 连接里的一个流。
type MsgConn interface {
	// ReadMsg 读下一条消息。ctx 结束时连接会被关掉
	ReadMsg(ctx context.Context) ([]byte, error)
	// WriteMsg 写一条消息；返回以后 b 可以复用
	WriteMsg(ctx context.Context, b []byte) error
	// Close 关闭连接；code、reason 是给对端看的关闭原因（不保证送达），不阻塞
	Close(code int, reason string)
}

// StreamPath 是直连入口相对直连地址的路径。
const StreamPath = "/remote/v1/stream"

// errTextMessage 是对端发了文本消息（协议只用二进制消息）。
var errTextMessage = errors.New("只接受二进制消息")

// wsConn 把一条 WebSocket 当成 MsgConn：每条二进制消息是一条 Noise 消息。
type wsConn struct{ c *websocket.Conn }

func newWSConn(c *websocket.Conn) *wsConn {
	c.SetReadLimit(MaxNoiseMessage)
	return &wsConn{c: c}
}

func (w *wsConn) ReadMsg(ctx context.Context) ([]byte, error) {
	typ, b, err := w.c.Read(ctx)
	if err != nil {
		return nil, err
	}
	if typ != websocket.MessageBinary {
		w.Close(int(websocket.StatusUnsupportedData), "binary only")
		return nil, errTextMessage
	}
	return b, nil
}

func (w *wsConn) WriteMsg(ctx context.Context, b []byte) error {
	return w.c.Write(ctx, websocket.MessageBinary, b)
}

// Close 在后台做关闭握手（coder/websocket 的 Close 最多要等对端几秒），调用方不等。
func (w *wsConn) Close(code int, reason string) {
	go func() { _ = w.c.Close(websocket.StatusCode(code), reason) }()
}

// DialDirect 连上主机的直连入口：direct 是配对链接里的直连地址（http:// 或 https://），换成 ws:// 或 wss:// 再加上 /remote/v1/stream。
// client 为 nil 时用 http.DefaultClient。
func DialDirect(ctx context.Context, direct string, client *http.Client) (MsgConn, error) {
	base, err := NormalizeDirectURL(direct)
	if err != nil {
		return nil, err
	}
	u := "ws" + strings.TrimPrefix(base, "http") + StreamPath
	return dialWS(ctx, u, client)
}

// DialRelay 作为客户端连上 relay：relay/v1/client/<hostId>。
func DialRelay(ctx context.Context, relay, hostID string, client *http.Client) (MsgConn, error) {
	base, err := NormalizeRelayURL(relay)
	if err != nil {
		return nil, err
	}
	if !ValidHostID(hostID) {
		return nil, fmt.Errorf("hostId 不对")
	}
	return dialWS(ctx, base+"/v1/client/"+hostID, client)
}

func dialWS(ctx context.Context, u string, client *http.Client) (MsgConn, error) {
	c, resp, err := websocket.Dial(ctx, u, &websocket.DialOptions{HTTPClient: client})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("连接 %s 失败: %w", u, err)
	}
	return newWSConn(c), nil
}
