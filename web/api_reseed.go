package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/iyuu"
	"github.com/sunerpy/pt-tools/internal/reseed"
	"github.com/sunerpy/pt-tools/internal/transfer"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

// IYUU 辅种接口：设置、站点对照、立即运行、记录与任务。全部只认 session（s.auth）。
// token 只写不读：接口只返回是否已设置。

// reseedAPITimeout 是读取 IYUU 站点列表等同步调用的时限。
const reseedAPITimeout = time.Minute

func (s *Server) registerReseedRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/reseed/settings", s.auth(s.apiReseedSettings))
	mux.HandleFunc("/api/reseed/sites", s.auth(s.apiReseedSites))
	mux.HandleFunc("/api/reseed/sites/", s.auth(s.apiReseedSite))
	mux.HandleFunc("/api/reseed/run", s.auth(s.apiReseedRun))
	mux.HandleFunc("/api/reseed/runs", s.auth(s.apiReseedRuns))
	mux.HandleFunc("/api/reseed/runs/", s.auth(s.apiReseedRunDetail))
	mux.HandleFunc("/api/reseed/records", s.auth(s.apiReseedRecords))
	mux.HandleFunc("/api/reseed/records/", s.auth(s.apiReseedRecordAction))
	mux.HandleFunc("/api/reseed/jobs", s.auth(s.apiReseedJobs))
}

// ReseedSettingsView 是设置加上「是否正在运行」与正在跑的那一轮。
type ReseedSettingsView struct {
	reseed.Settings
	Running    bool                        `json:"running"`
	CurrentRun *scheduler.ReseedCurrentRun `json:"current_run,omitempty"`
}

func reseedSettingsView(worker *scheduler.ReseedWorker, got reseed.Settings) ReseedSettingsView {
	cur := worker.Current()
	return ReseedSettingsView{Settings: got, Running: worker.Running() || cur != nil, CurrentRun: cur}
}

func (s *Server) reseedWorker(w http.ResponseWriter) (*scheduler.ReseedWorker, *reseed.Service, bool) {
	var worker *scheduler.ReseedWorker
	if s != nil && s.mgr != nil {
		worker = s.mgr.GetReseedWorker()
	}
	if worker == nil || worker.Service() == nil {
		http.Error(w, "辅种服务没有启动", http.StatusServiceUnavailable)
		return nil, nil, false
	}
	return worker, worker.Service(), true
}

func writeReseedError(w http.ResponseWriter, err error) {
	var rl *iyuu.RateLimitError
	var apiErr *iyuu.APIError
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, reseed.ErrInvalid), errors.Is(err, iyuu.ErrNoToken), errors.Is(err, reseed.ErrNoSites):
		status = http.StatusBadRequest
	case errors.Is(err, scheduler.ErrReseedBusy), errors.Is(err, scheduler.ErrReseedNotCurrent),
		errors.Is(err, reseed.ErrNotContinuable), errors.Is(err, reseed.ErrNoPendingJob),
		errors.Is(err, transfer.ErrNotCancelable):
		status = http.StatusConflict
	case errors.Is(err, reseed.ErrRunNotFound), errors.Is(err, reseed.ErrRecordNotFound):
		status = http.StatusNotFound
	case errors.As(err, &rl), reseed.IYUULimited(err):
		status = http.StatusTooManyRequests
	case errors.As(err, &apiErr):
		status = http.StatusBadGateway
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
	}
	http.Error(w, err.Error(), status)
}

// decodeStrictBody 解析请求体，拒绝不认识的字段和 JSON 之后多出来的内容：设置是整份保存，
// 拼错的字段会被当成没填，悄悄关掉辅种或者清空范围。
func decodeStrictBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		http.Error(w, "请求格式错误: "+err.Error(), http.StatusBadRequest)
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(w, "请求格式错误: JSON 之后还有多余的内容", http.StatusBadRequest)
		return false
	}
	return true
}

// apiReseedSettings handles GET / PUT /api/reseed/settings
func (s *Server) apiReseedSettings(w http.ResponseWriter, r *http.Request) {
	worker, svc, ok := s.reseedWorker(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		got, err := svc.Settings(r.Context())
		if err != nil {
			writeReseedError(w, err)
			return
		}
		writeJSON(w, reseedSettingsView(worker, got))
	case http.MethodPut:
		var req reseed.SettingsUpdate
		if !decodeStrictBody(w, r, &req) {
			return
		}
		got, err := svc.SaveSettings(r.Context(), req)
		if err != nil {
			writeReseedError(w, err)
			return
		}
		writeJSON(w, reseedSettingsView(worker, got))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// reseedSiteToggle 是打开或关掉站点的请求。
type reseedSiteToggle struct {
	Selected *bool `json:"selected"`
}

func decodeSiteToggle(w http.ResponseWriter, r *http.Request) (bool, bool) {
	var req reseedSiteToggle
	if !decodeStrictBody(w, r, &req) {
		return false, false
	}
	if req.Selected == nil {
		http.Error(w, "请求格式错误: 缺少 selected", http.StatusBadRequest)
		return false, false
	}
	return *req.Selected, true
}

// apiReseedSites handles GET /api/reseed/sites（IYUU 支持的站点与 pt-tools 站点的对照）
// 与 PUT /api/reseed/sites {"selected": bool}（全部打开或全部关掉）。
func (s *Server) apiReseedSites(w http.ResponseWriter, r *http.Request) {
	_, svc, ok := s.reseedWorker(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
	case http.MethodPut:
		selected, ok := decodeSiteToggle(w, r)
		if !ok {
			return
		}
		got, err := svc.SetAllSitesSelected(r.Context(), selected)
		if err != nil {
			writeReseedError(w, err)
			return
		}
		writeJSON(w, got)
		return
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), reseedAPITimeout)
	defer cancel()
	items, err := svc.SiteMap(ctx)
	if err != nil {
		writeReseedError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

// apiReseedSite handles PUT /api/reseed/sites/{site} {"selected": bool}：打开或关掉一个站点。
func (s *Server) apiReseedSite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	site := strings.TrimPrefix(r.URL.Path, "/api/reseed/sites/")
	if site == "" || strings.Contains(site, "/") {
		http.NotFound(w, r)
		return
	}
	_, svc, ok := s.reseedWorker(w)
	if !ok {
		return
	}
	selected, ok := decodeSiteToggle(w, r)
	if !ok {
		return
	}
	got, err := svc.SetSiteSelected(r.Context(), site, selected)
	if err != nil {
		writeReseedError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiReseedRun handles POST /api/reseed/run：在后台马上从头跑一轮，不等它结束（结果写进轮次记录与设置的 last_result）。
func (s *Server) apiReseedRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	worker, _, ok := s.reseedWorker(w)
	if !ok {
		return
	}
	id, err := worker.RunNow()
	if err != nil {
		writeReseedError(w, err)
		return
	}
	writeReseedStarted(w, id)
}

func writeReseedStarted(w http.ResponseWriter, id uint) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]any{"started": true, "run_id": id})
}

// reseedPage 读分页参数（page 从 1 起，page_size 最多 100）。
func reseedPage(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}

// apiReseedRuns handles GET /api/reseed/runs?page=&page_size=：轮次列表（新的在前，不带步骤）。
func (s *Server) apiReseedRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	_, svc, ok := s.reseedWorker(w)
	if !ok {
		return
	}
	page, size := reseedPage(r)
	items, total, err := svc.Runs(r.Context(), page, size)
	if err != nil {
		writeReseedError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": total, "page": page, "page_size": size})
}

// reseedStopRequest 是停止一轮的请求：cancel_pending 时这一轮结束以后一起取消它建的、还没加入下载器的任务。
type reseedStopRequest struct {
	CancelPending bool `json:"cancel_pending"`
}

// apiReseedRunDetail handles /api/reseed/runs/{id}：GET 详情（带步骤）、GET {id}/attempts（这一轮的尝试），
// POST {id}/pause、{id}/resume、{id}/stop（正在跑的那一轮）与 {id}/continue（接着最近一个提前停下的轮次往下查）。
func (s *Server) apiReseedRunDetail(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/reseed/runs/"), "/")
	idPart, action, _ := strings.Cut(rest, "/")
	id64, err := strconv.ParseUint(idPart, 10, 64)
	if err != nil || id64 == 0 || strings.Contains(action, "/") {
		http.NotFound(w, r)
		return
	}
	id := uint(id64)
	worker, svc, ok := s.reseedWorker(w)
	if !ok {
		return
	}
	switch action {
	case "":
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		d, gerr := svc.GetRun(r.Context(), id)
		if gerr != nil {
			writeReseedError(w, gerr)
			return
		}
		writeJSON(w, d)
		return
	case "attempts":
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		page, size := reseedPage(r)
		items, total, aerr := svc.RunAttempts(r.Context(), id, page, size)
		if aerr != nil {
			writeReseedError(w, aerr)
			return
		}
		writeJSON(w, map[string]any{"items": items, "total": total, "page": page, "page_size": size})
		return
	case "pause", "resume", "stop", "continue":
	default:
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "pause":
		err = worker.Pause(id)
	case "resume":
		err = worker.Resume(id)
	case "stop":
		var req reseedStopRequest
		if r.ContentLength != 0 && !decodeStrictBody(w, r, &req) {
			return
		}
		err = worker.StopRun(id, req.CancelPending)
	case "continue":
		newID, cerr := worker.Continue(id)
		if cerr != nil {
			writeReseedError(w, cerr)
			return
		}
		writeReseedStarted(w, newID)
		return
	}
	if err != nil {
		writeReseedError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "current_run": worker.Current()})
}

// apiReseedRecords handles GET /api/reseed/records?result=&site=&q=&page=&page_size=：辅种记录（按最后一次尝试从新到旧）
// 与对应任务的状态。result 是页面上的结果分组，q 匹配种子名或 info hash。
func (s *Server) apiReseedRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	_, svc, ok := s.reseedWorker(w)
	if !ok {
		return
	}
	page, size := reseedPage(r)
	q := r.URL.Query()
	got, err := svc.Records(r.Context(), reseed.RecordFilter{Result: q.Get("result"), Site: q.Get("site"), Query: q.Get("q")}, page, size)
	if err != nil {
		writeReseedError(w, err)
		return
	}
	writeJSON(w, got)
}

// reseedIDsRequest 是对一批记录的操作。
type reseedIDsRequest struct {
	IDs []uint `json:"ids"`
}

// apiReseedRecordAction handles POST /api/reseed/records/retry {"ids"}（在后台重试，返回 202 与轮次编号）、
// POST /api/reseed/records/delete {"ids"}（任务还没结束的不删）与 POST /api/reseed/records/{id}/cancel
// （取消还在等待加入下载器的任务）。
func (s *Server) apiReseedRecordAction(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/reseed/records/"), "/")
	var cancelID uint
	switch rest {
	case "retry", "delete":
	default:
		idPart, action, _ := strings.Cut(rest, "/")
		id64, err := strconv.ParseUint(idPart, 10, 64)
		if err != nil || id64 == 0 || action != "cancel" {
			http.NotFound(w, r)
			return
		}
		cancelID = uint(id64)
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	worker, svc, ok := s.reseedWorker(w)
	if !ok {
		return
	}
	if cancelID != 0 {
		if err := svc.CancelRecordJob(r.Context(), cancelID); err != nil {
			writeReseedError(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
		return
	}
	var req reseedIDsRequest
	if !decodeStrictBody(w, r, &req) {
		return
	}
	if rest == "retry" {
		id, err := worker.Retry(req.IDs)
		if err != nil {
			writeReseedError(w, err)
			return
		}
		writeReseedStarted(w, id)
		return
	}
	got, err := svc.DeleteRecords(r.Context(), req.IDs)
	if err != nil {
		writeReseedError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiReseedJobs handles GET（列表）/ DELETE（清除已结束的）/api/reseed/jobs：辅种任务（转移任务里种类为 reseed 的）。
func (s *Server) apiReseedJobs(w http.ResponseWriter, r *http.Request) {
	_, svc, ok := s.transferService(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		jobs, err := svc.ListJobs(r.Context(), r.URL.Query().Get("status"), models.JobKindReseed)
		if err != nil {
			writeTransferError(w, err)
			return
		}
		writeJSON(w, map[string]any{"items": transferJobViews(r.Context(), jobs)})
	case http.MethodDelete:
		// 由辅种服务来清：先把任务结果写进辅种记录，记录不会因为任务没了而一直显示进行中
		_, rs, ok := s.reseedWorker(w)
		if !ok {
			return
		}
		n, err := rs.ClearFinishedJobs(r.Context())
		if err != nil {
			writeReseedError(w, err)
			return
		}
		writeJSON(w, map[string]any{"deleted": n})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
