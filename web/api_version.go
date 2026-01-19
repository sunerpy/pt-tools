package web

import (
	"context"
	"net/http"
	"time"

	"github.com/sunerpy/pt-tools/version"
)

// apiVersion returns the current version information
// GET /api/version
func (s *Server) apiVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, version.GetVersionInfo())
}

// apiVersionCheck checks for new releases on GitHub
// GET /api/version/check?force=true&proxy=http://proxy:port
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
