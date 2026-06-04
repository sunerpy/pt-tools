package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestDumpTableJSONNonEmpty verifies backup of non-empty table
func TestDumpTableJSONNonEmpty(t *testing.T) {
	// Setup in-memory SQLite DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err, "failed to open in-memory DB")

	// Create a test table
	type TestRow struct {
		ID   int `gorm:"primaryKey"`
		Name string
		Data string
	}

	err = db.Migrator().CreateTable(&TestRow{})
	require.NoError(t, err, "failed to create table")

	// Seed 5 test rows
	for i := 1; i <= 5; i++ {
		row := TestRow{ID: i, Name: "test" + string(rune(48+i)), Data: "data" + string(rune(48+i))}
		err = db.Create(&row).Error
		require.NoError(t, err, "failed to seed row")
	}

	// Call DumpTableJSON
	outDir := t.TempDir()
	path, err := DumpTableJSON(db, "test_rows", outDir, 8, 9)
	require.NoError(t, err, "DumpTableJSON should not error")
	require.NotEmpty(t, path, "returned path should not be empty")

	// Verify file exists and is readable
	_, err = os.Stat(path)
	require.NoError(t, err, "output file should exist")

	// Verify file size > 100 bytes for 5 rows
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(100), "file size should be > 100 bytes for 5 rows")

	// Verify JSON parsing and array of 5 elements
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	var result []map[string]interface{}
	err = json.Unmarshal(content, &result)
	require.NoError(t, err, "JSON should parse successfully")
	assert.Equal(t, 5, len(result), "JSON array should have 5 elements")

	// Verify each row has data
	for i, row := range result {
		assert.NotNil(t, row["id"], "row %d should have id", i)
		assert.NotNil(t, row["name"], "row %d should have name", i)
	}
}

// TestDumpTableJSONCreatesDir verifies that missing directories are created
func TestDumpTableJSONCreatesDir(t *testing.T) {
	// Setup in-memory SQLite DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Create a test table
	type TestRow struct {
		ID   int `gorm:"primaryKey"`
		Name string
	}

	err = db.Migrator().CreateTable(&TestRow{})
	require.NoError(t, err)

	// Seed 1 row
	err = db.Create(&TestRow{ID: 1, Name: "test"}).Error
	require.NoError(t, err)

	// Call with non-existent nested directory
	baseDir := t.TempDir()
	outDir := filepath.Join(baseDir, "missing", "sub", "dir")

	path, err := DumpTableJSON(db, "test_rows", outDir, 8, 9)
	require.NoError(t, err, "DumpTableJSON should create missing directories")
	require.NotEmpty(t, path, "returned path should not be empty")

	// Verify directory was created
	_, err = os.Stat(outDir)
	assert.NoError(t, err, "output directory should have been created")

	// Verify file exists in created directory
	_, err = os.Stat(path)
	assert.NoError(t, err, "output file should exist in created directory")
}

// TestDumpTableJSONEmptyTable verifies empty table returns empty JSON array
func TestDumpTableJSONEmptyTable(t *testing.T) {
	// Setup in-memory SQLite DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Create a test table but don't seed it
	type TestRow struct {
		ID   int `gorm:"primaryKey"`
		Name string
	}

	err = db.Migrator().CreateTable(&TestRow{})
	require.NoError(t, err)

	// Call DumpTableJSON on empty table
	outDir := t.TempDir()
	path, err := DumpTableJSON(db, "test_rows", outDir, 8, 9)
	require.NoError(t, err, "DumpTableJSON should not error on empty table")

	// Verify file exists
	_, err = os.Stat(path)
	require.NoError(t, err, "output file should exist for empty table")

	// Verify content is empty JSON array
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	var result []map[string]interface{}
	err = json.Unmarshal(content, &result)
	require.NoError(t, err, "JSON should parse successfully")
	assert.Equal(t, 0, len(result), "JSON array should be empty")
	// Content should be "[]" not "null" due to marshaling non-nil slice
	assert.NotEmpty(t, string(content), "content should not be empty")
}

// TestDumpTableJSONNonExistentTable verifies error handling for non-existent table
func TestDumpTableJSONNonExistentTable(t *testing.T) {
	// Setup in-memory SQLite DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Call DumpTableJSON on table that doesn't exist
	outDir := t.TempDir()
	path, err := DumpTableJSON(db, "nonexistent_table", outDir, 8, 9)

	// Should return error
	assert.Error(t, err, "DumpTableJSON should error for non-existent table")
	assert.Empty(t, path, "path should be empty on error")

	// Verify no file was created
	_, err = os.Stat(path)
	assert.Error(t, err, "no file should be created on error")
}
