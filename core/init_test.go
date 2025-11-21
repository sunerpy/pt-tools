package core

import (
	"testing"

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
