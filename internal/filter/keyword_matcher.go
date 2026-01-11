package filter

import "strings"

// KeywordMatcher implements case-insensitive keyword matching.
type KeywordMatcher struct {
	pattern      string
	lowerPattern string
}

// NewKeywordMatcher creates a new KeywordMatcher.
func NewKeywordMatcher(pattern string) (*KeywordMatcher, error) {
	if pattern == "" {
		return nil, ErrEmptyPattern
	}
	if len(pattern) > MaxPatternLength {
		return nil, ErrPatternTooLong
	}
	return &KeywordMatcher{
		pattern:      pattern,
		lowerPattern: strings.ToLower(pattern),
	}, nil
}

// Match returns true if the title contains the keyword (case-insensitive).
func (m *KeywordMatcher) Match(title string) bool {
	return strings.Contains(strings.ToLower(title), m.lowerPattern)
}

// Validate checks if the pattern is valid.
func (m *KeywordMatcher) Validate() error {
	if m.pattern == "" {
		return ErrEmptyPattern
	}
	return nil
}

// Pattern returns the original pattern string.
func (m *KeywordMatcher) Pattern() string {
	return m.pattern
}

// Type returns the pattern type.
func (m *KeywordMatcher) Type() PatternType {
	return PatternKeyword
}
