//go:build integration
// +build integration

package nexusphp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/cloakdriver"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
)

// TestNexusPHPCloakDriverIntegration is the Phase Gate sentinel.
//
// To run:
//
//	docker run -d --rm -p 8080:8080 cloakhq/cloakbrowser-manager:0.0.4
//	export PT_TOOLS_CLOAK_MGR_URL=http://127.0.0.1:8080
//	export PT_TOOLS_CLOAK_MGR_TOKEN=""
//	export PT_TOOLS_CLOAK_PROFILE_ID=<id-of-pre-created-profile>
//	go test -v -tags integration -run TestNexusPHPCloakDriverIntegration ./internal/cloakdriver/nexusphp/...
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

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer ts.Close()

	mgr := cloakdriver.NewManagerClient(mgrURL, token, 30*time.Second)

	cookies := []*http.Cookie{
		{Name: "PHPSESSID", Value: "phase-gate-test", Domain: "127.0.0.1", Path: "/"},
	}

	d := NewDriver(mgr)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	res, err := d.Probe(ctx, ts.URL+"/", cookies, profileID)
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
