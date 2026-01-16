package utils

import "time"

// CSTLocation is the China Standard Time timezone (UTC+8).
// All PT sites (M-Team, HDSky, SpringSunday, HDDolby, etc.) return times in CST.
var CSTLocation = time.FixedZone("CST", 8*3600)

// ParseTimeInCST parses a time string in CST timezone.
func ParseTimeInCST(layout, value string) (time.Time, error) {
	return time.ParseInLocation(layout, value, CSTLocation)
}
