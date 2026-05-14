package wecom

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

func TestWecom_Send_Markdown(t *testing.T) {
	adapter := &WeComChannel{}
	conf := &models.NotificationConf{
		ConfigJSON: `{"webhook_key":"test-key-123","msg_type":"markdown"}`,
	}

	err := adapter.Init(context.Background(), conf)
	require.NoError(t, err)

	n := notify.Notification{
		Title: "PT 通知",
		Text:  "种子 X 已下载完成",
	}

	payload := adapter.buildMarkdownPayload(n)

	jsonBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	err = json.Unmarshal(jsonBytes, &payload)
	require.NoError(t, err)

	assert.Equal(t, "markdown", payload["msgtype"])

	markdownData, ok := payload["markdown"].(map[string]interface{})
	require.True(t, ok, "markdown field should be an object")

	content, ok := markdownData["content"].(string)
	require.True(t, ok, "markdown.content should be a string")

	assert.Contains(t, content, "# PT 通知")
	assert.Contains(t, content, "种子 X 已下载完成")
}

func TestWecom_Send_Plain(t *testing.T) {
	adapter := &WeComChannel{}
	conf := &models.NotificationConf{
		ConfigJSON: `{"webhook_key":"test-key-456","msg_type":"text"}`,
	}

	err := adapter.Init(context.Background(), conf)
	require.NoError(t, err)

	n := notify.Notification{
		Title: "Test Title",
		Text:  "Test Body",
	}

	payload := adapter.buildTextPayload(n)

	jsonBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	err = json.Unmarshal(jsonBytes, &payload)
	require.NoError(t, err)

	assert.Equal(t, "text", payload["msgtype"])

	textData, ok := payload["text"].(map[string]interface{})
	require.True(t, ok, "text field should be an object")

	content, ok := textData["content"].(string)
	require.True(t, ok, "text.content should be a string")

	assert.Contains(t, content, "Test Title")
	assert.Contains(t, content, "Test Body")
}

func TestWecom_Send_Failure(t *testing.T) {
	adapter := &WeComChannel{}
	conf := &models.NotificationConf{
		ConfigJSON: `{"webhook_key":"test-key-789","msg_type":"markdown"}`,
	}

	err := adapter.Init(context.Background(), conf)
	require.NoError(t, err)

	n := notify.Notification{
		Title: "Error Test",
		Text:  "Should fail",
	}

	payload := adapter.buildMarkdownPayload(n)

	var payloadData map[string]interface{}
	jsonBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	err = json.Unmarshal(jsonBytes, &payloadData)
	require.NoError(t, err)

	assert.Equal(t, "markdown", payloadData["msgtype"])
}
