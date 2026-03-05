package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

var HDFansDefinition = &v2.SiteDefinition{
	ID:              "hdfans",
	Name:            "HDFans",
	Aka:             []string{"红豆饭"},
	Description:     "与志同道合之人前行 分享更多值得珍藏的资源",
	Schema:          v2.SchemaNexusPHP,
	URLs:            []string{"https://hdfans.org/"},
	FaviconURL:      "https://hdfans.org/favicon.ico",
	TimezoneOffset:  "+0800",
	RateLimit:       0.5,
	RateBurst:       2,
	HREnabled:       true,
	HRSeedTimeHours: 72,
	UserInfo: &v2.UserInfoConfig{
		PickLast:     []string{"id"},
		RequestDelay: 500,
		Process: []v2.UserInfoProcess{
			{
				RequestConfig: v2.RequestConfig{
					URL:          "/index.php",
					ResponseType: "document",
				},
				Fields: []string{"id", "name", "uploaded", "downloaded", "ratio", "seeding", "leeching", "bonus"},
			},
			{
				RequestConfig: v2.RequestConfig{
					URL:          "/userdetails.php",
					ResponseType: "document",
				},
				Assertion: map[string]string{"id": "params.id"},
				Fields:    []string{"name", "uploaded", "downloaded", "ratio", "levelName", "bonus", "joinTime", "messageCount"},
			},
			{
				RequestConfig: v2.RequestConfig{
					URL:          "/mybonus.php",
					ResponseType: "document",
				},
				Fields: []string{"bonusPerHour"},
			},
		},
		Selectors: map[string]v2.FieldSelector{
			"id": {
				Selector: []string{
					"a[href*='userdetails.php'][class*='Name']",
					"a.User_Name[href*='userdetails.php']",
					"#info_block a[href*='userdetails.php']",
					"a[href*='userdetails.php']",
				},
				Attr:    "href",
				Filters: []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			"name": {
				Selector: []string{
					"a[href*='userdetails.php'][class*='Name']",
					"a.User_Name[href*='userdetails.php']",
					"#info_block a[href*='userdetails.php']",
				},
			},
			"uploaded": {
				Selector: []string{
					"#info_block",
					"td.rowhead:contains('上传量') + td",
					"td.rowhead:contains('上傳量') + td",
					"td.rowhead:contains('Uploaded') + td",
					"td.rowhead:contains('传输') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:上传量|上傳量|Uploaded)[^\d]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"downloaded": {
				Selector: []string{
					"#info_block",
					"td.rowhead:contains('下载量') + td",
					"td.rowhead:contains('下載量') + td",
					"td.rowhead:contains('Downloaded') + td",
					"td.rowhead:contains('传输') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:下载量|下載量|Downloaded)[^\d]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"ratio": {
				Selector: []string{
					"#info_block",
					"td.rowhead:contains('分享率') + td font",
					"td.rowhead:contains('分享率') + td",
					"td.rowhead:contains('Ratio') + td font",
					"td.rowhead:contains('Ratio') + td",
					"td.rowhead:contains('传输') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:分享率|Ratio|做种/下载时间比率)[^\d∞]*([\d.,]+|∞|Inf)`}},
					{Name: "parseNumber"},
				},
			},
			"levelName": {
				Selector: []string{
					"td.rowhead:contains('等级') + td > img",
					"td.rowhead:contains('等級') + td > img",
					"td.rowhead:contains('Class') + td > img",
					"td.rowhead:contains('等级') + td img",
				},
				Attr: "title",
			},
			"bonus": {
				Selector: []string{
					"td.rowhead:contains('魔力值') + td",
					"td.rowhead:contains('魔力') + td",
					"td.rowhead:contains('Bonus') + td",
					"#info_block",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:魔力值|Bonus)[^\d]*([\d.,]+)`}},
					{Name: "parseNumber"},
				},
			},
			"bonusPerHour": {
				Selector: []string{
					"#outer td[rowspan]",
					"div:contains('你当前每小时能获取')",
				},
				Filters: []v2.Filter{{Name: "parseNumber"}},
			},
			"messageCount": {
				Text:     "0",
				Selector: []string{"td[style*='background: red'] a[href*='messages.php']"},
				Filters:  []v2.Filter{{Name: "parseNumber"}},
			},
			"joinTime": {
				Selector: []string{
					"td.rowhead:contains('加入日期') + td",
					"td.rowhead:contains('Join') + td",
				},
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
	LevelRequirements: hdfansLevelRequirements,
}

var hdfansLevelRequirements = []v2.SiteLevelRequirement{
	{ID: 1, Name: "Peasant", NameAka: []string{"小卒"}, Privilege: "降级用户"},
	{ID: 2, Name: "User", NameAka: []string{"用户"}, Privilege: "新用户的默认级别"},
	{ID: 3, Name: "Power User", NameAka: []string{"高级用户"}, Interval: "P4W", Downloaded: "50GB", Ratio: 1.0, SeedingBonus: 50000},
	{ID: 4, Name: "Elite User", NameAka: []string{"精英用户"}, Interval: "P8W", Downloaded: "120GB", Ratio: 1.5, SeedingBonus: 100000},
	{ID: 5, Name: "Crazy User", NameAka: []string{"疯狂用户"}, Interval: "P15W", Downloaded: "256GB", Ratio: 2.0, SeedingBonus: 250000},
	{ID: 6, Name: "Insane User", NameAka: []string{"变态用户"}, Interval: "P30W", Downloaded: "512GB", Ratio: 2.5, SeedingBonus: 400000},
	{ID: 7, Name: "Veteran User", NameAka: []string{"资深用户"}, Interval: "P40W", Downloaded: "1TB", Ratio: 3.0, SeedingBonus: 600000},
	{ID: 8, Name: "Extreme User", NameAka: []string{"极限用户"}, Interval: "P50W", Downloaded: "2TB", Ratio: 3.5, SeedingBonus: 800000},
	{ID: 9, Name: "Ultimate User", NameAka: []string{"终极用户"}, Interval: "P60W", Downloaded: "4TB", Ratio: 4.0, SeedingBonus: 1000000},
	{ID: 10, Name: "Nexus Master", NameAka: []string{"大师"}, Interval: "P100W", Downloaded: "10TB", Ratio: 5.0, SeedingBonus: 1688888},
}

func init() {
	v2.RegisterSiteDefinition(HDFansDefinition)
}
