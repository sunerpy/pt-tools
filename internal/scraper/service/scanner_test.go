package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanner_InitialScanEmitsExisting(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "movie.mkv"), []byte("data"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("data"), 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	found, foundMu := newFoundCollector()
	s, err := NewScanner(ScannerConfig{
		Libraries: []ScanLibrary{{Name: "test", Path: tmpDir}},
		OnFound:   found,
	})
	require.NoError(t, err)

	require.NoError(t, s.Start(ctx))
	require.NoError(t, s.Stop())

	paths := foundMu.snapshot()
	require.Len(t, paths, 1)
	require.Contains(t, paths[0], "movie.mkv")
}

func TestScanner_NewFileTriggerFsnotify(t *testing.T) {
	tmpDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	found, foundMu := newFoundCollector()
	s, err := NewScanner(ScannerConfig{
		Libraries:      []ScanLibrary{{Name: "test", Path: tmpDir}},
		OnFound:        found,
		DebounceWindow: 100 * time.Millisecond,
	})
	require.NoError(t, err)
	require.NoError(t, s.Start(ctx))
	defer func() { require.NoError(t, s.Stop()) }()

	time.Sleep(100 * time.Millisecond)
	path := filepath.Join(tmpDir, "new.mkv")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))

	require.Eventually(t, func() bool {
		return foundMu.contains(path)
	}, 2*time.Second, 20*time.Millisecond)
}

func TestScanner_Debounce(t *testing.T) {
	tmpDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var count int32
	s, err := NewScanner(ScannerConfig{
		Libraries:      []ScanLibrary{{Name: "test", Path: tmpDir}},
		OnFound:        func(string) { atomic.AddInt32(&count, 1) },
		DebounceWindow: 5 * time.Second,
	})
	require.NoError(t, err)
	require.NoError(t, s.Start(ctx))
	defer func() { require.NoError(t, s.Stop()) }()

	path := filepath.Join(tmpDir, "x.mkv")
	time.Sleep(100 * time.Millisecond)
	for i := 0; i < 5; i++ {
		require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf("v%d", i)), 0o644))
		time.Sleep(50 * time.Millisecond)
	}

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&count) >= 1
	}, 2*time.Second, 20*time.Millisecond)
	require.LessOrEqual(t, atomic.LoadInt32(&count), int32(2), "debounce should limit emissions")
}

func TestScanner_NonVideoIgnored(t *testing.T) {
	tmpDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var count int32
	s, err := NewScanner(ScannerConfig{
		Libraries:      []ScanLibrary{{Name: "test", Path: tmpDir}},
		OnFound:        func(string) { atomic.AddInt32(&count, 1) },
		DebounceWindow: 100 * time.Millisecond,
	})
	require.NoError(t, err)
	require.NoError(t, s.Start(ctx))
	defer func() { require.NoError(t, s.Stop()) }()

	time.Sleep(100 * time.Millisecond)
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "doc.txt"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file.pdf"), []byte("x"), 0o644))
	time.Sleep(300 * time.Millisecond)

	require.Zero(t, atomic.LoadInt32(&count))
}

func TestScanner_ContextCancel(t *testing.T) {
	tmpDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())

	s, err := NewScanner(ScannerConfig{
		Libraries: []ScanLibrary{{Name: "test", Path: tmpDir}},
		OnFound:   func(string) {},
	})
	require.NoError(t, err)

	baseline := runtime.NumGoroutine()
	require.NoError(t, s.Start(ctx))

	cancel()
	require.NoError(t, s.Stop())

	require.Eventually(t, func() bool {
		return runtime.NumGoroutine() <= baseline+1
	}, 2*time.Second, 20*time.Millisecond)
}

func TestScanner_RecursiveSubdir(t *testing.T) {
	tmpDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	found, foundMu := newFoundCollector()
	s, err := NewScanner(ScannerConfig{
		Libraries:      []ScanLibrary{{Name: "test", Path: tmpDir}},
		OnFound:        found,
		DebounceWindow: 100 * time.Millisecond,
	})
	require.NoError(t, err)
	require.NoError(t, s.Start(ctx))
	defer func() { require.NoError(t, s.Stop()) }()

	time.Sleep(100 * time.Millisecond)
	subDir := filepath.Join(tmpDir, "sub")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	time.Sleep(200 * time.Millisecond)

	path := filepath.Join(subDir, "a.mkv")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))

	require.Eventually(t, func() bool {
		return foundMu.contains(path)
	}, 2*time.Second, 20*time.Millisecond)
}

type foundCollector struct {
	mu    sync.Mutex
	paths []string
}

func newFoundCollector() (FoundHandler, *foundCollector) {
	collector := &foundCollector{}
	return func(path string) {
		collector.mu.Lock()
		defer collector.mu.Unlock()
		collector.paths = append(collector.paths, path)
	}, collector
}

func (c *foundCollector) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.paths...)
}

func (c *foundCollector) contains(path string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, item := range c.paths {
		if item == path {
			return true
		}
	}
	return false
}

func TestScanner_ShouldEmitCleanup(t *testing.T) {
	s, err := NewScanner(ScannerConfig{
		Libraries:      []ScanLibrary{{Name: "test", Path: t.TempDir()}},
		OnFound:        func(string) {},
		DebounceWindow: 10 * time.Millisecond,
	})
	require.NoError(t, err)

	s.mu.Lock()
	s.recentEvents["old.mkv"] = time.Now().Add(-200 * time.Millisecond)
	s.mu.Unlock()

	assert.True(t, s.shouldEmit("new.mkv"))

	s.mu.Lock()
	_, exists := s.recentEvents["old.mkv"]
	s.mu.Unlock()
	assert.False(t, exists)
}
