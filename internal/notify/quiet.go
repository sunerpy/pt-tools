package notify

import (
	"strconv"
	"strings"
	"time"
)

// IsQuietNow 判断 now 是否落入 [start, end) 的静默窗口（分钟粒度，不含秒）。
// start/end 任一为空、格式非法、或两者相等时视为无静默。
// 当 start > end（如 22:00–08:00）窗口跨越午夜，等价于 [start,24:00) ∪ [00:00,end)。
func IsQuietNow(now time.Time, start, end string) bool {
	if start == "" || end == "" {
		return false
	}
	sH, sM, ok1 := parseHHMM(start)
	eH, eM, ok2 := parseHHMM(end)
	if !ok1 || !ok2 {
		return false
	}
	nowMin := now.Hour()*60 + now.Minute()
	sMin := sH*60 + sM
	eMin := eH*60 + eM
	if sMin == eMin {
		return false
	}
	if sMin < eMin {
		return nowMin >= sMin && nowMin < eMin
	}
	return nowMin >= sMin || nowMin < eMin
}

// NextQuietEnd 返回下一次静默结束时刻（>= now+1ns）。调用方应先确认当前处于静默时段。
// 当今天的 end 时刻已过，返回明天的 end 时刻；否则返回今天的 end 时刻。
func NextQuietEnd(now time.Time, end string) time.Time {
	eH, eM, ok := parseHHMM(end)
	if !ok {
		return now
	}
	candidate := time.Date(now.Year(), now.Month(), now.Day(), eH, eM, 0, 0, now.Location())
	if !candidate.After(now) {
		candidate = candidate.Add(24 * time.Hour)
	}
	return candidate
}

func parseHHMM(s string) (h, m int, ok bool) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	hh, err := strconv.Atoi(parts[0])
	if err != nil || hh < 0 || hh > 23 {
		return 0, 0, false
	}
	mm, err := strconv.Atoi(parts[1])
	if err != nil || mm < 0 || mm > 59 {
		return 0, 0, false
	}
	return hh, mm, true
}
