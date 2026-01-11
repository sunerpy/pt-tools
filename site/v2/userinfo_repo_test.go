package v2

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInMemoryUserInfoRepo(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	assert.NotNil(t, repo)
	assert.Equal(t, 0, repo.Count())
}

func TestInMemoryUserInfoRepo_Save(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	ctx := context.Background()

	info := UserInfo{
		Site:       "hdsky",
		Username:   "testuser",
		Uploaded:   1000000,
		Downloaded: 500000,
		Ratio:      2.0,
	}

	err := repo.Save(ctx, info)
	require.NoError(t, err)
	assert.Equal(t, 1, repo.Count())
}

func TestInMemoryUserInfoRepo_Save_EmptySite(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	ctx := context.Background()

	info := UserInfo{
		Username: "testuser",
	}

	err := repo.Save(ctx, info)
	assert.ErrorIs(t, err, ErrSiteNotFound)
}

func TestInMemoryUserInfoRepo_Save_UpdatesLastUpdate(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	ctx := context.Background()

	info := UserInfo{
		Site:       "hdsky",
		Username:   "testuser",
		LastUpdate: 0,
	}

	err := repo.Save(ctx, info)
	require.NoError(t, err)

	saved, err := repo.Get(ctx, "hdsky")
	require.NoError(t, err)
	assert.NotZero(t, saved.LastUpdate)
}

func TestInMemoryUserInfoRepo_Get(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	ctx := context.Background()

	info := UserInfo{
		Site:     "hdsky",
		Username: "testuser",
		Uploaded: 1000000,
	}

	err := repo.Save(ctx, info)
	require.NoError(t, err)

	retrieved, err := repo.Get(ctx, "hdsky")
	require.NoError(t, err)
	assert.Equal(t, "testuser", retrieved.Username)
	assert.Equal(t, int64(1000000), retrieved.Uploaded)
}

func TestInMemoryUserInfoRepo_Get_NotFound(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	ctx := context.Background()

	_, err := repo.Get(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrSiteNotFound)
}

func TestInMemoryUserInfoRepo_ListAll(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	ctx := context.Background()

	// Save multiple entries
	sites := []string{"hdsky", "mteam", "chdbits"}
	for _, site := range sites {
		err := repo.Save(ctx, UserInfo{Site: site, Username: "user_" + site})
		require.NoError(t, err)
	}

	all, err := repo.ListAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestInMemoryUserInfoRepo_ListAll_Empty(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	ctx := context.Background()

	all, err := repo.ListAll(ctx)
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestInMemoryUserInfoRepo_ListBySites(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	ctx := context.Background()

	// Save multiple entries
	sites := []string{"hdsky", "mteam", "chdbits", "ourbits"}
	for _, site := range sites {
		err := repo.Save(ctx, UserInfo{Site: site, Username: "user_" + site})
		require.NoError(t, err)
	}

	// Query subset
	result, err := repo.ListBySites(ctx, []string{"hdsky", "mteam"})
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

func TestInMemoryUserInfoRepo_ListBySites_PartialMatch(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	ctx := context.Background()

	err := repo.Save(ctx, UserInfo{Site: "hdsky", Username: "user1"})
	require.NoError(t, err)

	// Query with some non-existent sites
	result, err := repo.ListBySites(ctx, []string{"hdsky", "nonexistent"})
	require.NoError(t, err)
	assert.Len(t, result, 1)
}

func TestInMemoryUserInfoRepo_Delete(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	ctx := context.Background()

	err := repo.Save(ctx, UserInfo{Site: "hdsky", Username: "user1"})
	require.NoError(t, err)
	assert.Equal(t, 1, repo.Count())

	err = repo.Delete(ctx, "hdsky")
	require.NoError(t, err)
	assert.Equal(t, 0, repo.Count())
}

func TestInMemoryUserInfoRepo_Delete_NotFound(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	ctx := context.Background()

	err := repo.Delete(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrSiteNotFound)
}

func TestInMemoryUserInfoRepo_GetAggregated(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	ctx := context.Background()

	// Save multiple entries
	infos := []UserInfo{
		{Site: "hdsky", Uploaded: 1000000, Downloaded: 500000, Ratio: 2.0, Seeding: 10, Leeching: 1, Bonus: 100},
		{Site: "mteam", Uploaded: 2000000, Downloaded: 1000000, Ratio: 2.0, Seeding: 20, Leeching: 2, Bonus: 200},
		{Site: "chdbits", Uploaded: 500000, Downloaded: 250000, Ratio: 2.0, Seeding: 5, Leeching: 0, Bonus: 50},
	}

	for _, info := range infos {
		err := repo.Save(ctx, info)
		require.NoError(t, err)
	}

	stats, err := repo.GetAggregated(ctx)
	require.NoError(t, err)

	assert.Equal(t, int64(3500000), stats.TotalUploaded)
	assert.Equal(t, int64(1750000), stats.TotalDownloaded)
	assert.Equal(t, 35, stats.TotalSeeding)
	assert.Equal(t, 3, stats.TotalLeeching)
	assert.Equal(t, 350.0, stats.TotalBonus)
	assert.Equal(t, 3, stats.SiteCount)
	assert.Equal(t, 2.0, stats.AverageRatio)
	assert.Len(t, stats.PerSiteStats, 3)
}

func TestInMemoryUserInfoRepo_GetAggregated_Empty(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	ctx := context.Background()

	stats, err := repo.GetAggregated(ctx)
	require.NoError(t, err)

	assert.Equal(t, int64(0), stats.TotalUploaded)
	assert.Equal(t, 0, stats.SiteCount)
	assert.Equal(t, 0.0, stats.AverageRatio)
}

func TestInMemoryUserInfoRepo_GetAggregated_ExcludesInfiniteRatio(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	ctx := context.Background()

	infos := []UserInfo{
		{Site: "hdsky", Ratio: 2.0},
		{Site: "mteam", Ratio: -1}, // Infinite ratio
		{Site: "chdbits", Ratio: 4.0},
	}

	for _, info := range infos {
		err := repo.Save(ctx, info)
		require.NoError(t, err)
	}

	stats, err := repo.GetAggregated(ctx)
	require.NoError(t, err)

	// Average should only include valid ratios (2.0 and 4.0)
	assert.Equal(t, 3.0, stats.AverageRatio)
}

func TestInMemoryUserInfoRepo_Clear(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	ctx := context.Background()

	err := repo.Save(ctx, UserInfo{Site: "hdsky"})
	require.NoError(t, err)
	err = repo.Save(ctx, UserInfo{Site: "mteam"})
	require.NoError(t, err)

	assert.Equal(t, 2, repo.Count())

	repo.Clear()
	assert.Equal(t, 0, repo.Count())
}

func TestInMemoryUserInfoRepo_ConcurrentAccess(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	ctx := context.Background()

	var wg sync.WaitGroup
	sites := []string{"site1", "site2", "site3", "site4", "site5"}

	// Concurrent writes
	for _, site := range sites {
		wg.Add(1)
		go func(s string) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				err := repo.Save(ctx, UserInfo{Site: s, Uploaded: int64(i)})
				assert.NoError(t, err)
			}
		}(site)
	}

	// Concurrent reads
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = repo.ListAll(ctx)
				_, _ = repo.GetAggregated(ctx)
			}
		}()
	}

	wg.Wait()
	assert.Equal(t, 5, repo.Count())
}
