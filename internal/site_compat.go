// Package internal provides site compatibility types and functions
// These were moved from the deprecated site package to support existing functionality
package internal

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// SiteParser interface for parsing torrent pages
type SiteParser interface {
	ParseTitleAndID(e *colly.HTMLElement, info *models.PHPTorrentInfo)
	ParseDiscount(e *colly.HTMLElement, info *models.PHPTorrentInfo)
	ParseHR(e *colly.HTMLElement, info *models.PHPTorrentInfo)
	ParseTorrentSizeMB(e *colly.HTMLElement, info *models.PHPTorrentInfo)
}

// RefererProvider provides referer URL for requests
type RefererProvider interface {
	GetReferer() string
}

// DefaultReferer implements RefererProvider
type DefaultReferer struct {
	referer string
}

func (d *DefaultReferer) GetReferer() string {
	return d.referer
}

// NewDefaultReferer creates a new DefaultReferer based on site group
func NewDefaultReferer(name models.SiteGroup) *DefaultReferer {
	referers := map[models.SiteGroup]string{
		models.HDSKY:        "https://hdsky.me/",
		models.SpringSunday: "https://springsunday.net/",
		models.MTEAM:        "https://kp.m-team.cc/",
	}
	return &DefaultReferer{referer: referers[name]}
}

// SiteConfig holds site-specific configuration
type SiteConfig struct {
	RefererConf RefererProvider
}

// SharedSiteConfig holds shared configuration across sites
type SharedSiteConfig struct {
	Cookie  string
	Headers map[string]string
	SiteCfg SiteConfig
}

// SiteMapConfig holds complete site configuration
type SiteMapConfig struct {
	Name          string
	SharedConfig  *SharedSiteConfig
	Config        models.SiteConfig
	Parser        SiteParser
	CustomHeaders map[string]string
}

// newSharedSiteConfig creates a new SharedSiteConfig
func newSharedSiteConfig(cookie string, refererProvider RefererProvider) *SharedSiteConfig {
	return &SharedSiteConfig{
		Cookie: cookie,
		Headers: map[string]string{
			"User-Agent": "Mozilla/5.0",
		},
		SiteCfg: SiteConfig{
			RefererConf: refererProvider,
		},
	}
}

// NewSiteMapConfig creates a new SiteMapConfig
func NewSiteMapConfig(name models.SiteGroup, cookie string, conf models.SiteConfig, parser SiteParser) *SiteMapConfig {
	refererProvider := NewDefaultReferer(name)
	sharedCfg := newSharedSiteConfig(cookie, refererProvider)
	return &SiteMapConfig{
		Name:         string(name),
		Config:       conf,
		SharedConfig: sharedCfg,
		Parser:       parser,
	}
}

// NewCollectorWithTransport creates a new colly collector with custom transport
func NewCollectorWithTransport() *colly.Collector {
	c := colly.NewCollector()
	c.AllowURLRevisit = true
	c.SetRequestTimeout(30 * time.Second)
	c.WithTransport(&http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     30 * time.Second,
	})
	return c
}

// CommonFetchTorrentInfo fetches torrent info from a URL
func CommonFetchTorrentInfo(ctx context.Context, c *colly.Collector, cfg *SiteMapConfig, url string) (*models.PHPTorrentInfo, error) {
	info := &models.PHPTorrentInfo{}
	var fetchErr error

	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("Cookie", cfg.SharedConfig.Cookie)
		r.Headers.Set("User-Agent", cfg.SharedConfig.Headers["User-Agent"])
		if cfg.SharedConfig.SiteCfg.RefererConf != nil {
			r.Headers.Set("Referer", cfg.SharedConfig.SiteCfg.RefererConf.GetReferer())
		}
	})

	c.OnHTML("html", func(e *colly.HTMLElement) {
		cfg.Parser.ParseTitleAndID(e, info)
		cfg.Parser.ParseDiscount(e, info)
		cfg.Parser.ParseHR(e, info)
		cfg.Parser.ParseTorrentSizeMB(e, info)
	})

	c.OnError(func(r *colly.Response, err error) {
		fetchErr = err
	})

	if err := c.Visit(url); err != nil {
		return nil, err
	}

	if fetchErr != nil {
		return nil, fetchErr
	}

	return info, nil
}

// ParserConfig defines parser configuration
type ParserConfig struct {
	TimeLayout string
}

// Default parser config
var defaultParserConfig = ParserConfig{
	TimeLayout: "2006-01-02 15:04:05",
}

// ParserOption is a function that modifies ParserConfig
type ParserOption func(*ParserConfig)

// WithTimeLayout sets the time layout
func WithTimeLayout(layout string) ParserOption {
	return func(cfg *ParserConfig) {
		cfg.TimeLayout = layout
	}
}

// HDSkyParser implements SiteParser for HDSky
type HDSkyParser struct {
	Config ParserConfig
}

// NewHDSkyParser creates a new HDSkyParser
func NewHDSkyParser(options ...ParserOption) *HDSkyParser {
	config := defaultParserConfig
	for _, opt := range options {
		opt(&config)
	}
	return &HDSkyParser{Config: config}
}

// ParseTitleAndID parses title and torrent ID
func (p *HDSkyParser) ParseTitleAndID(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	title, exists := e.DOM.Find("input[name='torrent_name']").Attr("value")
	if exists {
		info.Title = title
	}
	torrentID, exists := e.DOM.Find("input[name='detail_torrent_id']").Attr("value")
	if exists {
		info.TorrentID = torrentID
	}
}

// ParseDiscount parses discount type and end time
func (p *HDSkyParser) ParseDiscount(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	discountMapping := map[string]models.DiscountType{
		"free":          models.DISCOUNT_FREE,
		"twoup":         models.DISCOUNT_TWO_X,
		"twoupfree":     models.DISCOUNT_TWO_X_FREE,
		"thirtypercent": models.DISCOUNT_THIRTY,
		"halfdown":      models.DISCOUNT_FIFTY,
		"twouphalfdown": models.DISCOUNT_TWO_X_FIFTY,
		"pro_custom":    models.DISCOUNT_CUSTOM,
	}
	fontSelection := e.DOM.Find("h1 font")
	found := false
	fontSelection.EachWithBreak(func(_ int, font *goquery.Selection) bool {
		class, exists := font.Attr("class")
		if exists {
			if discountType, ok := discountMapping[class]; ok {
				info.Discount = discountType
				found = true
				return false
			}
		}
		return true
	})
	if !found {
		info.Discount = models.DISCOUNT_NONE
	}
	endTimeAttr := e.DOM.Find("h1 span[title]").First().AttrOr("title", "")
	if endTimeAttr != "" {
		t, err := time.ParseInLocation(p.Config.TimeLayout, endTimeAttr, time.Local)
		if err == nil {
			info.EndTime = t
		} else {
			sLogger().Error("解析结束时间出错:", err)
		}
	}
}

// ParseHR parses HR status
func (p *HDSkyParser) ParseHR(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	html, _ := e.DOM.Html()
	hrKeywords := []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run"}
	for _, keyword := range hrKeywords {
		if strings.Contains(html, keyword) {
			info.HR = true
			break
		}
	}
}

// ParseTorrentSizeMB parses torrent size in MB
func (p *HDSkyParser) ParseTorrentSizeMB(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	e.DOM.Find("td.rowhead:contains('基本信息')").Each(func(_ int, s *goquery.Selection) {
		rowFollow := s.Next()
		text := rowFollow.Text()
		sizeRe := regexp.MustCompile(`大小：[^\d]*([\d.]+)\s*(GB|MB|KB)`)
		matches := sizeRe.FindStringSubmatch(text)
		if len(matches) < 3 {
			sLogger().Error("无法解析大小信息")
			return
		}
		sizeValue := matches[1]
		unit := matches[2]
		size, err := strconv.ParseFloat(sizeValue, 64)
		if err != nil {
			sLogger().Error("无法解析大小", err)
			return
		}
		switch strings.ToUpper(unit) {
		case "GB":
			size *= 1024
		case "KB":
			size /= 1024
		}
		info.SizeMB = size
	})
}

// SpringSundayParser implements SiteParser for SpringSunday
type SpringSundayParser struct {
	Config ParserConfig
}

// NewSpringSundayParser creates a new SpringSundayParser
func NewSpringSundayParser(options ...ParserOption) *SpringSundayParser {
	config := defaultParserConfig
	for _, opt := range options {
		opt(&config)
	}
	return &SpringSundayParser{Config: config}
}

// ParseTitleAndID parses title and torrent ID for SpringSunday
func (p *SpringSundayParser) ParseTitleAndID(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	title, exists := e.DOM.Find("input[name='torrent_name']").Attr("value")
	if exists {
		info.Title = title
	}
	torrentID, exists := e.DOM.Find("input[name='detail_torrent_id']").Attr("value")
	if exists {
		info.TorrentID = torrentID
	}
}

// ParseDiscount parses discount type and end time for SpringSunday
func (p *SpringSundayParser) ParseDiscount(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	discountMapping := map[string]models.DiscountType{
		"free":          models.DISCOUNT_FREE,
		"twoup":         models.DISCOUNT_TWO_X,
		"twoupfree":     models.DISCOUNT_TWO_X_FREE,
		"thirtypercent": models.DISCOUNT_THIRTY,
		"halfdown":      models.DISCOUNT_FIFTY,
		"twouphalfdown": models.DISCOUNT_TWO_X_FIFTY,
		"pro_custom":    models.DISCOUNT_CUSTOM,
	}
	fontSelection := e.DOM.Find("h1 font")
	found := false
	fontSelection.EachWithBreak(func(_ int, font *goquery.Selection) bool {
		class, exists := font.Attr("class")
		if exists {
			if discountType, ok := discountMapping[class]; ok {
				info.Discount = discountType
				found = true
				return false
			}
		}
		return true
	})
	if !found {
		info.Discount = models.DISCOUNT_NONE
	}
	endTimeAttr := e.DOM.Find("h1 span[title]").First().AttrOr("title", "")
	if endTimeAttr != "" {
		t, err := time.ParseInLocation(p.Config.TimeLayout, endTimeAttr, time.Local)
		if err == nil {
			info.EndTime = t
		} else {
			sLogger().Error("解析结束时间出错:", err)
		}
	}
}

// ParseHR parses HR status for SpringSunday
func (p *SpringSundayParser) ParseHR(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	html, _ := e.DOM.Html()
	hrKeywords := []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run"}
	for _, keyword := range hrKeywords {
		if strings.Contains(html, keyword) {
			info.HR = true
			break
		}
	}
}

// ParseTorrentSizeMB parses torrent size in MB for SpringSunday
func (p *SpringSundayParser) ParseTorrentSizeMB(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	e.DOM.Find("td.rowhead:contains('基本信息')").Each(func(_ int, s *goquery.Selection) {
		rowFollow := s.Next()
		text := rowFollow.Text()
		sizeRe := regexp.MustCompile(`大小：[^\d]*([\d.]+)\s*(GB|MB|KB)`)
		matches := sizeRe.FindStringSubmatch(text)
		if len(matches) < 3 {
			sLogger().Error("无法解析大小信息")
			return
		}
		sizeValue := matches[1]
		unit := matches[2]
		size, err := strconv.ParseFloat(sizeValue, 64)
		if err != nil {
			sLogger().Error("无法解析大小", err)
			return
		}
		switch strings.ToUpper(unit) {
		case "GB":
			size *= 1024
		case "KB":
			size /= 1024
		}
		info.SizeMB = size
	})
}

// ============================================================================
// Legacy Parser Adapter
// ============================================================================

// LegacyParserAdapter adapts v2.NexusPHPDetailParser to the legacy SiteParser interface
// This allows using the new v2 parsers with the existing colly-based fetching code
type LegacyParserAdapter struct {
	v2Parser v2.NexusPHPDetailParser
}

// NewLegacyParserAdapter creates a new adapter for a v2 parser
func NewLegacyParserAdapter(parser v2.NexusPHPDetailParser) *LegacyParserAdapter {
	return &LegacyParserAdapter{v2Parser: parser}
}

// ParseTitleAndID adapts the v2 parser's ParseTitleAndID method
func (a *LegacyParserAdapter) ParseTitleAndID(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	title, torrentID := a.v2Parser.ParseTitleAndID(e.DOM)
	info.Title = title
	info.TorrentID = torrentID
}

// ParseDiscount adapts the v2 parser's ParseDiscount method
func (a *LegacyParserAdapter) ParseDiscount(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	discount, endTime := a.v2Parser.ParseDiscount(e.DOM)
	info.Discount = mapV2DiscountToLegacy(discount)
	info.EndTime = endTime
}

// ParseHR adapts the v2 parser's ParseHR method
func (a *LegacyParserAdapter) ParseHR(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	info.HR = a.v2Parser.ParseHR(e.DOM)
}

// ParseTorrentSizeMB adapts the v2 parser's ParseSizeMB method
func (a *LegacyParserAdapter) ParseTorrentSizeMB(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	info.SizeMB = a.v2Parser.ParseSizeMB(e.DOM)
}

// mapV2DiscountToLegacy maps v2.DiscountLevel to models.DiscountType
func mapV2DiscountToLegacy(discount v2.DiscountLevel) models.DiscountType {
	switch discount {
	case v2.DiscountFree:
		return models.DISCOUNT_FREE
	case v2.Discount2xFree:
		return models.DISCOUNT_TWO_X_FREE
	case v2.Discount2xUp:
		return models.DISCOUNT_TWO_X
	case v2.DiscountPercent50:
		return models.DISCOUNT_FIFTY
	case v2.DiscountPercent30:
		return models.DISCOUNT_THIRTY
	case v2.Discount2x50:
		return models.DISCOUNT_TWO_X_FIFTY
	default:
		return models.DISCOUNT_NONE
	}
}
