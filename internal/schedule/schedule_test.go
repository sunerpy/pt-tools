package schedule

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	require.NoError(t, err)
	return loc
}

func TestNext(t *testing.T) {
	sh := mustLoc(t, "Asia/Shanghai")
	at := func(y int, m time.Month, d, hh, mm int) time.Time { return time.Date(y, m, d, hh, mm, 0, 0, sh) }
	cases := []struct {
		name  string
		spec  Spec
		after time.Time
		want  time.Time
	}{
		{"每天：今天还没到点", Spec{Kind: Daily, At: "20:30"}, at(2026, 10, 21, 9, 0), at(2026, 10, 21, 20, 30)},
		{"每天：今天过了点就是明天", Spec{Kind: Daily, At: "20:30"}, at(2026, 10, 21, 20, 30), at(2026, 10, 22, 20, 30)},
		{"每隔 3 天：按建立日对齐", Spec{Kind: EveryDays, At: "08:00", Every: 3, Anchor: at(2026, 10, 1, 15, 0)}, at(2026, 10, 21, 9, 0), at(2026, 10, 22, 8, 0)},
		{"每隔 3 天：正好是那天还没到点", Spec{Kind: EveryDays, At: "08:00", Every: 3, Anchor: at(2026, 10, 1, 15, 0)}, at(2026, 10, 19, 7, 0), at(2026, 10, 19, 8, 0)},
		{"每隔 3 天：起点在以后", Spec{Kind: EveryDays, At: "08:00", Every: 3, Anchor: at(2026, 11, 5, 0, 0)}, at(2026, 10, 21, 9, 0), at(2026, 11, 5, 8, 0)},
		{"每周：几天里最近的", Spec{Kind: Weekly, At: "21:00", Days: []int{0, 3}}, at(2026, 10, 21, 22, 0), at(2026, 10, 25, 21, 0)}, // 10-21 是星期三
		{"每周：当天还没到点", Spec{Kind: Weekly, At: "21:00", Days: []int{3}}, at(2026, 10, 21, 20, 0), at(2026, 10, 21, 21, 0)},
		{"每周：当天过了点就是下周", Spec{Kind: Weekly, At: "21:00", Days: []int{3}}, at(2026, 10, 21, 21, 0), at(2026, 10, 28, 21, 0)},
		{"每月：短月按月末", Spec{Kind: Monthly, At: "10:00", Days: []int{31}}, at(2027, 2, 1, 0, 0), at(2027, 2, 28, 10, 0)},
		{"每月：闰年二月", Spec{Kind: Monthly, At: "10:00", Days: []int{30}}, at(2028, 2, 1, 0, 0), at(2028, 2, 29, 10, 0)},
		{"每月：这个月的都过了是下个月", Spec{Kind: Monthly, At: "10:00", Days: []int{15, 1}}, at(2026, 10, 21, 0, 0), at(2026, 11, 1, 10, 0)},
		{"一次", Spec{Kind: Once, When: at(2026, 12, 1, 9, 0)}, at(2026, 10, 21, 0, 0), at(2026, 12, 1, 9, 0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := Next(c.spec, c.after, sh)
			require.True(t, ok)
			assert.True(t, got.Equal(c.want), "got %s want %s", got, c.want)
		})
	}
}

func TestNextKeepsWallClockAcrossDST(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	// 2026-03-08 美东开始夏令时：前一天 20:00 以后，下一次还是 20:00（墙上时间）
	after := time.Date(2026, 3, 7, 21, 0, 0, 0, ny)
	got, ok := Next(Spec{Kind: Daily, At: "20:00"}, after, ny)
	require.True(t, ok)
	assert.Equal(t, time.Date(2026, 3, 8, 20, 0, 0, 0, ny), got)
	assert.Equal(t, 20, got.In(ny).Hour())
}

func TestNextRejects(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	for _, s := range []Spec{
		{Kind: Daily, At: "25:00"},
		{Kind: Daily, At: "8点"},
		{Kind: EveryDays, At: "08:00", Every: 0},
		{Kind: EveryDays, At: "08:00", Every: 31},
		{Kind: Weekly, At: "08:00"},
		{Kind: Weekly, At: "08:00", Days: []int{7}},
		{Kind: Monthly, At: "08:00", Days: []int{0}},
		{Kind: "hourly", At: "08:00"},
		{Kind: Once},
	} {
		_, ok := Next(s, now, time.UTC)
		assert.False(t, ok, "%+v", s)
		assert.ErrorIs(t, s.Validate(), ErrInvalid, "%+v", s)
	}
	_, ok := Next(Spec{Kind: Once, When: now.Add(-time.Hour)}, now, time.UTC)
	assert.False(t, ok, "一次的时间过了")
}

func TestClock(t *testing.T) {
	h, m, err := Clock("8:05")
	require.NoError(t, err)
	assert.Equal(t, [2]int{8, 5}, [2]int{h, m})
	_, _, err = Clock("08:5")
	assert.ErrorIs(t, err, ErrInvalid)
}
