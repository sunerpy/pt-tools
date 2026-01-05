package filter

import (
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKeywordMatcherCaseInsensitive tests Property 2: Keyword Matcher Case-Insensitive Containment
// Feature: rss-filter-and-downloader-autostart, Property 2: Keyword Matcher Case-Insensitive Containment
// *For any* keyword pattern and *for any* torrent title, the KeywordMatcher should return true
// if and only if the lowercase title contains the lowercase keyword.
// **Validates: Requirements 2.3**
func TestKeywordMatcherCaseInsensitive(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Match returns true iff lowercase title contains lowercase keyword
	properties.Property("keyword match is case-insensitive containment", prop.ForAll(
		func(keyword, title string) bool {
			if keyword == "" {
				return true // Skip empty keywords as they are invalid
			}
			matcher, err := NewKeywordMatcher(keyword)
			if err != nil {
				return true // Skip invalid patterns
			}

			result := matcher.Match(title)
			expected := strings.Contains(strings.ToLower(title), strings.ToLower(keyword))
			return result == expected
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) <= 50 }),
		gen.AlphaString(),
	))

	// Property: Match is case-insensitive (same result for different cases)
	properties.Property("keyword match ignores case", prop.ForAll(
		func(keyword, title string) bool {
			if keyword == "" {
				return true
			}
			matcher, err := NewKeywordMatcher(keyword)
			if err != nil {
				return true
			}

			// Test with various case combinations
			result1 := matcher.Match(title)
			result2 := matcher.Match(strings.ToUpper(title))
			result3 := matcher.Match(strings.ToLower(title))

			// All should return the same result
			return result1 == result2 && result2 == result3
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) <= 50 }),
		gen.AlphaString(),
	))

	// Property: If keyword is substring of title, match returns true
	properties.Property("keyword substring always matches", prop.ForAll(
		func(prefix, keyword, suffix string) bool {
			matcher, err := NewKeywordMatcher(keyword)
			if err != nil {
				return true
			}

			title := prefix + keyword + suffix
			return matcher.Match(title)
		},
		gen.AnyString().Map(func(s string) string {
			if len(s) > 20 {
				return s[:20]
			}
			return s
		}),
		gen.AnyString().Map(func(s string) string {
			if len(s) == 0 {
				return "a"
			}
			if len(s) > 20 {
				return s[:20]
			}
			return s
		}),
		gen.AnyString().Map(func(s string) string {
			if len(s) > 20 {
				return s[:20]
			}
			return s
		}),
	))

	properties.TestingRun(t)
}

// TestKeywordMatcherUnit provides unit tests for KeywordMatcher
func TestKeywordMatcherUnit(t *testing.T) {
	tests := []struct {
		name     string
		keyword  string
		title    string
		expected bool
	}{
		{"exact match", "test", "test", true},
		{"case insensitive", "TEST", "test", true},
		{"mixed case", "TeSt", "tEsT", true},
		{"substring match", "game", "Game of Thrones S01E01", true},
		{"no match", "xyz", "Game of Thrones", false},
		{"empty title", "test", "", false},
		{"chinese characters", "权力的游戏", "权力的游戏 第一季", true},
		{"special characters", "S01E01", "Show.Name.S01E01.720p", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewKeywordMatcher(tt.keyword)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, matcher.Match(tt.title))
		})
	}
}

// TestKeywordMatcherValidation tests validation logic
func TestKeywordMatcherValidation(t *testing.T) {
	t.Run("empty pattern returns error", func(t *testing.T) {
		_, err := NewKeywordMatcher("")
		assert.ErrorIs(t, err, ErrEmptyPattern)
	})

	t.Run("pattern too long returns error", func(t *testing.T) {
		longPattern := strings.Repeat("a", MaxPatternLength+1)
		_, err := NewKeywordMatcher(longPattern)
		assert.ErrorIs(t, err, ErrPatternTooLong)
	})

	t.Run("valid pattern returns no error", func(t *testing.T) {
		matcher, err := NewKeywordMatcher("test")
		require.NoError(t, err)
		assert.NoError(t, matcher.Validate())
	})
}

// TestWildcardMatcherConversion tests Property 3: Wildcard to Regex Conversion
// Feature: rss-filter-and-downloader-autostart, Property 3: Wildcard to Regex Conversion
// *For any* wildcard pattern containing `*` and `?` characters, the WildcardMatcher should correctly
// convert it to a regex where `*` matches any sequence of characters and `?` matches exactly one character.
// **Validates: Requirements 2.4**
func TestWildcardMatcherConversion(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: * matches any sequence of characters (including empty)
	properties.Property("asterisk matches any sequence", prop.ForAll(
		func(prefix, middle, suffix string) bool {
			pattern := prefix + "*" + suffix
			matcher, err := NewWildcardMatcher(pattern)
			if err != nil {
				return true // Skip invalid patterns
			}

			// Title with any middle content should match
			title := prefix + middle + suffix
			return matcher.Match(title)
		},
		gen.AnyString().Map(func(s string) string {
			if len(s) > 10 {
				return s[:10]
			}
			return s
		}),
		gen.AnyString().Map(func(s string) string {
			if len(s) > 20 {
				return s[:20]
			}
			return s
		}),
		gen.AnyString().Map(func(s string) string {
			if len(s) > 10 {
				return s[:10]
			}
			return s
		}),
	))

	// Property: ? matches exactly one character
	properties.Property("question mark matches single character", prop.ForAll(
		func(prefix, suffix string, char rune) bool {
			pattern := prefix + "?" + suffix
			matcher, err := NewWildcardMatcher(pattern)
			if err != nil {
				return true
			}

			// Title with exactly one character in place of ? should match
			title := prefix + string(char) + suffix
			return matcher.Match(title)
		},
		gen.AnyString().Map(func(s string) string {
			if len(s) > 10 {
				return s[:10]
			}
			return s
		}),
		gen.AnyString().Map(func(s string) string {
			if len(s) > 10 {
				return s[:10]
			}
			return s
		}),
		gen.Rune(),
	))

	// Property: Wildcard matching is case-insensitive
	properties.Property("wildcard match is case-insensitive", prop.ForAll(
		func(pattern, title string) bool {
			if pattern == "" {
				return true
			}
			matcher, err := NewWildcardMatcher(pattern)
			if err != nil {
				return true
			}

			result1 := matcher.Match(title)
			result2 := matcher.Match(strings.ToUpper(title))
			result3 := matcher.Match(strings.ToLower(title))

			return result1 == result2 && result2 == result3
		},
		gen.AnyString().Map(func(s string) string {
			if len(s) == 0 {
				return "a"
			}
			if len(s) > 20 {
				return s[:20]
			}
			return s
		}),
		gen.AnyString().Map(func(s string) string {
			if len(s) > 30 {
				return s[:30]
			}
			return s
		}),
	))

	properties.TestingRun(t)
}

// TestWildcardMatcherUnit provides unit tests for WildcardMatcher
func TestWildcardMatcherUnit(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		title    string
		expected bool
	}{
		{"asterisk matches any", "*S01E*", "Show.Name.S01E01.720p", true},
		{"asterisk matches empty", "test*", "test", true},
		{"question mark single char", "S0?E01", "S01E01", true},
		{"question mark requires char", "S0?E01", "S0E01", false},
		{"multiple asterisks", "*Game*Thrones*", "Game of Thrones S01E01", true},
		{"multiple question marks", "S??E??", "S01E02", true},
		{"mixed wildcards", "*S??E??*", "Show.S01E02.720p", true},
		{"no wildcards exact match", "test", "test", true},
		{"no wildcards substring match", "test", "testing", true},
		{"no wildcards no match", "xyz", "testing", false},
		{"case insensitive", "*GAME*", "game of thrones", true},
		{"chinese with wildcards", "*权力的游戏*", "美剧 权力的游戏 第一季", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewWildcardMatcher(tt.pattern)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, matcher.Match(tt.title))
		})
	}
}

// TestWildcardMatcherValidation tests validation logic
func TestWildcardMatcherValidation(t *testing.T) {
	t.Run("empty pattern returns error", func(t *testing.T) {
		_, err := NewWildcardMatcher("")
		assert.ErrorIs(t, err, ErrEmptyPattern)
	})

	t.Run("pattern too long returns error", func(t *testing.T) {
		longPattern := strings.Repeat("a", MaxPatternLength+1)
		_, err := NewWildcardMatcher(longPattern)
		assert.ErrorIs(t, err, ErrPatternTooLong)
	})

	t.Run("valid pattern returns no error", func(t *testing.T) {
		matcher, err := NewWildcardMatcher("*test*")
		require.NoError(t, err)
		assert.NoError(t, matcher.Validate())
	})
}

// TestRegexMatcherDirectUsage tests Property 4: Regex Pattern Direct Usage
// Feature: rss-filter-and-downloader-autostart, Property 4: Regex Pattern Direct Usage
// *For any* valid regex pattern, the RegexMatcher should use it directly for matching.
// *For any* invalid regex pattern, the RegexMatcher should return a validation error.
// **Validates: Requirements 2.5**
func TestRegexMatcherDirectUsage(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property: Valid regex patterns work correctly
	properties.Property("valid regex patterns match correctly", prop.ForAll(
		func(pattern, title string) bool {
			if pattern == "" {
				return true
			}
			matcher, err := NewRegexMatcher(pattern)
			if err != nil {
				// If pattern is invalid, that's expected behavior
				return true
			}

			// The matcher should behave like the regex
			result := matcher.Match(title)
			// Verify it matches the expected regex behavior (case-insensitive)
			expected := matcher.regex.MatchString(title)
			return result == expected
		},
		gen.RegexMatch("[a-zA-Z0-9.*+?]+").Map(func(s string) string {
			if len(s) > 30 {
				return s[:30]
			}
			return s
		}),
		gen.AnyString().Map(func(s string) string {
			if len(s) > 50 {
				return s[:50]
			}
			return s
		}),
	))

	// Property: Regex matching is case-insensitive
	properties.Property("regex match is case-insensitive", prop.ForAll(
		func(pattern, title string) bool {
			if pattern == "" {
				return true
			}
			matcher, err := NewRegexMatcher(pattern)
			if err != nil {
				return true
			}

			result1 := matcher.Match(title)
			result2 := matcher.Match(strings.ToUpper(title))
			result3 := matcher.Match(strings.ToLower(title))

			return result1 == result2 && result2 == result3
		},
		gen.RegexMatch("[a-zA-Z]+").Map(func(s string) string {
			if len(s) == 0 {
				return "a"
			}
			if len(s) > 20 {
				return s[:20]
			}
			return s
		}),
		gen.AnyString().Map(func(s string) string {
			if len(s) > 30 {
				return s[:30]
			}
			return s
		}),
	))

	properties.TestingRun(t)
}

// TestRegexMatcherInvalidValidation tests Property 10: Invalid Regex Validation
// Feature: rss-filter-and-downloader-autostart, Property 10: Invalid Regex Validation
// *For any* pattern submitted as regex type, if the pattern is not a valid regular expression,
// the system should return a validation error and not create/update the filter rule.
// **Validates: Requirements 4.6**
func TestRegexMatcherInvalidValidation(t *testing.T) {
	invalidPatterns := []string{
		"[",          // Unclosed bracket
		"(",          // Unclosed parenthesis
		"*",          // Nothing to repeat
		"+",          // Nothing to repeat
		"?",          // Nothing to repeat
		"[a-",        // Incomplete range
		"(?P<>test)", // Empty group name
		"\\",         // Trailing backslash
	}

	for _, pattern := range invalidPatterns {
		t.Run("invalid_"+pattern, func(t *testing.T) {
			_, err := NewRegexMatcher(pattern)
			assert.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidRegex)
		})
	}
}

// TestRegexMatcherUnit provides unit tests for RegexMatcher
func TestRegexMatcherUnit(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		title    string
		expected bool
	}{
		{"simple pattern", "test", "this is a test", true},
		{"anchored start", "^Game", "Game of Thrones", true},
		{"anchored start no match", "^Game", "The Game", false},
		{"anchored end", "720p$", "Show.S01E01.720p", true},
		{"character class", "[Ss]\\d{2}[Ee]\\d{2}", "Show.S01E02.720p", true},
		{"alternation", "720p|1080p", "Show.S01E01.1080p", true},
		{"case insensitive", "GAME", "game of thrones", true},
		{"chinese pattern", "权力.*游戏", "权力的游戏 第一季", true},
		{"complex pattern", "S\\d{2}E\\d{2}", "Show.Name.S01E05.720p.BluRay", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewRegexMatcher(tt.pattern)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, matcher.Match(tt.title))
		})
	}
}

// TestRegexMatcherValidation tests validation logic
func TestRegexMatcherValidation(t *testing.T) {
	t.Run("empty pattern returns error", func(t *testing.T) {
		_, err := NewRegexMatcher("")
		assert.ErrorIs(t, err, ErrEmptyPattern)
	})

	t.Run("pattern too long returns error", func(t *testing.T) {
		longPattern := strings.Repeat("a", MaxPatternLength+1)
		_, err := NewRegexMatcher(longPattern)
		assert.ErrorIs(t, err, ErrPatternTooLong)
	})

	t.Run("invalid regex returns error", func(t *testing.T) {
		_, err := NewRegexMatcher("[invalid")
		assert.ErrorIs(t, err, ErrInvalidRegex)
	})

	t.Run("valid pattern returns no error", func(t *testing.T) {
		matcher, err := NewRegexMatcher("test.*pattern")
		require.NoError(t, err)
		assert.NoError(t, matcher.Validate())
	})
}

// TestValidateRegexPattern tests the standalone validation function
func TestValidateRegexPattern(t *testing.T) {
	t.Run("valid pattern", func(t *testing.T) {
		err := ValidateRegexPattern("test.*pattern")
		assert.NoError(t, err)
	})

	t.Run("empty pattern", func(t *testing.T) {
		err := ValidateRegexPattern("")
		assert.ErrorIs(t, err, ErrEmptyPattern)
	})

	t.Run("invalid pattern", func(t *testing.T) {
		err := ValidateRegexPattern("[invalid")
		assert.ErrorIs(t, err, ErrInvalidRegex)
	})
}
