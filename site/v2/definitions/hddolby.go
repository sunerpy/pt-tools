package definitions

import (
	"time"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// HDDolbyDefinition 是高清杜比（域名换成了 www.uhddolby.com，站点改名 UHDDolby）。
// 2026-11-02 按站点页面、API 与 wiki（wiki.newdobly.cc）核对：
//   - 搜索、用户数据、做种列表与下载都走站点 API（www.uhddolby.com/api/v1，以前在 api. 子域名），
//     认 x-api-key，值是 RSS 链接里的 RSS key（RSS 页生成，有效期 30～180 天，到期前要重新生成）。
//     wiki 给 MoviePilot 的配置也是只填 API Key、不要 Cookie；API 每小时最多 50 次请求，超了会被冷却。
//   - Cookie 可以不填，填了用于时魔（API 没有）、签到，以及只知道种子编号的下载（辅种、转种等：API 给的下载地址带 downhash，
//     只在搜索结果里；用 Cookie 下载 download.php?id= 什么种子都行，2026-11-02 生产核对过）。站点强制两步验证，Cookie 拿到别的
//     网络上用时页面可能先要两步验证，这时时魔、签到与按编号下载会报「要两步验证」，搜索与从搜索结果、RSS 下载照常。
//   - H&R（wiki H&R 规则）：带 H&R 标识的种子，下载完成起 30 天内做种满 48 小时或分享率达到 1.5；发布超过 90 天的不再考核。
//   - 旧的 tracker 是 t.hddolby.com，新的是 www.uhddolby.com/announce.php；旧地址留在 LegacyURLs 里，下载器里还没换 tracker 的种子照样认得。
var HDDolbyDefinition = &v2.SiteDefinition{
	ID:          "hddolby",
	Name:        "UHDDolby",
	Aka:         []string{"高清杜比", "HD Dolby", "HDDolby"},
	Description: "高清杜比 - 需要配置 API Key（RSS 链接里的 RSS key），用于搜索、下载与用户数据；Cookie 可选，用于时魔、签到与辅种等只知道种子编号的下载",
	Schema:      v2.SchemaHDDolby,
	AuthMethod:  v2.AuthMethodCookieAndAPIKey,
	URLs:        []string{"https://www.uhddolby.com/"},
	LegacyURLs:  []string{"https://www.hddolby.com/", "https://hddolby.com/"},
	FaviconURL:  "https://www.uhddolby.com/favicon.ico",
	Unavailable: false,
	// 签到页是 NexusPHP 的 attendance.php（打开就签到）：成功时写「这是您的第 N 次签到」，再打开写「您今天已经签到过了」，默认规则都认得
	Attendance:      &v2.AttendanceConfig{},
	HRSeedTimeHours: 48,
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
