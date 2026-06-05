package sitelogin

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestConsistencyCheckDrift verifies CheckConsistency produces "drift" only
// when both timestamps are non-nil AND |api - cookie| > 24h after UTC
// normalization. Cross-timezone same instants must NOT be flagged (Metis EC-9).
func TestConsistencyCheckDrift(t *testing.T) {
	mustParse := func(layout, s string) time.Time {
		t.Helper()
		ts, err := time.Parse(layout, s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		return ts
	}

	loc8 := time.FixedZone("CST", 8*3600)

	t.Run("both nil returns empty", func(t *testing.T) {
		assert.Equal(t, "", CheckConsistency(nil, nil))
	})

	t.Run("api nil cookie set returns empty", func(t *testing.T) {
		c := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
		assert.Equal(t, "", CheckConsistency(nil, &c))
	})

	t.Run("cookie nil api set returns empty", func(t *testing.T) {
		a := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
		assert.Equal(t, "", CheckConsistency(&a, nil))
	})

	t.Run("30 min apart returns empty", func(t *testing.T) {
		a := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
		c := time.Date(2026, 5, 18, 10, 30, 0, 0, time.UTC)
		assert.Equal(t, "", CheckConsistency(&a, &c))
	})

	t.Run("23h59m apart returns empty", func(t *testing.T) {
		a := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
		c := a.Add(23*time.Hour + 59*time.Minute)
		assert.Equal(t, "", CheckConsistency(&a, &c))
	})

	t.Run("exactly 24h apart returns empty (boundary)", func(t *testing.T) {
		a := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
		c := a.Add(24 * time.Hour)
		assert.Equal(t, "", CheckConsistency(&a, &c))
	})

	t.Run("24h01m apart returns drift", func(t *testing.T) {
		a := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
		c := a.Add(24*time.Hour + 1*time.Minute)
		assert.Equal(t, "drift", CheckConsistency(&a, &c))
	})

	t.Run("28h apart returns drift (R-Q-A5)", func(t *testing.T) {
		a := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
		c := a.Add(28 * time.Hour)
		assert.Equal(t, "drift", CheckConsistency(&a, &c))
	})

	t.Run("cookie before api by 25h returns drift", func(t *testing.T) {
		a := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
		c := a.Add(-25 * time.Hour)
		assert.Equal(t, "drift", CheckConsistency(&a, &c))
	})

	t.Run("cross-timezone same instant returns empty (UTC normalize EC-9)", func(t *testing.T) {
		// 2026-05-18T10:00:00+0800 == 2026-05-18T02:00:00Z (same instant)
		a := mustParse(time.RFC3339, "2026-05-18T10:00:00+08:00")
		c := time.Date(2026, 5, 18, 2, 0, 0, 0, time.UTC)
		// Sanity: same instant
		assert.True(t, a.Equal(c), "test inputs must be the same instant")
		assert.Equal(t, "", CheckConsistency(&a, &c))
	})

	t.Run("cross-timezone real drift returns drift", func(t *testing.T) {
		// 2026-05-15T10:00:00+0800 == 2026-05-15T02:00:00Z; vs 2026-05-18T10:00:00Z → ~80h
		a := mustParse(time.RFC3339, "2026-05-15T10:00:00+08:00")
		c := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
		assert.Equal(t, "drift", CheckConsistency(&a, &c))
	})

	t.Run("local-time pointer with non-UTC location still UTC-normalized", func(t *testing.T) {
		// Two pointers to the same instant but in different locations should not be flagged.
		instant := time.Date(2026, 5, 18, 2, 0, 0, 0, time.UTC)
		aLocal := instant.In(loc8)
		cLocal := instant.In(time.UTC)
		assert.Equal(t, "", CheckConsistency(&aLocal, &cLocal))
	})
}
