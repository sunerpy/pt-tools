package v2

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedClock 让仓库按指定时刻写快照。
func withRepoClock(repo *DBUserInfoRepo, t time.Time) {
	repo.SetClock(func() time.Time { return t }, time.UTC)
}

func day(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.UTC)
	if err != nil {
		panic(err)
	}
	return t
}

// 记录与当天快照在同一个事务里写：快照写失败时记录也回滚。
func TestDBUserInfoRepo_SaveWritesRecordAndSnapshotTogether(t *testing.T) {
	repo := newDBRepo(t)
	withRepoClock(repo, day("2026-10-06 09:00"))
	ctx := context.Background()

	require.NoError(t, repo.Save(ctx, sampleUserInfo("hdsky")))
	snaps, err := repo.ListSnapshots(ctx, "hdsky", "2026-10-01", "2026-10-31")
	require.NoError(t, err)
	require.Len(t, snaps, 1)
	assert.Equal(t, "2026-10-06", snaps[0].Date)
	assert.EqualValues(t, 1000, snaps[0].Uploaded)

	require.NoError(t, repo.db.Exec(`CREATE TRIGGER fail_snapshot BEFORE UPDATE ON user_info_daily_snapshot
		BEGIN SELECT RAISE(ABORT, 'disk I/O error'); END`).Error)
	next := sampleUserInfo("hdsky")
	next.Uploaded = 2000
	require.Error(t, repo.Save(ctx, next))

	got, err := repo.Get(ctx, "hdsky")
	require.NoError(t, err)
	assert.EqualValues(t, 1000, got.Uploaded, "快照写失败时记录也回滚")
}

// 同一天多次获取只留最后一次；跨天各留一行。
func TestDBUserInfoRepo_SnapshotUpsertPerDay(t *testing.T) {
	repo := newDBRepo(t)
	ctx := context.Background()

	info := sampleUserInfo("hdsky")
	withRepoClock(repo, day("2026-10-05 23:59"))
	require.NoError(t, repo.Save(ctx, info))
	info.Uploaded = 1500
	withRepoClock(repo, day("2026-10-06 00:01"))
	require.NoError(t, repo.Save(ctx, info))
	info.Uploaded = 1800
	withRepoClock(repo, day("2026-10-06 20:00"))
	require.NoError(t, repo.Save(ctx, info))

	snaps, err := repo.ListSnapshots(ctx, "hdsky", "2026-10-01", "2026-10-31")
	require.NoError(t, err)
	require.Len(t, snaps, 2)
	assert.Equal(t, "2026-10-05", snaps[0].Date)
	assert.EqualValues(t, 1000, snaps[0].Uploaded)
	assert.Equal(t, "2026-10-06", snaps[1].Date)
	assert.EqualValues(t, 1800, snaps[1].Uploaded, "同一天只留最后一次")
}

// 日期按仓库的时区分：UTC 16:30 在东八区已经是第二天。
func TestDBUserInfoRepo_SnapshotDateUsesLocation(t *testing.T) {
	repo := newDBRepo(t)
	shanghai := time.FixedZone("CST", 8*3600)
	repo.SetClock(func() time.Time { return day("2026-10-05 16:30") }, shanghai)

	require.NoError(t, repo.Save(context.Background(), sampleUserInfo("hdsky")))
	assert.Equal(t, "2026-10-06", repo.Today())
	snaps, err := repo.ListSnapshots(context.Background(), "hdsky", "2026-10-06", "2026-10-06")
	require.NoError(t, err)
	require.Len(t, snaps, 1)
}

func TestDBUserInfoRepo_BaselinesAndPrune(t *testing.T) {
	repo := newDBRepo(t)
	ctx := context.Background()
	for i, d := range []string{"2026-09-01", "2026-09-20", "2026-10-02"} {
		info := sampleUserInfo("hdsky")
		info.Uploaded = int64(1000 * (i + 1))
		withRepoClock(repo, day(d+" 12:00"))
		require.NoError(t, repo.Save(ctx, info))
	}
	withRepoClock(repo, day("2026-09-25 12:00"))
	require.NoError(t, repo.Save(ctx, sampleUserInfo("pter")))

	base, err := repo.SnapshotBaselines(ctx, "2026-10-01")
	require.NoError(t, err)
	assert.Equal(t, "2026-09-20", base["hdsky"].Date, "每站取 before 之前最近的一条")
	assert.Equal(t, "2026-09-25", base["pter"].Date)

	n, err := repo.PruneSnapshots(ctx, "2026-09-21")
	require.NoError(t, err)
	assert.EqualValues(t, 2, n)
	left, err := repo.ListSnapshots(ctx, "", "2026-01-01", "2026-12-31")
	require.NoError(t, err)
	assert.Len(t, left, 2)
}

func snap(site, date string, up, down int64, bonus float64) UserInfoDailySnapshot {
	return UserInfoDailySnapshot{Site: site, Date: date, Uploaded: up, Downloaded: down, Bonus: bonus}
}

// 每日增量：相对上一份快照；中间缺天时 SpanDays > 1（不插值）；数值回退的那一项按 0 计并标记。
func TestBuildDailyPoints(t *testing.T) {
	baseline := snap("hdsky", "2026-10-01", 100, 50, 10)
	points := BuildDailyPoints([]UserInfoDailySnapshot{
		snap("hdsky", "2026-10-02", 150, 60, 15),
		snap("hdsky", "2026-10-05", 300, 60, 5), // 缺 03、04；魔力回退（兑换）
	}, &baseline)

	require.Len(t, points, 2)
	assert.EqualValues(t, 50, points[0].DeltaUploaded)
	assert.EqualValues(t, 10, points[0].DeltaDownloaded)
	assert.InDelta(t, 5, points[0].DeltaBonus, 1e-9)
	assert.Equal(t, 1, points[0].SpanDays)
	assert.False(t, points[0].Negative)

	assert.EqualValues(t, 150, points[1].DeltaUploaded)
	assert.Equal(t, 3, points[1].SpanDays, "缺两天：这一份增量跨三天")
	assert.InDelta(t, 0, points[1].DeltaBonus, 1e-9, "回退按 0 计")
	assert.True(t, points[1].Negative)

	first := BuildDailyPoints([]UserInfoDailySnapshot{snap("hdsky", "2026-10-02", 150, 60, 15)}, nil)
	require.Len(t, first, 1)
	assert.Equal(t, 0, first[0].SpanDays, "没有基线的第一天不算增量")
	assert.Zero(t, first[0].DeltaUploaded)
}

// 区间汇总：基线取区间开始前最近的快照，没有就取区间里最早的一份；回退的项按 0 计、不进合计。
func TestSummarizeDeltas(t *testing.T) {
	baselines := map[string]UserInfoDailySnapshot{
		"hdsky": snap("hdsky", "2026-09-29", 1000, 500, 100),
		"pter":  snap("pter", "2026-09-30", 900, 100, 50),
	}
	inRange := []UserInfoDailySnapshot{
		snap("hdsky", "2026-10-01", 1100, 500, 120),
		snap("hdsky", "2026-10-06", 1600, 700, 130),
		snap("pter", "2026-10-06", 800, 150, 60), // 上传回退（站点数据重置）
		snap("audiences", "2026-10-03", 10, 0, 1),
		snap("audiences", "2026-10-06", 40, 5, 3),
		snap("ubits", "2026-10-06", 70, 7, 7), // 没有基线、区间里只有一份
	}

	sum := SummarizeDeltas("7d", "2026-09-30", "2026-10-06", baselines, inRange)

	bySite := map[string]SiteDelta{}
	for _, d := range sum.Sites {
		bySite[d.Site] = d
	}
	require.Len(t, bySite, 4)
	assert.EqualValues(t, 600, bySite["hdsky"].Uploaded)
	assert.EqualValues(t, 200, bySite["hdsky"].Downloaded)
	assert.Equal(t, "2026-09-29", bySite["hdsky"].From)
	assert.Equal(t, "2026-10-06", bySite["hdsky"].To)

	assert.Zero(t, bySite["pter"].Uploaded, "回退按 0 计")
	assert.True(t, bySite["pter"].Negative)
	assert.EqualValues(t, 50, bySite["pter"].Downloaded, "其他项照常计")

	assert.EqualValues(t, 30, bySite["audiences"].Uploaded, "没有基线时取区间里最早的一份")
	assert.True(t, bySite["audiences"].HasBaseline)

	assert.False(t, bySite["ubits"].HasBaseline)
	assert.Zero(t, bySite["ubits"].Uploaded)

	assert.EqualValues(t, 600+0+30, sum.TotalUploaded)
	assert.EqualValues(t, 200+50+5, sum.TotalDownloaded)
	assert.InDelta(t, 30+10+2, sum.TotalBonus, 1e-9)
}

func TestRangeBounds(t *testing.T) {
	from, to, err := RangeBounds("today", "2026-10-06")
	require.NoError(t, err)
	assert.Equal(t, [2]string{"2026-10-06", "2026-10-06"}, [2]string{from, to})
	from, _, err = RangeBounds("7d", "2026-10-06")
	require.NoError(t, err)
	assert.Equal(t, "2026-09-30", from)
	from, _, err = RangeBounds("30d", "2026-10-06")
	require.NoError(t, err)
	assert.Equal(t, "2026-09-07", from)
	_, _, err = RangeBounds("1y", "2026-10-06")
	require.Error(t, err)
}
