package web

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/sunerpy/pt-tools/internal/freewatch"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

// 等免费再下（媒体中心 M2b）：搜索结果里点「等免费」建一条，后台定时读优惠、免费了才推送。只认会话。

func (s *Server) registerFreeWatchRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/free-watches", s.auth(s.apiFreeWatches))
	mux.HandleFunc("POST /api/free-watches", s.auth(s.apiFreeWatchCreate))
	mux.HandleFunc("GET /api/free-watches/sites", s.auth(s.apiFreeWatchSites))
	mux.HandleFunc("PATCH /api/free-watches/{id}", s.auth(s.apiFreeWatchUpdate))
	mux.HandleFunc("POST /api/free-watches/{id}/cancel", s.auth(s.apiFreeWatchCancel))
}

func (s *Server) freeWatchService(w http.ResponseWriter) (*scheduler.FreeWatchWorker, *freewatch.Service, bool) {
	var worker *scheduler.FreeWatchWorker
	if s != nil && s.mgr != nil {
		worker = s.mgr.GetFreeWatchWorker()
	}
	if worker == nil || worker.Service() == nil {
		http.Error(w, "等免费服务没有启动", http.StatusServiceUnavailable)
		return nil, nil, false
	}
	return worker, worker.Service(), true
}

func writeFreeWatchError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, freewatch.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, freewatch.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
	}
	http.Error(w, err.Error(), status)
}

var freeWatchStatuses = []string{
	models.FreeWatchWatching, models.FreeWatchPushed, models.FreeWatchExpired, models.FreeWatchCanceled, models.FreeWatchFailed,
}

// apiFreeWatches handles GET /api/free-watches?status=：记录（新建的在前）与各状态的条数。
func (s *Server) apiFreeWatches(w http.ResponseWriter, r *http.Request) {
	_, svc, ok := s.freeWatchService(w)
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	if status != "" && !slices.Contains(freeWatchStatuses, status) {
		http.Error(w, "状态不对", http.StatusBadRequest)
		return
	}
	rows, counts, err := svc.List(r.Context(), status)
	if err != nil {
		writeFreeWatchError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": rows, "counts": counts})
}

// apiFreeWatchCreate handles POST /api/free-watches：新建一条（见 freewatch.CreateInput），建好马上检查一次。
func (s *Server) apiFreeWatchCreate(w http.ResponseWriter, r *http.Request) {
	worker, svc, ok := s.freeWatchService(w)
	if !ok {
		return
	}
	var in freewatch.CreateInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	row, err := svc.Create(r.Context(), in)
	if err != nil {
		writeFreeWatchError(w, err)
		return
	}
	worker.Trigger()
	writeJSON(w, row)
}

// apiFreeWatchSites handles GET /api/free-watches/sites：能等免费的站点（启用着、能读种子详情）。
func (s *Server) apiFreeWatchSites(w http.ResponseWriter, r *http.Request) {
	_, svc, ok := s.freeWatchService(w)
	if !ok {
		return
	}
	out := []string{}
	if searchOrchestrator != nil {
		enabled := s.getEnabledSiteIDs()
		for _, name := range filterEnabledSites(searchOrchestrator.ListSites(), enabled) {
			if svc.Supports(name) {
				out = append(out, name)
			}
		}
	}
	slices.Sort(out)
	writeJSON(w, map[string]any{"sites": out})
}

// freeWatchUpdate 是改一条在等的：现在只能改到期时间。
type freeWatchUpdate struct {
	ExpiresAt time.Time `json:"expires_at"`
}

// apiFreeWatchUpdate handles PATCH /api/free-watches/{id} {expires_at}：改到期时间。
func (s *Server) apiFreeWatchUpdate(w http.ResponseWriter, r *http.Request) {
	_, svc, ok := s.freeWatchService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in freeWatchUpdate
	if !decodeStrictBody(w, r, &in) {
		return
	}
	row, err := svc.SetExpiry(r.Context(), id, in.ExpiresAt)
	if err != nil {
		writeFreeWatchError(w, err)
		return
	}
	writeJSON(w, row)
}

// apiFreeWatchCancel handles POST /api/free-watches/{id}/cancel：取消一条在等的。
func (s *Server) apiFreeWatchCancel(w http.ResponseWriter, r *http.Request) {
	_, svc, ok := s.freeWatchService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	row, err := svc.Cancel(r.Context(), id)
	if err != nil {
		writeFreeWatchError(w, err)
		return
	}
	writeJSON(w, row)
}
