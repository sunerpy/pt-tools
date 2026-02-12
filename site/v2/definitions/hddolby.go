package definitions

import (
	"time"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

var HDDolbyDefinition = &v2.SiteDefinition{
	ID:              "hddolby",
	Name:            "HD Dolby",
	Aka:             []string{"高清杜比"},
	Description:     "高清杜比 - 需要同时配置 Cookie 和 RSS Key。RSS Key 用于搜索和下载，Cookie 用于获取时魔等信息",
	Schema:          v2.SchemaHDDolby,
	AuthMethod:      v2.AuthMethodCookieAndAPIKey,
	URLs:            []string{"https://www.hddolby.com/"},
	LegacyURLs:      []string{"https://hddolby.com/"},
	FaviconURL:      "https://www.hddolby.com/favicon.ico",
	Unavailable:     false,
	TimezoneOffset:  "+0800",
	RateLimit:       0.5,
	RateBurst:       3,
	RateWindow:      time.Hour,
	RateWindowLimit: 50,
	LevelRequirements: []v2.SiteLevelRequirement{
		{
			ID:        1,
			Name:      "User",
			Privilege: "新用户的默认级别。",
		},
		{
			ID:           2,
			Name:         "Power User",
			Interval:     "P2W",
			Downloaded:   "120GB",
			Ratio:        2.0,
			SeedingBonus: 60000,
			Privilege:    "得到0个邀请名额；可以直接发布种子；可以查看NFO文档；可以查看用户列表；可以请求续种；可以查看排行榜；可以查看其它用户的种子历史；可以删除自己上传的字幕。",
		},
		{
			ID:           3,
			Name:         "(连长)Elite User",
			NameAka:      []string{"Elite User"},
			Interval:     "P4W",
			Downloaded:   "256GB",
			Ratio:        2.5,
			SeedingBonus: 120000,
			Privilege:    "Elite User及以上用户封存账号后不会被删除。",
		},
		{
			ID:           4,
			Name:         "Crazy User",
			Interval:     "P8W",
			Downloaded:   "512GB",
			Ratio:        3.0,
			SeedingBonus: 240000,
			Privilege:    "得到0个邀请名额；可以在做种/下载/发布的时候选择匿名模式。",
		},
		{
			ID:           5,
			Name:         "Insane User",
			Interval:     "P12W",
			Downloaded:   "768GB",
			Ratio:        3.5,
			SeedingBonus: 360000,
			Privilege:    "无",
		},
		{
			ID:           6,
			Name:         "Veteran User",
			Interval:     "P20W",
			Downloaded:   "1TB",
			Ratio:        4.0,
			SeedingBonus: 600000,
			Privilege:    "可以查看其它用户的评论、帖子历史。",
		},
		{
			ID:           7,
			Name:         "Extreme User",
			Interval:     "P28W",
			Downloaded:   "2TB",
			Ratio:        4.5,
			SeedingBonus: 720000,
			Privilege:    "Extreme User及以上用户会永远保留账号。",
		},
		{
			ID:           8,
			Name:         "Ultimate User",
			Interval:     "P40W",
			Downloaded:   "4TB",
			Ratio:        5.0,
			SeedingBonus: 1200000,
			Privilege:    "无",
		},
		{
			ID:           9,
			Name:         "Nexus Master",
			Interval:     "P56W",
			Downloaded:   "8TB",
			Ratio:        5.5,
			SeedingBonus: 1680000,
			Privilege:    "无",
		},
	},
}

func init() {
	v2.RegisterSiteDefinition(HDDolbyDefinition)
}
