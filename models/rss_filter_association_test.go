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

func setupRSSFilterAssociationTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()

	// Create a temporary directory for the test database
	tmpDir, err := os.MkdirTemp("", "rss_filter_assoc_test")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Migrate the required tables
	err = db.AutoMigrate(&FilterRule{}, &RSSSubscription{}, &RSSFilterAssociation{})
	require.NoError(t, err)

	cleanup := func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		os.RemoveAll(tmpDir)
	}

	return db, cleanup
}

func createTestFilterRules(t *testing.T, db *gorm.DB, count int) []FilterRule {
	t.Helper()
	rules := make([]FilterRule, count)
	for i := 0; i < count; i++ {
		rules[i] = FilterRule{
			Name:        "Test Rule " + string(rune('A'+i)),
			Pattern:     "pattern" + string(rune('A'+i)),
			PatternType: PatternKeyword,
			Enabled:     true,
			Priority:    (i + 1) * 10,
		}
		err := db.Create(&rules[i]).Error
		require.NoError(t, err)
	}
	return rules
}

func createTestRSSSubscription(t *testing.T, db *gorm.DB, name string) RSSSubscription {
	t.Helper()
	rss := RSSSubscription{
		Name:            name,
		URL:             "https://example.com/rss/" + name,
		IntervalMinutes: 5, // Required: CHECK constraint requires >= 1
	}
	err := db.Create(&rss).Error
	require.NoError(t, err)
	return rss
}

func TestRSSFilterAssociationModel(t *testing.T) {
	db, cleanup := setupRSSFilterAssociationTestDB(t)
	defer cleanup()

	assocDB := NewRSSFilterAssociationDB(db)

	t.Run("Create association", func(t *testing.T) {
		rules := createTestFilterRules(t, db, 1)
		rss := createTestRSSSubscription(t, db, "test-rss-1")

		assoc := &RSSFilterAssociation{
			RSSID:        rss.ID,
			FilterRuleID: rules[0].ID,
		}

		err := assocDB.Create(assoc)
		require.NoError(t, err)
		assert.NotZero(t, assoc.ID)
	})

	t.Run("Unique constraint prevents duplicate associations", func(t *testing.T) {
		// Clear existing data
		db.Exec("DELETE FROM rss_filter_associations")
		db.Exec("DELETE FROM filter_rules")
		db.Exec("DELETE FROM rss_subscriptions")

		rules := createTestFilterRules(t, db, 1)
		rss := createTestRSSSubscription(t, db, "test-rss-unique")

		assoc1 := &RSSFilterAssociation{
			RSSID:        rss.ID,
			FilterRuleID: rules[0].ID,
		}
		err := assocDB.Create(assoc1)
		require.NoError(t, err)

		// Try to create duplicate
		assoc2 := &RSSFilterAssociation{
			RSSID:        rss.ID,
			FilterRuleID: rules[0].ID,
		}
		err = assocDB.Create(assoc2)
		assert.Error(t, err) // Should fail due to unique constraint
	})

	t.Run("GetByRSSID returns associated filter rule IDs", func(t *testing.T) {
		// Clear existing data
		db.Exec("DELETE FROM rss_filter_associations")
		db.Exec("DELETE FROM filter_rules")
		db.Exec("DELETE FROM rss_subscriptions")

		rules := createTestFilterRules(t, db, 3)
		rss := createTestRSSSubscription(t, db, "test-rss-get")

		// Associate first two rules
		for i := 0; i < 2; i++ {
			assoc := &RSSFilterAssociation{
				RSSID:        rss.ID,
				FilterRuleID: rules[i].ID,
			}
			err := assocDB.Create(assoc)
			require.NoError(t, err)
		}

		ids, err := assocDB.GetByRSSID(rss.ID)
		require.NoError(t, err)
		assert.Len(t, ids, 2)
		assert.Contains(t, ids, rules[0].ID)
		assert.Contains(t, ids, rules[1].ID)
		assert.NotContains(t, ids, rules[2].ID)
	})

	t.Run("GetByFilterRuleID returns associated RSS IDs", func(t *testing.T) {
		// Clear existing data
		db.Exec("DELETE FROM rss_filter_associations")
		db.Exec("DELETE FROM filter_rules")
		db.Exec("DELETE FROM rss_subscriptions")

		rules := createTestFilterRules(t, db, 1)
		rss1 := createTestRSSSubscription(t, db, "test-rss-a")
		rss2 := createTestRSSSubscription(t, db, "test-rss-b")
		rss3 := createTestRSSSubscription(t, db, "test-rss-c")

		// Associate rule with first two RSS
		for _, rss := range []RSSSubscription{rss1, rss2} {
			assoc := &RSSFilterAssociation{
				RSSID:        rss.ID,
				FilterRuleID: rules[0].ID,
			}
			err := assocDB.Create(assoc)
			require.NoError(t, err)
		}

		ids, err := assocDB.GetByFilterRuleID(rules[0].ID)
		require.NoError(t, err)
		assert.Len(t, ids, 2)
		assert.Contains(t, ids, rss1.ID)
		assert.Contains(t, ids, rss2.ID)
		assert.NotContains(t, ids, rss3.ID)
	})

	t.Run("GetFilterRulesForRSS returns enabled rules ordered by priority", func(t *testing.T) {
		// Clear existing data
		db.Exec("DELETE FROM rss_filter_associations")
		db.Exec("DELETE FROM filter_rules")
		db.Exec("DELETE FROM rss_subscriptions")

		// Create rules with different priorities
		rule1 := FilterRule{Name: "Low Priority", Pattern: "low", PatternType: PatternKeyword, Enabled: true, Priority: 100}
		rule2 := FilterRule{Name: "High Priority", Pattern: "high", PatternType: PatternKeyword, Enabled: true, Priority: 10}
		rule3 := FilterRule{Name: "Disabled", Pattern: "disabled", PatternType: PatternKeyword, Enabled: true, Priority: 5} // Will be disabled after creation
		db.Create(&rule1)
		db.Create(&rule2)
		db.Create(&rule3)

		// Disable rule3 after creation to avoid GORM default value issue
		db.Model(&rule3).Update("enabled", false)

		rss := createTestRSSSubscription(t, db, "test-rss-priority")

		// Associate all rules
		for _, rule := range []FilterRule{rule1, rule2, rule3} {
			assoc := &RSSFilterAssociation{RSSID: rss.ID, FilterRuleID: rule.ID}
			assocDB.Create(assoc)
		}

		rules, err := assocDB.GetFilterRulesForRSS(rss.ID)
		require.NoError(t, err)
		assert.Len(t, rules, 2) // Only enabled rules
		assert.Equal(t, "High Priority", rules[0].Name)
		assert.Equal(t, "Low Priority", rules[1].Name)
	})

	t.Run("SetFilterRulesForRSS replaces all associations", func(t *testing.T) {
		// Clear existing data
		db.Exec("DELETE FROM rss_filter_associations")
		db.Exec("DELETE FROM filter_rules")
		db.Exec("DELETE FROM rss_subscriptions")

		rules := createTestFilterRules(t, db, 4)
		rss := createTestRSSSubscription(t, db, "test-rss-set")

		// Initially associate first two rules
		err := assocDB.SetFilterRulesForRSS(rss.ID, []uint{rules[0].ID, rules[1].ID})
		require.NoError(t, err)

		ids, err := assocDB.GetByRSSID(rss.ID)
		require.NoError(t, err)
		assert.Len(t, ids, 2)

		// Replace with last two rules
		err = assocDB.SetFilterRulesForRSS(rss.ID, []uint{rules[2].ID, rules[3].ID})
		require.NoError(t, err)

		ids, err = assocDB.GetByRSSID(rss.ID)
		require.NoError(t, err)
		assert.Len(t, ids, 2)
		assert.Contains(t, ids, rules[2].ID)
		assert.Contains(t, ids, rules[3].ID)
		assert.NotContains(t, ids, rules[0].ID)
		assert.NotContains(t, ids, rules[1].ID)
	})

	t.Run("SetFilterRulesForRSS with empty list clears associations", func(t *testing.T) {
		// Clear existing data
		db.Exec("DELETE FROM rss_filter_associations")
		db.Exec("DELETE FROM filter_rules")
		db.Exec("DELETE FROM rss_subscriptions")

		rules := createTestFilterRules(t, db, 2)
		rss := createTestRSSSubscription(t, db, "test-rss-clear")

		// Initially associate rules
		err := assocDB.SetFilterRulesForRSS(rss.ID, []uint{rules[0].ID, rules[1].ID})
		require.NoError(t, err)

		// Clear associations
		err = assocDB.SetFilterRulesForRSS(rss.ID, []uint{})
		require.NoError(t, err)

		ids, err := assocDB.GetByRSSID(rss.ID)
		require.NoError(t, err)
		assert.Len(t, ids, 0)
	})

	t.Run("DeleteByRSSID removes all associations for RSS", func(t *testing.T) {
		// Clear existing data
		db.Exec("DELETE FROM rss_filter_associations")
		db.Exec("DELETE FROM filter_rules")
		db.Exec("DELETE FROM rss_subscriptions")

		rules := createTestFilterRules(t, db, 2)
		rss := createTestRSSSubscription(t, db, "test-rss-delete")

		// Create associations
		for _, rule := range rules {
			assoc := &RSSFilterAssociation{RSSID: rss.ID, FilterRuleID: rule.ID}
			assocDB.Create(assoc)
		}

		err := assocDB.DeleteByRSSID(rss.ID)
		require.NoError(t, err)

		ids, err := assocDB.GetByRSSID(rss.ID)
		require.NoError(t, err)
		assert.Len(t, ids, 0)
	})

	t.Run("DeleteByFilterRuleID removes all associations for filter rule", func(t *testing.T) {
		// Clear existing data
		db.Exec("DELETE FROM rss_filter_associations")
		db.Exec("DELETE FROM filter_rules")
		db.Exec("DELETE FROM rss_subscriptions")

		rules := createTestFilterRules(t, db, 1)
		rss1 := createTestRSSSubscription(t, db, "test-rss-del-1")
		rss2 := createTestRSSSubscription(t, db, "test-rss-del-2")

		// Create associations
		for _, rss := range []RSSSubscription{rss1, rss2} {
			assoc := &RSSFilterAssociation{RSSID: rss.ID, FilterRuleID: rules[0].ID}
			assocDB.Create(assoc)
		}

		err := assocDB.DeleteByFilterRuleID(rules[0].ID)
		require.NoError(t, err)

		ids, err := assocDB.GetByFilterRuleID(rules[0].ID)
		require.NoError(t, err)
		assert.Len(t, ids, 0)
	})

	t.Run("Exists checks association existence", func(t *testing.T) {
		// Clear existing data
		db.Exec("DELETE FROM rss_filter_associations")
		db.Exec("DELETE FROM filter_rules")
		db.Exec("DELETE FROM rss_subscriptions")

		rules := createTestFilterRules(t, db, 2)
		rss := createTestRSSSubscription(t, db, "test-rss-exists")

		// Create one association
		assoc := &RSSFilterAssociation{RSSID: rss.ID, FilterRuleID: rules[0].ID}
		assocDB.Create(assoc)

		exists, err := assocDB.Exists(rss.ID, rules[0].ID)
		require.NoError(t, err)
		assert.True(t, exists)

		exists, err = assocDB.Exists(rss.ID, rules[1].ID)
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("HasAssociations checks if RSS has any associations", func(t *testing.T) {
		// Clear existing data
		db.Exec("DELETE FROM rss_filter_associations")
		db.Exec("DELETE FROM filter_rules")
		db.Exec("DELETE FROM rss_subscriptions")

		rules := createTestFilterRules(t, db, 1)
		rss1 := createTestRSSSubscription(t, db, "test-rss-has-1")
		rss2 := createTestRSSSubscription(t, db, "test-rss-has-2")

		// Create association for rss1 only
		assoc := &RSSFilterAssociation{RSSID: rss1.ID, FilterRuleID: rules[0].ID}
		assocDB.Create(assoc)

		has, err := assocDB.HasAssociations(rss1.ID)
		require.NoError(t, err)
		assert.True(t, has)

		has, err = assocDB.HasAssociations(rss2.ID)
		require.NoError(t, err)
		assert.False(t, has)
	})
}

func TestRSSFilterAssociationTableName(t *testing.T) {
	assoc := RSSFilterAssociation{}
	assert.Equal(t, "rss_filter_associations", assoc.TableName())
}
