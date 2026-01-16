package v2

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FilterFunc is a function that transforms a value
type FilterFunc func(value any, args ...any) any

var (
	filtersMu       sync.RWMutex
	builtinFilters  map[string]FilterFunc
	customFilters   map[string]FilterFunc
	filtersInitOnce sync.Once
)

func initFilters() {
	filtersInitOnce.Do(func() {
		builtinFilters = map[string]FilterFunc{
			"parseNumber":     parseNumberFilter,
			"parseSize":       parseSizeFilter,
			"parseTime":       parseTimeFilter,
			"querystring":     querystringFilter,
			"split":           splitFilter,
			"prepend":         prependFilter,
			"append":          appendFilter,
			"replace":         replaceFilter,
			"trim":            trimFilter,
			"extDoubanId":     extDoubanIdFilter,
			"extImdbId":       extImdbIdFilter,
			"parseInt":        parseIntFilter,
			"parseFloat":      parseFloatFilter,
			"toLowerCase":     toLowerCaseFilter,
			"toUpperCase":     toUpperCaseFilter,
			"regex":           regexFilter,
			"regexReplace":    regexReplaceFilter,
			"sumRegexMatches": sumRegexMatchesFilter,
			"default":         defaultFilter,
			"multiply":        multiplyFilter,
			"divide":          divideFilter,
		}
		customFilters = make(map[string]FilterFunc)
	})
}

// RegisterFilter adds a custom filter
func RegisterFilter(name string, fn FilterFunc) {
	initFilters()
	filtersMu.Lock()
	defer filtersMu.Unlock()
	customFilters[name] = fn
}

// GetFilter returns a filter by name
func GetFilter(name string) (FilterFunc, bool) {
	initFilters()
	filtersMu.RLock()
	defer filtersMu.RUnlock()

	// Check custom filters first
	if fn, ok := customFilters[name]; ok {
		return fn, true
	}
	// Then check builtin filters
	if fn, ok := builtinFilters[name]; ok {
		return fn, true
	}
	return nil, false
}

// ApplyFilters applies a chain of filters to a value
func ApplyFilters(value any, filters []Filter) any {
	initFilters()
	for _, filter := range filters {
		fn, ok := GetFilter(filter.Name)
		if !ok {
			continue
		}
		value = fn(value, filter.Args...)
	}
	return value
}

// parseNumberFilter extracts a number from a string
func parseNumberFilter(value any, args ...any) any {
	str := toString(value)
	str = strings.ReplaceAll(str, ",", "")
	str = strings.ReplaceAll(str, " ", "")

	re := regexp.MustCompile(`-?[\d.]+`)
	match := re.FindString(str)
	if match == "" {
		return float64(0)
	}

	if strings.Contains(match, ".") {
		f, err := strconv.ParseFloat(match, 64)
		if err != nil {
			return float64(0)
		}
		return f
	}

	i, err := strconv.ParseInt(match, 10, 64)
	if err != nil {
		return float64(0)
	}
	return float64(i)
}

// parseSizeFilter parses a size string to bytes
func parseSizeFilter(value any, args ...any) any {
	str := toString(value)
	return parseSize(str)
}

// parseTimeFilter parses a time string to Unix timestamp
func parseTimeFilter(value any, args ...any) any {
	str := toString(value)
	t := parseTimeString(str)
	if t.IsZero() {
		return int64(0)
	}
	return t.Unix()
}

// querystringFilter extracts a query parameter from a URL
func querystringFilter(value any, args ...any) any {
	str := toString(value)
	if len(args) == 0 {
		return ""
	}
	paramName := toString(args[0])

	// Try to parse as URL
	u, err := url.Parse(str)
	if err != nil {
		// Try to parse just the query string
		if strings.Contains(str, "=") {
			values, err := url.ParseQuery(str)
			if err == nil {
				return values.Get(paramName)
			}
		}
		return ""
	}
	return u.Query().Get(paramName)
}

// splitFilter splits a string and returns the element at index
func splitFilter(value any, args ...any) any {
	str := toString(value)
	if len(args) < 2 {
		return str
	}
	separator := toString(args[0])
	index := toInt(args[1])

	parts := strings.Split(str, separator)
	if index < 0 {
		index = len(parts) + index
	}
	if index < 0 || index >= len(parts) {
		return ""
	}
	return strings.TrimSpace(parts[index])
}

// prependFilter prepends a string
func prependFilter(value any, args ...any) any {
	str := toString(value)
	if len(args) == 0 {
		return str
	}
	prefix := toString(args[0])
	return prefix + str
}

// appendFilter appends a string
func appendFilter(value any, args ...any) any {
	str := toString(value)
	if len(args) == 0 {
		return str
	}
	suffix := toString(args[0])
	return str + suffix
}

// replaceFilter replaces a substring
func replaceFilter(value any, args ...any) any {
	str := toString(value)
	if len(args) < 2 {
		return str
	}
	old := toString(args[0])
	new := toString(args[1])
	return strings.ReplaceAll(str, old, new)
}

// trimFilter trims whitespace
func trimFilter(value any, args ...any) any {
	str := toString(value)
	if len(args) > 0 {
		cutset := toString(args[0])
		return strings.Trim(str, cutset)
	}
	return strings.TrimSpace(str)
}

// extDoubanIdFilter extracts Douban ID from URL or string
func extDoubanIdFilter(value any, args ...any) any {
	str := toString(value)
	re := regexp.MustCompile(`(?:douban\.com/subject/|douban=)(\d+)`)
	matches := re.FindStringSubmatch(str)
	if len(matches) > 1 {
		return matches[1]
	}
	// Try direct number
	re = regexp.MustCompile(`^\d+$`)
	if re.MatchString(str) {
		return str
	}
	return ""
}

// extImdbIdFilter extracts IMDB ID from URL or string
func extImdbIdFilter(value any, args ...any) any {
	str := toString(value)
	re := regexp.MustCompile(`(?:imdb\.com/title/|imdb=)(tt\d+)`)
	matches := re.FindStringSubmatch(str)
	if len(matches) > 1 {
		return matches[1]
	}
	// Try direct tt format
	re = regexp.MustCompile(`^tt\d+$`)
	if re.MatchString(str) {
		return str
	}
	return ""
}

// parseIntFilter parses string to int
func parseIntFilter(value any, args ...any) any {
	str := toString(value)
	str = strings.ReplaceAll(str, ",", "")
	i, _ := strconv.ParseInt(str, 10, 64)
	return i
}

// parseFloatFilter parses string to float
func parseFloatFilter(value any, args ...any) any {
	str := toString(value)
	str = strings.ReplaceAll(str, ",", "")
	f, _ := strconv.ParseFloat(str, 64)
	return f
}

// toLowerCaseFilter converts to lowercase
func toLowerCaseFilter(value any, args ...any) any {
	return strings.ToLower(toString(value))
}

// toUpperCaseFilter converts to uppercase
func toUpperCaseFilter(value any, args ...any) any {
	return strings.ToUpper(toString(value))
}

// regexFilter extracts using regex
func regexFilter(value any, args ...any) any {
	str := toString(value)
	if len(args) == 0 {
		return str
	}
	pattern := toString(args[0])
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ""
	}
	matches := re.FindStringSubmatch(str)
	if len(matches) > 1 {
		return matches[1]
	}
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

// regexReplaceFilter replaces using regex
func regexReplaceFilter(value any, args ...any) any {
	str := toString(value)
	if len(args) < 2 {
		return str
	}
	pattern := toString(args[0])
	replacement := toString(args[1])
	re, err := regexp.Compile(pattern)
	if err != nil {
		return str
	}
	return re.ReplaceAllString(str, replacement)
}

// sumRegexMatchesFilter finds all regex matches and sums the captured numbers
// Usage: {Name: "sumRegexMatches", Args: []any{`你有(\d+)条新`}}
// This will find all occurrences like "你有1条新" and "你有2条新" and return 3
func sumRegexMatchesFilter(value any, args ...any) any {
	str := toString(value)
	if len(args) == 0 {
		return 0
	}
	pattern := toString(args[0])
	re, err := regexp.Compile(pattern)
	if err != nil {
		return 0
	}

	matches := re.FindAllStringSubmatch(str, -1)
	sum := 0
	for _, match := range matches {
		if len(match) > 1 {
			// Use the first capture group
			if num, err := strconv.Atoi(match[1]); err == nil {
				sum += num
			}
		}
	}
	return sum
}

// defaultFilter returns default value if empty
func defaultFilter(value any, args ...any) any {
	str := toString(value)
	if str == "" && len(args) > 0 {
		return args[0]
	}
	return value
}

// multiplyFilter multiplies a number
func multiplyFilter(value any, args ...any) any {
	num := toFloat64(value)
	if len(args) > 0 {
		multiplier := toFloat64(args[0])
		return num * multiplier
	}
	return num
}

// divideFilter divides a number
func divideFilter(value any, args ...any) any {
	num := toFloat64(value)
	if len(args) > 0 {
		divisor := toFloat64(args[0])
		if divisor != 0 {
			return num / divisor
		}
	}
	return num
}

// Helper functions for type conversion

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", val)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", val)
	case float32, float64:
		return fmt.Sprintf("%v", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func toInt(v any) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		i, _ := strconv.Atoi(val)
		return i
	default:
		return 0
	}
}

func toFloat64(v any) float64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0
	}
}

// parseTimeString parses various time formats
func parseTimeString(timeStr string) time.Time {
	timeStr = strings.TrimSpace(timeStr)
	if timeStr == "" {
		return time.Time{}
	}

	// Try various formats
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006/01/02 15:04:05",
		"2006/01/02 15:04",
		"01-02 15:04",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		time.RFC3339,
		time.RFC1123,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t
		}
	}

	// Try Unix timestamp
	if ts, err := strconv.ParseInt(timeStr, 10, 64); err == nil {
		if ts > 1e12 {
			// Milliseconds
			return time.Unix(ts/1000, (ts%1000)*1e6)
		}
		return time.Unix(ts, 0)
	}

	return time.Time{}
}
