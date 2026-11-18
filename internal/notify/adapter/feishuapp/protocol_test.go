package feishuapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest/observer"

	"github.com/sunerpy/pt-tools/internal/notify"
)

/*
 * 协议测试：真的 SDK（larkws.Client 与 lark.Client）连本地的假飞书。假服务照 SDK v3.13.0 的协议写：
 * POST /callback/ws/endpoint 回长连接地址与 ClientConfig（ws/model.go），GET /ws 升级成 WebSocket，
 * 帧是 SDK 的 ws.Frame（protobuf）；事件帧的 headers 是 type=event、sum=1、seq=0，SDK 处理完回一帧同样
 * headers 的应答，payload 是 {"code":200}（ws/model.go 的 Response）。
 */

type fakeFeishu struct {
	t      *testing.T
	srv    *httptest.Server
	appID  string
	secret string

	mu       sync.Mutex
	conns    map[*websocket.Conn]struct{}
	maxConns int
	acks     chan wsAck
	messages []fakeMessage
	pings    atomic.Int32
	// endpoints 是 /callback/ws/endpoint 被调了几次（每次 Start 或重连各一次）
	endpoints atomic.Int32
}

type wsAck struct {
	messageID string
	code      int
}

func newFakeFeishu(t *testing.T, appID, secret string) *fakeFeishu {
	t.Helper()
	f := &fakeFeishu{t: t, appID: appID, secret: secret, conns: map[*websocket.Conn]struct{}{}, acks: make(chan wsAck, 16)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /callback/ws/endpoint", f.endpoint)
	mux.HandleFunc("GET /ws", f.ws)
	mux.HandleFunc("POST /open-apis/auth/v3/tenant_access_token/internal", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","tenant_access_token":"t-g1044protocoltoken`+appID+`","expire":7200}`)
	})
	mux.HandleFunc("GET /open-apis/bot/v3/info", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","bot":{"open_id":"ou_bot_proto","app_name":"pt-tools"}}`)
	})
	mux.HandleFunc("POST /open-apis/im/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		var m fakeMessage
		_ = json.NewDecoder(r.Body).Decode(&m)
		m.ReceiveIDType = r.URL.Query().Get("receive_id_type")
		f.mu.Lock()
		f.messages = append(f.messages, m)
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"success","data":{"message_id":"om_out"}}`)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeFeishu) endpoint(w http.ResponseWriter, r *http.Request) {
	f.endpoints.Add(1)
	var req larkws.BootstrapRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	w.Header().Set("Content-Type", "application/json")
	if req.AppID != f.appID || req.AppSecret != f.secret {
		_, _ = io.WriteString(w, `{"code":1000040346,"msg":"app_id is invalid"}`)
		return
	}
	wsURL := "ws" + strings.TrimPrefix(f.srv.URL, "http") + "/ws?device_id=dev1&service_id=7"
	_ = json.NewEncoder(w).Encode(larkws.EndpointResp{Code: 0, Msg: "ok", Data: &larkws.Endpoint{
		Url:          wsURL,
		ClientConfig: &larkws.ClientConfig{ReconnectCount: -1, ReconnectInterval: 1, ReconnectNonce: 1, PingInterval: 120},
	}})
}

func (f *fakeFeishu) ws(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	f.mu.Lock()
	f.conns[conn] = struct{}{}
	f.maxConns = max(f.maxConns, len(f.conns))
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		delete(f.conns, conn)
		f.mu.Unlock()
		_ = conn.CloseNow()
	}()
	for {
		_, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		var frame larkws.Frame
		if frame.Unmarshal(data) != nil {
			continue
		}
		headers := larkws.Headers(frame.Headers)
		switch {
		case frame.Method == int32(larkws.FrameTypeControl) && headers.GetString(larkws.HeaderType) == string(larkws.MessageTypePing):
			f.pings.Add(1)
			pong := larkws.Frame{
				Method: int32(larkws.FrameTypeControl), Service: frame.Service,
				Headers: []larkws.Header{{Key: larkws.HeaderType, Value: string(larkws.MessageTypePong)}},
			}
			b, _ := pong.Marshal()
			_ = conn.Write(r.Context(), websocket.MessageBinary, b)
		case frame.Method == int32(larkws.FrameTypeData):
			var resp larkws.Response
			_ = json.Unmarshal(frame.Payload, &resp)
			f.acks <- wsAck{messageID: headers.GetString(larkws.HeaderMessageID), code: resp.StatusCode}
		}
	}
}

func (f *fakeFeishu) connCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.conns)
}

// push 往当前的连接推一帧接收消息事件，发出时间是现在。
func (f *fakeFeishu) push(t *testing.T, frameMsgID, messageID, openID, text string) {
	t.Helper()
	f.pushAt(t, frameMsgID, messageID, openID, text, time.Now())
}

// pushAt 往当前的连接推一帧接收消息事件，发出时间（create_time）是 sent。
func (f *fakeFeishu) pushAt(t *testing.T, frameMsgID, messageID, openID, text string, sent time.Time) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"schema": "2.0",
		"header": map[string]any{
			"event_id": "ev_" + messageID, "event_type": "im.message.receive_v1", "app_id": f.appID,
			"tenant_key": "tk", "create_time": strconv.FormatInt(time.Now().UnixMilli(), 10),
		},
		"event": map[string]any{
			"sender": map[string]any{"sender_id": map[string]any{"open_id": openID}, "sender_type": "user", "tenant_key": "tk"},
			"message": map[string]any{
				"message_id": messageID, "chat_id": "oc_p2p_proto", "chat_type": "p2p", "message_type": "text",
				"content": textJSON(text), "create_time": strconv.FormatInt(sent.UnixMilli(), 10),
			},
		},
	})
	require.NoError(t, err)
	frame := larkws.Frame{
		SeqID: 1, LogID: 1, Service: 7, Method: int32(larkws.FrameTypeData), Payload: payload,
		Headers: []larkws.Header{
			{Key: larkws.HeaderType, Value: string(larkws.MessageTypeEvent)},
			{Key: larkws.HeaderMessageID, Value: frameMsgID},
			{Key: larkws.HeaderSum, Value: "1"},
			{Key: larkws.HeaderSeq, Value: "0"},
			{Key: larkws.HeaderTraceID, Value: "trace-" + frameMsgID},
		},
	}
	b, err := frame.Marshal()
	require.NoError(t, err)
	f.mu.Lock()
	var conn *websocket.Conn
	for c := range f.conns {
		conn = c
	}
	f.mu.Unlock()
	require.NotNil(t, conn, "没有连接")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, conn.Write(ctx, websocket.MessageBinary, b))
}

func (f *fakeFeishu) waitAck(t *testing.T) wsAck {
	t.Helper()
	select {
	case a := <-f.acks:
		return a
	case <-time.After(10 * time.Second):
		t.Fatal("没有收到应答帧")
		return wsAck{}
	}
}

// protocolChannel 建一个用真 SDK 连假飞书的通道；setup 在 Init 以前、工厂与默认时长设好以后调用（可为空），
// 可以改时长、包一层工厂。
func protocolChannel(t *testing.T, f *fakeFeishu, confID uint, secret string, setup func(*Channel)) (*Channel, *chainRecorder) {
	t.Helper()
	c := New()
	c.newAPI = func(cfg *Config, log larkcore.Logger) apiSender { return newLarkAPIAt(f.srv.URL, cfg, log) }
	c.newRunner = func(cfg *Config, h wsHooks, log larkcore.Logger) wsRunner {
		return newLarkRunnerAt(f.srv.URL, cfg, h, log)
	}
	c.clientErrWait = time.Hour
	if setup != nil {
		setup(c)
	}
	raw := `{"app_id":"` + f.appID + `","app_secret":"` + secret + `","admin_users":["ou_admin"]}`
	require.NoError(t, c.Init(context.Background(), confOf(confID, raw)))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = c.Close(ctx)
	})
	rec := &chainRecorder{}
	c.OnInbound(rec.handle)
	return c, rec
}

func TestProtocol_EventRoundTrip(t *testing.T) {
	resetProcessState(t)
	appID := uniqueAppID("cli_proto")
	f := newFakeFeishu(t, appID, "proto-secret")
	c, rec := protocolChannel(t, f, 21, "proto-secret", nil)

	require.Eventually(t, func() bool { return c.LinkState() == notify.LinkConnected }, 10*time.Second, 5*time.Millisecond)
	require.Eventually(t, func() bool { return c.botOpenID() == "ou_bot_proto" }, 10*time.Second, 5*time.Millisecond)
	assert.True(t, c.Healthy())
	assert.Equal(t, 1, f.connCount())

	f.push(t, "frame-1", "om_proto_1", "ou_admin", "/help")
	ack := f.waitAck(t)
	assert.Equal(t, wsAck{messageID: "frame-1", code: http.StatusOK}, ack)
	require.Eventually(t, func() bool { return len(rec.got()) == 1 }, 10*time.Second, 5*time.Millisecond)
	assert.Equal(t, notify.InboundMessage{
		ChannelType: ChannelType, SourceConfID: 21, ChannelUserID: "ou_admin",
		ChatID: "oc_p2p_proto", MessageType: "private", Text: "/help",
	}, rec.got()[0])

	// 同一条消息重推：也回 200，只处理一次
	f.push(t, "frame-2", "om_proto_1", "ou_admin", "/help")
	assert.Equal(t, http.StatusOK, f.waitAck(t).code)
	time.Sleep(50 * time.Millisecond)
	assert.Len(t, rec.got(), 1)

	// 回复走真的 lark.Client 发到假接口
	require.NoError(t, c.Send(context.Background(), notify.Notification{Text: "帮助", Targets: map[string]string{"chat_id": "oc_p2p_proto"}}))
	f.mu.Lock()
	msgs := append([]fakeMessage(nil), f.messages...)
	f.mu.Unlock()
	require.Len(t, msgs, 1)
	assert.Equal(t, "chat_id", msgs[0].ReceiveIDType)
	assert.Equal(t, `{"text":"帮助"}`, msgs[0].Content)
	assert.Positive(t, f.pings.Load(), "连上以后马上发 ping")
}

// 凭据不对：/callback/ws/endpoint 回 1000040346，SDK 的 Start 返回不可重试的错误，通道标异常。
func TestProtocol_InvalidCredentialsMarksError(t *testing.T) {
	resetProcessState(t)
	appID := uniqueAppID("cli_proto_bad")
	f := newFakeFeishu(t, appID, "right-secret")
	var logs *observer.ObservedLogs
	c, _ := protocolChannel(t, f, 22, "wrong-secret", func(c *Channel) { logs = observedLogger(c) })
	require.Eventually(t, func() bool { return !c.Healthy() }, 10*time.Second, 5*time.Millisecond)
	require.Eventually(t, func() bool { return strings.Contains(logJoined(logs.All()), "1000040346") }, 10*time.Second, 5*time.Millisecond)
	assert.Equal(t, 0, f.connCount())
	assert.NotContains(t, logJoined(logs.All()), "wrong-secret")
}

// 凭据一直不对：真的 SDK 客户端没连上过，下一轮接着用它再 Start，不新建（每新建一个，SDK 就多留一个定时协程）。
func TestProtocol_InvalidCredentialsReusesClient(t *testing.T) {
	resetProcessState(t)
	appID := uniqueAppID("cli_proto_reuse")
	f := newFakeFeishu(t, appID, "right-secret")
	var built atomic.Int32
	c, _ := protocolChannel(t, f, 24, "wrong-secret", func(c *Channel) {
		c.clientErrWait = 20 * time.Millisecond
		inner := c.newRunner
		c.newRunner = func(cfg *Config, h wsHooks, log larkcore.Logger) wsRunner {
			built.Add(1)
			return inner(cfg, h, log)
		}
	})
	require.Eventually(t, func() bool { return f.endpoints.Load() >= 4 }, 10*time.Second, 5*time.Millisecond, "凭据错误时每轮都会再试")
	assert.Equal(t, int32(1), built.Load(), "四轮都用同一个客户端")
	assert.False(t, c.Healthy())
	assert.Equal(t, 0, f.connCount())
}

// 热重载：关掉旧实例再建新实例，服务端始终只有一条连接（同一应用的第二个客户端会把事件随机抢走）。
func TestProtocol_ReloadKeepsSingleConnection(t *testing.T) {
	resetProcessState(t)
	appID := uniqueAppID("cli_proto_reload")
	f := newFakeFeishu(t, appID, "s")
	c1, _ := protocolChannel(t, f, 23, "s", nil)
	require.Eventually(t, func() bool { return c1.LinkState() == notify.LinkConnected }, 10*time.Second, 5*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, c1.Close(ctx))
	require.Eventually(t, func() bool { return f.connCount() == 0 }, 10*time.Second, 5*time.Millisecond)

	c2, rec2 := protocolChannel(t, f, 23, "s", nil)
	require.Eventually(t, func() bool { return c2.LinkState() == notify.LinkConnected }, 10*time.Second, 5*time.Millisecond)
	assert.Equal(t, 1, f.connCount())
	f.mu.Lock()
	assert.Equal(t, 1, f.maxConns, "任何时候都只有一条连接")
	f.mu.Unlock()

	f.push(t, "frame-r", "om_reload", "ou_admin", "/status")
	assert.Equal(t, http.StatusOK, f.waitAck(t).code)
	require.Eventually(t, func() bool { return len(rec2.got()) == 1 }, 10*time.Second, 5*time.Millisecond)
}

// 真的 SDK 每帧一个 goroutine：连续两帧，第一帧的回调被拖到第二帧进队以后才跑，仍按发出时间处理。
func TestProtocol_FramesOrderedBySendTime(t *testing.T) {
	resetProcessState(t)
	appID := uniqueAppID("cli_proto_order")
	f := newFakeFeishu(t, appID, "s")
	secondQueued := make(chan struct{})
	c, rec := protocolChannel(t, f, 25, "s", func(c *Channel) {
		inner := c.newRunner
		c.newRunner = func(cfg *Config, h wsHooks, log larkcore.Logger) wsRunner {
			onMessage := h.onMessage
			h.onMessage = func(ctx context.Context, ev *larkim.P2MessageReceiveV1) error {
				id := deref(ev.Event.Message.MessageId)
				if id == "om_order_1" {
					<-secondQueued // 第一帧的回调等第二帧进了队才跑
				}
				err := onMessage(ctx, ev)
				if id == "om_order_2" {
					close(secondQueued)
				}
				return err
			}
			return inner(cfg, h, log)
		}
	})
	require.Eventually(t, func() bool { return c.LinkState() == notify.LinkConnected }, 10*time.Second, 5*time.Millisecond)

	sent := time.Now().Add(-time.Minute)
	f.pushAt(t, "frame-o1", "om_order_1", "ou_admin", "/delete 1", sent)
	f.pushAt(t, "frame-o2", "om_order_2", "ou_admin", "确认", sent.Add(time.Second))
	acks := []wsAck{f.waitAck(t), f.waitAck(t)}
	assert.ElementsMatch(t, []wsAck{{messageID: "frame-o1", code: http.StatusOK}, {messageID: "frame-o2", code: http.StatusOK}}, acks)
	require.Eventually(t, func() bool { return len(rec.got()) == 2 }, 10*time.Second, 5*time.Millisecond)
	assert.Equal(t, []string{"/delete 1", "确认"}, rec.texts())
}

// 真的 SDK 热重载：一条在处理、一条排着队（已经回了 200）。旧实例关闭、新实例连上以后接着处理排着队的那条，
// 只处理一次；任何时候都只有一条连接。
func TestProtocol_ReloadHandsOffQueuedMessage(t *testing.T) {
	resetProcessState(t)
	appID := uniqueAppID("cli_proto_handoff")
	f := newFakeFeishu(t, appID, "s")
	c1, rec1 := protocolChannel(t, f, 26, "s", nil)
	rec1.block = make(chan struct{}) // 一直不放行，只能靠 ctx 取消
	require.Eventually(t, func() bool { return c1.LinkState() == notify.LinkConnected }, 10*time.Second, 5*time.Millisecond)

	f.push(t, "frame-h1", "om_handoff_1", "ou_admin", "/status")
	assert.Equal(t, http.StatusOK, f.waitAck(t).code)
	require.Eventually(t, func() bool { return len(rec1.got()) == 1 }, 10*time.Second, 5*time.Millisecond, "第一条正在处理")
	f.push(t, "frame-h2", "om_handoff_2", "ou_admin", "/sites")
	assert.Equal(t, http.StatusOK, f.waitAck(t).code, "排着队的已经回了 200，平台不会再推")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, c1.Close(ctx))
	require.Eventually(t, func() bool { return f.connCount() == 0 }, 10*time.Second, 5*time.Millisecond)

	c2, rec2 := protocolChannel(t, f, 26, "s", nil)
	require.Eventually(t, func() bool { return len(rec2.got()) == 1 }, 10*time.Second, 5*time.Millisecond, "新实例接着处理")
	assert.Equal(t, "/sites", rec2.got()[0].Text)
	require.Eventually(t, func() bool { return c2.LinkState() == notify.LinkConnected }, 10*time.Second, 5*time.Millisecond)
	f.mu.Lock()
	assert.Equal(t, 1, f.maxConns, "任何时候都只有一条连接")
	f.mu.Unlock()
	assert.Len(t, rec1.got(), 1)
}
