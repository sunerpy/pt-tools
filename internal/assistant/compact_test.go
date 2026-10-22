package assistant

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// compactSettings 改上下文窗口、自动压缩与阈值（别的设置照旧）。
func (e *env) compactSettings(window int, auto bool, percent int) {
	e.t.Helper()
	key := testKey
	_, err := e.svc.SaveSettings(context.Background(), SettingsInput{
		Enabled: true, Provider: "custom", BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model", APIKey: &key,
		ContextWindow: &window, AutoCompact: &auto, CompactPercent: &percent,
	})
	require.NoError(e.t, err)
}

func systemText(req llmRequest) string {
	if len(req.Messages) == 0 {
		return ""
	}
	s, _ := req.Messages[0]["content"].(string)
	return s
}

// 每次请求都要服务商报用量（stream_options.include_usage）；报的 token 记在助手消息上，会话详情与 usage 事件给出上下文用量与合计。
func TestUsageRecordedAndReported(t *testing.T) {
	e := newEnv(t)
	e.compactSettings(100_000, true, 80)
	e.llm.push(reply{text: "你好", usage: [2]int64{1200, 30}})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "在吗")
	_, all := e.waitRun(admin, conv, res.RunID)

	reqs := e.llm.requests()
	require.Len(t, reqs, 1)
	require.NotNil(t, reqs[0].StreamOptions)
	assert.True(t, reqs[0].StreamOptions.IncludeUsage)

	rows := e.messages(conv)
	require.Equal(t, []string{"user", "assistant"}, roles(rows))
	assert.EqualValues(t, 1200, rows[1].PromptTokens)
	assert.EqualValues(t, 30, rows[1].CompletionTokens)

	var usage *UsageView
	for _, ev := range all {
		if ev.Type == EventUsage {
			usage = ev.Usage
		}
	}
	require.NotNil(t, usage, "回答以后发 usage 事件")
	assert.EqualValues(t, 1230, usage.ContextTokens)
	assert.False(t, usage.Estimated)
	assert.Equal(t, 100_000, usage.ContextWindow)
	assert.Equal(t, windowFromSetting, usage.WindowSource)

	d, err := e.svc.Conversation(context.Background(), admin, conv)
	require.NoError(t, err)
	require.NotNil(t, d.Usage)
	assert.Equal(t, UsageView{
		ContextTokens: 1230, ContextWindow: 100_000, WindowSource: windowFromSetting, AutoCompact: true, CompactPercent: 80,
		InputTokens: 1200, OutputTokens: 30,
	}, *d.Usage)
}

// 不认 stream_options 的兼容服务：去掉它重试一次，这个地址以后不再带；没有用量时按字数估算。
func TestStreamOptionsRejectedRetriesWithout(t *testing.T) {
	e := newEnv(t)
	e.llm.mu.Lock()
	e.llm.noStreamOptions = true
	e.llm.mu.Unlock()
	e.llm.push(reply{text: "第一次"}, reply{text: "第二次"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "一")
	fin, _ := e.waitRun(admin, conv, res.RunID)
	assert.Equal(t, RunDone, fin.Status)
	res = e.send(admin, conv, "二")
	fin, _ = e.waitRun(admin, conv, res.RunID)
	assert.Equal(t, RunDone, fin.Status)

	reqs := e.llm.requests()
	require.Len(t, reqs, 3, "第一次被拒一次、重试一次；第二次不再带")
	assert.NotNil(t, reqs[0].StreamOptions)
	assert.Nil(t, reqs[1].StreamOptions)
	assert.Nil(t, reqs[2].StreamOptions)
	for _, m := range e.messages(conv) {
		assert.NotEqual(t, models.AssistantRoleNotice, m.Role, "去掉 stream_options 不用告诉用户")
	}
	d, err := e.svc.Conversation(context.Background(), admin, conv)
	require.NoError(t, err)
	require.NotNil(t, d.Usage)
	assert.True(t, d.Usage.Estimated)
	assert.Positive(t, d.Usage.ContextTokens)
	assert.Zero(t, d.Usage.InputTokens)
}

// 上下文到了阈值：开始回答以前先让模型写摘要，会话记下摘要与截止的消息，留一条提示；
// 这次请求的历史从这次的用户消息开始，摘要在系统提示里。网页上照旧能看到全部消息。
func TestAutoCompactAtThreshold(t *testing.T) {
	e := newEnv(t)
	e.compactSettings(4096, true, 50)
	e.llm.push(
		reply{text: "站点都正常", usage: [2]int64{2400, 40}},
		reply{text: "- 用户问过站点情况：都正常", usage: [2]int64{500, 20}}, // 写摘要
		reply{text: "下载器也正常", usage: [2]int64{900, 10}},
	)
	conv := e.conversation(admin)
	res := e.send(admin, conv, "站点怎么样")
	e.waitRun(admin, conv, res.RunID)
	res = e.send(admin, conv, "下载器呢")
	_, all := e.waitRun(admin, conv, res.RunID)

	reqs := e.llm.requests()
	require.Len(t, reqs, 3)
	assert.False(t, reqs[1].Stream, "写摘要是一次不带工具的普通请求")
	assert.Empty(t, reqs[1].Tools)
	assert.Contains(t, reqs[1].Raw, "站点怎么样")
	assert.Contains(t, reqs[1].Raw, "站点都正常")
	assert.NotContains(t, reqs[1].Raw, "下载器呢", "这次的用户消息不压缩")

	answer := reqs[2]
	assert.Contains(t, systemText(answer), "前面的对话（已压缩成摘要）")
	assert.Contains(t, systemText(answer), "用户问过站点情况：都正常")
	require.Len(t, answer.Messages, 2, "系统提示加这次的用户消息")
	assert.Equal(t, "user", answer.Messages[1]["role"])
	assert.NotContains(t, answer.Raw, "站点怎么样")

	var c models.AssistantConversation
	require.NoError(t, e.db.First(&c, conv).Error)
	assert.Contains(t, c.Summary, "都正常")
	rows := e.messages(conv)
	assert.Equal(t, []string{"user", "assistant", "user", "notice", "assistant"}, roles(rows))
	assert.Equal(t, rows[1].ID, c.SummaryUntil)
	assert.Contains(t, rows[3].Content, "已经压缩成摘要")
	assert.EqualValues(t, 500, rows[3].PromptTokens, "写摘要用的 token 也算进会话")

	compacting := false
	for _, ev := range all {
		compacting = compacting || ev.Type == EventCompacting
	}
	assert.True(t, compacting, "开始压缩时发 compacting 事件")

	d, err := e.svc.Conversation(context.Background(), admin, conv)
	require.NoError(t, err)
	assert.Len(t, d.Messages, 5, "网页上照旧是全部消息")
	require.NotNil(t, d.Usage)
	assert.True(t, d.Usage.Compacted)
	assert.EqualValues(t, 910, d.Usage.ContextTokens, "压缩以后按最近一次请求报的算")
	assert.EqualValues(t, 2400+500+900, d.Usage.InputTokens)
}

// 窗口很小、压缩完还是超过阈值时：摘要以后的消息不到 minRecompactMessages 条就不再压缩，免得每问一句压一次。
func TestNoRecompactRightAfterCompact(t *testing.T) {
	e := newEnv(t)
	e.compactSettings(4096, true, 50)
	e.llm.push(
		reply{text: "一", usage: [2]int64{3000, 10}},
		reply{text: "- 摘要一", usage: [2]int64{300, 10}}, // 第二句以前压缩
		reply{text: "二", usage: [2]int64{3000, 10}},
		reply{text: "三", usage: [2]int64{3000, 10}},    // 第三句：摘要以后只有两条，不压
		reply{text: "- 摘要二", usage: [2]int64{300, 10}}, // 第四句以前：摘要以后有四条了，压缩
		reply{text: "四", usage: [2]int64{3000, 10}},
	)
	conv := e.conversation(admin)
	for _, q := range []string{"第一句", "第二句", "第三句", "第四句"} {
		res := e.send(admin, conv, q)
		fin, _ := e.waitRun(admin, conv, res.RunID)
		require.Equal(t, RunDone, fin.Status, fin.Error)
	}
	notices := 0
	for _, m := range e.messages(conv) {
		if m.Role == models.AssistantRoleNotice {
			notices++
		}
	}
	assert.Equal(t, 2, notices, "压缩了两次：第二句以前与第四句以前")
	assert.Len(t, e.llm.requests(), 6)
}

// 关了自动压缩：到了阈值也不压缩；历史照旧按预算从最旧的丢起。
func TestNoCompactWhenDisabled(t *testing.T) {
	e := newEnv(t)
	e.compactSettings(4096, false, 50)
	e.llm.push(reply{text: "一", usage: [2]int64{3000, 10}}, reply{text: "二", usage: [2]int64{3100, 10}})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "第一句")
	e.waitRun(admin, conv, res.RunID)
	res = e.send(admin, conv, "第二句")
	e.waitRun(admin, conv, res.RunID)
	reqs := e.llm.requests()
	require.Len(t, reqs, 2, "没有写摘要的请求")
	assert.NotContains(t, systemText(reqs[1]), "已压缩成摘要")
	var c models.AssistantConversation
	require.NoError(t, e.db.First(&c, conv).Error)
	assert.Zero(t, c.SummaryUntil)
}

// 请求因为超过上下文窗口被拒：压缩一次再试（阈值以下也压）。
func TestContextExceededCompactsAndRetries(t *testing.T) {
	e := newEnv(t)
	e.compactSettings(100_000, true, 80)
	e.llm.push(
		reply{text: "第一轮的回答"},
		reply{status: 400, body: "This model's maximum context length is 8192 tokens. However, your messages resulted in 9000 tokens."},
		reply{text: "- 第一轮问过一句"}, // 写摘要
		reply{text: "第二轮的回答"},
	)
	conv := e.conversation(admin)
	res := e.send(admin, conv, "第一轮")
	e.waitRun(admin, conv, res.RunID)
	res = e.send(admin, conv, "第二轮")
	fin, _ := e.waitRun(admin, conv, res.RunID)
	assert.Equal(t, RunDone, fin.Status)
	reqs := e.llm.requests()
	require.Len(t, reqs, 4)
	assert.Contains(t, systemText(reqs[3]), "第一轮问过一句")
	rows := e.messages(conv)
	assert.Equal(t, []string{"user", "assistant", "user", "notice", "assistant"}, roles(rows))
	assert.Contains(t, rows[3].Content, "超过了模型的上下文窗口")
}

// 上下文窗口：设置里填的 > 服务商模型列表报的 > 内置的表 > 默认 128K；自动时最多按 128K 算。
func TestWindowFor(t *testing.T) {
	e := newEnv(t)
	e.llm.mu.Lock()
	e.llm.models = []string{"fake-model", "big-model"}
	e.llm.modelWindows = map[string]int{"fake-model": 32_768, "big-model": 1_000_000}
	e.llm.mu.Unlock()
	ctx := context.Background()
	set, err := e.svc.effectiveSettings(ctx)
	require.NoError(t, err)

	w, src := e.svc.windowFor(set, "fake-model")
	assert.Equal(t, defaultWindow, w)
	assert.Equal(t, windowFromDefault, src)
	w, src = e.svc.windowFor(set, "deepseek-chat")
	assert.Equal(t, 128_000, w)
	assert.Equal(t, windowFromTable, src)
	w, _ = e.svc.windowFor(set, "gemini-2.5-pro")
	assert.Equal(t, autoWindowCap, w, "内置表里很大的也按 128K 算")
	w, src = e.svc.windowFor(set, "openrouter/mistral-small")
	assert.Equal(t, 32_768, w, "带厂商前缀的去掉前缀再认")
	assert.Equal(t, windowFromTable, src)

	_, err = e.svc.ChatModels(ctx)
	require.NoError(t, err)
	w, src = e.svc.windowFor(set, "fake-model")
	assert.Equal(t, 32_768, w)
	assert.Equal(t, windowFromProvider, src)
	w, _ = e.svc.windowFor(set, "big-model")
	assert.Equal(t, autoWindowCap, w)

	set.contextWindow = 200_000
	w, src = e.svc.windowFor(set, "fake-model")
	assert.Equal(t, 200_000, w)
	assert.Equal(t, windowFromSetting, src)
}

func TestCompactSettingsValidation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	key := testKey
	base := SettingsInput{Enabled: true, Provider: "custom", BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model", APIKey: &key}
	for _, bad := range []SettingsInput{
		func() SettingsInput { in := base; in.ContextWindow = ptr(100); return in }(),
		func() SettingsInput { in := base; in.ContextWindow = ptr(20_000_000); return in }(),
		func() SettingsInput { in := base; in.CompactPercent = ptr(30); return in }(),
		func() SettingsInput { in := base; in.CompactPercent = ptr(99); return in }(),
	} {
		_, err := e.svc.SaveSettings(ctx, bad)
		require.ErrorIs(t, err, ErrInvalid)
	}
	in := base
	in.ContextWindow, in.CompactPercent, in.AutoCompact = ptr(64_000), ptr(70), ptr(false)
	got, err := e.svc.SaveSettings(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, 64_000, got.ContextWindow)
	assert.Equal(t, 70, got.CompactPercent)
	assert.False(t, got.AutoCompact)
	// 不带这几项时不改
	got, err = e.svc.SaveSettings(ctx, base)
	require.NoError(t, err)
	assert.Equal(t, 64_000, got.ContextWindow)
	assert.False(t, got.AutoCompact)
}

func TestEstimateAndTranscript(t *testing.T) {
	assert.Equal(t, 1, estimateTokens("abcd"))
	assert.Equal(t, 4, estimateTokens("下载器呢"))
	rows := []models.AssistantMessage{
		{ID: 1, Role: "user", Content: "第一句" + strings.Repeat("长", 3000)},
		{ID: 2, Role: "assistant", Content: "好", ToolCalls: `[{"id":"c1","name":"list_tasks","arguments":"{\"site\":\"hdsky\"}"}]`},
		{ID: 3, Role: "tool", ToolName: "list_tasks", Content: `{"items":[]}`},
	}
	text := compactTranscript(rows, 100_000)
	assert.Contains(t, text, "（后面省略了）", "每条截一截")
	assert.Contains(t, text, "调用工具 list_tasks")
	assert.Contains(t, text, "工具 list_tasks 的结果")
	short := compactTranscript(rows, 50)
	assert.NotContains(t, short, "第一句", "放不下时丢最旧的")
	assert.Contains(t, short, "工具 list_tasks 的结果")
}
