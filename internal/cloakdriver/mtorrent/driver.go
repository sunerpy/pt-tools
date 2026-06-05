// Package mtorrent implements the mTorrent (M-Team) CloakBrowser driver.
//
// mTorrent is special among the four schema drivers (T11-T14): the v1 API key
// path (internal/sitelogin/probe_mtorrent.go, ProbeSourceHTTPAPIKey) is the
// primary authentication channel and remains untouched. The CloakBrowser path
// here is a *verification* fallback used ONLY when the user has optionally
// supplied a cookie (Q-detail-1=a) so the cookie-derived lastModifiedDate can
// be cross-checked against the API timestamp by the consistency checker (T5).
//
// Contract: when no cookie is configured, Probe returns
// Status=NOT_APPLICABLE and DOES NOT invoke the Manager — neither
// LaunchProfile nor StopProfile is called. M-Team has no separate last_login
// concept, so only LastAccessAt is populated on success; LastLoginAt is
// always nil for this driver.
package mtorrent

import (
	"context"
	"encoding/json"
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

// Driver is the mTorrent CloakBrowser cookie-verification transport.
type Driver struct {
	Manager cloakdriver.ManagerClient
}

// NewDriver constructs an mTorrent CloakBrowser driver bound to the given
// Manager client (T8).
func NewDriver(manager cloakdriver.ManagerClient) *Driver {
	return &Driver{Manager: manager}
}

// Probe executes the cookie-verification CloakBrowser path for an M-Team site.
//
// Lifecycle: cookie-empty short-circuit (NOT_APPLICABLE, no Manager call) →
// arg validation → launch profile → CDP connect → inject cookies → navigate
// to /profile → extract lastModifiedDate → close.
//
// The (result, err) tuple follows v1 probe convention: classification errors
// are encoded into the result; only programming-level errors (nil manager,
// empty URL, empty profile) return non-nil err. Non-nil err is bypassed for
// the cookie-empty case because that is the documented happy-path exit when
// the user has not opted into cookie verification.
func (d *Driver) Probe(
	ctx context.Context,
	indexURL string,
	cookies []*http.Cookie,
	profileID string,
) (*sitelogin.ProbeResult, error) {
	if len(cookies) == 0 && d != nil && d.Manager != nil && indexURL != "" && profileID != "" {
		return &sitelogin.ProbeResult{
			Status:     sitelogin.NOT_APPLICABLE,
			Source:     sitelogin.ProbeSourceCloak,
			Diagnostic: "mtorrent cloak: cookie not configured; verification path skipped",
		}, nil
	}

	if d == nil || d.Manager == nil {
		return nil, errors.New("mtorrent cloak driver: nil manager")
	}
	if indexURL == "" {
		return nil, errors.New("mtorrent cloak driver: empty indexURL")
	}
	if profileID == "" {
		return nil, errors.New("mtorrent cloak driver: empty profileID")
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
			Diagnostic: "mtorrent cloak: cdp session open failed",
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
			Diagnostic: "mtorrent cloak: cookie injection failed",
		}, nil
	}

	navCtx, navCancel := context.WithTimeout(session.TaskContext(), probeNavTimeout)
	defer navCancel()

	var parsedLastAccess time.Time
	err = session.LoadPageAndExtract(
		navCtx,
		indexURL,
		profileWidgetSelector,
		func(html string) error {
			var perr error
			parsedLastAccess, perr = parseMTorrentProfilePage(html)
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
	if !parsedLastAccess.IsZero() {
		la := parsedLastAccess.UTC()
		result.LastAccessAt = &la
	}
	if result.LastAccessAt == nil {
		result.Status = sitelogin.PARSE_ERROR
		result.Diagnostic = "mtorrent cloak: profile page rendered but no lastModifiedDate matched"
	}
	return result, nil
}

// profileWidgetSelector is a comma-OR set covering M-Team profile-page
// containers. Any one becoming visible proves the session rendered the
// profile page (post-Cloudflare).
const profileWidgetSelector = ".profile-card, [data-testid='profile-card'], #app section"

// parseMTorrentProfilePage extracts lastModifiedDate from a rendered
// /profile page. M-Team frontend exposes the value via two channels:
//  1. JSON-LD <script type="application/ld+json"> with mainEntity.lastModifiedDate
//     (or top-level dateModified)
//  2. CSS-selector dd.lastModifiedDate text
//
// The JSON-LD path is preferred (timezone-aware ISO 8601). Both layouts use
// RFC 3339 timestamps. Returned time is NOT UTC-normalised — caller must
// .UTC() before returning to scheduler downstream.
func parseMTorrentProfilePage(html string) (time.Time, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return time.Time{}, fmt.Errorf("mtorrent parse: %w", err)
	}

	var found time.Time
	doc.Find("script[type='application/ld+json']").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		raw := strings.TrimSpace(s.Text())
		if raw == "" {
			return true
		}
		var obj map[string]any
		if jerr := json.Unmarshal([]byte(raw), &obj); jerr != nil {
			return true
		}
		if t, ok := extractLastModifiedFromJSONLD(obj); ok {
			found = t
			return false
		}
		return true
	})
	if !found.IsZero() {
		return found, nil
	}

	if text := strings.TrimSpace(doc.Find(".lastModifiedDate").First().Text()); text != "" {
		if t, ok := parseTimestamp(text); ok {
			return t, nil
		}
	}
	if text := strings.TrimSpace(doc.Find(".lastBrowse").First().Text()); text != "" {
		if t, ok := parseTimestamp(text); ok {
			return t, nil
		}
	}
	return time.Time{}, errors.New("mtorrent parse: no lastModifiedDate found")
}

func extractLastModifiedFromJSONLD(obj map[string]any) (time.Time, bool) {
	if v, ok := obj["lastModifiedDate"].(string); ok {
		if t, p := parseTimestamp(v); p {
			return t, true
		}
	}
	if v, ok := obj["dateModified"].(string); ok {
		if t, p := parseTimestamp(v); p {
			return t, true
		}
	}
	if me, ok := obj["mainEntity"].(map[string]any); ok {
		if t, found := extractLastModifiedFromJSONLD(me); found {
			return t, true
		}
	}
	return time.Time{}, false
}

var timeLayouts = []string{
	time.RFC3339,
	time.RFC3339Nano,
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02 15:04:05Z07:00",
	"2006-01-02 15:04:05 (Z07:00)",
	"2006-01-02 15:04:05",
}

// parseTimestamp tries zone-aware layouts first; falls back to bare layouts
// interpreted in CST (UTC+8) to match M-Team's documented server timezone.
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
		r.Diagnostic = "mtorrent cloak: manager auth failed (check token)"
	case errors.Is(err, cloakdriver.ErrManagerNotFound):
		r.Status = sitelogin.UNKNOWN
		r.Diagnostic = "mtorrent cloak: manager profile not found"
	case errors.Is(err, cloakdriver.ErrManagerServerError),
		errors.Is(err, cloakdriver.ErrManagerDNSFailed),
		errors.Is(err, cloakdriver.ErrManagerConnRefused),
		errors.Is(err, cloakdriver.ErrManagerTimeout):
		r.Status = sitelogin.NETWORK_ERROR
		r.Diagnostic = "mtorrent cloak: manager unreachable"
	case errors.Is(err, cloakdriver.ErrManagerProtocolError):
		r.Status = sitelogin.UNKNOWN
		r.Diagnostic = "mtorrent cloak: manager protocol error"
	default:
		r.Status = sitelogin.UNKNOWN
		r.Diagnostic = "mtorrent cloak: manager error"
	}
	return r
}

func classifyChromedpError(err error) *sitelogin.ProbeResult {
	r := &sitelogin.ProbeResult{
		Source:   sitelogin.ProbeSourceCloak,
		RawError: err,
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		r.Status = sitelogin.CHALLENGE
		r.Diagnostic = "mtorrent cloak: navigate/wait deadline exceeded (likely Cloudflare challenge)"
	case errors.Is(err, context.Canceled):
		r.Status = sitelogin.UNKNOWN
		r.Diagnostic = "mtorrent cloak: context canceled"
	default:
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "no lastmodifieddate"):
			r.Status = sitelogin.PARSE_ERROR
			r.Diagnostic = "mtorrent cloak: page rendered but parse failed"
		case strings.Contains(msg, "challenge"), strings.Contains(msg, "cloudflare"):
			r.Status = sitelogin.CHALLENGE
			r.Diagnostic = "mtorrent cloak: challenge page detected"
		default:
			r.Status = sitelogin.NETWORK_ERROR
			r.Diagnostic = "mtorrent cloak: chromedp error"
		}
	}
	return r
}
