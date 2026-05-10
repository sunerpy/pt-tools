package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

var videoExts = map[string]bool{
	".avi":  true,
	".flv":  true,
	".m2ts": true,
	".mkv":  true,
	".mov":  true,
	".mp4":  true,
	".rmvb": true,
	".ts":   true,
	".webm": true,
	".wmv":  true,
}

// Scanner 稳定性阈值 —— 低于此值 + 最近 30s 内修改的文件视为"还在下载"，跳过本次。
// fsnotify 全量扫描时均应用；可通过 ScannerConfig 覆盖。
const (
	defaultMinFileSize   = 10 * 1024 * 1024 // 10 MB
	defaultMinFileAge    = 30 * time.Second
	defaultFullScanEvery = 6 * time.Hour
	defaultDebounce      = time.Minute
)

// FoundHandler 接收到新视频文件时的回调。带 lib 参数便于调用方区分来源库并
// 直接拿到 LibraryID / Type 做路由（movie vs tv）。
type FoundHandler func(lib ScanLibrary, path string)

// ScanLibrary 扫描目录配置。Type 字段供 OnFound 判定 movie/tv。
type ScanLibrary struct {
	ID        uint
	Name      string
	Path      string
	Type      string // "movie" / "tv" / "mixed"
	Recursive bool
}

// Scanner fsnotify + 定时扫描的组合器。支持热替换库集合（Reload）。
type Scanner struct {
	onFound        FoundHandler
	fullScanEvery  time.Duration
	debounceWindow time.Duration
	minFileSize    int64
	minFileAge     time.Duration

	mu           sync.Mutex
	libraries    []ScanLibrary
	watcher      *fsnotify.Watcher
	recentEvents map[string]time.Time

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	started bool
}

type ScannerConfig struct {
	Libraries      []ScanLibrary
	OnFound        FoundHandler
	FullScanEvery  time.Duration
	DebounceWindow time.Duration

	// MinFileSize / MinFileAge 文件稳定性 guard，防止刮削到下载中的不完整文件。
	// 零值使用默认（10 MB / 30s）。
	MinFileSize int64
	MinFileAge  time.Duration
}

func NewScanner(cfg ScannerConfig) (*Scanner, error) {
	if cfg.OnFound == nil {
		return nil, errors.New("scanner onFound is required")
	}
	if len(cfg.Libraries) == 0 {
		return nil, errors.New("scanner libraries is required")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create watcher: %w", err)
	}

	libraries, err := normalizeLibraries(cfg.Libraries)
	if err != nil {
		_ = watcher.Close()
		return nil, err
	}

	fullScan := cfg.FullScanEvery
	if fullScan <= 0 {
		fullScan = defaultFullScanEvery
	}
	debounce := cfg.DebounceWindow
	if debounce <= 0 {
		debounce = defaultDebounce
	}
	// 稳定性 guard 默认值仅在**两个字段都为零**时应用，避免测试里 size=1/age=0
	// 的显式宽松配置被静默覆盖。显式传过任何一个，视为调用方接管默认值策略。
	minSize := cfg.MinFileSize
	minAge := cfg.MinFileAge
	if minSize == 0 && minAge == 0 {
		minSize = defaultMinFileSize
		minAge = defaultMinFileAge
	}
	if minSize < 0 {
		minSize = 0
	}
	if minAge < 0 {
		minAge = 0
	}

	return &Scanner{
		libraries:      libraries,
		onFound:        cfg.OnFound,
		watcher:        watcher,
		fullScanEvery:  fullScan,
		debounceWindow: debounce,
		minFileSize:    minSize,
		minFileAge:     minAge,
		recentEvents:   make(map[string]time.Time),
	}, nil
}

func normalizeLibraries(input []ScanLibrary) ([]ScanLibrary, error) {
	out := make([]ScanLibrary, 0, len(input))
	for _, lib := range input {
		if strings.TrimSpace(lib.Path) == "" {
			return nil, errors.New("scanner library path is required")
		}
		lib.Path = filepath.Clean(lib.Path)
		// Scanner 内部一律递归。非递归用例请改用 DoFullScan 或单独 API。
		lib.Recursive = true
		out = append(out, lib)
	}
	return out, nil
}

// Start 启动扫描器：初始全量扫描 → 建立 watcher → 启动事件循环和周期扫描。
func (s *Scanner) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("scanner already started")
	}
	s.started = true
	s.ctx, s.cancel = context.WithCancel(ctx)
	libs := append([]ScanLibrary(nil), s.libraries...)
	s.mu.Unlock()

	for _, lib := range libs {
		s.walkLibraryAndEmit(lib)
	}
	for _, lib := range libs {
		if err := s.addLibraryWatch(lib); err != nil {
			s.cancel()
			_ = s.watcher.Close()
			return fmt.Errorf("add watcher %s: %w", lib.Path, err)
		}
	}

	s.wg.Add(2)
	go s.watchLoop()
	go s.tickerLoop()
	return nil
}

// Stop 停止扫描器。
func (s *Scanner) Stop() error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return nil
	}
	s.started = false
	cancel := s.cancel
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	err := s.watcher.Close()
	s.wg.Wait()
	return err
}

// DoFullScan 按 libraryID 立即触发一次全量 walk；0 = 扫描所有库。
// 用于 UI "手动扫描" 按钮——绕过 ticker，同步触发 OnFound。
// 注意：调用方应在 goroutine 里调用，扫描大库会阻塞。
func (s *Scanner) DoFullScan(libraryID uint) int {
	s.mu.Lock()
	libs := append([]ScanLibrary(nil), s.libraries...)
	s.mu.Unlock()

	count := 0
	for _, lib := range libs {
		if libraryID != 0 && lib.ID != libraryID {
			continue
		}
		count++
		s.walkLibraryAndEmit(lib)
	}
	return count
}

// Reload 用新 library 集合替换当前监控。实现：Stop → 重建 watcher → Start。
// 失败时保留原 Scanner 运行，返回错误。
func (s *Scanner) Reload(libs []ScanLibrary) error {
	normalized, err := normalizeLibraries(libs)
	if err != nil {
		return err
	}
	newWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("reload watcher: %w", err)
	}

	// 停旧 ctx/watcher
	s.mu.Lock()
	wasRunning := s.started
	oldCancel := s.cancel
	oldWatcher := s.watcher
	s.mu.Unlock()

	if oldCancel != nil {
		oldCancel()
	}
	if oldWatcher != nil {
		_ = oldWatcher.Close()
	}
	s.wg.Wait()

	// 装新
	s.mu.Lock()
	s.libraries = normalized
	s.watcher = newWatcher
	s.recentEvents = make(map[string]time.Time)
	s.started = false
	s.mu.Unlock()

	if !wasRunning {
		return nil
	}
	// 用上次的父 ctx 重启。为简化，Reload 要求调用方传入新 ctx，当前内部保留
	// 原 ctx 的父 ctx 不便，这里直接用 Background —— scannerManager 层负责
	// 按生命周期传递 ctx。
	return s.Start(context.Background())
}

// Libraries 返回当前监控的库（snapshot）。
func (s *Scanner) Libraries() []ScanLibrary {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ScanLibrary(nil), s.libraries...)
}

func (s *Scanner) addRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if shouldSkipDir(err, d) {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if addErr := s.watcher.Add(path); addErr != nil {
			if os.IsPermission(addErr) {
				return fs.SkipDir
			}
			return addErr
		}
		return nil
	})
}

func (s *Scanner) walkAndEmit(lib ScanLibrary) {
	_ = filepath.WalkDir(lib.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if shouldSkipDir(err, d) {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !isVideoFile(path) {
			return nil
		}
		if !s.isFileStable(path) {
			return nil
		}
		s.onFound(lib, path)
		return nil
	})
}

func (s *Scanner) watchLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		case ev, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			if ev.Op&(fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}

			if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
				if s.shouldWatchDir(ev.Name) {
					_ = s.addRecursive(ev.Name)
				}
				continue
			}

			if !isVideoFile(ev.Name) {
				continue
			}
			if !s.isFileStable(ev.Name) {
				// 下载中的文件——先不 emit；下一次 ticker 或 debounce 过后会补上
				continue
			}
			if !s.shouldEmit(ev.Name) {
				continue
			}
			lib := s.matchLibrary(ev.Name)
			if lib == nil {
				continue
			}
			s.onFound(*lib, ev.Name)
		case _, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func (s *Scanner) tickerLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.fullScanEvery)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			libs := append([]ScanLibrary(nil), s.libraries...)
			s.mu.Unlock()
			for _, lib := range libs {
				s.walkLibraryAndEmit(lib)
			}
		}
	}
}

// shouldEmit 去重窗口 —— 防止 fsnotify 同一文件多次 Create / Rename。
func (s *Scanner) shouldEmit(path string) bool {
	now := time.Now()
	path = filepath.Clean(path)

	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := now.Add(-(s.debounceWindow * 10))
	for key, ts := range s.recentEvents {
		if ts.Before(cutoff) {
			delete(s.recentEvents, key)
		}
	}

	last, ok := s.recentEvents[path]
	if ok && now.Sub(last) < s.debounceWindow {
		return false
	}
	s.recentEvents[path] = now
	return true
}

// isFileStable 文件稳定性 guard：防止刮削正在下载/复制的不完整文件。
// 若文件 size < minFileSize 或 mtime 距今 < minFileAge，视为不稳定，跳过。
// 这是本次 OnFound 的否决点，下次 ticker 或 fsnotify 事件会重新评估。
func (s *Scanner) isFileStable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.Size() < s.minFileSize {
		return false
	}
	if time.Since(info.ModTime()) < s.minFileAge {
		return false
	}
	return true
}

// matchLibrary 按最长路径前缀匹配 path 所属 library。
func (s *Scanner) matchLibrary(path string) *ScanLibrary {
	clean := filepath.Clean(path)
	s.mu.Lock()
	defer s.mu.Unlock()

	var best *ScanLibrary
	bestLen := -1
	for i := range s.libraries {
		lib := &s.libraries[i]
		if clean == lib.Path || isSubPath(lib.Path, clean) {
			if len(lib.Path) > bestLen {
				bestLen = len(lib.Path)
				best = lib
			}
		}
	}
	return best
}

func isVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return videoExts[ext]
}

func (s *Scanner) addLibraryWatch(lib ScanLibrary) error {
	return s.addRecursive(lib.Path)
}

func (s *Scanner) walkLibraryAndEmit(lib ScanLibrary) {
	s.walkAndEmit(lib)
}

func (s *Scanner) shouldWatchDir(path string) bool {
	clean := filepath.Clean(path)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, lib := range s.libraries {
		if clean == lib.Path || isSubPath(lib.Path, clean) {
			return true
		}
	}
	return false
}

func isSubPath(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func shouldSkipDir(err error, d fs.DirEntry) bool {
	if errors.Is(err, fs.ErrPermission) || os.IsPermission(err) {
		if d == nil || d.IsDir() {
			return true
		}
	}
	return false
}
