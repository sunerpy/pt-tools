package web

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// App API v1 的下载器种子：列表与暂停、继续、删除。

// AppTorrent 是下载器里的一个种子。
type AppTorrent struct {
	DownloaderID  uint    `json:"downloader_id"`
	Downloader    string  `json:"downloader"`
	TaskID        string  `json:"task_id"`
	InfoHash      string  `json:"info_hash"`
	Title         string  `json:"title"`
	Progress      float64 `json:"progress"`
	Size          int64   `json:"size"`
	State         string  `json:"state"`
	Ratio         float64 `json:"ratio"`
	Seeds         int     `json:"seeds"`
	Peers         int     `json:"peers"`
	UploadSpeed   int64   `json:"upload_speed"`
	DownloadSpeed int64   `json:"download_speed"`
	ETA           int64   `json:"eta"`
	AddedAt       int64   `json:"added_at"`
	CompletedAt   int64   `json:"completed_at"`
	Category      string  `json:"category,omitempty"`
	Tags          string  `json:"tags,omitempty"`
	SavePath      string  `json:"save_path,omitempty"`
}

// AppDownloaderFailure 是一台没取到种子的下载器。
type AppDownloaderFailure struct {
	DownloaderID uint   `json:"downloader_id"`
	Downloader   string `json:"downloader"`
	Error        string `json:"error"`
}

// AppTorrentPage 是 GET /torrents 的回应：Failures 不为空时列表不完整（有下载器没取到）。
type AppTorrentPage struct {
	Items    []AppTorrent           `json:"items"`
	Total    int                    `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
	Failures []AppDownloaderFailure `json:"failures"`
}

var appTorrentSorts = []string{"added_at", "completed_at", "title", "progress", "size", "ratio", "state", "upload_speed", "download_speed", "eta", "seeds"}

func (s *Server) appTorrents(w http.ResponseWriter, r *http.Request) {
	page, size, ok := appPaging(r, 200)
	if !ok {
		appError(w, http.StatusBadRequest, "invalid_argument", "page 从 1 开始，page_size 在 1 到 200 之间")
		return
	}
	qs := r.URL.Query()
	q := torrentQuery{
		Page: page, PageSize: size, Search: strings.ToLower(strings.TrimSpace(qs.Get("q"))), State: strings.TrimSpace(qs.Get("state")),
		SortBy: strings.TrimSpace(qs.Get("sort")), SortOrder: strings.TrimSpace(qs.Get("order")),
	}
	if len(q.Search) > 200 {
		appError(w, http.StatusBadRequest, "invalid_argument", "关键字太长")
		return
	}
	if q.SortBy != "" && !slices.Contains(appTorrentSorts, q.SortBy) {
		appError(w, http.StatusBadRequest, "invalid_argument", "sort 要选 "+strings.Join(appTorrentSorts, "、"))
		return
	}
	if q.SortOrder != "" && q.SortOrder != "asc" && q.SortOrder != "desc" {
		appError(w, http.StatusBadRequest, "invalid_argument", "order 要选 asc 或 desc")
		return
	}
	if v := qs.Get("downloader_id"); v != "" {
		id, err := strconv.ParseUint(v, 10, 64)
		if err != nil || id == 0 {
			appError(w, http.StatusBadRequest, "invalid_argument", "downloader_id 不对")
			return
		}
		u := uint(id)
		q.DownloaderID = &u
	}
	resp, err := s.collectDownloaderTorrents(r.Context(), q)
	if err != nil {
		appError(w, http.StatusInternalServerError, "internal", appRedactAddr(err.Error()))
		return
	}
	out := AppTorrentPage{Items: make([]AppTorrent, 0, len(resp.Items)), Total: resp.Total, Page: resp.Page, PageSize: resp.PageSize, Failures: []AppDownloaderFailure{}}
	for _, t := range resp.Items {
		out.Items = append(out.Items, AppTorrent{
			DownloaderID: t.DownloaderID, Downloader: t.DownloaderName, TaskID: t.TaskID, InfoHash: t.InfoHash, Title: t.Title,
			Progress: t.Progress, Size: t.Size, State: t.State, Ratio: t.Ratio, Seeds: t.Seeds, Peers: t.Connections,
			UploadSpeed: t.UploadSpeed, DownloadSpeed: t.DownloadSpeed, ETA: t.ETA, AddedAt: t.AddedAt, CompletedAt: t.CompletedAt,
			Category: t.Category, Tags: t.Tags, SavePath: t.SavePath,
		})
	}
	for _, f := range resp.Failures {
		out.Failures = append(out.Failures, AppDownloaderFailure{DownloaderID: f.DownloaderID, Downloader: f.DownloaderName, Error: appRedactAddr(f.Error)})
	}
	appJSON(w, out)
}

// AppTorrentActionsRequest 是 POST /torrents/actions：对一批种子暂停、继续、删除（只删种子）或连数据删除。
type AppTorrentActionsRequest struct {
	Action  string                `json:"action"`
	Targets []TorrentActionTarget `json:"targets"`
}

// AppTorrentActionResult 是一个种子的结果。
type AppTorrentActionResult struct {
	DownloaderID uint   `json:"downloader_id"`
	TaskID       string `json:"task_id"`
	Success      bool   `json:"success"`
	Message      string `json:"message,omitempty"`
}

// AppTorrentActionsResult 是 POST /torrents/actions 的回应。
type AppTorrentActionsResult struct {
	Succeeded int                      `json:"succeeded"`
	Failed    int                      `json:"failed"`
	Results   []AppTorrentActionResult `json:"results"`
}

var appTorrentActions = []string{"pause", "resume", "delete", "delete_with_files"}

const appMaxTargets = 100

func (s *Server) appTorrentActions(w http.ResponseWriter, r *http.Request) {
	var req AppTorrentActionsRequest
	if !appDecode(w, r, &req) {
		return
	}
	if !slices.Contains(appTorrentActions, req.Action) {
		appError(w, http.StatusBadRequest, "invalid_argument", "action 要选 "+strings.Join(appTorrentActions, "、"))
		return
	}
	if len(req.Targets) == 0 || len(req.Targets) > appMaxTargets {
		appError(w, http.StatusBadRequest, "invalid_argument", "targets 要有 1 到 100 个")
		return
	}
	for _, t := range req.Targets {
		if t.DownloaderID == 0 || strings.TrimSpace(t.TaskID) == "" || len(t.TaskID) > 128 {
			appError(w, http.StatusBadRequest, "invalid_argument", "每个 target 要有 downloader_id 与 task_id")
			return
		}
	}
	resp, err := s.runTorrentActions(r.Context(), BatchTorrentActionRequest{Action: req.Action, Targets: req.Targets})
	if errors.Is(err, errUnsupportedAction) {
		appError(w, http.StatusBadRequest, "invalid_argument", err.Error())
		return
	}
	if err != nil {
		appError(w, http.StatusInternalServerError, "internal", appRedactAddr(err.Error()))
		return
	}
	out := AppTorrentActionsResult{Succeeded: resp.SuccessCount, Failed: resp.FailedCount, Results: make([]AppTorrentActionResult, 0, len(resp.Results))}
	switch {
	case out.Failed > 0 && out.Succeeded == 0:
		appSetOutcome(r, "error:all_failed")
	case out.Failed > 0:
		appSetOutcome(r, "error:partial")
	}
	for _, res := range resp.Results {
		out.Results = append(out.Results, AppTorrentActionResult{DownloaderID: res.DownloaderID, TaskID: res.TaskID, Success: res.Success, Message: appRedactAddr(res.Message)})
	}
	appJSON(w, out)
}
