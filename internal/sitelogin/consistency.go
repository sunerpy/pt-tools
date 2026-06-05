package sitelogin

import "time"

// consistencyDriftThreshold is the maximum allowed delta between API-derived
// and cookie-derived last-login timestamps before flagging "drift". Both
// inputs are normalized to UTC to avoid timezone-induced false positives
// (Metis EC-9).
const consistencyDriftThreshold = 24 * time.Hour

// ConsistencyDrift is the sentinel string written into
// SiteLoginState.LastConsistencyCheck when the API and cookie last-login
// timestamps disagree by more than the threshold.
const ConsistencyDrift = "drift"

// CheckConsistency compares two last-login timestamps after UTC normalization.
// Returns "drift" only when both pointers are non-nil and the absolute delta
// exceeds 24h; returns "" in all other cases (including either side being nil).
func CheckConsistency(api, cookie *time.Time) string {
	if api == nil || cookie == nil {
		return ""
	}
	a := api.UTC()
	c := cookie.UTC()
	delta := a.Sub(c)
	if delta < 0 {
		delta = -delta
	}
	if delta > consistencyDriftThreshold {
		return ConsistencyDrift
	}
	return ""
}
