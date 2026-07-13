// MIT License
// Copyright (c) 2025 pt-tools

package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/scheduler"
)

type stubJobLister struct {
	mu   sync.Mutex
	jobs []scheduler.JobStatus
}

func (s *stubJobLister) ListJobs() []scheduler.JobStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]scheduler.JobStatus, len(s.jobs))
	copy(out, s.jobs)
	return out
}

func TestTaskService_ListJobs_Empty(t *testing.T) {
	stub := &stubJobLister{}
	svc := newTaskServiceWithLister(stub)

	got, err := svc.ListJobs(context.Background())

	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestTaskService_ListJobs_AfterStart(t *testing.T) {
	now := time.Now()
	stub := &stubJobLister{
		jobs: []scheduler.JobStatus{
			{SiteName: "MTeam", RSSName: "free", Running: true, StartedAt: now},
			{SiteName: "HDSky", RSSName: "rss-1", Running: true, StartedAt: now.Add(-time.Minute)},
		},
	}
	svc := newTaskServiceWithLister(stub)

	got, err := svc.ListJobs(context.Background())

	require.NoError(t, err)
	require.Len(t, got, 2)
	bySite := map[string]JobStatusDTO{}
	for _, j := range got {
		bySite[j.SiteName] = j
	}
	assert.Equal(t, "free", bySite["MTeam"].RSSName)
	assert.True(t, bySite["MTeam"].Running)
	assert.WithinDuration(t, now, bySite["MTeam"].StartedAt, time.Second)
	assert.Equal(t, "rss-1", bySite["HDSky"].RSSName)
}

func TestTaskService_StartStop_NotYetWired(t *testing.T) {
	svc := newTaskServiceWithLister(&stubJobLister{})

	startErr := svc.StartJob(context.Background(), "MTeam", "free")
	stopErr := svc.StopJob(context.Background(), "MTeam", "free")

	assert.True(t, errors.Is(startErr, ErrJobNotWired))
	assert.True(t, errors.Is(stopErr, ErrJobNotWired))
}

func TestTaskService_StartStop_RejectEmptyArgs(t *testing.T) {
	svc := newTaskServiceWithLister(&stubJobLister{})

	require.Error(t, svc.StartJob(context.Background(), "", "free"))
	require.Error(t, svc.StopJob(context.Background(), "MTeam", ""))
}
