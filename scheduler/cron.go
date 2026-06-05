package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronExpr is a tiny 5-field cron evaluator (minute hour dom month dow)
// covering the patterns used by LoginReminderMonitor:
//   - Numeric value:                     "10", "22"
//   - Comma list:                        "10,22"
//   - Wildcard:                          "*"
//   - Step:                              "*/5", "*/15"
//   - Range:                             "1-5"
//
// Day-of-week uses 0-6 with 0=Sunday (the standard cron convention).
// This is intentionally NOT a full cron implementation — it is purpose-built
// for reminder schedules and rejects unsupported syntax with a parse error
// so misconfigurations fail loud at config time, not silently at midnight.
type CronExpr struct {
	minute, hour, dom, month, dow []int
}

// ParseCron parses a 5-field cron expression like "0 10,22 * * *".
func ParseCron(spec string) (*CronExpr, error) {
	fields := strings.Fields(strings.TrimSpace(spec))
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron must have 5 fields (minute hour dom month dow), got %d in %q", len(fields), spec)
	}
	c := &CronExpr{}
	var err error
	if c.minute, err = parseCronField(fields[0], 0, 59); err != nil {
		return nil, fmt.Errorf("minute: %w", err)
	}
	if c.hour, err = parseCronField(fields[1], 0, 23); err != nil {
		return nil, fmt.Errorf("hour: %w", err)
	}
	if c.dom, err = parseCronField(fields[2], 1, 31); err != nil {
		return nil, fmt.Errorf("dom: %w", err)
	}
	if c.month, err = parseCronField(fields[3], 1, 12); err != nil {
		return nil, fmt.Errorf("month: %w", err)
	}
	if c.dow, err = parseCronField(fields[4], 0, 6); err != nil {
		return nil, fmt.Errorf("dow: %w", err)
	}
	return c, nil
}

// Match reports whether the given time falls exactly on a cron tick (minute
// granularity). It does NOT consider seconds.
func (c *CronExpr) Match(t time.Time) bool {
	return contains(c.minute, t.Minute()) &&
		contains(c.hour, t.Hour()) &&
		contains(c.dom, t.Day()) &&
		contains(c.month, int(t.Month())) &&
		contains(c.dow, int(t.Weekday()))
}

// WindowStart returns the most recent cron tick at or before t. The boundary
// is used by reminder dedup: two events that map to the same WindowStart are
// considered the same window and at most one reminder fires per window.
func (c *CronExpr) WindowStart(t time.Time) time.Time {
	probe := t.Truncate(time.Minute)
	for i := 0; i < 24*60*7; i++ {
		if c.Match(probe) {
			return probe
		}
		probe = probe.Add(-time.Minute)
	}
	return time.Time{}
}

func parseCronField(field string, lo, hi int) ([]int, error) {
	if field == "*" {
		return rangeSlice(lo, hi, 1), nil
	}
	if strings.HasPrefix(field, "*/") {
		step, err := strconv.Atoi(field[2:])
		if err != nil || step <= 0 {
			return nil, fmt.Errorf("invalid step %q", field)
		}
		return rangeSlice(lo, hi, step), nil
	}
	out := []int{}
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("empty list item in %q", field)
		}
		if strings.Contains(part, "-") {
			seg := strings.SplitN(part, "-", 2)
			start, err1 := strconv.Atoi(seg[0])
			end, err2 := strconv.Atoi(seg[1])
			if err1 != nil || err2 != nil || start > end || start < lo || end > hi {
				return nil, fmt.Errorf("invalid range %q", part)
			}
			for i := start; i <= end; i++ {
				out = append(out, i)
			}
			continue
		}
		v, err := strconv.Atoi(part)
		if err != nil || v < lo || v > hi {
			return nil, fmt.Errorf("invalid value %q (allowed %d..%d)", part, lo, hi)
		}
		out = append(out, v)
	}
	return out, nil
}

func rangeSlice(lo, hi, step int) []int {
	out := []int{}
	for i := lo; i <= hi; i += step {
		out = append(out, i)
	}
	return out
}

func contains(set []int, v int) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}
