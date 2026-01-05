package v2

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteKindConstants(t *testing.T) {
	assert.Equal(t, SiteKind("nexusphp"), SiteNexusPHP)
	assert.Equal(t, SiteKind("unit3d"), SiteUnit3D)
	assert.Equal(t, SiteKind("gazelle"), SiteGazelle)
	assert.Equal(t, SiteKind("mtorrent"), SiteMTorrent)
}

func TestDiscountLevelConstants(t *testing.T) {
	assert.Equal(t, DiscountLevel("NONE"), DiscountNone)
	assert.Equal(t, DiscountLevel("FREE"), DiscountFree)
	assert.Equal(t, DiscountLevel("2XFREE"), Discount2xFree)
	assert.Equal(t, DiscountLevel("PERCENT_50"), DiscountPercent50)
	assert.Equal(t, DiscountLevel("PERCENT_30"), DiscountPercent30)
	assert.Equal(t, DiscountLevel("PERCENT_70"), DiscountPercent70)
	assert.Equal(t, DiscountLevel("2XUP"), Discount2xUp)
	assert.Equal(t, DiscountLevel("2X50"), Discount2x50)
}

func TestIsFreeTorrent(t *testing.T) {
	tests := []struct {
		level    DiscountLevel
		expected bool
	}{
		{DiscountNone, false},
		{DiscountFree, true},
		{Discount2xFree, true},
		{DiscountPercent50, false},
		{DiscountPercent30, false},
		{DiscountPercent70, false},
		{Discount2xUp, false},
		{Discount2x50, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			assert.Equal(t, tt.expected, IsFreeTorrent(tt.level))
		})
	}
}

func TestDiscountLevel_GetDownloadRatio(t *testing.T) {
	tests := []struct {
		level    DiscountLevel
		expected float64
	}{
		{DiscountNone, 1.0},
		{DiscountFree, 0.0},
		{Discount2xFree, 0.0},
		{DiscountPercent50, 0.5},
		{DiscountPercent30, 0.3},
		{DiscountPercent70, 0.7},
		{Discount2xUp, 1.0},
		{Discount2x50, 0.5},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.level.GetDownloadRatio())
		})
	}
}

func TestDiscountLevel_GetUploadRatio(t *testing.T) {
	tests := []struct {
		level    DiscountLevel
		expected float64
	}{
		{DiscountNone, 1.0},
		{DiscountFree, 1.0},
		{Discount2xFree, 2.0},
		{DiscountPercent50, 1.0},
		{DiscountPercent30, 1.0},
		{DiscountPercent70, 1.0},
		{Discount2xUp, 2.0},
		{Discount2x50, 2.0},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.level.GetUploadRatio())
		})
	}
}

func TestSearchQuery_Validate(t *testing.T) {
	tests := []struct {
		name    string
		query   SearchQuery
		wantErr bool
	}{
		{
			name:    "valid empty query",
			query:   SearchQuery{},
			wantErr: false,
		},
		{
			name: "valid query with keyword",
			query: SearchQuery{
				Keyword:  "test",
				Page:     1,
				PageSize: 20,
			},
			wantErr: false,
		},
		{
			name: "negative page",
			query: SearchQuery{
				Page: -1,
			},
			wantErr: true,
		},
		{
			name: "negative page size",
			query: SearchQuery{
				PageSize: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.query.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTorrentItem_IsFree(t *testing.T) {
	tests := []struct {
		level    DiscountLevel
		expected bool
	}{
		{DiscountNone, false},
		{DiscountFree, true},
		{Discount2xFree, true},
		{DiscountPercent50, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			item := TorrentItem{DiscountLevel: tt.level}
			assert.Equal(t, tt.expected, item.IsFree())
		})
	}
}

func TestTorrentItem_IsDiscountActive(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		item     TorrentItem
		expected bool
	}{
		{
			name:     "no discount",
			item:     TorrentItem{DiscountLevel: DiscountNone},
			expected: false,
		},
		{
			name:     "free with no end time (permanent)",
			item:     TorrentItem{DiscountLevel: DiscountFree},
			expected: true,
		},
		{
			name: "free with future end time",
			item: TorrentItem{
				DiscountLevel:   DiscountFree,
				DiscountEndTime: now.Add(time.Hour),
			},
			expected: true,
		},
		{
			name: "free with past end time",
			item: TorrentItem{
				DiscountLevel:   DiscountFree,
				DiscountEndTime: now.Add(-time.Hour),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.item.IsDiscountActive())
		})
	}
}

func TestFreeDiscountLevels(t *testing.T) {
	require.Len(t, FreeDiscountLevels, 2)
	assert.Contains(t, FreeDiscountLevels, DiscountFree)
	assert.Contains(t, FreeDiscountLevels, Discount2xFree)
}
