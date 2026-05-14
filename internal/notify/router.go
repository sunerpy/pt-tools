package notify

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/models"
)

const (
	routerTimeout     = 5 * time.Second
	dedupWindow       = 60 * time.Second
	dedupCacheCap     = 1000
	dedupKeySeparator = "\x00"
)

// OutboxEnqueuer persists failed deliveries for asynchronous retry.
type OutboxEnqueuer interface {
	Enqueue(ctx context.Context, confID uint, n Notification, errMsg string) error
}

// ConfLister lists notification channel configurations available to the router.
type ConfLister interface {
	ListNotificationConfs(ctx context.Context) ([]models.NotificationConf, error)
}

// Router fans a notification out to enabled channel configurations, while
// keeping per-configuration channel instances private and reusable.
type Router struct {
	registry *Registry
	outbox   OutboxEnqueuer
	confs    ConfLister
	dedup    *DedupCache

	mu        sync.Mutex
	channels  map[uint]Channel
	unhealthy map[uint]struct{}
}

type RouteScope struct {
	ConfIDs    []uint
	EventType  string
	PrimaryID  string
	SkipDedupe bool
}

func NewRouter(registry *Registry, outbox OutboxEnqueuer, confs ConfLister) *Router {
	if registry == nil {
		registry = DefaultRegistry()
	}
	return &Router{
		registry:  registry,
		outbox:    outbox,
		confs:     confs,
		dedup:     NewDedupCache(dedupCacheCap, dedupWindow),
		channels:  make(map[uint]Channel),
		unhealthy: make(map[uint]struct{}),
	}
}

func (r *Router) Route(ctx context.Context, n Notification, scope RouteScope) error {
	if r == nil {
		return errors.New("notify router is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if r.confs == nil {
		return errors.New("notify router conf lister is nil")
	}

	targets, err := r.targetConfs(ctx, scope)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}

	routeCtx, cancel := context.WithTimeout(ctx, routerTimeout)
	defer cancel()

	results := make(chan error, len(targets))
	var wg sync.WaitGroup
	for _, conf := range targets {
		conf := conf
		if !scope.SkipDedupe && r.dedup.Seen(scope.EventType, scope.PrimaryID, conf.ID) {
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.deliver(routeCtx, conf, n); err != nil {
				results <- err
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		close(results)
	case <-routeCtx.Done():
		return routeCtx.Err()
	}

	var errs []error
	for err := range results {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (r *Router) targetConfs(ctx context.Context, scope RouteScope) ([]models.NotificationConf, error) {
	confs, err := r.confs.ListNotificationConfs(ctx)
	if err != nil {
		return nil, fmt.Errorf("加载通知通道配置失败: %w", err)
	}

	allowed := make(map[uint]struct{}, len(scope.ConfIDs))
	for _, id := range scope.ConfIDs {
		allowed[id] = struct{}{}
	}
	useAllowList := len(allowed) > 0

	targets := make([]models.NotificationConf, 0, len(confs))
	for _, conf := range confs {
		if !conf.Enabled {
			continue
		}
		if useAllowList {
			if _, ok := allowed[conf.ID]; !ok {
				continue
			}
		}
		targets = append(targets, conf)
	}
	return targets, nil
}

func (r *Router) deliver(ctx context.Context, conf models.NotificationConf, n Notification) error {
	ch, err := r.channel(ctx, conf)
	if err != nil {
		return r.enqueueFailure(ctx, conf, n, err)
	}

	n.ChannelType = conf.ChannelType
	n.SourceConfID = conf.ID
	if err := ch.Send(ctx, n); err != nil {
		return r.enqueueFailure(ctx, conf, n, err)
	}
	return nil
}

func (r *Router) channel(ctx context.Context, conf models.NotificationConf) (Channel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ch, ok := r.channels[conf.ID]; ok {
		return ch, nil
	}
	if _, ok := r.unhealthy[conf.ID]; ok {
		return nil, fmt.Errorf("通知通道配置 %d 不健康", conf.ID)
	}

	ch, err := r.registry.Make(conf.ChannelType)
	if err != nil {
		r.unhealthy[conf.ID] = struct{}{}
		return nil, err
	}
	if err := ch.Init(ctx, &conf); err != nil {
		r.unhealthy[conf.ID] = struct{}{}
		return nil, err
	}
	r.channels[conf.ID] = ch
	return ch, nil
}

func (r *Router) enqueueFailure(ctx context.Context, conf models.NotificationConf, n Notification, cause error) error {
	if r.outbox == nil {
		return cause
	}
	n.ChannelType = conf.ChannelType
	n.SourceConfID = conf.ID
	errMsg := truncateError(cause)
	if err := r.outbox.Enqueue(ctx, conf.ID, n, errMsg); err != nil {
		return errors.Join(cause, fmt.Errorf("通知失败入 outbox 失败: %w", err))
	}
	return cause
}

type DedupCache struct {
	mu       sync.Mutex
	capacity int
	window   time.Duration
	items    map[string]*list.Element
	order    *list.List
}

type dedupEntry struct {
	key  string
	seen time.Time
}

func NewDedupCache(capacity int, window time.Duration) *DedupCache {
	if capacity <= 0 {
		capacity = dedupCacheCap
	}
	if window <= 0 {
		window = dedupWindow
	}
	return &DedupCache{
		capacity: capacity,
		window:   window,
		items:    make(map[string]*list.Element, capacity),
		order:    list.New(),
	}
}

func (d *DedupCache) Seen(eventType, primaryID string, confID uint) bool {
	if d == nil || eventType == "" || primaryID == "" || confID == 0 {
		return false
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	key := strings.Join([]string{eventType, primaryID, fmt.Sprint(confID)}, dedupKeySeparator)
	if elem, ok := d.items[key]; ok {
		entry := elem.Value.(*dedupEntry)
		if now.Sub(entry.seen) < d.window {
			d.order.MoveToFront(elem)
			return true
		}
		entry.seen = now
		d.order.MoveToFront(elem)
		return false
	}

	d.items[key] = d.order.PushFront(&dedupEntry{key: key, seen: now})
	d.evictExpired(now)
	for len(d.items) > d.capacity {
		d.evictOldest()
	}
	return false
}

func (d *DedupCache) evictExpired(now time.Time) {
	for elem := d.order.Back(); elem != nil; {
		prev := elem.Prev()
		entry := elem.Value.(*dedupEntry)
		if now.Sub(entry.seen) < d.window {
			return
		}
		delete(d.items, entry.key)
		d.order.Remove(elem)
		elem = prev
	}
}

func (d *DedupCache) evictOldest() {
	elem := d.order.Back()
	if elem == nil {
		return
	}
	entry := elem.Value.(*dedupEntry)
	delete(d.items, entry.key)
	d.order.Remove(elem)
}
