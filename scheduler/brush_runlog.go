package scheduler

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 刷流的运行记录：每一轮（定时或「立即运行」）开始时写一条进行中的记录，跑完补上结果与步骤。
// 步骤是给人看的时间线：读了哪个列表、哪些种子不符合条件、加了什么、删了什么、为什么停下。

// 运行记录的上限：步骤 JSON 最多 16 KiB；不符合条件的种子逐个列出最多 30 个；每个任务保留最近 500 条、不超过 14 天。
const (
	brushRunMaxSteps   = 16 << 10
	brushRunMaxRejects = 30
	brushRunStepText   = 500
	brushRunKeep       = 500
	brushRunKeepFor    = 14 * 24 * time.Hour
)

// 步骤的种类（前端按它标颜色）。
const (
	brushStepList   = "list"
	brushStepReject = "reject"
	brushStepAdd    = "add"
	brushStepRemove = "remove"
	brushStepGone   = "gone"
	brushStepSkip   = "skip"
	brushStepStop   = "stop"
	brushStepError  = "error"
	brushStepInfo   = "info"
)

// brushRunLog 收集一轮的步骤；一轮在一个 goroutine 里跑，不用加锁。nil 时什么都不记（单测直接调 admit 等）。
type brushRunLog struct {
	steps     []models.BrushRunStep
	size      int
	truncated bool
}

func (l *brushRunLog) add(kind, text string) {
	if l == nil {
		return
	}
	if utf8.RuneCountInString(text) > brushRunStepText {
		text = string([]rune(text)[:brushRunStepText]) + "…"
	}
	// 估算 JSON 的长度：{"kind":"","text":""}, 加上两个字段
	n := len(kind) + len(text) + 24
	if l.size+n > brushRunMaxSteps {
		l.truncated = true
		return
	}
	l.size += n
	l.steps = append(l.steps, models.BrushRunStep{Kind: kind, Text: text})
}

// encoded 是存进 brush_runs.steps 的 JSON；没有步骤时为空。
func (l *brushRunLog) encoded() (string, bool) {
	if l == nil || len(l.steps) == 0 {
		return "", l != nil && l.truncated
	}
	b, err := json.Marshal(l.steps)
	if err != nil {
		return "", true
	}
	return string(b), l.truncated
}

// step 记一步。
func (r *BrushRunResult) step(kind, format string, args ...any) {
	r.log.add(kind, fmt.Sprintf(format, args...))
}

// addError 记一个不影响这一轮继续的错误：写进 Errors，也记成一步。
func (r *BrushRunResult) addError(text string) {
	r.Errors = append(r.Errors, text)
	r.log.add(brushStepError, text)
}

// addSkip 记一个下载以后跳过的种子：写进 Skipped，也记成一步。
func (r *BrushRunResult) addSkip(title, why string) {
	r.Skipped = append(r.Skipped, title+"："+why)
	r.log.add(brushStepSkip, title+"："+why)
}

// stop 记下这一轮为什么不再加种。
func (r *BrushRunResult) stop(why string) {
	r.Stopped = why
	r.log.add(brushStepStop, why)
}

// brushDiscountLabels 与前端 BRUSH_DISCOUNTS 的写法一致。
var brushDiscountLabels = map[v2.DiscountLevel]string{
	v2.DiscountFree:      "免费",
	v2.Discount2xFree:    "2x 免费",
	v2.Discount2x50:      "2x 50%",
	v2.DiscountPercent50: "50%",
	v2.DiscountPercent30: "30%",
	v2.DiscountPercent70: "70%",
	v2.Discount2xUp:      "2x 上传",
	v2.DiscountNone:      "无优惠",
}

func brushDiscountLabel(d v2.DiscountLevel) string {
	if s, ok := brushDiscountLabels[d]; ok {
		return s
	}
	if d == "" {
		return "无优惠"
	}
	return string(d)
}

func brushGB(bytes int64) string {
	return fmt.Sprintf("%.2f GB", float64(bytes)/gib)
}

// brushAddedText 是加入一个种子那一步的文字：标题与优惠、体积、优惠到什么时候、H&R 做种要求（「加入」由步骤的种类表示）。
func brushAddedText(title string, d v2.DiscountLevel, size int64, freeEnd *time.Time, hasHR bool, hrHours int, loc *time.Location) string {
	parts := []string{brushDiscountLabel(d), brushGB(size)}
	if freeEnd != nil {
		parts = append(parts, "优惠到 "+freeEnd.In(loc).Format("01-02 15:04"))
	}
	switch {
	case hasHR && hrHours > 0:
		parts = append(parts, fmt.Sprintf("H&R 做种 %d 小时", hrHours))
	case hasHR:
		parts = append(parts, "H&R")
	}
	return fmt.Sprintf("%s（%s）", title, strings.Join(parts, "，"))
}

// brushRejectReasons 是这一轮不符合条件的原因与个数（多的在前），要求未知时提示去站点设置里填。
func brushRejectReasons(rejects []BrushReject) string {
	reasons := make([]string, 0, len(rejects))
	hint := false
	for _, rj := range rejects {
		reasons = append(reasons, fmt.Sprintf("%s %d 个", rj.Reason, rj.Count))
		hint = hint || rj.Reason == brushRejectHRUnknown
	}
	s := "原因汇总：" + strings.Join(reasons, "、")
	if hint {
		s += "（可以在站点设置里填写 H&R 做种要求）"
	}
	return s
}

// startRun 写一条进行中的运行记录；写不进去时只记日志，这一轮照常跑。
func (m *BrushMonitor) startRun(task *models.BrushTask, trigger string, at time.Time) *models.BrushRun {
	run, err := m.repo.StartRun(task.ID, trigger, at)
	if err != nil {
		m.cfg.Logger.Warnf("[刷流] 任务 %s：%v", task.Name, err)
		return nil
	}
	return run
}

// finishRun 补上这一轮的结果、计数与步骤。进程正在退出时（root 已取消）记成没跑完。
func (m *BrushMonitor) finishRun(task *models.BrushTask, run *models.BrushRun, res *BrushRunResult, runErr error, errText string, at time.Time) {
	if run == nil {
		return
	}
	status := models.BrushRunOK
	switch {
	case m.root.Err() != nil:
		status = models.BrushRunInterrupted
		errText = strings.TrimSuffix("pt-tools 正在退出，这一轮没有跑完；"+errText, "；")
	case runErr != nil:
		status = models.BrushRunFailed
	case len(res.Errors) > 0:
		status = models.BrushRunPartial
	}
	steps, truncated := res.log.encoded()
	run.FinishedAt = &at
	run.Status = status
	run.Summary = res.Summary()
	run.Error = errText
	run.Listed, run.Eligible, run.Added, run.Removed, run.Gone = res.Listed, res.Eligible, res.Added, res.Removed, res.Gone
	run.Steps, run.StepsTruncated = steps, truncated
	if err := m.repo.FinishRun(run); err != nil {
		m.cfg.Logger.Warnf("[刷流] 任务 %s：%v", task.Name, err)
	}
}
