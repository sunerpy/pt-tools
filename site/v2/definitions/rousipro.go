package definitions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	// 签到走 Rousi 的接口（见 rousiDriver.Attend），这里只标出支持签到；路径和方法不参与签到流程
	Attendance:   &v2.AttendanceConfig{Path: rousiAttendanceEndpoint, Method: http.MethodPost},
	CreateDriver: createRousiDriver,
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
		BaseURL:     baseURL,
		Passkey:     opts.Passkey,
		LoginCookie: opts.Cookie,
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
	// Body 是 JSON 请求体（POST 时用），为空时不带请求体。
	Body []byte
	// Headers 是额外的请求头，同名时覆盖默认的（签到接口的 API Key 放在 api-token 头里）。
	Headers map[string]string
	// RawBody 为 true 时响应体按原始字节返回（种子文件下载），不按 JSON 解析。
	RawBody bool
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
	LoginCookie    string
	httpClient     *v2.SiteHTTPClient
	userAgent      string
	siteDefinition *v2.SiteDefinition
}

type rousiDriverConfig struct {
	BaseURL     string
	Passkey     string
	LoginCookie string
	HTTPClient  *v2.SiteHTTPClient
	UserAgent   string
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
		BaseURL:     baseURL,
		Passkey:     config.Passkey,
		LoginCookie: config.LoginCookie,
		httpClient:  httpClient,
		userAgent:   userAgent,
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
	for k, v := range req.Headers {
		headers[k] = v
	}
	var body io.Reader
	if len(req.Body) > 0 {
		body = bytes.NewReader(req.Body)
	}

	resp, err := d.httpClient.DoRequest(ctx, method, fullURL, body, headers)
	if err != nil {
		// 下载地址的路径里带 passkey，传输层错误会原样带出完整 URL，这里先隐去再返回。
		return rousiResponse{}, fmt.Errorf("request failed: %w", d.redactPasskey(err))
	}

	result := rousiResponse{
		RawBody:    resp.Body,
		StatusCode: resp.StatusCode,
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return result, v2.ErrInvalidCredentials
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return result, fmt.Errorf("HTTP 429: %w", v2.ErrRateLimited)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if len(resp.Body) > 0 {
			_ = json.Unmarshal(resp.Body, &result)
		}
		return result, fmt.Errorf("API error: HTTP %d - %s", resp.StatusCode, result.Message)
	}

	if req.RawBody {
		return result, nil
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
	// SearchQuery.Page 与 Rousi API 都从 1 开始，直接透传
	if query.Page > 0 {
		params["page"] = strconv.Itoa(query.Page)
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
		discount, discountEnd, untilErr := d.parsePromotion(t.Promotion)

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
			// 下载走 pt-tools 的代理地址，passkey 只在服务端拼接，不随搜索结果下发给浏览器。
			DownloadURL: fmt.Sprintf("/api/site/%s/torrent/%s/download", siteID, t.UUID),
			SourceSite:  siteID,
		}
		if untilErr != nil {
			v2.NoteDiscountTimeUnparsable(&item, "promotion.until")
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

var errRousiUntilFormat = errors.New("unrecognized promotion until format")

// parsePromotion 返回优惠级别、结束时间，以及生效中的促销 until 非空却所有写法都解析不了时的错误
// （这时结束时间留空；调用方要把它记成 unknown，不能当成没有结束时间）。
func (d *rousiDriver) parsePromotion(promo *rousiPromotion) (v2.DiscountLevel, time.Time, error) {
	if promo == nil || !promo.IsActive {
		return v2.DiscountNone, time.Time{}, nil
	}

	if logger := global.GetSloggerSafe(); logger != nil {
		logger.Debugf("[RousiPro] promotion: type=%d, down=%.2f, up=%.2f, text=%s, until=%s",
			promo.Type, promo.DownMultiplier, promo.UpMultiplier, promo.Text, promo.Until)
	}

	var endTime time.Time
	var untilErr error
	if until := strings.TrimSpace(promo.Until); until != "" {
		untilErr = errRousiUntilFormat
		for _, layout := range []string{
			time.RFC3339,
			"2006-01-02T15:04:05-0700",
			"2006-01-02 15:04:05",
		} {
			parse := time.Parse
			if layout == "2006-01-02 15:04:05" {
				// 不带时区的时间按北京时间解释；time.Parse 会当成 UTC，事后 In(CST) 只改显示时区，时刻会晚 8 小时。
				parse = v2.ParseTimeInCST
			}
			if t, err := parse(layout, until); err == nil {
				endTime, untilErr = t, nil
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
	return level, endTime, untilErr
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
	// 站点换到新系统以后 last_active_at 不再更新（2026-10 实测：当天网页登录、签到之后仍停在 2026-08-11），
	// 只看它保号提醒会一直报「即将封禁」。今天签到过就说明账号今天还在用，取两者中较新的。
	if signed := d.attendanceDay(ctx); signed > info.LastAccess {
		info.LastAccess = signed
	}

	return info, nil
}

// rousiAttendanceEndpoint 是 Rousi 的签到接口。个人 API Key 放在 api-token 头里，要有签到（attendance:claim）权限；
// 请求体 {"mode":"fixed"} 是固定奖励。返回 200 + code 0 是签到成功，400 + code 1 +「今日已签到」是今天签过了
// （2026-10-19 用真实 Key 验证过已签到的返回）。出处：MoviePilot-Plugins 的 plugins.v2/autosignin/sites/rousipro.py
// 与 jxxghp/MoviePilot-Plugins#1067。
const rousiAttendanceEndpoint = "/api/points/attendance"

// rousiDayZone 是 Rousi 划分签到日的时区（新接口返回 day_boundary_timezone=Asia/Shanghai），固定东八区，不依赖系统时区库。
var rousiDayZone = time.FixedZone("CST", 8*3600)

type rousiAttendanceStats struct {
	ClaimedToday  bool   `json:"claimed_today"`
	Today         string `json:"today"`
	CurrentStreak int    `json:"current_streak"`
	TotalDays     int    `json:"total_days"`
}

// attendanceDay 查签到统计：今天签到过时返回今天零点（东八区）的时间戳，没签或查不到时返回 0。
// 只是给最近活跃时间补一个依据，查不到不影响用户信息本身。
func (d *rousiDriver) attendanceDay(ctx context.Context) int64 {
	res, err := d.Execute(ctx, rousiRequest{Endpoint: rousiAttendanceEndpoint + "/stats", Method: http.MethodGet})
	if err != nil || !res.IsSuccess() {
		return 0
	}
	var stats rousiAttendanceStats
	if json.Unmarshal(res.Data, &stats) != nil || !stats.ClaimedToday {
		return 0
	}
	day, err := time.ParseInLocation("2006-01-02", stats.Today, rousiDayZone)
	if err != nil {
		return 0
	}
	return day.Unix()
}

type rousiAttendanceReward struct {
	BaseReward    json.Number `json:"base_reward"`
	TotalReward   json.Number `json:"total_reward"`
	Reward        json.Number `json:"reward"`
	CurrentStreak int         `json:"current_streak"`
	TotalDays     int         `json:"total_days"`
}

// Attend 签到一次。今天已经签过时返回「今天已签到」；API Key 失效或没有签到权限时返回 ErrInvalidCredentials，并说明怎么处理。
func (d *rousiDriver) Attend(ctx context.Context) (v2.AttendResult, error) {
	res, err := d.Execute(ctx, rousiRequest{
		Endpoint: rousiAttendanceEndpoint,
		Method:   http.MethodPost,
		Body:     []byte(`{"mode":"fixed"}`),
		Headers:  map[string]string{"api-token": d.Passkey},
	})
	switch {
	case err == nil && res.IsSuccess():
		return v2.AttendResult{Status: v2.AttendSigned, Message: rousiAttendMessage(res.Data)}, nil
	case res.StatusCode == http.StatusBadRequest && res.Code == 1 && strings.Contains(res.Message, "已签到"):
		return v2.AttendResult{Status: v2.AttendAlready, Message: res.Message}, nil
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return v2.AttendResult{}, fmt.Errorf("%w: Rousi 拒绝了签到：API Key 已失效或没有签到权限。在 Rousi 的个人 API Key 里勾上签到权限（attendance:claim），再把 Key 填到 pt-tools 的站点设置",
			v2.ErrInvalidCredentials)
	case err != nil:
		return v2.AttendResult{}, fmt.Errorf("Rousi 签到失败: %w", err)
	default:
		msg := strings.TrimSpace(res.Message)
		if msg == "" {
			msg = fmt.Sprintf("code=%d", res.Code)
		}
		return v2.AttendResult{}, fmt.Errorf("Rousi 签到失败: %s", msg)
	}
}

// rousiAttendMessage 把签到成功的返回写成一句话：有奖励和连续天数时带上，没有时只说签到成功。
func rousiAttendMessage(data json.RawMessage) string {
	var r rousiAttendanceReward
	if len(data) == 0 || json.Unmarshal(data, &r) != nil {
		return "签到成功"
	}
	msg := "签到成功"
	for _, n := range []json.Number{r.TotalReward, r.Reward, r.BaseReward} {
		if n != "" && n != "0" {
			msg += "，获得 " + n.String() + " 魔力"
			break
		}
	}
	if r.CurrentStreak > 0 {
		msg += fmt.Sprintf("，已连续签到 %d 天", r.CurrentStreak)
	}
	return msg
}

func (d *rousiDriver) PrepareDownload(torrentID string) (rousiRequest, error) {
	return rousiRequest{
		Endpoint: fmt.Sprintf("/api/torrent/%s/download/%s", torrentID, d.Passkey),
		Method:   http.MethodGet,
		RawBody:  true,
	}, nil
}

func (d *rousiDriver) ParseDownload(res rousiResponse) ([]byte, error) {
	if len(res.RawBody) == 0 {
		return nil, fmt.Errorf("empty download response")
	}
	if err := v2.ValidateTorrentFile(res.RawBody); err != nil {
		return nil, fmt.Errorf("下载内容不是有效的种子文件: %w", err)
	}
	return res.RawBody, nil
}

// redactPasskey 把错误文本里的 passkey 换成 ***，同时保留原错误链，errors.Is 仍可用。
func (d *rousiDriver) redactPasskey(err error) error {
	if err == nil || d.Passkey == "" || !strings.Contains(err.Error(), d.Passkey) {
		return err
	}
	return &redactedError{err: err, secret: d.Passkey}
}

type redactedError struct {
	err    error
	secret string
}

func (e *redactedError) Error() string { return strings.ReplaceAll(e.err.Error(), e.secret, "***") }

func (e *redactedError) Unwrap() error { return e.err }

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
		// 不能返回 (nil, nil)：调用方会把空详情当成功结果解引用。
		// 错误里不带编号：它取自 RSS 链接，链接带查询参数时可能含密钥
		return nil, fmt.Errorf("RousiPro 详情返回 HTTP 404: %w: %w", v2.ErrTorrentDetailUnavailable, v2.ErrTorrentNotFound)
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

	discount, discountEnd, untilErr := d.parsePromotion(result.Data.Promotion)

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
		FetchedAt:       time.Now().UTC(),
	}
	if untilErr != nil {
		v2.NoteDiscountTimeUnparsable(item, "promotion.until")
	}

	if result.Data.CreatedAt != "" {
		if uploadTime, err := time.Parse(time.RFC3339, result.Data.CreatedAt); err == nil {
			item.UploadedAt = uploadTime.Unix()
		}
	}

	return item, nil
}
