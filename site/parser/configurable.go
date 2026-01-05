package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"
)

// ConfigurableParser 可配置的站点解析器
// 通过配置文件定义解析规则，支持动态站点
type ConfigurableParser struct {
	config *ParserConfig
	// 编译后的正则表达式缓存
	sizeRegex *regexp.Regexp
	idRegex   *regexp.Regexp
}

// NewConfigurableParser 创建可配置解析器
func NewConfigurableParser(config *ParserConfig) (*ConfigurableParser, error) {
	if config == nil {
		config = DefaultParserConfig()
	}

	parser := &ConfigurableParser{
		config: config,
	}

	// 编译正则表达式
	if config.Patterns.SizePattern != "" {
		regex, err := regexp.Compile(config.Patterns.SizePattern)
		if err != nil {
			return nil, fmt.Errorf("invalid size pattern: %w", err)
		}
		parser.sizeRegex = regex
	}

	if config.Patterns.IDPattern != "" {
		regex, err := regexp.Compile(config.Patterns.IDPattern)
		if err != nil {
			return nil, fmt.Errorf("invalid ID pattern: %w", err)
		}
		parser.idRegex = regex
	}

	return parser, nil
}

// GetName 获取解析器名称
func (p *ConfigurableParser) GetName() string {
	return p.config.Name
}

// ParseTorrentPage 解析种子详情页面
func (p *ConfigurableParser) ParseTorrentPage(e *colly.HTMLElement) (*ParsedTorrentInfo, error) {
	info := &ParsedTorrentInfo{}

	// 解析标题
	if p.config.Selectors.TitleSelector != "" {
		info.Title = strings.TrimSpace(e.DOM.Find(p.config.Selectors.TitleSelector).First().Text())
	}

	// 解析ID
	if p.config.Selectors.IDSelector != "" {
		idVal, exists := e.DOM.Find(p.config.Selectors.IDSelector).First().Attr("value")
		if exists {
			info.ID = idVal
		}
	}

	// 解析大小
	if p.config.Selectors.SizeSelector != "" {
		p.parseSize(e.DOM, info)
	}

	// 解析优惠类型
	if p.config.Selectors.DiscountSelector != "" {
		p.parseDiscount(e.DOM, info)
	}

	// 解析免费结束时间
	if p.config.Selectors.FreeEndTimeSelector != "" {
		p.parseFreeEndTime(e.DOM, info)
	}

	// 解析HR状态
	if p.config.Selectors.HRSelector != "" || len(p.config.Patterns.HRKeywords) > 0 {
		p.parseHR(e.DOM, info)
	}

	// 解析下载链接
	if p.config.Selectors.DownloadURLSelector != "" {
		href, exists := e.DOM.Find(p.config.Selectors.DownloadURLSelector).First().Attr("href")
		if exists {
			info.DownloadURL = href
		}
	}

	return info, nil
}

// parseSize 解析种子大小
func (p *ConfigurableParser) parseSize(doc *goquery.Selection, info *ParsedTorrentInfo) {
	sizeText := doc.Find(p.config.Selectors.SizeSelector).First().Text()
	if sizeText == "" {
		// 尝试获取下一个兄弟元素
		sizeText = doc.Find(p.config.Selectors.SizeSelector).First().Next().Text()
	}

	if p.sizeRegex != nil && sizeText != "" {
		matches := p.sizeRegex.FindStringSubmatch(sizeText)
		if len(matches) >= 3 {
			sizeValue, err := strconv.ParseFloat(matches[1], 64)
			if err == nil {
				info.Size = convertToBytes(sizeValue, matches[2])
			}
		}
	}
}

// parseDiscount 解析优惠类型
func (p *ConfigurableParser) parseDiscount(doc *goquery.Selection, info *ParsedTorrentInfo) {
	doc.Find(p.config.Selectors.DiscountSelector).Each(func(_ int, s *goquery.Selection) {
		class, exists := s.Attr("class")
		if exists {
			if discountType, ok := p.config.DiscountMapping[class]; ok {
				info.DiscountType = discountType
				info.IsFree = strings.Contains(strings.ToLower(discountType), "free")
			}
		}
	})
}

// parseFreeEndTime 解析免费结束时间
func (p *ConfigurableParser) parseFreeEndTime(doc *goquery.Selection, info *ParsedTorrentInfo) {
	endTimeAttr := doc.Find(p.config.Selectors.FreeEndTimeSelector).First().AttrOr("title", "")
	if endTimeAttr != "" {
		t, err := time.ParseInLocation(p.config.TimeLayout, endTimeAttr, time.Local)
		if err == nil {
			info.FreeEndTime = t
		}
	}
}

// parseHR 解析HR状态
func (p *ConfigurableParser) parseHR(doc *goquery.Selection, info *ParsedTorrentInfo) {
	// 先检查选择器
	if p.config.Selectors.HRSelector != "" {
		if doc.Find(p.config.Selectors.HRSelector).Length() > 0 {
			info.HasHR = true
			return
		}
	}

	// 再检查关键词
	html, _ := doc.Html()
	for _, keyword := range p.config.Patterns.HRKeywords {
		if strings.Contains(html, keyword) {
			info.HasHR = true
			return
		}
	}
}

// ParseRSSItem 解析RSS条目
func (p *ConfigurableParser) ParseRSSItem(title, link, description string) (*ParsedTorrentInfo, error) {
	info := &ParsedTorrentInfo{
		Title: title,
	}

	// 从链接中提取ID
	if p.idRegex != nil {
		matches := p.idRegex.FindStringSubmatch(link)
		if len(matches) >= 2 {
			info.ID = matches[1]
		}
	}

	// 从描述中提取大小
	if p.sizeRegex != nil && description != "" {
		matches := p.sizeRegex.FindStringSubmatch(description)
		if len(matches) >= 3 {
			sizeValue, err := strconv.ParseFloat(matches[1], 64)
			if err == nil {
				info.Size = convertToBytes(sizeValue, matches[2])
			}
		}
	}

	return info, nil
}

// convertToBytes 将大小转换为字节
func convertToBytes(value float64, unit string) int64 {
	switch strings.ToUpper(unit) {
	case "TB":
		return int64(value * 1024 * 1024 * 1024 * 1024)
	case "GB":
		return int64(value * 1024 * 1024 * 1024)
	case "MB":
		return int64(value * 1024 * 1024)
	case "KB":
		return int64(value * 1024)
	default:
		return int64(value)
	}
}

// Validate 验证解析器配置
func (c *ParserConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("parser name is required")
	}
	if c.TimeLayout == "" {
		return fmt.Errorf("time layout is required")
	}
	// 验证正则表达式
	if c.Patterns.SizePattern != "" {
		if _, err := regexp.Compile(c.Patterns.SizePattern); err != nil {
			return fmt.Errorf("invalid size pattern: %w", err)
		}
	}
	if c.Patterns.IDPattern != "" {
		if _, err := regexp.Compile(c.Patterns.IDPattern); err != nil {
			return fmt.Errorf("invalid ID pattern: %w", err)
		}
	}
	return nil
}
