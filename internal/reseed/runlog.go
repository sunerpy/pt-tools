package reseed

import (
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/sunerpy/pt-tools/models"
)

// 轮次的步骤是给人看的时间线：读了哪些下载器、每一批查到什么、加入了什么、哪些失败、为什么停下。
// 上限与刷流的运行记录相同：步骤 JSON 最多 16 KiB，一步最多 500 个字。
const (
	runLogMaxBytes = 16 << 10
	runLogStepText = 500
)

// 步骤的种类（前端按它标颜色）。
const (
	stepList  = "list"
	stepQuery = "query"
	stepAdd   = "add"
	stepFail  = "fail"
	stepGone  = "gone"
	stepSkip  = "skip"
	stepStop  = "stop"
	stepError = "error"
	stepInfo  = "info"
)

// runLog 收集一轮的步骤；一轮在一个 goroutine 里跑，不用加锁。nil 时什么都不记。
type runLog struct {
	steps     []models.ReseedRunStep
	size      int
	truncated bool
}

func (l *runLog) add(kind, format string, args ...any) {
	if l == nil {
		return
	}
	text := format
	if len(args) > 0 {
		text = fmt.Sprintf(format, args...)
	}
	if utf8.RuneCountInString(text) > runLogStepText {
		text = string([]rune(text)[:runLogStepText]) + "…"
	}
	n := len(kind) + len(text) + 24 // 估算 JSON：{"kind":"","text":""},
	if l.size+n > runLogMaxBytes {
		l.truncated = true
		return
	}
	l.size += n
	l.steps = append(l.steps, models.ReseedRunStep{Kind: kind, Text: text})
}

// encoded 是存进 reseed_runs.steps 的 JSON；没有步骤时为空。
func (l *runLog) encoded() (string, bool) {
	if l == nil || len(l.steps) == 0 {
		return "", l != nil && l.truncated
	}
	b, err := json.Marshal(l.steps)
	if err != nil {
		return "", true
	}
	return string(b), l.truncated
}

// humanDuration 是给人看的时长：「3 分钟」「1 小时 5 分钟」「20 秒」。
func humanDuration(d time.Duration) string {
	d = d.Round(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d 秒", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟", int(d.Minutes()))
	}
	h, m := int(d.Hours()), int(d.Minutes())%60
	if m == 0 {
		return fmt.Sprintf("%d 小时", h)
	}
	return fmt.Sprintf("%d 小时 %d 分钟", h, m)
}
