package definitions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// IPTorrents（iptorrents.com）是站点自己写的程序，不是 NexusPHP、Gazelle 或 Unit3D：没有给第三方用的 API，
// 用登录后的 Cookie（uid、pass）读网页。页面结构按 2026-11-21 登录后取到的英文界面整理：
//
//   - /t：种子列表与搜索。q 是关键词，p 是页码（从 1 起），free 只看免费，分类编号直接写成参数名（?72）；
//     结果在 table#torrents，没有结果时表格是空的，另有一段「No results for …」
//   - /t/{id}：种子详情，跳到 /torrent.php?id={id}；种子不存在时直接回 404
//   - /mybonus.php：页头有自己的用户链接 /u/{id}，正文有积分余额与每小时的积分
//   - /user.php?u={id}：资料页（2026-11 改版的 upf-* 布局），有精确到字节的上传下载、等级、加入时间与最近访问（Last seen）
//   - /peers?u={id}：做种数与做种体积；「To seed」是还没做种到 1:1 或 14 天的种子数
//
// 页面上的时间都是 UTC：页头的注释写着「… UTC」，资料页的 <time datetime> 带 +00:00，详情页的上传时间与相对时间对得上。
var IPTorrentsDefinition = &v2.SiteDefinition{
	ID:             "iptorrents",
	Name:           "IPTorrents",
	Aka:            []string{"IPT"},
	Description:    "综合性 PT 站点（英文界面），站点自己写的程序，用 Cookie 访问",
	Schema:         v2.SchemaIPTorrents,
	AuthMethod:     v2.AuthMethodCookie,
	URLs:           []string{iptDefaultBaseURL},
	FaviconURL:     "https://iptorrents.com/favicon.ico",
	TimezoneOffset: "+0000",
	RateLimit:      0.5,
	RateBurst:      2,
	// FAQ「What are bonus points and how can I get them?」与积分页「How you earn」：积分只按做种计
	// （每个完整做种的种子每小时 0.2 分，最多算 15 个），站点没有每日签到
	Attendance: &v2.AttendanceConfig{Unsupported: "IPTorrents 没有每日签到，积分只按做种计"},
	// 整站 H&R（/rules「H&R Rules & Guidelines」、FAQ「Why have I received a H&R warning?」）：所有种子（免费种也算）
	// 都要做种到分享率 1:1 或 336 小时（两周）。VIP、Uploader 与管理组不收 H&R 警告，这里不区分用户等级；
	// 分享率 1:1 那一条可以在站点详情的设置里把「H&R 上传量达标（倍）」填成 1。
	HREnabled:         true,
	HRSeedTimeHours:   336,
	LevelRequirements: iptorrentsLevelRequirements,
	CreateDriver:      createIPTorrentsSite,
}

// iptorrentsLevelRequirements 按 FAQ 的「What are the different user classes?」「How does this promotion thing work anyway?」：
// 只有 Power User 是自动升级的（每天检查一次）；VIP 靠捐赠，Uploader 与管理组由站点任命。管理组各级不列。
var iptorrentsLevelRequirements = []v2.SiteLevelRequirement{
	{ID: 1, Name: "Leecher", Privilege: "被 H&R 系统关禁闭的账号：不能下载种子、不能发邀请；捐赠后恢复，不捐赠 31 天后停用"},
	{ID: 2, Name: "User", Privilege: "新用户的默认等级"},
	{
		ID: 3, Name: "Power User", Interval: "P4W", Uploaded: "50GB", Downloaded: "5GB", Ratio: 1.05,
		Privilege: "可以发求种、看 Top 10、申请 Uploader；每天检查一次，分享率低于 0.95 或上传不足 50GB 时降回 User 并收回邀请",
	},
	{ID: 100, Name: "VIP", GroupType: v2.LevelGroupVIP, Privilege: "捐赠获得：不会被自动降级，VIP 期间不收 H&R 警告，种子照样要做种"},
	{ID: 200, Name: "Uploader", GroupType: v2.LevelGroupManager, Privilege: "由站点任命：可以发布种子，不会被自动降级，不收 H&R 警告"},
}

func init() {
	v2.RegisterSiteDefinition(IPTorrentsDefinition)
}

func createIPTorrentsSite(config v2.SiteConfig, logger *zap.Logger) (v2.Site, error) {
	var opts v2.IPTorrentsOptions
	if len(config.Options) > 0 {
		if err := json.Unmarshal(config.Options, &opts); err != nil {
			return nil, fmt.Errorf("parse IPTorrents options: %w", err)
		}
	}
	if strings.TrimSpace(opts.Cookie) == "" {
		return nil, errors.New("IPTorrents 站点需要配置 Cookie（登录 iptorrents.com 以后浏览器里的整串 Cookie，含 uid 与 pass）")
	}

	siteID := config.ID
	if siteID == "" {
		siteID = iptDefaultSiteID
	}
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = iptDefaultBaseURL
		if def := v2.GetDefinitionRegistry().GetOrDefault(siteID); def != nil && len(def.URLs) > 0 {
			baseURL = def.URLs[0]
		}
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	driver := newIPTDriver(iptDriverConfig{BaseURL: baseURL, Cookie: opts.Cookie, SiteID: siteID})
	return v2.NewBaseSite(driver, v2.BaseSiteConfig{
		ID:        config.ID,
		Name:      config.Name,
		Kind:      v2.SiteIPTorrents,
		RateLimit: config.RateLimit,
		RateBurst: config.RateBurst,
		Logger:    logger.With(zap.String("site", config.ID)),
	}), nil
}

const (
	// iptDefaultSiteID、iptDefaultBaseURL 是站点定义里的 ID 与地址：工厂与驱动没拿到时用它们
	iptDefaultSiteID  = "iptorrents"
	iptDefaultBaseURL = "https://iptorrents.com/"
	// iptUserAgent 是请求网页用的浏览器 User-Agent（与 NexusPHP 驱动的默认值一样）
	iptUserAgent  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	iptAcceptHTML = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	// iptElapsedLayout 是详情页上传时间 <span class="elapsedDate" title="Sunday, November 8, 2026 at 3:32pm"> 的写法（UTC）
	iptElapsedLayout = "Monday, January 2, 2006 at 3:04pm"
)

// iptorrentsPageDelay 是取用户信息时两页之间隔多久（积分页、资料页、做种页三页）；测试里调成 0。
var iptorrentsPageDelay = 500 * time.Millisecond

var (
	iptNumericRe      = regexp.MustCompile(`^\d+$`)
	iptTorrentPathRe  = regexp.MustCompile(`^/t/(\d+)(?:[/?#]|$)`)
	iptDownloadIDRe   = regexp.MustCompile(`(?:^|/)download\.php/(\d+)(?:/|$)`)
	iptUserLinkRe     = regexp.MustCompile(`(?:^|/)(?:u/|user\.php\?u=)(\d+)`)
	iptSizeRe         = regexp.MustCompile(`(?i)(\d[\d,]*(?:\.\d+)?)\s*([KMGTP]?i?B)\b`)
	iptIntRe          = regexp.MustCompile(`\d[\d,]*`)
	iptFloatRe        = regexp.MustCompile(`\d[\d,]*(?:\.\d+)?`)
	iptAgoRe          = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(second|sec|minute|min|hour|day|week|month|year)s?\s+ago`)
	errIPTNoDownloads = errors.New("详情页上没有下载链接")
)

// iptRequest 是一次网页请求：Path 以 / 开头，Params 是查询参数。
type iptRequest struct {
	Path   string
	Params url.Values
	// Detail 为真时 404 按「种子不存在」报（种子详情页、按编号下载的第一步）
	Detail bool
}

type iptResponse struct {
	StatusCode int
	Body       []byte
	Document   *goquery.Document
}

type iptDriver struct {
	baseURL    string
	cookie     string
	siteID     string
	httpClient *v2.SiteHTTPClient
	// now 是换算列表上「3.2 minutes ago」这类相对时间用的当前时间；测试里固定
	now func() time.Time
}

type iptDriverConfig struct {
	BaseURL string
	Cookie  string
	// SiteID 是站点在 pt-tools 里注册的 ID：搜索结果的代理下载地址按它找站点，全局设置的「站点代理」按它判定
	SiteID     string
	HTTPClient *v2.SiteHTTPClient
	Now        func() time.Time
}

func newIPTDriver(cfg iptDriverConfig) *iptDriver {
	client := cfg.HTTPClient
	if client == nil {
		client = v2.NewSiteHTTPClient(v2.SiteHTTPClientConfig{
			Timeout:   30 * time.Second,
			UserAgent: iptUserAgent,
			SiteID:    cfg.SiteID,
		})
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	siteID := cfg.SiteID
	if siteID == "" {
		siteID = iptDefaultSiteID
	}
	return &iptDriver{
		baseURL:    strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		cookie:     strings.TrimSpace(cfg.Cookie),
		siteID:     siteID,
		httpClient: client,
		now:        now,
	}
}

// fetch 带着 Cookie 取一个地址（rawURL 是完整地址）。
func (d *iptDriver) fetch(ctx context.Context, rawURL, accept string) (*v2.HTTPResponse, error) {
	return d.httpClient.DoRequest(ctx, http.MethodGet, rawURL, nil, map[string]string{
		"Cookie":          d.cookie,
		"Accept":          accept,
		"Accept-Language": "en-US,en;q=0.9",
	})
}

// Execute 取一页网页。登录页（Cookie 失效、没填对）报 ErrSessionExpired，Cloudflare 质询、401/403、429 与别的 HTTP 错误
// 各报各的；req.Detail 时 404 报 ErrTorrentNotFound。
func (d *iptDriver) Execute(ctx context.Context, req iptRequest) (iptResponse, error) {
	fullURL := d.baseURL + req.Path
	if len(req.Params) > 0 {
		fullURL += "?" + req.Params.Encode()
	}
	resp, err := d.fetch(ctx, fullURL, iptAcceptHTML)
	if err != nil {
		return iptResponse{}, fmt.Errorf("请求 %s: %w", req.Path, err)
	}
	res := iptResponse{StatusCode: resp.StatusCode, Body: resp.Body}
	if err = iptStatusErr(resp, req.Detail); err != nil {
		return res, err
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return res, fmt.Errorf("parse HTML: %w", err)
	}
	res.Document = doc
	if iptIsLoginPage(doc) {
		return res, iptWithTitle(v2.ErrSessionExpired, doc)
	}
	return res, nil
}

func iptStatusErr(resp *v2.HTTPResponse, detail bool) error {
	switch {
	case resp.IsCloudflareChallenge():
		return fmt.Errorf("HTTP %d: %w", resp.StatusCode, v2.ErrCloudflareChallenge)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("HTTP %d: %w", resp.StatusCode, v2.ErrInvalidCredentials)
	case resp.StatusCode == http.StatusTooManyRequests:
		return fmt.Errorf("HTTP 429: %w", v2.ErrRateLimited)
	case detail && resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("HTTP 404: %w", v2.ErrTorrentNotFound)
	case resp.StatusCode != http.StatusOK:
		return &v2.HTTPStatusError{Code: resp.StatusCode}
	}
	return nil
}

// iptSignedIn 报告页面是不是登录以后的：页头有退出登录的表单（/lout.php，Jackett 与 Prowlarr 也按它判断）。
func iptSignedIn(doc *goquery.Document) bool {
	return doc.Find("form[action*='lout.php'], a[href*='lout.php']").Length() > 0
}

// iptIsLoginPage 报告页面是不是登录页（/login.php，标题「iPT – Sign In」，表单提交到 do-login.php）。
func iptIsLoginPage(doc *goquery.Document) bool {
	if doc.Find("form[action*='do-login']").Length() > 0 {
		return true
	}
	return doc.Find("input[type='password']").Length() > 0 && !iptSignedIn(doc)
}

// iptIsLoginBody 是 iptIsLoginPage 的字节版：下载种子时站点回的是不是登录页。
func iptIsLoginBody(body []byte) bool {
	return bytes.Contains(body, []byte("do-login")) && !bytes.Contains(body, []byte("lout.php"))
}

// iptWithTitle 在 err 后面写上页面标题，看得出站点回的是哪一页；errors.Is 照样认 err。
func iptWithTitle(err error, doc *goquery.Document) error {
	if doc == nil {
		return err
	}
	title := []rune(strings.Join(strings.Fields(doc.Find("title").First().Text()), " "))
	if len(title) == 0 {
		return err
	}
	if len(title) > 60 {
		title = append(title[:60], '…')
	}
	return fmt.Errorf("%w（页面标题：%s）", err, string(title))
}

// PrepareSearch 拼 /t 的查询：关键词 q、页码 p（从 1 起）、只看免费 free，分类编号直接写成参数名。
func (d *iptDriver) PrepareSearch(query v2.SearchQuery) (iptRequest, error) {
	params := url.Values{}
	if kw := strings.TrimSpace(query.Keyword); kw != "" {
		params.Set("q", kw)
	}
	if query.FreeOnly {
		params.Set("free", "")
	}
	for _, c := range strings.Split(query.Category, ",") {
		if c = strings.TrimSpace(c); iptNumericRe.MatchString(c) {
			params.Set(c, "")
		}
	}
	if query.Page > 1 {
		params.Set("p", strconv.Itoa(query.Page))
	}
	return iptRequest{Path: "/t", Params: params}, nil
}

// iptColumns 是种子列表里体积、完成数、做种、下载这几列的位置：按表头的提示文字（「Sort by size」这类）找，
// 站点设置里打开了文件数一列时位置会往后挪；表头认不出时用默认的九列布局。
type iptColumns struct {
	size, snatched, seeders, leechers int
}

func iptFindColumns(table *goquery.Selection) iptColumns {
	cols := iptColumns{size: 5, snatched: 6, seeders: 7, leechers: 8}
	table.Find("thead tr").First().Find("th").Each(func(i int, th *goquery.Selection) {
		label := strings.ToLower(th.Text() + " " + th.Find("i[alt]").AttrOr("alt", ""))
		switch {
		case strings.Contains(label, "sort by size"):
			cols.size = i
		case strings.Contains(label, "sort by snatches"):
			cols.snatched = i
		case strings.Contains(label, "sort by seeders"):
			cols.seeders = i
		case strings.Contains(label, "sort by leechers"):
			cols.leechers = i
		}
	})
	return cols
}

// ParseSearch 读种子列表。不是种子列表的页面报 ErrUnexpectedPage；列表上有行却一行也认不出（站点改了页面）报 ErrParseError，
// 不当成没有结果。
func (d *iptDriver) ParseSearch(res iptResponse) ([]v2.TorrentItem, error) {
	doc := res.Document
	if doc == nil {
		return nil, v2.ErrParseError
	}
	table := doc.Find("table#torrents").First()
	if table.Length() == 0 {
		return nil, iptWithTitle(v2.ErrUnexpectedPage, doc)
	}
	cols := iptFindColumns(table)
	now := d.now()
	rows := table.Find("tr").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return s.Find("td.al").Length() > 0
	})
	items := make([]v2.TorrentItem, 0, rows.Length())
	rows.Each(func(_ int, row *goquery.Selection) {
		if item, ok := d.parseRow(row, cols, now); ok {
			items = append(items, item)
		}
	})
	if rows.Length() > 0 && len(items) == 0 {
		return nil, fmt.Errorf("%w: 种子列表有 %d 行，一行也没认出来", v2.ErrParseError, rows.Length())
	}
	return items, nil
}

// parseRow 读列表上的一行：类型图标、名称（a.hv，下面的 div.sub 是「标签 | 3.2 minutes ago」）、收藏、下载、评论、体积、
// 完成数、做种、下载。免费是名称后面的 <span class="free">FreeLeech</span>，站点不写免费的结束时间。
func (d *iptDriver) parseRow(row *goquery.Selection, cols iptColumns, now time.Time) (v2.TorrentItem, bool) {
	cells := row.ChildrenFiltered("td")
	name := row.Find("td.al").First()
	link := name.Find("a.hv[href]").First()
	if link.Length() == 0 {
		link = name.Find("a[href^='/t/']").First()
	}
	m := iptTorrentPathRe.FindStringSubmatch(link.AttrOr("href", ""))
	title := iptText(link)
	if m == nil || title == "" {
		return v2.TorrentItem{}, false
	}
	id := m[1]
	tags, ago := iptSplitSub(iptText(name.Find("div.sub").First()))
	item := v2.TorrentItem{
		ID:            id,
		URL:           d.detailURL(id),
		Title:         title,
		Subtitle:      tags,
		SizeBytes:     iptParseSize(iptCellText(cells, cols.size)),
		Snatched:      iptParseInt(iptCellText(cells, cols.snatched)),
		Seeders:       iptParseInt(iptCellText(cells, cols.seeders)),
		Leechers:      iptParseInt(iptCellText(cells, cols.leechers)),
		UploadedAt:    iptParseAgo(ago, now),
		DiscountLevel: v2.DiscountNone,
		// 整站 H&R：所有种子都要做种到 1:1 或 336 小时，列表上不单独标
		HasHR:       true,
		Category:    strings.TrimSpace(cells.First().Find("img[alt]").AttrOr("alt", "")),
		DownloadURL: d.proxyDownloadURL(id),
		SourceSite:  d.siteID,
	}
	if name.Find("span.free").Length() > 0 {
		item.DiscountLevel = v2.DiscountFree
	}
	return item, true
}

// iptSplitSub 把名称下面那一行拆成标签（评分、年份、类型、分辨率）和发布时间（「3.2 minutes ago」，后面可能还有「by 发布者」）；
// 没有标签时整行都是时间。
func iptSplitSub(sub string) (tags, ago string) {
	if i := strings.LastIndex(sub, "|"); i >= 0 {
		return strings.TrimSpace(sub[:i]), strings.TrimSpace(sub[i+1:])
	}
	return "", strings.TrimSpace(sub)
}

// iptParseAgo 把「1.8 weeks ago」这类相对时间按 now 换成 Unix 秒；月按 30 天、年按 365 天算（站点只写到一位小数，本来就是约数）。
// 认不出时为 0（发布时间未知）。
func iptParseAgo(s string, now time.Time) int64 {
	m := iptAgoRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	var unit time.Duration
	switch strings.ToLower(m[2]) {
	case "second", "sec":
		unit = time.Second
	case "minute", "min":
		unit = time.Minute
	case "hour":
		unit = time.Hour
	case "day":
		unit = 24 * time.Hour
	case "week":
		unit = 7 * 24 * time.Hour
	case "month":
		unit = 30 * 24 * time.Hour
	default: // year
		unit = 365 * 24 * time.Hour
	}
	return now.Add(-time.Duration(v * float64(unit))).Unix()
}

// iptText 是元素的文字（去掉悬停提示 .tTip、合并空白）。
func iptText(sel *goquery.Selection) string {
	if sel == nil || sel.Length() == 0 {
		return ""
	}
	c := sel.Clone()
	c.Find(".tTip").Remove()
	return strings.Join(strings.Fields(c.Text()), " ")
}

func iptCellText(cells *goquery.Selection, i int) string {
	if i < 0 || i >= cells.Length() {
		return ""
	}
	return iptText(cells.Eq(i))
}

// iptParseSize 取文字里第一个体积（「2.95 GB」「1,099,511,627,776 B」「0.00 kB」），按 1024 进位换成字节：
// 站点就是这样算的（资料页精确到字节的上传量与显示的 GB 数按 1024 进位对得上）。认不出时为 0。
func iptParseSize(s string) int64 {
	m := iptSizeRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	if err != nil {
		return 0
	}
	mult := float64(1)
	switch strings.ToUpper(m[2][:1]) {
	case "K":
		mult = 1 << 10
	case "M":
		mult = 1 << 20
	case "G":
		mult = 1 << 30
	case "T":
		mult = 1 << 40
	case "P":
		mult = 1 << 50
	}
	return int64(v * mult)
}

// iptParseInt 取文字里第一个整数（可以带千分位逗号）；没有时为 0。
func iptParseInt(s string) int {
	m := iptIntRe.FindString(s)
	if m == "" {
		return 0
	}
	n, err := strconv.Atoi(strings.ReplaceAll(m, ",", ""))
	if err != nil {
		return 0
	}
	return n
}

// iptParseFloat 取文字里第一个数（可以带千分位逗号，前面的正负号不算）；没有时 ok 为假。
func iptParseFloat(s string) (float64, bool) {
	m := iptFloatRe.FindString(s)
	if m == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(m, ",", ""), 64)
	return v, err == nil
}

func (d *iptDriver) detailURL(id string) string { return d.baseURL + "/t/" + id }

// proxyDownloadURL 是搜索结果的下载地址：走 pt-tools 的代理，由服务端带着 Cookie 去下（见 Download）。
func (d *iptDriver) proxyDownloadURL(id string) string {
	return fmt.Sprintf("/api/site/%s/torrent/%s/download", d.siteID, id)
}

// GetUserInfo 依次取积分页（页头有自己的用户编号与用户名）、资料页、做种页。资料页上有站点记的最近访问时间（Last seen），
// 登录探测按它判断；做种页一时没取到时（质询、网络错误、站点出错）回取到的部分，做种体积与未达标 H&R 数沿用上一次的。
func (d *iptDriver) GetUserInfo(ctx context.Context) (v2.UserInfo, error) {
	bonus, err := d.Execute(ctx, iptRequest{Path: "/mybonus.php"})
	if err != nil {
		return v2.UserInfo{}, fmt.Errorf("打开积分页: %w", err)
	}
	userID, username := iptHeaderUser(bonus.Document)
	if userID == "" {
		return v2.UserInfo{}, iptWithTitle(fmt.Errorf("%w: 页头没有找到自己的用户链接", v2.ErrParseError), bonus.Document)
	}
	info := v2.UserInfo{Site: d.siteID, UserID: userID, Username: username}
	iptApplyBonus(&info, bonus.Document)

	if err = iptPause(ctx); err != nil {
		return v2.UserInfo{}, err
	}
	profile, err := d.Execute(ctx, iptRequest{Path: "/user.php", Params: url.Values{"u": {userID}}})
	if err != nil {
		return v2.UserInfo{}, fmt.Errorf("打开资料页: %w", err)
	}
	if err = iptApplyProfile(&info, profile.Document); err != nil {
		return v2.UserInfo{}, err
	}

	partial := func(err error) (v2.UserInfo, error) {
		return info, &v2.PartialUserInfoError{Missing: []string{"seederSize", "hnrUnsatisfied"}, Err: err}
	}
	if err = iptPause(ctx); err != nil {
		return partial(err)
	}
	peers, err := d.Execute(ctx, iptRequest{Path: "/peers", Params: url.Values{"u": {userID}}})
	if err != nil {
		err = fmt.Errorf("打开做种页: %w", err)
		if errors.Is(err, v2.ErrSessionExpired) || errors.Is(err, v2.ErrInvalidCredentials) {
			return v2.UserInfo{}, err
		}
		return partial(err)
	}
	if !iptApplyPeers(&info, peers.Document) {
		return partial(iptWithTitle(fmt.Errorf("%w: 做种页上没有认出做种统计", v2.ErrParseError), peers.Document))
	}
	return info, nil
}

func iptPause(ctx context.Context) error {
	if iptorrentsPageDelay <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(iptorrentsPageDelay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// iptHeaderUser 读页头自己的用户链接：<a class="tTipWrap uname" href="/u/{id}"><div class="tTip">Profile</div>用户名</a>。
func iptHeaderUser(doc *goquery.Document) (id, name string) {
	a := doc.Find(".stats a.uname[href]").First()
	if a.Length() == 0 {
		a = doc.Find("a.uname[href]").First()
	}
	m := iptUserLinkRe.FindStringSubmatch(a.AttrOr("href", ""))
	if m == nil {
		return "", ""
	}
	return m[1], iptText(a)
}

// iptApplyBonus 读积分页：余额（.bx-balance__n，没有时用页头的 Bonus Points）与「Earning now」的每小时积分（+3.0 an hour）。
func iptApplyBonus(info *v2.UserInfo, doc *goquery.Document) {
	if v, ok := iptParseFloat(iptText(doc.Find(".bx-balance__n").First())); ok {
		info.Bonus = v
	} else if v, ok := iptParseFloat(iptText(doc.Find(".stats a[href='/mybonus.php']").First())); ok {
		info.Bonus = v
	}
	doc.Find(".bx-summary .ipx-stat").Each(func(_ int, s *goquery.Selection) {
		if !strings.EqualFold(iptText(s.Find(".ipx-stat__label").First()), "Earning now") {
			return
		}
		if v, ok := iptParseFloat(iptText(s.Find(".ipx-stat__value").First())); ok {
			info.BonusPerHour = v
		}
	})
}

// iptApplyProfile 读资料页（div#upf）：用户名、等级、加入时间与最近访问（<time datetime>，UTC）、上传下载（精确到字节的
// .ipx-stat__sub）、分享率、正在做种与下载的数目、发布数。
func iptApplyProfile(info *v2.UserInfo, doc *goquery.Document) error {
	upf := doc.Find("#upf, div.upf").First()
	if upf.Length() == 0 || upf.Find(".upf-stats").Length() == 0 {
		return iptWithTitle(fmt.Errorf("%w: 资料页上没有认出用户资料", v2.ErrParseError), doc)
	}
	if name := iptText(upf.Find("h1.upf-name").First()); name != "" {
		info.Username = name
	}
	if level := iptText(upf.Find(".upf-class").First()); level != "" {
		info.LevelName, info.Rank = level, level
	}
	upf.Find("p.upf-meta > span").Each(func(_ int, s *goquery.Selection) {
		label := strings.ToLower(s.Text())
		ts := iptParseRFC3339(s.Find("time[datetime]").First().AttrOr("datetime", ""))
		switch {
		case strings.Contains(label, "joined"):
			info.JoinDate = ts
		case strings.Contains(label, "last seen"):
			info.LastAccess = ts
		}
	})
	info.Uploaded = iptStatBytes(upf, "up")
	info.Downloaded = iptStatBytes(upf, "down")
	info.Ratio = iptRatio(iptText(upf.Find(".upf-stat--ratio .ipx-stat__value").First()), info.Uploaded, info.Downloaded)
	upf.Find("dd.upf-peers a").Each(func(_ int, s *goquery.Selection) {
		text := strings.ToLower(iptText(s))
		n := iptParseInt(text)
		switch {
		case strings.Contains(text, "seeding"):
			info.Seeding, info.SeederCount = n, n
		case strings.Contains(text, "leeching"):
			info.Leeching, info.LeecherCount = n, n
		}
	})
	info.Uploads = iptParseInt(iptText(upf.Find("a[href^='/t?u='] b").First()))
	return nil
}

func iptParseRFC3339(s string) int64 {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return t.Unix()
}

// iptStatBytes 读资料页的上传（up）或下载（down）：先读精确到字节的那一行（1,099,511,627,776 B），没有时读显示的值（1.00 TB）。
func iptStatBytes(upf *goquery.Selection, kind string) int64 {
	stat := upf.Find(".upf-stat--" + kind).First()
	if sub := iptText(stat.Find(".ipx-stat__sub").First()); sub != "" {
		return iptParseSize(sub)
	}
	return iptParseSize(iptText(stat.Find(".ipx-stat__value").First()))
}

// iptRatio 读分享率：没下载过时页面写「—」（FAQ 里也有写「Inf.」的），有上传就记成无穷（v2.InfiniteRatio）。
func iptRatio(text string, uploaded, downloaded int64) float64 {
	if v, ok := iptParseFloat(text); ok {
		return v
	}
	switch {
	case downloaded > 0:
		return float64(uploaded) / float64(downloaded)
	case uploaded > 0:
		return v2.InfiniteRatio
	}
	return 0
}

// iptApplyPeers 读做种页顶上的统计（dl.pr-stats）：Seeding 的数目与体积、Leeching 的数目、To seed（还没做种到 1:1 或
// 14 天的种子数，记作未达标的 H&R）。没有这块统计时返回 false。
func iptApplyPeers(info *v2.UserInfo, doc *goquery.Document) bool {
	stats := doc.Find("dl.pr-stats > div")
	if stats.Length() == 0 {
		return false
	}
	stats.Each(func(_ int, s *goquery.Selection) {
		value := iptText(s.Find("dd").Not(".pr-stats__sub").First())
		switch strings.ToLower(iptText(s.Find("dt").First())) {
		case "seeding":
			n := iptParseInt(value)
			info.Seeding, info.SeederCount = n, n
			info.SeederSize = iptParseSize(iptText(s.Find("dd.pr-stats__sub").First()))
		case "leeching":
			n := iptParseInt(value)
			info.Leeching, info.LeecherCount = n, n
		case "to seed":
			info.HnRUnsatisfied = iptParseInt(value)
		}
	})
	return true
}

// iptTorrentID 检查种子编号：IPTorrents 的是数字。
func iptTorrentID(torrentID string) (string, error) {
	id := strings.TrimSpace(torrentID)
	if !iptNumericRe.MatchString(id) {
		return "", fmt.Errorf("%w: %q", v2.ErrInvalidTorrentID, torrentID)
	}
	return id, nil
}

// PrepareDownload 是按编号下载的第一步：打开详情页（/t/{id}），下载链接在详情页上（见 ParseDownloadContext）。
func (d *iptDriver) PrepareDownload(torrentID string) (iptRequest, error) {
	id, err := iptTorrentID(torrentID)
	if err != nil {
		return iptRequest{}, err
	}
	return iptRequest{Path: "/t/" + id, Detail: true}, nil
}

func (d *iptDriver) ParseDownload(res iptResponse) ([]byte, error) {
	return d.ParseDownloadContext(context.Background(), res)
}

// ParseDownloadContext 从详情页取下载链接（div.info 里的 download.php/{id}/{文件名}.torrent，相对站点根目录），
// 带着 Cookie 下载。下回来的不是种子文件时报错：登录页报 ErrSessionExpired，别的页面见 v2.CheckTorrentFile。
func (d *iptDriver) ParseDownloadContext(ctx context.Context, res iptResponse) ([]byte, error) {
	if res.Document == nil {
		return nil, v2.ErrParseError
	}
	href := strings.TrimSpace(res.Document.Find("div.dBox div.info a[href*='download.php']").First().AttrOr("href", ""))
	if href == "" {
		return nil, iptWithTitle(errIPTNoDownloads, res.Document)
	}
	target, err := d.siteURL(href)
	if err != nil {
		return nil, err
	}
	resp, err := d.fetch(ctx, target, "application/x-bittorrent,*/*")
	if err != nil {
		return nil, fmt.Errorf("下载种子文件: %w", err)
	}
	if err := iptStatusErr(resp, true); err != nil {
		return nil, fmt.Errorf("下载种子文件: %w", err)
	}
	// 种子文件是 bencode 的字典（以 d 开头），里面的文件名什么字都可能有；不是种子文件时才看是不是登录页
	if (len(resp.Body) == 0 || resp.Body[0] != 'd') && iptIsLoginBody(resp.Body) {
		return nil, fmt.Errorf("%w: 站点回了登录页: %w", v2.ErrNotTorrentFile, v2.ErrSessionExpired)
	}
	if err := v2.CheckTorrentFile(resp.Body); err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// Download 按种子编号下载（搜索结果、RSS、辅种这些入口都只给编号）：先打开详情页取下载链接，再下载种子文件。
// BaseSite 下载时交给它，一时的错误（网络、5xx）由 BaseSite 按退避重试整个过程。
func (d *iptDriver) Download(ctx context.Context, torrentID string) ([]byte, error) {
	req, err := d.PrepareDownload(torrentID)
	if err != nil {
		return nil, err
	}
	res, err := d.Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("打开种子详情页: %w", err)
	}
	return d.ParseDownloadContext(ctx, res)
}

// siteURL 把详情页里的链接补成站点上的完整地址。不在站点域名上的链接不去请求（请求要带 Cookie）。
func (d *iptDriver) siteURL(href string) (string, error) {
	base, err := url.Parse(d.baseURL + "/")
	if err != nil {
		return "", fmt.Errorf("站点地址不对: %w", err)
	}
	ref, err := url.Parse(href)
	if err != nil {
		return "", fmt.Errorf("下载链接不对: %w", err)
	}
	u := base.ResolveReference(ref)
	if !strings.EqualFold(u.Hostname(), base.Hostname()) {
		return "", fmt.Errorf("下载链接不在站点的域名上（%s），不带 Cookie 去请求", u.Hostname())
	}
	return u.String(), nil
}

// iptTorrentIDFromLink 从 RSS 条目或别的入口给的链接里取种子编号：/t/{id}、/download.php/{id}/…、
// torrent.php?id= 与 details.php?id=（推送前重新验证资格时拼的）。认不出时为空串。
func iptTorrentIDFromLink(link string) string {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || link == "" {
		return ""
	}
	path := u.EscapedPath()
	if m := iptTorrentPathRe.FindStringSubmatch(path); m != nil {
		return m[1]
	}
	if m := iptDownloadIDRe.FindStringSubmatch(path); m != nil {
		return m[1]
	}
	if strings.HasSuffix(path, "torrent.php") || strings.HasSuffix(path, "details.php") {
		if id := u.Query().Get("id"); iptNumericRe.MatchString(id) {
			return id
		}
	}
	return ""
}

// GetTorrentDetail 读一个种子的详情页（RSS 取详情、推送前核对免费）：标题、体积、做种与下载人数、上传时间、免费、分类、IMDb 编号。
// 免费没有结束时间，按免费、结束时间未知处理（见 TorrentItem.Promotion）。种子不在了报 ErrTorrentNotFound。
func (d *iptDriver) GetTorrentDetail(ctx context.Context, guid, link, _ string) (*v2.TorrentItem, error) {
	id := iptTorrentIDFromLink(link)
	if id == "" {
		id = iptTorrentIDFromLink(guid)
	}
	if id == "" && iptNumericRe.MatchString(strings.TrimSpace(guid)) {
		id = strings.TrimSpace(guid)
	}
	if id == "" {
		// 错误里不带链接：RSS 链接可能带着密钥
		return nil, fmt.Errorf("%w: %w: 认不出种子编号", v2.ErrTorrentDetailUnavailable, v2.ErrInvalidTorrentID)
	}
	res, err := d.Execute(ctx, iptRequest{Path: "/t/" + id, Detail: true})
	if err != nil {
		if errors.Is(err, v2.ErrTorrentNotFound) {
			return nil, fmt.Errorf("IPTorrents 详情返回 HTTP 404: %w: %w", v2.ErrTorrentDetailUnavailable, v2.ErrTorrentNotFound)
		}
		return nil, fmt.Errorf("取种子详情: %w", err)
	}
	item, err := d.parseDetail(res.Document, id)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", v2.ErrTorrentDetailUnavailable, err)
	}
	item.FetchedAt = d.now().UTC()
	return item, nil
}

// parseDetail 读详情页的 div.dBox：h2 是标题；div.info 里有下载链接、做种/下载人数（a.peer）、「Size: 19.03 GB in 2 files」、
// 「Uploaded: <span class="elapsedDate" title="…">」；div.tags 里免费是 <span class="tz-free">Freeleech</span>，分类是 a.tz-cat；
// IMDb 链接在 #media。
func (d *iptDriver) parseDetail(doc *goquery.Document, id string) (*v2.TorrentItem, error) {
	if doc == nil {
		return nil, v2.ErrParseError
	}
	box := doc.Find("div.dBox").First()
	title := iptText(box.Find("h2").First())
	if box.Length() == 0 || title == "" {
		return nil, iptWithTitle(fmt.Errorf("%w: 页面上没有种子详情", v2.ErrParseError), doc)
	}
	info := box.Find("div.info").First()
	if m := iptDownloadIDRe.FindStringSubmatch(info.Find("a[href*='download.php']").First().AttrOr("href", "")); m != nil {
		id = m[1]
	}
	item := &v2.TorrentItem{
		ID:            id,
		URL:           d.detailURL(id),
		Title:         title,
		DiscountLevel: v2.DiscountNone,
		HasHR:         true,
		DownloadURL:   d.proxyDownloadURL(id),
		SourceSite:    d.siteID,
	}
	info.Find("div.sub").Each(func(_ int, s *goquery.Selection) {
		if text := iptText(s); strings.HasPrefix(strings.ToLower(text), "size:") {
			item.SizeBytes = iptParseSize(text)
		}
	})
	peers := info.Find("a.peer span")
	item.Seeders = iptParseInt(iptText(peers.Eq(0)))
	item.Leechers = iptParseInt(iptText(peers.Eq(1)))
	uploaded := strings.TrimSpace(info.Find("span.elapsedDate[title]").First().AttrOr("title", ""))
	if t, err := time.ParseInLocation(iptElapsedLayout, uploaded, time.UTC); err == nil {
		item.UploadedAt = t.Unix()
	}
	if box.Find("div.tags .tz-free").Length() > 0 {
		item.DiscountLevel = v2.DiscountFree
	}
	item.Category = iptText(box.Find("div.tags a.tz-cat").First())
	item.IMDbID = v2.NormalizeIMDbID(doc.Find("#media a[href*='imdb.com/title/']").First().AttrOr("href", ""))
	return item, nil
}
