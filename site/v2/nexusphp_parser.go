package v2

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
)

// NexusPHPParserConfig 解析器配置
type NexusPHPParserConfig struct {
	TimeLayout       string
	DiscountMapping  map[string]DiscountLevel
	HRKeywords       []string
	TitleSelector    string
	IDSelector       string
	DiscountSelector string
	EndTimeSelector  string
	SizeSelector     string
	SizeRegex        string
}

// DefaultNexusPHPParserConfig 返回默认配置，适用于大多数 NexusPHP 站点
func DefaultNexusPHPParserConfig() NexusPHPParserConfig {
	detailConfig := DefaultDetailParserConfig()
	return NexusPHPParserConfig{
		TimeLayout:       detailConfig.TimeLayout,
		DiscountMapping:  detailConfig.DiscountMapping,
		HRKeywords:       []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run"},
		TitleSelector:    "input[name='torrent_name']",
		IDSelector:       "input[name='detail_torrent_id']",
		DiscountSelector: "h1 font",
		EndTimeSelector:  "h1 span[title]",
		SizeSelector:     "td.rowhead:contains('基本信息')",
		SizeRegex:        `大小：[^\d]*([\d.]+)\s*(GB|MB|KB|TB)`,
	}
}

type NexusPHPParserOption func(*NexusPHPParserConfig)

func WithTimeLayout(layout string) NexusPHPParserOption {
	return func(cfg *NexusPHPParserConfig) {
		cfg.TimeLayout = layout
	}
}

func WithDiscountMapping(mapping map[string]DiscountLevel) NexusPHPParserOption {
	return func(cfg *NexusPHPParserConfig) {
		cfg.DiscountMapping = mapping
	}
}

func WithHRKeywords(keywords []string) NexusPHPParserOption {
	return func(cfg *NexusPHPParserConfig) {
		cfg.HRKeywords = keywords
	}
}

// TorrentDetailInfo 解析后的种子详情
type TorrentDetailInfo struct {
	TorrentID     string
	Title         string
	SizeMB        float64
	DiscountLevel DiscountLevel
	DiscountEnd   time.Time
	HasHR         bool
}

// NexusPHPDetailParser 接口定义
type NexusPHPDetailParser interface {
	ParseTitleAndID(doc *goquery.Selection) (title, torrentID string)
	ParseDiscount(doc *goquery.Selection) (DiscountLevel, time.Time)
	ParseHR(doc *goquery.Selection) bool
	ParseSizeMB(doc *goquery.Selection) float64
	ParseAll(doc *goquery.Selection) *TorrentDetailInfo
}

// NexusPHPParser 通用 NexusPHP 详情页解析器
type NexusPHPParser struct {
	config    NexusPHPParserConfig
	sizeRegex *regexp.Regexp
}

// NewNexusPHPParser 创建通用解析器
func NewNexusPHPParser(options ...NexusPHPParserOption) *NexusPHPParser {
	config := DefaultNexusPHPParserConfig()
	for _, opt := range options {
		opt(&config)
	}
	return &NexusPHPParser{
		config:    config,
		sizeRegex: regexp.MustCompile(config.SizeRegex),
	}
}

// NewNexusPHPParserFromDefinition creates a parser from SiteDefinition
// Falls back to default config if def or def.DetailParser is nil
func NewNexusPHPParserFromDefinition(def *SiteDefinition) *NexusPHPParser {
	if def == nil || def.DetailParser == nil {
		return NewNexusPHPParser()
	}

	dp := def.DetailParser
	config := DefaultNexusPHPParserConfig()

	if dp.TimeLayout != "" {
		config.TimeLayout = dp.TimeLayout
	}
	if dp.DiscountMapping != nil {
		config.DiscountMapping = dp.DiscountMapping
	}
	if len(dp.HRKeywords) > 0 {
		config.HRKeywords = dp.HRKeywords
	}
	if dp.TitleSelector != "" {
		config.TitleSelector = dp.TitleSelector
	}
	if dp.IDSelector != "" {
		config.IDSelector = dp.IDSelector
	}
	if dp.DiscountSelector != "" {
		config.DiscountSelector = dp.DiscountSelector
	}
	if dp.EndTimeSelector != "" {
		config.EndTimeSelector = dp.EndTimeSelector
	}
	if dp.SizeSelector != "" {
		config.SizeSelector = dp.SizeSelector
	}
	if dp.SizeRegex != "" {
		config.SizeRegex = dp.SizeRegex
	}

	return &NexusPHPParser{
		config:    config,
		sizeRegex: regexp.MustCompile(config.SizeRegex),
	}
}

func (p *NexusPHPParser) ParseTitleAndID(doc *goquery.Selection) (title, torrentID string) {
	title = valueOrText(doc.Find(p.config.TitleSelector), p.config.DiscountMapping)
	torrentID = valueOrText(doc.Find(p.config.IDSelector), nil)
	return title, torrentID
}

func valueOrText(s *goquery.Selection, discountMapping map[string]DiscountLevel) string {
	if value, exists := s.Attr("value"); exists {
		return value
	}

	text := strings.TrimSpace(s.Text())
	if discountMapping == nil {
		return text
	}

	return stripTrailingPromotion(text, discountMapping)
}

func stripTrailingPromotion(text string, discountMapping map[string]DiscountLevel) string {
	runes := []rune(text)
	for start := 0; start < len(runes); {
		if !unicode.IsSpace(runes[start]) {
			start++
			continue
		}

		end := start
		for end < len(runes) && unicode.IsSpace(runes[end]) {
			end++
		}
		suffix := strings.TrimSpace(string(runes[end:]))
		if strings.HasPrefix(suffix, "[") && strings.HasSuffix(suffix, "]") &&
			isKnownPromotionLabel(suffix[1:len(suffix)-1], discountMapping) {
			return strings.TrimSpace(string(runes[:start]))
		}
		start = end
	}

	return text
}

func isKnownPromotionLabel(label string, discountMapping map[string]DiscountLevel) bool {
	normalized := normalizePromotionLabel(label)
	for key, level := range discountMapping {
		if level == DiscountNone {
			continue
		}
		if normalized == normalizePromotionLabel(key) {
			return true
		}
		for _, alias := range renderedPromotionAliases(level) {
			if normalized == normalizePromotionLabel(alias) {
				return true
			}
		}
	}
	return false
}

func normalizePromotionLabel(label string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, label)
}

// renderedPromotionAliases maps configured discount semantics to their common visible labels.
// CSS class names remain sourced from DiscountMapping; these aliases cover rendered human text.
func renderedPromotionAliases(level DiscountLevel) []string {
	switch level {
	case DiscountFree:
		return []string{"免费", "free"}
	case Discount2xUp:
		return []string{"2x", "2xup"}
	case Discount2xFree:
		return []string{"2x免费", "2xfree"}
	case DiscountPercent50:
		return []string{"50%"}
	case DiscountPercent30:
		return []string{"30%"}
	case Discount2x50:
		return []string{"2x50%", "2x50"}
	default:
		return nil
	}
}

func (p *NexusPHPParser) ParseDiscount(doc *goquery.Selection) (DiscountLevel, time.Time) {
	discount := DiscountNone
	doc.Find(p.config.DiscountSelector).EachWithBreak(func(_ int, el *goquery.Selection) bool {
		class, exists := el.Attr("class")
		if exists {
			// Handle multiple classes and whitespace: class="free highlight" or class="free "
			for _, cls := range strings.Fields(class) {
				if level, ok := p.config.DiscountMapping[cls]; ok {
					discount = level
					return false
				}
			}
		}
		return true
	})

	var endTime time.Time
	if attr := doc.Find(p.config.EndTimeSelector).First().AttrOr("title", ""); attr != "" {
		if t, err := ParseTimeInCST(p.config.TimeLayout, attr); err == nil {
			endTime = t
		}
	}

	return discount, endTime
}

func (p *NexusPHPParser) ParseHR(doc *goquery.Selection) bool {
	html, _ := doc.Html()
	for _, keyword := range p.config.HRKeywords {
		if strings.Contains(html, keyword) {
			return true
		}
	}
	return false
}

func (p *NexusPHPParser) ParseSizeMB(doc *goquery.Selection) float64 {
	var sizeMB float64
	doc.Find(p.config.SizeSelector).Each(func(_ int, s *goquery.Selection) {
		text := s.Next().Text()
		matches := p.sizeRegex.FindStringSubmatch(text)
		if len(matches) < 3 {
			return
		}
		size, err := strconv.ParseFloat(matches[1], 64)
		if err != nil {
			return
		}
		switch strings.ToUpper(matches[2]) {
		case "TB", "TIB":
			size *= 1024 * 1024
		case "GB", "GIB":
			size *= 1024
		case "KB", "KIB":
			size /= 1024
		}
		sizeMB = size
	})
	return sizeMB
}

func (p *NexusPHPParser) ParseAll(doc *goquery.Selection) *TorrentDetailInfo {
	title, torrentID := p.ParseTitleAndID(doc)
	discount, endTime := p.ParseDiscount(doc)
	return &TorrentDetailInfo{
		TorrentID:     torrentID,
		Title:         title,
		SizeMB:        p.ParseSizeMB(doc),
		DiscountLevel: discount,
		DiscountEnd:   endTime,
		HasHR:         p.ParseHR(doc),
	}
}

// 兼容旧代码的别名
var (
	NewHDSkyParser        = NewNexusPHPParser
	NewSpringSundayParser = NewNexusPHPParser
)

// 兼容旧代码的类型别名
type (
	HDSkyParser        = NexusPHPParser
	SpringSundayParser = NexusPHPParser
)

// 兼容旧选项函数
func WithParserTimeLayout(layout string) NexusPHPParserOption {
	return WithTimeLayout(layout)
}
