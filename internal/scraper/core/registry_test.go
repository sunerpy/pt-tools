package core

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type MockScraper struct {
	name string
	id   int
}

func (m *MockScraper) Info() ProviderInfo {
	return ProviderInfo{Name: m.name}
}

func (m *MockScraper) IsActive() bool {
	return true
}

func TestRegistryRegister(t *testing.T) {
	reg := NewRegistry[MediaScraper]()

	factory := func() MediaScraper {
		return &MockScraper{name: "test"}
	}

	err := reg.Register("test", factory)
	assert.NoError(t, err)
	assert.True(t, reg.Has("test"))
}

func TestRegistryDuplicateError(t *testing.T) {
	reg := NewRegistry[MediaScraper]()

	factory := func() MediaScraper {
		return &MockScraper{name: "test"}
	}

	err1 := reg.Register("test", factory)
	assert.NoError(t, err1)

	err2 := reg.Register("test", factory)
	assert.Error(t, err2)
	assert.True(t, errors.Is(err2, ErrAlreadyExists))
}

func TestRegistryGetNotFound(t *testing.T) {
	reg := NewRegistry[MediaScraper]()

	_, err := reg.Get("nonexistent")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestRegistryGet(t *testing.T) {
	reg := NewRegistry[MediaScraper]()

	factory := func() MediaScraper {
		return &MockScraper{name: "test"}
	}

	err := reg.Register("test", factory)
	require.NoError(t, err)

	instance, err := reg.Get("test")
	assert.NoError(t, err)
	assert.NotNil(t, instance)
	assert.Equal(t, "test", instance.Info().Name)
}

func TestRegistryList(t *testing.T) {
	reg := NewRegistry[MediaScraper]()

	names := []string{"charlie", "alpha", "bravo"}
	for _, name := range names {
		name := name
		factory := func() MediaScraper {
			return &MockScraper{name: name}
		}
		err := reg.Register(name, factory)
		require.NoError(t, err)
	}

	list := reg.List()
	assert.Equal(t, []string{"alpha", "bravo", "charlie"}, list)
}

func TestRegistryHas(t *testing.T) {
	reg := NewRegistry[MediaScraper]()

	factory := func() MediaScraper {
		return &MockScraper{name: "test"}
	}

	assert.False(t, reg.Has("test"))
	reg.Register("test", factory)
	assert.True(t, reg.Has("test"))
}

func TestRegistryEmptyName(t *testing.T) {
	reg := NewRegistry[MediaScraper]()

	factory := func() MediaScraper {
		return &MockScraper{name: "test"}
	}

	err := reg.Register("", factory)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidID))
}

func TestRegistryNilFactory(t *testing.T) {
	reg := NewRegistry[MediaScraper]()

	err := reg.Register("test", nil)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidID))
}

func TestRegistryConcurrent(t *testing.T) {
	reg := NewRegistry[MediaScraper]()
	numGoroutines := 100
	operationsPerGoroutine := 10

	var wg sync.WaitGroup
	var successCount atomic.Int32

	for i := 0; i < numGoroutines; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				name := "scraper-" + string(rune(i))

				factory := func() MediaScraper {
					id := i*operationsPerGoroutine + j
					return &MockScraper{name: name, id: id}
				}

				err := reg.Register(name, factory)
				if err == nil {
					successCount.Add(1)

					instance, getErr := reg.Get(name)
					if getErr == nil && instance != nil {
						list := reg.List()
						_ = list
						has := reg.Has(name)
						if has {
							successCount.Add(1)
						}
					}
				}
			}
		}()
	}

	wg.Wait()

	assert.Greater(t, int(successCount.Load()), 0)
	list := reg.List()
	assert.Greater(t, len(list), 0)
}

func TestScraperRegistryAlias(t *testing.T) {
	reg := NewRegistry[MediaScraper]()

	factory := func() MediaScraper {
		return &MockScraper{name: "movie-scraper"}
	}

	err := reg.Register("tmdb", factory)
	require.NoError(t, err)

	instance, err := reg.Get("tmdb")
	assert.NoError(t, err)
	assert.NotNil(t, instance)
}

func TestConnectorRegistryAlias(t *testing.T) {
	reg := NewRegistry[MediaServerConnector]()

	factory := func() MediaServerConnector {
		return NewMockConnector("jellyfin")
	}

	err := reg.Register("jellyfin", factory)
	require.NoError(t, err)

	assert.True(t, reg.Has("jellyfin"))
}

func TestWriterRegistryAlias(t *testing.T) {
	reg := NewRegistry[NfoWriter]()

	factory := func() NfoWriter {
		return NewMockWriter("kodi")
	}

	err := reg.Register("kodi", factory)
	require.NoError(t, err)

	list := reg.List()
	assert.Contains(t, list, "kodi")
}

type MockConnector struct {
	name string
}

func NewMockConnector(name string) MediaServerConnector {
	return &MockConnector{name: name}
}

func (m *MockConnector) Name() string {
	return m.name
}

func (m *MockConnector) Ping(ctx context.Context) (*ServerInfo, error) {
	return &ServerInfo{Name: m.name}, nil
}

func (m *MockConnector) Authenticate(ctx context.Context) (*ServerInfo, error) {
	return &ServerInfo{Name: m.name}, nil
}

func (m *MockConnector) ListLibraries(ctx context.Context) ([]Library, error) {
	return nil, nil
}

func (m *MockConnector) RefreshLibrary(ctx context.Context, libraryID string) error {
	return nil
}

func (m *MockConnector) ScanStatus(ctx context.Context) (*ScanStatus, error) {
	return &ScanStatus{}, nil
}

type MockWriter struct {
	dialect string
}

func NewMockWriter(dialect string) NfoWriter {
	return &MockWriter{dialect: dialect}
}

func (m *MockWriter) WriteMovieNfo(ctx context.Context, movie *Movie, paths []string) error {
	return nil
}

func (m *MockWriter) WriteTvShowNfo(ctx context.Context, s *TvShow, showDir string) error {
	return nil
}

func (m *MockWriter) WriteSeasonNfo(ctx context.Context, s *TvShowSeason, seasonDir string) error {
	return nil
}

func (m *MockWriter) WriteEpisodeNfo(ctx context.Context, e *TvShowEpisode, path string) error {
	return nil
}

func (m *MockWriter) Dialect() string {
	return m.dialect
}
