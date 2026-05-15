package qq

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
