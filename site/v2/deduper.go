package v2

// Deduper removes duplicate torrents based on InfoHash
type Deduper struct{}

// NewDeduper creates a new Deduper
func NewDeduper() *Deduper {
	return &Deduper{}
}

// Deduplicate removes duplicate torrents, keeping the best version of each
func (d *Deduper) Deduplicate(items []TorrentItem) []TorrentItem {
	if len(items) == 0 {
		return items
	}

	// Group by InfoHash
	byHash := make(map[string][]TorrentItem)
	noHash := make([]TorrentItem, 0)

	for _, item := range items {
		if item.InfoHash == "" {
			noHash = append(noHash, item)
			continue
		}
		byHash[item.InfoHash] = append(byHash[item.InfoHash], item)
	}

	// Merge duplicates
	result := make([]TorrentItem, 0, len(byHash)+len(noHash))

	for _, group := range byHash {
		merged := d.mergeDuplicates(group)
		result = append(result, merged)
	}

	// Add items without InfoHash (can't deduplicate these)
	result = append(result, noHash...)

	return result
}

// mergeDuplicates merges a group of duplicate torrents into one
func (d *Deduper) mergeDuplicates(items []TorrentItem) TorrentItem {
	if len(items) == 1 {
		return items[0]
	}

	// Start with the first item
	best := items[0]

	for i := 1; i < len(items); i++ {
		item := items[i]

		// Keep the one with more seeders
		if item.Seeders > best.Seeders {
			best.Seeders = item.Seeders
		}

		// Keep the one with more leechers
		if item.Leechers > best.Leechers {
			best.Leechers = item.Leechers
		}

		// Keep the one with more snatched
		if item.Snatched > best.Snatched {
			best.Snatched = item.Snatched
		}

		// Keep the most recent upload time
		if item.UploadedAt > best.UploadedAt {
			best.UploadedAt = item.UploadedAt
		}

		// Prefer free torrents
		if item.IsFree() && !best.IsFree() {
			best.DiscountLevel = item.DiscountLevel
			best.DiscountEndTime = item.DiscountEndTime
		}

		// Prefer better discount
		if DiscountPriority(item.DiscountLevel) > DiscountPriority(best.DiscountLevel) {
			best.DiscountLevel = item.DiscountLevel
			best.DiscountEndTime = item.DiscountEndTime
		}

		// Merge tags
		best.Tags = d.mergeTags(best.Tags, item.Tags)

		// Keep download URL if missing
		if best.DownloadURL == "" && item.DownloadURL != "" {
			best.DownloadURL = item.DownloadURL
		}

		// Keep magnet if missing
		if best.Magnet == "" && item.Magnet != "" {
			best.Magnet = item.Magnet
		}
	}

	return best
}

// mergeTags merges two tag lists, removing duplicates
func (d *Deduper) mergeTags(tags1, tags2 []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(tags1)+len(tags2))

	for _, tag := range tags1 {
		if !seen[tag] {
			seen[tag] = true
			result = append(result, tag)
		}
	}

	for _, tag := range tags2 {
		if !seen[tag] {
			seen[tag] = true
			result = append(result, tag)
		}
	}

	return result
}

// DeduplicateByTitle removes duplicates based on normalized title
// This is useful when InfoHash is not available
func (d *Deduper) DeduplicateByTitle(items []TorrentItem, normalizer *Normalizer) []TorrentItem {
	if len(items) == 0 {
		return items
	}

	// Group by normalized title
	byTitle := make(map[string][]TorrentItem)

	for _, item := range items {
		normalizedTitle := normalizer.NormalizeTitle(item.Title)
		byTitle[normalizedTitle] = append(byTitle[normalizedTitle], item)
	}

	// Merge duplicates
	result := make([]TorrentItem, 0, len(byTitle))

	for _, group := range byTitle {
		merged := d.mergeDuplicates(group)
		result = append(result, merged)
	}

	return result
}
