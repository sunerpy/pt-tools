package v2

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewDeduper(t *testing.T) {
	d := NewDeduper()
	assert.NotNil(t, d)
}

func TestDeduper_Deduplicate_Empty(t *testing.T) {
	d := NewDeduper()
	result := d.Deduplicate([]TorrentItem{})
	assert.Empty(t, result)
}

func TestDeduper_Deduplicate_NoDuplicates(t *testing.T) {
	d := NewDeduper()

	items := []TorrentItem{
		{ID: "1", InfoHash: "hash1", Title: "Torrent 1"},
		{ID: "2", InfoHash: "hash2", Title: "Torrent 2"},
		{ID: "3", InfoHash: "hash3", Title: "Torrent 3"},
	}

	result := d.Deduplicate(items)
	assert.Len(t, result, 3)
}

func TestDeduper_Deduplicate_WithDuplicates(t *testing.T) {
	d := NewDeduper()

	items := []TorrentItem{
		{ID: "1", InfoHash: "hash1", Title: "Torrent 1", Seeders: 10},
		{ID: "2", InfoHash: "hash1", Title: "Torrent 1 Copy", Seeders: 20},
		{ID: "3", InfoHash: "hash2", Title: "Torrent 2", Seeders: 5},
	}

	result := d.Deduplicate(items)
	assert.Len(t, result, 2)

	// Find the merged item
	var mergedItem TorrentItem
	for _, item := range result {
		if item.InfoHash == "hash1" {
			mergedItem = item
			break
		}
	}

	// Should keep the best seeders count
	assert.Equal(t, 20, mergedItem.Seeders)
}

func TestDeduper_Deduplicate_NoInfoHash(t *testing.T) {
	d := NewDeduper()

	items := []TorrentItem{
		{ID: "1", Title: "Torrent 1"},
		{ID: "2", Title: "Torrent 2"},
	}

	result := d.Deduplicate(items)
	assert.Len(t, result, 2)
}

func TestDeduper_Deduplicate_MixedInfoHash(t *testing.T) {
	d := NewDeduper()

	items := []TorrentItem{
		{ID: "1", InfoHash: "hash1", Title: "Torrent 1"},
		{ID: "2", Title: "Torrent 2 (no hash)"},
		{ID: "3", InfoHash: "hash1", Title: "Torrent 1 Copy"},
	}

	result := d.Deduplicate(items)
	assert.Len(t, result, 2) // 1 merged + 1 without hash
}

func TestDeduper_MergeDuplicates_Seeders(t *testing.T) {
	d := NewDeduper()

	items := []TorrentItem{
		{ID: "1", InfoHash: "hash1", Seeders: 10},
		{ID: "2", InfoHash: "hash1", Seeders: 20},
	}

	result := d.Deduplicate(items)
	assert.Len(t, result, 1)
	assert.Equal(t, 20, result[0].Seeders)
}

func TestDeduper_MergeDuplicates_Leechers(t *testing.T) {
	d := NewDeduper()

	items := []TorrentItem{
		{ID: "1", InfoHash: "hash1", Leechers: 5},
		{ID: "2", InfoHash: "hash1", Leechers: 15},
	}

	result := d.Deduplicate(items)
	assert.Len(t, result, 1)
	assert.Equal(t, 15, result[0].Leechers)
}

func TestDeduper_MergeDuplicates_Snatched(t *testing.T) {
	d := NewDeduper()

	items := []TorrentItem{
		{ID: "1", InfoHash: "hash1", Snatched: 100},
		{ID: "2", InfoHash: "hash1", Snatched: 200},
	}

	result := d.Deduplicate(items)
	assert.Len(t, result, 1)
	assert.Equal(t, 200, result[0].Snatched)
}

func TestDeduper_MergeDuplicates_UploadedAt(t *testing.T) {
	d := NewDeduper()

	items := []TorrentItem{
		{ID: "1", InfoHash: "hash1", UploadedAt: 1000},
		{ID: "2", InfoHash: "hash1", UploadedAt: 2000},
	}

	result := d.Deduplicate(items)
	assert.Len(t, result, 1)
	assert.Equal(t, int64(2000), result[0].UploadedAt)
}

func TestDeduper_MergeDuplicates_PreferFree(t *testing.T) {
	d := NewDeduper()

	items := []TorrentItem{
		{ID: "1", InfoHash: "hash1", DiscountLevel: DiscountNone},
		{ID: "2", InfoHash: "hash1", DiscountLevel: DiscountFree},
	}

	result := d.Deduplicate(items)
	assert.Len(t, result, 1)
	assert.Equal(t, DiscountFree, result[0].DiscountLevel)
}

func TestDeduper_MergeDuplicates_PreferBetterDiscount(t *testing.T) {
	d := NewDeduper()

	items := []TorrentItem{
		{ID: "1", InfoHash: "hash1", DiscountLevel: DiscountPercent50},
		{ID: "2", InfoHash: "hash1", DiscountLevel: Discount2xFree},
	}

	result := d.Deduplicate(items)
	assert.Len(t, result, 1)
	assert.Equal(t, Discount2xFree, result[0].DiscountLevel)
}

func TestDeduper_MergeDuplicates_Tags(t *testing.T) {
	d := NewDeduper()

	items := []TorrentItem{
		{ID: "1", InfoHash: "hash1", Tags: []string{"action", "movie"}},
		{ID: "2", InfoHash: "hash1", Tags: []string{"movie", "thriller"}},
	}

	result := d.Deduplicate(items)
	assert.Len(t, result, 1)
	assert.Contains(t, result[0].Tags, "action")
	assert.Contains(t, result[0].Tags, "movie")
	assert.Contains(t, result[0].Tags, "thriller")
}

func TestDeduper_MergeDuplicates_DownloadURL(t *testing.T) {
	d := NewDeduper()

	items := []TorrentItem{
		{ID: "1", InfoHash: "hash1", DownloadURL: ""},
		{ID: "2", InfoHash: "hash1", DownloadURL: "http://example.com/download"},
	}

	result := d.Deduplicate(items)
	assert.Len(t, result, 1)
	assert.Equal(t, "http://example.com/download", result[0].DownloadURL)
}

func TestDeduper_MergeDuplicates_Magnet(t *testing.T) {
	d := NewDeduper()

	items := []TorrentItem{
		{ID: "1", InfoHash: "hash1", Magnet: ""},
		{ID: "2", InfoHash: "hash1", Magnet: "magnet:?xt=urn:btih:hash1"},
	}

	result := d.Deduplicate(items)
	assert.Len(t, result, 1)
	assert.Equal(t, "magnet:?xt=urn:btih:hash1", result[0].Magnet)
}

func TestDeduper_MergeTags(t *testing.T) {
	d := NewDeduper()

	tests := []struct {
		name     string
		tags1    []string
		tags2    []string
		expected []string
	}{
		{
			name:     "no overlap",
			tags1:    []string{"a", "b"},
			tags2:    []string{"c", "d"},
			expected: []string{"a", "b", "c", "d"},
		},
		{
			name:     "with overlap",
			tags1:    []string{"a", "b"},
			tags2:    []string{"b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "empty first",
			tags1:    []string{},
			tags2:    []string{"a", "b"},
			expected: []string{"a", "b"},
		},
		{
			name:     "empty second",
			tags1:    []string{"a", "b"},
			tags2:    []string{},
			expected: []string{"a", "b"},
		},
		{
			name:     "both empty",
			tags1:    []string{},
			tags2:    []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.mergeTags(tt.tags1, tt.tags2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeduper_DeduplicateByTitle(t *testing.T) {
	d := NewDeduper()
	n := NewNormalizer()

	items := []TorrentItem{
		{ID: "1", Title: "[HDSky] Movie 1080p x264", Seeders: 10},
		{ID: "2", Title: "[CHDBits] Movie 1080p H.264", Seeders: 20},
		{ID: "3", Title: "Different Movie 720p", Seeders: 5},
	}

	result := d.DeduplicateByTitle(items, n)
	assert.Len(t, result, 2)
}

func TestDeduper_MergeDuplicates_DiscountEndTime(t *testing.T) {
	d := NewDeduper()

	now := time.Now()
	later := now.Add(24 * time.Hour)

	items := []TorrentItem{
		{ID: "1", InfoHash: "hash1", DiscountLevel: DiscountFree, DiscountEndTime: now},
		{ID: "2", InfoHash: "hash1", DiscountLevel: Discount2xFree, DiscountEndTime: later},
	}

	result := d.Deduplicate(items)
	assert.Len(t, result, 1)
	// Should keep the better discount (2xFree) with its end time
	assert.Equal(t, Discount2xFree, result[0].DiscountLevel)
	assert.Equal(t, later, result[0].DiscountEndTime)
}
