package app

import (
	"context"
	"errors"
	"time"

	"github.com/sunerpy/pt-tools/scheduler"
)

// ErrJobNotWired 表示 StartJob/StopJob 尚未在 T32（DI 接线）阶段完成。
// 接口契约稳定，方便上层（chatops 命令、web API）先编程对接。
var ErrJobNotWired = errors.New("task service start/stop not yet wired (pending T32)")

// JobStatusDTO 是 RSS 订阅任务的运行时快照，供 chatops `/tasks`、`/jobs` 等命令使用。
type JobStatusDTO struct {
	SiteName  string    `json:"site_name"`
	RSSName   string    `json:"rss_name"`
	Running   bool      `json:"running"`
	StartedAt time.Time `json:"started_at"`
}

// TaskService 封装 scheduler.Manager 的 RSS 任务生命周期，剥离上层对底层 manager 的直接依赖。
type TaskService interface {
	ListJobs(ctx context.Context) ([]JobStatusDTO, error)
	StartJob(ctx context.Context, siteName, rssName string) error
	StopJob(ctx context.Context, siteName, rssName string) error
}

// JobLister 是 scheduler.Manager 的最小读视图，便于单元测试 mock。
// *scheduler.Manager 自动满足该接口（ListJobs 已在 T8 公开）。
type JobLister interface {
	ListJobs() []scheduler.JobStatus
}

type taskService struct {
	mgr JobLister
}

// NewTaskService 构造 TaskService，注入 *scheduler.Manager。
func NewTaskService(mgr *scheduler.Manager) TaskService {
	return &taskService{mgr: mgr}
}

// newTaskServiceWithLister 用于注入 mock JobLister，仅供测试使用。
func newTaskServiceWithLister(l JobLister) TaskService {
	return &taskService{mgr: l}
}

func (s *taskService) ListJobs(_ context.Context) ([]JobStatusDTO, error) {
	if s.mgr == nil {
		return []JobStatusDTO{}, nil
	}
	jobs := s.mgr.ListJobs()
	out := make([]JobStatusDTO, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, JobStatusDTO{
			SiteName:  j.SiteName,
			RSSName:   j.RSSName,
			Running:   j.Running,
			StartedAt: j.StartedAt,
		})
	}
	return out, nil
}

// StartJob：当前阶段返回 ErrJobNotWired，待 T32 注入 ConfigStore + RSS runner 构造器后启用。
// 接口在此提前定型，方便 T24 chatops 命令、T25 web API 与之编程对接。
func (s *taskService) StartJob(_ context.Context, siteName, rssName string) error {
	if siteName == "" || rssName == "" {
		return errors.New("siteName and rssName must not be empty")
	}
	return ErrJobNotWired
}

// StopJob：与 StartJob 同步，待 T32 接线。接口签名稳定。
func (s *taskService) StopJob(_ context.Context, siteName, rssName string) error {
	if siteName == "" || rssName == "" {
		return errors.New("siteName and rssName must not be empty")
	}
	return ErrJobNotWired
}
