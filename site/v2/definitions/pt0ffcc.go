package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// PT0FFCCDefinition is the site definition for Farmm (pt.0ff.cc).
// The site is a standard NexusPHP template derived from CHD/scenetorrents with
// two significant deviations:
//   - Detail page: size is inline inside the `基本信息` row (not its own rowhead row).
//     The SizeRegex pattern extracts from the value cell's HTML.
//   - Userinfo page: HDSky-style combined `传输` row packs ratio + uploaded + downloaded
//     in a single cell; selectors use regex to parse the combined HTML.
var PT0FFCCDefinition = &v2.SiteDefinition{
	ID:             "pt0ffcc",
	Name:           "Farmm",
	Aka:            []string{"0ff", "pt.0ff.cc", "种子农场"},
	Description:    "Farmm 种子农场（pt.0ff.cc）",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://pt.0ff.cc/"},
	FaviconURL:     "https://pt.0ff.cc/favicon.ico",
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
				Fields: []string{
					"name", "uploaded", "downloaded", "ratio", "levelName", "joinTime",
				},
			},
		},
		Selectors: map[string]v2.FieldSelector{
			"id": {
				Selector: []string{
					"a[href*='userdetails.php'][class*='Name']",
					"#info_block a[href*='userdetails.php']",
					"a[href*='userdetails.php']",
				},
				Attr:    "href",
				Filters: []v2.Filter{{Name: "querystring", Args: []any{"id"}}},
			},
			"name": {
				Selector: []string{
					"h1 > span.nowrap > b",
					"h1 span.nowrap b",
					"a[href*='userdetails.php'][class*='Name'] b",
					"a[href*='userdetails.php'][class*='Name']",
				},
			},
			// 传输 row (HDSky-style combined cell) — regex extracts uploaded/downloaded/ratio.
			// Anchored via \b to distinguish 上传量 from 实际上传量 / 下载量 from 实际下载量.
			"uploaded": {
				Selector: []string{
					"td.rowhead:contains('传输') + td",
					"td.rowhead:contains('傳輸') + td",
					"td.rowhead:contains('上传量') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:^|[^实])上传量</strong>[：:\s]*\s*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"downloaded": {
				Selector: []string{
					"td.rowhead:contains('传输') + td",
					"td.rowhead:contains('傳輸') + td",
					"td.rowhead:contains('下载量') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`(?:^|[^实])下载量</strong>[：:\s]*\s*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"ratio": {
				Selector: []string{
					"td.rowhead:contains('传输') + td",
					"td.rowhead:contains('傳輸') + td",
					"td.rowhead:contains('分享率') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`分享率</strong>[：:\s]*\s*(?:<font[^>]*>)?([\d.,]+|∞|无限|Inf)`}},
					{Name: "parseNumber"},
				},
			},
			"levelName": {
				Selector: []string{
					"table.main td.rowhead:contains('等级') + td img",
					"td.rowhead:contains('等级') + td > img",
					"td.rowhead:contains('Class') + td > img",
				},
				Attr: "title",
			},
			"joinTime": {
				Selector: []string{
					"td.rowhead:contains('加入日期') + td span[title]",
					"td.rowhead:contains('加入日期') + td",
				},
				Attr: "title",
				Filters: []v2.Filter{
					{Name: "split", Args: []any{" (", 0}},
					{Name: "parseTime"},
				},
			},
			// 魔力值 lives in logged-in user's #info_block, or (on own profile) a separate row
			"bonus": {
				Selector: []string{
					"td.rowhead:contains('魔力值') + td",
					"#info_block",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`魔力值[^:：]*[:：]\s*(?:</?[^>]+>)*\s*([\d.,]+)`}},
					{Name: "parseNumber"},
				},
			},
			"seeding": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`class="arrowup"[^>]*/?>\s*(\d+)`}},
					{Name: "parseNumber"},
				},
			},
			"leeching": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`class="arrowdown"[^>]*/?>\s*(\d+)`}},
					{Name: "parseNumber"},
				},
			},
		},
	},
	Selectors: &v2.SiteSelectors{
		// Standard NexusPHP row structure with nested torrentname
		TableRows: "table.torrents > tbody > tr:has(table.torrentname), table.torrents > tr:has(table.torrentname)",
		Title:     "table.torrentname a[href*='details.php']",
		TitleLink: "table.torrentname a[href*='details.php']",
		Subtitle:  "table.torrentname td.embedded",
		// 9-column table: 1=category 2=title 3=comments 4=time 5=size 6=seeders 7=leechers 8=snatched 9=uploader
		Category:           "td:nth-child(1) a[href*='?cat=']",
		UploadTime:         "td:nth-child(4) span[title]",
		Size:               "td:nth-child(5)",
		Seeders:            "td:nth-child(6)",
		Leechers:           "td:nth-child(7)",
		Snatched:           "td:nth-child(8)",
		DiscountIcon:       "table.torrentname img.pro_free, table.torrentname img.pro_free2up, table.torrentname img.pro_2up, table.torrentname img.pro_50pctdown, table.torrentname img.pro_50pctdown2up, table.torrentname img.pro_30pctdown, table.torrentname img.pro_twoupfree, table.torrentname img.pro_twouphalfdown",
		DetailDownloadLink: "td.rowhead:contains('下载') + td a[href*='download.php'], a.index[href*='download.php']",
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
		TitleSelector:    "h1#top, h1",
		DiscountSelector: "h1#top b font[class], h1 b font[class], h1#top font[class], h1 font[class]",
		EndTimeSelector:  "h1#top span[title], h1 span[title]",
		// Size is inline in 基本信息 cell; SizeRegex pulls it from that cell's text.
		// Parser gets .Next() of SizeSelector match, so select the label cell.
		SizeSelector: "td.rowhead:contains('基本信息')",
		SizeRegex:    `大小[：:]\s*(?:</?b>)*\s*([\d.]+)\s*(GB|MB|KB|TB)`,
	},
}

func init() {
	v2.RegisterSiteDefinition(PT0FFCCDefinition)
}
