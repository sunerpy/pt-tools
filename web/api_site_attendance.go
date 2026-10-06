package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// SiteAttendanceResponse 是一个站点的每日签到状态：能否签到、开关，以及当天的结果（没有记录时 Status 为空）。
type SiteAttendanceResponse struct {
	SiteName          string `json:"site_name"`
	SiteEnabled       bool   `json:"site_enabled"`
	AttendanceEnabled bool   `json:"attendance_enabled"`
	Supported         bool   `json:"supported"`
	UnsupportedReason string `json:"unsupported_reason,omitempty"`
	Day               string `json:"day"`
	Status            string `json:"status"`
	Attempts          int    `json:"attempts"`
	Message           string `json:"message,omitempty"`
	LastError         string `json:"last_error,omitempty"`
	ScheduledAt       *int64 `json:"scheduled_at,omitempty"`
	NextAttemptAt     *int64 `json:"next_attempt_at,omitempty"`
	LastAttemptAt     *int64 `json:"last_attempt_at,omitempty"`
}

// AttendanceSettingsDTO 是签到时间窗（HH:MM，进程时区）。
type AttendanceSettingsDTO struct {
	WindowStart string `json:"window_start"`
	WindowEnd   string `json:"window_end"`
}

type attendanceToggleRequest struct {
	Enabled *bool `json:"enabled"`
}

func (s *Server) registerAttendanceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/sites/attendance", s.auth(s.apiAttendanceList))
	mux.HandleFunc("/api/sites/attendance/settings", s.auth(s.apiAttendanceSettings))
}

func (s *Server) attendanceMonitor() *scheduler.AttendanceMonitor {
	if s == nil || s.mgr == nil {
		return nil
	}
	return s.mgr.GetAttendanceMonitor()
}

func (s *Server) attendanceDay() string {
	if mon := s.attendanceMonitor(); mon != nil {
		return mon.Today()
	}
	return time.Now().Format("2006-01-02")
}

func (s *Server) apiAttendanceList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if global.GlobalDB == nil {
		writeJSONError(w, "数据库未初始化", http.StatusServiceUnavailable)
		return
	}
	db := global.GlobalDB.DB
	sites, err := models.NewSiteRepository(db).ListSites()
	if err != nil {
		writeJSONError(w, fmt.Sprintf("加载站点失败: %v", err), http.StatusInternalServerError)
		return
	}
	day := s.attendanceDay()
	rows, err := models.NewSiteAttendanceRepository(db).ListDay(day)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("加载签到记录失败: %v", err), http.StatusInternalServerError)
		return
	}
	byName := make(map[string]models.SiteAttendanceLog, len(rows))
	for _, row := range rows {
		byName[row.SiteName] = row
	}
	out := make([]SiteAttendanceResponse, 0, len(sites))
	for _, site := range sites {
		var today *models.SiteAttendanceLog
		if row, ok := byName[site.Name]; ok {
			today = &row
		}
		out = append(out, buildAttendanceResponse(site, day, today))
	}
	writeJSON(w, out)
}

func buildAttendanceResponse(site models.SiteSetting, day string, today *models.SiteAttendanceLog) SiteAttendanceResponse {
	def, _ := v2.GetDefinitionRegistry().Get(site.Name)
	supported, reason := v2.AttendanceSupport(def)
	resp := SiteAttendanceResponse{
		SiteName:          site.Name,
		SiteEnabled:       site.Enabled,
		AttendanceEnabled: site.AttendanceEnabled,
		Supported:         supported,
		UnsupportedReason: reason,
		Day:               day,
	}
	if today != nil {
		resp.Status = today.Status
		resp.Attempts = today.Attempts
		resp.Message = today.Message
		resp.LastError = today.LastError
		scheduled := today.ScheduledAt
		resp.ScheduledAt = timestampPtr(&scheduled)
		resp.NextAttemptAt = timestampPtr(today.NextAttemptAt)
		resp.LastAttemptAt = timestampPtr(today.LastAttemptAt)
	}
	return resp
}

// handleSiteAttendance 处理 /api/sites/{name}/attendance：POST 立即签到一次，PUT 打开或关闭每日自动签到。
func (s *Server) handleSiteAttendance(w http.ResponseWriter, r *http.Request, siteName string) {
	if global.GlobalDB == nil {
		writeJSONError(w, "数据库未初始化", http.StatusServiceUnavailable)
		return
	}
	site, err := models.NewSiteRepository(global.GlobalDB.DB).GetSiteByName(siteName)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("站点 %s 不存在", siteName), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodPost:
		s.handleAttendanceSignNow(w, r, *site)
	case http.MethodPut:
		s.handleAttendanceToggle(w, r, *site)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAttendanceSignNow(w http.ResponseWriter, r *http.Request, site models.SiteSetting) {
	mon := s.attendanceMonitor()
	if mon == nil {
		writeJSONError(w, "签到服务未启动", http.StatusServiceUnavailable)
		return
	}
	row, err := mon.SignNow(r.Context(), site.Name)
	if errors.Is(err, scheduler.ErrAttendanceBusy) {
		writeJSONError(w, err.Error(), http.StatusConflict)
		return
	}
	if err != nil {
		writeJSONError(w, fmt.Sprintf("签到失败: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, buildAttendanceResponse(site, row.Day, row))
}

func (s *Server) handleAttendanceToggle(w http.ResponseWriter, r *http.Request, site models.SiteSetting) {
	var req attendanceToggleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Enabled == nil {
		writeJSONError(w, "请求体应为 {\"enabled\": true|false}", http.StatusBadRequest)
		return
	}
	if *req.Enabled {
		def, _ := v2.GetDefinitionRegistry().Get(site.Name)
		if ok, reason := v2.AttendanceSupport(def); !ok {
			writeJSONError(w, "该站点不能开启自动签到："+reason, http.StatusBadRequest)
			return
		}
	}
	if err := s.store.SetSiteAttendanceEnabled(site.Name, *req.Enabled); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	site.AttendanceEnabled = *req.Enabled
	writeJSON(w, buildAttendanceResponse(site, s.attendanceDay(), nil))
}

func (s *Server) apiAttendanceSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		start, end, err := s.store.AttendanceWindow()
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, AttendanceSettingsDTO{WindowStart: start, WindowEnd: end})
	case http.MethodPut:
		var req AttendanceSettingsDTO
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, fmt.Sprintf("无效请求: %v", err), http.StatusBadRequest)
			return
		}
		if err := core.ValidateAttendanceWindow(req.WindowStart, req.WindowEnd); err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.store.SaveAttendanceWindow(req.WindowStart, req.WindowEnd); err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, req)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
