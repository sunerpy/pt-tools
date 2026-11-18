package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPeriodStartAdd(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	at := time.Date(2026, 10, 30, 21, 5, 0, 0, loc) // 星期五
	for gran, want := range map[string]string{
		PeriodDay:     "2026-10-30",
		PeriodWeek:    "2026-10-26",
		PeriodMonth:   "2026-10-01",
		PeriodQuarter: "2026-10-01",
		PeriodYear:    "2026-01-01",
		"nope":        "2026-10-30",
	} {
		got := PeriodStart(at, gran)
		assert.Equal(t, want, got.Format(time.DateOnly), gran)
		assert.Equal(t, loc, got.Location())
	}
	start := PeriodStart(at, PeriodQuarter)
	assert.Equal(t, "2026-07-01", PeriodAdd(start, PeriodQuarter, -1).Format(time.DateOnly))
	assert.Equal(t, "2027-01-01", PeriodAdd(start, PeriodQuarter, 1).Format(time.DateOnly))
	assert.Equal(t, "2026-11-02", PeriodAdd(PeriodStart(at, PeriodWeek), PeriodWeek, 1).Format(time.DateOnly))
	assert.Equal(t, "2026-10-31", PeriodAdd(PeriodStart(at, PeriodDay), PeriodDay, 1).Format(time.DateOnly))
	assert.Equal(t, "2025-01-01", PeriodAdd(PeriodStart(at, PeriodYear), PeriodYear, -1).Format(time.DateOnly))
	assert.Equal(t, 366, PeriodLimits[PeriodDay].Max)
}
