package qq

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RomiChan/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	zero "github.com/wdvxdr1123/ZeroBot"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

// TestQQAdapter_Init verifies that the adapter parses a valid config_json
// without error and exposes basic capability flags.
func TestQQAdapter_Init(t *testing.T) {
	conf := &models.NotificationConf{
		ID:          1,
		ChannelType: "qq_onebot",
		Name:        "test-qq",
		ConfigJSON: `{
			"listen_addr":"127.0.0.1:0",
			"path":"/onebot/v11/ws",
			"access_token":"secret",
			"admin_qq_users":[10001],
			"allowed_qq_users":[10001,10002]
		}`,
		Enabled: true,
	}

	ch := New()
	defer func() { _ = ch.Close(context.Background()) }()

	err := ch.Init(context.Background(), conf)
	require.NoError(t, err)
	assert.Equal(t, "qq_onebot", ch.Type())
	assert.True(t, ch.SupportsInbound())
}

// TestQQAdapter_PermissionGate_AdminWhitelistAllow verifies that messages
// from an admin/whitelisted QQ user are forwarded to OnInbound, while
// messages from unknown users are silently dropped.
func TestQQAdapter_PermissionGate_AdminWhitelistAllow(t *testing.T) {
	ch := New()
	conf := &models.NotificationConf{
		ID:          7,
		ChannelType: "qq_onebot",
		ConfigJSON: `{
			"listen_addr":"127.0.0.1:0",
			"path":"/onebot/v11/ws",
			"access_token":"",
			"admin_qq_users":[10001],
			"allowed_qq_users":[10002]
		}`,
		Enabled: true,
	}
	require.NoError(t, ch.Init(context.Background(), conf))
	defer func() { _ = ch.Close(context.Background()) }()

	received := make(chan notify.InboundMessage, 2)
	ch.OnInbound(func(_ context.Context, msg notify.InboundMessage) error {
		received <- msg
		return nil
	})

	// Inject events directly via the test hook (admin, whitelisted, unknown).
	allowedEvent := buildEvent(10001, "/torrents qb1")
	whitelistedEvent := buildEvent(10002, "/sites")
	deniedEvent := buildEvent(99999, "/help")

	require.NoError(t, ch.HandleRawEvent(allowedEvent))
	require.NoError(t, ch.HandleRawEvent(whitelistedEvent))
	require.NoError(t, ch.HandleRawEvent(deniedEvent))

	first := requireInboundMessage(t, received)
	second := requireInboundMessage(t, received)
	if first.ChannelUserID == "10002" {
		first, second = second, first
	}
	assert.Equal(t, "10001", first.ChannelUserID)
	assert.Equal(t, "10002", second.ChannelUserID)
	assert.Equal(t, "qq_onebot", first.ChannelType)
	assert.Equal(t, uint(7), first.SourceConfID)
	assertNoInboundMessage(t, received)
}

// TestQQAdapter_LongMessage_Pagination verifies the in-memory paginator:
// 200 lines of input become a single first-page reply with 20 items
// plus the `/next 查看更多` hint, and `/next` advances to the next page.
func TestQQAdapter_LongMessage_Pagination(t *testing.T) {
	pg := newPaginator(20, 5*time.Minute)
	defer pg.Stop()

	lines := make([]string, 0, 200)
	for i := 0; i < 200; i++ {
		lines = append(lines, "row-"+itoa(i))
	}

	page1 := pg.StartOrAdvance("conf-1", "10001", lines)
	require.Contains(t, page1, "row-0")
	require.Contains(t, page1, "row-19")
	require.NotContains(t, page1, "row-20")
	require.Contains(t, page1, "/next 查看更多")

	// `/next` advances to page 2.
	page2 := pg.AdvanceOnly("conf-1", "10001")
	require.Contains(t, page2, "row-20")
	require.Contains(t, page2, "row-39")
	require.NotContains(t, page2, "row-40")
	require.Contains(t, page2, "/next 查看更多")

	// Unrelated user has no session — AdvanceOnly returns empty hint.
	emptyResp := pg.AdvanceOnly("conf-1", "99999")
	require.Empty(t, emptyResp)
}

// TestQQAdapter_OnReconnect_StateCleared verifies pagination state is
// dropped when the websocket connection is reset.
func TestQQAdapter_OnReconnect_StateCleared(t *testing.T) {
	pg := newPaginator(20, 5*time.Minute)
	defer pg.Stop()

	rows := make([]string, 30)
	for i := range rows {
		rows[i] = "x" + itoa(i)
	}

	pg.StartOrAdvance("conf-7", "10001", rows)
	// User waiting on /next.
	require.True(t, pg.HasSession("conf-7", "10001"))

	// Simulate websocket disconnect.
	pg.OnReconnect("conf-7")
	require.False(t, pg.HasSession("conf-7", "10001"),
		"pagination session should be cleared after reconnect")
}

func TestHandleRawEventDispatchesAsync(t *testing.T) {
	ch := New()
	conf := &models.NotificationConf{
		ID:          9,
		ChannelType: "qq_onebot",
		ConfigJSON: `{
			"listen_addr":"127.0.0.1:0",
			"path":"/onebot/v11/ws",
			"admin_qq_users":[10001]
		}`,
		Enabled: true,
	}
	require.NoError(t, ch.Init(context.Background(), conf))
	defer func() { _ = ch.Close(context.Background()) }()

	blocker := make(chan struct{})
	received := make(chan notify.InboundMessage, 1)
	ch.OnInbound(func(_ context.Context, msg notify.InboundMessage) error {
		received <- msg
		<-blocker
		return nil
	})

	payload := buildEvent(10001, "/help")

	// HandleRawEvent must return promptly even though the handler is blocked.
	done := make(chan error, 1)
	go func() { done <- ch.HandleRawEvent(payload) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("HandleRawEvent did not return; handler is still blocking the read path")
	}

	// Verify the handler did get called.
	got := requireInboundMessage(t, received)
	require.Equal(t, "/help", got.Text)

	close(blocker)
}

// Note: TestCallAPI_TimesOutWhenNoResponse omitted — gorilla websocket.Conn
// is not easily mockable; CallAPI requires a concrete *websocket.Conn and the
// timeout path is covered as a defensive runtime guard around production WS I/O.

// --- helpers ---

// buildEvent constructs a OneBot v11 group message JSON payload as bytes.
func buildEvent(userID int64, text string) []byte {
	evt := map[string]interface{}{
		"post_type":    "message",
		"message_type": "group",
		"sub_type":     "normal",
		"self_id":      11111,
		"user_id":      userID,
		"group_id":     20001,
		"message_id":   42,
		"raw_message":  text,
		"message":      text,
		"sender": map[string]interface{}{
			"user_id":  userID,
			"nickname": "user-" + itoa(int(userID)),
		},
	}
	b, _ := json.Marshal(evt)
	return b
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return strings.Clone(string(buf[pos:]))
}

func requireInboundMessage(t *testing.T, received <-chan notify.InboundMessage) notify.InboundMessage {
	t.Helper()
	select {
	case msg := <-received:
		return msg
	case <-time.After(time.Second):
		t.Fatal("handler was never invoked")
	}
	return notify.InboundMessage{}
}

func assertNoInboundMessage(t *testing.T, received <-chan notify.InboundMessage) {
	t.Helper()
	select {
	case msg := <-received:
		t.Fatalf("unexpected inbound message: %#v", msg)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestEventToInboundPropagatesMessageType verifies that the QQ adapter
// preserves the original message_type ("private" or "group") in InboundMessage
// so Reply can route replies correctly via send_private_msg vs send_group_msg.
func TestEventToInboundPropagatesMessageType(t *testing.T) {
	ch := New()
	conf := &models.NotificationConf{
		ID:          1,
		ChannelType: "qq_onebot",
		ConfigJSON: `{
			"listen_addr":"127.0.0.1:0",
			"path":"/onebot/v11/ws",
			"admin_qq_users":[429471838]
		}`,
		Enabled: true,
	}
	require.NoError(t, ch.Init(context.Background(), conf))
	defer func() { _ = ch.Close(context.Background()) }()

	received := make(chan notify.InboundMessage, 2)
	ch.OnInbound(func(_ context.Context, msg notify.InboundMessage) error {
		received <- msg
		return nil
	})

	// Test case (a): private message
	privateEvent := []byte(`{
		"post_type":"message",
		"message_type":"private",
		"user_id":429471838,
		"group_id":0,
		"raw_message":"/help",
		"sender":{"user_id":429471838,"nickname":"TestUser"}
	}`)
	require.NoError(t, ch.HandleRawEvent(privateEvent))

	// Test case (b): group message
	groupEvent := []byte(`{
		"post_type":"message",
		"message_type":"group",
		"user_id":429471838,
		"group_id":522166605,
		"raw_message":"/help",
		"sender":{"user_id":429471838,"nickname":"TestUser"}
	}`)
	require.NoError(t, ch.HandleRawEvent(groupEvent))

	privateMsg := requireInboundMessage(t, received)
	groupMsg := requireInboundMessage(t, received)
	if privateMsg.MessageType == "group" {
		privateMsg, groupMsg = groupMsg, privateMsg
	}

	// Verify private message preservation
	assert.Equal(t, "private", privateMsg.MessageType)
	assert.Equal(t, "429471838", privateMsg.ChatID)

	// Verify group message preservation
	assert.Equal(t, "group", groupMsg.MessageType)
	assert.Equal(t, "522166605", groupMsg.ChatID)
}

// dialAdapter starts a real reverse-WS client against the adapter's bound
// listener, performs the OneBot handshake ({"self_id":...}), and returns the
// live client connection. The adapter side runs wsHandshakeHandler →
// newNapCatCaller → listenCaller as a result.
func dialAdapter(t *testing.T, q *QQChannel, selfID int64, header http.Header) *websocket.Conn {
	t.Helper()
	require.NotNil(t, q.listener, "adapter listener must be bound")
	addr := q.listener.Addr().String()
	path := q.cfg.Path
	if path == "" {
		path = "/onebot/v11/ws"
	}
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = 3 * time.Second
	conn, resp, err := dialer.Dial("ws://"+addr+path, header)
	require.NoError(t, err, "ws dial should succeed")
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, conn.WriteJSON(struct {
		SelfID int64 `json:"self_id"`
	}{SelfID: selfID}))
	return conn
}

// waitCaller polls until the adapter has stored a live napCatCaller (set inside
// wsHandshakeHandler once the {"self_id":...} frame is read). Healthy() alone is
// unreliable here because Init flips it true as soon as the listener binds,
// before any client handshake.
func waitCaller(t *testing.T, q *QQChannel) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if q.activeCaller() != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("adapter never stored a caller after handshake")
}

// TestQQ_WSHandshake_And_Send drives the complete live path: the adapter binds
// a port, a real WS client connects + handshakes, then Send() issues an OneBot
// API call over the socket. The test client reads that request, echoes back a
// success APIResponse, and Send returns nil — exercising wsHandshakeHandler,
// listenCaller, newNapCatCaller, activeCaller, Send, sendOutbound, CallAPI and
// dispatchAPIResponse together.
func TestQQ_WSHandshake_And_Send(t *testing.T) {
	q := New()
	conf := &models.NotificationConf{
		ID:          3,
		ChannelType: "qq_onebot",
		ConfigJSON:  `{"listen_addr":"127.0.0.1:0","path":"/onebot/v11/ws","admin_qq_users":[10001]}`,
		Enabled:     true,
	}
	require.NoError(t, q.Init(context.Background(), conf))
	defer func() { _ = q.Close(context.Background()) }()

	conn := dialAdapter(t, q, 99887766, nil)
	defer func() { _ = conn.Close() }()
	waitCaller(t, q)

	// Server goroutine: read the outbound API request, then send an echo reply.
	type apiReq struct {
		Action string                 `json:"action"`
		Params map[string]interface{} `json:"params"`
		Echo   uint64                 `json:"echo"`
	}
	gotReq := make(chan apiReq, 1)
	go func() {
		var req apiReq
		if err := conn.ReadJSON(&req); err != nil {
			return
		}
		gotReq <- req
		reply := map[string]interface{}{
			"status":  "ok",
			"retcode": 0,
			"echo":    req.Echo,
			"data":    map[string]interface{}{"message_id": 123},
		}
		_ = conn.WriteJSON(reply)
	}()

	n := notify.Notification{
		Title:   "**Hi**",
		Text:    "line `code`",
		Link:    "https://example.com",
		Targets: map[string]string{"chat_id": "20001", "message_type": "group"},
	}
	err := q.Send(context.Background(), n)
	require.NoError(t, err)

	select {
	case req := <-gotReq:
		assert.Equal(t, "send_group_msg", req.Action)
		assert.Contains(t, req.Params, "group_id")
	case <-time.After(2 * time.Second):
		t.Fatal("adapter never sent the outbound API request")
	}
}

// TestQQ_Send_PrivateMessage checks the private-message routing branch of
// sendOutbound (send_private_msg / user_id) end-to-end.
func TestQQ_Send_PrivateMessage(t *testing.T) {
	q := New()
	require.NoError(t, q.Init(context.Background(), &models.NotificationConf{
		ID:         4,
		ConfigJSON: `{"listen_addr":"127.0.0.1:0","admin_qq_users":[1]}`,
	}))
	defer func() { _ = q.Close(context.Background()) }()

	conn := dialAdapter(t, q, 5, nil)
	defer func() { _ = conn.Close() }()
	waitCaller(t, q)

	gotAction := make(chan string, 1)
	go func() {
		var req struct {
			Action string `json:"action"`
			Echo   uint64 `json:"echo"`
		}
		if err := conn.ReadJSON(&req); err != nil {
			return
		}
		gotAction <- req.Action
		_ = conn.WriteJSON(map[string]interface{}{"status": "ok", "retcode": 0, "echo": req.Echo})
	}()

	err := q.Send(context.Background(), notify.Notification{
		Text:    "hello",
		UserID:  "42",
		Targets: map[string]string{"message_type": "private"},
	})
	require.NoError(t, err)
	select {
	case a := <-gotAction:
		assert.Equal(t, "send_private_msg", a)
	case <-time.After(2 * time.Second):
		t.Fatal("no private API request received")
	}
}

// TestQQ_Send_RetCodeError verifies that a non-zero retcode from OneBot maps to
// an error out of sendOutbound.
func TestQQ_Send_RetCodeError(t *testing.T) {
	q := New()
	require.NoError(t, q.Init(context.Background(), &models.NotificationConf{
		ID:         6,
		ConfigJSON: `{"listen_addr":"127.0.0.1:0","admin_qq_users":[1]}`,
	}))
	defer func() { _ = q.Close(context.Background()) }()

	conn := dialAdapter(t, q, 5, nil)
	defer func() { _ = conn.Close() }()
	waitCaller(t, q)

	go func() {
		var req struct {
			Echo uint64 `json:"echo"`
		}
		if err := conn.ReadJSON(&req); err != nil {
			return
		}
		_ = conn.WriteJSON(map[string]interface{}{
			"status":  "failed",
			"retcode": 1404,
			"message": "group not found",
			"echo":    req.Echo,
		})
	}()

	err := q.Send(context.Background(), notify.Notification{
		Text:    "x",
		Targets: map[string]string{"chat_id": "20001", "message_type": "group"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "group not found")
}

// TestQQ_Send_NoCaller ensures Send fails cleanly when NapCat has not yet
// handshaked (activeCaller returns nil).
func TestQQ_Send_NoCaller(t *testing.T) {
	q := New()
	require.NoError(t, q.Init(context.Background(), &models.NotificationConf{
		ID:         8,
		ConfigJSON: `{"listen_addr":"127.0.0.1:0","admin_qq_users":[1]}`,
	}))
	defer func() { _ = q.Close(context.Background()) }()

	err := q.Send(context.Background(), notify.Notification{
		Text:    "x",
		Targets: map[string]string{"chat_id": "20001"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "未连接")
}

// TestQQ_Send_MissingChatID covers the missing-target guard in Send.
func TestQQ_Send_MissingChatID(t *testing.T) {
	q := New()
	require.NoError(t, q.Init(context.Background(), &models.NotificationConf{
		ID:         10,
		ConfigJSON: `{"listen_addr":"127.0.0.1:0"}`,
	}))
	defer func() { _ = q.Close(context.Background()) }()

	err := q.Send(context.Background(), notify.Notification{Text: "no target"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat_id")
}

// TestQQ_Send_InvalidChatID covers the ParseInt failure branch of sendOutbound.
func TestQQ_Send_InvalidChatID(t *testing.T) {
	q := New()
	require.NoError(t, q.Init(context.Background(), &models.NotificationConf{
		ID:         11,
		ConfigJSON: `{"listen_addr":"127.0.0.1:0","admin_qq_users":[1]}`,
	}))
	defer func() { _ = q.Close(context.Background()) }()

	conn := dialAdapter(t, q, 5, nil)
	defer func() { _ = conn.Close() }()
	waitCaller(t, q)

	err := q.Send(context.Background(), notify.Notification{
		Text:    "x",
		Targets: map[string]string{"chat_id": "not-a-number"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "无效的 QQ 目标 ID")
}

// TestQQ_Handshake_TokenRejected verifies checkAccessToken rejects a client
// that omits the configured access_token: the WS upgrade never completes so the
// adapter stays unhealthy.
func TestQQ_Handshake_TokenRejected(t *testing.T) {
	q := New()
	require.NoError(t, q.Init(context.Background(), &models.NotificationConf{
		ID:         12,
		ConfigJSON: `{"listen_addr":"127.0.0.1:0","access_token":"sekret","admin_qq_users":[1]}`,
	}))
	defer func() { _ = q.Close(context.Background()) }()

	addr := q.listener.Addr().String()
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = time.Second
	// No Authorization header → server replies 401 and refuses the upgrade.
	_, resp, err := dialer.Dial("ws://"+addr+"/onebot/v11/ws", nil)
	require.Error(t, err)
	if resp != nil {
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		_ = resp.Body.Close()
	}
	assert.Nil(t, q.activeCaller())
}

// TestQQ_Handshake_TokenAccepted verifies checkAccessToken accepts a matching
// bearer token supplied via the Authorization header.
func TestQQ_Handshake_TokenAccepted(t *testing.T) {
	q := New()
	require.NoError(t, q.Init(context.Background(), &models.NotificationConf{
		ID:         13,
		ConfigJSON: `{"listen_addr":"127.0.0.1:0","access_token":"sekret","admin_qq_users":[1]}`,
	}))
	defer func() { _ = q.Close(context.Background()) }()

	header := http.Header{}
	header.Set("Authorization", "Bearer sekret")
	conn := dialAdapter(t, q, 7, header)
	defer func() { _ = conn.Close() }()
	waitCaller(t, q)
	assert.True(t, q.Healthy())
}

// TestCheckAccessToken table-tests the pure token comparison helper across its
// empty-token, header, query-string and mismatch branches.
func TestCheckAccessToken(t *testing.T) {
	newReq := func(authHeader, queryToken string) *http.Request {
		r, _ := http.NewRequest(http.MethodGet, "http://x/ws?access_token="+queryToken, nil)
		if authHeader != "" {
			r.Header.Set("Authorization", authHeader)
		}
		return r
	}

	cases := []struct {
		name       string
		token      string
		authHeader string
		queryTok   string
		want       int
	}{
		{"no token configured", "", "", "", http.StatusOK},
		{"bearer prefix match", "abc", "Bearer abc", "", http.StatusOK},
		{"raw header match", "abc", "abc", "", http.StatusOK},
		{"query param match", "abc", "", "abc", http.StatusOK},
		{"missing auth", "abc", "", "", http.StatusUnauthorized},
		{"wrong token", "abc", "Bearer nope", "", http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkAccessToken(newReq(tc.authHeader, tc.queryTok), tc.token)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestStripMarkdown asserts the markdown-stripping used before sending plain QQ
// text removes bold/italic/code/strikethrough markers.
func TestStripMarkdown(t *testing.T) {
	got := stripMarkdown("**bold** *ital* `code` ~~strike~~")
	assert.Equal(t, "bold ital code strike", got)
}

// TestParseConfig covers the config parsing helper: default path injection,
// empty-input error and malformed-JSON error.
func TestParseConfig(t *testing.T) {
	t.Run("default path", func(t *testing.T) {
		cfg, err := parseConfig(`{"listen_addr":"127.0.0.1:0"}`)
		require.NoError(t, err)
		assert.Equal(t, "/onebot/v11/ws", cfg.Path)
	})
	t.Run("empty", func(t *testing.T) {
		_, err := parseConfig("   ")
		require.Error(t, err)
	})
	t.Run("malformed", func(t *testing.T) {
		_, err := parseConfig(`{bad`)
		require.Error(t, err)
	})
}

// TestQQ_Init_NilConf and empty listen_addr branches.
func TestQQ_Init_Branches(t *testing.T) {
	t.Run("nil conf", func(t *testing.T) {
		q := New()
		err := q.Init(context.Background(), nil)
		require.Error(t, err)
	})
	t.Run("bad config json", func(t *testing.T) {
		q := New()
		err := q.Init(context.Background(), &models.NotificationConf{ID: 1, ConfigJSON: `{bad`})
		require.Error(t, err)
	})
	t.Run("empty listen_addr skips server", func(t *testing.T) {
		q := New()
		err := q.Init(context.Background(), &models.NotificationConf{ID: 1, ConfigJSON: `{"admin_qq_users":[1]}`})
		require.NoError(t, err)
		assert.Nil(t, q.listener)
		assert.False(t, q.Healthy())
		_ = q.Close(context.Background())
	})
	t.Run("listen_addr in use records startErr", func(t *testing.T) {
		// Bind a first adapter, then a second on the same concrete address to
		// force startServer to fail; Init still returns nil (deferred retry).
		q1 := New()
		require.NoError(t, q1.Init(context.Background(), &models.NotificationConf{
			ID: 1, ConfigJSON: `{"listen_addr":"127.0.0.1:0"}`,
		}))
		defer func() { _ = q1.Close(context.Background()) }()
		addr := q1.listener.Addr().String()

		q2 := New()
		err := q2.Init(context.Background(), &models.NotificationConf{
			ID: 2, ConfigJSON: `{"listen_addr":"` + addr + `"}`,
		})
		require.NoError(t, err)
		assert.Error(t, q2.startErr)
		_ = q2.Close(context.Background())
	})
}

// TestDecodeMessageField covers all three shapes decodeMessageField accepts:
// empty, a plain string, and a OneBot segment array.
func TestDecodeMessageField(t *testing.T) {
	assert.Equal(t, "", decodeMessageField(nil))
	assert.Equal(t, "hello", decodeMessageField(json.RawMessage(`"hello"`)))

	seg := json.RawMessage(`[{"type":"text","data":{"text":"a"}},{"type":"at","data":{"qq":"1"}},{"type":"text","data":{"text":"b"}}]`)
	assert.Equal(t, "ab", decodeMessageField(seg))

	// Neither string nor array → empty.
	assert.Equal(t, "", decodeMessageField(json.RawMessage(`12345`)))
}

// TestQQ_HandleRawEvent_UsesMessageSegments verifies that when raw_message is
// empty the adapter falls back to decoding the structured message field.
func TestQQ_HandleRawEvent_UsesMessageSegments(t *testing.T) {
	q := New()
	require.NoError(t, q.Init(context.Background(), &models.NotificationConf{
		ID:         14,
		ConfigJSON: `{"admin_qq_users":[10001]}`,
	}))
	defer func() { _ = q.Close(context.Background()) }()

	received := make(chan notify.InboundMessage, 1)
	q.OnInbound(func(_ context.Context, msg notify.InboundMessage) error {
		received <- msg
		return nil
	})

	payload := []byte(`{
		"post_type":"message",
		"message_type":"private",
		"user_id":10001,
		"message":[{"type":"text","data":{"text":"/status"}}],
		"sender":{"user_id":10001,"nickname":"seg"}
	}`)
	require.NoError(t, q.HandleRawEvent(payload))

	select {
	case msg := <-received:
		assert.Equal(t, "/status", msg.Text)
	case <-time.After(time.Second):
		t.Fatal("segment-decoded message was not delivered")
	}
}

// TestQQ_HandleRawEvent_NonMessageAndHeartbeat covers the early-return branches:
// heartbeat meta_events and non-message post types are dropped silently.
func TestQQ_HandleRawEvent_NonMessageAndHeartbeat(t *testing.T) {
	q := New()
	require.NoError(t, q.Init(context.Background(), &models.NotificationConf{
		ID:         15,
		ConfigJSON: `{"admin_qq_users":[10001]}`,
	}))
	defer func() { _ = q.Close(context.Background()) }()

	var mu sync.Mutex
	calls := 0
	q.OnInbound(func(_ context.Context, _ notify.InboundMessage) error {
		mu.Lock()
		calls++
		mu.Unlock()
		return nil
	})

	require.NoError(t, q.HandleRawEvent([]byte(`{"post_type":"meta_event","meta_event_type":"heartbeat"}`)))
	require.NoError(t, q.HandleRawEvent([]byte(`{"post_type":"notice","notice_type":"group_increase"}`)))
	require.Error(t, q.HandleRawEvent([]byte(`{bad json`)))

	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	assert.Equal(t, 0, calls)
	mu.Unlock()
}

// TestQQ_HandleRawEvent_NoHandler covers the branch where an allowed message
// arrives but no inbound handler is registered.
func TestQQ_HandleRawEvent_NoHandler(t *testing.T) {
	q := New()
	require.NoError(t, q.Init(context.Background(), &models.NotificationConf{
		ID:         16,
		ConfigJSON: `{"admin_qq_users":[10001]}`,
	}))
	defer func() { _ = q.Close(context.Background()) }()

	// No OnInbound registered.
	require.NoError(t, q.HandleRawEvent(buildEvent(10001, "/orphan")))
}

// TestQQ_ActiveCaller_Nil verifies activeCaller returns nil before handshake.
func TestQQ_ActiveCaller_Nil(t *testing.T) {
	q := New()
	require.NoError(t, q.Init(context.Background(), &models.NotificationConf{
		ID:         17,
		ConfigJSON: `{"listen_addr":"127.0.0.1:0"}`,
	}))
	defer func() { _ = q.Close(context.Background()) }()
	assert.Nil(t, q.activeCaller())
	assert.Equal(t, "conf-17", q.confIDKey())
}

// TestNapCatCaller_ContextCancel verifies CallAPI returns the context error
// when the caller's context is cancelled before any response arrives. A pipe of
// two connected WS endpoints is used so WriteJSON succeeds but no echo is sent.
func TestNapCatCaller_ContextCancel(t *testing.T) {
	q := New()
	require.NoError(t, q.Init(context.Background(), &models.NotificationConf{
		ID:         18,
		ConfigJSON: `{"listen_addr":"127.0.0.1:0","admin_qq_users":[1]}`,
	}))
	defer func() { _ = q.Close(context.Background()) }()

	conn := dialAdapter(t, q, 5, nil)
	defer func() { _ = conn.Close() }()
	waitCaller(t, q)

	// Server drains the request but never replies, so CallAPI must block until
	// the context is cancelled.
	go func() {
		var discard map[string]interface{}
		_ = conn.ReadJSON(&discard)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	c := q.caller.Load()
	require.NotNil(t, c)
	_, err := c.CallAPI(ctx, zero.APIRequest{Action: "get_status"})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestPaginator_GC verifies the sweep loop evicts an expired session when gc is
// invoked with a time past its TTL.
func TestPaginator_GC(t *testing.T) {
	pg := newPaginator(5, time.Hour)
	defer pg.Stop()
	pg.StartOrAdvance("conf-x", "u1", []string{"a", "b", "c", "d", "e", "f"})
	require.True(t, pg.HasSession("conf-x", "u1"))

	// gc with a far-future clock drops the entry.
	pg.gc(time.Now().Add(2 * time.Hour))
	assert.False(t, pg.HasSession("conf-x", "u1"))
}
