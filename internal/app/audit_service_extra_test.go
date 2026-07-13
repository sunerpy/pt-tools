// MIT License
// Copyright (c) 2025 pt-tools

// Extra audit-service coverage: Query with Command / Result / Since / Until
// filters and default pagination clamping, plus Record with nil args.

package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func TestQuery_CommandAndResultFilters(t *testing.T) {
	db := setupAuditTestDB(t)
	svc := NewAuditService(db)
	now := time.Now()
	rows := []models.ActionAudit{
		{NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1", Command: "ping", ArgsJSON: "{}", Result: "ok", CreatedAt: now.Add(-3 * time.Minute)},
		{NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1", Command: "bind", ArgsJSON: "{}", Result: "error", CreatedAt: now.Add(-2 * time.Minute)},
	}
	for i := range rows {
		require.NoError(t, db.Create(&rows[i]).Error)
	}

	items, total, err := svc.Query(context.Background(), AuditQuery{Command: "bind"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, "bind", items[0].Command)

	items, total, err = svc.Query(context.Background(), AuditQuery{Result: "error"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, "error", items[0].Result)
}

func TestQuery_TimeWindowAndPaginationDefaults(t *testing.T) {
	db := setupAuditTestDB(t)
	svc := NewAuditService(db)
	now := time.Now()
	require.NoError(t, db.Create(&models.ActionAudit{
		NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1",
		Command: "ping", ArgsJSON: "{}", Result: "ok", CreatedAt: now.Add(-10 * time.Minute),
	}).Error)
	require.NoError(t, db.Create(&models.ActionAudit{
		NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1",
		Command: "ping", ArgsJSON: "{}", Result: "ok", CreatedAt: now.Add(-1 * time.Minute),
	}).Error)

	// Since window keeps only the recent row; negative page/pageSize clamp to defaults.
	items, total, err := svc.Query(context.Background(), AuditQuery{
		Since: now.Add(-5 * time.Minute), Page: -1, PageSize: -1,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)

	// Until window keeps only the old row.
	_, total, err = svc.Query(context.Background(), AuditQuery{Until: now.Add(-5 * time.Minute)})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
}

func TestRecord_NilArgs(t *testing.T) {
	db := setupAuditTestDB(t)
	svc := NewAuditService(db)
	require.NoError(t, svc.Record(context.Background(), AuditEntry{
		NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u1",
		Command: "ping", Result: "ok", LatencyMs: 5,
	}))
	var cnt int64
	require.NoError(t, db.Model(&models.ActionAudit{}).Count(&cnt).Error)
	assert.EqualValues(t, 1, cnt)
}
