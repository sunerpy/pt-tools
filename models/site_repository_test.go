package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSiteRepoTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&SiteSetting{}))
	return db
}

func TestSiteRepository_CreateSite(t *testing.T) {
	db := setupSiteRepoTestDB(t)
	repo := NewSiteRepository(db)

	siteID, err := repo.CreateSite(SiteData{
		Name:        "test-site",
		DisplayName: "Test Site",
		BaseURL:     "https://example.com",
		Enabled:     true,
		AuthMethod:  "cookie",
		Cookie:      "test-cookie",
	})

	require.NoError(t, err)
	assert.NotZero(t, siteID)

	var site SiteSetting
	require.NoError(t, db.Where("name = ?", "test-site").First(&site).Error)
	assert.Equal(t, "test-site", site.Name)
	assert.Equal(t, "Test Site", site.DisplayName)
	assert.Equal(t, "https://example.com", site.BaseURL)
	assert.Equal(t, "cookie", site.AuthMethod)
	assert.Equal(t, "test-cookie", site.Cookie)
	assert.True(t, site.Enabled)
}

func TestSiteRepository_CreateSite_DuplicateName(t *testing.T) {
	db := setupSiteRepoTestDB(t)
	repo := NewSiteRepository(db)

	_, err := repo.CreateSite(SiteData{
		Name:       "test-site",
		AuthMethod: "cookie",
	})
	require.NoError(t, err)

	_, err = repo.CreateSite(SiteData{
		Name:       "test-site",
		AuthMethod: "cookie",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "站点名称已存在")
}

func TestSiteRepository_CreateSite_EmptyName(t *testing.T) {
	db := setupSiteRepoTestDB(t)
	repo := NewSiteRepository(db)

	_, err := repo.CreateSite(SiteData{
		AuthMethod: "cookie",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "站点名称不能为空")
}

func TestSiteRepository_UpdateSiteCredentials(t *testing.T) {
	db := setupSiteRepoTestDB(t)
	repo := NewSiteRepository(db)

	_, err := repo.CreateSite(SiteData{
		Name:       "test-site",
		AuthMethod: "cookie",
		Cookie:     "old-cookie",
		Enabled:    false,
	})
	require.NoError(t, err)

	enabled := true
	err = repo.UpdateSiteCredentials("test-site", &enabled, "api_key", "", "new-api-key", "https://api.example.com")
	require.NoError(t, err)

	var site SiteSetting
	require.NoError(t, db.Where("name = ?", "test-site").First(&site).Error)
	assert.True(t, site.Enabled)
	assert.Equal(t, "api_key", site.AuthMethod)
	assert.Equal(t, "new-api-key", site.APIKey)
	assert.Equal(t, "https://api.example.com", site.APIUrl)
}

func TestSiteRepository_UpdateSiteCredentials_CreateIfNotExists(t *testing.T) {
	db := setupSiteRepoTestDB(t)
	repo := NewSiteRepository(db)

	enabled := true
	err := repo.UpdateSiteCredentials("new-site", &enabled, "cookie", "new-cookie", "", "")
	require.NoError(t, err)

	var site SiteSetting
	require.NoError(t, db.Where("name = ?", "new-site").First(&site).Error)
	assert.Equal(t, "new-cookie", site.Cookie)
	assert.Equal(t, "new-site", site.DisplayName)
	assert.True(t, site.IsBuiltin)
}

func TestSiteRepository_BatchUpdateSiteDownloader(t *testing.T) {
	db := setupSiteRepoTestDB(t)
	repo := NewSiteRepository(db)

	id1, _ := repo.CreateSite(SiteData{Name: "site1", AuthMethod: "cookie"})
	id2, _ := repo.CreateSite(SiteData{Name: "site2", AuthMethod: "cookie"})
	_, _ = repo.CreateSite(SiteData{Name: "site3", AuthMethod: "cookie"})

	downloaderID := uint(100)
	rowsAffected, err := repo.BatchUpdateSiteDownloader([]uint{id1, id2}, downloaderID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), rowsAffected)

	site1, _ := repo.GetSiteByID(id1)
	assert.NotNil(t, site1.DownloaderID)
	assert.Equal(t, downloaderID, *site1.DownloaderID)

	site2, _ := repo.GetSiteByID(id2)
	assert.NotNil(t, site2.DownloaderID)
	assert.Equal(t, downloaderID, *site2.DownloaderID)

	site3, _ := repo.GetSiteByName("site3")
	assert.Nil(t, site3.DownloaderID)
}

func TestSiteRepository_DeleteSite(t *testing.T) {
	db := setupSiteRepoTestDB(t)
	repo := NewSiteRepository(db)

	_, err := repo.CreateSite(SiteData{Name: "test-site", AuthMethod: "cookie"})
	require.NoError(t, err)

	err = repo.DeleteSite("test-site")
	require.NoError(t, err)

	exists, _ := repo.SiteExistsByName("test-site")
	assert.False(t, exists)

	var count int64
	db.Model(&SiteSetting{}).Where("name = ?", "test-site").Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestSiteRepository_ListMethods(t *testing.T) {
	db := setupSiteRepoTestDB(t)
	repo := NewSiteRepository(db)

	_, _ = repo.CreateSite(SiteData{Name: "site1", AuthMethod: "cookie", Enabled: true})
	_, _ = repo.CreateSite(SiteData{Name: "site2", AuthMethod: "cookie", Enabled: false})

	allSites, err := repo.ListSites()
	require.NoError(t, err)
	assert.Len(t, allSites, 2)

	enabledSites, err := repo.ListEnabledSites()
	require.NoError(t, err)
	assert.Len(t, enabledSites, 1)
	assert.Equal(t, "site1", enabledSites[0].Name)
}
