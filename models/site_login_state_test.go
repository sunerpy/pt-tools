package models

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSiteLoginStateTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = db.AutoMigrate(&SiteLoginState{})
	require.NoError(t, err)
	return db
}

func TestSiteLoginStateUpsertGet(t *testing.T) {
	db := setupSiteLoginStateTestDB(t)
	repo := NewSiteLoginStateRepository(db)

	tests := []struct {
		name            string
		siteName        string
		fields          map[string]any
		expectedBanDays int
	}{
		{
			name:            "upsert with custom ban threshold",
			siteName:        "HDSKY",
			fields:          map[string]any{"BanThresholdDays": 60},
			expectedBanDays: 60,
		},
		{
			name:            "upsert defaults to 30 days",
			siteName:        "MTEAM",
			fields:          map[string]any{},
			expectedBanDays: 30,
		},
		{
			name:            "upsert with multiple fields",
			siteName:        "PTPP",
			fields:          map[string]any{"BanThresholdDays": 45, "RemindBeforeDays": 7},
			expectedBanDays: 45,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.UpsertLoginState(tt.siteName, tt.fields)
			require.NoError(t, err)

			state, err := repo.GetLoginState(tt.siteName)
			require.NoError(t, err)
			require.NotNil(t, state)
			assert.Equal(t, tt.siteName, state.SiteName)
			assert.Equal(t, tt.expectedBanDays, state.BanThresholdDays)
		})
	}
}

func TestSiteLoginStateClampLastVisit(t *testing.T) {
	db := setupSiteLoginStateTestDB(t)
	repo := NewSiteLoginStateRepository(db)

	siteName := "HDSKY"
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	futureTS := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	pastTS := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	err := repo.UpsertLoginState(siteName, map[string]any{})
	require.NoError(t, err)

	tests := []struct {
		name           string
		inputTS        time.Time
		expectedPersis time.Time
		desc           string
	}{
		{
			name:           "future timestamp is clamped to now",
			inputTS:        futureTS,
			expectedPersis: now,
			desc:           "must not persist 2030-01-01, must clamp to 2026-05-18",
		},
		{
			name:           "past timestamp passes through",
			inputTS:        pastTS,
			expectedPersis: pastTS,
			desc:           "past timestamps are allowed",
		},
		{
			name:           "now timestamp passes through",
			inputTS:        now,
			expectedPersis: now,
			desc:           "now timestamp equals expected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.ClampLastVisit(siteName, tt.inputTS, now)
			require.NoError(t, err)

			state, err := repo.GetLoginState(siteName)
			require.NoError(t, err)
			require.NotNil(t, state)
			require.NotNil(t, state.LastVisitAt, "LastVisitAt should be set")
			assert.Equal(t, tt.expectedPersis, *state.LastVisitAt, "failure: %s", tt.desc)
		})
	}
}

func TestSiteLoginStateProbeFailureCounter(t *testing.T) {
	db := setupSiteLoginStateTestDB(t)
	repo := NewSiteLoginStateRepository(db)

	siteName := "HDSKY"
	err := repo.UpsertLoginState(siteName, map[string]any{})
	require.NoError(t, err)

	for i := 1; i <= 3; i++ {
		err = repo.IncrProbeFailures(siteName)
		require.NoError(t, err)

		state, getErr := repo.GetLoginState(siteName)
		require.NoError(t, getErr)
		assert.Equal(t, i, state.ConsecutiveProbeFailures)
	}

	err = repo.ResetProbeFailures(siteName)
	require.NoError(t, err)

	state, err := repo.GetLoginState(siteName)
	require.NoError(t, err)
	assert.Equal(t, 0, state.ConsecutiveProbeFailures)
}

func TestSiteLoginStateUniqueIndex(t *testing.T) {
	db := setupSiteLoginStateTestDB(t)
	repo := NewSiteLoginStateRepository(db)

	siteName := "HDSKY"

	err := repo.UpsertLoginState(siteName, map[string]any{"BanThresholdDays": 30})
	require.NoError(t, err)

	state, err := repo.GetLoginState(siteName)
	require.NoError(t, err)
	assert.Equal(t, 30, state.BanThresholdDays)

	err = repo.UpsertLoginState(siteName, map[string]any{"BanThresholdDays": 60})
	require.NoError(t, err)

	state, err = repo.GetLoginState(siteName)
	require.NoError(t, err)
	assert.Equal(t, 60, state.BanThresholdDays)

	var count int64
	err = db.Model(&SiteLoginState{}).Count(&count).Error
	require.NoError(t, err)
	assert.Equal(t, int64(1), count, "should only have 1 row for same SiteName")
}
