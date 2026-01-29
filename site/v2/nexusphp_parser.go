// Package v2 provides site parsers for NexusPHP-based sites
// These parsers are migrated from internal/site_compat.go
package v2

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// NexusPHPParserConfig defines parser configuration
type NexusPHPParserConfig struct {
	TimeLayout string
}

// DefaultNexusPHPParserConfig returns default parser configuration
func DefaultNexusPHPParserConfig() NexusPHPParserConfig {
	return NexusPHPParserConfig{
		TimeLayout: "2006-01-02 15:04:05",
	}
}

// NexusPHPParserOption is a function that modifies NexusPHPParserConfig
type NexusPHPParserOption func(*NexusPHPParserConfig)

// WithParserTimeLayout sets the time layout for parsing
func WithParserTimeLayout(layout string) NexusPHPParserOption {
	return func(cfg *NexusPHPParserConfig) {
		cfg.TimeLayout = layout
	}
}

// TorrentDetailInfo represents parsed torrent detail information
type TorrentDetailInfo struct {
	TorrentID     string
	Title         string
	SizeMB        float64
	DiscountLevel DiscountLevel
	DiscountEnd   time.Time
	HasHR         bool
}

// NexusPHPDetailParser interface for parsing torrent detail pages
type NexusPHPDetailParser interface {
	// ParseTitleAndID parses title and torrent ID from the page
	ParseTitleAndID(doc *goquery.Selection) (title, torrentID string)
	// ParseDiscount parses discount type and end time
	ParseDiscount(doc *goquery.Selection) (DiscountLevel, time.Time)
	// ParseHR parses HR (Hit and Run) status
	ParseHR(doc *goquery.Selection) bool
	// ParseSizeMB parses torrent size in MB
	ParseSizeMB(doc *goquery.Selection) float64
	// ParseAll parses all information from the page
	ParseAll(doc *goquery.Selection) *TorrentDetailInfo
}

// ============================================================================
// HDSky Parser
// ============================================================================

// HDSkyParser implements NexusPHPDetailParser for HDSky site
type HDSkyParser struct {
	Config NexusPHPParserConfig
}

// NewHDSkyParser creates a new HDSkyParser
func NewHDSkyParser(options ...NexusPHPParserOption) *HDSkyParser {
	config := DefaultNexusPHPParserConfig()
	for _, opt := range options {
		opt(&config)
	}
	return &HDSkyParser{Config: config}
}

// ParseTitleAndID parses title and torrent ID from HDSky page
func (p *HDSkyParser) ParseTitleAndID(doc *goquery.Selection) (title, torrentID string) {
	title, _ = doc.Find("input[name='torrent_name']").Attr("value")
	torrentID, _ = doc.Find("input[name='detail_torrent_id']").Attr("value")
	return title, torrentID
}

// ParseDiscount parses discount type and end time from HDSky page
func (p *HDSkyParser) ParseDiscount(doc *goquery.Selection) (DiscountLevel, time.Time) {
	discountMapping := map[string]DiscountLevel{
		"free":          DiscountFree,
		"twoup":         Discount2xUp,
		"twoupfree":     Discount2xFree,
		"thirtypercent": DiscountPercent30,
		"halfdown":      DiscountPercent50,
		"twouphalfdown": Discount2x50,
		"pro_custom":    DiscountNone, // Custom discount, treat as none
	}

	discount := DiscountNone
	fontSelection := doc.Find("h1 font")
	fontSelection.EachWithBreak(func(_ int, font *goquery.Selection) bool {
		class, exists := font.Attr("class")
		if exists {
			if discountType, ok := discountMapping[class]; ok {
				discount = discountType
				return false
			}
		}
		return true
	})

	// Parse end time
	var endTime time.Time
	endTimeAttr := doc.Find("h1 span[title]").First().AttrOr("title", "")
	if endTimeAttr != "" {
		if t, err := ParseTimeInCST(p.Config.TimeLayout, endTimeAttr); err == nil {
			endTime = t
		}
	}

	return discount, endTime
}

// ParseHR parses HR status from HDSky page
func (p *HDSkyParser) ParseHR(doc *goquery.Selection) bool {
	html, _ := doc.Html()
	hrKeywords := []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run"}
	for _, keyword := range hrKeywords {
		if strings.Contains(html, keyword) {
			return true
		}
	}
	return false
}

// ParseSizeMB parses torrent size in MB from HDSky page
func (p *HDSkyParser) ParseSizeMB(doc *goquery.Selection) float64 {
	var sizeMB float64
	doc.Find("td.rowhead:contains('基本信息')").Each(func(_ int, s *goquery.Selection) {
		rowFollow := s.Next()
		text := rowFollow.Text()
		sizeRe := regexp.MustCompile(`大小：[^\d]*([\d.]+)\s*(GB|MB|KB)`)
		matches := sizeRe.FindStringSubmatch(text)
		if len(matches) < 3 {
			return
		}
		sizeValue := matches[1]
		unit := matches[2]
		size, err := strconv.ParseFloat(sizeValue, 64)
		if err != nil {
			return
		}
		switch strings.ToUpper(unit) {
		case "GB":
			size *= 1024
		case "KB":
			size /= 1024
		}
		sizeMB = size
	})
	return sizeMB
}

// ParseAll parses all information from HDSky page
func (p *HDSkyParser) ParseAll(doc *goquery.Selection) *TorrentDetailInfo {
	title, torrentID := p.ParseTitleAndID(doc)
	discount, endTime := p.ParseDiscount(doc)
	hasHR := p.ParseHR(doc)
	sizeMB := p.ParseSizeMB(doc)

	return &TorrentDetailInfo{
		TorrentID:     torrentID,
		Title:         title,
		SizeMB:        sizeMB,
		DiscountLevel: discount,
		DiscountEnd:   endTime,
		HasHR:         hasHR,
	}
}

// ============================================================================
// SpringSunday Parser
// ============================================================================

// SpringSundayParser implements NexusPHPDetailParser for SpringSunday site
type SpringSundayParser struct {
	Config NexusPHPParserConfig
}

// NewSpringSundayParser creates a new SpringSundayParser
func NewSpringSundayParser(options ...NexusPHPParserOption) *SpringSundayParser {
	config := DefaultNexusPHPParserConfig()
	for _, opt := range options {
		opt(&config)
	}
	return &SpringSundayParser{Config: config}
}

// ParseTitleAndID parses title and torrent ID from SpringSunday page
func (p *SpringSundayParser) ParseTitleAndID(doc *goquery.Selection) (title, torrentID string) {
	title, _ = doc.Find("input[name='torrent_name']").Attr("value")
	torrentID, _ = doc.Find("input[name='detail_torrent_id']").Attr("value")
	return title, torrentID
}

// ParseDiscount parses discount type and end time from SpringSunday page
func (p *SpringSundayParser) ParseDiscount(doc *goquery.Selection) (DiscountLevel, time.Time) {
	discountMapping := map[string]DiscountLevel{
		"free":          DiscountFree,
		"twoup":         Discount2xUp,
		"twoupfree":     Discount2xFree,
		"thirtypercent": DiscountPercent30,
		"halfdown":      DiscountPercent50,
		"twouphalfdown": Discount2x50,
		"pro_custom":    DiscountNone,
	}

	discount := DiscountNone
	fontSelection := doc.Find("h1 font")
	fontSelection.EachWithBreak(func(_ int, font *goquery.Selection) bool {
		class, exists := font.Attr("class")
		if exists {
			if discountType, ok := discountMapping[class]; ok {
				discount = discountType
				return false
			}
		}
		return true
	})

	// Parse end time
	var endTime time.Time
	endTimeAttr := doc.Find("h1 span[title]").First().AttrOr("title", "")
	if endTimeAttr != "" {
		if t, err := ParseTimeInCST(p.Config.TimeLayout, endTimeAttr); err == nil {
			endTime = t
		}
	}

	return discount, endTime
}

// ParseHR parses HR status from SpringSunday page
func (p *SpringSundayParser) ParseHR(doc *goquery.Selection) bool {
	html, _ := doc.Html()
	hrKeywords := []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run"}
	for _, keyword := range hrKeywords {
		if strings.Contains(html, keyword) {
			return true
		}
	}
	return false
}

// ParseSizeMB parses torrent size in MB from SpringSunday page
func (p *SpringSundayParser) ParseSizeMB(doc *goquery.Selection) float64 {
	var sizeMB float64
	doc.Find("td.rowhead:contains('基本信息')").Each(func(_ int, s *goquery.Selection) {
		rowFollow := s.Next()
		text := rowFollow.Text()
		sizeRe := regexp.MustCompile(`大小：[^\d]*([\d.]+)\s*(GB|MB|KB)`)
		matches := sizeRe.FindStringSubmatch(text)
		if len(matches) < 3 {
			return
		}
		sizeValue := matches[1]
		unit := matches[2]
		size, err := strconv.ParseFloat(sizeValue, 64)
		if err != nil {
			return
		}
		switch strings.ToUpper(unit) {
		case "GB":
			size *= 1024
		case "KB":
			size /= 1024
		}
		sizeMB = size
	})
	return sizeMB
}

// ParseAll parses all information from SpringSunday page
func (p *SpringSundayParser) ParseAll(doc *goquery.Selection) *TorrentDetailInfo {
	title, torrentID := p.ParseTitleAndID(doc)
	discount, endTime := p.ParseDiscount(doc)
	hasHR := p.ParseHR(doc)
	sizeMB := p.ParseSizeMB(doc)

	return &TorrentDetailInfo{
		TorrentID:     torrentID,
		Title:         title,
		SizeMB:        sizeMB,
		DiscountLevel: discount,
		DiscountEnd:   endTime,
		HasHR:         hasHR,
	}
}

// ============================================================================
// TTG Parser
// ============================================================================

// TTGParser implements NexusPHPDetailParser for TTG (To The Glory) site
type TTGParser struct {
	Config NexusPHPParserConfig
}

// NewTTGParser creates a new TTGParser
func NewTTGParser(options ...NexusPHPParserOption) *TTGParser {
	config := DefaultNexusPHPParserConfig()
	for _, opt := range options {
		opt(&config)
	}
	return &TTGParser{Config: config}
}

// ParseTitleAndID parses title and torrent ID from TTG page
func (p *TTGParser) ParseTitleAndID(doc *goquery.Selection) (title, torrentID string) {
	// 标题从 h1 标签获取
	title = doc.Find("h1").First().Text()

	// 种子ID从多个可能的位置获取
	// 1. 从 bookmark 链接的 tid 属性
	if tid, exists := doc.Find("a.bookmark").Attr("tid"); exists && tid != "" {
		torrentID = tid
	} else {
		// 2. 从 mycart 的 value 属性
		if tid, exists := doc.Find("#mycart").Attr("value"); exists && tid != "" {
			torrentID = tid
		}
	}
	return title, torrentID
}

// ParseDiscount parses discount type and end time from TTG page
func (p *TTGParser) ParseDiscount(doc *goquery.Selection) (DiscountLevel, time.Time) {
	discount := DiscountNone
	var endTime time.Time

	// 方法1: 检查是否有 ico_free.gif 图片
	doc.Find("img[src*='ico_free.gif']").Each(func(_ int, s *goquery.Selection) {
		discount = DiscountFree
	})

	// 方法2: 检查所有红色 font 标签是否有免费相关的文本（作为后备）
	if discount == DiscountNone {
		doc.Find("font[color='red']").Each(func(_ int, s *goquery.Selection) {
			text := s.Text()
			if strings.Contains(text, "限时不计流量") || strings.Contains(text, "Freeleech") ||
				strings.Contains(text, "限时免费") || (strings.Contains(text, "免费") && strings.Contains(text, "下载")) {
				discount = DiscountFree
			}
		})
	}

	// 解析过期时间 - 从所有红色 font 标签中查找 "到期时间为2026-01-30 16:32" 格式
	timeRe := regexp.MustCompile(`到期时间为(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2})`)
	doc.Find("font[color='red']").Each(func(_ int, s *goquery.Selection) {
		if !endTime.IsZero() {
			return // 已经找到了时间
		}
		text := s.Text()
		matches := timeRe.FindStringSubmatch(text)
		if len(matches) >= 2 {
			// TTG 使用格式 "2026-01-30 16:32" 不包含秒
			if t, err := ParseTimeInCST("2006-01-02 15:04", matches[1]); err == nil {
				endTime = t
			}
		}
	})

	return discount, endTime
}

// ParseHR parses HR status from TTG page
func (p *TTGParser) ParseHR(doc *goquery.Selection) bool {
	html, _ := doc.Html()
	// TTG 使用 "禁转资源" 表示 H&R
	// 检查 "禁转资源"、"H&R"、"H&amp;R" (HTML实体转义后)
	return strings.Contains(html, "禁转资源") || strings.Contains(html, "H&R") || strings.Contains(html, "H&amp;R")
}

// ParseSizeMB parses torrent size in MB from TTG page
func (p *TTGParser) ParseSizeMB(doc *goquery.Selection) float64 {
	var sizeMB float64
	// TTG 使用 "尺寸" 标签
	doc.Find("td.heading:contains('尺寸')").Each(func(_ int, s *goquery.Selection) {
		rowFollow := s.Next()
		text := rowFollow.Text()
		// 格式: "45.27 GB (48,609,376,900 字节)"
		sizeRe := regexp.MustCompile(`([\d.]+)\s*GB`)
		matches := sizeRe.FindStringSubmatch(text)
		if len(matches) >= 2 {
			size, err := strconv.ParseFloat(matches[1], 64)
			if err == nil {
				sizeMB = size * 1024 // 转换为 MB
			}
		}
	})
	return sizeMB
}

// ParseAll parses all information from TTG page
func (p *TTGParser) ParseAll(doc *goquery.Selection) *TorrentDetailInfo {
	title, torrentID := p.ParseTitleAndID(doc)
	discount, endTime := p.ParseDiscount(doc)
	hasHR := p.ParseHR(doc)
	sizeMB := p.ParseSizeMB(doc)

	return &TorrentDetailInfo{
		TorrentID:     torrentID,
		Title:         title,
		SizeMB:        sizeMB,
		DiscountLevel: discount,
		DiscountEnd:   endTime,
		HasHR:         hasHR,
	}
}

// ============================================================================
// Parser Registry
// ============================================================================

// ParserRegistry maps site names to their parsers
var ParserRegistry = map[SiteName]func() NexusPHPDetailParser{
	SiteNameHDSky:        func() NexusPHPDetailParser { return NewHDSkyParser() },
	SiteNameSpringSunday: func() NexusPHPDetailParser { return NewSpringSundayParser() },
	SiteNameTTG:          func() NexusPHPDetailParser { return NewTTGParser() },
}

// GetParser returns a parser for the given site name
func GetParser(siteName SiteName) NexusPHPDetailParser {
	if factory, ok := ParserRegistry[siteName]; ok {
		return factory()
	}
	return nil
}
