// Package schedule 算定时的下次时间：每天、每隔几天、每周、每月与一次（追剧的主动搜索与定时 AI 任务共用）。
// 时间按给定的时区（通常是进程的时区）算墙上时间：夏令时切换的那天照样是那个钟点。
package schedule

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// 定时方式。
const (
	Daily     = "daily"
	EveryDays = "every_days"
	Weekly    = "weekly"
	Monthly   = "monthly"
	Once      = "once"
)

// MaxEvery 是「每隔几天」最多几天。
const MaxEvery = 30

// ErrInvalid 是定时写得不对。
var ErrInvalid = errors.New("定时不对")

// Spec 是一个定时。
type Spec struct {
	Kind string
	// At 是钟点（HH:MM，24 小时制）
	At string
	// Every 是每隔几天（EveryDays，1–MaxEvery）
	Every int
	// Days 是每周的星期几（Weekly，0–6，0 是星期日）或每月的几号（Monthly，1–31；那个月没有这一天时按月末）
	Days []int
	// Anchor 是每隔几天的起点（只看日期，通常是建立的那天）
	Anchor time.Time
	// When 是一次的时间（Once）
	When time.Time
}

// Clock 解析 HH:MM。
func Clock(at string) (hour, minute int, err error) {
	h, m, ok := strings.Cut(strings.TrimSpace(at), ":")
	if ok && len(h) >= 1 && len(h) <= 2 && len(m) == 2 {
		hh, err1 := strconv.Atoi(h)
		mm, err2 := strconv.Atoi(m)
		if err1 == nil && err2 == nil && hh >= 0 && hh < 24 && mm >= 0 && mm < 60 {
			return hh, mm, nil
		}
	}
	return 0, 0, fmt.Errorf("%w: 时间要写成 HH:MM（例如 20:30）", ErrInvalid)
}

// Validate 检查定时写得对不对。
func (s Spec) Validate() error {
	if s.Kind == Once {
		if s.When.IsZero() {
			return fmt.Errorf("%w: 一次的时间要填", ErrInvalid)
		}
		return nil
	}
	if _, _, err := Clock(s.At); err != nil {
		return err
	}
	switch s.Kind {
	case Daily:
	case EveryDays:
		if s.Every < 1 || s.Every > MaxEvery {
			return fmt.Errorf("%w: 每隔几天要在 1 到 %d 之间", ErrInvalid, MaxEvery)
		}
	case Weekly:
		if len(s.Days) == 0 || slices.ContainsFunc(s.Days, func(d int) bool { return d < 0 || d > 6 }) {
			return fmt.Errorf("%w: 每周要选星期几（0 是星期日，6 是星期六）", ErrInvalid)
		}
	case Monthly:
		if len(s.Days) == 0 || slices.ContainsFunc(s.Days, func(d int) bool { return d < 1 || d > 31 }) {
			return fmt.Errorf("%w: 每月要选几号（1 到 31）", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: 定时方式要选 daily、every_days、weekly、monthly 或 once", ErrInvalid)
	}
	return nil
}

// Next 是 after 以后（不含 after）第一个到点的时间；定时不对或一次的时间已经过了时第二个返回值为 false。
func Next(s Spec, after time.Time, loc *time.Location) (time.Time, bool) {
	if s.Validate() != nil {
		return time.Time{}, false
	}
	if loc == nil {
		loc = time.Local
	}
	if s.Kind == Once {
		return s.When, s.When.After(after)
	}
	hour, minute, _ := Clock(s.At)
	local := after.In(loc)
	at := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, hour, minute, 0, 0, loc) }
	switch s.Kind {
	case Daily:
		for d := 0; d <= 2; d++ {
			if t := at(local.Year(), local.Month(), local.Day()+d); t.After(after) {
				return t, true
			}
		}
	case EveryDays:
		a := s.Anchor.In(loc)
		if s.Anchor.IsZero() {
			a = local
		}
		start := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
		today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
		// 从今天（或起点，起点在以后时）往前对齐到起点以后的整数个间隔
		k := 0
		if days := int(today.Sub(start).Hours() / 24); days > 0 {
			k = days / s.Every
		}
		for range 3 {
			if t := at(a.Year(), a.Month(), a.Day()+k*s.Every); t.After(after) {
				return t, true
			}
			k++
		}
	case Weekly:
		for d := 0; d <= 7; d++ {
			t := at(local.Year(), local.Month(), local.Day()+d)
			if slices.Contains(s.Days, int(t.Weekday())) && t.After(after) {
				return t, true
			}
		}
	case Monthly:
		days := slices.Clone(s.Days)
		slices.Sort(days)
		for i := 0; i <= 13; i++ {
			first := time.Date(local.Year(), local.Month()+time.Month(i), 1, 0, 0, 0, 0, loc)
			last := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, loc).Day()
			for _, d := range days {
				if t := at(first.Year(), first.Month(), min(d, last)); t.After(after) {
					return t, true
				}
			}
		}
	}
	return time.Time{}, false
}
