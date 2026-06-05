package models

import "strings"

const (
	DefaultBanThresholdDays = 30
	DefaultRemindBeforeDays = 10
)

type SiteLoginPreset struct {
	BanThresholdDays   int
	RemindBeforeDays   int
	ProbeJitterSeconds int
	Notes              string
}

var SiteLoginPresets = map[string]SiteLoginPreset{
	"mteam": {
		BanThresholdDays: DefaultBanThresholdDays,
		RemindBeforeDays: DefaultRemindBeforeDays,
		Notes:            "M-Team default inactivity reminder policy; fallback 30/10 until site-specific source is confirmed.",
	},
}

func ApplyPresetIfMissing(siteName string) (banThresholdDays, remindBeforeDays int, hasPreset bool) {
	preset, ok := SiteLoginPresets[strings.ToLower(strings.TrimSpace(siteName))]
	if !ok {
		return DefaultBanThresholdDays, DefaultRemindBeforeDays, false
	}
	return preset.BanThresholdDays, preset.RemindBeforeDays, true
}
