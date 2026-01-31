// Package internal provides site compatibility types and functions
// These were moved from the deprecated site package to support existing functionality
package internal

import (
	"context"
	"net/http"
	"time"

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

func (a *LegacyParserAdapter) ParseTitleAndID(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	title, torrentID := a.v2Parser.ParseTitleAndID(e.DOM)
	info.Title = title
	info.TorrentID = torrentID
}

func (a *LegacyParserAdapter) ParseDiscount(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	discount, endTime := a.v2Parser.ParseDiscount(e.DOM)
	info.Discount = mapV2DiscountToLegacy(discount)
	info.EndTime = endTime
}

func (a *LegacyParserAdapter) ParseHR(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	info.HR = a.v2Parser.ParseHR(e.DOM)
}

func (a *LegacyParserAdapter) ParseTorrentSizeMB(e *colly.HTMLElement, info *models.PHPTorrentInfo) {
	info.SizeMB = a.v2Parser.ParseSizeMB(e.DOM)
}

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
