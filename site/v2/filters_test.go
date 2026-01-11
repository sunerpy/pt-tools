package v2

import (
	"testing"
)

func TestParseNumberFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected float64
	}{
		{"simple integer", "123", 123},
		{"integer with commas", "1,234,567", 1234567},
		{"float", "123.45", 123.45},
		{"negative number", "-123", -123},
		{"number with text", "Size: 123 MB", 123},
		{"number with spaces", "  456  ", 456},
		{"empty string", "", 0},
		{"no number", "abc", 0},
		{"float with commas", "1,234.56", 1234.56},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseNumberFilter(tt.input)
			if result != tt.expected {
				t.Errorf("parseNumberFilter(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseSizeFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected int64
	}{
		{"bytes", "100", 100},
		{"kilobytes", "1KB", 1024},
		{"megabytes", "1MB", 1024 * 1024},
		{"gigabytes", "1GB", 1024 * 1024 * 1024},
		{"terabytes", "1TB", 1024 * 1024 * 1024 * 1024},
		{"petabytes", "1PB", 1024 * 1024 * 1024 * 1024 * 1024},
		{"with decimal", "1.5GB", int64(1.5 * 1024 * 1024 * 1024)},
		{"with space", "100 MB", 100 * 1024 * 1024},
		{"lowercase", "500mb", 500 * 1024 * 1024},
		{"with commas", "1,024MB", 1024 * 1024 * 1024},
		{"empty string", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseSizeFilter(tt.input)
			if result != tt.expected {
				t.Errorf("parseSizeFilter(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseTimeFilter(t *testing.T) {
	tests := []struct {
		name        string
		input       interface{}
		expectZero  bool
		expectValid bool
	}{
		{"standard format", "2024-01-15 10:30:00", false, true},
		{"date only format", "2024-01-15 10:30", false, true},
		{"slash format", "2024/01/15 10:30:00", false, true},
		{"unix timestamp", "1705312200", false, true},
		{"unix timestamp ms", "1705312200000", false, true},
		{"empty string", "", true, false},
		{"invalid format", "not a date", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTimeFilter(tt.input)
			resultInt, ok := result.(int64)
			if !ok {
				t.Errorf("parseTimeFilter(%v) did not return int64", tt.input)
				return
			}
			if tt.expectZero && resultInt != 0 {
				t.Errorf("parseTimeFilter(%v) = %v, want 0", tt.input, resultInt)
			}
			if tt.expectValid && resultInt == 0 {
				t.Errorf("parseTimeFilter(%v) = 0, want non-zero", tt.input)
			}
		})
	}
}

func TestQuerystringFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		args     []interface{}
		expected string
	}{
		{"full URL", "https://example.com/page?id=123&name=test", []interface{}{"id"}, "123"},
		{"query string only", "?id=123&name=test", []interface{}{"id"}, "123"},
		{"missing param", "https://example.com/page?id=123", []interface{}{"name"}, ""},
		{"no args", "https://example.com/page?id=123", []interface{}{}, ""},
		{"relative URL", "/userdetails.php?id=456", []interface{}{"id"}, "456"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := querystringFilter(tt.input, tt.args...)
			if result != tt.expected {
				t.Errorf("querystringFilter(%v, %v) = %v, want %v", tt.input, tt.args, result, tt.expected)
			}
		})
	}
}

func TestSplitFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		args     []interface{}
		expected string
	}{
		{"split by space", "hello world", []interface{}{" ", 0}, "hello"},
		{"split by space second", "hello world", []interface{}{" ", 1}, "world"},
		{"split by comma", "a,b,c", []interface{}{",", 1}, "b"},
		{"negative index", "a,b,c", []interface{}{",", -1}, "c"},
		{"out of bounds", "a,b", []interface{}{",", 5}, ""},
		{"no args", "hello world", []interface{}{}, "hello world"},
		{"split with trim", "  hello  ,  world  ", []interface{}{",", 1}, "world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitFilter(tt.input, tt.args...)
			if result != tt.expected {
				t.Errorf("splitFilter(%v, %v) = %v, want %v", tt.input, tt.args, result, tt.expected)
			}
		})
	}
}

func TestPrependFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		args     []interface{}
		expected string
	}{
		{"prepend string", "world", []interface{}{"hello "}, "hello world"},
		{"prepend empty", "world", []interface{}{""}, "world"},
		{"no args", "world", []interface{}{}, "world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := prependFilter(tt.input, tt.args...)
			if result != tt.expected {
				t.Errorf("prependFilter(%v, %v) = %v, want %v", tt.input, tt.args, result, tt.expected)
			}
		})
	}
}

func TestAppendFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		args     []interface{}
		expected string
	}{
		{"append string", "hello", []interface{}{" world"}, "hello world"},
		{"append empty", "hello", []interface{}{""}, "hello"},
		{"no args", "hello", []interface{}{}, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := appendFilter(tt.input, tt.args...)
			if result != tt.expected {
				t.Errorf("appendFilter(%v, %v) = %v, want %v", tt.input, tt.args, result, tt.expected)
			}
		})
	}
}

func TestReplaceFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		args     []interface{}
		expected string
	}{
		{"replace substring", "hello world", []interface{}{"world", "go"}, "hello go"},
		{"replace multiple", "aaa", []interface{}{"a", "b"}, "bbb"},
		{"no match", "hello", []interface{}{"x", "y"}, "hello"},
		{"insufficient args", "hello", []interface{}{"x"}, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := replaceFilter(tt.input, tt.args...)
			if result != tt.expected {
				t.Errorf("replaceFilter(%v, %v) = %v, want %v", tt.input, tt.args, result, tt.expected)
			}
		})
	}
}

func TestTrimFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		args     []interface{}
		expected string
	}{
		{"trim whitespace", "  hello  ", []interface{}{}, "hello"},
		{"trim custom chars", "###hello###", []interface{}{"#"}, "hello"},
		{"no trim needed", "hello", []interface{}{}, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimFilter(tt.input, tt.args...)
			if result != tt.expected {
				t.Errorf("trimFilter(%v, %v) = %v, want %v", tt.input, tt.args, result, tt.expected)
			}
		})
	}
}

func TestExtDoubanIdFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{"douban URL", "https://movie.douban.com/subject/1234567/", "1234567"},
		{"douban param", "douban=1234567", "1234567"},
		{"direct ID", "1234567", "1234567"},
		{"no match", "hello world", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extDoubanIdFilter(tt.input)
			if result != tt.expected {
				t.Errorf("extDoubanIdFilter(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtImdbIdFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{"imdb URL", "https://www.imdb.com/title/tt1234567/", "tt1234567"},
		{"imdb param", "imdb=tt1234567", "tt1234567"},
		{"direct ID", "tt1234567", "tt1234567"},
		{"no match", "hello world", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extImdbIdFilter(tt.input)
			if result != tt.expected {
				t.Errorf("extImdbIdFilter(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRegexFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		args     []interface{}
		expected string
	}{
		{"capture group", "Size: 123 MB", []interface{}{`Size:\s*(\d+)`}, "123"},
		{"full match", "abc123def", []interface{}{`\d+`}, "123"},
		{"no match", "hello", []interface{}{`\d+`}, ""},
		{"no args", "hello", []interface{}{}, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := regexFilter(tt.input, tt.args...)
			if result != tt.expected {
				t.Errorf("regexFilter(%v, %v) = %v, want %v", tt.input, tt.args, result, tt.expected)
			}
		})
	}
}

func TestRegexReplaceFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		args     []interface{}
		expected string
	}{
		{"replace digits", "abc123def", []interface{}{`\d+`, "XXX"}, "abcXXXdef"},
		{"replace with capture", "hello world", []interface{}{`(\w+) (\w+)`, "$2 $1"}, "world hello"},
		{"no match", "hello", []interface{}{`\d+`, "X"}, "hello"},
		{"insufficient args", "hello", []interface{}{`\d+`}, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := regexReplaceFilter(tt.input, tt.args...)
			if result != tt.expected {
				t.Errorf("regexReplaceFilter(%v, %v) = %v, want %v", tt.input, tt.args, result, tt.expected)
			}
		})
	}
}

func TestDefaultFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		args     []interface{}
		expected interface{}
	}{
		{"empty string with default", "", []interface{}{"default"}, "default"},
		{"non-empty string", "hello", []interface{}{"default"}, "hello"},
		{"no args", "", []interface{}{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := defaultFilter(tt.input, tt.args...)
			if result != tt.expected {
				t.Errorf("defaultFilter(%v, %v) = %v, want %v", tt.input, tt.args, result, tt.expected)
			}
		})
	}
}

func TestMultiplyFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		args     []interface{}
		expected float64
	}{
		{"multiply int", 10, []interface{}{2}, 20},
		{"multiply float", 10.5, []interface{}{2}, 21},
		{"multiply string", "10", []interface{}{2}, 20},
		{"no args", 10, []interface{}{}, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := multiplyFilter(tt.input, tt.args...)
			if result != tt.expected {
				t.Errorf("multiplyFilter(%v, %v) = %v, want %v", tt.input, tt.args, result, tt.expected)
			}
		})
	}
}

func TestDivideFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		args     []interface{}
		expected float64
	}{
		{"divide int", 10, []interface{}{2}, 5},
		{"divide float", 10.5, []interface{}{2}, 5.25},
		{"divide by zero", 10, []interface{}{0}, 10},
		{"no args", 10, []interface{}{}, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := divideFilter(tt.input, tt.args...)
			if result != tt.expected {
				t.Errorf("divideFilter(%v, %v) = %v, want %v", tt.input, tt.args, result, tt.expected)
			}
		})
	}
}

func TestApplyFilters(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		filters  []Filter
		expected interface{}
	}{
		{
			name:  "chain parseNumber and multiply",
			input: "100",
			filters: []Filter{
				{Name: "parseNumber"},
				{Name: "multiply", Args: []interface{}{2}},
			},
			expected: float64(200),
		},
		{
			name:  "chain split and trim",
			input: "hello , world",
			filters: []Filter{
				{Name: "split", Args: []interface{}{",", 1}},
				{Name: "trim"},
			},
			expected: "world",
		},
		{
			name:     "empty filters",
			input:    "hello",
			filters:  []Filter{},
			expected: "hello",
		},
		{
			name:  "unknown filter ignored",
			input: "hello",
			filters: []Filter{
				{Name: "unknownFilter"},
			},
			expected: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyFilters(tt.input, tt.filters)
			if result != tt.expected {
				t.Errorf("ApplyFilters(%v, %v) = %v, want %v", tt.input, tt.filters, result, tt.expected)
			}
		})
	}
}

func TestSumRegexMatchesFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		args     []interface{}
		expected int
	}{
		{
			name:     "SpringSunday single message",
			input:    `你有1条新私人短讯！点击查看`,
			args:     []interface{}{`你有(\d+)条新`},
			expected: 1,
		},
		{
			name:     "SpringSunday multiple messages",
			input:    `你有2条新系统短讯！你有3条新私人短讯！`,
			args:     []interface{}{`你有(\d+)条新`},
			expected: 5, // 2 + 3
		},
		{
			name:     "SpringSunday full HTML",
			input:    `<b style="background: darkorange;">你有1条新系统短讯！点击查看</b><b style="background: red;">你有2条新私人短讯！点击查看</b>`,
			args:     []interface{}{`你有(\d+)条新`},
			expected: 3, // 1 + 2
		},
		{
			name:     "no match",
			input:    `无新短讯`,
			args:     []interface{}{`你有(\d+)条新`},
			expected: 0,
		},
		{
			name:     "no args",
			input:    `你有1条新私人短讯！`,
			args:     []interface{}{},
			expected: 0,
		},
		{
			name:     "invalid regex",
			input:    `你有1条新私人短讯！`,
			args:     []interface{}{`[invalid`},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sumRegexMatchesFilter(tt.input, tt.args...)
			if result != tt.expected {
				t.Errorf("sumRegexMatchesFilter(%v, %v) = %v, want %v", tt.input, tt.args, result, tt.expected)
			}
		})
	}
}

func TestRegisterFilter(t *testing.T) {
	// Register a custom filter
	RegisterFilter("double", func(value interface{}, args ...interface{}) interface{} {
		return toFloat64(value) * 2
	})

	// Test the custom filter
	fn, ok := GetFilter("double")
	if !ok {
		t.Fatal("custom filter 'double' not found")
	}

	result := fn(10)
	if result != float64(20) {
		t.Errorf("custom filter 'double'(10) = %v, want 20", result)
	}
}

func TestToString(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{"string", "hello", "hello"},
		{"int", 123, "123"},
		{"int64", int64(123), "123"},
		{"float64", 123.45, "123.45"},
		{"bool", true, "true"},
		{"nil", nil, ""},
		{"bytes", []byte("hello"), "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toString(tt.input)
			if result != tt.expected {
				t.Errorf("toString(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestToInt(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected int
	}{
		{"int", 123, 123},
		{"int64", int64(123), 123},
		{"float64", 123.7, 123},
		{"string", "123", 123},
		{"nil", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toInt(tt.input)
			if result != tt.expected {
				t.Errorf("toInt(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected float64
	}{
		{"float64", 123.45, 123.45},
		{"float32", float32(123.45), float64(float32(123.45))},
		{"int", 123, 123},
		{"int64", int64(123), 123},
		{"string", "123.45", 123.45},
		{"nil", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toFloat64(tt.input)
			if result != tt.expected {
				t.Errorf("toFloat64(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
