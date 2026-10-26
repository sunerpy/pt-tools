package v2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
	"golang.org/x/net/publicsuffix"
	"golang.org/x/sync/errgroup"

	"github.com/sunerpy/pt-tools/utils"
)

// NexusPHPRequest represents a request to a NexusPHP site
type NexusPHPRequest struct {
	// Path is the URL path (e.g., "/torrents.php")
	Path string
	// Params are the query parameters
	Params url.Values
	// Method is the HTTP method (default: GET)
	Method string
	// Referer 不为空时作为 Referer 请求头；以 / 开头的按站点地址补全（签到的验证码接口会看它）
	Referer string
}

// NexusPHPResponse wraps a goquery document for parsing
type NexusPHPResponse struct {
	// Document is the parsed HTML document
	Document *goquery.Document
	// RawBody is the raw response body (for downloads)
	RawBody []byte
	// StatusCode is the HTTP status code
	StatusCode int
}

// SiteSelectors defines CSS selectors for parsing NexusPHP pages
type SiteSelectors struct {
	// TableRows selects torrent rows in the search results
	TableRows string `json:"tableRows"`
	// Title selects the torrent title
	Title string `json:"title"`
	// TitleLink selects the link containing torrent ID
	TitleLink string `json:"titleLink"`
	// Size selects the torrent size
	Size string `json:"size"`
	// Seeders selects the seeder count
	Seeders string `json:"seeders"`
	// Leechers selects the leecher count
	Leechers string `json:"leechers"`
	// Snatched selects the snatch count
	Snatched string `json:"snatched"`
	// DiscountIcon selects the discount icon element
	DiscountIcon string `json:"discountIcon"`
	// DiscountMapping maps keywords to discount levels (optional, uses default if nil)
	// Keys are matched against class, src, alt attributes (case-insensitive)
	DiscountMapping map[string]DiscountLevel `json:"discountMapping,omitempty"`
	// DiscountEndTime selects the discount end time
	DiscountEndTime string `json:"discountEndTime"`
	// DownloadLink selects the download link
	DownloadLink string `json:"downloadLink"`
	// Category selects the category
	Category string `json:"category"`
	// UploadTime selects the upload time
	UploadTime string `json:"uploadTime"`
	// HRIcon selects the H&R icon
	HRIcon string `json:"hrIcon"`
	// Subtitle selects the subtitle in search results
	Subtitle string `json:"subtitle"`
	// SubtitleSelector extracts subtitles that require HTML attributes or filters
	SubtitleSelector *FieldSelector `json:"subtitleSelector,omitempty"`
	// UserInfo selectors for user page
	UserInfoUsername   string `json:"userInfoUsername"`
	UserInfoUploaded   string `json:"userInfoUploaded"`
	UserInfoDownloaded string `json:"userInfoDownloaded"`
	UserInfoRatio      string `json:"userInfoRatio"`
	UserInfoBonus      string `json:"userInfoBonus"`
	UserInfoRank       string `json:"userInfoRank"`
	// Detail page selectors
	// DetailDownloadLink selects the download link from details page
	DetailDownloadLink string `json:"detailDownloadLink"`
	// DetailSubtitle selects the subtitle from details page
	DetailSubtitle string `json:"detailSubtitle"`
}

// DefaultNexusPHPSelectors returns default selectors for standard NexusPHP sites
func DefaultNexusPHPSelectors() SiteSelectors {
	return SiteSelectors{
		TableRows:          "table.torrents > tbody > tr:not(:first-child)",
		Title:              "td:nth-child(2) a[href*='details.php']",
		TitleLink:          "td:nth-child(2) a[href*='details.php']",
		Size:               "td:nth-child(5)",
		Seeders:            "td:nth-child(6)",
		Leechers:           "td:nth-child(7)",
		Snatched:           "td:nth-child(8)",
		DiscountIcon:       "img.pro_free, img.pro_free2up, img.pro_50pctdown, img.pro_30pctdown, img.pro_2up",
		DiscountEndTime:    "span.free_end_time, span[title*='结束']",
		DownloadLink:       "a[href*='download.php']",
		Category:           "td:nth-child(1) img",
		UploadTime:         "td:nth-child(4) span",
		HRIcon:             "img.hitandrun, img[alt*='H&R'], img[title*='H&R']",
		Subtitle:           "td:nth-child(2) br + *",
		UserInfoUsername:   "#info_block a.User_Name, a[href*='userdetails.php']",
		UserInfoUploaded:   "td:contains('上传量') + td, td:contains('Uploaded') + td",
		UserInfoDownloaded: "td:contains('下载量') + td, td:contains('Downloaded') + td",
		UserInfoRatio:      "td:contains('分享率') + td, td:contains('Ratio') + td",
		UserInfoBonus:      "td:contains('魔力值') + td, td:contains('Bonus') + td",
		UserInfoRank:       "td:contains('等级') + td, td:contains('Class') + td",
		// Detail page selectors - default for standard NexusPHP sites
		DetailDownloadLink: "td.rowhead:contains('下载链接') + td a[href*='download.php'], form[action*='download.php']",
		DetailSubtitle:     "td.rowhead:contains('副标题') + td, td.rowhead:contains('小标题') + td",
	}
}

// DebugUserInfo enables debug output for user info parsing
// Set to true to see detailed parsing information
var DebugUserInfo = false

// truncateStr truncates a string to max length
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// NexusPHPDriver implements the Driver interface for NexusPHP sites
type NexusPHPDriver struct {
	BaseURL        string
	Cookie         string
	Selectors      SiteSelectors
	httpClient     *SiteHTTPClient
	failoverClient *FailoverHTTPClient
	userAgent      string
	useFailover    bool
	siteName       SiteName
	siteID         string
	siteDefinition *SiteDefinition
}

// NexusPHPDriverConfig holds configuration for creating a NexusPHP driver
type NexusPHPDriverConfig struct {
	BaseURL     string
	Cookie      string
	Selectors   *SiteSelectors
	HTTPClient  *SiteHTTPClient // Use SiteHTTPClient instead of *http.Client
	UserAgent   string
	UseFailover bool     // Enable multi-URL failover
	SiteName    SiteName // Site name for failover URL lookup
	// SiteID 是站点在 pt-tools 里注册的 ID：搜索结果的代理下载地址 /api/site/{id}/… 按它找站点
	SiteID string
}

// NewNexusPHPDriver creates a new NexusPHP driver
func NewNexusPHPDriver(config NexusPHPDriverConfig) *NexusPHPDriver {
	selectors := DefaultNexusPHPSelectors()
	if config.Selectors != nil {
		selectors = *config.Selectors
	}

	userAgent := config.UserAgent
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = NewSiteHTTPClient(SiteHTTPClientConfig{
			Timeout:           30 * time.Second,
			MaxIdleConns:      10,
			IdleConnTimeout:   30 * time.Second,
			DisableKeepAlives: true,
			UserAgent:         userAgent,
		})
	}

	driver := &NexusPHPDriver{
		BaseURL:     strings.TrimSuffix(config.BaseURL, "/"),
		Cookie:      config.Cookie,
		Selectors:   selectors,
		httpClient:  httpClient,
		userAgent:   userAgent,
		useFailover: config.UseFailover,
		siteName:    config.SiteName,
		siteID:      config.SiteID,
	}

	// Initialize failover client if enabled and site name is provided
	if config.UseFailover && config.SiteName != "" {
		registry := GetGlobalRegistry()
		if failoverClient, err := registry.GetFailoverClient(
			config.SiteName,
			WithUserAgent(userAgent),
		); err == nil {
			driver.failoverClient = failoverClient
		}
	}

	return driver
}

// NewNexusPHPDriverWithFailover creates a new NexusPHP driver with failover enabled
func NewNexusPHPDriverWithFailover(siteName SiteName, cookie string) *NexusPHPDriver {
	registry := GetGlobalRegistry()
	urls := registry.GetURLs(siteName)
	baseURL := ""
	if len(urls) > 0 {
		baseURL = urls[0]
	}

	return NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL:     baseURL,
		Cookie:      cookie,
		UseFailover: true,
		SiteName:    siteName,
	})
}

// SetSiteDefinition sets the site definition for custom parsing
func (d *NexusPHPDriver) SetSiteDefinition(def *SiteDefinition) {
	d.siteDefinition = def
}

// GetSiteDefinition returns the site definition
func (d *NexusPHPDriver) GetSiteDefinition() *SiteDefinition {
	return d.siteDefinition
}

// PrepareSearch converts a SearchQuery to a NexusPHP request
func (d *NexusPHPDriver) PrepareSearch(query SearchQuery) (NexusPHPRequest, error) {
	params := url.Values{}

	if query.Keyword != "" {
		params.Set("search", query.Keyword)
	}
	if query.Category != "" {
		params.Set("cat", query.Category)
	}
	if query.FreeOnly {
		params.Set("spstate", "2") // Free torrents in NexusPHP
	}
	if query.Page > 0 {
		params.Set("page", strconv.Itoa(query.Page-1)) // NexusPHP uses 0-indexed pages
	}

	return NexusPHPRequest{
		Path:   "/torrents.php",
		Params: params,
		Method: "GET",
	}, nil
}

// Execute performs the HTTP request
func (d *NexusPHPDriver) Execute(ctx context.Context, req NexusPHPRequest) (NexusPHPResponse, error) {
	// Use failover client if available
	if d.useFailover && d.failoverClient != nil {
		return d.executeWithFailover(ctx, req)
	}
	return d.executeDirectly(ctx, req, d.BaseURL)
}

// executeWithFailover executes request with automatic URL failover
func (d *NexusPHPDriver) executeWithFailover(ctx context.Context, req NexusPHPRequest) (NexusPHPResponse, error) {
	var result NexusPHPResponse
	err := d.failoverClient.manager.ExecuteWithFailover(ctx, func(baseURL string) error {
		res, err := d.executeDirectly(ctx, req, baseURL)
		if err != nil {
			return err
		}
		result = res
		return nil
	})
	return result, err
}

// executeDirectly performs the HTTP request to a specific base URL
func (d *NexusPHPDriver) executeDirectly(ctx context.Context, req NexusPHPRequest, baseURL string) (NexusPHPResponse, error) {
	method := req.Method
	if method == "" {
		method = "GET"
	}

	post := strings.EqualFold(method, http.MethodPost)
	fullURL := baseURL + req.Path
	if len(req.Params) > 0 && !post {
		fullURL += "?" + req.Params.Encode()
	}

	headers := map[string]string{
		"Cookie":          d.Cookie,
		"User-Agent":      d.userAgent,
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
	}
	if ref := req.Referer; ref != "" {
		if strings.HasPrefix(ref, "/") {
			ref = baseURL + ref
		}
		headers["Referer"] = ref
	}

	// Print curl command for debugging
	if DebugUserInfo {
		fmt.Printf("\n[CURL] %s\n", buildCurlCommand(method, fullURL, headers))
	}

	var resp *HTTPResponse
	var err error
	if post {
		// POST 时参数作为表单提交（目前只有配置了 POST 的签到会走这里）。
		headers["Content-Type"] = "application/x-www-form-urlencoded"
		resp, err = d.httpClient.Post(ctx, fullURL, []byte(req.Params.Encode()), headers)
	} else {
		resp, err = d.httpClient.Get(ctx, fullURL, headers)
	}
	if err != nil {
		return NexusPHPResponse{}, fmt.Errorf("execute request: %w", err)
	}

	result := NexusPHPResponse{
		RawBody:    resp.Body,
		StatusCode: resp.StatusCode,
	}

	if resp.IsCloudflareChallenge() {
		return result, fmt.Errorf("HTTP %d: %w", resp.StatusCode, ErrCloudflareChallenge)
	}

	// Check for authentication errors
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return result, ErrInvalidCredentials
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return result, fmt.Errorf("HTTP 429: %w", ErrRateLimited)
	}

	if resp.StatusCode != http.StatusOK {
		return result, &HTTPStatusError{Code: resp.StatusCode}
	}

	// Parse HTML document
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(resp.Body)))
	if err != nil {
		return result, fmt.Errorf("parse HTML: %w", err)
	}
	result.Document = doc

	// Check if we're on a login page (cookie expired or invalid)
	if isLoginPage(doc) {
		return result, ErrSessionExpired
	}

	// Check if 2FA verification is required
	if is2FAPage(doc) {
		return result, Err2FARequired
	}

	return result, nil
}

// isLoginPage checks if the HTML document is a login page
// This indicates the session/cookie has expired or is invalid
func isLoginPage(doc *goquery.Document) bool {
	// Check for common login page indicators
	// 1. Form action pointing to takelogin.php
	if doc.Find("form[action*='takelogin']").Length() > 0 {
		return true
	}

	// 2. Login form with username/password fields and specific class
	if doc.Find(".login-panel").Length() > 0 || doc.Find(".login-form").Length() > 0 {
		return true
	}

	// 3. Check title contains login-related keywords
	title := strings.ToLower(doc.Find("title").Text())
	if strings.Contains(title, "登录") || strings.Contains(title, "login") {
		// Make sure it's actually a login page, not just a page that mentions login
		// by checking for login form elements
		if doc.Find("input[name='username']").Length() > 0 &&
			doc.Find("input[name='password']").Length() > 0 {
			return true
		}
	}

	// 4. Check for redirect meta tag to login page
	redirectURL, exists := doc.Find("meta[http-equiv='refresh']").Attr("content")
	if exists && (strings.Contains(redirectURL, "login.php") || strings.Contains(redirectURL, "takelogin")) {
		return true
	}

	return false
}

// is2FAPage checks if the HTML document is a 2FA verification page
func is2FAPage(doc *goquery.Document) bool {
	// Check for 2FA redirect script
	scripts := doc.Find("script").Text()
	if strings.Contains(scripts, "take2fa.php") || strings.Contains(scripts, "2fa") {
		return true
	}

	// Check for 2FA form
	if doc.Find("form[action*='2fa']").Length() > 0 || doc.Find("form[action*='take2fa']").Length() > 0 {
		return true
	}

	// Check title for 2FA keywords
	title := strings.ToLower(doc.Find("title").Text())
	if strings.Contains(title, "二次验证") || strings.Contains(title, "2fa") || strings.Contains(title, "两步验证") {
		return true
	}

	return false
}

// buildCurlCommand generates a curl command string for debugging
func buildCurlCommand(method, url string, headers map[string]string) string {
	cmd := fmt.Sprintf("curl -X %s", method)
	for k, v := range headers {
		// Escape single quotes in value
		escapedV := strings.ReplaceAll(v, "'", "'\\''")
		cmd += fmt.Sprintf(" -H '%s: %s'", k, escapedV)
	}
	cmd += fmt.Sprintf(" '%s'", url)
	return cmd
}

// ParseSearch extracts torrent items from the response
func (d *NexusPHPDriver) ParseSearch(res NexusPHPResponse) ([]TorrentItem, error) {
	if res.Document == nil {
		return nil, ErrParseError
	}

	var items []TorrentItem

	res.Document.Find(d.Selectors.TableRows).Each(func(i int, s *goquery.Selection) {
		item := TorrentItem{
			SourceSite:    d.BaseURL,
			DiscountLevel: DiscountNone,
		}

		// Parse title and ID
		titleElem := s.Find(d.Selectors.Title)
		item.Title = strings.TrimSpace(titleElem.Text())
		if href, exists := titleElem.Attr("href"); exists {
			item.ID = extractTorrentID(href)
			item.URL = d.BaseURL + "/" + href
		}

		// Skip if no title
		if item.Title == "" {
			return
		}

		// Parse subtitle (副标题) - usually in the same cell as title
		if d.Selectors.Subtitle != "" {
			subtitleElem := s.Find(d.Selectors.Subtitle)
			if subtitleElem.Length() > 0 {
				item.Subtitle = strings.TrimSpace(subtitleElem.Text())
			}
		} else if d.Selectors.SubtitleSelector != nil {
			item.Subtitle = d.extractFieldValueFromSelection(s, *d.Selectors.SubtitleSelector)
		}

		// Parse size
		sizeText := strings.TrimSpace(s.Find(d.Selectors.Size).Text())
		item.SizeBytes = parseSize(sizeText)

		// Parse seeders
		// 计数单元格只含数字本身，因此仅去掉千分位分隔符：部分站点渲染 <b>7,170</b>
		// 这类值，直接 Atoi 会静默得到 0。刻意不做更宽松的数字提取——除逗号外的非数字
		// 内容（日期、相对时间、空格分组）应继续得到 0，让选择器配错时明显失败，而不是
		// 产生一个看似合理的错误计数；计数会喂给用户的 RSS 过滤规则，错值比 0 更难发现。
		seedersText := strings.TrimSpace(s.Find(d.Selectors.Seeders).Text())
		item.Seeders, _ = strconv.Atoi(strings.ReplaceAll(seedersText, ",", ""))

		// Parse leechers
		leechersText := strings.TrimSpace(s.Find(d.Selectors.Leechers).Text())
		item.Leechers, _ = strconv.Atoi(strings.ReplaceAll(leechersText, ",", ""))

		// Parse snatched
		snatchedText := strings.TrimSpace(s.Find(d.Selectors.Snatched).Text())
		item.Snatched, _ = strconv.Atoi(strings.ReplaceAll(snatchedText, ",", ""))

		// Parse discount level
		discountElem := s.Find(d.Selectors.DiscountIcon)
		if discountElem.Length() > 0 {
			item.DiscountLevel = parseDiscountFromElement(discountElem, d.Selectors.DiscountMapping)
		}

		// Parse discount end time
		endTimeUnparsable := false
		endTimeElem := s.Find(d.Selectors.DiscountEndTime)
		if endTimeElem.Length() > 0 {
			value := endTimeElem.Text()
			if title, exists := endTimeElem.Attr("title"); exists {
				value = title
			}
			endTime, err := parseDiscountEndTime(value)
			item.DiscountEndTime = endTime
			endTimeUnparsable = err != nil
		}

		// Fallback: parse discount end time from onmouseover attribute of discount icon
		// Some sites (like HDSky) embed the end time in the tooltip, not as a separate element
		// Format: domTT_activate(..., '<span title="2026-01-18 22:37:47">1时19分</span>', ...)
		if item.DiscountEndTime.IsZero() && discountElem.Length() > 0 {
			if onmouseover, exists := discountElem.Attr("onmouseover"); exists && onmouseover != "" {
				endTime, err := parseDiscountEndTimeFromOnmouseover(onmouseover)
				item.DiscountEndTime = endTime
				if err != nil {
					endTimeUnparsable = true
				}
			}
		}

		// 结束时间元素的值非空却解析不了、优惠图标的提示里也取不到：结束时间不可信，
		// 不能当成「没有结束时间」，免费的有效性是 unknown
		if endTimeUnparsable && item.DiscountEndTime.IsZero() {
			NoteDiscountTimeUnparsable(&item, "discountEndTime")
		}

		// Parse download link - use proxy URL instead of direct link for authentication handling
		// The backend proxy will handle cookie/passkey authentication
		if item.ID != "" {
			item.DownloadURL = fmt.Sprintf("/api/site/%s/torrent/%s/download", d.getSiteID(), item.ID)
		} else {
			// If no ID, try to get direct link (may not work without passkey)
			downloadElem := s.Find(d.Selectors.DownloadLink)
			if href, exists := downloadElem.Attr("href"); exists {
				item.DownloadURL = d.BaseURL + "/" + href
			}
		}

		// Parse category
		categoryElem := s.Find(d.Selectors.Category)
		if alt, exists := categoryElem.Attr("alt"); exists {
			item.Category = alt
		}

		// Parse upload time
		if d.Selectors.UploadTime != "" {
			uploadTimeElem := s.Find(d.Selectors.UploadTime)
			if uploadTimeElem.Length() > 0 {
				// Try to get time from title attribute first (more precise)
				if title, exists := uploadTimeElem.Attr("title"); exists && title != "" {
					if t := parseTime(title); !t.IsZero() {
						item.UploadedAt = t.Unix()
					}
				}
				// Fallback to text content
				if item.UploadedAt == 0 {
					timeText := strings.TrimSpace(uploadTimeElem.Text())
					if t := parseTime(timeText); !t.IsZero() {
						item.UploadedAt = t.Unix()
					}
				}
			}
		}

		// Check for H&R
		hrElem := s.Find(d.Selectors.HRIcon)
		item.HasHR = hrElem.Length() > 0

		items = append(items, item)
	})

	return items, nil
}

// TorrentDetail contains detailed information from a torrent detail page
type TorrentDetail struct {
	// DownloadURL is the direct download URL with passkey
	DownloadURL string `json:"downloadUrl"`
	// Subtitle is the torrent subtitle
	Subtitle string `json:"subtitle"`
	// InfoHash is the torrent info hash
	InfoHash string `json:"infoHash,omitempty"`
}

// PrepareDetail prepares a request for torrent detail page
func (d *NexusPHPDriver) PrepareDetail(torrentID string) (NexusPHPRequest, error) {
	params := url.Values{}
	params.Set("id", torrentID)
	params.Set("hit", "1")
	return NexusPHPRequest{
		Path:   "/details.php",
		Params: params,
		Method: "GET",
	}, nil
}

// ParseDetail extracts download URL and other info from detail page
func (d *NexusPHPDriver) ParseDetail(res NexusPHPResponse) (TorrentDetail, error) {
	if res.Document == nil {
		return TorrentDetail{}, ErrParseError
	}

	detail := TorrentDetail{}
	doc := res.Document

	// Try to find download link using multiple strategies
	// Strategy 1: Look for "下载链接" row with direct link
	downloadLinkSelectors := []string{
		"td.rowhead:contains('下载链接') + td a[href*='download.php']",
		"td.rowhead:contains('下載連結') + td a[href*='download.php']",
		"td.rowhead:contains('下载') + td a[href*='download.php']",
		// Generic download link patterns
		"a[href*='download.php?id=']",
		"a[href*='download.php?hash=']",
		"a.download[href*='download']",
	}
	for _, sel := range downloadLinkSelectors {
		elem := doc.Find(sel).First()
		if elem.Length() > 0 {
			if href, exists := elem.Attr("href"); exists && !strings.Contains(href, "type=zip") {
				detail.DownloadURL = href
				break
			}
		}
	}

	// Strategy 2: Look for download form action
	if detail.DownloadURL == "" {
		formSelectors := []string{
			"form[action*='download.php']:not([action*='type=zip'])",
			"td.rowhead:contains('下载') + td form[action*='download.php']",
			"td.rowhead:contains('行为') + td form[action*='download.php']",
		}
		for _, sel := range formSelectors {
			elem := doc.Find(sel).First()
			if elem.Length() > 0 {
				if action, exists := elem.Attr("action"); exists && !strings.Contains(action, "type=zip") {
					detail.DownloadURL = action
					break
				}
			}
		}
	}

	// Strategy 3: Look for any download.php link with id/passkey in URL
	if detail.DownloadURL == "" {
		doc.Find("a[href*='download.php']").Each(func(i int, s *goquery.Selection) {
			if detail.DownloadURL != "" {
				return // Already found
			}
			if href, exists := s.Attr("href"); exists {
				// Skip zip downloads
				if strings.Contains(href, "type=zip") {
					return
				}
				// Prefer links with passkey (full download URL)
				if strings.Contains(href, "passkey=") || strings.Contains(href, "hash=") {
					detail.DownloadURL = href
				}
			}
		})
	}

	// Strategy 4: If still not found, look for any download.php link with id parameter
	if detail.DownloadURL == "" {
		doc.Find("a[href*='download.php']").Each(func(i int, s *goquery.Selection) {
			if detail.DownloadURL != "" {
				return
			}
			if href, exists := s.Attr("href"); exists {
				if strings.Contains(href, "type=zip") {
					return
				}
				if strings.Contains(href, "id=") {
					detail.DownloadURL = href
				}
			}
		})
	}

	// Use custom selector if defined
	if detail.DownloadURL == "" && d.Selectors.DetailDownloadLink != "" {
		selectors := strings.Split(d.Selectors.DetailDownloadLink, ",")
		for _, sel := range selectors {
			sel = strings.TrimSpace(sel)
			elem := doc.Find(sel).First()
			if elem.Length() > 0 {
				// Check if it's a form or a link
				if elem.Is("form") {
					if action, exists := elem.Attr("action"); exists {
						detail.DownloadURL = action
						break
					}
				} else if href, exists := elem.Attr("href"); exists {
					detail.DownloadURL = href
					break
				}
			}
		}
	}

	// Parse subtitle
	subtitleSelectors := []string{
		"td.rowhead:contains('副标题') + td",
		"td.rowhead:contains('副標題') + td",
		"td.rowhead:contains('小标题') + td",
	}
	if d.Selectors.DetailSubtitle != "" {
		subtitleSelectors = append([]string{d.Selectors.DetailSubtitle}, subtitleSelectors...)
	}
	for _, sel := range subtitleSelectors {
		elem := doc.Find(sel).First()
		if elem.Length() > 0 {
			detail.Subtitle = strings.TrimSpace(elem.Text())
			if detail.Subtitle != "" {
				break
			}
		}
	}

	// Parse info hash
	hashSelectors := []string{
		"td:contains('Hash码') + td",
		"td:contains('Hash码:') ~ td",
		"td.no_border_wide:contains('Hash码')",
	}
	for _, sel := range hashSelectors {
		elem := doc.Find(sel).First()
		if elem.Length() > 0 {
			text := strings.TrimSpace(elem.Text())
			// Extract hash from text like "Hash码: 303a850dedc19e60bd7cc814f60e0e28d7f2c202"
			if strings.Contains(text, "Hash码") {
				parts := strings.Split(text, ":")
				if len(parts) >= 2 {
					text = strings.TrimSpace(parts[len(parts)-1])
				}
			}
			// Validate it looks like a hash (40 hex chars for SHA1)
			if len(text) == 40 && isHexString(text) {
				detail.InfoHash = text
				break
			}
		}
	}

	return detail, nil
}

// isHexString checks if a string contains only hexadecimal characters
func isHexString(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// PrepareUserInfo prepares a request for user info (index page to get user ID)
func (d *NexusPHPDriver) PrepareUserInfo() (NexusPHPRequest, error) {
	return NexusPHPRequest{
		Path:   "/index.php",
		Method: "GET",
	}, nil
}

// PrepareUserDetails prepares a request for user details page
func (d *NexusPHPDriver) PrepareUserDetails(userID string) (NexusPHPRequest, error) {
	params := url.Values{}
	params.Set("id", userID)
	return NexusPHPRequest{
		Path:   "/userdetails.php",
		Params: params,
		Method: "GET",
	}, nil
}

// ParseUserInfo extracts user info from the response
func (d *NexusPHPDriver) ParseUserInfo(res NexusPHPResponse) (UserInfo, error) {
	if res.Document == nil {
		return UserInfo{}, ErrParseError
	}

	info := UserInfo{
		LastUpdate: time.Now().Unix(),
	}

	doc := res.Document

	// Parse username - look for specific user link in info block
	// Try multiple selectors for username
	usernameSelectors := []string{
		"#info_block a.User_Name",
		"#info_block a[href*='userdetails.php']",
		"a.User_Name",
		"#userbar a[href*='userdetails.php']",
		"table.mainouter a[href*='userdetails.php']:first",
	}

	for _, sel := range usernameSelectors {
		elem := doc.Find(sel).First()
		if elem.Length() > 0 {
			info.Username = strings.TrimSpace(elem.Text())
			// Also try to extract user ID from href
			if href, exists := elem.Attr("href"); exists {
				info.UserID = extractUserID(href)
			}
			break
		}
	}

	// Parse uploaded - look in info block
	uploadedText := findTextByLabel(doc, "上传量", "上傳量", "Uploaded", "上传")
	if uploadedText == "" {
		// Try to find in info_block format: 上传量: xxx
		uploadedText = findInfoBlockValue(doc, "上传量", "上傳量", "Uploaded")
	}
	info.Uploaded = parseSize(uploadedText)

	// Parse downloaded
	downloadedText := findTextByLabel(doc, "下载量", "下載量", "Downloaded", "下载")
	if downloadedText == "" {
		downloadedText = findInfoBlockValue(doc, "下载量", "下載量", "Downloaded")
	}
	info.Downloaded = parseSize(downloadedText)

	// Parse ratio
	ratioText := findTextByLabel(doc, "分享率", "分享率", "Ratio")
	if ratioText == "" {
		ratioText = findInfoBlockValue(doc, "分享率", "Ratio")
	}
	info.Ratio = parseRatio(ratioText)

	// Parse bonus
	bonusText := findTextByLabel(doc, "魔力值", "魔力", "Bonus")
	if bonusText == "" {
		bonusText = findInfoBlockValue(doc, "魔力值", "魔力", "Bonus")
	}
	info.Bonus = parseFloat(bonusText)

	// Parse rank/level
	rankText := findTextByLabel(doc, "等级", "等級", "Class")
	if rankText == "" {
		rankText = findInfoBlockValue(doc, "等级", "等級", "Class")
	}
	info.Rank = strings.TrimSpace(rankText)

	return info, nil
}

// ParseUserDetails extracts detailed user info from userdetails.php page
func (d *NexusPHPDriver) ParseUserDetails(res NexusPHPResponse) (UserInfo, error) {
	if res.Document == nil {
		return UserInfo{}, ErrParseError
	}

	info := UserInfo{
		LastUpdate: time.Now().Unix(),
	}

	doc := res.Document

	// Parse username from page
	usernameElem := doc.Find("h1, #username, .username, td.rowhead:contains('用户名') + td, td.rowhead:contains('Username') + td").First()
	if usernameElem.Length() > 0 {
		info.Username = strings.TrimSpace(usernameElem.Text())
	}

	// Parse from table rows - NexusPHP style
	doc.Find("tr").Each(func(i int, row *goquery.Selection) {
		header := strings.TrimSpace(row.Find("td.rowhead").First().Text())
		value := strings.TrimSpace(row.Find("td.rowfollow").First().Text())

		// Clean up value - remove extra whitespace
		value = regexp.MustCompile(`\s+`).ReplaceAllString(value, " ")

		switch {
		case containsAny(header, "用户名", "Username"):
			if info.Username == "" {
				info.Username = value
			}
		case containsAny(header, "传输", "传送", "Transfers", "流量"):
			// Format: "上传量: 1.5 TB 下载量: 500 GB 分享率: 3.0"
			info.Uploaded = extractSizeFromTransfer(value, "上传量", "上傳量", "Uploaded", "上传")
			info.Downloaded = extractSizeFromTransfer(value, "下载量", "下載量", "Downloaded", "下载")
			ratioStr := extractValueFromTransfer(value, "分享率", "Ratio")
			if ratioStr != "" {
				info.Ratio = parseRatio(ratioStr)
			}
		case containsAny(header, "上传量", "Uploaded"):
			info.Uploaded = parseSize(value)
		case containsAny(header, "下载量", "Downloaded"):
			info.Downloaded = parseSize(value)
		case containsAny(header, "分享率", "Ratio"):
			info.Ratio = parseRatio(value)
		case containsAny(header, "魔力值", "魔力", "Bonus"):
			// Extract number from value like "123,456 (详情)"
			info.Bonus = parseFloat(extractNumber(value))
		case containsAny(header, "等级", "Class"):
			info.Rank = value
		case containsAny(header, "做种积分", "Seeding"):
			info.Seeding, _ = strconv.Atoi(extractNumber(value))
		case containsAny(header, "加入日期", "Join"):
			// Parse join date if needed
		case containsAny(header, "上次访问", "上次訪問", "Last access", "Last seen"):
			if t, err := ParseTimeInCST("2006-01-02 15:04:05", value); err == nil {
				info.LastAccess = t.Unix()
			}
		case containsAny(header, "上次登录", "上次登錄", "Last login"):
			if t, err := ParseTimeInCST("2006-01-02 15:04:05", value); err == nil {
				info.LastLogin = t.Unix()
			}
		}
	})

	return info, nil
}

// GetUserInfo fetches complete user information
// For NexusPHP sites, this involves two steps:
// 1. Fetch /index.php to get user ID and basic info from info_block
// 2. Fetch /userdetails.php?id=xxx to get detailed info
func (d *NexusPHPDriver) GetUserInfo(ctx context.Context) (UserInfo, error) {
	// If we have a site definition with UserInfo config, use the definition-based parsing
	if d.siteDefinition != nil && d.siteDefinition.UserInfo != nil {
		return d.getUserInfoWithDefinition(ctx)
	}

	// Fall back to legacy parsing
	return d.getUserInfoLegacy(ctx)
}

// getUserInfoWithDefinition fetches user info using site definition selectors.
// 先取不依赖别的页面的（index.php 这类），再取要用户 ID 的（userdetails.php）与做种情况。定义里写了 RequestDelay 的站点
// 一页一页地取、中间隔 RequestDelay（几页同时请求容易招来 Cloudflare 质询），其余的并发。每一页一时的网络错误与质询先重试
// （fetchUserInfoPage）；还是没取到的页面不让整次失败：用户名到手了就回取到的部分，错误是 *PartialUserInfoError，
// 写明缺哪些字段（保存时沿用上一次的）。登录失效、要二次验证时整次失败。
func (d *NexusPHPDriver) getUserInfoWithDefinition(ctx context.Context) (UserInfo, error) {
	startTime := time.Now()
	def := d.siteDefinition
	uiConfig := def.UserInfo

	info := UserInfo{
		Site:       def.ID,
		LastUpdate: time.Now().Unix(),
	}

	// Store parsed values for use in subsequent requests
	parsedValues := make(map[string]any)
	var mu sync.Mutex // Protect parsedValues, info and the failures below
	// 没取到的页面：缺的字段与第一个失败的原因；got 是别的页取到了的字段（几页都写了的字段，比如用户名，有一页取到就不算缺）
	var missing []string
	var firstErr error
	got := make(map[string]bool)
	fail := func(fields []string, err error) {
		mu.Lock()
		defer mu.Unlock()
		for _, f := range fields {
			if !slices.Contains(missing, f) {
				missing = append(missing, f)
			}
		}
		if firstErr == nil {
			firstErr = err
		}
	}

	// First pass: identify which processes have dependencies
	independentProcesses := []int{}
	dependentProcesses := []int{}

	for i, process := range uiConfig.Process {
		hasDependency := false
		if process.Assertion != nil {
			for _, valueRef := range process.Assertion {
				if strings.HasPrefix(valueRef, "params.") {
					hasDependency = true
					break
				}
			}
		}

		if hasDependency {
			dependentProcesses = append(dependentProcesses, i)
		} else {
			independentProcesses = append(independentProcesses, i)
		}
	}

	delay := time.Duration(uiConfig.RequestDelay) * time.Millisecond
	sequential := delay > 0
	// processStep 取一页：取到的值并进 parsedValues 与 info；没取到时记下缺的字段，登录失效这类错误中止整次获取。
	// 调用方的时限已经到了（或取消了）时不再请求，这一页算缺
	processStep := func(idx int, params map[string]any) func(context.Context) error {
		process := uiConfig.Process[idx]
		return func(ctx context.Context) error {
			if err := ctx.Err(); err != nil {
				fail(process.Fields, err)
				return nil
			}
			values, err := d.executeProcess(ctx, uiConfig, process, params)
			if err != nil {
				if fatalUserInfoError(err) {
					return err
				}
				fail(process.Fields, err)
			}
			mu.Lock()
			for k, v := range values {
				parsedValues[k] = v
				d.setUserInfoField(&info, k, v)
				got[k] = true
			}
			mu.Unlock()
			return nil
		}
	}

	// Phase 1: processes without dependencies
	phase1Start := time.Now()
	if len(independentProcesses) > 0 {
		if DebugUserInfo {
			fmt.Printf("[DEBUG] Phase 1: Executing %d independent processes (sequential=%v)\n", len(independentProcesses), sequential)
		}
		steps := make([]func(context.Context) error, 0, len(independentProcesses))
		for _, idx := range independentProcesses {
			steps = append(steps, processStep(idx, parsedValues))
		}
		if err := runUserInfoSteps(ctx, sequential, delay, steps); err != nil {
			return UserInfo{}, fmt.Errorf("phase 1 failed: %w", err)
		}
	}
	if DebugUserInfo {
		fmt.Printf("[DEBUG] Phase 1 completed in %v\n", time.Since(phase1Start))
	}

	// Phase 2: processes that need values from phase 1, and the seeding status
	phase2Start := time.Now()
	needSeedingStatus := false
	if uiConfig.Selectors != nil {
		_, hasSeedingSizeSelector := uiConfig.Selectors["seedingSize"]
		needSeedingStatus = !hasSeedingSizeSelector
	} else {
		needSeedingStatus = true
	}

	if len(dependentProcesses) > 0 || (needSeedingStatus && info.UserID != "") {
		// 第二轮的请求只读第一轮的结果：给一份拷贝，并发写 parsedValues 时不会和读撞上
		params := make(map[string]any, len(parsedValues))
		for k, v := range parsedValues {
			params[k] = v
		}
		steps := make([]func(context.Context) error, 0, len(dependentProcesses)+1)
		for _, idx := range dependentProcesses {
			steps = append(steps, processStep(idx, params))
		}
		if needSeedingStatus && info.UserID != "" {
			userID := info.UserID // capture
			steps = append(steps, func(ctx context.Context) error {
				if err := ctx.Err(); err != nil {
					fail([]string{"seederSize"}, err)
					return nil
				}
				seeding, seedingSize, err := d.FetchSeedingStatus(ctx, userID)
				if err != nil {
					if DebugUserInfo {
						fmt.Printf("[DEBUG] FetchSeedingStatus error: %v\n", err)
					}
					if fatalUserInfoError(err) {
						return err
					}
					// 做种体积一时没取到（质询、网络错误、站点出错、时限到了）：不让整次失败，保存时沿用上一次的。
					// 站点没有做种列表这一页（404 这类）时和以前一样不管它
					if temporaryFetchError(err) || ctx.Err() != nil {
						fail([]string{"seederSize"}, withContextErr(ctx, err))
					}
					return nil
				}
				if seedingSize > 0 {
					mu.Lock()
					info.SeederSize = seedingSize
					if seeding > 0 && info.Seeding == 0 {
						info.Seeding = seeding
						info.SeederCount = seeding
					}
					mu.Unlock()
					if DebugUserInfo {
						fmt.Printf("[DEBUG] Updated seeding status: count=%d, size=%d\n", seeding, seedingSize)
					}
				}
				return nil
			})
		}
		// 两轮之间也隔 RequestDelay（时限到了就不等了：第二轮的每一页自己记下没取到）
		if len(independentProcesses) > 0 && delay > 0 {
			sleepContext(ctx, delay)
		}
		if err := runUserInfoSteps(ctx, sequential, delay, steps); err != nil {
			return UserInfo{}, fmt.Errorf("phase 2 failed: %w", err)
		}
	}
	if DebugUserInfo {
		fmt.Printf("[DEBUG] Phase 2 completed in %v\n", time.Since(phase2Start))
	}

	if strings.TrimSpace(info.Username) == "" && firstErr != nil {
		// 一页都没取到用户名：按第一个失败的原因报（质询、网络错误、HTTP 出错）
		return UserInfo{}, fmt.Errorf("获取用户信息失败: %w", firstErr)
	}

	// Calculate ratio if not set
	if info.Ratio == 0 && info.Downloaded > 0 {
		info.Ratio = float64(info.Uploaded) / float64(info.Downloaded)
	}

	if DebugUserInfo {
		fmt.Printf("[DEBUG] getUserInfoWithDefinition total time: %v\n", time.Since(startTime))
	}

	missing = slices.DeleteFunc(missing, func(f string) bool { return got[f] })
	if len(missing) > 0 {
		return info, &PartialUserInfoError{Missing: missing, Err: firstErr}
	}
	return info, nil
}

// temporaryFetchError 是一时取不到：质询、网络错误、限流、站点出错（5xx）。
func temporaryFetchError(err error) bool {
	var status *HTTPStatusError
	return errors.Is(err, ErrCloudflareChallenge) || errors.Is(err, ErrNetworkError) || errors.Is(err, ErrRateLimited) ||
		(errors.As(err, &status) && status.Code >= 500)
}

// fatalUserInfoError 是要中止整次获取的错误：登录失效、凭证无效、要二次验证（别的页面也一样取不到）。
func fatalUserInfoError(err error) bool {
	return errors.Is(err, ErrSessionExpired) || errors.Is(err, ErrInvalidCredentials) || errors.Is(err, Err2FARequired)
}

// runUserInfoSteps 跑一组请求：sequential 时一个接一个、中间隔 delay，否则并发。返回第一个要中止整次获取的错误。
// 调用方的时限到了不算中止：剩下的每一步自己记下没取到（见 processStep），用户名到手了照样回取到的部分。
func runUserInfoSteps(ctx context.Context, sequential bool, delay time.Duration, steps []func(context.Context) error) error {
	if !sequential {
		g, gctx := errgroup.WithContext(ctx)
		for _, step := range steps {
			g.Go(func() error { return step(gctx) })
		}
		return g.Wait()
	}
	for i, step := range steps {
		if i > 0 {
			sleepContext(ctx, delay)
		}
		if err := step(ctx); err != nil {
			return err
		}
	}
	return nil
}

// fetchUserInfoPage 取用户信息的一页：一次请求最多等 userInfoAttemptTimeout；一时的网络错误（连接被重置、断开、超时）
// 隔一会儿再试（最多 userInfoNetRetries 次），Cloudflare 质询隔几秒再试一次。剩下的时间不够时不再试。
func (d *NexusPHPDriver) fetchUserInfoPage(ctx context.Context, req NexusPHPRequest) (NexusPHPResponse, error) {
	netTries, challengeTries := 0, 0
	for {
		actx, cancel := withAttemptTimeout(ctx)
		res, err := d.Execute(actx, req)
		cancel()
		if err == nil || ctx.Err() != nil {
			return res, withContextErr(ctx, err)
		}
		wait, again := retryWait(err, netTries, challengeTries)
		if !again {
			return res, err
		}
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < wait+userInfoMinAttempt {
			return res, err
		}
		if errors.Is(err, ErrCloudflareChallenge) {
			challengeTries++
		} else {
			netTries++
		}
		if DebugUserInfo {
			fmt.Printf("[DEBUG] %s: %v, retry in %v\n", req.Path, err, wait)
		}
		if !sleepContext(ctx, wait) {
			return res, withContextErr(ctx, err)
		}
	}
}

// withContextErr 在调用方的时限到了（或取消了）时把 ctx 的错误并进 err：原因分类按超时算（FetchFailureReason）。
func withContextErr(ctx context.Context, err error) error {
	ctxErr := ctx.Err()
	if err == nil || ctxErr == nil || errors.Is(err, ctxErr) {
		return err
	}
	return fmt.Errorf("%w: %w", ctxErr, err)
}

// executeProcess executes a single process and returns the parsed values
func (d *NexusPHPDriver) executeProcess(ctx context.Context, uiConfig *UserInfoConfig, process UserInfoProcess, parsedValues map[string]any) (map[string]string, error) {
	result := make(map[string]string)

	// Build request URL
	reqURL := process.RequestConfig.URL

	// Replace params in URL (e.g., params.id)
	if process.Assertion != nil {
		for key, valueRef := range process.Assertion {
			if strings.HasPrefix(valueRef, "params.") {
				paramKey := strings.TrimPrefix(valueRef, "params.")
				if val, ok := parsedValues[paramKey]; ok {
					reqURL = strings.ReplaceAll(reqURL, "{"+key+"}", toString(val))
					// Also add as query param if needed
					if !strings.Contains(reqURL, "?") {
						reqURL += "?" + key + "=" + toString(val)
					} else {
						reqURL += "&" + key + "=" + toString(val)
					}
				}
			}
		}
	}

	// Execute request
	req := NexusPHPRequest{
		Path:   reqURL,
		Method: "GET",
	}

	res, err := d.fetchUserInfoPage(ctx, req)
	if err != nil {
		// 交给调用方：登录失效这类中止整次获取；别的（质询、网络错误、HTTP 出错）只是少了这一页的字段
		return result, err
	}

	if res.Document == nil {
		return result, nil
	}

	// Parse fields for this request
	for _, fieldName := range process.Fields {
		selector, ok := uiConfig.Selectors[fieldName]
		if !ok {
			// Field not in selectors, skip
			continue
		}

		value := d.extractFieldValue(res.Document, selector)
		if DebugUserInfo {
			fmt.Printf("[DEBUG] Field %s: rawValue=%q, selectors=%v\n", fieldName, truncateStr(value, 100), selector.Selector)
		}
		if value != "" || selector.Text != "" {
			result[fieldName] = value
		}
	}

	// 定义里没写 lastAccessAt 的站点，在用户详情页上按 NexusPHP 的通用写法找「最近动向」：
	// 保号探测靠它判断最近一次访问，取不到就报解析失败（AGSVPT、星陨阁等十几个站点都是这样）。
	if !declaresLastAccess(uiConfig) && strings.Contains(reqURL, "userdetails.php") {
		if v := d.extractFieldValue(res.Document, defaultLastAccessSelector); v != "" && v != "0" {
			result["lastAccessAt"] = v
		}
	}

	// 同样，定义里没写实际上传 / 下载的站点，在用户详情页「传输」一行按通用写法找（#332：PTtime 等页面上有、定义里没写的站点）。
	if strings.Contains(reqURL, "userdetails.php") {
		for _, def := range defaultTrueTransferSelectors {
			if _, declared := uiConfig.Selectors[def.field]; declared {
				continue
			}
			if v := d.extractFieldValue(res.Document, def.selector); v != "" && v != "0" {
				result[def.field] = v
			}
		}
	}

	return result, nil
}

// transferRows 是 NexusPHP 用户详情页「传输」一行的几种写法
var transferRows = []string{
	"td.rowhead:contains('传输') + td",
	"td.rowhead:contains('傳輸') + td",
	"td.rowhead:contains('傳送') + td",
}

// defaultTrueTransferSelectors 是「传输」一行里实际（真实）上传、下载量的通用写法：标签后面跟着冒号和大小。
var defaultTrueTransferSelectors = []struct {
	field    string
	selector FieldSelector
}{
	{"trueUploaded", FieldSelector{
		Selector: transferRows,
		Attr:     "html",
		Filters: []Filter{
			{Name: "regex", Args: []any{`(?:实际上传量|實際上傳量|实际上传|實際上傳|真实上传量|真實上傳量|真实上传|真實上傳)(?:</strong>|</b>)?(?:[\s:：\x{00a0}]|&nbsp;)*([\d.,]+\s*[KMGTP]?i?B)`}},
			{Name: "parseSize"},
		},
	}},
	{"trueDownloaded", FieldSelector{
		Selector: transferRows,
		Attr:     "html",
		Filters: []Filter{
			{Name: "regex", Args: []any{`(?:实际下载量|實際下載量|实际下载|實際下載|真实下载量|真實下載量|真实下载|真實下載)(?:</strong>|</b>)?(?:[\s:：\x{00a0}]|&nbsp;)*([\d.,]+\s*[KMGTP]?i?B)`}},
			{Name: "parseSize"},
		},
	}},
}

// defaultLastAccessSelector 是 NexusPHP 用户详情页「最近动向 / 上次访问」一行的通用写法（简繁与英文界面）。
var defaultLastAccessSelector = FieldSelector{
	Selector: []string{
		"td.rowhead:contains('最近动向') + td",
		"td.rowhead:contains('最近動向') + td",
		"td.rowhead:contains('最后活动') + td",
		"td.rowhead:contains('最後活動') + td",
		"td.rowhead:contains('最后访问') + td",
		"td.rowhead:contains('最後訪問') + td",
		"td.rowhead:contains('上次访问') + td",
		"td.rowhead:contains('上次訪問') + td",
		"td.rowhead:contains('Last access') + td",
		"td.rowhead:contains('Last seen') + td",
	},
	Filters: []Filter{
		{Name: "regex", Args: []any{`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
		{Name: "parseTime"},
	},
}

// declaresLastAccess 报告站点定义自己有没有写最近访问时间的选择器（写了就不用通用写法）。
func declaresLastAccess(uiConfig *UserInfoConfig) bool {
	if uiConfig == nil {
		return false
	}
	_, a := uiConfig.Selectors["lastAccessAt"]
	_, b := uiConfig.Selectors["lastAccess"]
	return a || b
}

// extractFieldValue extracts a field value from the document using the selector config
func (d *NexusPHPDriver) extractFieldValue(doc *goquery.Document, selector FieldSelector) string {
	return d.extractFieldValueFromSelection(doc.Selection, selector)
}

func (d *NexusPHPDriver) extractFieldValueFromSelection(root *goquery.Selection, selector FieldSelector) string {
	var value string
	var matchedSelector string

	// Try each selector until one matches
	for _, sel := range selector.Selector {
		elem := root.Find(sel).First()
		if elem.Length() == 0 {
			if DebugUserInfo {
				fmt.Printf("[DEBUG]   Selector %q: no match\n", sel)
			}
			continue
		}

		matchedSelector = sel

		// Get value based on attribute, html, or text
		if selector.Attr != "" {
			if selector.Attr == "html" || selector.Attr == "innerHTML" {
				// Get inner HTML for regex matching against HTML structure
				html, err := elem.Html()
				if err == nil {
					value = html
				}
			} else {
				value, _ = elem.Attr(selector.Attr)
			}
		} else {
			value = strings.TrimSpace(elem.Text())
		}

		if value != "" {
			if DebugUserInfo {
				fmt.Printf("[DEBUG]   Selector %q: matched, rawValue=%q\n", sel, truncateStr(value, 200))
			}
			break
		}
	}

	// Use default text if no value found
	if value == "" && selector.Text != "" {
		value = selector.Text
		if DebugUserInfo {
			fmt.Printf("[DEBUG]   Using default text: %q\n", value)
		}
	}

	// Apply filters
	if len(selector.Filters) > 0 && value != "" {
		beforeFilter := value
		result := ApplyFilters(value, selector.Filters)
		filteredValue := toString(result)
		if DebugUserInfo {
			fmt.Printf("[DEBUG]   Filters %v: %q -> %q (selector: %s)\n", filterNames(selector.Filters), truncateStr(beforeFilter, 100), filteredValue, matchedSelector)
		}
		value = filteredValue
	}

	return value
}

// ExtractFieldValuePublic is a public wrapper for extractFieldValue for testing purposes
func (d *NexusPHPDriver) ExtractFieldValuePublic(doc *goquery.Document, selector FieldSelector) string {
	return d.extractFieldValue(doc, selector)
}

// filterNames returns filter names for debug output
func filterNames(filters []Filter) []string {
	names := make([]string, len(filters))
	for i, f := range filters {
		names[i] = f.Name
	}
	return names
}

// setUserInfoField sets a field on UserInfo based on field name
func (d *NexusPHPDriver) setUserInfoField(info *UserInfo, fieldName, value string) {
	switch fieldName {
	case "id":
		info.UserID = value
	case "name", "username":
		info.Username = value
	case "uploaded":
		info.Uploaded = parseSize(value)
	case "downloaded":
		info.Downloaded = parseSize(value)
	case "ratio":
		info.Ratio = parseRatio(value)
	case "bonus":
		info.Bonus = parseFloat(value)
	case "levelName", "rank", "class":
		info.LevelName = value
		info.Rank = value
	case "seedingBonus":
		info.SeedingBonus = parseFloat(value)
	case "bonusPerHour":
		info.BonusPerHour = parseFloat(value)
	case "seedingBonusPerHour":
		info.SeedingBonusPerHour = parseFloat(value)
	case "joinTime", "joinDate":
		// Value should already be Unix timestamp after parseTime filter
		if ts, err := strconv.ParseInt(value, 10, 64); err == nil {
			info.JoinDate = ts
		}
	case "lastAccessAt", "lastAccess":
		if ts, err := strconv.ParseInt(value, 10, 64); err == nil {
			info.LastAccess = ts
		}
	case "lastLoginAt", "lastLogin":
		if ts, err := strconv.ParseInt(value, 10, 64); err == nil {
			info.LastLogin = ts
		}
	case "messageCount", "unreadMessageCount":
		if count, err := strconv.Atoi(value); err == nil {
			info.UnreadMessageCount = count
		}
	case "hnrUnsatisfied":
		if count, err := strconv.Atoi(value); err == nil {
			info.HnRUnsatisfied = count
		}
	case "hnrPreWarning":
		if count, err := strconv.Atoi(value); err == nil {
			info.HnRPreWarning = count
		}
	case "seeding", "seederCount":
		if count, err := strconv.Atoi(value); err == nil {
			info.Seeding = count
			info.SeederCount = count
		}
	case "leeching", "leecherCount":
		if count, err := strconv.Atoi(value); err == nil {
			info.Leeching = count
			info.LeecherCount = count
		}
	case "uploads":
		if count, err := strconv.Atoi(value); err == nil {
			info.Uploads = count
		}
	case "trueUploaded":
		info.TrueUploaded = parseSize(value)
	case "trueDownloaded":
		info.TrueDownloaded = parseSize(value)
	case "seederSize":
		info.SeederSize = parseSize(value)
	case "leecherSize":
		info.LeecherSize = parseSize(value)
	}
}

// getUserInfoLegacy is the original implementation without site definitions
func (d *NexusPHPDriver) getUserInfoLegacy(ctx context.Context) (UserInfo, error) {
	// Step 1: Fetch index page to get user ID
	req, err := d.PrepareUserInfo()
	if err != nil {
		return UserInfo{}, err
	}

	res, err := d.Execute(ctx, req)
	if err != nil {
		return UserInfo{}, err
	}

	// Debug: log raw HTML response
	if len(res.RawBody) > 0 {
		// Print first 3000 chars of response for debugging
		bodyStr := string(res.RawBody)
		if len(bodyStr) > 3000 {
		} else {
		}
	}

	// Parse index page to get user ID and basic info
	info, err := d.ParseUserInfo(res)
	if err != nil {
		return UserInfo{}, err
	}

	// If we got a user ID, fetch detailed info from userdetails.php
	if info.UserID != "" {

		detailReq, err := d.PrepareUserDetails(info.UserID)
		if err != nil {
			return info, nil // Return basic info if we can't get details
		}

		detailRes, err := d.Execute(ctx, detailReq)
		if err != nil {
			return info, nil // Return basic info if we can't get details
		}

		// Debug: log userdetails response
		if len(detailRes.RawBody) > 0 {
			bodyStr := string(detailRes.RawBody)
			if len(bodyStr) > 3000 {
			} else {
			}
		}

		// Parse detailed info
		detailInfo, err := d.ParseUserDetails(detailRes)
		if err != nil {
			return info, nil
		}

		// Merge detailed info into basic info (prefer detailed values if available)
		if detailInfo.Username != "" {
			info.Username = detailInfo.Username
		}
		if detailInfo.Uploaded > 0 {
			info.Uploaded = detailInfo.Uploaded
		}
		if detailInfo.Downloaded > 0 {
			info.Downloaded = detailInfo.Downloaded
		}
		if detailInfo.Ratio > 0 {
			info.Ratio = detailInfo.Ratio
		}
		if detailInfo.Bonus > 0 {
			info.Bonus = detailInfo.Bonus
		}
		if detailInfo.Rank != "" {
			info.Rank = detailInfo.Rank
		}
		if detailInfo.Seeding > 0 {
			info.Seeding = detailInfo.Seeding
		}
	} else {
	}

	return info, nil
}

// PrepareDownload prepares a request for downloading a torrent
// For NexusPHP sites, we first need to visit the detail page to get the download URL with passkey
func (d *NexusPHPDriver) PrepareDownload(torrentID string) (NexusPHPRequest, error) {
	params := url.Values{}
	params.Set("id", torrentID)
	params.Set("hit", "1")

	// First, we request the detail page to get the download URL with passkey
	return NexusPHPRequest{
		Path:   "/details.php",
		Params: params,
		Method: "GET",
	}, nil
}

// ParseDownload extracts torrent file data from the response
// For NexusPHP, the response is a detail page - we need to extract the download URL and fetch the torrent
func (d *NexusPHPDriver) ParseDownload(res NexusPHPResponse) ([]byte, error) {
	return d.ParseDownloadContext(context.Background(), res)
}

// nexusTorrentFetchTimeout 是从详情页取到下载地址以后拉一次种子文件的超时；测试里会调小。
var nexusTorrentFetchTimeout = 30 * time.Second

// ParseDownloadContext 同 ParseDownload：拉种子文件的第二段请求跟着 ctx 取消，一时的错误隔几秒重拉同一个下载地址
// （见 retryDownload，不再重取详情页）。BaseSite 下载时用它。
func (d *NexusPHPDriver) ParseDownloadContext(ctx context.Context, res NexusPHPResponse) ([]byte, error) {
	if res.Document == nil {
		// If we have raw body (torrent file directly), return it
		if len(res.RawBody) > 0 {
			return res.RawBody, nil
		}
		return nil, ErrParseError
	}

	// Parse detail page to get download URL with passkey
	detail, err := d.ParseDetail(res)
	if err != nil {
		return nil, fmt.Errorf("parse detail page: %w", err)
	}

	if detail.DownloadURL == "" {
		return nil, fmt.Errorf("no download URL found in detail page")
	}

	downloadURL, sameSite, err := d.resolveDetailDownloadURL(detail.DownloadURL)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"User-Agent":      d.userAgent,
		"Accept":          "application/x-bittorrent,*/*",
		"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
		"Referer":         d.BaseURL + "/",
	}
	// 链接取自详情页，简介等区域是用户内容；只有指向站点自己的域名时才附带 Cookie，
	// 外域链接照常请求（passkey 下载链接不依赖 Cookie），但不把站点 Cookie 交给第三方。
	if sameSite {
		headers["Cookie"] = d.Cookie
	}
	return retryDownload(ctx, nil, func() ([]byte, error) { return d.fetchTorrentOnce(ctx, downloadURL, headers) })
}

// fetchTorrentOnce 拉一次种子文件（每次设上限超时）。下载地址带 passkey：错误里的地址一律脱敏。
func (d *NexusPHPDriver) fetchTorrentOnce(ctx context.Context, downloadURL string, headers map[string]string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, nexusTorrentFetchTimeout)
	defer cancel()

	resp, err := d.httpClient.Get(ctx, downloadURL, headers)
	if err != nil {
		return nil, fmt.Errorf("fetch torrent file: %w", redactURLsInError(err))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w fetching torrent from %s", &HTTPStatusError{Code: resp.StatusCode}, utils.SanitizeURL(downloadURL))
	}

	if len(resp.Body) == 0 {
		return nil, fmt.Errorf("empty torrent file response")
	}

	return resp.Body, nil
}

// Helper functions

// resolveDetailDownloadURL 把详情页里的下载链接补成绝对地址，并判断它是否指向站点自己的域名。
// 相对链接沿用原来的拼接方式；绝对链接只接受 http/https。
func (d *NexusPHPDriver) resolveDetailDownloadURL(href string) (string, bool, error) {
	href = strings.TrimSpace(href)
	if u, err := url.Parse(href); err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") {
		return u.String(), d.isSiteHost(u.Hostname()), nil
	} else if err == nil && u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https" {
		return "", false, fmt.Errorf("unsupported download URL scheme %q", u.Scheme)
	}
	if strings.HasPrefix(href, "/") {
		return d.BaseURL + href, true, nil
	}
	return d.BaseURL + "/" + href, true, nil
}

// isSiteHost 判断 host 是否属于本站：与 BaseURL 或站点定义里任一地址同主机，或同属一个可注册域名
// （如 pt.example.com 与 dl.example.com）。IP 地址只做精确匹配。
func (d *NexusPHPDriver) isSiteHost(host string) bool {
	candidates := []string{d.BaseURL}
	if d.siteDefinition != nil {
		candidates = append(candidates, d.siteDefinition.URLs...)
	}
	target := registrableDomain(host)
	for _, c := range candidates {
		u, err := url.Parse(c)
		if err != nil || u.Hostname() == "" {
			continue
		}
		if strings.EqualFold(u.Hostname(), host) {
			return true
		}
		if target != "" && strings.EqualFold(registrableDomain(u.Hostname()), target) {
			return true
		}
	}
	return false
}

// registrableDomain 返回 host 的可注册域名（eTLD+1）；IP 或无法判断时返回空串。
func registrableDomain(host string) string {
	if host == "" || net.ParseIP(host) != nil {
		return ""
	}
	domain, err := publicsuffix.EffectiveTLDPlusOne(strings.ToLower(host))
	if err != nil {
		return ""
	}
	return domain
}

// extractTorrentID extracts the torrent ID from a URL
func extractTorrentID(href string) string {
	// Match patterns like "details.php?id=12345" or "id=12345"
	re := regexp.MustCompile(`id=(\d+)`)
	matches := re.FindStringSubmatch(href)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// parseSize parses a size string like "1.5 GB" to bytes
func parseSize(sizeStr string) int64 {
	sizeStr = strings.TrimSpace(sizeStr)
	sizeStr = strings.ReplaceAll(sizeStr, ",", "")
	sizeStr = strings.ReplaceAll(sizeStr, " ", "")

	// Extract number and unit
	re := regexp.MustCompile(`([\d.]+)\s*([KMGTP]?i?B?)`)
	matches := re.FindStringSubmatch(strings.ToUpper(sizeStr))
	if len(matches) < 2 {
		return 0
	}

	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0
	}

	unit := ""
	if len(matches) > 2 {
		unit = matches[2]
	}

	multiplier := float64(1)
	switch {
	case strings.HasPrefix(unit, "K"):
		multiplier = 1024
	case strings.HasPrefix(unit, "M"):
		multiplier = 1024 * 1024
	case strings.HasPrefix(unit, "G"):
		multiplier = 1024 * 1024 * 1024
	case strings.HasPrefix(unit, "T"):
		multiplier = 1024 * 1024 * 1024 * 1024
	case strings.HasPrefix(unit, "P"):
		multiplier = 1024 * 1024 * 1024 * 1024 * 1024
	}

	return int64(value * multiplier)
}

// parseDiscountFromElement parses discount level from an HTML element
func parseDiscountFromElement(elem *goquery.Selection, customMapping map[string]DiscountLevel) DiscountLevel {
	class, _ := elem.Attr("class")
	class = strings.ToLower(class)

	src, _ := elem.Attr("src")
	src = strings.ToLower(src)

	alt, _ := elem.Attr("alt")
	alt = strings.ToLower(alt)

	combined := class + " " + src + " " + alt
	// 长的关键词先比：twoupfree 同时包含 free 与 twoup，按 map 的随机顺序会时而当成免费、时而当成 2x 上传
	keywords := make([]string, 0, len(customMapping))
	for keyword := range customMapping {
		keywords = append(keywords, keyword)
	}
	slices.SortFunc(keywords, func(a, b string) int {
		if len(a) != len(b) {
			return len(b) - len(a)
		}
		return strings.Compare(a, b)
	})
	for _, keyword := range keywords {
		if strings.Contains(combined, strings.ToLower(keyword)) {
			return customMapping[keyword]
		}
	}

	switch {
	case strings.Contains(combined, "2xfree") || strings.Contains(combined, "free2up"):
		return Discount2xFree
	case strings.Contains(combined, "free"):
		return DiscountFree
	case strings.Contains(combined, "50pct") || strings.Contains(combined, "50%"):
		return DiscountPercent50
	case strings.Contains(combined, "30pct") || strings.Contains(combined, "30%"):
		return DiscountPercent30
	case strings.Contains(combined, "70pct") || strings.Contains(combined, "70%"):
		return DiscountPercent70
	case strings.Contains(combined, "2xup") || strings.Contains(combined, "2up"):
		return Discount2xUp
	default:
		return DiscountNone
	}
}

var discountEndTimeInOnmouseoverRegex = regexp.MustCompile(`title=(?:&quot;|")(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})(?:&quot;|")`)

// parseDiscountEndTimeFromOnmouseover 从优惠图标的 onmouseover 提示里取结束时间：提示里没有时间时返回零时间且没有错误；
// 有时间却解析不了（例如日期越界）返回错误。
func parseDiscountEndTimeFromOnmouseover(onmouseover string) (time.Time, error) {
	matches := discountEndTimeInOnmouseoverRegex.FindStringSubmatch(onmouseover)
	if len(matches) < 2 {
		return time.Time{}, nil
	}
	return parseDiscountEndTime(matches[1])
}

// discountEndTimeLayouts 是列表页优惠结束时间不带时区的写法，按北京时间解释（与详情页解析一致；
// time.Parse 会当成 UTC，结束时刻晚 8 小时）。都带年份：不带年份的写法定不下时刻，按无法解析处理。
var discountEndTimeLayouts = []string{
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006/01/02 15:04:05",
	"2006/01/02 15:04",
}

var errDiscountEndTimeFormat = errors.New("unrecognized discount end time format")

// parseDiscountEndTime 解析列表页里的优惠结束时间：空值返回零时间且没有错误（没有结束时间）；非空但无法解析返回错误。
// 带时区的 RFC3339 照原样，不带时区的写法按北京时间。
func parseDiscountEndTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	for _, layout := range discountEndTimeLayouts {
		if t, err := ParseTimeInCST(layout, value); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errDiscountEndTimeFormat
}

// parseTime parses various time formats
func parseTime(timeStr string) time.Time {
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
		time.RFC3339,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t
		}
	}

	return time.Time{}
}

// parseRatio parses a ratio string
func parseRatio(ratioStr string) float64 {
	ratioStr = strings.TrimSpace(ratioStr)
	ratioStr = strings.ReplaceAll(ratioStr, ",", "")

	// Handle special cases
	if strings.Contains(strings.ToLower(ratioStr), "inf") || strings.Contains(ratioStr, "∞") {
		return -1 // Infinite ratio
	}

	value, _ := strconv.ParseFloat(ratioStr, 64)
	return value
}

// parseFloat parses a float string
func parseFloat(str string) float64 {
	str = strings.TrimSpace(str)
	str = strings.ReplaceAll(str, ",", "")
	value, _ := strconv.ParseFloat(str, 64)
	return value
}

// findTextByLabel finds text content by looking for labels
func findTextByLabel(doc *goquery.Document, labels ...string) string {
	for _, label := range labels {
		// Try finding td with label followed by value td
		selector := fmt.Sprintf("td:contains('%s')", label)
		elem := doc.Find(selector)
		if elem.Length() > 0 {
			// Get the next sibling td
			next := elem.Next()
			if next.Length() > 0 {
				return strings.TrimSpace(next.Text())
			}
		}
	}
	return ""
}

// extractUserID extracts user ID from a URL like "userdetails.php?id=12345"
func extractUserID(href string) string {
	re := regexp.MustCompile(`id=(\d+)`)
	matches := re.FindStringSubmatch(href)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// findInfoBlockValue finds value in NexusPHP info_block format
// Format: "label: value" or "label：value"
func findInfoBlockValue(doc *goquery.Document, labels ...string) string {
	infoBlock := doc.Find("#info_block, #userbar, .info_block")
	if infoBlock.Length() == 0 {
		return ""
	}

	text := infoBlock.Text()
	for _, label := range labels {
		// Try both : and ：
		patterns := []string{
			label + `: `,
			label + `：`,
			label + `:`,
		}
		for _, pattern := range patterns {
			idx := strings.Index(text, pattern)
			if idx >= 0 {
				// Extract value after the label
				start := idx + len(pattern)
				// Find end of value (next label or end of line)
				end := start
				for end < len(text) && text[end] != '\n' && text[end] != '|' {
					end++
				}
				value := strings.TrimSpace(text[start:end])
				if value != "" {
					return value
				}
			}
		}
	}
	return ""
}

// containsAny checks if s contains any of the substrings
func containsAny(s string, substrs ...string) bool {
	sLower := strings.ToLower(s)
	for _, sub := range substrs {
		if strings.Contains(sLower, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

// extractSizeFromTransfer extracts size value from transfer string
// Format: "上传量: 1.5 TB 下载量: 500 GB"
func extractSizeFromTransfer(text string, labels ...string) int64 {
	for _, label := range labels {
		patterns := []string{
			label + `: `,
			label + `：`,
			label + `:`,
			label + ` `,
		}
		for _, pattern := range patterns {
			idx := strings.Index(text, pattern)
			if idx >= 0 {
				start := idx + len(pattern)
				// Extract size string (e.g., "1.5 TB")
				end := start
				for end < len(text) {
					ch := text[end]
					// Stop at next label or separator
					if ch == '|' || (end > start+20) {
						break
					}
					// Check if we hit another label
					remaining := text[end:]
					if strings.HasPrefix(remaining, "下载") || strings.HasPrefix(remaining, "上传") ||
						strings.HasPrefix(remaining, "分享") || strings.HasPrefix(remaining, "Ratio") ||
						strings.HasPrefix(remaining, "Downloaded") || strings.HasPrefix(remaining, "Uploaded") {
						break
					}
					end++
				}
				sizeStr := strings.TrimSpace(text[start:end])
				if sizeStr != "" {
					return parseSize(sizeStr)
				}
			}
		}
	}
	return 0
}

// extractValueFromTransfer extracts a value from transfer string
func extractValueFromTransfer(text string, labels ...string) string {
	for _, label := range labels {
		patterns := []string{
			label + `: `,
			label + `：`,
			label + `:`,
		}
		for _, pattern := range patterns {
			idx := strings.Index(text, pattern)
			if idx >= 0 {
				start := idx + len(pattern)
				end := start
				for end < len(text) && text[end] != ' ' && text[end] != '\n' && text[end] != '|' {
					end++
				}
				return strings.TrimSpace(text[start:end])
			}
		}
	}
	return ""
}

// extractNumber extracts the first number from a string
func extractNumber(s string) string {
	re := regexp.MustCompile(`[\d,]+\.?\d*`)
	match := re.FindString(s)
	return strings.ReplaceAll(match, ",", "")
}

// PrepareUserSeedingPage prepares a request for user seeding page via AJAX
// This is used to fetch seeding size information from /getusertorrentlistajax.php
func (d *NexusPHPDriver) PrepareUserSeedingPage(userID, listType string) (NexusPHPRequest, error) {
	params := url.Values{}
	params.Set("userid", userID)
	params.Set("type", listType)
	return NexusPHPRequest{
		Path:   "/getusertorrentlistajax.php",
		Params: params,
		Method: "GET",
	}, nil
}

// ParseSeedingStatus parses the seeding status from the AJAX response
// Implements two parsing strategies based on NexusPHP.ts:
// 1. Direct parsing: Look for summary text like "10 | 100 GB" or "<b>94</b>条记录，共计<b>2.756 TB</b>"
// 2. Table accumulation: Sum up sizes from individual torrent rows
func (d *NexusPHPDriver) ParseSeedingStatus(res NexusPHPResponse) (seeding int, seedingSize int64, err error) {
	if res.Document == nil {
		return 0, 0, ErrParseError
	}

	doc := res.Document
	bodyStr := string(res.RawBody)

	// Method 1a: Try to parse SpringSunday format: "<b>94</b>条记录，共计<b>2.756 TB</b>"
	// Pattern: <b>数量</b>条记录，共计<b>大小</b>
	springSundayPattern := regexp.MustCompile(`<b>(\d+)</b>\s*条记录[^<]*共计\s*<b>([\d.]+\s*[KMGTP]?i?B)</b>`)
	if matches := springSundayPattern.FindStringSubmatch(bodyStr); len(matches) >= 3 {
		seeding = int(parseFloat(matches[1]))
		seedingSize = parseSize(matches[2])
		if DebugUserInfo {
			fmt.Printf("[DEBUG] ParseSeedingStatus Method1a (SpringSunday format): count=%d, size=%d from pattern match\n", seeding, seedingSize)
		}
		return seeding, seedingSize, nil
	}

	// Method 1b: Try to parse direct summary text (e.g., "10 | 100 GB")
	// Look for div containing " | " which typically shows "count | size"
	divSeeding := doc.Find("div > div:contains(' | ')")
	if divSeeding.Length() > 0 {
		text := strings.TrimSpace(divSeeding.First().Text())
		parts := strings.Split(text, "|")
		if len(parts) >= 2 {
			// Parse seeding count from first part. Upstream NexusPHP puts the record label after the
			// number ("94 条记录 | 总大小：2.756 TB", "94 records | Total size: ..."), so take the number only.
			seeding = int(parseFloat(extractNumber(parts[0])))
			// Parse seeding size from second part
			seedingSize = parseSize(strings.TrimSpace(parts[1]))
			if DebugUserInfo {
				fmt.Printf("[DEBUG] ParseSeedingStatus Method1b (pipe format): count=%d, size=%d from %q\n", seeding, seedingSize, text)
			}
			return seeding, seedingSize, nil
		}
	}

	// Method 2: Fallback - parse table rows and accumulate sizes
	// Find all rows except the header row
	rows := doc.Find("table:last tr:not(:first-child)")
	if rows.Length() == 0 {
		// Also try without :last if only one table exists
		rows = doc.Find("table tr:not(:first-child)")
	}

	if rows.Length() == 0 {
		if DebugUserInfo {
			fmt.Printf("[DEBUG] ParseSeedingStatus: no table rows found\n")
		}
		return 0, 0, nil
	}

	seeding = rows.Length()

	// Auto-detect size column index by finding the first column that matches size pattern
	sizeIndex := -1
	sizePattern := regexp.MustCompile(`(?i)[\d.]+\s*[KMGTP]?i?B`)

	// Check first row to determine size column
	firstRow := rows.First()
	firstRow.Find("td").Each(func(i int, td *goquery.Selection) {
		if sizeIndex < 0 && sizePattern.MatchString(td.Text()) {
			sizeIndex = i
		}
	})

	if sizeIndex < 0 {
		// Default to column index 2 if not found (common in NexusPHP)
		sizeIndex = 2
	}

	if DebugUserInfo {
		fmt.Printf("[DEBUG] ParseSeedingStatus Method2: detected sizeIndex=%d, rowCount=%d\n", sizeIndex, seeding)
	}

	// Accumulate sizes from all rows
	rows.Each(func(i int, row *goquery.Selection) {
		tds := row.Find("td")
		if tds.Length() > sizeIndex {
			sizeText := strings.TrimSpace(tds.Eq(sizeIndex).Text())
			size := parseSize(sizeText)
			seedingSize += size
			if DebugUserInfo && i < 3 { // Only log first 3 rows for debugging
				fmt.Printf("[DEBUG]   Row %d: sizeText=%q, parsed=%d\n", i, sizeText, size)
			}
		}
	})

	if DebugUserInfo {
		fmt.Printf("[DEBUG] ParseSeedingStatus Method2: total seeding=%d, seedingSize=%d\n", seeding, seedingSize)
	}

	return seeding, seedingSize, nil
}

// FetchSeedingStatus fetches the seeding status (count and size) for a user
// This method requests /getusertorrentlistajax.php and parses the response
func (d *NexusPHPDriver) FetchSeedingStatus(ctx context.Context, userID string) (seeding int, seedingSize int64, err error) {
	req, err := d.PrepareUserSeedingPage(userID, "seeding")
	if err != nil {
		return 0, 0, err
	}

	if DebugUserInfo {
		fmt.Printf("[DEBUG] FetchSeedingStatus: requesting %s?%s\n", req.Path, req.Params.Encode())
	}

	res, err := d.fetchUserInfoPage(ctx, req)
	if err != nil {
		if DebugUserInfo {
			fmt.Printf("[DEBUG] FetchSeedingStatus: request error: %v\n", err)
		}
		return 0, 0, err
	}

	// Check if response contains table data
	if res.Document == nil {
		if DebugUserInfo {
			fmt.Printf("[DEBUG] FetchSeedingStatus: document is nil\n")
		}
		return 0, 0, nil
	}

	// Check if the response contains a table (indicates valid data)
	bodyStr := string(res.RawBody)
	if DebugUserInfo {
		// Print first 500 chars of response for debugging
		preview := bodyStr
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		fmt.Printf("[DEBUG] FetchSeedingStatus: response preview: %s\n", preview)
	}

	if !strings.Contains(bodyStr, "<table") {
		if DebugUserInfo {
			fmt.Printf("[DEBUG] FetchSeedingStatus: no table in response, skipping\n")
		}
		return 0, 0, nil
	}

	return d.ParseSeedingStatus(res)
}

// extractSiteIDFromURL extracts site ID from a base URL
// e.g., "https://hdsky.me" -> "hdsky", "https://springsunday.net" -> "springsunday"
func extractSiteIDFromURL(baseURL string) string {
	// Parse the URL
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}

	// Get the host part
	host := u.Host
	if host == "" {
		return ""
	}

	// Remove port if present
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		host = host[:idx]
	}

	// Extract domain name (without TLD)
	// e.g., "hdsky.me" -> "hdsky", "api.m-team.cc" -> "mteam"
	parts := strings.Split(host, ".")
	if len(parts) >= 2 {
		// For subdomains like "api.m-team.cc", take the second-to-last part
		// But handle special cases like "m-team" which should become "mteam"
		domainPart := parts[len(parts)-2]
		// Handle cases like "api.m-team.cc" where parts[1] is "m-team"
		if domainPart == "api" && len(parts) >= 3 {
			domainPart = parts[len(parts)-2]
		}
		// Normalize: remove hyphens and lowercase
		return strings.ToLower(strings.ReplaceAll(domainPart, "-", ""))
	}

	return host
}

func (d *NexusPHPDriver) GetTorrentDetail(ctx context.Context, guid, link, _ string) (*TorrentItem, error) {
	torrentID := ""
	if link != "" {
		torrentID = extractTorrentIDFromURL(link)
	}
	if torrentID == "" {
		torrentID = guid
	}
	if torrentID == "" {
		return nil, fmt.Errorf("unable to determine torrent ID")
	}

	req, err := d.PrepareDetail(torrentID)
	if err != nil {
		return nil, fmt.Errorf("prepare detail request: %w", err)
	}

	res, err := d.Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("execute detail request: %w", err)
	}

	if res.Document == nil {
		return nil, ErrParseError
	}

	fetchedAt := time.Now().UTC()
	parser := NewNexusPHPParserFromDefinition(d.GetSiteDefinition())
	detailInfo := parser.ParseAll(res.Document.Selection)

	item := &TorrentItem{
		ID:              detailInfo.TorrentID,
		Title:           detailInfo.Title,
		SizeBytes:       int64(detailInfo.SizeMB * 1024 * 1024),
		DiscountLevel:   detailInfo.DiscountLevel,
		DiscountEndTime: detailInfo.DiscountEnd,
		HasHR:           detailInfo.HasHR,
		SourceSite:      d.getSiteID(),
		IMDbID:          detailInfo.IMDbID,
		DoubanID:        detailInfo.DoubanID,
		FetchedAt:       fetchedAt,
	}
	// 结束时间元素在、值解析不了：免费的有效性是 unknown（元素不在时不写，由 Promotion 按零结束时间推出 unknown）
	markDiscountUnreliable(item, detailInfo.DiscountErr)

	return item, nil
}

// getSiteID 是站点在 pt-tools 里的 ID。从域名猜只是最后的办法：不少站点的域名与 ID 对不上
// （pt.xingyungept.org 是 xingyunge、open.cd 是 opencd），按猜的 ID 拼的下载地址找不到站点（2026-10-26）。
func (d *NexusPHPDriver) getSiteID() string {
	if d.siteID != "" {
		return d.siteID
	}
	if d.siteDefinition != nil {
		return d.siteDefinition.ID
	}
	if d.siteName != "" {
		return string(d.siteName)
	}
	return extractSiteIDFromURL(d.BaseURL)
}

func extractTorrentIDFromURL(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	if id := u.Query().Get("id"); id != "" {
		return id
	}
	pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for _, part := range pathParts {
		if _, err := strconv.Atoi(part); err == nil {
			return part
		}
	}
	return ""
}

func init() {
	RegisterDriverForSchema("NexusPHP", createNexusPHPSite)
}

func createNexusPHPSite(config SiteConfig, logger *zap.Logger) (Site, error) {
	var opts NexusPHPOptions
	if len(config.Options) > 0 {
		if err := json.Unmarshal(config.Options, &opts); err != nil {
			return nil, fmt.Errorf("parse NexusPHP options: %w", err)
		}
	}

	if opts.Cookie == "" {
		return nil, fmt.Errorf("NexusPHP site requires cookie")
	}

	registry := GetDefinitionRegistry()
	siteDef := registry.GetOrDefault(config.ID)

	selectors := DefaultNexusPHPSelectors()
	if opts.Selectors != nil {
		mergeSelectors(&selectors, opts.Selectors)
	}
	if siteDef != nil && siteDef.Selectors != nil {
		mergeSelectors(&selectors, siteDef.Selectors)
	}

	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL:   config.BaseURL,
		Cookie:    opts.Cookie,
		Selectors: &selectors,
		SiteID:    config.ID,
	})

	if siteDef != nil {
		driver.SetSiteDefinition(siteDef)
	}

	return NewBaseSite(driver, BaseSiteConfig{
		ID:        config.ID,
		Name:      config.Name,
		Kind:      SiteNexusPHP,
		RateLimit: config.RateLimit,
		RateBurst: config.RateBurst,
		Logger:    logger.With(zap.String("site", config.ID)),
	}), nil
}
