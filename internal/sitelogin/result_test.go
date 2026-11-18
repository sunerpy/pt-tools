package sitelogin

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestProbeStatusValues(t *testing.T) {
	tests := []struct {
		name   string
		status ProbeStatus
		want   string
	}{
		{"OK", OK, "OK"},
		{"SESSION_EXPIRED", SESSION_EXPIRED, "SESSION_EXPIRED"},
		{"CHALLENGE", CHALLENGE, "CHALLENGE"},
		{"RATE_LIMITED", RATE_LIMITED, "RATE_LIMITED"},
		{"NETWORK_ERROR", NETWORK_ERROR, "NETWORK_ERROR"},
		{"PARSE_ERROR", PARSE_ERROR, "PARSE_ERROR"},
		{"KEY_ERROR", KEY_ERROR, "KEY_ERROR"},
		{"UNKNOWN", UNKNOWN, "UNKNOWN"},
		{"NOT_APPLICABLE", NOT_APPLICABLE, "NOT_APPLICABLE"},
		{"NOT_CONFIGURED", NOT_CONFIGURED, "NOT_CONFIGURED"},
		{"UNSUPPORTED", UNSUPPORTED, "UNSUPPORTED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, string(tt.status))
			assert.Equal(t, tt.want, tt.status.String())
		})
	}
}

func TestProbeStatusClassification(t *testing.T) {
	tests := []struct {
		status     ProbeStatus
		failure    bool
		credential bool
	}{
		{OK, false, false},
		{NOT_CONFIGURED, false, false},
		{UNSUPPORTED, false, false},
		{NOT_APPLICABLE, false, false},
		{"", false, false},
		{SESSION_EXPIRED, true, true},
		{KEY_ERROR, true, true},
		{CHALLENGE, true, false},
		{RATE_LIMITED, true, false},
		{NETWORK_ERROR, true, false},
		{PARSE_ERROR, true, false},
		{UNKNOWN, true, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.failure, tt.status.IsFailure())
			assert.Equal(t, tt.credential, tt.status.IsCredentialFailure())
		})
	}
}

func TestProbeResultZeroValue(t *testing.T) {
	var zr ProbeResult
	assert.Equal(t, ProbeStatus(""), zr.Status)
	assert.Nil(t, zr.LastLoginAt)
	assert.Nil(t, zr.LastAccessAt)
	assert.Nil(t, zr.RawError)
	assert.Equal(t, "", zr.Diagnostic)
}

func TestProbeResultWithValues(t *testing.T) {
	now := time.Now()
	err := v2.ErrSessionExpired
	diag := "test diagnostic"

	pr := ProbeResult{
		Status:       OK,
		LastLoginAt:  &now,
		LastAccessAt: &now,
		RawError:     err,
		Diagnostic:   diag,
	}

	assert.Equal(t, OK, pr.Status)
	assert.Equal(t, &now, pr.LastLoginAt)
	assert.Equal(t, &now, pr.LastAccessAt)
	assert.Equal(t, err, pr.RawError)
	assert.Equal(t, diag, pr.Diagnostic)
}
