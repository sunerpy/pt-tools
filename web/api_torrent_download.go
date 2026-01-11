// MIT License
// Copyright (c) 2025 pt-tools

package web

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/global"
)

// apiSiteRouter routes /api/site/* requests to appropriate handlers
func (s *Server) apiSiteRouter(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/site/")

	// Check for torrent download: {siteID}/torrent/{torrentID}/download
	if strings.Contains(path, "/torrent/") && strings.HasSuffix(path, "/download") {
		s.apiSiteTorrentDownload(w, r)
		return
	}

	// Check for free torrents download: {siteID}/free-torrents/download
	if strings.HasSuffix(path, "/free-torrents/download") {
		s.apiSiteFreeTorrentsDownload(w, r)
		return
	}

	// Check for free torrents list: {siteID}/free-torrents
	if strings.HasSuffix(path, "/free-torrents") {
		s.apiSiteFreeTorrentsList(w, r)
		return
	}

	// Unknown endpoint
	http.Error(w, "Not found", http.StatusNotFound)
}

// apiSiteTorrentDownload handles GET /api/site/{siteID}/torrent/{torrentID}/download
// Downloads a torrent file through the backend proxy, handling authentication automatically
// Query parameters:
// - title: optional torrent title for filename (will be saved as [siteID]title.torrent)
func (s *Server) apiSiteTorrentDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse path: /api/site/{siteID}/torrent/{torrentID}/download
	path := strings.TrimPrefix(r.URL.Path, "/api/site/")
	parts := strings.Split(path, "/")
	if len(parts) < 4 || parts[1] != "torrent" || parts[3] != "download" {
		http.Error(w, "Invalid path format. Expected /api/site/{siteID}/torrent/{torrentID}/download", http.StatusBadRequest)
		return
	}

	siteID := parts[0]
	torrentID := parts[2]

	if siteID == "" || torrentID == "" {
		http.Error(w, "Site ID and Torrent ID are required", http.StatusBadRequest)
		return
	}

	// Get optional title from query parameter
	title := r.URL.Query().Get("title")

	global.GetSlogger().Infof("[TorrentDownload] Downloading torrent: site=%s, id=%s, title=%s", siteID, torrentID, title)

	// Get the search orchestrator to access registered sites
	orchestrator := GetSearchOrchestrator()
	if orchestrator == nil {
		http.Error(w, "Search service not initialized", http.StatusServiceUnavailable)
		return
	}

	// Get site from orchestrator
	site := orchestrator.GetSite(siteID)
	if site == nil {
		http.Error(w, fmt.Sprintf("Site not found: %s", siteID), http.StatusNotFound)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Download torrent using site's Download method
	data, err := site.Download(ctx, torrentID)
	if err != nil {
		global.GetSlogger().Errorf("[TorrentDownload] Failed to download torrent: site=%s, id=%s, err=%v", siteID, torrentID, err)
		http.Error(w, fmt.Sprintf("Failed to download torrent: %v", err), http.StatusInternalServerError)
		return
	}

	// Generate filename: [siteID]title.torrent or siteID_torrentID.torrent if no title
	filename := generateTorrentFilename(siteID, torrentID, title)

	// Set response headers for torrent file download
	w.Header().Set("Content-Type", "application/x-bittorrent")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	// Write torrent data
	if _, err := w.Write(data); err != nil {
		global.GetSlogger().Errorf("[TorrentDownload] Failed to write response: %v", err)
	}

	global.GetSlogger().Infof("[TorrentDownload] Torrent downloaded successfully: site=%s, id=%s, size=%d, filename=%s", siteID, torrentID, len(data), filename)
}

// generateTorrentFilename creates a safe filename for torrent download
// Format: [siteID]title.torrent or siteID_torrentID.torrent if no title
func generateTorrentFilename(siteID, torrentID, title string) string {
	if title == "" {
		return fmt.Sprintf("%s_%s.torrent", siteID, torrentID)
	}

	// Sanitize title for filename - remove invalid characters
	safeTitle := sanitizeFilename(title)
	if safeTitle == "" {
		return fmt.Sprintf("%s_%s.torrent", siteID, torrentID)
	}

	// Limit filename length (max 200 chars for title to avoid filesystem issues)
	if len(safeTitle) > 200 {
		safeTitle = safeTitle[:200]
	}

	return fmt.Sprintf("[%s]%s.torrent", siteID, safeTitle)
}

// sanitizeFilename removes invalid characters from filename
func sanitizeFilename(name string) string {
	// Replace invalid filename characters with underscore
	invalidChars := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|", "\n", "\r", "\t"}
	result := name
	for _, char := range invalidChars {
		result = strings.ReplaceAll(result, char, "_")
	}

	// Remove leading/trailing spaces and dots
	result = strings.TrimSpace(result)
	result = strings.Trim(result, ".")

	return result
}

// BatchDownloadRequest represents a request for batch torrent download
type BatchDownloadRequest struct {
	Torrents []BatchDownloadItem `json:"torrents"`
}

// BatchDownloadItem represents a single torrent in batch download request
type BatchDownloadItem struct {
	SiteID    string `json:"siteId"`
	TorrentID string `json:"torrentId"`
	Title     string `json:"title"`
}

// apiBatchTorrentDownload handles POST /api/v2/torrents/batch-download
// Downloads multiple torrents from multiple sites and returns as tar.gz archive
func (s *Server) apiBatchTorrentDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req BatchDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if len(req.Torrents) == 0 {
		http.Error(w, "No torrents specified", http.StatusBadRequest)
		return
	}

	global.GetSlogger().Infof("[BatchDownload] Starting batch download: %d torrents", len(req.Torrents))

	// Get the search orchestrator to access registered sites
	orchestrator := GetSearchOrchestrator()
	if orchestrator == nil {
		http.Error(w, "Search service not initialized", http.StatusServiceUnavailable)
		return
	}

	// Create context with timeout (60s for batch operations)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// Download all torrents concurrently
	type downloadResult struct {
		filename string
		data     []byte
		err      error
	}

	results := make([]downloadResult, len(req.Torrents))
	var wg sync.WaitGroup

	for i, item := range req.Torrents {
		wg.Add(1)
		go func(idx int, t BatchDownloadItem) {
			defer wg.Done()

			site := orchestrator.GetSite(t.SiteID)
			if site == nil {
				results[idx] = downloadResult{err: fmt.Errorf("site not found: %s", t.SiteID)}
				return
			}

			data, err := site.Download(ctx, t.TorrentID)
			if err != nil {
				results[idx] = downloadResult{err: fmt.Errorf("download failed for %s/%s: %v", t.SiteID, t.TorrentID, err)}
				return
			}

			filename := generateTorrentFilename(t.SiteID, t.TorrentID, t.Title)
			results[idx] = downloadResult{filename: filename, data: data}
		}(i, item)
	}

	wg.Wait()

	// Count successful downloads
	successCount := 0
	for _, r := range results {
		if r.err == nil {
			successCount++
		}
	}

	if successCount == 0 {
		// All downloads failed
		errMsgs := make([]string, 0)
		for _, r := range results {
			if r.err != nil {
				errMsgs = append(errMsgs, r.err.Error())
			}
		}
		http.Error(w, fmt.Sprintf("All downloads failed: %s", strings.Join(errMsgs, "; ")), http.StatusInternalServerError)
		return
	}

	global.GetSlogger().Infof("[BatchDownload] Downloaded %d/%d torrents successfully", successCount, len(req.Torrents))

	// Create tar.gz archive
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"torrents_%s.tar.gz\"", time.Now().Format("20060102_150405")))

	gzWriter := gzip.NewWriter(w)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	for _, result := range results {
		if result.err != nil {
			continue
		}

		// Write tar header - use TypeReg and Format to avoid PaxHeaders
		header := &tar.Header{
			Name:     result.filename,
			Size:     int64(len(result.data)),
			Mode:     0o644,
			ModTime:  time.Now(),
			Typeflag: tar.TypeReg,
			Format:   tar.FormatGNU,
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			global.GetSlogger().Errorf("[BatchDownload] Failed to write tar header: %v", err)
			continue
		}

		// Write file data
		if _, err := tarWriter.Write(result.data); err != nil {
			global.GetSlogger().Errorf("[BatchDownload] Failed to write tar data: %v", err)
			continue
		}
	}

	global.GetSlogger().Infof("[BatchDownload] Batch download completed: %d torrents", successCount)
}
