package sitelogin

import "time"

// TimeFromUnix converts a positive Unix timestamp to a UTC-normalized time
// pointer. UTC normalization prevents timezone drift bugs (Metis EC-9).
func TimeFromUnix(sec int64) *time.Time {
	if sec <= 0 {
		return nil
	}
	t := time.Unix(sec, 0).UTC()
	return &t
}
