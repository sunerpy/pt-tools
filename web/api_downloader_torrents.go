package web

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
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

type DownloaderTorrentsResponse struct {
	Items    []DownloaderTorrentItem `json:"items"`
	Total    int                     `json:"total"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"page_size"`
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
	for _, rec := range records {
		dl, dlErr := dm.GetDownloader(rec.Name)
		if dlErr != nil {
			global.GetSlogger().Warnf("[DownloaderTorrents] 获取下载器失败: name=%s, err=%v", rec.Name, dlErr)
			continue
		}

		torrents, listErr := dl.GetAllTorrents()
		if listErr != nil {
			global.GetSlogger().Warnf("[DownloaderTorrents] 获取种子失败: downloader=%s, err=%v", rec.Name, listErr)
			continue
		}

		for _, t := range torrents {
			if stateFilter != "" && string(t.State) != stateFilter {
				continue
			}

			if search != "" {
				nameLower := strings.ToLower(t.Name)
				hashLower := strings.ToLower(t.InfoHash)
				dlNameLower := strings.ToLower(rec.Name)
				if !strings.Contains(nameLower, search) && !strings.Contains(hashLower, search) && !strings.Contains(dlNameLower, search) {
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

func (s *Server) apiDownloaderTorrentDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	downloaderIDStr := strings.TrimSpace(r.URL.Query().Get("downloader_id"))
	taskID := strings.TrimSpace(r.URL.Query().Get("task_id"))
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

	dl, err := dm.GetDownloader(rec.Name)
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

		dl, dlErr := dm.GetDownloader(rec.Name)
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
		dl, err := dm.GetDownloader(rec.Name)
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

		var result downloader.AddTorrentResult
		if len(torrentBytes) > 0 {
			result, err = dl.AddTorrentFileEx(torrentBytes, opt)
		} else {
			result, err = dl.AddTorrentEx(source, opt)
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

		resp.SuccessCount++
		resp.Results = append(resp.Results, AddDownloaderTorrentResult{
			DownloaderID:   rec.ID,
			DownloaderName: rec.Name,
			Success:        result.Success,
			TaskID:         result.ID,
			Hash:           result.Hash,
		})
	}

	writeJSON(w, resp)
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
		result = append(result, downloaderRecord{ID: dl.ID, Name: dl.Name, Type: dl.Type})
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
		dl, dlErr := dm.GetDownloader(rec.Name)
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
		dl, dlErr := dm.GetDownloader(rec.Name)
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
