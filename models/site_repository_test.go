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
	require.NoError(t, db.AutoMigrate(&SiteSetting{}, &RSSSubscription{}))
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
	err = repo.UpdateSiteCredentials("test-site", &enabled, "api_key", "", "new-api-key", "https://api.example.com", "")
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
	err := repo.UpdateSiteCredentials("new-site", &enabled, "cookie", "new-cookie", "", "", "")
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

func TestSiteRepository_BatchUpdateSiteDownloader_WithRSS(t *testing.T) {
	db := setupSiteRepoTestDB(t)
	repo := NewSiteRepository(db)

	id1, _ := repo.CreateSite(SiteData{Name: "site1", AuthMethod: "cookie"})
	id2, _ := repo.CreateSite(SiteData{Name: "site2", AuthMethod: "cookie"})

	rss1 := RSSSubscription{SiteID: id1, Name: "rss1", URL: "http://example.com/rss1", IntervalMinutes: 5}
	rss2 := RSSSubscription{SiteID: id1, Name: "rss2", URL: "http://example.com/rss2", IntervalMinutes: 5}
	rss3 := RSSSubscription{SiteID: id2, Name: "rss3", URL: "http://example.com/rss3", IntervalMinutes: 5}
	db.Create(&rss1)
	db.Create(&rss2)
	db.Create(&rss3)

	downloaderID := uint(100)
	rowsAffected, err := repo.BatchUpdateSiteDownloader([]uint{id1, id2}, downloaderID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), rowsAffected)

	site1, _ := repo.GetSiteByID(id1)
	assert.NotNil(t, site1.DownloaderID)
	assert.Equal(t, downloaderID, *site1.DownloaderID)

	var updatedRSS1, updatedRSS2, updatedRSS3 RSSSubscription
	db.First(&updatedRSS1, rss1.ID)
	db.First(&updatedRSS2, rss2.ID)
	db.First(&updatedRSS3, rss3.ID)

	assert.NotNil(t, updatedRSS1.DownloaderID, "RSS1 downloader_id should be set")
	assert.Equal(t, downloaderID, *updatedRSS1.DownloaderID, "RSS1 downloader_id should match")
	assert.NotNil(t, updatedRSS2.DownloaderID, "RSS2 downloader_id should be set")
	assert.Equal(t, downloaderID, *updatedRSS2.DownloaderID, "RSS2 downloader_id should match")
	assert.NotNil(t, updatedRSS3.DownloaderID, "RSS3 downloader_id should be set")
	assert.Equal(t, downloaderID, *updatedRSS3.DownloaderID, "RSS3 downloader_id should match")
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

func TestSiteRepository_BatchUpdateAndByID(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{})
	repo := NewSiteRepository(db)

	id1, err := repo.CreateSite(SiteData{Name: "a", AuthMethod: "cookie"})
	require.NoError(t, err)
	id2, err := repo.CreateSite(SiteData{Name: "b", AuthMethod: "cookie"})
	require.NoError(t, err)

	require.NoError(t, db.Create(&RSSSubscription{Name: "r", URL: "http://x", IntervalMinutes: 5, SiteID: id1}).Error)

	rows, err := repo.BatchUpdateSiteDownloader([]uint{id1, id2}, 99)
	require.NoError(t, err)
	assert.Equal(t, int64(2), rows)

	site, err := repo.GetSiteByID(id1)
	require.NoError(t, err)
	require.NotNil(t, site.DownloaderID)
	assert.Equal(t, uint(99), *site.DownloaderID)

	var rss RSSSubscription
	require.NoError(t, db.Where("name = ?", "r").First(&rss).Error)
	require.NotNil(t, rss.DownloaderID)
	assert.Equal(t, uint(99), *rss.DownloaderID)
}

func TestSiteRepository_ListAndCredentials(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{})
	repo := NewSiteRepository(db)

	_, err := repo.CreateSite(SiteData{Name: "a", AuthMethod: "cookie", Enabled: true})
	require.NoError(t, err)
	_, err = repo.CreateSite(SiteData{Name: "b", AuthMethod: "cookie", Enabled: false})
	require.NoError(t, err)

	all, err := repo.ListSites()
	require.NoError(t, err)
	assert.Len(t, all, 2)

	enabled, err := repo.ListEnabledSites()
	require.NoError(t, err)
	require.Len(t, enabled, 1)
	assert.Equal(t, "a", enabled[0].Name)

	exists, err := repo.SiteExistsByName("a")
	require.NoError(t, err)
	assert.True(t, exists)

	on := true
	require.NoError(t, repo.UpdateSiteCredentials("a", &on, "api_key", "ck", "ak", "https://api", "pk"))
	site, err := repo.GetSiteByName("a")
	require.NoError(t, err)
	assert.Equal(t, "api_key", site.AuthMethod)
	assert.Equal(t, "ak", site.APIKey)
	assert.Equal(t, "pk", site.Passkey)

	require.NoError(t, repo.UpdateSiteCredentials("brand-new", nil, "cookie", "c", "", "", ""))
	brand, err := repo.GetSiteByName("brand-new")
	require.NoError(t, err)
	assert.True(t, brand.IsBuiltin)

	require.NoError(t, repo.DeleteSite("b"))
	exists, err = repo.SiteExistsByName("b")
	require.NoError(t, err)
	assert.False(t, exists)

	_, err = repo.CreateSite(SiteData{Name: "a", AuthMethod: "cookie"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "已存在")

	_, err = repo.CreateSite(SiteData{AuthMethod: "cookie"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "站点名称不能为空")
}

func TestSiteRepository_UpdateDownloaderMethods(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{})
	repo := NewSiteRepository(db)

	id, err := repo.CreateSite(SiteData{Name: "s1", AuthMethod: "cookie"})
	require.NoError(t, err)

	dlID := uint(42)
	require.NoError(t, repo.UpdateSiteDownloader("s1", &dlID))
	site, err := repo.GetSiteByName("s1")
	require.NoError(t, err)
	require.NotNil(t, site.DownloaderID)
	assert.Equal(t, dlID, *site.DownloaderID)

	dlID2 := uint(99)
	require.NoError(t, repo.UpdateSiteDownloaderByID(id, &dlID2))
	site, err = repo.GetSiteByID(id)
	require.NoError(t, err)
	require.NotNil(t, site.DownloaderID)
	assert.Equal(t, dlID2, *site.DownloaderID)

	// clear downloader
	require.NoError(t, repo.UpdateSiteDownloader("s1", nil))
	site, _ = repo.GetSiteByName("s1")
	assert.Nil(t, site.DownloaderID)

	// empty batch → 0 rows, no error
	rows, err := repo.BatchUpdateSiteDownloader(nil, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(0), rows)
}

func TestSiteRepository_NotFoundPaths(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{})
	repo := NewSiteRepository(db)

	_, err := repo.GetSiteByName("nope")
	assert.Error(t, err)
	_, err = repo.GetSiteByID(12345)
	assert.Error(t, err)

	exists, err := repo.SiteExistsByName("nope")
	require.NoError(t, err)
	assert.False(t, exists)

	// CreateSite empty auth method
	_, err = repo.CreateSite(SiteData{Name: "x"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "认证方式不能为空")
}
