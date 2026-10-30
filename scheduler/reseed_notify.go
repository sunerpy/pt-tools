package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/reseed"
	"github.com/sunerpy/pt-tools/models"
)

const (
	// reseedNotifySource、reseedNotifyKind 是辅种每轮结果在 MonitorNotificationLog 里的来源与种类（审计里的命令是 notify:reseed:round）。
	reseedNotifySource = "reseed"
	reseedNotifyKind   = "round"
)

// notifyRound 在一轮结束以后按辅种设置推送这一轮的结果：每一轮，或者只在有变化时（见 reseed.RoundChanged）。
// 同一轮只写一次（EventKey 是轮次编号加开始时间），按通道静默时段顺延，由投递器发送并记进操作审计。
// pt-tools 退出时被打断的轮次不推送；paused 是这一轮暂停了多久（不算进用时）。
func (w *ReseedWorker) notifyRound(ctx context.Context, a *reseedActive, res reseed.RunResult, status, errText, summary string, paused time.Duration) {
	n := w.cfg.Notifier
	if !n.Enabled() || w.cfg.Service == nil || status == models.ReseedRunInterrupted {
		return
	}
	mode, channels, err := w.cfg.Service.NotifyTarget(ctx)
	if err != nil {
		w.cfg.Logger.Warnf("[辅种] 读取推送设置失败，第 %d 轮的结果没有推送: %v", a.id, err)
		return
	}
	switch {
	case mode == models.ReseedNotifyOff,
		// 没有选通道时不发：空的通道列表在投递器里是「全部启用的通道」
		len(channels) == 0,
		mode == models.ReseedNotifyChanges && !reseed.RoundChanged(res, status):
		return
	}
	now := w.cfg.Clock.Now()
	took := max(now.Sub(a.started)-paused, 0)
	notice := reseed.NewRoundNotice(reseed.Round{
		ID: a.id, Trigger: a.trigger, Status: status, Reason: errText, Summary: summary, Took: took, Result: res,
	})
	key := fmt.Sprintf("%d:%d", a.id, a.started.Unix())
	inserted, err := n.Enqueue(ctx, MonitorNotifyEntry{
		Source:   reseedNotifySource,
		Subject:  "*",
		Kind:     reseedNotifyKind,
		EventKey: key,
		Title:    notice.Title,
		Text:     notice.Text,
		ConfIDs:  channels,
		Event:    notify.NewEvent(notify.EventReseedRound, now, notice.Fields),
	})
	switch {
	case err != nil:
		w.cfg.Logger.Warnf("[辅种] 第 %d 轮的结果没有推送: %v", a.id, err)
	case inserted == 0 && !n.Logged(ctx, reseedNotifySource, "*", reseedNotifyKind, key):
		w.cfg.Logger.Warnf("[辅种] 第 %d 轮的结果没有推送：选中的通知通道都不可用（停用或已删除）", a.id)
	}
}
