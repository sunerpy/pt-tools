package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

func TestWebhook_Send_HappyPath(t *testing.T) {
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":0}`))
	}))
	defer server.Close()

	adapter := &WebhookChannel{}
	conf := &models.NotificationConf{
		ConfigJSON: `{"endpoint_url":"` + server.URL + `","timeout_seconds":5}`,
	}

	err := adapter.Init(context.Background(), conf)
	require.NoError(t, err)

	n := notify.Notification{
		Title: "Test Title",
		Text:  "Test Text",
		Link:  "https://example.com",
	}

	err = adapter.Send(context.Background(), n)
	require.NoError(t, err)

	var payload map[string]interface{}
	err = json.Unmarshal(receivedBody, &payload)
	require.NoError(t, err)

	assert.Equal(t, "notification", payload["event_type"])
	assert.Equal(t, "Test Title", payload["title"])
	assert.Equal(t, "Test Text", payload["text"])
	assert.Equal(t, "https://example.com", payload["link"])
}

func TestWebhook_Send_HMAC(t *testing.T) {
	var receivedSignature string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("X-PT-Signature")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":0}`))
	}))
	defer server.Close()

	adapter := &WebhookChannel{}
	conf := &models.NotificationConf{
		ConfigJSON: `{"endpoint_url":"` + server.URL + `","timeout_seconds":5,"hmac_secret":"s3cr3t"}`,
	}

	err := adapter.Init(context.Background(), conf)
	require.NoError(t, err)

	n := notify.Notification{
		Title: "HMAC Test",
		Text:  "Test Body",
	}

	err = adapter.Send(context.Background(), n)
	require.NoError(t, err)

	assert.NotEmpty(t, receivedSignature, "X-PT-Signature header should be present")
	decoded, err := base64.StdEncoding.DecodeString(receivedSignature)
	assert.NoError(t, err)
	assert.True(t, len(decoded) > 0, "decoded signature should not be empty")
}

func TestWebhook_Send_Timeout(t *testing.T) {
	adapter := &WebhookChannel{}
	conf := &models.NotificationConf{
		ConfigJSON: `{"endpoint_url":"http://127.0.0.1:1/","timeout_seconds":1}`,
	}

	err := adapter.Init(context.Background(), conf)
	require.NoError(t, err)

	n := notify.Notification{
		Title: "Timeout Test",
		Text:  "This should timeout",
	}

	err = adapter.Send(context.Background(), n)
	assert.Error(t, err, "Send should return error on timeout or connection failure")
}

func TestWebhook_4xx_NoRetry(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":400}`))
	}))
	defer server.Close()

	adapter := &WebhookChannel{}
	conf := &models.NotificationConf{
		ConfigJSON: `{"endpoint_url":"` + server.URL + `","timeout_seconds":5}`,
	}

	err := adapter.Init(context.Background(), conf)
	require.NoError(t, err)

	n := notify.Notification{
		Title: "4xx Test",
		Text:  "Business Error",
	}

	err = adapter.Send(context.Background(), n)
	assert.Error(t, err, "Send should return error on 4xx")
	assert.Equal(t, 1, callCount, "Should only call endpoint once (no retry)")
}

func TestWebhook_5xx_Retry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"code":503}`))
	}))
	defer server.Close()

	adapter := &WebhookChannel{}
	conf := &models.NotificationConf{
		ConfigJSON: `{"endpoint_url":"` + server.URL + `","timeout_seconds":5}`,
	}

	err := adapter.Init(context.Background(), conf)
	require.NoError(t, err)

	n := notify.Notification{
		Title: "5xx Test",
		Text:  "Server Error",
	}

	err = adapter.Send(context.Background(), n)
	assert.Error(t, err, "Send should return error on 5xx")
}

// TestWebhook_ChannelSurface pins the trivial interface methods:
// constructor, Type, SupportsInbound, OnInbound (no-op), Close and the
// Healthy transition from uninitialized to initialized.
func TestWebhook_ChannelSurface(t *testing.T) {
	ch := NewWebhookChannel()
	require.NotNil(t, ch)
	assert.Equal(t, "webhook", ch.Type())
	assert.False(t, ch.SupportsInbound())

	wc, ok := ch.(*WebhookChannel)
	require.True(t, ok)
	wc.logger = sLogger()
	assert.False(t, wc.Healthy())

	require.NoError(t, wc.Init(context.Background(), &models.NotificationConf{
		ConfigJSON: `{"endpoint_url":"http://example.com/hook"}`,
	}))
	assert.True(t, wc.Healthy())
	assert.NoError(t, wc.Close(context.Background()))

	// OnInbound only logs a warning; ensure it does not panic.
	wc.OnInbound(func(_ context.Context, _ notify.InboundMessage) error { return nil })
}

// TestWebhook_Init_Errors covers the two Init failure branches and the default
// timeout injection.
func TestWebhook_Init_Errors(t *testing.T) {
	t.Run("malformed json", func(t *testing.T) {
		wc := &WebhookChannel{}
		err := wc.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{bad`})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshal")
	})

	t.Run("missing endpoint_url", func(t *testing.T) {
		wc := &WebhookChannel{}
		err := wc.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{"timeout_seconds":3}`})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "endpoint_url")
	})

	t.Run("default timeout applied", func(t *testing.T) {
		wc := &WebhookChannel{}
		require.NoError(t, wc.Init(context.Background(), &models.NotificationConf{
			ConfigJSON: `{"endpoint_url":"http://x/hook","timeout_seconds":0}`,
		}))
		assert.Equal(t, 5, wc.config.TimeoutSeconds)
	})
}

// TestWebhook_Send_Uninitialized covers the guard when Send is called before a
// successful Init.
func TestWebhook_Send_Uninitialized(t *testing.T) {
	wc := &WebhookChannel{logger: sLogger()}
	err := wc.Send(context.Background(), notify.Notification{Title: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

// TestWebhook_Send_CustomHeaders_And_Payload asserts that configured custom
// headers ride along with the request and the payload envelope carries the
// channel metadata fields.
func TestWebhook_Send_CustomHeaders_And_Payload(t *testing.T) {
	var gotAuth, gotCustom string
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCustom = r.Header.Get("X-Tenant")
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	wc := &WebhookChannel{}
	require.NoError(t, wc.Init(context.Background(), &models.NotificationConf{
		ConfigJSON: `{"endpoint_url":"` + server.URL + `","headers":{"Authorization":"Bearer abc","X-Tenant":"acme"}}`,
	}))

	err := wc.Send(context.Background(), notify.Notification{
		Title:        "T",
		Text:         "B",
		Link:         "https://l",
		ChannelType:  "webhook",
		SourceConfID: 42,
	})
	require.NoError(t, err)

	assert.Equal(t, "Bearer abc", gotAuth)
	assert.Equal(t, "acme", gotCustom)

	var payload WebhookPayload
	require.NoError(t, json.Unmarshal(body, &payload))
	assert.Equal(t, "notification", payload.EventType)
	assert.Equal(t, "T", payload.Title)
	assert.Equal(t, float64(42), payload.Payload["source_conf_id"])
	assert.Equal(t, "webhook", payload.Payload["channel_type"])
}

// TestWebhook_Send_HMAC_Deterministic verifies the X-PT-Signature header equals
// the HMAC-SHA256 of the exact request body under the configured secret.
func TestWebhook_Send_HMAC_Deterministic(t *testing.T) {
	var gotSig string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-PT-Signature")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	wc := &WebhookChannel{}
	require.NoError(t, wc.Init(context.Background(), &models.NotificationConf{
		ConfigJSON: `{"endpoint_url":"` + server.URL + `","hmac_secret":"topsecret"}`,
	}))
	require.NoError(t, wc.Send(context.Background(), notify.Notification{Title: "sig", Text: "body"}))

	require.NotEmpty(t, gotSig)
	// Recompute the expected signature over the received body and compare.
	expected := computeSig(t, "topsecret", gotBody)
	assert.Equal(t, expected, gotSig)
}

// TestWebhook_Registered checks that init() registered the factory.
func TestWebhook_Registered(t *testing.T) {
	ch, err := notify.DefaultRegistry().Make("webhook")
	require.NoError(t, err)
	assert.Equal(t, "webhook", ch.Type())
}

func computeSig(t *testing.T, secret string, body []byte) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
