package web

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/transfer"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

// 转移做种接口：预览、建任务、任务列表与取消、路径映射、定时规则。全部只认 session（s.auth）。

// transferAPITimeout 是预览、建任务与「立即运行」规则的总时限。
const transferAPITimeout = 2 * time.Minute

func (s *Server) registerTransferRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/transfer/preview", s.auth(s.apiTransferPreview))
	mux.HandleFunc("/api/transfer/jobs", s.auth(s.apiTransferJobs))
	mux.HandleFunc("/api/transfer/jobs/", s.auth(s.apiTransferJobDetail))
	mux.HandleFunc("/api/transfer/path-maps", s.auth(s.apiTransferPathMaps))
	mux.HandleFunc("/api/transfer/rules", s.auth(s.apiTransferRules))
	mux.HandleFunc("/api/transfer/rules/", s.auth(s.apiTransferRuleDetail))
}

// TransferRequest 是预览与建任务的请求体：转到哪台下载器、哪些种子（源下载器 + info hash）。
type TransferRequest struct {
	TargetID uint            `json:"target_id"`
	Items    []transfer.Item `json:"items"`
}

// TransferJobView 是任务列表里的一项：任务加上两台下载器的名称。
type TransferJobView struct {
	models.TorrentTransferJob
	SourceName string `json:"source_name"`
	TargetName string `json:"target_name"`
	Final      bool   `json:"final"`
}

// TransferRuleView 是规则列表里的一项。
type TransferRuleView struct {
	models.TransferRule
	SourceName string `json:"source_name"`
	TargetName string `json:"target_name"`
}

// TransferRuleRequest 是新建 / 修改规则的请求体；字段含义见 models.TransferRule。
type TransferRuleRequest struct {
	Name               string `json:"name"`
	Enabled            bool   `json:"enabled"`
	SourceDownloaderID uint   `json:"source_downloader_id"`
	TargetDownloaderID uint   `json:"target_downloader_id"`
	Category           string `json:"category"`
	Tag                string `json:"tag"`
	SiteName           string `json:"site_name"`
	MinSeedingHours    int    `json:"min_seeding_hours"`
	MaxPerRun          int    `json:"max_per_run"`
	IntervalMin        int    `json:"interval_min"`
}

// TransferPathMapsRequest 用 items 整体替换一对下载器之间的路径映射。
type TransferPathMapsRequest struct {
	SourceID uint                    `json:"source_id"`
	TargetID uint                    `json:"target_id"`
	Items    []transfer.PathMapEntry `json:"items"`
}

func (s *Server) transferService(w http.ResponseWriter) (*scheduler.TransferWorker, *transfer.Service, bool) {
	var worker *scheduler.TransferWorker
	if s != nil && s.mgr != nil {
		worker = s.mgr.GetTransferWorker()
	}
	if worker == nil || worker.Service() == nil {
		http.Error(w, "转移做种服务没有启动", http.StatusServiceUnavailable)
		return nil, nil, false
	}
	return worker, worker.Service(), true
}

func writeTransferError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, transfer.ErrInvalid), errors.Is(err, transfer.ErrRuleInvalid), errors.Is(err, transfer.ErrPathMapInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, transfer.ErrJobNotFound), errors.Is(err, transfer.ErrRuleNotFound):
		status = http.StatusNotFound
	case errors.Is(err, transfer.ErrNotCancelable), errors.Is(err, transfer.ErrRuleNameTaken):
		status = http.StatusConflict
	case errors.Is(err, transfer.ErrTargetUnavailable):
		status = http.StatusBadGateway
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
	}
	http.Error(w, err.Error(), status)
}

// downloaderNames 返回下载器 ID 到名称的对照；读不到时返回空表（名称显示为空，不影响接口）。
func downloaderNames(ctx context.Context) map[uint]string {
	out := map[uint]string{}
	if global.GlobalDB == nil {
		return out
	}
	var rows []models.DownloaderSetting
	if err := global.GlobalDB.DB.WithContext(ctx).Select("id", "name").Find(&rows).Error; err != nil {
		return out
	}
	for _, r := range rows {
		out[r.ID] = r.Name
	}
	return out
}

func transferJobViews(ctx context.Context, jobs []models.TorrentTransferJob) []TransferJobView {
	names := downloaderNames(ctx)
	out := make([]TransferJobView, 0, len(jobs))
	for _, j := range jobs {
		j.TorrentData = nil
		out = append(out, TransferJobView{
			TorrentTransferJob: j,
			SourceName:         names[j.SourceDownloaderID],
			TargetName:         names[j.TargetDownloaderID],
			Final:              models.TransferStateFinal(j.State),
		})
	}
	return out
}

func transferRuleView(ctx context.Context, r models.TransferRule, names map[uint]string) TransferRuleView {
	if names == nil {
		names = downloaderNames(ctx)
	}
	return TransferRuleView{TransferRule: r, SourceName: names[r.SourceDownloaderID], TargetName: names[r.TargetDownloaderID]}
}

// apiTransferPreview handles POST /api/transfer/preview
func (s *Server) apiTransferPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req TransferRequest
	if !decodeAssistantBody(w, r, &req) {
		return
	}
	_, svc, ok := s.transferService(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), transferAPITimeout)
	defer cancel()
	items, err := svc.Preview(ctx, req.TargetID, req.Items)
	if err != nil {
		writeTransferError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

// apiTransferJobs handles GET（列表）/ POST（建任务）/ DELETE（清除已结束的）/api/transfer/jobs
func (s *Server) apiTransferJobs(w http.ResponseWriter, r *http.Request) {
	worker, svc, ok := s.transferService(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		jobs, err := svc.ListJobs(r.Context(), r.URL.Query().Get("status"))
		if err != nil {
			writeTransferError(w, err)
			return
		}
		writeJSON(w, map[string]any{"items": transferJobViews(r.Context(), jobs)})
	case http.MethodPost:
		var req TransferRequest
		if !decodeAssistantBody(w, r, &req) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), transferAPITimeout)
		defer cancel()
		created, skipped, err := svc.Create(ctx, req.TargetID, req.Items, nil)
		if err != nil {
			writeTransferError(w, err)
			return
		}
		if len(created) > 0 {
			worker.Trigger()
		}
		writeJSON(w, map[string]any{"created": transferJobViews(r.Context(), created), "skipped": skipped})
	case http.MethodDelete:
		n, err := svc.ClearFinished(r.Context())
		if err != nil {
			writeTransferError(w, err)
			return
		}
		writeJSON(w, map[string]any{"deleted": n})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// apiTransferJobDetail handles POST /api/transfer/jobs/{id}/cancel
func (s *Server) apiTransferJobDetail(w http.ResponseWriter, r *http.Request) {
	_, svc, ok := s.transferService(w)
	if !ok {
		return
	}
	id, action, ok := parseIDAction(w, r.URL.Path, "/api/transfer/jobs/", "无效的转移任务 ID")
	if !ok {
		return
	}
	if action != "cancel" {
		http.Error(w, "未知的转移任务接口", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := svc.Cancel(r.Context(), id); err != nil {
		writeTransferError(w, err)
		return
	}
	writeJSON(w, map[string]any{"success": true})
}

// apiTransferPathMaps handles GET / PUT /api/transfer/path-maps
func (s *Server) apiTransferPathMaps(w http.ResponseWriter, r *http.Request) {
	_, svc, ok := s.transferService(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		src, _ := strconv.ParseUint(q.Get("source_id"), 10, 64)
		dst, _ := strconv.ParseUint(q.Get("target_id"), 10, 64)
		maps, err := svc.ListPathMaps(r.Context(), uint(src), uint(dst))
		if err != nil {
			writeTransferError(w, err)
			return
		}
		writeJSON(w, map[string]any{"items": maps})
	case http.MethodPut:
		var req TransferPathMapsRequest
		if !decodeAssistantBody(w, r, &req) {
			return
		}
		maps, err := svc.SavePathMaps(r.Context(), req.SourceID, req.TargetID, req.Items)
		if err != nil {
			writeTransferError(w, err)
			return
		}
		writeJSON(w, map[string]any{"items": maps})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// apiTransferRules handles GET（列表）/ POST（新建）/api/transfer/rules
func (s *Server) apiTransferRules(w http.ResponseWriter, r *http.Request) {
	_, svc, ok := s.transferService(w)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		rules, err := svc.ListRules(r.Context())
		if err != nil {
			writeTransferError(w, err)
			return
		}
		names := downloaderNames(r.Context())
		out := make([]TransferRuleView, 0, len(rules))
		for _, rule := range rules {
			out = append(out, transferRuleView(r.Context(), rule, names))
		}
		writeJSON(w, map[string]any{"items": out})
	case http.MethodPost:
		var req TransferRuleRequest
		if !decodeAssistantBody(w, r, &req) {
			return
		}
		saved, err := svc.SaveRule(r.Context(), req.toModel(0))
		if err != nil {
			writeTransferError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, transferRuleView(r.Context(), saved, nil))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// apiTransferRuleDetail handles GET / PUT / DELETE /api/transfer/rules/{id} 与 POST /api/transfer/rules/{id}/run
func (s *Server) apiTransferRuleDetail(w http.ResponseWriter, r *http.Request) {
	worker, svc, ok := s.transferService(w)
	if !ok {
		return
	}
	id, action, ok := parseIDAction(w, r.URL.Path, "/api/transfer/rules/", "无效的转移规则 ID")
	if !ok {
		return
	}
	switch action {
	case "":
		switch r.Method {
		case http.MethodGet:
			rule, err := svc.GetRule(r.Context(), id)
			if err != nil {
				writeTransferError(w, err)
				return
			}
			writeJSON(w, transferRuleView(r.Context(), rule, nil))
		case http.MethodPut:
			var req TransferRuleRequest
			if !decodeAssistantBody(w, r, &req) {
				return
			}
			saved, err := svc.SaveRule(r.Context(), req.toModel(id))
			if err != nil {
				writeTransferError(w, err)
				return
			}
			writeJSON(w, transferRuleView(r.Context(), saved, nil))
		case http.MethodDelete:
			if err := svc.DeleteRule(r.Context(), id); err != nil {
				writeTransferError(w, err)
				return
			}
			writeJSON(w, map[string]any{"success": true})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	case "run":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		rule, err := svc.GetRule(r.Context(), id)
		if err != nil {
			writeTransferError(w, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), transferAPITimeout)
		defer cancel()
		res, runErr := svc.RunRule(ctx, rule)
		summary := res.Summary()
		if runErr != nil {
			summary = "运行失败：" + runErr.Error()
		}
		if err := svc.RecordRuleRun(ctx, id, svc.Now(), summary); err != nil {
			writeTransferError(w, err)
			return
		}
		if res.Created > 0 {
			worker.Trigger()
		}
		resp := map[string]any{"result": res, "summary": summary}
		if runErr != nil {
			resp["error"] = runErr.Error()
		}
		writeJSON(w, resp)
	default:
		http.Error(w, "未知的转移规则接口", http.StatusNotFound)
	}
}

func (req TransferRuleRequest) toModel(id uint) models.TransferRule {
	return models.TransferRule{
		ID:                 id,
		Name:               req.Name,
		Enabled:            req.Enabled,
		SourceDownloaderID: req.SourceDownloaderID,
		TargetDownloaderID: req.TargetDownloaderID,
		Category:           req.Category,
		Tag:                req.Tag,
		SiteName:           req.SiteName,
		MinSeedingHours:    req.MinSeedingHours,
		MaxPerRun:          req.MaxPerRun,
		IntervalMin:        req.IntervalMin,
	}
}

// parseIDAction 把 prefix 之后的「{id}/{action}」拆开；ID 不合法时写 400。
func parseIDAction(w http.ResponseWriter, path, prefix, bad string) (uint, string, bool) {
	rest := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	idPart, action, _ := strings.Cut(rest, "/")
	id, err := strconv.ParseUint(idPart, 10, 64)
	if err != nil || id == 0 {
		http.Error(w, bad, http.StatusBadRequest)
		return 0, "", false
	}
	return uint(id), action, true
}
