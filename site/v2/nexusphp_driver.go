package v2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// NexusPHPRequest represents a request to a NexusPHP site
type NexusPHPRequest struct {
	// Path is the URL path (e.g., "/torrents.php")
	Path string
	// Params are the query parameters
	Params url.Values
	// Method is the HTTP method (default: GET)
	Method string
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
	}

	// Initialize failover client if enabled and site name is provided
	if config.UseFailover && config.SiteName != "" {
		registry := GetGlobalRegistry()
		if failoverClient, err := registry.GetFailoverClient(config.SiteName,
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

	fullURL := baseURL + req.Path
	if len(req.Params) > 0 {
		fullURL += "?" + req.Params.Encode()
	}

	headers := map[string]string{
		"Cookie":          d.Cookie,
		"User-Agent":      d.userAgent,
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
	}

	// Print curl command for debugging
	if DebugUserInfo {
		fmt.Printf("\n[CURL] %s\n", buildCurlCommand(method, fullURL, headers))
	}

	resp, err := d.httpClient.Get(ctx, fullURL, headers)
	if err != nil {
		return NexusPHPResponse{}, fmt.Errorf("execute request: %w", err)
	}

	result := NexusPHPResponse{
		RawBody:    resp.Body,
		StatusCode: resp.StatusCode,
	}

	// Check for authentication errors
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return result, ErrInvalidCredentials
	}

	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
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
		}

		// Parse size
		sizeText := strings.TrimSpace(s.Find(d.Selectors.Size).Text())
		item.SizeBytes = parseSize(sizeText)

		// Parse seeders
		seedersText := strings.TrimSpace(s.Find(d.Selectors.Seeders).Text())
		item.Seeders, _ = strconv.Atoi(seedersText)

		// Parse leechers
		leechersText := strings.TrimSpace(s.Find(d.Selectors.Leechers).Text())
		item.Leechers, _ = strconv.Atoi(leechersText)

		// Parse snatched
		snatchedText := strings.TrimSpace(s.Find(d.Selectors.Snatched).Text())
		item.Snatched, _ = strconv.Atoi(snatchedText)

		// Parse discount level
		discountElem := s.Find(d.Selectors.DiscountIcon)
		if discountElem.Length() > 0 {
			item.DiscountLevel = parseDiscountFromElement(discountElem, d.Selectors.DiscountMapping)
		}

		// Parse discount end time
		endTimeElem := s.Find(d.Selectors.DiscountEndTime)
		if endTimeElem.Length() > 0 {
			if title, exists := endTimeElem.Attr("title"); exists {
				item.DiscountEndTime = parseTime(title)
			} else {
				item.DiscountEndTime = parseTime(endTimeElem.Text())
			}
		}

		// Fallback: parse discount end time from onmouseover attribute of discount icon
		// Some sites (like HDSky) embed the end time in the tooltip, not as a separate element
		// Format: domTT_activate(..., '<span title="2026-01-18 22:37:47">1时19分</span>', ...)
		if item.DiscountEndTime.IsZero() && discountElem.Length() > 0 {
			if onmouseover, exists := discountElem.Attr("onmouseover"); exists && onmouseover != "" {
				item.DiscountEndTime = parseDiscountEndTimeFromOnmouseover(onmouseover)
			}
		}

		// Parse download link - use proxy URL instead of direct link for authentication handling
		// The backend proxy will handle cookie/passkey authentication
		if item.ID != "" {
			// Use proxy download URL that handles authentication
			siteID := string(d.siteName)
			if siteID == "" {
				// Fallback to extracting site ID from BaseURL
				siteID = extractSiteIDFromURL(d.BaseURL)
			}
			item.DownloadURL = fmt.Sprintf("/api/site/%s/torrent/%s/download", siteID, item.ID)
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

// getUserInfoWithDefinition fetches user info using site definition selectors
// Uses concurrent requests where possible to improve performance
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
	var mu sync.Mutex // Protect parsedValues and info

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

	// Phase 1: Execute all independent processes in parallel using errgroup
	phase1Start := time.Now()
	if len(independentProcesses) > 0 {
		if DebugUserInfo {
			fmt.Printf("[DEBUG] Phase 1: Executing %d independent processes in parallel\n", len(independentProcesses))
		}

		g, gctx := errgroup.WithContext(ctx)
		for _, idx := range independentProcesses {
			idx := idx // capture loop variable
			g.Go(func() error {
				values, err := d.executeProcess(gctx, uiConfig, uiConfig.Process[idx], parsedValues)
				if err != nil {
					return err // Return critical errors like session expired
				}
				mu.Lock()
				for k, v := range values {
					parsedValues[k] = v
					d.setUserInfoField(&info, k, v)
				}
				mu.Unlock()
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return UserInfo{}, fmt.Errorf("phase 1 parallel execution failed: %w", err)
		}
	}
	if DebugUserInfo {
		fmt.Printf("[DEBUG] Phase 1 completed in %v\n", time.Since(phase1Start))
	}

	// Apply RequestDelay between phases if configured
	if uiConfig.RequestDelay > 0 {
		time.Sleep(time.Duration(uiConfig.RequestDelay) * time.Millisecond)
	}

	// Phase 2: Execute dependent processes AND FetchSeedingStatus in parallel
	phase2Start := time.Now()
	needSeedingStatus := false
	if uiConfig.Selectors != nil {
		_, hasSeedingSizeSelector := uiConfig.Selectors["seedingSize"]
		needSeedingStatus = !hasSeedingSizeSelector
	} else {
		needSeedingStatus = true
	}

	if len(dependentProcesses) > 0 || (needSeedingStatus && info.UserID != "") {
		if DebugUserInfo {
			fmt.Printf("[DEBUG] Phase 2: Executing %d dependent processes", len(dependentProcesses))
			if needSeedingStatus && info.UserID != "" {
				fmt.Printf(" + seeding status fetch")
			}
			fmt.Printf(" in parallel\n")
		}

		g, gctx := errgroup.WithContext(ctx)

		// Launch dependent processes
		for _, idx := range dependentProcesses {
			idx := idx // capture loop variable
			g.Go(func() error {
				values, err := d.executeProcess(gctx, uiConfig, uiConfig.Process[idx], parsedValues)
				if err != nil {
					return err // Return critical errors like session expired
				}
				mu.Lock()
				for k, v := range values {
					parsedValues[k] = v
					d.setUserInfoField(&info, k, v)
				}
				mu.Unlock()
				return nil
			})
		}

		// Launch seeding status fetch if needed (non-blocking error)
		if needSeedingStatus && info.UserID != "" {
			userID := info.UserID // capture
			g.Go(func() error {
				seeding, seedingSize, err := d.FetchSeedingStatus(gctx, userID)
				if err != nil {
					if DebugUserInfo {
						fmt.Printf("[DEBUG] FetchSeedingStatus error: %v\n", err)
					}
					// Don't fail the whole operation for seeding status
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

		if err := g.Wait(); err != nil {
			return UserInfo{}, fmt.Errorf("phase 2 parallel execution failed: %w", err)
		}
	}
	if DebugUserInfo {
		fmt.Printf("[DEBUG] Phase 2 completed in %v\n", time.Since(phase2Start))
	}

	// Calculate ratio if not set
	if info.Ratio == 0 && info.Downloaded > 0 {
		info.Ratio = float64(info.Uploaded) / float64(info.Downloaded)
	}

	if DebugUserInfo {
		fmt.Printf("[DEBUG] getUserInfoWithDefinition total time: %v\n", time.Since(startTime))
	}

	return info, nil
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

	res, err := d.Execute(ctx, req)
	if err != nil {
		// Return critical errors like session expired
		if errors.Is(err, ErrSessionExpired) || errors.Is(err, ErrInvalidCredentials) {
			return result, err
		}
		return result, nil // Ignore other errors, return empty result
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

	return result, nil
}

// extractFieldValue extracts a field value from the document using the selector config
func (d *NexusPHPDriver) extractFieldValue(doc *goquery.Document, selector FieldSelector) string {
	var value string
	var matchedSelector string

	// Try each selector until one matches
	for _, sel := range selector.Selector {
		elem := doc.Find(sel).First()
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

	// Build full download URL
	downloadURL := detail.DownloadURL
	if !strings.HasPrefix(downloadURL, "http") {
		// Relative URL - prepend base URL
		if strings.HasPrefix(downloadURL, "/") {
			downloadURL = d.BaseURL + downloadURL
		} else {
			downloadURL = d.BaseURL + "/" + downloadURL
		}
	}

	// Fetch the actual torrent file
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	headers := map[string]string{
		"Cookie":          d.Cookie,
		"User-Agent":      d.userAgent,
		"Accept":          "application/x-bittorrent,*/*",
		"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
		"Referer":         d.BaseURL + "/",
	}

	resp, err := d.httpClient.Get(ctx, downloadURL, headers)
	if err != nil {
		return nil, fmt.Errorf("fetch torrent file: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching torrent from %s", resp.StatusCode, downloadURL)
	}

	if len(resp.Body) == 0 {
		return nil, fmt.Errorf("empty torrent file response")
	}

	return resp.Body, nil
}

// Helper functions

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
	for keyword, level := range customMapping {
		if strings.Contains(combined, strings.ToLower(keyword)) {
			return level
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

func parseDiscountEndTimeFromOnmouseover(onmouseover string) time.Time {
	matches := discountEndTimeInOnmouseoverRegex.FindStringSubmatch(onmouseover)
	if len(matches) >= 2 {
		return parseTime(matches[1])
	}
	return time.Time{}
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
			// Parse seeding count from first part
			seeding = int(parseFloat(strings.TrimSpace(parts[0])))
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

	res, err := d.Execute(ctx, req)
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
	}

	return item, nil
}

func (d *NexusPHPDriver) getSiteID() string {
	if d.siteDefinition != nil {
		return d.siteDefinition.ID
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
