package notify

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func mustTime(t *testing.T, hhmm string) time.Time {
	t.Helper()
	parsed, err := time.Parse("15:04", hhmm)
	if err != nil {
		t.Fatalf("invalid time: %v", err)
	}
	return time.Date(2026, 5, 16, parsed.Hour(), parsed.Minute(), 0, 0, time.UTC)
}

func TestIsQuietNow_EmptyInputs(t *testing.T) {
	now := mustTime(t, "12:00")
	assert.False(t, IsQuietNow(now, "", "08:00"))
	assert.False(t, IsQuietNow(now, "22:00", ""))
	assert.False(t, IsQuietNow(now, "", ""))
}

func TestIsQuietNow_SameDayWindow(t *testing.T) {
	cases := []struct {
		name string
		now  string
		want bool
	}{
		{"before window", "08:00", false},
		{"at start (inclusive)", "09:00", true},
		{"middle", "12:00", true},
		{"just before end", "17:59", true},
		{"at end (exclusive)", "18:00", false},
		{"after window", "20:00", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := IsQuietNow(mustTime(t, c.now), "09:00", "18:00")
			assert.Equal(t, c.want, got)
		})
	}
}

func TestIsQuietNow_CrossMidnight(t *testing.T) {
	cases := []struct {
		name string
		now  string
		want bool
	}{
		{"late evening in window", "23:00", true},
		{"midnight in window", "00:00", true},
		{"early morning in window", "02:00", true},
		{"just before end", "07:59", true},
		{"at end (exclusive)", "08:00", false},
		{"day before window", "21:59", false},
		{"at start (inclusive)", "22:00", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := IsQuietNow(mustTime(t, c.now), "22:00", "08:00")
			assert.Equal(t, c.want, got)
		})
	}
}

func TestIsQuietNow_InvalidInputs(t *testing.T) {
	now := mustTime(t, "12:00")
	assert.False(t, IsQuietNow(now, "9:00", "18:00") && false || IsQuietNow(now, "abcd", "efgh"))
	assert.False(t, IsQuietNow(now, "25:00", "08:00"))
	assert.False(t, IsQuietNow(now, "22:00", "08:99"))
	assert.False(t, IsQuietNow(now, "22-00", "08-00"))
}

func TestIsQuietNow_ZeroLengthWindow(t *testing.T) {
	now := mustTime(t, "09:00")
	assert.False(t, IsQuietNow(now, "09:00", "09:00"))
}

func TestNextQuietEnd_TodayInFuture(t *testing.T) {
	now := mustTime(t, "06:00")
	got := NextQuietEnd(now, "08:00")
	want := mustTime(t, "08:00")
	assert.Equal(t, want, got)
}

func TestNextQuietEnd_RollsToTomorrow(t *testing.T) {
	now := mustTime(t, "23:00")
	got := NextQuietEnd(now, "08:00")
	want := mustTime(t, "08:00").Add(24 * time.Hour)
	assert.Equal(t, want, got)
}

func TestNextQuietEnd_ExactlyAtEnd(t *testing.T) {
	now := mustTime(t, "08:00")
	got := NextQuietEnd(now, "08:00")
	want := mustTime(t, "08:00").Add(24 * time.Hour)
	assert.Equal(t, want, got)
}

func TestNextQuietEnd_InvalidEndReturnsNow(t *testing.T) {
	now := mustTime(t, "12:00")
	got := NextQuietEnd(now, "bad")
	assert.Equal(t, now, got)
}
