package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/scraper/store"
)

// ScannerManager 管理 Scanner 的生命周期：按 DB 里 Enabled=true 的 library 集合
// 构造/重建/停止 Scanner，并桥接 OnFound 回调到 ScrapeService.Enqueue*。
//
// 设计目标：
//  1. Library CRUD 后调 Reload() —— 幂等重建 Scanner（Stop → NewScanner → Start）
//  2. OnFound 统一处理：按文件/目录推 movie 还是 tv，去重（scrape_results 已有则跳过）
//  3. AutoScrape=false 的库仍会被 Scanner 监控（目录已加 watch），但 OnFound
//     会过滤不入队。这样用户手动扫描按钮仍可工作。
type ScannerManager struct {
	db     *gorm.DB
	scrape *ScrapeService
	log    Logger

	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	scanner *Scanner
	started bool
}

// NewScannerManager 构造（不启动）。调用方需再调 Start(ctx)。
// db / scrape 必填；log 可选（nil → noopLogger）。
func NewScannerManager(db *gorm.DB, scrape *ScrapeService, log Logger) *ScannerManager {
	if log == nil {
		log = noopLogger{}
	}
	return &ScannerManager{db: db, scrape: scrape, log: log}
}

// Start 首次加载 DB 里所有 Enabled=true 的 library 并启动 Scanner。
// 传入的 ctx 作为 Scanner 内部所有 goroutine 的父 ctx；cancel 后 Scanner 停止。
func (m *ScannerManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return errors.New("scanner manager already started")
	}
	m.ctx, m.cancel = context.WithCancel(ctx)

	libs, err := m.buildScanLibraries()
	if err != nil {
		return err
	}
	if len(libs) == 0 {
		m.log.Infof("scanner: 没有启用的媒体库，Scanner 未启动")
		m.started = true
		return nil
	}
	if err := m.startScannerLocked(libs); err != nil {
		return err
	}
	m.started = true
	return nil
}

// Reload 从 DB 重新加载 library 列表并重建 Scanner。library CRUD 后调用。
// 加锁确保并发的 Reload 串行化。
func (m *ScannerManager) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.started {
		return nil
	}
	libs, err := m.buildScanLibraries()
	if err != nil {
		return err
	}
	// 先停旧的
	if m.scanner != nil {
		_ = m.scanner.Stop()
		m.scanner = nil
	}
	if len(libs) == 0 {
		m.log.Infof("scanner: 重载后无启用的媒体库，Scanner 保持停止状态")
		return nil
	}
	return m.startScannerLocked(libs)
}

// Stop 停止 Scanner + 取消 ctx。调用后可再次 Start。
func (m *ScannerManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
	}
	var err error
	if m.scanner != nil {
		err = m.scanner.Stop()
		m.scanner = nil
	}
	m.started = false
	return err
}

// TriggerManualScan 触发指定 library 的立即全量扫描（绕过 ticker）。
// libraryID=0 扫描所有已加载库。返回实际扫描的库数量。
// 调用方应在 goroutine 里执行，大库会阻塞数秒到数分钟。
func (m *ScannerManager) TriggerManualScan(libraryID uint) (int, error) {
	m.mu.Lock()
	scanner := m.scanner
	m.mu.Unlock()
	if scanner == nil {
		return 0, errors.New("scanner not started")
	}
	return scanner.DoFullScan(libraryID), nil
}

func (m *ScannerManager) buildScanLibraries() ([]ScanLibrary, error) {
	var libs []store.MediaLibraryConfig
	if err := m.db.Where("enabled = ?", true).Find(&libs).Error; err != nil {
		return nil, fmt.Errorf("list libraries: %w", err)
	}
	out := make([]ScanLibrary, 0, len(libs))
	for _, lib := range libs {
		if strings.TrimSpace(lib.Path) == "" {
			continue
		}
		out = append(out, ScanLibrary{
			ID:        lib.ID,
			Name:      lib.Name,
			Path:      lib.Path,
			Type:      lib.Type,
			Recursive: true,
		})
	}
	return out, nil
}

// startScannerLocked 构造并启动 Scanner。调用方必须持有 m.mu.Lock。
func (m *ScannerManager) startScannerLocked(libs []ScanLibrary) error {
	scanner, err := NewScanner(ScannerConfig{
		Libraries:      libs,
		OnFound:        m.onFound,
		FullScanEvery:  6 * time.Hour,
		DebounceWindow: time.Minute,
	})
	if err != nil {
		return fmt.Errorf("new scanner: %w", err)
	}
	if err := scanner.Start(m.ctx); err != nil {
		return fmt.Errorf("start scanner: %w", err)
	}
	m.scanner = scanner
	m.log.Infof("scanner: 已启动，监控 %d 个媒体库", len(libs))
	return nil
}

// onFound Scanner 回调：已刮削过则跳过，否则按 lib.Type 入队。
// 注意这里 library 的 AutoScrape 字段**不过滤** —— 改为 Enabled 控制。
// 见 AutoScrape 字段的 deprecation 说明。
func (m *ScannerManager) onFound(lib ScanLibrary, path string) {
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// 去重：scrape_results.file_path 已存在即视为已刮削成功，跳过。
	// 用户想重刮需先在 UI 删除对应 result（或未来加 overwrite_nfo 等入口）。
	var count int64
	if err := m.db.WithContext(ctx).Model(&store.ScrapeResult{}).
		Where("file_path = ?", path).Count(&count).Error; err != nil {
		m.log.Warnf("scanner: 查询 scrape_results 失败 path=%s err=%v", path, err)
	} else if count > 0 {
		return
	}

	// 按 library.Type 路由（启发式）：
	//   - movie: 单文件路径 → EnqueueMovie(file path)
	//   - tv:    传入文件路径，但 MediaPath 取文件所在目录 = 剧集根
	//   - mixed: 目录下视为 tv，顶层视为 movie（近似 Jellyfin 自动判定）
	libID := lib.ID
	switch normalizeLibraryType(lib.Type, lib.Path, path) {
	case "movie":
		if _, err := m.scrape.EnqueueMovie(ctx, ScrapeMovieRequest{
			LibraryID: &libID,
			MediaPath: path,
		}); err != nil {
			m.log.Warnf("scanner: EnqueueMovie 失败 path=%s err=%v", path, err)
		}
	case "tv":
		if _, err := m.scrape.EnqueueTvShow(ctx, ScrapeTvShowRequest{
			LibraryID: &libID,
			MediaPath: filepath.Dir(path),
		}); err != nil {
			m.log.Warnf("scanner: EnqueueTvShow 失败 path=%s err=%v", path, err)
		}
	default:
		m.log.Warnf("scanner: 无法判定 library %q 类型 (type=%s)，跳过 %s", lib.Name, lib.Type, path)
	}
}

// normalizeLibraryType 把 library.Type + path 位置组合成最终 "movie" 或 "tv"。
// mixed 的规则：直接落在 lib.Path 里（深度 1）视为 movie；在子目录里（深度 ≥ 2）
// 视为 tv。近似 Jellyfin "Movie library + TV library" 的物理隔离约定。
func normalizeLibraryType(libType, libPath, filePath string) string {
	switch strings.ToLower(strings.TrimSpace(libType)) {
	case "movie":
		return "movie"
	case "tv", "tvshow", "tv_show":
		return "tv"
	case "mixed", "":
		rel, err := filepath.Rel(libPath, filePath)
		if err != nil {
			return "movie"
		}
		if strings.Contains(rel, string(filepath.Separator)) {
			return "tv"
		}
		return "movie"
	default:
		return ""
	}
}
