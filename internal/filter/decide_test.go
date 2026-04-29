package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// createRuleForDecide inserts a rule into the test DB and associates it with
// the given RSS subscription so the FilterService cache picks it up.
// It uses Updates with an explicit map to overwrite zero-value booleans
// (e.g., RequireFree=false) that GORM would otherwise skip due to its
// "omit zero-value on struct Update" behavior combined with `default:true` tag.
func createRuleForDecide(t *testing.T, db *gorm.DB, svc FilterService, rssID uint, rule *models.FilterRule) *models.FilterRule {
	t.Helper()
	// Snapshot the caller's intended values BEFORE db.Create, because GORM mutates
	// the struct in-place (e.g., default:true backfills RequireFree from false to true).
	wantRequireFree := rule.RequireFree
	wantMin := rule.MinSizeGB
	wantMax := rule.MaxSizeGB
	wantEnabled := rule.Enabled

	require.NoError(t, db.Create(rule).Error)
	require.NoError(t, db.Exec(
		"UPDATE filter_rules SET require_free = ?, min_size_gb = ?, max_size_gb = ?, enabled = ? WHERE id = ?",
		wantRequireFree, wantMin, wantMax, wantEnabled, rule.ID,
	).Error)
	rule.RequireFree = wantRequireFree
	rule.MinSizeGB = wantMin
	rule.MaxSizeGB = wantMax
	rule.Enabled = wantEnabled

	assoc := models.RSSFilterAssociation{RSSID: rssID, FilterRuleID: rule.ID}
	require.NoError(t, db.Create(&assoc).Error)
	require.NoError(t, svc.RefreshCache())
	return rule
}

// ============================================================================
// Tests for models.FilterRule.MatchesSize
// ============================================================================

func TestFilterRule_MatchesSize(t *testing.T) {
	tests := []struct {
		name      string
		min       int
		max       int
		sizeGB    float64
		wantMatch bool
	}{
		{"no bounds", 0, 0, 100, true},
		{"no bounds zero size", 0, 0, 0, true},
		{"within min only", 10, 0, 20, true},
		{"below min", 10, 0, 5, false},
		{"at min boundary", 10, 0, 10, true},
		{"at min boundary float", 10, 0, 10.0, true},
		{"within max only", 0, 50, 30, true},
		{"above max", 0, 50, 100, false},
		{"at max boundary", 0, 50, 50, true},
		{"within min and max", 10, 50, 30, true},
		{"below min with max set", 10, 50, 5, false},
		{"above max with min set", 10, 50, 100, false},
		{"zero size with min=0", 0, 10, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := &models.FilterRule{MinSizeGB: tt.min, MaxSizeGB: tt.max}
			assert.Equal(t, tt.wantMatch, rule.MatchesSize(tt.sizeGB))
		})
	}
}

// ============================================================================
// Tests for filter.DecideWithoutRules (no RSS-associated rules)
// ============================================================================

func TestDecideWithoutRules(t *testing.T) {
	t.Run("global size limit rejects large torrent", func(t *testing.T) {
		d := DecideWithoutRules(DecisionContext{
			Input:      MatchInput{Title: "big", SizeGB: 100},
			IsFree:     true,
			CanFinish:  true,
			GlobalSize: 50,
			FilterMode: models.FilterModeAutoFree,
		})
		assert.False(t, d.ShouldDownload)
		assert.Equal(t, SourceNone, d.Source)
		assert.Contains(t, d.Reason, "全局大小限制")
	})

	t.Run("at global boundary accepts", func(t *testing.T) {
		d := DecideWithoutRules(DecisionContext{
			Input:      MatchInput{Title: "boundary", SizeGB: 50},
			IsFree:     true,
			CanFinish:  true,
			GlobalSize: 50,
			FilterMode: models.FilterModeAutoFree,
		})
		assert.True(t, d.ShouldDownload)
		assert.Equal(t, SourceFreeDownload, d.Source)
	})

	t.Run("global size zero means unlimited", func(t *testing.T) {
		d := DecideWithoutRules(DecisionContext{
			Input:      MatchInput{Title: "huge", SizeGB: 9999},
			IsFree:     true,
			CanFinish:  true,
			GlobalSize: 0,
			FilterMode: models.FilterModeAutoFree,
		})
		assert.True(t, d.ShouldDownload)
	})

	t.Run("auto_free: free + can finish accepts", func(t *testing.T) {
		d := DecideWithoutRules(DecisionContext{
			Input:      MatchInput{Title: "free", SizeGB: 10},
			IsFree:     true,
			CanFinish:  true,
			GlobalSize: 100,
			FilterMode: models.FilterModeAutoFree,
		})
		assert.True(t, d.ShouldDownload)
		assert.Equal(t, SourceFreeDownload, d.Source)
	})

	t.Run("auto_free: not free rejects", func(t *testing.T) {
		d := DecideWithoutRules(DecisionContext{
			Input:      MatchInput{Title: "paid", SizeGB: 10},
			IsFree:     false,
			CanFinish:  true,
			GlobalSize: 100,
			FilterMode: models.FilterModeAutoFree,
		})
		assert.False(t, d.ShouldDownload)
		assert.Contains(t, d.Reason, "非免费")
	})

	t.Run("auto_free: free but cannot finish rejects", func(t *testing.T) {
		d := DecideWithoutRules(DecisionContext{
			Input:      MatchInput{Title: "free", SizeGB: 10},
			IsFree:     true,
			CanFinish:  false,
			GlobalSize: 100,
			FilterMode: models.FilterModeAutoFree,
		})
		assert.False(t, d.ShouldDownload)
		assert.Contains(t, d.Reason, "免费期")
	})

	t.Run("filter_only without rules: always rejects", func(t *testing.T) {
		d := DecideWithoutRules(DecisionContext{
			Input:      MatchInput{Title: "anything", SizeGB: 10},
			IsFree:     true,
			CanFinish:  true,
			GlobalSize: 100,
			FilterMode: models.FilterModeFilterOnly,
		})
		assert.False(t, d.ShouldDownload)
		assert.Contains(t, d.Reason, "filter_only")
	})

	t.Run("free_only without rules: only free passes", func(t *testing.T) {
		freeD := DecideWithoutRules(DecisionContext{
			Input:      MatchInput{Title: "free", SizeGB: 10},
			IsFree:     true,
			CanFinish:  true,
			GlobalSize: 100,
			FilterMode: models.FilterModeFreeOnly,
		})
		assert.True(t, freeD.ShouldDownload)

		paidD := DecideWithoutRules(DecisionContext{
			Input:      MatchInput{Title: "paid", SizeGB: 10},
			IsFree:     false,
			CanFinish:  true,
			GlobalSize: 100,
			FilterMode: models.FilterModeFreeOnly,
		})
		assert.False(t, paidD.ShouldDownload)
	})

	t.Run("empty filter mode falls back to default", func(t *testing.T) {
		d := DecideWithoutRules(DecisionContext{
			Input:      MatchInput{Title: "free", SizeGB: 10},
			IsFree:     true,
			CanFinish:  true,
			GlobalSize: 0,
			FilterMode: "",
		})
		assert.True(t, d.ShouldDownload)
		assert.Equal(t, SourceFreeDownload, d.Source)
	})

	t.Run("unknown filter mode falls back to default", func(t *testing.T) {
		d := DecideWithoutRules(DecisionContext{
			Input:      MatchInput{Title: "free", SizeGB: 10},
			IsFree:     true,
			CanFinish:  true,
			GlobalSize: 0,
			FilterMode: "bogus_mode",
		})
		assert.True(t, d.ShouldDownload)
		assert.Equal(t, SourceFreeDownload, d.Source)
	})
}

// ============================================================================
// Tests for filter.Decide (with RSS-associated rules) — the core feature
// ============================================================================

func TestDecide_GlobalSizeLimit(t *testing.T) {
	db, cleanup := setupServiceTestDBWithAssociations(t)
	defer cleanup()
	svc := NewFilterService(db)
	rss := createTestRSSSubscription(t, db, "rss-1")

	// Regression guard for the core bug: a matching filter rule MUST NOT
	// bypass the global TorrentSizeGB hard limit.
	createRuleForDecide(t, db, svc, rss.ID, &models.FilterRule{
		Name: "always-match", Pattern: "movie", PatternType: models.PatternKeyword,
		MatchField: models.MatchFieldBoth, RequireFree: false, Enabled: true, Priority: 100,
	})

	tests := []struct {
		name       string
		sizeGB     float64
		globalSize int
		isFree     bool
		wantDL     bool
		wantReason string
	}{
		{"size>global rejects even with rule match", 100, 10, false, false, "全局大小限制"},
		{"size>global rejects even if free+can_finish", 100, 10, true, false, "全局大小限制"},
		{"size<global accepts via filter rule", 5, 10, false, true, ""},
		{"size==global accepts (boundary)", 10, 10, false, true, ""},
		{"global=0 means unlimited", 9999, 0, false, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := svc.Decide(DecisionContext{
				Input:      MatchInput{Title: "movie", SizeGB: tt.sizeGB},
				IsFree:     tt.isFree,
				CanFinish:  true,
				GlobalSize: tt.globalSize,
				FilterMode: models.FilterModeAutoFree,
			}, rss.ID)
			assert.Equal(t, tt.wantDL, d.ShouldDownload, "ShouldDownload mismatch")
			if tt.wantReason != "" {
				assert.Contains(t, d.Reason, tt.wantReason)
			}
		})
	}
}

func TestDecide_RuleSizeBounds(t *testing.T) {
	db, cleanup := setupServiceTestDBWithAssociations(t)
	defer cleanup()
	svc := NewFilterService(db)
	rss := createTestRSSSubscription(t, db, "rss-size")

	createRuleForDecide(t, db, svc, rss.ID, &models.FilterRule{
		Name: "size-range", Pattern: "movie", PatternType: models.PatternKeyword,
		MatchField: models.MatchFieldBoth, RequireFree: false,
		MinSizeGB: 10, MaxSizeGB: 50,
		Enabled: true, Priority: 100,
	})

	tests := []struct {
		name   string
		sizeGB float64
		wantDL bool
	}{
		{"below min size — filter rejects", 5, false},
		{"above max size — filter rejects", 100, false},
		{"within range — filter accepts", 30, true},
		{"at min boundary — accepts", 10, true},
		{"at max boundary — accepts", 50, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := svc.Decide(DecisionContext{
				Input:      MatchInput{Title: "movie", SizeGB: tt.sizeGB},
				IsFree:     false,
				CanFinish:  true,
				GlobalSize: 1000,
				FilterMode: models.FilterModeAutoFree,
			}, rss.ID)
			assert.Equal(t, tt.wantDL, d.ShouldDownload)
		})
	}
}

func TestDecide_RuleSizeCanOnlyNarrow(t *testing.T) {
	db, cleanup := setupServiceTestDBWithAssociations(t)
	defer cleanup()
	svc := NewFilterService(db)
	rss := createTestRSSSubscription(t, db, "rss-narrow")

	// Rule allows up to 500GB, but global caps at 100GB.
	createRuleForDecide(t, db, svc, rss.ID, &models.FilterRule{
		Name: "wide-rule", Pattern: "movie", PatternType: models.PatternKeyword,
		MatchField: models.MatchFieldBoth, MaxSizeGB: 500,
		Enabled: true, Priority: 100,
	})

	d := svc.Decide(DecisionContext{
		Input:      MatchInput{Title: "movie", SizeGB: 200},
		IsFree:     false,
		CanFinish:  true,
		GlobalSize: 100,
		FilterMode: models.FilterModeAutoFree,
	}, rss.ID)
	assert.False(t, d.ShouldDownload, "rule MaxSizeGB must not widen global limit")
	assert.Contains(t, d.Reason, "全局大小限制")
}

func TestDecide_RequireFree(t *testing.T) {
	db, cleanup := setupServiceTestDBWithAssociations(t)
	defer cleanup()
	svc := NewFilterService(db)
	rss := createTestRSSSubscription(t, db, "rss-rf")

	createRuleForDecide(t, db, svc, rss.ID, &models.FilterRule{
		Name: "needs-free", Pattern: "movie", PatternType: models.PatternKeyword,
		MatchField: models.MatchFieldBoth, RequireFree: true,
		Enabled: true, Priority: 100,
	})

	t.Run("RequireFree + not free + not free auto — rejects", func(t *testing.T) {
		d := svc.Decide(DecisionContext{
			Input:      MatchInput{Title: "movie", SizeGB: 10},
			IsFree:     false,
			CanFinish:  true,
			GlobalSize: 100,
			FilterMode: models.FilterModeAutoFree,
		}, rss.ID)
		assert.False(t, d.ShouldDownload)
		// matchedRule should still be recorded for logging
		assert.NotNil(t, d.MatchedRule)
	})

	t.Run("RequireFree + free — accepts via filter channel", func(t *testing.T) {
		d := svc.Decide(DecisionContext{
			Input:      MatchInput{Title: "movie", SizeGB: 10},
			IsFree:     true,
			CanFinish:  true,
			GlobalSize: 100,
			FilterMode: models.FilterModeAutoFree,
		}, rss.ID)
		assert.True(t, d.ShouldDownload)
		assert.Equal(t, SourceFilterRule, d.Source)
	})
}

func TestDecide_FilterOnlyMode(t *testing.T) {
	db, cleanup := setupServiceTestDBWithAssociations(t)
	defer cleanup()
	svc := NewFilterService(db)
	rss := createTestRSSSubscription(t, db, "rss-fo")

	createRuleForDecide(t, db, svc, rss.ID, &models.FilterRule{
		Name: "match-movie", Pattern: "movie", PatternType: models.PatternKeyword,
		MatchField: models.MatchFieldBoth, RequireFree: false,
		Enabled: true, Priority: 100,
	})

	t.Run("free torrent NOT matching rule — rejected (free channel disabled)", func(t *testing.T) {
		d := svc.Decide(DecisionContext{
			Input:      MatchInput{Title: "documentary", SizeGB: 5},
			IsFree:     true,
			CanFinish:  true,
			GlobalSize: 100,
			FilterMode: models.FilterModeFilterOnly,
		}, rss.ID)
		assert.False(t, d.ShouldDownload)
	})

	t.Run("free torrent matching rule — accepted via filter", func(t *testing.T) {
		d := svc.Decide(DecisionContext{
			Input:      MatchInput{Title: "movie", SizeGB: 5},
			IsFree:     true,
			CanFinish:  true,
			GlobalSize: 100,
			FilterMode: models.FilterModeFilterOnly,
		}, rss.ID)
		assert.True(t, d.ShouldDownload)
		assert.Equal(t, SourceFilterRule, d.Source)
	})

	t.Run("paid torrent matching rule (RequireFree=false) — accepted", func(t *testing.T) {
		d := svc.Decide(DecisionContext{
			Input:      MatchInput{Title: "movie", SizeGB: 5},
			IsFree:     false,
			CanFinish:  true,
			GlobalSize: 100,
			FilterMode: models.FilterModeFilterOnly,
		}, rss.ID)
		assert.True(t, d.ShouldDownload)
	})
}

func TestDecide_FreeOnlyMode(t *testing.T) {
	db, cleanup := setupServiceTestDBWithAssociations(t)
	defer cleanup()
	svc := NewFilterService(db)
	rss := createTestRSSSubscription(t, db, "rss-free")

	createRuleForDecide(t, db, svc, rss.ID, &models.FilterRule{
		Name: "match-anything", Pattern: "movie", PatternType: models.PatternKeyword,
		MatchField: models.MatchFieldBoth, RequireFree: false,
		Enabled: true, Priority: 100,
	})

	t.Run("free torrent — accepted via free channel (rule ignored)", func(t *testing.T) {
		d := svc.Decide(DecisionContext{
			Input:      MatchInput{Title: "movie", SizeGB: 5},
			IsFree:     true,
			CanFinish:  true,
			GlobalSize: 100,
			FilterMode: models.FilterModeFreeOnly,
		}, rss.ID)
		assert.True(t, d.ShouldDownload)
		assert.Equal(t, SourceFreeDownload, d.Source)
	})

	t.Run("paid torrent matching rule — rejected (filter channel disabled)", func(t *testing.T) {
		d := svc.Decide(DecisionContext{
			Input:      MatchInput{Title: "movie", SizeGB: 5},
			IsFree:     false,
			CanFinish:  true,
			GlobalSize: 100,
			FilterMode: models.FilterModeFreeOnly,
		}, rss.ID)
		assert.False(t, d.ShouldDownload)
	})

	t.Run("free torrent cannot finish — rejected", func(t *testing.T) {
		d := svc.Decide(DecisionContext{
			Input:      MatchInput{Title: "movie", SizeGB: 5},
			IsFree:     true,
			CanFinish:  false,
			GlobalSize: 100,
			FilterMode: models.FilterModeFreeOnly,
		}, rss.ID)
		assert.False(t, d.ShouldDownload)
	})
}

func TestDecide_AutoFreeMode_CombinedChannels(t *testing.T) {
	db, cleanup := setupServiceTestDBWithAssociations(t)
	defer cleanup()
	svc := NewFilterService(db)
	rss := createTestRSSSubscription(t, db, "rss-combo")

	createRuleForDecide(t, db, svc, rss.ID, &models.FilterRule{
		Name: "specific-match", Pattern: "exact-title", PatternType: models.PatternKeyword,
		MatchField: models.MatchFieldBoth, RequireFree: false,
		Enabled: true, Priority: 100,
	})

	tests := []struct {
		name      string
		title     string
		isFree    bool
		canFinish bool
		wantDL    bool
		wantSrc   string
	}{
		{"matching + paid → filter channel", "exact-title", false, true, true, SourceFilterRule},
		{"matching + free → filter channel (filter wins due to order)", "exact-title", true, true, true, SourceFilterRule},
		{"non-matching + free + can_finish → free channel", "other", true, true, true, SourceFreeDownload},
		{"non-matching + paid → rejected", "other", false, true, false, SourceNone},
		{"non-matching + free but cannot finish → rejected", "other", true, false, false, SourceNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := svc.Decide(DecisionContext{
				Input:      MatchInput{Title: tt.title, SizeGB: 5},
				IsFree:     tt.isFree,
				CanFinish:  tt.canFinish,
				GlobalSize: 100,
				FilterMode: models.FilterModeAutoFree,
			}, rss.ID)
			assert.Equal(t, tt.wantDL, d.ShouldDownload)
			assert.Equal(t, tt.wantSrc, d.Source)
		})
	}
}

func TestDecide_RuleSizeMismatch_FallsBackToFreeChannel(t *testing.T) {
	db, cleanup := setupServiceTestDBWithAssociations(t)
	defer cleanup()
	svc := NewFilterService(db)
	rss := createTestRSSSubscription(t, db, "rss-fallback")

	createRuleForDecide(t, db, svc, rss.ID, &models.FilterRule{
		Name: "min-100gb", Pattern: "movie", PatternType: models.PatternKeyword,
		MatchField: models.MatchFieldBoth, MinSizeGB: 100,
		Enabled: true, Priority: 100,
	})

	// Torrent text matches rule, but size 5GB < MinSizeGB 100. auto_free mode
	// should then check the free channel, which accepts.
	d := svc.Decide(DecisionContext{
		Input:      MatchInput{Title: "movie", SizeGB: 5},
		IsFree:     true,
		CanFinish:  true,
		GlobalSize: 1000,
		FilterMode: models.FilterModeAutoFree,
	}, rss.ID)
	assert.True(t, d.ShouldDownload)
	assert.Equal(t, SourceFreeDownload, d.Source)
	assert.NotNil(t, d.MatchedRule, "matchedRule should be retained for logging")
}

func TestDecide_RuleRequireFreeMismatch_FallsBackToFreeChannel(t *testing.T) {
	db, cleanup := setupServiceTestDBWithAssociations(t)
	defer cleanup()
	svc := NewFilterService(db)
	rss := createTestRSSSubscription(t, db, "rss-rf-fallback")

	createRuleForDecide(t, db, svc, rss.ID, &models.FilterRule{
		Name: "needs-free", Pattern: "movie", PatternType: models.PatternKeyword,
		MatchField: models.MatchFieldBoth, RequireFree: true,
		Enabled: true, Priority: 100,
	})

	// Matches pattern but NOT free. free channel also fails (not free).
	// Expect rejection with matchedRule retained.
	d := svc.Decide(DecisionContext{
		Input:      MatchInput{Title: "movie", SizeGB: 5},
		IsFree:     false,
		CanFinish:  true,
		GlobalSize: 100,
		FilterMode: models.FilterModeAutoFree,
	}, rss.ID)
	assert.False(t, d.ShouldDownload)
	assert.NotNil(t, d.MatchedRule)
}
