package definitions

import (
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// PigGoDefinition is the site definition for PigGo (piggo.me, 猪猪), issue #532.
//
// Standard NexusPHP with three site-specific quirks confirmed against the issue's
// collected HTML:
//
//  1. The real subtitle is a bare text node at the end of the title cell, placed AFTER
//     several coloured tag badges (<span style="background-color:...">完结/国语/中字</span>).
//     A plain `span` selector only yields the last badge, so subtitle extraction goes
//     through SubtitleSelector with an HTML regex that takes whatever follows the final
//     </span>.
//  2. Both the search row and the detail h1 carry a second title-bearing span for the
//     review status (<span style="margin-left: 6px" title="通过">). The discount end time
//     is the span WITHOUT a style attribute, hence the `:not([style])` guards — a bare
//     `span[title]` would parse "通过" as a timestamp on torrents that have no promotion.
//  3. User stats live in the #info_block table (a <table>, not a <div>) on index.php,
//     in the same layout family as ptchdbits, but this site has no 做种积分 field.
//
// Not covered by the issue's attachment, therefore intentionally left at defaults:
// free-torrent icons (the capture only contained 50% promotions), H&R markers (the site
// does have H&R per #info_block, but no marked torrent was captured), and level
// requirements. See the follow-up items on issue #532.
var PigGoDefinition = &v2.SiteDefinition{
	ID:             "piggo",
	Name:           "PigGo",
	Aka:            []string{"猪猪"},
	Description:    "猪猪，综合性 PT 站点",
	Schema:         v2.SchemaNexusPHP,
	URLs:           []string{"https://piggo.me/"},
	FaviconURL:     "https://piggo.me/favicon.ico",
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
				Fields:        []string{"id", "name", "uploaded", "downloaded", "ratio", "bonus", "seeding", "leeching"},
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
			// <font class='color_uploaded'>上传量:</font> 1.045 TB
			"uploaded": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`上传量[:：]\s*</font>\s*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			// <font class='color_downloaded'> 下载量:</font> 543.38 GB
			"downloaded": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`下载量[:：]\s*</font>\s*([\d.,]+\s*[KMGTP]?i?B)`}},
					{Name: "parseSize"},
				},
			},
			// <font class="color_ratio">分享率:</font> 1.969
			"ratio": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`分享率[:：]\s*</font>\s*(?:<font[^>]*>)?([\d.,]+|∞|Inf)`}},
					{Name: "parseNumber"},
				},
			},
			// <font class='color_bonus'>魔力值 </font>[<a href="mybonus.php">使用</a>]: 1,351,847.4
			"bonus": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`魔力值\s*</font>\s*\[[^\]]*\]\s*[:：]\s*([\d.,]+)`}},
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
					{Name: "regex", Args: []any{`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`}},
					{Name: "parseTime"},
				},
			},
			"lastAccessAt": {
				Selector: []string{
					"td.rowhead:contains('最近动向') + td",
					"td.rowhead:contains('最近動向') + td",
					"td.rowhead:contains('上次访问') + td",
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
		// Subtitle is deliberately empty: the driver prefers Selectors.Subtitle over
		// SubtitleSelector, and a CSS-only selector cannot reach the bare text node.
		SubtitleSelector: &v2.FieldSelector{
			Selector: []string{"table.torrentname td.embedded:has(a[href*='details.php'])"},
			Attr:     "html",
			Filters: []v2.Filter{
				{Name: "regex", Args: []any{`(?s)</span>\s*([^<]+?)\s*$`}},
			},
		},
		Size:               "td.rowfollow:nth-child(5)",
		Seeders:            "td.rowfollow:nth-child(6)",
		Leechers:           "td.rowfollow:nth-child(7)",
		Snatched:           "td.rowfollow:nth-child(8)",
		DiscountIcon:       "img.pro_free, img.pro_free2up, img.pro_2up, img.pro_50pctdown, img.pro_50pctdown2up, img.pro_30pctdown",
		DiscountEndTime:    "table.torrentname td.embedded > span[title]:not([style])",
		Category:           "td.rowfollow:nth-child(1) img[alt]",
		UploadTime:         "td.rowfollow:nth-child(4) span[title]",
		DetailDownloadLink: "td.rowhead:contains('下载') + td a[href*='download.php']",
		DetailSubtitle:     "td.rowhead:contains('副标题') + td",
	},
	DetailParser: &v2.DetailParserConfig{
		TimeLayout: "2006-01-02 15:04:05",
		DiscountMapping: map[string]v2.DiscountLevel{
			// detail h1 uses <font class='halfdown'>50%</font>
			"free":          v2.DiscountFree,
			"twoup":         v2.Discount2xUp,
			"twoupfree":     v2.Discount2xFree,
			"thirtypercent": v2.DiscountPercent30,
			"halfdown":      v2.DiscountPercent50,
			"twouphalfdown": v2.Discount2x50,
			// search rows use img.pro_* classes
			"pro_free":         v2.DiscountFree,
			"pro_free2up":      v2.Discount2xFree,
			"pro_2up":          v2.Discount2xUp,
			"pro_50pctdown":    v2.DiscountPercent50,
			"pro_50pctdown2up": v2.Discount2x50,
			"pro_30pctdown":    v2.DiscountPercent30,
		},
		HRKeywords:    []string{"hitandrun", "hit_run.gif", "Hit and Run", "Hit & Run"},
		TitleSelector: "input[name='torrent_name']",
		IDSelector:    "input[name='detail_torrent_id']",
		// h1 carries the promotion font and, separately, a styled review-status span.
		DiscountSelector: "h1 font[class], h1 img[class*='pro_']",
		EndTimeSelector:  "h1 span[title]:not([style])",
		SizeSelector:     "td.rowhead:contains('基本信息')",
		SizeRegex:        `大小[：:]\s*([\d.]+)\s*(GB|MB|KB|TB)`,
	},
}

func init() {
	v2.RegisterSiteDefinition(PigGoDefinition)
}
