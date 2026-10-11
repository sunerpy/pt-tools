package assistant

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/openai/openai-go/v3"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/redact"
	"github.com/sunerpy/pt-tools/models"
)

// run 是一次运行：从一条用户消息开始，循环「调模型 → 执行工具 → 结果交回」直到模型给出回答。
type run struct {
	id     string
	conv   uint
	actor  Actor
	ctx    context.Context
	cancel context.CancelCauseFunc
	// decided 在有人确认或拒绝这次运行的待确认操作时收到信号（等的一方再到库里读结论）
	decided chan struct{}
}

// SendResult 是发消息的结果：后台运行的编号与落库的用户消息。
type SendResult struct {
	RunID   string      `json:"run_id"`
	Message MessageView `json:"message"`
}

func newRunID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Send 往会话里发一条消息并在后台开始回答，立即返回。同一个会话同时只有一次运行（另一个回 ErrBusy）。
func (s *Service) Send(ctx context.Context, a Actor, convID uint, text string) (SendResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return SendResult{}, fmt.Errorf("%w: 消息不能为空", ErrInvalid)
	}
	if utf8.RuneCountInString(text) > maxUserRunes {
		return SendResult{}, fmt.Errorf("%w: 一条消息最多 %d 个字", ErrInvalid, maxUserRunes)
	}
	set, err := s.effectiveSettings(ctx)
	if err != nil {
		return SendResult{}, err
	}
	if a.Entry == models.AssistantEntryChatOps && !set.chatOps {
		return SendResult{}, ErrDisabled
	}
	if _, err = s.conversation(ctx, a, convID); err != nil {
		return SendResult{}, err
	}
	r := &run{id: newRunID(), conv: convID, actor: a, decided: make(chan struct{}, 1)}
	r.ctx, r.cancel = context.WithCancelCause(context.Background())
	s.mu.Lock()
	switch {
	case s.closing:
		err = ErrClosed
	case s.purging[convID]:
		err = ErrNotFound
	case s.runs[convID] != nil:
		err = ErrBusy
	case len(s.runs) >= maxRuns:
		err = ErrTooMany
	default:
		s.runs[convID] = r
		s.wg.Add(1)
	}
	s.mu.Unlock()
	if err != nil {
		r.cancel(nil)
		return SendResult{}, err
	}
	s.hub.publish(convID, Event{Type: EventRunStarted, RunID: r.id, At: s.now()})
	content := s.userText(text)
	msg, err := s.post(r, models.AssistantMessage{Role: models.AssistantRoleUser, Content: content})
	if err != nil {
		s.release(r)
		s.hub.publish(convID, Event{Type: EventRunFinished, RunID: r.id, Status: RunFailed, Error: err.Error(), At: s.now()})
		s.wg.Done()
		return SendResult{}, err
	}
	s.touch(ctx, convID, titleFrom(content))
	go s.execute(r, set)
	return SendResult{RunID: r.id, Message: messageView(msg)}, nil
}

// Cancel 停止会话进行中的运行（没有时什么也不做）：在等确认的操作标成过期，不执行。
func (s *Service) Cancel(ctx context.Context, a Actor, convID uint) error {
	if _, err := s.conversation(ctx, a, convID); err != nil {
		return err
	}
	if r := s.activeRun(convID); r != nil {
		r.cancel(errCanceled)
	}
	return nil
}

// Events 读会话 after 以后的事件，没有时最多等 wait（上限 30 秒）。
func (s *Service) Events(ctx context.Context, a Actor, convID uint, after int64, wait time.Duration) (EventPage, error) {
	if _, err := s.conversation(ctx, a, convID); err != nil {
		return EventPage{}, err
	}
	page := s.hub.wait(ctx, convID, after, min(wait, 30*time.Second))
	if page.Events == nil {
		page.Events = []Event{}
	}
	page.Running = s.isRunning(convID)
	return page, nil
}

// release 把运行从会话上摘下来。
func (s *Service) release(r *run) {
	s.mu.Lock()
	if s.runs[r.conv] == r {
		delete(s.runs, r.conv)
	}
	s.mu.Unlock()
	r.cancel(nil)
}

// shuttingDown 报告运行是不是因为关机停的（这时不再经 Notify 回复）。
func (r *run) shuttingDown() bool { return errors.Is(context.Cause(r.ctx), errShutdown) }

// notify 把事件交给属主的 Notify（ChatOps 的异步回复）。
func (s *Service) notify(r *run, ev Event) {
	if r.actor.Notify == nil || r.shuttingDown() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r.actor.Notify(ctx, ev)
}

// post 落一条消息、发 message 事件并交给 Notify。
func (s *Service) post(r *run, m models.AssistantMessage) (models.AssistantMessage, error) {
	m.ConversationID, m.RunID = r.conv, r.id
	saved, err := s.addMessage(r.ctx, m)
	if err != nil {
		return saved, err
	}
	if m.Role != models.AssistantRoleUser && m.Role != models.AssistantRoleTool {
		v := messageView(saved)
		s.notify(r, Event{Type: EventMessage, RunID: r.id, Message: &v, At: saved.CreatedAt})
	}
	return saved, nil
}

// execute 是后台运行的主体。
func (s *Service) execute(r *run, set effective) {
	defer s.wg.Done()
	status, msg := s.drive(r, set)
	switch status {
	case RunCanceled:
		text := "已停止回答。"
		if r.shuttingDown() {
			text = "pt-tools 关闭了，这次回答中断。"
		}
		if _, err := s.post(r, models.AssistantMessage{Role: models.AssistantRoleNotice, Content: text}); err != nil {
			global.GetSlogger().Warnf("[AI 助手] 记录停止提示失败: %v", err)
		}
	case RunFailed:
		if _, err := s.post(r, models.AssistantMessage{Role: models.AssistantRoleNotice, Content: msg, IsError: true}); err != nil {
			global.GetSlogger().Warnf("[AI 助手] 记录出错提示失败: %v", err)
		}
	}
	if status != RunDone && r.shuttingDown() {
		global.GetSlogger().Infof("[AI 助手] 关机中断了会话 %d 的回答", r.conv)
	}
	s.release(r)
	s.touch(r.ctx, r.conv, "")
	ev := s.hub.publish(r.conv, Event{Type: EventRunFinished, RunID: r.id, Status: status, Error: msg, At: s.now()})
	if status == RunDone {
		s.notify(r, ev)
	}
}

// stopped 报告运行是不是被停了。
func stopped(r *run) bool { return r.ctx.Err() != nil }

// drive 循环调模型与执行工具，返回结束状态与（失败时的）原因。
func (s *Service) drive(r *run, set effective) (string, string) {
	chat, err := s.chatService(set.llm)
	if err != nil {
		return RunFailed, redact.Secrets("模型设置不对：" + err.Error())
	}
	tb, err := s.openToolbox(r.ctx, r.actor)
	if err != nil {
		if stopped(r) {
			return RunCanceled, ""
		}
		return RunFailed, redact.Secrets(err.Error())
	}
	defer tb.Close()
	hist, err := s.history(r.ctx, r.conv)
	if err != nil {
		if stopped(r) {
			return RunCanceled, ""
		}
		return RunFailed, err.Error()
	}
	msgs := append([]openai.ChatCompletionMessageParamUnion{openai.SystemMessage(systemPrompt(s.now(), r.actor))}, hist...)
	for range set.maxSteps {
		rs := &redactedStream{}
		out, err := s.complete(r.ctx, chat, set.llm.Model, msgs, tb.defs, func(chunk string) {
			s.emitDelta(r, rs, chunk, false)
		})
		if err != nil {
			if stopped(r) {
				return RunCanceled, ""
			}
			return RunFailed, describeError(err, set.llm.APIKey)
		}
		calls := make([]pendingCall, 0, len(out.calls))
		stored := make([]storedCall, 0, len(out.calls))
		writes := make([]bool, 0, len(out.calls))
		for _, c := range out.calls {
			pc := s.normalize(c.id, c.name, c.args)
			write, known := tb.known(pc.name)
			if !known && pc.invalid == "" {
				pc.invalid = "没有这个工具：" + pc.name
			}
			calls, writes = append(calls, pc), append(writes, write)
			stored = append(stored, storedCall{ID: pc.id, Name: pc.name, Arguments: pc.display, Write: write})
		}
		content, reasoning := redact.Secrets(out.content), redact.Credentials(out.reasoning)
		am := models.AssistantMessage{Role: models.AssistantRoleAssistant, Content: content, Reasoning: reasoning}
		if len(stored) > 0 {
			buf, _ := json.Marshal(stored)
			am.ToolCalls = string(buf)
		}
		if _, err := s.post(r, am); err != nil {
			return RunFailed, err.Error()
		}
		msgs = append(msgs, assistantParam(content, reasoning, stored))
		if len(calls) == 0 {
			if stopped(r) {
				return RunCanceled, ""
			}
			return RunDone, ""
		}
		for i, pc := range calls {
			res := s.runCall(r, tb, set, pc, writes[i])
			tm := models.AssistantMessage{
				Role: models.AssistantRoleTool, Content: res.text, ToolName: pc.name, ToolCallID: pc.id, IsError: res.isError,
			}
			if _, err := s.post(r, tm); err != nil {
				return RunFailed, err.Error()
			}
			msgs = append(msgs, openai.ToolMessage(res.text, pc.id))
		}
		if stopped(r) {
			return RunCanceled, ""
		}
	}
	text := fmt.Sprintf("已经调用了 %d 轮工具，没有得出结论，先停在这里；可以换个说法再问。", set.maxSteps)
	if _, err := s.post(r, models.AssistantMessage{Role: models.AssistantRoleNotice, Content: text}); err != nil {
		return RunFailed, err.Error()
	}
	return RunDone, ""
}

// runCall 执行一个工具调用：参数不对的直接当工具错误；读工具立即执行；写工具先等人确认。
func (s *Service) runCall(r *run, tb *toolbox, set effective, pc pendingCall, write bool) toolResult {
	if pc.invalid != "" {
		return toolResult{text: pc.invalid, isError: true}
	}
	if stopped(r) {
		return toolResult{text: "运行已停止，操作没有执行。", isError: true}
	}
	if write {
		return s.confirmThenCall(r, tb, set, pc)
	}
	s.toolStarted(r, pc, false)
	return tb.call(r.ctx, pc.name, pc.args)
}

func (s *Service) toolStarted(r *run, pc pendingCall, write bool) {
	v := ToolCallView{ID: pc.id, Name: pc.name, Arguments: pc.display, Write: write}
	s.hub.publish(r.conv, Event{Type: EventToolStarted, RunID: r.id, Tool: &v, At: s.now()})
}

// redactedStream 让流式的文字增量也过脱敏：攒着全文，每次只发脱敏以后新多出来的部分；末尾还可能变长的一段
// （最后一个 ASCII 词，地址与长串都在里面）先不发。脱敏改了已经发出去的部分时整段替换。
type redactedStream struct {
	full strings.Builder
	sent string
}

// safeCut 是可以先发的长度：去掉末尾连着的非空白 ASCII 字符。
func safeCut(s string) int {
	i := len(s)
	for i > 0 {
		c := s[i-1]
		if c >= 0x80 || c == ' ' || c == '\n' || c == '\t' || c == '\r' {
			break
		}
		i--
	}
	return i
}

func (s *Service) emitDelta(r *run, rs *redactedStream, chunk string, final bool) {
	rs.full.WriteString(chunk)
	full := rs.full.String()
	cut := len(full)
	if !final {
		cut = safeCut(full)
	}
	red := redact.Secrets(full[:cut])
	if red == rs.sent {
		return
	}
	ev := Event{Type: EventDelta, RunID: r.id, At: s.now()}
	if strings.HasPrefix(red, rs.sent) {
		ev.Text = red[len(rs.sent):]
	} else {
		ev.Text, ev.Replace = red, true
	}
	rs.sent = red
	s.hub.publish(r.conv, ev)
}
