// Package gazelle implements the Gazelle-schema CloakBrowser driver
// (T13). The driver loads /user.php?id=<self> via CDP — NOT
// /ajax.php?action=user — because research showed the WhatCD-fork ajax
// endpoint does NOT update LastAccess on every fork (only OPSnet does).
// The HTML user.php page reliably advances the LastAccess clock across
// every Gazelle fork that pt-tools targets.
//
// Gazelle has NO last_login field — only LastAccess. This driver
// therefore populates only ProbeResult.LastAccessAt; LastLoginAt stays
// nil, and the consistency check (T5) treats nil correctly.
package gazelle

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

type Driver struct {
	Manager cloakdriver.ManagerClient
}

func NewDriver(manager cloakdriver.ManagerClient) *Driver {
	return &Driver{Manager: manager}
}

// Probe runs the full Gazelle CloakBrowser lifecycle (launch → CDP →
// inject cookies → navigate to /user.php?id=<self> → extract LastAccess
// → close). LastLoginAt is intentionally never populated — Gazelle has
// no such field.
//
// indexURL must be the full /user.php?id=<self> URL — the caller
// (dispatcher T4) is responsible for resolving the user ID.
func (d *Driver) Probe(
	ctx context.Context,
	indexURL string,
	cookies []*http.Cookie,
	profileID string,
) (*sitelogin.ProbeResult, error) {
	if d == nil || d.Manager == nil {
		return nil, errors.New("gazelle cloak driver: nil manager")
	}
	if indexURL == "" {
		return nil, errors.New("gazelle cloak driver: empty indexURL")
	}
	if profileID == "" {
		return nil, errors.New("gazelle cloak driver: empty profileID")
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

	var parsedLastAccess time.Time
	err = session.LoadPageAndExtract(
		navCtx,
		indexURL,
		userInfoWidgetSelector,
		func(html string) error {
			la, perr := parseGazelleUserPage(html)
			if perr != nil {
				return perr
			}
			parsedLastAccess = la
			return nil
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
		result.Diagnostic = "cloak: gazelle user.php rendered but no LastAccess row matched"
	}

	return result, nil
}

// userInfoWidgetSelector covers Gazelle stats-box variants across forks
// (.box_userinfo_stats canonical, .stats fallback, h2 user-page header
// fallback). Any one becoming visible proves the user.php page rendered.
const userInfoWidgetSelector = ".box_userinfo_stats, ul.stats, h2"

// parseGazelleUserPage extracts LastAccess from a Gazelle /user.php
// stats list. Gazelle stores the canonical timestamp in the
// `<span class="time" title="YYYY-MM-DD HH:MM:SS">` title attribute —
// the visible text is a relative "2 hours ago" string we ignore.
//
// Gazelle convention is server-side UTC (per the OPSnet/RED source
// reference); callers MUST .UTC() the returned value (the Probe method
// already does this) but the value here is interpreted as UTC directly.
//
// Returns parse error if no recognized "Last seen" / "Last access" row
// matched, so the reminder pipeline does not silently keep stale
// timestamps when the user enabled paranoia mode.
func parseGazelleUserPage(html string) (lastAccess time.Time, err error) {
	doc, derr := goquery.NewDocumentFromReader(strings.NewReader(html))
	if derr != nil {
		err = fmt.Errorf("gazelle parse: %w", derr)
		return lastAccess, err
	}

	matched := false
	doc.Find("li").EachWithBreak(func(_ int, li *goquery.Selection) bool {
		text := strings.ToLower(strings.TrimSpace(li.Text()))
		if !isLastAccessLabel(text) {
			return true
		}
		title, ok := li.Find("span.time").First().Attr("title")
		if !ok {
			title = strings.TrimSpace(li.Find("span.time").First().Text())
		}
		title = strings.TrimSpace(title)
		if title == "" {
			return true
		}
		if t, ok := parseGazelleTimestamp(title); ok {
			lastAccess = t
			matched = true
			return false
		}
		return true
	})

	if !matched {
		err = errors.New("gazelle parse: no LastAccess row found")
		return lastAccess, err
	}
	return lastAccess, nil
}

// isLastAccessLabel matches the localized labels Gazelle uses for the
// LastAccess row across forks (English canonical "last seen"; some
// forks render "last access" / "last visit").
func isLastAccessLabel(s string) bool {
	for _, prefix := range lastAccessLabels {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

var lastAccessLabels = []string{
	"last seen",
	"last access",
	"last visit",
}

var gazelleTimeLayouts = []string{
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05Z",
	"2006-01-02 15:04:05 -0700",
}

// parseGazelleTimestamp parses a Gazelle title-attribute timestamp.
// Gazelle stores server-side UTC by convention; the bare layout is
// interpreted as UTC (NOT CST as NexusPHP uses).
func parseGazelleTimestamp(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range gazelleTimeLayouts {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
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
		case strings.Contains(msg, "no lastaccess"):
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
