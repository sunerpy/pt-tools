package webhook

import (
	"context"
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
