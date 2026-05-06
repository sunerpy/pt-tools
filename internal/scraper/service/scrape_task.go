package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
	"github.com/sunerpy/pt-tools/internal/scraper/store"
)

const defaultScrapeTaskMaxRetries = 3

type scrapeTaskState struct {
	mu         sync.Mutex
	state      core.TaskState
	retryCount int
	maxRetries int
	lastErr    error
}

func newScrapeTaskState(maxRetries int) scrapeTaskState {
	if maxRetries <= 0 {
		maxRetries = defaultScrapeTaskMaxRetries
	}
	return scrapeTaskState{state: core.TaskPending, maxRetries: maxRetries}
}

func (s *scrapeTaskState) State() core.TaskState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *scrapeTaskState) RetryCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.retryCount
}

func (s *scrapeTaskState) MaxRetries() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxRetries
}

func (s *scrapeTaskState) SetState(state core.TaskState) {
	s.mu.Lock()
	s.state = state
	s.mu.Unlock()
}

func (s *scrapeTaskState) IncrementRetry() {
	s.mu.Lock()
	s.retryCount++
	s.mu.Unlock()
}

func (s *scrapeTaskState) LastError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

func (s *scrapeTaskState) SetLastError(err error) {
	s.mu.Lock()
	s.lastErr = err
	s.mu.Unlock()
}

// movieScrapeTask 适配 core.Task。
type movieScrapeTask struct {
	id  string
	req ScrapeMovieRequest
	svc *ScrapeService

	state scrapeTaskState
}

// tvShowScrapeTask 适配 core.Task。
type tvShowScrapeTask struct {
	id  string
	req ScrapeTvShowRequest
	svc *ScrapeService

	state scrapeTaskState
}

// episodeScrapeTask 适配 core.Task。
type episodeScrapeTask struct {
	id  string
	req ScrapeEpisodeRequest
	svc *ScrapeService

	state scrapeTaskState
}

// NewMovieScrapeTask 从持久化记录创建电影任务。
func NewMovieScrapeTask(record store.ScrapeTask, svc *ScrapeService, req ScrapeMovieRequest) core.Task {
	req.taskID = record.ID
	if req.LibraryID == nil {
		req.LibraryID = record.LibraryID
	}
	return &movieScrapeTask{
		id:    taskID(record.ID, record.TaskType, record.MediaPath),
		req:   req,
		svc:   svc,
		state: newScrapeTaskState(record.MaxRetries),
	}
}

// NewTvShowScrapeTask 从持久化记录创建剧集任务。
func NewTvShowScrapeTask(record store.ScrapeTask, svc *ScrapeService, req ScrapeTvShowRequest) core.Task {
	req.taskID = record.ID
	if req.LibraryID == nil {
		req.LibraryID = record.LibraryID
	}
	return &tvShowScrapeTask{
		id:    taskID(record.ID, record.TaskType, record.MediaPath),
		req:   req,
		svc:   svc,
		state: newScrapeTaskState(record.MaxRetries),
	}
}

// NewEpisodeScrapeTask 从持久化记录创建单集任务。
func NewEpisodeScrapeTask(record store.ScrapeTask, svc *ScrapeService, req ScrapeEpisodeRequest) core.Task {
	req.taskID = record.ID
	if req.LibraryID == nil {
		req.LibraryID = record.LibraryID
	}
	return &episodeScrapeTask{
		id:    taskID(record.ID, record.TaskType, record.MediaPath),
		req:   req,
		svc:   svc,
		state: newScrapeTaskState(record.MaxRetries),
	}
}

func (t *movieScrapeTask) ID() string                  { return t.id }
func (t *movieScrapeTask) Type() string                { return "movie" }
func (t *movieScrapeTask) State() core.TaskState       { return t.state.State() }
func (t *movieScrapeTask) RetryCount() int             { return t.state.RetryCount() }
func (t *movieScrapeTask) MaxRetries() int             { return t.state.MaxRetries() }
func (t *movieScrapeTask) SetState(s core.TaskState)   { t.state.SetState(s) }
func (t *movieScrapeTask) IncrementRetry()             { t.state.IncrementRetry() }
func (t *movieScrapeTask) LastError() error            { return t.state.LastError() }
func (t *movieScrapeTask) SetLastError(err error)      { t.state.SetLastError(err) }
func (t *tvShowScrapeTask) ID() string                 { return t.id }
func (t *tvShowScrapeTask) Type() string               { return "tv" }
func (t *tvShowScrapeTask) State() core.TaskState      { return t.state.State() }
func (t *tvShowScrapeTask) RetryCount() int            { return t.state.RetryCount() }
func (t *tvShowScrapeTask) MaxRetries() int            { return t.state.MaxRetries() }
func (t *tvShowScrapeTask) SetState(s core.TaskState)  { t.state.SetState(s) }
func (t *tvShowScrapeTask) IncrementRetry()            { t.state.IncrementRetry() }
func (t *tvShowScrapeTask) LastError() error           { return t.state.LastError() }
func (t *tvShowScrapeTask) SetLastError(err error)     { t.state.SetLastError(err) }
func (t *episodeScrapeTask) ID() string                { return t.id }
func (t *episodeScrapeTask) Type() string              { return "episode" }
func (t *episodeScrapeTask) State() core.TaskState     { return t.state.State() }
func (t *episodeScrapeTask) RetryCount() int           { return t.state.RetryCount() }
func (t *episodeScrapeTask) MaxRetries() int           { return t.state.MaxRetries() }
func (t *episodeScrapeTask) SetState(s core.TaskState) { t.state.SetState(s) }
func (t *episodeScrapeTask) IncrementRetry()           { t.state.IncrementRetry() }
func (t *episodeScrapeTask) LastError() error          { return t.state.LastError() }
func (t *episodeScrapeTask) SetLastError(err error)    { t.state.SetLastError(err) }

func (t *movieScrapeTask) Run(ctx context.Context) error {
	if t.svc == nil {
		return fmt.Errorf("movie task %s: nil service", t.id)
	}
	_, err := t.svc.ScrapeMovie(ctx, t.req)
	return err
}

func (t *tvShowScrapeTask) Run(ctx context.Context) error {
	if t.svc == nil {
		return fmt.Errorf("tv task %s: nil service", t.id)
	}
	_, err := t.svc.ScrapeTvShow(ctx, t.req)
	return err
}

func (t *episodeScrapeTask) Run(ctx context.Context) error {
	if t.svc == nil {
		return fmt.Errorf("episode task %s: nil service", t.id)
	}
	_, err := t.svc.ScrapeEpisode(ctx, t.req)
	return err
}

func taskID(recordID uint, taskType, mediaPath string) string {
	if recordID > 0 {
		return fmt.Sprintf("scrape-%s-%d", taskType, recordID)
	}
	return fmt.Sprintf("scrape-%s-%s", taskType, mediaPath)
}
