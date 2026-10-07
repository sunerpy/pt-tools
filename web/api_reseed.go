package web

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/sunerpy/pt-tools/internal/iyuu"
	"github.com/sunerpy/pt-tools/internal/reseed"
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
	mux.HandleFunc("/api/reseed/run", s.auth(s.apiReseedRun))
	mux.HandleFunc("/api/reseed/records", s.auth(s.apiReseedRecords))
	mux.HandleFunc("/api/reseed/jobs", s.auth(s.apiReseedJobs))
}

// ReseedSettingsView 是设置加上「是否正在运行」。
type ReseedSettingsView struct {
	reseed.Settings
	Running bool `json:"running"`
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
	case errors.Is(err, scheduler.ErrReseedBusy):
		status = http.StatusConflict
	case errors.As(err, &rl):
		status = http.StatusTooManyRequests
	case errors.As(err, &apiErr):
		status = http.StatusBadGateway
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
	}
	http.Error(w, err.Error(), status)
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
		writeJSON(w, ReseedSettingsView{Settings: got, Running: worker.Running()})
	case http.MethodPut:
		var req reseed.SettingsUpdate
		if !decodeAssistantBody(w, r, &req) {
			return
		}
		got, err := svc.SaveSettings(r.Context(), req)
		if err != nil {
			writeReseedError(w, err)
			return
		}
		writeJSON(w, ReseedSettingsView{Settings: got, Running: worker.Running()})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// apiReseedSites handles GET /api/reseed/sites：IYUU 支持的站点与 pt-tools 站点的对照。
func (s *Server) apiReseedSites(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	_, svc, ok := s.reseedWorker(w)
	if !ok {
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

// apiReseedRun handles POST /api/reseed/run：在后台马上跑一轮，不等它结束（结果写进设置的 last_result）。
func (s *Server) apiReseedRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	worker, _, ok := s.reseedWorker(w)
	if !ok {
		return
	}
	if err := worker.RunNow(); err != nil {
		writeReseedError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]any{"started": true})
}

// apiReseedRecords handles GET /api/reseed/records：最新的辅种尝试与对应任务的状态。
func (s *Server) apiReseedRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	_, svc, ok := s.reseedWorker(w)
	if !ok {
		return
	}
	recs, err := svc.Records(r.Context())
	if err != nil {
		writeReseedError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": recs})
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
		n, err := svc.ClearFinished(r.Context(), models.JobKindReseed)
		if err != nil {
			writeTransferError(w, err)
			return
		}
		writeJSON(w, map[string]any{"deleted": n})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
