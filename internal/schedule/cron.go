package schedule

import (
	"fmt"
	"math/bits"
	"strconv"
	"strings"
	"time"
)

// cron 表达式：标准的 5 段（分 时 日 月 星期），每段可以写 *、数字、a-b、*/n、a-b/n、a/n 与用逗号分开的几项；
// 月份与星期也认英文缩写（JAN–DEC、SUN–SAT），星期的 0 与 7 都是星期日。日与星期都限定了（都不是 *）时，
// 两个满足一个就到点（和 Vixie cron 一样）。也认 @hourly、@daily、@weekly、@monthly、@yearly 这几个简写。
// 一个定时可以写几条（分号或换行分开），到点的取最早的一条：例如综艺周五晚上播，写「0 22 * * 5; 0 9,13 * * 6」。

// MaxCronExprs 是一个定时最多几条 cron 表达式。
const MaxCronExprs = 10

// cronSearchDays 是往后找到点时间最多看多少天：2 月 29 日这类表达式最长要等 8 年（2100 年不是闰年）。
const cronSearchDays = 366 * 9

var cronMacros = map[string]string{
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
	"@monthly":  "0 0 1 * *",
	"@weekly":   "0 0 * * 0",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@hourly":   "0 * * * *",
}

var (
	monthNames   = []string{"JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"}
	weekdayNames = []string{"SUN", "MON", "TUE", "WED", "THU", "FRI", "SAT"}
)

// cronField 是一段的取值范围与名字。
type cronField struct {
	label    string
	min, max int
	names    []string // names[i] 对应 min+i
}

var cronFields = [5]cronField{
	{label: "分钟", min: 0, max: 59},
	{label: "小时", min: 0, max: 23},
	{label: "日", min: 1, max: 31},
	{label: "月", min: 1, max: 12, names: monthNames},
	// 星期按 0–7 解析（7 也是星期日），解析完把 7 并到 0
	{label: "星期", min: 0, max: 7, names: weekdayNames},
}

// cronExpr 是一条解析好的表达式：每段是一个位图。
type cronExpr struct {
	minute, hour, dom, month, dow uint64
	// domAny、dowAny 是日、星期写的是 *（不限定）：两个都限定时按「满足一个就行」
	domAny, dowAny bool
}

// CronSet 是一个定时里的几条 cron 表达式。
type CronSet struct {
	exprs []cronExpr
	text  string
}

// String 是规整后的写法：每条的几段用一个空格分开，几条之间用「; 」。
func (c CronSet) String() string { return c.text }

// ParseCron 解析一条或几条（分号或换行分开）cron 表达式。
func ParseCron(src string) (CronSet, error) {
	var out CronSet
	var texts []string
	for _, part := range strings.FieldsFunc(src, func(r rune) bool { return r == ';' || r == '\n' || r == '\r' }) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		expr, text, err := parseCronExpr(part)
		if err != nil {
			if strings.ContainsAny(strings.TrimSpace(src), ";\n") {
				return CronSet{}, fmt.Errorf("%w: 「%s」%s", ErrInvalid, part, err.Error())
			}
			return CronSet{}, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
		}
		out.exprs = append(out.exprs, expr)
		texts = append(texts, text)
	}
	switch {
	case len(out.exprs) == 0:
		return CronSet{}, fmt.Errorf("%w: cron 表达式要填，例如「0 20 * * *」是每天 20:00", ErrInvalid)
	case len(out.exprs) > MaxCronExprs:
		return CronSet{}, fmt.Errorf("%w: cron 表达式最多 %d 条", ErrInvalid, MaxCronExprs)
	}
	out.text = strings.Join(texts, "; ")
	return out, nil
}

func parseCronExpr(src string) (cronExpr, string, error) {
	if m, ok := cronMacros[strings.ToLower(src)]; ok {
		src = m
	} else if strings.HasPrefix(src, "@") {
		return cronExpr{}, "", fmt.Errorf("不认识 %s：简写只有 @hourly、@daily、@weekly、@monthly、@yearly", src)
	}
	parts := strings.Fields(src)
	if len(parts) != 5 {
		return cronExpr{}, "", fmt.Errorf("要写 5 段（分 时 日 月 星期，用空格分开），这里是 %d 段", len(parts))
	}
	var sets [5]uint64
	for i, p := range parts {
		set, err := parseCronField(p, cronFields[i])
		if err != nil {
			return cronExpr{}, "", fmt.Errorf("第 %d 段（%s）%s", i+1, cronFields[i].label, err.Error())
		}
		sets[i] = set
	}
	// 星期的 7 并到 0
	if sets[4]&(1<<7) != 0 {
		sets[4] = sets[4]&^(1<<7) | 1
	}
	expr := cronExpr{
		minute: sets[0], hour: sets[1], dom: sets[2], month: sets[3], dow: sets[4],
		domAny: isStar(parts[2]), dowAny: isStar(parts[4]),
	}
	if !expr.possible() {
		return cronExpr{}, "", fmt.Errorf("「%s」永远不会到点（这几个月里没有这一天）", strings.Join(parts, " "))
	}
	return expr, strings.Join(parts, " "), nil
}

// isStar 报告一段是不是不限定：和 crontab(5) 一样，以 * 开头的（*、*/2）都算，? 也算。日与星期都不是 * 开头时按「满足一个就行」。
func isStar(p string) bool { return strings.HasPrefix(p, "*") || p == "?" }

// parseCronField 解析一段：返回取值的位图。
func parseCronField(src string, f cronField) (uint64, error) {
	var set uint64
	for _, item := range strings.Split(src, ",") {
		if item == "" {
			return 0, fmt.Errorf("有空的一项（多了逗号）")
		}
		rangePart, stepPart, hasStep := strings.Cut(item, "/")
		step := 1
		if hasStep {
			n, err := strconv.Atoi(stepPart)
			if err != nil || n < 1 {
				return 0, fmt.Errorf("「%s」的间隔要是正整数", item)
			}
			step = n
		}
		lo, hi := f.min, f.max
		switch {
		case isStar(rangePart):
		case strings.Contains(rangePart, "-"):
			a, b, _ := strings.Cut(rangePart, "-")
			var err error
			if lo, err = cronValue(a, f); err != nil {
				return 0, err
			}
			if hi, err = cronValue(b, f); err != nil {
				return 0, err
			}
			if lo > hi {
				return 0, fmt.Errorf("「%s」的范围要从小到大", item)
			}
		default:
			v, err := cronValue(rangePart, f)
			if err != nil {
				return 0, err
			}
			lo, hi = v, v
			// a/n 是从 a 开始每隔 n 个直到最大值
			if hasStep {
				hi = f.max
			}
		}
		for v := lo; v <= hi; v += step {
			set |= 1 << uint(v)
		}
	}
	return set, nil
}

// cronValue 解析一个取值：数字或英文缩写。
func cronValue(s string, f cronField) (int, error) {
	if n, err := strconv.Atoi(s); err == nil {
		if n < f.min || n > f.max {
			return 0, fmt.Errorf("「%s」不在 %d 到 %d 之间", s, f.min, f.max)
		}
		return n, nil
	}
	for i, name := range f.names {
		if strings.EqualFold(s, name) {
			return f.min + i, nil
		}
	}
	return 0, fmt.Errorf("「%s」不是有效的%s", s, f.label)
}

// possible 报告这条表达式会不会到点：日限定、星期不限定时，选的月里至少要有一个月有选的日子（2 月 30 日永远不会到）；
// 星期也限定时满足星期就行，总会到点。
func (e cronExpr) possible() bool {
	if e.minute == 0 || e.hour == 0 || e.month == 0 {
		return false
	}
	if e.domAny || !e.dowAny {
		return true
	}
	maxDay := [13]int{0, 31, 29, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	for m := 1; m <= 12; m++ {
		if e.month&(1<<uint(m)) == 0 {
			continue
		}
		if e.dom&((uint64(1)<<uint(maxDay[m]+1))-2) != 0 {
			return true
		}
	}
	return false
}

func (e cronExpr) dayMatches(t time.Time) bool {
	if e.month&(1<<uint(t.Month())) == 0 {
		return false
	}
	dom := e.dom&(1<<uint(t.Day())) != 0
	dow := e.dow&(1<<uint(t.Weekday())) != 0
	switch {
	case e.domAny && e.dowAny:
		return true
	case e.domAny:
		return dow
	case e.dowAny:
		return dom
	default:
		return dom || dow
	}
}

// next 是 after 以后（不含）这条表达式第一个到点的时间。
func (e cronExpr) next(after time.Time, loc *time.Location) (time.Time, bool) {
	local := after.In(loc)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	for d := 0; d <= cronSearchDays; d++ {
		date := day.AddDate(0, 0, d)
		if !e.dayMatches(date) {
			continue
		}
		for h := e.hour; h != 0; h &= h - 1 {
			hour := bits.TrailingZeros64(h)
			for m := e.minute; m != 0; m &= m - 1 {
				minute := bits.TrailingZeros64(m)
				// 夏令时跳过的钟点（例如 02:30 不存在）time.Date 会顺延，顺延后的时间照样算到点
				t := time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, loc)
				if t.After(after) {
					return t, true
				}
			}
		}
	}
	return time.Time{}, false
}

// Next 是 after 以后（不含）几条表达式里最早到点的时间。
func (c CronSet) Next(after time.Time, loc *time.Location) (time.Time, bool) {
	if loc == nil {
		loc = time.Local
	}
	var best time.Time
	found := false
	for _, e := range c.exprs {
		if t, ok := e.next(after, loc); ok && (!found || t.Before(best)) {
			best, found = t, true
		}
	}
	return best, found
}
