package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

var MooKoDefinition = &v2.SiteDefinition{
	ID:             "mooko",
	Name:           "MooKo",
	Aka:            []string{"MK"},
	Description:    "MooKo is a CHINESE Private Torrent Tracker for MOVIES / TV / GENERAL",
	Schema:         v2.SchemaGazelle,
	URLs:           []string{"https://mooko.org/"},
	FaviconURL:     "https://mooko.org/favicon.ico",
	TimezoneOffset: "+0800",
	RateLimit:      0.5,
	RateBurst:      2,
	HREnabled:      true,
	// HR 押金机制（奖励型）：做种时长按种子体积分档 + 7 天窗口期
	// 规则来源: https://wiki.mooko.org/rules/hr
	HRCalcSeedTime: v2.NewSizeTieredHRCalc(
		[]v2.HRSeedTimeRule{
			{MinSizeGB: 0, MaxSizeGB: 10, SeedTimeH: 36},    // 0-10 GiB:  36h
			{MinSizeGB: 10, MaxSizeGB: 20, SeedTimeH: 72},   // 10-20 GiB: 72h
			{MinSizeGB: 20, MaxSizeGB: 50, SeedTimeH: 120},  // 20-50 GiB: 120h
			{MinSizeGB: 50, MaxSizeGB: 200, SeedTimeH: 168}, // 50-200 GiB: 168h
			{MinSizeGB: 200, MaxSizeGB: 0, SeedTimeH: 336},  // 200+ GiB:  336h
		},
		168, // 7 天窗口期
	),
	// LevelRequirements: 待补充，目前 MooKo 未公开完整等级要求表。
	// 已知等级：User, Member, Power User, Elite, Torrent Master,
	// Power Torrent Master, Elite Torrent Master, Guru
}

func init() {
	v2.RegisterSiteDefinition(MooKoDefinition)
}
