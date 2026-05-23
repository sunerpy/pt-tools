package qbit

import (
	"context"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
)

const ptToolsLoginHTML = `<!doctype html><html><head><meta charset="utf-8"><title>登录</title>
<link rel="stylesheet" href="/static/style.css"></head>
<body class="login-page">
<main class="login-layout">
  <div class="login-card">
    <p class="login-eyebrow">PT TOOLS</p>
    <h1 class="login-title">欢迎登录</h1>
    <p class="login-subtitle">集中管理站点、RSS 与下载任务</p>
  </div>
</main>
</body></html>`

func TestLooksLikeHTML(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"empty", "", false},
		{"qbit_ok", "Ok.", false},
		{"qbit_fails", "Fails.", false},
		{"qbit_version", "v5.2.0", false},
		{"qbit_json", `{"hash":"abc"}`, false},
		{"json_array", "[]", false},
		{"xml_processing_instruction", `<?xml version="1.0"?><root/>`, false},
		{"random_lt", "<not-html-just-stray-bracket", false},
		{"doctype_lower", `<!doctype html>`, true},
		{"doctype_upper", `<!DOCTYPE html>`, true},
		{"doctype_with_bom", "\xef\xbb\xbf<!doctype html>", true},
		{"html_tag", `<html><body></body></html>`, true},
		{"head_tag", `<head><title>x</title></head>`, true},
		{"body_tag", `<body>foo</body>`, true},
		{"leading_whitespace", "  \n\t<!doctype html>", true},
		{"pttools_login_page", ptToolsLoginHTML, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := looksLikeHTML([]byte(tc.body))
			if got != tc.want {
				t.Fatalf("looksLikeHTML(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}

func wrongServerMock(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(ptToolsLoginHTML))
	})
	return httptest.NewServer(mux)
}

func TestAuthenticate_RejectsHTMLBody(t *testing.T) {
	ts := wrongServerMock(t)
	defer ts.Close()

	client := newVersionTestClient(ts.URL)
	err := client.Authenticate()
	if err == nil {
		t.Fatal("expected error when WebUI URL points at non-qBit server returning HTML, got nil")
	}
	if !strings.Contains(err.Error(), "HTML") && !strings.Contains(err.Error(), "qBit") {
		t.Fatalf("expected error message to mention HTML / qBit / URL config, got: %v", err)
	}
	if client.IsHealthy() {
		t.Fatal("client must not be marked healthy after HTML response")
	}
}

func TestAuthenticate_204ThenHTMLVersionFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/v2/app/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(ptToolsLoginHTML))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := newVersionTestClient(ts.URL)
	err := client.Authenticate()
	if err == nil {
		t.Fatal("expected error when version probe returns HTML after 204 login, got nil")
	}
	if client.IsHealthy() {
		t.Fatal("client must not be marked healthy when version probe returned HTML")
	}
}

func TestDetectVersion_HTMLReturnsWrongServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/app/version", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(ptToolsLoginHTML))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := newVersionTestClient(ts.URL)
	client.healthy = true
	err := client.detectVersion(context.Background())
	if err == nil {
		t.Fatal("expected detectVersion to error on HTML body, got nil")
	}
	if !errors.Is(err, errWrongServer) {
		t.Fatalf("expected errors.Is(err, errWrongServer), got: %v", err)
	}
	if client.IsHealthy() {
		t.Fatal("detectVersion HTML failure must flip healthy=false")
	}
}

func TestRequestsHTTPDoer_PersistsCookiesAcrossRequests(t *testing.T) {
	const sidName = "QBT_SID_8080"
	const sidValue = "test-session-id"

	var versionSawCookie string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     sidName,
			Value:    sidValue,
			Path:     "/",
			HttpOnly: true,
		})
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/v2/app/version", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sidName); err == nil {
			versionSawCookie = c.Value
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("v5.2.0"))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	httpClient := &http.Client{Jar: jar}

	client := &QbitClient{
		name:     "test-qbit",
		baseURL:  ts.URL,
		username: "admin",
		password: "password",
		client:   &standardHTTPDoer{client: httpClient},
	}

	if err := client.Authenticate(); err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}
	if versionSawCookie != sidValue {
		t.Fatalf("version probe did not receive %s cookie: got %q want %q", sidName, versionSawCookie, sidValue)
	}
	if !client.isV520Plus {
		t.Fatal("expected isV520Plus=true after detecting v5.2.0")
	}
}
