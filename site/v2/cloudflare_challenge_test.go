package v2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// M1d：Cloudflare 质询页不是凭证无效。识别出来，登录探测才会记为「被反爬拦截」并改用 CloakBrowser 后备。
func TestHTTPResponse_IsCloudflareChallenge(t *testing.T) {
	header := func(k, v string) http.Header {
		h := http.Header{}
		h.Set(k, v)
		return h
	}
	cases := []struct {
		name string
		resp *HTTPResponse
		want bool
	}{
		{"cf-mitigated header on 403", &HTTPResponse{StatusCode: http.StatusForbidden, Headers: header("cf-mitigated", "challenge")}, true},
		{"challenge script on 503", &HTTPResponse{StatusCode: http.StatusServiceUnavailable, Body: []byte(`<script>window._cf_chl_opt={cvId: '3'}</script>`)}, true},
		{"challenge platform asset on 403", &HTTPResponse{StatusCode: http.StatusForbidden, Body: []byte(`<script src="/cdn-cgi/challenge-platform/h/b/orchestrate/chl_page/v1"></script>`)}, true},
		{"plain 403", &HTTPResponse{StatusCode: http.StatusForbidden, Body: []byte("Forbidden")}, false},
		{"marker on 200", &HTTPResponse{StatusCode: http.StatusOK, Body: []byte("cf_chl_opt")}, false},
		{"429 stays rate limiting", &HTTPResponse{StatusCode: http.StatusTooManyRequests, Headers: header("cf-mitigated", "challenge")}, false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.resp.IsCloudflareChallenge())
		})
	}
}

// 有后备的四种架构遇到质询页都报 ErrCloudflareChallenge，不再报 ErrInvalidCredentials；普通 403 仍是凭证无效。
func TestDrivers_CloudflareChallengeIsNotInvalidCredentials(t *testing.T) {
	challenge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cf-Mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<!DOCTYPE html><title>Just a moment...</title><script>window._cf_chl_opt={}</script>`))
	}))
	t.Cleanup(challenge.Close)
	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(forbidden.Close)

	execute := func(base string) map[string]error {
		ctx := context.Background()
		errs := map[string]error{}
		_, errs["nexusphp"] = NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: base, Cookie: "c=1"}).
			Execute(ctx, NexusPHPRequest{Path: "/index.php"})
		_, errs["unit3d"] = NewUnit3DDriver(Unit3DDriverConfig{BaseURL: base, APIKey: "k"}).
			Execute(ctx, Unit3DRequest{Endpoint: "/api/user"})
		_, errs["gazelle"] = NewGazelleDriver(GazelleDriverConfig{BaseURL: base, APIKey: "k"}).
			Execute(ctx, GazelleRequest{Action: "index"})
		_, errs["mtorrent"] = NewMTorrentDriver(MTorrentDriverConfig{BaseURL: base, APIKey: "k"}).
			Execute(ctx, MTorrentRequest{Endpoint: "/api/member/profile", Method: http.MethodPost})
		return errs
	}
	for name, err := range execute(challenge.URL) {
		assert.ErrorIs(t, err, ErrCloudflareChallenge, name)
		assert.NotErrorIs(t, err, ErrInvalidCredentials, name)
	}
	for name, err := range execute(forbidden.URL) {
		assert.ErrorIs(t, err, ErrInvalidCredentials, name)
		assert.NotErrorIs(t, err, ErrCloudflareChallenge, name)
	}
}
