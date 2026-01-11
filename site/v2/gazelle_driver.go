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

// GazelleRequest represents a request to a Gazelle site
type GazelleRequest struct {
	// Action is the API action
	Action string
	// Params are the query parameters
	Params url.Values
}

// GazelleResponse represents a response from Gazelle API
type GazelleResponse struct {
	// Status is the response status ("success" or "failure")
	Status string `json:"status"`
	// Error is the error message (if status is "failure")
	Error string `json:"error,omitempty"`
	// Response is the response data
	Response json.RawMessage `json:"response"`
	// RawBody is the raw response body
	RawBody []byte `json:"-"`
	// StatusCode is the HTTP status code
	StatusCode int `json:"-"`
}

// GazelleSearchResponse represents search results from Gazelle API
type GazelleSearchResponse struct {
	CurrentPage int                   `json:"currentPage"`
	Pages       int                   `json:"pages"`
	Results     []GazelleTorrentGroup `json:"results"`
}

// GazelleTorrentGroup represents a torrent group in Gazelle
type GazelleTorrentGroup struct {
	GroupID   int              `json:"groupId"`
	GroupName string           `json:"groupName"`
	Artist    string           `json:"artist,omitempty"`
	Tags      []string         `json:"tags"`
	Torrents  []GazelleTorrent `json:"torrents"`
}

// GazelleTorrent represents a torrent in Gazelle
type GazelleTorrent struct {
	TorrentID int `json:"torrentId"`
	EditionID int `json:"editionId,omitempty"`
	Artists   []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"artists,omitempty"`
	Remastered     bool   `json:"remastered"`
	RemasterYear   int    `json:"remasterYear,omitempty"`
	RemasterTitle  string `json:"remasterTitle,omitempty"`
	Media          string `json:"media"`
	Encoding       string `json:"encoding"`
	Format         string `json:"format"`
	HasLog         bool   `json:"hasLog"`
	LogScore       int    `json:"logScore"`
	HasCue         bool   `json:"hasCue"`
	Scene          bool   `json:"scene"`
	VanityHouse    bool   `json:"vanityHouse"`
	FileCount      int    `json:"fileCount"`
	Time           string `json:"time"`
	Size           int64  `json:"size"`
	Snatches       int    `json:"snatches"`
	Seeders        int    `json:"seeders"`
	Leechers       int    `json:"leechers"`
	IsFreeleech    bool   `json:"isFreeleech"`
	IsNeutralLeech bool   `json:"isNeutralLeech"`
	IsPersonalFL   bool   `json:"isPersonalFreeleech"`
	CanUseToken    bool   `json:"canUseToken"`
}

// GazelleUserResponse represents user info from Gazelle API
type GazelleUserResponse struct {
	Username string `json:"username"`
	ID       int    `json:"id"`
	Stats    struct {
		Uploaded   int64   `json:"uploaded"`
		Downloaded int64   `json:"downloaded"`
		Ratio      float64 `json:"ratio"`
		Buffer     int64   `json:"buffer"`
	} `json:"stats"`
	Ranks struct {
		Class string `json:"class"`
	} `json:"ranks"`
	Personal struct {
		Bonus float64 `json:"bonus"`
	} `json:"personal"`
	Community struct {
		Seeding  int `json:"seeding"`
		Leeching int `json:"leeching"`
	} `json:"community"`
}

// GazelleDriver implements the Driver interface for Gazelle sites
type GazelleDriver struct {
	BaseURL    string
	APIKey     string
	Cookie     string
	httpClient *SiteHTTPClient
	userAgent  string
}

// GazelleDriverConfig holds configuration for creating a Gazelle driver
type GazelleDriverConfig struct {
	BaseURL    string
	APIKey     string
	Cookie     string
	HTTPClient *SiteHTTPClient // Use SiteHTTPClient instead of *http.Client
	UserAgent  string
}

// NewGazelleDriver creates a new Gazelle driver
func NewGazelleDriver(config GazelleDriverConfig) *GazelleDriver {
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

	return &GazelleDriver{
		BaseURL:    strings.TrimSuffix(config.BaseURL, "/"),
		APIKey:     config.APIKey,
		Cookie:     config.Cookie,
		httpClient: httpClient,
		userAgent:  userAgent,
	}
}

// PrepareSearch converts a SearchQuery to a Gazelle request
func (d *GazelleDriver) PrepareSearch(query SearchQuery) (GazelleRequest, error) {
	params := url.Values{}

	if query.Keyword != "" {
		params.Set("searchstr", query.Keyword)
	}
	if query.Category != "" {
		params.Set("filter_cat["+query.Category+"]", "1")
	}
	if query.FreeOnly {
		params.Set("freetorrent", "1")
	}
	if query.Page > 0 {
		params.Set("page", strconv.Itoa(query.Page))
	}

	return GazelleRequest{
		Action: "browse",
		Params: params,
	}, nil
}

// Execute performs the HTTP request
func (d *GazelleDriver) Execute(ctx context.Context, req GazelleRequest) (GazelleResponse, error) {
	params := req.Params
	if params == nil {
		params = url.Values{}
	}
	params.Set("action", req.Action)

	fullURL := d.BaseURL + "/ajax.php?" + params.Encode()

	headers := map[string]string{
		"Accept":     "application/json",
		"User-Agent": d.userAgent,
	}

	// Use API key or cookie for authentication
	if d.APIKey != "" {
		headers["Authorization"] = d.APIKey
	}
	if d.Cookie != "" {
		headers["Cookie"] = d.Cookie
	}

	resp, err := d.httpClient.Get(ctx, fullURL, headers)
	if err != nil {
		return GazelleResponse{}, fmt.Errorf("execute request: %w", err)
	}

	result := GazelleResponse{
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

	if result.Status == "failure" {
		return result, fmt.Errorf("API error: %s", result.Error)
	}

	return result, nil
}

// ParseSearch extracts torrent items from the response
func (d *GazelleDriver) ParseSearch(res GazelleResponse) ([]TorrentItem, error) {
	var searchResp GazelleSearchResponse
	if err := json.Unmarshal(res.Response, &searchResp); err != nil {
		return nil, fmt.Errorf("parse search response: %w", err)
	}

	var items []TorrentItem
	for _, group := range searchResp.Results {
		for _, t := range group.Torrents {
			// Build title from group and torrent info
			title := group.GroupName
			if group.Artist != "" {
				title = group.Artist + " - " + title
			}
			if t.Format != "" {
				title += " [" + t.Format
				if t.Encoding != "" {
					title += " " + t.Encoding
				}
				title += "]"
			}

			item := TorrentItem{
				ID:            strconv.Itoa(t.TorrentID),
				Title:         title,
				SizeBytes:     t.Size,
				Seeders:       t.Seeders,
				Leechers:      t.Leechers,
				Snatched:      t.Snatches,
				SourceSite:    d.BaseURL,
				Tags:          group.Tags,
				DiscountLevel: parseGazelleDiscount(t.IsFreeleech, t.IsNeutralLeech, t.IsPersonalFL),
			}

			// Parse upload time
			if t.Time != "" {
				if uploadTime, err := time.Parse("2006-01-02 15:04:05", t.Time); err == nil {
					item.UploadedAt = uploadTime.Unix()
				}
			}

			// Build download URL
			item.DownloadURL = fmt.Sprintf("%s/torrents.php?action=download&id=%d", d.BaseURL, t.TorrentID)

			items = append(items, item)
		}
	}

	return items, nil
}

// PrepareUserInfo prepares a request for user info
func (d *GazelleDriver) PrepareUserInfo() (GazelleRequest, error) {
	return GazelleRequest{
		Action: "index",
	}, nil
}

// ParseUserInfo extracts user info from the response
func (d *GazelleDriver) ParseUserInfo(res GazelleResponse) (UserInfo, error) {
	var userResp GazelleUserResponse
	if err := json.Unmarshal(res.Response, &userResp); err != nil {
		return UserInfo{}, fmt.Errorf("parse user response: %w", err)
	}

	info := UserInfo{
		UserID:     strconv.Itoa(userResp.ID),
		Username:   userResp.Username,
		Uploaded:   userResp.Stats.Uploaded,
		Downloaded: userResp.Stats.Downloaded,
		Ratio:      userResp.Stats.Ratio,
		Bonus:      userResp.Personal.Bonus,
		Seeding:    userResp.Community.Seeding,
		Leeching:   userResp.Community.Leeching,
		Rank:       userResp.Ranks.Class,
		LastUpdate: time.Now().Unix(),
	}

	return info, nil
}

// GetUserInfo fetches complete user information
func (d *GazelleDriver) GetUserInfo(ctx context.Context) (UserInfo, error) {
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
func (d *GazelleDriver) PrepareDownload(torrentID string) (GazelleRequest, error) {
	params := url.Values{}
	params.Set("id", torrentID)

	return GazelleRequest{
		Action: "download",
		Params: params,
	}, nil
}

// ParseDownload extracts torrent file data from the response
func (d *GazelleDriver) ParseDownload(res GazelleResponse) ([]byte, error) {
	if len(res.RawBody) == 0 {
		return nil, ErrParseError
	}
	return res.RawBody, nil
}

// parseGazelleDiscount parses Gazelle freeleech status to DiscountLevel
func parseGazelleDiscount(isFreeleech, isNeutralLeech, isPersonalFL bool) DiscountLevel {
	if isFreeleech || isPersonalFL {
		return DiscountFree
	}
	if isNeutralLeech {
		return DiscountFree // Neutral leech is effectively free
	}
	return DiscountNone
}
