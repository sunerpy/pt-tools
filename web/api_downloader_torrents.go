package web

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

type DownloaderTorrentItem struct {
	DownloaderID   uint    `json:"downloader_id"`
	DownloaderName string  `json:"downloader_name"`
	DownloaderType string  `json:"downloader_type"`
	TaskID         string  `json:"task_id"`
	InfoHash       string  `json:"info_hash"`
	Title          string  `json:"title"`
	Progress       float64 `json:"progress"`
	Seeds          int     `json:"seeds"`
	Connections    int     `json:"connections"`
	Size           int64   `json:"size"`
	AddedAt        int64   `json:"added_at"`
	CompletedAt    int64   `json:"completed_at"`
	Ratio          float64 `json:"ratio"`
	State          string  `json:"state"`
	SavePath       string  `json:"save_path"`
	Category       string  `json:"category"`
	Tags           string  `json:"tags"`
	UploadSpeed    int64   `json:"upload_speed"`
	DownloadSpeed  int64   `json:"download_speed"`
	ETA            int64   `json:"eta"`
}

// DownloaderFailure 记录一台没能取到数据的下载器。
//
// 为什么要有它：这个接口会聚合多台下载器，以前某一台连不上就静默 continue，
// 然后照样返回 200 —— 前端看到的是一份「少了一台下载器的种子」的完整列表，
// 既没法提示用户，也无从判断数字为什么不对。设计文档 §5 的 partial 态
// （「14 个站点里 12 个成功时，页面既不该整体报错也不该假装正常」）就是为这种情形定的，
// 而要表达它，响应里必须带上失败信息。
type DownloaderFailure struct {
	DownloaderID   uint   `json:"downloader_id"`
	DownloaderName string `json:"downloader_name"`
	Error          string `json:"error"`
}

type DownloaderTorrentsResponse struct {
	Items    []DownloaderTorrentItem `json:"items"`
	Total    int                     `json:"total"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"page_size"`
	// Failures 为空表示所有已启用的下载器都取到了数据。非空即 partial：
	// Items 里的数据是真的，但不完整。
	Failures []DownloaderFailure `json:"failures,omitempty"`
}

type TorrentActionTarget struct {
	DownloaderID uint   `json:"downloader_id"`
	TaskID       string `json:"task_id"`
}

type BatchTorrentActionRequest struct {
	Action   string                `json:"action"`
	Targets  []TorrentActionTarget `json:"targets"`
	SavePath string                `json:"save_path"`
}

type DownloaderCapability struct {
	DownloaderID      uint   `json:"downloader_id"`
	DownloaderName    string `json:"downloader_name"`
	DownloaderType    string `json:"downloader_type"`
	CanPause          bool   `json:"can_pause"`
	CanResume         bool   `json:"can_resume"`
	CanDelete         bool   `json:"can_delete"`
	CanDeleteWithData bool   `json:"can_delete_with_data"`
	CanSetLocation    bool   `json:"can_set_location"`
	CanAddTorrent     bool   `json:"can_add_torrent"`
	CanRecheck        bool   `json:"can_recheck"`
	CanViewFiles      bool   `json:"can_view_files"`
	CanViewTrackers   bool   `json:"can_view_trackers"`
	// CanExportTorrent 表示能把种子导出成 .torrent（qBittorrent 4.5 起；实际版本在调用时再核对）。
	CanExportTorrent bool `json:"can_export_torrent"`
	// CanEditTrackers 表示能修改种子的 tracker 地址（qBittorrent 与 Transmission 都支持）。
	CanEditTrackers bool `json:"can_edit_trackers"`
}

type DownloaderCapabilitiesResponse struct {
	Items []DownloaderCapability `json:"items"`
}

type TorrentDetailRequest struct {
	DownloaderID uint   `json:"downloader_id"`
	TaskID       string `json:"task_id"`
}

type TorrentDetailFile struct {
	Index    int     `json:"index"`
	Name     string  `json:"name"`
	Size     int64   `json:"size"`
	Progress float64 `json:"progress"`
	Priority int     `json:"priority"`
}

type TorrentDetailTracker struct {
	URL     string `json:"url"`
	Status  int    `json:"status"`
	Peers   int    `json:"peers"`
	Seeds   int    `json:"seeds"`
	Leeches int    `json:"leeches"`
	Message string `json:"message"`
}

type TorrentDetailResponse struct {
	Torrent  DownloaderTorrentItem  `json:"torrent"`
	Files    []TorrentDetailFile    `json:"files"`
	Trackers []TorrentDetailTracker `json:"trackers"`
	Features DownloaderCapability   `json:"features"`
}

type BatchTorrentActionResult struct {
	DownloaderID   uint   `json:"downloader_id"`
	DownloaderName string `json:"downloader_name"`
	TaskID         string `json:"task_id"`
	Success        bool   `json:"success"`
	Message        string `json:"message,omitempty"`
}

type BatchTorrentActionResponse struct {
	SuccessCount int                        `json:"success_count"`
	FailedCount  int                        `json:"failed_count"`
	Results      []BatchTorrentActionResult `json:"results"`
}

type AddDownloaderTorrentRequest struct {
	DownloaderIDs []uint `json:"downloader_ids"`
	SourceURL     string `json:"source_url"`
	MagnetLink    string `json:"magnet_link"`
	TorrentBase64 string `json:"torrent_base64"`
	SavePath      string `json:"save_path"`
	Category      string `json:"category"`
	Tags          string `json:"tags"`
	AddPaused     bool   `json:"add_paused"`
}

type AddDownloaderTorrentResult struct {
	DownloaderID   uint   `json:"downloader_id"`
	DownloaderName string `json:"downloader_name"`
	Success        bool   `json:"success"`
	TaskID         string `json:"task_id,omitempty"`
	Hash           string `json:"hash,omitempty"`
	Message        string `json:"message,omitempty"`
}

type AddDownloaderTorrentResponse struct {
	SuccessCount int                          `json:"success_count"`
	FailedCount  int                          `json:"failed_count"`
	Results      []AddDownloaderTorrentResult `json:"results"`
}

type downloaderRecord struct {
	ID   uint
	Name string
	Type string
	// URL 只用于「按机器缓存版本号」的键：换了地址就是换了一台机器（见 clientVersionOf）
	URL string
}

func (s *Server) apiDownloaderTorrents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	page := 1
	pageSize := 100
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil {
			if v == 0 {
				pageSize = 0
			} else if v > 0 && v <= 500 {
				pageSize = v
			}
		}
	}

	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))
	stateFilter := strings.TrimSpace(r.URL.Query().Get("state"))
	downloaderIDStr := strings.TrimSpace(r.URL.Query().Get("downloader_id"))
	sortBy := strings.TrimSpace(r.URL.Query().Get("sort_by"))
	sortOrder := strings.TrimSpace(r.URL.Query().Get("sort_order"))
	categoryFilter := strings.TrimSpace(r.URL.Query().Get("category"))
	tagFilter := strings.TrimSpace(r.URL.Query().Get("tag"))

	var filterDownloaderID *uint
	if downloaderIDStr != "" {
		id64, err := strconv.ParseUint(downloaderIDStr, 10, 64)
		if err != nil {
			http.Error(w, "无效的 downloader_id", http.StatusBadRequest)
			return
		}
		id := uint(id64)
		filterDownloaderID = &id
	}

	records, err := s.listEnabledDownloaderRecords(filterDownloaderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	dm := s.getDownloaderManager()
	if dm == nil {
		http.Error(w, "下载器管理器未初始化", http.StatusInternalServerError)
		return
	}

	items := make([]DownloaderTorrentItem, 0)
	// 逐台记录失败而不是只打日志：不上报的话前端拿到的是一份静默缺料的列表
	failures := make([]DownloaderFailure, 0)
	for _, rec := range records {
		dl, dlErr := acquireDownloader(r.Context(), dm, rec.Name)
		if dlErr != nil {
			global.GetSlogger().Warnf("[DownloaderTorrents] 获取下载器失败: name=%s, err=%v", rec.Name, dlErr)
			failures = append(failures, DownloaderFailure{
				DownloaderID: rec.ID, DownloaderName: rec.Name, Error: dlErr.Error(),
			})
			continue
		}

		torrents, listErr := dl.GetAllTorrents()
		if listErr != nil {
			global.GetSlogger().Warnf("[DownloaderTorrents] 获取种子失败: downloader=%s, err=%v", rec.Name, listErr)
			failures = append(failures, DownloaderFailure{
				DownloaderID: rec.ID, DownloaderName: rec.Name, Error: listErr.Error(),
			})
			continue
		}

		for _, t := range torrents {
			if stateFilter != "" && string(t.State) != stateFilter {
				continue
			}

			if search != "" {
				// 画板 18 的 q 写的是「搜索标题、分类、标签…」，所以分类与标签也要参与匹配：
				// 只匹配标题时那句占位文字是在许一个做不到的承诺。
				haystacks := []string{t.Name, t.InfoHash, rec.Name, t.Category, t.Tags}
				hit := false
				for _, h := range haystacks {
					if h != "" && strings.Contains(strings.ToLower(h), search) {
						hit = true
						break
					}
				}
				if !hit {
					continue
				}
			}

			if categoryFilter != "" && t.Category != categoryFilter {
				continue
			}

			if tagFilter != "" {
				tagFound := false
				for _, tag := range strings.Split(t.Tags, ",") {
					if strings.TrimSpace(tag) == tagFilter {
						tagFound = true
						break
					}
				}
				if !tagFound {
					continue
				}
			}

			progress := t.Progress
			if progress <= 1 {
				progress *= 100
			}

			items = append(items, DownloaderTorrentItem{
				DownloaderID:   rec.ID,
				DownloaderName: rec.Name,
				DownloaderType: rec.Type,
				TaskID:         t.ID,
				InfoHash:       t.InfoHash,
				Title:          t.Name,
				Progress:       progress,
				Seeds:          t.NumSeeds,
				Connections:    t.NumPeers,
				Size:           t.TotalSize,
				AddedAt:        t.DateAdded,
				CompletedAt:    t.CompletionOn,
				Ratio:          t.Ratio,
				State:          string(t.State),
				SavePath:       t.SavePath,
				Category:       t.Category,
				Tags:           t.Tags,
				UploadSpeed:    t.UploadSpeed,
				DownloadSpeed:  t.DownloadSpeed,
				ETA:            t.ETA,
			})
		}
	}

	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}
	if sortBy == "" {
		sortBy = "added_at"
	}
	sort.Slice(items, func(i, j int) bool {
		cmp := compareDownloaderTorrentItem(items[i], items[j], sortBy)
		if sortOrder == "asc" {
			return cmp < 0
		}
		return cmp > 0
	})

	total := len(items)
	start := 0
	end := total
	if pageSize > 0 {
		start = (page - 1) * pageSize
		if start > total {
			start = total
		}
		end = start + pageSize
		if end > total {
			end = total
		}
	}

	writeJSON(w, DownloaderTorrentsResponse{
		Items:    items[start:end],
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Failures: failures,
	})
}

func (s *Server) apiDownloaderCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	records, err := s.listEnabledDownloaderRecords(nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	items := make([]DownloaderCapability, 0, len(records))
	for _, rec := range records {
		items = append(items, downloaderCapabilityFromRecord(rec))
	}

	writeJSON(w, DownloaderCapabilitiesResponse{Items: items})
}

// registerDownloaderHubRoutes 注册下载器控制台那一组接口。
//
// 单独抽出来是为了让测试能走**真实的 mux**：任务详情的地址是路径形式
// `/api/downloader-torrents/{id}/{task_id}`，而处理器一度只读查询串 —— 直接调处理器的测试
// 拿查询串传参，正好绕过了这个不一致，于是「详情打不开」在测试里看不出来。
func (s *Server) registerDownloaderHubRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/downloader-torrents", s.auth(s.apiDownloaderTorrents))
	mux.HandleFunc("/api/downloader-torrents/transfer-stats", s.auth(s.apiDownloaderTransferStats))
	mux.HandleFunc("/api/downloader-torrents/capabilities", s.auth(s.apiDownloaderCapabilities))
	mux.HandleFunc("/api/downloader-torrents/meta", s.auth(s.apiDownloaderTorrentMeta))
	mux.HandleFunc("/api/downloader-torrents/batch-action", s.auth(s.apiDownloaderTorrentActions))
	mux.HandleFunc("/api/downloader-torrents/add", s.auth(s.apiAddDownloaderTorrent))
	mux.HandleFunc("/api/downloader-torrents/", s.auth(s.apiDownloaderTorrentDetail))
}

// detailTargetOf 解析任务详情的目标。
//
// 前端用的是**路径形式** `/api/downloader-torrents/{downloader_id}/{task_id}`
// （api/index.ts 的 downloaderTorrentsApi.detail），而这个处理器原来只读查询串，
// 于是真实入口恒定 400「downloader_id 和 task_id 不能为空」——「任务详情」在生产里
// 根本打不开。两头从一开始就不一致（git blame 到 214b998），之所以一直没被发现：
// 已有的成功用例直接调处理器并用查询串传参，绕过了 mux；浏览器验收那边又被假数据接住了。
//
// 两种形式都接：路径形式是前端契约，查询串形式保留给已有调用方与手工排查。
func detailTargetOf(r *http.Request) (idStr, taskID string) {
	idStr = strings.TrimSpace(r.URL.Query().Get("downloader_id"))
	taskID = strings.TrimSpace(r.URL.Query().Get("task_id"))
	if idStr != "" && taskID != "" {
		return idStr, taskID
	}

	rest := strings.TrimPrefix(r.URL.Path, "/api/downloader-torrents/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 {
		return idStr, taskID
	}
	if idStr == "" {
		idStr = strings.TrimSpace(parts[0])
	}
	if taskID == "" {
		// task_id 里可能有被转义的字符（前端用 encodeURIComponent 编过）
		if decoded, err := url.PathUnescape(parts[1]); err == nil {
			taskID = strings.TrimSpace(decoded)
		} else {
			taskID = strings.TrimSpace(parts[1])
		}
	}
	return idStr, taskID
}

func (s *Server) apiDownloaderTorrentDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	downloaderIDStr, taskID := detailTargetOf(r)
	if downloaderIDStr == "" || taskID == "" {
		http.Error(w, "downloader_id 和 task_id 不能为空", http.StatusBadRequest)
		return
	}

	id64, err := strconv.ParseUint(downloaderIDStr, 10, 64)
	if err != nil {
		http.Error(w, "无效的 downloader_id", http.StatusBadRequest)
		return
	}
	downloaderID := uint(id64)

	recordMap, err := s.getDownloaderRecordMap()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rec, ok := recordMap[downloaderID]
	if !ok {
		http.Error(w, "下载器不存在", http.StatusNotFound)
		return
	}

	dm := s.getDownloaderManager()
	if dm == nil {
		http.Error(w, "下载器管理器未初始化", http.StatusInternalServerError)
		return
	}

	dl, err := acquireDownloader(r.Context(), dm, rec.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	t, err := dl.GetTorrent(taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	files, err := dl.GetTorrentFiles(taskID)
	if err != nil {
		files = []downloader.TorrentFile{}
	}

	trackers, err := dl.GetTorrentTrackers(taskID)
	if err != nil {
		trackers = []downloader.TorrentTracker{}
	}

	progress := t.Progress
	if progress <= 1 {
		progress *= 100
	}

	resp := TorrentDetailResponse{
		Torrent: DownloaderTorrentItem{
			DownloaderID:   rec.ID,
			DownloaderName: rec.Name,
			DownloaderType: rec.Type,
			TaskID:         t.ID,
			InfoHash:       t.InfoHash,
			Title:          t.Name,
			Progress:       progress,
			Seeds:          t.NumSeeds,
			Connections:    t.NumPeers,
			Size:           t.TotalSize,
			AddedAt:        t.DateAdded,
			CompletedAt:    t.CompletionOn,
			Ratio:          t.Ratio,
			State:          string(t.State),
			SavePath:       t.SavePath,
			Category:       t.Category,
			Tags:           t.Tags,
			UploadSpeed:    t.UploadSpeed,
			DownloadSpeed:  t.DownloadSpeed,
			ETA:            t.ETA,
		},
		Files:    make([]TorrentDetailFile, 0, len(files)),
		Trackers: make([]TorrentDetailTracker, 0, len(trackers)),
		Features: downloaderCapabilityFromRecord(rec),
	}

	for _, f := range files {
		resp.Files = append(resp.Files, TorrentDetailFile{
			Index:    f.Index,
			Name:     f.Name,
			Size:     f.Size,
			Progress: f.Progress,
			Priority: f.Priority,
		})
	}

	for _, tr := range trackers {
		resp.Trackers = append(resp.Trackers, TorrentDetailTracker{
			URL:     tr.URL,
			Status:  tr.Status,
			Peers:   tr.Peers,
			Seeds:   tr.Seeds,
			Leeches: tr.Leeches,
			Message: tr.Message,
		})
	}

	writeJSON(w, resp)
}

func (s *Server) apiDownloaderTorrentActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req BatchTorrentActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(req.Targets) == 0 {
		writeJSON(w, BatchTorrentActionResponse{})
		return
	}

	action := strings.TrimSpace(req.Action)
	if action == "" {
		http.Error(w, "action 不能为空", http.StatusBadRequest)
		return
	}

	dm := s.getDownloaderManager()
	if dm == nil {
		http.Error(w, "下载器管理器未初始化", http.StatusInternalServerError)
		return
	}

	records, err := s.getDownloaderRecordMap()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := BatchTorrentActionResponse{Results: make([]BatchTorrentActionResult, 0, len(req.Targets))}
	groupedTargets := make(map[uint][]TorrentActionTarget)
	for _, target := range req.Targets {
		groupedTargets[target.DownloaderID] = append(groupedTargets[target.DownloaderID], target)
	}

	for downloaderID, targets := range groupedTargets {
		rec, ok := records[downloaderID]
		if !ok {
			for _, target := range targets {
				resp.FailedCount++
				resp.Results = append(resp.Results, BatchTorrentActionResult{
					DownloaderID: target.DownloaderID,
					TaskID:       target.TaskID,
					Success:      false,
					Message:      "下载器不存在",
				})
			}
			continue
		}

		dl, dlErr := acquireDownloader(r.Context(), dm, rec.Name)
		if dlErr != nil {
			for _, target := range targets {
				resp.FailedCount++
				resp.Results = append(resp.Results, BatchTorrentActionResult{
					DownloaderID:   target.DownloaderID,
					DownloaderName: rec.Name,
					TaskID:         target.TaskID,
					Success:        false,
					Message:        dlErr.Error(),
				})
			}
			continue
		}

		ids := make([]string, 0, len(targets))
		for _, target := range targets {
			ids = append(ids, target.TaskID)
		}

		switch action {
		case "pause", "resume", "delete", "delete_with_files":
			var batchErr error
			switch action {
			case "pause":
				batchErr = dl.PauseTorrents(ids)
			case "resume":
				batchErr = dl.ResumeTorrents(ids)
			case "delete":
				batchErr = dl.RemoveTorrents(ids, false)
			case "delete_with_files":
				batchErr = dl.RemoveTorrents(ids, true)
			}

			if batchErr == nil {
				for _, target := range targets {
					resp.SuccessCount++
					resp.Results = append(resp.Results, BatchTorrentActionResult{
						DownloaderID:   target.DownloaderID,
						DownloaderName: rec.Name,
						TaskID:         target.TaskID,
						Success:        true,
					})
				}
				continue
			}

			for _, target := range targets {
				var singleErr error
				switch action {
				case "pause":
					singleErr = dl.PauseTorrent(target.TaskID)
				case "resume":
					singleErr = dl.ResumeTorrent(target.TaskID)
				case "delete":
					singleErr = dl.RemoveTorrent(target.TaskID, false)
				case "delete_with_files":
					singleErr = dl.RemoveTorrent(target.TaskID, true)
				}

				if singleErr != nil {
					resp.FailedCount++
					resp.Results = append(resp.Results, BatchTorrentActionResult{
						DownloaderID:   target.DownloaderID,
						DownloaderName: rec.Name,
						TaskID:         target.TaskID,
						Success:        false,
						Message:        singleErr.Error(),
					})
					continue
				}

				resp.SuccessCount++
				resp.Results = append(resp.Results, BatchTorrentActionResult{
					DownloaderID:   target.DownloaderID,
					DownloaderName: rec.Name,
					TaskID:         target.TaskID,
					Success:        true,
				})
			}
		case "set_location", "recheck":
			for _, target := range targets {
				var opErr error
				switch action {
				case "set_location":
					if strings.TrimSpace(req.SavePath) == "" {
						opErr = downloader.ErrInvalidConfig
					} else {
						opErr = dl.SetTorrentSavePath(target.TaskID, req.SavePath)
					}
				case "recheck":
					opErr = dl.RecheckTorrent(target.TaskID)
				}

				if opErr != nil {
					resp.FailedCount++
					resp.Results = append(resp.Results, BatchTorrentActionResult{
						DownloaderID:   target.DownloaderID,
						DownloaderName: rec.Name,
						TaskID:         target.TaskID,
						Success:        false,
						Message:        opErr.Error(),
					})
					continue
				}

				resp.SuccessCount++
				resp.Results = append(resp.Results, BatchTorrentActionResult{
					DownloaderID:   target.DownloaderID,
					DownloaderName: rec.Name,
					TaskID:         target.TaskID,
					Success:        true,
				})
			}
		default:
			http.Error(w, "不支持的 action", http.StatusBadRequest)
			return
		}
	}

	writeJSON(w, resp)
}

func (s *Server) apiAddDownloaderTorrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req AddDownloaderTorrentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.SourceURL) == "" && strings.TrimSpace(req.MagnetLink) == "" && strings.TrimSpace(req.TorrentBase64) == "" {
		http.Error(w, "请提供 source_url、magnet_link 或 torrent_base64", http.StatusBadRequest)
		return
	}

	dm := s.getDownloaderManager()
	if dm == nil {
		http.Error(w, "下载器管理器未初始化", http.StatusInternalServerError)
		return
	}

	var records []downloaderRecord
	if len(req.DownloaderIDs) > 0 {
		recMap, err := s.getDownloaderRecordMap()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, id := range req.DownloaderIDs {
			rec, ok := recMap[id]
			if ok {
				records = append(records, rec)
			}
		}
	} else {
		allRecords, err := s.listEnabledDownloaderRecords(nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		records = allRecords
	}

	if len(records) == 0 {
		http.Error(w, "没有可用下载器", http.StatusBadRequest)
		return
	}

	opt := downloader.AddTorrentOptions{
		AddAtPaused: req.AddPaused,
		SavePath:    req.SavePath,
		Category:    req.Category,
		Tags:        req.Tags,
	}

	var torrentBytes []byte
	if strings.TrimSpace(req.TorrentBase64) != "" {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.TorrentBase64))
		if err != nil {
			http.Error(w, "torrent_base64 无效", http.StatusBadRequest)
			return
		}
		torrentBytes = decoded
	}

	source := strings.TrimSpace(req.MagnetLink)
	if source == "" {
		source = strings.TrimSpace(req.SourceURL)
	}

	resp := AddDownloaderTorrentResponse{Results: make([]AddDownloaderTorrentResult, 0, len(records))}
	for _, rec := range records {
		dl, err := acquireDownloader(r.Context(), dm, rec.Name)
		if err != nil {
			resp.FailedCount++
			resp.Results = append(resp.Results, AddDownloaderTorrentResult{
				DownloaderID:   rec.ID,
				DownloaderName: rec.Name,
				Success:        false,
				Message:        err.Error(),
			})
			continue
		}
		reservedSize, guardErr := reserveDownloaderAddDiskBudget(r.Context(), dl, torrentBytes, source)
		if guardErr != nil {
			resp.FailedCount++
			resp.Results = append(resp.Results, AddDownloaderTorrentResult{
				DownloaderID:   rec.ID,
				DownloaderName: rec.Name,
				Success:        false,
				Message:        guardErr.Error(),
			})
			continue
		}

		var result downloader.AddTorrentResult
		if len(torrentBytes) > 0 {
			result, err = dl.AddTorrentFileEx(torrentBytes, opt)
		} else {
			result, err = dl.AddTorrentEx(source, opt)
		}
		if err != nil || !result.Success {
			if reservedSize > 0 {
				ptinternal.GetDiskBudget().Release(reservedSize)
			}
		}

		if err != nil {
			resp.FailedCount++
			resp.Results = append(resp.Results, AddDownloaderTorrentResult{
				DownloaderID:   rec.ID,
				DownloaderName: rec.Name,
				Success:        false,
				Message:        err.Error(),
			})
			continue
		}

		if result.Success {
			resp.SuccessCount++
		} else {
			resp.FailedCount++
		}
		resp.Results = append(resp.Results, AddDownloaderTorrentResult{
			DownloaderID:   rec.ID,
			DownloaderName: rec.Name,
			Success:        result.Success,
			TaskID:         result.ID,
			Hash:           result.Hash,
			Message:        fmt.Sprint(result.Message),
		})
	}

	writeJSON(w, resp)
}

// reserveDownloaderAddDiskBudget executes the disk-protect gate for the manual
// "add torrent" endpoint. Returns the reserved torrent size on success (caller
// is responsible for Releasing it on failure paths). When disk protection is
// disabled, returns (0, nil) and the caller must NOT call Release.
//
// Magnet / URL sources cannot be sized before the metadata is fetched, so they
// are rejected when disk protection is on. This closes a bypass where a user
// could submit a magnet and skip the gate entirely.
func reserveDownloaderAddDiskBudget(ctx context.Context, dl downloader.Downloader, torrentBytes []byte, source string) (int64, error) {
	glOnly, err := core.NewConfigStore(global.GlobalDB).GetGlobalOnly()
	if err != nil || !glOnly.CleanupDiskProtect || glOnly.CleanupMinDiskSpaceGB <= 0 {
		return 0, nil
	}

	if len(torrentBytes) == 0 {
		return 0, fmt.Errorf("[磁盘保护] 已启用，无法在添加前确定 magnet/URL 种子大小: %s。请改用 .torrent 文件上传，或在全局设置中关闭磁盘保护。", source)
	}

	torrentSize, sizeErr := qbit.ComputeTorrentSize(torrentBytes)
	if sizeErr != nil {
		return 0, fmt.Errorf("种子文件无效，无法解析大小: %w", sizeErr)
	}

	mu := ptinternal.PushMutex()
	mu.Lock()
	defer mu.Unlock()

	freeSpace, spaceErr := dl.GetClientFreeSpace(ctx)
	if spaceErr != nil {
		return 0, fmt.Errorf("[磁盘保护] %s: 获取磁盘空间失败，拒绝添加: %w", dl.GetName(), spaceErr)
	}
	pendingBytes, pendingErr := dl.GetIncompletePendingBytes(ctx)
	if pendingErr != nil {
		pendingBytes = 0
	}
	budget := ptinternal.GetDiskBudget()
	effectiveFreeBytes := freeSpace - pendingBytes - budget.Reserved()
	if effectiveFreeBytes < 0 {
		effectiveFreeBytes = 0
	}
	minBytes := int64(glOnly.CleanupMinDiskSpaceGB * 1024 * 1024 * 1024)
	if effectiveFreeBytes-torrentSize < minBytes {
		effGB := float64(effectiveFreeBytes) / (1024 * 1024 * 1024)
		tGB := float64(torrentSize) / (1024 * 1024 * 1024)
		freeGB := float64(freeSpace) / (1024 * 1024 * 1024)
		pendingGB := float64(pendingBytes) / (1024 * 1024 * 1024)
		reservedGB := float64(budget.Reserved()) / (1024 * 1024 * 1024)
		global.GetSlogger().Warnf("[磁盘保护] %s: 空间不足 (qBit可用 %.1f GB - 下载中待占用 %.1f GB - 本进程预留 %.1f GB = 有效 %.1f GB；有效 - 种子 %.1f GB < 保底 %.1f GB)，跳过手动添加",
			dl.GetName(), freeGB, pendingGB, reservedGB, effGB, tGB, glOnly.CleanupMinDiskSpaceGB)
		if glOnly.CleanupEnabled {
			events.Publish(events.Event{Type: events.DiskSpaceLow, Source: "downloader-add", At: time.Now()})
		}
		return 0, fmt.Errorf("磁盘空间不足 (有效 %.1f GB - 种子 %.1f GB < %.1f GB)", effGB, tGB, glOnly.CleanupMinDiskSpaceGB)
	}
	if torrentSize > 0 {
		budget.Reserve(torrentSize)
	}
	return torrentSize, nil
}

// downloaderAcquireTimeout 是 Web 请求获取下载器实例的预算上限。
//
// 下载器连不上时，manager 会在后台按重连策略跑最长约 31s 的退避序列。HTTP 请求
// 不能等它：前端每 30s 全局轮询一次，浏览器对同一来源只有 6 个并发连接，几个卡住
// 的请求就能把连接池占满，整个页面随之取不到任何数据。所以这里只给一个短预算，
// 拿不到就跳过这台下载器，让后台那次尝试自己跑完并缓存结果。
const downloaderAcquireTimeout = 3 * time.Second

// acquireDownloader 在有界预算内获取下载器实例，供所有 HTTP 处理器使用。
//
// 与 dm.GetDownloader 的区别只在「等多久」：已就绪的实例照样瞬时返回，
// 需要新建连时最多等 downloaderAcquireTimeout，也受请求自身取消的约束。
func acquireDownloader(
	ctx context.Context,
	dm *downloader.DownloaderManager,
	name string,
) (downloader.Downloader, error) {
	acquireCtx, cancel := context.WithTimeout(ctx, downloaderAcquireTimeout)
	defer cancel()
	return dm.GetDownloaderContext(acquireCtx, name)
}

func (s *Server) getDownloaderRecordMap() (map[uint]downloaderRecord, error) {
	var settings []models.DownloaderSetting
	if err := global.GlobalDB.DB.Where("enabled = ?", true).Find(&settings).Error; err != nil {
		return nil, err
	}

	result := make(map[uint]downloaderRecord, len(settings))
	for _, dl := range settings {
		result[dl.ID] = downloaderRecord{ID: dl.ID, Name: dl.Name, Type: dl.Type}
	}
	return result, nil
}

func (s *Server) listEnabledDownloaderRecords(filterID *uint) ([]downloaderRecord, error) {
	var settings []models.DownloaderSetting
	tx := global.GlobalDB.DB.Where("enabled = ?", true)
	if filterID != nil {
		tx = tx.Where("id = ?", *filterID)
	}
	if err := tx.Find(&settings).Error; err != nil {
		return nil, err
	}

	result := make([]downloaderRecord, 0, len(settings))
	for _, dl := range settings {
		result = append(result, downloaderRecord{ID: dl.ID, Name: dl.Name, Type: dl.Type, URL: dl.URL})
	}
	return result, nil
}

func downloaderCapabilityFromRecord(rec downloaderRecord) DownloaderCapability {
	return DownloaderCapability{
		DownloaderID:      rec.ID,
		DownloaderName:    rec.Name,
		DownloaderType:    rec.Type,
		CanPause:          true,
		CanResume:         true,
		CanDelete:         true,
		CanDeleteWithData: true,
		CanSetLocation:    true,
		CanAddTorrent:     true,
		CanRecheck:        true,
		CanViewFiles:      true,
		CanViewTrackers:   true,
		CanExportTorrent:  rec.Type == string(downloader.DownloaderQBittorrent),
		CanEditTrackers:   true,
	}
}

func compareDownloaderTorrentItem(a, b DownloaderTorrentItem, sortBy string) int {
	switch sortBy {
	case "downloader_name":
		return strings.Compare(strings.ToLower(a.DownloaderName), strings.ToLower(b.DownloaderName))
	case "downloader_type":
		return strings.Compare(strings.ToLower(a.DownloaderType), strings.ToLower(b.DownloaderType))
	case "title":
		return strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
	case "progress":
		return compareFloat64(a.Progress, b.Progress)
	case "seeds":
		return compareInt(a.Seeds, b.Seeds)
	case "connections":
		return compareInt(a.Connections, b.Connections)
	case "size":
		return compareInt64(a.Size, b.Size)
	case "upload_speed":
		return compareInt64(a.UploadSpeed, b.UploadSpeed)
	case "download_speed":
		return compareInt64(a.DownloadSpeed, b.DownloadSpeed)
	case "added_at":
		return compareInt64(a.AddedAt, b.AddedAt)
	case "completed_at":
		return compareInt64(a.CompletedAt, b.CompletedAt)
	case "ratio":
		return compareFloat64(a.Ratio, b.Ratio)
	case "state":
		return strings.Compare(strings.ToLower(a.State), strings.ToLower(b.State))
	case "eta":
		return compareInt64(a.ETA, b.ETA)
	default:
		return compareInt64(a.AddedAt, b.AddedAt)
	}
}

func compareInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func compareInt64(a, b int64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func compareFloat64(a, b float64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

type DownloaderTorrentMetaResponse struct {
	Categories []string `json:"categories"`
	Tags       []string `json:"tags"`
}

func (s *Server) apiDownloaderTorrentMeta(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	records, err := s.listEnabledDownloaderRecords(nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	dm := s.getDownloaderManager()
	if dm == nil {
		http.Error(w, "下载器管理器未初始化", http.StatusInternalServerError)
		return
	}

	categorySet := make(map[string]struct{})
	tagSet := make(map[string]struct{})

	for _, rec := range records {
		dl, dlErr := acquireDownloader(r.Context(), dm, rec.Name)
		if dlErr != nil {
			continue
		}

		torrents, listErr := dl.GetAllTorrents()
		if listErr != nil {
			continue
		}

		for _, t := range torrents {
			if t.Category != "" {
				categorySet[t.Category] = struct{}{}
			}
			if t.Tags != "" {
				for _, tag := range strings.Split(t.Tags, ",") {
					trimmed := strings.TrimSpace(tag)
					if trimmed != "" {
						tagSet[trimmed] = struct{}{}
					}
				}
			}
		}
	}

	categories := make([]string, 0, len(categorySet))
	for cat := range categorySet {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	writeJSON(w, DownloaderTorrentMetaResponse{
		Categories: categories,
		Tags:       tags,
	})
}

type DownloaderTransferStatsResponse struct {
	TotalUploadSpeed       int64                        `json:"total_upload_speed"`
	TotalDownloadSpeed     int64                        `json:"total_download_speed"`
	TotalUploaded          int64                        `json:"total_uploaded"`
	TotalDownloaded        int64                        `json:"total_downloaded"`
	TotalSessionUploaded   int64                        `json:"total_session_uploaded"`
	TotalSessionDownloaded int64                        `json:"total_session_downloaded"`
	TotalFreeSpace         int64                        `json:"total_free_space"`
	Downloaders            []DownloaderTransferStatItem `json:"downloaders"`
}

type DownloaderTransferStatItem struct {
	DownloaderID      uint   `json:"downloader_id"`
	DownloaderName    string `json:"downloader_name"`
	DownloaderType    string `json:"downloader_type"`
	UploadSpeed       int64  `json:"upload_speed"`
	DownloadSpeed     int64  `json:"download_speed"`
	Uploaded          int64  `json:"uploaded"`
	Downloaded        int64  `json:"downloaded"`
	SessionUploaded   int64  `json:"session_uploaded"`
	SessionDownloaded int64  `json:"session_downloaded"`
	FreeSpace         int64  `json:"free_space"`
	// Reachable 表示这一轮是否真的从这台下载器取到了数据（状态或剩余空间任一成功）。
	//
	// 为什么必须显式给出：本条目**只要能取到实例就会被追加**，取数失败时各字段留零值。
	// 于是「出现在 downloaders 里」只证明实例构造成功，不证明客户端连得上 ——
	// acquireDownloader 可能命中缓存实例，Transmission 的实现在普通 RPC 网络失败后
	// 也不会清掉缓存的 healthy 标志。前端曾据此显示「已连接」，那是会说谎的。
	Reachable bool `json:"reachable"`
	// Error 是这一轮失败的原因（取到数据时为空）。给人看的诊断，不参与聚合。
	Error string `json:"error,omitempty"`
	// ClientVersion 是下载器自报的版本（画板 41 状态栏那格的第三段）。
	//
	// 取不到就留空：它是背景信息，不该让这个接口因为多问一句版本而变慢或失败。
	// 值在 Server 上按「id + URL」缓存，所以每台只在第一次连上时问一次。
	ClientVersion string `json:"client_version,omitempty"`
}

// clientVersionOf 返回下载器自报的版本，带缓存。
//
// 缓存键带 URL：换了地址就是换了一台机器，旧版本号不能再用。
// 两个实现（qBittorrent 的 /api/v2/app/version、Transmission 的 session-get）都是一次
// 真实 HTTP 往返且自己不缓存，而调用方是 30 秒一拍的 transfer-stats —— 不缓存就等于
// 给每拍加一次往返。版本只在用户升级下载器时才变，缓存到进程结束足够。
func (s *Server) clientVersionOf(id uint, url string, dl downloader.Downloader) string {
	key := fmt.Sprintf("%d|%s", id, url)

	s.clientVersionMu.RLock()
	cached, ok := s.clientVersions[key]
	s.clientVersionMu.RUnlock()
	if ok {
		return cached
	}

	v, err := dl.GetClientVersion()
	if err != nil || strings.TrimSpace(v) == "" {
		// 不缓存失败：下次这台连上了应该能问到
		return ""
	}
	v = strings.TrimSpace(v)

	s.clientVersionMu.Lock()
	s.clientVersions[key] = v
	s.clientVersionMu.Unlock()
	return v
}

func (s *Server) apiDownloaderTransferStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	records, err := s.listEnabledDownloaderRecords(nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	dm := s.getDownloaderManager()
	if dm == nil {
		http.Error(w, "下载器管理器未初始化", http.StatusInternalServerError)
		return
	}

	resp := DownloaderTransferStatsResponse{
		Downloaders: make([]DownloaderTransferStatItem, 0, len(records)),
	}

	ctx := r.Context()
	for _, rec := range records {
		dl, dlErr := acquireDownloader(ctx, dm, rec.Name)
		if dlErr != nil {
			continue
		}

		item := DownloaderTransferStatItem{
			DownloaderID:   rec.ID,
			DownloaderName: rec.Name,
			DownloaderType: rec.Type,
		}

		status, statusErr := dl.GetClientStatus()
		if statusErr == nil {
			item.UploadSpeed = status.UpSpeed
			item.DownloadSpeed = status.DlSpeed
			item.Uploaded = status.UpData
			item.Downloaded = status.DlData
			item.SessionUploaded = status.SessionUpData
			item.SessionDownloaded = status.SessionDlData
		}

		freeSpace, fsErr := dl.GetClientFreeSpace(ctx)
		if fsErr == nil {
			item.FreeSpace = freeSpace
		}

		// 两个探测任一成功就算连得上：有的客户端（或权限配置）拿不到剩余空间，
		// 但状态照样能回，那台机器是活的。两个都失败才是「这一轮没连上」。
		item.Reachable = statusErr == nil || fsErr == nil
		if !item.Reachable {
			item.Error = statusErr.Error()
		} else {
			// 只在连得上的时候问版本，且问到就缓存 —— 断线的那台不必为这一格再等一次超时
			item.ClientVersion = s.clientVersionOf(rec.ID, rec.URL, dl)
		}

		resp.TotalUploadSpeed += item.UploadSpeed
		resp.TotalDownloadSpeed += item.DownloadSpeed
		resp.TotalUploaded += item.Uploaded
		resp.TotalDownloaded += item.Downloaded
		resp.TotalSessionUploaded += item.SessionUploaded
		resp.TotalSessionDownloaded += item.SessionDownloaded
		resp.TotalFreeSpace += item.FreeSpace
		resp.Downloaders = append(resp.Downloaders, item)
	}

	writeJSON(w, resp)
}
