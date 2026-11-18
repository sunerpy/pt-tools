package models

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newAttendanceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&SiteAttendanceLog{}))
	return db
}

// M1c：每站每天一行；同一天重复建行不覆盖已有的计划时间与结果。
func TestSiteAttendanceRepository_EnsureIsIdempotent(t *testing.T) {
	repo := NewSiteAttendanceRepository(newAttendanceTestDB(t))
	first := time.Date(2026, 10, 5, 0, 30, 0, 0, time.UTC)
	require.NoError(t, repo.EnsureDay(SiteAttendanceLog{SiteName: "hdtime", Day: "2026-10-05", ScheduledAt: first}))
	require.NoError(t, repo.EnsureDay(SiteAttendanceLog{SiteName: "hdtime", Day: "2026-10-05", ScheduledAt: first.Add(time.Hour)}))

	row, err := repo.GetDay("hdtime", "2026-10-05")
	require.NoError(t, err)
	assert.True(t, row.ScheduledAt.Equal(first), "the first schedule of the day is kept")
	assert.Equal(t, AttendancePending, row.Status)
	assert.False(t, row.Done())

	require.NoError(t, repo.EnsureDay(SiteAttendanceLog{SiteName: "hdtime", Day: "2026-10-06", ScheduledAt: first.Add(24 * time.Hour)}))
	rows, err := repo.ListDay("2026-10-05")
	require.NoError(t, err)
	assert.Len(t, rows, 1, "another day is another row")
}

func TestSiteAttendanceRepository_UpdateDay(t *testing.T) {
	repo := NewSiteAttendanceRepository(newAttendanceTestDB(t))
	require.NoError(t, repo.EnsureDay(SiteAttendanceLog{SiteName: "hdtime", Day: "2026-10-05", ScheduledAt: time.Now().UTC()}))
	require.NoError(t, repo.UpdateDay("hdtime", "2026-10-05", map[string]any{"status": AttendanceSigned, "attempts": 1, "message": "签到成功"}))

	row, err := repo.GetDay("hdtime", "2026-10-05")
	require.NoError(t, err)
	assert.Equal(t, AttendanceSigned, row.Status)
	assert.Equal(t, 1, row.Attempts)
	assert.True(t, row.Done())

	_, err = repo.GetDay("hdtime", "2026-10-04")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.Error(t, repo.EnsureDay(SiteAttendanceLog{Day: "2026-10-05"}), "site name is required")
}

func TestSiteAttendanceLog_Done(t *testing.T) {
	for status, done := range map[string]bool{
		AttendancePending: false, AttendanceSigned: true, AttendanceAlready: true,
		AttendanceFailed: true, AttendanceUnsupported: true,
	} {
		assert.Equal(t, done, SiteAttendanceLog{Status: status}.Done(), status)
	}
}

func TestSiteAttendanceRepository_Errors(t *testing.T) {
	db := newAttendanceTestDB(t)
	repo := NewSiteAttendanceRepository(db)
	assert.NoError(t, repo.UpdateDay("hdtime", "2026-10-05", nil), "nothing to write")

	require.NoError(t, db.Migrator().DropTable(&SiteAttendanceLog{}))
	assert.Error(t, repo.EnsureDay(SiteAttendanceLog{SiteName: "hdtime", Day: "2026-10-05"}))
	assert.Error(t, repo.UpdateDay("hdtime", "2026-10-05", map[string]any{"status": AttendanceSigned}))
	_, err := repo.ListDay("2026-10-05")
	assert.Error(t, err)
}
