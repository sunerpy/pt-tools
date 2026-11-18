package feishuapp

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
)

// decodeText 把发出去的 content（{"text":"…"} 的 JSON 字符串）解回文字。
func decodeText(t *testing.T, content string) string {
	t.Helper()
	var body map[string]string
	require.NoError(t, json.Unmarshal([]byte(content), &body), content)
	return body["text"]
}

func TestSend_NotInitialized(t *testing.T) {
	err := New().Send(context.Background(), notify.Notification{Text: "x", UserID: "ou_a"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "没有初始化")
}

func TestSend_AfterCloseRefuses(t *testing.T) {
	c, api, _ := newTestChannel(t, testConfig)
	require.NoError(t, c.Close(context.Background()))
	err := c.Send(context.Background(), notify.Notification{Text: "x", UserID: "ou_a"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "已关闭")
	assert.Empty(t, api.sent())
}

// #838 的教训：长连接断了不代表发不出去，Healthy 为 false 时照常发。
func TestSend_UnhealthyStillSends(t *testing.T) {
	c, api, _ := newTestChannel(t, testConfig)
	c.setWS(wsFailed)
	require.False(t, c.Healthy())
	require.NoError(t, c.Send(context.Background(), notify.Notification{Text: "日报", UserID: "ou_a"}))
	require.Len(t, api.sent(), 1)
}

func TestSend_ReceiveIDTypeAndPrecedence(t *testing.T) {
	c, api, _ := newTestChannel(t, `{"app_id":"cli_test","app_secret":"s","default_chat_id":"on_union"}`)
	ctx := context.Background()

	require.NoError(t, c.Send(ctx, notify.Notification{Text: "a", UserID: "ou_u", Targets: map[string]string{"chat_id": "oc_group"}}))
	require.NoError(t, c.Send(ctx, notify.Notification{Text: "b", UserID: "ou_u"}))
	require.NoError(t, c.Send(ctx, notify.Notification{Text: "c"}))

	sent := api.sent()
	require.Len(t, sent, 3)
	assert.Equal(t, sentText{typ: "chat_id", id: "oc_group", content: `{"text":"a"}`}, sent[0])
	assert.Equal(t, "open_id", sent[1].typ)
	assert.Equal(t, "ou_u", sent[1].id)
	assert.Equal(t, "union_id", sent[2].typ)
	assert.Equal(t, "on_union", sent[2].id)

	err := c.Send(ctx, notify.Notification{Text: "d", UserID: "12345"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oc_、ou_ 或 on_")
}

func TestSend_NoRecipient(t *testing.T) {
	c, api, _ := newTestChannel(t, testConfig)
	err := c.Send(context.Background(), notify.Notification{Text: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "收件人")
	assert.Empty(t, api.sent())
}

func TestSend_ContentEncoding(t *testing.T) {
	c, api, _ := newTestChannel(t, testConfig)
	text := "引号\"、反斜杠\\、换行\n第二行、emoji 🎬、<b>粗</b>、&amp;"
	require.NoError(t, c.Send(context.Background(), notify.Notification{Title: "标题", Text: text, Link: "https://example.com/a?b=1&c=2", UserID: "ou_a"}))
	sent := api.sent()
	require.Len(t, sent, 1)
	assert.Equal(t, "标题\n"+text+"\nhttps://example.com/a?b=1&c=2", decodeText(t, sent[0].content))
}

// 种子标题或模型输出里的 <at user_id="all"></at> 会在群里 @ 所有人，发出去之前换成全角的 ＜at。
func TestSend_NeutralizesAtTags(t *testing.T) {
	c, api, _ := newTestChannel(t, testConfig)
	require.NoError(t, c.Send(context.Background(), notify.Notification{
		Text:   `新种子 <at user_id="all"></at> 与 <AT user_id="ou_x">某人</AT>`,
		UserID: "oc_g",
	}))
	got := decodeText(t, api.sent()[0].content)
	assert.NotContains(t, strings.ToLower(got), "<at")
	assert.Contains(t, got, `＜at user_id="all">`)
	assert.Contains(t, got, `＜AT user_id="ou_x">某人`)
}

func TestSend_TruncatesLongText(t *testing.T) {
	c, api, _ := newTestChannel(t, testConfig)
	long := strings.Repeat("长", 80*1024) // 每个字 3 字节，远超 140 KiB
	require.NoError(t, c.Send(context.Background(), notify.Notification{Text: long, UserID: "ou_a"}))
	content := api.sent()[0].content
	assert.LessOrEqual(t, len(content), maxContentBytes)
	got := decodeText(t, content)
	assert.True(t, strings.HasSuffix(got, truncatedSuffix), "截断后要说明")
	assert.Greater(t, len(got), maxContentBytes/2, "截断不能截得太狠")
}

func TestTextContent_ShortTextUntouched(t *testing.T) {
	assert.Equal(t, `{"text":"a<b>&"}`, textContent("a<b>&"))
}

func TestSend_PerRecipientLimiter(t *testing.T) {
	c, _, _ := newTestChannel(t, testConfig)
	a := c.limiters.get("ou_a")
	assert.Same(t, a, c.limiters.get("ou_a"))
	assert.NotSame(t, a, c.limiters.get("ou_b"))
	assert.InDelta(t, 4, float64(a.Limit()), 0.001)
	assert.Equal(t, 4, a.Burst())
}

func TestLimiterSet_ResetsWhenFull(t *testing.T) {
	s := newLimiterSet(10, 1)
	first := s.get("x")
	for i := range maxLimiters {
		s.get("ou_" + strconv.Itoa(i))
	}
	assert.NotSame(t, first, s.get("x"), "表满了以后重新开始")
}

func TestSend_LimiterWaits(t *testing.T) {
	c, api, _ := newTestChannel(t, testConfig)
	c.limiters = newLimiterSet(50, 1) // 每 20ms 一条
	start := time.Now()
	for range 4 {
		require.NoError(t, c.Send(context.Background(), notify.Notification{Text: "x", UserID: "ou_same"}))
	}
	assert.GreaterOrEqual(t, time.Since(start), 50*time.Millisecond, "同一个收件人要排队")
	assert.Len(t, api.sent(), 4)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.limiters = newLimiterSet(0.001, 1)
	_ = c.limiters.get("ou_slow").Allow()
	err := c.Send(ctx, notify.Notification{Text: "x", UserID: "ou_slow"})
	require.Error(t, err, "等不到令牌就按 ctx 返回")
}

func TestSend_RateLimitedRetriesOnce(t *testing.T) {
	cases := []sendResult{
		{Status: 429, Code: 99991400, Msg: "request trigger frequency limit", RetryAfter: 20 * time.Millisecond},
		{Status: 200, Code: 230020, Msg: "message rate limit", RetryAfter: 20 * time.Millisecond},
	}
	for _, first := range cases {
		c, api, _ := newTestChannel(t, testConfig)
		api.results = []sendResult{first}
		require.NoError(t, c.Send(context.Background(), notify.Notification{Text: "x", UserID: "ou_a"}))
		assert.Len(t, api.sent(), 2, "等一次再发")
	}
}

func TestSend_RateLimitedTooLongReturnsError(t *testing.T) {
	c, api, _ := newTestChannel(t, testConfig)
	api.results = []sendResult{{Status: 429, Code: 99991400, Msg: "limit", RetryAfter: 30 * time.Second}}
	start := time.Now()
	err := c.Send(context.Background(), notify.Notification{Text: "x", UserID: "ou_a"})
	require.Error(t, err)
	assert.Less(t, time.Since(start), time.Second, "超过 5 秒不等，交给 outbox")
	assert.Contains(t, err.Error(), "99991400")
	assert.Contains(t, err.Error(), "太频繁")
	assert.Len(t, api.sent(), 1)
}

func TestSend_RateLimitedBeyondDeadline(t *testing.T) {
	c, api, _ := newTestChannel(t, testConfig)
	api.results = []sendResult{{Status: 429, Code: 99991400, RetryAfter: 2 * time.Second}}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	require.Error(t, c.Send(ctx, notify.Notification{Text: "x", UserID: "ou_a"}))
	assert.Len(t, api.sent(), 1, "ctx 来不及等就不等")
}

func TestSend_RateLimitedTwiceGivesUp(t *testing.T) {
	c, api, _ := newTestChannel(t, testConfig)
	limited := sendResult{Status: 429, Code: 99991400, RetryAfter: 10 * time.Millisecond}
	api.results = []sendResult{limited, limited, limited}
	require.Error(t, c.Send(context.Background(), notify.Notification{Text: "x", UserID: "ou_a"}))
	assert.Len(t, api.sent(), 2, "只重试一次")
}

func TestSend_RateLimitedWithoutResetHeader(t *testing.T) {
	c, api, _ := newTestChannel(t, testConfig)
	api.results = []sendResult{{Status: 429, Code: 99991400}}
	require.Error(t, c.Send(context.Background(), notify.Notification{Text: "x", UserID: "ou_a"}))
	assert.Len(t, api.sent(), 1)
}

func TestSend_ErrorHints(t *testing.T) {
	cases := map[int]string{
		230013:   "可用范围",
		230002:   "不在这个群",
		230006:   "机器人能力",
		230053:   "不再接收",
		99991663: "App ID 或 App Secret",
		99991664: "App ID 或 App Secret",
		99991672: "权限",
		12345:    "",
	}
	for code, hint := range cases {
		c, api, _ := newTestChannel(t, testConfig)
		api.results = []sendResult{{Status: 400, Code: code, Msg: "some msg"}}
		err := c.Send(context.Background(), notify.Notification{Text: "x", UserID: "ou_a"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "code=")
		assert.Contains(t, err.Error(), "some msg")
		if hint != "" {
			assert.Contains(t, err.Error(), hint, code)
		}
	}
}

func TestSend_HTTPErrorWithoutCode(t *testing.T) {
	c, api, _ := newTestChannel(t, testConfig)
	api.results = []sendResult{{Status: 502, Code: codeNotJSON, Msg: "响应不是 JSON"}}
	err := c.Send(context.Background(), notify.Notification{Text: "x", UserID: "ou_a"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 502")
}

// 发送错误会写进日志、通知记录与操作审计：里面不能有 App Secret 与 tenant_access_token。
func TestSend_ErrorsAreRedacted(t *testing.T) {
	c, api, _ := newTestChannel(t, testConfig)
	cause := errors.New(`Post "https://open.feishu.cn/x": token t-g1044ghJRUIJJ5ELPZNLNRIHCAAXQ1 secret sec-ret-value`)
	api.errs = []error{cause}
	err := c.Send(context.Background(), notify.Notification{Text: "x", UserID: "ou_a"})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "sec-ret-value")
	assert.NotContains(t, err.Error(), "t-g1044ghJRUIJJ5ELPZNLNRIHCAAXQ1")
	assert.ErrorIs(t, err, cause, "Unwrap 保留原错误")

	api.results = []sendResult{{Status: 400, Code: 1, Msg: "bad secret sec-ret-value"}}
	err = c.Send(context.Background(), notify.Notification{Text: "x", UserID: "ou_a"})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "sec-ret-value")
}

func TestRedactor(t *testing.T) {
	r := redactor{secret: "abc-secret"}
	got := r.text("x abc-secret t-g1044ghJRUIJJ5ELPZNLNRIHCAAXQ1 http://user:pw@proxy:3128 y")
	assert.Equal(t, "x *** t-*** http://***@proxy:3128 y", got)
	assert.Equal(t, "short", redactor{secret: "ab"}.text("short"), "太短的密钥不替换，以防误换普通文字")
	assert.NoError(t, r.wrap(nil))
}
