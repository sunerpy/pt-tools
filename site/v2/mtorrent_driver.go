package v2

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/requests"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// mteamCategoryMap maps M-Team category IDs to human-readable names
var mteamCategoryMap = map[string]string{
	// Normal categories
	"401": "电影/SD",
	"419": "电影/HD",
	"420": "电影/DVDiSo",
	"421": "电影/Blu-Ray",
	"439": "电影/Remux",
	"403": "影剧/综艺/SD",
	"402": "影剧/综艺/HD",
	"438": "影剧/综艺/BD",
	"435": "影剧/综艺/DVDiSo",
	"404": "纪录片",
	"434": "音乐(无损)",
	"406": "演唱会",
	"423": "PC游戏",
	"448": "TV游戏",
	"405": "动画",
	"407": "运动",
	"427": "电子书",
	"422": "软件",
	"442": "有声书",
	"451": "教育影片",
	"409": "其他",
	// Adult categories
	"410": "AV(有码)/HD",
	"424": "AV(有码)/SD",
	"437": "AV(有码)/DVDiSo",
	"431": "AV(无码)/HD",
	"429": "AV(无码)/SD",
	"430": "AV(无码)/DVDiSo",
	"426": "IV(写真)/HD",
	"432": "IV(写真)/SD",
	"436": "H-Anime",
	"425": "H-Game",
	"433": "H-Comic",
	"411": "Misc(其他)",
}

// getMTeamCategoryName returns the category name for a given category ID
func getMTeamCategoryName(catID string) string {
	if name, ok := mteamCategoryMap[catID]; ok {
		return name
	}
	return catID // Return ID if not found
}

// MTorrentRequest represents a request to M-Team API
type MTorrentRequest struct {
	// Endpoint is the API endpoint path
	Endpoint string
	// Method is the HTTP method
	Method string
	// Body is the request body (will be JSON encoded or form-urlencoded based on ContentType)
	Body any
	// ContentType specifies the request content type (default: "application/json")
	// Use "application/x-www-form-urlencoded" for form data
	ContentType string
}

// FlexibleCode handles M-Team API code field which can be either string or number
type FlexibleCode string

// UnmarshalJSON implements custom JSON unmarshaling for FlexibleCode
func (fc *FlexibleCode) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*fc = FlexibleCode(s)
		return nil
	}

	// Try to unmarshal as number
	var n json.Number
	if err := json.Unmarshal(data, &n); err == nil {
		*fc = FlexibleCode(n.String())
		return nil
	}

	// Try to unmarshal as int
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		*fc = FlexibleCode(strconv.Itoa(i))
		return nil
	}

	return fmt.Errorf("cannot unmarshal %s into FlexibleCode", string(data))
}

// String returns the string representation of the code
func (fc FlexibleCode) String() string {
	return string(fc)
}

// IsSuccess checks if the code represents success (typically "0" or "SUCCESS")
func (fc FlexibleCode) IsSuccess() bool {
	s := strings.ToUpper(string(fc))
	return s == "0" || s == "SUCCESS" || s == "200"
}

// FlexInt handles fields that can be either string or int in JSON
type FlexInt int

// UnmarshalJSON implements custom JSON unmarshaling for FlexInt
func (fi *FlexInt) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as int first
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		*fi = FlexInt(i)
		return nil
	}

	// Try to unmarshal as string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s == "" {
			*fi = 0
			return nil
		}
		parsed, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("cannot parse FlexInt from string %q: %w", s, err)
		}
		*fi = FlexInt(parsed)
		return nil
	}

	return fmt.Errorf("cannot unmarshal %s into FlexInt", string(data))
}

// Int returns the int value
func (fi FlexInt) Int() int {
	return int(fi)
}

// MTorrentResponse represents a response from M-Team API
type MTorrentResponse struct {
	// Code is the response code ("0" for success)
	Code FlexibleCode `json:"code"`
	// Message is the response message
	Message string `json:"message"`
	// Data is the response data
	Data json.RawMessage `json:"data"`
	// RawBody is the raw response body (for downloads)
	RawBody []byte `json:"-"`
	// StatusCode is the HTTP status code
	StatusCode int `json:"-"`
}

// MTorrentSearchRequest is the search request body
type MTorrentSearchRequest struct {
	Mode       string   `json:"mode"`
	Categories []string `json:"categories,omitempty"`
	Keyword    string   `json:"keyword,omitempty"`
	PageNumber int      `json:"pageNumber"`
	PageSize   int      `json:"pageSize"`
}

// MTorrentSearchData is the search response data
type MTorrentSearchData struct {
	Data  []MTorrentTorrent `json:"data"`
	Total FlexInt           `json:"total"`
}

// MTorrentPromotionRule represents an additional promotion rule from M-Team API
type MTorrentPromotionRule struct {
	Discount  string `json:"discount"`
	StartTime string `json:"startTime,omitempty"`
	EndTime   string `json:"endTime,omitempty"`
}

// MTorrentTorrent represents a torrent in M-Team API response
type MTorrentTorrent struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	SmallDescr  string `json:"smallDescr"`
	Size        string `json:"size"`
	CreatedDate string `json:"createdDate"`
	Status      struct {
		Seeders         FlexInt                `json:"seeders"`
		Leechers        FlexInt                `json:"leechers"`
		TimesCompleted  FlexInt                `json:"timesCompleted"`
		Discount        string                 `json:"discount"`
		DiscountEndTime string                 `json:"discountEndTime,omitempty"`
		PromotionRule   *MTorrentPromotionRule `json:"promotionRule,omitempty"`
	} `json:"status"`
	Category string `json:"category"`
}

// MTorrentUserInfo represents user info from M-Team API
type MTorrentUserInfo struct {
	ID           string               `json:"id"`
	Username     string               `json:"username"`
	CreatedDate  string               `json:"createdDate"`
	Role         string               `json:"role"`
	MemberCount  MTorrentMemberCount  `json:"memberCount"`
	MemberStatus MTorrentMemberStatus `json:"memberStatus"`
}

// MTorrentMemberCount contains upload/download stats
type MTorrentMemberCount struct {
	Uploaded   string `json:"uploaded"`
	Downloaded string `json:"downloaded"`
	Bonus      string `json:"bonus"`
	ShareRate  string `json:"shareRate"`
	Seedtime   string `json:"seedtime"`
	Leechtime  string `json:"leechtime"`
}

// MTorrentMemberStatus contains user status info
type MTorrentMemberStatus struct {
	LastLogin  string `json:"lastLogin"`
	LastBrowse string `json:"lastBrowse"`
	Vip        bool   `json:"vip"`
}

// MTorrentDriver implements the Driver interface for M-Team sites
type MTorrentDriver struct {
	BaseURL        string // API URL (e.g., https://api.m-team.cc)
	WebURL         string // Web URL for detail pages (e.g., https://kp.m-team.cc)
	APIKey         string
	httpClient     *SiteHTTPClient
	failoverClient *FailoverHTTPClient
	userAgent      string
	useFailover    bool
	siteDefinition *SiteDefinition
}

// MTorrentDriverConfig holds configuration for creating an M-Team driver
type MTorrentDriverConfig struct {
	BaseURL     string
	WebURL      string // Optional: Web URL for detail pages, defaults to "https://kp.m-team.cc"
	APIKey      string
	HTTPClient  *SiteHTTPClient // Use SiteHTTPClient instead of *http.Client
	UserAgent   string
	UseFailover bool // Enable multi-URL failover
}

// NewMTorrentDriver creates a new M-Team driver
func NewMTorrentDriver(config MTorrentDriverConfig) *MTorrentDriver {
	userAgent := config.UserAgent
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
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

	// Default web URL for detail pages
	webURL := config.WebURL
	if webURL == "" {
		webURL = "https://kp.m-team.cc"
	}

	driver := &MTorrentDriver{
		BaseURL:     strings.TrimSuffix(config.BaseURL, "/"),
		WebURL:      strings.TrimSuffix(webURL, "/"),
		APIKey:      config.APIKey,
		httpClient:  httpClient,
		userAgent:   userAgent,
		useFailover: config.UseFailover,
	}

	// Initialize failover client if enabled
	if config.UseFailover {
		registry := GetGlobalRegistry()
		if failoverClient, err := registry.GetFailoverClient(SiteNameMTeam,
			WithUserAgent(userAgent),
		); err == nil {
			driver.failoverClient = failoverClient
		}
	}

	return driver
}

// NewMTorrentDriverWithFailover creates a new M-Team driver with failover enabled
func NewMTorrentDriverWithFailover(apiKey string) *MTorrentDriver {
	return NewMTorrentDriver(MTorrentDriverConfig{
		BaseURL:     "https://api.m-team.cc", // Default, will be overridden by failover
		APIKey:      apiKey,
		UseFailover: true,
	})
}

// SetSiteDefinition sets the site definition for custom parsing
func (d *MTorrentDriver) SetSiteDefinition(def *SiteDefinition) {
	d.siteDefinition = def
}

// GetSiteDefinition returns the site definition
func (d *MTorrentDriver) GetSiteDefinition() *SiteDefinition {
	return d.siteDefinition
}

// PrepareSearch converts a SearchQuery to an M-Team request
func (d *MTorrentDriver) PrepareSearch(query SearchQuery) (MTorrentRequest, error) {
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}

	pageNumber := query.Page
	if pageNumber <= 0 {
		pageNumber = 1
	}

	body := MTorrentSearchRequest{
		Mode:       "normal",
		Keyword:    query.Keyword,
		PageNumber: pageNumber,
		PageSize:   pageSize,
	}

	if query.Category != "" {
		body.Categories = []string{query.Category}
	}

	return MTorrentRequest{
		Endpoint: "/api/torrent/search",
		Method:   "POST",
		Body:     body,
	}, nil
}

// Execute performs the HTTP request
func (d *MTorrentDriver) Execute(ctx context.Context, req MTorrentRequest) (MTorrentResponse, error) {
	// Use failover client if available
	if d.useFailover && d.failoverClient != nil {
		return d.executeWithFailover(ctx, req)
	}
	return d.executeDirectly(ctx, req, d.BaseURL)
}

// executeWithFailover executes request with automatic URL failover
func (d *MTorrentDriver) executeWithFailover(ctx context.Context, req MTorrentRequest) (MTorrentResponse, error) {
	var result MTorrentResponse
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
func (d *MTorrentDriver) executeDirectly(ctx context.Context, req MTorrentRequest, baseURL string) (MTorrentResponse, error) {
	var bodyBytes []byte
	contentType := req.ContentType
	if contentType == "" {
		contentType = "application/json"
	}

	if req.Body != nil {
		var err error
		if contentType == "application/x-www-form-urlencoded" {
			// Form-urlencoded format: id=xxx
			if bodyMap, ok := req.Body.(map[string]any); ok {
				parts := make([]string, 0, len(bodyMap))
				for k, v := range bodyMap {
					parts = append(parts, fmt.Sprintf("%s=%v", k, v))
				}
				bodyBytes = []byte(strings.Join(parts, "&"))
			} else if bodyStr, ok := req.Body.(string); ok {
				bodyBytes = []byte(bodyStr)
			}
		} else {
			// JSON format
			bodyBytes, err = json.Marshal(req.Body)
			if err != nil {
				return MTorrentResponse{}, fmt.Errorf("marshal request body: %w", err)
			}
		}
	}

	fullURL := baseURL + req.Endpoint

	headers := map[string]string{
		"Content-Type": contentType,
		"Accept":       "application/json",
		"User-Agent":   d.userAgent,
		"x-api-key":    d.APIKey,
	}

	// Debug log for download requests
	if strings.Contains(req.Endpoint, "genDlToken") {
		fmt.Printf("[DEBUG MTorrent] genDlToken request: URL=%s, ContentType=%s, Body=%s\n", fullURL, contentType, string(bodyBytes))
	}

	resp, err := d.httpClient.Post(ctx, fullURL, bodyBytes, headers)
	if err != nil {
		return MTorrentResponse{}, fmt.Errorf("execute request: %w", err)
	}

	result := MTorrentResponse{
		RawBody:    resp.Body,
		StatusCode: resp.StatusCode,
	}

	// Debug log for download response
	if strings.Contains(req.Endpoint, "genDlToken") {
		fmt.Printf("[DEBUG MTorrent] genDlToken response: StatusCode=%d, Body=%s\n", resp.StatusCode, string(resp.Body))
	}

	// Check for authentication errors
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return result, ErrInvalidCredentials
	}

	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	// Parse JSON response
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return result, fmt.Errorf("parse JSON: %w", err)
	}

	return result, nil
}

// ParseSearch extracts torrent items from the response
func (d *MTorrentDriver) ParseSearch(res MTorrentResponse) ([]TorrentItem, error) {
	if !res.Code.IsSuccess() {
		return nil, fmt.Errorf("API error: %s - %s", res.Code.String(), res.Message)
	}

	var searchData MTorrentSearchData
	if err := json.Unmarshal(res.Data, &searchData); err != nil {
		return nil, fmt.Errorf("parse search data: %w", err)
	}

	// Debug: log raw data for first torrent to check field names
	if len(searchData.Data) > 0 && DebugUserInfo {
		fmt.Printf("[DEBUG MTorrent] First torrent raw data: Name=%s, SmallDescr=%s\n",
			searchData.Data[0].Name, searchData.Data[0].SmallDescr)
	}

	items := make([]TorrentItem, 0, len(searchData.Data))
	for _, t := range searchData.Data {
		discount, discountEndTime := parseMTorrentDiscountWithPromotion(t.Status.Discount, t.Status.DiscountEndTime, t.Status.PromotionRule)
		item := TorrentItem{
			ID:              t.ID,
			Title:           t.Name,
			Subtitle:        t.SmallDescr,
			SizeBytes:       parseSize(t.Size),
			Seeders:         t.Status.Seeders.Int(),
			Leechers:        t.Status.Leechers.Int(),
			Snatched:        t.Status.TimesCompleted.Int(),
			SourceSite:      d.BaseURL,
			Category:        getMTeamCategoryName(t.Category),
			DiscountLevel:   discount,
			DiscountEndTime: discountEndTime,
		}

		// Parse upload time
		if t.CreatedDate != "" {
			if uploadTime, err := ParseTimeInCST("2006-01-02 15:04:05", t.CreatedDate); err == nil {
				item.UploadedAt = uploadTime.Unix()
			}
		}

		// Build download URL
		// For M-Team sites, direct download is not possible, we set a proxy download URL
		// The frontend should call /api/site/{siteId}/torrent/{id}/download for actual download
		item.DownloadURL = fmt.Sprintf("/api/site/mteam/torrent/%s/download", t.ID)
		// Build detail URL using web URL (not API URL)
		item.URL = fmt.Sprintf("%s/detail/%s", d.WebURL, t.ID)

		items = append(items, item)
	}

	return items, nil
}

// PrepareUserInfo prepares a request for user info
func (d *MTorrentDriver) PrepareUserInfo() (MTorrentRequest, error) {
	return MTorrentRequest{
		Endpoint: "/api/member/profile",
		Method:   "POST",
		Body:     map[string]any{},
	}, nil
}

// ParseUserInfo extracts user info from the response
func (d *MTorrentDriver) ParseUserInfo(res MTorrentResponse) (UserInfo, error) {
	if !res.Code.IsSuccess() {
		// Log raw response for debugging
		rawBody := string(res.RawBody)
		if len(rawBody) > 500 {
			rawBody = rawBody[:500] + "..."
		}
		return UserInfo{}, fmt.Errorf("API error: %s - %s (raw: %s)", res.Code.String(), res.Message, rawBody)
	}

	var userData MTorrentUserInfo
	if err := json.Unmarshal(res.Data, &userData); err != nil {
		return UserInfo{}, fmt.Errorf("parse user data: %w (data: %s)", err, string(res.Data))
	}

	// Parse ratio from shareRate string
	var ratio float64
	if userData.MemberCount.ShareRate != "" {
		ratio, _ = strconv.ParseFloat(userData.MemberCount.ShareRate, 64)
	}

	// Parse bonus from string
	var bonus float64
	if userData.MemberCount.Bonus != "" {
		bonus, _ = strconv.ParseFloat(userData.MemberCount.Bonus, 64)
	}

	// Map role ID to user class name
	rank := mapMTorrentRole(userData.Role)

	// Parse role ID to level ID
	levelID, _ := strconv.Atoi(userData.Role)

	info := UserInfo{
		UserID:     userData.ID,
		Username:   userData.Username,
		Uploaded:   parseSize(userData.MemberCount.Uploaded),
		Downloaded: parseSize(userData.MemberCount.Downloaded),
		Ratio:      ratio,
		Bonus:      bonus,
		Rank:       rank,
		LevelName:  rank,
		LevelID:    levelID,
		LastUpdate: time.Now().Unix(),
	}

	// Parse join date
	if userData.CreatedDate != "" {
		if joinTime, err := ParseTimeInCST("2006-01-02 15:04:05", userData.CreatedDate); err == nil {
			info.JoinDate = joinTime.Unix()
		}
	}

	// Parse last access from lastBrowse
	if userData.MemberStatus.LastBrowse != "" {
		if accessTime, err := ParseTimeInCST("2006-01-02 15:04:05", userData.MemberStatus.LastBrowse); err == nil {
			info.LastAccess = accessTime.Unix()
		}
	}

	return info, nil
}

// mapMTorrentRole maps M-Team role ID to human readable class name
func mapMTorrentRole(roleID string) string {
	roles := map[string]string{
		"1":  "User",
		"2":  "Power User",
		"3":  "Elite User",
		"4":  "Crazy User",
		"5":  "Insane User",
		"6":  "Veteran User",
		"7":  "Extreme User",
		"8":  "Ultimate User",
		"9":  "Nexus Master",
		"10": "VIP",
		"11": "Retiree",
		"12": "Uploader",
		"13": "Moderator",
		"14": "Administrator",
		"15": "Sysop",
	}
	if name, ok := roles[roleID]; ok {
		return name
	}
	return "User"
}

// PrepareDownload prepares a request for downloading a torrent
func (d *MTorrentDriver) PrepareDownload(torrentID string) (MTorrentRequest, error) {
	// M-Team genDlToken API - use form-urlencoded format
	// The API expects: id=<torrent_id>
	return MTorrentRequest{
		Endpoint:    "/api/torrent/genDlToken",
		Method:      "POST",
		Body:        fmt.Sprintf("id=%s", torrentID),
		ContentType: "application/x-www-form-urlencoded",
	}, nil
}

// ParseDownload extracts torrent file data from the response
func (d *MTorrentDriver) ParseDownload(res MTorrentResponse) ([]byte, error) {
	if !res.Code.IsSuccess() {
		// Log raw response for debugging
		rawBody := string(res.RawBody)
		if len(rawBody) > 500 {
			rawBody = rawBody[:500] + "..."
		}
		return nil, fmt.Errorf("API error: %s - %s (raw: %s)", res.Code.String(), res.Message, rawBody)
	}

	// M-Team returns the download URL directly as a string in the data field
	// Example: {"code":"0","message":"SUCCESS","data":"https://api.m-team.cc/api/rss/dlv2?sign=..."}
	var downloadURL string
	if err := json.Unmarshal(res.Data, &downloadURL); err != nil {
		return nil, fmt.Errorf("parse download URL: %w (raw: %s)", err, string(res.Data))
	}

	if downloadURL == "" {
		return nil, fmt.Errorf("empty download URL in response (data: %s)", string(res.Data))
	}

	// Debug log
	if DebugUserInfo {
		fmt.Printf("[DEBUG MTorrent] Download URL: %s\n", downloadURL)
	}

	// Fetch the actual torrent file using requests library
	resp, err := requests.Get(downloadURL, requests.WithHeader("User-Agent", d.userAgent))
	if err != nil {
		return nil, fmt.Errorf("fetch torrent file: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching torrent from %s", resp.StatusCode, downloadURL)
	}

	return resp.Bytes(), nil
}

// parseMTorrentDiscount parses M-Team discount string to DiscountLevel
func parseMTorrentDiscount(discount string) DiscountLevel {
	discount = strings.ToUpper(strings.TrimSpace(discount))

	switch discount {
	case "FREE":
		return DiscountFree
	case "2XFREE", "_2X_FREE":
		return Discount2xFree
	case "PERCENT_50", "50%":
		return DiscountPercent50
	case "PERCENT_30", "30%":
		return DiscountPercent30
	case "PERCENT_70", "70%":
		return DiscountPercent70
	case "2XUP", "_2X_UP":
		return Discount2xUp
	case "_2X_PERCENT_50", "2X50%":
		return Discount2x50
	default:
		// Check for numeric discount
		if strings.Contains(discount, "50") {
			return DiscountPercent50
		}
		if strings.Contains(discount, "30") {
			return DiscountPercent30
		}
		if strings.Contains(discount, "70") {
			return DiscountPercent70
		}
		return DiscountNone
	}
}

func parseMTorrentDiscountWithPromotion(baseDiscount, baseEndTime string, promotion *MTorrentPromotionRule) (DiscountLevel, time.Time) {
	return parseMTorrentDiscountWithPromotionAt(baseDiscount, baseEndTime, promotion, time.Now())
}

func parseMTorrentDiscountWithPromotionAt(baseDiscount, baseEndTime string, promotion *MTorrentPromotionRule, now time.Time) (DiscountLevel, time.Time) {
	baseLevel := parseMTorrentDiscount(baseDiscount)
	var baseEnd time.Time
	if baseEndTime != "" {
		baseEnd, _ = ParseTimeInCST("2006-01-02 15:04:05", baseEndTime)
	}

	if promotion != nil && promotion.Discount != "" {
		var promoStart, promoEnd time.Time
		if promotion.StartTime != "" {
			promoStart, _ = ParseTimeInCST("2006-01-02 15:04:05", promotion.StartTime)
		}
		if promotion.EndTime != "" {
			promoEnd, _ = ParseTimeInCST("2006-01-02 15:04:05", promotion.EndTime)
		}

		promoActive := (promoStart.IsZero() || !now.Before(promoStart)) && (promoEnd.IsZero() || now.Before(promoEnd))
		if promoActive {
			promoLevel := parseMTorrentDiscount(promotion.Discount)
			if IsBetterDiscount(promoLevel, baseLevel) {
				return promoLevel, promoEnd
			}
			if promoLevel == baseLevel && promoEnd.After(baseEnd) {
				return promoLevel, promoEnd
			}
		}
	}

	return baseLevel, baseEnd
}

// ============================================================================
// Extended User Info Methods (时魔、未读消息、做种统计)
// ============================================================================

// MTorrentBonusResponse represents the response from /api/tracker/mybonus
type MTorrentBonusResponse struct {
	Code    FlexibleCode `json:"code"`
	Message string       `json:"message"`
	Data    struct {
		FormulaParams struct {
			FinalBs json.Number `json:"finalBs"` // Bonus per hour (时魔) - can be string or number
		} `json:"formulaParams"`
	} `json:"data"`
}

// MTorrentMessageStatResponse represents the response from /api/msg/notify/statistic
type MTorrentMessageStatResponse struct {
	Code    FlexibleCode `json:"code"`
	Message string       `json:"message"`
	Data    struct {
		Count  string `json:"count"`  // Total message count
		UnMake string `json:"unMake"` // Unread message count (string in API response)
	} `json:"data"`
}

// MTorrentPeerStatResponse represents the response from /api/tracker/myPeerStatistics
type MTorrentPeerStatResponse struct {
	Code    FlexibleCode `json:"code"`
	Message string       `json:"message"`
	Data    struct {
		UID          string `json:"uid"`
		SeederCount  string `json:"seederCount"`
		SeederSize   string `json:"seederSize"`
		LeecherCount string `json:"leecherCount"`
		LeecherSize  string `json:"leecherSize"`
		UploadCount  string `json:"uploadCount"`
	} `json:"data"`
}

// PrepareGetBonusPerHour prepares a request for fetching bonus per hour
func (d *MTorrentDriver) PrepareGetBonusPerHour() (MTorrentRequest, error) {
	return MTorrentRequest{
		Endpoint: "/api/tracker/mybonus",
		Method:   "POST",
		Body:     map[string]any{},
	}, nil
}

// ParseBonusPerHour extracts bonus per hour from the response
func (d *MTorrentDriver) ParseBonusPerHour(res MTorrentResponse) (float64, error) {
	if !res.Code.IsSuccess() {
		return 0, fmt.Errorf("%w: %s - %s", ErrBonusInfoUnavailable, res.Code.String(), res.Message)
	}

	// Debug: log raw response

	var bonusData MTorrentBonusResponse
	if err := json.Unmarshal(res.RawBody, &bonusData); err != nil {
		return 0, fmt.Errorf("parse bonus data: %w", err)
	}

	// Parse finalBs which can be string or number
	bonus, err := bonusData.Data.FormulaParams.FinalBs.Float64()
	if err != nil {
		return 0, fmt.Errorf("parse finalBs: %w", err)
	}

	return bonus, nil
}

// PrepareGetUnreadMessageCount prepares a request for fetching unread message count
func (d *MTorrentDriver) PrepareGetUnreadMessageCount() (MTorrentRequest, error) {
	return MTorrentRequest{
		Endpoint: "/api/msg/notify/statistic",
		Method:   "POST",
		Body:     map[string]any{},
	}, nil
}

// ParseUnreadMessageCount extracts unread message count from the response
func (d *MTorrentDriver) ParseUnreadMessageCount(res MTorrentResponse) (int, int, error) {
	if !res.Code.IsSuccess() {
		return 0, 0, fmt.Errorf("%w: %s - %s", ErrMessageCountUnavailable, res.Code.String(), res.Message)
	}

	// Debug: log raw response

	var msgData MTorrentMessageStatResponse
	if err := json.Unmarshal(res.RawBody, &msgData); err != nil {
		return 0, 0, fmt.Errorf("parse message data: %w", err)
	}

	// Parse string values to integers
	unread, _ := strconv.Atoi(msgData.Data.UnMake)
	total, _ := strconv.Atoi(msgData.Data.Count)

	return unread, total, nil
}

// PrepareGetPeerStatistics prepares a request for fetching peer statistics
func (d *MTorrentDriver) PrepareGetPeerStatistics() (MTorrentRequest, error) {
	return MTorrentRequest{
		Endpoint: "/api/tracker/myPeerStatistics",
		Method:   "POST",
		Body:     map[string]any{},
	}, nil
}

// ParsePeerStatistics extracts peer statistics from the response
func (d *MTorrentDriver) ParsePeerStatistics(res MTorrentResponse) (*PeerStatistics, error) {
	if !res.Code.IsSuccess() {
		return nil, fmt.Errorf("%w: %s - %s", ErrPeerStatsUnavailable, res.Code.String(), res.Message)
	}

	var peerData MTorrentPeerStatResponse
	if err := json.Unmarshal(res.RawBody, &peerData); err != nil {
		return nil, fmt.Errorf("parse peer data: %w", err)
	}

	// Parse string values to integers
	seederCount, _ := strconv.Atoi(peerData.Data.SeederCount)
	seederSize, _ := strconv.ParseInt(peerData.Data.SeederSize, 10, 64)
	leecherCount, _ := strconv.Atoi(peerData.Data.LeecherCount)
	leecherSize, _ := strconv.ParseInt(peerData.Data.LeecherSize, 10, 64)

	return &PeerStatistics{
		SeederCount:  seederCount,
		SeederSize:   seederSize,
		LeecherCount: leecherCount,
		LeecherSize:  leecherSize,
	}, nil
}

// GetBonusPerHour fetches the bonus per hour (时魔) for the user
func (d *MTorrentDriver) GetBonusPerHour(ctx context.Context) (float64, error) {
	req, err := d.PrepareGetBonusPerHour()
	if err != nil {
		return 0, err
	}

	res, err := d.Execute(ctx, req)
	if err != nil {
		return 0, err
	}

	return d.ParseBonusPerHour(res)
}

// GetUnreadMessageCount fetches the unread message count for the user
// Returns (unread, total, error)
func (d *MTorrentDriver) GetUnreadMessageCount(ctx context.Context) (int, int, error) {
	req, err := d.PrepareGetUnreadMessageCount()
	if err != nil {
		return 0, 0, err
	}

	res, err := d.Execute(ctx, req)
	if err != nil {
		return 0, 0, err
	}

	return d.ParseUnreadMessageCount(res)
}

// GetPeerStatistics fetches the peer statistics for the user
func (d *MTorrentDriver) GetPeerStatistics(ctx context.Context) (*PeerStatistics, error) {
	req, err := d.PrepareGetPeerStatistics()
	if err != nil {
		return nil, err
	}

	res, err := d.Execute(ctx, req)
	if err != nil {
		return nil, err
	}

	return d.ParsePeerStatistics(res)
}

// GetUserInfo fetches complete user information including extended stats
// Uses parallel requests for independent API calls to improve performance
func (d *MTorrentDriver) GetUserInfo(ctx context.Context) (UserInfo, error) {
	startTime := time.Now()

	var info UserInfo
	var bonusPerHour float64
	var unread, total int
	var peerStats *PeerStatistics
	var mu sync.Mutex

	// Use errgroup for cleaner parallel execution with error handling
	g, gctx := errgroup.WithContext(ctx)

	// 1. Get basic user info (critical - errors will cancel other goroutines)
	g.Go(func() error {
		userInfoReq, err := d.PrepareUserInfo()
		if err != nil {
			return fmt.Errorf("prepare user info: %w", err)
		}

		userInfoRes, err := d.Execute(gctx, userInfoReq)
		if err != nil {
			return fmt.Errorf("execute user info: %w", err)
		}

		parsedInfo, err := d.ParseUserInfo(userInfoRes)
		if err != nil {
			return fmt.Errorf("parse user info: %w", err)
		}

		mu.Lock()
		info = parsedInfo
		mu.Unlock()
		return nil
	})

	// 2. Get bonus per hour (non-critical - errors are ignored)
	g.Go(func() error {
		bonus, err := d.GetBonusPerHour(gctx)
		if err == nil {
			mu.Lock()
			bonusPerHour = bonus
			mu.Unlock()
		}
		return nil // Don't fail the whole operation
	})

	// 3. Get unread message count (non-critical)
	g.Go(func() error {
		u, t, err := d.GetUnreadMessageCount(gctx)
		if err == nil {
			mu.Lock()
			unread = u
			total = t
			mu.Unlock()
		}
		return nil
	})

	// 4. Get peer statistics (non-critical)
	g.Go(func() error {
		stats, err := d.GetPeerStatistics(gctx)
		if err == nil && stats != nil {
			mu.Lock()
			peerStats = stats
			mu.Unlock()
		}
		return nil
	})

	// Wait for all requests to complete
	if err := g.Wait(); err != nil {
		return UserInfo{}, err
	}

	// Merge results
	info.BonusPerHour = bonusPerHour
	info.UnreadMessageCount = unread
	info.TotalMessageCount = total

	if peerStats != nil {
		info.SeederCount = peerStats.SeederCount
		info.SeederSize = peerStats.SeederSize
		info.LeecherCount = peerStats.LeecherCount
		info.LeecherSize = peerStats.LeecherSize
		info.Seeding = peerStats.SeederCount
		info.Leeching = peerStats.LeecherCount
	}

	if DebugUserInfo {
		fmt.Printf("[DEBUG] MTorrent GetUserInfo completed in %v\n", time.Since(startTime))
	}

	return info, nil
}

func (d *MTorrentDriver) GetTorrentDetail(ctx context.Context, guid, _, _ string) (*TorrentItem, error) {
	req := MTorrentRequest{
		Endpoint:    "/api/torrent/detail",
		Method:      "POST",
		Body:        fmt.Sprintf("id=%s", guid),
		ContentType: "application/x-www-form-urlencoded",
	}

	res, err := d.Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	if !res.Code.IsSuccess() {
		return nil, fmt.Errorf("API error: %s - %s", res.Code.String(), res.Message)
	}

	var detail MTorrentTorrent
	if err := json.Unmarshal(res.Data, &detail); err != nil {
		return nil, fmt.Errorf("parse torrent detail: %w", err)
	}

	discount, discountEndTime := parseMTorrentDiscountWithPromotion(detail.Status.Discount, detail.Status.DiscountEndTime, detail.Status.PromotionRule)
	item := &TorrentItem{
		ID:              detail.ID,
		Title:           detail.Name,
		Seeders:         int(detail.Status.Seeders),
		Leechers:        int(detail.Status.Leechers),
		Snatched:        int(detail.Status.TimesCompleted),
		SourceSite:      d.getSiteID(),
		DiscountLevel:   discount,
		DiscountEndTime: discountEndTime,
	}

	if detail.SmallDescr != "" {
		item.Tags = []string{detail.SmallDescr}
	}
	if detail.Size != "" {
		if sizeBytes, err := strconv.ParseInt(detail.Size, 10, 64); err == nil {
			item.SizeBytes = sizeBytes
		}
	}

	return item, nil
}

func (d *MTorrentDriver) getSiteID() string {
	if d.siteDefinition != nil {
		return d.siteDefinition.ID
	}
	return "mteam"
}

func init() {
	RegisterDriverForSchema("mTorrent", createMTorrentSite)
}

func createMTorrentSite(config SiteConfig, logger *zap.Logger) (Site, error) {
	var opts MTorrentOptions
	if len(config.Options) > 0 {
		if err := json.Unmarshal(config.Options, &opts); err != nil {
			return nil, fmt.Errorf("parse MTorrent options: %w", err)
		}
	}

	if opts.APIKey == "" {
		return nil, fmt.Errorf("MTorrent site requires apiKey")
	}

	siteDef := GetDefinitionRegistry().GetOrDefault(config.ID)

	driver := NewMTorrentDriver(MTorrentDriverConfig{
		BaseURL: config.BaseURL,
		APIKey:  opts.APIKey,
	})

	if siteDef != nil {
		driver.SetSiteDefinition(siteDef)
	}

	return NewBaseSite(driver, BaseSiteConfig{
		ID:        config.ID,
		Name:      config.Name,
		Kind:      SiteMTorrent,
		RateLimit: config.RateLimit,
		RateBurst: config.RateBurst,
		Logger:    logger.With(zap.String("site", config.ID)),
	}), nil
}
