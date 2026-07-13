package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateTorrentFilename(t *testing.T) {
	tests := []struct {
		name      string
		siteID    string
		torrentID string
		title     string
		want      string
	}{
		{"no title", "hdsky", "42", "", "hdsky_42.torrent"},
		{"with title", "mteam", "7", "Cool Movie", "[mteam]Cool Movie.torrent"},
		{"title with invalid chars", "hdsky", "9", "a/b:c*d?", "[hdsky]a_b_c_d_.torrent"},
		{"title only dots trimmed", "hdsky", "1", "...", "hdsky_1.torrent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateTorrentFilename(tt.siteID, tt.torrentID, tt.title)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGenerateTorrentFilename_LongTitleTruncated(t *testing.T) {
	longTitle := strings.Repeat("x", 300)
	got := generateTorrentFilename("s", "1", longTitle)
	assert.True(t, strings.HasPrefix(got, "[s]"))
	assert.True(t, strings.HasSuffix(got, ".torrent"))
	// [s] (3) + 200 chars + .torrent (8) == 211
	assert.Equal(t, 3+200+len(".torrent"), len(got))
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"clean", "hello world", "hello world"},
		{"slashes", "a/b\\c", "a_b_c"},
		{"colon and pipe", "a:b|c", "a_b_c"},
		{"quotes and brackets", `a"b<c>d`, "a_b_c_d"},
		{"newlines tabs", "a\nb\tc\rd", "a_b_c_d"},
		{"leading trailing spaces dots", "  .name.  ", "name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitizeFilename(tt.in))
		})
	}
}

func TestApiSiteRouter_Routing(t *testing.T) {
	s := &Server{}

	tests := []struct {
		name       string
		path       string
		method     string
		wantStatus int
	}{
		// torrent download route -> orchestrator nil -> 503
		{"torrent download unavailable", "/api/site/hdsky/torrent/42/download", http.MethodGet, http.StatusServiceUnavailable},
		// free-torrents download route -> placeholder 200
		{"free torrents download", "/api/site/hdsky/free-torrents/download", http.MethodGet, http.StatusOK},
		// free-torrents list route -> 200 empty
		{"free torrents list", "/api/site/hdsky/free-torrents", http.MethodGet, http.StatusOK},
		// unknown -> 404
		{"unknown endpoint", "/api/site/hdsky/other", http.MethodGet, http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			s.apiSiteRouter(w, req)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestApiSiteTorrentDownload_BadPath(t *testing.T) {
	s := &Server{}

	tests := []struct {
		name       string
		path       string
		method     string
		wantStatus int
	}{
		{"method not allowed", "/api/site/hdsky/torrent/42/download", http.MethodPost, http.StatusMethodNotAllowed},
		{"invalid path format", "/api/site/hdsky/bad/42/download", http.MethodGet, http.StatusBadRequest},
		{"orchestrator not init", "/api/site/hdsky/torrent/42/download", http.MethodGet, http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			s.apiSiteTorrentDownload(w, req)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestApiBatchTorrentDownload_BadInput(t *testing.T) {
	s := &Server{}

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v2/torrents/batch-download", nil)
		s.apiBatchTorrentDownload(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-download", bytes.NewBufferString(`{bad`))
		s.apiBatchTorrentDownload(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty torrents", func(t *testing.T) {
		body, _ := json.Marshal(BatchDownloadRequest{Torrents: nil})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-download", bytes.NewReader(body))
		s.apiBatchTorrentDownload(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("orchestrator not init", func(t *testing.T) {
		body, _ := json.Marshal(BatchDownloadRequest{Torrents: []BatchDownloadItem{{SiteID: "hdsky", TorrentID: "1"}}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-download", bytes.NewReader(body))
		s.apiBatchTorrentDownload(w, req)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}
