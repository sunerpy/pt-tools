package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// TestMatcher_PatternAndType covers the previously-uncovered Pattern()/Type()
// accessors on all three matcher implementations.
func TestMatcher_PatternAndType(t *testing.T) {
	kw, err := NewKeywordMatcher("hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", kw.Pattern())
	assert.Equal(t, PatternKeyword, kw.Type())

	wc, err := NewWildcardMatcher("he*o")
	require.NoError(t, err)
	assert.Equal(t, "he*o", wc.Pattern())
	assert.Equal(t, PatternWildcard, wc.Type())

	rx, err := NewRegexMatcher("h.llo")
	require.NoError(t, err)
	assert.Equal(t, "h.llo", rx.Pattern())
	assert.Equal(t, PatternRegex, rx.Type())
}

// TestNewMatcher_AllBranches covers NewMatcher's dispatch + error branches.
func TestNewMatcher_AllBranches(t *testing.T) {
	t.Run("empty pattern", func(t *testing.T) {
		_, err := NewMatcher(PatternKeyword, "")
		assert.ErrorIs(t, err, ErrEmptyPattern)
	})

	t.Run("pattern too long", func(t *testing.T) {
		long := make([]byte, MaxPatternLength+1)
		for i := range long {
			long[i] = 'a'
		}
		_, err := NewMatcher(PatternKeyword, string(long))
		assert.ErrorIs(t, err, ErrPatternTooLong)
	})

	t.Run("keyword", func(t *testing.T) {
		m, err := NewMatcher(PatternKeyword, "kw")
		require.NoError(t, err)
		assert.Equal(t, PatternKeyword, m.Type())
	})

	t.Run("wildcard", func(t *testing.T) {
		m, err := NewMatcher(PatternWildcard, "w*")
		require.NoError(t, err)
		assert.Equal(t, PatternWildcard, m.Type())
	})

	t.Run("regex", func(t *testing.T) {
		m, err := NewMatcher(PatternRegex, "r.*")
		require.NoError(t, err)
		assert.Equal(t, PatternRegex, m.Type())
	})

	t.Run("unknown type", func(t *testing.T) {
		_, err := NewMatcher(PatternType("bogus"), "x")
		assert.ErrorIs(t, err, ErrUnknownType)
	})
}

// TestMatchesInput_MatchFieldTag covers the MatchFieldTitle / MatchFieldTag /
// unknown-value branches of matchesInput via MatchRulesWithInput.
func TestMatchesInput_MatchFields(t *testing.T) {
	db, cleanup := setupServiceTestDBWithAssociations(t)
	defer cleanup()

	// Tag-only rule: matches on Tag field only.
	tagRule := &models.FilterRule{
		Name: "tag-rule", Pattern: "anime", PatternType: models.PatternKeyword,
		MatchField: models.MatchFieldTag, Enabled: true, Priority: 10,
	}
	require.NoError(t, db.Create(tagRule).Error)

	svc := NewFilterService(db)

	// Matches via Tag but title doesn't contain "anime".
	rule, matched := svc.MatchRulesWithInput(MatchInput{Title: "some movie", Tag: "anime pack"}, nil, nil)
	assert.True(t, matched)
	assert.NotNil(t, rule)

	// Title contains keyword but rule is tag-only -> no match.
	_, matched = svc.MatchRulesWithInput(MatchInput{Title: "anime show", Tag: "other"}, nil, nil)
	assert.False(t, matched)
}

// TestMatchesInput_TitleOnly covers MatchFieldTitle explicit branch.
func TestMatchesInput_TitleOnly(t *testing.T) {
	db, cleanup := setupServiceTestDBWithAssociations(t)
	defer cleanup()

	titleRule := &models.FilterRule{
		Name: "title-rule", Pattern: "movie", PatternType: models.PatternKeyword,
		MatchField: models.MatchFieldTitle, Enabled: true, Priority: 10,
	}
	require.NoError(t, db.Create(titleRule).Error)
	svc := NewFilterService(db)

	_, matched := svc.MatchRulesWithInput(MatchInput{Title: "a movie", Tag: "nope"}, nil, nil)
	assert.True(t, matched)

	// Only tag has the keyword; title-only rule should not match.
	_, matched = svc.MatchRulesWithInput(MatchInput{Title: "nope", Tag: "movie tag"}, nil, nil)
	assert.False(t, matched)
}

// TestRuleApplies_SiteAndRSSRestrictions covers the ruleApplies branches:
// site-restricted and RSS-restricted rules.
func TestRuleApplies_SiteAndRSSRestrictions(t *testing.T) {
	db, cleanup := setupServiceTestDBWithAssociations(t)
	defer cleanup()

	siteID := uint(7)
	rssID := uint(9)

	// Rule restricted to a specific site.
	siteRule := &models.FilterRule{
		Name: "site-rule", Pattern: "x", PatternType: models.PatternKeyword,
		MatchField: models.MatchFieldBoth, Enabled: true, Priority: 10, SiteID: &siteID,
	}
	require.NoError(t, db.Create(siteRule).Error)
	// Rule restricted to a specific RSS.
	rssRule := &models.FilterRule{
		Name: "rss-rule", Pattern: "y", PatternType: models.PatternKeyword,
		MatchField: models.MatchFieldBoth, Enabled: true, Priority: 20, RSSID: &rssID,
	}
	require.NoError(t, db.Create(rssRule).Error)
	svc := NewFilterService(db)

	// site rule: siteID nil -> should NOT apply
	_, matched := svc.MatchRulesWithInput(MatchInput{Title: "x"}, nil, nil)
	assert.False(t, matched)

	// site rule: mismatched siteID -> should NOT apply
	otherSite := uint(99)
	_, matched = svc.MatchRulesWithInput(MatchInput{Title: "x"}, &otherSite, nil)
	assert.False(t, matched)

	// site rule: correct siteID -> applies
	_, matched = svc.MatchRulesWithInput(MatchInput{Title: "x"}, &siteID, nil)
	assert.True(t, matched)

	// rss rule: rssID nil -> should NOT apply
	_, matched = svc.MatchRulesWithInput(MatchInput{Title: "y"}, nil, nil)
	assert.False(t, matched)

	// rss rule: mismatched rssID -> should NOT apply
	otherRSS := uint(88)
	_, matched = svc.MatchRulesWithInput(MatchInput{Title: "y"}, nil, &otherRSS)
	assert.False(t, matched)

	// rss rule: correct rssID -> applies
	_, matched = svc.MatchRulesWithInput(MatchInput{Title: "y"}, nil, &rssID)
	assert.True(t, matched)
}

// TestShouldNotifyForRSS covers the notify-purpose channel including require_free.
func TestShouldNotifyForRSS(t *testing.T) {
	db, cleanup := setupServiceTestDBWithAssociations(t)
	defer cleanup()
	svc := NewFilterService(db)
	rss := createTestRSSSubscription(t, db, "rss-notify")

	// Notify-purpose rule that requires free.
	createRuleForDecide(t, db, svc, rss.ID, &models.FilterRule{
		Name: "notify-free", Pattern: "series", PatternType: models.PatternKeyword,
		MatchField: models.MatchFieldBoth, RequireFree: true, Purpose: "notify",
		Enabled: true, Priority: 5,
	})

	t.Run("matches + free -> notify", func(t *testing.T) {
		ok, rule := svc.ShouldNotifyForRSS("new series ep", true, rss.ID)
		assert.True(t, ok)
		assert.NotNil(t, rule)
	})

	t.Run("matches but not free -> no notify", func(t *testing.T) {
		ok, rule := svc.ShouldNotifyForRSS("new series ep", false, rss.ID)
		assert.False(t, ok)
		assert.NotNil(t, rule)
	})

	t.Run("no match -> no notify", func(t *testing.T) {
		ok, _ := svc.ShouldNotifyForRSS("unrelated title", true, rss.ID)
		assert.False(t, ok)
	})
}

// TestBuildDecisionReason_AllBranches drives buildDecisionReason indirectly
// through Decide across the different FilterMode reason branches.
func TestBuildDecisionReason_FilterOnlyReasons(t *testing.T) {
	db, cleanup := setupServiceTestDBWithAssociations(t)
	defer cleanup()
	svc := NewFilterService(db)
	rss := createTestRSSSubscription(t, db, "rss-reason-fo")

	// require_free rule under filter_only.
	createRuleForDecide(t, db, svc, rss.ID, &models.FilterRule{
		Name: "fo-needfree", Pattern: "movie", PatternType: models.PatternKeyword,
		MatchField: models.MatchFieldBoth, RequireFree: true,
		Enabled: true, Priority: 10,
	})

	// filter_only, matches but not free -> "匹配规则要求免费" branch.
	d := svc.Decide(DecisionContext{
		Input:      MatchInput{Title: "movie", SizeGB: 5},
		IsFree:     false,
		CanFinish:  true,
		GlobalSize: 100,
		FilterMode: models.FilterModeFilterOnly,
	}, rss.ID)
	assert.False(t, d.ShouldDownload)
	assert.Contains(t, d.Reason, "免费")

	// filter_only, no match at all -> "未匹配过滤规则" branch.
	d2 := svc.Decide(DecisionContext{
		Input:      MatchInput{Title: "documentary", SizeGB: 5},
		IsFree:     true,
		CanFinish:  true,
		GlobalSize: 100,
		FilterMode: models.FilterModeFilterOnly,
	}, rss.ID)
	assert.False(t, d2.ShouldDownload)
	assert.Contains(t, d2.Reason, "filter_only")
}

func TestBuildDecisionReason_FilterOnlySizeMismatch(t *testing.T) {
	db, cleanup := setupServiceTestDBWithAssociations(t)
	defer cleanup()
	svc := NewFilterService(db)
	rss := createTestRSSSubscription(t, db, "rss-reason-fo-size")

	// filter_only rule, matches text but size out of bounds (RequireFree=false).
	createRuleForDecide(t, db, svc, rss.ID, &models.FilterRule{
		Name: "fo-size", Pattern: "movie", PatternType: models.PatternKeyword,
		MatchField: models.MatchFieldBoth, MinSizeGB: 100,
		Enabled: true, Priority: 10,
	})

	d := svc.Decide(DecisionContext{
		Input:      MatchInput{Title: "movie", SizeGB: 5},
		IsFree:     false,
		CanFinish:  true,
		GlobalSize: 1000,
		FilterMode: models.FilterModeFilterOnly,
	}, rss.ID)
	assert.False(t, d.ShouldDownload)
	assert.Contains(t, d.Reason, "大小")
}

func TestBuildDecisionReason_FreeOnlyReasons(t *testing.T) {
	db, cleanup := setupServiceTestDBWithAssociations(t)
	defer cleanup()
	svc := NewFilterService(db)
	rss := createTestRSSSubscription(t, db, "rss-reason-free")

	createRuleForDecide(t, db, svc, rss.ID, &models.FilterRule{
		Name: "free-rule", Pattern: "movie", PatternType: models.PatternKeyword,
		MatchField: models.MatchFieldBoth, Enabled: true, Priority: 10,
	})

	// free_only + not free -> "非免费种子" reason.
	d := svc.Decide(DecisionContext{
		Input:      MatchInput{Title: "movie", SizeGB: 5},
		IsFree:     false,
		CanFinish:  true,
		GlobalSize: 100,
		FilterMode: models.FilterModeFreeOnly,
	}, rss.ID)
	assert.False(t, d.ShouldDownload)
	assert.Contains(t, d.Reason, "非免费")

	// free_only + free but cannot finish -> "免费期剩余时间不足".
	d2 := svc.Decide(DecisionContext{
		Input:      MatchInput{Title: "movie", SizeGB: 5},
		IsFree:     true,
		CanFinish:  false,
		GlobalSize: 100,
		FilterMode: models.FilterModeFreeOnly,
	}, rss.ID)
	assert.False(t, d2.ShouldDownload)
	assert.Contains(t, d2.Reason, "免费期")
}
