package filter

import "regexp"

// WildcardMatcher implements wildcard pattern matching using * and ? characters.
// * matches any sequence of characters (including empty).
// ? matches exactly one character.
type WildcardMatcher struct {
	pattern string
	regex   *regexp.Regexp
}

// NewWildcardMatcher creates a new WildcardMatcher.
// It converts the wildcard pattern to a regular expression.
func NewWildcardMatcher(pattern string) (*WildcardMatcher, error) {
	if pattern == "" {
		return nil, ErrEmptyPattern
	}
	if len(pattern) > MaxPatternLength {
		return nil, ErrPatternTooLong
	}

	// Convert wildcard pattern to regex:
	// 1. Escape all regex special characters
	// 2. Replace escaped \* with .* (match any sequence)
	// 3. Replace escaped \? with . (match single character)
	regexPattern := regexp.QuoteMeta(pattern)
	regexPattern = replaceWildcards(regexPattern)

	// Add case-insensitive flag and anchor the pattern
	regex, err := regexp.Compile("(?i)" + regexPattern)
	if err != nil {
		return nil, ErrInvalidPattern
	}

	return &WildcardMatcher{
		pattern: pattern,
		regex:   regex,
	}, nil
}

// replaceWildcards replaces escaped wildcard characters with regex equivalents.
func replaceWildcards(pattern string) string {
	result := make([]byte, 0, len(pattern)*2)
	i := 0
	for i < len(pattern) {
		if i+1 < len(pattern) && pattern[i] == '\\' {
			switch pattern[i+1] {
			case '*':
				result = append(result, '.', '*')
				i += 2
				continue
			case '?':
				result = append(result, '.')
				i += 2
				continue
			}
		}
		result = append(result, pattern[i])
		i++
	}
	return string(result)
}

// Match returns true if the title matches the wildcard pattern.
func (m *WildcardMatcher) Match(title string) bool {
	return m.regex.MatchString(title)
}

// Validate checks if the pattern is valid.
func (m *WildcardMatcher) Validate() error {
	if m.pattern == "" {
		return ErrEmptyPattern
	}
	return nil
}

// Pattern returns the original pattern string.
func (m *WildcardMatcher) Pattern() string {
	return m.pattern
}

// Type returns the pattern type.
func (m *WildcardMatcher) Type() PatternType {
	return PatternWildcard
}
