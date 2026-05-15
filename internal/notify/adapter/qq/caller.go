package qq

import (
	"context"
	"io"
	"sync"
	"sync/atomic"

	"github.com/RomiChan/websocket"
	"github.com/tidwall/gjson"
	zero "github.com/wdvxdr1123/ZeroBot"
)

// napCatCaller 实现 zero.APICaller 接口，承载与 NapCat 反向 WS 客户端的
// 请求/响应映射。设计参考 ZeroBot driver/wsserver.go 中的 *WSSCaller，
// 但其构造与 listen 方法均未导出，故此处自带 echo seq、并发写锁与回包通道映射。
type napCatCaller struct {
	conn   *websocket.Conn
	selfID int64

	writeMu sync.Mutex
	seq     uint64

	pendingMu sync.Mutex
	pending   map[uint64]chan zero.APIResponse
}

func newNapCatCaller(conn *websocket.Conn, selfID int64) *napCatCaller {
	return &napCatCaller{
		conn:    conn,
		selfID:  selfID,
		pending: make(map[uint64]chan zero.APIResponse),
	}
}

func (c *napCatCaller) nextSeq() uint64 {
	return atomic.AddUint64(&c.seq, 1)
}

func (c *napCatCaller) CallAPI(ctx context.Context, req zero.APIRequest) (zero.APIResponse, error) {
	ch := make(chan zero.APIResponse, 1)
	req.Echo = c.nextSeq()

	c.pendingMu.Lock()
	c.pending[req.Echo] = ch
	c.pendingMu.Unlock()

	c.writeMu.Lock()
	err := c.conn.WriteJSON(&req)
	c.writeMu.Unlock()

	if err != nil {
		c.pendingMu.Lock()
		delete(c.pending, req.Echo)
		c.pendingMu.Unlock()
		return zero.APIResponse{}, err
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return zero.APIResponse{}, io.ErrClosedPipe
		}
		return resp, nil
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, req.Echo)
		c.pendingMu.Unlock()
		return zero.APIResponse{}, ctx.Err()
	}
}

// dispatchAPIResponse 在 payload 含 echo 字段时将其投递给挂起的 CallAPI 调用，
// 返回 true 表示该 payload 是 API 响应，不应再作为事件分发。
func (c *napCatCaller) dispatchAPIResponse(payload []byte) bool {
	rsp := gjson.ParseBytes(payload)
	echo := rsp.Get("echo")
	if !echo.Exists() {
		return false
	}
	echoVal := echo.Uint()

	c.pendingMu.Lock()
	ch, ok := c.pending[echoVal]
	if ok {
		delete(c.pending, echoVal)
	}
	c.pendingMu.Unlock()
	if !ok {
		return true
	}

	msg := rsp.Get("message").String()
	if msg == "" {
		msg = rsp.Get("msg").String()
	}
	ch <- zero.APIResponse{
		Status:  rsp.Get("status").String(),
		Data:    rsp.Get("data"),
		Message: msg,
		Wording: rsp.Get("wording").String(),
		RetCode: rsp.Get("retcode").Int(),
		Echo:    echoVal,
	}
	close(ch)
	return true
}

var _ zero.APICaller = (*napCatCaller)(nil)
