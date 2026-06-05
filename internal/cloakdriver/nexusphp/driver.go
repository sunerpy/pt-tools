// Package nexusphp implements the NexusPHP-schema CloakBrowser driver.
// Phase 1 sentinel: T11 must pass before T12-T14 (Unit3D, Gazelle,
// mTorrent CloakBrowser drivers) may proceed in parallel (Metis AP-1).
//
// The driver is the v2.0 fallback transport for NexusPHP-schema sites
// where cookie-only HTTP probing is blocked by Cloudflare/ja3
// fingerprinting. It is invoked by the cloakdriver dispatcher when a
// site has EnableCloakFallback=true and the HTTP probe (T4) returned a
// fallback-eligible status (RATE_LIMITED / CHALLENGE / NETWORK_ERROR).
package nexusphp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/sunerpy/pt-tools/internal/cloakdriver"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
)

const (
	launchTimeout   = 30 * time.Second
	stopTimeout     = 10 * time.Second
	probeNavTimeout = 45 * time.Second
)

// Driver is the NexusPHP-schema CloakBrowser transport.
type Driver struct {
	Manager cloakdriver.ManagerClient
}

// NewDriver constructs a NexusPHP CloakBrowser driver bound to the given
// Manager client (T8).
func NewDriver(manager cloakdriver.ManagerClient) *Driver {
	return &Driver{Manager: manager}
}

// Probe runs the full CloakBrowser lifecycle (launch → CDP connect →
// inject cookies → navigate → extract → close) and returns a populated
// *sitelogin.ProbeResult with Source=ProbeSourceCloak.
//
// The (result, err) tuple follows v1 probe convention: classification
// errors are encoded into the result; only programming-level errors
// (nil manager, empty URL, empty profile) return non-nil err.
//
// R35 (clear-before-inject) is enforced inside CDPSession.InjectCookies.
// The launched profile is always Stop()ped via defer, even on error.
// Timestamps in the result are UTC-normalised.
func (d *Driver) Probe(
	ctx context.Context,
	indexURL string,
	cookies []*http.Cookie,
	profileID string,
) (*sitelogin.ProbeResult, error) {
	if d == nil || d.Manager == nil {
		return nil, errors.New("nexusphp cloak driver: nil manager")
	}
	if indexURL == "" {
		return nil, errors.New("nexusphp cloak driver: empty indexURL")
	}
	if profileID == "" {
		return nil, errors.New("nexusphp cloak driver: empty profileID")
	}

	launchCtx, launchCancel := context.WithTimeout(ctx, launchTimeout)
	defer launchCancel()
	launch, err := d.Manager.LaunchProfile(launchCtx, profileID)
	if err != nil {
		return classifyManagerError(err), nil
	}

	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), stopTimeout)
		defer stopCancel()
		_ = d.Manager.StopProfile(stopCtx, profileID)
	}()

	session, err := cloakdriver.NewCDPSession(ctx, launch.CdpURL)
	if err != nil {
		return &sitelogin.ProbeResult{
			Status:     sitelogin.NETWORK_ERROR,
			Source:     sitelogin.ProbeSourceCloak,
			RawError:   err,
			Diagnostic: "cloak: cdp session open failed",
		}, nil
	}
	defer session.Close()

	injectCtx, injectCancel := context.WithTimeout(session.TaskContext(), 10*time.Second)
	defer injectCancel()
	if injectErr := session.InjectCookies(injectCtx, indexURL, cookies); injectErr != nil {
		return &sitelogin.ProbeResult{
			Status:     sitelogin.UNKNOWN,
			Source:     sitelogin.ProbeSourceCloak,
			RawError:   fmt.Errorf("inject cookies: %w", injectErr),
			Diagnostic: "cloak: cookie injection failed",
		}, nil
	}

	navCtx, navCancel := context.WithTimeout(session.TaskContext(), probeNavTimeout)
	defer navCancel()

	var (
		parsedLastLogin  time.Time
		parsedLastAccess time.Time
	)
	err = session.LoadPageAndExtract(
		navCtx,
		indexURL,
		userInfoWidgetSelector,
		func(html string) error {
			var perr error
			parsedLastLogin, parsedLastAccess, perr = parseNexusPHPUserPage(html)
			return perr
		},
	)
	if err != nil {
		return classifyChromedpError(err), nil
	}

	result := &sitelogin.ProbeResult{
		Status: sitelogin.OK,
		Source: sitelogin.ProbeSourceCloak,
	}
	if !parsedLastLogin.IsZero() {
		ll := parsedLastLogin.UTC()
		result.LastLoginAt = &ll
	}
	if !parsedLastAccess.IsZero() {
		la := parsedLastAccess.UTC()
		result.LastAccessAt = &la
	}

	if result.LastLoginAt == nil && result.LastAccessAt == nil {
		result.Status = sitelogin.PARSE_ERROR
		result.Diagnostic = "cloak: user widget rendered but no last_login/last_access row matched"
	}

	return result, nil
}

// userInfoWidgetSelector is a comma-OR set covering NexusPHP user-info
// containers across forks (#info_block standard, #userinfo TJUPT-style,
// .user_info HDDolby-style, h1 /userdetails.php fallback). Any one
// becoming visible proves the session rendered the user page.
const userInfoWidgetSelector = "#info_block, #userinfo, .user_info, h1"

// parseNexusPHPUserPage extracts last_login and last_access from a
// NexusPHP rowhead/rowfollow user-details table. Returns zero time.Time
// for fields not present; if no recognized row matched at all, returns
// a parse error so the reminder pipeline does not silently keep stale
// timestamps. Returned times are NOT UTC-normalised — callers must
// .UTC() before returning to scheduler downstream.
func parseNexusPHPUserPage(html string) (lastLogin, lastAccess time.Time, err error) {
	doc, derr := goquery.NewDocumentFromReader(strings.NewReader(html))
	if derr != nil {
		err = fmt.Errorf("nexusphp parse: %w", derr)
		return lastLogin, lastAccess, err
	}

	matched := false
	doc.Find("tr").Each(func(_ int, row *goquery.Selection) {
		header := strings.TrimSpace(row.Find("td.rowhead").First().Text())
		value := strings.TrimSpace(row.Find("td.rowfollow").First().Text())
		if header == "" || value == "" {
			return
		}
		if matchHeader(header, lastLoginHeaders) {
			if t, ok := parseTimestamp(value); ok {
				lastLogin = t
				matched = true
			}
			return
		}
		if matchHeader(header, lastAccessHeaders) {
			if t, ok := parseTimestamp(value); ok {
				lastAccess = t
				matched = true
			}
		}
	})

	if !matched {
		err = errors.New("nexusphp parse: no last_login/last_access row found")
		return lastLogin, lastAccess, err
	}
	return lastLogin, lastAccess, nil
}

var (
	lastLoginHeaders = []string{
		"上次登录", "上次登錄",
		"last login", "last seen",
	}
	lastAccessHeaders = []string{
		"上次访问", "上次訪問",
		"last access", "last action",
	}
)

func matchHeader(header string, candidates []string) bool {
	h := strings.ToLower(strings.TrimSpace(header))
	for _, c := range candidates {
		if strings.Contains(h, strings.ToLower(c)) {
			return true
		}
	}
	return false
}

var timeLayouts = []string{
	"2006-01-02 15:04:05 (Z07:00)",
	"2006-01-02 15:04:05 Z07:00",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02 15:04:05",
	"2006/01/02 15:04:05",
}

// parseTimestamp tries zone-aware layouts first, falling back to bare
// layouts interpreted in CST (UTC+8) — the NexusPHP convention shared
// with site/v2/nexusphp_driver.go. Returns ok=false on no match.
func parseTimestamp(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range timeLayouts {
		if strings.ContainsAny(layout, "Z") {
			if t, err := time.Parse(layout, s); err == nil {
				return t, true
			}
		}
	}
	cst := time.FixedZone("CST", 8*3600)
	for _, layout := range timeLayouts {
		if strings.ContainsAny(layout, "Z") {
			continue
		}
		if t, err := time.ParseInLocation(layout, s, cst); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func classifyManagerError(err error) *sitelogin.ProbeResult {
	r := &sitelogin.ProbeResult{
		Source:   sitelogin.ProbeSourceCloak,
		RawError: err,
	}
	switch {
	case errors.Is(err, cloakdriver.ErrManagerAuthFailed):
		r.Status = sitelogin.KEY_ERROR
		r.Diagnostic = "cloak: manager auth failed (check token)"
	case errors.Is(err, cloakdriver.ErrManagerNotFound):
		r.Status = sitelogin.UNKNOWN
		r.Diagnostic = "cloak: manager profile not found"
	case errors.Is(err, cloakdriver.ErrManagerServerError),
		errors.Is(err, cloakdriver.ErrManagerDNSFailed),
		errors.Is(err, cloakdriver.ErrManagerConnRefused),
		errors.Is(err, cloakdriver.ErrManagerTimeout):
		r.Status = sitelogin.NETWORK_ERROR
		r.Diagnostic = "cloak: manager unreachable"
	case errors.Is(err, cloakdriver.ErrManagerProtocolError):
		r.Status = sitelogin.UNKNOWN
		r.Diagnostic = "cloak: manager protocol error"
	default:
		r.Status = sitelogin.UNKNOWN
		r.Diagnostic = "cloak: manager error"
	}
	return r
}

// classifyChromedpError maps chromedp navigation/extraction errors to
// ProbeStatus. Deadline timeouts on the user-widget WaitVisible are
// classified as CHALLENGE (Cloudflare-style hold) rather than
// NETWORK_ERROR — matches Metis classification for fallback eligibility.
func classifyChromedpError(err error) *sitelogin.ProbeResult {
	r := &sitelogin.ProbeResult{
		Source:   sitelogin.ProbeSourceCloak,
		RawError: err,
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		r.Status = sitelogin.CHALLENGE
		r.Diagnostic = "cloak: navigate/wait deadline exceeded (likely Cloudflare challenge)"
	case errors.Is(err, context.Canceled):
		r.Status = sitelogin.UNKNOWN
		r.Diagnostic = "cloak: context canceled"
	default:
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "no last_login"):
			r.Status = sitelogin.PARSE_ERROR
			r.Diagnostic = "cloak: page rendered but parse failed"
		case strings.Contains(msg, "challenge"), strings.Contains(msg, "cloudflare"):
			r.Status = sitelogin.CHALLENGE
			r.Diagnostic = "cloak: challenge page detected"
		default:
			r.Status = sitelogin.NETWORK_ERROR
			r.Diagnostic = "cloak: chromedp error"
		}
	}
	return r
}
