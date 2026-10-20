package web

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

// AttendanceEventView 是一次签到尝试（时间为 Unix 秒）。
type AttendanceEventView struct {
	ID               uint   `json:"id"`
	Trigger          string `json:"trigger"`
	Result           string `json:"result"`
	Method           string `json:"method"`
	AIModel          string `json:"ai_model,omitempty"`
	AICalls          int    `json:"ai_calls"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	DurationMs       int64  `json:"duration_ms"`
	Message          string `json:"message,omitempty"`
	Error            string `json:"error,omitempty"`
	StartedAt        int64  `json:"started_at"`
	FinishedAt       int64  `json:"finished_at"`
}

// AttendanceDaySite 是某天一个站点的签到：当天的结果（签到记录）加上每次尝试（签到事件）。
type AttendanceDaySite struct {
	SiteName      string `json:"site_name"`
	Status        string `json:"status"`
	Attempts      int    `json:"attempts"`
	Message       string `json:"message,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	LastAttemptAt *int64 `json:"last_attempt_at,omitempty"`
	NextAttemptAt *int64 `json:"next_attempt_at,omitempty"`
	// Method 是这一天签到用的方式：有尝试时取最后一次的，没有时按站点的签到方式
	Method string `json:"method"`
	// AI 这一天一共调了几次、最后用的模型与 token 合计
	AIModel          string                `json:"ai_model,omitempty"`
	AICalls          int                   `json:"ai_calls"`
	PromptTokens     int64                 `json:"prompt_tokens"`
	CompletionTokens int64                 `json:"completion_tokens"`
	Events           []AttendanceEventView `json:"events"`
}

// AttendanceDaySummary 是某天所有站点签到的合计。
type AttendanceDaySummary struct {
	Sites       int `json:"sites"`
	Signed      int `json:"signed"`
	Already     int `json:"already"`
	Failed      int `json:"failed"`
	Pending     int `json:"pending"`
	Unsupported int `json:"unsupported"`
	// AISites 是这一天借助 AI 签到（调过模型）的站点数
	AISites          int   `json:"ai_sites"`
	AICalls          int   `json:"ai_calls"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
}

// AttendanceDayResponse 是「签到详情」：某天每个站点怎么签的。
type AttendanceDayResponse struct {
	Day     string               `json:"day"`
	Today   string               `json:"today"`
	Summary AttendanceDaySummary `json:"summary"`
	Sites   []AttendanceDaySite  `json:"sites"`
}

// apiAttendanceEvents 返回某天（day=YYYY-MM-DD，默认今天）各站点的签到结果与每次尝试。
func (s *Server) apiAttendanceEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if global.GlobalDB == nil {
		writeJSONError(w, "数据库未初始化", http.StatusServiceUnavailable)
		return
	}
	today := s.attendanceDay()
	day := strings.TrimSpace(r.URL.Query().Get("day"))
	if day == "" {
		day = today
	}
	if _, err := time.Parse("2006-01-02", day); err != nil {
		writeJSONError(w, "day 应为 YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	repo := models.NewSiteAttendanceRepository(global.GlobalDB.DB)
	logs, err := repo.ListDay(day)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("加载签到记录失败: %v", err), http.StatusInternalServerError)
		return
	}
	events, err := repo.ListEvents(day)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, attendanceDay(day, today, logs, events))
}

// attendanceDay 把某天的签到记录与签到事件合成「签到详情」：站点按名字排，事件按时间排。
func attendanceDay(day, today string, logs []models.SiteAttendanceLog, events []models.SiteAttendanceEvent) AttendanceDayResponse {
	out := AttendanceDayResponse{Day: day, Today: today, Sites: []AttendanceDaySite{}}
	bySite := map[string]*AttendanceDaySite{}
	site := func(name string) *AttendanceDaySite {
		if v, ok := bySite[name]; ok {
			return v
		}
		v := &AttendanceDaySite{SiteName: name, Status: models.AttendancePending, Events: []AttendanceEventView{}}
		bySite[name] = v
		return v
	}
	for _, l := range logs {
		v := site(l.SiteName)
		v.Status, v.Attempts, v.Message, v.LastError = l.Status, l.Attempts, l.Message, l.LastError
		v.LastAttemptAt, v.NextAttemptAt = timestampPtr(l.LastAttemptAt), timestampPtr(l.NextAttemptAt)
	}
	for _, e := range events {
		v := site(e.SiteName)
		v.Events = append(v.Events, AttendanceEventView{
			ID: e.ID, Trigger: e.Trigger, Result: e.Result, Method: e.Method, AIModel: e.AIModel,
			AICalls: e.AICalls, PromptTokens: e.PromptTokens, CompletionTokens: e.CompletionTokens,
			DurationMs: e.DurationMs, Message: e.Message, Error: e.Error,
			StartedAt: e.StartedAt.Unix(), FinishedAt: e.FinishedAt.Unix(),
		})
		v.Method = e.Method
		v.AICalls += e.AICalls
		v.PromptTokens += e.PromptTokens
		v.CompletionTokens += e.CompletionTokens
		if e.AIModel != "" {
			v.AIModel = e.AIModel
		}
	}
	names := make([]string, 0, len(bySite))
	for name := range bySite {
		names = append(names, name)
	}
	slices.Sort(names)
	sum := &out.Summary
	for _, name := range names {
		v := bySite[name]
		if v.Method == "" {
			v.Method = scheduler.AttendanceMethodOf(name)
		}
		sum.Sites++
		switch v.Status {
		case models.AttendanceSigned:
			sum.Signed++
		case models.AttendanceAlready:
			sum.Already++
		case models.AttendanceFailed:
			sum.Failed++
		case models.AttendanceUnsupported:
			sum.Unsupported++
		default:
			sum.Pending++
		}
		if v.AICalls > 0 {
			sum.AISites++
		}
		sum.AICalls += v.AICalls
		sum.PromptTokens += v.PromptTokens
		sum.CompletionTokens += v.CompletionTokens
		out.Sites = append(out.Sites, *v)
	}
	return out
}
