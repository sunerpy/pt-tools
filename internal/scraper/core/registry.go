package core

import (
	"fmt"
	"sort"
	"sync"
)

// Registry is a generic registry for managing factory functions.
// T is the interface type stored by the registry; factories return new instances of T.
// Registry uses RWMutex for safe concurrent access (read-heavy workload).
type Registry[T any] struct {
	mu      sync.RWMutex
	entries map[string]func() T
}

// NewRegistry creates a new registry for type T.
func NewRegistry[T any]() *Registry[T] {
	return &Registry[T]{
		entries: make(map[string]func() T),
	}
}

// Register registers a factory function for a given name.
// Returns ErrInvalidID if name is empty or factory is nil.
// Returns ErrAlreadyExists if name is already registered.
func (r *Registry[T]) Register(name string, factory func() T) error {
	if name == "" {
		return fmt.Errorf("%w: empty name", ErrInvalidID)
	}
	if factory == nil {
		return fmt.Errorf("%w: nil factory for %q", ErrInvalidID, name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.entries[name]; exists {
		return fmt.Errorf("%w: %q already registered", ErrAlreadyExists, name)
	}

	r.entries[name] = factory
	return nil
}

// Get retrieves an instance from the registry by name.
// Calls the factory function to create a new instance.
// Returns ErrNotFound if name is not registered.
func (r *Registry[T]) Get(name string) (T, error) {
	r.mu.RLock()
	factory, ok := r.entries[name]
	r.mu.RUnlock()

	var zero T
	if !ok {
		return zero, fmt.Errorf("%w: %q not registered", ErrNotFound, name)
	}

	return factory(), nil
}

// List returns all registered names in alphabetically sorted order.
// Useful for deterministic testing and debugging.
func (r *Registry[T]) List() []string {
	r.mu.RLock()
	names := make([]string, 0, len(r.entries))
	for n := range r.entries {
		names = append(names, n)
	}
	r.mu.RUnlock()

	sort.Strings(names)
	return names
}

// Has checks whether a name is registered.
func (r *Registry[T]) Has(name string) bool {
	r.mu.RLock()
	_, ok := r.entries[name]
	r.mu.RUnlock()

	return ok
}

// Type aliases for improved readability.
// Each registry is specialized for a specific interface type.

// ScraperRegistry stores MovieMetadataScraper, TvShowMetadataScraper, and ArtworkScraper implementations.
type ScraperRegistry = Registry[MediaScraper]

// ConnectorRegistry stores MediaServerConnector implementations.
type ConnectorRegistry = Registry[MediaServerConnector]

// WriterRegistry stores NfoWriter implementations.
type WriterRegistry = Registry[NfoWriter]
