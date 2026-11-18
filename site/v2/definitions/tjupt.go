package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// TJUPTDefinition is the site definition for TJUPT (tjupt.org, 北洋园PT).
// 改得很多的 NexusPHP（2026-11-01 在站点上核对）：
//   - 不记下载量、没有分享率（站点规则：无视分享率模式），只靠 H&R 积分考核做种；个人信息里只有上传量
//   - 列表的时间格是「2026-10-31<br>20:35:49」，没有 title；副标题是标题后 <br> 之后的裸文本，要从 HTML 里取
//   - 标题格的第二行是标签（救活、保种、中字……），促销图标也在这一行；「救活」标签旁有救活者的 userdetails.php 链接，
//     所以标题链接只认 href 以 details.php 开头的
//   - 促销只有 2X 上传一种（列表的促销筛选只有「普通」「2倍上传」），没有免费；详情页的促销图标在「基本信息」一格，
//     「其它版本」表里还有别的种子的图标
//   - 时魔在 bonus.php?show=description（mybonus.php 是兑换页，没有这一句）
//
// 站点规则（rules.php 总则）：没在白名单里的三方程序、脚本访问站点会被永久封禁。白名单与报备在论坛
// 「机器人爬虫/程序脚本报备专帖」（forums.php?action=viewtopic&topicid=16580），2026-11-01 时还没有 pt-tools；
// 站点自带的 RSS 不用报备。签到页写明禁止用任何自动化脚本、程序签到，答错还扣魔力值，所以不做签到。
var TJUPTDefinition = &v2.SiteDefinition{
	ID:          "tjupt",
	Name:        "TJUPT",
	Aka:         []string{"北洋园PT", "北洋园"},
	Description: "天津大学校园 PT（教育网）；用 Cookie 访问站点的程序要先在论坛报备，否则有封号风险",
	Schema:      v2.SchemaNexusPHP,
	URLs:        []string{"https://tjupt.org/"},
	FaviconURL:  "https://tjupt.org/favicon.ico",
	AuthMethod:  v2.AuthMethodCookie,
	Attendance: &v2.AttendanceConfig{
		Unsupported: "北洋园签到页写明禁止用脚本或程序签到（答错还会扣魔力值），请在网页上手动签到；180 天内签到过 1 天才算活跃",
	},
	TimezoneOffset: "+0800",
	RateLimit:      0.5,
	RateBurst:      2,
	// 全站 H&R：下载超过种子体积的 10% 就要考核，要在 10 倍于做种要求的考核期内累计做种（rules.php「H&R 规则」）。
	// 不超过 10GiB 的站点算 1 天，这里正好 10GiB 的按 2 天算，宁多不少；完整下载后单种分享率越高要求越短，这里不算这一项。
	HREnabled: true,
	HRSeedTimeRules: []v2.HRSeedTimeRule{
		{MinSizeGB: 0, MaxSizeGB: 10, SeedTimeH: 24},
		{MinSizeGB: 10, MaxSizeGB: 20, SeedTimeH: 48},
		{MinSizeGB: 20, MaxSizeGB: 30, SeedTimeH: 72},
		{MinSizeGB: 30, MaxSizeGB: 40, SeedTimeH: 96},
		{MinSizeGB: 40, MaxSizeGB: 50, SeedTimeH: 120},
		{MinSizeGB: 50, SeedTimeH: 168},
	},
	UserInfo: &v2.UserInfoConfig{
		PickLast:     []string{"id"},
		RequestDelay: 500,
		Process: []v2.UserInfoProcess{
			{
				RequestConfig: v2.RequestConfig{URL: "/index.php", ResponseType: "document"},
				Fields:        []string{"id", "name", "levelName", "bonus", "seedingBonus", "uploaded", "seeding", "leeching"},
			},
			{
				RequestConfig: v2.RequestConfig{URL: "/userdetails.php", ResponseType: "document"},
				Assertion:     map[string]string{"id": "params.id"},
				Fields:        []string{"joinTime", "lastAccessAt"},
			},
			{
				// 「你当前每小时能获取112.58个魔力值 (A = 6533.9)」
				// NexusPHP 驱动不读 RequestConfig.Params，查询串写在 URL 里
				RequestConfig: v2.RequestConfig{URL: "/bonus.php?show=description", ResponseType: "document"},
				Fields:        []string{"bonusPerHour"},
			},
		},
		Selectors: map[string]v2.FieldSelector{
			"id": {
				Selector: []string{
					"#info_block a[href*='userdetails.php'][class*='Name']",
					"#info_block a[href*='userdetails.php']",
				},
				Attr:    "href",
				Filters: []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			"name": {
				Selector: []string{
					"#info_block a[href*='userdetails.php'][class*='Name'] b",
					"#info_block a[href*='userdetails.php'][class*='Name']",
				},
			},
			// 顶栏「等级: [小有名气]」，链接到 classes.php
			"levelName": {
				Selector: []string{"#info_block a[href*='classes.php']"},
			},
			// 「魔力值 [使用|茉莉园]: 36,218.4」，数值是指向魔力值日志的链接
			"bonus": {
				Selector: []string{"#info_block a[href*='bonus.php?show=log']"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			// 「做种积分: 1,864,215.6」
			"seedingBonus": {
				Selector: []string{"#info_block a[href*='bonus.php?show=description']"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			"uploaded": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`上传量[:：]?(?:<[^>]*>|[\s\p{Zs}])*([\d.,]+[\s\p{Zs}]*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"seeding": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`class="arrowup"[^>]*>\s*(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			"leeching": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`class="arrowdown"[^>]*>\s*(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			"joinTime": {
				Selector: []string{"td.rowhead:contains('加入日期') + td"},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
			},
			"lastAccessAt": {
				Selector: []string{"td.rowhead:contains('最近动向') + td"},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
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
		Title:     "table.torrentname a[href^='details.php']",
		TitleLink: "table.torrentname a[href^='details.php']",
		SubtitleSelector: &v2.FieldSelector{
			Selector: []string{"table.torrentname td.embedded:has(a[href^='details.php'])"},
			Attr:     "html",
			Filters: []v2.Filter{
				{Name: "regex", Args: []any{`(?s)<br\s*/?>\s*([^<]+)`}},
				// 走 html 取值会保留实体编码，需手动还原；&amp; 必须最后替换以避免二次解码
				{Name: "replace", Args: []any{"&#39;", "'"}},
				{Name: "replace", Args: []any{"&#34;", `"`}},
				{Name: "replace", Args: []any{"&quot;", `"`}},
				{Name: "replace", Args: []any{"&lt;", "<"}},
				{Name: "replace", Args: []any{"&gt;", ">"}},
				{Name: "replace", Args: []any{"&amp;", "&"}},
				{Name: "trim"},
			},
		},
		Size:               "td.rowfollow:nth-child(5)",
		Seeders:            "td.rowfollow:nth-child(6)",
		Leechers:           "td.rowfollow:nth-child(7)",
		Snatched:           "td.rowfollow:nth-child(8)",
		DiscountIcon:       "img.pro_free, img.pro_free2up, img.pro_2up, img.pro_50pctdown, img.pro_50pctdown2up, img.pro_30pctdown",
		Category:           "td.rowfollow:nth-child(1) img[alt]",
		UploadTime:         "td.rowfollow:nth-child(4)",
		DetailDownloadLink: "td.rowhead:contains('种子名称') + td a[href*='download.php']",
		DetailSubtitle:     "td.rowhead:contains('副标题') + td",
	},
	DetailParser: &v2.DetailParserConfig{
		TimeLayout: "2006-01-02 15:04:05",
		DiscountMapping: map[string]v2.DiscountLevel{
			"pro_free":         v2.DiscountFree,
			"pro_2up":          v2.Discount2xUp,
			"pro_free2up":      v2.Discount2xFree,
			"pro_50pctdown":    v2.DiscountPercent50,
			"pro_30pctdown":    v2.DiscountPercent30,
			"pro_50pctdown2up": v2.Discount2x50,
		},
		// 详情页有一行「Hit&Run」写这个种子的做种要求（全站都有 H&R）
		HRKeywords:       []string{"Hit&amp;Run"},
		TitleSelector:    "input[name='torrent_name']",
		IDSelector:       "input[name='detail_torrent_id']",
		DiscountSelector: "td.rowhead:contains('基本信息') + td img[class^='pro_']",
		SizeSelector:     "td.rowhead:contains('基本信息')",
		// 「大小:&nbsp;11.11 GiB」：冒号后是 &nbsp;（RE2 的 \s 不认），单位是 GiB
		SizeRegex: `大小[：:][\s\p{Zs}]*([\d.]+)[\s\p{Zs}]*([KMGT]i?B)`,
	},
	LevelRequirements: tjuptLevelRequirements,
}

// tjuptLevelRequirements 按站点等级页（classes.php，2026-11-01 读）：升级要逐级满足注册周数、上传量、发布种子数
// （现存且完成数 ≥ 1）以及完成种子数（体积大于 200MiB）、累计做种时间、H&R 积分 100，再到等级页手动点升级，
// 花掉所列的魔力值（Bonus 记的是这笔花费）。完成种子数、做种时间与 H&R 积分没有对应字段，写在说明里。
// 站点显示中文等级名，英文名作别名。页面上的 GiB 写成 GB（都按 1024 进位）。
var tjuptLevelRequirements = []v2.SiteLevelRequirement{
	{
		ID: 1, Name: "无名小辈", NameAka: []string{"User"},
		Privilege: "初始等级：发布、下载种子，在求种区求种，查看种子 NFO。威震一方以下未封存的账号 180 天内要签到过 1 天、发过 1 个种子（或趣味盒、论坛新主题），或实际上传增量超过 5TiB 且做种积分增量超过 30 万，否则禁用；只访问网页不算",
	},
	{
		ID: 2, Name: "拜师学艺", NameAka: []string{"Power User"},
		Interval: "P4W", Uploaded: "50GB", Bonus: 10000,
		Privilege: "另需完成 20 个种子、累计做种 30 天、H&R 积分 100；查看用户列表，请求续种，查看其他用户的种子历史（对方隐私等级不为高时），删除自己上传的字幕",
	},
	{
		ID: 3, Name: "持剑下山", NameAka: []string{"Elite User"},
		Interval: "P8W", Uploaded: "200GB", Bonus: 30000,
		Privilege: "另需完成 60 个种子、累计做种 120 天、H&R 积分 100；封存账号后不会被删除",
	},
	{
		ID: 4, Name: "初入江湖", NameAka: []string{"Crazy User"},
		Interval: "P16W", Uploaded: "800GB", Uploads: 1, Bonus: 80000,
		Privilege: "另需完成 150 个种子、累计做种 450 天、H&R 积分 100；首次升级送 1 个永久邀请；可以发送邀请，做种、下载、发布时可以选择匿名",
	},
	{
		ID: 5, Name: "小有名气", NameAka: []string{"Insane User"},
		Interval: "P28W", Uploaded: "2000GB", Uploads: 5, Bonus: 150000,
		Privilege: "另需完成 300 个种子、累计做种 1500 天、H&R 积分 100；首次升级送 1 个永久邀请；可以查看普通日志",
	},
	{
		ID: 6, Name: "威震一方", NameAka: []string{"Veteran User"},
		Interval: "P48W", Uploaded: "5000GB", Uploads: 10, Bonus: 300000,
		Privilege: "另需完成 600 个种子、累计做种 4200 天、H&R 积分 100；首次升级送 1 个永久邀请；可以查看其它用户的评论、帖子历史；账号永久保留，不受不活跃规则限制",
	},
	{
		ID: 7, Name: "横扫群雄", NameAka: []string{"Extreme User"},
		Interval: "P72W", Uploaded: "10000GB", Uploads: 15, Bonus: 400000,
		Privilege: "另需完成 1000 个种子、累计做种 28000 天、H&R 积分 100；首次升级送 1 个永久邀请",
	},
	{
		ID: 8, Name: "开宗立派", NameAka: []string{"Ultimate User"},
		Interval: "P100W", Uploaded: "20000GB", Uploads: 30, Bonus: 600000,
		Privilege: "另需完成 1800 个种子、累计做种 90000 天、H&R 积分 100；首次升级送 2 个永久邀请",
	},
	{
		ID: 9, Name: "天下无敌", NameAka: []string{"Nexus Master"},
		Interval: "P132W", Uploaded: "50000GB", Uploads: 50, Bonus: 1000000,
		Privilege: "另需完成 3000 个种子、累计做种 300000 天、H&R 积分 100；首次升级送 3 个永久邀请",
	},
	{
		ID: v2.MinVipLevelID, Name: "西域来客", NameAka: []string{"VIP"},
		GroupType: v2.LevelGroupVIP,
		Privilege: "对本站有特殊贡献的用户（捐赠、美工等）、友站合作账号；免除自动降级，不记录下载量，免除 H&R 考核",
	},
	{
		ID: v2.MinManagerLevelID, Name: "扫地僧", NameAka: []string{"Retiree"},
		GroupType: v2.LevelGroupManager,
		Privilege: "退休的管理组成员",
	},
	{
		ID: v2.MinManagerLevelID + 1, Name: "龙门镖局", NameAka: []string{"Uploader"},
		GroupType: v2.LevelGroupManager,
		Privilege: "发布员、TJUPT 官方小组成员与其他驻站小组发布者，管理组审核后提升，每月有考核任务；查看匿名用户的真实身份，按发种情况每月领魔力值与邀请码",
	},
	{
		ID: v2.MinManagerLevelID + 2, Name: "六扇门", NameAka: []string{"Moderator"},
		GroupType: v2.LevelGroupManager,
		Privilege: "负责各自大类的种子审核、发布与规则；查看管理组信箱、举报信箱，编辑、删除种子，设置置顶和种子优惠，管理候选和字幕区，编辑、删除论坛帖子与种子评论",
	},
	{
		ID: v2.MinManagerLevelID + 3, Name: "中枢六部", NameAka: []string{"Administrator"},
		GroupType: v2.LevelGroupManager,
		Privilege: "站点综合管理：管理用户账号，发送全员消息，设置全局优惠与 IP 封禁，修改站点信息，发放魔力值、批量邀请用户",
	},
	{
		ID: v2.MinManagerLevelID + 4, Name: "锦衣卫", NameAka: []string{"SysOp"},
		GroupType: v2.LevelGroupManager,
		Privilege: "负责站点功能开发与维护；修改站点设定，管理站点代码、服务器",
	},
	{
		ID: v2.MinManagerLevelID + 5, Name: "九五之尊", NameAka: []string{"Staff Leader"},
		GroupType: v2.LevelGroupManager,
		Privilege: "总管理员，统筹站点事务，拥有站点全部操作权限",
	},
}

func init() {
	v2.RegisterSiteDefinition(TJUPTDefinition)
}
