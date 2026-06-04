// Package cloakdriver provides Go bindings for CloakBrowser-Manager REST API
// and CDP automation. The package is the v2.0 fallback transport for sites
// where cookie-only HTTP probing is blocked by Cloudflare/ja3 fingerprinting.
package cloakdriver

import (
	"context"
	"errors"
	"time"
)

// ProfileLaunchResult is what Manager returns after POST /api/profiles/{id}/launch.
type ProfileLaunchResult struct {
	ProfileID string    `json:"profile_id"`
	CdpURL    string    `json:"cdp_url"`
	VncURL    string    `json:"vnc_url,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

// ProfileStatus is what Manager returns from GET /api/profiles/{id}/status.
type ProfileStatus struct {
	ProfileID string `json:"profile_id"`
	Running   bool   `json:"running"`
	CdpURL    string `json:"cdp_url,omitempty"`
}

// ManagerStatusInfo is the Manager-level status payload returned by GET /api/status.
// Used by the test-connection endpoint to report Manager version on success.
type ManagerStatusInfo struct {
	Status  string `json:"status,omitempty"`
	Version string `json:"version,omitempty"`
}

// ManagerClient is the abstraction over CloakBrowser-Manager REST.
// Implementations: real HTTP client (manager_client.go), httptest mock (tests).
type ManagerClient interface {
	LaunchProfile(ctx context.Context, profileID string) (*ProfileLaunchResult, error)
	GetProfileStatus(ctx context.Context, profileID string) (*ProfileStatus, error)
	StopProfile(ctx context.Context, profileID string) error
	DeleteProfile(ctx context.Context, profileID string) error
	ManagerStatus(ctx context.Context) error // GET /api/status — used by test-connection
	ManagerStatusFull(ctx context.Context) (*ManagerStatusInfo, error)
}

// Sentinel errors classify Manager failures (R19).
var (
	ErrManagerNotFound      = errors.New("cloakbrowser-manager: profile not found")
	ErrManagerAuthFailed    = errors.New("cloakbrowser-manager: auth failed (check token)")
	ErrManagerServerError   = errors.New("cloakbrowser-manager: 5xx server error")
	ErrManagerDNSFailed     = errors.New("cloakbrowser-manager: DNS resolution failed")
	ErrManagerConnRefused   = errors.New("cloakbrowser-manager: connection refused")
	ErrManagerTimeout       = errors.New("cloakbrowser-manager: timeout")
	ErrManagerProtocolError = errors.New("cloakbrowser-manager: protocol/parse error")
)
