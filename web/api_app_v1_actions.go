package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// App API v1 的搜索、推送与签到。

// ---- 搜索 ----

// AppSearchRequest 是 POST /search：多站搜索。Sites 为空时搜所有启用的站点。
type AppSearchRequest struct {
	Keyword    string   `json:"keyword"`
	Sites      []string `json:"sites"`
	Category   string   `json:"category"`
	FreeOnly   bool     `json:"free_only"`
	MinSeeders int      `json:"min_seeders"`
}

// AppSearchItem 是一个搜索结果：只有站点与种子编号，不含详情、下载与磁力链接；推送用 POST /push。
type AppSearchItem struct {
	Site          string   `json:"site"`
	TorrentID     string   `json:"torrent_id"`
	Title         string   `json:"title"`
	Subtitle      string   `json:"subtitle,omitempty"`
	InfoHash      string   `json:"info_hash,omitempty"`
	Size          int64    `json:"size"`
	Seeders       int      `json:"seeders"`
	Leechers      int      `json:"leechers"`
	Snatched      int      `json:"snatched,omitempty"`
	UploadedAt    int64    `json:"uploaded_at,omitempty"`
	Category      string   `json:"category,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Discount      string   `json:"discount,omitempty"`
	DiscountEndAt int64    `json:"discount_end_at,omitempty"`
	Free          bool     `json:"free"`
	HasHR         bool     `json:"has_hr"`
	IMDbID        string   `json:"imdb_id,omitempty"`
	DoubanID      string   `json:"douban_id,omitempty"`
}

// AppSearchError 是一个站点的搜索错误。
type AppSearchError struct {
	Site  string `json:"site"`
	Error string `json:"error"`
}

// AppSearchResult 是 POST /search 的回应。
type AppSearchResult struct {
	Items      []AppSearchItem  `json:"items"`
	Total      int              `json:"total"`
	Sites      map[string]int   `json:"sites"`
	Errors     []AppSearchError `json:"errors"`
	DurationMs int64            `json:"duration_ms"`
}

const (
	appSearchTimeout = 30 * time.Second
	appMaxSites      = 100
)

func (s *Server) appSearch(w http.ResponseWriter, r *http.Request) {
	var req AppSearchRequest
	if !appDecode(w, r, &req) {
		return
	}
	req.Keyword = strings.TrimSpace(req.Keyword)
	if req.Keyword == "" || len([]rune(req.Keyword)) > 100 {
		appError(w, http.StatusBadRequest, "invalid_argument", "keyword 要填，最多 100 个字")
		return
	}
	if len(req.Sites) > appMaxSites || req.MinSeeders < 0 || len(req.Category) > 64 {
		appError(w, http.StatusBadRequest, "invalid_argument", "sites 最多 100 个，min_seeders 不能是负数，category 最多 64 个字符")
		return
	}
	orch := GetSearchOrchestrator()
	if orch == nil {
		appError(w, http.StatusServiceUnavailable, "unavailable", "搜索服务没有启动")
		return
	}
	enabled := s.getEnabledSiteIDs()
	sites := filterEnabledSites(req.Sites, enabled)
	out := AppSearchResult{Items: []AppSearchItem{}, Sites: map[string]int{}, Errors: []AppSearchError{}}
	// 指定的站点都没有启用：不搜（空的站点列表会被当成搜所有站点）
	if len(req.Sites) > 0 && len(sites) == 0 {
		appJSON(w, out)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), appSearchTimeout)
	defer cancel()
	res, err := orch.Search(ctx, v2.MultiSiteSearchQuery{
		SearchQuery: v2.SearchQuery{Keyword: req.Keyword, Category: req.Category, FreeOnly: req.FreeOnly},
		Sites:       sites, Timeout: appSearchTimeout, MinSeeders: req.MinSeeders,
	})
	if err != nil {
		appError(w, http.StatusBadGateway, "search_failed", appRedact(err.Error()))
		return
	}
	for _, it := range res.Items {
		rememberHit(it)
		item := AppSearchItem{
			Site: it.SourceSite, TorrentID: it.ID, Title: it.Title, Subtitle: it.Subtitle, InfoHash: it.InfoHash, Size: it.SizeBytes,
			Seeders: it.Seeders, Leechers: it.Leechers, Snatched: it.Snatched, UploadedAt: it.UploadedAt, Category: it.Category, Tags: it.Tags,
			Discount: string(it.DiscountLevel), Free: it.IsFree(), HasHR: it.HasHR, IMDbID: it.IMDbID, DoubanID: it.DoubanID,
		}
		if !it.DiscountEndTime.IsZero() {
			item.DiscountEndAt = it.DiscountEndTime.Unix()
		}
		out.Items = append(out.Items, item)
	}
	out.Total = res.TotalResults
	for site, n := range res.SiteResults {
		out.Sites[site] = n
	}
	for _, e := range res.Errors {
		out.Errors = append(out.Errors, AppSearchError{Site: e.Site, Error: appRedact(e.Error)})
	}
	out.DurationMs = res.Duration.Milliseconds()
	appJSON(w, out)
}

// appHits 记下 App 搜索到的种子（只在内存里，1 小时过期，最多 2000 条），推送时按站点与种子编号找回：
//   - downhash：HDDolby 这类站点的下载要带它，搜索结果不把下载地址交给 App；
//   - 体积、H&R、免费与免费到期：推送时一并写进种子记录，H&R 保护与免费到期清理要用。
type appHits struct {
	mu sync.Mutex
	m  map[string]appHit
}

type appHit struct {
	downhash string
	size     int64
	hasHR    bool
	free     bool
	level    string
	freeEnd  time.Time
	at       time.Time
}

const (
	appHitTTL = time.Hour
	appHitMax = 2000
)

var appSearchHits = &appHits{m: map[string]appHit{}}

func appHitKey(site, id string) string { return strings.ToLower(site) + "|" + id }

func rememberHit(it v2.TorrentItem) {
	h := appHit{
		size: it.SizeBytes, hasHR: it.HasHR, free: it.IsFree(), level: string(it.DiscountLevel), freeEnd: it.DiscountEndTime,
	}
	if it.DownloadURL != "" {
		if u, err := url.Parse(it.DownloadURL); err == nil {
			h.downhash = u.Query().Get("downhash")
		}
	}
	appSearchHits.mu.Lock()
	defer appSearchHits.mu.Unlock()
	now := time.Now()
	if len(appSearchHits.m) >= appHitMax {
		for k, e := range appSearchHits.m {
			if now.Sub(e.at) > appHitTTL {
				delete(appSearchHits.m, k)
			}
		}
		if len(appSearchHits.m) >= appHitMax {
			appSearchHits.m = map[string]appHit{}
		}
	}
	h.at = now
	appSearchHits.m[appHitKey(it.SourceSite, it.ID)] = h
}

func lookupHit(site, id string) (appHit, bool) {
	appSearchHits.mu.Lock()
	defer appSearchHits.mu.Unlock()
	h, ok := appSearchHits.m[appHitKey(site, id)]
	if !ok || time.Since(h.at) > appHitTTL {
		return appHit{}, false
	}
	return h, true
}

// lookupDownhash 是推送时下载要带的 downhash（没有搜索过、或者站点不用它时是空的）。
func lookupDownhash(site, id string) string {
	h, _ := lookupHit(site, id)
	return h.downhash
}

// pushMeta 是推送时写进种子记录的种子信息：按搜索结果取体积、H&R、免费与免费到期，站点定义开了 H&R 的也算 H&R。
// 没有搜索过时返回 nil，不覆盖库里已有的值（和网页上的推送一样）。
func pushMeta(site, id string, data []byte) *internal.PushTorrentMeta {
	h, ok := lookupHit(site, id)
	if !ok {
		return nil
	}
	size := h.size
	if size <= 0 {
		if n, err := qbit.ComputeTorrentSize(data); err == nil {
			size = n
		}
	}
	hasHR, hours := h.hasHR, 0
	if def, ok := v2.GetDefinitionRegistry().Get(site); ok {
		hasHR = hasHR || def.HREnabled
		if hasHR {
			hours = def.CalcHRSeedTimeH(size)
		}
	}
	meta := &internal.PushTorrentMeta{SizeBytes: size, HasHR: hasHR, HRSeedTimeH: hours, IsFree: h.free, FreeLevel: h.level}
	// 半价、2X 这类优惠也有结束时间，但那不是免费到期：写进去的话，免费到期清理会删掉没下完的种子
	if h.free && !h.freeEnd.IsZero() {
		fe := h.freeEnd.UTC()
		meta.FreeEndTime = &fe
	}
	return meta
}

// ---- 推送 ----

// AppPushRequest 是 POST /push：服务端经站点下载种子文件，再经推送闸门（磁盘空间、站点做种容量）放进下载器。
// DownloaderID 为 0 时用默认下载器。
type AppPushRequest struct {
	Site         string `json:"site"`
	TorrentID    string `json:"torrent_id"`
	DownloaderID uint   `json:"downloader_id"`
	Title        string `json:"title"`
	Category     string `json:"category"`
	Tags         string `json:"tags"`
	SavePath     string `json:"save_path"`
}

// AppPushResult 是 POST /push 的回应。Skipped 为真时下载器里已经有这个种子。
type AppPushResult struct {
	Success      bool   `json:"success"`
	Skipped      bool   `json:"skipped"`
	Message      string `json:"message,omitempty"`
	InfoHash     string `json:"info_hash,omitempty"`
	DownloaderID uint   `json:"downloader_id"`
	Downloader   string `json:"downloader"`
}

const appPushTimeout = 60 * time.Second

// appPushSource 是 App 推送的种子记录的来源；经 MCP 的 push_torrent 推的记 mcpPushSource。
const (
	appPushSource = "app_push"
	mcpPushSource = "mcp_push"
)

// appTorrentIDRe 是推送接受的种子编号：各站点的编号都是数字或短的字母数字串，有的站点驱动把它直接拼进下载地址，
// 所以不收 &、/、空格之类的字符。
var appTorrentIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

func (s *Server) appPush(w http.ResponseWriter, r *http.Request) {
	var req AppPushRequest
	if !appDecode(w, r, &req) {
		return
	}
	req.Site, req.TorrentID = strings.TrimSpace(req.Site), strings.TrimSpace(req.TorrentID)
	if req.Site == "" || len(req.Site) > 64 || !appTorrentIDRe.MatchString(req.TorrentID) {
		appError(w, http.StatusBadRequest, "invalid_argument", "site 要填；torrent_id 要填，只能是字母、数字与 ._-")
		return
	}
	if len(req.Title) > 512 || len(req.Category) > 128 || len(req.Tags) > 255 || len(req.SavePath) > 1024 {
		appError(w, http.StatusBadRequest, "invalid_argument", "title、category、tags 或 save_path 太长")
		return
	}
	if enabled := s.getEnabledSiteIDs(); enabled != nil && !enabled[req.Site] {
		appError(w, http.StatusNotFound, "not_found", "站点 "+req.Site+" 没有启用")
		return
	}
	orch := GetSearchOrchestrator()
	if orch == nil {
		appError(w, http.StatusServiceUnavailable, "unavailable", "站点服务没有启动")
		return
	}
	site := orch.GetSite(req.Site)
	if site == nil {
		appError(w, http.StatusNotFound, "not_found", "站点 "+req.Site+" 没有启用")
		return
	}
	dl, err := appDownloaderFor(req.DownloaderID)
	if err != nil {
		appError(w, http.StatusBadRequest, "invalid_argument", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), appPushTimeout)
	defer cancel()
	var data []byte
	if h := lookupDownhash(req.Site, req.TorrentID); h != "" {
		if hd, ok := site.(v2.HashDownloader); ok {
			data, err = hd.DownloadWithHash(ctx, req.TorrentID, h)
		} else {
			data, err = site.Download(ctx, req.TorrentID)
		}
	} else {
		data, err = site.Download(ctx, req.TorrentID)
	}
	if err != nil {
		appError(w, http.StatusBadGateway, "download_failed", "下载种子文件失败: "+appRedact(err.Error()))
		return
	}
	// 没带标题：用种子文件里的名字，任务列表里不留没有标题的记录
	if strings.TrimSpace(req.Title) == "" {
		if parsed, perr := v2.ParseTorrent(data); perr == nil {
			req.Title = parsed.Name
		}
	}
	source := appPushSource
	if p := middleware.PrincipalFrom(r.Context()); p != nil && p.Kind == middleware.KindMCP {
		source = mcpPushSource
	}
	res, err := internal.PushTorrentToDownloader(ctx, internal.PushTorrentRequest{
		SiteID: req.Site, TorrentID: req.TorrentID, TorrentData: data, Title: req.Title, Category: req.Category,
		Tags: req.Tags, SavePath: req.SavePath, DownloaderID: dl.ID, Source: source, Meta: pushMeta(req.Site, req.TorrentID, data),
	})
	out := AppPushResult{DownloaderID: dl.ID, Downloader: dl.Name}
	switch {
	case err != nil:
		out.Message = appRedact(err.Error())
	case res == nil || !res.Success:
		if res != nil {
			out.Message, out.InfoHash = appRedact(res.Message), res.TorrentHash
		}
	default:
		out.Success, out.Skipped, out.InfoHash, out.Message = true, res.Skipped, res.TorrentHash, appRedact(res.Message)
	}
	if !out.Success {
		appSetOutcome(r, "error:push_failed")
	}
	appJSON(w, out)
}

// appDownloaderFor 是推送用的下载器：指定的要存在并启用；没指定时用默认下载器，没有默认时用第一个启用的。
func appDownloaderFor(id uint) (models.DownloaderSetting, error) {
	var ds models.DownloaderSetting
	if global.GlobalDB == nil {
		return ds, errors.New("数据库没有初始化")
	}
	db := global.GlobalDB.DB
	if id != 0 {
		if err := db.Where("id = ? AND enabled = ?", id, true).Limit(1).Find(&ds).Error; err != nil || ds.ID == 0 {
			return ds, errors.New("下载器不存在或没有启用")
		}
		return ds, nil
	}
	if err := db.Where("enabled = ?", true).Order("is_default DESC, id").Limit(1).Find(&ds).Error; err != nil || ds.ID == 0 {
		return ds, errors.New("没有启用的下载器")
	}
	return ds, nil
}

// ---- 签到 ----

func (s *Server) appAttend(w http.ResponseWriter, r *http.Request) {
	if !appNoBody(w, r) {
		return
	}
	name := strings.TrimSpace(r.PathValue("site"))
	if global.GlobalDB == nil {
		appError(w, http.StatusServiceUnavailable, "unavailable", "数据库没有初始化")
		return
	}
	site, err := models.NewSiteRepository(global.GlobalDB.DB).GetSiteByName(name)
	if err != nil || site == nil {
		appError(w, http.StatusNotFound, "not_found", "站点 "+name+" 不存在")
		return
	}
	mon := s.attendanceMonitor()
	if mon == nil {
		appError(w, http.StatusServiceUnavailable, "unavailable", "签到服务没有启动")
		return
	}
	row, err := mon.SignNow(r.Context(), site.Name)
	if errors.Is(err, scheduler.ErrAttendanceBusy) {
		appError(w, http.StatusConflict, "busy", err.Error())
		return
	}
	if err != nil {
		appError(w, http.StatusBadGateway, "attend_failed", "签到失败: "+appRedact(err.Error()))
		return
	}
	att := buildAttendanceResponse(*site, row.Day, row)
	appJSON(w, AppSiteAttendance{
		Supported: att.Supported, Enabled: att.AttendanceEnabled, Day: att.Day, Status: att.Status,
		Message: appRedact(att.Message), LastAttemptAt: att.LastAttemptAt,
	})
}
