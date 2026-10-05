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
			// 每个用例从空值开始：last_visit_at 只前进，复用同一行会让后面的较早时间被忽略。
			require.NoError(t, db.Model(&SiteLoginState{}).Where("site_name = ?", siteName).Update("last_visit_at", nil).Error)
			err := repo.ClampLastVisit(siteName, tt.inputTS, now)
			require.NoError(t, err)

			state, err := repo.GetLoginState(siteName)
			require.NoError(t, err)
			require.NotNil(t, state)
			require.NotNil(t, state.LastVisitAt, "LastVisitAt should be set")
			assert.True(t, tt.expectedPersis.Equal(*state.LastVisitAt), "failure: %s (got %v)", tt.desc, *state.LastVisitAt)
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

func TestSiteLoginState_IncrResetClamp(t *testing.T) {
	db := newMemDB(t, &SiteLoginState{})
	repo := NewSiteLoginStateRepository(db)

	require.NoError(t, repo.UpsertLoginState("hdsky", map[string]any{
		"BanThresholdDays":       45,
		"RemindBeforeDays":       7,
		"ReminderCron":           "0 8 * * *",
		"NotificationChannelIDs": "[1,2,3]",
	}))

	require.NoError(t, repo.IncrProbeFailures("hdsky"))
	require.NoError(t, repo.IncrProbeFailures("hdsky"))
	st, err := repo.GetLoginState("hdsky")
	require.NoError(t, err)
	assert.Equal(t, 2, st.ConsecutiveProbeFailures)
	assert.Equal(t, 45, st.BanThresholdDays)
	assert.Equal(t, "0 8 * * *", st.ReminderCron)
	assert.Equal(t, "[1,2,3]", st.NotificationChannelIDs)

	require.NoError(t, repo.ResetProbeFailures("hdsky"))
	st, err = repo.GetLoginState("hdsky")
	require.NoError(t, err)
	assert.Equal(t, 0, st.ConsecutiveProbeFailures)

	now := time.Now()
	future := now.Add(24 * time.Hour)
	require.NoError(t, repo.ClampLastVisit("hdsky", future, now))
	st, err = repo.GetLoginState("hdsky")
	require.NoError(t, err)
	require.NotNil(t, st.LastVisitAt)
	assert.WithinDuration(t, now, *st.LastVisitAt, 2*time.Second)

	past := now.Add(-time.Hour)
	require.NoError(t, repo.ClampLastVisit("hdsky", past, now))
	st, err = repo.GetLoginState("hdsky")
	require.NoError(t, err)
	assert.WithinDuration(t, now, *st.LastVisitAt, 2*time.Second, "an older visit never moves last_visit_at backwards")
}

func TestSiteLoginState_ClampLastVisitOnlyMovesForward(t *testing.T) {
	db := newMemDB(t, &SiteLoginState{})
	repo := NewSiteLoginStateRepository(db)
	require.NoError(t, repo.EnsureLoginStateRow(DefaultSiteLoginState("hdsky")))

	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	newer := now.Add(-time.Hour)
	older := now.Add(-48 * time.Hour)
	require.NoError(t, repo.ClampLastVisit("hdsky", newer, now))
	require.NoError(t, repo.ClampLastVisit("hdsky", older, now))
	st, err := repo.GetLoginState("hdsky")
	require.NoError(t, err)
	require.NotNil(t, st.LastVisitAt)
	assert.True(t, st.LastVisitAt.Equal(newer), "got %v", st.LastVisitAt)

	// 历史数据可能以本地时区写入：比较必须按时刻，不能按字符串。
	loc := time.FixedZone("CST", 8*3600)
	require.NoError(t, db.Model(&SiteLoginState{}).Where("site_name = ?", "hdsky").
		Update("last_visit_at", now.Add(-30*time.Minute).In(loc)).Error)
	require.NoError(t, repo.ClampLastVisit("hdsky", now.Add(-40*time.Minute), now))
	st, err = repo.GetLoginState("hdsky")
	require.NoError(t, err)
	assert.True(t, st.LastVisitAt.Equal(now.Add(-30*time.Minute)), "an earlier instant written in UTC must not win over a later local-time value")

	// 缺行时不建行、不报错。
	require.NoError(t, repo.ClampLastVisit("ghost", now, now))
	_, err = repo.GetLoginState("ghost")
	assert.Error(t, err)
}

func TestSiteLoginState_EnsureRowIsIdempotent(t *testing.T) {
	db := newMemDB(t, &SiteLoginState{})
	repo := NewSiteLoginStateRepository(db)
	defaults := DefaultSiteLoginState("hdsky")
	defaults.BanThresholdDays = 45
	require.NoError(t, repo.EnsureLoginStateRow(defaults))
	require.NoError(t, repo.UpsertLoginState("hdsky", map[string]any{"ProbeMode": "manual"}))
	require.NoError(t, repo.EnsureLoginStateRow(DefaultSiteLoginState("hdsky")), "an existing row is left untouched")

	st, err := repo.GetLoginState("hdsky")
	require.NoError(t, err)
	assert.Equal(t, 45, st.BanThresholdDays)
	assert.Equal(t, "manual", st.ProbeMode)
	var count int64
	require.NoError(t, db.Model(&SiteLoginState{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	assert.Error(t, repo.EnsureLoginStateRow(SiteLoginState{}))
}

func TestSiteLoginState_UpsertOnlyWritesGivenColumns(t *testing.T) {
	db := newMemDB(t, &SiteLoginState{})
	repo := NewSiteLoginStateRepository(db)
	require.NoError(t, repo.EnsureLoginStateRow(DefaultSiteLoginState("hdsky")))

	// 模拟探测在配置接口读取之后写入的列。
	access := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	require.NoError(t, repo.UpdateColumns("hdsky", map[string]any{
		"last_access_at": access, "last_probe_status": "OK", "consecutive_probe_failures": 2,
	}))
	require.NoError(t, repo.UpsertLoginState("hdsky", map[string]any{
		"ProbeMode": "disabled", "BanThresholdDays": 60, "Unknown": "ignored", "RemindBeforeDays": 3.5,
	}))

	st, err := repo.GetLoginState("hdsky")
	require.NoError(t, err)
	assert.Equal(t, "disabled", st.ProbeMode)
	assert.Equal(t, 60, st.BanThresholdDays)
	assert.Equal(t, 10, st.RemindBeforeDays, "values of unsupported types are ignored")
	require.NotNil(t, st.LastAccessAt)
	assert.True(t, st.LastAccessAt.Equal(access), "probe-owned columns are not rewritten by a config update")
	assert.Equal(t, "OK", st.LastProbeStatus)
	assert.Equal(t, 2, st.ConsecutiveProbeFailures)

	require.NoError(t, repo.UpdateColumns("hdsky", nil))
	assert.Error(t, repo.UpdateColumns("", map[string]any{"probe_mode": "auto"}))
}

func TestSiteLoginState_ListAndUpdateProbe(t *testing.T) {
	db := newMemDB(t, &SiteLoginState{})
	repo := NewSiteLoginStateRepository(db)

	require.NoError(t, repo.UpsertLoginState("hdsky", map[string]any{
		"LastProbeStatus":    "OK",
		"ProbeJitterSeconds": 30,
		"ProbeMode":          "api",
	}))
	require.NoError(t, repo.UpsertLoginState("mteam", map[string]any{"LastProbeStatus": "FAIL"}))

	all, err := repo.ListLoginStates(false)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	okOnly, err := repo.ListLoginStates(true)
	require.NoError(t, err)
	assert.Len(t, okOnly, 1)
	assert.Equal(t, "hdsky", okOnly[0].SiteName)

	// UpdateProbeResult with login/access times + error
	login := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	access := login.Add(time.Hour)
	require.NoError(t, repo.UpdateProbeResult("hdsky", "OK", &login, &access, assert.AnError))
	st, err := repo.GetLoginState("hdsky")
	require.NoError(t, err)
	require.NotNil(t, st.LastLoginAt)
	require.NotNil(t, st.LastAccessAt)
	assert.Equal(t, assert.AnError.Error(), st.LastProbeError)

	// nil-time, nil-error path clears error message
	require.NoError(t, repo.UpdateProbeResult("hdsky", "OK", nil, nil, nil))
	st, err = repo.GetLoginState("hdsky")
	require.NoError(t, err)
	assert.Empty(t, st.LastProbeError)
}

func TestSiteLoginState_EmptyNameErrors(t *testing.T) {
	db := newMemDB(t, &SiteLoginState{})
	repo := NewSiteLoginStateRepository(db)

	assert.Error(t, repo.UpsertLoginState("", nil))
	_, err := repo.GetLoginState("")
	assert.Error(t, err)
	assert.Error(t, repo.UpdateProbeResult("", "OK", nil, nil, nil))
	assert.Error(t, repo.ClampLastVisit("", time.Now(), time.Now()))
	assert.Error(t, repo.IncrProbeFailures(""))
	assert.Error(t, repo.ResetProbeFailures(""))

	// GetLoginState on missing site → error
	_, err = repo.GetLoginState("does-not-exist")
	assert.Error(t, err)
}
