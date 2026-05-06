package store

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openMemoryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	return db
}

func TestMigrateCreatesTables(t *testing.T) {
	db := openMemoryDB(t)
	require.NoError(t, Migrate(db))

	expected := []string{
		"media_library_configs",
		"provider_credentials",
		"connector_configs",
		"scrape_tasks",
		"scrape_results",
		"scraper_overrides",
		"scraper_schema_versions",
	}
	for _, table := range expected {
		var count int
		require.NoError(t, db.Raw(
			"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?",
			table,
		).Scan(&count).Error)
		require.Equalf(t, 1, count, "table %s should exist", table)
	}
}

func TestMigrateIdempotent(t *testing.T) {
	db := openMemoryDB(t)
	require.NoError(t, Migrate(db))
	require.NoError(t, Migrate(db), "second Migrate call should succeed")
	require.NoError(t, Migrate(db), "third Migrate call should succeed")

	// 版本应该只是 current（而非增长）
	var versions []ScraperSchemaVersion
	require.NoError(t, db.Find(&versions).Error)
	require.Len(t, versions, 1)
	require.Equal(t, CurrentScraperSchemaVersion, versions[0].Version)
}

func TestInsertAndQueryLibrary(t *testing.T) {
	db := openMemoryDB(t)
	require.NoError(t, Migrate(db))

	lib := MediaLibraryConfig{
		Name: "Movies",
		Type: "movie",
		Path: "/media/movies",
	}
	require.NoError(t, db.Create(&lib).Error)
	require.Greater(t, lib.ID, uint(0))

	var got MediaLibraryConfig
	require.NoError(t, db.First(&got, lib.ID).Error)
	require.Equal(t, "Movies", got.Name)
}
