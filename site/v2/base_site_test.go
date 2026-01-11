package v2

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// MockDriver is a mock implementation of the Driver interface
type MockDriver struct {
	mock.Mock
}

func (m *MockDriver) PrepareSearch(query SearchQuery) (string, error) {
	args := m.Called(query)
	return args.String(0), args.Error(1)
}

func (m *MockDriver) Execute(ctx context.Context, req string) (string, error) {
	args := m.Called(ctx, req)
	return args.String(0), args.Error(1)
}

func (m *MockDriver) ParseSearch(res string) ([]TorrentItem, error) {
	args := m.Called(res)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]TorrentItem), args.Error(1)
}

func (m *MockDriver) GetUserInfo(ctx context.Context) (UserInfo, error) {
	args := m.Called(ctx)
	return args.Get(0).(UserInfo), args.Error(1)
}

func (m *MockDriver) PrepareDownload(torrentID string) (string, error) {
	args := m.Called(torrentID)
	return args.String(0), args.Error(1)
}

func (m *MockDriver) ParseDownload(res string) ([]byte, error) {
	args := m.Called(res)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func TestNewBaseSite(t *testing.T) {
	driver := &MockDriver{}
	config := BaseSiteConfig{
		ID:        "test-site",
		Name:      "Test Site",
		Kind:      SiteNexusPHP,
		RateLimit: 2.0,
		RateBurst: 5,
	}

	site := NewBaseSite(driver, config)

	assert.Equal(t, "test-site", site.ID())
	assert.Equal(t, "Test Site", site.Name())
	assert.Equal(t, SiteNexusPHP, site.Kind())
	assert.NotNil(t, site.GetRateLimiter())
	assert.Equal(t, driver, site.GetDriver())
}

func TestNewBaseSite_DefaultValues(t *testing.T) {
	driver := &MockDriver{}
	config := BaseSiteConfig{
		ID:   "test-site",
		Name: "Test Site",
		Kind: SiteNexusPHP,
		// No rate limit or burst specified
	}

	site := NewBaseSite(driver, config)

	// Should use default values
	assert.NotNil(t, site.GetRateLimiter())
}

func TestBaseSite_Login(t *testing.T) {
	driver := &MockDriver{}
	site := NewBaseSite(driver, BaseSiteConfig{
		ID:   "test-site",
		Name: "Test Site",
		Kind: SiteNexusPHP,
	})

	assert.False(t, site.IsLoggedIn())

	err := site.Login(context.Background(), Credentials{Cookie: "test-cookie"})
	require.NoError(t, err)

	assert.True(t, site.IsLoggedIn())
}

func TestBaseSite_Search(t *testing.T) {
	driver := &MockDriver{}
	site := NewBaseSite(driver, BaseSiteConfig{
		ID:        "test-site",
		Name:      "Test Site",
		Kind:      SiteNexusPHP,
		RateLimit: 100, // High rate limit for testing
		RateBurst: 100,
		Logger:    zap.NewNop(),
	})

	query := SearchQuery{Keyword: "test"}
	expectedItems := []TorrentItem{
		{ID: "1", Title: "Test Torrent 1"},
		{ID: "2", Title: "Test Torrent 2"},
	}

	driver.On("PrepareSearch", query).Return("prepared-request", nil)
	driver.On("Execute", mock.Anything, "prepared-request").Return("response", nil)
	driver.On("ParseSearch", "response").Return(expectedItems, nil)

	items, err := site.Search(context.Background(), query)
	require.NoError(t, err)
	require.Len(t, items, 2)

	// Check that source site is set
	assert.Equal(t, "test-site", items[0].SourceSite)
	assert.Equal(t, "test-site", items[1].SourceSite)

	driver.AssertExpectations(t)
}

func TestBaseSite_Search_InvalidQuery(t *testing.T) {
	driver := &MockDriver{}
	site := NewBaseSite(driver, BaseSiteConfig{
		ID:   "test-site",
		Name: "Test Site",
		Kind: SiteNexusPHP,
	})

	query := SearchQuery{Page: -1} // Invalid

	_, err := site.Search(context.Background(), query)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid query")
}

func TestBaseSite_Search_PrepareError(t *testing.T) {
	driver := &MockDriver{}
	site := NewBaseSite(driver, BaseSiteConfig{
		ID:        "test-site",
		Name:      "Test Site",
		Kind:      SiteNexusPHP,
		RateLimit: 100,
		RateBurst: 100,
		Logger:    zap.NewNop(),
	})

	query := SearchQuery{Keyword: "test"}
	driver.On("PrepareSearch", query).Return("", errors.New("prepare error"))

	_, err := site.Search(context.Background(), query)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "prepare search")

	driver.AssertExpectations(t)
}

func TestBaseSite_Search_ExecuteError(t *testing.T) {
	driver := &MockDriver{}
	site := NewBaseSite(driver, BaseSiteConfig{
		ID:        "test-site",
		Name:      "Test Site",
		Kind:      SiteNexusPHP,
		RateLimit: 100,
		RateBurst: 100,
		Logger:    zap.NewNop(),
	})

	query := SearchQuery{Keyword: "test"}
	driver.On("PrepareSearch", query).Return("prepared-request", nil)
	driver.On("Execute", mock.Anything, "prepared-request").Return("", errors.New("execute error"))

	_, err := site.Search(context.Background(), query)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "execute search")

	driver.AssertExpectations(t)
}

func TestBaseSite_Search_ParseError(t *testing.T) {
	driver := &MockDriver{}
	site := NewBaseSite(driver, BaseSiteConfig{
		ID:        "test-site",
		Name:      "Test Site",
		Kind:      SiteNexusPHP,
		RateLimit: 100,
		RateBurst: 100,
		Logger:    zap.NewNop(),
	})

	query := SearchQuery{Keyword: "test"}
	driver.On("PrepareSearch", query).Return("prepared-request", nil)
	driver.On("Execute", mock.Anything, "prepared-request").Return("response", nil)
	driver.On("ParseSearch", "response").Return(nil, errors.New("parse error"))

	_, err := site.Search(context.Background(), query)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse search")

	driver.AssertExpectations(t)
}

func TestBaseSite_GetUserInfo(t *testing.T) {
	driver := &MockDriver{}
	site := NewBaseSite(driver, BaseSiteConfig{
		ID:        "test-site",
		Name:      "Test Site",
		Kind:      SiteNexusPHP,
		RateLimit: 100,
		RateBurst: 100,
		Logger:    zap.NewNop(),
	})

	expectedInfo := UserInfo{
		Username: "testuser",
		Uploaded: 1000000,
	}

	driver.On("GetUserInfo", mock.Anything).Return(expectedInfo, nil)

	info, err := site.GetUserInfo(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "testuser", info.Username)
	assert.Equal(t, "test-site", info.Site)
	assert.NotZero(t, info.LastUpdate)

	driver.AssertExpectations(t)
}

func TestBaseSite_Download(t *testing.T) {
	driver := &MockDriver{}
	site := NewBaseSite(driver, BaseSiteConfig{
		ID:        "test-site",
		Name:      "Test Site",
		Kind:      SiteNexusPHP,
		RateLimit: 100,
		RateBurst: 100,
		Logger:    zap.NewNop(),
	})

	expectedData := []byte("torrent-file-data")

	driver.On("PrepareDownload", "12345").Return("download-request", nil)
	driver.On("Execute", mock.Anything, "download-request").Return("download-response", nil)
	driver.On("ParseDownload", "download-response").Return(expectedData, nil)

	data, err := site.Download(context.Background(), "12345")
	require.NoError(t, err)
	assert.Equal(t, expectedData, data)

	driver.AssertExpectations(t)
}

func TestBaseSite_Close(t *testing.T) {
	driver := &MockDriver{}
	site := NewBaseSite(driver, BaseSiteConfig{
		ID:   "test-site",
		Name: "Test Site",
		Kind: SiteNexusPHP,
	})

	// Login first
	err := site.Login(context.Background(), Credentials{Cookie: "test"})
	require.NoError(t, err)
	assert.True(t, site.IsLoggedIn())

	// Close
	err = site.Close()
	require.NoError(t, err)
	assert.False(t, site.IsLoggedIn())
}

func TestBaseSite_RateLimiting(t *testing.T) {
	driver := &MockDriver{}
	site := NewBaseSite(driver, BaseSiteConfig{
		ID:        "test-site",
		Name:      "Test Site",
		Kind:      SiteNexusPHP,
		RateLimit: 10, // 10 requests per second
		RateBurst: 1,  // Burst of 1
		Logger:    zap.NewNop(),
	})

	query := SearchQuery{Keyword: "test"}
	expectedItems := []TorrentItem{{ID: "1", Title: "Test"}}

	driver.On("PrepareSearch", query).Return("req", nil)
	driver.On("Execute", mock.Anything, "req").Return("res", nil)
	driver.On("ParseSearch", "res").Return(expectedItems, nil)

	// First request should be immediate
	start := time.Now()
	_, err := site.Search(context.Background(), query)
	require.NoError(t, err)
	firstDuration := time.Since(start)

	// Second request should be rate limited
	start = time.Now()
	_, err = site.Search(context.Background(), query)
	require.NoError(t, err)
	secondDuration := time.Since(start)

	// The second request should take at least ~100ms (1/10 second)
	// due to rate limiting, but we allow some tolerance
	assert.Less(t, firstDuration, 50*time.Millisecond)
	assert.GreaterOrEqual(t, secondDuration, 50*time.Millisecond)
}

func TestBaseSite_RateLimiting_ContextCanceled(t *testing.T) {
	driver := &MockDriver{}
	site := NewBaseSite(driver, BaseSiteConfig{
		ID:        "test-site",
		Name:      "Test Site",
		Kind:      SiteNexusPHP,
		RateLimit: 0.1, // Very slow rate
		RateBurst: 1,
		Logger:    zap.NewNop(),
	})

	query := SearchQuery{Keyword: "test"}
	expectedItems := []TorrentItem{{ID: "1", Title: "Test"}}

	driver.On("PrepareSearch", query).Return("req", nil)
	driver.On("Execute", mock.Anything, "req").Return("res", nil)
	driver.On("ParseSearch", "res").Return(expectedItems, nil)

	// First request to consume the burst
	_, err := site.Search(context.Background(), query)
	require.NoError(t, err)

	// Second request with canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = site.Search(ctx, query)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rate limit")
}
