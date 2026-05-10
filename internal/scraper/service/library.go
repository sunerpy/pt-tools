package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
	"github.com/sunerpy/pt-tools/internal/scraper/store"
)

// LibraryService 管理媒体库配置（store.MediaLibraryConfig）的 CRUD。
// 校验路径存在 + Provider/Connector 存在性。
type LibraryService struct {
	db            *gorm.DB
	pathValidator func(path string) error
}

// LibraryConfig 构造 LibraryService 的依赖。
type LibraryConfig struct {
	DB            *gorm.DB
	PathValidator func(path string) error // 可选，默认 osPathExists
}

// NewLibraryService 构造 LibraryService。
func NewLibraryService(cfg LibraryConfig) (*LibraryService, error) {
	if cfg.DB == nil {
		return nil, errors.New("nil db")
	}
	validator := cfg.PathValidator
	if validator == nil {
		validator = osPathExists
	}
	return &LibraryService{db: cfg.DB, pathValidator: validator}, nil
}

// CreateLibraryRequest 创建媒体库入参。
type CreateLibraryRequest struct {
	Name        string
	Type        string // "movie"/"tv"/"mixed"
	Path        string
	ProviderIDs []string // e.g. ["tmdb", "douban", "llm"]
	ConnectorID *uint
	ScanCron    string
	AutoScrape  bool
	NfoDialect  string // kodi/jellyfin/emby/universal
}

// CreateLibrary 创建媒体库，校验路径 + provider/connector。
// 失败场景：
//   - Path 为空或不存在
//   - Name 重复（唯一索引）
//   - ConnectorID 指向不存在的 connector（若非 nil）
func (s *LibraryService) CreateLibrary(ctx context.Context, req CreateLibraryRequest) (*store.MediaLibraryConfig, error) {
	if req.Name == "" {
		return nil, errors.New("name required")
	}
	if req.Path == "" {
		return nil, errors.New("path required")
	}
	if err := s.pathValidator(req.Path); err != nil {
		return nil, fmt.Errorf("path validation: %w", err)
	}
	if req.ConnectorID != nil && *req.ConnectorID > 0 {
		var conn store.ConnectorConfig
		if err := s.db.WithContext(ctx).First(&conn, *req.ConnectorID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("%w: connector %d", core.ErrNotFound, *req.ConnectorID)
			}
			return nil, err
		}
	}

	lib := store.MediaLibraryConfig{
		Name:        req.Name,
		Type:        req.Type,
		Path:        req.Path,
		Enabled:     true,
		NfoDialect:  req.NfoDialect,
		ProviderIDs: strings.Join(req.ProviderIDs, ","),
		ConnectorID: req.ConnectorID,
		ScanCron:    req.ScanCron,
		AutoScrape:  req.AutoScrape,
	}
	if lib.NfoDialect == "" {
		lib.NfoDialect = "universal"
	}
	if lib.Type == "" {
		lib.Type = "mixed"
	}

	if err := s.db.WithContext(ctx).Create(&lib).Error; err != nil {
		return nil, fmt.Errorf("create library: %w", err)
	}
	return &lib, nil
}

// GetLibrary 根据 ID 读取，不存在返回 core.ErrNotFound。
func (s *LibraryService) GetLibrary(ctx context.Context, id uint) (*store.MediaLibraryConfig, error) {
	var lib store.MediaLibraryConfig
	if err := s.db.WithContext(ctx).First(&lib, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: library %d", core.ErrNotFound, id)
		}
		return nil, err
	}
	return &lib, nil
}

// UpdateLibraryRequest 部分更新入参，nil 字段不更新。
type UpdateLibraryRequest struct {
	Name        *string
	Type        *string
	Path        *string
	Enabled     *bool
	ProviderIDs []string // 非 nil 即更新
	ConnectorID *uint
	ScanCron    *string
	AutoScrape  *bool
	NfoDialect  *string
}

// UpdateLibrary 部分更新媒体库字段。
func (s *LibraryService) UpdateLibrary(ctx context.Context, id uint, updates UpdateLibraryRequest) (*store.MediaLibraryConfig, error) {
	lib, err := s.GetLibrary(ctx, id)
	if err != nil {
		return nil, err
	}

	m := map[string]any{}
	if updates.Name != nil {
		m["name"] = *updates.Name
	}
	if updates.Type != nil {
		m["type"] = *updates.Type
	}
	if updates.Path != nil {
		if err := s.pathValidator(*updates.Path); err != nil {
			return nil, fmt.Errorf("path validation: %w", err)
		}
		m["path"] = *updates.Path
	}
	if updates.Enabled != nil {
		m["enabled"] = *updates.Enabled
	}
	if updates.ProviderIDs != nil {
		m["provider_ids"] = strings.Join(updates.ProviderIDs, ",")
	}
	if updates.ConnectorID != nil {
		var conn store.ConnectorConfig
		if err := s.db.WithContext(ctx).First(&conn, *updates.ConnectorID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("%w: connector %d", core.ErrNotFound, *updates.ConnectorID)
			}
			return nil, err
		}
		m["connector_id"] = *updates.ConnectorID
	}
	if updates.ScanCron != nil {
		m["scan_cron"] = *updates.ScanCron
	}
	if updates.AutoScrape != nil {
		m["auto_scrape"] = *updates.AutoScrape
	}
	if updates.NfoDialect != nil {
		m["nfo_dialect"] = *updates.NfoDialect
	}

	if len(m) > 0 {
		if err := s.db.WithContext(ctx).Model(lib).Updates(m).Error; err != nil {
			return nil, fmt.Errorf("update library: %w", err)
		}
	}
	return s.GetLibrary(ctx, id)
}

// DeleteLibrary 软删除库 + 级联硬删除关联的 ScrapeTask/ScrapeResult。
// 在单个事务内执行。
func (s *LibraryService) DeleteLibrary(ctx context.Context, id uint) error {
	lib, err := s.GetLibrary(ctx, id)
	if err != nil {
		return err
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 硬删关联的 ScrapeTask（它们没有 DeletedAt）
		if err := tx.Where("library_id = ?", lib.ID).Delete(&store.ScrapeTask{}).Error; err != nil {
			return fmt.Errorf("delete tasks: %w", err)
		}
		// 硬删 ScrapeResult
		if err := tx.Where("library_id = ?", lib.ID).Delete(&store.ScrapeResult{}).Error; err != nil {
			return fmt.Errorf("delete results: %w", err)
		}
		// 软删 library
		if err := tx.Delete(lib).Error; err != nil {
			return fmt.Errorf("delete library: %w", err)
		}
		return nil
	})
}

// ListLibraries 返回所有未软删库。
func (s *LibraryService) ListLibraries(ctx context.Context) ([]store.MediaLibraryConfig, error) {
	var libs []store.MediaLibraryConfig
	if err := s.db.WithContext(ctx).Order("id ASC").Find(&libs).Error; err != nil {
		return nil, fmt.Errorf("list libraries: %w", err)
	}
	return libs, nil
}

// osPathExists 默认路径校验器：路径存在且为目录。
func osPathExists(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", core.ErrNotFound, path)
		}
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path %s is not a directory", path)
	}
	return nil
}
