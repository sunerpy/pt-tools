package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

var FiveTwoPTDefinition = &v2.SiteDefinition{
	ID:              "52pt",
	Name:            "52PT",
	Aka:             []string{"我爱PT"},
	Description:     "低调地在这个PT校园快乐成长 快乐分享",
	Schema:          v2.SchemaNexusPHP,
	URLs:            []string{"https://52pt.site/"},
	FaviconURL:      "https://52pt.site/favicon.ico",
	TimezoneOffset:  "+0800",
	RateLimit:       0.5,
	RateBurst:       2,
	HREnabled:       true,
	HRSeedTimeHours: 24, // 5天窗口内做种24小时
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
				Fields:        []string{"name", "uploaded", "downloaded", "ratio", "levelName", "joinTime"},
			},
		},
		Selectors: map[string]v2.FieldSelector{
			"id": {
				Selector: []string{"a[href*='userdetails.php'][class*='Name']", "a.User_Name[href*='userdetails.php']", "#info_block a[href*='userdetails.php']", "a[href*='userdetails.php']"},
				Attr:     "href",
				Filters:  []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			"name": {
				Selector: []string{"a[href*='userdetails.php'][class*='Name']", "a.User_Name[href*='userdetails.php']", "#info_block a[href*='userdetails.php']"},
			},
			"uploaded": {
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('傳輸') + td", "td.rowhead:contains('Transfer') + td", "td.rowhead:contains('上传量') + td", "td.rowhead:contains('上傳量') + td", "td.rowhead:contains('Uploaded') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`上[传傳][量]?</strong>[：:\s]*\s*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"downloaded": {
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('傳輸') + td", "td.rowhead:contains('Transfer') + td", "td.rowhead:contains('下载量') + td", "td.rowhead:contains('下載量') + td", "td.rowhead:contains('Downloaded') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`下[载載][量]?</strong>[：:\s]*\s*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"ratio": {
				Selector: []string{"td.rowhead:contains('BT时间') + td", "td.rowhead:contains('传输') + td", "td.rowhead:contains('分享率') + td font", "td.rowhead:contains('分享率') + td", "td.rowhead:contains('Ratio') + td font", "td.rowhead:contains('Ratio') + td", "#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:分享率|Ratio|做种/下载时间比率)[^\d∞]*([\d.,]+|∞|Inf)`}},
					{Name: "parseNumber"},
				},
			},
			"levelName": {
				Selector: []string{"td.rowhead:contains('等级') + td > img", "td.rowhead:contains('等級') + td > img", "td.rowhead:contains('Class') + td > img", "td.rowhead:contains('等级') + td img"},
				Attr:     "title",
			},
			"bonus": {
				Selector: []string{"#info_block", "#info_block a[href*='mybonus.php']", "td.rowhead:contains('魔力值') + td", "td.rowhead:contains('Bonus') + td"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:魔力值|Bonus)[^\d]*([\d.,]+)`}},
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
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`class="arrowup"[^>]*/>\s*(?:做种数:)?(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			"leeching": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`class="arrowdown"[^>]*/>\s*(?:下载数:)?(\d+)`}},
					{Name: "parseNumber"},
				},
			},
		},
	},
	Selectors: &v2.SiteSelectors{
		TableRows:          "table.torrents > tbody > tr:has(table.torrentname), table.torrents > tr:has(table.torrentname)",
		Title:              "table.torrentname a[href*='details.php']",
		TitleLink:          "table.torrentname a[href*='details.php']",
		Subtitle:           "table.torrentname td.embedded > span:not(.optiontag)",
		Size:               "td.rowfollow:nth-child(5)",
		Seeders:            "td.rowfollow:nth-child(6)",
		Leechers:           "td.rowfollow:nth-child(7)",
		Snatched:           "td.rowfollow:nth-child(8)",
		DiscountIcon:       "img.pro_free, img.pro_free2up, img.pro_50pctdown, img.pro_30pctdown, img.pro_2up, img.pro_50pctdown2up",
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
		HRKeywords:       []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run", "H&R"},
		TitleSelector:    "h1",
		DiscountSelector: "h1 font.free, h1 font[class]",
		EndTimeSelector:  "h1 span[title]",
		SizeSelector:     "td.rowfollow:contains('大小')",
		SizeRegex:        `大小[：:]\s*([\d.]+)\s*(GB|MB|KB|TB)`,
	},
	LevelRequirements: fiveTwoPTLevelRequirements,
}

var fiveTwoPTLevelRequirements = []v2.SiteLevelRequirement{
	{ID: 1, Name: "User", NameAka: []string{"用户"}, Privilege: "新用户的默认级别"},
	{ID: 2, Name: "Power User", NameAka: []string{"高级用户"}, Interval: "P4W", Downloaded: "50GB", Ratio: 1.05},
	{ID: 3, Name: "Elite User", NameAka: []string{"精英用户"}, Interval: "P8W", Downloaded: "120GB", Ratio: 1.55},
	{ID: 4, Name: "Crazy User", NameAka: []string{"疯狂用户"}, Interval: "P15W", Downloaded: "300GB", Ratio: 2.05},
	{ID: 5, Name: "Insane User", NameAka: []string{"变态用户"}, Interval: "P25W", Downloaded: "1536GB", Ratio: 2.55},
	{ID: 6, Name: "Veteran User", NameAka: []string{"资深用户"}, Interval: "P40W", Downloaded: "2560GB", Ratio: 3.05},
	{ID: 7, Name: "Extreme User", NameAka: []string{"极限用户"}, Interval: "P60W", Downloaded: "3072GB", Ratio: 3.55},
	{ID: 8, Name: "Ultimate User", NameAka: []string{"终极用户"}, Interval: "P80W", Downloaded: "4608GB", Ratio: 4.05},
	{ID: 9, Name: "Nexus Master", NameAka: []string{"大师"}, Interval: "P100W", Downloaded: "5632GB", Ratio: 4.55},
}

func init() {
	v2.RegisterSiteDefinition(FiveTwoPTDefinition)
}
