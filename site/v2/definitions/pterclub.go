package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// PTerClubDefinition is the site definition for PTerClub (pterclub.net, 猫站).
// Standard NexusPHP site; user stats live in the #info_block div. Notable quirks:
//   - bonus label is "猫粮" (not 魔力值), inside a <span class="color_bonus"> wrapped in <a>
//   - 上传量/下载量/分享率 use full-width colons (：)
//   - detail-page free is encoded in the H1 <font class='free'> onmouseover tooltip
//   - 列表的副标题在标题格第二个 div 的最后一个 span 里（前面是官方、国语这些标签），不是 td.embedded 的直接子元素
//   - 没有 getusertorrentlistajax.php（404），做种体积读个人详情页的「做种大小」
//   - 游戏种子的链接是 detailsgame.php，列表里不收（详情与下载都走 details.php）
//
// 2026-11-01 在站点与官方 wiki 上核对：wiki「下载规则」写明本站暂不实行 H&R；「账号保留规则」写明未封存账号连续 60 天
// 不登录会被禁用，只认自己手动登录，自动签到、工具刷新都不算登录。
var PTerClubDefinition = &v2.SiteDefinition{
	ID:          "pterclub",
	Name:        "PTerClub",
	Aka:         []string{"猫站", "PTer", "PT之友"},
	Description: "综合性 PT 站点",
	Schema:      v2.SchemaNexusPHP,
	// 首页「签到得猫粮」由 JS 发 GET attendance-ajax.php，回 JSON（status、message、data）
	Attendance:     &v2.AttendanceConfig{Path: "/attendance-ajax.php"},
	URLs:           []string{"https://pterclub.net/"},
	FaviconURL:     "https://pterclub.net/favicon.ico",
	AuthMethod:     v2.AuthMethodCookie,
	TimezoneOffset: "+0800",
	RateLimit:      0.5,
	RateBurst:      2,
	UserInfo: &v2.UserInfoConfig{
		PickLast:     []string{"id"},
		RequestDelay: 500,
		Process: []v2.UserInfoProcess{
			{
				RequestConfig: v2.RequestConfig{URL: "/index.php", ResponseType: "document"},
				Fields:        []string{"id", "name", "seeding", "leeching", "bonus", "seedingBonus", "uploaded", "downloaded", "ratio"},
			},
			{
				RequestConfig: v2.RequestConfig{URL: "/userdetails.php", ResponseType: "document"},
				Assertion:     map[string]string{"id": "params.id"},
				Fields:        []string{"levelName", "joinTime", "lastAccessAt", "seedingSize"},
			},
			{
				// 时魔：「你当前每小时能获取97.452克猫粮 ( A = 2210.389， B = 77.452 )」
				RequestConfig: v2.RequestConfig{URL: "/mybonus.php", ResponseType: "document"},
				Fields:        []string{"bonusPerHour"},
			},
		},
		Selectors: map[string]v2.FieldSelector{
			"id": {
				Selector: []string{
					"#info_block a[href*='userdetails.php'][class*='Name']",
					"#info_block a[href*='userdetails.php']",
					"a[href*='userdetails.php']",
				},
				Attr:    "href",
				Filters: []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			"name": {
				Selector: []string{
					"#info_block a[href*='userdetails.php'][class*='Name'] b",
					"#info_block a[href*='userdetails.php'][class*='Name']",
					"#info_block a[href*='userdetails.php']",
				},
			},
			"uploaded": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`上传量[:：]\s*</font>\s*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"downloaded": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`下载量[:：]\s*</font>\s*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"ratio": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`分享率[:：]\s*</font>\s*(?:<font[^>]*>)?([\d.,]+|∞|Inf)`}},
					{Name: "parseNumber"},
				},
			},
			// 猫粮 (= 魔力值): <span class="color_bonus">猫粮 </span> </a> [使用 | 站免池]: 1,452,469.7
			"bonus": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`猫粮\s*</span>\s*</a>\s*\[[^\]]*\]\s*[:：]\s*([\d.,]+)`}},
					{Name: "parseNumber"},
				},
			},
			// 做种积分：</span> 1,333,648.2 (full-width colon, value after </span>)
			"seedingBonus": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`做种积分\s*[:：]\s*</span>\s*([\d.,]+)`}},
					{Name: "parseNumber"},
				},
			},
			"seeding": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`class="arrowup"[^>]*/>\s*(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			"leeching": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`class="arrowdown"[^>]*/>\s*(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			"levelName": {
				Selector: []string{
					"td.rowhead:contains('等级') + td img",
					"td.rowhead:contains('等級') + td img",
					"td.rowhead:contains('等级') + td",
					"td.rowhead:contains('等級') + td",
				},
				Attr: "title",
			},
			"joinTime": {
				Selector: []string{
					"td.rowhead:contains('加入日期') + td",
					"td.rowhead:contains('加入時間') + td",
					"td.rowhead:contains('Join') + td",
				},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
			},
			"lastAccessAt": {
				Selector: []string{
					"td.rowhead:contains('最近动向') + td",
					"td.rowhead:contains('最近動向') + td",
					"td.rowhead:contains('最近活动') + td",
					"td.rowhead:contains('上次访问') + td",
					"td.rowhead:contains('上次訪問') + td",
					"td.rowhead:contains('Last access') + td",
				},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
			},
			// 个人详情页的「做种大小」一行（4.208 TB）；写了这个字段就不再去取做种列表（站点上没有 getusertorrentlistajax.php）
			"seedingSize": {
				Selector: []string{
					"td.rowhead:contains('做种大小') + td",
					"td.rowhead:contains('做種大小') + td",
					"td.rowhead:contains('Seeding Size') + td",
				},
				Filters: []v2.Filter{{Name: "parseSize"}},
			},
			"bonusPerHour": {
				Selector: []string{"#outer", "body"},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:你當前每小時能獲取|你当前每小时能获取|您當前每小時能獲取|您当前每小时能获取)[^\d-]{0,20}([\d.,]+)`}},
					{Name: "parseNumber"},
				},
			},
		},
	},
	Selectors: &v2.SiteSelectors{
		TableRows: "table.torrents > tbody > tr:has(table.torrentname), table.torrents > tr:has(table.torrentname)",
		Title:     "table.torrentname a[href*='details.php']",
		TitleLink: "table.torrentname a[href*='details.php']",
		// 现在的列表：标题格第二个 div 是「官方」「国语」这些标签加副标题 span；以前的列表是 span.torrents-subtitle
		Subtitle:           "table.torrentname td.embedded > div > div:last-child > span, table.torrentname span.torrents-subtitle",
		Size:               "td.rowfollow:nth-child(5)",
		Seeders:            "td.rowfollow:nth-child(6)",
		Leechers:           "td.rowfollow:nth-child(7)",
		Snatched:           "td.rowfollow:nth-child(8)",
		DiscountIcon:       "img.pro_free, img.pro_free2up, img.pro_2up, img.pro_50pctdown, img.pro_50pctdown2up, img.pro_30pctdown",
		Category:           "td.rowfollow:nth-child(1) img[alt]",
		UploadTime:         "td.rowfollow:nth-child(4) span[title]",
		DetailDownloadLink: "td.rowhead:contains('下载') + td a[href*='download.php']",
		DetailSubtitle:     "td.rowhead:contains('副标题') + td",
	},
	DetailParser: &v2.DetailParserConfig{
		TimeLayout: "2006-01-02 15:04:05",
		DiscountMapping: map[string]v2.DiscountLevel{
			"free":          v2.DiscountFree,
			"twoup":         v2.Discount2xUp,
			"twoupfree":     v2.Discount2xFree,
			"thirtypercent": v2.DiscountPercent30,
			"halfdown":      v2.DiscountPercent50,
			"twouphalfdown": v2.Discount2x50,
		},
		HRKeywords:       []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run"},
		TitleSelector:    "input[name='torrent_name']",
		IDSelector:       "input[name='detail_torrent_id']",
		DiscountSelector: "h1 font.free, h1 font[class]",
		EndTimeSelector:  "h1 span[title]",
		SizeSelector:     "td.rowhead:contains('基本信息')",
		SizeRegex:        `大小[：:]\s*([\d.]+)\s*(GB|MB|KB|TB)`,
	},
	LevelRequirements: pterclubLevelRequirements,
}

// pterclubLevelRequirements 按官方 wiki「用户分享率及等级说明」（wiki.pterclub.net，最后编辑 2026-01-29，2026-11-01 读）：
// 升级要逐级满足注册时间、下载量、分享率、做种积分与发布种子数，分享率低于降级线时降级。发种只算审核通过（初审或终审）
// 并且没断种的种子，电子书、学习类不算；系统 6–12 小时更新一次等级。站点显示的等级名是「猫名 + 英文等级」（伯曼猫 USER），
// ID 沿用 wiki 的 Class 编号。页面上的 GB 按原样写。
var pterclubLevelRequirements = []v2.SiteLevelRequirement{
	{
		ID: 0, Name: "波斯猫 Peasant", NameAka: []string{"波斯猫", "Peasant"},
		Privilege: "降级用户：满足其一即可回到伯曼猫——下载量不超过 10GB，或下载量超过 10GB 且分享率不低于 0.5（超过 50GB 不低于 0.6、超过 100GB 不低于 0.7、超过 200GB 不低于 0.8、超过 400GB 不低于 0.9）；30 天内不改善分享率会被禁用，流控更严，不能上传字幕",
	},
	{
		ID: 1, Name: "伯曼猫 User", NameAka: []string{"伯曼猫", "User"},
		Privilege: "新用户的默认级别，不满足波斯猫的升级条件时降为波斯猫。未封存账号连续 60 天不登录会被禁用，只认自己手动登录，自动签到和工具访问不算",
	},
	{
		ID: 2, Name: "加菲猫 Power User", NameAka: []string{"加菲猫", "Power User"},
		Interval: "P5W", Downloaded: "50GB", Ratio: 1.05, SeedingBonus: 10000, Uploads: 10,
		Privilege: "分享率低于 0.95 降级；可以直接发布种子、查看邀请区、上传字幕和删除自己上传的字幕",
	},
	{
		ID: 3, Name: "布偶猫 Elite User", NameAka: []string{"布偶猫", "Elite User"},
		Interval: "P10W", Downloaded: "120GB", Ratio: 1.55, SeedingBonus: 30000, Uploads: 20,
		Privilege: "分享率低于 1.45 降级；可以请求续种",
	},
	{
		ID: 4, Name: "雪鞋猫 Crazy User", NameAka: []string{"雪鞋猫", "Crazy User"},
		Interval: "P24W", Downloaded: "300GB", Ratio: 2.05, SeedingBonus: 80000, Uploads: 30,
		Privilege: "分享率低于 1.95 降级；可以查看排行榜",
	},
	{
		ID: 5, Name: "暹罗猫 Insane User", NameAka: []string{"暹罗猫", "Insane User"},
		Interval: "P32W", Downloaded: "500GB", Ratio: 2.55, SeedingBonus: 150000, Uploads: 40,
		Privilege: "分享率低于 2.45 降级；可以查看普通日志",
	},
	{
		ID: 6, Name: "安哥拉猫 Veteran User", NameAka: []string{"安哥拉猫", "Veteran User"},
		Interval: "P40W", Downloaded: "750GB", Ratio: 3.05,
		Alternative: []v2.AlternativeRequirement{
			{SeedingBonus: 300000, Uploads: 50},
			{SeedingBonus: 900000, Uploads: 10},
		},
		Privilege: "分享率低于 2.95 降级；封存账号后不会因不活跃被禁用；可以查看其它用户的评论、帖子历史",
	},
	{
		ID: 7, Name: "孟加拉猫 Extreme User", NameAka: []string{"孟加拉猫", "Extreme User"},
		Interval: "P48W", Downloaded: "1024GB", Ratio: 3.55,
		Alternative: []v2.AlternativeRequirement{
			{SeedingBonus: 600000, Uploads: 60},
			{SeedingBonus: 1800000, Uploads: 10},
		},
		Privilege: "分享率低于 3.45 降级；初次升级送 1 枚永久邀请码；可 2 次无条件不活跃解封；可以查看邀请树统计",
	},
	{
		ID: 8, Name: "山东狮子猫 Ultimate User", NameAka: []string{"山东狮子猫", "Ultimate User"},
		Interval: "P56W", Downloaded: "1536GB", Ratio: 4.05,
		Alternative: []v2.AlternativeRequirement{
			{SeedingBonus: 1200000, Uploads: 70},
			{SeedingBonus: 3600000, Uploads: 10},
		},
		Privilege: "分享率低于 3.95 降级；初次升级送 2 枚永久邀请码；追加 1 次无条件不活跃解封；可以查看邀请树图",
	},
	{
		ID: 9, Name: "四川简州猫 Nexus Master", NameAka: []string{"四川简州猫", "Nexus Master"},
		Interval: "P112W", Downloaded: "3072GB", Ratio: 4.55,
		Alternative: []v2.AlternativeRequirement{
			{SeedingBonus: 2400000, Uploads: 100},
			{SeedingBonus: 9600000, Uploads: 10},
		},
		Privilege: "分享率低于 4.45 降级；初次升级送 3 枚永久邀请码；不会因不活跃被禁用",
	},
	{
		ID: v2.MinVipLevelID, Name: "挪威森林猫 VIP", NameAka: []string{"挪威森林猫", "VIP"},
		GroupType: v2.LevelGroupVIP,
		Privilege: "大转盘获得、猫粮购买（临时），或管理组手动提升，只会因到期变更等级；拥有黄星期间免下载量；默认从这一级起可以发送邀请，可以查看 VIP 交流区",
	},
	{
		ID: v2.MinManagerLevelID, Name: "美国短毛猫 Retiree", NameAka: []string{"美国短毛猫", "Retiree"},
		GroupType: v2.LevelGroupManager,
		Privilege: "退休了的工作组成员，管理组讨论后手动提升；拥有黄星期间免下载量",
	},
	{
		ID: v2.MinManagerLevelID + 1, Name: "英国短毛猫 Uploader", NameAka: []string{"英国短毛猫", "Uploader"},
		GroupType: v2.LevelGroupManager,
		Privilege: "工作组成员，管理组讨论后手动提升，只会因到期变更等级；拥有黄星期间免下载量；按工作组别有不同的权限",
	},
	{
		ID: v2.MinManagerLevelID + 2, Name: "俄罗斯蓝猫 Moderator", NameAka: []string{"俄罗斯蓝猫", "Moderator"},
		GroupType: v2.LevelGroupManager,
		Privilege: "查看管理组信箱、举报信箱；编辑或删除任何种子，管理候选、论坛帖子与用户评论，把种子设为置顶或促销",
	},
	{
		ID: v2.MinManagerLevelID + 3, Name: "中华田园猫 Administrator", NameAka: []string{"中华田园猫", "Administrator"},
		GroupType: v2.LevelGroupManager,
		Privilege: "除了不能改变站点设定，可以做任何事",
	},
	{
		ID: v2.MinManagerLevelID + 4, Name: "苏格兰折耳猫 SysOp", NameAka: []string{"苏格兰折耳猫", "SysOp"},
		GroupType: v2.LevelGroupManager,
		Privilege: "网站开发与维护人员；可以改变站点设定，不能管理捐赠",
	},
	{
		ID: v2.MinManagerLevelID + 5, Name: "阿比西尼亚猫 Staff Leader", NameAka: []string{"阿比西尼亚猫", "Staff Leader"},
		GroupType: v2.LevelGroupManager,
		Privilege: "网站主管，可以做任何事",
	},
}

func init() {
	v2.RegisterSiteDefinition(PTerClubDefinition)
}
