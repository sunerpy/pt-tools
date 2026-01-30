package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// MTeamDefinition is the site definition for M-Team
var MTeamDefinition = &v2.SiteDefinition{
	ID:          "mteam",
	Name:        "M-Team - TP",
	Aka:         []string{"MTeam", "馒头"},
	Description: "综合性网站，有分享率要求",
	Schema:      "mTorrent",
	RateLimit:   1.0,
	RateBurst:   3,
	URLs: []string{
		"https://api.m-team.cc",
		"https://kp.m-team.cc",
		"https://zp.m-team.io",
		"https://xp.m-team.cc",
		"https://ap.m-team.cc",
		"https://next.m-team.cc",
		"https://ob.m-team.cc",
	},
	LegacyURLs:     []string{"https://xp.m-team.io/", "https://pt.m-team.cc/", "https://tp.m-team.cc/"},
	FaviconURL:     "https://kp.m-team.cc/favicon.ico",
	TimezoneOffset: "+0800",
	UserInfo: &v2.UserInfoConfig{
		PickLast:     []string{"id", "name", "joinTime"},
		RequestDelay: 300,
		Process: []v2.UserInfoProcess{
			{
				RequestConfig: v2.RequestConfig{
					URL:          "/api/member/profile",
					Method:       "POST",
					ResponseType: "json",
				},
				Fields: []string{
					"id", "name", "joinTime", "uploaded", "downloaded",
					"levelName", "levelId", "bonus", "lastAccessAt",
				},
			},
			{
				RequestConfig: v2.RequestConfig{
					URL:          "/api/tracker/myPeerStatistics",
					Method:       "POST",
					ResponseType: "json",
				},
				Fields: []string{"seeding", "seedingSize", "uploads"},
			},
			{
				RequestConfig: v2.RequestConfig{
					URL:          "/api/tracker/mybonus",
					Method:       "POST",
					ResponseType: "json",
				},
				Fields: []string{"bonusPerHour"},
			},
			{
				RequestConfig: v2.RequestConfig{
					URL:          "/api/msg/notify/statistic",
					Method:       "POST",
					ResponseType: "json",
				},
				Fields: []string{"messageCount"},
			},
		},
		Selectors: map[string]v2.FieldSelector{
			"id": {
				Selector: []string{"data.id"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			"name": {
				Selector: []string{"data.username"},
			},
			"joinTime": {
				Selector: []string{"data.createdDate"},
				Filters:  []v2.Filter{{Name: "parseTime"}},
			},
			"uploaded": {
				Selector: []string{"data.memberCount.uploaded"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			"downloaded": {
				Selector: []string{"data.memberCount.downloaded"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			"levelName": {
				Selector: []string{"data.role"},
			},
			"levelId": {
				Selector: []string{"data.role"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			"bonus": {
				Selector: []string{"data.memberCount.bonus"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			"lastAccessAt": {
				Selector: []string{"data.memberStatus.lastBrowse"},
				Filters:  []v2.Filter{{Name: "parseTime"}},
			},
			"seeding": {
				Selector: []string{"data.seederCount"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			"seedingSize": {
				Selector: []string{"data.seederSize"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			"uploads": {
				Selector: []string{"data.uploadCount"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			"bonusPerHour": {
				Selector: []string{"data.formulaParams.finalBs"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			"messageCount": {
				Selector: []string{"data.unMake"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
		},
	},
	LevelRequirements: []v2.SiteLevelRequirement{
		{
			ID:   1,
			Name: "User",
		},
		{
			ID:         2,
			Name:       "Power User",
			Interval:   "P4W",
			Downloaded: "200GB",
			Ratio:      2,
			Privilege:  "魔力值加成：+1%；可以使用匿名發表候選種子；可以上傳字幕",
		},
		{
			ID:         3,
			Name:       "Elite User",
			Interval:   "P8W",
			Downloaded: "400GB",
			Ratio:      3,
			Privilege:  "魔力值加成：+2%；可以發送邀請；可以管理自己上傳的字幕；可以檢視別人的下載紀錄；可以使用個性條",
		},
		{
			ID:         4,
			Name:       "Crazy User",
			Interval:   "P12W",
			Downloaded: "500GB",
			Ratio:      4,
			Privilege:  "魔力值加成：+3%",
		},
		{
			ID:         5,
			Name:       "Insane User",
			Interval:   "P16W",
			Downloaded: "800GB",
			Ratio:      5,
			Privilege:  "魔力值加成：+4%；可以檢視排行榜",
		},
		{
			ID:         6,
			Name:       "Veteran User",
			Interval:   "P20W",
			Downloaded: "1000GB",
			Ratio:      6,
			Privilege:  "魔力值加成：+5%；封存帳號後不會被刪除帳號",
		},
		{
			ID:         7,
			Name:       "Extreme User",
			Interval:   "P24W",
			Downloaded: "2000GB",
			Ratio:      7,
			Privilege:  "魔力值加成：+6%；永遠保留",
		},
		{
			ID:         8,
			Name:       "Ultimate User",
			Interval:   "P28W",
			Downloaded: "2500GB",
			Ratio:      8,
			Privilege:  "魔力值加成：+7%",
		},
		{
			ID:         9,
			Name:       "Nexus Master",
			Interval:   "P32W",
			Downloaded: "3000GB",
			Ratio:      9,
			Privilege:  "魔力值加成：+8%",
		},
		{
			ID:        100,
			Name:      "VIP",
			GroupType: v2.LevelGroupVIP,
		},
	},
}

// MTeamLevelIDMap maps API role ID to level name
var MTeamLevelIDMap = map[string]string{
	"1":  "User",
	"2":  "Power User",
	"3":  "Elite User",
	"4":  "Crazy User",
	"5":  "Insane User",
	"6":  "Veteran User",
	"7":  "Extreme User",
	"8":  "Ultimate User",
	"9":  "Nexus Master",
	"10": "VIP",
}

// GetMTeamLevelName returns level name from role ID
func GetMTeamLevelName(roleID string) string {
	if name, ok := MTeamLevelIDMap[roleID]; ok {
		return name
	}
	return roleID
}

func init() {
	v2.RegisterSiteDefinition(MTeamDefinition)
}
