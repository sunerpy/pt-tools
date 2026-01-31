// MIT License
// Copyright (c) 2025 pt-tools

package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// TorrentPushRequest 种子推送请求
type TorrentPushRequest struct {
	// 种子信息（二选一）
	DownloadURL string `json:"downloadUrl,omitempty"` // 种子下载链接（内部API路径）
	TorrentID   string `json:"torrentId,omitempty"`   // 种子ID

	// 下载器配置
	DownloaderIDs []uint `json:"downloaderIds"`       // 目标下载器ID列表
	SavePath      string `json:"savePath,omitempty"`  // 保存路径（可选）
	Category      string `json:"category,omitempty"`  // 分类
	Tags          string `json:"tags,omitempty"`      // 标签
	AutoStart     *bool  `json:"autoStart,omitempty"` // 是否自动开始（覆盖下载器默认设置）

	// 种子元信息（用于日志和数据库记录）
	TorrentTitle string `json:"torrentTitle,omitempty"`
	SourceSite   string `json:"sourceSite,omitempty"`
	SizeBytes    int64  `json:"sizeBytes,omitempty"`
}

// TorrentPushResponse 种子推送响应
type TorrentPushResponse struct {
	Success bool                    `json:"success"`
	Results []TorrentPushResultItem `json:"results"`
	Message string                  `json:"message,omitempty"`
}

// TorrentPushResultItem 单个下载器的推送结果
type TorrentPushResultItem struct {
	DownloaderID   uint   `json:"downloaderId"`
	DownloaderName string `json:"downloaderName"`
	Success        bool   `json:"success"`
	Skipped        bool   `json:"skipped,omitempty"` // 种子已存在时跳过
	Message        string `json:"message,omitempty"`
	TorrentHash    string `json:"torrentHash,omitempty"`
}

// BatchTorrentPushRequest 批量种子推送请求
type BatchTorrentPushRequest struct {
	Torrents      []TorrentPushItem `json:"torrents"`
	DownloaderIDs []uint            `json:"downloaderIds"`
	SavePath      string            `json:"savePath,omitempty"`
	Category      string            `json:"category,omitempty"`
	Tags          string            `json:"tags,omitempty"`
	AutoStart     *bool             `json:"autoStart,omitempty"`
}

// TorrentPushItem 批量推送中的单个种子项
type TorrentPushItem struct {
	DownloadURL  string `json:"downloadUrl,omitempty"`
	TorrentID    string `json:"torrentId,omitempty"`
	TorrentTitle string `json:"torrentTitle,omitempty"`
	SourceSite   string `json:"sourceSite,omitempty"`
	SizeBytes    int64  `json:"sizeBytes,omitempty"`
}

// BatchTorrentPushResponse 批量推送响应
type BatchTorrentPushResponse struct {
	Success      bool                         `json:"success"`
	TotalCount   int                          `json:"totalCount"`
	SuccessCount int                          `json:"successCount"`
	SkippedCount int                          `json:"skippedCount"` // 跳过的数量（种子已存在）
	FailedCount  int                          `json:"failedCount"`
	Results      []BatchTorrentPushResultItem `json:"results"`
}

// BatchTorrentPushResultItem 批量推送单项结果
type BatchTorrentPushResultItem struct {
	TorrentTitle string                  `json:"torrentTitle"`
	SourceSite   string                  `json:"sourceSite"`
	Success      bool                    `json:"success"`
	Skipped      bool                    `json:"skipped,omitempty"` // 所有下载器都跳过时为true
	Message      string                  `json:"message,omitempty"`
	Results      []TorrentPushResultItem `json:"results,omitempty"`
}

// apiTorrentPush handles POST /api/v2/torrents/push
// Push a single torrent to one or more downloaders
func (s *Server) apiTorrentPush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req TorrentPushRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate request
	if req.DownloadURL == "" {
		http.Error(w, "downloadUrl is required", http.StatusBadRequest)
		return
	}

	if len(req.DownloaderIDs) == 0 {
		http.Error(w, "At least one downloaderId is required", http.StatusBadRequest)
		return
	}

	// Process push
	response := s.processTorrentPush(r, req)
	writeJSON(w, response)
}

// apiTorrentBatchPush handles POST /api/v2/torrents/batch-push
// Push multiple torrents to one or more downloaders
func (s *Server) apiTorrentBatchPush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req BatchTorrentPushRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate request
	if len(req.Torrents) == 0 {
		http.Error(w, "At least one torrent is required", http.StatusBadRequest)
		return
	}

	if len(req.DownloaderIDs) == 0 {
		http.Error(w, "At least one downloaderId is required", http.StatusBadRequest)
		return
	}

	// Process batch push
	response := s.processBatchTorrentPush(r, req)
	writeJSON(w, response)
}

// processTorrentPush processes a single torrent push request
// 复用 internal.PushTorrentToDownloader 逻辑，支持数据库记录
func (s *Server) processTorrentPush(r *http.Request, req TorrentPushRequest) TorrentPushResponse {
	ctx := r.Context()
	response := TorrentPushResponse{
		Success: true,
		Results: make([]TorrentPushResultItem, 0, len(req.DownloaderIDs)),
	}

	// 解析 downloadUrl 获取 siteID 和 torrentID
	parsed, err := parseDownloadURL(req.DownloadURL)
	if err != nil {
		response.Success = false
		response.Message = fmt.Sprintf("Invalid download URL: %v", err)
		return response
	}

	siteID := parsed.SiteID
	torrentID := parsed.TorrentID
	downhash := parsed.Downhash

	// 如果请求中提供了 torrentID，使用它
	if req.TorrentID != "" {
		torrentID = req.TorrentID
	}

	// 如果请求中提供了 sourceSite，使用它
	if req.SourceSite != "" {
		siteID = req.SourceSite
	}

	// 使用 SearchOrchestrator 下载种子数据
	orchestrator := GetSearchOrchestrator()
	if orchestrator == nil {
		response.Success = false
		response.Message = "Search service not initialized"
		return response
	}

	site := orchestrator.GetSite(siteID)
	if site == nil {
		response.Success = false
		response.Message = fmt.Sprintf("Site not found: %s", siteID)
		return response
	}

	// 下载种子数据 - use hash if available (required by some sites like HDDolby)
	var torrentData []byte
	if downhash != "" {
		if hd, ok := site.(v2.HashDownloader); ok {
			torrentData, err = hd.DownloadWithHash(ctx, torrentID, downhash)
		} else {
			torrentData, err = site.Download(ctx, torrentID)
		}
	} else {
		torrentData, err = site.Download(ctx, torrentID)
	}
	if err != nil {
		response.Success = false
		response.Message = fmt.Sprintf("Failed to download torrent: %v", err)
		return response
	}

	// 推送到每个下载器
	successCount := 0
	for _, dlID := range req.DownloaderIDs {
		result := TorrentPushResultItem{
			DownloaderID: dlID,
		}

		// 获取下载器名称
		var dlName string
		if global.GlobalDB != nil {
			var dlSetting struct {
				Name string
			}
			if err := global.GlobalDB.DB.Table("downloader_settings").Select("name").Where("id = ?", dlID).First(&dlSetting).Error; err == nil {
				dlName = dlSetting.Name
			}
		}
		result.DownloaderName = dlName

		// 使用 internal.PushTorrentToDownloader 进行推送（复用现有逻辑，记录数据库）
		pushReq := internal.PushTorrentRequest{
			SiteID:       siteID,
			TorrentID:    torrentID,
			TorrentData:  torrentData,
			Title:        req.TorrentTitle,
			Category:     req.Category,
			Tags:         req.Tags,
			SavePath:     req.SavePath,
			DownloaderID: dlID,
		}

		pushResult, err := internal.PushTorrentToDownloader(ctx, pushReq)
		if err != nil {
			result.Success = false
			result.Message = err.Error()
		} else if !pushResult.Success {
			result.Success = false
			result.Message = pushResult.Message
			result.TorrentHash = pushResult.TorrentHash
		} else {
			result.Success = true
			result.TorrentHash = pushResult.TorrentHash
			result.Skipped = pushResult.Skipped
			if pushResult.Skipped {
				result.Message = pushResult.Message // "种子已存在于下载器中"
			} else {
				result.Message = "推送成功"
			}
			successCount++
		}

		response.Results = append(response.Results, result)
	}

	if successCount == 0 {
		response.Success = false
		response.Message = "Failed to push torrent to any downloader"
	} else if successCount < len(req.DownloaderIDs) {
		response.Message = fmt.Sprintf("Pushed to %d/%d downloaders", successCount, len(req.DownloaderIDs))
	} else {
		response.Message = "Torrent pushed successfully to all downloaders"
	}

	global.GetSlogger().Infof("[TorrentPush] Push completed: title=%s, site=%s, success=%d/%d",
		req.TorrentTitle, siteID, successCount, len(req.DownloaderIDs))

	return response
}

// processBatchTorrentPush processes a batch torrent push request
func (s *Server) processBatchTorrentPush(r *http.Request, req BatchTorrentPushRequest) BatchTorrentPushResponse {
	response := BatchTorrentPushResponse{
		Success:    true,
		TotalCount: len(req.Torrents),
		Results:    make([]BatchTorrentPushResultItem, 0, len(req.Torrents)),
	}

	for _, torrent := range req.Torrents {
		itemResult := BatchTorrentPushResultItem{
			TorrentTitle: torrent.TorrentTitle,
			SourceSite:   torrent.SourceSite,
		}

		// Build single push request
		pushReq := TorrentPushRequest{
			DownloadURL:   torrent.DownloadURL,
			TorrentID:     torrent.TorrentID,
			DownloaderIDs: req.DownloaderIDs,
			SavePath:      req.SavePath,
			Category:      req.Category,
			Tags:          req.Tags,
			AutoStart:     req.AutoStart,
			TorrentTitle:  torrent.TorrentTitle,
			SourceSite:    torrent.SourceSite,
			SizeBytes:     torrent.SizeBytes,
		}

		// Process single push
		pushResult := s.processTorrentPush(r, pushReq)
		itemResult.Success = pushResult.Success
		itemResult.Message = pushResult.Message
		itemResult.Results = pushResult.Results

		// Check if all results were skipped
		allSkipped := true
		for _, r := range pushResult.Results {
			if !r.Skipped {
				allSkipped = false
				break
			}
		}
		itemResult.Skipped = allSkipped && pushResult.Success && len(pushResult.Results) > 0

		if pushResult.Success {
			if itemResult.Skipped {
				response.SkippedCount++
			} else {
				response.SuccessCount++
			}
		} else {
			response.FailedCount++
		}

		response.Results = append(response.Results, itemResult)
	}

	if response.FailedCount > 0 {
		if response.SuccessCount == 0 && response.SkippedCount == 0 {
			response.Success = false
		}
	}

	global.GetSlogger().Infof("[BatchTorrentPush] Batch push completed: total=%d, success=%d, skipped=%d, failed=%d",
		response.TotalCount, response.SuccessCount, response.SkippedCount, response.FailedCount)

	return response
}

type parsedDownloadURL struct {
	SiteID    string
	TorrentID string
	Downhash  string
}

// parseDownloadURL 解析内部下载 URL
// 格式: /api/site/{siteID}/torrent/{torrentID}/download[?downhash={hash}]
func parseDownloadURL(urlStr string) (parsedDownloadURL, error) {
	var result parsedDownloadURL

	baseURL := urlStr
	if idx := strings.Index(urlStr, "?"); idx != -1 {
		baseURL = urlStr[:idx]
		query := urlStr[idx+1:]
		for _, param := range strings.Split(query, "&") {
			if strings.HasPrefix(param, "downhash=") {
				result.Downhash = strings.TrimPrefix(param, "downhash=")
			}
		}
	}

	if !strings.HasPrefix(baseURL, "/api/site/") {
		return result, fmt.Errorf("invalid internal download URL format")
	}

	path := strings.TrimPrefix(baseURL, "/api/site/")
	parts := strings.Split(path, "/")

	// Expected: [siteID, "torrent", torrentID, "download"] or [siteID, "torrent", torrentID]
	if len(parts) < 3 || parts[1] != "torrent" {
		return result, fmt.Errorf("invalid internal download URL format: expected /api/site/{siteID}/torrent/{torrentID}/download")
	}

	result.SiteID = parts[0]
	result.TorrentID = parts[2]
	return result, nil
}
