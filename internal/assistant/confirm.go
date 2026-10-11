package assistant

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/sunerpy/pt-tools/models"
)

// confirmThenCall 给写操作建一条待确认记录，等结论：批准了才带 confirm=true 调用一次，拒绝、超时、停止都不执行，
// 把原因当工具结果交回模型。
func (s *Service) confirmThenCall(r *run, tb *toolbox, set effective, pc pendingCall) toolResult {
	timeout := time.Duration(set.confirmMinutes) * s.confirmUnit
	now := s.now()
	c := models.AssistantConfirmation{
		ConversationID: r.conv, RunID: r.id, ToolCallID: pc.id, ToolName: pc.name, Arguments: pc.display,
		Status: models.AssistantConfirmPending, ExpiresAt: now.Add(timeout), CreatedAt: now,
	}
	if err := s.db(context.WithoutCancel(r.ctx)).Create(&c).Error; err != nil {
		return toolResult{text: "建待确认记录失败，操作没有执行。", isError: true}
	}
	s.publishConfirmation(r, c)
	final, err := s.awaitDecision(r, c.ID, timeout)
	if err != nil {
		return toolResult{text: "读取确认结论失败，操作没有执行。", isError: true}
	}
	s.publishConfirmation(r, final)
	switch final.Status {
	case models.AssistantConfirmApproved:
		if stopped(r) {
			return toolResult{text: "运行已停止，操作没有执行。", isError: true}
		}
		args := maps.Clone(pc.args)
		args["confirm"] = true
		s.toolStarted(r, pc, true)
		return tb.call(r.ctx, pc.name, args)
	case models.AssistantConfirmDenied:
		return toolResult{text: "用户拒绝了这个操作，没有执行。"}
	}
	if final.DecidedBy == decidedByTimeout {
		return toolResult{text: fmt.Sprintf("用户没有在 %d 分钟内确认，操作没有执行。", set.confirmMinutes)}
	}
	return toolResult{text: "运行已停止，操作没有执行。", isError: true}
}

// 待确认记录在没有人定的时候 decided_by 记的原因。
const (
	decidedByTimeout  = "timeout"
	decidedByCancel   = "cancel"
	decidedByShutdown = "shutdown"
)

// publishConfirmation 发 confirmation 事件并交给 Notify。
func (s *Service) publishConfirmation(r *run, c models.AssistantConfirmation) {
	v := confirmationView(c)
	ev := s.hub.publish(r.conv, Event{Type: EventConfirmation, RunID: r.id, Confirmation: &v, At: s.now()})
	s.notify(r, ev)
}

// awaitDecision 等待确认记录有结论：有人确认或拒绝（Decide 发信号）、超时、运行被停（后两种由这里经条件更新标成过期）。
// 结论一律以库里读到的为准。
func (s *Service) awaitDecision(r *run, id uint, timeout time.Duration) (models.AssistantConfirmation, error) {
	ctx := context.WithoutCancel(r.ctx)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	timerC, done := timer.C, r.ctx.Done()
	for {
		c, err := s.loadConfirmation(ctx, id)
		if err != nil {
			return c, err
		}
		if c.Status != models.AssistantConfirmPending {
			return c, nil
		}
		if timerC == nil && done == nil {
			return c, errors.New("待确认记录改不动")
		}
		select {
		case <-r.decided:
		case <-timerC:
			timerC = nil
			if _, err := s.decide(ctx, id, models.AssistantConfirmExpired, decidedByTimeout); err != nil {
				return c, err
			}
		case <-done:
			done = nil
			by := decidedByCancel
			if r.shuttingDown() {
				by = decidedByShutdown
			}
			if _, err := s.decide(ctx, id, models.AssistantConfirmExpired, by); err != nil {
				return c, err
			}
		}
	}
}

func (s *Service) loadConfirmation(ctx context.Context, id uint) (models.AssistantConfirmation, error) {
	var rows []models.AssistantConfirmation
	if err := s.db(ctx).Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return models.AssistantConfirmation{}, fmt.Errorf("读取待确认操作失败: %w", err)
	}
	if len(rows) == 0 {
		return models.AssistantConfirmation{}, ErrNotFound
	}
	return rows[0], nil
}

// decide 是唯一改待确认状态的地方：只有 status 还是 pending 时才改，改到一行的一方赢。
func (s *Service) decide(ctx context.Context, id uint, status, by string) (bool, error) {
	now := s.now()
	res := s.db(ctx).Model(&models.AssistantConfirmation{}).
		Where("id = ? AND status = ?", id, models.AssistantConfirmPending).
		Updates(map[string]any{"status": status, "decided_by": by, "decided_at": &now})
	if res.Error != nil {
		return false, fmt.Errorf("更新待确认操作失败: %w", res.Error)
	}
	return res.RowsAffected == 1, nil
}

// Decide 是属主确认（approve 为真）或拒绝一个待确认操作。别人的当作不存在；不能写的主体回 ErrForbidden；
// 已经有结论的（含同时点的另一次）回 ErrDecided 与现在的状态。
func (s *Service) Decide(ctx context.Context, a Actor, id uint, approve bool) (ConfirmationView, error) {
	c, err := s.loadConfirmation(ctx, id)
	if err != nil {
		return ConfirmationView{}, err
	}
	if _, err = s.conversation(ctx, a, c.ConversationID); err != nil {
		return ConfirmationView{}, err
	}
	if !a.CanWrite {
		return ConfirmationView{}, ErrForbidden
	}
	status := models.AssistantConfirmDenied
	if approve {
		status = models.AssistantConfirmApproved
	}
	won, err := s.decide(ctx, id, status, a.Owner)
	if err != nil {
		return ConfirmationView{}, err
	}
	if c, err = s.loadConfirmation(ctx, id); err != nil {
		return ConfirmationView{}, err
	}
	if !won {
		return confirmationView(c), ErrDecided
	}
	if r := s.activeRun(c.ConversationID); r != nil {
		select {
		case r.decided <- struct{}{}:
		default:
		}
	}
	return confirmationView(c), nil
}

// PendingConfirmation 是属主最近一个还在等的待确认操作（ChatOps 用它解读回复）；没有时 ok 为假。
func (s *Service) PendingConfirmation(ctx context.Context, a Actor) (ConfirmationView, bool, error) {
	if !a.valid() {
		return ConfirmationView{}, false, ErrNotFound
	}
	var rows []models.AssistantConfirmation
	err := s.db(ctx).Table("assistant_confirmations AS c").Select("c.*").
		Joins("JOIN assistant_conversations AS v ON v.id = c.conversation_id").
		Where("v.owner = ? AND c.status = ?", a.Owner, models.AssistantConfirmPending).
		Order("c.id desc").Limit(1).Find(&rows).Error
	if err != nil {
		return ConfirmationView{}, false, fmt.Errorf("读取待确认操作失败: %w", err)
	}
	if len(rows) == 0 {
		return ConfirmationView{}, false, nil
	}
	return confirmationView(rows[0]), true, nil
}
