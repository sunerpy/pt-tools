package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

type bridgeEventBus struct {
	mu          sync.Mutex
	subscribed  []string
	subscribers map[string][]chan core.Event
	closed      []bool
}

func newBridgeEventBus() *bridgeEventBus {
	return &bridgeEventBus{subscribers: make(map[string][]chan core.Event)}
}

func (b *bridgeEventBus) Publish(_ context.Context, ev core.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subscribers[ev.Type] {
		ch <- ev
	}
	return nil
}

func (b *bridgeEventBus) Subscribe(_ context.Context, eventType string) (<-chan core.Event, func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribed = append(b.subscribed, eventType)
	ch := make(chan core.Event, 4)
	b.subscribers[eventType] = append(b.subscribers[eventType], ch)
	return ch, func() { close(ch) }, nil
}

type mockTVScraper struct {
	info          core.ProviderInfo
	searchResults []core.MediaSearchCandidate
	show          *core.TvShow
	searchErr     error
	getErr        error
}

func (m *mockTVScraper) Info() core.ProviderInfo { return m.info }
func (m *mockTVScraper) IsActive() bool          { return true }
func (m *mockTVScraper) SearchTvShow(context.Context, core.TvShowSearchOptions) ([]core.MediaSearchCandidate, error) {
	return m.searchResults, m.searchErr
}

func (m *mockTVScraper) GetTvShowMetadata(context.Context, core.TvShowSearchOptions) (*core.TvShow, error) {
	return m.show, m.getErr
}

func (m *mockTVScraper) GetEpisodeList(context.Context, core.TvShowSearchOptions) ([]core.TvShowEpisode, error) {
	return nil, nil
}

func (m *mockTVScraper) GetEpisodeMetadata(context.Context, core.TvShowEpisodeSearchOptions) (*core.TvShowEpisode, error) {
	return nil, errors.New("not implemented")
}

type mockTVFuser struct {
	merged *core.TvShow
}

func (m *mockTVFuser) Merge(context.Context, map[string]*core.RawMediaInfo) (*core.Movie, error) {
	return nil, errors.New("not implemented")
}

func (m *mockTVFuser) MergeTv(context.Context, map[string]*core.RawMediaInfo) (*core.TvShow, error) {
	return m.merged, nil
}

func (m *mockTVFuser) MergeEpisode(context.Context, map[string]*core.RawMediaInfo) (*core.TvShowEpisode, error) {
	return nil, errors.New("not implemented")
}

func TestEventBridge_StartSubscribesTorrentCompleted(t *testing.T) {
	bus := newBridgeEventBus()
	bridge := NewEventBridge(bus, nil, noopLogger{})
	require.Error(t, bridge.Start(context.Background()))

	db := openTestDB(t)
	sourceReg := core.NewRegistry[core.MediaScraper]()
	writerReg := core.NewRegistry[core.NfoWriter]()
	require.NoError(t, writerReg.Register(defaultNfoDialect, func() core.NfoWriter { return &mockWriter{} }))
	svc, err := NewScrapeService(ServiceConfig{DB: db, SourceReg: sourceReg, WriterReg: writerReg, Fuser: &mockFuser{}})
	require.NoError(t, err)

	bridge = NewEventBridge(bus, svc, noopLogger{})
	require.NoError(t, bridge.Start(context.Background()))
	require.Equal(t, []string{core.EventTypeTorrentCompleted}, bus.subscribed)
}

func TestEventBridge_PublishMovieTriggersScrape(t *testing.T) {
	db := openTestDB(t)
	mediaDir := t.TempDir()
	mediaPath := filepath.Join(mediaDir, "Inception (2010).mkv")
	require.NoError(t, os.WriteFile(mediaPath, []byte("dummy"), 0o644))

	sourceReg := core.NewRegistry[core.MediaScraper]()
	writerReg := core.NewRegistry[core.NfoWriter]()
	movie := &core.Movie{MediaEntity: core.MediaEntity{Title: "Inception", Year: 2010}}
	require.NoError(t, sourceReg.Register("tmdb", func() core.MediaScraper {
		return &mockMovieScraper{
			info:          core.ProviderInfo{Name: "tmdb"},
			searchResults: []core.MediaSearchCandidate{{ID: "27205", Title: "Inception", Year: 2010}},
			movie:         movie,
		}
	}))
	writer := &mockWriter{}
	require.NoError(t, writerReg.Register(defaultNfoDialect, func() core.NfoWriter { return writer }))
	svc, err := NewScrapeService(ServiceConfig{DB: db, SourceReg: sourceReg, WriterReg: writerReg, Fuser: &mockFuser{merged: movie}})
	require.NoError(t, err)

	bus := newBridgeEventBus()
	bridge := NewEventBridge(bus, svc, noopLogger{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, bridge.Start(ctx))

	require.NoError(t, bus.Publish(ctx, core.Event{
		Type: core.EventTypeTorrentCompleted,
		Payload: TorrentCompletedPayload{
			MediaPath: mediaPath,
			Type:      "movie",
		},
	}))

	require.Eventually(t, func() bool {
		var count int64
		if err := db.Table("scrape_results").Count(&count).Error; err != nil {
			return false
		}
		return count == 1
	}, 2*time.Second, 20*time.Millisecond)
}

func TestEventBridge_PublishTVTriggersScrape(t *testing.T) {
	db := openTestDB(t)
	showDir := filepath.Join(t.TempDir(), "Breaking Bad (2008)")
	require.NoError(t, os.MkdirAll(showDir, 0o755))

	sourceReg := core.NewRegistry[core.MediaScraper]()
	writerReg := core.NewRegistry[core.NfoWriter]()
	show := &core.TvShow{MediaEntity: core.MediaEntity{Title: "Breaking Bad", Year: 2008}}
	require.NoError(t, sourceReg.Register("tmdb", func() core.MediaScraper {
		return &mockTVScraper{
			info:          core.ProviderInfo{Name: "tmdb"},
			searchResults: []core.MediaSearchCandidate{{ID: "1396", Title: "Breaking Bad", Year: 2008}},
			show:          show,
		}
	}))
	require.NoError(t, writerReg.Register(defaultNfoDialect, func() core.NfoWriter { return &mockWriter{} }))
	svc, err := NewScrapeService(ServiceConfig{DB: db, SourceReg: sourceReg, WriterReg: writerReg, Fuser: &mockTVFuser{merged: show}})
	require.NoError(t, err)

	bus := newBridgeEventBus()
	bridge := NewEventBridge(bus, svc, noopLogger{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, bridge.Start(ctx))

	require.NoError(t, bus.Publish(ctx, core.Event{Type: core.EventTypeTorrentCompleted, Payload: map[string]any{
		"media_path": showDir,
		"type":       "tv",
	}}))

	require.Eventually(t, func() bool {
		var count int64
		if err := db.Table("scrape_results").Count(&count).Error; err != nil {
			return false
		}
		return count == 1
	}, 2*time.Second, 20*time.Millisecond)
}

func TestEventBridge_UnsupportedTypeIgnored(t *testing.T) {
	db := openTestDB(t)
	sourceReg := core.NewRegistry[core.MediaScraper]()
	writerReg := core.NewRegistry[core.NfoWriter]()
	require.NoError(t, writerReg.Register(defaultNfoDialect, func() core.NfoWriter { return &mockWriter{} }))
	svc, err := NewScrapeService(ServiceConfig{DB: db, SourceReg: sourceReg, WriterReg: writerReg, Fuser: &mockFuser{}})
	require.NoError(t, err)

	bus := newBridgeEventBus()
	bridge := NewEventBridge(bus, svc, noopLogger{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, bridge.Start(ctx))

	require.NoError(t, bus.Publish(ctx, core.Event{Type: core.EventTypeTorrentCompleted, Payload: TorrentCompletedPayload{
		MediaPath: "/tmp/ignored",
		Type:      "unknown",
	}}))

	time.Sleep(100 * time.Millisecond)
	var count int64
	require.NoError(t, db.Table("scrape_results").Count(&count).Error)
	require.Zero(t, count)
}
