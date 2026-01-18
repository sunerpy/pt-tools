package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// PausedTorrentResponse 暂停种子响应结构
type PausedTorrentResponse struct {
	ID               uint       `json:"id"`
	SiteName         string     `json:"site_name"`
	Title            string     `json:"title"`
	TorrentHash      *string    `json:"torrent_hash,omitempty"`
	Progress         float64    `json:"progress"`
	TorrentSize      int64      `json:"torrent_size"`
	DownloaderName   string     `json:"downloader_name"`
	DownloaderTaskID string     `json:"downloader_task_id"`
	PausedAt         *time.Time `json:"paused_at"`
	PauseReason      string     `json:"pause_reason"`
	FreeEndTime      *time.Time `json:"free_end_time,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// DeletePausedRequest 删除暂停种子请求
type DeletePausedRequest struct {
	IDs        []uint `json:"ids"`         // 指定删除的种子 ID 列表（为空则删除全部）
	RemoveData bool   `json:"remove_data"` // 是否同时删除数据文件
}

// DeletePausedResponse 删除暂停种子响应
type DeletePausedResponse struct {
	Success      int      `json:"success"`       // 成功删除数量
	Failed       int      `json:"failed"`        // 失败数量
	FailedIDs    []uint   `json:"failed_ids"`    // 失败的种子 ID
	FailedErrors []string `json:"failed_errors"` // 失败原因
}

// ArchiveTorrentResponse 归档种子响应结构
type ArchiveTorrentResponse struct {
	ID                uint       `json:"id"`
	OriginalID        uint       `json:"original_id"`
	SiteName          string     `json:"site_name"`
	Title             string     `json:"title"`
	TorrentHash       *string    `json:"torrent_hash,omitempty"`
	IsFree            bool       `json:"is_free"`
	FreeEndTime       *time.Time `json:"free_end_time,omitempty"`
	IsCompleted       bool       `json:"is_completed"`
	Progress          float64    `json:"progress"`
	IsPausedBySystem  bool       `json:"is_paused_by_system"`
	PauseReason       string     `json:"pause_reason,omitempty"`
	DownloaderName    string     `json:"downloader_name,omitempty"`
	OriginalCreatedAt time.Time  `json:"original_created_at"`
	ArchivedAt        time.Time  `json:"archived_at"`
}

// ResumeTorrentResponse 恢复种子响应
type ResumeTorrentResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// apiPausedTorrents 获取已暂停种子列表
// GET /api/torrents/paused
func (s *Server) apiPausedTorrents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// 解析分页参数
	page := 1
	pageSize := 50
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 && v <= 200 {
			pageSize = v
		}
	}

	// 可选的站点过滤
	siteFilter := r.URL.Query().Get("site")

	db := global.GlobalDB.DB
	tx := db.Model(&models.TorrentInfo{}).Where("is_paused_by_system = ?", true)

	if siteFilter != "" {
		tx = tx.Where("site_name = ?", siteFilter)
	}

	// 计数
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		http.Error(w, "查询失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 查询
	var torrents []models.TorrentInfo
	if err := tx.Order("paused_at DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&torrents).Error; err != nil {
		http.Error(w, "查询失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 转换为响应结构
	resp := make([]PausedTorrentResponse, 0, len(torrents))
	for _, t := range torrents {
		resp = append(resp, PausedTorrentResponse{
			ID:               t.ID,
			SiteName:         t.SiteName,
			Title:            t.Title,
			TorrentHash:      t.TorrentHash,
			Progress:         t.Progress,
			TorrentSize:      t.TorrentSize,
			DownloaderName:   t.DownloaderName,
			DownloaderTaskID: t.DownloaderTaskID,
			PausedAt:         t.PausedAt,
			PauseReason:      t.PauseReason,
			FreeEndTime:      t.FreeEndTime,
			CreatedAt:        t.CreatedAt,
		})
	}

	writeJSON(w, struct {
		Items    []PausedTorrentResponse `json:"items"`
		Total    int64                   `json:"total"`
		Page     int                     `json:"page"`
		PageSize int                     `json:"page_size"`
	}{Items: resp, Total: total, Page: page, PageSize: pageSize})
}

// apiDeletePausedTorrents 删除已暂停种子
// POST /api/torrents/delete-paused
func (s *Server) apiDeletePausedTorrents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req DeletePausedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "请求体解析失败: "+err.Error(), http.StatusBadRequest)
		return
	}

	db := global.GlobalDB.DB

	// 查询要删除的种子
	var torrents []models.TorrentInfo
	tx := db.Where("is_paused_by_system = ?", true)
	if len(req.IDs) > 0 {
		tx = tx.Where("id IN ?", req.IDs)
	}
	if err := tx.Find(&torrents).Error; err != nil {
		http.Error(w, "查询失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if len(torrents) == 0 {
		writeJSON(w, DeletePausedResponse{Success: 0, Failed: 0})
		return
	}

	// 获取下载器管理器
	dlMgr := s.getDownloaderManager()
	if dlMgr == nil {
		http.Error(w, "下载器管理器未初始化", http.StatusInternalServerError)
		return
	}

	var success, failed int
	var failedIDs []uint
	var failedErrors []string

	for _, t := range torrents {
		// 从下载器删除
		if t.DownloaderTaskID != "" && t.DownloaderName != "" {
			dl, err := dlMgr.GetDownloader(t.DownloaderName)
			if err != nil {
				global.GetSlogger().Warnf("获取下载器失败 (种子:%s): %v", t.Title, err)
				// 即使获取下载器失败，也尝试从数据库删除记录
			} else {
				if err := dl.RemoveTorrent(t.DownloaderTaskID, req.RemoveData); err != nil {
					// 记录错误但继续处理
					global.GetSlogger().Warnf("从下载器删除种子失败 (种子:%s): %v", t.Title, err)
					// 如果是种子不存在的错误，视为成功
					if err != downloader.ErrTorrentNotFound {
						failed++
						failedIDs = append(failedIDs, t.ID)
						failedErrors = append(failedErrors, t.Title+": "+err.Error())
						continue
					}
				}
			}
		}

		// 从数据库删除记录
		if err := db.Delete(&t).Error; err != nil {
			failed++
			failedIDs = append(failedIDs, t.ID)
			failedErrors = append(failedErrors, t.Title+": 数据库删除失败: "+err.Error())
			continue
		}

		success++
		global.GetSlogger().Infof("已删除暂停种子: %s (ID:%d)", t.Title, t.ID)
	}

	writeJSON(w, DeletePausedResponse{
		Success:      success,
		Failed:       failed,
		FailedIDs:    failedIDs,
		FailedErrors: failedErrors,
	})
}

// apiArchiveTorrents 获取归档种子列表
// GET /api/torrents/archive
func (s *Server) apiArchiveTorrents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// 解析分页参数
	page := 1
	pageSize := 50
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 && v <= 200 {
			pageSize = v
		}
	}

	// 可选的站点过滤
	siteFilter := r.URL.Query().Get("site")

	db := global.GlobalDB.DB
	tx := db.Model(&models.TorrentInfoArchive{})

	if siteFilter != "" {
		tx = tx.Where("site_name = ?", siteFilter)
	}

	// 计数
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		http.Error(w, "查询失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 查询
	var archives []models.TorrentInfoArchive
	if err := tx.Order("archived_at DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&archives).Error; err != nil {
		http.Error(w, "查询失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 转换为响应结构
	resp := make([]ArchiveTorrentResponse, 0, len(archives))
	for _, a := range archives {
		resp = append(resp, ArchiveTorrentResponse{
			ID:                a.ID,
			OriginalID:        a.OriginalID,
			SiteName:          a.SiteName,
			Title:             a.Title,
			TorrentHash:       a.TorrentHash,
			IsFree:            a.IsFree,
			FreeEndTime:       a.FreeEndTime,
			IsCompleted:       a.IsCompleted,
			Progress:          a.Progress,
			IsPausedBySystem:  a.IsPausedBySystem,
			PauseReason:       a.PauseReason,
			DownloaderName:    a.DownloaderName,
			OriginalCreatedAt: a.OriginalCreatedAt,
			ArchivedAt:        a.ArchivedAt,
		})
	}

	writeJSON(w, struct {
		Items    []ArchiveTorrentResponse `json:"items"`
		Total    int64                    `json:"total"`
		Page     int                      `json:"page"`
		PageSize int                      `json:"page_size"`
	}{Items: resp, Total: total, Page: page, PageSize: pageSize})
}

// apiResumeTorrent 恢复已暂停的种子
// POST /api/torrents/:id/resume
func (s *Server) apiResumeTorrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// 解析路径中的 ID
	path := strings.TrimPrefix(r.URL.Path, "/api/torrents/")
	path = strings.TrimSuffix(path, "/resume")
	id, err := strconv.ParseUint(path, 10, 64)
	if err != nil {
		http.Error(w, "无效的种子ID", http.StatusBadRequest)
		return
	}

	db := global.GlobalDB.DB

	// 查找种子
	var torrent models.TorrentInfo
	if dbErr := db.First(&torrent, uint(id)).Error; dbErr != nil {
		http.Error(w, "种子不存在", http.StatusNotFound)
		return
	}

	// 检查是否是暂停状态
	if !torrent.IsPausedBySystem {
		writeJSON(w, ResumeTorrentResponse{
			Success: false,
			Message: "种子未被系统暂停",
		})
		return
	}

	// 获取下载器
	if torrent.DownloaderTaskID == "" || torrent.DownloaderName == "" {
		http.Error(w, "种子缺少下载器信息", http.StatusBadRequest)
		return
	}

	dlMgr := s.getDownloaderManager()
	if dlMgr == nil {
		http.Error(w, "下载器管理器未初始化", http.StatusInternalServerError)
		return
	}

	dl, err := dlMgr.GetDownloader(torrent.DownloaderName)
	if err != nil {
		http.Error(w, "获取下载器失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 恢复下载
	if err := dl.ResumeTorrent(torrent.DownloaderTaskID); err != nil {
		http.Error(w, "恢复下载失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 更新数据库状态
	now := time.Now()
	updates := map[string]any{
		"is_paused_by_system": false,
		"paused_at":           nil,
		"pause_reason":        "",
		"last_check_time":     now,
	}
	if err := db.Model(&torrent).Updates(updates).Error; err != nil {
		global.GetSlogger().Errorf("更新种子恢复状态失败 (种子:%s): %v", torrent.Title, err)
		// 下载器已恢复，即使数据库更新失败也返回成功
	}

	global.GetSlogger().Infof("已恢复暂停种子: %s (ID:%d)", torrent.Title, torrent.ID)

	writeJSON(w, ResumeTorrentResponse{
		Success: true,
		Message: "种子已恢复下载",
	})
}

// apiTorrentManagementRouter 种子管理路由
// 处理 /api/torrents/* 路径
func (s *Server) apiTorrentManagementRouter(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/torrents")

	switch {
	case path == "/paused" || path == "/paused/":
		s.apiPausedTorrents(w, r)
	case path == "/delete-paused" || path == "/delete-paused/":
		s.apiDeletePausedTorrents(w, r)
	case path == "/archive" || path == "/archive/":
		s.apiArchiveTorrents(w, r)
	case strings.HasSuffix(path, "/resume"):
		s.apiResumeTorrent(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) getDownloaderManager() *downloader.DownloaderManager {
	if s.mgr == nil {
		return nil
	}
	return s.mgr.GetDownloaderManager()
}
