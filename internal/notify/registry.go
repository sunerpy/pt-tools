package notify

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

var ErrChannelTypeUnknown = errors.New("未知通知通道类型")

type Factory func() Channel

type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

var defaultRegistry = NewRegistry()

func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]Factory)}
}

func (r *Registry) Register(typ string, factory Factory) {
	if r == nil {
		panic("notify: Register called on nil registry")
	}
	if typ == "" {
		panic("notify: Register called with empty channel type")
	}
	if factory == nil {
		panic(fmt.Sprintf("notify: Register called with nil factory for %q", typ))
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[typ]; exists {
		panic(fmt.Sprintf("notify: channel type %q already registered", typ))
	}
	r.factories[typ] = factory
}

func (r *Registry) Make(typ string) (Channel, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: %s", ErrChannelTypeUnknown, typ)
	}

	r.mu.RLock()
	factory, ok := r.factories[typ]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrChannelTypeUnknown, typ)
	}

	ch := factory()
	if ch == nil {
		return nil, fmt.Errorf("通知通道工厂返回空实例: %s", typ)
	}
	return ch, nil
}

func (r *Registry) Types() []string {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.factories))
	for typ := range r.factories {
		types = append(types, typ)
	}
	sort.Strings(types)
	return types
}

func RegisterChannel(typ string, factory Factory) {
	defaultRegistry.Register(typ, factory)
}

func DefaultRegistry() *Registry { return defaultRegistry }
