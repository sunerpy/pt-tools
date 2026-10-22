package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/internal/assistant"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

type plainTestCipher struct{}

func (plainTestCipher) Encrypt(s string) (string, error) { return "enc:" + s, nil }
func (plainTestCipher) Decrypt(s string) (string, error) { return strings.TrimPrefix(s, "enc:"), nil }

// 签到用的 AI 只在「用 AI 签到」打开、AI 助手启用以后才给；认验证码用签到设置里的看图模型，答题用助手的模型。
func TestAttendanceAI_Solver(t *testing.T) {
	db, err := core.OpenTestDB(t)
	require.NoError(t, err)
	store := core.NewConfigStore(db)
	require.NoError(t, db.DB.Create(&models.SettingsGlobal{DownloadDir: "downloads"}).Error)

	var mu sync.Mutex
	var seenModels []string
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(body, &req)
		mu.Lock()
		seenModels = append(seenModels, req.Model)
		n := len(seenModels)
		mu.Unlock()
		answer := "2"
		if strings.Contains(string(body), "image_url") {
			answer = "CODE01"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"x","object":"chat.completion","created":1,"model":%q,"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":%q}}],"usage":{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d}}`,
			req.Model, answer, 100+n, 3, 103+n)
	}))
	t.Cleanup(llm.Close)
	svc := assistant.New(assistant.Config{DB: db.DB, Cipher: plainTestCipher{}, HTTPClient: llm.Client()})
	require.NoError(t, svc.Start(context.Background()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = svc.Close(ctx)
	})
	src := attendanceAI{store: store, svc: svc}
	ctx := context.Background()

	_, err = src.AttendanceSolver(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "用 AI 签到")

	require.NoError(t, store.SaveAttendanceAI(true, "vision-x"))
	_, err = src.AttendanceSolver(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI 助手设置")

	key := "sk-cmd-test-0000000000"
	_, err = svc.SaveSettings(ctx, assistant.SettingsInput{Enabled: true, Provider: "custom", BaseURL: llm.URL + "/v1", Model: "chat-y", APIKey: &key})
	require.NoError(t, err)
	solver, err := src.AttendanceSolver(ctx)
	require.NoError(t, err)

	// 签到监控放进 ctx 的用量记录：每次模型回答记一次调用，token 照服务商报的加起来
	var usage v2.AttendanceAIUsage
	uctx := v2.WithAttendanceAIUsage(ctx, &usage)
	code, err := solver.ReadCaptcha(uctx, v2.CaptchaImage{Data: []byte("\x89PNG\r\n\x1a\n"), MIME: "image/png", Length: 6})
	require.NoError(t, err)
	assert.Equal(t, "CODE01", code)
	calls, model, prompt, completion := usage.Snapshot()
	assert.Equal(t, 1, calls)
	assert.Equal(t, "vision-x", model)
	assert.EqualValues(t, 101, prompt)
	assert.EqualValues(t, 3, completion)

	picked, err := solver.ChooseAnswer(uctx, v2.AttendanceQuestion{Question: "q", Options: []string{"a", "b", "c"}})
	require.NoError(t, err)
	assert.Equal(t, []int{1}, picked)
	calls, model, prompt, completion = usage.Snapshot()
	assert.Equal(t, 2, calls)
	assert.Equal(t, "chat-y", model)
	assert.EqualValues(t, 203, prompt)
	assert.EqualValues(t, 6, completion)

	// 没有用量记录的 ctx 照常签到
	_, err = solver.ChooseAnswer(ctx, v2.AttendanceQuestion{Question: "q", Options: []string{"a", "b", "c"}})
	require.NoError(t, err)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"vision-x", "chat-y", "chat-y"}, seenModels)
}
