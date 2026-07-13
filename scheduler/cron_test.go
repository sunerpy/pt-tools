// MIT License
// Copyright (c) 2025 pt-tools

package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCronValid(t *testing.T) {
	cases := []string{
		"0 10,22 * * *",
		"*/5 * * * *",
		"0 0 1-7 * 0",
		"0 9-17 * * 1-5",
		"30 8 * * *",
	}
	for _, spec := range cases {
		t.Run(spec, func(t *testing.T) {
			_, err := ParseCron(spec)
			assert.NoError(t, err)
		})
	}
}

func TestParseCronInvalid(t *testing.T) {
	cases := []struct {
		name string
		spec string
	}{
		{"too few fields", "0 10,22 *"},
		{"too many fields", "0 10,22 * * * *"},
		{"hour out of range", "0 25 * * *"},
		{"minute out of range", "60 0 * * *"},
		{"empty list item", "0 10,, * * *"},
		{"bad range", "0 5-3 * * *"},
		{"bad step", "*/0 * * * *"},
		{"non-numeric", "abc * * * *"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseCron(tc.spec)
			assert.Error(t, err)
		})
	}
}

func TestCronMatchAndWindowStart(t *testing.T) {
	c, err := ParseCron("0 10,22 * * *")
	require.NoError(t, err)

	matches := []time.Time{
		time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 18, 22, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC),
	}
	for _, ts := range matches {
		assert.True(t, c.Match(ts), "expected match for %s", ts)
	}

	misses := []time.Time{
		time.Date(2026, 5, 18, 10, 1, 0, 0, time.UTC),
		time.Date(2026, 5, 18, 11, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
	}
	for _, ts := range misses {
		assert.False(t, c.Match(ts), "expected miss for %s", ts)
	}

	at1130 := time.Date(2026, 5, 18, 11, 30, 0, 0, time.UTC)
	assert.Equal(t, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC), c.WindowStart(at1130))

	at0930 := time.Date(2026, 5, 18, 9, 30, 0, 0, time.UTC)
	assert.Equal(t, time.Date(2026, 5, 17, 22, 0, 0, 0, time.UTC), c.WindowStart(at0930))
}

func TestParseCron_FieldOutOfRangeErrors(t *testing.T) {
	cases := []string{
		"0 0 40 * *", // dom > 31
		"0 0 * 13 *", // month > 12
		"0 0 * * 9",  // dow > 6
	}
	for _, spec := range cases {
		_, err := ParseCron(spec)
		require.Error(t, err, "spec %q should error", spec)
	}
}

func TestCronWindowStart_NoMatchReturnsZero(t *testing.T) {
	// Feb 30 never exists → WindowStart scans a week and returns zero.
	c, err := ParseCron("0 0 30 2 *")
	require.NoError(t, err)
	got := c.WindowStart(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	assert.True(t, got.IsZero())
}
