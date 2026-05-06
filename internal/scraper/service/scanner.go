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

// 视频文件扩展名。
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

// FoundHandler 接收到新视频文件时的回调。
type FoundHandler func(path string)

// ScanLibrary 扫描目录配置。
type ScanLibrary struct {
	ID        uint
	Name      string
	Path      string
	Recursive bool
}

// Scanner fsnotify + 定时扫描的组合器。
type Scanner struct {
	libraries      []ScanLibrary
	onFound        FoundHandler
	watcher        *fsnotify.Watcher
	fullScanEvery  time.Duration
	debounceWindow time.Duration

	mu           sync.Mutex
	recentEvents map[string]time.Time

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type ScannerConfig struct {
	Libraries      []ScanLibrary
	OnFound        FoundHandler
	FullScanEvery  time.Duration
	DebounceWindow time.Duration
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

	libraries := make([]ScanLibrary, 0, len(cfg.Libraries))
	for _, lib := range cfg.Libraries {
		if strings.TrimSpace(lib.Path) == "" {
			watcher.Close()
			return nil, errors.New("scanner library path is required")
		}
		lib.Path = filepath.Clean(lib.Path)
		if !lib.Recursive {
			lib.Recursive = true
		}
		libraries = append(libraries, lib)
	}

	fullScan := cfg.FullScanEvery
	if fullScan <= 0 {
		fullScan = 6 * time.Hour
	}
	debounce := cfg.DebounceWindow
	if debounce <= 0 {
		debounce = time.Minute
	}

	return &Scanner{
		libraries:      libraries,
		onFound:        cfg.OnFound,
		watcher:        watcher,
		fullScanEvery:  fullScan,
		debounceWindow: debounce,
		recentEvents:   make(map[string]time.Time),
	}, nil
}

// Start 启动扫描器：初始全量扫描 → 建立 watcher → 启动事件循环和周期扫描。
func (s *Scanner) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	for _, lib := range s.libraries {
		s.walkLibraryAndEmit(lib)
	}

	for _, lib := range s.libraries {
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
	if s.cancel != nil {
		s.cancel()
	}
	err := s.watcher.Close()
	s.wg.Wait()
	return err
}

// addRecursive 递归为目录添加 watcher。
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

// walkAndEmit 全量扫描目录，对每个视频文件调 onFound。
func (s *Scanner) walkAndEmit(root string) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if shouldSkipDir(err, d) {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !isVideoFile(path) {
			return nil
		}
		s.onFound(path)
		return nil
	})
}

// watchLoop 从 s.watcher.Events 读事件，filter + debounce + emit。
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
			if s.shouldEmit(ev.Name) {
				s.onFound(ev.Name)
			}
		case _, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

// tickerLoop 定时触发 full scan。
func (s *Scanner) tickerLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.fullScanEvery)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			for _, lib := range s.libraries {
				s.walkLibraryAndEmit(lib)
			}
		}
	}
}

// shouldEmit 检查去重窗口，并顺带清理过期记录避免 map 无限增长。
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

func isVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return videoExts[ext]
}

func (s *Scanner) addLibraryWatch(lib ScanLibrary) error {
	if lib.Recursive {
		return s.addRecursive(lib.Path)
	}
	return s.watcher.Add(lib.Path)
}

func (s *Scanner) walkLibraryAndEmit(lib ScanLibrary) {
	if lib.Recursive {
		s.walkAndEmit(lib.Path)
		return
	}

	entries, err := os.ReadDir(lib.Path)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(lib.Path, entry.Name())
		if isVideoFile(path) {
			s.onFound(path)
		}
	}
}

func (s *Scanner) shouldWatchDir(path string) bool {
	clean := filepath.Clean(path)
	for _, lib := range s.libraries {
		if !lib.Recursive {
			continue
		}
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
