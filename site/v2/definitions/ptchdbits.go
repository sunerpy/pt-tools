package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// PTCHDBitsDefinition is the site definition for PTCHDBits (ptchdbits.co, CHDBits mirror).
// Standard NexusPHP site; user stats (上传量/下载量/分享率/魔力值/做种/下载) live in the
// #info_block div present on both index.php and userdetails.php.
//
// 2026-11-02 按真实页面核对（简体界面）：
//   - 列表的促销跟着用户的「促销种子显示方式」：文字 <b>[<font class='free' …>免费</font>]</b>（本站默认），
//     或图标 img.pro_*。限时促销在后面跟着「(限时<span title="2026-11-03 11:27:47">23时16分</span>)」，
//     font 标签里也塞了同样一段（写坏的属性），浏览器与 goquery 都把它读成 font 的属性，不影响取值。
//   - 行背景 class（thirtypercentdown_bg、halfdown_bg……）标的是置顶级别，不是促销，不能拿来判断优惠。
//   - H&R 只看种子：标题后的圆圈写着 h3、h5，详情页「基本信息」里有「H&R: 3day / 5day」（见 HRSeedTimeHours）。
var PTCHDBitsDefinition = &v2.SiteDefinition{
	ID:             "ptchdbits",
	Name:           "PTCHDBits",
	Aka:            []string{"CHD", "彩虹岛"},
	Description:    "CHDBits 综合性 PT 站点",
	Schema:         v2.SchemaNexusPHP,
	Attendance:     &v2.AttendanceConfig{Challenge: v2.ChallengeBakaQuiz},
	URLs:           []string{"https://ptchdbits.co/"},
	FaviconURL:     "https://ptchdbits.co/favicon.ico",
	AuthMethod:     v2.AuthMethodCookie,
	TimezoneOffset: "+0800",
	RateLimit:      0.5,
	RateBurst:      2,
	// H&R（rules.php、hnr.php）：三级置顶里标了 H3、H5 的种子才算，下载超过一半以后，H3 要在 20 天内做种满 72 小时，
	// H5 满 120 小时；没做满扣 1 点 HP，HP 为 0 不能下载，20 天不恢复封号。黄星与 VIP 不计。
	// pt-tools 只记一个种子有没有 H&R、分不出 H3 和 H5，一律按较长的 120 小时算。整站不是 H&R，不开 HREnabled。
	HRSeedTimeHours: 120,
	UserInfo: &v2.UserInfoConfig{
		PickLast:     []string{"id"},
		RequestDelay: 500,
		Process: []v2.UserInfoProcess{
			{
				RequestConfig: v2.RequestConfig{URL: "/index.php", ResponseType: "document"},
				Fields: []string{
					"id", "name", "seeding", "leeching", "bonus", "seedingBonus", "uploaded", "downloaded", "ratio",
					"hnrUnsatisfied",
				},
			},
			{
				RequestConfig: v2.RequestConfig{URL: "/mybonus.php", ResponseType: "document"},
				Fields:        []string{"bonusPerHour"},
			},
			{
				RequestConfig: v2.RequestConfig{URL: "/userdetails.php", ResponseType: "document"},
				Assertion:     map[string]string{"id": "params.id"},
				Fields:        []string{"levelName", "joinTime", "lastAccessAt"},
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
			"bonus": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`魔力值\s*</font>\s*\[[^\]]*\]\s*[:：]\s*([\d.,]+)`}},
					{Name: "parseNumber"},
				},
			},
			// 做种积分: </font>1664472.0 (value after </font>, half-width colon)
			"seedingBonus": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`做种积分\s*[:：]\s*</font>\s*([\d.,]+)`}},
					{Name: "parseNumber"},
				},
			},
			// 时魔：魔力值页有好几句「你当前每小时能获取…个魔力值」，依次是做种（B，后面带 A 值）、做种体积（C）、
			// 官种保种（D），只有「合计获得魔力值F」下面那句是合计
			"bonusPerHour": {
				Selector: []string{"h1:contains('合计获得魔力值') + div"},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`([\d.,]+)`}},
					{Name: "parseNumber"},
				},
			},
			// 页头「H&R:」后面的数，链接到 hnr.php 的 H&R 记录（没有时那一页写「当前没有H&R」）
			"hnrUnsatisfied": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`hnr\.php[^>]*>\s*(?:<[^>]+>\s*)*H&amp;R[:：]\s*(?:</[^>]+>\s*)*(\d+)`}},
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
		},
	},
	Selectors: &v2.SiteSelectors{
		TableRows: "table.torrents > tbody > tr:has(table.torrentname), table.torrents > tr:has(table.torrentname)",
		Title:     "table.torrentname a[href*='details.php']",
		TitleLink: "table.torrentname a[href*='details.php']",
		// 副标题在 font.subtitle 里：前面一组标签（官方、中字……），H&R 种子后面还有写着 h3、h5 的圆圈，只取中间的文字。
		// 不能用 span：限时促销的倒计时也是 td.embedded 下面的 span
		SubtitleSelector: &v2.FieldSelector{
			Selector: []string{"table.torrentname td.embedded > font.subtitle"},
			Attr:     "html",
			Filters: []v2.Filter{
				// 先去掉只含文字的 div（标签与圆圈里的字），再去掉剩下的标签
				{Name: "regexReplace", Args: []any{`<div[^>]*>[^<]*</div>`, ""}},
				{Name: "regexReplace", Args: []any{`<[^>]+>`, ""}},
				// 走 html 取值会保留实体编码，需手动还原；&amp; 必须最后替换以避免二次解码
				{Name: "replace", Args: []any{"&#39;", "'"}},
				{Name: "replace", Args: []any{"&#34;", `"`}},
				{Name: "replace", Args: []any{"&lt;", "<"}},
				{Name: "replace", Args: []any{"&gt;", ">"}},
				{Name: "replace", Args: []any{"&amp;", "&"}},
				{Name: "replace", Args: []any{"\u00a0", " "}},
				{Name: "trim"},
			},
		},
		Size:     "td.rowfollow:nth-child(5)",
		Seeders:  "td.rowfollow:nth-child(6)",
		Leechers: "td.rowfollow:nth-child(7)",
		Snatched: "td.rowfollow:nth-child(8)",
		// 图标与文字两种显示方式的促销都认；同一行的 font.hot（热门）、font.new 不是促销，不能写成 font[class]
		DiscountIcon: "table.torrentname img.pro_free, table.torrentname img.pro_free2up, table.torrentname img.pro_2up, " +
			"table.torrentname img.pro_50pctdown, table.torrentname img.pro_50pctdown2up, table.torrentname img.pro_30pctdown, " +
			"table.torrentname font.free, table.torrentname font.twoupfree, table.torrentname font.twoup, " +
			"table.torrentname font.halfdown, table.torrentname font.twouphalfdown, table.torrentname font.thirtypercent",
		// 默认的判断只认带 free、50pct 这类字样的写法：文字的 halfdown、twoup、thirtypercent 会读成没有优惠，
		// pro_50pctdown2up 会读成 50%
		DiscountMapping: map[string]v2.DiscountLevel{
			"pro_free":         v2.DiscountFree,
			"pro_free2up":      v2.Discount2xFree,
			"pro_2up":          v2.Discount2xUp,
			"pro_50pctdown":    v2.DiscountPercent50,
			"pro_50pctdown2up": v2.Discount2x50,
			"pro_30pctdown":    v2.DiscountPercent30,
			"free":             v2.DiscountFree,
			"twoupfree":        v2.Discount2xFree,
			"twoup":            v2.Discount2xUp,
			"halfdown":         v2.DiscountPercent50,
			"twouphalfdown":    v2.Discount2x50,
			"thirtypercent":    v2.DiscountPercent30,
		},
		// 限时促销的结束时间：标题后面「(限时<span title="2026-11-03 11:27:47">23时16分</span>)」。
		// 没写限时的促销没有这个 span，结束时间按未知处理
		DiscountEndTime:    "table.torrentname td.embedded > span[title]",
		HRIcon:             `table.torrentname div.circle-text:matchesOwn((?i)^\s*h\d+\s*$)`,
		Category:           "td.rowfollow:nth-child(1) img[alt]",
		UploadTime:         "td.rowfollow:nth-child(4) span[title]",
		DetailDownloadLink: "td.rowhead:contains('下载') + td a[href*='download.php']",
		DetailSubtitle:     "td.rowhead:contains('副标题') + td",
	},
	DetailParser: &v2.DetailParserConfig{
		TimeLayout: "2006-01-02 15:04:05",
		DiscountMapping: map[string]v2.DiscountLevel{
			// img-class based (icon display mode)
			"pro_free":         v2.DiscountFree,
			"pro_free2up":      v2.Discount2xFree,
			"pro_2up":          v2.Discount2xUp,
			"pro_50pctdown":    v2.DiscountPercent50,
			"pro_50pctdown2up": v2.Discount2x50,
			"pro_30pctdown":    v2.DiscountPercent30,
			// font-class based (word display mode)
			"free":          v2.DiscountFree,
			"twoup":         v2.Discount2xUp,
			"twoupfree":     v2.Discount2xFree,
			"thirtypercent": v2.DiscountPercent30,
			"halfdown":      v2.DiscountPercent50,
			"twouphalfdown": v2.Discount2x50,
		},
		// 详情页「基本信息」里只有 H&R 种子才有「<b>H&R:&nbsp;</b>3day」这一项（goquery 输出的 html 里 & 是 &amp;、
		// &nbsp; 是原样的 U+00A0）。页头的「H&R:」（链接到 hnr.php）后面跟的是 </font>，不会误认
		HRKeywords: []string{
			"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run",
			"H&amp;R:\u00a0</b>", "H&amp;R: </b>", "H&amp;R:</b>",
		},
		TitleSelector:    "input[name='torrent_name']",
		IDSelector:       "input[name='detail_torrent_id']",
		DiscountSelector: "h1 img.pro_free, h1 img[class*='pro_'], h1 font.free, h1 font[class]",
		EndTimeSelector:  "h1 span[title]",
		SizeSelector:     "td.rowhead:contains('基本信息')",
		SizeRegex:        `大小[：:]\s*([\d.]+)\s*(GB|MB|KB|TB)`,
	},
	LevelRequirements: ptchdbitsLevelRequirements,
}

// ptchdbitsLevelRequirements 按常见问题（faq.php「提升和降级又是怎样被执行的呢？」，2026-11-02 读）：注册时间、下载量、
// 分享率与魔力值都满足时自动升级；分享率低于降级线（要求减 0.02）或魔力值低于要求时自动降级。站点显示的等级名是
// 「(未烧)User」这样的写法。账号保留（rules.php）：Extreme User 起封存后不删号，Ultimate User 起永久保留。
var ptchdbitsLevelRequirements = []v2.SiteLevelRequirement{
	{
		ID: 0, Name: "(冰人)Peasant", NameAka: []string{"冰人", "Peasant"},
		Privilege: "降级用户：下载超过 50GB 分享率不到 0.4（超过 100GB 不到 0.5、200GB 不到 0.6、400GB 不到 0.7、800GB 不到 0.8）时降为本级，7 天内分享率回到 1 恢复未烧，否则会被清理",
	},
	{ID: 1, Name: "(未烧)User", NameAka: []string{"未烧", "User"}, Privilege: "新用户的默认级别"},
	{
		ID: 2, Name: "(感冒)Power User", NameAka: []string{"感冒", "Power User"},
		Interval: "P5W", Downloaded: "200GB", Ratio: 2.0, Bonus: 80000,
		Privilege: "分享率低于 1.98 或魔力值少于 8 万降级；可以查看 NFO、请求续种、查看排行榜",
	},
	{
		ID: 3, Name: "(发热)Elite User", NameAka: []string{"发热", "Elite User"},
		Interval: "P10W", Downloaded: "500GB", Ratio: 3.0, Bonus: 150000,
		Privilege: "分享率低于 2.98 或魔力值少于 15 万降级",
	},
	{
		ID: 4, Name: "(低烧)Crazy User", NameAka: []string{"低烧", "Crazy User"},
		Interval: "P15W", Downloaded: "800GB", Ratio: 4.0, Bonus: 300000,
		Privilege: "分享率低于 3.98 或魔力值少于 30 万降级；可以查看用户列表，做种、下载、发布时可以匿名",
	},
	{
		ID: 5, Name: "(中烧)Insane User", NameAka: []string{"中烧", "Insane User"},
		Interval: "P20W", Downloaded: "999GB", Ratio: 5.0, Bonus: 650000,
		Privilege: "分享率低于 4.98 或魔力值少于 65 万降级；可以查看普通日志",
	},
	{
		ID: 6, Name: "(高烧)Veteran User", NameAka: []string{"高烧", "Veteran User"},
		Interval: "P25W", Downloaded: "1500GB", Ratio: 6.0, Bonus: 1000000,
		Privilege: "分享率低于 5.98 或魔力值少于 100 万降级；可以查看其它用户的评论、帖子历史",
	},
	{
		ID: 7, Name: "(烧糊涂)Extreme User", NameAka: []string{"烧糊涂", "Extreme User"},
		Interval: "P30W", Downloaded: "2TB", Ratio: 7.0, Bonus: 2200000,
		Privilege: "分享率低于 6.98 或魔力值少于 220 万降级；首次升级送 1 枚邀请；封存账号后不删号",
	},
	{
		ID: 8, Name: "(走火入魔)Ultimate User", NameAka: []string{"走火入魔", "Ultimate User"},
		Interval: "P40W", Downloaded: "3TB", Ratio: 8.0, Bonus: 3500000,
		Privilege: "分享率低于 7.98 或魔力值少于 350 万降级；首次升级送 2 枚邀请；账号永久保留",
	},
	{
		ID: 9, Name: "(骨灰)Nexus Master", NameAka: []string{"骨灰", "Nexus Master"},
		Interval: "P52W", Downloaded: "4TB", Ratio: 10.0, Bonus: 5000000,
		Privilege: "分享率低于 9.98 或魔力值少于 500 万降级；首次升级送 3 枚邀请；账号永久保留",
	},
	{
		ID: v2.MinVipLevelID, Name: "贵宾(VIP)", NameAka: []string{"贵宾", "VIP"},
		GroupType: v2.LevelGroupVIP,
		Privilege: "管理员分配，和 Nexus Master 权限相同，免除自动降级；有效期内不计 H&R",
	},
}

func init() {
	v2.RegisterSiteDefinition(PTCHDBitsDefinition)
}
