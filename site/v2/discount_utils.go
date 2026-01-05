package v2

import (
	"time"
)

// DownloadFeasibility represents the result of download feasibility calculation
type DownloadFeasibility struct {
	// CanComplete indicates if the download can complete before discount ends
	CanComplete bool `json:"canComplete"`
	// EstimatedTime is the estimated download time
	EstimatedTime time.Duration `json:"estimatedTime"`
	// TimeRemaining is the time remaining before discount ends
	TimeRemaining time.Duration `json:"timeRemaining"`
	// EffectiveSize is the size that counts towards download quota
	EffectiveSize int64 `json:"effectiveSize"`
	// Margin is the time margin (positive = can complete with time to spare)
	Margin time.Duration `json:"margin"`
}

// CanDownloadInTime calculates if a torrent can be downloaded before the discount expires
// Parameters:
//   - sizeBytes: total torrent size in bytes
//   - downloadSpeedBps: download speed in bytes per second
//   - discountLevel: current discount level
//   - discountEndTime: when the discount expires (zero time means permanent)
//
// Returns DownloadFeasibility with detailed information
func CanDownloadInTime(sizeBytes, downloadSpeedBps int64, discountLevel DiscountLevel, discountEndTime time.Time) DownloadFeasibility {
	result := DownloadFeasibility{}

	// If no discount or permanent discount, always feasible
	if discountLevel == DiscountNone {
		result.CanComplete = true
		result.EffectiveSize = sizeBytes
		return result
	}

	// Calculate effective size based on discount
	downloadRatio := discountLevel.GetDownloadRatio()
	result.EffectiveSize = int64(float64(sizeBytes) * downloadRatio)

	// If free download, always feasible (no quota impact)
	if downloadRatio == 0 {
		result.CanComplete = true
		return result
	}

	// If permanent discount (zero end time), always feasible
	if discountEndTime.IsZero() {
		result.CanComplete = true
		return result
	}

	// Calculate time remaining
	result.TimeRemaining = time.Until(discountEndTime)
	if result.TimeRemaining <= 0 {
		// Discount already expired
		result.CanComplete = false
		return result
	}

	// Calculate estimated download time
	if downloadSpeedBps <= 0 {
		// Unknown speed, assume not feasible
		result.CanComplete = false
		return result
	}

	downloadSeconds := float64(sizeBytes) / float64(downloadSpeedBps)
	result.EstimatedTime = time.Duration(downloadSeconds * float64(time.Second))

	// Calculate margin
	result.Margin = result.TimeRemaining - result.EstimatedTime
	result.CanComplete = result.Margin >= 0

	return result
}

// CanDownloadInTimeSimple is a simplified version that returns just a boolean
func CanDownloadInTimeSimple(sizeBytes, downloadSpeedBps int64, discountLevel DiscountLevel, discountEndTime time.Time) bool {
	return CanDownloadInTime(sizeBytes, downloadSpeedBps, discountLevel, discountEndTime).CanComplete
}

// EstimateDownloadTime estimates the download time for a given size and speed
func EstimateDownloadTime(sizeBytes, downloadSpeedBps int64) time.Duration {
	if downloadSpeedBps <= 0 {
		return 0
	}
	seconds := float64(sizeBytes) / float64(downloadSpeedBps)
	return time.Duration(seconds * float64(time.Second))
}

// CalculateEffectiveDownload calculates the effective download size based on discount
func CalculateEffectiveDownload(sizeBytes int64, discountLevel DiscountLevel) int64 {
	return int64(float64(sizeBytes) * discountLevel.GetDownloadRatio())
}

// CalculateEffectiveUpload calculates the effective upload size based on discount
func CalculateEffectiveUpload(sizeBytes int64, discountLevel DiscountLevel) int64 {
	return int64(float64(sizeBytes) * discountLevel.GetUploadRatio())
}

// CalculateRatioImpact calculates the ratio impact of downloading a torrent
// Returns the change in ratio (positive = ratio increases, negative = ratio decreases)
func CalculateRatioImpact(currentUploaded, currentDownloaded, torrentSize int64, discountLevel DiscountLevel, expectedUploadRatio float64) float64 {
	if currentDownloaded == 0 && currentUploaded == 0 {
		return 0
	}

	// Calculate effective download and upload
	effectiveDownload := CalculateEffectiveDownload(torrentSize, discountLevel)
	effectiveUpload := int64(float64(torrentSize) * expectedUploadRatio * discountLevel.GetUploadRatio())

	// Calculate new totals
	newUploaded := currentUploaded + effectiveUpload
	newDownloaded := currentDownloaded + effectiveDownload

	// Calculate ratios
	var currentRatio, newRatio float64
	if currentDownloaded > 0 {
		currentRatio = float64(currentUploaded) / float64(currentDownloaded)
	}
	if newDownloaded > 0 {
		newRatio = float64(newUploaded) / float64(newDownloaded)
	}

	return newRatio - currentRatio
}

// SuggestBestDiscount suggests the best discount level to wait for
// based on current discount and time constraints
func SuggestBestDiscount(currentDiscount DiscountLevel, discountEndTime time.Time, minTimeNeeded time.Duration) DiscountLevel {
	// If already free, no better option
	if IsFreeTorrent(currentDiscount) {
		return currentDiscount
	}

	// If discount is permanent or has enough time, current is fine
	if discountEndTime.IsZero() || time.Until(discountEndTime) >= minTimeNeeded {
		return currentDiscount
	}

	// Otherwise, suggest waiting for a better discount
	// This is a simplified heuristic - in practice, you'd want to
	// consider site-specific discount patterns
	return currentDiscount
}

// DiscountPriority returns a priority value for sorting torrents by discount
// Higher priority = better discount
func DiscountPriority(level DiscountLevel) int {
	switch level {
	case Discount2xFree:
		return 100 // Best: free download + 2x upload
	case DiscountFree:
		return 90 // Free download
	case Discount2x50:
		return 70 // 50% download + 2x upload
	case DiscountPercent30:
		return 60 // 30% download
	case DiscountPercent50:
		return 50 // 50% download
	case DiscountPercent70:
		return 40 // 70% download
	case Discount2xUp:
		return 30 // 2x upload only
	default:
		return 0 // No discount
	}
}

// CompareDiscounts compares two discount levels
// Returns: -1 if a < b, 0 if a == b, 1 if a > b
func CompareDiscounts(a, b DiscountLevel) int {
	pa, pb := DiscountPriority(a), DiscountPriority(b)
	if pa < pb {
		return -1
	}
	if pa > pb {
		return 1
	}
	return 0
}

// IsBetterDiscount returns true if a is a better discount than b
func IsBetterDiscount(a, b DiscountLevel) bool {
	return CompareDiscounts(a, b) > 0
}
