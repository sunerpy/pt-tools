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
