package models

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"moul.io/zapgorm2"
)

const (
	DBFile  = "torrents.db"
	WorkDir = ".pt-tools"
)

// TorrentInfo 表示种子信息
type TorrentInfo struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	SiteName       string     `gorm:"uniqueIndex:idx_site_torrent" json:"siteName"`
	TorrentID      string     `gorm:"uniqueIndex:idx_site_torrent" json:"torrentId"`
	TorrentHash    *string    `gorm:"index" json:"torrentHash"`
	IsFree         bool       `gorm:"default:false" json:"isFree"`
	IsDownloaded   bool       `gorm:"default:false" json:"isDownloaded"`
	IsPushed       *bool      `gorm:"default:null" json:"isPushed"`
	IsSkipped      bool       `gorm:"default:false" json:"isSkipped"`
	FreeLevel      string     `gorm:"default:'normal'" json:"freeLevel"`
	FreeEndTime    *time.Time `gorm:"default:null" json:"freeEndTime"`
	PushTime       *time.Time `gorm:"default:null" json:"pushTime"`
	Title          string     `gorm:"default:''" json:"title"`
	Category       string     `gorm:"default:''" json:"category"`
	Tag            string     `gorm:"default:''" json:"tag"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	IsExpired      bool       `gorm:"default:false" json:"isExpired"`
	LastCheckTime  *time.Time `gorm:"default:null" json:"lastCheckTime"`
	RetryCount     int        `gorm:"default:0" json:"retryCount"`
	LastError      string     `gorm:"default:''" json:"lastError"`
	DownloadSource string     `gorm:"size:32;default:'free_download'" json:"downloadSource"` // free_download or filter_rule
	FilterRuleID   *uint      `gorm:"index" json:"filterRuleId"`                             // ID of the matched filter rule

	// 免费结束管理相关字段
	DownloaderID     *uint      `gorm:"index" json:"downloaderId"`                   // 推送到的下载器 ID
	DownloaderName   string     `gorm:"size:64;default:''" json:"downloaderName"`    // 下载器名称（冗余存储便于查询）
	CompletedAt      *time.Time `gorm:"default:null" json:"completedAt"`             // 下载完成时间
	IsPausedBySystem bool       `gorm:"default:false" json:"isPausedBySystem"`       // 是否被系统自动暂停
	PauseOnFreeEnd   bool       `gorm:"default:false" json:"pauseOnFreeEnd"`         // 免费结束时是否暂停（来自 RSS 配置）
	PausedAt         *time.Time `gorm:"default:null" json:"pausedAt"`                // 系统暂停时间
	PauseReason      string     `gorm:"size:256;default:''" json:"pauseReason"`      // 暂停原因
	IsCompleted      bool       `gorm:"default:false;index" json:"isCompleted"`      // 下载是否已完成
	Progress         float64    `gorm:"default:0" json:"progress"`                   // 下载进度 (0-100)
	TorrentSize      int64      `gorm:"default:0" json:"torrentSize"`                // 种子大小（字节）
	DownloaderTaskID string     `gorm:"size:128;default:''" json:"downloaderTaskId"` // 下载器中的任务 ID（用于暂停/删除操作）
	CheckCount       int        `gorm:"default:0" json:"checkCount"`                 // 进度检查次数
}

// TorrentInfoArchive 种子信息归档表（存储超过保留期的记录）
type TorrentInfoArchive struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	OriginalID        uint       `gorm:"index" json:"originalId"`
	SiteName          string     `gorm:"index" json:"siteName"`
	TorrentID         string     `gorm:"index" json:"torrentId"`
	TorrentHash       *string    `gorm:"index" json:"torrentHash"`
	IsFree            bool       `json:"isFree"`
	IsDownloaded      bool       `json:"isDownloaded"`
	IsPushed          *bool      `json:"isPushed"`
	IsSkipped         bool       `json:"isSkipped"`
	FreeLevel         string     `json:"freeLevel"`
	FreeEndTime       *time.Time `json:"freeEndTime"`
	PushTime          *time.Time `json:"pushTime"`
	Title             string     `json:"title"`
	Category          string     `json:"category"`
	Tag               string     `json:"tag"`
	OriginalCreatedAt time.Time  `json:"originalCreatedAt"`
	OriginalUpdatedAt time.Time  `json:"originalUpdatedAt"`
	IsExpired         bool       `json:"isExpired"`
	LastCheckTime     *time.Time `json:"lastCheckTime"`
	RetryCount        int        `json:"retryCount"`
	LastError         string     `json:"lastError"`
	DownloadSource    string     `json:"downloadSource"`
	FilterRuleID      *uint      `json:"filterRuleId"`
	DownloaderID      *uint      `json:"downloaderId"`
	DownloaderName    string     `json:"downloaderName"`
	CompletedAt       *time.Time `json:"completedAt"`
	IsPausedBySystem  bool       `json:"isPausedBySystem"`
	PauseOnFreeEnd    bool       `json:"pauseOnFreeEnd"`
	PausedAt          *time.Time `json:"pausedAt"`
	PauseReason       string     `json:"pauseReason"`
	IsCompleted       bool       `json:"isCompleted"`
	Progress          float64    `json:"progress"`
	TorrentSize       int64      `json:"torrentSize"`
	DownloaderTaskID  string     `json:"downloaderTaskId"`
	CheckCount        int        `json:"checkCount"`
	ArchivedAt        time.Time  `gorm:"autoCreateTime" json:"archivedAt"`
}

func (t *TorrentInfo) GetExpired() bool {
	if t.IsExpired {
		return true
	}
	if t.FreeEndTime == nil {
		isFreeLevel := t.FreeLevel != "" && t.FreeLevel != "NONE" && t.FreeLevel != "normal"
		return !isFreeLevel
	}
	buffer := 5 * time.Minute
	return time.Now().Add(buffer).After(*t.FreeEndTime)
}

// TorrentDB 封装数据库操作
type TorrentDB struct {
	DB *gorm.DB
}

// NewDB 初始化并返回 TorrentDB
func NewDB(gormLg zapgorm2.Logger) (*TorrentDB, error) {
	return NewDBWithVersion(gormLg, "unknown")
}

// NewDBWithVersion 初始化并返回 TorrentDB（带应用版本）
func NewDBWithVersion(gormLg zapgorm2.Logger, appVersion string) (*TorrentDB, error) {
	// 确保工作目录存在
	homeDir, _ := os.UserHomeDir()
	dbDir := filepath.Join(homeDir, WorkDir)
	if err := os.MkdirAll(dbDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("创建工作目录失败: %w", err)
	}
	// 数据库文件路径
	dbFile := filepath.Join(dbDir, DBFile)
	// 初始化 GORM with SQLite optimizations for concurrent access
	// _busy_timeout: wait up to 30s for locks (prevents "database is locked" errors)
	// _txlock: use IMMEDIATE transactions to detect conflicts early
	// cache=shared: share cache between connections
	dsn := fmt.Sprintf("file:%s?_busy_timeout=30000&_txlock=immediate&cache=shared", dbFile)
	db, err := gorm.Open(
		sqlite.Open(dsn), &gorm.Config{
			Logger: gormLg,
		})
	if err != nil {
		return nil, fmt.Errorf("无法初始化 GORM: %w", err)
	}

	// Configure connection pool for SQLite
	// SQLite handles concurrency at the file level, so limit connections
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取底层数据库连接失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	// 启用 WAL 模式
	if err := db.Exec("PRAGMA journal_mode=WAL;").Error; err != nil {
		return nil, fmt.Errorf("无法启用 WAL 模式: %w", err)
	}
	// Set synchronous=NORMAL for better performance while maintaining durability
	if err := db.Exec("PRAGMA synchronous=NORMAL;").Error; err != nil {
		return nil, fmt.Errorf("无法设置 synchronous 模式: %w", err)
	}
	// 自动迁移表结构（包括版本表）
	if err := db.AutoMigrate(
		&SchemaVersion{}, // 版本表必须最先迁移
		&TorrentInfo{},
		&TorrentInfoArchive{},
		&AdminUser{},
		&SettingsGlobal{},
		&QbitSettings{},
		&SiteSetting{},
		&RSSSubscription{},
		// New tables for downloader and site extensibility
		&DownloaderSetting{},
		&DownloaderDirectory{},
		&SiteTemplate{},
		// Filter rules for RSS filtering
		&FilterRule{},
		// RSS-Filter association table for many-to-many relationship
		&RSSFilterAssociation{},
		// Favicon cache for site icons
		&FaviconCache{},
		&SiteRateLimit{},
	); err != nil {
		return nil, fmt.Errorf("自动迁移失败: %w", err)
	}

	// 运行架构版本迁移
	schemaManager := NewSchemaManager(db, appVersion)
	if err := schemaManager.RunMigrations(); err != nil {
		return nil, fmt.Errorf("架构迁移失败: %w", err)
	}

	// 保证存在全局设置条目（仅在空时写入默认）
	var glCnt int64
	if err := db.Model(&SettingsGlobal{}).Count(&glCnt).Error; err != nil {
		return nil, fmt.Errorf("统计全局设置失败: %w", err)
	}
	if glCnt == 0 {
		def := SettingsGlobal{
			DownloadDir:            "downloads",
			DefaultIntervalMinutes: DefaultIntervalMinutes,
			DefaultConcurrency:     DefaultConcurrency,
			DefaultEnabled:         false,
			DownloadLimitEnabled:   false,
			DownloadSpeedLimit:     0,
			TorrentSizeGB:          0,
			AutoStart:              false,
			RetainHours:            24,
			MaxRetry:               3,
		}
		if err := db.Create(&def).Error; err != nil {
			return nil, fmt.Errorf("写入默认全局设置失败: %w", err)
		}
	}
	// 站点同步由 core.ConfigStore.SyncSites() 在应用启动时处理

	var mode string
	if err := db.Raw("PRAGMA journal_mode;").Scan(&mode).Error; err != nil {
		return nil, fmt.Errorf("无法验证 WAL 模式: %w", err)
	}
	return &TorrentDB{DB: db}, nil
}

// UpsertTorrent 插入或更新种子信息
func (t *TorrentDB) UpsertTorrent(torrent *TorrentInfo) error {
	return t.DB.Save(torrent).Error
}

// GetTorrentBySiteAndID 根据 SiteName 和 TorrentID 查询种子信息
func (t *TorrentDB) GetTorrentBySiteAndID(siteName, torrentID string) (*TorrentInfo, error) {
	var torrent TorrentInfo
	err := t.DB.Where("site_name = ? AND torrent_id = ?", siteName, torrentID).First(&torrent).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil // 未找到记录
	}
	return &torrent, err
}

// GetTorrentBySiteAndHash 根据 SiteName 和 TorrentHash 查询种子信息
func (t *TorrentDB) GetTorrentBySiteAndHash(siteName, torrentHash string) (*TorrentInfo, error) {
	var torrent TorrentInfo
	err := t.DB.Where("site_name = ? AND torrent_hash = ?", siteName, torrentHash).First(&torrent).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil // 未找到记录
	}
	return &torrent, err
}

// GetAllTorrents 查询所有种子信息
func (t *TorrentDB) GetAllTorrents() ([]TorrentInfo, error) {
	var torrents []TorrentInfo
	err := t.DB.Find(&torrents).Error
	return torrents, err
}

// DeleteTorrent 删除种子信息
func (t *TorrentDB) DeleteTorrent(torrentHash string) error {
	return t.DB.Where("torrent_hash = ?", torrentHash).Delete(&TorrentInfo{}).Error
}

// UpdateTorrentStatus 更新种子状态
func (t *TorrentDB) UpdateTorrentStatus(torrentHash string, isDownloaded, isPushed bool, pushTime *time.Time) error {
	return t.DB.Model(&TorrentInfo{}).
		Where("torrent_hash = ?", torrentHash).
		Updates(map[string]any{
			"is_downloaded": isDownloaded,
			"is_pushed":     isPushed,
			"push_time":     pushTime,
		}).Error
}

// WithTransaction 使用事务
func (t *TorrentDB) WithTransaction(fn func(tx *gorm.DB) error) error {
	return t.DB.Transaction(fn)
}

// WithTransactionContext 使用事务（支持 context）
func (t *TorrentDB) WithTransactionContext(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return t.DB.WithContext(ctx).Transaction(fn)
}
