package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// ==== merged from api_torrent_download_cov3_test.go ====
// plainV2Site implements v2.Site WITHOUT HashDownloader, to exercise the
// download-hash fallback branch in apiSiteTorrentDownload.
type plainV2Site struct {
	id   string
	data []byte
	err  error
}

func (p *plainV2Site) ID() string        { return p.id }
func (p *plainV2Site) Name() string      { return p.id }
func (p *plainV2Site) Kind() v2.SiteKind { return v2.SiteNexusPHP }
func (p *plainV2Site) Login(context.Context, v2.Credentials) error {
	return nil
}

func (p *plainV2Site) Search(context.Context, v2.SearchQuery) ([]v2.TorrentItem, error) {
	return nil, nil
}

func (p *plainV2Site) GetUserInfo(context.Context) (v2.UserInfo, error) { return v2.UserInfo{}, nil }
func (p *plainV2Site) Download(context.Context, string) ([]byte, error) { return p.data, p.err }
func (p *plainV2Site) Close() error                                     { return nil }

func TestApiSiteTorrentDownload_HashFallbackNonHashSite(t *testing.T) {
	withOrchestrator(t, &plainV2Site{id: "plainsite", data: []byte("plaindata")})

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/site/plainsite/torrent/1/download?downhash=abc", nil)
	s.apiSiteTorrentDownload(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "plaindata", w.Body.String())
}

func TestApiSiteTorrentDownload_EmptyIDs(t *testing.T) {
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky"})
	s := &Server{}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/site//torrent//download", nil)
	s.apiSiteTorrentDownload(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestApiSiteTorrentDownload_OrchestratorNil(t *testing.T) {
	prev := searchOrchestrator
	searchOrchestrator = nil
	t.Cleanup(func() { searchOrchestrator = prev })

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/site/hdsky/torrent/1/download", nil)
	s.apiSiteTorrentDownload(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestApiBatchTorrentDownload_DownloadErrorPerItem(t *testing.T) {
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: []byte("ok")},
		&fakeV2Site{id: "badsite", name: "Bad", err: assertErr("dlfail")})

	s := &Server{}
	body := `{"torrents":[{"siteId":"hdsky","torrentId":"1","title":"A"},{"siteId":"badsite","torrentId":"2"}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-download", strings.NewReader(body))
	s.apiBatchTorrentDownload(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/gzip", w.Header().Get("Content-Type"))
}

// ==== merged from api_torrent_download_cov_test.go ====
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

// ==== merged from api_torrent_download_data_test.go ====
func TestApiSiteTorrentDownload_WithOrchestrator(t *testing.T) {
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: []byte("torrentdata")})

	s := &Server{}

	t.Run("download succeeds", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site/hdsky/torrent/42/download?title=Cool", nil)
		s.apiSiteTorrentDownload(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/x-bittorrent", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Header().Get("Content-Disposition"), "[hdsky]Cool.torrent")
		assert.Equal(t, "torrentdata", w.Body.String())
	})

	t.Run("site not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site/unknown/torrent/1/download", nil)
		s.apiSiteTorrentDownload(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestApiSiteTorrentDownload_WithHash(t *testing.T) {
	withOrchestrator(t, &fakeV2Site{id: "hddolby", name: "HDDolby", hashData: []byte("hashdata")})

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/site/hddolby/torrent/7/download?downhash=abc", nil)
	s.apiSiteTorrentDownload(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "hashdata", w.Body.String())
}

func TestApiBatchTorrentDownload_WithOrchestrator(t *testing.T) {
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: []byte("data1")})

	s := &Server{}

	t.Run("all fail returns 500", func(t *testing.T) {
		body, _ := json.Marshal(BatchDownloadRequest{Torrents: []BatchDownloadItem{
			{SiteID: "unknown", TorrentID: "1"},
		}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-download", bytes.NewReader(body))
		s.apiBatchTorrentDownload(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("partial success returns archive", func(t *testing.T) {
		body, _ := json.Marshal(BatchDownloadRequest{Torrents: []BatchDownloadItem{
			{SiteID: "hdsky", TorrentID: "1", Title: "One"},
			{SiteID: "unknown", TorrentID: "2"},
		}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-download", bytes.NewReader(body))
		s.apiBatchTorrentDownload(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/gzip", w.Header().Get("Content-Type"))
		assert.NotEmpty(t, w.Body.Bytes())
	})
}

func TestProcessTorrentPush_WithOrchestrator(t *testing.T) {
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: []byte("data")})

	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", nil)

	t.Run("site not found", func(t *testing.T) {
		resp := s.processTorrentPush(req, TorrentPushRequest{
			DownloadURL:   "/api/site/unknown/torrent/1/download",
			DownloaderIDs: []uint{1},
		})
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Message, "Site not found")
	})
}
