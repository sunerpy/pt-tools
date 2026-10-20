package assistant

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
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
	// model、effort 是这次运行选的模型与推理等级（为空用设置里的模型、不指定推理等级）
	model, effort string
	// images 报告这次的用户消息带没带图片（模型拒绝时提示可能不能看图）
	images bool
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

// SendOptions 是发一条消息：文字、模型（为空用设置里的）、推理等级（为空不指定，见 Efforts）与附件（先 Upload 拿到的编号）。
// 文字与附件至少有一样。
type SendOptions struct {
	Text        string
	Model       string
	Effort      string
	Attachments []uint
}

// Efforts 是可以选的推理等级（OpenAI 的 reasoning_effort）：各家认的不一样，模型服务因为它拒绝请求时去掉它重试一次。
var Efforts = []string{"none", "minimal", "low", "medium", "high", "xhigh"}

// effortNames 是推理等级在提示里的叫法。
var effortNames = map[string]string{"none": "不思考", "minimal": "最低", "low": "低", "medium": "中", "high": "高", "xhigh": "最高"}

// checkOptions 检查发消息的选项，返回整理好的模型与推理等级。
func checkOptions(o SendOptions) (model, effort string, err error) {
	model, effort = strings.TrimSpace(o.Model), strings.TrimSpace(o.Effort)
	if len(model) > maxModelLen || hasSpace(model) {
		return "", "", fmt.Errorf("%w: 模型名不对（不能有空格，最长 %d 个字符）", ErrInvalid, maxModelLen)
	}
	if effort != "" && !slices.Contains(Efforts, effort) {
		return "", "", fmt.Errorf("%w: 推理等级要是 %s 之一（不指定就留空）", ErrInvalid, strings.Join(Efforts, "、"))
	}
	return model, effort, nil
}

// Send 往会话里发一条文字消息，用设置里的模型（见 SendWith）。
func (s *Service) Send(ctx context.Context, a Actor, convID uint, text string) (SendResult, error) {
	return s.SendWith(ctx, a, convID, SendOptions{Text: text})
}

// SendWith 往会话里发一条消息并在后台开始回答，立即返回。同一个会话同时只有一次运行（另一个回 ErrBusy）。
func (s *Service) SendWith(ctx context.Context, a Actor, convID uint, o SendOptions) (SendResult, error) {
	text := strings.TrimSpace(o.Text)
	if text == "" && len(o.Attachments) == 0 {
		return SendResult{}, fmt.Errorf("%w: 消息不能为空", ErrInvalid)
	}
	if utf8.RuneCountInString(text) > maxUserRunes {
		return SendResult{}, fmt.Errorf("%w: 一条消息最多 %d 个字", ErrInvalid, maxUserRunes)
	}
	model, effort, err := checkOptions(o)
	if err != nil {
		return SendResult{}, err
	}
	set, err := s.effectiveSettings(ctx)
	if err != nil {
		return SendResult{}, err
	}
	if a.Entry == models.AssistantEntryChatOps && !set.chatOps {
		return SendResult{}, ErrDisabled
	}
	if !a.valid() {
		return SendResult{}, ErrNotFound
	}
	if _, err = s.conversation(ctx, a, convID); err != nil {
		return SendResult{}, err
	}
	atts, attIDs, err := s.pendingAttachments(ctx, a, o.Attachments)
	if err != nil {
		return SendResult{}, err
	}
	r := &run{id: newRunID(), conv: convID, actor: a, decided: make(chan struct{}, 1), model: model, effort: effort}
	for _, at := range atts {
		r.images = r.images || at.Kind == models.AssistantAttachImage
	}
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
		// 别人的会话一律当作不存在：忙不忙也不告诉
		if _, cerr := s.conversation(ctx, a, convID); cerr != nil {
			return SendResult{}, cerr
		}
		return SendResult{}, err
	}
	// 登记以后再查会话：登记着运行的会话删不掉（beginPurge 会拒），登记以前删掉的这里查不到
	if _, err = s.conversation(ctx, a, convID); err != nil {
		s.release(r)
		s.wg.Done()
		return SendResult{}, err
	}
	s.hub.publish(convID, Event{Type: EventRunStarted, RunID: r.id, At: s.now()})
	content := s.userText(text)
	view, err := s.addUserMessage(r.ctx, models.AssistantMessage{
		ConversationID: convID, RunID: r.id, Role: models.AssistantRoleUser, Content: content, Model: model, Effort: effort,
	}, a.Owner, attIDs)
	if err != nil {
		s.release(r)
		s.hub.publish(convID, Event{Type: EventRunFinished, RunID: r.id, Status: RunFailed, Error: err.Error(), At: s.now()})
		s.wg.Done()
		return SendResult{}, err
	}
	title := content
	if title == "" && len(atts) > 0 {
		title = atts[0].Name
	}
	s.touch(ctx, convID, titleFrom(title))
	go s.execute(r, set)
	return SendResult{RunID: r.id, Message: view}, nil
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
	return s.EventsFor(ctx, a, convID, after, wait, nil)
}

// EventsFor 同 Events，keep 不为空时只给它挑中的事件（App API 只给契约里有的种类）：一个也没挑中时接着等到 wait。
func (s *Service) EventsFor(ctx context.Context, a Actor, convID uint, after int64, wait time.Duration, keep func(Event) bool) (EventPage, error) {
	if _, err := s.conversation(ctx, a, convID); err != nil {
		return EventPage{}, err
	}
	page := s.hub.wait(ctx, convID, after, min(wait, 30*time.Second), keep)
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
	scrub := keyScrubber(set.llm.APIKey)
	model, effort := set.llm.Model, r.effort
	if r.model != "" {
		model = r.model
	}
	if set.contextWindow == 0 {
		// 自动的上下文窗口要用服务商模型列表报的长度：没读过就在后台读一次
		s.refreshModelsSoon(set.llm)
	}
	// 上下文用到了阈值：开始回答以前先压缩（压缩不成也照样回答，历史超了预算从最旧的丢起）
	if set.autoCompact {
		if u, uerr := s.runUsage(r, set, model); uerr == nil && needsCompact(u) {
			reason := fmt.Sprintf("上下文用了窗口的约 %d%%", u.ContextTokens*100/int64(u.ContextWindow))
			if cerr := s.compact(r, set, chat, model, reason, true); cerr != nil && !errors.Is(cerr, errNothingToCompact) {
				if stopped(r) {
					return RunCanceled, ""
				}
				text := "压缩上下文没有成功（" + describeError(cerr, set.llm.APIKey) + "），这次按原样回答。"
				if _, perr := s.post(r, models.AssistantMessage{Role: models.AssistantRoleNotice, Content: text}); perr != nil {
					return RunFailed, perr.Error()
				}
			}
		}
	}
	msgs, err := s.modelMessages(r, set, model)
	if err != nil {
		if stopped(r) {
			return RunCanceled, ""
		}
		return RunFailed, err.Error()
	}
	compacted := false
	for range set.maxSteps {
		out, err := s.callModel(r, chat, set, step{model: model, effort: effort, msgs: msgs, tools: tb.defs})
		if err != nil && effort != "" && effortRejected(err) && !stopped(r) {
			// 这个模型不认这个推理等级：去掉重试一次，这次运行后面也不再带
			text := fmt.Sprintf("模型 %s 不认推理等级「%s」，这次按默认的回答。", model, effortNames[effort])
			effort = ""
			if _, perr := s.post(r, models.AssistantMessage{Role: models.AssistantRoleNotice, Content: text}); perr != nil {
				return RunFailed, perr.Error()
			}
			out, err = s.callModel(r, chat, set, step{model: model, msgs: msgs, tools: tb.defs})
		}
		if err != nil && !compacted && contextExceeded(err) && !stopped(r) {
			// 请求超过了模型的上下文窗口：压缩一次再试（这次运行里只压缩一次）
			compacted = true
			if cerr := s.compact(r, set, chat, model, "请求超过了模型的上下文窗口", false); cerr == nil {
				if msgs, err = s.modelMessages(r, set, model); err == nil {
					out, err = s.callModel(r, chat, set, step{model: model, effort: effort, msgs: msgs, tools: tb.defs})
				}
			}
		}
		if err != nil {
			if stopped(r) {
				return RunCanceled, ""
			}
			msg := describeError(err, set.llm.APIKey)
			if r.images && imageRejected(err) {
				msg += "（这个模型可能不能看图：换一个能看图的模型，或者去掉图片再发）"
			}
			return RunFailed, msg
		}
		// 服务回显了收到的 Key（恶意或配错的兼容服务）也不让它进库、事件与工具参数；先按原文换掉 Key，再截工具调用标记
		out.content, out.reasoning = scrub(out.content), scrub(out.reasoning)
		out.content = stripToolMarkup(out.content)
		calls := make([]pendingCall, 0, len(out.calls))
		stored := make([]storedCall, 0, len(out.calls))
		writes := make([]bool, 0, len(out.calls))
		for _, c := range out.calls {
			args := scrub(c.args)
			pc := s.normalize(scrub(c.id), scrub(c.name), args)
			if jsonMentions(args, set.llm.APIKey) {
				// 参数里还有用 \u 转义写的 Key（原文替换认不出来）：不执行，参数也不留
				pc.args, pc.display, pc.invalid = map[string]any{}, "{}", "参数里有 API Key，没有执行"
			}
			write, known := tb.known(pc.name)
			if !known && pc.invalid == "" {
				pc.invalid = "没有这个工具：" + pc.name
			}
			calls, writes = append(calls, pc), append(writes, write)
			stored = append(stored, storedCall{ID: pc.id, Name: pc.name, Arguments: pc.display, Write: write, Extra: rawExtra(c.extra, set.llm.APIKey)})
		}
		content, reasoning := redact.Text(out.content), redact.Credentials(out.reasoning)
		am := models.AssistantMessage{
			Role: models.AssistantRoleAssistant, Content: content, Reasoning: reasoning, ReasoningField: out.reasoningField,
			Model: model, Effort: effort, PromptTokens: out.usage.prompt, CompletionTokens: out.usage.completion,
		}
		if len(stored) > 0 {
			buf, _ := json.Marshal(stored)
			am.ToolCalls = string(buf)
		}
		if _, err := s.post(r, am); err != nil {
			return RunFailed, err.Error()
		}
		s.publishUsage(r, set, model)
		msgs = append(msgs, assistantParam(content, replayed(reasoning, out.reasoningField), stored))
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

// modelMessages 是交给模型的消息：系统提示（压缩过的接上摘要）加上摘要截止以后的历史（按窗口算的预算以内）。
func (s *Service) modelMessages(r *run, set effective, model string) ([]openai.ChatCompletionMessageParamUnion, error) {
	var convs []models.AssistantConversation
	if err := s.db(r.ctx).Where("id = ?", r.conv).Limit(1).Find(&convs).Error; err != nil {
		return nil, fmt.Errorf("读取会话失败: %w", err)
	}
	var c models.AssistantConversation
	if len(convs) > 0 {
		c = convs[0]
	}
	window, _ := s.windowFor(set, model)
	system := withSummary(systemPrompt(s.now(), r.actor), c.Summary)
	budget := historyBudget(window, set.compactPercent, set.autoCompact, estimateTokens(system)+s.toolsOverhead())
	hist, err := s.history(r.ctx, r.conv, c.SummaryUntil, budget)
	if err != nil {
		return nil, err
	}
	return append([]openai.ChatCompletionMessageParamUnion{openai.SystemMessage(system)}, hist...), nil
}

// runUsage 是这次运行开始时会话的上下文用量。
func (s *Service) runUsage(r *run, set effective, model string) (UsageView, error) {
	var convs []models.AssistantConversation
	if err := s.db(r.ctx).Where("id = ?", r.conv).Limit(1).Find(&convs).Error; err != nil || len(convs) == 0 {
		return UsageView{}, fmt.Errorf("读取会话失败: %v", err)
	}
	return s.conversationUsage(r.ctx, convs[0], set, model)
}

// callModel 调一次模型；服务商不认 stream_options（要用量的参数）时去掉重试一次，记下这个地址以后不再带。
func (s *Service) callModel(r *run, chat openai.ChatCompletionService, set effective, st step) (completion, error) {
	st.noUsage = s.usageUnsupported(set.llm.BaseURL)
	out, err := s.stepOnce(r, chat, set, st)
	if err != nil && !st.noUsage && streamOptionsRejected(err) && !stopped(r) {
		s.markUsageUnsupported(set.llm.BaseURL)
		st.noUsage = true
		out, err = s.stepOnce(r, chat, set, st)
	}
	return out, err
}

// usageUnsupported 报告这个接口地址不认 stream_options（这个进程里试过）。
func (s *Service) usageUnsupported(baseURL string) bool {
	s.noUsageMu.Lock()
	defer s.noUsageMu.Unlock()
	return s.noUsage[baseURL]
}

func (s *Service) markUsageUnsupported(baseURL string) {
	s.noUsageMu.Lock()
	defer s.noUsageMu.Unlock()
	if s.noUsage == nil {
		s.noUsage = map[string]bool{}
	}
	s.noUsage[baseURL] = true
}

// stepOnce 调一次模型：回答文字与推理内容的增量各经一道脱敏发出去（都过 redact.Text，和落库以后给人看的一样）。
func (s *Service) stepOnce(r *run, chat openai.ChatCompletionService, set effective, st step) (completion, error) {
	scrub := keyScrubber(set.llm.APIKey)
	rs := &redactedStream{scrub: scrub, key: set.llm.APIKey, kind: EventDelta, markup: true}
	rr := &redactedStream{scrub: scrub, key: set.llm.APIKey, kind: EventReasoning}
	out, err := s.complete(r.ctx, chat, st, func(chunk string) { s.emitStream(r, rs, chunk, false) },
		func(chunk string) { s.emitStream(r, rr, chunk, false) })
	if err != nil {
		return out, err
	}
	s.emitStream(r, rr, "", true)
	s.emitStream(r, rs, "", true)
	return out, nil
}

// replayed 是要回传给模型的推理内容：只有 reasoning_content 来的回传（见 models.AssistantMessage.ReasoningField）。
func replayed(reasoning, field string) string {
	if field != fieldReasoningContent {
		return ""
	}
	return reasoning
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

// rawExtra 是工具调用的 extra_content，原样留着回传（思考签名不透明，改不得）：不是合法 JSON 的不要；里面有 Key 的
// （原文或者用转义写的）整个不要，免得 Key 进库。
func rawExtra(s, key string) json.RawMessage {
	if s == "" || !json.Valid([]byte(s)) || jsonMentions(s, key) {
		return nil
	}
	return json.RawMessage(s)
}

// jsonMentions 报告 JSON 文本里有没有 Key，原文或者用转义写的（\u0071、\/ 这类，按原文替换认不出来）：整段的转义都还原了再找，
// 不是合法 JSON（出错的地方后面还有）也找得到。和 keyScrubber 一样，太短的 Key 不管。
func jsonMentions(raw, key string) bool {
	if len(key) < minKeyLen {
		return false
	}
	return strings.Contains(raw, key) || strings.Contains(jsonUnescape(raw), key)
}

// jsonEscape 是 JSON 字符串里的一个转义：一对代理（\uD83D\uDE00）、\uXXXX，或者反斜杠加一个字符
var jsonEscape = regexp.MustCompile(`\\(u[dD][89abAB][0-9a-fA-F]{2}\\u[dD][c-fC-F][0-9a-fA-F]{2}|u[0-9a-fA-F]{4}|.)`)

// jsonUnescape 把文本里的 JSON 转义都还原，和 encoding/json 一样（从左到右，\\ 是一个反斜杠；一对代理合成一个字符，落单的代理
// 与不是 UTF-8 的字节都是 U+FFFD），不管它在不在字符串里、JSON 合不合法。
func jsonUnescape(s string) string {
	return jsonEscape.ReplaceAllStringFunc(string([]rune(s)), func(m string) string {
		switch m[1] {
		case 'u':
			if len(m) == len(`\uD83D\uDE00`) {
				hi, _ := strconv.ParseUint(m[2:6], 16, 32)
				lo, _ := strconv.ParseUint(m[8:], 16, 32)
				return string(utf16.DecodeRune(rune(hi), rune(lo)))
			}
			n, _ := strconv.ParseUint(m[2:], 16, 32)
			return string(rune(n))
		case 'b':
			return "\b"
		case 'f':
			return "\f"
		case 'n':
			return "\n"
		case 'r':
			return "\r"
		case 't':
			return "\t"
		default:
			return m[1:]
		}
	})
}

// redactedStream 让流式的文字增量也过脱敏（redact.Text，和落库的回答同一个函数）：攒着全文，每次只发脱敏以后新多出来的部分；末尾还可能变长的一段
// （最后一个 ASCII 词，地址与长串都在里面）先不发。脱敏改了已经发出去的部分时整段替换。
type redactedStream struct {
	full  strings.Builder
	sent  string
	scrub func(string) string
	// key 是这次运行用的 API Key：末尾可能是它开头的一段先不发
	key string
	// kind 是发的事件种类（delta 或 reasoning）；markup 为真时截掉漏进正文的工具调用标记（推理内容不截）
	kind   string
	markup bool
}

// keyScrubber 把这次运行用的 API Key 原样换成 ***（设置要求 Key 至少 minKeyLen 个字符；更短的只能是旧数据，不换，免得误伤正常文字）。
func keyScrubber(key string) func(string) string {
	if len(key) < minKeyLen {
		return func(s string) string { return s }
	}
	return func(s string) string { return strings.ReplaceAll(s, key, "***") }
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

// keyHold 是可以先发的长度：末尾可能是 Key 开头的一段先不发（Key 还没收全，换不掉）。和 keyScrubber 一样，太短的 Key 不管。
func keyHold(s, key string) int {
	if len(key) < minKeyLen {
		return len(s)
	}
	for n := min(len(key)-1, len(s)); n > 0; n-- {
		if strings.HasSuffix(s, key[:n]) {
			return len(s) - n
		}
	}
	return len(s)
}

func (s *Service) emitStream(r *run, rs *redactedStream, chunk string, final bool) {
	rs.full.WriteString(chunk)
	full := rs.full.String()
	if rs.scrub != nil {
		// 先按原文换掉 Key，再截工具调用标记、通用脱敏（顺序反了，Key 可能被截断或改写，原文就对不上了）
		full = rs.scrub(full)
	}
	cut := len(full)
	if rs.markup {
		// 漏进正文的工具调用标记不发
		full = cutToolMarkup(full)
		cut = len(full)
		if !final {
			cut = markupHold(full)
		}
	}
	if !final {
		// 末尾还可能变的一段先不发：ASCII 串（地址与长串）、可能是标记开头的、可能是 Key 开头的
		cut = min(cut, safeCut(full), keyHold(full, rs.key))
	}
	red := redact.Text(full[:cut])
	if red == rs.sent {
		return
	}
	ev := Event{Type: rs.kind, RunID: r.id, At: s.now()}
	if strings.HasPrefix(red, rs.sent) {
		ev.Text = red[len(rs.sent):]
	} else {
		ev.Text, ev.Replace = red, true
	}
	rs.sent = red
	s.hub.publish(r.conv, ev)
}
