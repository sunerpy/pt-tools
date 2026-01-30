package models

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// SiteRepository 站点数据库操作封装
type SiteRepository struct {
	db *gorm.DB
}

func NewSiteRepository(db *gorm.DB) *SiteRepository {
	return &SiteRepository{db: db}
}

// SiteData 站点数据（用于创建/更新）
type SiteData struct {
	Name         string
	DisplayName  string
	BaseURL      string
	Enabled      bool
	AuthMethod   string
	Cookie       string
	APIKey       string
	APIURL       string
	DownloaderID *uint
	ParserConfig string
	IsBuiltin    bool
	TemplateID   *uint
}

func (r *SiteRepository) CreateSite(data SiteData) (uint, error) {
	if data.Name == "" {
		return 0, errors.New("站点名称不能为空")
	}
	if data.AuthMethod == "" {
		return 0, errors.New("认证方式不能为空")
	}

	var count int64
	if err := r.db.Model(&SiteSetting{}).Where("name = ?", data.Name).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("检查站点名称失败: %w", err)
	}
	if count > 0 {
		return 0, errors.New("站点名称已存在")
	}

	displayName := data.DisplayName
	if displayName == "" {
		displayName = data.Name
	}

	site := SiteSetting{
		Name:         data.Name,
		DisplayName:  displayName,
		BaseURL:      data.BaseURL,
		Enabled:      data.Enabled,
		AuthMethod:   data.AuthMethod,
		Cookie:       data.Cookie,
		APIKey:       data.APIKey,
		APIUrl:       data.APIURL,
		DownloaderID: data.DownloaderID,
		ParserConfig: data.ParserConfig,
		IsBuiltin:    data.IsBuiltin,
		TemplateID:   data.TemplateID,
	}

	if err := r.db.Create(&site).Error; err != nil {
		return 0, fmt.Errorf("创建站点失败: %w", err)
	}

	return site.ID, nil
}

func (r *SiteRepository) UpdateSiteCredentials(name string, enabled *bool, authMethod, cookie, apiKey, apiURL string) error {
	var site SiteSetting
	if err := r.db.Where("name = ?", name).First(&site).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			site = SiteSetting{Name: name, DisplayName: name, IsBuiltin: true}
		} else {
			return fmt.Errorf("查询站点失败: %w", err)
		}
	}

	if enabled != nil {
		site.Enabled = *enabled
	}
	if authMethod != "" {
		site.AuthMethod = authMethod
	}
	site.Cookie = cookie
	site.APIKey = apiKey
	site.APIUrl = apiURL

	return r.db.Save(&site).Error
}

func (r *SiteRepository) UpdateSiteDownloader(name string, downloaderID *uint) error {
	return r.db.Model(&SiteSetting{}).
		Where("name = ?", name).
		Update("downloader_id", downloaderID).Error
}

func (r *SiteRepository) UpdateSiteDownloaderByID(siteID uint, downloaderID *uint) error {
	return r.db.Model(&SiteSetting{}).
		Where("id = ?", siteID).
		Update("downloader_id", downloaderID).Error
}

func (r *SiteRepository) BatchUpdateSiteDownloader(siteIDs []uint, downloaderID uint) (int64, error) {
	if len(siteIDs) == 0 {
		return 0, nil
	}

	var rowsAffected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		siteResult := tx.Model(&SiteSetting{}).
			Where("id IN ?", siteIDs).
			Update("downloader_id", downloaderID)
		if siteResult.Error != nil {
			return siteResult.Error
		}
		rowsAffected = siteResult.RowsAffected

		return tx.Model(&RSSSubscription{}).
			Where("site_id IN ?", siteIDs).
			Update("downloader_id", downloaderID).Error
	})

	return rowsAffected, err
}

func (r *SiteRepository) GetSiteByName(name string) (*SiteSetting, error) {
	var site SiteSetting
	if err := r.db.Where("name = ?", name).First(&site).Error; err != nil {
		return nil, err
	}
	return &site, nil
}

func (r *SiteRepository) GetSiteByID(id uint) (*SiteSetting, error) {
	var site SiteSetting
	if err := r.db.First(&site, id).Error; err != nil {
		return nil, err
	}
	return &site, nil
}

func (r *SiteRepository) ListSites() ([]SiteSetting, error) {
	var sites []SiteSetting
	if err := r.db.Find(&sites).Error; err != nil {
		return nil, err
	}
	return sites, nil
}

func (r *SiteRepository) ListEnabledSites() ([]SiteSetting, error) {
	var sites []SiteSetting
	if err := r.db.Where("enabled = ?", true).Find(&sites).Error; err != nil {
		return nil, err
	}
	return sites, nil
}

func (r *SiteRepository) DeleteSite(name string) error {
	return r.db.Where("name = ?", name).Delete(&SiteSetting{}).Error
}

func (r *SiteRepository) SiteExistsByName(name string) (bool, error) {
	var count int64
	if err := r.db.Model(&SiteSetting{}).Where("name = ?", name).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
