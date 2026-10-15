package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/scheduler"
)

// 自动删种的状态与预览（只认 session）：上一轮删了哪些、下一轮大约什么时候；按页面上的规则（还没保存的也行）
// 现在会删哪些种子，只算、不删。

func (s *Server) registerCleanupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/cleanup/status", s.auth(s.apiCleanupStatus))
	mux.HandleFunc("POST /api/cleanup/preview", s.auth(s.apiCleanupPreview))
}

// CleanupStatusView 是自动删种页上的「上次运行」。
type CleanupStatusView struct {
	Enabled     bool `json:"enabled"`
	IntervalMin int  `json:"interval_min"`
	// LastRun 是最近一轮（重启以后没有）；NextRunAt 是下一轮大约什么时候（没启用时为空；磁盘空间不足时会提前）
	LastRun   *scheduler.CleanupRun `json:"last_run,omitempty"`
	NextRunAt *time.Time            `json:"next_run_at,omitempty"`
}

func (s *Server) apiCleanupStatus(w http.ResponseWriter, _ *http.Request) {
	if global.GlobalDB == nil || s.mgr == nil {
		http.Error(w, "服务没有初始化", http.StatusServiceUnavailable)
		return
	}
	gs, err := core.NewConfigStore(global.GlobalDB).GetGlobalSettings()
	if err != nil {
		http.Error(w, "读取设置失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	v := CleanupStatusView{Enabled: gs.CleanupEnabled, IntervalMin: gs.CleanupIntervalMin}
	last, next := s.mgr.CleanupStatus()
	v.LastRun = last
	if !next.IsZero() && gs.CleanupEnabled {
		v.NextRunAt = &next
	}
	writeJSON(w, v)
}

// CleanupPreviewInput 是预览用的删种规则（自动删种页上的那几项，可以还没保存）。开关与间隔不影响会删哪些，不用给。
type CleanupPreviewInput struct {
	CleanupScope          string  `json:"cleanup_scope"`
	CleanupScopeTags      string  `json:"cleanup_scope_tags"`
	CleanupRemoveData     bool    `json:"cleanup_remove_data"`
	CleanupConditionMode  string  `json:"cleanup_condition_mode"`
	CleanupMaxSeedTimeH   int     `json:"cleanup_max_seed_time_h"`
	CleanupMinRatio       float64 `json:"cleanup_min_ratio"`
	CleanupMaxInactiveH   int     `json:"cleanup_max_inactive_h"`
	CleanupSlowSeedTimeH  int     `json:"cleanup_slow_seed_time_h"`
	CleanupSlowMaxRatio   float64 `json:"cleanup_slow_max_ratio"`
	CleanupDelFreeExpired bool    `json:"cleanup_del_free_expired"`
	CleanupDiskProtect    bool    `json:"cleanup_disk_protect"`
	CleanupMinDiskSpaceGB float64 `json:"cleanup_min_disk_space_gb"`
	CleanupProtectDL      bool    `json:"cleanup_protect_dl"`
	CleanupProtectHR      bool    `json:"cleanup_protect_hr"`
	CleanupMinRetainH     int     `json:"cleanup_min_retain_h"`
	CleanupProtectTags    string  `json:"cleanup_protect_tags"`
}

// CleanupPreviewView 是预览的结果：每台连得上的下载器一个。
type CleanupPreviewView struct {
	Results []scheduler.CleanupResult `json:"results"`
}

func (s *Server) apiCleanupPreview(w http.ResponseWriter, r *http.Request) {
	if global.GlobalDB == nil || s.mgr == nil {
		http.Error(w, "服务没有初始化", http.StatusServiceUnavailable)
		return
	}
	var in CleanupPreviewInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	switch in.CleanupScope {
	case "", "database", "tag", "all":
	default:
		http.Error(w, "删种范围要选 database、tag 或 all", http.StatusBadRequest)
		return
	}
	if mode := strings.TrimSpace(in.CleanupConditionMode); mode != "" && mode != "or" && mode != "and" {
		http.Error(w, "条件之间要选 or 或 and", http.StatusBadRequest)
		return
	}
	gs, err := core.NewConfigStore(global.GlobalDB).GetGlobalSettings()
	if err != nil {
		http.Error(w, "读取设置失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	cfg := gs
	cfg.CleanupScope, cfg.CleanupScopeTags, cfg.CleanupRemoveData = in.CleanupScope, in.CleanupScopeTags, in.CleanupRemoveData
	cfg.CleanupConditionMode = strings.TrimSpace(in.CleanupConditionMode)
	cfg.CleanupMaxSeedTimeH, cfg.CleanupMinRatio, cfg.CleanupMaxInactiveH = in.CleanupMaxSeedTimeH, in.CleanupMinRatio, in.CleanupMaxInactiveH
	cfg.CleanupSlowSeedTimeH, cfg.CleanupSlowMaxRatio = in.CleanupSlowSeedTimeH, in.CleanupSlowMaxRatio
	cfg.CleanupDelFreeExpired, cfg.CleanupDiskProtect, cfg.CleanupMinDiskSpaceGB = in.CleanupDelFreeExpired, in.CleanupDiskProtect, in.CleanupMinDiskSpaceGB
	cfg.CleanupProtectDL, cfg.CleanupProtectHR, cfg.CleanupMinRetainH, cfg.CleanupProtectTags = in.CleanupProtectDL, in.CleanupProtectHR, in.CleanupMinRetainH, in.CleanupProtectTags
	res := s.mgr.CleanupPreview(r.Context(), &cfg)
	if res == nil {
		res = []scheduler.CleanupResult{}
	}
	writeJSON(w, CleanupPreviewView{Results: res})
}
