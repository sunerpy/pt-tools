package definitions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/global"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

var RousiProDefinition = &v2.SiteDefinition{
	ID:          "rousipro",
	Name:        "Rousi Pro",
	Aka:         []string{"Rousi", "肉丝"},
	Description: "综合性PT站点，使用Bearer Token认证",
	Schema:      v2.SchemaRousi,
	AuthMethod:  v2.AuthMethodPasskey,
	RateLimit:   2.0,
	RateBurst:   5,
	URLs: []string{
		"https://rousi.pro",
	},
	FaviconURL:     "https://rousi.pro/favicon.ico",
	TimezoneOffset: "+0800",
	CreateDriver:   createRousiDriver,
}

func init() {
	v2.RegisterSiteDefinition(RousiProDefinition)
}

func createRousiDriver(config v2.SiteConfig, logger *zap.Logger) (v2.Site, error) {
	var opts v2.RousiOptions
	if len(config.Options) > 0 {
		if err := json.Unmarshal(config.Options, &opts); err != nil {
			return nil, fmt.Errorf("parse Rousi options: %w", err)
		}
	}

	if opts.Passkey == "" {
		return nil, fmt.Errorf("Rousi 站点需要配置 Passkey（从站点账户设置-Passkey中获取）")
	}

	siteDef := v2.GetDefinitionRegistry().GetOrDefault(config.ID)

	baseURL := config.BaseURL
	if baseURL == "" && siteDef != nil && len(siteDef.URLs) > 0 {
		baseURL = siteDef.URLs[0]
	}

	driver := newRousiDriver(rousiDriverConfig{
		BaseURL: baseURL,
		Passkey: opts.Passkey,
	})

	if siteDef != nil {
		driver.siteDefinition = siteDef
	}

	return v2.NewBaseSite(driver, v2.BaseSiteConfig{
		ID:        config.ID,
		Name:      config.Name,
		Kind:      v2.SiteKind("rousi"),
		RateLimit: config.RateLimit,
		RateBurst: config.RateBurst,
		Logger:    logger.With(zap.String("site", config.ID)),
	}), nil
}

type rousiRequest struct {
	Endpoint string
	Method   string
	Params   map[string]string
}

type rousiResponse struct {
	Code       int             `json:"code"`
	Message    string          `json:"message"`
	Data       json.RawMessage `json:"data"`
	RawBody    []byte          `json:"-"`
	StatusCode int             `json:"-"`
}

func (r *rousiResponse) IsSuccess() bool {
	return r.Code == 0
}

type rousiSearchResponse struct {
	Torrents []rousiTorrent `json:"torrents"`
	Total    int            `json:"total"`
}

type rousiTorrent struct {
	ID           int             `json:"id"`
	UUID         string          `json:"uuid"`
	Title        string          `json:"title"`
	Subtitle     string          `json:"subtitle"`
	Size         int64           `json:"size"`
	Seeders      int             `json:"seeders"`
	Leechers     int             `json:"leechers"`
	Downloads    int             `json:"downloads"`
	CreatedAt    string          `json:"created_at"`
	Uploader     string          `json:"uploader"`
	CategoryName string          `json:"category_name"`
	Promotion    *rousiPromotion `json:"promotion,omitempty"`
}

type rousiPromotion struct {
	Type           int     `json:"type"`
	IsActive       bool    `json:"is_active"`
	Until          string  `json:"until,omitempty"`
	DownMultiplier float64 `json:"down_multiplier"`
	UpMultiplier   float64 `json:"up_multiplier"`
	TimeType       int     `json:"time_type"`
	IsGlobal       bool    `json:"is_global"`
	Text           string  `json:"text,omitempty"`
}

type rousiUserData struct {
	ID                   int                       `json:"id"`
	Username             string                    `json:"username"`
	Level                int                       `json:"level"`
	LevelText            string                    `json:"level_text"`
	Uploaded             int64                     `json:"uploaded"`
	Downloaded           int64                     `json:"downloaded"`
	Ratio                float64                   `json:"ratio"`
	Karma                float64                   `json:"karma"`
	Credits              float64                   `json:"credits"`
	SeedingKarmaPerHour  float64                   `json:"seeding_karma_per_hour"`
	SeedingPointsPerHour float64                   `json:"seeding_points_per_hour"`
	SeedingTime          int64                     `json:"seeding_time"`
	RegisteredAt         string                    `json:"registered_at"`
	LastActiveAt         string                    `json:"last_active_at"`
	SeedingLeechingData  *rousiSeedingLeechingData `json:"seeding_leeching_data,omitempty"`
}

type rousiSeedingLeechingData struct {
	SeedingCount int   `json:"seeding_count"`
	SeedingSize  int64 `json:"seeding_size"`
}

type rousiDriver struct {
	BaseURL        string
	Passkey        string
	httpClient     *v2.SiteHTTPClient
	userAgent      string
	siteDefinition *v2.SiteDefinition
}

type rousiDriverConfig struct {
	BaseURL    string
	Passkey    string
	HTTPClient *v2.SiteHTTPClient
	UserAgent  string
}

func newRousiDriver(config rousiDriverConfig) *rousiDriver {
	userAgent := config.UserAgent
	if userAgent == "" {
		userAgent = "pt-tools/1.0"
	}

	baseURL := strings.TrimSuffix(config.BaseURL, "/")

	var httpClient *v2.SiteHTTPClient
	if config.HTTPClient != nil {
		httpClient = config.HTTPClient
	} else {
		httpClient = v2.NewSiteHTTPClient(v2.SiteHTTPClientConfig{
			Timeout:   30 * time.Second,
			UserAgent: userAgent,
		})
	}

	return &rousiDriver{
		BaseURL:    baseURL,
		Passkey:    config.Passkey,
		httpClient: httpClient,
		userAgent:  userAgent,
	}
}

func (d *rousiDriver) Execute(ctx context.Context, req rousiRequest) (rousiResponse, error) {
	fullURL := d.BaseURL + req.Endpoint

	if len(req.Params) > 0 {
		params := make([]string, 0, len(req.Params))
		for k, v := range req.Params {
			params = append(params, fmt.Sprintf("%s=%s", url.QueryEscape(k), url.QueryEscape(v)))
		}
		fullURL += "?" + strings.Join(params, "&")
	}

	method := req.Method
	if method == "" {
		method = http.MethodGet
	}

	headers := map[string]string{
		"Accept":        "application/json, text/plain, */*",
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + d.Passkey,
		"User-Agent":    d.userAgent,
	}

	resp, err := d.httpClient.DoRequest(ctx, method, fullURL, nil, headers)
	if err != nil {
		return rousiResponse{}, fmt.Errorf("request failed: %w", err)
	}

	result := rousiResponse{
		RawBody:    resp.Body,
		StatusCode: resp.StatusCode,
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return result, v2.ErrInvalidCredentials
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if len(resp.Body) > 0 {
			_ = json.Unmarshal(resp.Body, &result)
		}
		return result, fmt.Errorf("API error: HTTP %d - %s", resp.StatusCode, result.Message)
	}

	if len(resp.Body) > 0 {
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			return result, fmt.Errorf("parse response: %w", err)
		}
	}

	return result, nil
}

func (d *rousiDriver) PrepareSearch(query v2.SearchQuery) (rousiRequest, error) {
	params := map[string]string{
		"page_size": "100",
	}
	if query.Keyword != "" {
		params["keyword"] = query.Keyword
	}
	// API page starts from 1, not 0
	if query.Page > 0 {
		params["page"] = strconv.Itoa(query.Page + 1)
	} else {
		params["page"] = "1"
	}

	return rousiRequest{
		Endpoint: "/api/v1/torrents",
		Method:   http.MethodGet,
		Params:   params,
	}, nil
}

func (d *rousiDriver) ParseSearch(res rousiResponse) ([]v2.TorrentItem, error) {
	if !res.IsSuccess() {
		return nil, fmt.Errorf("API error: code=%d, message=%s", res.Code, res.Message)
	}

	var searchData rousiSearchResponse
	if err := json.Unmarshal(res.Data, &searchData); err != nil {
		return nil, fmt.Errorf("parse search data: %w (data: %s)", err, string(res.Data))
	}

	items := make([]v2.TorrentItem, 0, len(searchData.Torrents))
	siteID := d.getSiteID()

	for _, t := range searchData.Torrents {
		discount, discountEnd := d.parsePromotion(t.Promotion)

		item := v2.TorrentItem{
			ID:              t.UUID,
			Title:           t.Title,
			Subtitle:        t.Subtitle,
			SizeBytes:       t.Size,
			Seeders:         t.Seeders,
			Leechers:        t.Leechers,
			Snatched:        t.Downloads,
			DiscountLevel:   discount,
			DiscountEndTime: discountEnd,
			Category:        t.CategoryName,
			URL:             fmt.Sprintf("%s/torrent/%s", d.BaseURL, t.UUID),
			DownloadURL:     fmt.Sprintf("%s/api/torrent/%s/download/%s", d.BaseURL, t.UUID, d.Passkey),
			SourceSite:      siteID,
		}

		if t.CreatedAt != "" {
			if uploadTime, err := time.Parse(time.RFC3339, t.CreatedAt); err == nil {
				item.UploadedAt = uploadTime.Unix()
			} else if uploadTime, err := v2.ParseTimeInCST("2006-01-02 15:04:05", t.CreatedAt); err == nil {
				item.UploadedAt = uploadTime.Unix()
			}
		}

		items = append(items, item)
	}

	return items, nil
}

func (d *rousiDriver) parsePromotion(promo *rousiPromotion) (v2.DiscountLevel, time.Time) {
	if promo == nil || !promo.IsActive {
		return v2.DiscountNone, time.Time{}
	}

	if logger := global.GetSloggerSafe(); logger != nil {
		logger.Debugf("[RousiPro] promotion: type=%d, down=%.2f, up=%.2f, text=%s, until=%s",
			promo.Type, promo.DownMultiplier, promo.UpMultiplier, promo.Text, promo.Until)
	}

	var endTime time.Time
	if promo.Until != "" {
		for _, layout := range []string{
			time.RFC3339,
			"2006-01-02T15:04:05-0700",
			"2006-01-02 15:04:05",
		} {
			if t, err := time.Parse(layout, promo.Until); err == nil {
				if layout == "2006-01-02 15:04:05" {
					t = t.In(time.FixedZone("CST", 8*3600))
				}
				endTime = t
				break
			}
		}
	}

	var level v2.DiscountLevel
	switch promo.Type {
	case 0, 1:
		level = v2.DiscountNone
	case 2:
		level = v2.DiscountFree
	case 3:
		level = v2.Discount2xUp
	case 4:
		level = v2.Discount2xFree
	case 5:
		level = v2.DiscountPercent50
	case 6:
		level = v2.Discount2x50
	case 7:
		level = v2.DiscountPercent30
	default:
		if promo.DownMultiplier == 0 && promo.UpMultiplier >= 2 {
			level = v2.Discount2xFree
		} else if promo.DownMultiplier == 0 {
			level = v2.DiscountFree
		} else {
			level = v2.DiscountNone
		}
	}
	return level, endTime
}

func (d *rousiDriver) GetUserInfo(ctx context.Context) (v2.UserInfo, error) {
	req := rousiRequest{
		Endpoint: "/api/v1/profile",
		Method:   http.MethodGet,
		Params: map[string]string{
			"include_fields[user]": "seeding_leeching_data",
		},
	}

	res, err := d.Execute(ctx, req)
	if err != nil {
		return v2.UserInfo{}, fmt.Errorf("execute user info: %w", err)
	}

	if logger := global.GetSloggerSafe(); logger != nil {
		logger.Debugf("[RousiPro] GetUserInfo raw response: %s", string(res.RawBody))
	}

	if !res.IsSuccess() {
		return v2.UserInfo{}, fmt.Errorf("API error: code=%d, message=%s", res.Code, res.Message)
	}

	var userData rousiUserData
	if err := json.Unmarshal(res.Data, &userData); err != nil {
		return v2.UserInfo{}, fmt.Errorf("parse user data: %w (data: %s)", err, string(res.Data))
	}

	info := v2.UserInfo{
		Site:                d.getSiteID(),
		UserID:              strconv.Itoa(userData.ID),
		Username:            userData.Username,
		Uploaded:            userData.Uploaded,
		Downloaded:          userData.Downloaded,
		Ratio:               userData.Ratio,
		Bonus:               userData.Karma,
		SeedingBonus:        userData.Credits,
		BonusPerHour:        userData.SeedingKarmaPerHour,
		SeedingBonusPerHour: userData.SeedingPointsPerHour,
		LevelID:             userData.Level,
		LevelName:           userData.LevelText,
		Rank:                userData.LevelText,
		LastUpdate:          time.Now().Unix(),
	}

	if userData.SeedingLeechingData != nil {
		info.SeederCount = userData.SeedingLeechingData.SeedingCount
		info.Seeding = userData.SeedingLeechingData.SeedingCount
		info.SeederSize = userData.SeedingLeechingData.SeedingSize
	}

	if userData.RegisteredAt != "" {
		if joinTime, err := time.Parse(time.RFC3339, userData.RegisteredAt); err == nil {
			info.JoinDate = joinTime.Unix()
		} else if joinTime, err := time.Parse("2006-01-02T15:04:05-0700", userData.RegisteredAt); err == nil {
			info.JoinDate = joinTime.Unix()
		}
	}

	if userData.LastActiveAt != "" {
		if accessTime, err := time.Parse(time.RFC3339, userData.LastActiveAt); err == nil {
			info.LastAccess = accessTime.Unix()
		} else if accessTime, err := time.Parse("2006-01-02T15:04:05-0700", userData.LastActiveAt); err == nil {
			info.LastAccess = accessTime.Unix()
		}
	}

	return info, nil
}

func (d *rousiDriver) PrepareDownload(torrentID string) (rousiRequest, error) {
	return rousiRequest{
		Endpoint: fmt.Sprintf("/api/torrent/%s/download/%s", torrentID, d.Passkey),
		Method:   http.MethodGet,
	}, nil
}

func (d *rousiDriver) ParseDownload(res rousiResponse) ([]byte, error) {
	if len(res.RawBody) == 0 {
		return nil, fmt.Errorf("empty download response")
	}
	return res.RawBody, nil
}

func (d *rousiDriver) getSiteID() string {
	if d.siteDefinition != nil {
		return d.siteDefinition.ID
	}
	return "rousipro"
}

func extractUUIDFromLink(link string) string {
	if link == "" {
		return ""
	}
	parts := strings.Split(strings.TrimRight(link, "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

func (d *rousiDriver) GetTorrentDetail(ctx context.Context, guid, link, _ string) (*v2.TorrentItem, error) {
	torrentUUID := extractUUIDFromLink(link)
	if torrentUUID == "" {
		torrentUUID = guid
	}
	apiPath := fmt.Sprintf("%s/api/v1/torrents/%s", d.BaseURL, torrentUUID)

	resp, err := d.httpClient.DoRequest(ctx, http.MethodGet, apiPath, nil, map[string]string{
		"Accept":        "application/json",
		"Authorization": "Bearer " + d.Passkey,
		"User-Agent":    d.userAgent,
	})
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: HTTP %d", resp.StatusCode)
	}

	var result struct {
		Code    int          `json:"code"`
		Message string       `json:"message"`
		Data    rousiTorrent `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("API error: %s", result.Message)
	}

	discount, discountEnd := d.parsePromotion(result.Data.Promotion)

	item := &v2.TorrentItem{
		ID:              result.Data.UUID,
		Title:           result.Data.Title,
		Subtitle:        result.Data.Subtitle,
		SizeBytes:       result.Data.Size,
		Seeders:         result.Data.Seeders,
		Leechers:        result.Data.Leechers,
		Snatched:        result.Data.Downloads,
		SourceSite:      d.getSiteID(),
		DiscountLevel:   discount,
		DiscountEndTime: discountEnd,
	}

	if result.Data.CreatedAt != "" {
		if uploadTime, err := time.Parse(time.RFC3339, result.Data.CreatedAt); err == nil {
			item.UploadedAt = uploadTime.Unix()
		}
	}

	return item, nil
}
