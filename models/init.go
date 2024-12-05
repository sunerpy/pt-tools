package models

import (
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
	ID           uint       `gorm:"primaryKey"`                   // 主键
	SiteName     string     `gorm:"uniqueIndex:idx_site_torrent"` // 与 TorrentID 组合唯一约束
	TorrentID    string     `gorm:"uniqueIndex:idx_site_torrent"` // 与 SiteName 组合唯一约束
	TorrentHash  *string    `gorm:"index"`                        // 允许为 NULL 且添加普通索引
	IsDownloaded bool       `gorm:"default:false"`                // 默认值
	IsPushed     *bool      `gorm:"default:null"`                 // 默认值
	IsSkipped    bool       `gorm:"default:false"`                // 默认值
	FreeLevel    string     `gorm:"default:'normal'"`             // 默认值
	FreeEndTime  *time.Time `gorm:"default:null"`                 // 允许为空
	PushTime     *time.Time `gorm:"default:null"`                 // 允许为空
	CreatedAt    time.Time  // GORM 自动管理
	UpdatedAt    time.Time  // GORM 自动管理
}

// TorrentDB 封装数据库操作
type TorrentDB struct {
	DB *gorm.DB
}

// NewDB 初始化并返回 TorrentDB
func NewDB(gormLg zapgorm2.Logger) (*TorrentDB, error) {
	// 确保工作目录存在
	homeDir, _ := os.UserHomeDir()
	dbDir := filepath.Join(homeDir, WorkDir)
	if err := os.MkdirAll(dbDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("创建工作目录失败: %w", err)
	}
	// 数据库文件路径
	dbFile := filepath.Join(dbDir, DBFile)
	// 初始化 GORM
	db, err := gorm.Open(
		sqlite.Open("file:"+dbFile), &gorm.Config{
			Logger: gormLg,
		})
	if err != nil {
		return nil, fmt.Errorf("无法初始化 GORM: %w", err)
	}
	// 启用 WAL 模式
	if err := db.Exec("PRAGMA journal_mode=WAL;").Error; err != nil {
		return nil, fmt.Errorf("无法启用 WAL 模式: %w", err)
	}
	// 自动迁移表结构
	if err := db.AutoMigrate(&TorrentInfo{}); err != nil {
		return nil, fmt.Errorf("自动迁移失败: %w", err)
	}
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
		Updates(map[string]interface{}{
			"is_downloaded": isDownloaded,
			"is_pushed":     isPushed,
			"push_time":     pushTime,
		}).Error
}

// WithTransaction 使用事务
func (t *TorrentDB) WithTransaction(fn func(tx *gorm.DB) error) error {
	return t.DB.Transaction(fn)
}
