package v2

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLevelManager(t *testing.T) {
	lm := NewLevelManager()
	assert.NotNil(t, lm)

	// Should have default levels for all site types
	assert.NotNil(t, lm.GetLevels(SiteNexusPHP))
	assert.NotNil(t, lm.GetLevels(SiteMTorrent))
	assert.NotNil(t, lm.GetLevels(SiteUnit3D))
	assert.NotNil(t, lm.GetLevels(SiteGazelle))
}

func TestLevelManager_GetLevels(t *testing.T) {
	lm := NewLevelManager()

	levels := lm.GetLevels(SiteNexusPHP)
	require.NotNil(t, levels)
	assert.Greater(t, len(levels), 0)

	// First level should be "User"
	assert.Equal(t, "User", levels[0].Level)

	// Levels should be sorted by order
	for i := 1; i < len(levels); i++ {
		assert.Greater(t, levels[i].Order, levels[i-1].Order)
	}
}

func TestLevelManager_GetLevels_Unknown(t *testing.T) {
	lm := NewLevelManager()

	levels := lm.GetLevels("unknown")
	assert.Nil(t, levels)
}

func TestLevelManager_SetLevels(t *testing.T) {
	lm := NewLevelManager()

	customLevels := []LevelRequirement{
		{Level: "Newbie", MinUpload: 0, MinRatio: 0, MinDays: 0, Order: 0},
		{Level: "Regular", MinUpload: 10 * GB, MinRatio: 1.0, MinDays: 7, Order: 1},
		{Level: "VIP", MinUpload: 100 * GB, MinRatio: 2.0, MinDays: 30, Order: 2},
	}

	lm.SetLevels(SiteNexusPHP, customLevels)

	levels := lm.GetLevels(SiteNexusPHP)
	require.Len(t, levels, 3)
	assert.Equal(t, "Newbie", levels[0].Level)
	assert.Equal(t, "Regular", levels[1].Level)
	assert.Equal(t, "VIP", levels[2].Level)
}

func TestLevelManager_GetCurrentLevel(t *testing.T) {
	lm := NewLevelManager()

	tests := []struct {
		name     string
		kind     SiteKind
		upload   int64
		ratio    float64
		days     int
		expected string
	}{
		{
			name:     "New user",
			kind:     SiteNexusPHP,
			upload:   0,
			ratio:    0,
			days:     0,
			expected: "User",
		},
		{
			name:     "Power User",
			kind:     SiteNexusPHP,
			upload:   50 * GB,
			ratio:    1.05,
			days:     14,
			expected: "Power User",
		},
		{
			name:     "Elite User",
			kind:     SiteNexusPHP,
			upload:   120 * GB,
			ratio:    1.55,
			days:     28,
			expected: "Elite User",
		},
		{
			name:     "Almost Power User - missing ratio",
			kind:     SiteNexusPHP,
			upload:   50 * GB,
			ratio:    1.0, // Below 1.05
			days:     14,
			expected: "User",
		},
		{
			name:     "Almost Power User - missing days",
			kind:     SiteNexusPHP,
			upload:   50 * GB,
			ratio:    1.05,
			days:     10, // Below 14
			expected: "User",
		},
		{
			name:     "Unknown site kind",
			kind:     "unknown",
			upload:   100 * GB,
			ratio:    2.0,
			days:     30,
			expected: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := lm.GetCurrentLevel(tt.kind, tt.upload, tt.ratio, tt.days)
			assert.Equal(t, tt.expected, level)
		})
	}
}

func TestLevelManager_CalculateNextLevel(t *testing.T) {
	lm := NewLevelManager()

	t.Run("New user progress to Power User", func(t *testing.T) {
		progress := lm.CalculateNextLevel(SiteNexusPHP, 25*GB, 0.5, 7)
		require.NotNil(t, progress)

		assert.Equal(t, "User", progress.CurrentLevel)
		assert.Equal(t, "Power User", progress.NextLevel)
		assert.Greater(t, progress.UploadNeeded, int64(0))
		assert.Greater(t, progress.RatioNeeded, 0.0)
		assert.Greater(t, progress.TimeNeeded, time.Duration(0))
		assert.Less(t, progress.ProgressPercent, 100.0)
	})

	t.Run("Max level user", func(t *testing.T) {
		progress := lm.CalculateNextLevel(SiteNexusPHP, 10*TB, 5.0, 500)
		require.NotNil(t, progress)

		assert.Equal(t, "Nexus Master", progress.CurrentLevel)
		assert.Equal(t, "", progress.NextLevel)
		assert.Equal(t, 100.0, progress.ProgressPercent)
	})

	t.Run("Unknown site kind", func(t *testing.T) {
		progress := lm.CalculateNextLevel("unknown", 100*GB, 2.0, 30)
		assert.Nil(t, progress)
	})

	t.Run("Progress calculation accuracy", func(t *testing.T) {
		// User with 50% of upload, 50% of ratio, 50% of days
		progress := lm.CalculateNextLevel(SiteNexusPHP, 25*GB, 0.525, 7)
		require.NotNil(t, progress)

		// Progress should be around 50%
		assert.Greater(t, progress.ProgressPercent, 40.0)
		assert.Less(t, progress.ProgressPercent, 60.0)
	})
}

func TestLevelManager_MTorrent(t *testing.T) {
	lm := NewLevelManager()

	levels := lm.GetLevels(SiteMTorrent)
	require.NotNil(t, levels)
	assert.Greater(t, len(levels), 0)

	// Test level progression
	level := lm.GetCurrentLevel(SiteMTorrent, 100*GB, 1.5, 14)
	assert.Equal(t, "Power User", level)

	level = lm.GetCurrentLevel(SiteMTorrent, 500*GB, 2.0, 28)
	assert.Equal(t, "Elite User", level)
}

func TestLevelManager_Unit3D(t *testing.T) {
	lm := NewLevelManager()

	levels := lm.GetLevels(SiteUnit3D)
	require.NotNil(t, levels)
	assert.Greater(t, len(levels), 0)

	level := lm.GetCurrentLevel(SiteUnit3D, 25*GB, 0.8, 7)
	assert.Equal(t, "Uploader", level)
}

func TestLevelManager_Gazelle(t *testing.T) {
	lm := NewLevelManager()

	levels := lm.GetLevels(SiteGazelle)
	require.NotNil(t, levels)
	assert.Greater(t, len(levels), 0)

	level := lm.GetCurrentLevel(SiteGazelle, 10*GB, 0.65, 7)
	assert.Equal(t, "Member", level)

	level = lm.GetCurrentLevel(SiteGazelle, 25*GB, 1.0, 14)
	assert.Equal(t, "Power User", level)
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1024 * 1024, "1.00 MB"},
		{1024 * 1024 * 1024, "1.00 GB"},
		{1024 * 1024 * 1024 * 1024, "1.00 TB"},
		{1536 * 1024 * 1024 * 1024, "1.50 TB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := FormatBytes(tt.bytes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLevelManager_Concurrent(t *testing.T) {
	lm := NewLevelManager()

	done := make(chan bool)

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = lm.GetLevels(SiteNexusPHP)
				_ = lm.GetCurrentLevel(SiteNexusPHP, 100*GB, 2.0, 30)
				_ = lm.CalculateNextLevel(SiteNexusPHP, 100*GB, 2.0, 30)
			}
			done <- true
		}()
	}

	// Concurrent writes
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 50; j++ {
				lm.SetLevels(SiteNexusPHP, []LevelRequirement{
					{Level: "Test", MinUpload: 0, MinRatio: 0, MinDays: 0, Order: 0},
				})
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 15; i++ {
		<-done
	}
}
