package remote

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/flynn/noise"
)

// sinkConn 收下写出去的消息就丢掉，读一直等到关闭。
type sinkConn struct{ closed chan struct{} }

func (c *sinkConn) ReadMsg(ctx context.Context) ([]byte, error) {
	select {
	case <-c.closed:
	case <-ctx.Done():
	}
	return nil, errMemClosed
}

func (c *sinkConn) WriteMsg(context.Context, []byte) error {
	select {
	case <-c.closed:
		return errMemClosed
	default:
		return nil
	}
}

func (c *sinkConn) Close(int, string) {
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
}

// FuzzSessionFrames 把任意的帧序列喂给一条设备会话（不经加密，直接交给 handle）：不能 panic、不能卡住，
// 关掉以后所有处理函数都退出。输入是一串 [长度 u16][帧]。
func FuzzSessionFrames(f *testing.F) {
	head := func(method, path string) []byte {
		b, _ := json.Marshal(RequestHead{Method: method, Path: path})
		return b
	}
	enc := func(frames ...Frame) []byte {
		var out []byte
		for _, fr := range frames {
			b, err := AppendFrame(nil, fr)
			if err != nil {
				panic(err)
			}
			out = binary.BigEndian.AppendUint16(out, uint16(len(b)))
			out = append(out, b...)
		}
		return out
	}
	f.Add(enc(Frame{Type: FrameReqHead, ID: 1, Payload: head("GET", "/api/app/v1/meta")}, Frame{Type: FrameReqEnd, ID: 1}))
	f.Add(enc(Frame{Type: FrameReqHead, ID: 2, Payload: head("POST", "/api/app/v1/push")}, Frame{Type: FrameReqBody, ID: 2, Payload: []byte("x")},
		Frame{Type: FrameCancel, ID: 2}, Frame{Type: FrameReqEnd, ID: 2}))
	f.Add(enc(Frame{Type: FrameReqHead, ID: 3, Payload: head("GET", "/api/app/v1/slow")}, Frame{Type: FrameReqEnd, ID: 3}, Frame{Type: FrameCancel, ID: 3}))
	f.Add(enc(Frame{Type: FrameReqHead, ID: 4, Payload: head("GET", "/api/app/v1/panic")}, Frame{Type: FrameReqEnd, ID: 4}, Frame{Type: FramePing}))
	f.Add(enc(Frame{Type: FrameReqHead, ID: 5, Payload: []byte("{")}, Frame{Type: FrameRespEnd, ID: 5}))

	th := newFuzzHost(f)
	f.Fuzz(func(t *testing.T, in []byte) {
		sink := &sinkConn{closed: make(chan struct{})}
		cs := noise.UnsafeNewCipherState(cipherSuite, [32]byte{1}, 0)
		s := newSession(th, newSecureConn(sink, cs, cs), sink, ViaDirect, ModeDevice, make([]byte, KeyLen), th.sessions.currentEpoch())
		s.device = Device{ID: 1, Scopes: ScopesFull}
		for len(in) >= 2 {
			n := int(binary.BigEndian.Uint16(in))
			in = in[2:]
			if n > len(in) {
				break
			}
			fr, err := ParseFrame(in[:n])
			in = in[n:]
			if err != nil {
				continue
			}
			if s.handle(fr) != nil {
				break
			}
		}
		s.shutdown("")
		s.handlers.Wait()
	})
}

// fuzzDispatcher 不记请求、不往通道里写（模糊测试跑很多轮）：/slow 等到取消，/panic panic，别的回一点内容。
type fuzzDispatcher struct{}

func (fuzzDispatcher) ServeRemote(w http.ResponseWriter, r *http.Request, _ Peer) {
	_, _ = io.Copy(io.Discard, r.Body)
	switch {
	case strings.Contains(r.URL.Path, "/slow"):
		<-r.Context().Done()
	case strings.Contains(r.URL.Path, "/panic"):
		panic("boom")
	default:
		_, _ = w.Write([]byte("ok"))
	}
}

func newFuzzHost(f *testing.F) *Host {
	f.Helper()
	h := New(Config{Store: newTestStore(f), Dispatcher: fuzzDispatcher{}})
	f.Cleanup(h.Close)
	return h
}
