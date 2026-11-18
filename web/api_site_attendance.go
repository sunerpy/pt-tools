package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/assistant"
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
	// Challenge 是签到要过的一关（hdsky_captcha、opencd_captcha 要识别验证码，baka_quiz 要答题），要用 AI；不用时为空
	Challenge     string `json:"challenge,omitempty"`
	Day           string `json:"day"`
	Status        string `json:"status"`
	Attempts      int    `json:"attempts"`
	Message       string `json:"message,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	ScheduledAt   *int64 `json:"scheduled_at,omitempty"`
	NextAttemptAt *int64 `json:"next_attempt_at,omitempty"`
	LastAttemptAt *int64 `json:"last_attempt_at,omitempty"`
}

// AttendanceSettingsDTO 是每日签到设置：时间窗（HH:MM，进程时区）与 AI 签到。
type AttendanceSettingsDTO struct {
	WindowStart string `json:"window_start"`
	WindowEnd   string `json:"window_end"`
	// AIEnabled 让 AI 过签到的验证码与答题；VisionModel 是识别验证码用的模型，空时用 AI 助手的模型
	AIEnabled   bool   `json:"ai_enabled"`
	VisionModel string `json:"vision_model"`
	// AssistantReady、AssistantModel 只读：AI 助手是否启用并配好、用的哪个模型（界面提示用）
	AssistantReady bool   `json:"assistant_ready"`
	AssistantModel string `json:"assistant_model,omitempty"`
	// NotifyEnabled：每天所有站点签完后推一条签到结果（默认开）；NotifyChannelIDs 是推到哪些通道，空表示所有启用的通道
	NotifyEnabled    bool   `json:"notify_enabled"`
	NotifyChannelIDs []uint `json:"notify_channel_ids"`
}

// attendanceSettingsInput 是保存签到设置的参数：AI 两项为 nil 时不改（旧版前端只发时间窗）；只读字段原样回传时忽略。
type attendanceSettingsInput struct {
	WindowStart string  `json:"window_start"`
	WindowEnd   string  `json:"window_end"`
	AIEnabled   *bool   `json:"ai_enabled"`
	VisionModel *string `json:"vision_model"`
	// 推送两项为 nil 时不改（旧版前端不发）
	NotifyEnabled    *bool   `json:"notify_enabled"`
	NotifyChannelIDs *[]uint `json:"notify_channel_ids"`
}

type attendanceToggleRequest struct {
	Enabled *bool `json:"enabled"`
}

func (s *Server) registerAttendanceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/sites/attendance", s.auth(s.apiAttendanceList))
	mux.HandleFunc("/api/sites/attendance/settings", s.auth(s.apiAttendanceSettings))
	mux.HandleFunc("/api/sites/attendance/ai-test", s.auth(s.apiAttendanceAITest))
	mux.HandleFunc("/api/sites/attendance/run", s.auth(s.apiAttendanceRun))
	mux.HandleFunc("/api/sites/attendance/history", s.auth(s.apiAttendanceHistory))
	mux.HandleFunc("/api/sites/attendance/events", s.auth(s.apiAttendanceEvents))
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
		Challenge:         string(v2.AttendanceChallengeOf(def)),
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

func (s *Server) attendanceSettings(r *http.Request) (AttendanceSettingsDTO, error) {
	start, end, err := s.store.AttendanceWindow()
	if err != nil {
		return AttendanceSettingsDTO{}, err
	}
	aiOn, vision, err := s.store.AttendanceAI()
	if err != nil {
		return AttendanceSettingsDTO{}, err
	}
	notifyOn, channelIDs, err := s.store.AttendanceNotify()
	if err != nil {
		return AttendanceSettingsDTO{}, err
	}
	out := AttendanceSettingsDTO{
		WindowStart: start, WindowEnd: end, AIEnabled: aiOn, VisionModel: vision,
		NotifyEnabled: notifyOn, NotifyChannelIDs: channelIDs,
	}
	if s.assistant != nil {
		if st, err := s.assistant.Status(r.Context(), assistant.Actor{}); err == nil && st.Enabled {
			out.AssistantReady, out.AssistantModel = true, st.Model
		}
	}
	return out, nil
}

func (s *Server) apiAttendanceSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		out, err := s.attendanceSettings(r)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, out)
	case http.MethodPut:
		var req attendanceSettingsInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, fmt.Sprintf("无效请求: %v", err), http.StatusBadRequest)
			return
		}
		if err := core.ValidateAttendanceWindow(req.WindowStart, req.WindowEnd); err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.VisionModel != nil {
			if err := core.ValidateVisionModel(strings.TrimSpace(*req.VisionModel)); err != nil {
				writeJSONError(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if err := s.store.SaveAttendanceWindow(req.WindowStart, req.WindowEnd); err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if req.AIEnabled != nil || req.VisionModel != nil {
			aiOn, vision, err := s.store.AttendanceAI()
			if err != nil {
				writeJSONError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if req.AIEnabled != nil {
				aiOn = *req.AIEnabled
			}
			if req.VisionModel != nil {
				vision = *req.VisionModel
			}
			if err := s.store.SaveAttendanceAI(aiOn, vision); err != nil {
				writeJSONError(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		if req.NotifyEnabled != nil || req.NotifyChannelIDs != nil {
			notifyOn, channelIDs, err := s.store.AttendanceNotify()
			if err != nil {
				writeJSONError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if req.NotifyEnabled != nil {
				notifyOn = *req.NotifyEnabled
			}
			if req.NotifyChannelIDs != nil {
				channelIDs = *req.NotifyChannelIDs
			}
			// 通道不存在是请求的问题
			if err := s.store.SaveAttendanceNotify(notifyOn, channelIDs); err != nil {
				writeJSONError(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		out, err := s.attendanceSettings(r)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, out)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// apiAttendanceAITest 是「测试看图」：现画一张 6 位验证码，让看图模型（为空时用 AI 助手的模型）认，回认得对不对。
// 模型只经 AI 助手的接口地址与 Key 调用；还没保存的模型名也能试。
func (s *Server) apiAttendanceAITest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	svc, ok := s.assistantService(w)
	if !ok {
		return
	}
	var in struct {
		VisionModel string `json:"vision_model"`
	}
	if !decodeStrictBody(w, r, &in) {
		return
	}
	model := strings.TrimSpace(in.VisionModel)
	if err := core.ValidateVisionModel(model); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	got, err := svc.TestVision(r.Context(), model)
	if err != nil {
		writeAIError(w, err)
		return
	}
	writeJSON(w, got)
}

// attendanceRunMax 是一次「一键签到」最多跑多久：要 AI 认验证码的站点一站最多 3 分钟。
const attendanceRunMax = 30 * time.Minute

// attendanceRunState 是网页「一键签到」的进度：同一时间只跑一轮，页面轮询它显示进度。零值可用。
type attendanceRunState struct {
	mu         sync.Mutex
	running    bool
	startedAt  time.Time
	finishedAt time.Time
	done       int
	total      int
	err        string
}

// AttendanceRunResponse 是一键签到的进度：Running 为 false 且有 FinishedAt 时是上一轮的结果。
type AttendanceRunResponse struct {
	Running    bool   `json:"running"`
	StartedAt  *int64 `json:"started_at,omitempty"`
	FinishedAt *int64 `json:"finished_at,omitempty"`
	Done       int    `json:"done"`
	Total      int    `json:"total"`
	Error      string `json:"error,omitempty"`
}

func (st *attendanceRunState) snapshot() AttendanceRunResponse {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := AttendanceRunResponse{Running: st.running, Done: st.done, Total: st.total, Error: st.err}
	if !st.startedAt.IsZero() {
		v := st.startedAt.Unix()
		out.StartedAt = &v
	}
	if !st.finishedAt.IsZero() {
		v := st.finishedAt.Unix()
		out.FinishedAt = &v
	}
	return out
}

// apiAttendanceRun 是「一键签到」：GET 看进度；POST 在后台给开启了自动签到、今天还没有结果的站点各签一次，
// 马上回 202，页面轮询 GET 与签到列表看结果。已经在跑时 POST 回 409。
func (s *Server) apiAttendanceRun(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, s.attendanceRun.snapshot())
	case http.MethodPost:
		mon := s.attendanceMonitor()
		if mon == nil {
			writeJSONError(w, "签到服务未启动", http.StatusServiceUnavailable)
			return
		}
		st := &s.attendanceRun
		st.mu.Lock()
		if st.running {
			st.mu.Unlock()
			writeJSONError(w, "一键签到正在进行，等这一轮做完再试", http.StatusConflict)
			return
		}
		st.running, st.startedAt, st.finishedAt, st.done, st.total, st.err = true, time.Now(), time.Time{}, 0, 0, ""
		st.mu.Unlock()
		s.goBackground(func() {
			ctx, cancel := context.WithTimeout(context.Background(), attendanceRunMax)
			defer cancel()
			_, err := mon.SignAllWithProgress(ctx, func(done, total int) {
				st.mu.Lock()
				st.done, st.total = done, total
				st.mu.Unlock()
			})
			st.mu.Lock()
			st.running, st.finishedAt = false, time.Now()
			if err != nil {
				st.err = err.Error()
			}
			st.mu.Unlock()
			if err != nil {
				global.GetSlogger().Warnf("[Attendance] 一键签到没有做完: %v", err)
			}
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(st.snapshot())
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// AttendanceHistoryRow 是某站某天的签到结果。
type AttendanceHistoryRow struct {
	SiteName      string `json:"site_name"`
	Day           string `json:"day"`
	Status        string `json:"status"`
	Attempts      int    `json:"attempts"`
	Message       string `json:"message,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	LastAttemptAt *int64 `json:"last_attempt_at,omitempty"`
}

// AttendanceHistoryResponse 是最近几天的签到记录：Days 从今天往前排。
type AttendanceHistoryResponse struct {
	Days []string               `json:"days"`
	Rows []AttendanceHistoryRow `json:"rows"`
}

// attendanceHistoryMaxDays 是签到记录最多往前看几天。
const attendanceHistoryMaxDays = 60

// apiAttendanceHistory 返回最近 days 天（默认 14，最多 60）各站点的签到记录。
func (s *Server) apiAttendanceHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if global.GlobalDB == nil {
		writeJSONError(w, "数据库未初始化", http.StatusServiceUnavailable)
		return
	}
	days := 14
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeJSONError(w, "days 应为正整数", http.StatusBadRequest)
			return
		}
		days = min(n, attendanceHistoryMaxDays)
	}
	today, err := time.Parse("2006-01-02", s.attendanceDay())
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := AttendanceHistoryResponse{Days: make([]string, 0, days), Rows: []AttendanceHistoryRow{}}
	for i := range days {
		out.Days = append(out.Days, today.AddDate(0, 0, -i).Format("2006-01-02"))
	}
	rows, err := models.NewSiteAttendanceRepository(global.GlobalDB.DB).ListRange(out.Days[len(out.Days)-1], out.Days[0])
	if err != nil {
		writeJSONError(w, fmt.Sprintf("加载签到记录失败: %v", err), http.StatusInternalServerError)
		return
	}
	for _, row := range rows {
		out.Rows = append(out.Rows, AttendanceHistoryRow{
			SiteName: row.SiteName, Day: row.Day, Status: row.Status, Attempts: row.Attempts,
			Message: row.Message, LastError: row.LastError, LastAttemptAt: timestampPtr(row.LastAttemptAt),
		})
	}
	writeJSON(w, out)
}
