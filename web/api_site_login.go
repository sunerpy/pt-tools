package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// SiteLoginStateResponse is the API representation of a site_login_state row.
// Cookie / CookieEncrypted MUST never appear here — the row is enriched with
// computed fields (effective_last_active, days_remaining, tier) at read time.
type SiteLoginStateResponse struct {
	SiteName                 string `json:"site_name"`
	DisplayName              string `json:"display_name,omitempty"`
	BaseURL                  string `json:"base_url,omitempty"`
	Enabled                  bool   `json:"enabled"`
	LastLoginAt              *int64 `json:"last_login_at,omitempty"`
	LastAccessAt             *int64 `json:"last_access_at,omitempty"`
	LastVisitAt              *int64 `json:"last_visit_at,omitempty"`
	EffectiveLastActiveAt    *int64 `json:"effective_last_active_at,omitempty"`
	LastProbeAt              *int64 `json:"last_probe_at,omitempty"`
	LastProbeStatus          string `json:"last_probe_status,omitempty"`
	LastProbeError           string `json:"last_probe_error,omitempty"`
	ConsecutiveProbeFailures int    `json:"consecutive_probe_failures"`
	BanThresholdDays         int    `json:"ban_threshold_days"`
	RemindBeforeDays         int    `json:"remind_before_days"`
	ReminderCron             string `json:"reminder_cron"`
	NotificationChannelIDs   []uint `json:"notification_channel_ids"`
	LastReminderTier         string `json:"last_reminder_tier"`
	LastReminderSentAt       *int64 `json:"last_reminder_sent_at,omitempty"`
	DaysRemaining            int    `json:"days_remaining"`
	Tier                     string `json:"tier"`
	ProbeMode                string `json:"probe_mode"`
}

// SiteLoginConfigUpdateRequest is the payload for PUT
// /api/sites/{name}/login-state/config. All fields are optional; only
// non-nil pointers are applied so that callers can update one field at a
// time.
type SiteLoginConfigUpdateRequest struct {
	BanThresholdDays       *int    `json:"ban_threshold_days,omitempty"`
	RemindBeforeDays       *int    `json:"remind_before_days,omitempty"`
	ReminderCron           *string `json:"reminder_cron,omitempty"`
	NotificationChannelIDs *[]uint `json:"notification_channel_ids,omitempty"`
	ProbeMode              *string `json:"probe_mode,omitempty"`
}

// SiteVisitReportRequest is the payload for POST /api/sites/visit emitted
// by the browser extension after webNavigation.onCompleted fires for a
// known PT site domain.
type SiteVisitReportRequest struct {
	SiteName    string `json:"site_name"`
	LastVisitAt string `json:"last_visit_at"`
}

func (s *Server) registerLoginStateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/sites/login-state", s.auth(s.apiSiteLoginStateList))
	mux.HandleFunc("/api/sites/visit", s.auth(s.apiSiteLoginStateVisit))
	mux.HandleFunc("/api/sites/login-state/", s.auth(s.apiSiteLoginStateRouter))
}

func (s *Server) apiSiteLoginStateList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if global.GlobalDB == nil {
		writeJSONError(w, "数据库未初始化", http.StatusServiceUnavailable)
		return
	}
	db := global.GlobalDB.DB

	siteRepo := models.NewSiteRepository(db)
	sites, err := siteRepo.ListSites()
	if err != nil {
		writeJSONError(w, fmt.Sprintf("加载站点失败: %v", err), http.StatusInternalServerError)
		return
	}
	stateRepo := models.NewSiteLoginStateRepository(db)
	states, err := stateRepo.ListLoginStates(false)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("加载站点登录状态失败: %v", err), http.StatusInternalServerError)
		return
	}
	stateByName := map[string]models.SiteLoginState{}
	for i := range states {
		stateByName[states[i].SiteName] = states[i]
	}

	now := time.Now()
	out := make([]SiteLoginStateResponse, 0, len(sites))
	for _, st := range sites {
		state, ok := stateByName[st.Name]
		if !ok {
			banDays, remindDays, _ := models.ApplyPresetIfMissing(st.Name)
			state = models.SiteLoginState{
				SiteName:         st.Name,
				BanThresholdDays: banDays,
				RemindBeforeDays: remindDays,
				ReminderCron:     "0 10,22 * * *",
				LastReminderTier: "none",
				ProbeMode:        "auto",
			}
		}
		out = append(out, buildLoginStateResponse(st, state, now))
	}
	writeJSON(w, out)
}

func (s *Server) apiSiteLoginStateVisit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if global.GlobalDB == nil {
		writeJSONError(w, "数据库未初始化", http.StatusServiceUnavailable)
		return
	}
	var req SiteVisitReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, fmt.Sprintf("无效请求: %v", err), http.StatusBadRequest)
		return
	}
	req.SiteName = strings.TrimSpace(req.SiteName)
	if req.SiteName == "" {
		writeJSONError(w, "site_name 不能为空", http.StatusBadRequest)
		return
	}
	ts, err := parseVisitTimestamp(req.LastVisitAt)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("last_visit_at 解析失败: %v", err), http.StatusBadRequest)
		return
	}
	db := global.GlobalDB.DB
	if err := ensureLoginStateRow(db, req.SiteName); err != nil {
		writeJSONError(w, fmt.Sprintf("初始化站点登录状态失败: %v", err), http.StatusInternalServerError)
		return
	}
	repo := models.NewSiteLoginStateRepository(db)
	if err := repo.ClampLastVisit(req.SiteName, ts, time.Now()); err != nil {
		writeJSONError(w, fmt.Sprintf("写入访问时间失败: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) apiSiteLoginStateRouter(w http.ResponseWriter, r *http.Request) {
	if global.GlobalDB == nil {
		writeJSONError(w, "数据库未初始化", http.StatusServiceUnavailable)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/sites/login-state/")
	if rest == "" {
		writeJSONError(w, "缺少站点名称", http.StatusBadRequest)
		return
	}
	parts := strings.SplitN(rest, "/", 2)
	siteName := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}
	switch action {
	case "":
		s.handleLoginStateGet(w, r, siteName)
	case "probe":
		s.handleLoginStateProbe(w, r, siteName)
	case "test-reminder":
		s.handleLoginStateTestReminder(w, r, siteName)
	case "config":
		s.handleLoginStateConfigUpdate(w, r, siteName)
	default:
		writeJSONError(w, "未知操作", http.StatusNotFound)
	}
}

func (s *Server) handleLoginStateGet(w http.ResponseWriter, r *http.Request, siteName string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	db := global.GlobalDB.DB
	siteRepo := models.NewSiteRepository(db)
	site, err := siteRepo.GetSiteByName(siteName)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("站点 %s 不存在", siteName), http.StatusNotFound)
		return
	}
	state, err := loadOrInitLoginState(db, siteName)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("加载登录状态失败: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, buildLoginStateResponse(*site, *state, time.Now()))
}

func (s *Server) handleLoginStateProbe(w http.ResponseWriter, r *http.Request, siteName string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	mon := s.mgr.GetLoginReminderMonitor()
	if mon == nil {
		writeJSONError(w, "登录提醒监控器未初始化", http.StatusServiceUnavailable)
		return
	}
	db := global.GlobalDB.DB
	siteRepo := models.NewSiteRepository(db)
	if _, err := siteRepo.GetSiteByName(siteName); err != nil {
		writeJSONError(w, fmt.Sprintf("站点 %s 不存在", siteName), http.StatusNotFound)
		return
	}
	release, ok := mon.TryAcquireProbeLock(siteName)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":   "probe_in_progress",
			"message": "探测进行中，请稍候",
		})
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	mon.RunProbeOnceForSiteLocked(ctx, siteName)
	state, err := loadOrInitLoginState(db, siteName)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("加载登录状态失败: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"ok":                true,
		"last_probe_at":     timestampPtr(state.LastProbeAt),
		"last_probe_status": state.LastProbeStatus,
		"last_probe_error":  state.LastProbeError,
	})
}

func (s *Server) handleLoginStateTestReminder(w http.ResponseWriter, r *http.Request, siteName string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	mon := s.mgr.GetLoginReminderMonitor()
	if mon == nil {
		writeJSONError(w, "登录提醒监控器未初始化", http.StatusServiceUnavailable)
		return
	}
	db := global.GlobalDB.DB
	siteRepo := models.NewSiteRepository(db)
	if _, err := siteRepo.GetSiteByName(siteName); err != nil {
		writeJSONError(w, fmt.Sprintf("站点 %s 不存在", siteName), http.StatusNotFound)
		return
	}
	if err := mon.SendTestReminder(r.Context(), siteName); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"success": true, "message": "测试提醒已发送"})
}

func (s *Server) handleLoginStateConfigUpdate(w http.ResponseWriter, r *http.Request, siteName string) {
	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req SiteLoginConfigUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, fmt.Sprintf("无效请求: %v", err), http.StatusBadRequest)
		return
	}
	if req.ReminderCron != nil {
		if _, err := scheduler.ParseCron(*req.ReminderCron); err != nil {
			writeJSONError(w, fmt.Sprintf("无效 cron 表达式: %v", err), http.StatusBadRequest)
			return
		}
	}
	if req.BanThresholdDays != nil && (*req.BanThresholdDays < 1 || *req.BanThresholdDays > 365) {
		writeJSONError(w, "ban_threshold_days 必须在 1..365 之间", http.StatusBadRequest)
		return
	}
	if req.RemindBeforeDays != nil && (*req.RemindBeforeDays < 1 || *req.RemindBeforeDays > 365) {
		writeJSONError(w, "remind_before_days 必须在 1..365 之间", http.StatusBadRequest)
		return
	}
	if req.ProbeMode != nil {
		switch *req.ProbeMode {
		case "auto", "manual", "disabled":
		default:
			writeJSONError(w, "probe_mode 必须为 auto、manual 或 disabled", http.StatusBadRequest)
			return
		}
	}
	db := global.GlobalDB.DB
	if err := ensureLoginStateRow(db, siteName); err != nil {
		writeJSONError(w, fmt.Sprintf("初始化站点登录状态失败: %v", err), http.StatusInternalServerError)
		return
	}

	updates := map[string]any{}
	if req.BanThresholdDays != nil {
		updates["BanThresholdDays"] = *req.BanThresholdDays
	}
	if req.RemindBeforeDays != nil {
		updates["RemindBeforeDays"] = *req.RemindBeforeDays
	}
	if req.ReminderCron != nil {
		updates["ReminderCron"] = *req.ReminderCron
	}
	if req.NotificationChannelIDs != nil {
		raw, err := json.Marshal(*req.NotificationChannelIDs)
		if err != nil {
			writeJSONError(w, fmt.Sprintf("序列化通知通道失败: %v", err), http.StatusBadRequest)
			return
		}
		updates["NotificationChannelIDs"] = string(raw)
	}
	if req.ProbeMode != nil {
		updates["ProbeMode"] = *req.ProbeMode
	}
	if len(updates) == 0 {
		writeJSONError(w, "未提供任何可更新字段", http.StatusBadRequest)
		return
	}
	repo := models.NewSiteLoginStateRepository(db)
	if err := repo.UpsertLoginState(siteName, updates); err != nil {
		writeJSONError(w, fmt.Sprintf("更新失败: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func buildLoginStateResponse(site models.SiteSetting, state models.SiteLoginState, now time.Time) SiteLoginStateResponse {
	resp := SiteLoginStateResponse{
		SiteName:                 site.Name,
		DisplayName:              site.DisplayName,
		BaseURL:                  site.BaseURL,
		Enabled:                  site.Enabled,
		LastLoginAt:              timestampPtr(state.LastLoginAt),
		LastAccessAt:             timestampPtr(state.LastAccessAt),
		LastVisitAt:              timestampPtr(state.LastVisitAt),
		LastProbeAt:              timestampPtr(state.LastProbeAt),
		LastProbeStatus:          state.LastProbeStatus,
		LastProbeError:           state.LastProbeError,
		ConsecutiveProbeFailures: state.ConsecutiveProbeFailures,
		BanThresholdDays:         state.BanThresholdDays,
		RemindBeforeDays:         state.RemindBeforeDays,
		ReminderCron:             state.ReminderCron,
		NotificationChannelIDs:   parseChannelIDs(state.NotificationChannelIDs),
		LastReminderTier:         state.LastReminderTier,
		LastReminderSentAt:       timestampPtr(state.LastReminderSentAt),
		ProbeMode:                state.ProbeMode,
	}

	effective := scheduler.EffectiveLastActive(&state, now)
	if !effective.IsZero() {
		t := effective.Unix()
		resp.EffectiveLastActiveAt = &t
		resp.DaysRemaining = scheduler.DaysRemaining(&state, effective, now)
		resp.Tier = scheduler.ComputeTier(&state, effective, now)
	} else {
		resp.Tier = "unknown"
	}
	return resp
}

func loadOrInitLoginState(db *gorm.DB, siteName string) (*models.SiteLoginState, error) {
	repo := models.NewSiteLoginStateRepository(db)
	state, err := repo.GetLoginState(siteName)
	if err == nil {
		return state, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) &&
		!strings.Contains(err.Error(), "record not found") &&
		!strings.Contains(err.Error(), "登录状态不存在") {
		return nil, err
	}
	if err := ensureLoginStateRow(db, siteName); err != nil {
		return nil, err
	}
	return repo.GetLoginState(siteName)
}

func ensureLoginStateRow(db *gorm.DB, siteName string) error {
	repo := models.NewSiteLoginStateRepository(db)
	if _, err := repo.GetLoginState(siteName); err == nil {
		return nil
	}
	banDays, remindDays, _ := models.ApplyPresetIfMissing(siteName)
	row := &models.SiteLoginState{
		SiteName:         siteName,
		BanThresholdDays: banDays,
		RemindBeforeDays: remindDays,
		ReminderCron:     "0 10,22 * * *",
		LastReminderTier: "none",
	}
	return db.Create(row).Error
}

func parseChannelIDs(raw string) []uint {
	if raw == "" {
		return []uint{}
	}
	var out []uint
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []uint{}
	}
	if out == nil {
		return []uint{}
	}
	return out
}

func parseVisitTimestamp(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("empty timestamp")
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("expected RFC3339, got %q", s)
}

func timestampPtr(t *time.Time) *int64 {
	if t == nil || t.IsZero() {
		return nil
	}
	v := t.Unix()
	return &v
}

var (
	_ = sitelogin.OK
	_ = v2.SchemaNexusPHP
)
