package filter

import (
	"sync"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// MatchInput represents the input fields for matching against filter rules.
type MatchInput struct {
	Title string
	Tag   string
	// SizeGB is the torrent size in GB. Zero means unknown (skip size checks).
	SizeGB float64
}

// DecisionContext bundles the full set of inputs required to make a download decision.
type DecisionContext struct {
	Input      MatchInput
	IsFree     bool
	CanFinish  bool
	GlobalSize int
	FilterMode models.FilterMode
}

// Decision captures the outcome of a full download decision, including which
// channel (if any) approved the download and the reason if rejected.
type Decision struct {
	ShouldDownload bool
	MatchedRule    *models.FilterRule
	Source         string
	Reason         string
}

// FilterService provides filter rule matching functionality.
type FilterService interface {
	// MatchRules checks if a torrent title matches any enabled filter rule.
	// Returns the matched rule and true if a match is found, nil and false otherwise.
	MatchRules(title string, siteID, rssID *uint) (*models.FilterRule, bool)

	// MatchRulesWithInput checks if input matches any enabled filter rule.
	// Supports matching against title, tag, or both based on rule configuration.
	MatchRulesWithInput(input MatchInput, siteID, rssID *uint) (*models.FilterRule, bool)

	// MatchRulesForRSS checks if a torrent title matches any filter rule associated with the RSS.
	// This uses the many-to-many association table for rule lookup.
	// Returns the matched rule and true if a match is found, nil and false otherwise.
	MatchRulesForRSS(title string, rssID uint) (*models.FilterRule, bool)

	// MatchRulesForRSSWithInput checks if input matches any filter rule associated with the RSS.
	// Supports matching against title, tag, or both based on rule configuration.
	MatchRulesForRSSWithInput(input MatchInput, rssID uint) (*models.FilterRule, bool)

	// ShouldDownload determines if a torrent should be downloaded based on filter rules.
	// Returns true if the torrent should be downloaded, along with the matched rule (if any).
	ShouldDownload(title string, isFree bool, siteID, rssID *uint) (bool, *models.FilterRule)

	// ShouldDownloadWithInput determines if a torrent should be downloaded based on filter rules.
	// Supports matching against title, tag, or both based on rule configuration.
	ShouldDownloadWithInput(input MatchInput, isFree bool, siteID, rssID *uint) (bool, *models.FilterRule)

	// ShouldDownloadForRSS determines if a torrent should be downloaded based on RSS-associated filter rules.
	// This uses the many-to-many association table for rule lookup.
	// Returns true if the torrent should be downloaded, along with the matched rule (if any).
	ShouldDownloadForRSS(title string, isFree bool, rssID uint) (bool, *models.FilterRule)

	// ShouldDownloadForRSSWithInput determines if a torrent should be downloaded based on RSS-associated filter rules.
	// Supports matching against title, tag, or both based on rule configuration.
	ShouldDownloadForRSSWithInput(input MatchInput, isFree bool, rssID uint) (bool, *models.FilterRule)

	// Decide evaluates the full download decision tree for an RSS item, honoring
	// the configured FilterMode, global hard size limit, and per-rule size bounds.
	// It is the canonical entry point for the v0.25+ download decision logic.
	Decide(ctx DecisionContext, rssID uint) Decision

	// GetEnabledRules returns all enabled filter rules ordered by priority.
	GetEnabledRules() ([]models.FilterRule, error)

	// GetRulesForRSS returns all enabled filter rules associated with an RSS subscription.
	// Uses the many-to-many association table.
	GetRulesForRSS(rssID uint) ([]models.FilterRule, error)

	// RefreshCache refreshes the cached matchers from the database.
	RefreshCache() error
}

// filterService implements FilterService.
type filterService struct {
	db       *gorm.DB
	assocDB  *models.RSSFilterAssociationDB
	matchers map[uint]PatternMatcher // Cached compiled matchers by rule ID
	rules    []models.FilterRule     // Cached rules ordered by priority
	rssRules map[uint][]uint         // Cached RSS ID -> associated rule IDs
	mu       sync.RWMutex
}

// NewFilterService creates a new FilterService.
func NewFilterService(db *gorm.DB) FilterService {
	svc := &filterService{
		db:       db,
		assocDB:  models.NewRSSFilterAssociationDB(db),
		matchers: make(map[uint]PatternMatcher),
		rssRules: make(map[uint][]uint),
	}
	// Initialize cache
	_ = svc.RefreshCache()
	return svc
}

// MatchRules checks if a torrent title matches any enabled filter rule.
func (s *filterService) MatchRules(title string, siteID, rssID *uint) (*models.FilterRule, bool) {
	// For backward compatibility, use title-only matching
	return s.MatchRulesWithInput(MatchInput{Title: title}, siteID, rssID)
}

// MatchRulesWithInput checks if input matches any enabled filter rule.
func (s *filterService) MatchRulesWithInput(input MatchInput, siteID, rssID *uint) (*models.FilterRule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := range s.rules {
		rule := &s.rules[i]

		// Check if rule applies to this site/RSS
		if !s.ruleApplies(rule, siteID, rssID) {
			continue
		}

		// Get cached matcher
		matcher, ok := s.matchers[rule.ID]
		if !ok {
			continue
		}

		// Check if input matches based on match_field configuration
		if s.matchesInput(matcher, rule, input) {
			return rule, true
		}
	}

	return nil, false
}

// matchesInput checks if the input matches the rule based on match_field configuration.
func (s *filterService) matchesInput(matcher PatternMatcher, rule *models.FilterRule, input MatchInput) bool {
	matchField := rule.MatchField
	if matchField == "" {
		matchField = models.MatchFieldBoth // Default to both
	}

	switch matchField {
	case models.MatchFieldTitle:
		return matcher.Match(input.Title)
	case models.MatchFieldTag:
		return matcher.Match(input.Tag)
	case models.MatchFieldBoth:
		return matcher.Match(input.Title) || matcher.Match(input.Tag)
	default:
		// Default to both for unknown values
		return matcher.Match(input.Title) || matcher.Match(input.Tag)
	}
}

// MatchRulesForRSS checks if a torrent title matches any filter rule associated with the RSS.
func (s *filterService) MatchRulesForRSS(title string, rssID uint) (*models.FilterRule, bool) {
	// For backward compatibility, use title-only matching
	return s.MatchRulesForRSSWithInput(MatchInput{Title: title}, rssID)
}

// MatchRulesForRSSWithInput checks if input matches any filter rule associated with the RSS.
func (s *filterService) MatchRulesForRSSWithInput(input MatchInput, rssID uint) (*models.FilterRule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get associated rule IDs for this RSS
	ruleIDs, ok := s.rssRules[rssID]
	if !ok || len(ruleIDs) == 0 {
		return nil, false
	}

	// Create a set for quick lookup
	ruleIDSet := make(map[uint]bool)
	for _, id := range ruleIDs {
		ruleIDSet[id] = true
	}
	// Check rules in priority order
	for i := range s.rules {
		rule := &s.rules[i]

		// Only check rules associated with this RSS
		if !ruleIDSet[rule.ID] {
			continue
		}

		// Get cached matcher
		matcher, ok := s.matchers[rule.ID]
		if !ok {
			continue
		}

		// Check if input matches based on match_field configuration
		if s.matchesInput(matcher, rule, input) {
			return rule, true
		}
	}

	return nil, false
}

// ShouldDownload determines if a torrent should be downloaded based on filter rules.
func (s *filterService) ShouldDownload(title string, isFree bool, siteID, rssID *uint) (bool, *models.FilterRule) {
	return s.ShouldDownloadWithInput(MatchInput{Title: title}, isFree, siteID, rssID)
}

// ShouldDownloadWithInput determines if a torrent should be downloaded based on filter rules.
func (s *filterService) ShouldDownloadWithInput(input MatchInput, isFree bool, siteID, rssID *uint) (bool, *models.FilterRule) {
	rule, matched := s.MatchRulesWithInput(input, siteID, rssID)
	if !matched {
		return false, nil
	}

	// If rule requires free, check free status
	if rule.RequireFree && !isFree {
		return false, rule
	}

	return true, rule
}

// ShouldDownloadForRSS determines if a torrent should be downloaded based on RSS-associated filter rules.
func (s *filterService) ShouldDownloadForRSS(title string, isFree bool, rssID uint) (bool, *models.FilterRule) {
	return s.ShouldDownloadForRSSWithInput(MatchInput{Title: title}, isFree, rssID)
}

// ShouldDownloadForRSSWithInput determines if a torrent should be downloaded based on RSS-associated filter rules.
func (s *filterService) ShouldDownloadForRSSWithInput(input MatchInput, isFree bool, rssID uint) (bool, *models.FilterRule) {
	rule, matched := s.MatchRulesForRSSWithInput(input, rssID)
	if !matched {
		return false, nil
	}

	// If rule requires free, check free status
	if rule.RequireFree && !isFree {
		return false, rule
	}

	return true, rule
}

// GetEnabledRules returns all enabled filter rules ordered by priority.
func (s *filterService) GetEnabledRules() ([]models.FilterRule, error) {
	var rules []models.FilterRule
	err := s.db.Where("enabled = ?", true).
		Order("priority ASC, id ASC").
		Find(&rules).Error
	return rules, err
}

// GetRulesForRSS returns all enabled filter rules associated with an RSS subscription.
func (s *filterService) GetRulesForRSS(rssID uint) ([]models.FilterRule, error) {
	return s.assocDB.GetFilterRulesForRSS(rssID)
}

// RefreshCache refreshes the cached matchers from the database.
func (s *filterService) RefreshCache() error {
	rules, err := s.GetEnabledRules()
	if err != nil {
		return err
	}

	matchers := make(map[uint]PatternMatcher)
	for _, rule := range rules {
		patternType := PatternType(rule.PatternType)
		matcher, err := NewMatcher(patternType, rule.Pattern)
		if err != nil {
			// Skip invalid patterns
			continue
		}
		matchers[rule.ID] = matcher
	}

	// Refresh RSS-rule associations
	rssRules := make(map[uint][]uint)
	var associations []models.RSSFilterAssociation
	if err := s.db.Find(&associations).Error; err == nil {
		for _, assoc := range associations {
			rssRules[assoc.RSSID] = append(rssRules[assoc.RSSID], assoc.FilterRuleID)
		}
	}

	s.mu.Lock()
	s.rules = rules
	s.matchers = matchers
	s.rssRules = rssRules
	s.mu.Unlock()

	return nil
}

// ruleApplies checks if a rule applies to the given site and RSS.
func (s *filterService) ruleApplies(rule *models.FilterRule, siteID, rssID *uint) bool {
	// If rule has no site restriction, it applies to all sites
	if rule.SiteID != nil {
		if siteID == nil || *rule.SiteID != *siteID {
			return false
		}
	}

	// If rule has no RSS restriction, it applies to all RSS
	if rule.RSSID != nil {
		if rssID == nil || *rule.RSSID != *rssID {
			return false
		}
	}

	return true
}

// MatchResult represents the result of a filter match.
type MatchResult struct {
	Matched        bool
	Rule           *models.FilterRule
	ShouldDownload bool
}

// MatchTorrent is a convenience method that returns a complete match result.
func (s *filterService) MatchTorrent(title string, isFree bool, siteID, rssID *uint) MatchResult {
	return s.MatchTorrentWithInput(MatchInput{Title: title}, isFree, siteID, rssID)
}

// MatchTorrentWithInput is a convenience method that returns a complete match result with multi-field support.
func (s *filterService) MatchTorrentWithInput(input MatchInput, isFree bool, siteID, rssID *uint) MatchResult {
	rule, matched := s.MatchRulesWithInput(input, siteID, rssID)
	if !matched {
		return MatchResult{Matched: false}
	}

	shouldDownload := !rule.RequireFree || isFree
	return MatchResult{
		Matched:        true,
		Rule:           rule,
		ShouldDownload: shouldDownload,
	}
}

// MatchTorrentForRSS is a convenience method that returns a complete match result using RSS associations.
func (s *filterService) MatchTorrentForRSS(title string, isFree bool, rssID uint) MatchResult {
	return s.MatchTorrentForRSSWithInput(MatchInput{Title: title}, isFree, rssID)
}

// MatchTorrentForRSSWithInput is a convenience method that returns a complete match result using RSS associations with multi-field support.
func (s *filterService) MatchTorrentForRSSWithInput(input MatchInput, isFree bool, rssID uint) MatchResult {
	rule, matched := s.MatchRulesForRSSWithInput(input, rssID)
	if !matched {
		return MatchResult{Matched: false}
	}

	shouldDownload := !rule.RequireFree || isFree
	return MatchResult{
		Matched:        true,
		Rule:           rule,
		ShouldDownload: shouldDownload,
	}
}

// Download source tags persisted on TorrentInfo.DownloadSource.
const (
	SourceFreeDownload = "free_download"
	SourceFilterRule   = "filter_rule"
	SourceNone         = ""
)

// hasAssociatedRules reports whether the given RSS has any filter rule associated.
// It must be called without holding s.mu; it takes the read lock internally.
func (s *filterService) hasAssociatedRules(rssID uint) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids, ok := s.rssRules[rssID]
	return ok && len(ids) > 0
}

// Decide implements the FilterMode-aware decision tree. Order of checks:
//  1. Global hard size limit — if exceeded, reject immediately regardless of mode.
//  2. Filter-rule channel (enabled unless mode == free_only):
//     matches pattern + satisfies RequireFree + per-rule size bounds.
//  3. Free channel:
//     - Disabled when mode == filter_only.
//     - Disabled when mode == auto_free AND the RSS has associated rules (hasRules).
//     This enforces the "associating rules = precise opt-in" user expectation:
//     once any rule is attached, non-matching free torrents are NOT auto-downloaded.
//     - Enabled when mode == auto_free AND no rules are associated (preserves legacy behavior).
//     - Always enabled when mode == free_only.
//
// Per-rule bounds can only narrow the global limit (checked after step 1 is passed).
// Filter mode falls back to FilterModeAutoFree when unrecognized.
func (s *filterService) Decide(ctx DecisionContext, rssID uint) Decision {
	mode := models.NormalizeFilterMode(ctx.FilterMode)

	if ctx.GlobalSize > 0 && ctx.Input.SizeGB > float64(ctx.GlobalSize) {
		return Decision{
			ShouldDownload: false,
			Source:         SourceNone,
			Reason:         "超出全局大小限制",
		}
	}

	var matchedRule *models.FilterRule
	var hasRules bool
	if mode != models.FilterModeFreeOnly {
		rule, matched := s.MatchRulesForRSSWithInput(ctx.Input, rssID)
		hasRules = s.hasAssociatedRules(rssID)
		if matched {
			matchedRule = rule
			if rule.RequireFree && !ctx.IsFree {
				// Don't approve via filter channel, but keep the matchedRule for
				// logging; the free channel may still approve below.
			} else if !rule.MatchesSize(ctx.Input.SizeGB) {
				// Rule matched text but not size — same handling as above.
			} else {
				return Decision{
					ShouldDownload: true,
					MatchedRule:    rule,
					Source:         SourceFilterRule,
				}
			}
		}
	}

	// Free channel gating (Plan A semantics):
	//   filter_only         → never allow free channel
	//   auto_free + hasRules → never allow free channel (implicit filter-only)
	//   auto_free + no rules → allow free channel (legacy behavior)
	//   free_only           → always allow free channel (rules skipped entirely)
	freeAllowed := mode != models.FilterModeFilterOnly && (mode != models.FilterModeAutoFree || !hasRules)
	if freeAllowed {
		if ctx.IsFree && ctx.CanFinish {
			return Decision{
				ShouldDownload: true,
				MatchedRule:    matchedRule,
				Source:         SourceFreeDownload,
			}
		}
	}

	return Decision{
		ShouldDownload: false,
		MatchedRule:    matchedRule,
		Source:         SourceNone,
		Reason:         buildDecisionReason(mode, matchedRule, ctx.IsFree, ctx.CanFinish, hasRules),
	}
}

// DecideWithoutRules runs the same decision tree as Decide but skips the
// filter-rule channel entirely. Callers use it when the RSS has no associated
// rules; it preserves the global hard size limit and free-channel semantics
// without requiring a FilterService.
func DecideWithoutRules(ctx DecisionContext) Decision {
	mode := models.NormalizeFilterMode(ctx.FilterMode)

	if ctx.GlobalSize > 0 && ctx.Input.SizeGB > float64(ctx.GlobalSize) {
		return Decision{
			ShouldDownload: false,
			Source:         SourceNone,
			Reason:         "超出全局大小限制",
		}
	}

	if mode == models.FilterModeFilterOnly {
		return Decision{
			ShouldDownload: false,
			Source:         SourceNone,
			Reason:         "filter_only 模式下未匹配过滤规则（RSS 无关联规则）",
		}
	}

	if ctx.IsFree && ctx.CanFinish {
		return Decision{
			ShouldDownload: true,
			Source:         SourceFreeDownload,
		}
	}

	if !ctx.IsFree {
		return Decision{
			ShouldDownload: false,
			Source:         SourceNone,
			Reason:         "非免费且无关联过滤规则",
		}
	}
	return Decision{
		ShouldDownload: false,
		Source:         SourceNone,
		Reason:         "免费期剩余时间不足",
	}
}

func buildDecisionReason(mode models.FilterMode, rule *models.FilterRule, isFree, canFinish, hasRules bool) string {
	switch mode {
	case models.FilterModeFilterOnly:
		if rule == nil {
			return "未匹配过滤规则（filter_only 模式下免费通道已关闭）"
		}
		if rule.RequireFree && !isFree {
			return "匹配规则要求免费，但种子非免费"
		}
		return "匹配规则但大小不符合规则约束"
	case models.FilterModeFreeOnly:
		if !isFree {
			return "非免费种子（free_only 模式下过滤规则通道已关闭）"
		}
		return "免费期剩余时间不足"
	default:
		// auto_free branch
		if rule != nil && rule.RequireFree && !isFree {
			if hasRules {
				return "匹配规则要求免费但种子非免费；RSS 关联了过滤规则，非匹配的免费种子不再自动下载"
			}
			return "匹配规则要求免费，种子非免费；且非免费或无法完成"
		}
		if rule != nil && !rule.MatchesSize(0) && rule.MaxSizeGB > 0 {
			if hasRules {
				return "匹配规则但大小不符合；RSS 关联了过滤规则，非匹配的免费种子不再自动下载"
			}
			return "匹配规则但大小不符合；且非免费或无法完成"
		}
		if hasRules {
			if isFree && !canFinish {
				return "未匹配过滤规则且免费期剩余时间不足（RSS 关联了规则，未匹配种子不自动下载）"
			}
			return "未匹配过滤规则（RSS 关联了规则，非匹配的免费种子不再自动下载）"
		}
		if !isFree {
			return "非免费且未匹配过滤规则"
		}
		return "免费期剩余时间不足"
	}
}
