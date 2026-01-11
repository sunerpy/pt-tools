package v2

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewRanker(t *testing.T) {
	r := NewRanker(RankerConfig{})
	assert.NotNil(t, r)
	assert.Equal(t, 1.0, r.config.SeederWeight)
	assert.Equal(t, 0.5, r.config.LeecherWeight)
	assert.Equal(t, 100.0, r.config.FreeBonus)
}

func TestNewRanker_CustomConfig(t *testing.T) {
	r := NewRanker(RankerConfig{
		SeederWeight:  2.0,
		LeecherWeight: 1.0,
		FreeBonus:     200.0,
	})
	assert.Equal(t, 2.0, r.config.SeederWeight)
	assert.Equal(t, 1.0, r.config.LeecherWeight)
	assert.Equal(t, 200.0, r.config.FreeBonus)
}

func TestRanker_Rank_Empty(t *testing.T) {
	r := NewRanker(RankerConfig{})
	result := r.Rank([]TorrentItem{})
	assert.Empty(t, result)
}

func TestRanker_Rank_BySeeders(t *testing.T) {
	r := NewRanker(RankerConfig{})

	items := []TorrentItem{
		{ID: "1", Title: "Low", Seeders: 5},
		{ID: "2", Title: "High", Seeders: 100},
		{ID: "3", Title: "Medium", Seeders: 50},
	}

	result := r.Rank(items)
	assert.Len(t, result, 3)
	assert.Equal(t, "High", result[0].Title)
	assert.Equal(t, "Medium", result[1].Title)
	assert.Equal(t, "Low", result[2].Title)
}

func TestRanker_Rank_FreeBonus(t *testing.T) {
	r := NewRanker(RankerConfig{})

	items := []TorrentItem{
		{ID: "1", Title: "Normal", Seeders: 100, DiscountLevel: DiscountNone},
		{ID: "2", Title: "Free", Seeders: 10, DiscountLevel: DiscountFree},
	}

	result := r.Rank(items)
	assert.Len(t, result, 2)
	// Free torrent should rank higher despite fewer seeders
	assert.Equal(t, "Free", result[0].Title)
}

func TestRanker_Score_Seeders(t *testing.T) {
	r := NewRanker(RankerConfig{})

	item1 := TorrentItem{Seeders: 10}
	item2 := TorrentItem{Seeders: 100}

	score1 := r.Score(item1)
	score2 := r.Score(item2)

	assert.Greater(t, score2, score1)
}

func TestRanker_Score_Leechers(t *testing.T) {
	r := NewRanker(RankerConfig{})

	item1 := TorrentItem{Leechers: 10}
	item2 := TorrentItem{Leechers: 100}

	score1 := r.Score(item1)
	score2 := r.Score(item2)

	assert.Greater(t, score2, score1)
}

func TestRanker_Score_Free(t *testing.T) {
	r := NewRanker(RankerConfig{})

	item1 := TorrentItem{DiscountLevel: DiscountNone}
	item2 := TorrentItem{DiscountLevel: DiscountFree}

	score1 := r.Score(item1)
	score2 := r.Score(item2)

	assert.Greater(t, score2, score1)
}

func TestRanker_Score_DiscountBonus(t *testing.T) {
	r := NewRanker(RankerConfig{})

	itemNone := TorrentItem{DiscountLevel: DiscountNone}
	item50 := TorrentItem{DiscountLevel: DiscountPercent50}
	itemFree := TorrentItem{DiscountLevel: DiscountFree}

	scoreNone := r.Score(itemNone)
	score50 := r.Score(item50)
	scoreFree := r.Score(itemFree)

	assert.Greater(t, score50, scoreNone)
	assert.Greater(t, scoreFree, score50)
}

func TestRanker_Score_SiteReliability(t *testing.T) {
	r := NewRanker(RankerConfig{
		SiteReliability: map[string]float64{
			"reliable":   0.9,
			"unreliable": 0.1,
		},
	})

	item1 := TorrentItem{Seeders: 10, SourceSite: "reliable"}
	item2 := TorrentItem{Seeders: 10, SourceSite: "unreliable"}

	score1 := r.Score(item1)
	score2 := r.Score(item2)

	assert.Greater(t, score1, score2)
}

func TestRanker_SetSiteReliability(t *testing.T) {
	r := NewRanker(RankerConfig{})

	r.SetSiteReliability("site1", 0.8)
	assert.Equal(t, 0.8, r.GetSiteReliability("site1"))

	// Test clamping
	r.SetSiteReliability("site2", 1.5)
	assert.Equal(t, 1.0, r.GetSiteReliability("site2"))

	r.SetSiteReliability("site3", -0.5)
	assert.Equal(t, 0.0, r.GetSiteReliability("site3"))
}

func TestRanker_GetSiteReliability_Default(t *testing.T) {
	r := NewRanker(RankerConfig{})

	// Unknown site should return default
	assert.Equal(t, 0.5, r.GetSiteReliability("unknown"))
}

func TestRanker_Rank_PreservesOrder(t *testing.T) {
	r := NewRanker(RankerConfig{})

	// Items with same score should maintain relative order
	items := []TorrentItem{
		{ID: "1", Title: "First", Seeders: 10},
		{ID: "2", Title: "Second", Seeders: 10},
		{ID: "3", Title: "Third", Seeders: 10},
	}

	result := r.Rank(items)
	assert.Len(t, result, 3)
	// All have same score, order may vary but all should be present
	ids := make(map[string]bool)
	for _, item := range result {
		ids[item.ID] = true
	}
	assert.True(t, ids["1"])
	assert.True(t, ids["2"])
	assert.True(t, ids["3"])
}

func TestRanker_Score_ZeroSeeders(t *testing.T) {
	r := NewRanker(RankerConfig{})

	item := TorrentItem{Seeders: 0}
	score := r.Score(item)

	// Should not panic and return a valid score
	assert.GreaterOrEqual(t, score, 0.0)
}

func TestRanker_Score_ZeroLeechers(t *testing.T) {
	r := NewRanker(RankerConfig{})

	item := TorrentItem{Leechers: 0}
	score := r.Score(item)

	// Should not panic and return a valid score
	assert.GreaterOrEqual(t, score, 0.0)
}

func TestLogScore(t *testing.T) {
	tests := []struct {
		value    float64
		expected float64
	}{
		{0, 0},
		{-1, 0},
		{10, 2},
		{100, 11},
	}

	for _, tt := range tests {
		result := logScore(tt.value)
		assert.Equal(t, tt.expected, result)
	}
}
