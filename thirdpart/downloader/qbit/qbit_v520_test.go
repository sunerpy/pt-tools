package qbit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func mockQbitServer(t *testing.T, loginStatus int, loginBody, versionBody string, versionStatus int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(loginStatus)
		if loginBody != "" {
			_, _ = w.Write([]byte(loginBody))
		}
	})
	mux.HandleFunc("/api/v2/app/version", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(versionStatus)
		if versionBody != "" {
			_, _ = w.Write([]byte(versionBody))
		}
	})
	return httptest.NewServer(mux)
}

func newVersionTestClient(baseURL string) *QbitClient {
	return &QbitClient{
		name:     "test-qbit",
		baseURL:  baseURL,
		username: "admin",
		password: "password",
		client:   &standardHTTPDoer{client: &http.Client{}},
	}
}

func TestAuthenticate_200LegacyOkBody(t *testing.T) {
	ts := mockQbitServer(t, http.StatusOK, "Ok.", "v4.6.5", http.StatusOK)
	defer ts.Close()

	client := newVersionTestClient(ts.URL)
	if err := client.Authenticate(); err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}
	if !client.IsHealthy() {
		t.Fatal("expected client healthy after 200 Ok.")
	}
}

func TestAuthenticate_200FailsBody(t *testing.T) {
	ts := mockQbitServer(t, http.StatusOK, "Fails.", "", http.StatusOK)
	defer ts.Close()

	client := newVersionTestClient(ts.URL)
	err := client.Authenticate()
	if err == nil {
		t.Fatal("expected authentication error")
	}
	if !strings.Contains(err.Error(), "用户名或密码错误") {
		t.Fatalf("expected invalid credential error, got %v", err)
	}
}

func TestAuthenticate_204LoginSuccess(t *testing.T) {
	ts := mockQbitServer(t, http.StatusNoContent, "", "v5.2.0", http.StatusOK)
	defer ts.Close()

	client := newVersionTestClient(ts.URL)
	if err := client.Authenticate(); err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}
	if !client.IsHealthy() {
		t.Fatal("expected client healthy after 204 login")
	}
}

func TestAuthenticate_401InvalidCreds(t *testing.T) {
	ts := mockQbitServer(t, http.StatusUnauthorized, "", "", http.StatusOK)
	defer ts.Close()

	client := newVersionTestClient(ts.URL)
	err := client.Authenticate()
	if err == nil {
		t.Fatal("expected authentication error")
	}
	if !strings.Contains(err.Error(), "认证失败") {
		t.Fatalf("expected 401 auth error, got %v", err)
	}
}

func TestDetectVersion_520Plus(t *testing.T) {
	ts := mockQbitServer(t, http.StatusOK, "Ok.", "v5.2.0", http.StatusOK)
	defer ts.Close()

	client := newVersionTestClient(ts.URL)
	if err := client.Authenticate(); err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}

	client.versionMu.RLock()
	defer client.versionMu.RUnlock()
	if !client.isV520Plus {
		t.Fatal("expected v5.2.0+ mode")
	}
	if client.appVersion != "v5.2.0" {
		t.Fatalf("expected appVersion v5.2.0, got %q", client.appVersion)
	}
}

func TestDetectVersion_Legacy(t *testing.T) {
	ts := mockQbitServer(t, http.StatusOK, "Ok.", "v4.6.5", http.StatusOK)
	defer ts.Close()

	client := newVersionTestClient(ts.URL)
	if err := client.Authenticate(); err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}

	client.versionMu.RLock()
	defer client.versionMu.RUnlock()
	if client.isV520Plus {
		t.Fatal("expected legacy mode")
	}
	if client.appVersion != "v4.6.5" {
		t.Fatalf("expected appVersion v4.6.5, got %q", client.appVersion)
	}
}

func TestDetectVersion_Fallback(t *testing.T) {
	ts := mockQbitServer(t, http.StatusOK, "Ok.", "server error", http.StatusInternalServerError)
	defer ts.Close()

	client := newVersionTestClient(ts.URL)
	if err := client.Authenticate(); err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}

	client.versionMu.RLock()
	defer client.versionMu.RUnlock()
	if client.isV520Plus {
		t.Fatal("expected fallback legacy mode")
	}
	if client.appVersion != "" {
		t.Fatalf("expected empty appVersion on detection failure, got %q", client.appVersion)
	}
}

func TestParseQBitVersion(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		major       int
		minor       int
		patch       int
		expectMatch bool
	}{
		{name: "prefixed", raw: "v5.2.0", major: 5, minor: 2, patch: 0, expectMatch: true},
		{name: "plain", raw: "5.2.0", major: 5, minor: 2, patch: 0, expectMatch: true},
		{name: "suffix", raw: "v5.2.0-rc1", major: 5, minor: 2, patch: 0, expectMatch: true},
		{name: "legacy", raw: "v4.6.5", major: 4, minor: 6, patch: 5, expectMatch: true},
		{name: "embedded", raw: "qBittorrent v5.2.0", major: 5, minor: 2, patch: 0, expectMatch: true},
		{name: "new minor", raw: "v5.3.1", major: 5, minor: 3, patch: 1, expectMatch: true},
		{name: "new major", raw: "v6.0.0", major: 6, minor: 0, patch: 0, expectMatch: true},
		{name: "empty", raw: "", expectMatch: false},
		{name: "invalid", raw: "not a version", expectMatch: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			major, minor, patch, ok := parseQBitVersion(tt.raw)
			if ok != tt.expectMatch {
				t.Fatalf("expected ok=%v, got %v", tt.expectMatch, ok)
			}
			if !tt.expectMatch {
				return
			}
			if major != tt.major || minor != tt.minor || patch != tt.patch {
				t.Fatalf("expected %d.%d.%d, got %d.%d.%d", tt.major, tt.minor, tt.patch, major, minor, patch)
			}
		})
	}
}

func TestIsSuccessStatus(t *testing.T) {
	tests := []struct {
		name       string
		isV520Plus bool
		codes      map[int]bool
	}{
		{
			name:       "v520plus",
			isV520Plus: true,
			codes: map[int]bool{
				http.StatusOK:                  true,
				http.StatusAccepted:            true,
				http.StatusNoContent:           true,
				299:                            true,
				300:                            false,
				http.StatusBadRequest:          false,
				http.StatusInternalServerError: false,
			},
		},
		{
			name:       "legacy",
			isV520Plus: false,
			codes: map[int]bool{
				http.StatusOK:                  true,
				http.StatusAccepted:            false,
				http.StatusNoContent:           false,
				299:                            false,
				300:                            false,
				http.StatusBadRequest:          false,
				http.StatusInternalServerError: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &QbitClient{}
			client.versionMu.Lock()
			client.isV520Plus = tt.isV520Plus
			client.versionMu.Unlock()
			for code, expected := range tt.codes {
				if got := client.isSuccessStatus(code); got != expected {
					t.Fatalf("code %d expected %v, got %v", code, expected, got)
				}
			}
		})
	}
}

func TestAddTorrentFileEx_202PendingJSON(t *testing.T) {
	torrentData := fixtureTorrentBytes()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/torrents/add":
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success_count":     0,
				"pending_count":     1,
				"failure_count":     0,
				"added_torrent_ids": []string{"hash-pending"},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	client := newVersionTestClient(ts.URL)
	client.versionMu.Lock()
	client.isV520Plus = true
	client.appVersion = "v5.2.0"
	client.versionMu.Unlock()

	result, err := client.AddTorrentFileEx(torrentData, downloader.AddTorrentOptions{})
	if err != nil {
		t.Fatalf("AddTorrentFileEx failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success result, got %+v", result)
	}
	message := fmt.Sprint(result.Message)
	if !strings.Contains(message, "待定 1") {
		t.Fatalf("expected pending message, got %q", message)
	}
	if result.Hash != "hash-pending" {
		t.Fatalf("expected returned hash, got %q", result.Hash)
	}
}

func TestAddTorrentFileEx_409Duplicate(t *testing.T) {
	torrentData := fixtureTorrentBytes()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/torrents/add" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusConflict)
	}))
	defer ts.Close()

	client := newVersionTestClient(ts.URL)
	client.versionMu.Lock()
	client.isV520Plus = true
	client.appVersion = "v5.2.0"
	client.versionMu.Unlock()

	result, err := client.AddTorrentFileEx(torrentData, downloader.AddTorrentOptions{})
	if err != nil {
		t.Fatalf("expected nil error for 409 duplicate, got %v", err)
	}
	if result.Success {
		t.Fatalf("expected duplicate result failure, got %+v", result)
	}
	message := fmt.Sprint(result.Message)
	if !strings.Contains(message, "已存在") {
		t.Fatalf("expected duplicate message, got %q", message)
	}
}
