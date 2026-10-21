package subscribe

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

// 运行记录：每次主动搜索、RSS 命中、建立与手动搜索各记一条，步骤是给人看的时间线（追剧页与 AI 助手都读它）。

// RunStep 是一次运行里的一步。
type RunStep struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// 步骤的种类。
const (
	stepSearch = "search"
	stepReject = "reject"
	stepLock   = "lock"
	stepPush   = "push"
	stepFail   = "fail"
	stepAdopt  = "adopt"
	stepWait   = "wait"
	stepInfo   = "info"
)

// 运行记录的上限：步骤 JSON 最多 16 KiB，每个订阅保留最近 100 条、不超过 90 天；拒绝的候选最多记 20 条。
const (
	maxRunSteps   = 16 << 10
	keepRuns      = 100
	keepRunsFor   = 90 * 24 * time.Hour
	maxRunRejects = 20
	pruneEvery    = time.Hour
	// failStreak 是连续几次运行失败时发一次通知
	failStreak = 3
)

// runLog 收集一次运行的步骤与结果（经 ctx 传到评估与推送）。
type runLog struct {
	mu        sync.Mutex
	steps     []RunStep
	size      int
	truncated bool
	rejects   int
	// pushed 是这次推送成功的种子，pushFailed 是推送失败的次数，searchFailed 是搜索出错的次数
	pushed       []string
	pushFailed   int
	searchFailed int
	// waiting：这次没推是在等（版本锁、读不到下载器、缺的都在下载）
	waiting bool
	// changed：这是一次手动改动，不是搜索
	changed bool
	// fetchFailed 是种子文件没下载下来的次数，siteFetchFails 是每个站点这一轮连着没下载下来的次数（下载成一次就清零）
	fetchFailed    int
	siteFetchFails map[string]int
	// unfinished 不为空时这一轮没推完就停了（时间用完了、推满了一轮的上限），是原因
	unfinished string
}

// fetched 记下一次在 site 下载种子文件的结果（调用方持有 rl.mu，见 markRun）。
func (rl *runLog) fetched(site string, ok bool) {
	if ok {
		delete(rl.siteFetchFails, site)
		return
	}
	rl.fetchFailed++
	if rl.siteFetchFails == nil {
		rl.siteFetchFails = map[string]int{}
	}
	rl.siteFetchFails[site]++
}

// siteFetchDown 报告这一轮在 site 是不是已经连着 siteFetchFailLimit 次没下载到种子文件（ctx 里没有运行记录时为假）。
func siteFetchDown(ctx context.Context, site string) bool {
	rl := runOf(ctx)
	if rl == nil {
		return false
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.siteFetchFails[site] >= siteFetchFailLimit
}

func (rl *runLog) add(kind, text string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if kind == stepReject {
		rl.rejects++
		if rl.rejects > maxRunRejects {
			rl.truncated = true
			return
		}
	}
	text = truncate(text, 1024)
	if rl.size+len(text)+len(kind)+24 > maxRunSteps {
		rl.truncated = true
		return
	}
	rl.size += len(text) + len(kind) + 24
	rl.steps = append(rl.steps, RunStep{Kind: kind, Text: text})
}

type runKey struct{}

func withRun(ctx context.Context, rl *runLog) context.Context {
	return context.WithValue(ctx, runKey{}, rl)
}

func runOf(ctx context.Context) *runLog {
	rl, _ := ctx.Value(runKey{}).(*runLog)
	return rl
}

// step 记一步（ctx 里没有运行记录时什么都不做）。
func step(ctx context.Context, kind, format string, args ...any) {
	if rl := runOf(ctx); rl != nil {
		rl.add(kind, fmt.Sprintf(format, args...))
	}
}

// markRun 改一次运行的结果标记。
func markRun(ctx context.Context, f func(rl *runLog)) {
	if rl := runOf(ctx); rl != nil {
		rl.mu.Lock()
		f(rl)
		rl.mu.Unlock()
	}
}

// startRun 开始一次运行：先写一条进行中的记录（进程中途退出时也看得到开始过）。
func (s *Service) startRun(ctx context.Context, sub *models.MediaSubscription, trigger string) (context.Context, *models.MediaSubscriptionRun) {
	rl := &runLog{}
	run := &models.MediaSubscriptionRun{SubscriptionID: sub.ID, Trigger: trigger, StartedAt: s.cfg.Now()}
	if err := s.cfg.DB.WithContext(ctx).Create(run).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 写运行记录失败: %v", err)
		run = nil
	}
	return withRun(ctx, rl), run
}

// runStatus 按收集到的结果定这次运行的状态。
func runStatus(rl *runLog) string {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	switch {
	case len(rl.pushed) > 0 && rl.pushFailed > 0:
		return models.MediaRunPartial
	case len(rl.pushed) > 0:
		return models.MediaRunOK
	case rl.pushFailed > 0 || rl.searchFailed > 0:
		return models.MediaRunFailed
	case rl.waiting:
		return models.MediaRunWaiting
	case rl.changed:
		return models.MediaRunChanged
	}
	return models.MediaRunNothing
}

// finishRun 写完一次运行：状态、摘要与步骤，订阅的最近一次运行。onlyIfSteps 为真时（RSS 命中）没有任何步骤就删掉这条记录。
// 连续 failStreak 次失败时发一次通知。
func (s *Service) finishRun(ctx context.Context, sub *models.MediaSubscription, run *models.MediaSubscriptionRun, summary string, onlyIfSteps bool) {
	rl := runOf(ctx)
	if run == nil || rl == nil {
		return
	}
	db := s.cfg.DB.WithContext(context.WithoutCancel(ctx))
	rl.mu.Lock()
	steps, truncated := append([]RunStep(nil), rl.steps...), rl.truncated
	rl.mu.Unlock()
	if onlyIfSteps && len(steps) == 0 {
		db.Delete(&models.MediaSubscriptionRun{}, run.ID)
		return
	}
	status := runStatus(rl)
	raw := ""
	if len(steps) > 0 {
		b, _ := json.Marshal(steps)
		raw = string(b)
	}
	now := s.cfg.Now()
	run.FinishedAt, run.Status, run.Summary, run.Steps, run.StepsTruncated = &now, status, truncate(summary, 1024), raw, truncated
	if err := db.Model(&models.MediaSubscriptionRun{}).Where("id = ?", run.ID).Updates(map[string]any{
		"finished_at": now, "status": status, "summary": run.Summary, "steps": raw, "steps_truncated": truncated,
	}).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 写运行记录失败: %v", err)
		return
	}
	db.Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).Update("last_run_id", run.ID)
	if status == models.MediaRunFailed && isFollow(sub) {
		s.notifyFailStreak(ctx, sub, run)
	}
}

// notifyFailStreak 在最近 failStreak 次运行都失败、而且正好是第 failStreak 次时发一次通知（之后接着失败不再发，成功一次以后重新算）。
func (s *Service) notifyFailStreak(ctx context.Context, sub *models.MediaSubscription, run *models.MediaSubscriptionRun) {
	if s.cfg.Notify == nil {
		return
	}
	set, err := s.Settings(ctx)
	if err != nil || len(set.NotifyChannels) == 0 {
		return
	}
	var last []models.MediaSubscriptionRun
	// 手动改动不是搜索：不打断、也不算进连续失败
	if err := s.cfg.DB.WithContext(ctx).Select("id", "status").Where("subscription_id = ? AND status NOT IN ?", sub.ID, []string{"", models.MediaRunChanged}).
		Order("id DESC").Limit(failStreak + 1).Find(&last).Error; err != nil || len(last) < failStreak {
		return
	}
	for i := range failStreak {
		if last[i].Status != models.MediaRunFailed {
			return
		}
	}
	if len(last) > failStreak && last[failStreak].Status == models.MediaRunFailed {
		return
	}
	title := subTitle(sub)
	n := Notice{
		Channels: set.NotifyChannels, Kind: NoticeFollowFailed, Subject: fmt.Sprintf("sub-%d", sub.ID),
		EventKey: fmt.Sprintf("followshow-failed-%d-%d", sub.ID, run.ID), Title: "追剧出错：" + title,
		Body: fmt.Sprintf("连续 %d 次没有搜成：%s", failStreak, run.Summary),
	}
	n.Event = notify.NewEvent(notify.EventFollowFailed, s.cfg.Now(), map[string]any{
		"subscription": title, "season": sub.Season, "fails": failStreak, "error": run.Summary,
	})
	if err := s.cfg.Notify(ctx, n); err != nil {
		s.cfg.Logger.Warnf("[订阅] 发通知失败: %v", err)
	}
}

// pruneRuns 删掉过期的运行记录：超过 90 天的，以及每个订阅最近 100 条以前的（最多每小时一次）。
func (s *Service) pruneRuns(ctx context.Context) {
	now := s.cfg.Now()
	s.pruneMu.Lock()
	if now.Sub(s.prunedAt) < pruneEvery {
		s.pruneMu.Unlock()
		return
	}
	s.prunedAt = now
	s.pruneMu.Unlock()
	db := s.cfg.DB.WithContext(ctx)
	if err := db.Where("started_at < ?", now.Add(-keepRunsFor)).Delete(&models.MediaSubscriptionRun{}).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 清理运行记录失败: %v", err)
		return
	}
	var counts []struct {
		SubscriptionID uint
		N              int
	}
	if err := db.Model(&models.MediaSubscriptionRun{}).Select("subscription_id, COUNT(*) AS n").Group("subscription_id").
		Having("COUNT(*) > ?", keepRuns).Scan(&counts).Error; err != nil {
		return
	}
	for _, c := range counts {
		var cut models.MediaSubscriptionRun
		if err := db.Select("id").Where("subscription_id = ?", c.SubscriptionID).Order("id DESC").Offset(keepRuns - 1).Limit(1).Find(&cut).Error; err != nil || cut.ID == 0 {
			continue
		}
		db.Where("subscription_id = ? AND id < ?", c.SubscriptionID, cut.ID).Delete(&models.MediaSubscriptionRun{})
	}
}

// RunView 是一次运行记录（带上解开的步骤）。
type RunView struct {
	models.MediaSubscriptionRun
	Steps []RunStep `json:"steps"`
}

// RunPage 是运行记录的一页（新的在前）。
type RunPage struct {
	Items []RunView `json:"items"`
	Total int64     `json:"total"`
}

// Runs 列出一个订阅的运行记录（新的在前）。
func (s *Service) Runs(ctx context.Context, id uint, limit, offset int) (*RunPage, error) {
	if _, err := s.subRow(ctx, id); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	db := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscriptionRun{}).Where("subscription_id = ?", id)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("读取运行记录失败: %w", err)
	}
	var rows []models.MediaSubscriptionRun
	if err := db.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取运行记录失败: %w", err)
	}
	out := &RunPage{Items: make([]RunView, 0, len(rows)), Total: total}
	for _, r := range rows {
		v := RunView{MediaSubscriptionRun: r, Steps: []RunStep{}}
		if r.Steps != "" {
			_ = json.Unmarshal([]byte(r.Steps), &v.Steps)
		}
		out.Items = append(out.Items, v)
	}
	return out, nil
}

// isFollow 报告订阅是不是追剧：剧集、按自己的定时搜索（升级以前的订阅定时为空，通知与以前一样）。
func isFollow(sub *models.MediaSubscription) bool {
	return sub.MediaType == models.MediaKindTV && sub.ScheduleKind != ""
}

// subTitle 是给人看的订阅名：名字（年份），剧集加第几季。
func subTitle(sub *models.MediaSubscription) string {
	title := sub.Title
	if sub.Year > 0 {
		title = fmt.Sprintf("%s (%d)", title, sub.Year)
	}
	if sub.MediaType == models.MediaKindTV {
		title += fmt.Sprintf(" 第 %d 季", sub.Season)
	}
	return title
}

// episodeList 是给人看的集号列表（E03、E04）。
func episodeList(eps []int) string {
	parts := make([]string, 0, len(eps))
	for _, n := range eps {
		parts = append(parts, fmt.Sprintf("E%02d", n))
	}
	return strings.Join(parts, "、")
}
