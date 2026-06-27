package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// PTerClubDefinition is the site definition for PTerClub (pterclub.net, 猫站).
// Standard NexusPHP site; user stats live in the #info_block div. Notable quirks:
//   - bonus label is "猫粮" (not 魔力值), inside a <span class="color_bonus"> wrapped in <a>
//   - 上传量/下载量/分享率 use full-width colons (：)
//   - detail-page free is encoded in the H1 <font class='free'> onmouseover tooltip
var PTerClubDefinition = &v2.SiteDefinition{
	ID:             "pterclub",
	Name:           "PTerClub",
	Aka:            []string{"猫站", "PTer", "PT之友"},
	Description:    "综合性 PT 站点",
	Schema:         v2.SchemaNexusPHP,
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
}

func init() {
	v2.RegisterSiteDefinition(PTerClubDefinition)
}
