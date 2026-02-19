package web

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/sunerpy/pt-tools/version"
)

func (s *Server) apiPing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"status": "ok", "version": version.GetVersionInfo().Version})
}

func (s *Server) apiVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, version.GetVersionInfo())
}

func (s *Server) apiVersionCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	force := r.URL.Query().Get("force") == "true"
	proxyURL := r.URL.Query().Get("proxy")
	checker := version.GetChecker()

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	opts := version.CheckOptions{
		Force:    force,
		ProxyURL: proxyURL,
	}
	result, err := checker.CheckForUpdates(ctx, opts)
	if err != nil && result == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, result)
}

func (s *Server) apiVersionRuntime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	env := version.DetectEnvironment()
	upgrader := version.GetUpgrader()
	progress := upgrader.GetProgress()

	resp := map[string]any{
		"runtime":          env,
		"upgrade_progress": progress,
	}
	writeJSON(w, resp)
}

func (s *Server) apiVersionUpgrade(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.apiUpgradeProgress(w, r)
	case http.MethodPost:
		s.apiUpgradeStart(w, r)
	case http.MethodDelete:
		s.apiUpgradeCancel(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) apiUpgradeProgress(w http.ResponseWriter, _ *http.Request) {
	upgrader := version.GetUpgrader()
	writeJSON(w, upgrader.GetProgress())
}

type upgradeRequest struct {
	Version  string `json:"version"`
	ProxyURL string `json:"proxy_url"`
}

func (s *Server) apiUpgradeStart(w http.ResponseWriter, r *http.Request) {
	upgrader := version.GetUpgrader()

	if err := upgrader.CanUpgrade(); err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req upgradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "请求格式错误", http.StatusBadRequest)
		return
	}

	if req.Version == "" {
		writeJSONError(w, "未指定目标版本", http.StatusBadRequest)
		return
	}

	checker := version.GetChecker()
	result := checker.GetCachedResult()
	if result == nil {
		writeJSONError(w, "请先检查更新", http.StatusBadRequest)
		return
	}

	var targetRelease *version.ReleaseInfo
	for i := range result.NewReleases {
		if result.NewReleases[i].Version == req.Version {
			targetRelease = &result.NewReleases[i]
			break
		}
	}

	if targetRelease == nil {
		writeJSONError(w, "未找到指定版本的更新信息", http.StatusBadRequest)
		return
	}

	if len(targetRelease.Assets) == 0 {
		writeJSONError(w, "该版本没有可用的安装包", http.StatusBadRequest)
		return
	}

	if err := upgrader.Upgrade(r.Context(), targetRelease, req.ProxyURL); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"success": true,
		"message": "升级已开始",
	})
}

func (s *Server) apiUpgradeCancel(w http.ResponseWriter, _ *http.Request) {
	upgrader := version.GetUpgrader()
	upgrader.Cancel()
	writeJSON(w, map[string]any{
		"success": true,
		"message": "升级已取消",
	})
}

func writeJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": false,
		"error":   message,
	})
}
