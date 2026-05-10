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

// testScannerConfig 构造一个测试用的宽松 Config：size=1 byte、age=0 —— 让
// 单测不需要写 10 MB 文件也能通过稳定性 guard。生产代码的默认值保持严格。
func testScannerConfig(libs []ScanLibrary, onFound FoundHandler) ScannerConfig {
	return ScannerConfig{
		Libraries:      libs,
		OnFound:        onFound,
		DebounceWindow: 100 * time.Millisecond,
		MinFileSize:    1,
		MinFileAge:     0,
	}
}

func TestScanner_InitialScanEmitsExisting(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "movie.mkv"), []byte("data"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("data"), 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	found, foundMu := newFoundCollector()
	s, err := NewScanner(testScannerConfig(
		[]ScanLibrary{{ID: 1, Name: "test", Path: tmpDir}},
		found,
	))
	require.NoError(t, err)

	require.NoError(t, s.Start(ctx))
	require.NoError(t, s.Stop())

	paths := foundMu.snapshot()
	require.Len(t, paths, 1)
	require.Contains(t, paths[0].path, "movie.mkv")
	require.Equal(t, uint(1), paths[0].libID, "OnFound 应收到库 ID")
}

func TestScanner_NewFileTriggerFsnotify(t *testing.T) {
	tmpDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	found, foundMu := newFoundCollector()
	s, err := NewScanner(testScannerConfig(
		[]ScanLibrary{{ID: 42, Name: "test", Path: tmpDir}},
		found,
	))
	require.NoError(t, err)
	require.NoError(t, s.Start(ctx))
	defer func() { require.NoError(t, s.Stop()) }()

	time.Sleep(100 * time.Millisecond)
	path := filepath.Join(tmpDir, "new.mkv")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))

	require.Eventually(t, func() bool {
		return foundMu.containsPath(path)
	}, 2*time.Second, 20*time.Millisecond)
	require.Equal(t, uint(42), foundMu.lastLibID())
}

func TestScanner_Debounce(t *testing.T) {
	tmpDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var count int32
	cfg := testScannerConfig(
		[]ScanLibrary{{ID: 1, Name: "test", Path: tmpDir}},
		func(_ ScanLibrary, _ string) { atomic.AddInt32(&count, 1) },
	)
	cfg.DebounceWindow = 5 * time.Second
	s, err := NewScanner(cfg)
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
	s, err := NewScanner(testScannerConfig(
		[]ScanLibrary{{ID: 1, Name: "test", Path: tmpDir}},
		func(_ ScanLibrary, _ string) { atomic.AddInt32(&count, 1) },
	))
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

	s, err := NewScanner(testScannerConfig(
		[]ScanLibrary{{ID: 1, Name: "test", Path: tmpDir}},
		func(_ ScanLibrary, _ string) {},
	))
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
	s, err := NewScanner(testScannerConfig(
		[]ScanLibrary{{ID: 1, Name: "test", Path: tmpDir}},
		found,
	))
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
		return foundMu.containsPath(path)
	}, 2*time.Second, 20*time.Millisecond)
}

// TestScanner_FileStabilityGuardRejectsSmall 验证小于 MinFileSize 的文件被 guard 拒绝。
func TestScanner_FileStabilityGuardRejectsSmall(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "small.mkv"), []byte("tiny"), 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var count int32
	cfg := ScannerConfig{
		Libraries:      []ScanLibrary{{ID: 1, Name: "test", Path: tmpDir}},
		OnFound:        func(_ ScanLibrary, _ string) { atomic.AddInt32(&count, 1) },
		DebounceWindow: 100 * time.Millisecond,
		MinFileSize:    1024,
		MinFileAge:     0,
	}
	s, err := NewScanner(cfg)
	require.NoError(t, err)
	require.NoError(t, s.Start(ctx))
	defer func() { require.NoError(t, s.Stop()) }()

	time.Sleep(200 * time.Millisecond)
	require.Zero(t, atomic.LoadInt32(&count), "小于 MinFileSize 应被过滤")
}

// TestScanner_FileStabilityGuardRejectsFresh 验证 mtime 过新的文件被 guard 拒绝。
func TestScanner_FileStabilityGuardRejectsFresh(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "fresh.mkv")
	require.NoError(t, os.WriteFile(path, make([]byte, 2048), 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var count int32
	cfg := ScannerConfig{
		Libraries:      []ScanLibrary{{ID: 1, Name: "test", Path: tmpDir}},
		OnFound:        func(_ ScanLibrary, _ string) { atomic.AddInt32(&count, 1) },
		DebounceWindow: 100 * time.Millisecond,
		MinFileSize:    1024,
		MinFileAge:     5 * time.Second,
	}
	s, err := NewScanner(cfg)
	require.NoError(t, err)
	require.NoError(t, s.Start(ctx))
	defer func() { require.NoError(t, s.Stop()) }()

	time.Sleep(200 * time.Millisecond)
	require.Zero(t, atomic.LoadInt32(&count), "mtime < MinFileAge 应被过滤")
}

// TestScanner_DoFullScan 手动扫描指定库；支持 0 = 所有。
func TestScanner_DoFullScan(t *testing.T) {
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir1, "a.mkv"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir2, "b.mkv"), []byte("x"), 0o644))

	found, foundMu := newFoundCollector()
	s, err := NewScanner(testScannerConfig(
		[]ScanLibrary{
			{ID: 1, Name: "lib1", Path: tmpDir1},
			{ID: 2, Name: "lib2", Path: tmpDir2},
		},
		found,
	))
	require.NoError(t, err)

	// 不调 Start，直接调 DoFullScan（测试同步 walk 能力）
	n := s.DoFullScan(1)
	assert.Equal(t, 1, n)
	require.Len(t, foundMu.snapshot(), 1)
	require.Equal(t, uint(1), foundMu.snapshot()[0].libID)

	n2 := s.DoFullScan(0)
	assert.Equal(t, 2, n2) // 扫描了全部 2 个库
	require.GreaterOrEqual(t, len(foundMu.snapshot()), 2)
}

// TestScanner_MatchLibraryLongestPrefix 验证多库重叠时按最长前缀匹配。
func TestScanner_MatchLibraryLongestPrefix(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "media")
	nested := filepath.Join(base, "media", "movies")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	s, err := NewScanner(testScannerConfig(
		[]ScanLibrary{
			{ID: 1, Name: "parent", Path: parent},
			{ID: 2, Name: "nested", Path: nested},
		},
		func(_ ScanLibrary, _ string) {},
	))
	require.NoError(t, err)

	lib := s.matchLibrary(filepath.Join(nested, "a.mkv"))
	require.NotNil(t, lib)
	assert.Equal(t, uint(2), lib.ID, "应匹配更深的 nested 库")

	lib2 := s.matchLibrary(filepath.Join(parent, "b.mkv"))
	require.NotNil(t, lib2)
	assert.Equal(t, uint(1), lib2.ID, "应匹配 parent 库")
}

type foundEntry struct {
	libID uint
	path  string
}

type foundCollector struct {
	mu      sync.Mutex
	entries []foundEntry
}

func newFoundCollector() (FoundHandler, *foundCollector) {
	collector := &foundCollector{}
	return func(lib ScanLibrary, path string) {
		collector.mu.Lock()
		defer collector.mu.Unlock()
		collector.entries = append(collector.entries, foundEntry{libID: lib.ID, path: path})
	}, collector
}

func (c *foundCollector) snapshot() []foundEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]foundEntry(nil), c.entries...)
}

func (c *foundCollector) containsPath(path string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, item := range c.entries {
		if item.path == path {
			return true
		}
	}
	return false
}

func (c *foundCollector) lastLibID() uint {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) == 0 {
		return 0
	}
	return c.entries[len(c.entries)-1].libID
}

func TestScanner_ShouldEmitCleanup(t *testing.T) {
	s, err := NewScanner(ScannerConfig{
		Libraries:      []ScanLibrary{{ID: 1, Name: "test", Path: t.TempDir()}},
		OnFound:        func(_ ScanLibrary, _ string) {},
		DebounceWindow: 10 * time.Millisecond,
		MinFileSize:    1,
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
