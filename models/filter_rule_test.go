package models

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupFilterRuleTestDB(t *testing.T) (*TorrentDB, func()) {
	t.Helper()

	// Create a temporary directory for the test database
	tmpDir, err := os.MkdirTemp("", "filter_rule_test")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Migrate the FilterRule table
	err = db.AutoMigrate(&FilterRule{})
	require.NoError(t, err)

	torrentDB := &TorrentDB{DB: db}

	cleanup := func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		os.RemoveAll(tmpDir)
	}

	return torrentDB, cleanup
}

func TestFilterRuleModel(t *testing.T) {
	db, cleanup := setupFilterRuleTestDB(t)
	defer cleanup()

	filterDB := NewFilterRuleDB(db)

	t.Run("Create and GetByID", func(t *testing.T) {
		rule := &FilterRule{
			Name:        "Test Rule",
			Pattern:     "test*",
			PatternType: PatternWildcard,
			RequireFree: true,
			Enabled:     true,
			Priority:    50,
		}

		err := filterDB.Create(rule)
		require.NoError(t, err)
		assert.NotZero(t, rule.ID)

		retrieved, err := filterDB.GetByID(rule.ID)
		require.NoError(t, err)
		assert.Equal(t, rule.Name, retrieved.Name)
		assert.Equal(t, rule.Pattern, retrieved.Pattern)
		assert.Equal(t, rule.PatternType, retrieved.PatternType)
		assert.Equal(t, rule.RequireFree, retrieved.RequireFree)
		assert.Equal(t, rule.Enabled, retrieved.Enabled)
		assert.Equal(t, rule.Priority, retrieved.Priority)
	})

	t.Run("Default values", func(t *testing.T) {
		rule := &FilterRule{
			Name:    "Default Test",
			Pattern: "keyword",
		}

		err := filterDB.Create(rule)
		require.NoError(t, err)

		retrieved, err := filterDB.GetByID(rule.ID)
		require.NoError(t, err)
		assert.Equal(t, PatternKeyword, retrieved.PatternType)
		assert.True(t, retrieved.RequireFree)
		assert.True(t, retrieved.Enabled)
		assert.Equal(t, 100, retrieved.Priority)
	})

	t.Run("GetAll returns ordered by priority", func(t *testing.T) {
		// Clear existing rules
		db.DB.Exec("DELETE FROM filter_rules")

		rules := []*FilterRule{
			{Name: "Low Priority", Pattern: "low", Priority: 200},
			{Name: "High Priority", Pattern: "high", Priority: 10},
			{Name: "Medium Priority", Pattern: "medium", Priority: 100},
		}

		for _, r := range rules {
			err := filterDB.Create(r)
			require.NoError(t, err)
		}

		all, err := filterDB.GetAll()
		require.NoError(t, err)
		require.Len(t, all, 3)
		assert.Equal(t, "High Priority", all[0].Name)
		assert.Equal(t, "Medium Priority", all[1].Name)
		assert.Equal(t, "Low Priority", all[2].Name)
	})

	t.Run("GetEnabled filters disabled rules", func(t *testing.T) {
		// Clear existing rules
		db.DB.Exec("DELETE FROM filter_rules")

		enabledRule := &FilterRule{Name: "Enabled Rule", Pattern: "enabled", Enabled: true}
		disabledRule := &FilterRule{Name: "Disabled Rule", Pattern: "disabled"}

		err := filterDB.Create(enabledRule)
		require.NoError(t, err)
		err = filterDB.Create(disabledRule)
		require.NoError(t, err)

		// Explicitly disable the second rule
		disabledRule.Enabled = false
		err = filterDB.Update(disabledRule)
		require.NoError(t, err)

		enabled, err := filterDB.GetEnabled()
		require.NoError(t, err)
		require.Len(t, enabled, 1)
		assert.Equal(t, "Enabled Rule", enabled[0].Name)
	})

	t.Run("Update", func(t *testing.T) {
		rule := &FilterRule{
			Name:    "Update Test",
			Pattern: "original",
		}

		err := filterDB.Create(rule)
		require.NoError(t, err)

		rule.Pattern = "updated"
		rule.Priority = 25

		err = filterDB.Update(rule)
		require.NoError(t, err)

		retrieved, err := filterDB.GetByID(rule.ID)
		require.NoError(t, err)
		assert.Equal(t, "updated", retrieved.Pattern)
		assert.Equal(t, 25, retrieved.Priority)
	})

	t.Run("Delete", func(t *testing.T) {
		rule := &FilterRule{
			Name:    "Delete Test",
			Pattern: "delete",
		}

		err := filterDB.Create(rule)
		require.NoError(t, err)

		err = filterDB.Delete(rule.ID)
		require.NoError(t, err)

		_, err = filterDB.GetByID(rule.ID)
		assert.Error(t, err)
	})

	t.Run("Exists", func(t *testing.T) {
		rule := &FilterRule{
			Name:    "Exists Test",
			Pattern: "exists",
		}

		err := filterDB.Create(rule)
		require.NoError(t, err)

		exists, err := filterDB.Exists("Exists Test")
		require.NoError(t, err)
		assert.True(t, exists)

		exists, err = filterDB.Exists("Non-existent")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("ExistsExcluding", func(t *testing.T) {
		rule1 := &FilterRule{Name: "Rule 1", Pattern: "r1"}
		rule2 := &FilterRule{Name: "Rule 2", Pattern: "r2"}

		err := filterDB.Create(rule1)
		require.NoError(t, err)
		err = filterDB.Create(rule2)
		require.NoError(t, err)

		// Should find Rule 1 when excluding Rule 2
		exists, err := filterDB.ExistsExcluding("Rule 1", rule2.ID)
		require.NoError(t, err)
		assert.True(t, exists)

		// Should not find Rule 1 when excluding Rule 1
		exists, err = filterDB.ExistsExcluding("Rule 1", rule1.ID)
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("GetBySiteID", func(t *testing.T) {
		// Clear existing rules
		db.DB.Exec("DELETE FROM filter_rules")

		siteID := uint(1)
		otherSiteID := uint(2)

		rules := []*FilterRule{
			{Name: "Site 1 Rule", Pattern: "s1", SiteID: &siteID, Enabled: true},
			{Name: "Site 2 Rule", Pattern: "s2", SiteID: &otherSiteID, Enabled: true},
			{Name: "Global Rule", Pattern: "global", SiteID: nil, Enabled: true},
		}

		for _, r := range rules {
			err := filterDB.Create(r)
			require.NoError(t, err)
		}

		// Should return site-specific and global rules
		siteRules, err := filterDB.GetBySiteID(siteID)
		require.NoError(t, err)
		require.Len(t, siteRules, 2)
	})

	t.Run("GetByRSSID", func(t *testing.T) {
		// Clear existing rules
		db.DB.Exec("DELETE FROM filter_rules")

		rssID := uint(1)
		otherRSSID := uint(2)

		rules := []*FilterRule{
			{Name: "RSS 1 Rule", Pattern: "r1", RSSID: &rssID, Enabled: true},
			{Name: "RSS 2 Rule", Pattern: "r2", RSSID: &otherRSSID, Enabled: true},
			{Name: "Global Rule", Pattern: "global", RSSID: nil, Enabled: true},
		}

		for _, r := range rules {
			err := filterDB.Create(r)
			require.NoError(t, err)
		}

		// Should return RSS-specific and global rules
		rssRules, err := filterDB.GetByRSSID(rssID)
		require.NoError(t, err)
		require.Len(t, rssRules, 2)
	})
}

func TestPatternTypeConstants(t *testing.T) {
	assert.Equal(t, PatternType("keyword"), PatternKeyword)
	assert.Equal(t, PatternType("wildcard"), PatternWildcard)
	assert.Equal(t, PatternType("regex"), PatternRegex)
}
