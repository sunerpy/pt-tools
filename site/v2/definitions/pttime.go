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
				Fields:        []string{"joinTime"},
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
			// Level embedded as text: "[(初中)Elite User]" — capture the Chinese tier name
			"levelName": {
				Selector: []string{"#info_block"},
				Attr:     "html",
				Filters: []v2.Filter{
					{Name: "regex", Args: []any{`\(([^)]+)\)\s*[A-Za-z][A-Za-z ]+User`}},
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
		// PTT-NP uses <font class="promotion <type>"> instead of <img class="pro_<type>">
		DiscountIcon: "table.torrentname font.promotion, td.embedded font.promotion, table.torrentname img.pro_free, table.torrentname img.pro_free2up, table.torrentname img.pro_50pctdown, table.torrentname img.pro_30pctdown, table.torrentname img.pro_2up, table.torrentname img.pro_50pctdown2up",
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
}

func init() {
	v2.RegisterSiteDefinition(PTTimeDefinition)
}
