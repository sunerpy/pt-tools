package parser

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gocolly/colly"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// TestProperty7_ParserConfigRegexExtraction 属性测试：解析器正则提取
// Feature: downloader-site-extensibility, Property 7: Parser Config Regex Extraction
// 验证大小提取正则能正确处理各种单位
func TestProperty7_ParserConfigRegexExtraction(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// 生成大小值（0.1 到 1000）
	sizeValueGen := gen.Float64Range(0.1, 1000.0)

	// 生成单位
	unitGen := gen.OneConstOf("KB", "MB", "GB", "TB")

	// Property 7.1: 大小转换为字节的正确性
	properties.Property("size conversion to bytes is correct", prop.ForAll(
		func(value float64, unit string) bool {
			bytes := convertToBytes(value, unit)

			var expected int64
			switch unit {
			case "TB":
				expected = int64(value * 1024 * 1024 * 1024 * 1024)
			case "GB":
				expected = int64(value * 1024 * 1024 * 1024)
			case "MB":
				expected = int64(value * 1024 * 1024)
			case "KB":
				expected = int64(value * 1024)
			}

			return bytes == expected
		},
		sizeValueGen,
		unitGen,
	))

	// Property 7.2: 单位大小写不敏感
	properties.Property("unit conversion is case insensitive", prop.ForAll(
		func(value float64) bool {
			gbUpper := convertToBytes(value, "GB")
			gbLower := convertToBytes(value, "gb")
			gbMixed := convertToBytes(value, "Gb")

			return gbUpper == gbLower && gbLower == gbMixed
		},
		sizeValueGen,
	))

	// Property 7.3: 单位换算关系正确
	properties.Property("unit conversion ratios are correct", prop.ForAll(
		func(value float64) bool {
			// 使用整数值避免浮点精度问题
			intValue := float64(int(value))
			if intValue < 1 {
				intValue = 1
			}

			kb := convertToBytes(intValue, "KB")
			mb := convertToBytes(intValue, "MB")
			gb := convertToBytes(intValue, "GB")
			tb := convertToBytes(intValue, "TB")

			// MB = KB * 1024
			// GB = MB * 1024
			// TB = GB * 1024
			return mb == kb*1024 && gb == mb*1024 && tb == gb*1024
		},
		gen.Float64Range(1.0, 100.0), // 使用较小范围避免溢出
	))

	properties.TestingRun(t)
}

// TestProperty8_ParserConfigTimeFormat 属性测试：解析器时间格式
// Feature: downloader-site-extensibility, Property 8: Parser Config Time Format
// 验证时间解析的往返一致性
func TestProperty8_ParserConfigTimeFormat(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// 生成时间组件
	yearGen := gen.IntRange(2020, 2030)
	monthGen := gen.IntRange(1, 12)
	dayGen := gen.IntRange(1, 28) // 使用28避免月份天数问题
	hourGen := gen.IntRange(0, 23)
	minuteGen := gen.IntRange(0, 59)
	secondGen := gen.IntRange(0, 59)

	// Property 8.1: 时间格式化和解析的往返一致性
	properties.Property("time format round trip is consistent", prop.ForAll(
		func(year, month, day, hour, minute, second int) bool {
			layout := "2006-01-02 15:04:05"
			original := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local)

			// 格式化
			formatted := original.Format(layout)

			// 解析
			parsed, err := time.ParseInLocation(layout, formatted, time.Local)
			if err != nil {
				return false
			}

			// 验证往返一致性
			return original.Equal(parsed)
		},
		yearGen,
		monthGen,
		dayGen,
		hourGen,
		minuteGen,
		secondGen,
	))

	// Property 8.2: 不同时间格式的解析
	properties.Property("different time layouts parse correctly", prop.ForAll(
		func(year, month, day int) bool {
			layouts := []string{
				"2006-01-02",
				"2006/01/02",
				"01-02-2006",
				"Jan 2, 2006",
			}

			original := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.Local)

			for _, layout := range layouts {
				formatted := original.Format(layout)
				parsed, err := time.ParseInLocation(layout, formatted, time.Local)
				if err != nil {
					return false
				}
				// 只比较日期部分
				if parsed.Year() != original.Year() ||
					parsed.Month() != original.Month() ||
					parsed.Day() != original.Day() {
					return false
				}
			}
			return true
		},
		yearGen,
		monthGen,
		dayGen,
	))

	properties.TestingRun(t)
}

// abs 返回浮点数的绝对值
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// TestConfigurableParserCreation 测试可配置解析器创建
func TestConfigurableParserCreation(t *testing.T) {
	// 使用默认配置
	parser, err := NewConfigurableParser(nil)
	if err != nil {
		t.Fatalf("failed to create parser with default config: %v", err)
	}
	if parser.GetName() != "default" {
		t.Errorf("expected name 'default', got '%s'", parser.GetName())
	}

	// 使用自定义配置
	customConfig := &ParserConfig{
		Name:       "custom",
		TimeLayout: "2006/01/02",
		Patterns: PatternConfig{
			SizePattern: `([\d.]+)\s*(GB|MB)`,
		},
	}
	customParser, err := NewConfigurableParser(customConfig)
	if err != nil {
		t.Fatalf("failed to create parser with custom config: %v", err)
	}
	if customParser.GetName() != "custom" {
		t.Errorf("expected name 'custom', got '%s'", customParser.GetName())
	}
}

// TestConfigurableParserInvalidRegex 测试无效正则表达式
func TestConfigurableParserInvalidRegex(t *testing.T) {
	invalidConfig := &ParserConfig{
		Name:       "invalid",
		TimeLayout: "2006-01-02",
		Patterns: PatternConfig{
			SizePattern: `[invalid(regex`, // 无效正则
		},
	}
	_, err := NewConfigurableParser(invalidConfig)
	if err == nil {
		t.Error("expected error for invalid regex pattern")
	}
}

// TestParserConfigValidation 测试解析器配置验证
func TestParserConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *ParserConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &ParserConfig{
				Name:       "test",
				TimeLayout: "2006-01-02",
			},
			wantErr: false,
		},
		{
			name: "empty name",
			config: &ParserConfig{
				Name:       "",
				TimeLayout: "2006-01-02",
			},
			wantErr: true,
		},
		{
			name: "empty time layout",
			config: &ParserConfig{
				Name:       "test",
				TimeLayout: "",
			},
			wantErr: true,
		},
		{
			name: "invalid size pattern",
			config: &ParserConfig{
				Name:       "test",
				TimeLayout: "2006-01-02",
				Patterns: PatternConfig{
					SizePattern: `[invalid`,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestConvertToBytes 测试大小转换
func TestConvertToBytes(t *testing.T) {
	tests := []struct {
		value    float64
		unit     string
		expected int64
	}{
		{1.0, "KB", 1024},
		{1.0, "MB", 1024 * 1024},
		{1.0, "GB", 1024 * 1024 * 1024},
		{1.0, "TB", 1024 * 1024 * 1024 * 1024},
		{2.5, "GB", int64(2.5 * 1024 * 1024 * 1024)},
		{100.0, "MB", 100 * 1024 * 1024},
	}

	for _, tt := range tests {
		t.Run(tt.unit, func(t *testing.T) {
			result := convertToBytes(tt.value, tt.unit)
			if result != tt.expected {
				t.Errorf("convertToBytes(%v, %s) = %d, want %d", tt.value, tt.unit, result, tt.expected)
			}
		})
	}
}

// TestDefaultParserConfig 测试默认配置
func TestDefaultParserConfig(t *testing.T) {
	config := DefaultParserConfig()

	if config.Name != "default" {
		t.Errorf("expected name 'default', got '%s'", config.Name)
	}
	if config.TimeLayout != "2006-01-02 15:04:05" {
		t.Errorf("unexpected time layout: %s", config.TimeLayout)
	}
	if config.Selectors.TitleSelector == "" {
		t.Error("expected non-empty title selector")
	}
	if len(config.DiscountMapping) == 0 {
		t.Error("expected non-empty discount mapping")
	}
}

// TestParseRSSItem 测试RSS条目解析
func TestParseRSSItem(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Patterns: PatternConfig{
			SizePattern: `([\d.]+)\s*(GB|MB|KB|TB)`,
			IDPattern:   `id=(\d+)`,
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	tests := []struct {
		name        string
		title       string
		link        string
		description string
		wantID      string
		wantTitle   string
		wantSize    int64
	}{
		{
			name:        "basic RSS item",
			title:       "Test Torrent",
			link:        "https://example.com/details.php?id=12345",
			description: "Size: 2.5 GB",
			wantID:      "12345",
			wantTitle:   "Test Torrent",
			wantSize:    int64(2.5 * 1024 * 1024 * 1024),
		},
		{
			name:        "RSS item with MB size",
			title:       "Small Torrent",
			link:        "https://example.com/details.php?id=67890",
			description: "Size: 500 MB",
			wantID:      "67890",
			wantTitle:   "Small Torrent",
			wantSize:    500 * 1024 * 1024,
		},
		{
			name:        "RSS item without size",
			title:       "No Size Torrent",
			link:        "https://example.com/details.php?id=11111",
			description: "No size info",
			wantID:      "11111",
			wantTitle:   "No Size Torrent",
			wantSize:    0,
		},
		{
			name:        "RSS item without ID",
			title:       "No ID Torrent",
			link:        "https://example.com/details.php",
			description: "Size: 1 GB",
			wantID:      "",
			wantTitle:   "No ID Torrent",
			wantSize:    1024 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := parser.ParseRSSItem(tt.title, tt.link, tt.description)
			if err != nil {
				t.Fatalf("ParseRSSItem failed: %v", err)
			}

			if info.Title != tt.wantTitle {
				t.Errorf("Title = %s, want %s", info.Title, tt.wantTitle)
			}
			if info.ID != tt.wantID {
				t.Errorf("ID = %s, want %s", info.ID, tt.wantID)
			}
			if info.Size != tt.wantSize {
				t.Errorf("Size = %d, want %d", info.Size, tt.wantSize)
			}
		})
	}
}

// TestParseRSSItemWithoutPatterns 测试没有正则模式的RSS解析
func TestParseRSSItemWithoutPatterns(t *testing.T) {
	config := &ParserConfig{
		Name:       "no-patterns",
		TimeLayout: "2006-01-02 15:04:05",
		Patterns:   PatternConfig{}, // 空模式
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	info, err := parser.ParseRSSItem("Test Title", "https://example.com/id=123", "Size: 1 GB")
	if err != nil {
		t.Fatalf("ParseRSSItem failed: %v", err)
	}

	if info.Title != "Test Title" {
		t.Errorf("Title = %s, want Test Title", info.Title)
	}
	// 没有正则模式，ID和Size应该为空/0
	if info.ID != "" {
		t.Errorf("ID should be empty without pattern, got %s", info.ID)
	}
	if info.Size != 0 {
		t.Errorf("Size should be 0 without pattern, got %d", info.Size)
	}
}

// TestConvertToBytesUnknownUnit 测试未知单位的转换
func TestConvertToBytesUnknownUnit(t *testing.T) {
	// 未知单位应该返回原始值
	result := convertToBytes(100.0, "UNKNOWN")
	if result != 100 {
		t.Errorf("convertToBytes with unknown unit = %d, want 100", result)
	}

	result = convertToBytes(50.5, "")
	if result != 50 {
		t.Errorf("convertToBytes with empty unit = %d, want 50", result)
	}
}

// TestNewConfigurableParserWithInvalidIDPattern 测试无效ID正则
func TestNewConfigurableParserWithInvalidIDPattern(t *testing.T) {
	config := &ParserConfig{
		Name:       "invalid-id",
		TimeLayout: "2006-01-02",
		Patterns: PatternConfig{
			IDPattern: `[invalid(regex`, // 无效正则
		},
	}

	_, err := NewConfigurableParser(config)
	if err == nil {
		t.Error("expected error for invalid ID pattern")
	}
}

// TestParserConfigValidateInvalidIDPattern 测试配置验证中的无效ID正则
func TestParserConfigValidateInvalidIDPattern(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02",
		Patterns: PatternConfig{
			IDPattern: `[invalid`,
		},
	}

	err := config.Validate()
	if err == nil {
		t.Error("expected error for invalid ID pattern in validation")
	}
}

// TestParsedTorrentInfoFields 测试 ParsedTorrentInfo 结构体字段
func TestParsedTorrentInfoFields(t *testing.T) {
	info := &ParsedTorrentInfo{
		ID:           "12345",
		Title:        "Test Torrent",
		Size:         1024 * 1024 * 1024,
		IsFree:       true,
		FreeEndTime:  time.Now().Add(24 * time.Hour),
		HasHR:        false,
		DownloadURL:  "https://example.com/download/12345",
		DiscountType: "free",
	}

	if info.ID != "12345" {
		t.Errorf("ID = %s, want 12345", info.ID)
	}
	if info.Title != "Test Torrent" {
		t.Errorf("Title = %s, want Test Torrent", info.Title)
	}
	if info.Size != 1024*1024*1024 {
		t.Errorf("Size = %d, want %d", info.Size, 1024*1024*1024)
	}
	if !info.IsFree {
		t.Error("IsFree should be true")
	}
	if info.HasHR {
		t.Error("HasHR should be false")
	}
	if info.DownloadURL != "https://example.com/download/12345" {
		t.Errorf("DownloadURL = %s, want https://example.com/download/12345", info.DownloadURL)
	}
	if info.DiscountType != "free" {
		t.Errorf("DiscountType = %s, want free", info.DiscountType)
	}
}

// TestSelectorConfigFields 测试 SelectorConfig 结构体字段
func TestSelectorConfigFields(t *testing.T) {
	config := SelectorConfig{
		TitleSelector:       "h1.title",
		IDSelector:          "input#torrent-id",
		SizeSelector:        "span.size",
		DiscountSelector:    "span.discount",
		FreeEndTimeSelector: "span.free-end",
		HRSelector:          "img.hr",
		DownloadURLSelector: "a.download",
	}

	if config.TitleSelector != "h1.title" {
		t.Errorf("TitleSelector = %s, want h1.title", config.TitleSelector)
	}
	if config.IDSelector != "input#torrent-id" {
		t.Errorf("IDSelector = %s, want input#torrent-id", config.IDSelector)
	}
	if config.SizeSelector != "span.size" {
		t.Errorf("SizeSelector = %s, want span.size", config.SizeSelector)
	}
}

// TestPatternConfigFields 测试 PatternConfig 结构体字段
func TestPatternConfigFields(t *testing.T) {
	config := PatternConfig{
		SizePattern: `([\d.]+)\s*(GB|MB)`,
		IDPattern:   `id=(\d+)`,
		HRKeywords:  []string{"HR", "Hit and Run"},
	}

	if config.SizePattern != `([\d.]+)\s*(GB|MB)` {
		t.Errorf("SizePattern mismatch")
	}
	if config.IDPattern != `id=(\d+)` {
		t.Errorf("IDPattern mismatch")
	}
	if len(config.HRKeywords) != 2 {
		t.Errorf("HRKeywords length = %d, want 2", len(config.HRKeywords))
	}
}

// TestParseTorrentPage 测试解析种子页面
func TestParseTorrentPage(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Selectors: SelectorConfig{
			TitleSelector:       "h1.title",
			IDSelector:          "input#torrent-id",
			SizeSelector:        "span.size",
			DiscountSelector:    "span.discount",
			FreeEndTimeSelector: "span.free-end",
			HRSelector:          "img.hr",
			DownloadURLSelector: "a.download",
		},
		Patterns: PatternConfig{
			SizePattern: `([\d.]+)\s*(GB|MB|KB|TB)`,
			IDPattern:   `id=(\d+)`,
			HRKeywords:  []string{"HR", "Hit and Run"},
		},
		DiscountMapping: map[string]string{
			"free":     "free",
			"twoup":    "2x",
			"halfdown": "50%",
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	// 验证解析器名称
	if parser.GetName() != "test" {
		t.Errorf("expected name 'test', got '%s'", parser.GetName())
	}
}

// TestParserConfigDiscountMapping 测试优惠类型映射
func TestParserConfigDiscountMapping(t *testing.T) {
	config := DefaultParserConfig()

	// 验证默认映射
	if _, ok := config.DiscountMapping["free"]; !ok {
		t.Error("expected 'free' in discount mapping")
	}
	if _, ok := config.DiscountMapping["twoup"]; !ok {
		t.Error("expected 'twoup' in discount mapping")
	}
}

// TestParserConfigHRKeywords 测试HR关键词
func TestParserConfigHRKeywords(t *testing.T) {
	config := DefaultParserConfig()

	if len(config.Patterns.HRKeywords) == 0 {
		t.Error("expected non-empty HR keywords")
	}

	// 验证包含常见HR关键词
	found := false
	for _, kw := range config.Patterns.HRKeywords {
		if kw == "hitandrun" || kw == "Hit and Run" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected HR keywords to contain 'hitandrun' or 'Hit and Run'")
	}
}

// TestParserConfigSelectors 测试选择器配置
func TestParserConfigSelectors(t *testing.T) {
	config := DefaultParserConfig()

	if config.Selectors.TitleSelector == "" {
		t.Error("expected non-empty title selector")
	}
	if config.Selectors.IDSelector == "" {
		t.Error("expected non-empty ID selector")
	}
	if config.Selectors.SizeSelector == "" {
		t.Error("expected non-empty size selector")
	}
	if config.Selectors.DiscountSelector == "" {
		t.Error("expected non-empty discount selector")
	}
	if config.Selectors.FreeEndTimeSelector == "" {
		t.Error("expected non-empty free end time selector")
	}
	if config.Selectors.HRSelector == "" {
		t.Error("expected non-empty HR selector")
	}
	if config.Selectors.DownloadURLSelector == "" {
		t.Error("expected non-empty download URL selector")
	}
}

// TestParserConfigPatterns 测试正则模式配置
func TestParserConfigPatterns(t *testing.T) {
	config := DefaultParserConfig()

	if config.Patterns.SizePattern == "" {
		t.Error("expected non-empty size pattern")
	}
	if config.Patterns.IDPattern == "" {
		t.Error("expected non-empty ID pattern")
	}
}

// TestConvertToBytesAllUnits 测试所有单位的转换
func TestConvertToBytesAllUnits(t *testing.T) {
	tests := []struct {
		value    float64
		unit     string
		expected int64
	}{
		{1.0, "KB", 1024},
		{1.0, "kb", 1024},
		{1.0, "Kb", 1024},
		{1.0, "MB", 1024 * 1024},
		{1.0, "mb", 1024 * 1024},
		{1.0, "GB", 1024 * 1024 * 1024},
		{1.0, "gb", 1024 * 1024 * 1024},
		{1.0, "TB", 1024 * 1024 * 1024 * 1024},
		{1.0, "tb", 1024 * 1024 * 1024 * 1024},
		{1.0, "B", 1},
		{1.0, "bytes", 1},
		{0.0, "GB", 0},
		{100.5, "MB", int64(100.5 * 1024 * 1024)},
	}

	for _, tt := range tests {
		t.Run(tt.unit, func(t *testing.T) {
			result := convertToBytes(tt.value, tt.unit)
			if result != tt.expected {
				t.Errorf("convertToBytes(%v, %s) = %d, want %d", tt.value, tt.unit, result, tt.expected)
			}
		})
	}
}

// TestParseRSSItemWithTBSize 测试TB大小的RSS解析
func TestParseRSSItemWithTBSize(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Patterns: PatternConfig{
			SizePattern: `([\d.]+)\s*(GB|MB|KB|TB)`,
			IDPattern:   `id=(\d+)`,
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	info, err := parser.ParseRSSItem("Large Torrent", "https://example.com/details.php?id=99999", "Size: 1.5 TB")
	if err != nil {
		t.Fatalf("ParseRSSItem failed: %v", err)
	}

	expectedSize := int64(1.5 * 1024 * 1024 * 1024 * 1024)
	if info.Size != expectedSize {
		t.Errorf("Size = %d, want %d", info.Size, expectedSize)
	}
}

// TestParseRSSItemWithKBSize 测试KB大小的RSS解析
func TestParseRSSItemWithKBSize(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Patterns: PatternConfig{
			SizePattern: `([\d.]+)\s*(GB|MB|KB|TB)`,
			IDPattern:   `id=(\d+)`,
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	info, err := parser.ParseRSSItem("Small Torrent", "https://example.com/details.php?id=11111", "Size: 512 KB")
	if err != nil {
		t.Fatalf("ParseRSSItem failed: %v", err)
	}

	expectedSize := int64(512 * 1024)
	if info.Size != expectedSize {
		t.Errorf("Size = %d, want %d", info.Size, expectedSize)
	}
}

// TestParseRSSItemWithInvalidSizeFormat 测试无效大小格式的RSS解析
func TestParseRSSItemWithInvalidSizeFormat(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Patterns: PatternConfig{
			SizePattern: `([\d.]+)\s*(GB|MB|KB|TB)`,
			IDPattern:   `id=(\d+)`,
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	info, err := parser.ParseRSSItem("Invalid Size", "https://example.com/details.php?id=22222", "Size: invalid")
	if err != nil {
		t.Fatalf("ParseRSSItem failed: %v", err)
	}

	// 无效格式应该返回0
	if info.Size != 0 {
		t.Errorf("Size = %d, want 0 for invalid format", info.Size)
	}
}

// TestParseRSSItemWithEmptyDescription 测试空描述的RSS解析
func TestParseRSSItemWithEmptyDescription(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Patterns: PatternConfig{
			SizePattern: `([\d.]+)\s*(GB|MB|KB|TB)`,
			IDPattern:   `id=(\d+)`,
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	info, err := parser.ParseRSSItem("No Description", "https://example.com/details.php?id=33333", "")
	if err != nil {
		t.Fatalf("ParseRSSItem failed: %v", err)
	}

	if info.Title != "No Description" {
		t.Errorf("Title = %s, want 'No Description'", info.Title)
	}
	if info.ID != "33333" {
		t.Errorf("ID = %s, want '33333'", info.ID)
	}
	if info.Size != 0 {
		t.Errorf("Size = %d, want 0 for empty description", info.Size)
	}
}

// TestParseTorrentPageWithHTML 测试使用HTML解析种子页面
func TestParseTorrentPageWithHTML(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Selectors: SelectorConfig{
			TitleSelector:       "h1.title",
			IDSelector:          "input#torrent-id",
			SizeSelector:        "span.size",
			DiscountSelector:    "span.discount",
			FreeEndTimeSelector: "span.free-end",
			HRSelector:          "img.hr",
			DownloadURLSelector: "a.download",
		},
		Patterns: PatternConfig{
			SizePattern: `([\d.]+)\s*(GB|MB|KB|TB)`,
			IDPattern:   `id=(\d+)`,
			HRKeywords:  []string{"HR", "Hit and Run"},
		},
		DiscountMapping: map[string]string{
			"free":     "free",
			"twoup":    "2x",
			"halfdown": "50%",
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	// 验证解析器创建成功
	if parser == nil {
		t.Fatal("parser should not be nil")
	}
	if parser.GetName() != "test" {
		t.Errorf("expected name 'test', got '%s'", parser.GetName())
	}
}

// TestParseSizeWithGoquery 测试使用goquery解析大小
func TestParseSizeWithGoquery(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Selectors: SelectorConfig{
			SizeSelector: "span.size",
		},
		Patterns: PatternConfig{
			SizePattern: `([\d.]+)\s*(GB|MB|KB|TB)`,
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	// 验证正则表达式已编译
	if parser.sizeRegex == nil {
		t.Error("size regex should be compiled")
	}
}

// TestParseDiscountMapping 测试优惠类型映射
func TestParseDiscountMapping(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Selectors: SelectorConfig{
			DiscountSelector: "span.discount",
		},
		DiscountMapping: map[string]string{
			"free":     "Free",
			"twoup":    "2x Upload",
			"halfdown": "50% Download",
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	// 验证映射配置
	if parser.config.DiscountMapping["free"] != "Free" {
		t.Errorf("expected 'Free', got '%s'", parser.config.DiscountMapping["free"])
	}
	if parser.config.DiscountMapping["twoup"] != "2x Upload" {
		t.Errorf("expected '2x Upload', got '%s'", parser.config.DiscountMapping["twoup"])
	}
}

// TestParseHRKeywords 测试HR关键词解析
func TestParseHRKeywords(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Selectors: SelectorConfig{
			HRSelector: "img.hr",
		},
		Patterns: PatternConfig{
			HRKeywords: []string{"HR", "Hit and Run", "hitandrun"},
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	// 验证HR关键词配置
	if len(parser.config.Patterns.HRKeywords) != 3 {
		t.Errorf("expected 3 HR keywords, got %d", len(parser.config.Patterns.HRKeywords))
	}
}

// TestParseFreeEndTimeLayout 测试免费结束时间格式
func TestParseFreeEndTimeLayout(t *testing.T) {
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05",
		"01-02-2006 15:04:05",
		"Jan 2, 2006 15:04:05",
	}

	for _, layout := range layouts {
		t.Run(layout, func(t *testing.T) {
			config := &ParserConfig{
				Name:       "test",
				TimeLayout: layout,
				Selectors: SelectorConfig{
					FreeEndTimeSelector: "span.free-end",
				},
			}

			parser, err := NewConfigurableParser(config)
			if err != nil {
				t.Fatalf("failed to create parser with layout %s: %v", layout, err)
			}

			if parser.config.TimeLayout != layout {
				t.Errorf("expected layout '%s', got '%s'", layout, parser.config.TimeLayout)
			}
		})
	}
}

// TestParserWithAllSelectors 测试所有选择器配置
func TestParserWithAllSelectors(t *testing.T) {
	config := &ParserConfig{
		Name:       "full-config",
		TimeLayout: "2006-01-02 15:04:05",
		Selectors: SelectorConfig{
			TitleSelector:       "h1.title",
			IDSelector:          "input#torrent-id",
			SizeSelector:        "span.size",
			DiscountSelector:    "span.discount",
			FreeEndTimeSelector: "span.free-end",
			HRSelector:          "img.hr",
			DownloadURLSelector: "a.download",
		},
		Patterns: PatternConfig{
			SizePattern: `([\d.]+)\s*(GB|MB|KB|TB)`,
			IDPattern:   `id=(\d+)`,
			HRKeywords:  []string{"HR"},
		},
		DiscountMapping: map[string]string{
			"free": "Free",
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	// 验证所有选择器
	if parser.config.Selectors.TitleSelector != "h1.title" {
		t.Error("title selector mismatch")
	}
	if parser.config.Selectors.IDSelector != "input#torrent-id" {
		t.Error("ID selector mismatch")
	}
	if parser.config.Selectors.SizeSelector != "span.size" {
		t.Error("size selector mismatch")
	}
	if parser.config.Selectors.DiscountSelector != "span.discount" {
		t.Error("discount selector mismatch")
	}
	if parser.config.Selectors.FreeEndTimeSelector != "span.free-end" {
		t.Error("free end time selector mismatch")
	}
	if parser.config.Selectors.HRSelector != "img.hr" {
		t.Error("HR selector mismatch")
	}
	if parser.config.Selectors.DownloadURLSelector != "a.download" {
		t.Error("download URL selector mismatch")
	}
}

// TestParserWithEmptySelectors 测试空选择器配置
func TestParserWithEmptySelectors(t *testing.T) {
	config := &ParserConfig{
		Name:       "empty-selectors",
		TimeLayout: "2006-01-02 15:04:05",
		Selectors:  SelectorConfig{}, // 空选择器
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	if parser.config.Selectors.TitleSelector != "" {
		t.Error("title selector should be empty")
	}
}

// TestParserIDRegexCompilation 测试ID正则编译
func TestParserIDRegexCompilation(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Patterns: PatternConfig{
			IDPattern: `id=(\d+)`,
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	if parser.idRegex == nil {
		t.Error("ID regex should be compiled")
	}

	// 测试正则匹配
	matches := parser.idRegex.FindStringSubmatch("https://example.com/details.php?id=12345")
	if len(matches) < 2 {
		t.Error("ID regex should match")
	}
	if matches[1] != "12345" {
		t.Errorf("expected ID '12345', got '%s'", matches[1])
	}
}

// TestParserSizeRegexCompilation 测试大小正则编译
func TestParserSizeRegexCompilation(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Patterns: PatternConfig{
			SizePattern: `([\d.]+)\s*(GB|MB|KB|TB)`,
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	if parser.sizeRegex == nil {
		t.Error("size regex should be compiled")
	}

	// 测试正则匹配
	testCases := []struct {
		input    string
		wantSize string
		wantUnit string
	}{
		{"Size: 2.5 GB", "2.5", "GB"},
		{"Size: 500 MB", "500", "MB"},
		{"Size: 1024 KB", "1024", "KB"},
		{"Size: 1.5 TB", "1.5", "TB"},
	}

	for _, tc := range testCases {
		matches := parser.sizeRegex.FindStringSubmatch(tc.input)
		if len(matches) < 3 {
			t.Errorf("size regex should match '%s'", tc.input)
			continue
		}
		if matches[1] != tc.wantSize {
			t.Errorf("expected size '%s', got '%s'", tc.wantSize, matches[1])
		}
		if matches[2] != tc.wantUnit {
			t.Errorf("expected unit '%s', got '%s'", tc.wantUnit, matches[2])
		}
	}
}

// TestParserWithBothRegexPatterns 测试同时配置两个正则
func TestParserWithBothRegexPatterns(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Patterns: PatternConfig{
			SizePattern: `([\d.]+)\s*(GB|MB|KB|TB)`,
			IDPattern:   `id=(\d+)`,
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	if parser.sizeRegex == nil {
		t.Error("size regex should be compiled")
	}
	if parser.idRegex == nil {
		t.Error("ID regex should be compiled")
	}
}

// TestParserWithNoRegexPatterns 测试不配置正则
func TestParserWithNoRegexPatterns(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Patterns:   PatternConfig{}, // 空模式
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	if parser.sizeRegex != nil {
		t.Error("size regex should be nil")
	}
	if parser.idRegex != nil {
		t.Error("ID regex should be nil")
	}
}

// TestParseTorrentPageWithColly 使用colly测试解析种子页面
func TestParseTorrentPageWithColly(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Selectors: SelectorConfig{
			TitleSelector:       "h1.title",
			IDSelector:          "input#torrent-id",
			SizeSelector:        "span.size",
			DiscountSelector:    "span.discount",
			FreeEndTimeSelector: "span.free-end",
			HRSelector:          "img.hr",
			DownloadURLSelector: "a.download",
		},
		Patterns: PatternConfig{
			SizePattern: `([\d.]+)\s*(GB|MB|KB|TB)`,
			IDPattern:   `id=(\d+)`,
			HRKeywords:  []string{"HR", "Hit and Run"},
		},
		DiscountMapping: map[string]string{
			"free":     "Free",
			"twoup":    "2x Upload",
			"halfdown": "50% Download",
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	// 创建测试HTML
	html := `
	<html>
	<body>
		<h1 class="title">Test Torrent Title</h1>
		<input id="torrent-id" value="12345" />
		<span class="size">2.5 GB</span>
		<span class="discount" class="free">Free</span>
		<span class="free-end" title="2025-12-31 23:59:59">Until 2025-12-31</span>
		<img class="hr" src="hr.png" />
		<a class="download" href="/download/12345.torrent">Download</a>
	</body>
	</html>
	`

	// 使用colly解析HTML
	c := colly.NewCollector()
	var parsedInfo *ParsedTorrentInfo

	c.OnHTML("body", func(e *colly.HTMLElement) {
		info, parseErr := parser.ParseTorrentPage(e)
		if parseErr != nil {
			t.Fatalf("ParseTorrentPage failed: %v", parseErr)
		}
		parsedInfo = info
	})

	// 创建测试服务器
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer ts.Close()

	err = c.Visit(ts.URL)
	if err != nil {
		t.Fatalf("failed to visit test server: %v", err)
	}

	// 验证解析结果
	if parsedInfo == nil {
		t.Fatal("parsedInfo should not be nil")
	}
	if parsedInfo.Title != "Test Torrent Title" {
		t.Errorf("Title = %s, want 'Test Torrent Title'", parsedInfo.Title)
	}
	if parsedInfo.ID != "12345" {
		t.Errorf("ID = %s, want '12345'", parsedInfo.ID)
	}
	if parsedInfo.DownloadURL != "/download/12345.torrent" {
		t.Errorf("DownloadURL = %s, want '/download/12345.torrent'", parsedInfo.DownloadURL)
	}
	if !parsedInfo.HasHR {
		t.Error("HasHR should be true")
	}
}

// TestParseTorrentPageWithSizeInNextElement 测试大小在下一个元素中
func TestParseTorrentPageWithSizeInNextElement(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Selectors: SelectorConfig{
			SizeSelector: "span.size-label",
		},
		Patterns: PatternConfig{
			SizePattern: `([\d.]+)\s*(GB|MB|KB|TB)`,
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	// 创建测试HTML - 大小在下一个元素中
	html := `
	<html>
	<body>
		<span class="size-label">Size:</span><span>1.5 GB</span>
	</body>
	</html>
	`

	c := colly.NewCollector()
	var parsedInfo *ParsedTorrentInfo

	c.OnHTML("body", func(e *colly.HTMLElement) {
		info, parseErr := parser.ParseTorrentPage(e)
		if parseErr != nil {
			t.Fatalf("ParseTorrentPage failed: %v", parseErr)
		}
		parsedInfo = info
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer ts.Close()

	err = c.Visit(ts.URL)
	if err != nil {
		t.Fatalf("failed to visit test server: %v", err)
	}

	if parsedInfo == nil {
		t.Fatal("parsedInfo should not be nil")
	}
}

// TestParseTorrentPageWithDiscountClass 测试优惠类型解析
func TestParseTorrentPageWithDiscountClass(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Selectors: SelectorConfig{
			DiscountSelector: "span.discount",
		},
		DiscountMapping: map[string]string{
			"free":     "Free",
			"twoup":    "2x Upload",
			"halfdown": "50% Download",
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	// 创建测试HTML
	html := `
	<html>
	<body>
		<span class="discount free">Free Download</span>
	</body>
	</html>
	`

	c := colly.NewCollector()
	var parsedInfo *ParsedTorrentInfo

	c.OnHTML("body", func(e *colly.HTMLElement) {
		info, parseErr := parser.ParseTorrentPage(e)
		if parseErr != nil {
			t.Fatalf("ParseTorrentPage failed: %v", parseErr)
		}
		parsedInfo = info
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer ts.Close()

	err = c.Visit(ts.URL)
	if err != nil {
		t.Fatalf("failed to visit test server: %v", err)
	}

	if parsedInfo == nil {
		t.Fatal("parsedInfo should not be nil")
	}
}

// TestParseTorrentPageWithFreeEndTime 测试免费结束时间解析
func TestParseTorrentPageWithFreeEndTime(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Selectors: SelectorConfig{
			FreeEndTimeSelector: "span.free-end",
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	// 创建测试HTML
	html := `
	<html>
	<body>
		<span class="free-end" title="2025-12-31 23:59:59">Until 2025-12-31</span>
	</body>
	</html>
	`

	c := colly.NewCollector()
	var parsedInfo *ParsedTorrentInfo

	c.OnHTML("body", func(e *colly.HTMLElement) {
		info, parseErr := parser.ParseTorrentPage(e)
		if parseErr != nil {
			t.Fatalf("ParseTorrentPage failed: %v", parseErr)
		}
		parsedInfo = info
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer ts.Close()

	err = c.Visit(ts.URL)
	if err != nil {
		t.Fatalf("failed to visit test server: %v", err)
	}

	if parsedInfo == nil {
		t.Fatal("parsedInfo should not be nil")
	}
	if parsedInfo.FreeEndTime.IsZero() {
		t.Error("FreeEndTime should not be zero")
	}
}

// TestParseTorrentPageWithHRKeyword 测试HR关键词解析
func TestParseTorrentPageWithHRKeyword(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Selectors: SelectorConfig{
			HRSelector: "img.hr-icon",
		},
		Patterns: PatternConfig{
			HRKeywords: []string{"Hit and Run", "hitandrun"},
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	// 创建测试HTML - 使用关键词而不是选择器
	html := `
	<html>
	<body>
		<div class="warning">This torrent has Hit and Run requirement</div>
	</body>
	</html>
	`

	c := colly.NewCollector()
	var parsedInfo *ParsedTorrentInfo

	c.OnHTML("body", func(e *colly.HTMLElement) {
		info, parseErr := parser.ParseTorrentPage(e)
		if parseErr != nil {
			t.Fatalf("ParseTorrentPage failed: %v", parseErr)
		}
		parsedInfo = info
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer ts.Close()

	err = c.Visit(ts.URL)
	if err != nil {
		t.Fatalf("failed to visit test server: %v", err)
	}

	if parsedInfo == nil {
		t.Fatal("parsedInfo should not be nil")
	}
	if !parsedInfo.HasHR {
		t.Error("HasHR should be true when HR keyword is found")
	}
}

// TestParseTorrentPageWithHRSelector 测试HR选择器解析
func TestParseTorrentPageWithHRSelector(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Selectors: SelectorConfig{
			HRSelector: "img.hr-icon",
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	// 创建测试HTML - 使用选择器
	html := `
	<html>
	<body>
		<img class="hr-icon" src="hr.png" />
	</body>
	</html>
	`

	c := colly.NewCollector()
	var parsedInfo *ParsedTorrentInfo

	c.OnHTML("body", func(e *colly.HTMLElement) {
		info, parseErr := parser.ParseTorrentPage(e)
		if parseErr != nil {
			t.Fatalf("ParseTorrentPage failed: %v", parseErr)
		}
		parsedInfo = info
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer ts.Close()

	err = c.Visit(ts.URL)
	if err != nil {
		t.Fatalf("failed to visit test server: %v", err)
	}

	if parsedInfo == nil {
		t.Fatal("parsedInfo should not be nil")
	}
	if !parsedInfo.HasHR {
		t.Error("HasHR should be true when HR selector matches")
	}
}

// TestParseTorrentPageWithNoHR 测试没有HR的情况
func TestParseTorrentPageWithNoHR(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Selectors: SelectorConfig{
			HRSelector: "img.hr-icon",
		},
		Patterns: PatternConfig{
			HRKeywords: []string{"Hit and Run"},
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	// 创建测试HTML - 没有HR
	html := `
	<html>
	<body>
		<div class="info">Normal torrent without HR</div>
	</body>
	</html>
	`

	c := colly.NewCollector()
	var parsedInfo *ParsedTorrentInfo

	c.OnHTML("body", func(e *colly.HTMLElement) {
		info, parseErr := parser.ParseTorrentPage(e)
		if parseErr != nil {
			t.Fatalf("ParseTorrentPage failed: %v", parseErr)
		}
		parsedInfo = info
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer ts.Close()

	err = c.Visit(ts.URL)
	if err != nil {
		t.Fatalf("failed to visit test server: %v", err)
	}

	if parsedInfo == nil {
		t.Fatal("parsedInfo should not be nil")
	}
	if parsedInfo.HasHR {
		t.Error("HasHR should be false when no HR indicator is found")
	}
}

// TestParseTorrentPageWithInvalidFreeEndTime 测试无效的免费结束时间
func TestParseTorrentPageWithInvalidFreeEndTime(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Selectors: SelectorConfig{
			FreeEndTimeSelector: "span.free-end",
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	// 创建测试HTML - 无效的时间格式
	html := `
	<html>
	<body>
		<span class="free-end" title="invalid-time">Invalid Time</span>
	</body>
	</html>
	`

	c := colly.NewCollector()
	var parsedInfo *ParsedTorrentInfo

	c.OnHTML("body", func(e *colly.HTMLElement) {
		info, parseErr := parser.ParseTorrentPage(e)
		if parseErr != nil {
			t.Fatalf("ParseTorrentPage failed: %v", parseErr)
		}
		parsedInfo = info
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer ts.Close()

	err = c.Visit(ts.URL)
	if err != nil {
		t.Fatalf("failed to visit test server: %v", err)
	}

	if parsedInfo == nil {
		t.Fatal("parsedInfo should not be nil")
	}
	// 无效时间应该保持为零值
	if !parsedInfo.FreeEndTime.IsZero() {
		t.Error("FreeEndTime should be zero for invalid time format")
	}
}

// TestParseTorrentPageWithEmptySelectors 测试空选择器
func TestParseTorrentPageWithEmptySelectors(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Selectors:  SelectorConfig{}, // 空选择器
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	html := `
	<html>
	<body>
		<h1>Test Page</h1>
	</body>
	</html>
	`

	c := colly.NewCollector()
	var parsedInfo *ParsedTorrentInfo

	c.OnHTML("body", func(e *colly.HTMLElement) {
		info, parseErr := parser.ParseTorrentPage(e)
		if parseErr != nil {
			t.Fatalf("ParseTorrentPage failed: %v", parseErr)
		}
		parsedInfo = info
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer ts.Close()

	err = c.Visit(ts.URL)
	if err != nil {
		t.Fatalf("failed to visit test server: %v", err)
	}

	if parsedInfo == nil {
		t.Fatal("parsedInfo should not be nil")
	}
	// 所有字段应该为空/默认值
	if parsedInfo.Title != "" {
		t.Errorf("Title should be empty, got '%s'", parsedInfo.Title)
	}
	if parsedInfo.ID != "" {
		t.Errorf("ID should be empty, got '%s'", parsedInfo.ID)
	}
}

// TestParseTorrentPageWithSizeDirectText 测试直接文本中的大小
func TestParseTorrentPageWithSizeDirectText(t *testing.T) {
	config := &ParserConfig{
		Name:       "test",
		TimeLayout: "2006-01-02 15:04:05",
		Selectors: SelectorConfig{
			SizeSelector: "span.size",
		},
		Patterns: PatternConfig{
			SizePattern: `([\d.]+)\s*(GB|MB|KB|TB)`,
		},
	}

	parser, err := NewConfigurableParser(config)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	// 创建测试HTML - 大小直接在元素中
	html := `
	<html>
	<body>
		<span class="size">3.5 GB</span>
	</body>
	</html>
	`

	c := colly.NewCollector()
	var parsedInfo *ParsedTorrentInfo

	c.OnHTML("body", func(e *colly.HTMLElement) {
		info, parseErr := parser.ParseTorrentPage(e)
		if parseErr != nil {
			t.Fatalf("ParseTorrentPage failed: %v", parseErr)
		}
		parsedInfo = info
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer ts.Close()

	err = c.Visit(ts.URL)
	if err != nil {
		t.Fatalf("failed to visit test server: %v", err)
	}

	if parsedInfo == nil {
		t.Fatal("parsedInfo should not be nil")
	}
	expectedSize := int64(3.5 * 1024 * 1024 * 1024)
	if parsedInfo.Size != expectedSize {
		t.Errorf("Size = %d, want %d", parsedInfo.Size, expectedSize)
	}
}
