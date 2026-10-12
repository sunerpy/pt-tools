package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/mcp"
	"github.com/sunerpy/pt-tools/models"
)

var writeTools = []string{"pause_torrent", "resume_torrent", "delete_torrent", "push_torrent", "add_subscription"}

func TestDirectAnswer(t *testing.T) {
	e := newEnv(t)
	e.llm.push(reply{text: "你好，我是 pt-tools 的助手。"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "你好")
	fin, events := e.waitRun(admin, conv, res.RunID)
	assert.Equal(t, RunDone, fin.Status)

	assert.Equal(t, []string{"user", "assistant"}, roles(e.messages(conv)))
	assert.Equal(t, "你好，我是 pt-tools 的助手。", e.messages(conv)[1].Content)
	var types []string
	var streamed strings.Builder
	for _, ev := range events {
		types = append(types, ev.Type)
		if ev.Type == EventDelta {
			streamed.WriteString(ev.Text)
		}
	}
	assert.Equal(t, EventRunStarted, types[0])
	assert.Contains(t, types, EventDelta)
	assert.Equal(t, EventRunFinished, types[len(types)-1])
	assert.True(t, strings.HasPrefix("你好，我是 pt-tools 的助手。", streamed.String()), "增量拼起来是回答的开头：%q", streamed.String())

	reqs := e.llm.requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "Bearer "+testKey, reqs[0].Auth)
	assert.Equal(t, "fake-model", reqs[0].Model)
	assert.True(t, reqs[0].Stream)
	assert.Equal(t, "system", reqs[0].Messages[0]["role"])
	assert.Equal(t, "你好", reqs[0].Messages[1]["content"])
	assert.ElementsMatch(t, mcp.ToolNames, reqs[0].toolNames(), "能写的主体拿到全部工具")
	for _, tool := range reqs[0].Tools {
		props, _ := tool.Function.Parameters["properties"].(map[string]any)
		assert.NotContains(t, props, "confirm", "给模型的 %s 没有 confirm 参数", tool.Function.Name)
		required, _ := tool.Function.Parameters["required"].([]any)
		assert.NotContains(t, required, "confirm")
		assert.NotContains(t, tool.Function.Description, "confirm=true")
	}
	assert.NotContains(t, reqs[0].Raw, "temperature")

	d, err := e.svc.Conversation(context.Background(), admin, conv)
	require.NoError(t, err)
	assert.Equal(t, "你好", d.Title)
	assert.False(t, d.Running)
	assert.Len(t, d.Messages, 2)
}

func TestReadToolThenAnswer(t *testing.T) {
	e := newEnv(t)
	e.llm.push(call("call_1", "list_tasks", `{"page":1,"keyword":"Fake"}`), reply{text: "找到 1 个种子：Fake.Movie.2026。"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "最近抓了什么种子")
	fin, events := e.waitRun(admin, conv, res.RunID)
	require.Equal(t, RunDone, fin.Status, fin.Error)

	reqs := e.backend.requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "/tasks?page=1&q=Fake", reqs[0].Path)
	assert.Equal(t, mcp.KindAssistant, e.backend.callers[0].Kind)
	assert.Equal(t, "web:admin", e.backend.callers[0].Subject)

	msgs := e.messages(conv)
	assert.Equal(t, []string{"user", "assistant", "tool", "assistant"}, roles(msgs))
	assert.Contains(t, msgs[1].ToolCalls, `"name":"list_tasks"`)
	assert.Equal(t, "call_1", msgs[2].ToolCallID)
	assert.Contains(t, msgs[2].Content, "Fake.Movie.2026")

	second := e.llm.requests()[1]
	assistant := second.Messages[2]
	assert.Equal(t, "assistant", assistant["role"])
	calls := assistant["tool_calls"].([]any)
	require.Len(t, calls, 1)
	fn := calls[0].(map[string]any)["function"].(map[string]any)
	assert.Equal(t, "list_tasks", fn["name"])
	assert.JSONEq(t, `{"keyword":"Fake","page":1}`, fn["arguments"].(string))
	assert.Equal(t, "tool", second.Messages[3]["role"])
	assert.Equal(t, "call_1", second.Messages[3]["tool_call_id"])

	started := 0
	for _, ev := range events {
		if ev.Type == EventToolStarted {
			started++
			assert.Equal(t, "list_tasks", ev.Tool.Name)
		}
	}
	assert.Equal(t, 1, started)
}

// startWrite 让模型调一个写工具，等到待确认事件出现。
func startWrite(t *testing.T, e *env, args string) (uint, SendResult, ConfirmationView) {
	t.Helper()
	e.llm.push(call("call_w", "pause_torrent", args), reply{text: "处理好了。"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "暂停那个种子")
	events, _ := e.until(admin, conv, 0, pendingConfirmation)
	c := *events[len(events)-1].Confirmation
	return conv, res, c
}

func TestWriteToolApproved(t *testing.T) {
	e := newEnv(t)
	conv, res, c := startWrite(t, e, `{"downloader_id":1,"task_id":"abc"}`)
	assert.Equal(t, "pause_torrent", c.Tool)
	assert.Equal(t, "暂停种子：下载器 1，任务 abc", c.Summary)
	assert.Empty(t, e.backend.writes(), "确认以前不执行")

	got, err := e.svc.Decide(context.Background(), admin, c.ID, true)
	require.NoError(t, err)
	assert.Equal(t, models.AssistantConfirmApproved, got.Status)
	assert.Equal(t, "web:admin", got.DecidedBy)
	fin, events := e.waitRun(admin, conv, res.RunID)
	require.Equal(t, RunDone, fin.Status, fin.Error)

	writes := e.backend.writes()
	require.Len(t, writes, 1)
	assert.Equal(t, "/torrents/actions", writes[0].Path)
	audits := e.audit.all()
	require.Len(t, audits, 1)
	assert.Equal(t, "assistant", audits[0].ChannelType)
	assert.Equal(t, "web:admin", audits[0].ChannelUserID)
	assert.Equal(t, "pause_torrent", audits[0].Command)
	assert.Equal(t, "success", audits[0].Result)

	var statuses []string
	for _, ev := range events {
		if ev.Type == EventConfirmation {
			statuses = append(statuses, ev.Confirmation.Status)
		}
	}
	assert.Equal(t, []string{"pending", "approved"}, statuses)
	msgs := e.messages(conv)
	assert.Equal(t, []string{"user", "assistant", "tool", "assistant"}, roles(msgs))
	assert.Contains(t, msgs[2].Content, `"ok":true`)

	_, err = e.svc.Decide(context.Background(), admin, c.ID, true)
	assert.ErrorIs(t, err, ErrDecided, "有结论以后再确认回 ErrDecided")
}

func TestWriteToolDenied(t *testing.T) {
	e := newEnv(t)
	conv, res, c := startWrite(t, e, `{"downloader_id":1,"task_id":"abc"}`)
	_, err := e.svc.Decide(context.Background(), admin, c.ID, false)
	require.NoError(t, err)
	fin, _ := e.waitRun(admin, conv, res.RunID)
	require.Equal(t, RunDone, fin.Status)
	assert.Empty(t, e.backend.writes())
	assert.Empty(t, e.audit.all())
	msgs := e.messages(conv)
	assert.Contains(t, msgs[2].Content, "用户拒绝")
	// 交回模型的工具结果也是拒绝
	last := e.llm.requests()[1].Messages
	assert.Contains(t, last[len(last)-1]["content"], "用户拒绝")
}

func TestWriteToolTimeout(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.SaveSettings(context.Background(), SettingsInput{
		Enabled: true, BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model", ConfirmMinutes: 1,
	})
	require.NoError(t, err)
	conv, res, c := startWrite(t, e, `{"downloader_id":1,"task_id":"abc"}`)
	fin, _ := e.waitRun(admin, conv, res.RunID)
	require.Equal(t, RunDone, fin.Status)
	assert.Empty(t, e.backend.writes())
	var row models.AssistantConfirmation
	require.NoError(t, e.db.First(&row, c.ID).Error)
	assert.Equal(t, models.AssistantConfirmExpired, row.Status)
	assert.Equal(t, decidedByTimeout, row.DecidedBy)
	assert.Contains(t, e.messages(conv)[2].Content, "没有在 1 分钟内确认")
	_, err = e.svc.Decide(context.Background(), admin, c.ID, true)
	assert.ErrorIs(t, err, ErrDecided, "超时以后再确认不算")
}

func TestConcurrentDecideRunsOnce(t *testing.T) {
	e := newEnv(t)
	e.svc.confirmUnit = time.Minute
	conv, res, c := startWrite(t, e, `{"downloader_id":1,"task_id":"abc"}`)
	var wins, decided atomic.Int32
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.svc.Decide(context.Background(), admin, c.ID, i%2 == 0)
			switch {
			case err == nil:
				wins.Add(1)
			case errors.Is(err, ErrDecided):
				decided.Add(1)
			default:
				t.Errorf("意外的错误: %v", err)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(1), wins.Load(), "只有一次确认算数")
	assert.Equal(t, int32(19), decided.Load())
	fin, _ := e.waitRun(admin, conv, res.RunID)
	require.Equal(t, RunDone, fin.Status)
	var row models.AssistantConfirmation
	require.NoError(t, e.db.First(&row, c.ID).Error)
	want := 0
	if row.Status == models.AssistantConfirmApproved {
		want = 1
	}
	assert.Len(t, e.backend.writes(), want, "按库里赢下来的结论执行，最多一次")
}

func TestDecideRacingTimeout(t *testing.T) {
	for i := range 8 {
		e := newEnv(t)
		e.svc.confirmUnit = 30 * time.Millisecond
		_, err := e.svc.SaveSettings(context.Background(), SettingsInput{
			Enabled: true, BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model", ConfirmMinutes: 1,
		})
		require.NoError(t, err)
		conv, res, c := startWrite(t, e, `{"downloader_id":1,"task_id":"abc"}`)
		time.Sleep(time.Duration(20+i*2) * time.Millisecond) // 让确认与超时撞在一起
		_, derr := e.svc.Decide(context.Background(), admin, c.ID, true)
		fin, _ := e.waitRun(admin, conv, res.RunID)
		require.Equal(t, RunDone, fin.Status)
		var row models.AssistantConfirmation
		require.NoError(t, e.db.First(&row, c.ID).Error)
		switch row.Status {
		case models.AssistantConfirmApproved:
			assert.NoError(t, derr)
			assert.Len(t, e.backend.writes(), 1)
		case models.AssistantConfirmExpired:
			assert.ErrorIs(t, derr, ErrDecided)
			assert.Empty(t, e.backend.writes())
		default:
			t.Fatalf("意外的状态 %s", row.Status)
		}
	}
}

func TestMultipleWritesConfirmedOneByOne(t *testing.T) {
	e := newEnv(t)
	e.svc.confirmUnit = time.Minute
	e.llm.push(reply{calls: []replyCall{
		{id: "c1", name: "pause_torrent", args: `{"downloader_id":1,"task_id":"a"}`},
		{id: "c2", name: "list_tasks", args: `{}`},
		{id: "c3", name: "resume_torrent", args: `{"downloader_id":1,"task_id":"b"}`},
	}}, reply{text: "都处理好了。"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "暂停 a，继续 b")
	events, after := e.until(admin, conv, 0, pendingConfirmation)
	first := *events[len(events)-1].Confirmation
	assert.Equal(t, "c1", first.ToolCallID)
	var pending int64
	require.NoError(t, e.db.Model(&models.AssistantConfirmation{}).Where("status = ?", "pending").Count(&pending).Error)
	assert.Equal(t, int64(1), pending, "一次只挂一个待确认")

	_, err := e.svc.Decide(context.Background(), admin, first.ID, true)
	require.NoError(t, err)
	events, _ = e.until(admin, conv, after, pendingConfirmation)
	second := *events[len(events)-1].Confirmation
	assert.Equal(t, "c3", second.ToolCallID)
	require.Len(t, e.backend.writes(), 1, "第二个写操作在确认以前不执行")
	_, err = e.svc.Decide(context.Background(), admin, second.ID, true)
	require.NoError(t, err)
	fin, _ := e.waitRun(admin, conv, res.RunID)
	require.Equal(t, RunDone, fin.Status)

	var paths []string
	for _, r := range e.backend.requests() {
		paths = append(paths, r.Path)
	}
	assert.Equal(t, []string{"/torrents/actions", "/tasks", "/torrents/actions"}, paths, "按顺序执行")
	w := e.backend.writes()
	assert.Equal(t, "pause", w[0].Body.(map[string]any)["action"])
	assert.Equal(t, "resume", w[1].Body.(map[string]any)["action"])
}

func TestModelConfirmIgnored(t *testing.T) {
	e := newEnv(t)
	e.svc.confirmUnit = time.Minute
	conv, res, c := startWrite(t, e, `{"downloader_id":1,"task_id":"abc","confirm":true}`)
	assert.Empty(t, e.backend.writes(), "模型自己带 confirm=true 也要等人确认")
	assert.NotContains(t, c.Arguments, "confirm")
	require.NoError(t, e.svc.Cancel(context.Background(), admin, conv))
	fin, _ := e.waitRun(admin, conv, res.RunID)
	assert.Equal(t, RunCanceled, fin.Status)
	assert.Empty(t, e.backend.writes())
	var row models.AssistantConfirmation
	require.NoError(t, e.db.First(&row, c.ID).Error)
	assert.Equal(t, models.AssistantConfirmExpired, row.Status)
	assert.Equal(t, decidedByCancel, row.DecidedBy)
}

func TestReadOnlyActorHasNoWriteTools(t *testing.T) {
	e := newEnv(t)
	reader := Actor{Owner: "token:7", Entry: models.AssistantEntryApp, Name: "只读令牌"}
	e.llm.push(call("c1", "pause_torrent", `{"downloader_id":1,"task_id":"abc"}`), reply{text: "这个入口不能暂停种子。"})
	conv := e.conversation(reader)
	res := e.send(reader, conv, "暂停那个种子")
	fin, _ := e.waitRun(reader, conv, res.RunID)
	require.Equal(t, RunDone, fin.Status)
	names := e.llm.requests()[0].toolNames()
	for _, w := range writeTools {
		assert.NotContains(t, names, w)
	}
	assert.Contains(t, names, "list_tasks")
	assert.Empty(t, e.backend.requests())
	var n int64
	require.NoError(t, e.db.Model(&models.AssistantConfirmation{}).Count(&n).Error)
	assert.Zero(t, n)
	assert.Contains(t, e.messages(conv)[2].Content, "没有这个工具")
	assert.Contains(t, e.llm.requests()[0].Messages[0]["content"], "只能查询")

	st, err := e.svc.Status(context.Background(), reader)
	require.NoError(t, err)
	assert.True(t, st.Enabled)
	assert.False(t, st.CanConfirm)
}

func TestPushTorrentURLNeverLeaks(t *testing.T) {
	e := newEnv(t)
	e.svc.confirmUnit = time.Minute
	e.llm.push(call("c1", "push_torrent", `{"torrent_url":"`+torrentURL+`","downloader_id":1}`), reply{text: "推送好了。"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "下载这个 "+torrentURL+" 谢谢")
	events, _ := e.until(admin, conv, 0, pendingConfirmation)
	c := *events[len(events)-1].Confirmation
	assert.Equal(t, map[string]any{"site": "hdsky", "torrent_id": "123", "downloader_id": float64(1)}, c.Arguments)
	assert.Equal(t, "推送种子到下载器：站点 hdsky，种子编号 123，下载器 1", c.Summary)
	_, err := e.svc.Decide(context.Background(), admin, c.ID, true)
	require.NoError(t, err)
	fin, all := e.waitRun(admin, conv, res.RunID)
	require.Equal(t, RunDone, fin.Status, fin.Error)

	w := e.backend.writes()
	require.Len(t, w, 1)
	assert.Equal(t, "/push", w[0].Path)
	assert.Equal(t, "hdsky", w[0].Body.(map[string]any)["site"])
	assert.Equal(t, "123", w[0].Body.(map[string]any)["torrent_id"])

	// 交给模型的、落库的、事件里的、待确认记录、审计：都没有 passkey
	for _, r := range e.llm.requests() {
		assert.NotContains(t, r.Raw[strings.Index(r.Raw, `"messages"`):], passkey[:12])
	}
	assert.Contains(t, e.llm.requests()[0].Messages[1]["content"], "「站点 hdsky 的种子 123」")
	buf, _ := json.Marshal(all)
	assert.NotContains(t, string(buf), passkey[:12])
	for _, m := range e.messages(conv) {
		assert.NotContains(t, m.Content+m.ToolCalls+m.Reasoning, passkey[:12])
	}
	var confs []models.AssistantConfirmation
	require.NoError(t, e.db.Find(&confs).Error)
	for _, cf := range confs {
		assert.NotContains(t, cf.Arguments, passkey[:12])
	}
	buf, _ = json.Marshal(e.audit.all())
	assert.NotContains(t, string(buf), passkey[:12])
	d, err := e.svc.Conversation(context.Background(), admin, conv)
	require.NoError(t, err)
	buf, _ = json.Marshal(d)
	assert.NotContains(t, string(buf), passkey[:12])
}

func TestUnknownTorrentURLIsToolError(t *testing.T) {
	e := newEnv(t)
	e.llm.push(call("c1", "push_torrent", `{"torrent_url":"https://unknown.example/dl?id=1&passkey=`+passkey+`"}`), reply{text: "认不出。"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "下载")
	fin, all := e.waitRun(admin, conv, res.RunID)
	require.Equal(t, RunDone, fin.Status)
	assert.Empty(t, e.backend.requests())
	tool := e.messages(conv)[2]
	assert.True(t, tool.IsError)
	assert.Contains(t, tool.Content, "认不出这个地址")
	buf, _ := json.Marshal(all)
	assert.NotContains(t, string(buf), passkey[:12])
	assert.NotContains(t, e.messages(conv)[1].ToolCalls, passkey[:12])
}

func TestStepLimit(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.SaveSettings(context.Background(), SettingsInput{
		Enabled: true, BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model", MaxSteps: 2,
	})
	require.NoError(t, err)
	e.llm.push(call("c1", "list_tasks", `{}`), call("c2", "list_tasks", `{}`))
	conv := e.conversation(admin)
	res := e.send(admin, conv, "一直查")
	fin, _ := e.waitRun(admin, conv, res.RunID)
	require.Equal(t, RunDone, fin.Status)
	msgs := e.messages(conv)
	last := msgs[len(msgs)-1]
	assert.Equal(t, models.AssistantRoleNotice, last.Role)
	assert.Contains(t, last.Content, "调用了 2 轮工具")
	assert.Len(t, e.llm.requests(), 2)
}

func TestCancelAndBusy(t *testing.T) {
	e := newEnv(t)
	block := make(chan struct{})
	defer close(block)
	e.llm.push(reply{text: "我想想……再想想", block: block}, reply{text: "第二次"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "第一个问题")
	<-e.llm.started
	_, err := e.svc.Send(context.Background(), admin, conv, "插一句")
	assert.ErrorIs(t, err, ErrBusy)
	d, err := e.svc.Conversation(context.Background(), admin, conv)
	require.NoError(t, err)
	assert.True(t, d.Running)
	assert.Equal(t, res.RunID, d.RunID)

	require.NoError(t, e.svc.Cancel(context.Background(), admin, conv))
	fin, _ := e.waitRun(admin, conv, res.RunID)
	assert.Equal(t, RunCanceled, fin.Status)
	msgs := e.messages(conv)
	assert.Equal(t, "已停止回答。", msgs[len(msgs)-1].Content)

	res2 := e.send(admin, conv, "第二个问题")
	fin, _ = e.waitRun(admin, conv, res2.RunID)
	assert.Equal(t, RunDone, fin.Status)
	// 停止的那次没有回答，交给模型的历史里也没有「已停止」的提示
	hist := e.llm.requests()[1].Messages
	for _, m := range hist {
		assert.NotEqual(t, "已停止回答。", m["content"])
	}
}

func TestToolResultTruncated(t *testing.T) {
	e := newEnv(t)
	e.backend.big = true
	e.llm.push(call("c1", "list_tasks", `{}`), reply{text: "太多了。"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "列出全部")
	fin, _ := e.waitRun(admin, conv, res.RunID)
	require.Equal(t, RunDone, fin.Status)
	tool := e.messages(conv)[2]
	assert.LessOrEqual(t, len(tool.Content), maxToolResult+100)
	assert.Contains(t, tool.Content, "结果太长，只保留了前 16 KB")
}

func TestModelErrorDoesNotLeakKey(t *testing.T) {
	e := newEnv(t)
	e.llm.push(reply{status: 401, body: "Incorrect API key provided: " + testKey})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "你好")
	fin, all := e.waitRun(admin, conv, res.RunID)
	assert.Equal(t, RunFailed, fin.Status)
	assert.Contains(t, fin.Error, "模型服务回 401")
	assert.Contains(t, fin.Error, "检查 API Key")
	buf, _ := json.Marshal(all)
	assert.NotContains(t, string(buf), testKey)
	assert.NotContains(t, string(buf), e.llm.srv.URL)
	msgs := e.messages(conv)
	last := msgs[len(msgs)-1]
	assert.Equal(t, models.AssistantRoleNotice, last.Role)
	assert.True(t, last.IsError)
	assert.NotContains(t, last.Content, testKey)
}

func TestReasoningReplayedButHidden(t *testing.T) {
	e := newEnv(t)
	e.llm.push(reply{reasoning: "用户在打招呼。", text: "你好。"}, reply{text: "还在。"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "你好")
	_, all := e.waitRun(admin, conv, res.RunID)
	res = e.send(admin, conv, "还在吗")
	e.waitRun(admin, conv, res.RunID)
	second := e.llm.requests()[1].Messages
	assert.Equal(t, "assistant", second[2]["role"])
	assert.Equal(t, "用户在打招呼。", second[2]["reasoning_content"], "推理内容回传给模型")
	buf, _ := json.Marshal(all)
	assert.NotContains(t, string(buf), "用户在打招呼", "推理内容不下发")
	d, err := e.svc.Conversation(context.Background(), admin, conv)
	require.NoError(t, err)
	buf, _ = json.Marshal(d)
	assert.NotContains(t, string(buf), "用户在打招呼")
}

func TestHistoryRepairsInterruptedCalls(t *testing.T) {
	e := newEnv(t)
	conv := e.conversation(admin)
	rows := []models.AssistantMessage{
		{ConversationID: conv, Role: "user", Content: "暂停两个"},
		{ConversationID: conv, Role: "assistant", ToolCalls: `[{"id":"c1","name":"list_tasks","arguments":"{}"},{"id":"c2","name":"list_tasks","arguments":"{}"}]`},
		{ConversationID: conv, Role: "tool", ToolCallID: "c1", Content: "{}"},
		{ConversationID: conv, Role: "tool", ToolCallID: "zz", Content: "对不上的结果"},
		{ConversationID: conv, Role: "notice", Content: "pt-tools 关闭了"},
		{ConversationID: conv, Role: "user", Content: "再来"},
	}
	require.NoError(t, e.db.Create(&rows).Error)
	hist, err := e.svc.history(context.Background(), conv)
	require.NoError(t, err)
	buf, _ := json.Marshal(hist)
	var got []map[string]any
	require.NoError(t, json.Unmarshal(buf, &got))
	var shape []string
	for _, m := range got {
		s := m["role"].(string)
		if id, ok := m["tool_call_id"].(string); ok {
			s += ":" + id
		}
		shape = append(shape, s)
	}
	assert.Equal(t, []string{"user", "assistant", "tool:c1", "tool:c2", "user"}, shape)
	assert.Equal(t, interrupted, got[3]["content"])
}

func TestDeltasAreRedacted(t *testing.T) {
	e := newEnv(t)
	e.llm.push(reply{text: "下载地址是 https://x.example/dl?passkey=" + passkey + " 结束。"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "给我地址")
	_, all := e.waitRun(admin, conv, res.RunID)
	partial := ""
	for _, ev := range all {
		if ev.Type == EventDelta {
			assert.NotContains(t, ev.Text, passkey[:8])
			if ev.Replace {
				partial = ev.Text
			} else {
				partial += ev.Text
			}
		}
	}
	assert.NotContains(t, partial, passkey[:8])
	msgs := e.messages(conv)
	assert.NotContains(t, msgs[1].Content, passkey[:8])
	assert.Contains(t, msgs[1].Content, "https://x.example/dl?…")
}

func TestOwnersAreIsolated(t *testing.T) {
	e := newEnv(t)
	e.llm.push(reply{text: "好"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "你好")
	e.waitRun(admin, conv, res.RunID)
	other := Actor{Owner: "device:3", Entry: models.AssistantEntryApp, CanWrite: true}
	ctx := context.Background()
	_, err := e.svc.Conversation(ctx, other, conv)
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = e.svc.Send(ctx, other, conv, "偷看")
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = e.svc.Events(ctx, other, conv, 0, 0)
	assert.ErrorIs(t, err, ErrNotFound)
	assert.ErrorIs(t, e.svc.Cancel(ctx, other, conv), ErrNotFound)
	assert.ErrorIs(t, e.svc.DeleteConversation(ctx, other, conv), ErrNotFound)
	list, err := e.svc.Conversations(ctx, other)
	require.NoError(t, err)
	assert.Empty(t, list)
	list, err = e.svc.Conversations(ctx, admin)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// 别人的待确认也当作不存在
	c := models.AssistantConfirmation{ConversationID: conv, ToolName: "pause_torrent", Status: "pending", Arguments: "{}"}
	require.NoError(t, e.db.Create(&c).Error)
	_, err = e.svc.Decide(ctx, other, c.ID, true)
	assert.ErrorIs(t, err, ErrNotFound)
	reader := admin
	reader.CanWrite = false
	_, err = e.svc.Decide(ctx, reader, c.ID, true)
	assert.ErrorIs(t, err, ErrForbidden)
}

func TestEventsResume(t *testing.T) {
	e := newEnv(t)
	e.llm.push(reply{text: "第一"}, reply{text: "第二"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "一")
	_, first := e.waitRun(admin, conv, res.RunID)
	last := first[len(first)-1].Seq
	res = e.send(admin, conv, "二")
	e.waitRun(admin, conv, res.RunID)
	page, err := e.svc.Events(context.Background(), admin, conv, last, 0)
	require.NoError(t, err)
	require.NotEmpty(t, page.Events)
	for _, ev := range page.Events {
		assert.Greater(t, ev.Seq, last)
		assert.Equal(t, res.RunID, ev.RunID)
	}
	assert.Equal(t, page.Events[len(page.Events)-1].Seq, page.Last)

	// 重启以前的序号：要重新读会话
	page, err = e.svc.Events(context.Background(), admin, conv, e.svc.hub.base-5, 0)
	require.NoError(t, err)
	assert.True(t, page.Reset)
	// 没有新事件时等到超时，回空列表
	start := time.Now()
	page, err = e.svc.Events(context.Background(), admin, conv, page.Last, 50*time.Millisecond)
	require.NoError(t, err)
	assert.Empty(t, page.Events)
	assert.NotNil(t, page.Events)
	assert.GreaterOrEqual(t, time.Since(start), 40*time.Millisecond)
}

func TestStartExpiresPending(t *testing.T) {
	db := newTestDB(t)
	c := models.AssistantConfirmation{ConversationID: 1, ToolName: "pause_torrent", Status: "pending"}
	require.NoError(t, db.Create(&c).Error)
	svc := New(Config{DB: db, Cipher: plainCipher{}})
	require.NoError(t, svc.Start(context.Background()))
	defer func() { _ = svc.Close(context.Background()) }()
	var row models.AssistantConfirmation
	require.NoError(t, db.First(&row, c.ID).Error)
	assert.Equal(t, models.AssistantConfirmExpired, row.Status)
	assert.Equal(t, "restart", row.DecidedBy)
	assert.NotNil(t, row.DecidedAt)
}

func TestCleanupRetention(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	old := time.Now().AddDate(0, 0, -40)
	mk := func(updated time.Time) uint {
		c := models.AssistantConversation{Owner: "web:admin", Entry: "web"}
		require.NoError(t, e.db.Create(&c).Error)
		require.NoError(t, e.db.Model(&c).UpdateColumn("updated_at", updated).Error)
		require.NoError(t, e.db.Create(&models.AssistantMessage{ConversationID: c.ID, Role: "user", Content: "x"}).Error)
		require.NoError(t, e.db.Create(&models.AssistantConfirmation{ConversationID: c.ID, Status: "denied"}).Error)
		return c.ID
	}
	stale, fresh, busy := mk(old), mk(time.Now()), mk(old)
	e.svc.mu.Lock()
	e.svc.runs[busy] = &run{id: "busy", conv: busy}
	e.svc.mu.Unlock()
	defer func() {
		e.svc.mu.Lock()
		delete(e.svc.runs, busy)
		e.svc.mu.Unlock()
	}()

	n, err := e.svc.Cleanup(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	count := func(model any, conv uint) int64 {
		var c int64
		q := e.db.Model(model)
		if _, ok := model.(*models.AssistantConversation); ok {
			q = q.Where("id = ?", conv)
		} else {
			q = q.Where("conversation_id = ?", conv)
		}
		require.NoError(t, q.Count(&c).Error)
		return c
	}
	for _, m := range []any{&models.AssistantConversation{}, &models.AssistantMessage{}, &models.AssistantConfirmation{}} {
		assert.Zero(t, count(m, stale), "过期的连消息与确认一起删")
		assert.Equal(t, int64(1), count(m, fresh), "新的不动")
		assert.Equal(t, int64(1), count(m, busy), "进行中的跳过")
	}

	zero := 0
	_, err = e.svc.SaveSettings(ctx, SettingsInput{Enabled: true, BaseURL: e.llm.srv.URL, Model: "m", RetentionDays: &zero})
	require.NoError(t, err)
	e.svc.mu.Lock()
	delete(e.svc.runs, busy)
	e.svc.mu.Unlock()
	n, err = e.svc.Cleanup(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "保留天数 0 不清理")
}

func TestDeleteConversation(t *testing.T) {
	e := newEnv(t)
	block := make(chan struct{})
	e.llm.push(reply{text: "慢慢想", block: block})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "问题")
	<-e.llm.started
	assert.ErrorIs(t, e.svc.DeleteConversation(context.Background(), admin, conv), ErrBusy, "正在回答的不能删")
	close(block)
	e.waitRun(admin, conv, res.RunID)
	require.NoError(t, e.svc.DeleteConversation(context.Background(), admin, conv))
	assert.Empty(t, e.messages(conv))
	_, err := e.svc.Conversation(context.Background(), admin, conv)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestCloseCancelsRunsAndClosesSessions(t *testing.T) {
	e := newEnv(t)
	e.svc.confirmUnit = time.Minute
	var notified atomic.Int32
	a := admin
	a.Notify = func(context.Context, Event) { notified.Add(1) }
	e.llm.push(call("c1", "pause_torrent", `{"downloader_id":1,"task_id":"abc"}`))
	conv := e.conversation(a)
	_, err := e.svc.Send(context.Background(), a, conv, "暂停")
	require.NoError(t, err)
	events, _ := e.until(a, conv, 0, pendingConfirmation)
	c := events[len(events)-1].Confirmation
	before := notified.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, e.svc.Close(ctx))
	assert.False(t, e.svc.isRunning(conv))
	assert.Equal(t, before, notified.Load(), "关机取消的运行不再经 Notify 回复")
	var row models.AssistantConfirmation
	require.NoError(t, e.db.First(&row, c.ID).Error)
	assert.Equal(t, models.AssistantConfirmExpired, row.Status)
	assert.Equal(t, decidedByShutdown, row.DecidedBy)
	assert.Empty(t, e.backend.writes())
	e.mu.Lock()
	for _, srv := range e.servers {
		for range srv.Sessions() {
			t.Error("运行结束以后 MCP 会话要关掉")
		}
	}
	e.mu.Unlock()
	_, err = e.svc.Send(context.Background(), admin, conv, "还在吗")
	assert.ErrorIs(t, err, ErrClosed)
}

func TestNotifyGetsAnswersAndConfirmations(t *testing.T) {
	e := newEnv(t)
	e.svc.confirmUnit = time.Minute
	var mu sync.Mutex
	var got []Event
	a := Actor{Owner: "chatops:1", Entry: models.AssistantEntryChatOps, CanWrite: true, Notify: func(_ context.Context, ev Event) {
		mu.Lock()
		got = append(got, ev)
		mu.Unlock()
	}}
	_, err := e.svc.SaveSettings(context.Background(), SettingsInput{
		Enabled: true, BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model", ChatOps: true,
	})
	require.NoError(t, err)
	e.llm.push(call("c1", "pause_torrent", `{"downloader_id":1,"task_id":"abc"}`), reply{text: "暂停好了。"})
	conv := e.conversation(a)
	res, err := e.svc.Send(context.Background(), a, conv, "暂停")
	require.NoError(t, err)
	events, _ := e.until(a, conv, 0, pendingConfirmation)
	pc, ok, err := e.svc.PendingConfirmation(context.Background(), a)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, events[len(events)-1].Confirmation.ID, pc.ID)
	_, err = e.svc.Decide(context.Background(), a, pc.ID, true)
	require.NoError(t, err)
	e.waitRun(a, conv, res.RunID)
	// run_finished 先进事件流、再交给 Notify：等 Notify 也收到
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) > 0 && got[len(got)-1].Type == EventRunFinished
	}, 5*time.Second, 10*time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	var kinds []string
	for _, ev := range got {
		k := ev.Type
		switch {
		case ev.Confirmation != nil:
			k += ":" + ev.Confirmation.Status
		case ev.Message != nil:
			k += ":" + ev.Message.Role
		}
		kinds = append(kinds, k)
	}
	assert.Equal(t, []string{
		"message:assistant", "confirmation:pending", "confirmation:approved", "message:assistant", "run_finished",
	}, kinds)
	_, ok, err = e.svc.PendingConfirmation(context.Background(), a)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestChatOpsNeedsSwitch(t *testing.T) {
	e := newEnv(t)
	a := Actor{Owner: "chatops:1", Entry: models.AssistantEntryChatOps}
	conv := e.conversation(a)
	_, err := e.svc.Send(context.Background(), a, conv, "你好")
	assert.ErrorIs(t, err, ErrDisabled, "ChatOps 开关没开")
}

func TestUserTypedTaskIDReachesTool(t *testing.T) {
	e := newEnv(t)
	e.svc.confirmUnit = time.Minute
	hash := "abcdef0123456789abcdef0123456789abcdef01"
	e.llm.push(call("c1", "pause_torrent", `{"downloader_id":1,"task_id":"`+hash+`"}`), reply{text: "好了"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "暂停 "+hash+"，我的 key 是 sk-proj-abcdefghijklmnop1234")
	events, _ := e.until(admin, conv, 0, pendingConfirmation)
	c := *events[len(events)-1].Confirmation
	assert.Equal(t, "暂停种子：下载器 1，任务 "+hash, c.Summary, "info hash 是编号，不当成密钥遮掉")
	user := e.messages(conv)[0].Content
	assert.Contains(t, user, hash)
	assert.NotContains(t, user, "sk-proj-abcdefghijklmnop1234", "常见格式的 API Key 要遮")
	assert.Contains(t, e.llm.requests()[0].Messages[1]["content"], hash)
	_, err := e.svc.Decide(context.Background(), admin, c.ID, true)
	require.NoError(t, err)
	e.waitRun(admin, conv, res.RunID)
	w := e.backend.writes()
	require.Len(t, w, 1)
	targets := w[0].Body.(map[string]any)["targets"].([]map[string]any)
	assert.Equal(t, hash, targets[0]["task_id"])
}

func TestEchoedKeyNeverStored(t *testing.T) {
	e := newEnv(t)
	e.svc.confirmUnit = time.Minute
	e.llm.push(reply{
		reasoning: "Key 是 " + testKey,
		text:      "你的 Key 是 " + testKey + " 呀",
		calls: []replyCall{
			{id: "c1", name: "pause_torrent", args: `{"downloader_id":1,"task_id":"` + testKey + `"}`},
			{id: testKey, name: "list_tasks" + testKey, args: `{}`},
		},
	}, reply{text: "好了"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "回显")
	events, _ := e.until(admin, conv, 0, pendingConfirmation)
	c := *events[len(events)-1].Confirmation
	_, err := e.svc.Decide(context.Background(), admin, c.ID, true)
	require.NoError(t, err)
	_, all := e.waitRun(admin, conv, res.RunID)
	buf, _ := json.Marshal(all)
	assert.NotContains(t, string(buf), testKey, "事件里没有 Key")
	for _, m := range e.messages(conv) {
		assert.NotContains(t, m.Content+m.ToolCalls+m.Reasoning, testKey)
	}
	var confs []models.AssistantConfirmation
	require.NoError(t, e.db.Find(&confs).Error)
	for _, cf := range confs {
		assert.NotContains(t, cf.Arguments, testKey)
	}
	buf, _ = json.Marshal(e.backend.requests())
	assert.NotContains(t, string(buf), testKey, "工具参数里也没有")
	buf, _ = json.Marshal(e.audit.all())
	assert.NotContains(t, string(buf), testKey)
}

func TestSendToDeletedConversation(t *testing.T) {
	e := newEnv(t)
	conv := e.conversation(admin)
	require.NoError(t, e.db.Delete(&models.AssistantConversation{}, conv).Error)
	_, err := e.svc.Send(context.Background(), admin, conv, "你好")
	assert.ErrorIs(t, err, ErrNotFound)
	assert.False(t, e.svc.isRunning(conv), "查不到会话时不留下运行")
	var n int64
	require.NoError(t, e.db.Model(&models.AssistantMessage{}).Where("conversation_id = ?", conv).Count(&n).Error)
	assert.Zero(t, n, "不留孤儿消息")
}

func TestBusyIsHiddenFromOtherOwners(t *testing.T) {
	e := newEnv(t)
	block := make(chan struct{})
	defer close(block)
	e.llm.push(reply{text: "慢慢想", block: block})
	conv := e.conversation(admin)
	e.send(admin, conv, "问题")
	<-e.llm.started
	other := Actor{Owner: "token:5", Entry: models.AssistantEntryApp}
	_, err := e.svc.Send(context.Background(), other, conv, "偷看")
	assert.ErrorIs(t, err, ErrNotFound, "别人的会话忙不忙也不告诉")
	assert.True(t, e.svc.isRunning(conv))
}

func TestCloseIsBoundedWhenDeadlinePassed(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Create(&models.AssistantConfirmation{ConversationID: 1, Status: "pending"}).Error)
	svc := New(Config{DB: db, Cipher: plainCipher{}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	require.NoError(t, svc.Close(ctx))
	assert.Less(t, time.Since(start), 2*time.Second)
	var row models.AssistantConfirmation
	require.NoError(t, db.First(&row).Error)
	assert.Equal(t, models.AssistantConfirmExpired, row.Status, "时限过了也给一秒把待确认标成过期")
}

func TestEchoedPlainKeyNeverStreamed(t *testing.T) {
	e := newEnv(t)
	// Key 里有一段长串：流式增量也要先按原文整个换掉，再通用脱敏
	plainKey := "qa_k-ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	_, err := e.svc.SaveSettings(context.Background(), SettingsInput{
		Enabled: true, BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model", APIKey: &plainKey,
	})
	require.NoError(t, err)
	e.llm.push(reply{text: "你的 Key 是 " + plainKey + " 对吧？我记下了 " + plainKey + "。"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "回显")
	_, all := e.waitRun(admin, conv, res.RunID)
	buf, _ := json.Marshal(all)
	assert.NotContains(t, string(buf), "qa_k-")
	assert.NotContains(t, e.messages(conv)[1].Content, "qa_k-")
	assert.Contains(t, e.messages(conv)[1].Content, "你的 Key 是 ***")
}

// DeepSeek 经部分 OpenAI 兼容网关（2026-10-11 验收时的 deepseek.v3.2）会把自己的工具调用标记漏进正文：
// 网关已经把调用解析成 tool_calls，正文末尾还剩「<｜DSML｜function_calls」。这一段不发增量、不落库、不回传给模型。
func TestLeakedToolMarkupStripped(t *testing.T) {
	e := newEnv(t)
	first := call("call_1", "list_tasks", `{"page":1}`)
	first.text = "我来看看最近抓了什么。\n\n<｜DSML｜function_calls"
	e.llm.push(first, reply{text: "找到 1 个种子。"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "最近抓了什么种子")
	fin, all := e.waitRun(admin, conv, res.RunID)
	require.Equal(t, RunDone, fin.Status, fin.Error)
	// 按界面的做法拼增量（Replace 时整段换掉，消息到了清空），每一段都不该带标记
	var partials []string
	partial := ""
	for _, ev := range all {
		switch ev.Type {
		case EventDelta:
			assert.NotContains(t, ev.Text, "｜", "增量里不该出现标记的任何一段")
			if ev.Replace {
				partial = ev.Text
			} else {
				partial += ev.Text
			}
		case EventMessage:
			assert.NotContains(t, ev.Message.Content, "DSML")
			if partial != "" {
				partials = append(partials, partial)
			}
			partial = ""
		}
	}
	require.NotEmpty(t, partials)
	assert.Equal(t, "我来看看最近抓了什么。\n\n", partials[0], "流式显示的正文停在标记前面，一个字也不少")
	msgs := e.messages(conv)
	assert.Equal(t, "我来看看最近抓了什么。", msgs[1].Content)
	second := e.llm.requests()[1]
	assert.Equal(t, "我来看看最近抓了什么。", second.Messages[2]["content"], "回传给模型的也是去掉标记的")
}

// Key 里正好有工具调用标记的开头（设置只要求 8 到 512 个字符、没有空格）：先按原文换掉 Key 再截标记，
// Key 的前半段不能漏进落库的回答、回传与任何事件（流式时末尾可能是 Key 开头的一段也先不发）。
func TestKeyContainingMarkupNeverLeaks(t *testing.T) {
	e := newEnv(t)
	odd := "qa_k-AB<｜DSML｜rest-0123"
	_, err := e.svc.SaveSettings(context.Background(), SettingsInput{
		Enabled: true, BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model", APIKey: &odd,
	})
	require.NoError(t, err)
	e.llm.push(reply{text: "你的 Key 是 " + odd + " 对吧？"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "回显")
	_, all := e.waitRun(admin, conv, res.RunID)
	for _, ev := range all {
		if ev.Type == EventDelta {
			assert.NotContains(t, ev.Text, "qa_k-AB", "增量")
		}
		if ev.Type == EventMessage {
			assert.NotContains(t, ev.Message.Content, "qa_k-AB", "消息事件")
		}
	}
	assert.Equal(t, "你的 Key 是 *** 对吧？", e.messages(conv)[1].Content)
}

// 模型给的工具参数不是 JSON（2026-10-11 验收时 deepseek.v3-v1:0 回了 {"keyword": "Fake", "limit": 20)）：当工具错误交回模型；
// 回传的这次调用参数换成 {}。部分兼容网关会校验历史里的参数，坏 JSON 会让这个会话以后的每次请求都回 400。
func TestInvalidToolArgumentsReplayedAsEmptyObject(t *testing.T) {
	e := newEnv(t)
	e.llm.strict()
	e.llm.push(call("call_1", "search_torrents", `{"keyword": "Fake", "limit": 20)`), reply{text: "参数写错了。"}, reply{text: "不客气。"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "搜一下 Fake")
	fin, _ := e.waitRun(admin, conv, res.RunID)
	require.Equal(t, RunDone, fin.Status, fin.Error)
	msgs := e.messages(conv)
	require.Equal(t, []string{"user", "assistant", "tool", "assistant"}, roles(msgs))
	assert.True(t, msgs[2].IsError)
	assert.Contains(t, msgs[2].Content, "参数不是 JSON 对象")
	assert.Contains(t, msgs[1].ToolCalls, `"limit\": 20)`, "界面上照样看得到模型给的原样参数")
	second := e.llm.requests()[1]
	fn := second.Messages[2]["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)
	assert.Equal(t, "{}", fn["arguments"])
	// 同一个会话接着问：历史里的这次调用照样能回传
	res = e.send(admin, conv, "谢谢")
	fin, _ = e.waitRun(admin, conv, res.RunID)
	require.Equal(t, RunDone, fin.Status, fin.Error)
}

// Gemini 3 经 OpenAI 兼容接口调工具时，tool_calls 里带 extra_content.google.thought_signature，之后的请求要原样带回去，
// 否则回 400「Function call is missing a thought_signature」（2026-10-12 用真实 Key 实测）。签名不出现在详情与事件里。
func TestThoughtSignatureRoundTrips(t *testing.T) {
	e := newEnv(t)
	const sig = "c2lnbmF0dXJlLWZyb20tZ2VtaW5pLTEyMzQ1Njc4OTA="
	first := reply{calls: []replyCall{{id: "call_1", name: "list_tasks", args: `{"page":1}`, extra: `{"google":{"thought_signature":"` + sig + `"}}`}}}
	e.llm.push(first, reply{text: "找到 1 个种子。"}, reply{text: "不客气。"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "最近抓了什么")
	fin, all := e.waitRun(admin, conv, res.RunID)
	require.Equal(t, RunDone, fin.Status, fin.Error)
	signed := func(req llmRequest) bool {
		for _, m := range req.Messages {
			calls, _ := m["tool_calls"].([]any)
			for _, c := range calls {
				extra, _ := c.(map[string]any)["extra_content"].(map[string]any)
				google, _ := extra["google"].(map[string]any)
				if google["thought_signature"] == sig {
					return true
				}
			}
		}
		return false
	}
	assert.True(t, signed(e.llm.requests()[1]), "这次运行里的下一次请求带回签名")

	res = e.send(admin, conv, "谢谢")
	fin, _ = e.waitRun(admin, conv, res.RunID)
	require.Equal(t, RunDone, fin.Status, fin.Error)
	assert.True(t, signed(e.llm.requests()[2]), "同一个会话接着问：历史里的调用照样带着签名")

	d, err := e.svc.Conversation(context.Background(), admin, conv)
	require.NoError(t, err)
	buf, _ := json.Marshal(d)
	assert.NotContains(t, string(buf), sig, "详情里没有签名")
	buf, _ = json.Marshal(all)
	assert.NotContains(t, string(buf), sig, "事件里没有签名")
}

// 服务回显收到的 Key 时可能用 \u 转义写（原文替换认不出来）：工具参数里有的，不执行、参数不留；extra_content 里有的（转义或原文），
// 整个不要（思考签名改不得）。库里、事件里、之后的请求里都没有 Key。
func TestEscapedKeyInToolCallsNeverStored(t *testing.T) {
	e := newEnv(t)
	key := "qa-key-0123456789abcdef"
	_, err := e.svc.SaveSettings(context.Background(), SettingsInput{
		Enabled: true, BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model", APIKey: &key,
	})
	require.NoError(t, err)
	var b strings.Builder
	for _, r := range key {
		fmt.Fprintf(&b, `\u%04x`, r)
	}
	esc := b.String()
	e.llm.push(reply{calls: []replyCall{
		{id: "call_1", name: "search_torrents", args: `{"keyword":"` + esc + `"}`, extra: `{"google":{"thought_signature":"` + esc + `"}}`},
		{id: "call_2", name: "list_tasks", args: `{"page":1}`, extra: `{"google":{"thought_signature":"sig-` + key + `"}}`},
	}}, reply{text: "好的。"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "搜一下")
	fin, all := e.waitRun(admin, conv, res.RunID)
	require.Equal(t, RunDone, fin.Status, fin.Error)

	msgs := e.messages(conv)
	require.Equal(t, []string{"user", "assistant", "tool", "tool", "assistant"}, roles(msgs))
	assert.True(t, msgs[2].IsError)
	assert.Contains(t, msgs[2].Content, "参数里有 API Key")
	assert.False(t, msgs[3].IsError, "另一个调用照常执行")
	for _, m := range msgs {
		for _, v := range []string{m.Content, m.Reasoning, m.ToolCalls} {
			assert.False(t, jsonMentions(v, key) || strings.Contains(v, esc), "库里有 Key：%s", v)
		}
	}
	buf, _ := json.Marshal(all)
	assert.False(t, jsonMentions(string(buf), key) || strings.Contains(string(buf), esc), "事件里有 Key")
	next, _ := json.Marshal(e.llm.requests()[1].Messages)
	assert.False(t, jsonMentions(string(next), key) || strings.Contains(string(next), esc), "交回模型的请求里有 Key")
	assert.NotContains(t, string(next), "extra_content", "有 Key 的签名整个不要")
}
