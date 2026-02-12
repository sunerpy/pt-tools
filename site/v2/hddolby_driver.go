package v2

import (
	"bytes"
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
)

type HDDolbyRequest struct {
	Endpoint    string
	Method      string
	Body        any
	ContentType string
}

type HDDolbyResponse struct {
	Status     int              `json:"status"`
	Error      *HDDolbyAPIError `json:"error,omitempty"`
	Data       json.RawMessage  `json:"data"`
	RawBody    []byte           `json:"-"`
	StatusCode int              `json:"-"`
}

type HDDolbyAPIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type HDDolbySearchRequest struct {
	Keyword    string `json:"keyword,omitempty"`
	PageNumber int    `json:"page_number"`
	PageSize   int    `json:"page_size"`
	Categories []int  `json:"categories,omitempty"`
	Visible    int    `json:"visible"`
}

type HDDolbySearchResponse struct {
	Data  []HDDolbyTorrent `json:"data"`
	Total int              `json:"total"`
}

type HDDolbyTorrent struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	SmallDescr        string `json:"small_descr"`
	Category          int    `json:"category"`
	Size              int64  `json:"size"`
	Seeders           int    `json:"seeders"`
	Leechers          int    `json:"leechers"`
	TimesCompleted    int    `json:"times_completed"`
	Added             string `json:"added"`
	PromotionTimeType int    `json:"promotion_time_type"`
	PromotionUntil    string `json:"promotion_until"`
	Tags              string `json:"tags"`
	DownHash          string `json:"downhash"`
	HR                int    `json:"hr"`
}

type HDDolbyUserData struct {
	ID             string `json:"id"`
	Username       string `json:"username"`
	Added          string `json:"added"`
	LastAccess     string `json:"last_access"`
	Class          string `json:"class"`
	Uploaded       string `json:"uploaded"`
	Downloaded     string `json:"downloaded"`
	SeedBonus      string `json:"seedbonus"`
	SeBonus        string `json:"sebonus"`
	UnreadMessages string `json:"unread_messages"`
}

type HDDolbyPeerData struct {
	SeedingCount  int   `json:"seeding_count"`
	LeechingCount int   `json:"leeching_count"`
	SeedingSize   int64 `json:"seeding_size"`
	LeechingSize  int64 `json:"leeching_size"`
}

type HDDolbyDriver struct {
	BaseURL        string
	APIURL         string
	APIKey         string
	Cookie         string
	httpClient     *SiteHTTPClient
	userAgent      string
	siteDefinition *SiteDefinition

	detailCacheMu   sync.RWMutex
	detailCache     []HDDolbyTorrent
	detailCacheTime time.Time
	detailCacheMiss int
}

type HDDolbyDriverConfig struct {
	BaseURL    string
	APIURL     string
	APIKey     string
	Cookie     string
	HTTPClient *SiteHTTPClient
	UserAgent  string
}

func NewHDDolbyDriver(config HDDolbyDriverConfig) *HDDolbyDriver {
	// HDDolby API rejects browser User-Agent with "Browser access is blocked!"
	// Use simple client identifier instead
	userAgent := config.UserAgent
	if userAgent == "" {
		userAgent = "pt-tools/1.0"
	}

	baseURL := strings.TrimSuffix(config.BaseURL, "/")
	apiURL := config.APIURL
	if apiURL == "" {
		apiURL = strings.Replace(baseURL, "www.", "api.", 1)
	}
	apiURL = strings.TrimSuffix(apiURL, "/")

	var httpClient *SiteHTTPClient
	if config.HTTPClient != nil {
		httpClient = config.HTTPClient
	} else {
		httpClient = NewSiteHTTPClient(SiteHTTPClientConfig{
			Timeout:   30 * time.Second,
			UserAgent: userAgent,
		})
	}

	return &HDDolbyDriver{
		BaseURL:    baseURL,
		APIURL:     apiURL,
		APIKey:     config.APIKey,
		Cookie:     config.Cookie,
		httpClient: httpClient,
		userAgent:  userAgent,
	}
}

func (d *HDDolbyDriver) SetSiteDefinition(def *SiteDefinition) {
	d.siteDefinition = def
}

func (d *HDDolbyDriver) Execute(ctx context.Context, req HDDolbyRequest) (HDDolbyResponse, error) {
	fullURL := d.APIURL + req.Endpoint

	method := req.Method
	if method == "" {
		method = http.MethodGet
	}

	contentType := req.ContentType
	if contentType == "" {
		contentType = "application/json"
	}

	headers := map[string]string{
		"Accept":       "application/json, text/plain, */*",
		"Content-Type": contentType,
		"x-api-key":    d.APIKey,
		"User-Agent":   d.userAgent,
	}

	var resp *HTTPResponse
	var err error

	if method == http.MethodPost && req.Body != nil {
		bodyBytes, marshalErr := json.Marshal(req.Body)
		if marshalErr != nil {
			return HDDolbyResponse{}, fmt.Errorf("marshal request body: %w", marshalErr)
		}
		resp, err = d.httpClient.DoRequest(ctx, method, fullURL, bytes.NewReader(bodyBytes), headers)
	} else {
		resp, err = d.httpClient.DoRequest(ctx, method, fullURL, nil, headers)
	}

	if err != nil {
		return HDDolbyResponse{}, fmt.Errorf("request failed: %w", err)
	}

	body := resp.Body
	result := HDDolbyResponse{
		RawBody:    body,
		StatusCode: resp.StatusCode,
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyStr := string(body)
		if strings.Contains(bodyStr, "Just a moment") || strings.Contains(bodyStr, "cf_chl_opt") {
			return result, fmt.Errorf("HTTP %d: Cloudflare 验证拦截，请检查网络环境或更换代理", resp.StatusCode)
		}
		if len(body) > 0 {
			_ = json.Unmarshal(body, &result)
		}
		if result.Error != nil {
			return result, fmt.Errorf("API error (HTTP %d): %s - %s", resp.StatusCode, result.Error.Code, result.Error.Message)
		}
		return result, fmt.Errorf("API error: HTTP %d", resp.StatusCode)
	}

	if len(body) > 0 && body[0] == '{' {
		if err := json.Unmarshal(body, &result); err != nil {
			result.Data = body
		}
	} else {
		result.Data = body
	}

	if result.Error != nil {
		return result, fmt.Errorf("API error: %s - %s", result.Error.Code, result.Error.Message)
	}

	return result, nil
}

func (d *HDDolbyDriver) PrepareSearch(query SearchQuery) (HDDolbyRequest, error) {
	searchReq := HDDolbySearchRequest{
		Keyword:    query.Keyword,
		PageNumber: 1,
		PageSize:   100,
		Visible:    1,
	}

	return HDDolbyRequest{
		Endpoint:    "/api/v1/torrent/search",
		Method:      http.MethodPost,
		Body:        searchReq,
		ContentType: "application/json",
	}, nil
}

func (d *HDDolbyDriver) ParseSearch(res HDDolbyResponse) ([]TorrentItem, error) {
	var searchData HDDolbySearchResponse
	if err := json.Unmarshal(res.Data, &searchData); err != nil {
		var torrents []HDDolbyTorrent
		if err2 := json.Unmarshal(res.Data, &torrents); err2 != nil {
			return nil, fmt.Errorf("parse search data: %w (data: %s)", err, string(res.Data))
		}
		searchData.Data = torrents
	}

	items := make([]TorrentItem, 0, len(searchData.Data))
	for _, t := range searchData.Data {
		discount := d.parseDiscount(t.PromotionTimeType, t.Tags)
		discountEnd := d.parseDiscountEndTime(t.PromotionUntil)

		siteID := d.getSiteID()
		// Include downhash in download URL - required by HDDolby to avoid Cloudflare challenge
		item := TorrentItem{
			ID:            strconv.Itoa(t.ID),
			Title:         t.Name,
			Subtitle:      t.SmallDescr,
			SizeBytes:     t.Size,
			Seeders:       t.Seeders,
			Leechers:      t.Leechers,
			Snatched:      t.TimesCompleted,
			DiscountLevel: discount,
			Category:      d.getCategoryName(t.Category),
			URL:           fmt.Sprintf("%s/details.php?id=%d", d.BaseURL, t.ID),
			DownloadURL:   fmt.Sprintf("/api/site/%s/torrent/%d/download?downhash=%s", siteID, t.ID, t.DownHash),
			SourceSite:    siteID,
		}

		if t.HR == 1 {
			item.HasHR = true
		}

		if !discountEnd.IsZero() {
			item.DiscountEndTime = discountEnd
		}

		if t.Added != "" {
			if uploadTime, err := ParseTimeInCST("2006-01-02 15:04:05", t.Added); err == nil {
				item.UploadedAt = uploadTime.Unix()
			}
		}

		items = append(items, item)
	}

	return items, nil
}

func (d *HDDolbyDriver) parseDiscount(promotionType int, tags string) DiscountLevel {
	if strings.Contains(tags, "gf") {
		return Discount2xFree
	}
	if strings.Contains(tags, "f") {
		return DiscountFree
	}
	if strings.Contains(tags, "g") {
		return Discount2xUp
	}

	switch promotionType {
	case 1:
		return DiscountFree
	case 2:
		return Discount2xUp
	case 3:
		return Discount2xFree
	case 4:
		return DiscountPercent50
	case 5:
		return Discount2x50
	case 6:
		return DiscountPercent30
	default:
		return DiscountNone
	}
}

func (d *HDDolbyDriver) parseDiscountEndTime(until string) time.Time {
	if until == "" || until == "0000-00-00 00:00:00" {
		return time.Time{}
	}
	if t, err := ParseTimeInCST("2006-01-02 15:04:05", until); err == nil {
		return t
	}
	return time.Time{}
}

func (d *HDDolbyDriver) getCategoryName(catID int) string {
	categories := map[int]string{
		401: "Movies/SD",
		402: "Movies/HD",
		403: "Movies/UHD",
		404: "Movies/BluRay",
		405: "Movies/Remux",
		406: "Movies/3D",
		407: "TV/SD",
		408: "TV/HD",
		409: "TV/UHD",
		410: "TV/BluRay",
		411: "Documentary",
		412: "Animation",
		413: "Music",
		414: "Sports",
		415: "Games",
		416: "Software",
		417: "Education",
		418: "Other",
	}
	if name, ok := categories[catID]; ok {
		return name
	}
	return strconv.Itoa(catID)
}

func (d *HDDolbyDriver) PrepareUserInfo() (HDDolbyRequest, error) {
	return HDDolbyRequest{
		Endpoint: "/api/v1/user/data",
		Method:   http.MethodGet,
	}, nil
}

func (d *HDDolbyDriver) ParseUserInfo(res HDDolbyResponse) (UserInfo, error) {
	var userDataList []HDDolbyUserData
	if err := json.Unmarshal(res.Data, &userDataList); err != nil {
		return UserInfo{}, fmt.Errorf("parse user data: %w (data: %s)", err, string(res.Data))
	}

	if len(userDataList) == 0 {
		return UserInfo{}, fmt.Errorf("empty user data response")
	}

	userData := userDataList[0]

	uploaded, _ := strconv.ParseInt(userData.Uploaded, 10, 64)
	downloaded, _ := strconv.ParseInt(userData.Downloaded, 10, 64)
	seedBonus, _ := strconv.ParseFloat(userData.SeedBonus, 64)
	seBonus, _ := strconv.ParseFloat(userData.SeBonus, 64)
	unreadMessages, _ := strconv.Atoi(userData.UnreadMessages)

	var ratio float64
	if downloaded > 0 {
		ratio = float64(uploaded) / float64(downloaded)
	}

	levelName := d.getLevelName(userData.Class)

	info := UserInfo{
		Site:               d.getSiteID(),
		UserID:             userData.ID,
		Username:           userData.Username,
		Uploaded:           uploaded,
		Downloaded:         downloaded,
		Ratio:              ratio,
		Bonus:              seedBonus,
		SeedingBonus:       seBonus,
		LevelName:          levelName,
		Rank:               levelName,
		UnreadMessageCount: unreadMessages,
		LastUpdate:         time.Now().Unix(),
	}

	if userData.Added != "" {
		if joinTime, err := ParseTimeInCST("2006-01-02 15:04:05", userData.Added); err == nil {
			info.JoinDate = joinTime.Unix()
		}
	}

	if userData.LastAccess != "" {
		if accessTime, err := ParseTimeInCST("2006-01-02 15:04:05", userData.LastAccess); err == nil {
			info.LastAccess = accessTime.Unix()
		}
	}

	return info, nil
}

func (d *HDDolbyDriver) getLevelName(classID string) string {
	if d.siteDefinition == nil {
		return classID
	}

	id, err := strconv.Atoi(classID)
	if err != nil {
		return classID
	}

	for _, level := range d.siteDefinition.LevelRequirements {
		if level.ID == id {
			return level.Name
		}
	}
	return classID
}

func (d *HDDolbyDriver) getSiteID() string {
	if d.siteDefinition != nil {
		return d.siteDefinition.ID
	}
	return "hddolby"
}

func (d *HDDolbyDriver) GetUserInfo(ctx context.Context) (UserInfo, error) {
	startTime := time.Now()

	var info UserInfo
	var peerData *HDDolbyPeerData
	var bonusPerHour float64
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		req, err := d.PrepareUserInfo()
		if err != nil {
			return fmt.Errorf("prepare user info: %w", err)
		}

		res, err := d.Execute(gctx, req)
		if err != nil {
			return fmt.Errorf("execute user info: %w", err)
		}

		parsedInfo, err := d.ParseUserInfo(res)
		if err != nil {
			return fmt.Errorf("parse user info: %w", err)
		}

		mu.Lock()
		info = parsedInfo
		mu.Unlock()
		return nil
	})

	g.Go(func() error {
		peers, err := d.GetPeerData(gctx)
		if err == nil && peers != nil {
			mu.Lock()
			peerData = peers
			mu.Unlock()
		}
		return nil
	})

	g.Go(func() error {
		bph, err := d.GetBonusPerHour(gctx)
		if err == nil && bph > 0 {
			mu.Lock()
			bonusPerHour = bph
			mu.Unlock()
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return UserInfo{}, err
	}

	if peerData != nil {
		if info.SeederCount == 0 {
			info.SeederCount = peerData.SeedingCount
			info.Seeding = peerData.SeedingCount
		}
		if info.SeederSize == 0 {
			info.SeederSize = peerData.SeedingSize
		}
		if info.LeecherCount == 0 {
			info.LeecherCount = peerData.LeechingCount
			info.Leeching = peerData.LeechingCount
		}
		if info.LeecherSize == 0 {
			info.LeecherSize = peerData.LeechingSize
		}
	}

	if bonusPerHour > 0 {
		info.BonusPerHour = bonusPerHour
	}

	if DebugUserInfo {
		fmt.Printf("[DEBUG] HDDolby GetUserInfo completed in %v\n", time.Since(startTime))
	}

	return info, nil
}

type HDDolbyPeerItem struct {
	ID       int   `json:"id"`
	Size     int64 `json:"size"`
	Seeders  int   `json:"seeders"`
	Leechers int   `json:"leechers"`
}

func (d *HDDolbyDriver) GetPeerData(ctx context.Context) (*HDDolbyPeerData, error) {
	req := HDDolbyRequest{
		Endpoint: "/api/v1/user/peers",
		Method:   http.MethodGet,
	}

	res, err := d.Execute(ctx, req)
	if err != nil {
		return nil, err
	}

	var peerItems []HDDolbyPeerItem
	if err := json.Unmarshal(res.Data, &peerItems); err != nil {
		return nil, fmt.Errorf("parse peer data: %w", err)
	}

	peerData := &HDDolbyPeerData{
		SeedingCount: len(peerItems),
	}
	for _, item := range peerItems {
		peerData.SeedingSize += item.Size
	}

	return peerData, nil
}

// GetBonusPerHour fetches bonus per hour from /mybonus.php using cookie
func (d *HDDolbyDriver) GetBonusPerHour(ctx context.Context) (float64, error) {
	if d.Cookie == "" {
		return 0, nil
	}

	bonusURL := d.BaseURL + "/mybonus.php"
	headers := map[string]string{
		"Cookie":     d.Cookie,
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Accept":     "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
	}

	resp, err := d.httpClient.DoRequest(ctx, http.MethodGet, bonusURL, nil, headers)
	if err != nil {
		return 0, fmt.Errorf("fetch mybonus.php: %w", err)
	}

	html := string(resp.Body)

	// HTML structure: <td>合计</td><td colspan="5">-</td><td>460</td><td>25.98 / 25.98</td>
	re := regexp.MustCompile(`(?s)<td>合计</td>.*?<td[^>]*>(\d+(?:\.\d+)?)\s*/\s*\d+(?:\.\d+)?</td>`)
	matches := re.FindStringSubmatch(html)
	if len(matches) >= 2 {
		bonus, err := strconv.ParseFloat(matches[1], 64)
		if err == nil {
			return bonus, nil
		}
	}

	return 0, nil
}

func (d *HDDolbyDriver) PrepareDownload(torrentID string) (HDDolbyRequest, error) {
	return HDDolbyRequest{
		Endpoint: fmt.Sprintf("/download.php?id=%s", torrentID),
		Method:   http.MethodGet,
	}, nil
}

func (d *HDDolbyDriver) ParseDownload(res HDDolbyResponse) ([]byte, error) {
	if len(res.RawBody) == 0 {
		return nil, fmt.Errorf("empty download response")
	}
	return res.RawBody, nil
}

func (d *HDDolbyDriver) Download(ctx context.Context, torrentID string) ([]byte, error) {
	return d.DownloadWithHash(ctx, torrentID, "")
}

func (d *HDDolbyDriver) DownloadWithHash(ctx context.Context, torrentID, downhash string) ([]byte, error) {
	var downloadURL string
	if downhash != "" {
		downloadURL = fmt.Sprintf("%s/download.php?id=%s&downhash=%s", d.BaseURL, torrentID, downhash)
	} else {
		downloadURL = fmt.Sprintf("%s/download.php?id=%s", d.APIURL, torrentID)
	}

	headers := map[string]string{
		"Accept":    "*/*",
		"x-api-key": d.APIKey,
	}

	resp, err := d.httpClient.DoRequest(ctx, http.MethodGet, downloadURL, nil, headers)
	if err != nil {
		return nil, fmt.Errorf("download request failed: %w", err)
	}

	body := resp.Body
	if len(body) == 0 {
		return nil, fmt.Errorf("empty download response")
	}

	return body, nil
}

func (d *HDDolbyDriver) Search(ctx context.Context, query SearchQuery) ([]TorrentItem, error) {
	req, err := d.PrepareSearch(query)
	if err != nil {
		return nil, err
	}

	res, err := d.Execute(ctx, req)
	if err != nil {
		return nil, err
	}

	return d.ParseSearch(res)
}

// GetTorrentDetail fetches torrent detail from the detail page using cookie authentication.
// HDDolby uses NexusPHP-style HTML detail pages, so we use the NexusPHP parser.
func (d *HDDolbyDriver) GetTorrentDetail(ctx context.Context, guid, link, title string) (*TorrentItem, error) {
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

	numericID, err := strconv.Atoi(torrentID)
	if err != nil {
		return nil, fmt.Errorf("invalid torrent ID %q: %w", torrentID, err)
	}

	torrents, err := d.getOrRefreshDetailCache(ctx)
	if err != nil {
		return nil, err
	}

	for _, t := range torrents {
		if t.ID == numericID {
			d.detailCacheMu.Lock()
			d.detailCacheMiss = 0
			d.detailCacheMu.Unlock()
			return d.torrentToItem(t), nil
		}
	}

	d.detailCacheMu.Lock()
	d.detailCacheMiss++
	missCount := d.detailCacheMiss
	d.detailCacheMu.Unlock()

	if missCount >= 3 {
		d.invalidateDetailCache()
		torrents, err = d.getOrRefreshDetailCache(ctx)
		if err == nil {
			for _, t := range torrents {
				if t.ID == numericID {
					d.detailCacheMu.Lock()
					d.detailCacheMiss = 0
					d.detailCacheMu.Unlock()
					return d.torrentToItem(t), nil
				}
			}
		}
	}

	return &TorrentItem{
		ID:            torrentID,
		DiscountLevel: DiscountNone,
		SourceSite:    d.getSiteID(),
	}, nil
}

func (d *HDDolbyDriver) torrentToItem(t HDDolbyTorrent) *TorrentItem {
	discount := d.parseDiscount(t.PromotionTimeType, t.Tags)
	discountEnd := d.parseDiscountEndTime(t.PromotionUntil)
	return &TorrentItem{
		ID:              strconv.Itoa(t.ID),
		Title:           t.Name,
		Subtitle:        t.SmallDescr,
		SizeBytes:       t.Size,
		Seeders:         t.Seeders,
		Leechers:        t.Leechers,
		Snatched:        t.TimesCompleted,
		DiscountLevel:   discount,
		DiscountEndTime: discountEnd,
		HasHR:           t.HR > 0,
		SourceSite:      d.getSiteID(),
	}
}

func (d *HDDolbyDriver) getOrRefreshDetailCache(ctx context.Context) ([]HDDolbyTorrent, error) {
	d.detailCacheMu.RLock()
	if d.detailCache != nil && time.Since(d.detailCacheTime) < 5*time.Minute {
		defer d.detailCacheMu.RUnlock()
		return d.detailCache, nil
	}
	d.detailCacheMu.RUnlock()

	d.detailCacheMu.Lock()
	defer d.detailCacheMu.Unlock()

	if d.detailCache != nil && time.Since(d.detailCacheTime) < 5*time.Minute {
		return d.detailCache, nil
	}

	req := HDDolbyRequest{
		Endpoint:    "/api/v1/torrent/search",
		Method:      http.MethodPost,
		ContentType: "application/json",
		Body: map[string]any{
			"keyword":     "",
			"page_number": 0,
			"page_size":   100,
			"visible":     1,
		},
	}

	res, err := d.Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("execute search API: %w", err)
	}

	var searchData HDDolbySearchResponse
	if err := json.Unmarshal(res.Data, &searchData); err != nil {
		var torrents []HDDolbyTorrent
		if err2 := json.Unmarshal(res.Data, &torrents); err2 != nil {
			return nil, fmt.Errorf("parse search data: %w", err)
		}
		searchData.Data = torrents
	}

	d.detailCache = searchData.Data
	d.detailCacheTime = time.Now()
	d.detailCacheMiss = 0
	return d.detailCache, nil
}

func (d *HDDolbyDriver) invalidateDetailCache() {
	d.detailCacheMu.Lock()
	defer d.detailCacheMu.Unlock()
	d.detailCache = nil
	d.detailCacheTime = time.Time{}
	d.detailCacheMiss = 0
}

func init() {
	RegisterDriverForSchema("HDDolby", createHDDolbySite)
}

func createHDDolbySite(config SiteConfig, logger *zap.Logger) (Site, error) {
	var opts HDDolbyOptions
	if len(config.Options) > 0 {
		if err := json.Unmarshal(config.Options, &opts); err != nil {
			return nil, fmt.Errorf("parse HDDolby options: %w", err)
		}
	}

	if opts.APIKey == "" {
		return nil, fmt.Errorf("HDDolby 站点需要配置 RSS Key（从站点 RSS 订阅页面获取）")
	}

	if opts.Cookie == "" {
		return nil, fmt.Errorf("HDDolby 站点需要配置 Cookie（用于获取时魔等信息）")
	}

	siteDef := GetDefinitionRegistry().GetOrDefault(config.ID)

	driver := NewHDDolbyDriver(HDDolbyDriverConfig{
		BaseURL: config.BaseURL,
		APIKey:  opts.APIKey,
		Cookie:  opts.Cookie,
	})

	if siteDef != nil {
		driver.SetSiteDefinition(siteDef)
	}

	return NewBaseSite(driver, BaseSiteConfig{
		ID:        config.ID,
		Name:      config.Name,
		Kind:      SiteHDDolby,
		RateLimit: config.RateLimit,
		RateBurst: config.RateBurst,
		Logger:    logger.With(zap.String("site", config.ID)),
	}), nil
}
