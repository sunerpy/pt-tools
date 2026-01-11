package v2

import (
	"sort"
)

// Ranker scores and ranks torrent search results
type Ranker struct {
	config RankerConfig
}

// RankerConfig holds configuration for the Ranker
type RankerConfig struct {
	// SeederWeight is the weight for seeders in scoring (default: 1.0)
	SeederWeight float64 `json:"seederWeight,omitempty"`
	// LeecherWeight is the weight for leechers in scoring (default: 0.5)
	LeecherWeight float64 `json:"leecherWeight,omitempty"`
	// FreeBonus is the bonus score for free torrents (default: 100)
	FreeBonus float64 `json:"freeBonus,omitempty"`
	// SizeWeight is the weight for size in scoring (default: 0.1)
	SizeWeight float64 `json:"sizeWeight,omitempty"`
	// RecencyWeight is the weight for recency in scoring (default: 0.2)
	RecencyWeight float64 `json:"recencyWeight,omitempty"`
	// SiteReliability maps site IDs to reliability scores (0-1)
	SiteReliability map[string]float64 `json:"siteReliability,omitempty"`
}

// NewRanker creates a new Ranker with the given configuration
func NewRanker(config RankerConfig) *Ranker {
	// Apply defaults
	if config.SeederWeight == 0 {
		config.SeederWeight = 1.0
	}
	if config.LeecherWeight == 0 {
		config.LeecherWeight = 0.5
	}
	if config.FreeBonus == 0 {
		config.FreeBonus = 100
	}
	if config.SizeWeight == 0 {
		config.SizeWeight = 0.1
	}
	if config.RecencyWeight == 0 {
		config.RecencyWeight = 0.2
	}
	if config.SiteReliability == nil {
		config.SiteReliability = make(map[string]float64)
	}

	return &Ranker{config: config}
}

// Rank sorts torrents by score in descending order
func (r *Ranker) Rank(items []TorrentItem) []TorrentItem {
	if len(items) == 0 {
		return items
	}

	// Calculate scores
	type scoredItem struct {
		item  TorrentItem
		score float64
	}

	scored := make([]scoredItem, len(items))
	for i, item := range items {
		scored[i] = scoredItem{
			item:  item,
			score: r.Score(item),
		}
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Extract sorted items
	result := make([]TorrentItem, len(items))
	for i, s := range scored {
		result[i] = s.item
	}

	return result
}

// Score calculates a score for a torrent item
func (r *Ranker) Score(item TorrentItem) float64 {
	var score float64

	// Seeder score (logarithmic to prevent huge seeders from dominating)
	if item.Seeders > 0 {
		score += r.config.SeederWeight * logScore(float64(item.Seeders))
	}

	// Leecher score (indicates demand)
	if item.Leechers > 0 {
		score += r.config.LeecherWeight * logScore(float64(item.Leechers))
	}

	// Free bonus
	if item.IsFree() {
		score += r.config.FreeBonus
	}

	// Discount bonus (proportional to discount level)
	discountBonus := r.config.FreeBonus * (1 - item.DiscountLevel.GetDownloadRatio())
	score += discountBonus

	// Site reliability bonus
	if reliability, ok := r.config.SiteReliability[item.SourceSite]; ok {
		score *= (1 + reliability)
	}

	return score
}

// logScore calculates a logarithmic score to prevent extreme values from dominating
func logScore(value float64) float64 {
	if value <= 0 {
		return 0
	}
	// Use log10 for more intuitive scaling
	// log10(1) = 0, log10(10) = 1, log10(100) = 2, etc.
	return 1 + (value / 10) // Simple linear scaling with diminishing returns
}

// SetSiteReliability sets the reliability score for a site
func (r *Ranker) SetSiteReliability(siteID string, reliability float64) {
	if reliability < 0 {
		reliability = 0
	}
	if reliability > 1 {
		reliability = 1
	}
	r.config.SiteReliability[siteID] = reliability
}

// GetSiteReliability returns the reliability score for a site
func (r *Ranker) GetSiteReliability(siteID string) float64 {
	if reliability, ok := r.config.SiteReliability[siteID]; ok {
		return reliability
	}
	return 0.5 // Default reliability
}
