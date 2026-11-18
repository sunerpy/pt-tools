package cmd

import (
	"context"
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
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
)

// 订阅的下载前复核接到 AI 助手：助手没启用时写明去哪里打开；回答看不懂时是 subscribe.ErrReviewUnclear；
// 是与不是带上理由与 token 数。
func TestSubscribeReviewer(t *testing.T) {
	db, err := core.OpenTestDB(t)
	require.NoError(t, err)
	var mu sync.Mutex
	answers := []string{"不是\n这是第一季", "嗯", "是"}
	var prompts []string
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		prompts = append(prompts, string(body))
		answer := answers[0]
		answers = answers[1:]
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"x","object":"chat.completion","created":1,"model":"chat-y","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":%q}}],"usage":{"prompt_tokens":100,"completion_tokens":5,"total_tokens":105}}`, answer)
	}))
	t.Cleanup(llm.Close)
	svc := assistant.New(assistant.Config{DB: db.DB, Cipher: plainTestCipher{}, HTTPClient: llm.Client()})
	require.NoError(t, svc.Start(context.Background()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = svc.Close(ctx)
	})
	review := subscribeReviewer(svc)
	ctx := context.Background()
	in := subscribe.ReviewInput{
		Kind: "tv", Title: "最后生还者", OriginalTitle: "The Last of Us", Year: 2025, Season: 2, Aliases: []string{"TLOU"},
		TorrentTitle: "The.Last.of.Us.S01E02.1080p", TorrentSubtitle: "第一季", Parsed: "英文名 The Last of Us，第 1 季，第 2 集",
	}

	_, err = review(ctx, in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI 助手设置")

	key := "sk-cmd-test-0000000000"
	_, err = svc.SaveSettings(ctx, assistant.SettingsInput{Enabled: true, Provider: "custom", BaseURL: llm.URL + "/v1", Model: "chat-y", APIKey: &key})
	require.NoError(t, err)
	got, err := review(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, subscribe.ReviewResult{Match: false, Reason: "这是第一季", Tokens: 105}, got)
	mu.Lock()
	prompt := prompts[0]
	mu.Unlock()
	for _, want := range []string{"最后生还者", "TLOU", "The.Last.of.Us.S01E02.1080p", "第一季", "英文名 The Last of Us，第 1 季，第 2 集"} {
		assert.True(t, strings.Contains(prompt, want), want)
	}

	_, err = review(ctx, in)
	require.ErrorIs(t, err, subscribe.ErrReviewUnclear)

	got, err = review(ctx, in)
	require.NoError(t, err)
	assert.True(t, got.Match)

	wireSubscribeReview(nil, svc)
	wireSubscribeReview(subscribe.New(subscribe.Config{DB: db.DB}), svc)
}
