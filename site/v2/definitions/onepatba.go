package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

var OnePTBADefinition = &v2.SiteDefinition{
	ID:              "1ptba",
	Name:            "1PTBar",
	Aka:             []string{"壹PT", "1PTBA"},
	Description:     "分享互联，收获快乐",
	Schema:          v2.SchemaNexusPHP,
	URLs:            []string{"https://1ptba.com/"},
	FaviconURL:      "https://1ptba.com/favicon.ico",
	AuthMethod:      v2.AuthMethodCookie,
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
				Fields:        []string{"uploaded", "downloaded", "ratio", "levelName", "joinTime"},
			},
		},
		Selectors: map[string]v2.FieldSelector{
			"id": {
				Selector: []string{"a[href*='userdetails.php'][class*='Name']", "#info_block a[href*='userdetails.php']", "a[href*='userdetails.php']"},
				Attr:     "href",
				Filters:  []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			"name": {
				Selector: []string{"a[href*='userdetails.php'][class*='Name']", "#info_block a[href*='userdetails.php']"},
			},
			"uploaded": {
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('傳輸') + td"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`<strong>上传量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}}, {Name: "parseSize"}},
			},
			"downloaded": {
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('傳輸') + td"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`<strong>下载量</strong>[：:\s]*([\d.,]+\s*[KMGTP]?i?B)`}}, {Name: "parseSize"}},
			},
			"ratio": {
				Selector: []string{"td.rowhead:contains('传输') + td", "td.rowhead:contains('傳輸') + td"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`<strong>分享率</strong>[：:\s]*(?:<font[^>]*>)?([\d.,]+|∞|Inf)`}}, {Name: "parseNumber"}},
			},
			"levelName": {
				Selector: []string{"td.rowhead:contains('等级') + td img", "td.rowhead:contains('等級') + td img"},
				Attr:     "title",
			},
			"bonus": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`魔力值\s*</font>\s*\[[^\]]*\]\s*[:：]\s*([\d.,]+)`}}, {Name: "parseNumber"}},
			},
			"joinTime": {
				Selector: []string{"td.rowhead:contains('加入日期') + td", "td.rowhead:contains('Join') + td"},
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}}, {Name: "parseTime"}},
			},
			"seeding": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`class="arrowup"[^>]*/>\s*(\d+)`}}, {Name: "parseNumber"}},
			},
			"leeching": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters:  []v2.Filter{{Name: "regex", Args: []any{`class="arrowdown"[^>]*/>\s*(\d+)`}}, {Name: "parseNumber"}},
			},
		},
	},
	Selectors: &v2.SiteSelectors{
		TableRows:          "table.torrents > tbody > tr:has(table.torrentname), table.torrents > tr:has(table.torrentname)",
		Title:              "table.torrentname a[href*='details.php']",
		TitleLink:          "table.torrentname a[href*='details.php']",
		Subtitle:           "table.torrentname td.embedded > span:last-of-type, table.torrentname td.embedded > span",
		Size:               "td.rowfollow:nth-child(5)",
		Seeders:            "td.rowfollow:nth-child(6)",
		Leechers:           "td.rowfollow:nth-child(7)",
		Snatched:           "td.rowfollow:nth-child(8)",
		DiscountIcon:       "img.pro_free, img.pro_free2up, img.pro_50pctdown, img.pro_30pctdown, img.pro_2up, img.pro_50pctdown2up",
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
	LevelRequirements: onePTBALevelRequirements,
}

var onePTBALevelRequirements = []v2.SiteLevelRequirement{
	{ID: 1, Name: "User"},
	{ID: 2, Name: "Power User", Interval: "P5W", Downloaded: "50GB", Ratio: 1.3, SeedingBonus: 40000},
	{ID: 3, Name: "Elite User", Interval: "P8W", Downloaded: "120GB", Ratio: 1.9, SeedingBonus: 80000},
	{ID: 4, Name: "Crazy User", Interval: "P15W", Downloaded: "300GB", Ratio: 2.3, SeedingBonus: 150000},
	{ID: 5, Name: "Insane User", Interval: "P30W", Downloaded: "500GB", Ratio: 2.7, SeedingBonus: 250000},
	{ID: 6, Name: "Veteran User", Interval: "P60W", Downloaded: "1TB", Ratio: 3.2, SeedingBonus: 400000},
	{ID: 7, Name: "Extreme User", Interval: "P90W", Downloaded: "2TB", Ratio: 3.7, SeedingBonus: 600000},
	{ID: 8, Name: "Ultimate User", Interval: "P120W", Downloaded: "4TB", Ratio: 4.2, SeedingBonus: 800000},
	{ID: 9, Name: "Nexus Master", Interval: "P150W", Downloaded: "10TB", Ratio: 5.2, SeedingBonus: 1000000},
}

func init() {
	v2.RegisterSiteDefinition(OnePTBADefinition)
}
