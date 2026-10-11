package assistant

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func ptr[T any](v T) *T { return &v }

func TestSettingsDefaultsAndKeyIsWriteOnly(t *testing.T) {
	svc := New(Config{DB: newTestDB(t), Cipher: plainCipher{}})
	ctx := context.Background()
	got, err := svc.Settings(ctx)
	require.NoError(t, err)
	assert.Equal(t, Settings{MaxSteps: DefaultMaxSteps, ConfirmMinutes: DefaultConfirmMinutes, RetentionDays: DefaultRetentionDays}, got)

	got, err = svc.SaveSettings(ctx, SettingsInput{
		Enabled: true, Provider: "deepseek", BaseURL: " https://api.deepseek.com/ ", Model: "deepseek-flash",
		APIKey: ptr("sk-abcdef12"), ProxyURL: ptr("http://user:pw@127.0.0.1:7890"), ChatOps: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "https://api.deepseek.com", got.BaseURL)
	assert.True(t, got.HasAPIKey)
	assert.Equal(t, "http://user:***@127.0.0.1:7890", got.ProxyURL)
	assert.Equal(t, DefaultRetentionDays, got.RetentionDays)
	var row models.AssistantSetting
	require.NoError(t, svc.cfg.DB.First(&row).Error)
	assert.Equal(t, "enc:sk-abcdef12", row.APIKeyEncrypted, "Key 加密保存")
	assert.Equal(t, "enc:http://user:pw@127.0.0.1:7890", row.ProxyEncrypted)

	// 不给 Key、原样提交打了码的代理：都不改
	got, err = svc.SaveSettings(ctx, SettingsInput{
		Enabled: true, BaseURL: "https://api.deepseek.com", Model: "deepseek-v4-pro", ProxyURL: ptr(got.ProxyURL),
		RetentionDays: ptr(0),
	})
	require.NoError(t, err)
	assert.True(t, got.HasAPIKey)
	assert.Equal(t, 0, got.RetentionDays)
	require.NoError(t, svc.cfg.DB.First(&row).Error)
	assert.Equal(t, "enc:http://user:pw@127.0.0.1:7890", row.ProxyEncrypted)

	// 换了接口地址又没给 Key：存着的 Key 清掉
	got, err = svc.SaveSettings(ctx, SettingsInput{Enabled: true, BaseURL: "http://127.0.0.1:11434/v1", Model: "qwen3"})
	require.NoError(t, err)
	assert.False(t, got.HasAPIKey)
	assert.Equal(t, 0, got.RetentionDays, "没给保留天数不改")

	// 空串清除
	_, err = svc.SaveSettings(ctx, SettingsInput{BaseURL: "http://127.0.0.1:11434/v1", Model: "qwen3", APIKey: ptr("sk-x123456")})
	require.NoError(t, err)
	got, err = svc.SaveSettings(ctx, SettingsInput{BaseURL: "http://127.0.0.1:11434/v1", Model: "qwen3", APIKey: ptr(""), ProxyURL: ptr("")})
	require.NoError(t, err)
	assert.False(t, got.HasAPIKey)
	assert.Empty(t, got.ProxyURL)
}

func TestSettingsValidation(t *testing.T) {
	svc := New(Config{DB: newTestDB(t), Cipher: plainCipher{}})
	ctx := context.Background()
	bad := []SettingsInput{
		{Enabled: true, Model: "m"},
		{Enabled: true, BaseURL: "https://x"},
		{BaseURL: "ftp://x", Model: "m"},
		{BaseURL: "https://user:pw@x", Model: "m"},
		{BaseURL: "https://x/v1?key=1", Model: "m"},
		{BaseURL: "https://x", Model: "has space"},
		{BaseURL: "https://x", Model: "m", APIKey: ptr("sk x")},
		{BaseURL: "https://x", Model: "m", APIKey: ptr("short")},
		{BaseURL: "https://x", Model: "m", ProxyURL: ptr("ftp://p")},
		{BaseURL: "https://x", Model: "m", MaxSteps: 21},
		{BaseURL: "https://x", Model: "m", ConfirmMinutes: 61},
		{BaseURL: "https://x", Model: "m", RetentionDays: ptr(-1)},
	}
	for i, in := range bad {
		_, err := svc.SaveSettings(ctx, in)
		assert.ErrorIs(t, err, ErrInvalid, "第 %d 个", i)
	}
}

func TestStatusAndDisabled(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	st, err := e.svc.Status(ctx, admin)
	require.NoError(t, err)
	assert.Equal(t, Status{Enabled: true, Model: "fake-model", CanConfirm: true}, st)

	_, err = e.svc.SaveSettings(ctx, SettingsInput{BaseURL: e.llm.srv.URL, Model: "fake-model"})
	require.NoError(t, err)
	st, err = e.svc.Status(ctx, admin)
	require.NoError(t, err)
	assert.Equal(t, Status{}, st, "关着的时候不报模型")
	conv := e.conversation(admin)
	_, err = e.svc.Send(ctx, admin, conv, "你好")
	assert.ErrorIs(t, err, ErrDisabled)
}

func TestTestSettings(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.llm.push(reply{text: "OK " + testKey})
	res, err := e.svc.TestSettings(ctx, SettingsInput{BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model"})
	require.NoError(t, err)
	assert.Equal(t, "OK ***", res.Reply, "回显的 Key 也换掉")
	assert.Equal(t, "fake-model", res.Model)
	req := e.llm.requests()[0]
	assert.False(t, req.Stream)
	assert.Equal(t, "Bearer "+testKey, req.Auth, "没给 Key 时用存着的")

	// 换了地址：存着的 Key 不跟过去
	e.llm.push(reply{text: "OK"})
	_, err = e.svc.TestSettings(ctx, SettingsInput{BaseURL: e.llm.srv.URL + "/other", Model: "fake-model"})
	require.NoError(t, err)
	assert.Empty(t, e.llm.requests()[1].Auth)

	e.llm.push(reply{status: 404, body: "model not found"})
	_, err = e.svc.TestSettings(ctx, SettingsInput{BaseURL: e.llm.srv.URL + "/v1", Model: "nope", APIKey: ptr("sk-new-key-000000000000")})
	var te *TestError
	require.True(t, errors.As(err, &te), "%v", err)
	assert.Contains(t, te.Message, "模型服务回 404")
	assert.Contains(t, te.Message, "检查接口地址与模型名")
	assert.False(t, strings.Contains(te.Message, "sk-new-key"))

	got, err := e.svc.Settings(ctx)
	require.NoError(t, err)
	assert.Equal(t, "fake-model", got.Model, "测试不保存")
}
