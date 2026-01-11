// Package filter provides pattern matching and filtering functionality for RSS items.
package filter

import (
	"errors"
)

// PatternType represents the type of pattern matching to use.
type PatternType string

const (
	// PatternKeyword matches if the title contains the keyword (case-insensitive).
	PatternKeyword PatternType = "keyword"
	// PatternWildcard uses * and ? wildcards for matching.
	PatternWildcard PatternType = "wildcard"
	// PatternRegex uses regular expressions for matching.
	PatternRegex PatternType = "regex"
)

// Pattern matching errors.
var (
	ErrInvalidPattern = errors.New("invalid pattern")
	ErrInvalidRegex   = errors.New("invalid regular expression")
	ErrEmptyPattern   = errors.New("pattern cannot be empty")
	ErrPatternTooLong = errors.New("pattern exceeds maximum length")
	ErrUnknownType    = errors.New("unknown pattern type")
)

// MaxPatternLength is the maximum allowed length for a pattern.
const MaxPatternLength = 512

// PatternMatcher defines the interface for pattern matching.
type PatternMatcher interface {
	// Match returns true if the title matches the pattern.
	Match(title string) bool
	// Validate checks if the pattern is valid.
	Validate() error
	// Pattern returns the original pattern string.
	Pattern() string
	// Type returns the pattern type.
	Type() PatternType
}

// NewMatcher creates a new PatternMatcher based on the pattern type.
func NewMatcher(patternType PatternType, pattern string) (PatternMatcher, error) {
	if pattern == "" {
		return nil, ErrEmptyPattern
	}
	if len(pattern) > MaxPatternLength {
		return nil, ErrPatternTooLong
	}

	switch patternType {
	case PatternKeyword:
		return NewKeywordMatcher(pattern)
	case PatternWildcard:
		return NewWildcardMatcher(pattern)
	case PatternRegex:
		return NewRegexMatcher(pattern)
	default:
		return nil, ErrUnknownType
	}
}
