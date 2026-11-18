//go:build integration
// +build integration

package nexusphp

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/cloakdriver"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
)

// TestNexusPHPCloakDriverIntegration is the Phase Gate sentinel.
//
// To run (verified against cloakhq/cloakbrowser-manager:v0.1.8):
//
//	docker run -d --rm -e AUTH_TOKEN=<token> -p 127.0.0.1:18090:8080 cloakhq/cloakbrowser-manager:v0.1.8
//	curl -H "Authorization: Bearer <token>" -H 'Content-Type: application/json' \
//	  -d '{"name":"pt-tools-test"}' http://127.0.0.1:18090/api/profiles   # prints the profile id
//	export PT_TOOLS_CLOAK_MGR_URL=http://127.0.0.1:18090
//	export PT_TOOLS_CLOAK_MGR_TOKEN=<token>
//	export PT_TOOLS_CLOAK_PROFILE_ID=<id-of-pre-created-profile>
//	export PT_TOOLS_CLOAK_SITE_HOST=172.17.0.1   # the docker bridge gateway: the browser runs inside the container
//	go test -v -tags integration -run TestNexusPHPCloakDriverIntegration ./internal/cloakdriver/nexusphp/...
//
// PT_TOOLS_CLOAK_SITE_HOST makes the fake site listen on all interfaces and
// gives the browser that host; without it the site is on 127.0.0.1, which only
// a Manager on the same network namespace can reach.
//
// If the env vars are unset, the test SKIPs — that signals the Phase
// Gate is YELLOW (manually deferred) rather than failed.
//
// The test does NOT hit any real PT site (compliance requirement). It
// stands up an httptest.NewServer that serves canned NexusPHP-formatted
// HTML and points the chromedp navigation there. This proves the full
// real-Manager + real-Chromium + real-cookies + real-parser flow without
// touching tracker infrastructure.
func TestNexusPHPCloakDriverIntegration(t *testing.T) {
	mgrURL := os.Getenv("PT_TOOLS_CLOAK_MGR_URL")
	profileID := os.Getenv("PT_TOOLS_CLOAK_PROFILE_ID")
	if mgrURL == "" || profileID == "" {
		t.Skip("Phase Gate YELLOW: PT_TOOLS_CLOAK_MGR_URL or PT_TOOLS_CLOAK_PROFILE_ID unset; integration test deferred")
	}
	token := os.Getenv("PT_TOOLS_CLOAK_MGR_TOKEN")

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html>
<html>
<head><title>Fake NexusPHP</title></head>
<body>
<div id="info_block">testuser</div>
<table>
<tr><td class="rowhead">上次登录</td><td class="rowfollow">2026-05-15 12:34:56</td></tr>
<tr><td class="rowhead">上次访问</td><td class="rowfollow">2026-05-18 09:00:00</td></tr>
</table>
</body>
</html>`))
	}))
	siteHost := os.Getenv("PT_TOOLS_CLOAK_SITE_HOST")
	if siteHost != "" {
		l, err := net.Listen("tcp", "0.0.0.0:0")
		require.NoError(t, err)
		ts.Listener = l
	}
	ts.Start()
	defer ts.Close()
	siteURL := ts.URL
	if siteHost != "" {
		siteURL = "http://" + net.JoinHostPort(siteHost, strconv.Itoa(ts.Listener.Addr().(*net.TCPAddr).Port))
	} else {
		siteHost = "127.0.0.1"
	}

	mgr := cloakdriver.NewManagerClient(mgrURL, token, 30*time.Second)

	cookies := []*http.Cookie{
		{Name: "PHPSESSID", Value: "phase-gate-test", Domain: siteHost, Path: "/"},
	}

	d := NewDriver(mgr)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	res, err := d.Probe(ctx, siteURL+"/", cookies, profileID)
	require.NoError(t, err, "Probe must not return programming error")
	require.NotNil(t, res)

	t.Logf("phase-gate result: status=%s source=%s last_login=%v last_access=%v diag=%q raw=%v",
		res.Status, res.Source, res.LastLoginAt, res.LastAccessAt, res.Diagnostic, res.RawError)

	assert.Equal(t, sitelogin.ProbeSourceCloak, res.Source)
	assert.Equal(t, sitelogin.OK, res.Status, "Phase Gate FAILED: status=%s diag=%s", res.Status, res.Diagnostic)
	require.NotNil(t, res.LastLoginAt, "Phase Gate FAILED: last_login not extracted")
	require.NotNil(t, res.LastAccessAt, "Phase Gate FAILED: last_access not extracted")

	wantLogin := time.Date(2026, 5, 15, 4, 34, 56, 0, time.UTC)
	wantAccess := time.Date(2026, 5, 18, 1, 0, 0, 0, time.UTC)
	assert.True(t, res.LastLoginAt.Equal(wantLogin),
		"last_login UTC mismatch: got %s want %s", res.LastLoginAt, wantLogin)
	assert.True(t, res.LastAccessAt.Equal(wantAccess),
		"last_access UTC mismatch: got %s want %s", res.LastAccessAt, wantAccess)
}
