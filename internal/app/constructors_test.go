// MIT License
// Copyright (c) 2025 pt-tools

package app

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestNewConstructors(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	global.GlobalDB = nil

	assert.NotNil(t, NewSiteService(core.NewConfigStore(nil), nil))
	assert.NotNil(t, NewTaskService(scheduler.NewManager()))
	assert.NotNil(t, NewTorrentService(downloader.NewDownloaderManager()))
	assert.NotNil(t, NewAuditService(db))
	assert.NotNil(t, NewRSSCallbackActions(db, nil))
}

func TestNewAuditServiceWithCap_Defaults(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// Non-positive rowCap/retention fall back to defaults.
	svc := NewAuditServiceWithCap(db, 0, 0).(*auditService)
	assert.Greater(t, svc.rowCap, int64(0))
	assert.Greater(t, svc.retention, time.Duration(0))
}

func TestTaskService_ListJobs_NilManager(t *testing.T) {
	svc := newTaskServiceWithLister(nil)
	jobs, err := svc.ListJobs(context.Background())
	require.NoError(t, err)
	assert.Empty(t, jobs)
}

func TestSiteService_ListSites_NilStore(t *testing.T) {
	svc := newSiteServiceWithDeps(nil, nil)
	sites, err := svc.ListSites(context.Background())
	require.NoError(t, err)
	assert.Empty(t, sites)
}

func TestRSSRetryWorker_RunStopsOnCancel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.RSSNotificationLog{}))

	w := NewRSSRetryWorker(db, &fakePushSvc{})
	w.interval = time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	time.Sleep(5 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}
