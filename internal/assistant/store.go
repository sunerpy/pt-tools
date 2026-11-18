package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/mcp"
	"github.com/sunerpy/pt-tools/internal/redact"
	"github.com/sunerpy/pt-tools/models"
)

// ConversationView 是会话列表里的一项。
type ConversationView struct {
	ID        uint      `json:"id"`
	Title     string    `json:"title"`
	Entry     string    `json:"entry"`
	Running   bool      `json:"running"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ConversationDetail 是一个会话：消息、待确认操作，以及接着读事件用的位置。
type ConversationDetail struct {
	ConversationView
	Messages      []MessageView      `json:"messages"`
	Confirmations []ConfirmationView `json:"confirmations"`
	// RunID 是进行中的运行（没有时为空）
	RunID string `json:"run_id,omitempty"`
	// Partial 是进行中的运行到现在的回答文字
	Partial string `json:"partial,omitempty"`
	// PartialReasoning 是进行中的运行这一步到现在的推理内容（思考过程）
	PartialReasoning string `json:"partial_reasoning,omitempty"`
	// LastSeq 是读事件的 after：从它接着读不会漏
	LastSeq int64 `json:"last_seq"`
	// Usage 是上下文用量（读不出来时没有）
	Usage *UsageView `json:"usage,omitempty"`
}

// MessageView 是一条消息（脱敏以后的）。
type MessageView struct {
	ID      uint   `json:"id"`
	RunID   string `json:"run_id,omitempty"`
	Role    string `json:"role"`
	Content string `json:"content"`
	// Reasoning 是模型的推理内容（思考过程，只有助手消息有）
	Reasoning string `json:"reasoning,omitempty"`
	// Model、Effort 是这一轮的模型与推理等级（为空是设置里的模型、不指定推理等级）
	Model     string         `json:"model,omitempty"`
	Effort    string         `json:"effort,omitempty"`
	ToolCalls []ToolCallView `json:"tool_calls,omitempty"`
	// Attachments 是用户消息带的附件
	Attachments []AttachmentView `json:"attachments,omitempty"`
	// ToolName、ToolCallID 是工具结果对应的调用
	ToolName   string    `json:"tool_name,omitempty"`
	ToolCallID string    `json:"tool_call_id,omitempty"`
	IsError    bool      `json:"is_error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// ToolCallView 是模型的一个工具调用：参数是规范化脱敏以后的 JSON。
type ToolCallView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	// Write 报告是不是改动类操作（要确认）
	Write bool `json:"write,omitempty"`
}

// ConfirmationView 是一个待确认操作。
type ConfirmationView struct {
	ID         uint   `json:"id"`
	RunID      string `json:"run_id"`
	ToolCallID string `json:"tool_call_id"`
	Tool       string `json:"tool"`
	// Summary 是给人看的一句话（有预览时是预览写的，例如「暂停下载器「qb」里的「Dune」」）
	Summary   string         `json:"summary"`
	Arguments map[string]any `json:"arguments"`
	// Changes 是会改什么（改以前、改以后），Impact 是可能有什么影响：写工具的预览给的，没有时为空
	Changes   []mcp.Change `json:"changes,omitempty"`
	Impact    []string     `json:"impact,omitempty"`
	Status    string       `json:"status"`
	DecidedBy string       `json:"decided_by,omitempty"`
	DecidedAt *time.Time   `json:"decided_at,omitempty"`
	ExpiresAt time.Time    `json:"expires_at"`
	CreatedAt time.Time    `json:"created_at"`
}

// 读会话的上限。
const (
	detailMessages      = 200
	detailConfirmations = 100
	listConversations   = 100
)

func (s *Service) conversationView(c models.AssistantConversation) ConversationView {
	return ConversationView{
		ID: c.ID, Title: c.Title, Entry: c.Entry, Running: s.isRunning(c.ID), CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

func messageView(m models.AssistantMessage) MessageView {
	v := MessageView{
		ID: m.ID, RunID: m.RunID, Role: m.Role, Content: m.Content, Model: m.Model, Effort: m.Effort, ToolName: m.ToolName,
		ToolCallID: m.ToolCallID, IsError: m.IsError, CreatedAt: m.CreatedAt,
	}
	if m.Role == models.AssistantRoleAssistant {
		// 落库的推理内容只遮了凭证（要原样回传给模型）；给人看的和回答一样全遮
		v.Reasoning = redact.Text(m.Reasoning)
	}
	if m.ToolCalls != "" {
		_ = json.Unmarshal([]byte(m.ToolCalls), &v.ToolCalls)
	}
	return v
}

func confirmationView(c models.AssistantConfirmation) ConfirmationView {
	args := map[string]any{}
	_ = json.Unmarshal([]byte(c.Arguments), &args)
	v := ConfirmationView{
		ID: c.ID, RunID: c.RunID, ToolCallID: c.ToolCallID, Tool: c.ToolName, Summary: summarize(c.ToolName, args),
		Arguments: args, Status: c.Status, DecidedBy: c.DecidedBy, DecidedAt: c.DecidedAt, ExpiresAt: c.ExpiresAt,
		CreatedAt: c.CreatedAt,
	}
	var pv mcp.Preview
	if c.Preview != "" && json.Unmarshal([]byte(c.Preview), &pv) == nil {
		if strings.TrimSpace(pv.Summary) != "" {
			v.Summary = pv.Summary
		}
		v.Changes, v.Impact = pv.Changes, pv.Impact
	}
	return v
}

// conversation 按属主取会话；不是这个属主的当作不存在。
func (s *Service) conversation(ctx context.Context, a Actor, id uint) (models.AssistantConversation, error) {
	if !a.valid() || id == 0 {
		return models.AssistantConversation{}, ErrNotFound
	}
	var rows []models.AssistantConversation
	if err := s.db(ctx).Where("id = ? AND owner = ?", id, a.Owner).Limit(1).Find(&rows).Error; err != nil {
		return models.AssistantConversation{}, fmt.Errorf("读取会话失败: %w", err)
	}
	if len(rows) == 0 {
		return models.AssistantConversation{}, ErrNotFound
	}
	return rows[0], nil
}

// Conversations 列出属主的会话，最近的在前。
func (s *Service) Conversations(ctx context.Context, a Actor) ([]ConversationView, error) {
	if !a.valid() {
		return nil, ErrNotFound
	}
	var rows []models.AssistantConversation
	if err := s.db(ctx).Where("owner = ?", a.Owner).Order("updated_at desc, id desc").Limit(listConversations).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取会话失败: %w", err)
	}
	out := make([]ConversationView, 0, len(rows))
	for _, c := range rows {
		out = append(out, s.conversationView(c))
	}
	return out, nil
}

// titleFrom 取一段文字的开头当会话标题。
func titleFrom(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= maxTitleRunes {
		return text
	}
	return string([]rune(text)[:maxTitleRunes]) + "…"
}

// CreateConversation 建一个会话。标题为空时用第一条消息的开头。
func (s *Service) CreateConversation(ctx context.Context, a Actor, title string) (ConversationView, error) {
	if !a.valid() {
		return ConversationView{}, ErrNotFound
	}
	c := models.AssistantConversation{Owner: a.Owner, Entry: a.Entry, Title: titleFrom(s.userText(title))}
	now := s.now()
	c.CreatedAt, c.UpdatedAt = now, now
	if err := s.db(ctx).Create(&c).Error; err != nil {
		return ConversationView{}, fmt.Errorf("建会话失败: %w", err)
	}
	return s.conversationView(c), nil
}

// LatestConversation 是属主最近建的会话（ChatOps 的滚动会话）；没有时 ok 为假。按建的先后（编号）选，不按更新时间：
// 开了新会话以后，旧会话里还在跑的回答结束时会更新旧会话的时间，下一条消息也不能回到旧会话。
func (s *Service) LatestConversation(ctx context.Context, a Actor) (ConversationView, bool, error) {
	if !a.valid() {
		return ConversationView{}, false, ErrNotFound
	}
	var rows []models.AssistantConversation
	if err := s.db(ctx).Where("owner = ?", a.Owner).Order("id desc").Limit(1).Find(&rows).Error; err != nil {
		return ConversationView{}, false, fmt.Errorf("读取会话失败: %w", err)
	}
	if len(rows) == 0 {
		return ConversationView{}, false, nil
	}
	return s.conversationView(rows[0]), true, nil
}

// Conversation 读一个会话：最近的消息、待确认操作与接着读事件的位置。
func (s *Service) Conversation(ctx context.Context, a Actor, id uint) (ConversationDetail, error) {
	c, err := s.conversation(ctx, a, id)
	if err != nil {
		return ConversationDetail{}, err
	}
	// 先取事件位置再读库：这之后的事件从 LastSeq 续读都拿得到
	seq, partial, partialReasoning := s.hub.snapshot(id)
	var msgs []models.AssistantMessage
	if err = s.db(ctx).Where("conversation_id = ?", id).Order("id desc").Limit(detailMessages).Find(&msgs).Error; err != nil {
		return ConversationDetail{}, fmt.Errorf("读取消息失败: %w", err)
	}
	var confs []models.AssistantConfirmation
	if err = s.db(ctx).Where("conversation_id = ?", id).Order("id desc").Limit(detailConfirmations).Find(&confs).Error; err != nil {
		return ConversationDetail{}, fmt.Errorf("读取待确认操作失败: %w", err)
	}
	d := ConversationDetail{
		ConversationView: s.conversationView(c), LastSeq: seq,
		Messages: make([]MessageView, 0, len(msgs)), Confirmations: make([]ConfirmationView, 0, len(confs)),
	}
	var userIDs []uint
	for _, m := range msgs {
		if m.Role == models.AssistantRoleUser {
			userIDs = append(userIDs, m.ID)
		}
	}
	atts, err := s.attachmentViews(ctx, userIDs)
	if err != nil {
		return ConversationDetail{}, err
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		v := messageView(msgs[i])
		v.Attachments = atts[msgs[i].ID]
		d.Messages = append(d.Messages, v)
	}
	for i := len(confs) - 1; i >= 0; i-- {
		d.Confirmations = append(d.Confirmations, confirmationView(confs[i]))
	}
	if r := s.activeRun(id); r != nil {
		d.RunID, d.Partial, d.PartialReasoning, d.Running = r.id, partial, partialReasoning, true
	}
	if set, ready, uerr := s.usageSettings(ctx); uerr == nil {
		if ready && set.contextWindow == 0 {
			s.refreshModelsSoon(set.llm)
		}
		if u, uerr := s.conversationUsage(ctx, c, set, ""); uerr == nil {
			d.Usage = &u
		}
	}
	return d, nil
}

// DeleteConversation 删掉会话连同消息与待确认记录；正在回答的回 ErrBusy。
func (s *Service) DeleteConversation(ctx context.Context, a Actor, id uint) error {
	if _, err := s.conversation(ctx, a, id); err != nil {
		return err
	}
	if !s.beginPurge(id) {
		return ErrBusy
	}
	defer s.endPurge(id)
	return s.purge(ctx, id, time.Time{})
}

// beginPurge 占住会话准备删掉：有进行中的运行时返回假。占住期间 Send 当会话不存在。
func (s *Service) beginPurge(id uint) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runs[id] != nil || s.purging[id] {
		return false
	}
	s.purging[id] = true
	return true
}

func (s *Service) endPurge(id uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.purging, id)
}

// purge 在一个事务里删掉会话的消息、附件、待确认记录与会话本身。before 不为零时只删 updated_at 早于它的（保留期清理）。
func (s *Service) purge(ctx context.Context, id uint, before time.Time) error {
	err := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Where("id = ?", id)
		if !before.IsZero() {
			q = q.Where("updated_at < ?", before)
		}
		res := q.Delete(&models.AssistantConversation{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		if err := tx.Where("conversation_id = ?", id).Delete(&models.AssistantMessage{}).Error; err != nil {
			return err
		}
		if err := tx.Where("conversation_id = ?", id).Delete(&models.AssistantAttachment{}).Error; err != nil {
			return err
		}
		return tx.Where("conversation_id = ?", id).Delete(&models.AssistantConfirmation{}).Error
	})
	if err != nil {
		return fmt.Errorf("删除会话失败: %w", err)
	}
	s.hub.forget(id)
	return nil
}

// Cleanup 删掉传了一天还没发的附件，再删保留期外（updated_at 早于现在减保留天数）、没有进行中运行的会话；
// 保留天数为 0 时不清理会话。返回删了几个会话。
func (s *Service) Cleanup(ctx context.Context) (int, error) {
	if err := s.cleanPendingAttachments(ctx); err != nil {
		return 0, err
	}
	row, _, err := s.loadSettings(ctx)
	if err != nil {
		return 0, err
	}
	if row.RetentionDays <= 0 {
		return 0, nil
	}
	before := s.now().AddDate(0, 0, -row.RetentionDays)
	var ids []uint
	if err := s.db(ctx).Model(&models.AssistantConversation{}).Where("updated_at < ?", before).Order("id").
		Limit(500).Pluck("id", &ids).Error; err != nil {
		return 0, fmt.Errorf("查过期会话失败: %w", err)
	}
	n := 0
	for _, id := range ids {
		if !s.beginPurge(id) {
			continue
		}
		err := s.purge(ctx, id, before)
		s.endPurge(id)
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// touch 更新会话的 updated_at（标题为空时顺便用 title 补上）。
func (s *Service) touch(ctx context.Context, id uint, title string) {
	updates := map[string]any{"updated_at": s.now()}
	db := s.db(context.WithoutCancel(ctx)).Model(&models.AssistantConversation{}).Where("id = ?", id)
	if err := db.Updates(updates).Error; err != nil {
		return
	}
	if title != "" {
		s.db(context.WithoutCancel(ctx)).Model(&models.AssistantConversation{}).
			Where("id = ? AND title = ''", id).Update("title", title)
	}
}

// addMessage 落一条消息并发 message 事件。
func (s *Service) addMessage(ctx context.Context, m models.AssistantMessage) (models.AssistantMessage, error) {
	m.CreatedAt = s.now()
	if err := s.db(context.WithoutCancel(ctx)).Create(&m).Error; err != nil {
		return m, fmt.Errorf("保存消息失败: %w", err)
	}
	v := messageView(m)
	s.hub.publish(m.ConversationID, Event{Type: EventMessage, RunID: m.RunID, Message: &v, At: m.CreatedAt})
	return m, nil
}
