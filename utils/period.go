package utils

import "time"

// 统计的分段：天、周（从周一开始）、月、季度（从 1、4、7、10 月开始）、年。刷流收益与辅种统计共用。
const (
	PeriodDay     = "day"
	PeriodWeek    = "week"
	PeriodMonth   = "month"
	PeriodQuarter = "quarter"
	PeriodYear    = "year"
)

// PeriodLimit 是一种分段默认几段、最多几段。
type PeriodLimit struct{ Default, Max int }

// PeriodLimits 是每种分段默认、最多几段。
var PeriodLimits = map[string]PeriodLimit{
	PeriodDay:     {30, 366},
	PeriodWeek:    {12, 104},
	PeriodMonth:   {12, 60},
	PeriodQuarter: {8, 40},
	PeriodYear:    {5, 20},
}

// PeriodStart 是 t 所在那一段的第一天（零点，t 的时区）。不认识的分段按天。
func PeriodStart(t time.Time, gran string) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	switch gran {
	case PeriodWeek:
		return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
	case PeriodMonth:
		return time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, d.Location())
	case PeriodQuarter:
		return time.Date(d.Year(), time.Month((int(d.Month())-1)/3*3+1), 1, 0, 0, 0, 0, d.Location())
	case PeriodYear:
		return time.Date(d.Year(), 1, 1, 0, 0, 0, 0, d.Location())
	}
	return d
}

// PeriodAdd 是从 start（某一段的第一天）往后 n 段的第一天（n 可以是负数）。
func PeriodAdd(start time.Time, gran string, n int) time.Time {
	switch gran {
	case PeriodWeek:
		return start.AddDate(0, 0, 7*n)
	case PeriodMonth:
		return start.AddDate(0, n, 0)
	case PeriodQuarter:
		return start.AddDate(0, 3*n, 0)
	case PeriodYear:
		return start.AddDate(n, 0, 0)
	}
	return start.AddDate(0, 0, n)
}
