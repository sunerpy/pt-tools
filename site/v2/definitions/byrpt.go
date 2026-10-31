package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

var BYRPTDefinition = &v2.SiteDefinition{
	ID:          "byrpt",
	Name:        "BYRPT",
	Aka:         []string{"BYR", "北邮人"},
	Description: "BYRPT 教育网综合资源站",
	Schema:      v2.SchemaNexusPHP,
	URLs:        []string{"https://byr.pt/"},
	FaviconURL:  "https://byr.pt/favicon.ico",
	AuthMethod:  v2.AuthMethodCookie,
	// 2026-10 起登录是 auth_token（JWT，15 分钟）+ refresh_token + session_id，过期后站点换发一对新令牌，refresh_token 也换
	SessionCookies: []string{"auth_token", "refresh_token", "session_id"},
	TimezoneOffset: "+0800",
	RateLimit:      0.5,
	RateBurst:      2,
	// BYR 没有签到：attendance.php 回 404（2026-10-31 生产上手动签到的一次请求）。
	Attendance: &v2.AttendanceConfig{Unsupported: "BYRPT 没有签到（站点上没有签到页）"},
	UserInfo: &v2.UserInfoConfig{
		PickLast:     []string{"id"},
		RequestDelay: 500,
		Process: []v2.UserInfoProcess{
			{
				RequestConfig: v2.RequestConfig{URL: "/index.php", ResponseType: "document"},
				Fields:        []string{"id", "name", "seeding", "leeching", "bonus"},
			},
			{
				RequestConfig: v2.RequestConfig{URL: "/userdetails.php", ResponseType: "document"},
				Assertion:     map[string]string{"id": "params.id"},
				// 魔力值只从首页顶栏读：详情页那一格是「50058.4 魔力值历史」，按顶栏的规则读出 0，又读在首页之后，会把顶栏的值盖掉
				Fields: []string{"name", "uploaded", "downloaded", "ratio", "levelName", "joinTime"},
			},
			{
				// 时魔：魔力值页（顶栏「明细」的链接）写「你当前每小时能获取98.481个魔力值 (A = 2228.5)」
				RequestConfig: v2.RequestConfig{URL: "/mybonus.php", ResponseType: "document"},
				Fields:        []string{"bonusPerHour"},
			},
		},
		Selectors: map[string]v2.FieldSelector{
			"id": {
				Selector: []string{"td.navbar-user-data a[href*='userdetails.php']", "a[href*='userdetails.php'][class*='Name']", "a[href*='userdetails.php']"},
				Attr:     "href",
				Filters:  []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			"name": {
				Selector: []string{"td.navbar-user-data a[href*='userdetails.php']", "a[href*='userdetails.php'][class*='Name']"},
			},
			"uploaded": {
				Selector: []string{"td.rowhead:contains('传输') + td", ".navbar-user-data", "body"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:上传量|Uploaded)[^\d]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"downloaded": {
				Selector: []string{"td.rowhead:contains('传输') + td", ".navbar-user-data", "body"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:下载量|Downloaded)[^\d]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"ratio": {
				Selector: []string{"td.rowhead:contains('传输') + td", ".navbar-user-data", "body"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:分享率|Ratio|做种/下载时间比率)(?:<[^>]*>|[\s\p{Zs}：:])*([\d.,]+|∞|Inf)`}},
					{Name: "parseNumber"},
				},
			},
			"levelName": {
				Selector: []string{"td.rowhead:contains('等级') + td img", "td.rowhead:contains('Class') + td img"},
				Attr:     "title",
			},
			// 顶栏写法：「魔力值 [明细]: 50,058.4」
			"bonus": {
				Selector: []string{"td.navbar-user-data", ".navbar-user-data", "body"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?s)(?:魔力值|Bonus).*?:\s*([\d.,]+)`}},
					{Name: "parseNumber"},
				},
			},
			// 只取「每小时能获取」后面的数，后面括号里的 A 值不是时魔
			"bonusPerHour": {
				Selector: []string{"body"},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`每小时能获取\s*([\d.,]+)\s*个魔力值`}},
					{Name: "parseNumber"},
				},
			},
			"joinTime": {
				Selector: []string{"td.rowhead:contains('加入日期') + td", "td.rowhead:contains('Join') + td"},
				Filters: []v2.Filter{
					{Name: "split", Args: []any{" (", 0}},
					{Name: "parseTime"},
				},
			},
			"seeding": {
				Selector: []string{".navbar-user-data", "body"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`arrowup[^>]*></div>\s*(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			"leeching": {
				Selector: []string{".navbar-user-data", "body"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`arrowdown[^>]*></div>\s*(\d+)`}},
					{Name: "parseNumber"},
				},
			},
		},
	},
	Selectors: &v2.SiteSelectors{
		TableRows:          "table.torrents > tbody > tr:has(table.torrentname), table.torrents > tr:has(table.torrentname)",
		Title:              "table.torrentname a[href*='details.php']",
		TitleLink:          "table.torrentname a[href*='details.php']",
		Subtitle:           "table.torrentname td.embedded > span:last-of-type",
		Size:               "td.rowfollow:nth-child(6)",
		Seeders:            "td.rowfollow:nth-child(7)",
		Leechers:           "td.rowfollow:nth-child(8)",
		Snatched:           "td.rowfollow:nth-child(9)",
		DiscountIcon:       "img.pro_free, img.pro_free2up, img.pro_50pctdown, img.pro_30pctdown, img.pro_2up, img.pro_50pctdown2up",
		Category:           "td.rowfollow:nth-child(2) a.cat-link",
		UploadTime:         "td.rowfollow:nth-child(5)",
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
			"pro_free":      v2.DiscountFree,
			"pro_50pctdown": v2.DiscountPercent50,
		},
		DiscountSelector: "h1 font[class], h1 img[class]",
		EndTimeSelector:  "h1 span[title]",
		SizeSelector:     "td.rowhead:contains('基本信息')",
		SizeRegex:        `大小[：:]\s*([\d.]+)\s*(GB|MB|KB|TB)`,
	},
	LevelRequirements: byrptLevelRequirements,
}

// byrptLevelRequirements 按站点规则页的用户等级（2026-10-31 用户贴来的内容）：升级要同时满足注册天数、
// 上传量与升级分享率，分享率低于降级分享率时降级；升降级由系统在条件达标后一段时间内执行，不是实时的。
// 升级时至少要有 1 点下载量（分享率是无限时不升级）。页面上的 GiB 写成 GB、TB（都按 1024 进位）。
var byrptLevelRequirements = []v2.SiteLevelRequirement{
	{
		ID: 0, Name: "Peasant",
		Privilege: "降级用户：下载量超过 50GB 且分享率不足 0.4（超过 100GB 不足 0.5、200GB 不足 0.6、400GB 不足 0.7、800GB 不足 0.8）时降到这一级，14 天内不改善分享率会被系统封禁；下载种子时强制阅读规则",
	},
	{
		ID: 1, Name: "User",
		Privilege: "新用户的默认级别：上传字幕、发布趣味盒、查看用户列表、查看种子的 NFO 信息。升级时至少要有 1 点下载量，分享率是无限时不升级",
	},
	{
		ID: 2, Name: "Power User",
		Interval: "P14D", Uploaded: "32GB", Ratio: 1.05,
		Privilege: "分享率低于 0.95 降级；请求续种、查看排行榜、查看普通日志、删除自己上传的字幕、使用流量条、更新外部信息、新增求种",
	},
	{
		ID: 3, Name: "Elite User",
		Interval: "P56D", Uploaded: "512GB", Ratio: 1.55,
		Privilege: "分享率低于 1.45 降级；升级奖励 1 个邀请；查看用户种子历史记录（对方隐私等级不是「强」时）、直接发布种子",
	},
	{
		ID: 4, Name: "Crazy User",
		Interval: "P84D", Uploaded: "1TB", Ratio: 2.05,
		Privilege: "分享率低于 1.95 降级；升级奖励 1 个邀请；购买邀请、发送邀请，发布种子、上传字幕时可以选择匿名",
	},
	{
		ID: 5, Name: "Insane User",
		Interval: "P168D", Uploaded: "2TB", Ratio: 2.55,
		Privilege: "分享率低于 2.45 降级；升级奖励 1 个邀请；申请发布徽章、使用魔力值修改用户名、购买用户名特效",
	},
	{
		ID: 6, Name: "Veteran User",
		Interval: "P168D", Uploaded: "4TB", Ratio: 3.05,
		Privilege: "分享率低于 2.95 降级；升级奖励 1 个邀请；查看其他用户的评论和帖子历史记录（对方隐私等级不是「强」时）、查看种子结构",
	},
	{
		ID: 7, Name: "Extreme User",
		Interval: "P168D", Uploaded: "8TB", Ratio: 3.55,
		Privilege: "分享率低于 3.45 降级；升级奖励 2 个邀请；购买用户名特效（动态）",
	},
	{
		ID: 8, Name: "Ultimate User",
		Interval: "P336D", Uploaded: "32TB", Ratio: 4.05,
		Privilege: "分享率低于 3.95 降级；升级奖励 2 个邀请",
	},
	{
		ID: 9, Name: "Nexus Master",
		Interval: "P336D", Uploaded: "128TB", Ratio: 4.55,
		Privilege: "最高晋级等级；分享率低于 4.45 降级；升级奖励 2 个邀请；使用魔力值修改用户名（支持中文）、领取专属荣誉徽章",
	},
	{
		ID: v2.MinVipLevelID, Name: "VIP", NameAka: []string{"贵宾"},
		GroupType: v2.LevelGroupVIP,
		Privilege: "分永久与限时两种：管理员授予对站点有特殊贡献的用户，也可以用魔力值购买临时贵宾；免除分享率考核",
	},
	{
		ID: v2.MinManagerLevelID, Name: "Retiree", NameAka: []string{"养老族"},
		GroupType: v2.LevelGroupManager,
		Privilege: "管理员授予退休后的管理组成员和贡献很大的工作组成员，不接受申请；免除上传速度监测",
	},
	{
		ID: v2.MinManagerLevelID + 1, Name: "Uploader", NameAka: []string{"发布员"},
		GroupType: v2.LevelGroupManager,
		Privilege: "发布种子的主力，面向全体成员招募，每月按发布考核领工资；查看匿名用户的真实身份、查看被禁止的种子、访问论坛工作组专区",
	},
	{
		ID: v2.MinManagerLevelID + 2, Name: "Moderator", NameAka: []string{"总版主"},
		GroupType: v2.LevelGroupManager,
		Privilege: "维护所管分区的秩序（种子与候选、发布规则、举报），兼管论坛帖子、群聊区；管理种子（编辑、删除、设优惠、置顶）、种子评论、论坛帖子、群聊区、趣味盒、字幕区，查看机密日志与管理组信箱",
	},
	{
		ID: v2.MinManagerLevelID + 3, Name: "Administrator", NameAka: []string{"管理员"},
		GroupType: v2.LevelGroupManager,
		Privilege: "日常事务、用户奖惩、修订站点规则与常见问题，评审发布员与总版主的申请；管理用户账号、首页公告、论坛版块、站点史册、投票，发送全员消息，设定全局优惠",
	},
	{
		ID: v2.MinManagerLevelID + 4, Name: "SysOp", NameAka: []string{"维护开发员"},
		GroupType: v2.LevelGroupManager,
		Privilege: "保障网站运行、修正 BUG、开发新功能，处理论坛 BT 建议区与 BUG 反馈区的技术问题；查看网站运行状态，管理站点设定与网站代码",
	},
	{
		ID: v2.MinManagerLevelID + 5, Name: "Staff Leader", NameAka: []string{"主管"},
		GroupType: v2.LevelGroupManager,
		Privilege: "站点负责人，决定重大事项；任免管理组成员，发放特殊用户组与管理组的工资（魔力值），管理站点任务系统",
	},
}

func init() {
	v2.RegisterSiteDefinition(BYRPTDefinition)
}
