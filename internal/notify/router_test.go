package notify

import (
	"context"
	"errors"
	"sync"
	"testing"

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
