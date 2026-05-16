package chatops

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/notify"
)

// BindingInfo is the subset of binding data needed by the chain.
// Defined locally (instead of importing internal/app) to avoid an import
// cycle: internal/app already imports internal/chatops for GenerateBindCode.
type BindingInfo struct {
	ID            uint
	ConfID        uint
	ChannelType   string
	ChannelUserID string
	ReplyLang     string
	PtAdmin       bool
	Allowed       bool
}

// AuditEntry is the audit row written by the chain.
type AuditEntry struct {
	NotificationConfID uint
	ChannelType        string
	ChannelUserID      string
	Command            string
	Args               map[string]any
	Result             string
	LatencyMs          int64
}

// CommandRegistryAPI abstracts CommandRegistry so the chain can be tested
// without depending on the concrete registry implementation.
type CommandRegistryAPI interface {
	Get(name string) (CommandSpec, bool)
	List() []CommandSpec
}

// RateLimiterAPI abstracts RateLimiter for the chain.
type RateLimiterAPI interface {
	Allow(channel, userID, command string) bool
}

// SessionStoreAPI abstracts SessionStore for the chain.
type SessionStoreAPI interface {
	Pending(channel string, confID uint, userID string) (SessionState, bool)
	Set(channel string, confID uint, userID string, state SessionState, ttl time.Duration)
	Clear(channel string, confID uint, userID string)
}

// Replier is the channel-side reply egress (implemented by adapters T17-T20).
type Replier interface {
	Reply(ctx context.Context, msg notify.InboundMessage, reply Reply) error
}

// BindingLookup resolves a (channel, user) pair to a BindingInfo.
type BindingLookup interface {
	FindByChannelUser(ctx context.Context, channelType, channelUserID string) (BindingInfo, bool, error)
}

// BindCodeConsumer consumes a /bind <code> request. Implemented by app.BindingService.
type BindCodeConsumer interface {
	ConsumeCode(ctx context.Context, code, channelType, channelUserID string) error
}

// AuditRecorder writes a single audit row. Implemented by app.AuditService.
type AuditRecorder interface {
	Record(ctx context.Context, e AuditEntry) error
}

// MessageChain dispatches inbound messages through the permission gate per
// plan T22 (binding → allowed → command parse → session → registry →
// ratelimit → admin check → handler).
type MessageChain struct {
	registry    CommandRegistryAPI
	bindings    BindingLookup
	bindCoder   BindCodeConsumer
	auditSvc    AuditRecorder
	rateLimiter RateLimiterAPI
	sessions    SessionStoreAPI
	replier     Replier
	now         func() time.Time
}

// NewMessageChain wires the dependencies. replier may be nil when there is
// no outbound channel (e.g. CLI debugging).
func NewMessageChain(
	registry CommandRegistryAPI,
	bindings BindingLookup,
	bindCoder BindCodeConsumer,
	audit AuditRecorder,
	rl RateLimiterAPI,
	sessions SessionStoreAPI,
	replier Replier,
) *MessageChain {
	return &MessageChain{
		registry:    registry,
		bindings:    bindings,
		bindCoder:   bindCoder,
		auditSvc:    audit,
		rateLimiter: rl,
		sessions:    sessions,
		replier:     replier,
		now:         time.Now,
	}
}

// Process is the inbound entry point. It returns an error only on
// unrecoverable conditions (lookup failure, nil handler); business denies
// are surfaced via audit + reply, never as errors.
func (mc *MessageChain) Process(ctx context.Context, msg notify.InboundMessage) error {
	start := mc.now()

	text := strings.TrimSpace(msg.Text)
	cmdName, args := parseCommand(text)

	binding, hasBinding, err := mc.bindings.FindByChannelUser(ctx, msg.ChannelType, msg.ChannelUserID)
	if err != nil {
		mc.recordAudit(ctx, msg, 0, cmdName, args, "error:lookup_binding", start)
		mc.tryReply(ctx, msg, Reply{Text: "内部错误，请稍后重试"})
		return err
	}

	if !hasBinding {
		if cmdName == "bind" {
			return mc.executeBind(ctx, msg, args, start)
		}
		mc.recordAudit(ctx, msg, 0, cmdName, args, "denied:not_bound", start)
		mc.tryReply(ctx, msg, Reply{Text: "请先 /bind <绑定码> 完成绑定"})
		return nil
	}

	if !binding.Allowed {
		mc.recordAudit(ctx, msg, binding.ConfID, cmdName, args, "denied:not_allowed", start)
		mc.tryReply(ctx, msg, Reply{Text: "该绑定已被禁用"})
		return nil
	}

	src := Source{
		ChannelType:   msg.ChannelType,
		ChannelConfID: binding.ConfID,
		ChannelUserID: msg.ChannelUserID,
		Username:      msg.Username,
		ChatID:        msg.ChatID,
		ReplyLang:     binding.ReplyLang,
		PtAdmin:       binding.PtAdmin,
		IsAdmin:       binding.PtAdmin,
		BindingID:     binding.ID,
	}

	if state, ok := mc.sessions.Pending(msg.ChannelType, binding.ConfID, msg.ChannelUserID); ok {
		mc.sessions.Clear(msg.ChannelType, binding.ConfID, msg.ChannelUserID)
		if state.Handler != nil {
			reply, herr := state.Handler(ctx, []string{text}, src)
			result := "success"
			if herr != nil {
				result = "error:session_handler"
			}
			mc.recordAudit(ctx, msg, binding.ConfID, "session:"+state.Step, args, result, start)
			mc.tryReply(ctx, msg, reply)
			return nil
		}
	}

	if cmdName == "" {
		mc.recordAudit(ctx, msg, binding.ConfID, "", nil, "user_message_ignored", start)
		return nil
	}

	spec, ok := mc.registry.Get(cmdName)
	if !ok {
		mc.recordAudit(ctx, msg, binding.ConfID, cmdName, args, "denied:unknown_command", start)
		mc.tryReply(ctx, msg, Reply{Text: "命令未知，发送 /help 查看"})
		return nil
	}

	if !mc.rateLimiter.Allow(msg.ChannelType, msg.ChannelUserID, spec.Name) {
		mc.recordAudit(ctx, msg, binding.ConfID, spec.Name, args, "denied:rate_limit", start)
		return nil
	}

	if spec.AdminOnly && !binding.PtAdmin {
		mc.recordAudit(ctx, msg, binding.ConfID, spec.Name, args, "denied:not_admin", start)
		mc.tryReply(ctx, msg, Reply{Text: "需管理员权限"})
		return nil
	}

	if spec.Handler == nil {
		mc.recordAudit(ctx, msg, binding.ConfID, spec.Name, args, "error:nil_handler", start)
		return errors.New("command handler is nil: " + spec.Name)
	}
	reply, herr := spec.Handler(ctx, args, src)
	result := "success"
	if herr != nil {
		result = "error:handler"
	}
	mc.recordAudit(ctx, msg, binding.ConfID, spec.Name, args, result, start)
	mc.tryReply(ctx, msg, reply)
	return nil
}

func (mc *MessageChain) executeBind(ctx context.Context, msg notify.InboundMessage, args []string, start time.Time) error {
	if len(args) == 0 {
		mc.recordAudit(ctx, msg, 0, "bind", args, "denied:missing_code", start)
		mc.tryReply(ctx, msg, Reply{Text: "用法: /bind <绑定码>"})
		return nil
	}
	code := args[0]
	if mc.bindCoder == nil {
		mc.recordAudit(ctx, msg, 0, "bind", args, "error:no_binding_service", start)
		return errors.New("binding service unavailable")
	}
	if err := mc.bindCoder.ConsumeCode(ctx, code, msg.ChannelType, msg.ChannelUserID); err != nil {
		mc.recordAudit(ctx, msg, 0, "bind", args, "error:bind_failed", start)
		mc.tryReply(ctx, msg, Reply{Text: "绑定失败：" + err.Error()})
		return nil
	}
	mc.recordAudit(ctx, msg, 0, "bind", args, "success", start)
	mc.tryReply(ctx, msg, Reply{Text: "绑定成功"})
	return nil
}

func (mc *MessageChain) recordAudit(ctx context.Context, msg notify.InboundMessage, confID uint, command string, args []string, result string, start time.Time) {
	if mc.auditSvc == nil {
		return
	}
	latency := mc.now().Sub(start).Milliseconds()
	argsMap := map[string]any{}
	if len(args) > 0 {
		argsMap["args"] = args
	}
	_ = mc.auditSvc.Record(ctx, AuditEntry{
		NotificationConfID: confID,
		ChannelType:        msg.ChannelType,
		ChannelUserID:      msg.ChannelUserID,
		Command:            command,
		Args:               argsMap,
		Result:             result,
		LatencyMs:          latency,
	})
}

func (mc *MessageChain) tryReply(ctx context.Context, msg notify.InboundMessage, reply Reply) {
	if mc.replier == nil || reply.SilentDrop {
		return
	}
	if reply.Text == "" && len(reply.Buttons) == 0 {
		return
	}
	_ = mc.replier.Reply(ctx, msg, reply)
}

func parseCommand(text string) (string, []string) {
	if !strings.HasPrefix(text, "/") {
		return "", nil
	}
	parts := strings.Fields(text[1:])
	if len(parts) == 0 {
		return "", nil
	}
	name := strings.ToLower(parts[0])
	if at := strings.Index(name, "@"); at >= 0 {
		name = name[:at]
	}
	return name, parts[1:]
}
