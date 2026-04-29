package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// OpenCDDefinition is the site definition for OpenCD (open.cd)
// OpenCD uses Traditional Chinese NexusPHP with several template customizations:
//   - Detail page uses <div class="title"> + td.rowtitle (not <h1> + td.rowhead)
//   - Detail links use plugin_details.php (not details.php)
//   - Discount end time is embedded in the onmouseover tooltip of the free icon
var OpenCDDefinition = &v2.SiteDefinition{
	ID:             "opencd",
	Name:           "OpenCD",
	Aka:            []string{"OCD", "皇后"},
	Description:    "OpenCD 音樂 PT 站點",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://open.cd/"},
	FaviconURL:     "https://open.cd/favicon.ico",
	TimezoneOffset: "+0800",
	RateLimit:      0.5,
	RateBurst:      2,
	UserInfo: &v2.UserInfoConfig{
		PickLast:     []string{"id"},
		RequestDelay: 500,
		Process: []v2.UserInfoProcess{
			{
				RequestConfig: v2.RequestConfig{URL: "/index.php", ResponseType: "document"},
				Fields:        []string{"id", "name", "seeding", "leeching"},
			},
			{
				RequestConfig: v2.RequestConfig{URL: "/userdetails.php", ResponseType: "document"},
				Assertion:     map[string]string{"id": "params.id"},
				Fields: []string{
					"name", "uploaded", "downloaded", "ratio", "levelName",
					"bonus", "joinTime",
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
					"a[href*='userdetails.php'][class*='Name'] b",
					"a[href*='userdetails.php'][class*='Name']",
					"#info_block a[href*='userdetails.php']",
				},
			},
			// OpenCD uses "傳送" row on userdetails.php containing uploaded/downloaded/ratio
			// (same combined-row pattern as HDSky's "传输" row)
			"uploaded": {
				Selector: []string{
					"td.rowhead:contains('傳送') + td",
					"td.rowhead:contains('传送') + td",
					"td.rowhead:contains('傳輸') + td",
					"td.rowhead:contains('传输') + td",
					"td.rowhead:contains('上傳量') + td",
					"td.rowhead:contains('上传量') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`上[傳传][量]?</strong>[：:\s]*\s*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"downloaded": {
				Selector: []string{
					"td.rowhead:contains('傳送') + td",
					"td.rowhead:contains('传送') + td",
					"td.rowhead:contains('傳輸') + td",
					"td.rowhead:contains('传输') + td",
					"td.rowhead:contains('下載量') + td",
					"td.rowhead:contains('下载量') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`下[載载][量]?</strong>[：:\s]*\s*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			"ratio": {
				Selector: []string{
					"td.rowhead:contains('傳送') + td",
					"td.rowhead:contains('传送') + td",
					"td.rowhead:contains('分享率') + td",
				},
				Attr: "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`分享率</strong>[：:\s]*\s*(?:<font[^>]*>)?([\d.,]+|∞|Inf)`}},
					{Name: "parseNumber"},
				},
			},
			"levelName": {
				Selector: []string{
					"td.rowhead:contains('等級') + td > img",
					"td.rowhead:contains('等级') + td > img",
					"td.rowhead:contains('Class') + td > img",
					"td.rowhead:contains('等級') + td img",
					"td.rowhead:contains('等级') + td img",
				},
				Attr: "title",
			},
			"bonus": {
				Selector: []string{
					"td.rowhead:contains('魔力值') + td",
					"td.rowhead:contains('魔力') + td",
					"td.rowhead:contains('Bonus') + td",
				},
				Filters: []v2.Filter{{Name: "parseNumber"}},
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
		},
	},
	Selectors: &v2.SiteSelectors{
		// Standard NexusPHP row structure
		TableRows: "table.torrents > tbody > tr:has(table.torrentname), table.torrents > tr:has(table.torrentname)",
		// OpenCD uses plugin_details.php (not details.php)
		Title:     "table.torrentname a[href*='plugin_details.php'], table.torrentname a[href*='details.php']",
		TitleLink: "table.torrentname a[href*='plugin_details.php'], table.torrentname a[href*='details.php']",
		// Subtitle is a grey <font color='#888888'> inside the embedded td
		Subtitle: "table.torrentname td.embedded font[color='#888888']",
		// 11-column search table: 1=type 2=cover 3=title 4=log 5=comments 6=time 7=size 8=seeders 9=leechers 10=snatched 11=uploader
		Category:   "td:nth-child(1) img[alt]",
		UploadTime: "td:nth-child(6) span[title]",
		Size:       "td:nth-child(7)",
		Seeders:    "td:nth-child(8)",
		Leechers:   "td:nth-child(9)",
		Snatched:   "td:nth-child(10)",
		// Discount icon lives inside table.torrentname (alongside span[title] with end time)
		DiscountIcon:    "table.torrentname img.pro_free, table.torrentname img.pro_free2up, table.torrentname img.pro_50pctdown, table.torrentname img.pro_50pctdown2up, table.torrentname img.pro_30pctdown, table.torrentname img.pro_2up",
		DiscountEndTime: "table.torrentname span[title]",
		// Detail page uses td.rowtitle (NOT rowhead) — support both for robustness
		DetailDownloadLink: "td.rowtitle:contains('下載鏈接') + td a[href*='download.php'], td.rowhead:contains('下载链接') + td a[href*='download.php'], td.rowtitle:contains('連結') + td a[href*='download.php']",
		DetailSubtitle:     "div.smalltitle, td.rowtitle:contains('副標題') + td, td.rowhead:contains('副标题') + td",
	},
	DetailParser: &v2.DetailParserConfig{
		TimeLayout: "2006-01-02 15:04:05",
		DiscountMapping: map[string]v2.DiscountLevel{
			"free":             v2.DiscountFree,
			"twoup":            v2.Discount2xUp,
			"twoupfree":        v2.Discount2xFree,
			"thirtypercent":    v2.DiscountPercent30,
			"halfdown":         v2.DiscountPercent50,
			"twouphalfdown":    v2.Discount2x50,
			"pro_free":         v2.DiscountFree,
			"pro_free2up":      v2.Discount2xFree,
			"pro_2up":          v2.Discount2xUp,
			"pro_50pctdown":    v2.DiscountPercent50,
			"pro_50pctdown2up": v2.Discount2x50,
			"pro_30pctdown":    v2.DiscountPercent30,
		},
		HRKeywords: []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run", "H&R"},
		// OpenCD uses div.title instead of <h1>
		TitleSelector: "div.title, h1",
		// Discount icon on detail page is inside div.title
		DiscountSelector: "div.title img[class*='pro_'], div.title font[class], h1 font[class]",
		// Discount end time: OpenCD embeds it in the img.pro_* onmouseover attribute
		// (the driver's parseDiscountEndTimeFromOnmouseover fallback handles this)
		EndTimeSelector: "div.title span[title], h1 span[title]",
		SizeSelector:    "td.rowtitle:contains('大小') + td, td.rowhead:contains('大小') + td, td.rowfollow:contains('大小')",
		SizeRegex:       `大小[：:]\s*([\d.]+)\s*(GB|MB|KB|TB)`,
	},
}

func init() {
	v2.RegisterSiteDefinition(OpenCDDefinition)
}
