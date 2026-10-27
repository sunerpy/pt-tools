package v2

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/sunerpy/pt-tools/utils"
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

// MTorrentPromotionRule represents an additional promotion rule from M-Team API.
// 接口里的促销规则没有状态字段，只能按起止时间判断是否生效；Categories、Teams 是适用范围，空列表表示全站。
type MTorrentPromotionRule struct {
	Discount   string         `json:"discount"`
	StartTime  string         `json:"startTime,omitempty"`
	EndTime    string         `json:"endTime,omitempty"`
	Categories MTorrentIDList `json:"categories"`
	Teams      MTorrentIDList `json:"teams"`
}

// MallSingleFree represents a mall single free from M-Team API
type MallSingleFree struct {
	Status    string `json:"status,omitempty"`
	StartDate string `json:"startDate,omitempty"`
	EndDate   string `json:"endDate,omitempty"`
}

// MTorrentID 是可能以字符串或数字出现的编号（例如种子的 team）。解不开时当作没有，不报错：
// 新读取的字段不能让原本能解析的整页结果失败。
type MTorrentID string

// UnmarshalJSON 接受字符串或数字；null、对象、数组当作没有
func (id *MTorrentID) UnmarshalJSON(data []byte) error {
	v, _ := decodeMTorrentID(data)
	*id = MTorrentID(v)
	return nil
}

// MTorrentIDList 是促销规则里的编号列表（categories、teams），元素可能是字符串也可能是数字。
// 解不开时不报错（一个字段异常不该让整页结果都解析失败），记为 Invalid：无法证明覆盖，促销不适用。
type MTorrentIDList struct {
	Values  []string
	Invalid bool
}

// UnmarshalJSON 接受字符串或数字组成的数组；null 当作空列表
func (l *MTorrentIDList) UnmarshalJSON(data []byte) error {
	*l = MTorrentIDList{}
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		l.Invalid = true
		return nil
	}
	for _, r := range raw {
		v, ok := decodeMTorrentID(r)
		if !ok {
			l.Invalid = true
			continue
		}
		l.Values = append(l.Values, v)
	}
	return nil
}

// covers 报告范围是否覆盖 id：空列表表示全站适用；列表非空时 id 必须在列表里，id 缺失就无法证明覆盖。
func (l MTorrentIDList) covers(id string) bool {
	if l.Invalid {
		return false
	}
	if len(l.Values) == 0 {
		return true
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, v := range l.Values {
		if v == id {
			return true
		}
	}
	return false
}

// decodeMTorrentID 把字符串或数字形式的编号解成去掉首尾空白的字符串；其他形式返回 ok=false
func decodeMTorrentID(data []byte) (string, bool) {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		return strings.TrimSpace(s), true
	}
	var n json.Number
	if err := json.Unmarshal(data, &n); err == nil {
		return n.String(), true
	}
	return "", false
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
		MallSingleFree  *MallSingleFree        `json:"mallSingleFree,omitempty"`
	} `json:"status"`
	Category string `json:"category"`
	// Team 是制作组编号，用来判断促销规则的 teams 范围
	Team MTorrentID `json:"team"`
	// IMDb、Douban 是条目链接（或编号），用 NormalizeIMDbID、NormalizeDoubanID 规整
	IMDb   string `json:"imdb"`
	Douban string `json:"douban"`
}

// MTorrentUserInfo represents user info from M-Team API
type MTorrentUserInfo struct {
	ID               string               `json:"id"`
	Username         string               `json:"username"`
	CreatedDate      string               `json:"createdDate"`
	LastModifiedDate string               `json:"lastModifiedDate"`
	Role             string               `json:"role"`
	MemberCount      MTorrentMemberCount  `json:"memberCount"`
	MemberStatus     MTorrentMemberStatus `json:"memberStatus"`
}

// MTorrentMemberCount contains upload/download stats
type MTorrentMemberCount struct {
	Uploaded       string `json:"uploaded"`
	Downloaded     string `json:"downloaded"`
	TrueUploaded   string `json:"trueUploaded"`
	TrueDownloaded string `json:"trueDownloaded"`
	Bonus          string `json:"bonus"`
	ShareRate      string `json:"shareRate"`
	Seedtime       string `json:"seedtime"`
	Leechtime      string `json:"leechtime"`
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
	LoginCookie    string // 仅用于登录态探测，绝不用于检索/下载请求
	httpClient     *SiteHTTPClient
	failoverClient *FailoverHTTPClient
	userAgent      string
	useFailover    bool
	siteDefinition *SiteDefinition
	// now 是判断优惠是否生效用的时钟，测试用来固定时间；为空时用 time.Now
	now func() time.Time
}

// MTorrentDriverConfig holds configuration for creating an M-Team driver
type MTorrentDriverConfig struct {
	BaseURL     string
	WebURL      string // Optional: Web URL for detail pages, defaults to "https://kp.m-team.cc"
	APIKey      string
	LoginCookie string          // 仅用于登录态探测，绝不用于检索/下载请求
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
		LoginCookie: config.LoginCookie,
		httpClient:  httpClient,
		userAgent:   userAgent,
		useFailover: config.UseFailover,
	}

	// Initialize failover client if enabled
	if config.UseFailover {
		registry := GetGlobalRegistry()
		if failoverClient, err := registry.GetFailoverClient(
			SiteNameMTeam,
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

// MTeamSectionNormal、MTeamSectionAdult 是 M-Team 搜索接口的 mode：综合区与成人区。
const (
	MTeamSectionNormal = "normal"
	MTeamSectionAdult  = "adult"
)

// PrepareSearch converts a SearchQuery to an M-Team request.
//
// mode 取 query.Section，只认 normal 和 adult，空值或别的值都用 normal（综合区）。
// 有意忽略 FreeOnly，不往请求里加 discount 筛选：单种免费（mallSingleFree）的种子本身的优惠不是 FREE，
// 交给服务端按 discount 筛会把它漏掉；免费与否由 evaluateMTorrentDiscount 在本地判断。
func (d *MTorrentDriver) PrepareSearch(query SearchQuery) (MTorrentRequest, error) {
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}

	pageNumber := query.Page
	if pageNumber <= 0 {
		pageNumber = 1
	}

	mode := MTeamSectionNormal
	if query.Section == MTeamSectionAdult {
		mode = MTeamSectionAdult
	}
	body := MTorrentSearchRequest{
		Mode:       mode,
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

	resp, err := d.httpClient.Post(ctx, fullURL, bodyBytes, headers)
	if err != nil {
		return MTorrentResponse{}, fmt.Errorf("execute request: %w", err)
	}

	result := MTorrentResponse{
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

	now := d.clock()
	items := make([]TorrentItem, 0, len(searchData.Data))
	for _, t := range searchData.Data {
		item := TorrentItem{
			ID:         t.ID,
			Title:      t.Name,
			Subtitle:   t.SmallDescr,
			SizeBytes:  parseSize(t.Size),
			Seeders:    t.Status.Seeders.Int(),
			Leechers:   t.Status.Leechers.Int(),
			Snatched:   t.Status.TimesCompleted.Int(),
			SourceSite: d.BaseURL,
			Category:   getMTeamCategoryName(t.Category),
			IMDbID:     NormalizeIMDbID(t.IMDb),
			DoubanID:   NormalizeDoubanID(t.Douban),
		}
		applyMTorrentDiscount(&item, evaluateMTorrentDiscount(&t, now))

		// Parse upload time
		if t.CreatedDate != "" {
			if uploadTime, err := ParseTimeInCST("2006-01-02 15:04:05", t.CreatedDate); err == nil {
				item.UploadedAt = uploadTime.Unix()
			}
		}

		// Build download URL
		// For M-Team sites, direct download is not possible, we set a proxy download URL
		// The frontend should call /api/site/{siteId}/torrent/{id}/download for actual download
		item.DownloadURL = fmt.Sprintf("/api/site/%s/torrent/%s/download", d.getSiteID(), t.ID)
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
		UserID:         userData.ID,
		Username:       userData.Username,
		Uploaded:       parseSize(userData.MemberCount.Uploaded),
		Downloaded:     parseSize(userData.MemberCount.Downloaded),
		TrueUploaded:   parseSize(userData.MemberCount.TrueUploaded),
		TrueDownloaded: parseSize(userData.MemberCount.TrueDownloaded),
		Ratio:          ratio,
		Bonus:          bonus,
		Rank:           rank,
		LevelName:      rank,
		LevelID:        levelID,
		LastUpdate:     time.Now().Unix(),
	}

	// Parse join date
	if userData.CreatedDate != "" {
		if joinTime, err := ParseTimeInCST("2006-01-02 15:04:05", userData.CreatedDate); err == nil {
			info.JoinDate = joinTime.Unix()
		}
	}

	// 最近登录：登录探测把它写进 API 登录时间
	if userData.MemberStatus.LastLogin != "" {
		if loginTime, err := ParseTimeInCST("2006-01-02 15:04:05", userData.MemberStatus.LastLogin); err == nil {
			info.LastLogin = loginTime.Unix()
		}
	}

	// Parse last access from lastBrowse
	if userData.MemberStatus.LastBrowse != "" {
		if accessTime, err := ParseTimeInCST("2006-01-02 15:04:05", userData.MemberStatus.LastBrowse); err == nil {
			info.LastAccess = accessTime.Unix()
		}
	}

	// lastModifiedDate is the authoritative profile-update timestamp used by
	// the site-login keepalive subsystem; format is "2026-05-15T12:00:00.000+0800".
	if userData.LastModifiedDate != "" {
		if modTime, err := time.Parse("2006-01-02T15:04:05.000-0700", userData.LastModifiedDate); err == nil {
			info.LastAccess = modTime.Unix()
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

// rawURLRe 是原始响应里的地址：签名下载地址的 sign 参数本身就是凭证，写进错误前先脱敏。
var rawURLRe = regexp.MustCompile(`https?://[^\s"'<>\\]+`)

// sanitizeRawURLs 把原始响应文本里的地址换成脱敏后的样子（凭证参数打码），其余原样留着便于排查。
func sanitizeRawURLs(raw string) string {
	return rawURLRe.ReplaceAllStringFunc(raw, utils.SanitizeURL)
}

// ParseDownload extracts torrent file data from the response
func (d *MTorrentDriver) ParseDownload(res MTorrentResponse) ([]byte, error) {
	return d.ParseDownloadContext(context.Background(), res)
}

// ParseDownloadContext 同 ParseDownload：拉种子文件的第二段请求跟着 ctx 取消（BaseSite 下载时用它）。
func (d *MTorrentDriver) ParseDownloadContext(ctx context.Context, res MTorrentResponse) ([]byte, error) {
	if !res.Code.IsSuccess() {
		// Log raw response for debugging
		rawBody := string(res.RawBody)
		if len(rawBody) > 500 {
			rawBody = rawBody[:500] + "..."
		}
		return nil, fmt.Errorf("API error: %s - %s (raw: %s)", res.Code.String(), sanitizeRawURLs(res.Message), sanitizeRawURLs(rawBody))
	}

	// M-Team returns the download URL directly as a string in the data field
	// Example: {"code":"0","message":"SUCCESS","data":"https://api.m-team.cc/api/rss/dlv2?sign=..."}
	var downloadURL string
	if err := json.Unmarshal(res.Data, &downloadURL); err != nil {
		return nil, fmt.Errorf("parse download URL: %w (raw: %s)", err, sanitizeRawURLs(string(res.Data)))
	}

	if downloadURL == "" {
		return nil, fmt.Errorf("empty download URL in response (data: %s)", sanitizeRawURLs(string(res.Data)))
	}
	return d.fetchTorrentFile(ctx, downloadURL)
}

// mteamTorrentFetchTimeout 是取签名下载地址之后拉一次种子文件的超时；测试里会调小。
var mteamTorrentFetchTimeout = 30 * time.Second

// fetchTorrentFile 拉签名地址指向的种子文件：签名地址会跳到走 Cloudflare 的下载节点，连接被重置、超时时有发生，
// 一时的错误按指数退避重拉同一个签名地址（见 retryTransient，不再调取下载令牌的接口），调用方取消时不再试。签名地址本身
// 就是凭证（sign 参数），它跳转到的下载节点地址也带签名：日志和错误信息里只出现脱敏后的地址。
func (d *MTorrentDriver) fetchTorrentFile(ctx context.Context, downloadURL string) ([]byte, error) {
	safeURL := utils.SanitizeURL(downloadURL)
	if DebugUserInfo {
		fmt.Printf("[DEBUG MTorrent] Download URL: %s\n", safeURL)
	}
	return retryTransient(ctx, nil, func() ([]byte, error) { return d.fetchTorrentOnce(ctx, downloadURL, safeURL) })
}

// fetchTorrentOnce 拉一次种子文件。第二段请求同样走驱动的 HTTP 客户端（代理设置一致），每次设上限超时。
func (d *MTorrentDriver) fetchTorrentOnce(ctx context.Context, downloadURL, safeURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, mteamTorrentFetchTimeout)
	defer cancel()
	resp, err := d.httpClient.Get(ctx, downloadURL, map[string]string{
		"User-Agent": d.userAgent,
		"Accept":     "application/x-bittorrent, */*",
	})
	if err != nil {
		return nil, fmt.Errorf("fetch torrent file from %s: %w", safeURL, redactURLsInError(err))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w fetching torrent from %s", &HTTPStatusError{Code: resp.StatusCode}, safeURL)
	}

	data := resp.Body
	if err := ValidateTorrentFile(data); err != nil {
		return nil, fmt.Errorf("invalid torrent response from %s: size=%d, preview=%q, err=%w", safeURL, len(data), torrentResponsePreview(data), err)
	}

	return data, nil
}

func torrentResponsePreview(data []byte) string {
	preview := strings.ToValidUTF8(string(data), "")
	preview = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, preview)
	preview = strings.TrimSpace(preview)
	preview = regexp.MustCompile(`\s+`).ReplaceAllString(preview, " ")
	runes := []rune(preview)
	if len(runes) > 160 {
		preview = string(runes[:160])
	}
	return preview
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

// M-Team 优惠的来源（TorrentItem.DiscountSource）
const (
	mteamSourceBase           = "base"
	mteamSourcePromotionRule  = "promotion_rule"
	mteamSourceMallSingleFree = "mall_single_free"
)

// mteamTimeLayout 是 M-Team 接口的时间格式：北京时间，不带时区
const mteamTimeLayout = "2006-01-02 15:04:05"

// mteamDiscount 是一条 M-Team 种子优惠的严格解析结果
type mteamDiscount struct {
	Level   DiscountLevel
	EndTime time.Time // 零值：没有可信的结束时间
	// Validity：免费时是 free_until 或 unknown，不是免费时是 not_free。
	// M-Team 没有「长期有效」的明确标记，所以从不给 free_permanent。
	Validity FreeValidity
	Source   string // base、promotion_rule、mall_single_free；没有生效的优惠时为空
	Err      string // 读取时遇到的问题（字段名: 问题类型），多条用「; 」连接
}

// mteamCandidate 是一条当前生效（或因时间无法解析而可能生效）的优惠
type mteamCandidate struct {
	level   DiscountLevel
	end     time.Time // 零值：没有结束时间（缺失或无法解析）
	trusted bool      // 起止时间都可信，且结束时间晚于现在
	source  string
}

// better 报告 c 是否比 o 更该作为结果：下载计费比例低的优先；同比例时结束时间可信的优先；
// 都可信时结束更晚的优先（免费期取最长的那段）；最后看优惠本身（2X 免费优于免费）。
func (c mteamCandidate) better(o mteamCandidate) bool {
	if cr, or := c.level.GetDownloadRatio(), o.level.GetDownloadRatio(); cr != or {
		return cr < or
	}
	if c.trusted != o.trusted {
		return c.trusted
	}
	if c.trusted && !c.end.Equal(o.end) {
		return c.end.After(o.end)
	}
	return IsBetterDiscount(c.level, o.level)
}

// evaluateMTorrentDiscount 在 now 时严格解析一条种子的优惠。基础优惠、promotionRule、mallSingleFree 三者都看，
// 只留下当前生效的（时间无法解析的算「可能生效」，但不可信），再按 better 选出结果。
// 缺失或无法解析的结束时间不当作长期免费；基础优惠的结束时间已过就不再有效，不论哪一级。
func evaluateMTorrentDiscount(t *MTorrentTorrent, now time.Time) mteamDiscount {
	var notes []string
	var cands []mteamCandidate

	if level := parseMTorrentDiscount(t.Status.Discount); level != DiscountNone {
		if end, active, trusted := mteamWindow("", "", "discountEndTime", t.Status.DiscountEndTime, now, &notes); active {
			cands = append(cands, mteamCandidate{level: level, end: end, trusted: trusted, source: mteamSourceBase})
		}
	}

	// promotionRule 没有状态字段：适用范围覆盖这条种子、且在起止时间内才算生效
	if p := t.Status.PromotionRule; p != nil {
		if level := parseMTorrentDiscount(p.Discount); level != DiscountNone && mteamPromotionCovers(p, t, &notes) {
			end, active, trusted := mteamWindow("promotionRule.startTime", p.StartTime, "promotionRule.endTime", p.EndTime, now, &notes)
			if active {
				cands = append(cands, mteamCandidate{level: level, end: end, trusted: trusted, source: mteamSourcePromotionRule})
			}
		}
	}

	// 商城单种免费只认进行中（ONGOING）；没有结束时间同样是未知，不是长期免费
	if m := t.Status.MallSingleFree; m != nil && strings.EqualFold(strings.TrimSpace(m.Status), "ONGOING") {
		end, active, trusted := mteamWindow("mallSingleFree.startDate", m.StartDate, "mallSingleFree.endDate", m.EndDate, now, &notes)
		if active {
			cands = append(cands, mteamCandidate{level: DiscountFree, end: end, trusted: trusted, source: mteamSourceMallSingleFree})
		}
	}

	result := mteamDiscount{Level: DiscountNone, Validity: FreeNotFree, Err: joinDiscountNotes(notes...)}
	if len(cands) == 0 {
		return result
	}
	best := cands[0]
	for _, c := range cands[1:] {
		if c.better(best) {
			best = c
		}
	}
	result.Level, result.EndTime, result.Source = best.level, best.end, best.source
	switch {
	case !IsFreeTorrent(best.level):
		result.Validity = FreeNotFree
	case best.trusted:
		result.Validity = FreeUntil
	default:
		result.Validity = FreeUnknown
	}
	return result
}

// mteamWindow 按起止时间判断一条优惠在 now 时是否生效。空的起止时间表示这一端没有约束（没有结束时间
// 不等于长期有效，只是结束时间未知）；非空但无法解析的时间记进 notes，这条优惠变成「可能生效、不可信」。
// active=false 表示确定不生效：还没开始，或已经结束（结束时间不晚于 now）。
func mteamWindow(startField, start, endField, end string, now time.Time, notes *[]string) (endAt time.Time, active, trusted bool) {
	startAt, startErr := parseMTeamTime(start)
	if startErr != nil {
		*notes = append(*notes, DiscountTimeParseNote(startField))
	}
	endAt, endErr := parseMTeamTime(end)
	if endErr != nil {
		*notes = append(*notes, DiscountTimeParseNote(endField))
	}
	if !startAt.IsZero() && startAt.After(now) {
		return time.Time{}, false, false
	}
	if !endAt.IsZero() && !endAt.After(now) {
		return time.Time{}, false, false
	}
	return endAt, true, startErr == nil && endErr == nil && !endAt.IsZero()
}

// parseMTeamTime 解析 M-Team 的时间字段；空串返回零时间且没有错误（这一端没有约束）
func parseMTeamTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	return ParseTimeInCST(mteamTimeLayout, value)
}

// mteamPromotionCovers 报告促销规则的适用范围是否覆盖这条种子；范围解不开时记下原因，当作不覆盖
func mteamPromotionCovers(p *MTorrentPromotionRule, t *MTorrentTorrent, notes *[]string) bool {
	if p.Categories.Invalid {
		*notes = append(*notes, discountFormatParseNote("promotionRule.categories"))
	}
	if p.Teams.Invalid {
		*notes = append(*notes, discountFormatParseNote("promotionRule.teams"))
	}
	return p.Categories.covers(t.Category) && p.Teams.covers(string(t.Team))
}

// applyMTorrentDiscount 把严格解析的结果写进 TorrentItem 的优惠字段
func applyMTorrentDiscount(item *TorrentItem, d mteamDiscount) {
	item.DiscountLevel = d.Level
	item.DiscountEndTime = d.EndTime
	item.DiscountValidity = d.Validity
	item.DiscountSource = d.Source
	item.DiscountErr = d.Err
}

// clock 返回判断优惠是否生效用的当前时间
func (d *MTorrentDriver) clock() time.Time {
	if d.now != nil {
		return d.now()
	}
	return time.Now()
}

// parseMTorrentDiscountWithPromotionAndMallSingleFreeAt 只返回级别与结束时间（兼容旧调用）；
// 有效性、来源与诊断说明见 evaluateMTorrentDiscount。不带种子的分类与制作组，有范围限制的促销不会生效。
func parseMTorrentDiscountWithPromotionAndMallSingleFreeAt(baseDiscount, baseEndTime string, promotion *MTorrentPromotionRule, mallSingleFree *MallSingleFree, now time.Time) (DiscountLevel, time.Time) {
	var t MTorrentTorrent
	t.Status.Discount, t.Status.DiscountEndTime = baseDiscount, baseEndTime
	t.Status.PromotionRule, t.Status.MallSingleFree = promotion, mallSingleFree
	d := evaluateMTorrentDiscount(&t, now)
	return d.Level, d.EndTime
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

	// 详情是实时请求：取得数据的时间既是判断优惠是否生效的时间，也是 FetchedAt
	fetchedAt := d.clock()
	item := &TorrentItem{
		ID:         detail.ID,
		Title:      detail.Name,
		Seeders:    int(detail.Status.Seeders),
		Leechers:   int(detail.Status.Leechers),
		Snatched:   int(detail.Status.TimesCompleted),
		SourceSite: d.getSiteID(),
		IMDbID:     NormalizeIMDbID(detail.IMDb),
		DoubanID:   NormalizeDoubanID(detail.Douban),
		FetchedAt:  fetchedAt.UTC(),
	}
	applyMTorrentDiscount(item, evaluateMTorrentDiscount(&detail, fetchedAt))

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
		BaseURL:     config.BaseURL,
		APIKey:      opts.APIKey,
		LoginCookie: opts.Cookie,
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
