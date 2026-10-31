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
}

func init() {
	v2.RegisterSiteDefinition(BYRPTDefinition)
}
