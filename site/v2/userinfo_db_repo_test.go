package v2

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newDBRepo(t *testing.T) *DBUserInfoRepo {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	repo, err := NewDBUserInfoRepo(db)
	require.NoError(t, err)
	return repo
}

func sampleUserInfo(site string) UserInfo {
	return UserInfo{
		Site:               site,
		Username:           "user_" + site,
		UserID:             "42",
		Rank:               "Power User",
		Uploaded:           1000,
		Downloaded:         500,
		Ratio:              2.0,
		Seeding:            10,
		Leeching:           1,
		Bonus:              5000,
		JoinDate:           1600000000,
		LastAccess:         1700000000,
		LevelName:          "PU",
		LevelID:            3,
		BonusPerHour:       100,
		SeedingBonus:       200,
		UnreadMessageCount: 2,
		SeederCount:        10,
		SeederSize:         1024,
		LeecherCount:       1,
		LeecherSize:        512,
		Uploads:            5,
	}
}

func TestUserInfoRecord_TableName(t *testing.T) {
	assert.Equal(t, "user_info", UserInfoRecord{}.TableName())
}

func TestUserInfoRecord_RoundTrip(t *testing.T) {
	info := sampleUserInfo("hdsky")
	record := FromUserInfo(info)
	back := record.ToUserInfo()
	assert.Equal(t, info.Site, back.Site)
	assert.Equal(t, info.Username, back.Username)
	assert.Equal(t, info.Uploaded, back.Uploaded)
	assert.Equal(t, info.Ratio, back.Ratio)
	assert.Equal(t, info.BonusPerHour, back.BonusPerHour)
	assert.Equal(t, info.SeederSize, back.SeederSize)
	assert.Equal(t, info.Uploads, back.Uploads)
}

func TestDBUserInfoRepo_SaveAndGet(t *testing.T) {
	repo := newDBRepo(t)
	ctx := context.Background()

	// Empty site rejected
	assert.ErrorIs(t, repo.Save(ctx, UserInfo{}), ErrSiteNotFound)

	require.NoError(t, repo.Save(ctx, sampleUserInfo("hdsky")))

	got, err := repo.Get(ctx, "hdsky")
	require.NoError(t, err)
	assert.Equal(t, "user_hdsky", got.Username)
	assert.Greater(t, got.LastUpdate, int64(0))

	// Missing site
	_, err = repo.Get(ctx, "notexist")
	assert.ErrorIs(t, err, ErrSiteNotFound)
}

func TestDBUserInfoRepo_Upsert(t *testing.T) {
	repo := newDBRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Save(ctx, sampleUserInfo("hdsky")))
	updated := sampleUserInfo("hdsky")
	updated.Uploaded = 9999
	require.NoError(t, repo.Save(ctx, updated))

	got, err := repo.Get(ctx, "hdsky")
	require.NoError(t, err)
	assert.Equal(t, int64(9999), got.Uploaded)

	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestDBUserInfoRepo_ListAllAndBySites(t *testing.T) {
	repo := newDBRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Save(ctx, sampleUserInfo("hdsky")))
	require.NoError(t, repo.Save(ctx, sampleUserInfo("mteam")))
	require.NoError(t, repo.Save(ctx, sampleUserInfo("ourbits")))

	all, err := repo.ListAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	subset, err := repo.ListBySites(ctx, []string{"hdsky", "mteam"})
	require.NoError(t, err)
	assert.Len(t, subset, 2)
}

func TestDBUserInfoRepo_Delete(t *testing.T) {
	repo := newDBRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Save(ctx, sampleUserInfo("hdsky")))
	require.NoError(t, repo.Delete(ctx, "hdsky"))

	// Delete missing
	assert.ErrorIs(t, repo.Delete(ctx, "hdsky"), ErrSiteNotFound)
}

func TestDBUserInfoRepo_DeleteAll(t *testing.T) {
	repo := newDBRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Save(ctx, sampleUserInfo("a")))
	require.NoError(t, repo.Save(ctx, sampleUserInfo("b")))
	require.NoError(t, repo.DeleteAll(ctx))

	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestDBUserInfoRepo_GetAggregated(t *testing.T) {
	repo := newDBRepo(t)
	ctx := context.Background()

	a := sampleUserInfo("a")
	a.Uploaded, a.Downloaded, a.Ratio = 1000, 500, 2.0
	b := sampleUserInfo("b")
	b.Uploaded, b.Downloaded, b.Ratio = 3000, 1000, 3.0
	// Invalid ratio should be excluded from average
	c := sampleUserInfo("c")
	c.Ratio = 5000

	require.NoError(t, repo.Save(ctx, a))
	require.NoError(t, repo.Save(ctx, b))
	require.NoError(t, repo.Save(ctx, c))

	stats, err := repo.GetAggregated(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, stats.SiteCount)
	// a=1000, b=3000, c=1000 (sample default) => 5000
	assert.Equal(t, int64(5000), stats.TotalUploaded)
	// average ratio only over a and b => (2+3)/2 = 2.5
	assert.InDelta(t, 2.5, stats.AverageRatio, 0.001)
	assert.Greater(t, stats.TotalBonus, float64(0))
}

func TestDBUserInfoRepo_GetAggregated_Empty(t *testing.T) {
	repo := newDBRepo(t)
	stats, err := repo.GetAggregated(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, stats.SiteCount)
	assert.Equal(t, float64(0), stats.AverageRatio)
}

func TestNewDBUserInfoRepo_Success(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	repo, err := NewDBUserInfoRepo(db)
	require.NoError(t, err)
	require.NotNil(t, repo)
}

// ---------------------------------------------------------------------------
// mtorrent_driver.go — executeDirectly form-urlencoded body + string body
// ---------------------------------------------------------------------------

// 会合法变成 0 的计数（做种、下载中、两种体积、未读消息、H&R、时魔）这次取到 0 时要存成 0：
// 原来按结构体更新，GORM 跳过零值，记录一直停在上一次的非零值（M-Team 记录里下载中 149，站点上是 1）。
// 别的字段这次是零值时照旧不动（比如加入时间）。
func TestDBUserInfoRepo_SaveWritesZeroCounters(t *testing.T) {
	repo := newDBRepo(t)
	ctx := context.Background()
	first := sampleUserInfo("mteam")
	first.Seeding, first.SeederCount, first.SeederSize = 257, 257, 12888616364822
	first.Leeching, first.LeecherCount, first.LeecherSize = 149, 149, 3872265847223
	first.UnreadMessageCount, first.TotalMessageCount = 15, 67
	first.HnRUnsatisfied, first.HnRPreWarning = 2, 1
	first.BonusPerHour, first.SeedingBonusPerHour = 92.2, 3.5
	require.NoError(t, repo.Save(ctx, first))

	second := sampleUserInfo("mteam")
	second.Seeding, second.SeederCount, second.SeederSize = 0, 0, 0
	second.Leeching, second.LeecherCount, second.LeecherSize = 0, 0, 0
	second.UnreadMessageCount, second.TotalMessageCount = 0, 0
	second.HnRUnsatisfied, second.HnRPreWarning = 0, 0
	second.BonusPerHour, second.SeedingBonusPerHour = 0, 0
	second.JoinDate = 0
	require.NoError(t, repo.Save(ctx, second))

	got, err := repo.Get(ctx, "mteam")
	require.NoError(t, err)
	assert.Zero(t, got.Seeding)
	assert.Zero(t, got.SeederCount)
	assert.Zero(t, got.SeederSize)
	assert.Zero(t, got.Leeching)
	assert.Zero(t, got.LeecherCount)
	assert.Zero(t, got.LeecherSize)
	assert.Zero(t, got.UnreadMessageCount)
	assert.Zero(t, got.TotalMessageCount)
	assert.Zero(t, got.HnRUnsatisfied)
	assert.Zero(t, got.HnRPreWarning)
	assert.Zero(t, got.BonusPerHour)
	assert.Zero(t, got.SeedingBonusPerHour)
	assert.Equal(t, first.JoinDate, got.JoinDate, "不在计数里的字段这次是零值时照旧不动")
}
