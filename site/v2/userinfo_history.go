package v2

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// UserInfoDailySnapshot 是每站每天一行的用户数据快照，用来算每日增量与趋势。
//
// 日期按仓库的时区（默认进程时区，Docker 里即 TZ）取 YYYY-MM-DD。只由 DBUserInfoRepo.Save 写：
// 与 UserInfoRecord 在同一个事务里 upsert，同一天多次获取只留最后一次。
type UserInfoDailySnapshot struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Site       string    `gorm:"size:64;not null;uniqueIndex:idx_userinfo_snapshot_site_date,priority:1" json:"site"`
	Date       string    `gorm:"size:10;not null;uniqueIndex:idx_userinfo_snapshot_site_date,priority:2;index" json:"date"`
	Uploaded   int64     `json:"uploaded"`
	Downloaded int64     `json:"downloaded"`
	Bonus      float64   `json:"bonus"`
	Ratio      float64   `json:"ratio"`
	Seeding    int       `json:"seeding"`
	SeederSize int64     `json:"seederSize"`
	CapturedAt int64     `json:"capturedAt"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// TableName returns the table name for UserInfoDailySnapshot
func (UserInfoDailySnapshot) TableName() string {
	return "user_info_daily_snapshot"
}

// SnapshotRetentionDays 是每日快照的保留天数，更早的由定期清理删除。
const SnapshotRetentionDays = 400

// UserInfoHistoryRepo 是带每日快照的仓库。DBUserInfoRepo 实现它；内存仓库不实现，没有历史数据。
type UserInfoHistoryRepo interface {
	// ListSnapshots 返回 [from, to]（含两端）之间的快照，按站点、日期升序；site 为空时返回所有站点。
	ListSnapshots(ctx context.Context, site, from, to string) ([]UserInfoDailySnapshot, error)
	// SnapshotBaselines 返回每站在 before 之前（不含）最近的一份快照，作为区间增量的基线。
	SnapshotBaselines(ctx context.Context, before string) (map[string]UserInfoDailySnapshot, error)
	// PruneSnapshots 删除日期早于 before 的快照，返回删除的行数。
	PruneSnapshots(ctx context.Context, before string) (int64, error)
	// Today 是仓库时区的当天日期（YYYY-MM-DD）。
	Today() string
}

const dateLayout = "2006-01-02"

// AddDays 在 YYYY-MM-DD 日期上加减天数。
func AddDays(date string, n int) (string, error) {
	t, err := time.Parse(dateLayout, date)
	if err != nil {
		return "", fmt.Errorf("日期格式应为 YYYY-MM-DD: %w", err)
	}
	return t.AddDate(0, 0, n).Format(dateLayout), nil
}

func daysBetween(from, to string) int {
	a, errA := time.Parse(dateLayout, from)
	b, errB := time.Parse(dateLayout, to)
	if errA != nil || errB != nil {
		return 0
	}
	return int(b.Sub(a).Hours() / 24)
}

// RangeBounds 把 today / 7d / 30d 换成含两端的日期区间。
func RangeBounds(name, today string) (from, to string, err error) {
	days := map[string]int{"today": 1, "7d": 7, "30d": 30}[name]
	if days == 0 {
		return "", "", fmt.Errorf("不支持的区间 %q，可选 today、7d、30d", name)
	}
	from, err = AddDays(today, -(days - 1))
	if err != nil {
		return "", "", err
	}
	return from, today, nil
}

// DailyPoint 是某站某天的数据与相对上一份快照的增量。
type DailyPoint struct {
	Date            string  `json:"date"`
	Uploaded        int64   `json:"uploaded"`
	Downloaded      int64   `json:"downloaded"`
	Bonus           float64 `json:"bonus"`
	Seeding         int     `json:"seeding"`
	DeltaUploaded   int64   `json:"deltaUploaded"`
	DeltaDownloaded int64   `json:"deltaDownloaded"`
	DeltaBonus      float64 `json:"deltaBonus"`
	// SpanDays 是这份增量覆盖的天数：上一份快照不是前一天时大于 1（中间缺天，不插值）；
	// 没有更早的快照可比时为 0，增量记 0。
	SpanDays int `json:"spanDays"`
	// Negative 表示有数值比上一份快照小（站点数据回退、魔力兑换）：那一项增量按 0 计。
	Negative bool `json:"negative"`
}

func delta64(cur, prev int64) (int64, bool) {
	if cur < prev {
		return 0, true
	}
	return cur - prev, false
}

func deltaFloat(cur, prev float64) (float64, bool) {
	if cur < prev {
		return 0, true
	}
	return cur - prev, false
}

// BuildDailyPoints 按日期升序给出每天相对上一份快照的增量。snaps 须是同一站点；
// baseline 是区间开始前最近的一份快照，没有时第一天不算增量。
func BuildDailyPoints(snaps []UserInfoDailySnapshot, baseline *UserInfoDailySnapshot) []DailyPoint {
	sorted := append([]UserInfoDailySnapshot(nil), snaps...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date < sorted[j].Date })
	points := make([]DailyPoint, 0, len(sorted))
	prev := baseline
	for i := range sorted {
		s := sorted[i]
		p := DailyPoint{Date: s.Date, Uploaded: s.Uploaded, Downloaded: s.Downloaded, Bonus: s.Bonus, Seeding: s.Seeding}
		if prev != nil {
			var n1, n2, n3 bool
			p.DeltaUploaded, n1 = delta64(s.Uploaded, prev.Uploaded)
			p.DeltaDownloaded, n2 = delta64(s.Downloaded, prev.Downloaded)
			p.DeltaBonus, n3 = deltaFloat(s.Bonus, prev.Bonus)
			p.Negative = n1 || n2 || n3
			p.SpanDays = daysBetween(prev.Date, s.Date)
		}
		points = append(points, p)
		prev = &sorted[i]
	}
	return points
}

// SiteDelta 是某站在区间内的增量。
type SiteDelta struct {
	Site string `json:"site"`
	// From 是基线快照的日期，To 是区间内最新一份快照的日期。
	From       string  `json:"from"`
	To         string  `json:"to"`
	Uploaded   int64   `json:"uploaded"`
	Downloaded int64   `json:"downloaded"`
	Bonus      float64 `json:"bonus"`
	// HasBaseline 为 false 表示区间里只有一份快照、之前也没有，增量记 0。
	HasBaseline bool `json:"hasBaseline"`
	// Negative 表示有数值回退，那一项按 0 计，不进合计。
	Negative bool `json:"negative"`
}

// DeltaSummary 是一个区间内各站的增量与合计。
type DeltaSummary struct {
	Range           string      `json:"range"`
	From            string      `json:"from"`
	To              string      `json:"to"`
	Sites           []SiteDelta `json:"sites"`
	TotalUploaded   int64       `json:"totalUploaded"`
	TotalDownloaded int64       `json:"totalDownloaded"`
	TotalBonus      float64     `json:"totalBonus"`
}

// SummarizeDeltas 计算区间 [from, to] 内各站的增量：最新一份快照减基线。基线取区间开始前最近的
// 一份快照（baselines），没有就取区间里最早的一份。区间里没有快照的站点不出现。
func SummarizeDeltas(rangeName, from, to string, baselines map[string]UserInfoDailySnapshot, inRange []UserInfoDailySnapshot) DeltaSummary {
	bySite := map[string][]UserInfoDailySnapshot{}
	for _, s := range inRange {
		if s.Date < from || s.Date > to {
			continue
		}
		bySite[s.Site] = append(bySite[s.Site], s)
	}
	sites := make([]string, 0, len(bySite))
	for site := range bySite {
		sites = append(sites, site)
	}
	sort.Strings(sites)

	sum := DeltaSummary{Range: rangeName, From: from, To: to, Sites: make([]SiteDelta, 0, len(sites))}
	for _, site := range sites {
		list := bySite[site]
		sort.Slice(list, func(i, j int) bool { return list[i].Date < list[j].Date })
		latest := list[len(list)-1]
		d := SiteDelta{Site: site, To: latest.Date}
		base, ok := baselines[site]
		if !ok && len(list) > 1 {
			base, ok = list[0], true
		}
		if ok {
			var n1, n2, n3 bool
			d.HasBaseline = true
			d.From = base.Date
			d.Uploaded, n1 = delta64(latest.Uploaded, base.Uploaded)
			d.Downloaded, n2 = delta64(latest.Downloaded, base.Downloaded)
			d.Bonus, n3 = deltaFloat(latest.Bonus, base.Bonus)
			d.Negative = n1 || n2 || n3
		} else {
			d.From = latest.Date
		}
		sum.TotalUploaded += d.Uploaded
		sum.TotalDownloaded += d.Downloaded
		sum.TotalBonus += d.Bonus
		sum.Sites = append(sum.Sites, d)
	}
	return sum
}

// LoadDeltaSummary 从仓库读出区间 range 的增量汇总。
func LoadDeltaSummary(ctx context.Context, repo UserInfoHistoryRepo, rangeName string) (DeltaSummary, error) {
	from, to, err := RangeBounds(rangeName, repo.Today())
	if err != nil {
		return DeltaSummary{}, err
	}
	baselines, err := repo.SnapshotBaselines(ctx, from)
	if err != nil {
		return DeltaSummary{}, err
	}
	inRange, err := repo.ListSnapshots(ctx, "", from, to)
	if err != nil {
		return DeltaSummary{}, err
	}
	return SummarizeDeltas(rangeName, from, to, baselines, inRange), nil
}

// TrendPoint 是某天的增量。
type TrendPoint struct {
	Date       string  `json:"date"`
	Uploaded   int64   `json:"uploaded"`
	Downloaded int64   `json:"downloaded"`
	Bonus      float64 `json:"bonus"`
}

// BuildTrends 按 dates（升序、连续的日期）给出每站每天的增量与各站合计。没有快照的那天记 0；
// 一份快照跨了好几天时，增量记在快照所在的那天。数值回退的项按 0 计。
func BuildTrends(dates []string, baselines map[string]UserInfoDailySnapshot, snaps []UserInfoDailySnapshot) ([]TrendPoint, map[string][]TrendPoint) {
	index := make(map[string]int, len(dates))
	totals := make([]TrendPoint, len(dates))
	for i, d := range dates {
		index[d] = i
		totals[i].Date = d
	}
	bySite := map[string][]UserInfoDailySnapshot{}
	for _, s := range snaps {
		bySite[s.Site] = append(bySite[s.Site], s)
	}
	sites := make(map[string][]TrendPoint, len(bySite))
	for site, list := range bySite {
		series := make([]TrendPoint, len(dates))
		for i, d := range dates {
			series[i].Date = d
		}
		var baseline *UserInfoDailySnapshot
		if b, ok := baselines[site]; ok {
			baseline = &b
		}
		for _, p := range BuildDailyPoints(list, baseline) {
			i, ok := index[p.Date]
			if !ok || p.SpanDays == 0 {
				continue
			}
			series[i] = TrendPoint{Date: p.Date, Uploaded: p.DeltaUploaded, Downloaded: p.DeltaDownloaded, Bonus: p.DeltaBonus}
			totals[i].Uploaded += p.DeltaUploaded
			totals[i].Downloaded += p.DeltaDownloaded
			totals[i].Bonus += p.DeltaBonus
		}
		sites[site] = series
	}
	return totals, sites
}

// LoadTrends 从仓库读出最近 days 天（含今天）的走势。
func LoadTrends(ctx context.Context, repo UserInfoHistoryRepo, days int) ([]string, []TrendPoint, map[string][]TrendPoint, error) {
	to := repo.Today()
	from, err := AddDays(to, -(days - 1))
	if err != nil {
		return nil, nil, nil, err
	}
	dates := make([]string, 0, days)
	for i := 0; i < days; i++ {
		d, derr := AddDays(from, i)
		if derr != nil {
			return nil, nil, nil, derr
		}
		dates = append(dates, d)
	}
	baselines, err := repo.SnapshotBaselines(ctx, from)
	if err != nil {
		return nil, nil, nil, err
	}
	snaps, err := repo.ListSnapshots(ctx, "", from, to)
	if err != nil {
		return nil, nil, nil, err
	}
	totals, sites := BuildTrends(dates, baselines, snaps)
	return dates, totals, sites, nil
}
