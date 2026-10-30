package reseed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// ---------- 轮次 ----------

var (
	// ErrRunNotFound 表示轮次不存在。
	ErrRunNotFound = errors.New("辅种轮次不存在")
	// ErrNotContinuable 表示这一轮不能继续：只有最近一个不是重试的轮次、提前停下了（被停止、限流、超时或退出）才能继续。
	ErrNotContinuable = errors.New("这一轮不能继续：只有最近一个提前停下的轮次（重试的除外）可以继续")
)

// 尝试列表、轮次列表每页最多的条数。
const maxPageSize = 100

// RunView 是轮次列表里的一行。
type RunView struct {
	models.ReseedRun
	// SkippedDetail 是跳过的原因与个数
	SkippedDetail map[string]int `json:"skipped_detail,omitempty"`
	// Continuable 表示可以「继续」：最近一个不是重试的轮次，而且提前停下了
	Continuable bool `json:"continuable,omitempty"`
}

// RunDetail 是一轮的详情：加上步骤。
type RunDetail struct {
	RunView
	Steps []models.ReseedRunStep `json:"steps"`
}

// AttemptView 是轮次里的一次尝试。任务还在时 JobState 是任务现在的状态；JobMissing 表示任务已经不在了、结果不知道。
type AttemptView struct {
	models.ReseedAttempt
	JobMessage string `json:"job_message,omitempty"`
	JobMissing bool   `json:"job_missing,omitempty"`
}

// StartRun 写一条进行中的轮次记录。
func (s *Service) StartRun(ctx context.Context, trigger, after string, from *uint) (models.ReseedRun, error) {
	run := models.ReseedRun{Trigger: trigger, StartedAt: s.cfg.Now(), Status: models.ReseedRunRunning, After: after, ContinuedFrom: from}
	if err := s.cfg.DB.WithContext(context.WithoutCancel(ctx)).Create(&run).Error; err != nil {
		return run, fmt.Errorf("记录辅种轮次失败: %w", err)
	}
	return run, nil
}

// FinishRun 补上这一轮的结果、计数与步骤。
func (s *Service) FinishRun(ctx context.Context, id uint, res RunResult, status, errText string, paused time.Duration) error {
	steps, truncated := res.log.encoded()
	detail := ""
	if len(res.Skipped) > 0 {
		detail = encode(res.Skipped)
	}
	cols := map[string]any{
		"finished_at": s.cfg.Now(), "status": status, "summary": res.Summary(), "error": errText,
		"downloaders": res.Downloaders, "hashes": res.Hashes, "batches": res.Batches, "batches_done": res.BatchesDone,
		"candidates": res.Candidates, "created": res.Created, "failed": res.Failed, "gone": res.Gone,
		"interrupted": res.Interrupted, "skipped": res.SkippedTotal(), "skipped_detail": detail, "cursor": res.Cursor,
		"iyuu_calls": res.IYUUCalls,
		"paused_at":  nil, "paused_seconds": int(paused.Seconds()), "steps": steps, "steps_truncated": truncated,
	}
	if err := s.cfg.DB.WithContext(context.WithoutCancel(ctx)).Model(&models.ReseedRun{}).Where("id = ?", id).
		Updates(cols).Error; err != nil {
		return fmt.Errorf("记录辅种轮次的结果失败: %w", err)
	}
	return nil
}

// saveRunCursor 每查完、处理完一批写一次进度：pt-tools 中途退出时，接着查能从这里开始。
func (s *Service) saveRunCursor(ctx context.Context, id uint, res *RunResult) {
	if id == 0 {
		return
	}
	if err := s.cfg.DB.WithContext(context.WithoutCancel(ctx)).Model(&models.ReseedRun{}).Where("id = ?", id).
		Updates(map[string]any{"cursor": res.Cursor, "batches": res.Batches, "batches_done": res.BatchesDone}).Error; err != nil {
		s.cfg.Logger.Warnf("[辅种] 记录轮次 %d 的进度失败: %v", id, err)
	}
}

// SetRunPaused 记下这一轮暂停（从 since 开始）或继续（total 是一共暂停过多久）。
func (s *Service) SetRunPaused(ctx context.Context, id uint, paused bool, since time.Time, total time.Duration) error {
	cols := map[string]any{"status": models.ReseedRunRunning, "paused_at": nil, "paused_seconds": int(total.Seconds())}
	if paused {
		cols = map[string]any{"status": models.ReseedRunPaused, "paused_at": since}
	}
	if err := s.cfg.DB.WithContext(context.WithoutCancel(ctx)).Model(&models.ReseedRun{}).
		Where("id = ? AND status IN ?", id, []string{models.ReseedRunRunning, models.ReseedRunPaused}).Updates(cols).Error; err != nil {
		return fmt.Errorf("记录辅种轮次的暂停失败: %w", err)
	}
	return nil
}

// MarkInterrupted 把 before 以前开始、还写着进行中或暂停的轮次，以及还写着 running 的尝试，记成被打断
// （启动时调用：上一个进程在一轮中途退出了）。建成任务的尝试与任务在同一个事务里写，留下 running 的一定没建成任务。
func (s *Service) MarkInterrupted(ctx context.Context, before time.Time) (int64, int64, error) {
	db := s.cfg.DB.WithContext(ctx)
	now := s.cfg.Now()
	runs := db.Model(&models.ReseedRun{}).
		Where("status IN ? AND julianday(started_at) < julianday(?)", []string{models.ReseedRunRunning, models.ReseedRunPaused}, before.UTC()).
		Updates(map[string]any{"status": models.ReseedRunInterrupted, "finished_at": now, "paused_at": nil, "error": "pt-tools 退出时这一轮还没跑完"})
	if runs.Error != nil {
		return 0, 0, fmt.Errorf("补记没跑完的辅种轮次失败: %w", runs.Error)
	}
	atts := db.Model(&models.ReseedAttempt{}).
		Where("outcome = ? AND julianday(started_at) < julianday(?)", models.ReseedAttemptRunning, before.UTC()).
		Updates(map[string]any{"outcome": models.ReseedAttemptInterrupted, "finished_at": now, "message": "pt-tools 退出时这次还没试完"})
	if atts.Error != nil {
		return runs.RowsAffected, 0, fmt.Errorf("补记没试完的辅种尝试失败: %w", atts.Error)
	}
	return runs.RowsAffected, atts.RowsAffected, nil
}

// PruneRuns 清理旧的轮次（只留最近 ReseedRunKeep 条、不超过 ReseedRunKeepFor）与过了 ReseedAttemptKeepFor 的尝试。
// 时间按时刻比较（julianday），不按文本。
func (s *Service) PruneRuns(ctx context.Context) error {
	db := s.cfg.DB.WithContext(ctx)
	now := s.cfg.Now()
	if err := db.Where("julianday(started_at) < julianday(?)", now.Add(-models.ReseedRunKeepFor).UTC()).
		Delete(&models.ReseedRun{}).Error; err != nil {
		return fmt.Errorf("清理旧的辅种轮次失败: %w", err)
	}
	var cut models.ReseedRun
	err := db.Order("id DESC").Offset(models.ReseedRunKeep).Limit(1).First(&cut).Error
	switch {
	case err == nil:
		if derr := db.Where("id <= ?", cut.ID).Delete(&models.ReseedRun{}).Error; derr != nil {
			return fmt.Errorf("清理旧的辅种轮次失败: %w", derr)
		}
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return fmt.Errorf("清理旧的辅种轮次失败: %w", err)
	}
	if err := db.Where("julianday(started_at) < julianday(?)", now.Add(-models.ReseedAttemptKeepFor).UTC()).
		Delete(&models.ReseedAttempt{}).Error; err != nil {
		return fmt.Errorf("清理旧的辅种尝试失败: %w", err)
	}
	return nil
}

// latestRegular 是最近一个不是重试的轮次；没有时返回 nil。
func (s *Service) latestRegular(ctx context.Context) (*models.ReseedRun, error) {
	var rows []models.ReseedRun
	if err := s.cfg.DB.WithContext(ctx).Omit("steps").Where("`trigger` <> ?", models.ReseedTriggerRetry).
		Order("id DESC").Limit(1).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取辅种轮次失败: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// continueAfter 是接着 run 往下查的起点：它完整处理完的最后一批的最后一个 info hash；一批都没处理完时是它自己的起点。
func continueAfter(run *models.ReseedRun) string {
	if run.Cursor != "" {
		return run.Cursor
	}
	return run.After
}

// ContinuePoint 是定时轮次的起点：最近一个不是重试的轮次提前停下时接着它往下查（返回它的编号），否则从头开始。
func (s *Service) ContinuePoint(ctx context.Context) (string, *uint, error) {
	run, err := s.latestRegular(ctx)
	if err != nil || run == nil || !models.ReseedRunEndedEarly(run.Status) {
		return "", nil, err
	}
	id := run.ID
	return continueAfter(run), &id, nil
}

// CanContinue 报告能不能继续 id 这一轮，能的话返回起点。
func (s *Service) CanContinue(ctx context.Context, id uint) (string, error) {
	run, err := s.latestRegular(ctx)
	if err != nil {
		return "", err
	}
	if run == nil || run.ID != id || !models.ReseedRunEndedEarly(run.Status) {
		var n int64
		if err := s.cfg.DB.WithContext(ctx).Model(&models.ReseedRun{}).Where("id = ?", id).Count(&n).Error; err != nil {
			return "", fmt.Errorf("读取辅种轮次失败: %w", err)
		}
		if n == 0 {
			return "", ErrRunNotFound
		}
		return "", ErrNotContinuable
	}
	return continueAfter(run), nil
}

func (s *Service) runView(r models.ReseedRun, latestRegular uint) RunView {
	v := RunView{ReseedRun: r}
	if r.SkippedDetail != "" {
		_ = json.Unmarshal([]byte(r.SkippedDetail), &v.SkippedDetail)
	}
	v.Continuable = r.ID == latestRegular && r.Trigger != models.ReseedTriggerRetry && models.ReseedRunEndedEarly(r.Status)
	return v
}

func (s *Service) latestRegularID(ctx context.Context) (uint, error) {
	run, err := s.latestRegular(ctx)
	if err != nil || run == nil {
		return 0, err
	}
	return run.ID, nil
}

func pageArgs(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > maxPageSize {
		size = 20
	}
	return page, size
}

// Runs 按开始时间从新到旧列出轮次（不带步骤）。
func (s *Service) Runs(ctx context.Context, page, size int) ([]RunView, int64, error) {
	page, size = pageArgs(page, size)
	db := s.cfg.DB.WithContext(ctx)
	var total int64
	if err := db.Model(&models.ReseedRun{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("读取辅种轮次失败: %w", err)
	}
	var rows []models.ReseedRun
	if err := db.Omit("steps").Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("读取辅种轮次失败: %w", err)
	}
	latest, err := s.latestRegularID(ctx)
	if err != nil {
		return nil, 0, err
	}
	out := make([]RunView, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.runView(r, latest))
	}
	return out, total, nil
}

// GetRun 是一轮的详情。
func (s *Service) GetRun(ctx context.Context, id uint) (RunDetail, error) {
	var r models.ReseedRun
	err := s.cfg.DB.WithContext(ctx).First(&r, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return RunDetail{}, ErrRunNotFound
	}
	if err != nil {
		return RunDetail{}, fmt.Errorf("读取辅种轮次失败: %w", err)
	}
	latest, err := s.latestRegularID(ctx)
	if err != nil {
		return RunDetail{}, err
	}
	d := RunDetail{RunView: s.runView(r, latest), Steps: []models.ReseedRunStep{}}
	if r.Steps != "" {
		var steps []models.ReseedRunStep
		if json.Unmarshal([]byte(r.Steps), &steps) == nil && steps != nil {
			d.Steps = steps
		}
	}
	return d, nil
}

// RunAttempts 列出一轮里的尝试（按尝试的先后），带上对应任务现在的状态。
func (s *Service) RunAttempts(ctx context.Context, id uint, page, size int) ([]AttemptView, int64, error) {
	page, size = pageArgs(page, size)
	db := s.cfg.DB.WithContext(ctx)
	var n int64
	if err := db.Model(&models.ReseedRun{}).Where("id = ?", id).Count(&n).Error; err != nil {
		return nil, 0, fmt.Errorf("读取辅种轮次失败: %w", err)
	}
	if n == 0 {
		return nil, 0, ErrRunNotFound
	}
	q := db.Model(&models.ReseedAttempt{}).Where("run_id = ?", id)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("读取辅种尝试失败: %w", err)
	}
	var rows []models.ReseedAttempt
	if err := db.Where("run_id = ?", id).Order("id").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("读取辅种尝试失败: %w", err)
	}
	views, err := s.attemptViews(ctx, rows)
	return views, total, err
}

// attemptViews 给尝试补上对应任务现在的状态（任务清除时结果已经抄进尝试行的，用尝试行里的）。
func (s *Service) attemptViews(ctx context.Context, rows []models.ReseedAttempt) ([]AttemptView, error) {
	ids := make([]uint, 0)
	for _, a := range rows {
		if a.JobID != nil && a.JobState == "" {
			ids = append(ids, *a.JobID)
		}
	}
	jobs := map[uint]models.TorrentTransferJob{}
	if len(ids) > 0 {
		var js []models.TorrentTransferJob
		if err := s.cfg.DB.WithContext(ctx).Omit("torrent_data").Where("id IN ?", ids).Find(&js).Error; err != nil {
			return nil, fmt.Errorf("读取辅种任务失败: %w", err)
		}
		for _, j := range js {
			jobs[j.ID] = j
		}
	}
	out := make([]AttemptView, 0, len(rows))
	for _, a := range rows {
		v := AttemptView{ReseedAttempt: a}
		if a.JobID != nil && a.JobState == "" {
			if j, ok := jobs[*a.JobID]; ok {
				v.JobState, v.JobMessage, v.JobFinishedAt = j.State, j.Message, j.FinishedAt
			} else {
				v.JobMissing = true
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// RunJobsWaiting 是 id 这一轮建的、还在等待加入下载器的任务（停止时可以一起取消）。
func (s *Service) RunJobsWaiting(ctx context.Context, id uint) ([]uint, error) {
	var ids []uint
	if err := s.cfg.DB.WithContext(ctx).Model(&models.ReseedAttempt{}).
		Joins("JOIN torrent_transfer_jobs j ON j.id = reseed_attempts.job_id").
		Where("reseed_attempts.run_id = ? AND j.state IN ?", id, []string{models.TransferPending, models.TransferExported}).
		Pluck("reseed_attempts.job_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("读取这一轮的辅种任务失败: %w", err)
	}
	return ids, nil
}

// CancelJobs 取消还没加入下载器的任务，返回取消了几个；已经加入的跳过。
func (s *Service) CancelJobs(ctx context.Context, ids []uint) int {
	n := 0
	for _, id := range ids {
		if err := s.cfg.Transfer.Cancel(ctx, id); err == nil {
			n++
		}
	}
	return n
}
