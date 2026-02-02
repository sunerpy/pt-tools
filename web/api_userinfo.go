package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// UserInfoResponse represents user info API response
type UserInfoResponse struct {
	Site       string  `json:"site"`
	Username   string  `json:"username"`
	UserID     string  `json:"userId"`
	Uploaded   int64   `json:"uploaded"`
	Downloaded int64   `json:"downloaded"`
	Ratio      float64 `json:"ratio"`
	Bonus      float64 `json:"bonus"`
	Seeding    int     `json:"seeding"`
	Leeching   int     `json:"leeching"`
	Rank       string  `json:"rank"`
	JoinDate   int64   `json:"joinDate,omitempty"`
	LastAccess int64   `json:"lastAccess,omitempty"`
	LastUpdate int64   `json:"lastUpdate"`
	// Extended fields
	LevelName           string  `json:"levelName,omitempty"`
	LevelID             int     `json:"levelId,omitempty"`
	BonusPerHour        float64 `json:"bonusPerHour,omitempty"`
	SeedingBonus        float64 `json:"seedingBonus,omitempty"`
	SeedingBonusPerHour float64 `json:"seedingBonusPerHour,omitempty"`
	UnreadMessageCount  int     `json:"unreadMessageCount,omitempty"`
	TotalMessageCount   int     `json:"totalMessageCount,omitempty"`
	SeederCount         int     `json:"seederCount,omitempty"`
	SeederSize          int64   `json:"seederSize,omitempty"`
	LeecherCount        int     `json:"leecherCount,omitempty"`
	LeecherSize         int64   `json:"leecherSize,omitempty"`
	HnRUnsatisfied      int     `json:"hnrUnsatisfied,omitempty"`
	HnRPreWarning       int     `json:"hnrPreWarning,omitempty"`
	TrueUploaded        int64   `json:"trueUploaded,omitempty"`
	TrueDownloaded      int64   `json:"trueDownloaded,omitempty"`
	Uploads             int     `json:"uploads,omitempty"`
}

// AggregatedStatsResponse represents aggregated stats API response
type AggregatedStatsResponse struct {
	TotalUploaded   int64              `json:"totalUploaded"`
	TotalDownloaded int64              `json:"totalDownloaded"`
	AverageRatio    float64            `json:"averageRatio"`
	TotalSeeding    int                `json:"totalSeeding"`
	TotalLeeching   int                `json:"totalLeeching"`
	TotalBonus      float64            `json:"totalBonus"`
	SiteCount       int                `json:"siteCount"`
	LastUpdate      int64              `json:"lastUpdate"`
	PerSiteStats    []UserInfoResponse `json:"perSiteStats"`
	// Extended aggregated fields
	TotalBonusPerHour   float64 `json:"totalBonusPerHour,omitempty"`
	TotalSeedingBonus   float64 `json:"totalSeedingBonus,omitempty"`
	TotalUnreadMessages int     `json:"totalUnreadMessages,omitempty"`
	TotalSeederSize     int64   `json:"totalSeederSize,omitempty"`
	TotalLeecherSize    int64   `json:"totalLeecherSize,omitempty"`
}

// SyncRequest represents a sync request
type SyncRequest struct {
	Sites []string `json:"sites,omitempty"` // Empty means sync all
}

// SyncResponse represents a sync response
type SyncResponse struct {
	Success []string          `json:"success"`
	Failed  []SyncFailedEntry `json:"failed,omitempty"`
}

// SyncFailedEntry represents a failed sync entry
type SyncFailedEntry struct {
	Site  string `json:"site"`
	Error string `json:"error"`
}

// userInfoService is the global user info service instance
var userInfoService *v2.UserInfoService

// siteRegistry is the global site registry for creating site instances
var siteRegistry *v2.SiteRegistry

// InitUserInfoService initializes the global user info service
func InitUserInfoService(service *v2.UserInfoService) {
	userInfoService = service
}

// InitSiteRegistry initializes the global site registry
func InitSiteRegistry(registry *v2.SiteRegistry) {
	siteRegistry = registry
}

// GetUserInfoService returns the global user info service
func GetUserInfoService() *v2.UserInfoService {
	return userInfoService
}

// GetSiteRegistry returns the global site registry
func GetSiteRegistry() *v2.SiteRegistry {
	return siteRegistry
}

// RefreshSiteRegistrations refreshes site registrations based on current enabled sites
// This should be called when site configurations change
func RefreshSiteRegistrations(store interface {
	ListSites() (map[models.SiteGroup]models.SiteConfig, error)
},
) error {
	if userInfoService == nil || siteRegistry == nil {
		return nil
	}

	sites, err := store.ListSites()
	if err != nil {
		return err
	}

	// Get current registered sites in UserInfoService
	currentUserInfoSites := make(map[string]bool)
	for _, id := range userInfoService.ListSites() {
		currentUserInfoSites[id] = true
	}

	// Get current registered sites in SearchOrchestrator
	currentSearchSites := make(map[string]bool)
	if searchOrchestrator != nil {
		for _, id := range searchOrchestrator.ListSites() {
			currentSearchSites[id] = true
		}
	}

	// Track which sites should be enabled
	enabledSites := make(map[string]bool)

	for siteGroup, siteConfig := range sites {
		siteID := string(siteGroup)
		if siteConfig.Enabled == nil || !*siteConfig.Enabled {
			// Site is disabled, unregister if currently registered
			if currentUserInfoSites[siteID] {
				userInfoService.UnregisterSite(siteID)
				global.GetSlogger().Infof("[UserInfo] 站点 %s 已从 UserInfoService 注销 (已禁用)", siteID)
			}
			if currentSearchSites[siteID] && searchOrchestrator != nil {
				searchOrchestrator.UnregisterSite(siteID)
				global.GetSlogger().Infof("[Search] 站点 %s 已从 SearchOrchestrator 注销 (已禁用)", siteID)
			}
			continue
		}

		enabledSites[siteID] = true

		// Always re-create site instance to ensure we have the latest credentials
		// This handles both new registrations and credential updates
		site, createErr := siteRegistry.CreateSite(
			siteID,
			v2.SiteCredentials{
				Cookie:  siteConfig.Cookie,
				APIKey:  siteConfig.APIKey,
				Passkey: siteConfig.Passkey,
			},
			siteConfig.APIUrl,
		)
		if createErr != nil {
			global.GetSlogger().Warnf("[Site] 创建站点 %s 失败: %v", siteID, createErr)
			continue
		}

		// Update UserInfoService
		if currentUserInfoSites[siteID] {
			// Site was already registered, unregister first then re-register with new credentials
			userInfoService.UnregisterSite(siteID)
			global.GetSlogger().Debugf("[UserInfo] 站点 %s 凭证已更新，重新注册", siteID)
		}
		userInfoService.RegisterSite(site)
		if !currentUserInfoSites[siteID] {
			global.GetSlogger().Infof("[UserInfo] 站点 %s 已动态注册到 UserInfoService", siteID)
		}

		// Update SearchOrchestrator
		if searchOrchestrator != nil {
			if currentSearchSites[siteID] {
				searchOrchestrator.UnregisterSite(siteID)
				global.GetSlogger().Debugf("[Search] 站点 %s 凭证已更新，重新注册", siteID)
			}
			searchOrchestrator.RegisterSite(site)
			if !currentSearchSites[siteID] {
				global.GetSlogger().Infof("[Search] 站点 %s 已动态注册到 SearchOrchestrator", siteID)
			}
		}
	}

	// Unregister sites that are no longer in the configuration
	for siteID := range currentUserInfoSites {
		if !enabledSites[siteID] {
			userInfoService.UnregisterSite(siteID)
			global.GetSlogger().Infof("[UserInfo] 站点 %s 已从 UserInfoService 注销 (配置中不存在)", siteID)
		}
	}
	for siteID := range currentSearchSites {
		if !enabledSites[siteID] && searchOrchestrator != nil {
			searchOrchestrator.UnregisterSite(siteID)
			global.GetSlogger().Infof("[Search] 站点 %s 已从 SearchOrchestrator 注销 (配置中不存在)", siteID)
		}
	}

	return nil
}

// apiUserInfoAggregated handles GET /api/v2/userinfo/aggregated
func (s *Server) apiUserInfoAggregated(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if userInfoService == nil {
		http.Error(w, "User info service not initialized", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	stats, err := userInfoService.GetAggregated(ctx)
	if err != nil {
		global.GetSlogger().Errorf("[UserInfo] Failed to get aggregated stats: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 获取已启用的站点列表，只返回已启用站点的统计数据
	enabledSites := make(map[string]bool)
	if s.store != nil {
		sites, err := s.store.ListSites()
		if err == nil {
			for siteGroup, siteConfig := range sites {
				if siteConfig.Enabled != nil && *siteConfig.Enabled {
					enabledSites[strings.ToLower(string(siteGroup))] = true
				}
			}
		}
	}

	// 过滤只保留已启用站点的数据
	filteredStats := filterStatsByEnabledSites(stats, enabledSites)

	response := toAggregatedStatsResponse(filteredStats)
	writeJSON(w, response)
}

// apiUserInfoSites handles GET /api/v2/userinfo/sites
func (s *Server) apiUserInfoSites(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if userInfoService == nil {
		http.Error(w, "User info service not initialized", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	infos, err := userInfoService.GetAllUserInfo(ctx)
	if err != nil {
		global.GetSlogger().Errorf("[UserInfo] Failed to get all user info: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	responses := make([]UserInfoResponse, len(infos))
	for i, info := range infos {
		responses[i] = toUserInfoResponse(info)
	}

	writeJSON(w, responses)
}

// apiUserInfoSiteDetail handles GET/POST /api/v2/userinfo/sites/:site
func (s *Server) apiUserInfoSiteDetail(w http.ResponseWriter, r *http.Request) {
	// Extract site ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v2/userinfo/sites/")
	siteID := strings.TrimSuffix(path, "/")

	if siteID == "" {
		http.Error(w, "Site ID required", http.StatusBadRequest)
		return
	}

	if userInfoService == nil {
		http.Error(w, "User info service not initialized", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		// Get user info for specific site
		info, err := userInfoService.GetUserInfo(ctx, siteID)
		if err != nil {
			global.GetSlogger().Errorf("[UserInfo] Failed to get user info for site %s: %v", siteID, err)
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, toUserInfoResponse(info))

	case http.MethodPost:
		// Refresh site registrations to ensure we have the latest credentials from database
		if s.store != nil {
			if err := RefreshSiteRegistrations(s.store); err != nil {
				global.GetSlogger().Warnf("[UserInfo] 刷新站点注册失败: %v", err)
			}
		}
		// Sync (fetch and save) user info for specific site
		info, err := userInfoService.FetchAndSave(ctx, siteID)
		if err != nil {
			global.GetSlogger().Errorf("[UserInfo] Failed to sync user info for site %s: %v", siteID, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		global.GetSlogger().Infof("[UserInfo] Synced user info for site %s", siteID)
		writeJSON(w, toUserInfoResponse(info))

	case http.MethodDelete:
		// Delete user info for specific site
		if err := userInfoService.DeleteUserInfo(ctx, siteID); err != nil {
			global.GetSlogger().Errorf("[UserInfo] Failed to delete user info for site %s: %v", siteID, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		global.GetSlogger().Infof("[UserInfo] Deleted user info for site %s", siteID)
		writeJSON(w, map[string]string{"status": "deleted"})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// apiUserInfoSync handles POST /api/v2/userinfo/sync
func (s *Server) apiUserInfoSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if userInfoService == nil {
		http.Error(w, "User info service not initialized", http.StatusServiceUnavailable)
		return
	}

	var req SyncRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Refresh site registrations to ensure we have the latest credentials from database
	if s.store != nil {
		if err := RefreshSiteRegistrations(s.store); err != nil {
			global.GetSlogger().Warnf("[UserInfo] 刷新站点注册失败: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	var response SyncResponse

	if len(req.Sites) == 0 {
		// Sync all sites with concurrency control
		results, errors := userInfoService.FetchAndSaveAllWithConcurrency(ctx, 3, 30*time.Second)
		for _, info := range results {
			response.Success = append(response.Success, info.Site)
		}
		for _, syncErr := range errors {
			response.Failed = append(response.Failed, SyncFailedEntry{
				Site:  syncErr.Site,
				Error: syncErr.Error.Error(),
			})
		}
	} else {
		// Sync specific sites
		for _, siteID := range req.Sites {
			_, err := userInfoService.FetchAndSave(ctx, siteID)
			if err != nil {
				response.Failed = append(response.Failed, SyncFailedEntry{
					Site:  siteID,
					Error: err.Error(),
				})
			} else {
				response.Success = append(response.Success, siteID)
			}
		}
	}

	global.GetSlogger().Infof("[UserInfo] Sync completed: %d success, %d failed",
		len(response.Success), len(response.Failed))

	writeJSON(w, response)
}

// apiUserInfoRegisteredSites handles GET /api/v2/userinfo/registered
func (s *Server) apiUserInfoRegisteredSites(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if userInfoService == nil {
		http.Error(w, "User info service not initialized", http.StatusServiceUnavailable)
		return
	}

	sites := userInfoService.ListSites()
	writeJSON(w, map[string][]string{"sites": sites})
}

// apiUserInfoClearCache handles POST /api/v2/userinfo/cache/clear
func (s *Server) apiUserInfoClearCache(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if userInfoService == nil {
		http.Error(w, "User info service not initialized", http.StatusServiceUnavailable)
		return
	}

	userInfoService.ClearCache()
	global.GetSlogger().Info("[UserInfo] Cache cleared")
	writeJSON(w, map[string]string{"status": "ok"})
}

// Helper functions

func toUserInfoResponse(info v2.UserInfo) UserInfoResponse {
	return UserInfoResponse{
		Site:                info.Site,
		Username:            info.Username,
		UserID:              info.UserID,
		Uploaded:            info.Uploaded,
		Downloaded:          info.Downloaded,
		Ratio:               info.Ratio,
		Bonus:               info.Bonus,
		Seeding:             info.Seeding,
		Leeching:            info.Leeching,
		Rank:                info.Rank,
		JoinDate:            info.JoinDate,
		LastAccess:          info.LastAccess,
		LastUpdate:          info.LastUpdate,
		LevelName:           info.LevelName,
		LevelID:             info.LevelID,
		BonusPerHour:        info.BonusPerHour,
		SeedingBonus:        info.SeedingBonus,
		SeedingBonusPerHour: info.SeedingBonusPerHour,
		UnreadMessageCount:  info.UnreadMessageCount,
		TotalMessageCount:   info.TotalMessageCount,
		SeederCount:         info.SeederCount,
		SeederSize:          info.SeederSize,
		LeecherCount:        info.LeecherCount,
		LeecherSize:         info.LeecherSize,
		HnRUnsatisfied:      info.HnRUnsatisfied,
		HnRPreWarning:       info.HnRPreWarning,
		TrueUploaded:        info.TrueUploaded,
		TrueDownloaded:      info.TrueDownloaded,
		Uploads:             info.Uploads,
	}
}

func toAggregatedStatsResponse(stats v2.AggregatedStats) AggregatedStatsResponse {
	perSite := make([]UserInfoResponse, len(stats.PerSiteStats))
	for i, info := range stats.PerSiteStats {
		perSite[i] = toUserInfoResponse(info)
	}

	return AggregatedStatsResponse{
		TotalUploaded:       stats.TotalUploaded,
		TotalDownloaded:     stats.TotalDownloaded,
		AverageRatio:        stats.AverageRatio,
		TotalSeeding:        stats.TotalSeeding,
		TotalLeeching:       stats.TotalLeeching,
		TotalBonus:          stats.TotalBonus,
		SiteCount:           stats.SiteCount,
		LastUpdate:          stats.LastUpdate,
		PerSiteStats:        perSite,
		TotalBonusPerHour:   stats.TotalBonusPerHour,
		TotalSeedingBonus:   stats.TotalSeedingBonus,
		TotalUnreadMessages: stats.TotalUnreadMessages,
		TotalSeederSize:     stats.TotalSeederSize,
		TotalLeecherSize:    stats.TotalLeecherSize,
	}
}

// filterStatsByEnabledSites 过滤统计数据，只保留已启用站点的数据
func filterStatsByEnabledSites(stats v2.AggregatedStats, enabledSites map[string]bool) v2.AggregatedStats {
	// 过滤 PerSiteStats，只保留已启用的站点
	var filteredPerSite []v2.UserInfo
	for _, info := range stats.PerSiteStats {
		siteLower := strings.ToLower(info.Site)
		if enabledSites[siteLower] {
			filteredPerSite = append(filteredPerSite, info)
		}
	}

	// 重新计算聚合数据
	filtered := v2.AggregatedStats{
		LastUpdate:   stats.LastUpdate,
		PerSiteStats: filteredPerSite,
		SiteCount:    len(filteredPerSite),
	}

	var totalRatio float64
	var ratioCount int

	for _, info := range filteredPerSite {
		filtered.TotalUploaded += info.Uploaded
		filtered.TotalDownloaded += info.Downloaded
		filtered.TotalSeeding += info.Seeding
		filtered.TotalLeeching += info.Leeching
		filtered.TotalBonus += info.Bonus

		// Aggregate extended fields
		filtered.TotalBonusPerHour += info.BonusPerHour
		filtered.TotalSeedingBonus += info.SeedingBonus
		filtered.TotalUnreadMessages += info.UnreadMessageCount
		filtered.TotalSeederSize += info.SeederSize
		filtered.TotalLeecherSize += info.LeecherSize

		// Only count valid ratios for average
		if info.Ratio > 0 && info.Ratio < 1000 {
			totalRatio += info.Ratio
			ratioCount++
		}
	}

	if ratioCount > 0 {
		filtered.AverageRatio = totalRatio / float64(ratioCount)
	}

	return filtered
}
