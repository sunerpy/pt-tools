package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func TestInitRuntimeSetsGlobals(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	lg, err := InitRuntime()
	require.NoError(t, err)
	assert.NotNil(t, lg)
	assert.NotNil(t, lg.Sugar())
}

func TestGetLoggerReturnsGlobal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	lg, err := InitRuntime()
	require.NoError(t, err)
	got := GetLogger()
	require.NotNil(t, got)
	require.Equal(t, lg, got)
}

func TestNewTempDBDir_MigratesAll(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, db)
	_ = db.UpsertTorrent(&models.TorrentInfo{SiteName: "cmct", TorrentID: "x"})
}

func TestDispatchV2BroadcastIfReady(t *testing.T) {
	db := newCloakDB(t)
	require.NoError(t, models.UpsertMigrationState(db.DB, V2BroadcastSchemaVersion, time.Now().UTC()))

	var called bool
	SetV2Broadcaster(V2BroadcasterFunc(func(context.Context) error {
		called = true
		return nil
	}))
	t.Cleanup(func() { SetV2Broadcaster(nil) })

	dispatchV2BroadcastIfReady(db.DB, nil)
	assert.True(t, called)

	st, ok := models.GetMigrationState(db.DB, V2BroadcastSchemaVersion)
	require.True(t, ok)
	assert.True(t, st.BroadcastSent)
}

func TestDispatchV2BroadcastIfReady_NoBroadcaster(t *testing.T) {
	SetV2Broadcaster(nil)
	db := newCloakDB(t)
	dispatchV2BroadcastIfReady(db.DB, nil)
}

func TestNewTempDBDir_BadPath(t *testing.T) {
	_, err := NewTempDBDir(filepath.Join(os.DevNull, "nope"))
	require.Error(t, err)
}
