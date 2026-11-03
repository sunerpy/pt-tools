package web

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// SiteSearchParams represents site-specific search parameters
type SiteSearchParams struct {
	Cat        any `json:"cat,omitempty"`        // 类别
	Medium     any `json:"medium,omitempty"`     // 媒介
	Codec      any `json:"codec,omitempty"`      // 编码
	AudioCodec any `json:"audiocodec,omitempty"` // 音频编码
	Standard   any `json:"standard,omitempty"`   // 分辨率
	Team       any `json:"team,omitempty"`       // 制作组
	Source     any `json:"source,omitempty"`     // 地区（某些站点）
	Incldead   any `json:"incldead,omitempty"`   // 断种/活种
	Spstate    any `json:"spstate,omitempty"`    // 促销状态
}

// MultiSiteSearchRequest represents a multi-site search request
type MultiSiteSearchRequest struct {
	Keyword      string                      `json:"keyword"`
	Category     string                      `json:"category,omitempty"`
	FreeOnly     bool                        `json:"freeOnly,omitempty"`
	Sites        []string                    `json:"sites,omitempty"`
	MinSeeders   int                         `json:"minSeeders,omitempty"`
	MaxSizeBytes int64                       `json:"maxSizeBytes,omitempty"`
	MinSizeBytes int64                       `json:"minSizeBytes,omitempty"`
	Page         int                         `json:"page,omitempty"`
	PageSize     int                         `json:"pageSize,omitempty"`
	TimeoutSecs  int                         `json:"timeoutSecs,omitempty"`
	SortBy       string                      `json:"sortBy,omitempty"`     // 排序字段
	OrderDesc    bool                        `json:"orderDesc,omitempty"`  // 降序排列
	SiteParams   map[string]SiteSearchParams `json:"siteParams,omitempty"` // 每个站点的特定搜索参数
}

// MultiSiteSearchResponse represents a multi-site search response
type MultiSiteSearchResponse struct {
	Items        []TorrentItemResponse `json:"items"`
	TotalResults int                   `json:"totalResults"`
	SiteResults  map[string]int        `json:"siteResults"`
	Errors       []SearchErrorResponse `json:"errors,omitempty"`
	DurationMs   int64                 `json:"durationMs"`
}

// TorrentItemResponse represents a torrent item in API response
type TorrentItemResponse struct {
	ID              string   `json:"id"`
	URL             string   `json:"url,omitempty"`
	Title           string   `json:"title"`
	Subtitle        string   `json:"subtitle,omitempty"`
	InfoHash        string   `json:"infoHash,omitempty"`
	Magnet          string   `json:"magnet,omitempty"`
	SizeBytes       int64    `json:"sizeBytes"`
	Seeders         int      `json:"seeders"`
	Leechers        int      `json:"leechers"`
	Snatched        int      `json:"snatched,omitempty"`
	UploadedAt      int64    `json:"uploadedAt,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	SourceSite      string   `json:"sourceSite"`
	DiscountLevel   string   `json:"discountLevel"`
	DiscountEndTime int64    `json:"discountEndTime,omitempty"`
	HasHR           bool     `json:"hasHR,omitempty"`
	DownloadURL     string   `json:"downloadUrl,omitempty"`
	Category        string   `json:"category,omitempty"`
	IsFree          bool     `json:"isFree"`
	// Meta 是标题与副标题解析出的信息（分辨率、来源、平台、编码、HDR、制作组、中字、季集），页面按它筛选；没有标题时不带
	Meta *meta.Meta `json:"meta,omitempty"`
	// Block 是这个种子对所有条目拉黑了（关键字搜索里点的「不要」）；没拉黑时不带
	Block *SearchBlockResponse `json:"block,omitempty"`
}

// SearchBlockResponse 是拉黑的记录：ID 用来移出黑名单。
type SearchBlockResponse struct {
	ID     uint   `json:"id"`
	Reason string `json:"reason,omitempty"`
}

// SearchErrorResponse represents a search error in API response
type SearchErrorResponse struct {
	Site  string `json:"site"`
	Error string `json:"error"`
}

// searchOrchestrator is the global search orchestrator instance
var searchOrchestrator *v2.CachedSearchOrchestrator

// InitSearchOrchestrator initializes the global search orchestrator
func InitSearchOrchestrator(orchestrator *v2.CachedSearchOrchestrator) {
	searchOrchestrator = orchestrator
}

// GetSearchOrchestrator returns the global search orchestrator
func GetSearchOrchestrator() *v2.CachedSearchOrchestrator {
	return searchOrchestrator
}

// apiMultiSiteSearch handles POST /api/v2/search/multi
func (s *Server) apiMultiSiteSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if searchOrchestrator == nil {
		http.Error(w, "Search service not initialized", http.StatusServiceUnavailable)
		return
	}

	var req MultiSiteSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Keyword == "" {
		http.Error(w, "Keyword is required", http.StatusBadRequest)
		return
	}

	enabledSites := s.getEnabledSiteIDs()
	requested := req.Sites
	req.Sites = filterEnabledSites(req.Sites, enabledSites)
	// 指定了站点时只搜它们：没启用的不搜、写进 errors；一个都没启用时直接返回，
	// 不能变成搜所有站点（空的站点列表在搜索里是全部）
	skipped := skippedSites(requested, req.Sites)
	if len(requested) > 0 && len(req.Sites) == 0 {
		writeJSON(w, MultiSiteSearchResponse{Items: []TorrentItemResponse{}, SiteResults: map[string]int{}, Errors: skipped})
		return
	}

	timeout := 30 * time.Second
	if req.TimeoutSecs > 0 {
		timeout = time.Duration(req.TimeoutSecs) * time.Second
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	// Build query
	query := v2.MultiSiteSearchQuery{
		SearchQuery: v2.SearchQuery{
			Keyword:   req.Keyword,
			Category:  req.Category,
			FreeOnly:  req.FreeOnly,
			Page:      req.Page,
			PageSize:  req.PageSize,
			SortBy:    req.SortBy,
			OrderDesc: req.OrderDesc,
		},
		Sites:        req.Sites,
		Timeout:      timeout,
		MinSeeders:   req.MinSeeders,
		MaxSizeBytes: req.MaxSizeBytes,
		MinSizeBytes: req.MinSizeBytes,
	}

	// Execute search
	result, err := searchOrchestrator.Search(ctx, query)
	if err != nil {
		global.GetSlogger().Errorf("[Search] Multi-site search failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert to response
	response := toMultiSiteSearchResponse(result)
	response.Errors = append(response.Errors, skipped...)
	s.markBlocked(r.Context(), response.Items)

	global.GetSlogger().Infof("[Search] Multi-site search completed: keyword=%s, results=%d, duration=%dms",
		req.Keyword, len(response.Items), response.DurationMs)

	writeJSON(w, response)
}

// apiSearchSites handles GET /api/v2/search/sites
func (s *Server) apiSearchSites(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if searchOrchestrator == nil {
		writeJSON(w, map[string][]string{"sites": {}})
		return
	}

	registered := searchOrchestrator.ListSites()
	enabledSites := s.getEnabledSiteIDs()
	filtered := filterEnabledSites(registered, enabledSites)

	writeJSON(w, map[string][]string{"sites": filtered})
}

// apiSearchCacheClear handles POST /api/v2/search/cache/clear
func (s *Server) apiSearchCacheClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// 如果搜索服务未初始化，直接返回成功（没有缓存需要清理）
	if searchOrchestrator == nil {
		writeJSON(w, map[string]string{"status": "ok", "message": "search service not initialized"})
		return
	}

	searchOrchestrator.ClearCache()
	global.GetSlogger().Info("[Search] Cache cleared")
	writeJSON(w, map[string]string{"status": "ok"})
}

// apiSearchCacheStats handles GET /api/v2/search/cache/stats
func (s *Server) apiSearchCacheStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// 如果搜索服务未初始化，返回空统计
	if searchOrchestrator == nil {
		writeJSON(w, map[string]int{"size": 0})
		return
	}

	writeJSON(w, map[string]int{"size": searchOrchestrator.CacheSize()})
}

// Helper functions

func toMultiSiteSearchResponse(result *v2.MultiSiteSearchResult) MultiSiteSearchResponse {
	items := make([]TorrentItemResponse, len(result.Items))
	for i, item := range result.Items {
		items[i] = toTorrentItemResponse(item)
	}

	errors := make([]SearchErrorResponse, len(result.Errors))
	for i, err := range result.Errors {
		errors[i] = SearchErrorResponse{
			Site:  err.Site,
			Error: err.Error,
		}
	}

	return MultiSiteSearchResponse{
		Items:        items,
		TotalResults: result.TotalResults,
		SiteResults:  result.SiteResults,
		Errors:       errors,
		DurationMs:   result.Duration.Milliseconds(),
	}
}

func toTorrentItemResponse(item v2.TorrentItem) TorrentItemResponse {
	var discountEndTime int64
	if !item.DiscountEndTime.IsZero() {
		discountEndTime = item.DiscountEndTime.Unix()
	}
	var parsed *meta.Meta
	if item.Title != "" || item.Subtitle != "" {
		m := meta.Parse(item.Title, item.Subtitle)
		parsed = &m
	}

	return TorrentItemResponse{
		ID:              item.ID,
		URL:             item.URL,
		Title:           item.Title,
		Subtitle:        item.Subtitle,
		InfoHash:        item.InfoHash,
		Magnet:          item.Magnet,
		SizeBytes:       item.SizeBytes,
		Seeders:         item.Seeders,
		Leechers:        item.Leechers,
		Snatched:        item.Snatched,
		UploadedAt:      item.UploadedAt,
		Tags:            item.Tags,
		SourceSite:      item.SourceSite,
		DiscountLevel:   string(item.DiscountLevel),
		DiscountEndTime: discountEndTime,
		HasHR:           item.HasHR,
		DownloadURL:     item.DownloadURL,
		Category:        item.Category,
		IsFree:          item.IsFree(),
		Meta:            parsed,
	}
}

func (s *Server) getEnabledSiteIDs() map[string]bool {
	if s.store == nil {
		return nil
	}
	sites, err := s.store.ListSites()
	if err != nil {
		return nil
	}
	enabled := make(map[string]bool, len(sites))
	for sg, sc := range sites {
		if sc.Enabled != nil && *sc.Enabled {
			enabled[string(sg)] = true
		}
	}
	return enabled
}

// skippedSites 是指定了、却因为没启用而没搜的站点（每个只记一次）。
func skippedSites(requested, kept []string) []SearchErrorResponse {
	keep := make(map[string]bool, len(kept))
	for _, id := range kept {
		keep[id] = true
	}
	var out []SearchErrorResponse
	for _, id := range requested {
		if !keep[id] {
			keep[id] = true
			out = append(out, SearchErrorResponse{Site: id, Error: "站点没有启用，没有搜索"})
		}
	}
	return out
}

func filterEnabledSites(requested []string, enabled map[string]bool) []string {
	if enabled == nil {
		return requested
	}
	if len(requested) == 0 {
		result := make([]string, 0, len(enabled))
		for id := range enabled {
			result = append(result, id)
		}
		return result
	}
	filtered := make([]string, 0, len(requested))
	for _, id := range requested {
		if enabled[id] {
			filtered = append(filtered, id)
		}
	}
	return filtered
}

// markBlocked 给对所有条目拉黑了的结果带上 block；读不到黑名单时不标，只记日志（搜索结果照常返回）。
func (s *Server) markBlocked(ctx context.Context, items []TorrentItemResponse) {
	if s.subscriber == nil || len(items) == 0 {
		return
	}
	blocks, err := s.subscriber.GlobalBlocks(ctx)
	if err != nil {
		global.GetSlogger().Warnf("[Search] 读取黑名单失败: %v", err)
		return
	}
	for i := range items {
		if b, ok := blocks[subscribe.BlockKey(items[i].SourceSite, items[i].ID)]; ok {
			items[i].Block = &SearchBlockResponse{ID: b.ID, Reason: b.Reason}
		}
	}
}
