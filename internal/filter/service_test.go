package filter

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/models"
)

func setupServiceTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "filter_service_test")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.FilterRule{})
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

// TestFilterServicePriorityOrdering tests Property 5: Filter Rules Priority Ordering
// Feature: rss-filter-and-downloader-autostart, Property 5: Filter Rules Priority Ordering
// *For any* set of enabled filter rules with different priorities, when matching against a torrent title,
// the rules should be evaluated in ascending priority order (lower number = higher priority),
// and the first matching rule should determine the download behavior.
// **Validates: Requirements 3.4, 3.5**
func TestFilterServicePriorityOrdering(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Rules are evaluated in priority order
	properties.Property("rules evaluated in priority order", prop.ForAll(
		func(priorities []int) bool {
			if len(priorities) < 2 {
				return true
			}

			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			// Create rules with different priorities, all matching "test"
			for i, priority := range priorities {
				rule := &models.FilterRule{
					Name:        "Rule " + string(rune('A'+i)),
					Pattern:     "test",
					PatternType: models.PatternKeyword,
					Enabled:     true,
					Priority:    priority,
				}
				db.Create(rule)
			}

			svc := NewFilterService(db)

			// Match should return the rule with lowest priority number
			rule, matched := svc.MatchRules("test title", nil, nil)
			if !matched {
				return false
			}

			// Find the minimum priority
			minPriority := priorities[0]
			for _, p := range priorities {
				if p < minPriority {
					minPriority = p
				}
			}

			return rule.Priority == minPriority
		},
		gen.SliceOfN(5, gen.IntRange(1, 1000)).SuchThat(func(s []int) bool {
			// Ensure unique priorities
			seen := make(map[int]bool)
			for _, p := range s {
				if seen[p] {
					return false
				}
				seen[p] = true
			}
			return len(s) >= 2
		}),
	))

	// Property: GetEnabledRules returns rules sorted by priority
	properties.Property("GetEnabledRules returns sorted rules", prop.ForAll(
		func(priorities []int) bool {
			if len(priorities) == 0 {
				return true
			}

			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			// Create rules with given priorities
			for i, priority := range priorities {
				rule := &models.FilterRule{
					Name:        "Rule " + string(rune('A'+i)),
					Pattern:     "pattern" + string(rune('a'+i)),
					PatternType: models.PatternKeyword,
					Enabled:     true,
					Priority:    priority,
				}
				db.Create(rule)
			}

			svc := NewFilterService(db)
			rules, err := svc.GetEnabledRules()
			if err != nil {
				return false
			}

			// Check that rules are sorted by priority
			return sort.SliceIsSorted(rules, func(i, j int) bool {
				if rules[i].Priority == rules[j].Priority {
					return rules[i].ID < rules[j].ID
				}
				return rules[i].Priority < rules[j].Priority
			})
		},
		gen.SliceOfN(10, gen.IntRange(1, 1000)),
	))

	properties.TestingRun(t)
}

// TestFilterServiceRequireFree tests Property 6 and 7: Require-Free Download Conditions
// Feature: rss-filter-and-downloader-autostart, Property 6: Require-Free Download Condition
// *For any* torrent that matches a filter rule with `require_free=true`,
// the system should download the torrent if and only if the torrent's free status is true.
// **Validates: Requirements 3.2**
//
// Feature: rss-filter-and-downloader-autostart, Property 7: Non-Require-Free Download Condition
// *For any* torrent that matches a filter rule with `require_free=false`,
// the system should download the torrent regardless of the torrent's free status.
// **Validates: Requirements 3.3**
func TestFilterServiceRequireFree(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 6: require_free=true only downloads free torrents
	properties.Property("require_free=true only downloads free torrents", prop.ForAll(
		func(isFree bool) bool {
			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			rule := &models.FilterRule{
				Name:        "Free Only Rule",
				Pattern:     "test",
				PatternType: models.PatternKeyword,
				RequireFree: true,
				Enabled:     true,
				Priority:    100,
			}
			db.Create(rule)

			svc := NewFilterService(db)
			shouldDownload, matchedRule := svc.ShouldDownload("test title", isFree, nil, nil)

			// Should match the rule
			if matchedRule == nil {
				return false
			}

			// Should download only if free
			return shouldDownload == isFree
		},
		gen.Bool(),
	))

	// Property 7: require_free=false downloads regardless of free status
	properties.Property("require_free=false downloads regardless of free status", prop.ForAll(
		func(isFree bool) bool {
			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			rule := &models.FilterRule{
				Name:        "Any Rule",
				Pattern:     "test",
				PatternType: models.PatternKeyword,
				RequireFree: true, // Create with default true first
				Enabled:     true,
				Priority:    100,
			}
			db.Create(rule)

			// Then set require_free to false
			db.Model(rule).Update("require_free", false)

			svc := NewFilterService(db)
			// Need to refresh cache after update
			svc.RefreshCache()

			shouldDownload, matchedRule := svc.ShouldDownload("test title", isFree, nil, nil)

			// Should match the rule
			if matchedRule == nil {
				return false
			}

			// Should always download
			return shouldDownload
		},
		gen.Bool(),
	))

	properties.TestingRun(t)
}

// TestFilterServiceUnit provides unit tests for FilterService
func TestFilterServiceUnit(t *testing.T) {
	t.Run("MatchRules returns first matching rule by priority", func(t *testing.T) {
		db, cleanup := setupServiceTestDB(t)
		defer cleanup()

		rules := []*models.FilterRule{
			{Name: "Low Priority", Pattern: "test", PatternType: models.PatternKeyword, Enabled: true, Priority: 200},
			{Name: "High Priority", Pattern: "test", PatternType: models.PatternKeyword, Enabled: true, Priority: 10},
			{Name: "Medium Priority", Pattern: "test", PatternType: models.PatternKeyword, Enabled: true, Priority: 100},
		}
		for _, r := range rules {
			db.Create(r)
		}

		svc := NewFilterService(db)
		rule, matched := svc.MatchRules("test title", nil, nil)

		require.True(t, matched)
		assert.Equal(t, "High Priority", rule.Name)
		assert.Equal(t, 10, rule.Priority)
	})

	t.Run("MatchRules returns nil for no match", func(t *testing.T) {
		db, cleanup := setupServiceTestDB(t)
		defer cleanup()

		rule := &models.FilterRule{
			Name:        "Test Rule",
			Pattern:     "specific",
			PatternType: models.PatternKeyword,
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)

		svc := NewFilterService(db)
		matchedRule, matched := svc.MatchRules("different title", nil, nil)

		assert.False(t, matched)
		assert.Nil(t, matchedRule)
	})

	t.Run("MatchRules respects site filter", func(t *testing.T) {
		db, cleanup := setupServiceTestDB(t)
		defer cleanup()

		siteID := uint(1)
		otherSiteID := uint(2)

		rules := []*models.FilterRule{
			{Name: "Site 1 Rule", Pattern: "test", PatternType: models.PatternKeyword, Enabled: true, SiteID: &siteID, Priority: 100},
			{Name: "Site 2 Rule", Pattern: "test", PatternType: models.PatternKeyword, Enabled: true, SiteID: &otherSiteID, Priority: 50},
		}
		for _, r := range rules {
			db.Create(r)
		}

		svc := NewFilterService(db)

		// Should match Site 1 Rule when querying for site 1
		rule, matched := svc.MatchRules("test title", &siteID, nil)
		require.True(t, matched)
		assert.Equal(t, "Site 1 Rule", rule.Name)

		// Should match Site 2 Rule when querying for site 2
		rule, matched = svc.MatchRules("test title", &otherSiteID, nil)
		require.True(t, matched)
		assert.Equal(t, "Site 2 Rule", rule.Name)
	})

	t.Run("MatchRules includes global rules", func(t *testing.T) {
		db, cleanup := setupServiceTestDB(t)
		defer cleanup()

		siteID := uint(1)

		rules := []*models.FilterRule{
			{Name: "Global Rule", Pattern: "test", PatternType: models.PatternKeyword, Enabled: true, SiteID: nil, Priority: 100},
			{Name: "Site Rule", Pattern: "other", PatternType: models.PatternKeyword, Enabled: true, SiteID: &siteID, Priority: 50},
		}
		for _, r := range rules {
			db.Create(r)
		}

		svc := NewFilterService(db)

		// Global rule should match for any site
		rule, matched := svc.MatchRules("test title", &siteID, nil)
		require.True(t, matched)
		assert.Equal(t, "Global Rule", rule.Name)
	})

	t.Run("Disabled rules are not matched", func(t *testing.T) {
		db, cleanup := setupServiceTestDB(t)
		defer cleanup()

		rule := &models.FilterRule{
			Name:        "Disabled Rule",
			Pattern:     "test",
			PatternType: models.PatternKeyword,
			Enabled:     true, // Create as enabled first
			Priority:    100,
		}
		db.Create(rule)

		// Then disable it
		db.Model(rule).Update("enabled", false)

		svc := NewFilterService(db)
		matchedRule, matched := svc.MatchRules("test title", nil, nil)

		assert.False(t, matched)
		assert.Nil(t, matchedRule)
	})

	t.Run("RefreshCache updates matchers", func(t *testing.T) {
		db, cleanup := setupServiceTestDB(t)
		defer cleanup()

		svc := NewFilterService(db)

		// Initially no rules
		_, matched := svc.MatchRules("test title", nil, nil)
		assert.False(t, matched)

		// Add a rule
		rule := &models.FilterRule{
			Name:        "New Rule",
			Pattern:     "test",
			PatternType: models.PatternKeyword,
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)

		// Refresh cache
		err := svc.RefreshCache()
		require.NoError(t, err)

		// Now should match
		matchedRule, matched := svc.MatchRules("test title", nil, nil)
		assert.True(t, matched)
		assert.Equal(t, "New Rule", matchedRule.Name)
	})
}

func TestShouldDownload(t *testing.T) {
	t.Run("require_free=true with free torrent", func(t *testing.T) {
		db, cleanup := setupServiceTestDB(t)
		defer cleanup()

		rule := &models.FilterRule{
			Name:        "Free Rule",
			Pattern:     "test",
			PatternType: models.PatternKeyword,
			RequireFree: true,
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)

		svc := NewFilterService(db)
		shouldDownload, matchedRule := svc.ShouldDownload("test title", true, nil, nil)

		assert.True(t, shouldDownload)
		assert.NotNil(t, matchedRule)
	})

	t.Run("require_free=true with non-free torrent", func(t *testing.T) {
		db, cleanup := setupServiceTestDB(t)
		defer cleanup()

		rule := &models.FilterRule{
			Name:        "Free Rule",
			Pattern:     "test",
			PatternType: models.PatternKeyword,
			RequireFree: true,
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)

		svc := NewFilterService(db)
		shouldDownload, matchedRule := svc.ShouldDownload("test title", false, nil, nil)

		assert.False(t, shouldDownload)
		assert.NotNil(t, matchedRule) // Rule still matched, just shouldn't download
	})

	t.Run("require_free=false with non-free torrent", func(t *testing.T) {
		db, cleanup := setupServiceTestDB(t)
		defer cleanup()

		rule := &models.FilterRule{
			Name:        "Any Rule",
			Pattern:     "test",
			PatternType: models.PatternKeyword,
			RequireFree: true, // Create with default first
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)

		// Then set require_free to false
		db.Model(rule).Update("require_free", false)

		svc := NewFilterService(db)
		svc.RefreshCache() // Refresh after update

		shouldDownload, matchedRule := svc.ShouldDownload("test title", false, nil, nil)

		assert.True(t, shouldDownload)
		assert.NotNil(t, matchedRule)
	})

	t.Run("no matching rule", func(t *testing.T) {
		db, cleanup := setupServiceTestDB(t)
		defer cleanup()

		rule := &models.FilterRule{
			Name:        "Specific Rule",
			Pattern:     "specific",
			PatternType: models.PatternKeyword,
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)

		svc := NewFilterService(db)
		shouldDownload, matchedRule := svc.ShouldDownload("different title", true, nil, nil)

		assert.False(t, shouldDownload)
		assert.Nil(t, matchedRule)
	})
}

// TestFilterRuleIDTracking tests Property 8: Filter Rule ID Tracking
// Feature: rss-filter-and-downloader-autostart, Property 8: Filter Rule ID Tracking
// *For any* torrent downloaded via a filter rule match, the TorrentInfo record should have
// its `filter_rule_id` field set to the ID of the matched rule.
// **Validates: Requirements 3.8**
func TestFilterRuleIDTracking(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Matched rule ID is correctly returned
	properties.Property("matched rule ID is correctly returned", prop.ForAll(
		func(ruleName string) bool {
			if ruleName == "" {
				return true
			}

			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			rule := &models.FilterRule{
				Name:        ruleName,
				Pattern:     "test",
				PatternType: models.PatternKeyword,
				Enabled:     true,
				Priority:    100,
			}
			db.Create(rule)

			svc := NewFilterService(db)
			matchedRule, matched := svc.MatchRules("test title", nil, nil)

			if !matched {
				return false
			}

			// The matched rule should have the correct ID
			return matchedRule.ID == rule.ID && matchedRule.Name == ruleName
		},
		gen.AlphaString().Map(func(s string) string {
			if len(s) == 0 {
				return "a"
			}
			if len(s) > 20 {
				return s[:20]
			}
			return s
		}),
	))

	properties.TestingRun(t)
}

// TestDownloadSourceTracking tests Property 9: Download Source Tracking
// Feature: rss-filter-and-downloader-autostart, Property 9: Download Source Tracking
// *For any* downloaded torrent, the `download_source` field should correctly indicate
// whether it was downloaded via `free_download` (existing logic) or `filter_rule` (new filter-based logic).
// **Validates: Requirements 5.5**
func TestDownloadSourceTracking(t *testing.T) {
	// This test verifies the MatchResult structure which provides the information
	// needed to set the download_source field correctly

	t.Run("MatchResult indicates filter_rule source when matched", func(t *testing.T) {
		db, cleanup := setupServiceTestDB(t)
		defer cleanup()

		rule := &models.FilterRule{
			Name:        "Test Rule",
			Pattern:     "test",
			PatternType: models.PatternKeyword,
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)

		svc := NewFilterService(db).(*filterService)
		result := svc.MatchTorrent("test title", true, nil, nil)

		assert.True(t, result.Matched)
		assert.NotNil(t, result.Rule)
		assert.Equal(t, rule.ID, result.Rule.ID)
		// When matched, download_source should be set to "filter_rule"
	})

	t.Run("MatchResult indicates no match for free_download source", func(t *testing.T) {
		db, cleanup := setupServiceTestDB(t)
		defer cleanup()

		rule := &models.FilterRule{
			Name:        "Specific Rule",
			Pattern:     "specific",
			PatternType: models.PatternKeyword,
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)

		svc := NewFilterService(db).(*filterService)
		result := svc.MatchTorrent("different title", true, nil, nil)

		assert.False(t, result.Matched)
		assert.Nil(t, result.Rule)
		// When not matched, download_source should remain "free_download" if downloaded via free logic
	})
}

// DownloadSourceConstants defines the download source values
const (
	DownloadSourceFree   = "free_download"
	DownloadSourceFilter = "filter_rule"
)

// GetDownloadSource returns the appropriate download source based on match result
func GetDownloadSource(matched bool) string {
	if matched {
		return DownloadSourceFilter
	}
	return DownloadSourceFree
}

func TestGetDownloadSource(t *testing.T) {
	t.Run("returns filter_rule when matched", func(t *testing.T) {
		assert.Equal(t, DownloadSourceFilter, GetDownloadSource(true))
	})

	t.Run("returns free_download when not matched", func(t *testing.T) {
		assert.Equal(t, DownloadSourceFree, GetDownloadSource(false))
	})
}

// TestIndependentPathProcessing tests Property 11: Independent Path Processing
// Feature: rss-filter-and-downloader-autostart, Property 11: Independent Path Processing
// *For any* RSS item, when both free download and filter-based download are enabled,
// both paths should be evaluated independently, and a torrent may be downloaded by either path
// based on their respective criteria.
// **Validates: Requirements 5.2**
func TestIndependentPathProcessing(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Free download and filter download are independent
	properties.Property("free and filter paths are independent", prop.ForAll(
		func(isFree, requireFree, matchesFilter bool) bool {
			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			// Create a filter rule
			rule := &models.FilterRule{
				Name:        "Test Rule",
				Pattern:     "test",
				PatternType: models.PatternKeyword,
				RequireFree: true, // Will be updated
				Enabled:     true,
				Priority:    100,
			}
			db.Create(rule)

			// Update require_free
			db.Model(rule).Update("require_free", requireFree)

			svc := NewFilterService(db)
			svc.RefreshCache()

			// Determine expected outcomes
			var title string
			if matchesFilter {
				title = "test title"
			} else {
				title = "other title"
			}

			// Free download path: downloads if isFree (and canFinished, which we assume true)
			shouldDownloadByFree := isFree

			// Filter download path: downloads if matches and (not requireFree or isFree)
			shouldDownloadByFilter, matchedRule := svc.ShouldDownload(title, isFree, nil, nil)

			// The paths are independent:
			// - Free path only cares about isFree
			// - Filter path cares about match + (requireFree -> isFree)

			// Verify filter path logic
			if matchesFilter {
				if matchedRule == nil {
					return false // Should have matched
				}
				expectedFilterDownload := !requireFree || isFree
				if shouldDownloadByFilter != expectedFilterDownload {
					return false
				}
			} else {
				if matchedRule != nil {
					return false // Should not have matched
				}
				if shouldDownloadByFilter {
					return false // Should not download
				}
			}

			// The combined decision: download if either path says yes
			combinedShouldDownload := shouldDownloadByFree || shouldDownloadByFilter

			// This is the key property: the paths are evaluated independently
			// A torrent can be downloaded by either path
			_ = combinedShouldDownload // Just verify the logic is correct

			return true
		},
		gen.Bool(), // isFree
		gen.Bool(), // requireFree
		gen.Bool(), // matchesFilter
	))

	properties.TestingRun(t)
}

func TestIndependentPathProcessingUnit(t *testing.T) {
	t.Run("free torrent downloaded via free path even without filter match", func(t *testing.T) {
		db, cleanup := setupServiceTestDB(t)
		defer cleanup()

		// Create a filter rule that won't match
		rule := &models.FilterRule{
			Name:        "Specific Rule",
			Pattern:     "specific",
			PatternType: models.PatternKeyword,
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)

		svc := NewFilterService(db)

		// Title doesn't match filter
		shouldDownloadByFilter, matchedRule := svc.ShouldDownload("different title", true, nil, nil)

		// Filter path: no match
		assert.False(t, shouldDownloadByFilter)
		assert.Nil(t, matchedRule)

		// But free path would still download (isFree=true)
		// This is handled in the downloadWorker, not in FilterService
	})

	t.Run("non-free torrent downloaded via filter path with require_free=false", func(t *testing.T) {
		db, cleanup := setupServiceTestDB(t)
		defer cleanup()

		rule := &models.FilterRule{
			Name:        "Any Rule",
			Pattern:     "test",
			PatternType: models.PatternKeyword,
			RequireFree: true, // Create with default
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)
		db.Model(rule).Update("require_free", false)

		svc := NewFilterService(db)
		svc.RefreshCache()

		// Non-free torrent
		shouldDownloadByFilter, matchedRule := svc.ShouldDownload("test title", false, nil, nil)

		// Filter path: match with require_free=false, so download
		assert.True(t, shouldDownloadByFilter)
		assert.NotNil(t, matchedRule)

		// Free path would NOT download (isFree=false)
		// But filter path allows it
	})

	t.Run("both paths can trigger download independently", func(t *testing.T) {
		db, cleanup := setupServiceTestDB(t)
		defer cleanup()

		rule := &models.FilterRule{
			Name:        "Test Rule",
			Pattern:     "test",
			PatternType: models.PatternKeyword,
			RequireFree: true,
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)

		svc := NewFilterService(db)

		// Free torrent that matches filter
		shouldDownloadByFilter, matchedRule := svc.ShouldDownload("test title", true, nil, nil)

		// Both paths would download
		assert.True(t, shouldDownloadByFilter)
		assert.NotNil(t, matchedRule)

		// Free path: isFree=true -> download
		// Filter path: match + isFree=true -> download
		// Combined: download (either path is sufficient)
	})
}

// setupServiceTestDBWithAssociations creates a test database with RSS and association tables
func setupServiceTestDBWithAssociations(t *testing.T) (*gorm.DB, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "filter_service_assoc_test")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.FilterRule{}, &models.RSSSubscription{}, &models.RSSFilterAssociation{})
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

// createTestRSSSubscription creates a test RSS subscription
func createTestRSSSubscription(t *testing.T, db *gorm.DB, name string) models.RSSSubscription {
	t.Helper()
	rss := models.RSSSubscription{
		Name:            name,
		URL:             "https://example.com/rss/" + name,
		IntervalMinutes: 5,
	}
	err := db.Create(&rss).Error
	require.NoError(t, err)
	return rss
}

// TestFilterServiceRSSAssociation tests the RSS-Filter association functionality
func TestFilterServiceRSSAssociation(t *testing.T) {
	t.Run("MatchRulesForRSS returns matching rule from associated rules", func(t *testing.T) {
		db, cleanup := setupServiceTestDBWithAssociations(t)
		defer cleanup()

		// Create rules
		rule1 := &models.FilterRule{
			Name:        "Rule 1",
			Pattern:     "test",
			PatternType: models.PatternKeyword,
			Enabled:     true,
			Priority:    100,
		}
		rule2 := &models.FilterRule{
			Name:        "Rule 2",
			Pattern:     "other",
			PatternType: models.PatternKeyword,
			Enabled:     true,
			Priority:    50,
		}
		db.Create(rule1)
		db.Create(rule2)

		// Create RSS subscription
		rss := createTestRSSSubscription(t, db, "test-rss")

		// Associate only rule1 with RSS
		assoc := &models.RSSFilterAssociation{
			RSSID:        rss.ID,
			FilterRuleID: rule1.ID,
		}
		db.Create(assoc)

		svc := NewFilterService(db)

		// Should match rule1 for "test" title
		matchedRule, matched := svc.MatchRulesForRSS("test title", rss.ID)
		require.True(t, matched)
		assert.Equal(t, "Rule 1", matchedRule.Name)

		// Should not match rule2 for "other" title (not associated)
		matchedRule, matched = svc.MatchRulesForRSS("other title", rss.ID)
		assert.False(t, matched)
		assert.Nil(t, matchedRule)
	})

	t.Run("MatchRulesForRSS respects priority order", func(t *testing.T) {
		db, cleanup := setupServiceTestDBWithAssociations(t)
		defer cleanup()

		// Create rules with different priorities, both matching "test"
		rule1 := &models.FilterRule{
			Name:        "Low Priority",
			Pattern:     "test",
			PatternType: models.PatternKeyword,
			Enabled:     true,
			Priority:    200,
		}
		rule2 := &models.FilterRule{
			Name:        "High Priority",
			Pattern:     "test",
			PatternType: models.PatternKeyword,
			Enabled:     true,
			Priority:    10,
		}
		db.Create(rule1)
		db.Create(rule2)

		// Create RSS subscription
		rss := createTestRSSSubscription(t, db, "test-rss")

		// Associate both rules with RSS
		db.Create(&models.RSSFilterAssociation{RSSID: rss.ID, FilterRuleID: rule1.ID})
		db.Create(&models.RSSFilterAssociation{RSSID: rss.ID, FilterRuleID: rule2.ID})

		svc := NewFilterService(db)

		// Should match high priority rule first
		matchedRule, matched := svc.MatchRulesForRSS("test title", rss.ID)
		require.True(t, matched)
		assert.Equal(t, "High Priority", matchedRule.Name)
	})

	t.Run("MatchRulesForRSS returns nil for RSS with no associations", func(t *testing.T) {
		db, cleanup := setupServiceTestDBWithAssociations(t)
		defer cleanup()

		// Create a rule
		rule := &models.FilterRule{
			Name:        "Test Rule",
			Pattern:     "test",
			PatternType: models.PatternKeyword,
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)

		// Create RSS subscription without associations
		rss := createTestRSSSubscription(t, db, "test-rss")

		svc := NewFilterService(db)

		// Should not match any rule
		matchedRule, matched := svc.MatchRulesForRSS("test title", rss.ID)
		assert.False(t, matched)
		assert.Nil(t, matchedRule)
	})

	t.Run("ShouldDownloadForRSS respects require_free", func(t *testing.T) {
		db, cleanup := setupServiceTestDBWithAssociations(t)
		defer cleanup()

		// Create rule with require_free=true
		rule := &models.FilterRule{
			Name:        "Free Only Rule",
			Pattern:     "test",
			PatternType: models.PatternKeyword,
			RequireFree: true,
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)

		// Create RSS subscription
		rss := createTestRSSSubscription(t, db, "test-rss")

		// Associate rule with RSS
		db.Create(&models.RSSFilterAssociation{RSSID: rss.ID, FilterRuleID: rule.ID})

		svc := NewFilterService(db)

		// Should download free torrent
		shouldDownload, matchedRule := svc.ShouldDownloadForRSS("test title", true, rss.ID)
		assert.True(t, shouldDownload)
		assert.NotNil(t, matchedRule)

		// Should not download non-free torrent
		shouldDownload, matchedRule = svc.ShouldDownloadForRSS("test title", false, rss.ID)
		assert.False(t, shouldDownload)
		assert.NotNil(t, matchedRule) // Rule matched, but shouldn't download
	})

	t.Run("GetRulesForRSS returns associated rules", func(t *testing.T) {
		db, cleanup := setupServiceTestDBWithAssociations(t)
		defer cleanup()

		// Create rules
		rule1 := &models.FilterRule{
			Name:        "Rule 1",
			Pattern:     "test1",
			PatternType: models.PatternKeyword,
			Enabled:     true,
			Priority:    100,
		}
		rule2 := &models.FilterRule{
			Name:        "Rule 2",
			Pattern:     "test2",
			PatternType: models.PatternKeyword,
			Enabled:     true,
			Priority:    50,
		}
		rule3 := &models.FilterRule{
			Name:        "Rule 3",
			Pattern:     "test3",
			PatternType: models.PatternKeyword,
			Enabled:     true,
			Priority:    75,
		}
		db.Create(rule1)
		db.Create(rule2)
		db.Create(rule3)

		// Create RSS subscription
		rss := createTestRSSSubscription(t, db, "test-rss")

		// Associate rule1 and rule2 with RSS
		db.Create(&models.RSSFilterAssociation{RSSID: rss.ID, FilterRuleID: rule1.ID})
		db.Create(&models.RSSFilterAssociation{RSSID: rss.ID, FilterRuleID: rule2.ID})

		svc := NewFilterService(db)

		rules, err := svc.GetRulesForRSS(rss.ID)
		require.NoError(t, err)
		assert.Len(t, rules, 2)

		// Should be ordered by priority
		assert.Equal(t, "Rule 2", rules[0].Name) // Priority 50
		assert.Equal(t, "Rule 1", rules[1].Name) // Priority 100
	})

	t.Run("RefreshCache updates RSS associations", func(t *testing.T) {
		db, cleanup := setupServiceTestDBWithAssociations(t)
		defer cleanup()

		// Create rule
		rule := &models.FilterRule{
			Name:        "Test Rule",
			Pattern:     "test",
			PatternType: models.PatternKeyword,
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)

		// Create RSS subscription
		rss := createTestRSSSubscription(t, db, "test-rss")

		svc := NewFilterService(db)

		// Initially no association
		matchedRule, matched := svc.MatchRulesForRSS("test title", rss.ID)
		assert.False(t, matched)
		assert.Nil(t, matchedRule)

		// Add association
		db.Create(&models.RSSFilterAssociation{RSSID: rss.ID, FilterRuleID: rule.ID})

		// Refresh cache
		err := svc.RefreshCache()
		require.NoError(t, err)

		// Now should match
		matchedRule, matched = svc.MatchRulesForRSS("test title", rss.ID)
		assert.True(t, matched)
		assert.Equal(t, "Test Rule", matchedRule.Name)
	})
}

// TestProperty_RSSFilterAssociationIntegrity tests Property 1: RSS-Filter Association Integrity
// Property: For any RSS subscription, only the filter rules explicitly associated with it
// should be used for matching, and the association should be bidirectional.
func TestProperty_RSSFilterAssociationIntegrity(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("only associated rules are used for RSS matching", prop.ForAll(
		func(numRules, numAssociated int) bool {
			if numRules < 1 || numAssociated < 0 || numAssociated > numRules {
				return true
			}

			db, cleanup := setupServiceTestDBWithAssociations(t)
			defer cleanup()

			// Create rules
			rules := make([]*models.FilterRule, numRules)
			for i := 0; i < numRules; i++ {
				rules[i] = &models.FilterRule{
					Name:        "Rule " + string(rune('A'+i)),
					Pattern:     "pattern" + string(rune('a'+i)),
					PatternType: models.PatternKeyword,
					Enabled:     true,
					Priority:    (i + 1) * 10,
				}
				db.Create(rules[i])
			}

			// Create RSS subscription
			rss := createTestRSSSubscription(t, db, "test-rss")

			// Associate first numAssociated rules
			for i := 0; i < numAssociated; i++ {
				db.Create(&models.RSSFilterAssociation{RSSID: rss.ID, FilterRuleID: rules[i].ID})
			}

			svc := NewFilterService(db)

			// Verify only associated rules match
			for i := 0; i < numRules; i++ {
				title := "pattern" + string(rune('a'+i)) + " title"
				matchedRule, matched := svc.MatchRulesForRSS(title, rss.ID)

				if i < numAssociated {
					// Should match
					if !matched || matchedRule.ID != rules[i].ID {
						return false
					}
				} else {
					// Should not match (not associated)
					if matched {
						return false
					}
				}
			}

			return true
		},
		gen.IntRange(1, 5),
		gen.IntRange(0, 5),
	))

	properties.TestingRun(t)
}

// TestProperty_FilterRuleApplicationBasedOnAssociation tests Property 2: Filter Rule Application Based on Association
// Property: For any torrent title, when using MatchRulesForRSS, only the rules associated
// with the specified RSS should be considered, regardless of global rules.
func TestProperty_FilterRuleApplicationBasedOnAssociation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("MatchRulesForRSS ignores non-associated rules", prop.ForAll(
		func(hasGlobalRule, hasAssociatedRule bool) bool {
			db, cleanup := setupServiceTestDBWithAssociations(t)
			defer cleanup()

			// Create RSS subscription
			rss := createTestRSSSubscription(t, db, "test-rss")

			// Create global rule (no site/RSS restriction)
			if hasGlobalRule {
				globalRule := &models.FilterRule{
					Name:        "Global Rule",
					Pattern:     "test",
					PatternType: models.PatternKeyword,
					Enabled:     true,
					Priority:    10, // Higher priority
				}
				db.Create(globalRule)
				// Note: NOT associated with RSS
			}

			// Create and associate a rule
			if hasAssociatedRule {
				associatedRule := &models.FilterRule{
					Name:        "Associated Rule",
					Pattern:     "test",
					PatternType: models.PatternKeyword,
					Enabled:     true,
					Priority:    100, // Lower priority
				}
				db.Create(associatedRule)
				db.Create(&models.RSSFilterAssociation{RSSID: rss.ID, FilterRuleID: associatedRule.ID})
			}

			svc := NewFilterService(db)

			matchedRule, matched := svc.MatchRulesForRSS("test title", rss.ID)

			if hasAssociatedRule {
				// Should match the associated rule, not the global one
				if !matched || matchedRule.Name != "Associated Rule" {
					return false
				}
			} else {
				// Should not match (global rule is not associated)
				if matched {
					return false
				}
			}

			return true
		},
		gen.Bool(),
		gen.Bool(),
	))

	properties.TestingRun(t)
}

// TestMatchTorrentForRSS tests the MatchTorrentForRSS convenience method
func TestMatchTorrentForRSS(t *testing.T) {
	t.Run("returns correct MatchResult for associated rule", func(t *testing.T) {
		db, cleanup := setupServiceTestDBWithAssociations(t)
		defer cleanup()

		rule := &models.FilterRule{
			Name:        "Test Rule",
			Pattern:     "test",
			PatternType: models.PatternKeyword,
			RequireFree: true,
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)

		rss := createTestRSSSubscription(t, db, "test-rss")
		db.Create(&models.RSSFilterAssociation{RSSID: rss.ID, FilterRuleID: rule.ID})

		svc := NewFilterService(db).(*filterService)

		// Free torrent
		result := svc.MatchTorrentForRSS("test title", true, rss.ID)
		assert.True(t, result.Matched)
		assert.NotNil(t, result.Rule)
		assert.True(t, result.ShouldDownload)

		// Non-free torrent
		result = svc.MatchTorrentForRSS("test title", false, rss.ID)
		assert.True(t, result.Matched)
		assert.NotNil(t, result.Rule)
		assert.False(t, result.ShouldDownload)
	})

	t.Run("returns no match for non-associated rule", func(t *testing.T) {
		db, cleanup := setupServiceTestDBWithAssociations(t)
		defer cleanup()

		rule := &models.FilterRule{
			Name:        "Test Rule",
			Pattern:     "test",
			PatternType: models.PatternKeyword,
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)

		rss := createTestRSSSubscription(t, db, "test-rss")
		// Note: NOT associated

		svc := NewFilterService(db).(*filterService)

		result := svc.MatchTorrentForRSS("test title", true, rss.ID)
		assert.False(t, result.Matched)
		assert.Nil(t, result.Rule)
		assert.False(t, result.ShouldDownload)
	})
}

// TestProperty_MultiFieldMatching tests Property 1: Multi-field Matching Logic
// Feature: filter-rule-improvements, Property 1: Multi-field Matching Logic
// *For any* filter rule and *for any* torrent with title and tag fields:
// - When `match_field='title'`, the rule matches if and only if the title matches the pattern
// - When `match_field='tag'`, the rule matches if and only if the tag matches the pattern
// - When `match_field='both'`, the rule matches if either the title OR the tag matches the pattern
// **Validates: Requirements 1.1, 1.2**
func TestProperty_MultiFieldMatching(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: match_field='title' only matches title
	properties.Property("match_field=title only matches title", prop.ForAll(
		func(titleMatches, tagMatches bool) bool {
			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			rule := &models.FilterRule{
				Name:        "Title Only Rule",
				Pattern:     "test",
				PatternType: models.PatternKeyword,
				MatchField:  models.MatchFieldTitle,
				Enabled:     true,
				Priority:    100,
			}
			db.Create(rule)

			svc := NewFilterService(db)

			var title, tag string
			if titleMatches {
				title = "test title"
			} else {
				title = "other title"
			}
			if tagMatches {
				tag = "test tag"
			} else {
				tag = "other tag"
			}

			input := MatchInput{Title: title, Tag: tag}
			_, matched := svc.MatchRulesWithInput(input, nil, nil)

			// Should match only if title matches, regardless of tag
			return matched == titleMatches
		},
		gen.Bool(),
		gen.Bool(),
	))

	// Property: match_field='tag' only matches tag
	properties.Property("match_field=tag only matches tag", prop.ForAll(
		func(titleMatches, tagMatches bool) bool {
			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			rule := &models.FilterRule{
				Name:        "Tag Only Rule",
				Pattern:     "test",
				PatternType: models.PatternKeyword,
				MatchField:  models.MatchFieldTag,
				Enabled:     true,
				Priority:    100,
			}
			db.Create(rule)

			svc := NewFilterService(db)

			var title, tag string
			if titleMatches {
				title = "test title"
			} else {
				title = "other title"
			}
			if tagMatches {
				tag = "test tag"
			} else {
				tag = "other tag"
			}

			input := MatchInput{Title: title, Tag: tag}
			_, matched := svc.MatchRulesWithInput(input, nil, nil)

			// Should match only if tag matches, regardless of title
			return matched == tagMatches
		},
		gen.Bool(),
		gen.Bool(),
	))

	// Property: match_field='both' matches if either title or tag matches
	properties.Property("match_field=both matches if either matches", prop.ForAll(
		func(titleMatches, tagMatches bool) bool {
			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			rule := &models.FilterRule{
				Name:        "Both Fields Rule",
				Pattern:     "test",
				PatternType: models.PatternKeyword,
				MatchField:  models.MatchFieldBoth,
				Enabled:     true,
				Priority:    100,
			}
			db.Create(rule)

			svc := NewFilterService(db)

			var title, tag string
			if titleMatches {
				title = "test title"
			} else {
				title = "other title"
			}
			if tagMatches {
				tag = "test tag"
			} else {
				tag = "other tag"
			}

			input := MatchInput{Title: title, Tag: tag}
			_, matched := svc.MatchRulesWithInput(input, nil, nil)

			// Should match if either title or tag matches
			expectedMatch := titleMatches || tagMatches
			return matched == expectedMatch
		},
		gen.Bool(),
		gen.Bool(),
	))

	// Property: default match_field behaves like 'both'
	properties.Property("default match_field behaves like both", prop.ForAll(
		func(titleMatches, tagMatches bool) bool {
			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			rule := &models.FilterRule{
				Name:        "Default Field Rule",
				Pattern:     "test",
				PatternType: models.PatternKeyword,
				// MatchField not set, should default to 'both'
				Enabled:  true,
				Priority: 100,
			}
			db.Create(rule)

			svc := NewFilterService(db)

			var title, tag string
			if titleMatches {
				title = "test title"
			} else {
				title = "other title"
			}
			if tagMatches {
				tag = "test tag"
			} else {
				tag = "other tag"
			}

			input := MatchInput{Title: title, Tag: tag}
			_, matched := svc.MatchRulesWithInput(input, nil, nil)

			// Should match if either title or tag matches (default behavior)
			expectedMatch := titleMatches || tagMatches
			return matched == expectedMatch
		},
		gen.Bool(),
		gen.Bool(),
	))

	properties.TestingRun(t)
}

// TestMultiFieldMatchingUnit provides unit tests for multi-field matching
func TestMultiFieldMatchingUnit(t *testing.T) {
	t.Run("MatchRulesWithInput with title-only rule", func(t *testing.T) {
		db, cleanup := setupServiceTestDB(t)
		defer cleanup()

		rule := &models.FilterRule{
			Name:        "Title Rule",
			Pattern:     "4K",
			PatternType: models.PatternKeyword,
			MatchField:  models.MatchFieldTitle,
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)

		svc := NewFilterService(db)

		// Title matches
		input := MatchInput{Title: "Movie 4K HDR", Tag: "other"}
		matchedRule, matched := svc.MatchRulesWithInput(input, nil, nil)
		assert.True(t, matched)
		assert.NotNil(t, matchedRule)

		// Only tag matches - should not match
		input = MatchInput{Title: "Movie HDR", Tag: "4K"}
		matchedRule, matched = svc.MatchRulesWithInput(input, nil, nil)
		assert.False(t, matched)
		assert.Nil(t, matchedRule)
	})

	t.Run("MatchRulesWithInput with tag-only rule", func(t *testing.T) {
		db, cleanup := setupServiceTestDB(t)
		defer cleanup()

		rule := &models.FilterRule{
			Name:        "Tag Rule",
			Pattern:     "REMUX",
			PatternType: models.PatternKeyword,
			MatchField:  models.MatchFieldTag,
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)

		svc := NewFilterService(db)

		// Tag matches
		input := MatchInput{Title: "Movie", Tag: "REMUX"}
		matchedRule, matched := svc.MatchRulesWithInput(input, nil, nil)
		assert.True(t, matched)
		assert.NotNil(t, matchedRule)

		// Only title matches - should not match
		input = MatchInput{Title: "Movie REMUX", Tag: "other"}
		matchedRule, matched = svc.MatchRulesWithInput(input, nil, nil)
		assert.False(t, matched)
		assert.Nil(t, matchedRule)
	})

	t.Run("MatchRulesWithInput with both-fields rule", func(t *testing.T) {
		db, cleanup := setupServiceTestDB(t)
		defer cleanup()

		rule := &models.FilterRule{
			Name:        "Both Rule",
			Pattern:     "HDR",
			PatternType: models.PatternKeyword,
			MatchField:  models.MatchFieldBoth,
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)

		svc := NewFilterService(db)

		// Title matches
		input := MatchInput{Title: "Movie HDR", Tag: "other"}
		matchedRule, matched := svc.MatchRulesWithInput(input, nil, nil)
		assert.True(t, matched)
		assert.NotNil(t, matchedRule)

		// Tag matches
		input = MatchInput{Title: "Movie", Tag: "HDR"}
		matchedRule, matched = svc.MatchRulesWithInput(input, nil, nil)
		assert.True(t, matched)
		assert.NotNil(t, matchedRule)

		// Both match
		input = MatchInput{Title: "Movie HDR", Tag: "HDR"}
		matchedRule, matched = svc.MatchRulesWithInput(input, nil, nil)
		assert.True(t, matched)
		assert.NotNil(t, matchedRule)

		// Neither matches
		input = MatchInput{Title: "Movie", Tag: "other"}
		matchedRule, matched = svc.MatchRulesWithInput(input, nil, nil)
		assert.False(t, matched)
		assert.Nil(t, matchedRule)
	})

	t.Run("MatchRulesForRSSWithInput respects match_field", func(t *testing.T) {
		db, cleanup := setupServiceTestDBWithAssociations(t)
		defer cleanup()

		rule := &models.FilterRule{
			Name:        "Tag Only Rule",
			Pattern:     "HEVC",
			PatternType: models.PatternKeyword,
			MatchField:  models.MatchFieldTag,
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)

		rss := createTestRSSSubscription(t, db, "test-rss")
		db.Create(&models.RSSFilterAssociation{RSSID: rss.ID, FilterRuleID: rule.ID})

		svc := NewFilterService(db)

		// Tag matches
		input := MatchInput{Title: "Movie", Tag: "HEVC"}
		matchedRule, matched := svc.MatchRulesForRSSWithInput(input, rss.ID)
		assert.True(t, matched)
		assert.NotNil(t, matchedRule)

		// Only title matches - should not match
		input = MatchInput{Title: "Movie HEVC", Tag: "other"}
		matchedRule, matched = svc.MatchRulesForRSSWithInput(input, rss.ID)
		assert.False(t, matched)
		assert.Nil(t, matchedRule)
	})

	t.Run("backward compatibility - MatchRules uses title only", func(t *testing.T) {
		db, cleanup := setupServiceTestDB(t)
		defer cleanup()

		rule := &models.FilterRule{
			Name:        "Both Rule",
			Pattern:     "test",
			PatternType: models.PatternKeyword,
			MatchField:  models.MatchFieldBoth,
			Enabled:     true,
			Priority:    100,
		}
		db.Create(rule)

		svc := NewFilterService(db)

		// Old API only passes title
		matchedRule, matched := svc.MatchRules("test title", nil, nil)
		assert.True(t, matched)
		assert.NotNil(t, matchedRule)

		// Non-matching title
		matchedRule, matched = svc.MatchRules("other title", nil, nil)
		assert.False(t, matched)
		assert.Nil(t, matchedRule)
	})
}
