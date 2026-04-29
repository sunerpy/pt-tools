package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
// FilterRule.MatchesSize - exhaustive size-bound behavior
// ============================================================================

func TestFilterRule_MatchesSize_AllCombinations(t *testing.T) {
	tests := []struct {
		name   string
		min    int
		max    int
		sizeGB float64
		want   bool
	}{
		// No bounds
		{"no bounds / zero size", 0, 0, 0, true},
		{"no bounds / huge size", 0, 0, 9999, true},
		// Min only
		{"min=10 / below", 10, 0, 5, false},
		{"min=10 / at boundary", 10, 0, 10, true},
		{"min=10 / above", 10, 0, 20, true},
		// Max only
		{"max=50 / below", 0, 50, 25, true},
		{"max=50 / at boundary", 0, 50, 50, true},
		{"max=50 / above", 0, 50, 100, false},
		// Both bounds
		{"min=10 max=50 / below min", 10, 50, 5, false},
		{"min=10 max=50 / at min", 10, 50, 10, true},
		{"min=10 max=50 / middle", 10, 50, 30, true},
		{"min=10 max=50 / at max", 10, 50, 50, true},
		{"min=10 max=50 / above max", 10, 50, 100, false},
		// Fractional sizes
		{"min=5 / fractional at boundary", 5, 0, 5.0, true},
		{"min=5 / fractional just below", 5, 0, 4.999, false},
		{"max=10 / fractional just above", 0, 10, 10.001, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := &FilterRule{MinSizeGB: tt.min, MaxSizeGB: tt.max}
			assert.Equal(t, tt.want, rule.MatchesSize(tt.sizeGB))
		})
	}
}

// ============================================================================
// NormalizeFilterMode — enum validation and fallback
// ============================================================================

func TestNormalizeFilterMode(t *testing.T) {
	tests := []struct {
		input FilterMode
		want  FilterMode
	}{
		{FilterModeAutoFree, FilterModeAutoFree},
		{FilterModeFilterOnly, FilterModeFilterOnly},
		{FilterModeFreeOnly, FilterModeFreeOnly},
		{"", DefaultFilterMode},
		{"bogus", DefaultFilterMode},
		{"AUTO_FREE", DefaultFilterMode},
	}
	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeFilterMode(tt.input))
		})
	}
}

// ============================================================================
// RSSConfig.GetEffectiveFilterMode — priority chain: RSS > Global > Default
// ============================================================================

func TestRSSConfig_GetEffectiveFilterMode(t *testing.T) {
	tests := []struct {
		name        string
		rssMode     FilterMode
		globalMode  FilterMode
		wantMode    FilterMode
		nilSettings bool
	}{
		{"RSS filter_only overrides global auto_free", FilterModeFilterOnly, FilterModeAutoFree, FilterModeFilterOnly, false},
		{"RSS free_only overrides global", FilterModeFreeOnly, FilterModeAutoFree, FilterModeFreeOnly, false},
		{"empty RSS falls back to global free_only", "", FilterModeFreeOnly, FilterModeFreeOnly, false},
		{"empty RSS + empty global = default", "", "", DefaultFilterMode, false},
		{"empty RSS + nil global settings = default", "", "", DefaultFilterMode, true},
		{"invalid RSS mode falls back to default", "bogus", FilterModeFilterOnly, DefaultFilterMode, false},
		{"invalid global mode falls back to default", "", "bogus", DefaultFilterMode, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RSSConfig{FilterMode: tt.rssMode}
			var gs *SettingsGlobal
			if !tt.nilSettings {
				gs = &SettingsGlobal{DefaultFilterMode: tt.globalMode}
			}
			assert.Equal(t, tt.wantMode, r.GetEffectiveFilterMode(gs))
		})
	}
}
