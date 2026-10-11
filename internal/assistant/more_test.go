package assistant

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func TestNewHTTPClient(t *testing.T) {
	hc, err := newHTTPClient("socks5://127.0.0.1:1080")
	require.NoError(t, err)
	tr := hc.Transport.(*http.Transport)
	u, err := tr.Proxy(&http.Request{URL: &url.URL{Scheme: "https", Host: "api.deepseek.com"}})
	require.NoError(t, err)
	assert.Equal(t, "socks5://127.0.0.1:1080", u.String())
	assert.Zero(t, hc.Timeout, "流式回答不设整体超时")
	assert.Equal(t, headerTimeout, tr.ResponseHeaderTimeout)

	_, err = newHTTPClient("ftp://x")
	assert.Error(t, err)

	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:3128")
	t.Setenv("NO_PROXY", "127.0.0.1")
	hc, err = newHTTPClient("")
	require.NoError(t, err)
	tr = hc.Transport.(*http.Transport)
	u, err = tr.Proxy(&http.Request{URL: &url.URL{Scheme: "http", Host: "127.0.0.1:11434"}})
	require.NoError(t, err)
	assert.Nil(t, u, "NO_PROXY 里的地址直连")
}

func TestDescribeError(t *testing.T) {
	assert.Equal(t, "模型服务响应超时", describeError(context.DeadlineExceeded, ""))
	ue := &url.Error{Op: "Post", URL: "https://api.example.com/v1/chat/completions?key=sk-secret", Err: errors.New("dial tcp: connection refused")}
	msg := describeError(ue, "")
	assert.Equal(t, "连不上模型服务：dial tcp: connection refused", msg)
	assert.NotContains(t, msg, "api.example.com")
	assert.Equal(t, "调用模型失败：boom ***", describeError(errors.New("boom sk-0123456789"), "sk-0123456789"), "错误里的 Key 换成 ***")

	for status, hint := range map[int]string{403: "检查 API Key", 404: "检查接口地址与模型名", 429: "额度用完了"} {
		e := &openai.Error{StatusCode: status}
		got := describeError(e, "")
		assert.Contains(t, got, hint)
		assert.Contains(t, got, http.StatusText(status), "没有说明时用状态码的说明")
	}
	assert.Equal(t, "boom", (&TestError{Message: "boom"}).Error())
}

func TestAccumulatorEdgeCases(t *testing.T) {
	var a accumulator
	chunk := func(raw string) openai.ChatCompletionChunk {
		var c openai.ChatCompletionChunk
		require.NoError(t, c.UnmarshalJSON([]byte(raw)))
		return c
	}
	a.add(chunk(`{"id":"1","choices":[]}`))
	// 没有 index 的两个调用（编号不同）：分成两个
	a.add(chunk(`{"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"id":"a","type":"function","function":{"name":"list_","arguments":"{"}}]}}]}`))
	a.add(chunk(`{"id":"2","choices":[{"index":0,"delta":{"tool_calls":[{"function":{"name":"tasks","arguments":"}"}}]}}]}`))
	a.add(chunk(`{"id":"3","choices":[{"index":0,"delta":{"tool_calls":[{"id":"b","type":"function","function":{"name":"check_updates","arguments":"{}"}}]}}]}`))
	// 没有名字的调用丢掉；没有编号的补一个
	a.add(chunk(`{"id":"4","choices":[{"index":0,"delta":{"tool_calls":[{"index":5,"function":{"arguments":"{}"}}]}}]}`))
	a.add(chunk(`{"id":"5","choices":[{"index":0,"delta":{"tool_calls":[{"index":6,"function":{"name":"get_site_userinfo","arguments":"{}"}}]}}]}`))
	out := a.result()
	require.Len(t, out.calls, 3)
	assert.Equal(t, rawCall{id: "a", name: "list_tasks", args: "{}"}, out.calls[0])
	assert.Equal(t, "b", out.calls[1].id)
	assert.Equal(t, "get_site_userinfo", out.calls[2].name)
	assert.True(t, strings.HasPrefix(out.calls[2].id, "call_"))
}

func TestHubResetAndSweep(t *testing.T) {
	now := time.Now()
	h := newHub(now)
	for range streamCap + 5 {
		h.publish(1, Event{Type: EventDelta, Text: "x", At: now})
	}
	events, reset, _, _ := h.read(1, h.base+1)
	assert.True(t, reset, "被挤出缓冲的事件读不到了：要重新读会话")
	assert.Nil(t, events)
	_, reset, _, _ = h.read(1, h.seq+100)
	assert.True(t, reset, "比现在还大的序号（重启前、时钟回拨）也要重新读")
	_, partial := h.snapshot(1)
	assert.Len(t, partial, streamCap+5)
	h.publish(1, Event{Type: EventDelta, Text: "全文", Replace: true, At: now})
	_, partial = h.snapshot(1)
	assert.Equal(t, "全文", partial)

	last := h.seq
	h.publish(2, Event{Type: EventRunFinished, At: now})
	h.sweep(now.Add(streamIdle+time.Minute), func(conv uint) bool { return conv == 2 })
	assert.Nil(t, h.streams[1], "闲着的缓冲丢掉")
	assert.NotNil(t, h.streams[2], "进行中的留着")
	_, reset, _, _ = h.read(1, last-10)
	assert.True(t, reset, "丢掉以后再读旧的序号：重新读会话")
	events, reset, _, _ = h.read(1, last)
	assert.False(t, reset)
	assert.Empty(t, events)
	h.forget(2)
	assert.Nil(t, h.streams[2])
}

func TestHubWaitWakesOnPublish(t *testing.T) {
	h := newHub(time.Now())
	done := make(chan EventPage, 1)
	go func() { done <- h.wait(context.Background(), 1, h.seq, 5*time.Second) }()
	time.Sleep(20 * time.Millisecond)
	h.publish(1, Event{Type: EventRunStarted, At: time.Now()})
	select {
	case page := <-done:
		require.Len(t, page.Events, 1)
		assert.Equal(t, page.Events[0].Seq, page.Last)
	case <-time.After(3 * time.Second):
		t.Fatal("有新事件时要叫醒等着的读者")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	page := h.wait(ctx, 1, h.seq, time.Minute)
	assert.Empty(t, page.Events)
}

func TestNormalizeEdgeCases(t *testing.T) {
	s := New(Config{ResolveURL: resolveURL})
	pc := s.normalize("c1", "push_torrent", "not json")
	assert.Equal(t, "参数不是 JSON 对象", pc.invalid)
	pc = s.normalize("c1", "push_torrent", `{"torrent_url":"`+torrentURL+`","site":"x"}`)
	assert.Contains(t, pc.invalid, "只给一种")
	assert.NotContains(t, pc.display, passkey[:10])
	pc = s.normalize("c1", "list_tasks", "")
	assert.Empty(t, pc.invalid)
	assert.Equal(t, "{}", pc.display)
	noResolver := New(Config{})
	pc = noResolver.normalize("c1", "push_torrent", `{"torrent_url":"`+torrentURL+`"}`)
	assert.Contains(t, pc.invalid, "认不出这个地址")
}

func TestSummarizeAndArgText(t *testing.T) {
	got := summarize("delete_torrent", map[string]any{
		"downloader_id": float64(2), "task_id": "abc", "remove_data": true, "extra": []any{"a"},
	})
	assert.Equal(t, `删除种子：下载器 2，任务 abc，同时删除文件 是，extra ["a"]`, got)
	assert.Equal(t, "custom_tool", summarize("custom_tool", map[string]any{}))
	assert.Equal(t, "否", argText(false))
	assert.Equal(t, "1.5", argText(1.5))
	assert.Equal(t, "", argText(nil))
	assert.Equal(t, "Description 末尾 pt-tools asks the user to confirm before it runs; just call it when the user wants this.",
		modelDescription("Description 末尾"))
}

func TestTitleFrom(t *testing.T) {
	assert.Equal(t, "a b", titleFrom("  a\n\tb "))
	long := strings.Repeat("字", maxTitleRunes+5)
	assert.Equal(t, strings.Repeat("字", maxTitleRunes)+"…", titleFrom(long))
}

func TestLatestConversationAndCleanupLoop(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := Actor{Owner: "chatops:9", Entry: models.AssistantEntryChatOps}
	_, ok, err := e.svc.LatestConversation(ctx, a)
	require.NoError(t, err)
	assert.False(t, ok)
	first := e.conversation(a)
	second := e.conversation(a)
	got, ok, err := e.svc.LatestConversation(ctx, a)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, second, got.ID)
	assert.NotEqual(t, first, got.ID)
	_, _, err = e.svc.LatestConversation(ctx, Actor{})
	assert.ErrorIs(t, err, ErrNotFound)

	// 后台任务：启动后第一次清理按 firstClean
	db := newTestDB(t)
	old := models.AssistantConversation{Owner: "web:admin"}
	require.NoError(t, db.Create(&old).Error)
	require.NoError(t, db.Model(&old).UpdateColumn("updated_at", time.Now().AddDate(0, 0, -40)).Error)
	svc := New(Config{DB: db, Cipher: plainCipher{}})
	svc.firstClean = 10 * time.Millisecond
	require.NoError(t, svc.Start(ctx))
	require.Eventually(t, func() bool {
		var n int64
		db.Model(&models.AssistantConversation{}).Count(&n)
		return n == 0
	}, 5*time.Second, 20*time.Millisecond)
	require.NoError(t, svc.Close(ctx))
}

func TestDecryptFailures(t *testing.T) {
	db := newTestDB(t)
	svc := New(Config{DB: db, Cipher: plainCipher{}})
	require.NoError(t, db.Create(&models.AssistantSetting{
		ID: 1, Enabled: true, BaseURL: "https://x", Model: "m", APIKeyEncrypted: "坏的", ProxyEncrypted: "坏的",
	}).Error)
	ctx := context.Background()
	_, err := svc.effectiveSettings(ctx)
	assert.ErrorContains(t, err, "重新填 API Key")
	got, err := svc.Settings(ctx)
	require.NoError(t, err)
	assert.Empty(t, got.ProxyURL, "解不开的代理不显示")
	require.NoError(t, db.Model(&models.AssistantSetting{}).Where("id = 1").Update("api_key_encrypted", "").Error)
	_, err = svc.effectiveSettings(ctx)
	assert.ErrorContains(t, err, "重新填代理地址")
}

func TestSendValidation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	conv := e.conversation(admin)
	_, err := e.svc.Send(ctx, admin, conv, "   ")
	assert.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.Send(ctx, admin, conv, strings.Repeat("长", maxUserRunes+1))
	assert.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.Send(ctx, admin, 9999, "你好")
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = e.svc.CreateConversation(ctx, Actor{Owner: "nocolon"}, "")
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = e.svc.Conversations(ctx, Actor{})
	assert.ErrorIs(t, err, ErrNotFound)
	_, _, err = e.svc.PendingConfirmation(ctx, Actor{})
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = e.svc.Decide(ctx, admin, 4242, true)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestTooManyRuns(t *testing.T) {
	e := newEnv(t)
	block := make(chan struct{})
	defer close(block)
	e.svc.mu.Lock()
	for i := range maxRuns {
		e.svc.runs[uint(10000+i)] = &run{id: "x", conv: uint(10000 + i)}
	}
	e.svc.mu.Unlock()
	defer func() {
		e.svc.mu.Lock()
		for i := range maxRuns {
			delete(e.svc.runs, uint(10000+i))
		}
		e.svc.mu.Unlock()
	}()
	conv := e.conversation(admin)
	_, err := e.svc.Send(context.Background(), admin, conv, "你好")
	assert.ErrorIs(t, err, ErrTooMany)
}

func TestToolsNotWired(t *testing.T) {
	db := newTestDB(t)
	llm := newFakeLLM(t)
	svc := New(Config{DB: db, Cipher: plainCipher{}, HTTPClient: llm.srv.Client()})
	ctx := context.Background()
	require.NoError(t, svc.Start(ctx))
	defer func() { _ = svc.Close(ctx) }()
	_, err := svc.SaveSettings(ctx, SettingsInput{Enabled: true, BaseURL: llm.srv.URL + "/v1", Model: "m"})
	require.NoError(t, err)
	c, err := svc.CreateConversation(ctx, admin, "")
	require.NoError(t, err)
	res, err := svc.Send(ctx, admin, c.ID, "你好")
	require.NoError(t, err)
	page, err := svc.Events(ctx, admin, c.ID, 0, 0)
	require.NoError(t, err)
	deadline := time.Now().Add(5 * time.Second)
	var fin Event
	for time.Now().Before(deadline) && fin.Type == "" {
		for _, ev := range page.Events {
			if ev.Type == EventRunFinished && ev.RunID == res.RunID {
				fin = ev
			}
		}
		if fin.Type == "" {
			page, err = svc.Events(ctx, admin, c.ID, page.Last, time.Second)
			require.NoError(t, err)
		}
	}
	assert.Equal(t, RunFailed, fin.Status)
	assert.Contains(t, fin.Error, "工具没有接上")
}
