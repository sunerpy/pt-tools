package notify

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

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

// TestNewDigestBuffer_Defaults verifies the convenience constructor wires the
// default window/threshold.
func TestNewDigestBuffer_Defaults(t *testing.T) {
	b := NewDigestBuffer(context.Background(), func(_ context.Context, _ uint, _ []DigestItem) {})
	require.NotNil(t, b)
	assert.Equal(t, DigestWindow, b.window)
	assert.Equal(t, DigestThreshold, b.threshold)
}

// TestNewDigestBufferWithWindow_DefaultsOnNonPositive covers the <=0 branches.
func TestNewDigestBufferWithWindow_DefaultsOnNonPositive(t *testing.T) {
	b := NewDigestBufferWithWindow(context.Background(), func(_ context.Context, _ uint, _ []DigestItem) {}, 0, 0)
	assert.Equal(t, DigestWindow, b.window)
	assert.Equal(t, DigestThreshold, b.threshold)
}

// TestNewOutboxWorker_Defaults covers the interval default and nil-registry
// substitution.
func TestNewOutboxWorker_Defaults(t *testing.T) {
	w := NewOutboxWorker(nil, nil, 0)
	require.NotNil(t, w)
	assert.Equal(t, defaultInterval, w.interval)
	assert.NotNil(t, w.registry)
}

// TestOutboxWorker_Tick_Guards covers the nil-worker, nil-db and nil-registry
// guards of Tick.
func TestOutboxWorker_Tick_Guards(t *testing.T) {
	var nilWorker *OutboxWorker
	require.Error(t, nilWorker.Tick(context.Background()))

	require.Error(t, (&OutboxWorker{}).Tick(context.Background()))

	require.Error(t, (&OutboxWorker{db: &gorm.DB{}, registry: nil}).Tick(context.Background()))
}

// TestOutboxWorker_DeliverOne_ConfMissing seeds a row whose conf row was
// deleted, so deliverOne's First(&conf) fails and the row moves to backoff.
func TestOutboxWorker_DeliverOne_ConfMissing(t *testing.T) {
	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	withFixedNow(t, now)
	db := newOutboxTestDB(t)
	ch := &mockOutboxChannel{}
	worker := newOutboxWorkerWithMock(t, db, ch)

	row := models.NotificationOutbox{
		NotificationConfID: 99999, // no such conf
		PayloadJSON:        `{"title":"x"}`,
		Status:             "pending",
		RetryCount:         0,
		NextRetryAt:        now.Add(-time.Second),
	}
	require.NoError(t, db.Create(&row).Error)

	require.NoError(t, worker.Tick(context.Background()))

	var got models.NotificationOutbox
	require.NoError(t, db.First(&got, row.ID).Error)
	assert.Equal(t, "pending", got.Status)
	assert.Equal(t, 1, got.RetryCount)
	assert.Contains(t, got.ErrorMsg, "加载通知通道配置失败")
}

// TestOutboxWorker_DeliverOne_MalformedPayload seeds a valid conf but an invalid
// payload JSON so json.Unmarshal fails inside deliverOne.
func TestOutboxWorker_DeliverOne_MalformedPayload(t *testing.T) {
	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	withFixedNow(t, now)
	db := newOutboxTestDB(t)
	ch := &mockOutboxChannel{}
	worker := newOutboxWorkerWithMock(t, db, ch)

	conf := models.NotificationConf{ChannelType: "mock", Name: "m", Enabled: true}
	require.NoError(t, db.Create(&conf).Error)
	row := models.NotificationOutbox{
		NotificationConfID: conf.ID,
		PayloadJSON:        `{bad json`,
		Status:             "pending",
		NextRetryAt:        now.Add(-time.Second),
	}
	require.NoError(t, db.Create(&row).Error)

	require.NoError(t, worker.Tick(context.Background()))

	var got models.NotificationOutbox
	require.NoError(t, db.First(&got, row.ID).Error)
	assert.Contains(t, got.ErrorMsg, "解析通知 payload 失败")
	assert.Equal(t, 0, ch.calls, "malformed payload must not reach Send")
}

// TestOutboxWorker_DeliverOne_InitError covers the channel Init failure path.
func TestOutboxWorker_DeliverOne_InitError(t *testing.T) {
	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	withFixedNow(t, now)
	db := newOutboxTestDB(t)
	ch := &mockOutboxChannel{initErr: errors.New("init boom")}
	worker := newOutboxWorkerWithMock(t, db, ch)
	seedOutbox(t, db, 0, now)

	require.NoError(t, worker.Tick(context.Background()))

	var got models.NotificationOutbox
	require.NoError(t, db.Where("status = ?", "pending").First(&got).Error)
	assert.Contains(t, got.ErrorMsg, "init boom")
	assert.Equal(t, 0, ch.calls)
}

// TestOutboxWorker_Tick_UnknownChannelType covers the registry.Make failure
// branch inside deliverOne.
func TestOutboxWorker_Tick_UnknownChannelType(t *testing.T) {
	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	withFixedNow(t, now)
	db := newOutboxTestDB(t)
	worker := NewOutboxWorker(db, NewRegistry(), 10*time.Millisecond)

	conf := models.NotificationConf{ChannelType: "ghost", Name: "g", Enabled: true}
	require.NoError(t, db.Create(&conf).Error)
	row := models.NotificationOutbox{
		NotificationConfID: conf.ID,
		PayloadJSON:        `{"title":"x"}`,
		Status:             "pending",
		NextRetryAt:        now.Add(-time.Second),
	}
	require.NoError(t, db.Create(&row).Error)

	require.NoError(t, worker.Tick(context.Background()))

	var got models.NotificationOutbox
	require.NoError(t, db.First(&got, row.ID).Error)
	assert.Equal(t, "pending", got.Status)
	assert.NotEmpty(t, got.ErrorMsg)
}

// TestTruncateError covers nil, short and over-long error messages.
func TestTruncateError(t *testing.T) {
	assert.Equal(t, "", truncateError(nil))
	assert.Equal(t, "short", truncateError(errors.New("short")))

	long := errors.New(strings.Repeat("x", maxErrorMsgLen+50))
	got := truncateError(long)
	assert.Len(t, got, maxErrorMsgLen)
}

// TestRegistry_MakeNilFactory covers Make returning an error when a factory
// yields nil.
func TestRegistry_MakeNilFactory(t *testing.T) {
	r := NewRegistry()
	r.Register("nilmaker", func() Channel { return nil })
	ch, err := r.Make("nilmaker")
	require.Error(t, err)
	assert.Nil(t, ch)
	assert.Contains(t, err.Error(), "空实例")
}

// TestRegistry_NilReceiver covers the nil-registry branches of Make and Types.
func TestRegistry_NilReceiver(t *testing.T) {
	var r *Registry
	_, err := r.Make("x")
	require.Error(t, err)
	assert.Nil(t, r.Types())
}
