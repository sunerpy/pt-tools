package v2

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockSite is a mock implementation of the Site interface
type MockSite struct {
	mock.Mock
}

func (m *MockSite) ID() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockSite) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockSite) Kind() SiteKind {
	args := m.Called()
	return args.Get(0).(SiteKind)
}

func (m *MockSite) Login(ctx context.Context, creds Credentials) error {
	args := m.Called(ctx, creds)
	return args.Error(0)
}

func (m *MockSite) Search(ctx context.Context, query SearchQuery) ([]TorrentItem, error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]TorrentItem), args.Error(1)
}

func (m *MockSite) GetUserInfo(ctx context.Context) (UserInfo, error) {
	args := m.Called(ctx)
	return args.Get(0).(UserInfo), args.Error(1)
}

func (m *MockSite) Download(ctx context.Context, torrentID string) ([]byte, error) {
	args := m.Called(ctx, torrentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockSite) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestNewUserInfoService(t *testing.T) {
	service := NewUserInfoService(UserInfoServiceConfig{})
	assert.NotNil(t, service)
}

func TestNewUserInfoService_WithConfig(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	service := NewUserInfoService(UserInfoServiceConfig{
		Repo:     repo,
		CacheTTL: 10 * time.Minute,
	})
	assert.NotNil(t, service)
}

func TestUserInfoService_RegisterSite(t *testing.T) {
	service := NewUserInfoService(UserInfoServiceConfig{})

	mockSite := &MockSite{}
	mockSite.On("ID").Return("hdsky")

	service.RegisterSite(mockSite)

	site, ok := service.GetSite("hdsky")
	assert.True(t, ok)
	assert.Equal(t, mockSite, site)
}

func TestUserInfoService_UnregisterSite(t *testing.T) {
	service := NewUserInfoService(UserInfoServiceConfig{})

	mockSite := &MockSite{}
	mockSite.On("ID").Return("hdsky")

	service.RegisterSite(mockSite)
	service.UnregisterSite("hdsky")

	_, ok := service.GetSite("hdsky")
	assert.False(t, ok)
}

func TestUserInfoService_ListSites(t *testing.T) {
	service := NewUserInfoService(UserInfoServiceConfig{})

	site1 := &MockSite{}
	site1.On("ID").Return("hdsky")
	site2 := &MockSite{}
	site2.On("ID").Return("mteam")

	service.RegisterSite(site1)
	service.RegisterSite(site2)

	sites := service.ListSites()
	assert.Len(t, sites, 2)
	assert.Contains(t, sites, "hdsky")
	assert.Contains(t, sites, "mteam")
}

func TestUserInfoService_FetchAndSave(t *testing.T) {
	service := NewUserInfoService(UserInfoServiceConfig{})
	ctx := context.Background()

	expectedInfo := UserInfo{
		Site:     "hdsky",
		Username: "testuser",
		Uploaded: 1000000,
	}

	mockSite := &MockSite{}
	mockSite.On("ID").Return("hdsky")
	mockSite.On("GetUserInfo", ctx).Return(expectedInfo, nil)

	service.RegisterSite(mockSite)

	info, err := service.FetchAndSave(ctx, "hdsky")
	require.NoError(t, err)
	assert.Equal(t, "testuser", info.Username)
	assert.Equal(t, int64(1000000), info.Uploaded)

	mockSite.AssertExpectations(t)
}

func TestUserInfoService_FetchAndSave_SiteNotFound(t *testing.T) {
	service := NewUserInfoService(UserInfoServiceConfig{})
	ctx := context.Background()

	_, err := service.FetchAndSave(ctx, "nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrSiteNotFound)
}

func TestUserInfoService_FetchAndSave_FetchError(t *testing.T) {
	service := NewUserInfoService(UserInfoServiceConfig{})
	ctx := context.Background()

	mockSite := &MockSite{}
	mockSite.On("ID").Return("hdsky")
	mockSite.On("GetUserInfo", ctx).Return(UserInfo{}, errors.New("network error"))

	service.RegisterSite(mockSite)

	_, err := service.FetchAndSave(ctx, "hdsky")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "network error")
}

func TestUserInfoService_FetchAndSaveAll(t *testing.T) {
	service := NewUserInfoService(UserInfoServiceConfig{})
	ctx := context.Background()

	site1 := &MockSite{}
	site1.On("ID").Return("hdsky")
	site1.On("GetUserInfo", ctx).Return(UserInfo{Site: "hdsky", Username: "user1"}, nil)

	site2 := &MockSite{}
	site2.On("ID").Return("mteam")
	site2.On("GetUserInfo", ctx).Return(UserInfo{Site: "mteam", Username: "user2"}, nil)

	service.RegisterSite(site1)
	service.RegisterSite(site2)

	results, errs := service.FetchAndSaveAll(ctx)
	assert.Len(t, results, 2)
	assert.Empty(t, errs)
}

func TestUserInfoService_FetchAndSaveAll_PartialFailure(t *testing.T) {
	service := NewUserInfoService(UserInfoServiceConfig{})
	ctx := context.Background()

	site1 := &MockSite{}
	site1.On("ID").Return("hdsky")
	site1.On("GetUserInfo", ctx).Return(UserInfo{Site: "hdsky", Username: "user1"}, nil)

	site2 := &MockSite{}
	site2.On("ID").Return("mteam")
	site2.On("GetUserInfo", ctx).Return(UserInfo{}, errors.New("error"))

	service.RegisterSite(site1)
	service.RegisterSite(site2)

	results, errs := service.FetchAndSaveAll(ctx)
	assert.Len(t, results, 1)
	assert.Len(t, errs, 1)
}

func TestUserInfoService_GetUserInfo_FromCache(t *testing.T) {
	service := NewUserInfoService(UserInfoServiceConfig{
		CacheTTL: 1 * time.Hour,
	})
	ctx := context.Background()

	expectedInfo := UserInfo{
		Site:     "hdsky",
		Username: "testuser",
	}

	mockSite := &MockSite{}
	mockSite.On("ID").Return("hdsky")
	mockSite.On("GetUserInfo", ctx).Return(expectedInfo, nil)

	service.RegisterSite(mockSite)

	// First fetch - should call site
	_, err := service.FetchAndSave(ctx, "hdsky")
	require.NoError(t, err)

	// Second fetch - should use cache
	info, err := service.GetUserInfo(ctx, "hdsky")
	require.NoError(t, err)
	assert.Equal(t, "testuser", info.Username)

	// GetUserInfo should only be called once (for FetchAndSave)
	mockSite.AssertNumberOfCalls(t, "GetUserInfo", 1)
}

func TestUserInfoService_GetUserInfo_CacheExpired(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	service := NewUserInfoService(UserInfoServiceConfig{
		Repo:     repo,
		CacheTTL: 1 * time.Millisecond, // Very short TTL
	})
	ctx := context.Background()

	// Save directly to repo
	err := repo.Save(ctx, UserInfo{Site: "hdsky", Username: "testuser"})
	require.NoError(t, err)

	// Wait for cache to expire
	time.Sleep(10 * time.Millisecond)

	// Should fetch from repo since cache expired
	info, err := service.GetUserInfo(ctx, "hdsky")
	require.NoError(t, err)
	assert.Equal(t, "testuser", info.Username)
}

func TestUserInfoService_GetUserInfo_NotFound(t *testing.T) {
	service := NewUserInfoService(UserInfoServiceConfig{})
	ctx := context.Background()

	_, err := service.GetUserInfo(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrSiteNotFound)
}

func TestUserInfoService_GetAllUserInfo(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	service := NewUserInfoService(UserInfoServiceConfig{Repo: repo})
	ctx := context.Background()

	// Save some data
	err := repo.Save(ctx, UserInfo{Site: "hdsky"})
	require.NoError(t, err)
	err = repo.Save(ctx, UserInfo{Site: "mteam"})
	require.NoError(t, err)

	all, err := service.GetAllUserInfo(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestUserInfoService_GetAggregated(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	service := NewUserInfoService(UserInfoServiceConfig{Repo: repo})
	ctx := context.Background()

	// Save some data
	err := repo.Save(ctx, UserInfo{Site: "hdsky", Uploaded: 1000000, Downloaded: 500000})
	require.NoError(t, err)
	err = repo.Save(ctx, UserInfo{Site: "mteam", Uploaded: 2000000, Downloaded: 1000000})
	require.NoError(t, err)

	stats, err := service.GetAggregated(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(3000000), stats.TotalUploaded)
	assert.Equal(t, int64(1500000), stats.TotalDownloaded)
	assert.Equal(t, 2, stats.SiteCount)
}

func TestUserInfoService_DeleteUserInfo(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	service := NewUserInfoService(UserInfoServiceConfig{Repo: repo})
	ctx := context.Background()

	// Save data
	err := repo.Save(ctx, UserInfo{Site: "hdsky"})
	require.NoError(t, err)

	// Delete
	err = service.DeleteUserInfo(ctx, "hdsky")
	require.NoError(t, err)

	// Verify deleted
	_, err = service.GetUserInfo(ctx, "hdsky")
	assert.ErrorIs(t, err, ErrSiteNotFound)
}

func TestUserInfoService_ClearCache(t *testing.T) {
	service := NewUserInfoService(UserInfoServiceConfig{
		CacheTTL: 1 * time.Hour,
	})
	ctx := context.Background()

	mockSite := &MockSite{}
	mockSite.On("ID").Return("hdsky")
	mockSite.On("GetUserInfo", ctx).Return(UserInfo{Site: "hdsky", Username: "testuser"}, nil)

	service.RegisterSite(mockSite)

	// Fetch to populate cache
	_, err := service.FetchAndSave(ctx, "hdsky")
	require.NoError(t, err)

	// Clear cache
	service.ClearCache()

	// Next GetUserInfo should go to repo (not cache)
	// Since we saved to repo, it should still work
	info, err := service.GetUserInfo(ctx, "hdsky")
	require.NoError(t, err)
	assert.Equal(t, "testuser", info.Username)
}
