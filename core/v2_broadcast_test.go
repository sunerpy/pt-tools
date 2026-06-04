package core

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

type countingBroadcaster struct {
	calls atomic.Int32
	err   error
}

func (c *countingBroadcaster) BroadcastV2Upgrade(ctx context.Context) error {
	c.calls.Add(1)
	return c.err
}

func newBroadcastTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.MigrationState{}))
	return db
}

func TestV2BroadcastSentOnce(t *testing.T) {
	db := newBroadcastTestDB(t)
	now := time.Now().UTC()
	require.NoError(t, models.UpsertMigrationState(db, V2BroadcastSchemaVersion, now))

	bc := &countingBroadcaster{}
	logger := zap.NewNop().Sugar()

	first := MaybeSendV2Broadcast(context.Background(), db, bc, logger, now)
	assert.True(t, first.Sent)
	assert.Equal(t, "sent", first.Reason)
	assert.Equal(t, int32(1), bc.calls.Load())

	state, ok := models.GetMigrationState(db, V2BroadcastSchemaVersion)
	require.True(t, ok)
	assert.True(t, state.BroadcastSent)

	second := MaybeSendV2Broadcast(context.Background(), db, bc, logger, now.Add(1*time.Second))
	assert.False(t, second.Sent)
	assert.Equal(t, "already_sent", second.Reason)
	assert.Equal(t, int32(1), bc.calls.Load(), "broadcaster must NOT be called again")

	third := MaybeSendV2Broadcast(context.Background(), db, bc, logger, now.Add(48*time.Hour))
	assert.False(t, third.Sent)
	assert.Equal(t, "already_sent", third.Reason)
	assert.Equal(t, int32(1), bc.calls.Load())
}

func TestV2BroadcastSkippedWhenNoMigrationState(t *testing.T) {
	db := newBroadcastTestDB(t)
	bc := &countingBroadcaster{}

	result := MaybeSendV2Broadcast(context.Background(), db, bc, zap.NewNop().Sugar(), time.Now().UTC())
	assert.False(t, result.Sent)
	assert.Equal(t, "no_migration_state", result.Reason)
	assert.Equal(t, int32(0), bc.calls.Load())
}

func TestV2BroadcastSkippedWhenStale(t *testing.T) {
	db := newBroadcastTestDB(t)
	old := time.Now().UTC().Add(-1 * time.Hour)
	require.NoError(t, models.UpsertMigrationState(db, V2BroadcastSchemaVersion, old))

	bc := &countingBroadcaster{}
	now := time.Now().UTC()

	result := MaybeSendV2Broadcast(context.Background(), db, bc, zap.NewNop().Sugar(), now)
	assert.False(t, result.Sent)
	assert.Equal(t, "stale", result.Reason)
	assert.Equal(t, int32(0), bc.calls.Load(), "stale rows must NOT trigger broadcast")

	state, ok := models.GetMigrationState(db, V2BroadcastSchemaVersion)
	require.True(t, ok)
	assert.True(t, state.BroadcastSent, "stale rows should be marked sent so we never retry")

	again := MaybeSendV2Broadcast(context.Background(), db, bc, zap.NewNop().Sugar(), now)
	assert.Equal(t, "already_sent", again.Reason)
	assert.Equal(t, int32(0), bc.calls.Load())
}

func TestV2BroadcastBroadcasterErrorDoesNotMarkSent(t *testing.T) {
	db := newBroadcastTestDB(t)
	now := time.Now().UTC()
	require.NoError(t, models.UpsertMigrationState(db, V2BroadcastSchemaVersion, now))

	bc := &countingBroadcaster{err: errors.New("transport down")}
	logger := zap.NewNop().Sugar()

	first := MaybeSendV2Broadcast(context.Background(), db, bc, logger, now)
	assert.False(t, first.Sent)
	assert.Equal(t, "broadcaster_error", first.Reason)
	assert.NotNil(t, first.BcastErr)
	assert.Equal(t, int32(1), bc.calls.Load())

	state, ok := models.GetMigrationState(db, V2BroadcastSchemaVersion)
	require.True(t, ok)
	assert.False(t, state.BroadcastSent, "broadcast failure must NOT mark sentinel")

	bc.err = nil
	second := MaybeSendV2Broadcast(context.Background(), db, bc, logger, now.Add(1*time.Second))
	assert.True(t, second.Sent)
	assert.Equal(t, "sent", second.Reason)
	assert.Equal(t, int32(2), bc.calls.Load(), "retry succeeds after error clears")
}

func TestV2BroadcastNilBroadcaster(t *testing.T) {
	db := newBroadcastTestDB(t)
	require.NoError(t, models.UpsertMigrationState(db, V2BroadcastSchemaVersion, time.Now().UTC()))

	result := MaybeSendV2Broadcast(context.Background(), db, nil, zap.NewNop().Sugar(), time.Now().UTC())
	assert.False(t, result.Sent)
	assert.Equal(t, "broadcaster_nil", result.Reason)

	state, ok := models.GetMigrationState(db, V2BroadcastSchemaVersion)
	require.True(t, ok)
	assert.False(t, state.BroadcastSent)
}

func TestV2BroadcastNilDB(t *testing.T) {
	bc := &countingBroadcaster{}
	result := MaybeSendV2Broadcast(context.Background(), nil, bc, zap.NewNop().Sugar(), time.Now().UTC())
	assert.False(t, result.Sent)
	assert.Equal(t, "db_nil", result.Reason)
	assert.Equal(t, int32(0), bc.calls.Load())
}

func TestV2BroadcasterFuncAdapter(t *testing.T) {
	called := atomic.Int32{}
	bc := V2BroadcasterFunc(func(ctx context.Context) error {
		called.Add(1)
		return nil
	})

	db := newBroadcastTestDB(t)
	require.NoError(t, models.UpsertMigrationState(db, V2BroadcastSchemaVersion, time.Now().UTC()))

	result := MaybeSendV2Broadcast(context.Background(), db, bc, zap.NewNop().Sugar(), time.Now().UTC())
	assert.True(t, result.Sent)
	assert.Equal(t, int32(1), called.Load())
}

func TestSetV2BroadcasterRoundTrip(t *testing.T) {
	t.Cleanup(func() { SetV2Broadcaster(nil) })

	bc := &countingBroadcaster{}
	SetV2Broadcaster(bc)
	got := currentV2Broadcaster()
	require.NotNil(t, got)
	require.NoError(t, got.BroadcastV2Upgrade(context.Background()))
	assert.Equal(t, int32(1), bc.calls.Load())

	SetV2Broadcaster(nil)
	assert.Nil(t, currentV2Broadcaster())
}
