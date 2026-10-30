package reseed

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/utils"
)

// ---------- 通知 ----------
//
// 2026-10-30 用户：「辅种任务相关统计也可以通过开关加入到 chatops 消息通知渠道中」。
// 两处：每一轮结束以后推送这一轮的结果（关闭、每一轮、只在有变化时），每日战报里加一行今天的辅种。

// notifyModeOf 是生效的推送方式：空串（旧数据）与不认识的都是关闭。
func notifyModeOf(mode string) string {
	switch mode {
	case models.ReseedNotifyAlways, models.ReseedNotifyChanges:
		return mode
	}
	return models.ReseedNotifyOff
}

// notifyCols 校验推送设置，把要改的列放进 cols：推送开着时至少要选一个通道，选中的通道都要存在（同每日战报的校验）。
// 没有给的项保留原来的。
func (s *Service) notifyCols(ctx context.Context, r models.ReseedSetting, u SettingsUpdate, cols map[string]any) error {
	mode, channels := notifyModeOf(r.NotifyMode), decodeUints(r.NotifyChannelIDs)
	if u.NotifyMode != nil {
		switch m := strings.TrimSpace(*u.NotifyMode); m {
		case "", models.ReseedNotifyOff:
			mode = models.ReseedNotifyOff
		case models.ReseedNotifyAlways, models.ReseedNotifyChanges:
			mode = m
		default:
			return fmt.Errorf("%w：每轮结果推送只能是 off、always、changes", ErrInvalid)
		}
		cols["notify_mode"] = mode
	}
	if u.NotifyChannelIDs != nil {
		channels = dedupeUints(*u.NotifyChannelIDs)
		cols["notify_channel_ids"] = encode(channels)
	}
	if u.DailyReport != nil {
		cols["daily_report"] = *u.DailyReport
	}
	if mode != models.ReseedNotifyOff && len(channels) == 0 {
		return fmt.Errorf("%w：推送每轮结果至少要选一个通知通道", ErrInvalid)
	}
	if (u.NotifyMode == nil && u.NotifyChannelIDs == nil) || len(channels) == 0 {
		return nil
	}
	var n int64
	if err := s.cfg.DB.WithContext(ctx).Model(&models.NotificationConf{}).Where("id IN ?", channels).Count(&n).Error; err != nil {
		return fmt.Errorf("检查通知通道失败: %w", err)
	}
	if int(n) != len(channels) {
		return fmt.Errorf("%w：选中的通知通道有的已经不存在，请重新选择", ErrInvalid)
	}
	return nil
}

// NotifyTarget 是每轮结果推送的设置：mode 是生效的推送方式（off 时不推送），channels 是接收的通道。
func (s *Service) NotifyTarget(ctx context.Context) (mode string, channels []uint, err error) {
	r, err := s.row(ctx)
	if err != nil {
		return models.ReseedNotifyOff, nil, err
	}
	return notifyModeOf(r.NotifyMode), decodeUints(r.NotifyChannelIDs), nil
}

// RoundChanged 报告这一轮算不算「有变化」：加入了辅种、有失败，或者没跑完（部分出错、提前停下、被停止、没能开始）。
func RoundChanged(res RunResult, status string) bool {
	return res.Created > 0 || res.Failed > 0 || status != models.ReseedRunOK
}

// 推送里轮次状态与触发方式的叫法（与网页上的一致）。
var (
	roundStatusText = map[string]string{
		models.ReseedRunOK:      "完成",
		models.ReseedRunPartial: "完成，部分出错",
		models.ReseedRunLimited: "提前停下",
		models.ReseedRunStopped: "已停止",
		models.ReseedRunFailed:  "没能开始",
	}
	roundTriggerText = map[string]string{
		models.ReseedTriggerSchedule: "定时",
		models.ReseedTriggerManual:   "立即运行",
		models.ReseedTriggerRetry:    "重试",
		models.ReseedTriggerContinue: "继续",
	}
)

// RoundNotice 是一轮结束以后推送的内容。
type RoundNotice struct {
	Title string
	Text  string
	// Fields 是事件 reseed.round 的变量
	Fields map[string]any
}

// Round 是推送需要的这一轮的情况：Took 是用了多久（不含暂停的时间），Reason 是没跑完的原因。
type Round struct {
	ID      uint
	Trigger string
	Status  string
	Reason  string
	Summary string
	Took    time.Duration
	Result  RunResult
}

// NewRoundNotice 写这一轮的推送：正文只有计数与状态，不列种子名。
func NewRoundNotice(r Round) RoundNotice {
	status := roundStatusText[r.Status]
	if status == "" {
		status = r.Status
	}
	trigger := roundTriggerText[r.Trigger]
	if trigger == "" {
		trigger = r.Trigger
	}
	res := r.Result
	var b strings.Builder
	b.WriteString(strings.TrimRight(r.Summary, "。 "))
	b.WriteString("。\n用时 ")
	b.WriteString(humanDuration(r.Took))
	if res.IYUUCalls > 0 {
		fmt.Fprintf(&b, "，请求 IYUU %d 次", res.IYUUCalls)
	}
	b.WriteString("。")
	if (r.Status == models.ReseedRunLimited || r.Status == models.ReseedRunStopped) && r.Trigger != models.ReseedTriggerRetry {
		b.WriteString("\n下一轮接着往下查；也可以在「IYUU 辅种 → 轮次」里点「继续」。")
	}
	reason := ""
	switch r.Status {
	case models.ReseedRunLimited, models.ReseedRunStopped, models.ReseedRunFailed:
		reason = r.Reason
	}
	return RoundNotice{
		Title: fmt.Sprintf("辅种：第 %d 轮%s（%s）", r.ID, status, trigger),
		Text:  b.String(),
		Fields: map[string]any{
			"run_id": r.ID, "trigger": r.Trigger, "trigger_label": trigger, "status": r.Status, "status_label": status,
			"hashes": res.Hashes, "candidates": res.Candidates, "created": res.Created, "failed": res.Failed, "gone": res.Gone,
			"skipped": res.SkippedTotal(), "iyuu_calls": res.IYUUCalls,
			"duration": humanDuration(r.Took), "duration_seconds": int64(r.Took.Round(time.Second) / time.Second),
			"stopped": reason, "summary": r.Summary,
		},
	}
}

// DailyReportLine 是每日战报里今天的辅种那一行：设置里没开「每日战报」、或者这天没有建成、成功、失败与站点已删除时为空串。
// day 是战报的日期（YYYY-MM-DD，进程时区）；数字与统计面板同一套算法。不写跳过：每一轮都会把同一个种子再算一次。
func (s *Service) DailyReportLine(ctx context.Context, day string) (string, error) {
	r, err := s.row(ctx)
	if err != nil || !r.DailyReport {
		return "", err
	}
	st, err := s.Stats(ctx, StatsQuery{Granularity: utils.PeriodDay, From: day, To: day})
	if err != nil {
		return "", err
	}
	t := st.Totals
	failed := t.Failed + t.JobFailed + t.RolledBack
	if t.Created+t.Succeeded+failed+t.Gone == 0 {
		return "", nil
	}
	return fmt.Sprintf("🌱 今日辅种：建成 %d · 成功 %d · 失败 %d · 站点已删除 %d", t.Created, t.Succeeded, failed, t.Gone), nil
}
