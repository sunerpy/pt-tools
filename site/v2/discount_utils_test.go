package v2

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCanDownloadInTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name            string
		sizeBytes       int64
		speedBps        int64
		discountLevel   DiscountLevel
		discountEndTime time.Time
		wantCanComplete bool
	}{
		{
			name:            "no discount - always feasible",
			sizeBytes:       1024 * 1024 * 1024, // 1 GB
			speedBps:        1024 * 1024,        // 1 MB/s
			discountLevel:   DiscountNone,
			discountEndTime: now.Add(time.Hour),
			wantCanComplete: true,
		},
		{
			name:            "free discount - always feasible",
			sizeBytes:       1024 * 1024 * 1024,
			speedBps:        1024,
			discountLevel:   DiscountFree,
			discountEndTime: now.Add(time.Minute),
			wantCanComplete: true,
		},
		{
			name:            "2xfree discount - always feasible",
			sizeBytes:       1024 * 1024 * 1024,
			speedBps:        1024,
			discountLevel:   Discount2xFree,
			discountEndTime: now.Add(time.Minute),
			wantCanComplete: true,
		},
		{
			name:            "permanent discount - always feasible",
			sizeBytes:       1024 * 1024 * 1024,
			speedBps:        1024,
			discountLevel:   DiscountPercent50,
			discountEndTime: time.Time{}, // Zero time = permanent
			wantCanComplete: true,
		},
		{
			name:            "expired discount - not feasible",
			sizeBytes:       1024 * 1024,
			speedBps:        1024 * 1024,
			discountLevel:   DiscountPercent50,
			discountEndTime: now.Add(-time.Hour), // Already expired
			wantCanComplete: false,
		},
		{
			name:            "enough time - feasible",
			sizeBytes:       1024 * 1024 * 100, // 100 MB
			speedBps:        1024 * 1024,       // 1 MB/s = 100 seconds
			discountLevel:   DiscountPercent50,
			discountEndTime: now.Add(2 * time.Minute), // 120 seconds
			wantCanComplete: true,
		},
		{
			name:            "not enough time - not feasible",
			sizeBytes:       1024 * 1024 * 100, // 100 MB
			speedBps:        1024 * 1024,       // 1 MB/s = 100 seconds
			discountLevel:   DiscountPercent50,
			discountEndTime: now.Add(30 * time.Second), // Only 30 seconds
			wantCanComplete: false,
		},
		{
			name:            "zero speed - not feasible",
			sizeBytes:       1024,
			speedBps:        0,
			discountLevel:   DiscountPercent50,
			discountEndTime: now.Add(time.Hour),
			wantCanComplete: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CanDownloadInTime(tt.sizeBytes, tt.speedBps, tt.discountLevel, tt.discountEndTime)
			assert.Equal(t, tt.wantCanComplete, result.CanComplete)
		})
	}
}

func TestCanDownloadInTime_Details(t *testing.T) {
	now := time.Now()

	// 100 MB at 1 MB/s = 100 seconds
	result := CanDownloadInTime(
		100*1024*1024,          // 100 MB
		1024*1024,              // 1 MB/s
		DiscountPercent50,      // 50% discount
		now.Add(2*time.Minute), // 120 seconds
	)

	assert.True(t, result.CanComplete)
	assert.InDelta(t, 100*time.Second, result.EstimatedTime, float64(time.Second))
	assert.InDelta(t, 120*time.Second, result.TimeRemaining, float64(time.Second))
	assert.True(t, result.Margin > 0)
	assert.Equal(t, int64(50*1024*1024), result.EffectiveSize) // 50% of 100 MB
}

func TestCanDownloadInTimeSimple(t *testing.T) {
	now := time.Now()

	assert.True(t, CanDownloadInTimeSimple(1024, 1024, DiscountFree, now.Add(time.Hour)))
	assert.False(t, CanDownloadInTimeSimple(1024*1024*1024, 1024, DiscountPercent50, now.Add(time.Second)))
}

func TestEstimateDownloadTime(t *testing.T) {
	tests := []struct {
		sizeBytes int64
		speedBps  int64
		expected  time.Duration
	}{
		{1024, 1024, time.Second},
		{1024 * 1024, 1024 * 1024, time.Second},
		{1024 * 1024 * 100, 1024 * 1024, 100 * time.Second},
		{1024, 0, 0}, // Zero speed
	}

	for _, tt := range tests {
		result := EstimateDownloadTime(tt.sizeBytes, tt.speedBps)
		assert.InDelta(t, tt.expected, result, float64(time.Millisecond*100))
	}
}

func TestCalculateEffectiveDownload(t *testing.T) {
	tests := []struct {
		sizeBytes     int64
		discountLevel DiscountLevel
		expected      int64
	}{
		{1000, DiscountNone, 1000},
		{1000, DiscountFree, 0},
		{1000, Discount2xFree, 0},
		{1000, DiscountPercent50, 500},
		{1000, DiscountPercent30, 300},
		{1000, DiscountPercent70, 700},
		{1000, Discount2xUp, 1000},
		{1000, Discount2x50, 500},
	}

	for _, tt := range tests {
		t.Run(string(tt.discountLevel), func(t *testing.T) {
			result := CalculateEffectiveDownload(tt.sizeBytes, tt.discountLevel)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateEffectiveUpload(t *testing.T) {
	tests := []struct {
		sizeBytes     int64
		discountLevel DiscountLevel
		expected      int64
	}{
		{1000, DiscountNone, 1000},
		{1000, DiscountFree, 1000},
		{1000, Discount2xFree, 2000},
		{1000, DiscountPercent50, 1000},
		{1000, Discount2xUp, 2000},
		{1000, Discount2x50, 2000},
	}

	for _, tt := range tests {
		t.Run(string(tt.discountLevel), func(t *testing.T) {
			result := CalculateEffectiveUpload(tt.sizeBytes, tt.discountLevel)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateRatioImpact(t *testing.T) {
	// Current: 1TB uploaded, 500GB downloaded = ratio 2.0
	// Download 100GB free torrent, upload 50GB (50% ratio)
	// New: 1.05TB uploaded, 500GB downloaded = ratio 2.1
	impact := CalculateRatioImpact(
		1024*1024*1024*1024, // 1 TB uploaded
		512*1024*1024*1024,  // 512 GB downloaded
		100*1024*1024*1024,  // 100 GB torrent
		DiscountFree,        // Free download
		0.5,                 // 50% upload ratio
	)

	// Free download means no download impact, but upload increases
	// So ratio should increase
	assert.Greater(t, impact, 0.0)
}

func TestCalculateRatioImpact_NoDiscount(t *testing.T) {
	// Current: 1TB uploaded, 500GB downloaded = ratio 2.0
	// Download 100GB normal torrent, upload 50GB
	// New: 1.05TB uploaded, 600GB downloaded = ratio 1.75
	impact := CalculateRatioImpact(
		1024*1024*1024*1024, // 1 TB uploaded
		512*1024*1024*1024,  // 512 GB downloaded
		100*1024*1024*1024,  // 100 GB torrent
		DiscountNone,        // No discount
		0.5,                 // 50% upload ratio
	)

	// Normal download increases download, ratio should decrease
	assert.Less(t, impact, 0.0)
}

func TestDiscountPriority(t *testing.T) {
	// Verify priority ordering
	assert.Greater(t, DiscountPriority(Discount2xFree), DiscountPriority(DiscountFree))
	assert.Greater(t, DiscountPriority(DiscountFree), DiscountPriority(Discount2x50))
	assert.Greater(t, DiscountPriority(Discount2x50), DiscountPriority(DiscountPercent30))
	assert.Greater(t, DiscountPriority(DiscountPercent30), DiscountPriority(DiscountPercent50))
	assert.Greater(t, DiscountPriority(DiscountPercent50), DiscountPriority(DiscountPercent70))
	assert.Greater(t, DiscountPriority(DiscountPercent70), DiscountPriority(Discount2xUp))
	assert.Greater(t, DiscountPriority(Discount2xUp), DiscountPriority(DiscountNone))
}

func TestCompareDiscounts(t *testing.T) {
	assert.Equal(t, 1, CompareDiscounts(Discount2xFree, DiscountFree))
	assert.Equal(t, -1, CompareDiscounts(DiscountFree, Discount2xFree))
	assert.Equal(t, 0, CompareDiscounts(DiscountFree, DiscountFree))
	assert.Equal(t, 1, CompareDiscounts(DiscountFree, DiscountNone))
}

func TestIsBetterDiscount(t *testing.T) {
	assert.True(t, IsBetterDiscount(Discount2xFree, DiscountFree))
	assert.True(t, IsBetterDiscount(DiscountFree, DiscountPercent50))
	assert.False(t, IsBetterDiscount(DiscountNone, DiscountFree))
	assert.False(t, IsBetterDiscount(DiscountFree, DiscountFree))
}

func TestSuggestBestDiscount(t *testing.T) {
	now := time.Now()

	// Already free - return same
	result := SuggestBestDiscount(DiscountFree, now.Add(time.Hour), time.Minute)
	assert.Equal(t, DiscountFree, result)

	// Permanent discount - return same
	result = SuggestBestDiscount(DiscountPercent50, time.Time{}, time.Hour)
	assert.Equal(t, DiscountPercent50, result)

	// Enough time - return same
	result = SuggestBestDiscount(DiscountPercent50, now.Add(2*time.Hour), time.Hour)
	assert.Equal(t, DiscountPercent50, result)
}
