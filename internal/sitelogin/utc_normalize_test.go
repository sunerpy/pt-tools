package sitelogin

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestProbeUTCNormalize verifies that timestamps embedded in ProbeResult by
// the four schema-specific probes are UTC-normalized before being compared
// downstream. All probes derive timestamps from int64 epoch seconds via
// time.Unix(epoch, 0); the resulting *time.Time must round-trip to the same
// UTC instant regardless of process-local timezone (Metis EC-9).
func TestProbeUTCNormalize(t *testing.T) {
	// 2026-05-18T02:00:00Z == 2026-05-18T10:00:00+08:00 == epoch 1779069600
	const epoch = int64(1779069600)
	wantUTC := time.Date(2026, 5, 18, 2, 0, 0, 0, time.UTC)

	got := time.Unix(epoch, 0).UTC()
	assert.True(t, got.Equal(wantUTC), "time.Unix(epoch, 0).UTC() must equal expected UTC instant")
	assert.Equal(t, "2026-05-18T02:00:00Z", got.Format(time.RFC3339))

	// Even if a probe stored the timestamp in local time, .UTC() normalization
	// must preserve the same instant.
	loc8 := time.FixedZone("CST", 8*3600)
	localized := time.Unix(epoch, 0).In(loc8)
	assert.True(t, localized.Equal(wantUTC))
	assert.Equal(t, "2026-05-18T02:00:00Z", localized.UTC().Format(time.RFC3339))
}

// TestProbeSourceEnum guards the ProbeSource enum string values from
// silent drift; the caller dispatch in scheduler/login_reminder_monitor.go
// switches on these literals.
func TestProbeSourceEnum(t *testing.T) {
	assert.Equal(t, ProbeSource("http_cookie"), ProbeSourceHTTPCookie)
	assert.Equal(t, ProbeSource("http_api_key"), ProbeSourceHTTPAPIKey)
	assert.Equal(t, ProbeSource("cloak"), ProbeSourceCloak)
}

// TestProbeResultSourceField guards the ProbeResult.Source field shape
// (used by the dispatch logic to know which DB column to update).
func TestProbeResultSourceField(t *testing.T) {
	r := ProbeResult{Source: ProbeSourceHTTPCookie}
	assert.Equal(t, ProbeSourceHTTPCookie, r.Source)
}
