package assistant

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

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

	// OpenAI：gpt-audio 支持 Chat Completions 与工具调用，要留着；babbage、davinci 只有旧的 Completions，gpt-realtime 只走 Realtime 接口
	got = chatModels("https://api.openai.com/v1", []string{"gpt-audio", "babbage-002", "davinci-002", "gpt-realtime", "gpt-6-luna"}, "")
	assert.Equal(t, []string{"gpt-6-luna", "gpt-audio"}, got)

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

// Google AI Studio 对不对的 Key 回 400「Please pass a valid API key」，不是 401：照样提示检查 Key。
func TestDescribeErrorRecognizesKeyErrorsWithout401(t *testing.T) {
	e := newEnv(t)
	e.llm.push(reply{status: http.StatusBadRequest, body: "Please pass a valid API key."})
	_, err := e.svc.TestSettings(context.Background(), SettingsInput{BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model"})
	var te *TestError
	require.True(t, errors.As(err, &te), err)
	assert.Contains(t, te.Message, "检查 API Key")
	e.llm.push(reply{status: http.StatusBadRequest, body: "max_tokens is too large"})
	_, err = e.svc.TestSettings(context.Background(), SettingsInput{BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model"})
	require.True(t, errors.As(err, &te), err)
	assert.NotContains(t, te.Message, "检查 API Key", "别的 400 不说是 Key 的问题")
}

// 服务商要等很久（Retry-After 超过 20 秒）时不在这里干等：不重试，直接说要等多久（测试连接、获取模型列表只有 1 分钟）。
func TestLongRetryAfterIsReportedInsteadOfWaited(t *testing.T) {
	e := newEnv(t)
	e.llm.push(reply{status: http.StatusTooManyRequests, body: "Rate limit reached", header: map[string]string{"Retry-After": "90"}})
	start := time.Now()
	_, err := e.svc.TestSettings(context.Background(), SettingsInput{BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model"})
	var te *TestError
	require.True(t, errors.As(err, &te), err)
	assert.Contains(t, te.Message, "约 90 秒后再试")
	assert.Len(t, e.llm.requests(), 1, "要等的比上限长：不重试")
	assert.Less(t, time.Since(start), 10*time.Second)

	// Retry-After 也可以是 HTTP 日期
	at := time.Now().Add(45 * time.Second).UTC().Format(http.TimeFormat)
	e.llm.push(reply{status: http.StatusTooManyRequests, body: "Rate limit reached", header: map[string]string{"Retry-After": at}})
	_, err = e.svc.TestSettings(context.Background(), SettingsInput{BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model"})
	require.True(t, errors.As(err, &te), err)
	assert.Regexp(t, `约 4\d 秒后再试`, te.Message)
}

// SDK 先看 Retry-After-Ms 再看 Retry-After：提示的等待时间要和它用的同一个。
func TestRetryAfterPrefersMilliseconds(t *testing.T) {
	e := newEnv(t)
	e.llm.push(reply{status: http.StatusTooManyRequests, body: "Rate limit reached", header: map[string]string{
		"Retry-After-Ms": "90000", "Retry-After": "1",
	}})
	_, err := e.svc.TestSettings(context.Background(), SettingsInput{BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model"})
	var te *TestError
	require.True(t, errors.As(err, &te), err)
	assert.Contains(t, te.Message, "约 90 秒后再试")
	assert.Len(t, e.llm.requests(), 1, "按毫秒值算要等 90 秒：不重试")
}
