package migration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// MigrationService 配置迁移服务
type MigrationService struct {
	db        *gorm.DB
	backupDir string
}

// NewMigrationService 创建迁移服务
func NewMigrationService(db *gorm.DB) *MigrationService {
	homeDir, _ := os.UserHomeDir()
	backupDir := filepath.Join(homeDir, models.WorkDir, "backups")
	return &MigrationService{
		db:        db,
		backupDir: backupDir,
	}
}

// NewMigrationServiceWithBackupDir 创建迁移服务（指定备份目录）
func NewMigrationServiceWithBackupDir(db *gorm.DB, backupDir string) *MigrationService {
	return &MigrationService{
		db:        db,
		backupDir: backupDir,
	}
}

// MigrationResult 迁移结果
type MigrationResult struct {
	Success             bool      `json:"success"`
	Message             string    `json:"message"`
	BackupPath          string    `json:"backup_path,omitempty"`
	MigratedAt          time.Time `json:"migrated_at"`
	DownloadersMigrated int       `json:"downloaders_migrated"`
	SitesMigrated       int       `json:"sites_migrated"`
	Errors              []string  `json:"errors,omitempty"`
}

// BackupData 备份数据结构
type BackupData struct {
	Version   string                   `json:"version"`
	CreatedAt time.Time                `json:"created_at"`
	Global    models.SettingsGlobal    `json:"global"`
	Qbit      models.QbitSettings      `json:"qbit"`
	Sites     []models.SiteSetting     `json:"sites"`
	RSS       []models.RSSSubscription `json:"rss"`
}

// CreateBackup 创建配置备份
func (m *MigrationService) CreateBackup() (string, error) {
	// 确保备份目录存在
	if err := os.MkdirAll(m.backupDir, 0o755); err != nil {
		return "", fmt.Errorf("创建备份目录失败: %w", err)
	}

	// 收集当前配置
	backup := BackupData{
		Version:   "v1",
		CreatedAt: time.Now(),
	}

	// 获取全局设置
	if err := m.db.First(&backup.Global).Error; err != nil && err != gorm.ErrRecordNotFound {
		return "", fmt.Errorf("获取全局设置失败: %w", err)
	}

	// 获取qBittorrent设置
	if err := m.db.First(&backup.Qbit).Error; err != nil && err != gorm.ErrRecordNotFound {
		return "", fmt.Errorf("获取qBittorrent设置失败: %w", err)
	}

	// 获取站点设置
	if err := m.db.Find(&backup.Sites).Error; err != nil {
		return "", fmt.Errorf("获取站点设置失败: %w", err)
	}

	// 获取RSS订阅
	if err := m.db.Find(&backup.RSS).Error; err != nil {
		return "", fmt.Errorf("获取RSS订阅失败: %w", err)
	}

	// 生成备份文件名
	timestamp := time.Now().Format("20060102_150405")
	backupFile := filepath.Join(m.backupDir, fmt.Sprintf("config_backup_%s.json", timestamp))

	// 写入备份文件
	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化备份数据失败: %w", err)
	}

	if err := os.WriteFile(backupFile, data, 0o644); err != nil {
		return "", fmt.Errorf("写入备份文件失败: %w", err)
	}

	return backupFile, nil
}

// RestoreBackup 从备份恢复
func (m *MigrationService) RestoreBackup(backupPath string) error {
	// 读取备份文件
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("读取备份文件失败: %w", err)
	}

	var backup BackupData
	if err := json.Unmarshal(data, &backup); err != nil {
		return fmt.Errorf("解析备份数据失败: %w", err)
	}

	// 使用事务恢复
	return m.db.Transaction(func(tx *gorm.DB) error {
		// 清空现有数据
		if err := tx.Where("1 = 1").Delete(&models.DownloaderSetting{}).Error; err != nil {
			return fmt.Errorf("清空下载器设置失败: %w", err)
		}
		if err := tx.Where("1 = 1").Delete(&models.DynamicSiteSetting{}).Error; err != nil {
			return fmt.Errorf("清空动态站点设置失败: %w", err)
		}

		// 恢复全局设置
		if backup.Global.ID != 0 {
			if err := tx.Save(&backup.Global).Error; err != nil {
				return fmt.Errorf("恢复全局设置失败: %w", err)
			}
		}

		// 恢复qBittorrent设置
		if backup.Qbit.ID != 0 {
			if err := tx.Save(&backup.Qbit).Error; err != nil {
				return fmt.Errorf("恢复qBittorrent设置失败: %w", err)
			}
		}

		// 恢复站点设置
		for _, site := range backup.Sites {
			if err := tx.Save(&site).Error; err != nil {
				return fmt.Errorf("恢复站点设置失败: %w", err)
			}
		}

		// 恢复RSS订阅
		for _, rss := range backup.RSS {
			if err := tx.Save(&rss).Error; err != nil {
				return fmt.Errorf("恢复RSS订阅失败: %w", err)
			}
		}

		return nil
	})
}

// MigrateV1ToV2 执行v1到v2的迁移
func (m *MigrationService) MigrateV1ToV2() *MigrationResult {
	result := &MigrationResult{
		MigratedAt: time.Now(),
		Errors:     make([]string, 0),
	}

	// 1. 创建备份
	backupPath, err := m.CreateBackup()
	if err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("创建备份失败: %v", err)
		return result
	}
	result.BackupPath = backupPath

	// 2. 执行迁移
	err = m.db.Transaction(func(tx *gorm.DB) error {
		// 迁移qBittorrent设置到downloader_settings
		var qbit models.QbitSettings
		if qbitErr := tx.First(&qbit).Error; qbitErr == nil && qbit.URL != "" {
			downloader := models.DownloaderSetting{
				Name:      "qbittorrent-default",
				Type:      "qbittorrent",
				URL:       qbit.URL,
				Username:  qbit.User,
				Password:  qbit.Password,
				IsDefault: true,
				Enabled:   qbit.Enabled,
			}
			if createErr := tx.Create(&downloader).Error; createErr != nil {
				return fmt.Errorf("迁移qBittorrent设置失败: %w", createErr)
			}
			result.DownloadersMigrated++
		}

		// 迁移站点设置到dynamic_site_settings
		var sites []models.SiteSetting
		if findErr := tx.Find(&sites).Error; findErr != nil {
			return fmt.Errorf("获取站点设置失败: %w", findErr)
		}

		for _, site := range sites {
			dynamicSite := models.DynamicSiteSetting{
				Name:        site.Name,
				DisplayName: site.Name, // 使用name作为display_name
				Enabled:     site.Enabled,
				AuthMethod:  site.AuthMethod,
				Cookie:      site.Cookie,
				APIKey:      site.APIKey,
				APIURL:      site.APIUrl,
				IsBuiltin:   true, // 现有站点标记为内置
			}
			if createErr := tx.Create(&dynamicSite).Error; createErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("迁移站点 %s 失败: %v", site.Name, createErr))
				continue
			}
			result.SitesMigrated++
		}

		return nil
	})
	if err != nil {
		// 迁移失败，尝试恢复
		result.Success = false
		result.Message = fmt.Sprintf("迁移失败: %v", err)
		if restoreErr := m.RestoreBackup(backupPath); restoreErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("恢复备份失败: %v", restoreErr))
		}
		return result
	}

	result.Success = true
	result.Message = "迁移成功"
	return result
}

// IsMigrationNeeded 检查是否需要迁移
func (m *MigrationService) IsMigrationNeeded() bool {
	// 检查是否已有downloader_settings记录
	var count int64
	m.db.Model(&models.DownloaderSetting{}).Count(&count)
	if count > 0 {
		return false // 已经迁移过
	}

	// 检查是否有旧的qbit设置需要迁移
	var qbit models.QbitSettings
	if err := m.db.First(&qbit).Error; err == nil && qbit.URL != "" {
		return true
	}

	return false
}

// GetMigrationStatus 获取迁移状态
type MigrationStatus struct {
	NeedsMigration       bool `json:"needs_migration"`
	HasOldQbitConfig     bool `json:"has_old_qbit_config"`
	HasNewDownloaders    bool `json:"has_new_downloaders"`
	OldSitesCount        int  `json:"old_sites_count"`
	NewDynamicSitesCount int  `json:"new_dynamic_sites_count"`
}

func (m *MigrationService) GetMigrationStatus() *MigrationStatus {
	status := &MigrationStatus{}

	// 检查旧qbit配置
	var qbit models.QbitSettings
	if err := m.db.First(&qbit).Error; err == nil && qbit.URL != "" {
		status.HasOldQbitConfig = true
	}

	// 检查新下载器配置
	var downloaderCount int64
	m.db.Model(&models.DownloaderSetting{}).Count(&downloaderCount)
	status.HasNewDownloaders = downloaderCount > 0

	// 检查旧站点数量
	var oldSitesCount int64
	m.db.Model(&models.SiteSetting{}).Count(&oldSitesCount)
	status.OldSitesCount = int(oldSitesCount)

	// 检查新动态站点数量
	var newSitesCount int64
	m.db.Model(&models.DynamicSiteSetting{}).Count(&newSitesCount)
	status.NewDynamicSitesCount = int(newSitesCount)

	// 判断是否需要迁移
	status.NeedsMigration = status.HasOldQbitConfig && !status.HasNewDownloaders

	return status
}
