package filter

import (
	"fmt"
	"regexp"
)

// RegexMatcher implements regular expression pattern matching.
type RegexMatcher struct {
	pattern string
	regex   *regexp.Regexp
}

// NewRegexMatcher creates a new RegexMatcher.
func NewRegexMatcher(pattern string) (*RegexMatcher, error) {
	if pattern == "" {
		return nil, ErrEmptyPattern
	}
	if len(pattern) > MaxPatternLength {
		return nil, ErrPatternTooLong
	}

	// Compile the regex pattern with case-insensitive flag
	regex, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRegex, err)
	}

	return &RegexMatcher{
		pattern: pattern,
		regex:   regex,
	}, nil
}

// Match returns true if the title matches the regex pattern.
func (m *RegexMatcher) Match(title string) bool {
	return m.regex.MatchString(title)
}

// Validate checks if the pattern is valid.
func (m *RegexMatcher) Validate() error {
	if m.pattern == "" {
		return ErrEmptyPattern
	}
	_, err := regexp.Compile("(?i)" + m.pattern)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRegex, err)
	}
	return nil
}

// Pattern returns the original pattern string.
func (m *RegexMatcher) Pattern() string {
	return m.pattern
}

// Type returns the pattern type.
func (m *RegexMatcher) Type() PatternType {
	return PatternRegex
}

// ValidateRegexPattern validates a regex pattern without creating a matcher.
func ValidateRegexPattern(pattern string) error {
	if pattern == "" {
		return ErrEmptyPattern
	}
	if len(pattern) > MaxPatternLength {
		return ErrPatternTooLong
	}
	_, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRegex, err)
	}
	return nil
}
