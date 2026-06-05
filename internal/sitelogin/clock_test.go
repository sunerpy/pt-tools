package sitelogin

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealClockNow(t *testing.T) {
	c := NewRealClock()
	before := time.Now()
	now := c.Now()
	after := time.Now()

	assert.True(t, before.Before(now) || before.Equal(now), "clock should return current time")
	assert.True(t, now.Before(after) || now.Equal(after), "clock should return time not in future")
}

func TestFakeClockNow(t *testing.T) {
	fixedTime := time.Date(2026, 5, 18, 10, 30, 0, 0, time.UTC)
	fc := NewFakeClock(fixedTime)
	assert.Equal(t, fixedTime, fc.Now())
}

func TestFakeClockAdvance(t *testing.T) {
	tests := []struct {
		name     string
		initial  time.Time
		advances []time.Duration
		expected time.Time
	}{
		{
			name:     "single advance",
			initial:  time.Date(2026, 5, 18, 10, 30, 0, 0, time.UTC),
			advances: []time.Duration{1 * time.Hour},
			expected: time.Date(2026, 5, 18, 11, 30, 0, 0, time.UTC),
		},
		{
			name:     "multiple advances cumulative",
			initial:  time.Date(2026, 5, 18, 10, 30, 0, 0, time.UTC),
			advances: []time.Duration{1 * time.Hour, 45 * time.Minute, 15 * time.Second},
			expected: time.Date(2026, 5, 18, 12, 15, 15, 0, time.UTC),
		},
		{
			name:     "advance zero duration",
			initial:  time.Date(2026, 5, 18, 10, 30, 0, 0, time.UTC),
			advances: []time.Duration{0},
			expected: time.Date(2026, 5, 18, 10, 30, 0, 0, time.UTC),
		},
		{
			name:     "advance negative duration (monotonic)",
			initial:  time.Date(2026, 5, 18, 10, 30, 0, 0, time.UTC),
			advances: []time.Duration{-1 * time.Hour},
			expected: time.Date(2026, 5, 18, 9, 30, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := NewFakeClock(tt.initial)
			for _, d := range tt.advances {
				fc.Advance(d)
			}
			assert.Equal(t, tt.expected, fc.Now())
		})
	}
}

func TestFakeClockMonotonic(t *testing.T) {
	fc := NewFakeClock(time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))
	times := []time.Time{}

	for i := 0; i < 10; i++ {
		times = append(times, fc.Now())
		fc.Advance(1 * time.Second)
	}

	// Verify monotonic increase
	for i := 1; i < len(times); i++ {
		assert.True(t, times[i-1].Before(times[i]), "times should be monotonically increasing")
	}
}

func TestClockInterface(t *testing.T) {
	// Verify that both implementations satisfy the Clock interface
	var _ Clock = (*RealClock)(nil)
	var _ Clock = (*FakeClock)(nil)

	rc := NewRealClock()
	fc := NewFakeClock(time.Now())

	require.NotNil(t, rc.Now())
	require.NotNil(t, fc.Now())
}
