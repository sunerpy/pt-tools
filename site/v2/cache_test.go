package v2

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheEntry_IsExpired(t *testing.T) {
	past := &CacheEntry{Key: "k", Value: "v", ExpiresAt: time.Now().Add(-time.Minute)}
	assert.True(t, past.IsExpired())
	future := &CacheEntry{Key: "k", Value: "v", ExpiresAt: time.Now().Add(time.Minute)}
	assert.False(t, future.IsExpired())
}

func TestLRUCache_SetGetDeleteLen(t *testing.T) {
	c := NewLRUCache(3, time.Minute)
	assert.Equal(t, 0, c.Len())

	c.Set("a", 1)
	c.Set("b", 2)
	assert.Equal(t, 2, c.Len())

	v, ok := c.Get("a")
	assert.True(t, ok)
	assert.Equal(t, 1, v)

	_, ok = c.Get("missing")
	assert.False(t, ok)

	// update existing
	c.Set("a", 100)
	v, ok = c.Get("a")
	assert.True(t, ok)
	assert.Equal(t, 100, v)

	c.Delete("a")
	_, ok = c.Get("a")
	assert.False(t, ok)
	c.Delete("nonexistent") // no-op branch
}

func TestLRUCache_Eviction(t *testing.T) {
	c := NewLRUCache(2, time.Minute)
	c.Set("a", 1)
	c.Set("b", 2)
	// Touch a so b becomes LRU
	c.Get("a")
	c.Set("c", 3) // should evict b
	assert.Equal(t, 2, c.Len())
	_, ok := c.Get("b")
	assert.False(t, ok)
	_, ok = c.Get("a")
	assert.True(t, ok)
	_, ok = c.Get("c")
	assert.True(t, ok)
}

func TestLRUCache_Expiry(t *testing.T) {
	c := NewLRUCache(5, time.Minute)
	c.SetWithTTL("x", 42, 5*time.Millisecond)
	v, ok := c.Get("x")
	assert.True(t, ok)
	assert.Equal(t, 42, v)

	time.Sleep(10 * time.Millisecond)
	_, ok = c.Get("x") // expired -> removed
	assert.False(t, ok)
	assert.Equal(t, 0, c.Len())
}

func TestLRUCache_Cleanup(t *testing.T) {
	c := NewLRUCache(10, time.Minute)
	c.SetWithTTL("a", 1, 5*time.Millisecond)
	c.SetWithTTL("b", 2, 5*time.Millisecond)
	c.SetWithTTL("c", 3, time.Hour)
	time.Sleep(10 * time.Millisecond)

	removed := c.Cleanup()
	assert.Equal(t, 2, removed)
	assert.Equal(t, 1, c.Len())
}

func TestLRUCache_Clear(t *testing.T) {
	c := NewLRUCache(5, time.Minute)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Clear()
	assert.Equal(t, 0, c.Len())
	_, ok := c.Get("a")
	assert.False(t, ok)
}

// fakeL2Cache implements L2Cache for testing.
type fakeL2Cache struct {
	store   map[string][]byte
	getErr  error
	setErr  error
	delErr  error
	deleted []string
}

func newFakeL2() *fakeL2Cache {
	return &fakeL2Cache{store: make(map[string][]byte)}
}

func (f *fakeL2Cache) Get(key string) ([]byte, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	v, ok := f.store[key]
	if !ok {
		return nil, nil
	}
	return v, nil
}

func (f *fakeL2Cache) Set(key string, value []byte, ttl time.Duration) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.store[key] = value
	return nil
}

func (f *fakeL2Cache) Delete(key string) error {
	f.deleted = append(f.deleted, key)
	return f.delErr
}

func TestNewTwoLevelCache_Defaults(t *testing.T) {
	c := NewTwoLevelCache(TwoLevelCacheConfig{})
	assert.Equal(t, 1000, c.l1.capacity)
	assert.Equal(t, 5*time.Minute, c.l1TTL)
	assert.Equal(t, time.Hour, c.l2TTL)
	assert.False(t, c.useL2)
}

func TestTwoLevelCache_L1Only(t *testing.T) {
	c := NewTwoLevelCache(TwoLevelCacheConfig{L1Capacity: 10, L1TTL: time.Minute})

	err := c.Set("k", "value", nil)
	require.NoError(t, err)

	v, ok := c.Get("k", nil)
	assert.True(t, ok)
	assert.Equal(t, "value", v)

	_, ok = c.Get("missing", nil)
	assert.False(t, ok)

	err = c.Delete("k")
	require.NoError(t, err)
	c.Clear()
}

func TestTwoLevelCache_WithL2(t *testing.T) {
	l2 := newFakeL2()
	c := NewTwoLevelCache(TwoLevelCacheConfig{L2Cache: l2, L2TTL: time.Hour})
	assert.True(t, c.useL2)

	marshal := func(v any) ([]byte, error) { return []byte(v.(string)), nil }
	unmarshal := func(b []byte) (any, error) { return string(b), nil }

	err := c.Set("k", "hello", marshal)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), l2.store["k"])

	// Clear L1 so Get falls through to L2 and repopulates L1.
	c.l1.Clear()
	v, ok := c.Get("k", unmarshal)
	assert.True(t, ok)
	assert.Equal(t, "hello", v)
	// Now present in L1
	v2, ok := c.l1.Get("k")
	assert.True(t, ok)
	assert.Equal(t, "hello", v2)

	err = c.Delete("k")
	require.NoError(t, err)
	assert.Contains(t, l2.deleted, "k")
}

func TestTwoLevelCache_L2MarshalError(t *testing.T) {
	l2 := newFakeL2()
	c := NewTwoLevelCache(TwoLevelCacheConfig{L2Cache: l2})
	marshalErr := errors.New("marshal failed")
	err := c.Set("k", "v", func(any) ([]byte, error) { return nil, marshalErr })
	assert.ErrorIs(t, err, marshalErr)
}

func TestTwoLevelCache_L2GetMiss(t *testing.T) {
	l2 := newFakeL2()
	l2.getErr = errors.New("boom")
	c := NewTwoLevelCache(TwoLevelCacheConfig{L2Cache: l2})
	unmarshal := func(b []byte) (any, error) { return string(b), nil }
	_, ok := c.Get("k", unmarshal)
	assert.False(t, ok)
}
