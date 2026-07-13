package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrationState_Lifecycle(t *testing.T) {
	db := newMemDB(t, &MigrationState{})

	// nil db is safe
	_, ok := GetLatestMigrationCompletedAt(nil)
	assert.False(t, ok)
	require.NoError(t, UpsertMigrationState(nil, 1, time.Now()))
	require.NoError(t, MarkBroadcastSent(nil, 1))
	_, ok = GetMigrationState(nil, 1)
	assert.False(t, ok)

	// no rows yet
	_, ok = GetLatestMigrationCompletedAt(db)
	assert.False(t, ok)
	_, ok = GetMigrationState(db, 10)
	assert.False(t, ok)

	// insert
	ts := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, UpsertMigrationState(db, 10, ts))
	got, ok := GetLatestMigrationCompletedAt(db)
	require.True(t, ok)
	assert.Equal(t, ts, got.UTC())

	state, ok := GetMigrationState(db, 10)
	require.True(t, ok)
	assert.Equal(t, 10, state.SchemaVersion)
	assert.False(t, state.BroadcastSent)

	// update existing row (idempotent upsert)
	ts2 := ts.Add(time.Hour)
	require.NoError(t, UpsertMigrationState(db, 10, ts2))
	got, ok = GetLatestMigrationCompletedAt(db)
	require.True(t, ok)
	assert.Equal(t, ts2, got.UTC())

	// mark broadcast sent
	require.NoError(t, MarkBroadcastSent(db, 10))
	state, ok = GetMigrationState(db, 10)
	require.True(t, ok)
	assert.True(t, state.BroadcastSent)
}
