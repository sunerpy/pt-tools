package v2

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Unit3DRequest represents a request to a Unit3D site
type Unit3DRequest struct {
	// Endpoint is the API endpoint path
	Endpoint string
	// Method is the HTTP method
	Method string
	// Params are the query parameters
	Params url.Values
}

// Unit3DResponse represents a response from Unit3D API
type Unit3DResponse struct {
	// Data is the response data
	Data json.RawMessage `json:"data"`
	// Links contains pagination links
	Links json.RawMessage `json:"links,omitempty"`
	// Meta contains pagination metadata
	Meta json.RawMessage `json:"meta,omitempty"`
	// RawBody is the raw response body
	RawBody []byte `json:"-"`
	// StatusCode is the HTTP status code
	StatusCode int `json:"-"`
}

// Unit3DTorrent represents a torrent in Unit3D API response
type Unit3DTorrent struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	InfoHash       string `json:"info_hash"`
	Size           int64  `json:"size"`
	Seeders        int    `json:"seeders"`
	Leechers       int    `json:"leechers"`
	TimesCompleted int    `json:"times_completed"`
	Category       struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"category"`
	Type struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"type"`
	Freeleech     string `json:"freeleech"`
	FreeleechEnds string `json:"freeleech_ends,omitempty"`
	DoubleUpload  bool   `json:"double_upload"`
	Featured      bool   `json:"featured"`
	CreatedAt     string `json:"created_at"`
	DownloadLink  string `json:"download_link,omitempty"`
}

// Unit3DUserProfile represents user profile from Unit3D API
type Unit3DUserProfile struct {
	ID         int     `json:"id"`
	Username   string  `json:"username"`
	Uploaded   int64   `json:"uploaded"`
	Downloaded int64   `json:"downloaded"`
	Ratio      float64 `json:"ratio"`
	Buffer     int64   `json:"buffer"`
	Seedbonus  float64 `json:"seedbonus"`
	Seeding    int     `json:"seeding"`
	Leeching   int     `json:"leeching"`
	Group      struct {
		Name string `json:"name"`
	} `json:"group"`
	CreatedAt string `json:"created_at"`
}

// Unit3DDriver implements the Driver interface for Unit3D sites
type Unit3DDriver struct {
	BaseURL    string
	APIKey     string
	httpClient *SiteHTTPClient
	userAgent  string
}

// Unit3DDriverConfig holds configuration for creating a Unit3D driver
type Unit3DDriverConfig struct {
	BaseURL    string
	APIKey     string
	HTTPClient *SiteHTTPClient // Use SiteHTTPClient instead of *http.Client
	UserAgent  string
}

// NewUnit3DDriver creates a new Unit3D driver
func NewUnit3DDriver(config Unit3DDriverConfig) *Unit3DDriver {
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

	return &Unit3DDriver{
		BaseURL:    strings.TrimSuffix(config.BaseURL, "/"),
		APIKey:     config.APIKey,
		httpClient: httpClient,
		userAgent:  userAgent,
	}
}

// PrepareSearch converts a SearchQuery to a Unit3D request
func (d *Unit3DDriver) PrepareSearch(query SearchQuery) (Unit3DRequest, error) {
	params := url.Values{}

	if query.Keyword != "" {
		params.Set("name", query.Keyword)
	}
	if query.Category != "" {
		params.Set("categories[]", query.Category)
	}
	if query.FreeOnly {
		params.Set("freeleech", "1")
	}
	if query.Page > 0 {
		params.Set("page", strconv.Itoa(query.Page))
	}
	if query.PageSize > 0 {
		params.Set("perPage", strconv.Itoa(query.PageSize))
	}

	return Unit3DRequest{
		Endpoint: "/api/torrents/filter",
		Method:   "GET",
		Params:   params,
	}, nil
}

// Execute performs the HTTP request
func (d *Unit3DDriver) Execute(ctx context.Context, req Unit3DRequest) (Unit3DResponse, error) {
	fullURL := d.BaseURL + req.Endpoint
	if len(req.Params) > 0 {
		fullURL += "?" + req.Params.Encode()
	}

	headers := map[string]string{
		"Accept":        "application/json",
		"User-Agent":    d.userAgent,
		"Authorization": "Bearer " + d.APIKey,
	}

	resp, err := d.httpClient.Get(ctx, fullURL, headers)
	if err != nil {
		return Unit3DResponse{}, fmt.Errorf("execute request: %w", err)
	}

	result := Unit3DResponse{
		RawBody:    resp.Body,
		StatusCode: resp.StatusCode,
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return result, ErrInvalidCredentials
	}

	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return result, fmt.Errorf("parse JSON: %w", err)
	}

	return result, nil
}

// ParseSearch extracts torrent items from the response
func (d *Unit3DDriver) ParseSearch(res Unit3DResponse) ([]TorrentItem, error) {
	var torrents []Unit3DTorrent
	if err := json.Unmarshal(res.Data, &torrents); err != nil {
		return nil, fmt.Errorf("parse torrents: %w", err)
	}

	items := make([]TorrentItem, 0, len(torrents))
	for _, t := range torrents {
		item := TorrentItem{
			ID:            strconv.Itoa(t.ID),
			Title:         t.Name,
			InfoHash:      t.InfoHash,
			SizeBytes:     t.Size,
			Seeders:       t.Seeders,
			Leechers:      t.Leechers,
			Snatched:      t.TimesCompleted,
			SourceSite:    d.BaseURL,
			Category:      t.Category.Name,
			DiscountLevel: parseUnit3DDiscount(t.Freeleech, t.DoubleUpload),
			DownloadURL:   t.DownloadLink,
		}

		// Parse upload time
		if t.CreatedAt != "" {
			if uploadTime, err := time.Parse(time.RFC3339, t.CreatedAt); err == nil {
				item.UploadedAt = uploadTime.Unix()
			}
		}

		// Parse freeleech end time
		if t.FreeleechEnds != "" {
			if endTime, err := time.Parse(time.RFC3339, t.FreeleechEnds); err == nil {
				item.DiscountEndTime = endTime
			}
		}

		items = append(items, item)
	}

	return items, nil
}

// PrepareUserInfo prepares a request for user info
func (d *Unit3DDriver) PrepareUserInfo() (Unit3DRequest, error) {
	return Unit3DRequest{
		Endpoint: "/api/user",
		Method:   "GET",
	}, nil
}

// ParseUserInfo extracts user info from the response
func (d *Unit3DDriver) ParseUserInfo(res Unit3DResponse) (UserInfo, error) {
	var profile Unit3DUserProfile
	if err := json.Unmarshal(res.Data, &profile); err != nil {
		return UserInfo{}, fmt.Errorf("parse user profile: %w", err)
	}

	info := UserInfo{
		UserID:     strconv.Itoa(profile.ID),
		Username:   profile.Username,
		Uploaded:   profile.Uploaded,
		Downloaded: profile.Downloaded,
		Ratio:      profile.Ratio,
		Bonus:      profile.Seedbonus,
		Seeding:    profile.Seeding,
		Leeching:   profile.Leeching,
		Rank:       profile.Group.Name,
		LastUpdate: time.Now().Unix(),
	}

	// Parse join date
	if profile.CreatedAt != "" {
		if joinTime, err := time.Parse(time.RFC3339, profile.CreatedAt); err == nil {
			info.JoinDate = joinTime.Unix()
		}
	}

	return info, nil
}

// GetUserInfo fetches complete user information
func (d *Unit3DDriver) GetUserInfo(ctx context.Context) (UserInfo, error) {
	req, err := d.PrepareUserInfo()
	if err != nil {
		return UserInfo{}, err
	}

	res, err := d.Execute(ctx, req)
	if err != nil {
		return UserInfo{}, err
	}

	return d.ParseUserInfo(res)
}

// PrepareDownload prepares a request for downloading a torrent
func (d *Unit3DDriver) PrepareDownload(torrentID string) (Unit3DRequest, error) {
	return Unit3DRequest{
		Endpoint: fmt.Sprintf("/api/torrents/%s/download", torrentID),
		Method:   "GET",
	}, nil
}

// ParseDownload extracts torrent file data from the response
func (d *Unit3DDriver) ParseDownload(res Unit3DResponse) ([]byte, error) {
	if len(res.RawBody) == 0 {
		return nil, ErrParseError
	}
	return res.RawBody, nil
}

// parseUnit3DDiscount parses Unit3D freeleech status to DiscountLevel
func parseUnit3DDiscount(freeleech string, doubleUpload bool) DiscountLevel {
	freeleech = strings.ToLower(strings.TrimSpace(freeleech))

	if freeleech == "100" || freeleech == "1" || freeleech == "true" {
		if doubleUpload {
			return Discount2xFree
		}
		return DiscountFree
	}

	if freeleech == "50" {
		if doubleUpload {
			return Discount2x50
		}
		return DiscountPercent50
	}

	if freeleech == "25" || freeleech == "30" {
		return DiscountPercent30
	}

	if freeleech == "75" || freeleech == "70" {
		return DiscountPercent70
	}

	if doubleUpload {
		return Discount2xUp
	}

	return DiscountNone
}
