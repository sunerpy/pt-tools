package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// PTTimeDefinition is the site definition for PTT (pttime.org)
// PTT uses a custom "PTT-NP" NexusPHP fork with several deviations:
//   - Discount in search: <font class="promotion free"> etc. (NOT img.pro_*)
//   - Category cell uses <span class="category" title="..."> (NOT img[alt])
//   - Userinfo labels use "上传:" / "下载:" / "分享率:" (no 量 suffix)
//   - Info block emoji arrows for seeding/leeching (NOT class="arrowup/arrowdown")
//   - Hidden columns (PTR, comments) use style="display:none" but still consume nth-child indices
var PTTimeDefinition = &v2.SiteDefinition{
	ID:             "pttime",
	Name:           "PTT",
	Aka:            []string{"PTTime", "PT時間"},
	Description:    "PTT (PTT-NP fork of NexusPHP)",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://www.pttime.org/"},
	FaviconURL:     "https://www.pttime.org/favicon.ico",
	TimezoneOffset: "+0800",
	RateLimit:      0.5,
	RateBurst:      2,
	UserInfo: &v2.UserInfoConfig{
		PickLast:     []string{"id"},
		RequestDelay: 500,
		Process: []v2.UserInfoProcess{
			{
				RequestConfig: v2.RequestConfig{URL: "/index.php", ResponseType: "document"},
				Fields:        []string{"id", "name", "uploaded", "downloaded", "ratio", "seeding", "leeching", "bonus", "levelName"},
			},
			{
				RequestConfig: v2.RequestConfig{URL: "/userdetails.php", ResponseType: "document"},
				Assertion:     map[string]string{"id": "params.id"},
				Fields:        []string{"joinTime", "bonusPerHour", "lastAccessAt"},
			},
		},
		Selectors: map[string]v2.FieldSelector{
			"id": {
				Selector: []string{
					"#info_block a[href*='userdetails.php'][class*='Name']",
					"a[href*='userdetails.php'][class*='Name']",
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
					"a[href*='userdetails.php'][class*='Name']",
				},
			},
			// PTT-NP info_block: "上传:" / "下载:" (no 量 suffix) — values are AFTER </font>
			"uploaded": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:上传|上傳|Uploaded)[：:]?\s*</font>\s*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"downloaded": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:下载|下載|Downloaded)[：:]?\s*</font>\s*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"ratio": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:分享率|Ratio)[：:]?\s*</font>\s*([\d.,]+|∞|Inf)`}},
					{Name: "parseNumber"},
				},
			},
			// PTT-NP uses emoji arrows (⬆/⬇) inside <font title="当前做种|当前下载">
			"seeding": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`title="当前做种"[^>]*>[^<]*</font>\s*(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			"leeching": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`title="当前下载"[^>]*>[^<]*</font>\s*(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			"bonus": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`魔力值[^:：]*[:：]\s*([\d.,]+)`}},
					{Name: "parseNumber"},
				},
			},
			// 等级紧跟在 UID 后面：「[UID=117144][(幼儿园)User]」，取括号里的中文等级名。
			// 只认 UID 后面这一处：同一块里还有「私人邀请：2[(小学)Power User及以上可用]」，
			// 原来按「(…)…User」在整块里找，User 等级（幼儿园）对不上、取到了邀请说明里的「小学」
			"levelName": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`\[UID=\d+\]\s*\[(?:<[^>]+>)*\(([^)<]+)\)`}},
				},
			},
			"joinTime": {
				Selector: []string{
					"td.rowhead:contains('加入日期') + td",
					"td.rowhead:contains('加入时间') + td",
					"td.rowhead:contains('Join') + td",
				},
				Filters: []v2.Filter{
					{Name: "split", Args: []any{" (", 0}},
					{Name: "parseTime"},
				},
			},
			"lastAccessAt": {
				Selector: []string{
					"td.rowhead:contains('最近动向') + td",
					"td.rowhead:contains('最近動向') + td",
					"td.rowhead:contains('最后活动') + td",
					"td.rowhead:contains('最後活動') + td",
					"td.rowhead:contains('上次访问') + td",
					"td.rowhead:contains('上次訪問') + td",
					"td.rowhead:contains('Last access') + td",
				},
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
			},
			"bonusPerHour": {
				Selector: []string{
					"#info_block",
					"body",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`\(([\d.,]+)\s*魔力/小[時时]\)`}},
					{Name: "parseNumber"},
				},
			},
		},
	},
	Selectors: &v2.SiteSelectors{
		TableRows: "table.torrents > tbody > tr:has(table.torrentname), table.torrents > tr:has(table.torrentname)",
		Title:     "table.torrentname a.torrentname_title, table.torrentname a[href*='details.php']",
		TitleLink: "table.torrentname a.torrentname_title, table.torrentname a[href*='details.php']",
		Subtitle:  "table.torrentname td.embedded > font[title]",
		// PTT-NP uses <span class="category" title="..."> (NOT img[alt])
		Category: "td.rowfollow:nth-child(1) span.category[title], td.rowfollow:nth-child(1) img[alt]",
		// PTT-NP hides columns 3 (PTR) and 4 (comments) but they still consume nth-child indices:
		// 1=type 2=title 3=PTR(hidden) 4=comments(hidden) 5=time 6=size 7=seeders 8=leechers 9=snatched 10=progress 11=uploader
		UploadTime: "td.rowfollow:nth-child(5) span[title]",
		Size:       "td.rowfollow:nth-child(6)",
		Seeders:    "td.rowfollow:nth-child(7)",
		Leechers:   "td.rowfollow:nth-child(8)",
		Snatched:   "td.rowfollow:nth-child(9)",
		// PTT-NP uses <font class="promotion <type>"> instead of <img class="pro_<type>">;
		// <font class="promotion bb"> 是「字幕/附件(1)」角标，不是优惠
		DiscountIcon: "table.torrentname font.promotion:not(.bb), td.embedded font.promotion:not(.bb), table.torrentname img.pro_free, table.torrentname img.pro_free2up, table.torrentname img.pro_50pctdown, table.torrentname img.pro_30pctdown, table.torrentname img.pro_2up, table.torrentname img.pro_50pctdown2up",
		// 优惠的结束时间在角标后面紧跟的 <span title="2026-10-26 18:46:52">1小时34分钟</span>（title 是结束时间，文字是剩余时间）。
		// 2026-10-26 从站点免费列表核对：没有这一项时刷流把免费种子全按「免费结束时间未知」拒掉
		DiscountEndTime: "table.torrentname font.promotion:not(.bb) + span[title]",
		// Explicit mapping required: parseDiscountFromElement's default switch doesn't know "halfdown"/"twouphalfdown" keywords
		DiscountMapping: map[string]v2.DiscountLevel{
			"free":           v2.DiscountFree,
			"twoup":          v2.Discount2xUp,
			"twoupfree":      v2.Discount2xFree,
			"halfdown":       v2.DiscountPercent50,
			"twouphalfdown":  v2.Discount2x50,
			"thirtypercent":  v2.DiscountPercent30,
			"zeroupzerodown": v2.DiscountFree,
		},
		DetailDownloadLink: "tr:has(td.rowhead:contains('下载')) a[href*='download.php'], td.rowhead:contains('下载') + td a[href*='download.php']",
		DetailSubtitle:     "td.rowhead:contains('副标题') + td.rowfollow, td.rowhead:contains('副标题') + td",
	},
	DetailParser: &v2.DetailParserConfig{
		TimeLayout: "2006-01-02 15:04:05",
		DiscountMapping: map[string]v2.DiscountLevel{
			"free":           v2.DiscountFree,
			"twoup":          v2.Discount2xUp,
			"twoupfree":      v2.Discount2xFree,
			"halfdown":       v2.DiscountPercent50,
			"twouphalfdown":  v2.Discount2x50,
			"thirtypercent":  v2.DiscountPercent30,
			"zeroupzerodown": v2.DiscountFree,
		},
		HRKeywords:       []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run", "H&R"},
		TitleSelector:    "h1#top, h1",
		DiscountSelector: "h1#top font[class], h1 font[class]",
		EndTimeSelector:  "h1#top span[title], h1 span[title]",
		// 基本信息 label is td.rowhead; parser reads .Next() to get the value cell
		// which contains inline "大小：136.99 GB" — matched by SizeRegex
		SizeSelector: "td.rowhead:contains('基本信息')",
		SizeRegex:    `大小[：:]\s*(?:</?b>)*\s*([\d.]+)\s*(GB|MB|KB|TB)`,
	},
	LevelRequirements: pttimeLevelRequirements,
}

// pttimeLevelRequirements 按站点「校验用户等级」页（action.php?type=vertify_class，2026-10-26 读）的等级规则：
// 每个等级都要同时满足注册时长、下载量、分享率，符合后 24 小时内自动升级；ID 沿用站点的「N级」。
// 下载量小于 100G 的用户不参与升降级，保持 User(幼儿园)。页面上的 G 按原样写成 GB。
var pttimeLevelRequirements = []v2.SiteLevelRequirement{
	{
		ID: 0, Name: "Peasant(婴儿)", NameAka: []string{"婴儿", "Peasant"},
		Privilege: "降级用户：下载超 100G 且分享率低于 0.4（超 200G 低于 0.5、超 400G 低于 0.6、超 800G 低于 0.7、超 1600G 低于 0.8）降到这一级，要在 7 天内改善分享率，否则封禁",
	},
	{
		ID: 1, Name: "User(幼儿园)", NameAka: []string{"幼儿园", "User"},
		Privilege: "注册默认等级。新人考核：注册后 30 天内上传 30GB、下载 30GB、魔力 3000（也可捐赠直接通过）。0–6 级要在 200 天内至少登录或访问一次网站，只用脚本签到不算；每天页面访问上限 500 次（客户端辅种不算）",
	},
	{
		ID: 2, Name: "Power User(小学)", NameAka: []string{"小学", "Power User"},
		Interval: "P4W", Downloaded: "512GB", Ratio: 1.05,
		Privilege: "分享率低于 0.95 自动降级；首次升到这一级送 2 个邀请，可以用私人邀请和开注邀请码",
	},
	{
		ID: 3, Name: "Elite User(初中)", NameAka: []string{"初中", "Elite User"},
		Interval: "P8W", Downloaded: "2048GB", Ratio: 1.55,
		Privilege: "分享率低于 1.45 自动降级；首次升到这一级送 2 个邀请",
	},
	{
		ID: 4, Name: "Crazy User(高中)", NameAka: []string{"高中", "Crazy User"},
		Interval: "P15W", Downloaded: "4096GB", Ratio: 2.05,
		Privilege: "分享率低于 1.95 自动降级；首次升到这一级送 2 个邀请",
	},
	{
		ID: 5, Name: "Insane User(专科)", NameAka: []string{"专科", "Insane User"},
		Interval: "P25W", Downloaded: "8192GB", Ratio: 2.55,
		Privilege: "分享率低于 2.45 自动降级；首次升到这一级送 2 个邀请",
	},
	{
		ID: 6, Name: "Veteran User(本科)", NameAka: []string{"本科", "Veteran User"},
		Interval: "P52W", Downloaded: "16384GB", Ratio: 3.05,
		Privilege: "分享率低于 2.95 自动降级；首次升到这一级送 2 个邀请",
	},
	{
		ID: 7, Name: "Extreme User(研究生)", NameAka: []string{"研究生", "Extreme User"},
		Interval: "P80W", Downloaded: "25000GB", Ratio: 3.55,
		Privilege: "分享率低于 3.45 自动降级；首次升到这一级送 2 个邀请；从这一级起不再要求 200 天内登录一次",
	},
	{
		ID: 8, Name: "Ultimate User(博士生)", NameAka: []string{"博士生", "Ultimate User"},
		Interval: "P104W", Downloaded: "45000GB", Ratio: 4.05,
		Privilege: "分享率低于 3.95 自动降级；首次升到这一级送 5 个邀请",
	},
	{
		ID: 9, Name: "Nexus Master(博士后)", NameAka: []string{"博士后", "Nexus Master"},
		Interval: "P130W", Downloaded: "90000GB", Ratio: 4.55,
		Privilege: "分享率低于 4.45 自动降级；首次升到这一级送 10 个邀请",
	},
	{
		ID: v2.MinVipLevelID, Name: "VIP(贵宾)", NameAka: []string{"贵宾", "临时贵宾", "永久贵宾", "VIP"},
		GroupType: v2.LevelGroupVIP,
		Privilege: "临时贵宾、永久贵宾，捐赠获得，权限见站点的贵宾特权说明",
	},
}

func init() {
	v2.RegisterSiteDefinition(PTTimeDefinition)
}
