package notify

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

type mockRouterChannel struct {
	typ     string
	sendErr error

	mu        sync.Mutex
	initCalls int
	sendCalls int
	received  []Notification
}

func (m *mockRouterChannel) Type() string { return m.typ }

func (m *mockRouterChannel) Init(_ context.Context, _ *models.NotificationConf) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.initCalls++
	return nil
}

func (m *mockRouterChannel) SupportsInbound() bool { return false }

func (m *mockRouterChannel) Send(_ context.Context, n Notification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendCalls++
	m.received = append(m.received, n)
	return m.sendErr
}

func (m *mockRouterChannel) OnInbound(_ InboundHandler) {}

func (m *mockRouterChannel) Close(_ context.Context) error { return nil }

func (m *mockRouterChannel) Healthy() bool { return true }

func (m *mockRouterChannel) SendCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sendCalls
}

type mockConfLister struct {
	confs []models.NotificationConf
}

func (m mockConfLister) ListNotificationConfs(_ context.Context) ([]models.NotificationConf, error) {
	return append([]models.NotificationConf(nil), m.confs...), nil
}

type outboxEntry struct {
	confID uint
	n      Notification
	errMsg string
}

type mockRouterOutbox struct {
	mu      sync.Mutex
	entries []outboxEntry
}

func (m *mockRouterOutbox) Enqueue(_ context.Context, confID uint, n Notification, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, outboxEntry{confID: confID, n: n, errMsg: errMsg})
	return nil
}

func (m *mockRouterOutbox) Entries() []outboxEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]outboxEntry(nil), m.entries...)
}

func newRouterTestRegistry(channels map[string]*mockRouterChannel) *Registry {
	registry := NewRegistry()
	for typ, ch := range channels {
		channel := ch
		registry.Register(typ, func() Channel { return channel })
	}
	return registry
}

func newRouterTestConfs(ids ...uint) []models.NotificationConf {
	confs := make([]models.NotificationConf, 0, len(ids))
	for _, id := range ids {
		confs = append(confs, models.NotificationConf{
			ID:          id,
			ChannelType: routerTestType(id),
			Name:        routerTestType(id),
			Enabled:     true,
		})
	}
	return confs
}

func routerTestType(id uint) string { return "router_mock_" + string(rune('a'+id-1)) }

func TestRouter_FanOut(t *testing.T) {
	channels := map[string]*mockRouterChannel{
		"router_mock_a": {typ: "router_mock_a"},
		"router_mock_b": {typ: "router_mock_b"},
		"router_mock_c": {typ: "router_mock_c"},
	}
	router := NewRouter(
		newRouterTestRegistry(channels),
		&mockRouterOutbox{},
		mockConfLister{confs: newRouterTestConfs(1, 2, 3)},
	)

	err := router.Route(context.Background(), Notification{Title: "PT", Text: "done"}, RouteScope{
		ConfIDs:    []uint{1, 2, 3},
		EventType:  "torrent.completed",
		PrimaryID:  "torrent-1",
		SkipDedupe: true,
	})

	require.NoError(t, err)
	require.Equal(t, 1, channels["router_mock_a"].SendCalls())
	require.Equal(t, 1, channels["router_mock_b"].SendCalls())
	require.Equal(t, 1, channels["router_mock_c"].SendCalls())
}

func TestRouter_DedupeWithin60s(t *testing.T) {
	channel := &mockRouterChannel{typ: "router_mock_a"}
	router := NewRouter(
		newRouterTestRegistry(map[string]*mockRouterChannel{"router_mock_a": channel}),
		&mockRouterOutbox{},
		mockConfLister{confs: newRouterTestConfs(1)},
	)

	for i := 0; i < 5; i++ {
		err := router.Route(context.Background(), Notification{Title: "PT", Text: "done"}, RouteScope{
			ConfIDs:   []uint{1},
			EventType: "torrent.completed",
			PrimaryID: "torrent-1",
		})
		require.NoError(t, err)
	}

	require.Equal(t, 1, channel.SendCalls())
}

func TestRouter_DedupeKeyDistinct(t *testing.T) {
	channels := map[string]*mockRouterChannel{
		"router_mock_a": {typ: "router_mock_a"},
		"router_mock_b": {typ: "router_mock_b"},
	}
	router := NewRouter(
		newRouterTestRegistry(channels),
		&mockRouterOutbox{},
		mockConfLister{confs: newRouterTestConfs(1, 2)},
	)

	err := router.Route(context.Background(), Notification{Title: "PT", Text: "done"}, RouteScope{
		ConfIDs:   []uint{1, 2},
		EventType: "torrent.completed",
		PrimaryID: "torrent-1",
	})

	require.NoError(t, err)
	require.Equal(t, 1, channels["router_mock_a"].SendCalls())
	require.Equal(t, 1, channels["router_mock_b"].SendCalls())
}

func TestRouter_PartialFailure(t *testing.T) {
	channels := map[string]*mockRouterChannel{
		"router_mock_a": {typ: "router_mock_a", sendErr: errors.New("send failed")},
		"router_mock_b": {typ: "router_mock_b"},
		"router_mock_c": {typ: "router_mock_c"},
	}
	outbox := &mockRouterOutbox{}
	router := NewRouter(
		newRouterTestRegistry(channels),
		outbox,
		mockConfLister{confs: newRouterTestConfs(1, 2, 3)},
	)

	err := router.Route(context.Background(), Notification{Title: "PT", Text: "done"}, RouteScope{
		ConfIDs:    []uint{1, 2, 3},
		EventType:  "torrent.completed",
		PrimaryID:  "torrent-1",
		SkipDedupe: true,
	})

	require.Error(t, err)
	require.Equal(t, 1, channels["router_mock_a"].SendCalls())
	require.Equal(t, 1, channels["router_mock_b"].SendCalls())
	require.Equal(t, 1, channels["router_mock_c"].SendCalls())
	entries := outbox.Entries()
	require.Len(t, entries, 1)
	require.Equal(t, uint(1), entries[0].confID)
	require.Equal(t, "send failed", entries[0].errMsg)
}

// errConfLister is a ConfLister whose ListNotificationConfs always fails,
// exercising the targetConfs error path in Route.
type errConfLister struct{ err error }

func (e errConfLister) ListNotificationConfs(_ context.Context) ([]models.NotificationConf, error) {
	return nil, e.err
}

// TestRouter_NilReceiver ensures a nil *Router returns an error rather than
// panicking.
func TestRouter_NilReceiver(t *testing.T) {
	var r *Router
	err := r.Route(context.Background(), Notification{}, RouteScope{})
	require.Error(t, err)
}

// TestRouter_NilConfLister covers the guard for a router built without a conf
// lister.
func TestRouter_NilConfLister(t *testing.T) {
	r := NewRouter(NewRegistry(), &mockRouterOutbox{}, nil)
	err := r.Route(context.Background(), Notification{}, RouteScope{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conf lister")
}

// TestRouter_ConfListerError propagates a lister failure.
func TestRouter_ConfListerError(t *testing.T) {
	r := NewRouter(NewRegistry(), &mockRouterOutbox{}, errConfLister{err: errors.New("db down")})
	err := r.Route(context.Background(), Notification{}, RouteScope{ConfIDs: []uint{1}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}

// TestRouter_NoTargets returns nil when no enabled conf matches the scope.
func TestRouter_NoTargets(t *testing.T) {
	r := NewRouter(
		NewRegistry(),
		&mockRouterOutbox{},
		mockConfLister{confs: []models.NotificationConf{{ID: 1, Enabled: false}}},
	)
	err := r.Route(context.Background(), Notification{}, RouteScope{ConfIDs: []uint{1}})
	require.NoError(t, err)
}

// TestRouter_NilContextDefaulted verifies Route tolerates a nil context.
func TestRouter_NilContextDefaulted(t *testing.T) {
	channel := &mockRouterChannel{typ: "router_ctx"}
	registry := NewRegistry()
	registry.Register("router_ctx", func() Channel { return channel })
	r := NewRouter(registry, &mockRouterOutbox{}, mockConfLister{confs: []models.NotificationConf{
		{ID: 1, ChannelType: "router_ctx", Enabled: true},
	}})

	//nolint:staticcheck // deliberately passing nil ctx to exercise the guard
	err := r.Route(nil, Notification{Title: "x"}, RouteScope{ConfIDs: []uint{1}, SkipDedupe: true})
	require.NoError(t, err)
	assert.Equal(t, 1, channel.SendCalls())
}

// TestRouter_ChannelMakeError_Unhealthy covers the channel() failure path: an
// unknown channel type makes Make fail, the conf is cached as unhealthy, and
// the failure is enqueued to the outbox. A second Route hits the cached
// unhealthy branch.
func TestRouter_ChannelMakeError_Unhealthy(t *testing.T) {
	outbox := &mockRouterOutbox{}
	r := NewRouter(
		NewRegistry(), // empty → Make always fails
		outbox,
		mockConfLister{confs: []models.NotificationConf{
			{ID: 1, ChannelType: "does_not_exist", Enabled: true},
		}},
	)

	err := r.Route(context.Background(), Notification{Title: "x"}, RouteScope{ConfIDs: []uint{1}, SkipDedupe: true})
	require.Error(t, err)

	err = r.Route(context.Background(), Notification{Title: "y"}, RouteScope{ConfIDs: []uint{1}, SkipDedupe: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不健康")

	require.Len(t, outbox.Entries(), 2)
}

// TestRouter_ChannelReuse verifies a successfully initialised channel is cached
// and Init is called only once across multiple routes.
func TestRouter_ChannelReuse(t *testing.T) {
	channel := &mockRouterChannel{typ: "router_reuse"}
	registry := NewRegistry()
	registry.Register("router_reuse", func() Channel { return channel })
	r := NewRouter(registry, &mockRouterOutbox{}, mockConfLister{confs: []models.NotificationConf{
		{ID: 1, ChannelType: "router_reuse", Enabled: true},
	}})

	for i := 0; i < 3; i++ {
		require.NoError(t, r.Route(context.Background(), Notification{Title: "x"},
			RouteScope{ConfIDs: []uint{1}, SkipDedupe: true}))
	}
	channel.mu.Lock()
	initCalls := channel.initCalls
	channel.mu.Unlock()
	assert.Equal(t, 1, initCalls, "channel should be initialised exactly once and reused")
	assert.Equal(t, 3, channel.SendCalls())
}

// TestRouter_EnqueueFailure_NoOutbox verifies that with no outbox the original
// send error is returned unchanged.
func TestRouter_EnqueueFailure_NoOutbox(t *testing.T) {
	channel := &mockRouterChannel{typ: "router_noout", sendErr: errors.New("send boom")}
	registry := NewRegistry()
	registry.Register("router_noout", func() Channel { return channel })
	r := NewRouter(registry, nil, mockConfLister{confs: []models.NotificationConf{
		{ID: 1, ChannelType: "router_noout", Enabled: true},
	}})

	err := r.Route(context.Background(), Notification{Title: "x"}, RouteScope{ConfIDs: []uint{1}, SkipDedupe: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "send boom")
}

// TestDedupCache_Seen_Guards covers the nil/empty guard clauses of Seen.
func TestDedupCache_Seen_Guards(t *testing.T) {
	var nilCache *DedupCache
	assert.False(t, nilCache.Seen("e", "p", 1))

	d := NewDedupCache(10, time.Minute)
	assert.False(t, d.Seen("", "p", 1))
	assert.False(t, d.Seen("e", "", 1))
	assert.False(t, d.Seen("e", "p", 0))
}

// TestDedupCache_ExpiryWindow verifies that an entry seen outside the window is
// no longer treated as a duplicate (exercising the re-seen branch).
func TestDedupCache_ExpiryWindow(t *testing.T) {
	d := NewDedupCache(10, 20*time.Millisecond)
	assert.False(t, d.Seen("evt", "id", 1), "first observation is not a dup")
	assert.True(t, d.Seen("evt", "id", 1), "immediate re-observation is a dup")
	time.Sleep(40 * time.Millisecond)
	assert.False(t, d.Seen("evt", "id", 1), "after the window elapses it is fresh again")
}

// TestDedupCache_EvictOldest forces capacity-based eviction: with capacity 2,
// inserting three distinct keys must evict the least-recently-used one.
func TestDedupCache_EvictOldest(t *testing.T) {
	d := NewDedupCache(2, time.Hour)
	require.False(t, d.Seen("e", "a", 1))
	require.False(t, d.Seen("e", "b", 1))
	require.False(t, d.Seen("e", "c", 1)) // triggers evictOldest for "a"

	// "a" was evicted, so observing it again is fresh (not a dup).
	assert.False(t, d.Seen("e", "a", 1))
	// "c" is still cached.
	assert.True(t, d.Seen("e", "c", 1))
}

// TestNewDedupCache_Defaults covers the <=0 default substitution branch.
func TestNewDedupCache_Defaults(t *testing.T) {
	d := NewDedupCache(0, 0)
	require.NotNil(t, d)
	assert.Equal(t, dedupCacheCap, d.capacity)
	assert.Equal(t, dedupWindow, d.window)
}
