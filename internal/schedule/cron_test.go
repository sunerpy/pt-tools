package schedule

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCronNext(t *testing.T) {
	sh := mustLoc(t, "Asia/Shanghai")
	at := func(y int, m time.Month, d, hh, mm int) time.Time { return time.Date(y, m, d, hh, mm, 0, 0, sh) }
	cases := []struct {
		name  string
		expr  string
		after time.Time
		want  time.Time
	}{
		{"每天 20:00", "0 20 * * *", at(2026, 10, 21, 9, 0), at(2026, 10, 21, 20, 0)},
		{"正好在点上不算", "0 20 * * *", at(2026, 10, 21, 20, 0), at(2026, 10, 22, 20, 0)},
		{"秒数也不算到点", "0 20 * * *", at(2026, 10, 21, 20, 0).Add(30 * time.Second), at(2026, 10, 22, 20, 0)},
		{"每 2 小时", "0 */2 * * *", at(2026, 10, 21, 9, 5), at(2026, 10, 21, 10, 0)},
		{"工作日 8:30", "30 8 * * 1-5", at(2026, 10, 23, 9, 0), at(2026, 10, 26, 8, 30)}, // 10-23 是星期五
		{"英文缩写", "0 22 * * FRI,sat", at(2026, 10, 21, 0, 0), at(2026, 10, 23, 22, 0)},
		{"星期 7 是星期日", "0 9 * * 7", at(2026, 10, 21, 0, 0), at(2026, 10, 25, 9, 0)},
		{"每月 1 号与 15 号", "0 9 1,15 * *", at(2026, 10, 21, 0, 0), at(2026, 11, 1, 9, 0)},
		{"月份缩写与范围", "0 0 1 jan-mar *", at(2026, 10, 21, 0, 0), at(2027, 1, 1, 0, 0)},
		{"日与星期都限定：满足一个就行", "0 12 13 * 5", at(2026, 10, 21, 0, 0), at(2026, 10, 23, 12, 0)},
		{"日 */2 算不限定：日与星期都要满足", "0 12 */2 * 5", at(2026, 10, 21, 0, 0), at(2026, 10, 23, 12, 0)},
		{"闰年 2 月 29 日", "0 0 29 2 *", at(2026, 10, 21, 0, 0), at(2028, 2, 29, 0, 0)},
		{"a/n 从 a 起", "5/20 10 * * *", at(2026, 10, 21, 10, 6), at(2026, 10, 21, 10, 25)},
		{"简写 @daily", "@daily", at(2026, 10, 21, 9, 0), at(2026, 10, 22, 0, 0)},
		{"简写 @weekly 是星期日零点", "@weekly", at(2026, 10, 21, 9, 0), at(2026, 10, 25, 0, 0)},
		{"几条取最早的", "0 22 * * 5; 0 9,13 * * 6", at(2026, 10, 23, 23, 0), at(2026, 10, 24, 9, 0)},
		{"换行分开也行", "0 22 * * 5\n0 9,13 * * 6\n", at(2026, 10, 24, 9, 0), at(2026, 10, 24, 13, 0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := Next(Spec{Kind: Cron, Expr: c.expr}, c.after, sh)
			require.True(t, ok)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestCronInvalid(t *testing.T) {
	for expr, want := range map[string]string{
		"":                       "要填",
		"0 20 * *":               "5 段",
		"0 20 * * * *":           "这里是 6 段",
		"60 20 * * *":            "第 1 段（分钟）",
		"0 24 * * *":             "第 2 段（小时）",
		"0 0 0 * *":              "第 3 段（日）",
		"0 0 1 13 *":             "第 4 段（月）",
		"0 0 * * 8":              "第 5 段（星期）",
		"0 0 * * FOO":            "不是有效的星期",
		"0 0 5-1 * *":            "从小到大",
		"*/0 * * * *":            "间隔要是正整数",
		"0,,1 * * * *":           "多了逗号",
		"0 0 30 2 *":             "永远不会到点",
		"@every 5m":              "不认识 @every",
		"0 20 * * *; 0 25 * * *": "「0 25 * * *」第 2 段（小时）",
	} {
		_, err := ParseCron(expr)
		require.Error(t, err, expr)
		assert.ErrorIs(t, err, ErrInvalid, expr)
		assert.Contains(t, err.Error(), want, expr)
	}
	_, err := ParseCron(strings.Repeat("0 0 * * *;", MaxCronExprs+1))
	assert.Contains(t, err.Error(), "最多")
	// 星期限定时日写 31 也会到点（满足星期就行）
	_, err = ParseCron("0 0 31 2 1")
	assert.NoError(t, err)
}

func TestCronNormalizedText(t *testing.T) {
	set, err := ParseCron("  0   20 * *  FRI ;\n@daily  ")
	require.NoError(t, err)
	assert.Equal(t, "0 20 * * FRI; 0 0 * * *", set.String())
}

// 和逐分钟往后数的笨办法对照：随机表达式、随机起点，两边要一样
func TestCronNextMatchesBruteForce(t *testing.T) {
	sh := mustLoc(t, "Asia/Shanghai")
	r := rand.New(rand.NewPCG(1, 2))
	field := func(lo, hi int) string {
		switch r.IntN(5) {
		case 0:
			return "*"
		case 1:
			return "*/" + strconv.Itoa(1+r.IntN(4))
		case 2:
			a := lo + r.IntN(hi-lo+1)
			b := a + r.IntN(hi-a+1)
			return strconv.Itoa(a) + "-" + strconv.Itoa(b)
		case 3:
			return strconv.Itoa(lo+r.IntN(hi-lo+1)) + "," + strconv.Itoa(lo+r.IntN(hi-lo+1))
		default:
			return strconv.Itoa(lo + r.IntN(hi-lo+1))
		}
	}
	for i := 0; i < 300; i++ {
		expr := strings.Join([]string{field(0, 59), field(0, 23), field(1, 28), field(1, 12), field(0, 6)}, " ")
		set, err := ParseCron(expr)
		require.NoError(t, err, expr)
		after := time.Date(2026, time.Month(1+r.IntN(12)), 1+r.IntN(28), r.IntN(24), r.IntN(60), r.IntN(60), 0, sh)
		got, ok := set.Next(after, sh)
		require.True(t, ok, expr)
		e := set.exprs[0]
		want := after.Truncate(time.Minute).Add(time.Minute)
		for !e.dayMatches(want) || e.hour&(1<<uint(want.Hour())) == 0 || e.minute&(1<<uint(want.Minute())) == 0 {
			want = want.Add(time.Minute)
		}
		assert.Equal(t, want, got, "%s after %s", expr, after)
	}
}

// 夏令时跳过的钟点顺延到跳过以后
func TestCronDST(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	got, ok := Next(Spec{Kind: Cron, Expr: "30 2 * * *"}, time.Date(2027, 3, 14, 0, 0, 0, 0, ny), ny)
	require.True(t, ok)
	assert.True(t, got.After(time.Date(2027, 3, 14, 0, 0, 0, 0, ny)))
	assert.Equal(t, 14, got.Day())
}
