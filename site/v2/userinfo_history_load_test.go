package v2

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubHistoryRepo 是按字段回结果的历史仓库，用来走 LoadDeltaSummary / LoadTrends 的错误分支。
type stubHistoryRepo struct {
	today       string
	baselines   map[string]UserInfoDailySnapshot
	snaps       []UserInfoDailySnapshot
	baselineErr error
	listErr     error
}

func (s stubHistoryRepo) ListSnapshots(_ context.Context, _, _, _ string) ([]UserInfoDailySnapshot, error) {
	return s.snaps, s.listErr
}

func (s stubHistoryRepo) SnapshotBaselines(context.Context, string) (map[string]UserInfoDailySnapshot, error) {
	return s.baselines, s.baselineErr
}

func (s stubHistoryRepo) PruneSnapshots(context.Context, string) (int64, error) { return 0, nil }

func (s stubHistoryRepo) Today() string { return s.today }

func TestLoadDeltaSummary(t *testing.T) {
	ctx := context.Background()
	repo := stubHistoryRepo{
		today:     "2026-10-06",
		baselines: map[string]UserInfoDailySnapshot{"hdsky": snap("hdsky", "2026-09-29", 100, 10, 1)},
		snaps:     []UserInfoDailySnapshot{snap("hdsky", "2026-10-06", 300, 30, 5)},
	}
	sum, err := LoadDeltaSummary(ctx, repo, "7d")
	require.NoError(t, err)
	assert.Equal(t, "2026-09-30", sum.From)
	assert.EqualValues(t, 200, sum.TotalUploaded)

	_, err = LoadDeltaSummary(ctx, repo, "1y")
	require.Error(t, err, "不支持的区间")

	bad := repo
	bad.today = "not-a-date"
	_, err = LoadDeltaSummary(ctx, bad, "7d")
	require.Error(t, err, "仓库日期坏了")

	boom := errors.New("boom")
	bad = repo
	bad.baselineErr = boom
	_, err = LoadDeltaSummary(ctx, bad, "7d")
	require.ErrorIs(t, err, boom)
	bad = repo
	bad.listErr = boom
	_, err = LoadDeltaSummary(ctx, bad, "7d")
	require.ErrorIs(t, err, boom)
}

func TestLoadTrends(t *testing.T) {
	ctx := context.Background()
	repo := stubHistoryRepo{
		today:     "2026-10-06",
		baselines: map[string]UserInfoDailySnapshot{"hdsky": snap("hdsky", "2026-10-03", 100, 10, 1)},
		snaps: []UserInfoDailySnapshot{
			snap("hdsky", "2026-10-05", 150, 10, 2),
			snap("hdsky", "2026-10-06", 250, 20, 3),
		},
	}
	dates, totals, sites, err := LoadTrends(ctx, repo, 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-10-04", "2026-10-05", "2026-10-06"}, dates)
	require.Len(t, totals, 3)
	assert.EqualValues(t, 0, totals[0].Uploaded)
	assert.EqualValues(t, 50, totals[1].Uploaded, "跨了两天的增量记在快照那天")
	assert.EqualValues(t, 100, sites["hdsky"][2].Uploaded)

	bad := repo
	bad.today = "x"
	_, _, _, err = LoadTrends(ctx, bad, 3)
	require.Error(t, err)

	boom := errors.New("boom")
	bad = repo
	bad.baselineErr = boom
	_, _, _, err = LoadTrends(ctx, bad, 3)
	require.ErrorIs(t, err, boom)
	bad = repo
	bad.listErr = boom
	_, _, _, err = LoadTrends(ctx, bad, 3)
	require.ErrorIs(t, err, boom)
}

func TestDeltaSummaryFilter(t *testing.T) {
	sum := DeltaSummary{Range: "7d", Sites: []SiteDelta{
		{Site: "HDSky", Uploaded: 10, Downloaded: 1, Bonus: 2, HasBaseline: true},
		{Site: "pter", Uploaded: 5, Downloaded: 1, Bonus: 1, HasBaseline: true},
	}, TotalUploaded: 15, TotalDownloaded: 2, TotalBonus: 3}
	assert.Equal(t, sum, sum.Filter(nil), "nil 表示不过滤")
	got := sum.Filter(map[string]bool{"hdsky": true})
	require.Len(t, got.Sites, 1)
	assert.Equal(t, "HDSky", got.Sites[0].Site, "按小写站点名匹配")
	assert.EqualValues(t, 10, got.TotalUploaded)
	assert.EqualValues(t, 1, got.TotalDownloaded)
	assert.InDelta(t, 2, got.TotalBonus, 1e-9)
}

func TestHistoryDateHelpers(t *testing.T) {
	_, err := AddDays("2026/10/06", 1)
	require.Error(t, err)
	d, err := AddDays("2026-10-06", -6)
	require.NoError(t, err)
	assert.Equal(t, "2026-09-30", d)
	assert.Equal(t, 0, daysBetween("bad", "2026-10-06"), "日期坏了按 0 天")
	assert.Equal(t, 3, daysBetween("2026-10-03", "2026-10-06"))
}

func TestUserInfoServiceHistory(t *testing.T) {
	var nilSvc *UserInfoService
	_, ok := nilSvc.History()
	assert.False(t, ok, "nil 服务没有历史")

	mem := NewUserInfoService(UserInfoServiceConfig{Repo: NewInMemoryUserInfoRepo(), CacheTTL: time.Minute})
	_, ok = mem.History()
	assert.False(t, ok, "内存仓库没有每日快照")

	db := NewUserInfoService(UserInfoServiceConfig{Repo: newDBRepo(t), CacheTTL: time.Minute})
	h, ok := db.History()
	assert.True(t, ok)
	assert.NotNil(t, h)
}
