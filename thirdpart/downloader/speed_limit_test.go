package downloader

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAddTorrentOptions_EffectiveUploadLimitBytes verifies the priority chain
// and unit conversion for the upload speed limit field on AddTorrentOptions.
//
// This is a regression guard for the v0.26 per-site speed limit feature
// (issue #276). Key contracts:
//   - KBs field (new) takes priority over MB field (deprecated)
//   - 0 means "unlimited" (no bounds)
//   - Returned value is bytes/second (NOT KB/s or MB/s)
func TestAddTorrentOptions_EffectiveUploadLimitBytes(t *testing.T) {
	tests := []struct {
		name      string
		opts      AddTorrentOptions
		wantBytes int64
	}{
		{"no limit set", AddTorrentOptions{}, 0},
		{"KBs set to 500 → 500*1024 bytes", AddTorrentOptions{UploadSpeedLimitKBs: 500}, 500 * 1024},
		{"MB set to 2 → 2*1024*1024 bytes", AddTorrentOptions{UploadSpeedLimitMB: 2}, 2 * 1024 * 1024},
		{"both set, KBs wins", AddTorrentOptions{UploadSpeedLimitMB: 5, UploadSpeedLimitKBs: 100}, 100 * 1024},
		{"negative KBs treated as unset", AddTorrentOptions{UploadSpeedLimitKBs: -5}, 0},
		{"negative MB treated as unset", AddTorrentOptions{UploadSpeedLimitMB: -1}, 0},
		{"zero KBs + positive MB falls back to MB", AddTorrentOptions{UploadSpeedLimitKBs: 0, UploadSpeedLimitMB: 3}, 3 * 1024 * 1024},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantBytes, tt.opts.EffectiveUploadLimitBytes())
		})
	}
}

// TestAddTorrentOptions_EffectiveDownloadLimitBytes verifies the download
// speed limit field produces bytes/second correctly.
func TestAddTorrentOptions_EffectiveDownloadLimitBytes(t *testing.T) {
	tests := []struct {
		name      string
		opts      AddTorrentOptions
		wantBytes int64
	}{
		{"no limit set", AddTorrentOptions{}, 0},
		{"KBs=1000 → 1000*1024 bytes", AddTorrentOptions{DownloadSpeedLimitKBs: 1000}, 1000 * 1024},
		{"KBs=0 → 0", AddTorrentOptions{DownloadSpeedLimitKBs: 0}, 0},
		{"negative treated as unset", AddTorrentOptions{DownloadSpeedLimitKBs: -10}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantBytes, tt.opts.EffectiveDownloadLimitBytes())
		})
	}
}
