package output

import (
	"fmt"
	"time"
)

// FormatBytes converts bytes to a human-readable string.
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// FormatTime formats a Unix timestamp to a readable string.
func FormatTime(ts int64) string {
	if ts == 0 {
		return "-"
	}
	t := time.Unix(ts, 0)
	return t.Format("2006-01-02 15:04")
}

// FormatDuration formats seconds to a readable string.
func FormatDuration(seconds int64) string {
	if seconds == 0 {
		return "-"
	}
	d := time.Duration(seconds) * time.Second
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
}

// FormatSpeed converts bytes/sec to a human-readable string.
func FormatSpeed(bps int64) string {
	if bps == 0 {
		return "0 B/s"
	}
	return FormatBytes(bps) + "/s"
}

// Truncate truncates a string to maxLen, adding "…" if truncated.
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
