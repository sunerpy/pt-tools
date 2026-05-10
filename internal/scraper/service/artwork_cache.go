// Package service provides helper services for the scraper pipeline.
//
// artwork_cache.go implements an on-disk LRU cache for artwork downloads.
// Keys are SHA1(url) hex strings; values are files named "<key><ext>" in
// the cache directory. Eviction is mtime-based (oldest first) when the
// total cache size exceeds the configured maxSize.
package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// FSCache is a simple filesystem-backed LRU cache for artwork files.
//
// It is safe for concurrent use.
type FSCache struct {
	dir     string
	maxSize int64
	mu      sync.Mutex
}

// NewFSCache creates a new FSCache rooted at dir with the given maxSize
// (bytes). The directory is created if it does not exist.
func NewFSCache(dir string, maxSize int64) (*FSCache, error) {
	if dir == "" {
		return nil, errors.New("fs cache: dir is empty")
	}
	if maxSize <= 0 {
		return nil, errors.New("fs cache: maxSize must be > 0")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("fs cache: mkdir %s: %w", dir, err)
	}
	return &FSCache{dir: dir, maxSize: maxSize}, nil
}

// Dir returns the cache directory.
func (c *FSCache) Dir() string { return c.dir }

// Get returns the cached file path for the given key, or ("", false) if
// the key is not present. Caller should not modify the returned file.
//
// As a side effect, Get updates the file's mtime to mark it recently used,
// so future Evict passes treat it as fresh.
func (c *FSCache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	path, ok := c.findByKeyLocked(key)
	if !ok {
		return "", false
	}
	// Touch mtime for LRU bookkeeping (best-effort).
	_ = touchFile(path)
	return path, true
}

// Put writes content to the cache under the given key. If ext is empty,
// ".jpg" is used. Returns the absolute path of the cached file.
//
// After writing, if the total cache size exceeds maxSize, the oldest
// files are evicted (excluding the just-written file when possible).
func (c *FSCache) Put(key string, content []byte, ext string) (string, error) {
	if key == "" {
		return "", errors.New("fs cache: key is empty")
	}
	if ext == "" {
		ext = ".jpg"
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Drop any pre-existing entry with a different extension for the same key.
	if existing, ok := c.findByKeyLocked(key); ok {
		_ = os.Remove(existing)
	}

	path := filepath.Join(c.dir, key+ext)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", fmt.Errorf("fs cache: write %s: %w", path, err)
	}

	// Enforce size ceiling.
	size, err := c.sizeLocked()
	if err != nil {
		return "", err
	}
	if size > c.maxSize {
		if err := c.evictLocked(c.maxSize, path); err != nil {
			return "", err
		}
	}

	return path, nil
}

// Size returns the current total size of the cache in bytes.
func (c *FSCache) Size() (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sizeLocked()
}

// Evict trims the cache so that its total size is <= targetSize.
// Oldest files (by mtime) are removed first.
func (c *FSCache) Evict(targetSize int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.evictLocked(targetSize, "")
}

// --- internal helpers ---

type cacheEntry struct {
	path  string
	size  int64
	mtime int64
}

func (c *FSCache) listEntriesLocked() ([]cacheEntry, error) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return nil, fmt.Errorf("fs cache: readdir: %w", err)
	}
	out := make([]cacheEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, cacheEntry{
			path:  filepath.Join(c.dir, e.Name()),
			size:  info.Size(),
			mtime: info.ModTime().UnixNano(),
		})
	}
	return out, nil
}

func (c *FSCache) sizeLocked() (int64, error) {
	es, err := c.listEntriesLocked()
	if err != nil {
		return 0, err
	}
	var total int64
	for _, e := range es {
		total += e.size
	}
	return total, nil
}

func (c *FSCache) findByKeyLocked(key string) (string, bool) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return "", false
	}
	prefix := key
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) < len(prefix) {
			continue
		}
		if name[:len(prefix)] != prefix {
			continue
		}
		// Ensure the next character is '.' (extension boundary) or EOF.
		if len(name) == len(prefix) || name[len(prefix)] == '.' {
			return filepath.Join(c.dir, name), true
		}
	}
	return "", false
}

func (c *FSCache) evictLocked(targetSize int64, keep string) error {
	es, err := c.listEntriesLocked()
	if err != nil {
		return err
	}
	// Oldest first.
	sort.Slice(es, func(i, j int) bool { return es[i].mtime < es[j].mtime })

	var total int64
	for _, e := range es {
		total += e.size
	}
	for _, e := range es {
		if total <= targetSize {
			break
		}
		if e.path == keep {
			continue
		}
		if err := os.Remove(e.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("fs cache: remove %s: %w", e.path, err)
		}
		total -= e.size
	}
	return nil
}

// touchFile updates the mtime/atime of path to now.
func touchFile(path string) error {
	now := nowFunc()
	return os.Chtimes(path, now, now)
}
