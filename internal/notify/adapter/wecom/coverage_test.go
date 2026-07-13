package wecom

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sunerpy/requests"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

// redirectTarget is consulted by the once-installed middleware to rewrite the
// hardcoded qyapi.weixin.qq.com host onto a local httptest server. Tests set it
// before triggering Send and clear it afterwards.
var (
	redirectMu     sync.Mutex
	redirectTarget *url.URL
	installOnce    sync.Once
)

func installRedirect() {
	installOnce.Do(func() {
		requests.DefaultSession().WithMiddleware(requests.MiddlewareFunc(
			func(req *requests.Request, next requests.Handler) (*requests.Response, error) {
				redirectMu.Lock()
				target := redirectTarget
				redirectMu.Unlock()
				if target != nil && req.URL != nil {
					req.URL.Scheme = target.Scheme
					req.URL.Host = target.Host
				}
				return next(req)
			},
		))
	})
}

func setRedirect(rawURL string) {
	u, _ := url.Parse(rawURL)
	redirectMu.Lock()
	redirectTarget = u
	redirectMu.Unlock()
}

func clearRedirect() {
	redirectMu.Lock()
	redirectTarget = nil
	redirectMu.Unlock()
}

func newWecom(t *testing.T, configJSON string) *WeComChannel {
	t.Helper()
	ch := &WeComChannel{}
	require.NoError(t, ch.Init(context.Background(), &models.NotificationConf{ConfigJSON: configJSON}))
	return ch
}

// TestWecom_Send_Markdown_RealRoundTrip drives the full Send → sendNotification
// → httpclient.Post path against a local server, asserting the produced payload
// shape, the msgtype, and the webhook key that lands in the query string.
func TestWecom_Send_Markdown_RealRoundTrip(t *testing.T) {
	installRedirect()

	var gotBody []byte
	var gotPath, gotKey, gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotPath = r.URL.Path
		gotKey = r.URL.Query().Get("key")
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer server.Close()
	setRedirect(server.URL)
	defer clearRedirect()

	ch := newWecom(t, `{"webhook_key":"KEY-abc","msg_type":"markdown"}`)
	err := ch.Send(context.Background(), notify.Notification{
		Title: "标题A",
		Text:  "正文B",
	})
	require.NoError(t, err)

	assert.Equal(t, "/cgi-bin/webhook/send", gotPath)
	assert.Equal(t, "KEY-abc", gotKey)
	assert.Equal(t, "application/json", gotContentType)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(gotBody, &payload))
	assert.Equal(t, "markdown", payload["msgtype"])
	md, ok := payload["markdown"].(map[string]any)
	require.True(t, ok)
	content, _ := md["content"].(string)
	assert.Contains(t, content, "# 标题A")
	assert.Contains(t, content, "正文B")
}

// TestWecom_Send_Text_RealRoundTrip verifies the text msgtype path produces a
// text payload rather than markdown.
func TestWecom_Send_Text_RealRoundTrip(t *testing.T) {
	installRedirect()

	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errcode":0}`))
	}))
	defer server.Close()
	setRedirect(server.URL)
	defer clearRedirect()

	ch := newWecom(t, `{"webhook_key":"k","msg_type":"text"}`)
	require.NoError(t, ch.Send(context.Background(), notify.Notification{Title: "T", Text: "Body"}))

	var payload map[string]any
	require.NoError(t, json.Unmarshal(gotBody, &payload))
	assert.Equal(t, "text", payload["msgtype"])
	txt, ok := payload["text"].(map[string]any)
	require.True(t, ok)
	content, _ := txt["content"].(string)
	assert.Contains(t, content, "T")
	assert.Contains(t, content, "Body")
}

// TestWecom_Send_4xxError checks that a 4xx provider response surfaces as an
// error containing the status code and body.
func TestWecom_Send_4xxError(t *testing.T) {
	installRedirect()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errcode":40001,"errmsg":"invalid key"}`))
	}))
	defer server.Close()
	setRedirect(server.URL)
	defer clearRedirect()

	ch := newWecom(t, `{"webhook_key":"k","msg_type":"markdown"}`)
	err := ch.Send(context.Background(), notify.Notification{Title: "X", Text: "Y"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "4xx")
	assert.Contains(t, err.Error(), "invalid key")
}

// TestWecom_Send_5xxError checks that a 5xx provider response surfaces as an
// error.
func TestWecom_Send_5xxError(t *testing.T) {
	installRedirect()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`upstream down`))
	}))
	defer server.Close()
	setRedirect(server.URL)
	defer clearRedirect()

	ch := newWecom(t, `{"webhook_key":"k"}`)
	err := ch.Send(context.Background(), notify.Notification{Title: "X", Text: "Y"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "5xx")
}

// TestWecom_Send_DefaultMsgType verifies an empty msg_type defaults to markdown
// (both in Init and the sendNotification switch default branch).
func TestWecom_Send_DefaultMsgType(t *testing.T) {
	installRedirect()

	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	setRedirect(server.URL)
	defer clearRedirect()

	ch := newWecom(t, `{"webhook_key":"k"}`)
	assert.Equal(t, "markdown", ch.msgType)
	require.NoError(t, ch.Send(context.Background(), notify.Notification{Title: "T", Text: "B"}))

	var payload map[string]any
	require.NoError(t, json.Unmarshal(gotBody, &payload))
	assert.Equal(t, "markdown", payload["msgtype"])
}

// TestWecom_Init_Errors covers all Init validation branches.
func TestWecom_Init_Errors(t *testing.T) {
	t.Run("nil conf", func(t *testing.T) {
		ch := &WeComChannel{}
		err := ch.Init(context.Background(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nil")
	})

	t.Run("malformed json", func(t *testing.T) {
		ch := &WeComChannel{}
		err := ch.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{not-json`})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "解析")
	})

	t.Run("empty webhook_key", func(t *testing.T) {
		ch := &WeComChannel{}
		err := ch.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{"webhook_key":""}`})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "webhook_key")
	})

	t.Run("invalid msg_type", func(t *testing.T) {
		ch := &WeComChannel{}
		err := ch.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{"webhook_key":"k","msg_type":"card"}`})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "msg_type")
	})
}

// TestWecom_ChannelSurface exercises the trivial interface methods so their
// behaviour is pinned.
func TestWecom_ChannelSurface(t *testing.T) {
	ch := newWecom(t, `{"webhook_key":"k"}`)
	assert.Equal(t, "wecom_webhook", ch.Type())
	assert.False(t, ch.SupportsInbound())
	assert.True(t, ch.Healthy())
	assert.NoError(t, ch.Close(context.Background()))

	// OnInbound is a no-op for a push-only channel; calling it must not panic
	// nor register anything observable.
	ch.OnInbound(func(_ context.Context, _ notify.InboundMessage) error { return nil })
}

// TestWecom_Registered ensures the init() side-effect registered the factory in
// the default registry and that it yields a usable channel.
func TestWecom_Registered(t *testing.T) {
	ch, err := notify.DefaultRegistry().Make("wecom_webhook")
	require.NoError(t, err)
	require.NotNil(t, ch)
	assert.Equal(t, "wecom_webhook", ch.Type())
}
