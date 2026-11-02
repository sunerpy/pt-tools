package v2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
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
	Details string `json:"details"`
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

// HDDolbyTorrent 是 API 搜索结果里的一个种子。sp_state 是优惠（NexusPHP 的促销状态），promotion_time_type 是优惠的时限：
// 0 跟站点默认、1 永久、2 到 promotion_until 为止（见 applyDiscount）。tags 是分号分隔的标签代码（gf 官方、zz 中字……），不是优惠。
type HDDolbyTorrent struct {
	ID                int    `json:"id"`
	SpState           int    `json:"sp_state"`
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
	InfoHash          string `json:"info_hash"`
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
	BaseURL    string
	APIURL     string
	APIKey     string
	Cookie     string
	httpClient *SiteHTTPClient
	// pageClient 用浏览器的 User-Agent 打开网页（时魔、签到、用 Cookie 下载）；客户端会先写上自己的 User-Agent，请求头里另给的不起作用
	pageClient     *SiteHTTPClient
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
	// SiteID 是站点在 pt-tools 里注册的 ID：全局设置里的「站点代理」按它判定走不走代理
	SiteID string
}

func NewHDDolbyDriver(config HDDolbyDriverConfig) *HDDolbyDriver {
	// HDDolby API rejects browser User-Agent with "Browser access is blocked!"
	// Use simple client identifier instead
	userAgent := config.UserAgent
	if userAgent == "" {
		userAgent = "pt-tools/1.0"
	}

	baseURL := strings.TrimSuffix(config.BaseURL, "/")
	// 换到 www.uhddolby.com 以后 API 和网页在同一个主机上（www.uhddolby.com/api/v1/…），以前在 api. 子域名
	apiURL := config.APIURL
	if apiURL == "" {
		apiURL = baseURL
	}
	apiURL = strings.TrimSuffix(apiURL, "/")

	var httpClient, pageClient *SiteHTTPClient
	if config.HTTPClient != nil {
		httpClient = config.HTTPClient
		pageClient = config.HTTPClient
	} else {
		httpClient = NewSiteHTTPClient(SiteHTTPClientConfig{
			Timeout:   30 * time.Second,
			UserAgent: userAgent,
			SiteID:    config.SiteID,
		})
		pageClient = NewSiteHTTPClient(SiteHTTPClientConfig{
			Timeout:   30 * time.Second,
			UserAgent: hddolbyBrowserUA,
			SiteID:    config.SiteID,
		})
	}

	return &HDDolbyDriver{
		BaseURL:    baseURL,
		APIURL:     apiURL,
		APIKey:     config.APIKey,
		Cookie:     config.Cookie,
		httpClient: httpClient,
		pageClient: pageClient,
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

	if resp.StatusCode == http.StatusTooManyRequests {
		return result, fmt.Errorf("HTTP 429: %w", ErrRateLimited)
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
			return result, fmt.Errorf("HTTP %d: %w", resp.StatusCode, hddolbyAPIError(result))
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

	// 出错时 HTTP 照样是 200：看返回的 status（成功是 0）与 error
	if result.Error != nil || result.Status != 0 {
		return result, hddolbyAPIError(result)
	}

	return result, nil
}

// hddolbyStatusIllegalKey 是 key 不对（包括过期）时 API 返回的 status（「Illegal api key」）；没带 key、用浏览器访问时是 10405
// （「Missing x-api-key」「Browser access is blocked!」）。
const hddolbyStatusIllegalKey = 30001

// hddolbyAPIError 把 API 返回的错误写成一句话：key 不对或过期、没带 key 时包着 ErrInvalidCredentials；
// 超了每小时 50 次（「Limit of api call per hour reached! please wait for four hour!」，status 也是 10405）时包着 ErrRateLimited。
func hddolbyAPIError(res HDDolbyResponse) error {
	msg := ""
	if res.Error != nil {
		msg = strings.TrimSpace(res.Error.Message)
		if msg == "" {
			msg = strings.TrimSpace(res.Error.Code)
		}
	}
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "limit of api call") || strings.Contains(lower, "per hour reached") {
		return fmt.Errorf("%w: 站点 API 每小时最多 50 次，超了要等 4 小时（status %d: %s）", ErrRateLimited, res.Status, msg)
	}
	if res.Status == hddolbyStatusIllegalKey || strings.Contains(lower, "api key") || strings.Contains(lower, "x-api-key") {
		return fmt.Errorf("%w: API Key（RSS key）不对或已过期，到站点的 RSS 页重新生成链接，把链接里的 RSS key 填进站点设置（status %d: %s）",
			ErrInvalidCredentials, res.Status, msg)
	}
	return fmt.Errorf("API error (status %d): %s", res.Status, msg)
}

func (d *HDDolbyDriver) PrepareSearch(query SearchQuery) (HDDolbyRequest, error) {
	// page_number 其实是跳过前面多少条（2026-11-02 试过：1 比 0 少了第一条），从第一条读要给 0
	searchReq := HDDolbySearchRequest{
		Keyword:    query.Keyword,
		PageNumber: 0,
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
			// 不把返回的内容写进错误：里面有每个种子的 downhash（下载令牌）
			return nil, fmt.Errorf("parse search data (%d bytes): %w", len(res.Data), err)
		}
		searchData.Data = torrents
	}

	items := make([]TorrentItem, 0, len(searchData.Data))
	for _, t := range searchData.Data {
		items = append(items, *d.torrentToItem(t))
	}

	return items, nil
}

// hddolbyDiscount 是 sp_state 对应的优惠（NexusPHP 的促销状态：1 普通、2 免费、3 2X、4 2X 免费、5 50%、6 2X 50%、7 30%）。
// 2026-11-02 拿 API 的前 100 条与种子列表的促销图标逐条对过：免费是 2、2X 免费是 4、50% 是 5。
func hddolbyDiscount(spState int) DiscountLevel {
	switch spState {
	case 2:
		return DiscountFree
	case 3:
		return Discount2xUp
	case 4:
		return Discount2xFree
	case 5:
		return DiscountPercent50
	case 6:
		return Discount2x50
	case 7:
		return DiscountPercent30
	default:
		return DiscountNone
	}
}

// promotion_time_type：优惠的时限。
const (
	hddolbyPromotionDefault   = 0 // 跟站点默认（发布时随机给的促销）
	hddolbyPromotionPermanent = 1 // 永久
	hddolbyPromotionUntil     = 2 // 到 promotion_until 为止
)

// hddolbyDefaultFreeHours 是时限跟站点默认的免费、2X 免费从发布起多少小时结束：站点按全站设置在页面上算出结束时间，
// API 只给 0000-00-00。2026-11-02 对过 11 个这样的种子，列表上的结束时间都是发布时间加 24 小时；这样的 50% 没有结束时间。
const hddolbyDefaultFreeHours = 24

// applyDiscount 按 sp_state 写优惠，按 promotion_time_type 写结束时间：到某个时间为止的取 promotion_until；永久的免费
// 记成长期有效；跟站点默认的免费、2X 免费按发布时间加 hddolbyDefaultFreeHours 小时。时间解析不了、时限认不出时免费的有效性是 unknown。
func (d *HDDolbyDriver) applyDiscount(item *TorrentItem, t HDDolbyTorrent) {
	item.DiscountLevel = hddolbyDiscount(t.SpState)
	if item.DiscountLevel == DiscountNone {
		return
	}
	switch t.PromotionTimeType {
	case hddolbyPromotionUntil:
		end, err := d.parseDiscountEndTime(t.PromotionUntil)
		item.DiscountEndTime = end
		if err != nil {
			NoteDiscountTimeUnparsable(item, "promotion_until")
		}
	case hddolbyPromotionPermanent:
		if IsFreeTorrent(item.DiscountLevel) {
			item.DiscountValidity = FreePermanent
		}
	case hddolbyPromotionDefault:
		if item.DiscountLevel != DiscountFree && item.DiscountLevel != Discount2xFree {
			return
		}
		added, err := ParseTimeInCST("2006-01-02 15:04:05", strings.TrimSpace(t.Added))
		if err != nil {
			NoteDiscountTimeUnparsable(item, "added")
			return
		}
		item.DiscountEndTime = added.Add(hddolbyDefaultFreeHours * time.Hour)
	default:
		markDiscountUnreliable(item, discountFormatParseNote("promotion_time_type"))
	}
}

// parseDiscountEndTime 解析 promotion_until：空值与 0000-00-00 00:00:00 表示没有结束时间（零时间、没有错误，
// 不等于长期免费）；非空但无法解析返回错误。
func (d *HDDolbyDriver) parseDiscountEndTime(until string) (time.Time, error) {
	until = strings.TrimSpace(until)
	if until == "" || until == "0000-00-00 00:00:00" {
		return time.Time{}, nil
	}
	t, err := ParseTimeInCST("2006-01-02 15:04:05", until)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}

// hddolbyCategories 是站点的分类（RSS 页与种子列表的分类编号，2026-11-02 读）
var hddolbyCategories = map[int]string{
	401: "电影",
	402: "电视剧",
	403: "综艺",
	404: "纪录片",
	405: "动漫",
	406: "音乐视频",
	407: "体育",
	408: "音乐",
	409: "其他",
	410: "游戏",
	411: "学习",
}

func (d *HDDolbyDriver) getCategoryName(catID int) string {
	if name, ok := hddolbyCategories[catID]; ok {
		return name
	}
	return strconv.Itoa(catID)
}

// hddolbyTagNames 是标签代码的名字：种子列表里标签写成 <span class="tags tgf">官方</span>，class 里 t 后面就是 API 的代码。
var hddolbyTagNames = map[string]string{
	"gf": "官方", "gy": "国语", "yy": "粤语", "ja": "日语", "ko": "韩语", "zz": "中字", "jz": "禁转", "xz": "限转",
	"diy": "DIY", "sf": "首发", "yq": "应求", "m0": "零魔", "yc": "原创", "gz": "官字", "db": "Dolby Vision",
	"hdr10": "HDR10", "hdrm": "HDR10+", "tx": "特效", "lz": "连载", "wj": "完结", "hdrv": "HDR Vivid", "hlg": "HLG",
	"hq": "高码率", "hfr": "高帧率",
}

// hddolbyTags 把分号分隔的标签代码换成名字；不认识的代码不写。
func hddolbyTags(raw string) []string {
	var out []string
	for code := range strings.SplitSeq(raw, ";") {
		if name, ok := hddolbyTagNames[strings.ToLower(strings.TrimSpace(code))]; ok {
			out = append(out, name)
		}
	}
	return out
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
		return UserInfo{}, fmt.Errorf("parse user data (%d bytes): %w", len(res.Data), err)
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
	var peerErr, bonusErr error
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

	// 做种列表与时魔没取到不让整次失败：按缺的字段交回去（*PartialUserInfoError），保存时沿用上一次的
	g.Go(func() error {
		peers, err := d.GetPeerData(gctx)
		mu.Lock()
		defer mu.Unlock()
		peerData, peerErr = peers, err
		return nil
	})

	g.Go(func() error {
		bph, err := d.GetBonusPerHour(gctx)
		mu.Lock()
		defer mu.Unlock()
		bonusPerHour, bonusErr = bph, err
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

	var missing []string
	var firstErr error
	if peerErr != nil {
		missing = append(missing, "seeding", "seederSize", "leeching", "leecherSize")
		firstErr = fmt.Errorf("做种列表: %w", peerErr)
	}
	if bonusErr != nil {
		missing = append(missing, "bonusPerHour")
		if firstErr == nil {
			firstErr = fmt.Errorf("时魔: %w", bonusErr)
		}
	}
	if len(missing) > 0 {
		return info, &PartialUserInfoError{Missing: missing, Err: firstErr}
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

// hddolbyBrowserUA 是用 Cookie 打开网页（时魔、签到）时的 User-Agent。API 不认浏览器的 User-Agent（「Browser access is blocked!」），
// 网页这边用浏览器的。
const hddolbyBrowserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// errHDDolbyNoCookie 是要打开网页、站点设置里却没填 Cookie。
var errHDDolbyNoCookie = errors.New("没有配置 Cookie")

// getPage 用 Cookie 打开站点的网页（时魔、签到），返回页面内容。Cloudflare 质询、登录页、两步验证页都返回对应的错误：
// 站点强制两步验证，Cookie 拿到另一个网络、另一台设备上用时，页面可能先要输两步验证码（页面里跳到 take2fa.php）。
func (d *HDDolbyDriver) getPage(ctx context.Context, path string) ([]byte, error) {
	if strings.TrimSpace(d.Cookie) == "" {
		return nil, errHDDolbyNoCookie
	}
	headers := map[string]string{
		"Cookie": d.Cookie,
		"Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
	}
	resp, err := d.pageClient.DoRequest(ctx, http.MethodGet, d.BaseURL+path, nil, headers)
	if err != nil {
		return nil, fmt.Errorf("打开 %s: %w", path, err)
	}
	switch {
	case resp.IsCloudflareChallenge():
		return nil, fmt.Errorf("HTTP %d: %w", resp.StatusCode, ErrCloudflareChallenge)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, ErrInvalidCredentials
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, fmt.Errorf("HTTP 429: %w", ErrRateLimited)
	case resp.StatusCode != http.StatusOK:
		return nil, &HTTPStatusError{Code: resp.StatusCode}
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}
	if isLoginPage(doc) {
		return nil, withPageTitle(ErrSessionExpired, doc)
	}
	if is2FAPage(doc) {
		return nil, fmt.Errorf("%w: 站点要先输两步验证码，这份 Cookie 在 pt-tools 这里打开网页时还没通过两步验证（换了网络或设备时会这样）；"+
			"搜索、用户数据与从搜索结果、RSS 下载种子走 API，不受影响", withPageTitle(Err2FARequired, doc))
	}
	return resp.Body, nil
}

// hddolbyBonusTotal 取时魔页「您目前每小时可以获取到的鲸币/做种积分如下」表格里「合计」那一行的最后一格「鲸币 / 做种积分」：
// <td>合计</td><td colspan="5">-</td><td>460</td><td>25.98 / 25.98</td>
var hddolbyBonusTotal = regexp.MustCompile(`(?s)<td>合计</td>.*?<td[^>]*>(\d+(?:\.\d+)?)\s*/\s*\d+(?:\.\d+)?</td>`)

// GetBonusPerHour 用 Cookie 读时魔页的每小时鲸币。没配 Cookie 时是 0、没有错误（API 不给时魔）；
// 页面打不开、要两步验证、或者认不出「合计」那一行时返回错误。
func (d *HDDolbyDriver) GetBonusPerHour(ctx context.Context) (float64, error) {
	if strings.TrimSpace(d.Cookie) == "" {
		return 0, nil
	}
	body, err := d.getPage(ctx, "/mybonus.php")
	if err != nil {
		return 0, err
	}
	m := hddolbyBonusTotal.FindSubmatch(body)
	if len(m) < 2 {
		return 0, errors.New("时魔页里没有「合计」那一行")
	}
	return strconv.ParseFloat(string(m[1]), 64)
}

// Attend 用 Cookie 打开签到页签到（NexusPHP 的 attendance.php，打开就签到），按页面文字判断结果。
func (d *HDDolbyDriver) Attend(ctx context.Context) (AttendResult, error) {
	cfg := ResolveAttendanceConfig(d.siteDefinition)
	if cfg == nil {
		return AttendResult{}, ErrAttendanceUnsupported
	}
	if cfg.Unsupported != "" {
		return AttendResult{}, fmt.Errorf("%w: %s", ErrAttendanceUnsupported, cfg.Unsupported)
	}
	if strings.TrimSpace(d.Cookie) == "" {
		return AttendResult{}, fmt.Errorf("%w: %s", ErrAttendanceUnsupported, hddolbyAttendNeedsCookie)
	}
	if m := strings.ToUpper(strings.TrimSpace(cfg.Method)); m != "" && m != http.MethodGet {
		return AttendResult{}, fmt.Errorf("unsupported attendance method %q", cfg.Method)
	}
	body, err := d.getPage(ctx, cfg.Path)
	if err != nil {
		return AttendResult{}, fmt.Errorf("attendance request: %w", err)
	}
	if result, ok := cfg.Match(attendancePageText(body)); ok {
		return result, nil
	}
	return AttendResult{}, fmt.Errorf("%w: 页面里没有签到成功或今日已签到的文字", ErrAttendanceUnrecognized)
}

// hddolbyAttendNeedsCookie 是没配 Cookie 时签到页上写的原因。
const hddolbyAttendNeedsCookie = "签到要用 Cookie：在站点设置里填上 Cookie（API Key 不能签到）"

// AttendanceUnsupportedReason 报告这个站点实例能不能签到：站点定义没写签到，或者没配 Cookie 时返回原因。
func (d *HDDolbyDriver) AttendanceUnsupportedReason() string {
	cfg := ResolveAttendanceConfig(d.siteDefinition)
	switch {
	case cfg == nil:
		return "该站点的架构暂不支持签到"
	case cfg.Unsupported != "":
		return cfg.Unsupported
	case strings.TrimSpace(d.Cookie) == "":
		return hddolbyAttendNeedsCookie
	}
	return ""
}

// PrepareDownload、ParseDownload 是 Driver 接口要的；BaseSite 下载时用驱动自己的 Download（下载要 downhash 或 Cookie，
// 这一个请求给不了）。
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

// Download 按种子编号下载（辅种、转种、批量下载这些只知道编号的入口；BaseSite 下载时用它）。
func (d *HDDolbyDriver) Download(ctx context.Context, torrentID string) ([]byte, error) {
	return d.DownloadWithHash(ctx, torrentID, "")
}

// DownloadWithHash 下载种子文件：带 downhash 的 download.php（downhash 只在 API 搜索结果里）不要 Cookie，不带 downhash 又没有
// Cookie 时站点回登录页。调用方没有 downhash（只知道种子编号）时：配了 Cookie 就像在网页上点「下载」一样用 Cookie 下，
// 什么种子都行（见 downloadWithCookie）；没配 Cookie 时到详情缓存（最近 100 条搜索结果）里找 downhash，老种子找不到。
func (d *HDDolbyDriver) DownloadWithHash(ctx context.Context, torrentID, downhash string) ([]byte, error) {
	if strings.TrimSpace(downhash) == "" {
		if strings.TrimSpace(d.Cookie) != "" {
			return d.downloadWithCookie(ctx, torrentID)
		}
		h, err := d.lookupDownhash(ctx, torrentID)
		if err != nil {
			return nil, err
		}
		downhash = h
	}
	downloadURL := fmt.Sprintf("%s/download.php?id=%s&downhash=%s", d.BaseURL, torrentID, downhash)

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
	// 种子文件以 d 开头（bencode 的字典）；downhash 不对时站点回「error B!」这类文字或网页
	if body[0] != 'd' {
		return nil, fmt.Errorf("下载到的不是种子文件（%d 字节）：downhash 可能不对或已失效", len(body))
	}

	return body, nil
}

// downloadWithCookie 用 Cookie 下载种子文件：download.php?id= 不带 downhash，和网页上点「下载」一样（2026-11-02 生产核对过，
// 辅种的老种子拿到的种子与 IYUU 给的 info hash 一致），不为找 downhash 调 API（每小时只有 50 次）。网页用浏览器的 User-Agent。
// 账号开着「第一次下载提示」时带上 letdown=1 再下；登录页、两步验证页、别的提示页返回对应的错误（见 CheckTorrentFile）。
func (d *HDDolbyDriver) downloadWithCookie(ctx context.Context, torrentID string) ([]byte, error) {
	id, err := strconv.Atoi(strings.TrimSpace(torrentID))
	if err != nil {
		return nil, fmt.Errorf("invalid torrent ID %q: %w", torrentID, err)
	}
	headers := map[string]string{
		"Cookie": d.Cookie,
		"Accept": "application/x-bittorrent,*/*",
	}
	return fetchSkippingFirstNotice(fmt.Sprintf("%s/download.php?id=%d", d.BaseURL, id), func(u string) ([]byte, error) {
		resp, err := d.pageClient.DoRequest(ctx, http.MethodGet, u, nil, headers)
		if err != nil {
			return nil, fmt.Errorf("download request failed: %w", err)
		}
		switch {
		case resp.IsCloudflareChallenge():
			return nil, fmt.Errorf("HTTP %d: %w", resp.StatusCode, ErrCloudflareChallenge)
		case resp.StatusCode == http.StatusTooManyRequests:
			return nil, fmt.Errorf("HTTP 429: %w", ErrRateLimited)
		case resp.StatusCode != http.StatusOK:
			return nil, &HTTPStatusError{Code: resp.StatusCode}
		}
		if err := CheckTorrentFile(resp.Body); err != nil {
			return nil, err
		}
		return resp.Body, nil
	})
}

// lookupDownhash 在详情缓存（最近 100 条搜索结果）里找种子的 downhash。
func (d *HDDolbyDriver) lookupDownhash(ctx context.Context, torrentID string) (string, error) {
	id, err := strconv.Atoi(strings.TrimSpace(torrentID))
	if err != nil {
		return "", fmt.Errorf("invalid torrent ID %q: %w", torrentID, err)
	}
	torrents, _, _, err := d.getOrRefreshDetailCache(ctx, time.Time{})
	if err != nil {
		return "", fmt.Errorf("查种子的 downhash: %w", err)
	}
	for _, t := range torrents {
		if t.ID == id && strings.TrimSpace(t.DownHash) != "" {
			return t.DownHash, nil
		}
	}
	return "", fmt.Errorf("下载种子 %s 要带 downhash，最近 %d 条搜索结果里没有这个种子：在站点设置里填上 Cookie 就能按编号下载"+
		"（辅种、转种这些只知道编号的入口都要），或者从搜索结果、RSS 里下载", torrentID, len(torrents))
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

// hddolbyDetailCacheTTL 是详情缓存（最近 100 条列表结果）的有效期
const hddolbyDetailCacheTTL = 5 * time.Minute

// GetTorrentDetail 在详情缓存（最近 100 条列表结果，最多 hddolbyDetailCacheTTL 刷新一次）里找种子。
// 结果的 FetchedAt 是缓存取得的时间；ctx 带 WithFreshAfter 时缓存早于那个时间就先刷新，刷新失败返回错误。
// 缓存里找不到时返回 ErrTorrentDetailUnavailable：读不到优惠不等于不是免费。
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

	freshAfter, _ := FreshAfterFrom(ctx)
	torrents, fetchedAt, refreshed, err := d.getOrRefreshDetailCache(ctx, freshAfter)
	if err != nil {
		return nil, err
	}
	if item := d.findDetail(torrents, numericID, fetchedAt); item != nil {
		return item, nil
	}

	d.detailCacheMu.Lock()
	d.detailCacheMiss++
	missCount := d.detailCacheMiss
	d.detailCacheMu.Unlock()

	// 连续几次都找不到：缓存可能已经落后，刷新一次再找（这次调用刚刷新过就不重复请求站点）
	if missCount >= 3 && !refreshed {
		d.invalidateDetailCache()
		torrents, fetchedAt, _, err = d.getOrRefreshDetailCache(ctx, freshAfter)
		if err != nil {
			return nil, fmt.Errorf("refresh detail cache: %w", err)
		}
		if item := d.findDetail(torrents, numericID, fetchedAt); item != nil {
			return item, nil
		}
	}

	return nil, fmt.Errorf("HDDolby 最近 %d 条结果里没有种子 %s: %w", len(torrents), torrentID, ErrTorrentDetailUnavailable)
}

// findDetail 在详情缓存里找种子，结果的 FetchedAt 是缓存取得的时间；找到时清零连续未命中次数
func (d *HDDolbyDriver) findDetail(torrents []HDDolbyTorrent, id int, fetchedAt time.Time) *TorrentItem {
	for _, t := range torrents {
		if t.ID != id {
			continue
		}
		d.detailCacheMu.Lock()
		d.detailCacheMiss = 0
		d.detailCacheMu.Unlock()
		item := d.torrentToItem(t)
		item.FetchedAt = fetchedAt.UTC()
		return item
	}
	return nil
}

// torrentToItem 把 API 的一个种子写成搜索结果。下载地址是 pt-tools 自己的下载接口，带上种子的 downhash（站点下载种子要它）。
func (d *HDDolbyDriver) torrentToItem(t HDDolbyTorrent) *TorrentItem {
	siteID := d.getSiteID()
	item := &TorrentItem{
		ID:          strconv.Itoa(t.ID),
		Title:       t.Name,
		Subtitle:    t.SmallDescr,
		InfoHash:    strings.ToLower(strings.TrimSpace(t.InfoHash)),
		SizeBytes:   t.Size,
		Seeders:     t.Seeders,
		Leechers:    t.Leechers,
		Snatched:    t.TimesCompleted,
		Tags:        hddolbyTags(t.Tags),
		Category:    d.getCategoryName(t.Category),
		URL:         fmt.Sprintf("%s/details.php?id=%d", d.BaseURL, t.ID),
		DownloadURL: fmt.Sprintf("/api/site/%s/torrent/%d/download?downhash=%s", siteID, t.ID, t.DownHash),
		HasHR:       t.HR > 0,
		SourceSite:  siteID,
	}
	if t.Added != "" {
		if uploadTime, err := ParseTimeInCST("2006-01-02 15:04:05", t.Added); err == nil {
			item.UploadedAt = uploadTime.Unix()
		}
	}
	d.applyDiscount(item, t)
	return item
}

// getOrRefreshDetailCache 返回详情缓存、缓存取得的时间，以及这次是否刷新过。缓存为空、超过有效期，
// 或早于 freshAfter（非零时）就先刷新；刷新失败返回错误，不拿旧缓存充数。刷新在写锁里做：
// 等锁期间别的调用已经刷新到足够新的话直接用，不重复请求站点。
func (d *HDDolbyDriver) getOrRefreshDetailCache(ctx context.Context, freshAfter time.Time) ([]HDDolbyTorrent, time.Time, bool, error) {
	d.detailCacheMu.RLock()
	if d.detailCacheUsable(freshAfter) {
		defer d.detailCacheMu.RUnlock()
		return d.detailCache, d.detailCacheTime, false, nil
	}
	d.detailCacheMu.RUnlock()

	d.detailCacheMu.Lock()
	defer d.detailCacheMu.Unlock()

	if d.detailCacheUsable(freshAfter) {
		return d.detailCache, d.detailCacheTime, false, nil
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
		return nil, time.Time{}, false, fmt.Errorf("execute search API: %w", err)
	}

	var searchData HDDolbySearchResponse
	if err := json.Unmarshal(res.Data, &searchData); err != nil {
		var torrents []HDDolbyTorrent
		if err2 := json.Unmarshal(res.Data, &torrents); err2 != nil {
			return nil, time.Time{}, false, fmt.Errorf("parse search data: %w", err)
		}
		searchData.Data = torrents
	}

	d.detailCache = searchData.Data
	d.detailCacheTime = time.Now()
	d.detailCacheMiss = 0
	return d.detailCache, d.detailCacheTime, true, nil
}

// detailCacheUsable 报告详情缓存能不能直接用：有数据、没过有效期，且不早于 freshAfter（非零时）。调用方持有 detailCacheMu。
func (d *HDDolbyDriver) detailCacheUsable(freshAfter time.Time) bool {
	if d.detailCache == nil || time.Since(d.detailCacheTime) >= hddolbyDetailCacheTTL {
		return false
	}
	return freshAfter.IsZero() || !d.detailCacheTime.Before(freshAfter)
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
		return nil, fmt.Errorf("HDDolby 站点需要配置 API Key（站点 RSS 页生成的链接里的 RSS key）")
	}
	// Cookie 可以不填：时魔、签到与只知道种子编号的下载（辅种、转种等）用它

	siteDef := GetDefinitionRegistry().GetOrDefault(config.ID)

	driver := NewHDDolbyDriver(HDDolbyDriverConfig{
		BaseURL: config.BaseURL,
		APIKey:  opts.APIKey,
		Cookie:  opts.Cookie,
		SiteID:  config.ID,
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
