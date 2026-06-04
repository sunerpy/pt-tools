package models

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteLoginPresetMTeam(t *testing.T) {
	// Happy path: M-Team preset exists with correct values and Notes
	preset, ok := SiteLoginPresets["mteam"]
	require.True(t, ok, "mteam preset should exist in SiteLoginPresets")
	assert.Equal(t, 30, preset.BanThresholdDays, "M-Team BanThresholdDays should be 30")
	assert.Equal(t, 10, preset.RemindBeforeDays, "M-Team RemindBeforeDays should be 10")
	assert.NotEmpty(t, preset.Notes, "M-Team Notes field must be non-empty (cite source)")
	assert.True(t, strings.Contains(strings.ToLower(preset.Notes), "m-team") || strings.Contains(strings.ToLower(preset.Notes), "mteam"), "Notes should mention M-Team")
}

func TestSiteLoginPresetUnknownFallback(t *testing.T) {
	// Negative case: Unknown site falls back to defaults
	banDays, remindDays, hasPreset := ApplyPresetIfMissing("nonexistent-site")
	assert.False(t, hasPreset, "nonexistent site should return hasPreset=false")
	assert.Equal(t, DefaultBanThresholdDays, banDays, "unknown site should use DefaultBanThresholdDays")
	assert.Equal(t, DefaultRemindBeforeDays, remindDays, "unknown site should use DefaultRemindBeforeDays")
	assert.Equal(t, 30, banDays, "default ban threshold should be 30")
	assert.Equal(t, 10, remindDays, "default remind days should be 10")
}

func TestSiteLoginPresetCaseInsensitive(t *testing.T) {
	// Robustness: Case-insensitive lookup (MTEAM vs mteam)
	banDays, remindDays, hasPreset := ApplyPresetIfMissing("MTEAM")
	assert.True(t, hasPreset, "MTEAM (uppercase) should match mteam preset")
	assert.Equal(t, 30, banDays, "uppercase MTEAM should resolve to preset value 30")
	assert.Equal(t, 10, remindDays, "uppercase MTEAM should resolve to preset value 10")

	// Mixed case variant
	banDays2, remindDays2, hasPreset2 := ApplyPresetIfMissing("MTeam")
	assert.True(t, hasPreset2, "MTeam (mixed case) should match mteam preset")
	assert.Equal(t, 30, banDays2)
	assert.Equal(t, 10, remindDays2)
}

func TestSiteLoginPresetAllHaveNotes(t *testing.T) {
	// Governance: Every preset must have a non-empty Notes field
	// This forces future contributors to cite sources
	for siteName, preset := range SiteLoginPresets {
		assert.NotEmpty(t, preset.Notes, "preset for site '%s' must have non-empty Notes (cite source: URL, screenshot, date, etc)", siteName)
		assert.NotEmpty(t, preset.BanThresholdDays, "preset for site '%s' must have positive BanThresholdDays", siteName)
		assert.NotEmpty(t, preset.RemindBeforeDays, "preset for site '%s' must have positive RemindBeforeDays", siteName)
	}
}
