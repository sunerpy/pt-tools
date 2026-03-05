package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

var AGSVPTDefinition = &v2.SiteDefinition{
	ID:              "agsvpt",
	Name:            "AGSVPT",
	Aka:             []string{"末日种子库", "AGSV"},
	Description:     "Arctic Global Seed Vault",
	Schema:          v2.SchemaNexusPHP,
	URLs:            []string{"https://www.agsvpt.com/"},
	FaviconURL:      "https://www.agsvpt.com/favicon.ico",
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
				Fields: []string{"id", "name", "seeding", "leeching"},
			},
			{
				RequestConfig: v2.RequestConfig{
					URL:          "/userdetails.php",
					ResponseType: "document",
				},
				Assertion: map[string]string{"id": "params.id"},
				Fields: []string{
					"name", "uploaded", "downloaded", "ratio", "levelName",
					"bonus", "joinTime", "messageCount",
				},
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
					"td.rowhead:contains('传输') + td",
					"td.rowhead:contains('傳輸') + td",
					"td.rowhead:contains('Transfer') + td",
					"td.rowhead:contains('上传量') + td",
					"td.rowhead:contains('上傳量') + td",
					"td.rowhead:contains('Uploaded') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>上[传傳]量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"downloaded": {
				Selector: []string{
					"td.rowhead:contains('传输') + td",
					"td.rowhead:contains('傳輸') + td",
					"td.rowhead:contains('Transfer') + td",
					"td.rowhead:contains('下载量') + td",
					"td.rowhead:contains('下載量') + td",
					"td.rowhead:contains('Downloaded') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`<strong>下[载載]量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"ratio": {
				Selector: []string{
					"td.rowhead:contains('传输') + td",
					"td.rowhead:contains('傳輸') + td",
					"td.rowhead:contains('Transfer') + td",
					"td.rowhead:contains('分享率') + td font",
					"td.rowhead:contains('分享率') + td",
					"td.rowhead:contains('Ratio') + td font",
					"td.rowhead:contains('Ratio') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:分享率|Ratio)</strong>[：:\s]*(?:<font[^>]*>)?([\d.,]+|∞|Inf)`}},
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
					"td.rowhead:contains('冰晶') + td",
					"td.rowhead:contains('Bonus') + td",
				},
				Filters: []v2.Filter{{Name: "parseNumber"}},
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
					{Name: "regex", Args: []any{`class="arrowup"[^>]*/>(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			"leeching": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`class="arrowdown"[^>]*/>(\d+)`}},
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
		HRKeywords:       []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run"},
		TitleSelector:    "h1",
		DiscountSelector: "h1 font.free, h1 font[class]",
		EndTimeSelector:  "h1 span[title]",
		SizeSelector:     "td.rowfollow:contains('大小')",
		SizeRegex:        `大小[：:]\s*([\d.]+)\s*(GB|MB|KB|TB)`,
	},
	LevelRequirements: agsvptLevelRequirements,
}

var agsvptLevelRequirements = []v2.SiteLevelRequirement{
	{ID: 1, Name: "User", NameAka: []string{"用户"}, Privilege: "新用户的默认级别"},
	{ID: 2, Name: "Power User", NameAka: []string{"北冰珍珠熊"}, Downloaded: "50GB", Ratio: 1.05, Bonus: 40000},
	{ID: 3, Name: "Elite User", NameAka: []string{"深渊蔚蓝熊"}, Interval: "P8W", Downloaded: "120GB", Ratio: 1.55, Bonus: 80000},
	{ID: 4, Name: "Crazy User", NameAka: []string{"翡翠森林熊"}, Interval: "P12W", Downloaded: "300GB", Ratio: 2.05, Bonus: 150000},
	{ID: 5, Name: "Insane User", NameAka: []string{"神秘紫晶熊"}, Interval: "P20W", Downloaded: "500GB", Ratio: 2.55, Bonus: 400000},
	{ID: 6, Name: "Veteran User", NameAka: []string{"寒冰白金熊"}, Interval: "P28W", Downloaded: "750GB", Ratio: 4.05, Bonus: 800000},
	{ID: 7, Name: "Extreme User", NameAka: []string{"皇家金辉熊"}, Interval: "P40W", Downloaded: "1TB", Ratio: 5.05, Bonus: 1400000},
	{ID: 8, Name: "Ultimate User", NameAka: []string{"永恒铂金熊"}, Interval: "P52W", Downloaded: "1.5TB", Ratio: 6.05, Bonus: 2200000},
	{ID: 9, Name: "Nexus Master", NameAka: []string{"钻石之冠北极熊"}, Interval: "P70W", Downloaded: "3TB", Ratio: 7.05, Bonus: 3200000},
}

func init() {
	v2.RegisterSiteDefinition(AGSVPTDefinition)
}
