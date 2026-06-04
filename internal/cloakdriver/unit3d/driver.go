// Package unit3d implements the Unit3D-schema CloakBrowser driver.
// Unit3D is a Laravel-based PT site framework (BLU, HUNO, Aither, etc.).
// The HTTP probe path uses the JSON API (/api/user); this CloakBrowser
// fallback scrapes the rendered HTML user page when Cloudflare/ja3
// fingerprinting blocks cookie-based HTTP probing.
package unit3d

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

// Driver is the Unit3D-schema CloakBrowser transport.
type Driver struct {
	Manager cloakdriver.ManagerClient
}

// NewDriver constructs a Unit3D CloakBrowser driver bound to the given
// Manager client (T8).
func NewDriver(manager cloakdriver.ManagerClient) *Driver {
	return &Driver{Manager: manager}
}

// Probe runs the full CloakBrowser lifecycle against a Unit3D site and
// returns a populated *sitelogin.ProbeResult with Source=ProbeSourceCloak.
//
// Classification errors are encoded into the result; only programming
// errors (nil manager, empty URL, empty profile) return non-nil err.
// Timestamps in the result are UTC-normalised. R35 (clear-before-inject)
// is enforced inside CDPSession.InjectCookies. The launched profile is
// always Stop()ped via defer, even on error.
func (d *Driver) Probe(
	ctx context.Context,
	indexURL string,
	cookies []*http.Cookie,
	profileID string,
) (*sitelogin.ProbeResult, error) {
	if d == nil || d.Manager == nil {
		return nil, errors.New("unit3d cloak driver: nil manager")
	}
	if indexURL == "" {
		return nil, errors.New("unit3d cloak driver: empty indexURL")
	}
	if profileID == "" {
		return nil, errors.New("unit3d cloak driver: empty profileID")
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
		sessionExpired   bool
	)
	err = session.LoadPageAndExtract(
		navCtx,
		indexURL,
		userInfoWidgetSelector,
		func(html string) error {
			if isUnit3DLoginPage(html) {
				sessionExpired = true
				return nil
			}
			var perr error
			parsedLastLogin, parsedLastAccess, perr = parseUnit3DUserPage(html)
			return perr
		},
	)
	if err != nil {
		return classifyChromedpError(err), nil
	}

	if sessionExpired {
		return &sitelogin.ProbeResult{
			Status:     sitelogin.SESSION_EXPIRED,
			Source:     sitelogin.ProbeSourceCloak,
			Diagnostic: "cloak: unit3d login page detected (session expired)",
		}, nil
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
		result.Diagnostic = "cloak: user widget rendered but no last_login/last_action found"
	}

	return result, nil
}

// userInfoWidgetSelector is a comma-OR set covering Unit3D user-info
// containers across forks (.user-info standard, .navbar-user nav variant,
// h1 fallback). Any one becoming visible proves the session rendered
// the user page (or the login page for session-expired detection).
const userInfoWidgetSelector = ".user-info, .navbar-user, h1"

// isUnit3DLoginPage detects a Unit3D session-expired login redirect.
// Unit3D returns either an explicit <title>Login</title> or a /login
// form action; both are non-localized across all known forks.
func isUnit3DLoginPage(html string) bool {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return false
	}
	title := strings.TrimSpace(doc.Find("title").First().Text())
	if strings.EqualFold(title, "login") {
		return true
	}
	hasLoginForm := false
	doc.Find("form").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		action, _ := s.Attr("action")
		if strings.HasSuffix(strings.TrimRight(action, "/"), "/login") || action == "/login" {
			hasLoginForm = true
			return false
		}
		return true
	})
	return hasLoginForm
}

// parseUnit3DUserPage extracts last_login and last_action from a Unit3D
// rendered user page. Unit3D uses CSS classes (.user-info__last-login,
// .user-info__last-action) plus a generic <time datetime="..."> attribute
// fallback for forks that customized the markup. Returns a parse error
// if neither field was extracted. Returned times are NOT UTC-normalised —
// callers must .UTC() before returning to scheduler downstream.
func parseUnit3DUserPage(html string) (lastLogin, lastAccess time.Time, err error) {
	doc, derr := goquery.NewDocumentFromReader(strings.NewReader(html))
	if derr != nil {
		err = fmt.Errorf("unit3d parse: %w", derr)
		return lastLogin, lastAccess, err
	}

	if v, ok := readTimeNode(doc.Find(".user-info__last-login").First()); ok {
		lastLogin = v
	}
	if v, ok := readTimeNode(doc.Find(".user-info__last-action").First()); ok {
		lastAccess = v
	}

	if lastLogin.IsZero() && lastAccess.IsZero() {
		doc.Find(".user-info li").Each(func(_ int, li *goquery.Selection) {
			label := strings.ToLower(strings.TrimSpace(li.Find(".user-info__label").Text()))
			if label == "" {
				return
			}
			t, ok := readTimeNode(li.Find("time").First())
			if !ok {
				return
			}
			switch {
			case strings.Contains(label, "last login"):
				if lastLogin.IsZero() {
					lastLogin = t
				}
			case strings.Contains(label, "last action"), strings.Contains(label, "last seen"):
				if lastAccess.IsZero() {
					lastAccess = t
				}
			}
		})
	}

	if lastLogin.IsZero() && lastAccess.IsZero() {
		err = errors.New("unit3d parse: no last_login/last_action found")
		return lastLogin, lastAccess, err
	}
	return lastLogin, lastAccess, nil
}

// readTimeNode prefers the machine-readable datetime= attribute (RFC3339)
// over the rendered text content. Falls back to text-content with the
// Laravel default layout assumed UTC.
func readTimeNode(sel *goquery.Selection) (time.Time, bool) {
	if sel.Length() == 0 {
		return time.Time{}, false
	}
	if dt, ok := sel.Attr("datetime"); ok {
		if t, parsed := parseTimestamp(strings.TrimSpace(dt)); parsed {
			return t, true
		}
	}
	text := strings.TrimSpace(sel.Text())
	if text == "" {
		return time.Time{}, false
	}
	return parseTimestamp(text)
}

var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05 (Z07:00)",
	"2006-01-02 15:04:05 Z07:00",
	"2006-01-02 15:04:05",
}

// parseTimestamp tries zone-aware layouts first, falling back to bare
// "2006-01-02 15:04:05" interpreted as UTC — the Laravel/Unit3D default
// shared with site/v2/unit3d_driver.go parseUnit3DTimestamp.
func parseTimestamp(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
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
