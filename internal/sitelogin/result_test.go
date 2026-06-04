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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, string(tt.status))
			assert.Equal(t, tt.want, tt.status.String())
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
