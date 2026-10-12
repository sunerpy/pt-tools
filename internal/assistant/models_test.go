package assistant

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatModelsKeepsChatModelsOnly(t *testing.T) {
	// Google AI Studio：编号是 models/gemini-…，还列出嵌入、语音、图像、视频模型
	google := "https://generativelanguage.googleapis.com/v1beta/openai"
	got := chatModels(google, []string{
		"models/gemini-3.8-flash", "models/text-embedding-004", "models/gemini-2.5-flash-preview-tts",
		"models/imagen-4.0-generate-001", "models/gemma-3-27b-it", "models/gemini-3.8-flash", "models/aqa",
		"models/veo-3.0-generate-001", "models/gemini-live-2.5-flash", "models/gemini-2.5-flash-image",
	}, "")
	assert.Equal(t, []string{"gemini-3.8-flash", "gemma-3-27b-it"}, got)

	// 别的服务商：去掉语音、嵌入、审核、图像这类不能对话的；去重、去空白、按名字排
	got = chatModels("https://api.groq.com/openai/v1", []string{
		"qwen/qwen3.8-27b", "whisper-large-v3-turbo", "meta-llama/llama-prompt-guard-2-22m", "openai/gpt-oss-120b",
		"canopylabs/orpheus-v1-english", "text-embedding-3-small", "omni-moderation-latest", "dall-e-3", "gpt-image-1",
		"tts-1", "has space", "", "  qwen/qwen3.8-27b  ", "BAAI/bge-reranker-v2-m3",
	}, "")
	assert.Equal(t, []string{"openai/gpt-oss-120b", "qwen/qwen3.8-27b"}, got)

	// 编号里回显了 Key 的不要；太长的不要
	long := string(make([]byte, maxModelLen+1))
	got = chatModels("https://x.example/v1", []string{"ok-model", "leak-" + testKey, long}, testKey)
	assert.Equal(t, []string{"ok-model"}, got)
}

func TestListModelsUsesStoredKeyOnlyForTheSameAddress(t *testing.T) {
	e := newEnv(t)
	e.llm.models = []string{"fake-model", "text-embedding-3-small", "another-model"}
	base := e.llm.srv.URL + "/v1"
	got, err := e.svc.ListModels(context.Background(), SettingsInput{BaseURL: base})
	require.NoError(t, err)
	assert.Equal(t, ModelList{Models: []string{"another-model", "fake-model"}, Total: 3}, got)
	assert.Equal(t, []string{"Bearer " + testKey}, e.llm.modelAuths, "地址没变：用存着的 Key")

	// 换了地址又没给新 Key：存着的 Key 不发过去
	other := newFakeLLM(t)
	other.models = []string{"m1"}
	got, err = e.svc.ListModels(context.Background(), SettingsInput{BaseURL: other.srv.URL + "/v1"})
	require.NoError(t, err)
	assert.Equal(t, []string{"m1"}, got.Models)
	assert.Equal(t, []string{""}, other.modelAuths)

	// 给了新 Key 就用新的
	fresh := "sk-fresh-key-abcdefghij"
	_, err = e.svc.ListModels(context.Background(), SettingsInput{BaseURL: other.srv.URL + "/v1", APIKey: &fresh})
	require.NoError(t, err)
	assert.Equal(t, "Bearer "+fresh, other.modelAuths[1])
}

func TestListModelsChecksInput(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.ListModels(context.Background(), SettingsInput{})
	assert.ErrorIs(t, err, ErrInvalid, "先填接口地址")
	bad := "ftp://127.0.0.1:1"
	_, err = e.svc.ListModels(context.Background(), SettingsInput{BaseURL: e.llm.srv.URL + "/v1", ProxyURL: &bad})
	assert.ErrorIs(t, err, ErrInvalid, "代理地址不对")
}

func TestListModelsErrorIsReadableAndHidesTheKey(t *testing.T) {
	e := newEnv(t)
	e.llm.modelStatus = http.StatusUnauthorized
	_, err := e.svc.ListModels(context.Background(), SettingsInput{BaseURL: e.llm.srv.URL + "/v1"})
	var te *TestError
	require.True(t, errors.As(err, &te), err)
	assert.Contains(t, te.Message, "401")
	assert.Contains(t, te.Message, "检查 API Key")
	assert.NotContains(t, te.Message, testKey)
}

// 服务商不对所在地区开放（Google AI Studio 对中国大陆、香港的 IP 回 400 User location is not supported）：提示填代理。
func TestDescribeErrorSuggestsAProxyForLocationErrors(t *testing.T) {
	e := newEnv(t)
	e.llm.push(reply{status: http.StatusBadRequest, body: "User location is not supported for the API use."})
	_, err := e.svc.TestSettings(context.Background(), SettingsInput{BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model"})
	var te *TestError
	require.True(t, errors.As(err, &te), err)
	assert.Contains(t, te.Message, "代理地址")
}

// 429：限流或免费额度用完；SDK 按 Retry-After 重试两次，还不行就带上建议等多久。
func TestDescribeErrorForRateLimitsWithRetryAfter(t *testing.T) {
	e := newEnv(t)
	limited := reply{status: http.StatusTooManyRequests, body: "Rate limit reached for model", header: map[string]string{"Retry-After": "1"}}
	e.llm.push(limited, limited, limited)
	_, err := e.svc.TestSettings(context.Background(), SettingsInput{BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model"})
	var te *TestError
	require.True(t, errors.As(err, &te), err)
	assert.Contains(t, te.Message, "429")
	assert.Contains(t, te.Message, "免费额度")
	assert.Contains(t, te.Message, "1 秒")
	assert.Len(t, e.llm.requests(), 3, "一次请求加两次重试")
}
